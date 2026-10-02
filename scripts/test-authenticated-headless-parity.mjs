// Operator-only authenticated X/Facebook parity QA. Default is read-only preflight.
import { spawn } from 'node:child_process';
import { execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, readFile, realpath, stat, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { randomUUID } from 'node:crypto';
import { canonicalSourceURL, evidenceKey } from '../internal/collection/headless/worker/observation.mjs';
import { compareReport } from './authenticated-parity-comparison.mjs';
import { startNativeObserver, summarizeNativeVisibility } from './headless-native-observer.mjs';

const execFile = promisify(execFileCallback);
const sidecar = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const workspace = dirname(sidecar);
const browserRepo = join(workspace, 'AkuBrowser');
const supervisorExe = join(workspace, 'AkuSupervisor', 'target', 'dev', 'aku-supervisor.exe');
const pause = milliseconds => new Promise(resolveDelay => setTimeout(resolveDelay, milliseconds));
const MAX_INTERRUPTION_MS = 6 * 60_000;
const RESTORE_RESERVE_MS = 90_000;
const SOURCES = ['x', 'facebook'];

class HarnessError extends Error {
  constructor(code, message = code) {
    super(message);
    this.code = code;
  }
}

function parseArguments(values) {
  let artifact = null;
  let baseline = null;
  let allowRuntimeStop = false;
  let diagnosticWorker = null;
  let targetsOnly = false;
  let selectedSource = null;
  let nativeObserver = null;
  for (let index = 0; index < values.length; index++) {
    const value = values[index];
    if (value === '--targets-only') {
      if (targetsOnly) throw new HarnessError('invalid_arguments');
      targetsOnly = true;
    } else if (value === '--source') {
      if (selectedSource || !SOURCES.includes(values[index + 1])) throw new HarnessError('invalid_arguments');
      selectedSource = values[++index];
    } else if (value === '--native-observer') {
      if (nativeObserver || !isAbsolute(values[index + 1] || '')) throw new HarnessError('invalid_arguments');
      nativeObserver = values[++index];
    } else if (value === '--diagnostic-worker') {
      if (diagnosticWorker || !isAbsolute(values[index + 1] || '')) throw new HarnessError('invalid_arguments');
      diagnosticWorker = values[++index];
    } else if (value === '--allow-runtime-stop') {
      if (allowRuntimeStop) throw new HarnessError('invalid_arguments');
      allowRuntimeStop = true;
    } else if (value === '--artifact' || value === '--baseline') {
      const path = values[++index];
      if (!path || !isAbsolute(path)) throw new HarnessError('invalid_arguments');
      if (value === '--artifact') {
        if (artifact) throw new HarnessError('invalid_arguments');
        artifact = path;
      } else {
        if (baseline) throw new HarnessError('invalid_arguments');
        baseline = path;
      }
    } else {
      throw new HarnessError('invalid_arguments');
    }
  }
  if (!artifact || !baseline) throw new HarnessError('invalid_arguments');
  return {artifact, baseline, allowRuntimeStop, diagnosticWorker, targetsOnly, selectedSource, nativeObserver};
}

function isInside(parent, child, allowEqual = false) {
  const childPath = relative(parent, child);
  if (!childPath) return allowEqual;
  return childPath !== '..' && !childPath.startsWith(`..${sep}`) && !isAbsolute(childPath);
}

function validatePlatformId(source, value) {
  if (typeof value !== 'string' || value.length > 128) return false;
  if (source === 'x') return /^x:status:\d+$/.test(value);
  return source === 'facebook' && /^facebook:post:(?:pfbid[A-Za-z0-9]+|\d+)$/i.test(value);
}

export function validateBaseline(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value) || !Array.isArray(value.targets)) {
    throw new HarnessError('invalid_baseline');
  }
  const counts = {x: 0, facebook: 0};
  const ids = new Map();
  const links = new Map();
  for (const target of value.targets) {
    if (!target || typeof target !== 'object' || Array.isArray(target)
        || !SOURCES.includes(target.source) || !validatePlatformId(target.source, target.platformId)
        || typeof target.permalink !== 'string' || typeof target.author !== 'string'
        || target.author.length > 1200 || typeof target.text !== 'string'
        || Array.from(target.text).length > 4000
        || (target.media !== undefined && (!Array.isArray(target.media) || target.media.length > 20))
        || (target.observedAt !== undefined && (typeof target.observedAt !== 'string' || !Number.isFinite(Date.parse(target.observedAt))))) {
      throw new HarnessError('invalid_baseline_target');
    }
    const canonical = canonicalSourceURL(target.source, target.permalink);
    if (!canonical || canonical !== target.permalink) throw new HarnessError('invalid_baseline_permalink');
    if (target.source === 'x') {
      const pathId = new URL(canonical).pathname.match(/^\/[^/]+\/status\/(\d+)$/)?.[1];
      if (!pathId || target.platformId !== `x:status:${pathId}`) throw new HarnessError('baseline_identity_permalink_mismatch');
    }
    // Exercise the packaged evidence-key contract after source, ID, and permalink checks.
    const key = evidenceKey(target.source, target.platformId, canonical, target.author, target.text);
    if (!key) throw new HarnessError('invalid_baseline_identity');
    const idKey = `${target.source}\0${target.platformId}`;
    const priorLink = ids.get(idKey);
    if (priorLink && priorLink !== canonical) throw new HarnessError('conflicting_baseline_identity');
    const linkKey = `${target.source}\0${canonical}`;
    const priorId = links.get(linkKey);
    if (priorId && priorId !== target.platformId) throw new HarnessError('conflicting_baseline_permalink');
    ids.set(idKey, canonical);
    links.set(linkKey, target.platformId);
    counts[target.source]++;
    if (counts[target.source] > 2) throw new HarnessError('baseline_target_limit');
  }
  if (value.targets.length !== 4 || counts.x !== 2 || counts.facebook !== 2) {
    throw new HarnessError('baseline_requires_two_targets_per_source');
  }
  return value;
}

export function continuationFor(source, result) {
  const frontier = result?.coverage?.frontier;
  if (!frontier || frontier.hasMoreCandidateSignal !== true
      || !Number.isSafeInteger(frontier.scrollY) || frontier.scrollY < 0
      || !Array.isArray(frontier.anchorKeys) || frontier.anchorKeys.length < 1 || frontier.anchorKeys.length > 3
      || frontier.anchorKeys.some(anchor => !validatePlatformId(source, anchor))) return null;
  return {startScrollY: frontier.scrollY, settleMs: 900, anchorKeys: frontier.anchorKeys.slice()};
}

async function containedRealpath(root, path, label, allowEqual = false) {
  const resolvedRoot = await realpath(root);
  const resolvedPath = await realpath(path);
  if (!isInside(resolvedRoot, resolvedPath, allowEqual)) throw new HarnessError(`path_outside_${label}`);
  return resolvedPath;
}

async function loadCandidate(artifactArgument) {
  const allowedRoot = await realpath(join(browserRepo, 'build'));
  const artifactRoot = await containedRealpath(allowedRoot, artifactArgument, 'candidate');
  const pointer = JSON.parse(await readFile(join(artifactRoot, 'runtime', 'current.json'), 'utf8'));
  if (!pointer || !/^[A-Za-z0-9._-]+$/.test(pointer.version)
      || pointer.manifestPath !== `runtime/versions/${pointer.version}/manifest.json`) {
    throw new HarnessError('invalid_candidate_pointer');
  }
  const versionRoot = await containedRealpath(artifactRoot, join(artifactRoot, 'runtime', 'versions', pointer.version), 'candidate_version');
  const workerRoot = join(versionRoot, 'headless-worker');
  const nodeExe = await containedRealpath(versionRoot, join(workerRoot, 'node.exe'), 'candidate_node');
  const worker = await containedRealpath(versionRoot, join(workerRoot, 'worker.mjs'), 'candidate_worker');
  const bridgePath = await containedRealpath(versionRoot, join(versionRoot, 'AkuBridge'), 'candidate_bridge');
  const [nodeInfo, workerInfo, bridgeInfo] = await Promise.all([stat(nodeExe), stat(worker), stat(bridgePath)]);
  if (!nodeInfo.isFile() || !workerInfo.isFile() || !bridgeInfo.isDirectory()) throw new HarnessError('candidate_runtime_incomplete');
  return {artifactRoot, versionRoot, nodeExe, worker, bridgePath};
}

function registrationArgument(registration, flag) {
  const index = registration.args.indexOf(flag);
  const value = registration.args[index + 1];
  if (index < 0 || !value || !isAbsolute(value)) throw new HarnessError('registration_missing_argument');
  return value;
}

async function loadRegistration() {
  if (!process.env.LOCALAPPDATA) throw new HarnessError('local_app_data_unavailable');
  const configPath = join(process.env.LOCALAPPDATA, 'AkuSupervisor', 'services.json');
  const config = JSON.parse(await readFile(configPath, 'utf8'));
  const registration = config.services?.akusidecar;
  if (!registration || !Array.isArray(registration.args)) throw new HarnessError('registered_service_missing');
  const profile = await realpath(registrationArgument(registration, '--browser-profile'));
  const captureExe = await realpath(registrationArgument(registration, '--chromium-path'));
  const runtimeRoot = await realpath(join(sidecar, 'runtime'));
  if (!isInside(runtimeRoot, profile) || profile.toLowerCase().endsWith('-ui-split-cft')) {
    throw new HarnessError('registered_profile_outside_runtime');
  }
  if (!(await stat(captureExe)).isFile()) throw new HarnessError('registered_chrome_unavailable');
  const localState = JSON.parse(await readFile(join(profile, 'Local State'), 'utf8'));
  const preferred = localState.profile?.last_used;
  const profileDirectory = typeof preferred === 'string' && /^[A-Za-z0-9 _-]{1,64}$/.test(preferred)
    ? preferred : 'Default';
  let profileInfo;
  try { profileInfo = await stat(join(profile, profileDirectory)); } catch {}
  if (!profileInfo?.isDirectory()) throw new HarnessError('registered_profile_directory_unavailable');
  return {profile, captureExe, profileDirectory};
}

async function supervisor(command) {
  const {stdout} = await execFile(supervisorExe, [...command, '--json', '--config', join(process.env.LOCALAPPDATA, 'AkuSupervisor', 'services.json')], {
    windowsHide: true, timeout: 45_000, maxBuffer: 1024 * 1024,
  });
  return JSON.parse(stdout);
}

async function serviceStatus() {
  const response = (await supervisor(['status'])).response;
  const service = response?.services?.find(value => value.id === 'akusidecar');
  if (!service) throw new HarnessError('supervisor_service_status_missing');
  return service;
}

async function exactProfileOwners(profile) {
  const script = '$values=@(Get-CimInstance Win32_Process -Filter "name = \'chrome.exe\'" | Where-Object { $_.CommandLine -notlike \'*--type=*\' } | ForEach-Object { $m=[regex]::Match($_.CommandLine, \'--user-data-dir=(?:"([^"]+)"|(\\S+))\'); $p=$m.Groups[1].Value+$m.Groups[2].Value; if ($p -and [IO.Path]::GetFullPath($p).TrimEnd(\'\\\') -eq $env:AKU_PARITY_PROFILE.TrimEnd(\'\\\')) { [pscustomobject]@{pid=$_.ProcessId; executable=$_.ExecutablePath} } }); ConvertTo-Json -InputObject $values -Compress';
  const {stdout} = await execFile('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {
    windowsHide: true, timeout: 10_000, env: {...process.env, AKU_PARITY_PROFILE: profile}, maxBuffer: 128 * 1024,
  });
  const parsed = JSON.parse(stdout.trim() || '[]');
  return Array.isArray(parsed) ? parsed : [parsed];
}

async function bridgeHealth() {
  const response = await fetch('http://127.0.0.1:11122/api/bridge/health', {signal: AbortSignal.timeout(5000)});
  if (!response.ok) throw new HarnessError('bridge_health_unavailable');
  const value = (await response.json()).bridge;
  return {state: value?.state || 'unknown', compatible: value?.compatible === true};
}

async function inboxState() {
  const response = await fetch('http://127.0.0.1:11122/api/inbox?limit=100', {signal: AbortSignal.timeout(5000)});
  if (!response.ok) throw new HarnessError('inbox_unavailable');
  const value = await response.json();
  if (!Array.isArray(value.sessions) || value.sessions.length >= 100) throw new HarnessError('inbox_unverifiable');
  const terminal = new Set(['completed', 'partial', 'failed', 'cancelled', 'canceled']);
  if (value.sessions.some(session => !terminal.has(session.status) || !session.completedAt)) throw new HarnessError('inbox_active_or_unverifiable');
  return {sessions: value.sessions.length, activeSessions: 0};
}

async function preflight(registration) {
  const [service, owners, bridge, inbox] = await Promise.all([
    serviceStatus(), exactProfileOwners(registration.profile), bridgeHealth(), inboxState(),
  ]);
  if (service.lifecycle !== 'running' || service.health?.status !== 'healthy'
      || service.operatorHold !== 'none' || !Array.isArray(service.ownedPids) || service.ownedPids.length === 0) {
    throw new HarnessError('registered_supervisor_unhealthy');
  }
  if (owners.length !== 1 || owners[0].executable?.toLowerCase() !== registration.captureExe.toLowerCase()) {
    throw new HarnessError('original_profile_ownership_unhealthy');
  }
  if (!bridge.compatible) throw new HarnessError('bridge_incompatible');
  return {
    service: {lifecycle: service.lifecycle, health: service.health.status, ownedProcessCount: service.ownedPids.length},
    exactProfileOwnerCount: owners.length,
    registeredChromeExecutableMatch: true,
    bridge,
    inbox,
  };
}

async function verifyTuple(artifactRoot) {
  await execFile('pwsh.exe', ['-NoProfile', '-File', join(browserRepo, 'scripts', 'test-windows-installed-app-builder.ps1'), '-ArtifactDirectory', artifactRoot], {
    windowsHide: true, timeout: 60_000, maxBuffer: 1024 * 1024,
  });
}

function workerClient(candidate) {
  const child = spawn(candidate.nodeExe, [candidate.worker], {
    cwd: candidate.versionRoot,
    windowsHide: true,
    stdio: ['pipe', 'pipe', 'pipe'],
    env: {...process.env, AKU_PARITY_WORKER_ROOT:dirname(candidate.packagedWorker || candidate.worker)},
  });
  const pending = new Map();
  let textBuffer = '';
  let closed = false;
  let protocolFailure = null;
  let stderrBytes = 0;
  child.stderr.on('data', chunk => { stderrBytes += chunk.length; });
  const closePromise = new Promise(resolveClose => {
    const rejectPending = error => {
      for (const request of pending.values()) {
        clearTimeout(request.timer);
        request.reject(error);
      }
      pending.clear();
    };
    child.on('error', error => {
      protocolFailure = new HarnessError('worker_spawn_failed');
      rejectPending(protocolFailure);
      resolveClose({closed: true, code: null, signal: null});
    });
    child.on('close', (code, signal) => {
      closed = true;
      const error = new HarnessError('worker_exited');
      rejectPending(error);
      resolveClose({closed: true, code, signal});
    });
    child.stdin.on('error', () => {
      protocolFailure = new HarnessError('worker_stdin_failed');
      rejectPending(protocolFailure);
    });
  });
  child.stdout.setEncoding('utf8');
  child.stdout.on('data', chunk => {
    textBuffer += chunk;
    if (Buffer.byteLength(textBuffer, 'utf8') > 9 * 1024 * 1024 && !textBuffer.includes('\n')) {
      protocolFailure = new HarnessError('worker_response_too_large');
      for (const request of pending.values()) {
        clearTimeout(request.timer);
        request.reject(protocolFailure);
      }
      pending.clear();
      child.stdin.end();
      return;
    }
    let newline;
    while ((newline = textBuffer.indexOf('\n')) >= 0) {
      const line = textBuffer.slice(0, newline).replace(/\r$/, '');
      textBuffer = textBuffer.slice(newline + 1);
      if (!line) continue;
      let response;
      try { response = JSON.parse(line); } catch {
        protocolFailure = new HarnessError('worker_protocol_invalid_json');
        for (const request of pending.values()) {
          clearTimeout(request.timer);
          request.reject(protocolFailure);
        }
        pending.clear();
        continue;
      }
      const request = pending.get(response.id);
      if (!request) continue;
      pending.delete(response.id);
      clearTimeout(request.timer);
      request.resolve(response);
    }
  });
  return {
    child,
    closePromise,
    get closed() { return closed; },
    get protocolFailure() { return protocolFailure; },
    get stderrBytes() { return stderrBytes; },
    request(message, timeoutMs) {
      if (closed || protocolFailure) return Promise.reject(protocolFailure || new HarnessError('worker_closed'));
      return new Promise((resolveResponse, rejectResponse) => {
        const timer = setTimeout(() => {
          pending.delete(message.id);
          rejectResponse(new HarnessError('worker_request_timeout'));
        }, timeoutMs);
        pending.set(message.id, {resolve: resolveResponse, reject: rejectResponse, timer});
        child.stdin.write(`${JSON.stringify(message)}\n`, 'utf8', error => {
          if (!error) return;
          const request = pending.get(message.id);
          if (!request) return;
          pending.delete(message.id);
          clearTimeout(timer);
          rejectResponse(new HarnessError('worker_stdin_failed'));
        });
      });
    },
  };
}

async function waitForClosed(client, timeoutMs) {
  let timer;
  try {
    const timeout = new Promise(resolveTimeout => { timer = setTimeout(() => resolveTimeout({closed: false}), timeoutMs); });
    return await Promise.race([client.closePromise, timeout]);
  } finally {
    clearTimeout(timer);
  }
}

function responseErrorCode(response) {
  return typeof response?.error?.code === 'string' ? response.error.code.slice(0, 80) : 'worker_request_failed';
}

function addCapture(report, source, kind, targetPlatformId, responseOrError) {
  if (responseOrError instanceof Error) {
    report.captures.push({source, kind, targetPlatformId, ok: false, result: null,
      error: {code: responseOrError.code || 'harness_error', message: String(responseOrError.message || '').slice(0,500)}});
  } else if (!responseOrError?.ok) {
    report.captures.push({source, kind, targetPlatformId, ok: false, result: null,
      error: {code: responseErrorCode(responseOrError), message: String(responseOrError?.error?.message || '').slice(0,500),
        ...(responseOrError?.error?.diagnostics ? {diagnostics:responseOrError.error.diagnostics} : {})}});
  } else {
    report.captures.push({source, kind, targetPlatformId, ok: true, result: responseOrError.result, error: null});
  }
  return report.captures.at(-1);
}

function makeCapturePayload(kind, target) {
  const readOnly = {pendingContentPolicy: 'detect_only', sourceFreshnessPolicy: 'preserve_frontier', sameTabMutationAllowed: false};
  if (kind === 'feed') return {
    scrolls: 1, maxBlocksPerSnapshot: 20, captureTimeoutMs: 45_000,
    sourceHydrationTimeoutMs: 12_000, restoreScroll: true, ...readOnly,
  };
  if (kind === 'target') return {
    pageUrl: target.permalink, scrolls: 0, maxBlocksPerSnapshot: 20, captureTimeoutMs: 45_000,
    sourceHydrationTimeoutMs: 12_000, restoreScroll: true, ...readOnly,
  };
  throw new HarnessError('unsupported_capture_kind');
}

async function captureRequest(client, report, source, kind, target, runtimeWorkDeadline) {
  const targetPlatformId = kind === 'target' ? target.platformId : null;
  if (Date.now() + 55_000 > runtimeWorkDeadline) {
    return addCapture(report, source, kind, targetPlatformId, new HarnessError('runtime_window_exhausted'));
  }
  const id = `capture-${report.captures.length + 1}`;
  try {
    const response = await client.request({id, type: 'capture', source, payload: makeCapturePayload(kind, target)}, 53_000);
    return addCapture(report, source, kind, targetPlatformId, response);
  } catch (error) {
    return addCapture(report, source, kind, targetPlatformId, error);
  }
}

async function restoreHealthy(registration) {
  const [service, owners, bridge] = await Promise.all([
    serviceStatus(), exactProfileOwners(registration.profile), bridgeHealth(),
  ]);
  const healthy = service.lifecycle === 'running' && service.health?.status === 'healthy'
    && service.operatorHold === 'none' && Array.isArray(service.ownedPids) && service.ownedPids.length > 0
    && owners.length === 1 && owners[0].executable?.toLowerCase() === registration.captureExe.toLowerCase()
    && bridge.compatible === true;
  return {
    healthy,
    serviceRunning: service.lifecycle === 'running' && service.health?.status === 'healthy',
    exactProfileOwnerCount: owners.length,
    registeredChromeExecutableMatch: owners.length === 1 && owners[0].executable?.toLowerCase() === registration.captureExe.toLowerCase(),
    bridge,
  };
}

async function waitForStopped(profile) {
  const deadline = Date.now() + 15_000;
  while (Date.now() < deadline) {
    const service = await serviceStatus();
    const owners = await exactProfileOwners(profile);
    if (service.lifecycle === 'stopped' && Array.isArray(service.ownedPids) && service.ownedPids.length === 0 && owners.length === 0) {
      return {stopped: true, ownedProcessCount: 0, exactProfileOwnerCount: 0};
    }
    await pause(300);
  }
  return {stopped: false};
}

async function closeWorker(client, runtimeDeadline) {
  if (!client) return {closed: true, explicitShutdown: false, exitCode: null};
  let explicitShutdown = false;
  if (!client.closed) {
    const shutdownBudget = Math.max(1_000, Math.min(15_000, runtimeDeadline - Date.now()));
    try {
      const response = await client.request({id: 'shutdown', type: 'shutdown'}, shutdownBudget);
      explicitShutdown = response?.ok === true && response.result?.stopped === true;
    } catch {}
    client.child.stdin.end();
  }
  const result = await waitForClosed(client, Math.max(0, Math.min(25_000, runtimeDeadline - Date.now())));
  return {closed: result.closed, explicitShutdown, exitCode: result.code ?? null};
}

async function waitForNoProfileOwners(profile, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const owners = await exactProfileOwners(profile);
    if (owners.length === 0) return {clear: true, exactProfileOwnerCount: 0};
    await pause(300);
  }
  const owners = await exactProfileOwners(profile);
  return {clear: false, exactProfileOwnerCount: owners.length};
}

async function restoreOriginal(registration, stopIssued, runtimeDeadline) {
  if (!stopIssued) return {restored: false, blocked: false, reason: 'runtime_stop_not_issued'};
  try {
    const current = await serviceStatus();
    const owners = await exactProfileOwners(registration.profile);
    if (owners.length > 0) {
      if (owners.length !== 1 || owners[0].executable?.toLowerCase() !== registration.captureExe.toLowerCase()
          || current.lifecycle !== 'running' || current.health?.status !== 'healthy' || current.operatorHold !== 'none') {
        return {restored: false, blocked: true, reason: 'exact_profile_owner_remains', verified: {
          serviceRunning: current.lifecycle === 'running' && current.health?.status === 'healthy',
          exactProfileOwnerCount: owners.length,
          registeredChromeExecutableMatch: owners.length === 1 && owners[0].executable?.toLowerCase() === registration.captureExe.toLowerCase(),
        }};
      }
      try {
        const bridge = await bridgeHealth();
        if (bridge.compatible) return {restored: true, blocked: false, verified: {
          serviceRunning: true, exactProfileOwnerCount: 1, registeredChromeExecutableMatch: true, bridge,
        }};
      } catch {}
      return {restored: false, blocked: true, reason: 'profile_owner_present_bridge_unverified'};
    }
    // In the stopped/zero-owner state, Bridge health must not prevent Supervisor start.
    await supervisor(['start', 'akusidecar', '--actor', 'codex', '--reason', 'restore original runtime after authenticated parity QA', '--request-id', randomUUID()]);
    const deadline = Math.min(runtimeDeadline, Date.now() + 40_000);
    while (Date.now() < deadline) {
      try {
        const verified = await restoreHealthy(registration);
        if (verified.healthy) return {restored: true, blocked: false, verified};
      } catch {}
      await pause(500);
    }
    const final = await restoreHealthy(registration).catch(() => null);
    return {restored: false, blocked: Boolean(final?.exactProfileOwnerCount), reason: final?.exactProfileOwnerCount ? 'exact_profile_owner_remains' : 'restore_verification_timeout', verified: final};
  } catch {
    return {restored: false, blocked: false, reason: 'restore_verification_failed'};
  }
}

function publicSummary(summary) {
  return {
    scope: summary.scope,
    fullParityVerified: summary.fullParityVerified,
    sources: summary.sources.map(({source, captureCount, successfulCaptures, failureCodes, uniqueNativeIds, observedBlocks, mediaItems}) => ({
      source, captureCount, successfulCaptures, failureCodes, uniqueNativeIds, observedBlocks, mediaItems,
    })),
  };
}

async function main() {
  if (process.platform !== 'win32') throw new HarnessError('windows_required');
  const args = parseArguments(process.argv.slice(2));
  const baselinePath = await containedRealpath(join(sidecar, 'build'), args.baseline, 'baseline');
  const baseline = validateBaseline(JSON.parse(await readFile(baselinePath, 'utf8')));
  const candidate = await loadCandidate(args.artifact);
  const observerExe = args.nativeObserver ? await containedRealpath(join(sidecar,'build'),args.nativeObserver,'native_observer') : null;
  if (observerExe && !(await stat(observerExe)).isFile()) throw new HarnessError('invalid_native_observer');
  if (args.diagnosticWorker) {
    candidate.packagedWorker = candidate.worker;
    candidate.worker = await containedRealpath(join(sidecar,'build'),args.diagnosticWorker,'diagnostic_worker');
    const info = await stat(candidate.worker);
    if (!info.isFile() || info.size > 256 * 1024 || !candidate.worker.endsWith('.mjs')) throw new HarnessError('invalid_diagnostic_worker');
  }
  const selectedSources = args.selectedSource ? [args.selectedSource] : SOURCES;
  const feedSources = args.targetsOnly ? [] : selectedSources;
  const registration = await loadRegistration();
  const before = await preflight(registration);
  if (!args.allowRuntimeStop) {
    process.stdout.write(`${JSON.stringify({status: 'preflight_only', requiresExplicitRuntimeStop: true,
      sourceTargets: {x: 2, facebook: 2}, preflight: before})}\n`);
    return;
  }

  await verifyTuple(candidate.artifactRoot);
  const secondPreflight = await preflight(registration);
  const receiptRoot = join(sidecar, 'build', `authenticated-parity-${randomUUID()}`);
  await mkdir(receiptRoot, {recursive: false});
  const report = {
    schema: 'aku.authenticated-parity-report.v1',
    baseline,
    captures: [],
    workerIdentity: null,
    execution: {
      scope: 'saved_timeline_vs_live_headless_sequential',
      workerMode:args.diagnosticWorker ? 'instrumented_diagnostic' : 'packaged_worker',
      selectedSources, targetsOnly:args.targetsOnly,
      runtimeControl: {
        before, secondPreflight, stopIssued: false, stoppedConfirmed: false,
        workerStarted: false, workerExitConfirmed: null, restored: false, restoreBlocked: false,
      },
      followupEligibility: [],
      targetRecaptures: [],
      startedAt: new Date().toISOString(),
    },
  };
  let operationError = null;
  let client = null;
  let nativeObserver = null;
  let runtimeDeadline = 0;
  try {
    runtimeDeadline = Date.now() + MAX_INTERRUPTION_MS;
    report.execution.runtimeControl.stopIssued = true;
    await supervisor(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'authorized authenticated read-only parity QA', '--request-id', randomUUID()]);
    const stopped = await waitForStopped(registration.profile);
    report.execution.runtimeControl.stoppedConfirmed = stopped.stopped;
    if (!stopped.stopped) throw new HarnessError('registered_service_stop_unconfirmed');
    if (observerExe) nativeObserver = await startNativeObserver(observerExe);
    const runtimeWorkDeadline = runtimeDeadline - RESTORE_RESERVE_MS;
    client = workerClient(candidate);
    report.execution.runtimeControl.workerStarted = true;
    const init = await client.request({id: 'init', type: 'init', chrome: registration.captureExe,
      profile: registration.profile, profileDirectory: registration.profileDirectory, bridgePath: candidate.bridgePath}, 45_000);
    if (!init?.ok || !init.result) throw new HarnessError(responseErrorCode(init));
    report.workerIdentity = init.result;

    const feedResults = new Map();
    for (const source of feedSources) {
      const capture = await captureRequest(client, report, source, 'feed', null, runtimeWorkDeadline);
      feedResults.set(source, capture);
    }
    for (const source of feedSources) {
      const feed = feedResults.get(source);
      const continuation = feed?.ok ? continuationFor(source, feed.result) : null;
      const decision = {source, eligible: Boolean(continuation), reason: continuation ? 'valid_frontier_with_more_candidates' : 'no_valid_continuation'};
      report.execution.followupEligibility.push(decision);
      if (!continuation) continue;
      if (Date.now() + 55_000 > runtimeWorkDeadline) {
        decision.reason = 'runtime_window_exhausted';
        continue;
      }
      const id = `capture-${report.captures.length + 1}`;
      try {
        const response = await client.request({id, type: 'capture', source,
          payload: {acquisitionRound: 2, continuation, scrolls: 1, maxBlocksPerSnapshot: 20,
            captureTimeoutMs: 45_000, sourceHydrationTimeoutMs: 12_000, restoreScroll: true,
            pendingContentPolicy: 'detect_only', sourceFreshnessPolicy: 'preserve_frontier', sameTabMutationAllowed: false}}, 53_000);
        addCapture(report, source, 'followup', null, response);
      } catch (error) {
        addCapture(report, source, 'followup', null, error);
      }
    }
    for (const source of selectedSources) {
      const target = baseline.targets.find(value => value.source === source);
      const capture = await captureRequest(client, report, source, 'target', target, runtimeWorkDeadline);
      report.execution.targetRecaptures.push({source, attemptedPlatformId: target.platformId, ok: capture.ok,
        matchedCopies: capture.ok ? (capture.result?.snapshots || []).flatMap(snapshot => snapshot.blocks || []).filter(block => block.platformId === target.platformId).length : null});
    }
  } catch (error) {
    operationError = new HarnessError(error?.code || 'harness_error');
  } finally {
    if (client) {
      try {
        const closure = await closeWorker(client, runtimeDeadline || (Date.now() + 30_000));
        report.execution.runtimeControl.workerExitConfirmed = closure.closed;
        report.execution.runtimeControl.workerExplicitShutdown = closure.explicitShutdown;
      } catch {
        report.execution.runtimeControl.workerExitConfirmed = false;
        report.execution.runtimeControl.workerExplicitShutdown = false;
      }
    }
    if (nativeObserver) {
      try {
        const trace = await nativeObserver.close();
        await writeFile(join(receiptRoot,'native-window-trace.json'), JSON.stringify(trace,null,2), 'utf8');
        report.execution.nativeVisibility = summarizeNativeVisibility(trace, report.workerIdentity?.pid);
      } catch {
        report.execution.nativeVisibility = {status:'unavailable',reason:'observer_close_or_receipt_failed'};
      }
    }
    if (report.execution.runtimeControl.stopIssued) {
      let noOwners;
      try {
        noOwners = await waitForNoProfileOwners(registration.profile,
          Math.max(0, Math.min(20_000, (runtimeDeadline || Date.now() + 20_000) - Date.now())));
      } catch {
        noOwners = {clear: false, exactProfileOwnerCount: null};
      }
      report.execution.runtimeControl.profileReleasedBeforeRestore = noOwners.clear;
      const workerExited = !client || report.execution.runtimeControl.workerExitConfirmed !== false;
      let restore;
      if (!workerExited) restore = {restored: false, blocked: true, reason: 'worker_exit_unconfirmed'};
      else if (client && !noOwners.clear) restore = {restored: false, blocked: true, reason: 'exact_profile_owner_remains'};
      else {
        try {
          restore = await restoreOriginal(registration, true, runtimeDeadline || (Date.now() + 45_000));
        } catch {
          restore = {restored: false, blocked: false, reason: 'restore_verification_failed'};
        }
      }
      report.execution.runtimeControl.restored = restore.restored;
      report.execution.runtimeControl.restoreBlocked = restore.blocked;
      report.execution.runtimeControl.restoreVerification = restore.verified || null;
      if (!restore.restored && !operationError) operationError = new HarnessError(restore.blocked ? 'restore_blocked_for_profile_owner' : 'restore_unverified');
    }
    report.execution.finishedAt = new Date().toISOString();
    if (operationError) report.execution.failureCode = operationError.code;
    try {
      await writeFile(join(receiptRoot, 'report.json'), JSON.stringify(report, null, 2), 'utf8');
      const summary = compareReport(report);
      await writeFile(join(receiptRoot, 'summary.json'), JSON.stringify(summary, null, 2), 'utf8');
      process.stdout.write(`${JSON.stringify({status: operationError ? 'failed' : 'complete',
        receipt: relative(sidecar, receiptRoot), summary: publicSummary(summary)})}\n`);
    } catch {
      if (!operationError) operationError = new HarnessError('receipt_write_failed');
    }
  }
  if (operationError) throw operationError;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  void main().catch(error => {
    const code = typeof error?.code === 'string' ? error.code : 'harness_error';
    process.stderr.write(`${JSON.stringify({status: 'failed', code})}\n`);
    process.exitCode = 1;
  });
}
