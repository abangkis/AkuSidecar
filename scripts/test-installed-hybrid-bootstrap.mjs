// Operator-only installed-package bootstrap smoke. Default is a read-only plan.
import { spawn, execFile as execFileCallback } from 'node:child_process';
import { promisify } from 'node:util';
import { createHash, randomUUID } from 'node:crypto';
import { createConnection } from 'node:net';
import { lstat, mkdir, readFile, realpath, stat } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import {
  loadRegistration, preflight, supervisor, verifyTuple, waitForStopped, restoreOriginal,
} from './test-authenticated-headless-parity.mjs';

const execFile = promisify(execFileCallback);
const sidecarRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const workspaceRoot = dirname(sidecarRoot);
const browserRoot = join(workspaceRoot, 'AkuBrowser');
const browserBuildRoot = join(browserRoot, 'build');
const defaultCandidate = join(browserBuildRoot, 'headless-hybrid-collection-20261004-8354db7d', 'AkuBrowser-0.9.0-windows-x64-installed-app');
const pause = milliseconds => new Promise(resolveDelay => setTimeout(resolveDelay, milliseconds));
const MAX_BOOTSTRAP_MS = 120_000;
const CANDIDATE_DRAIN_MS = 20_000;
const HEALTH_ORIGIN = 'http://127.0.0.1';

export class BootstrapError extends Error {
  constructor(code) { super(code); this.code = code; }
}

export function parseArguments(values) {
  let candidate = defaultCandidate;
  let candidateSeen = false;
  let allowRuntimeStop = false;
  let allowForeground = false;
  for (let index = 0; index < values.length; index++) {
    const value = values[index];
    if (value === '--candidate') {
      if (candidateSeen || !isAbsolute(values[index + 1] || '')) throw new BootstrapError('invalid_arguments');
      candidate = values[++index];
      candidateSeen = true;
    } else if (value === '--allow-runtime-stop') {
      if (allowRuntimeStop) throw new BootstrapError('invalid_arguments');
      allowRuntimeStop = true;
    } else if (value === '--allow-foreground') {
      if (allowForeground) throw new BootstrapError('invalid_arguments');
      allowForeground = true;
    } else {
      throw new BootstrapError('invalid_arguments');
    }
  }
  if (allowRuntimeStop !== allowForeground) throw new BootstrapError('both_runtime_and_foreground_approval_required');
  return {candidate, allowRuntimeStop, allowForeground};
}

function isInside(parent, child, allowEqual = false) {
  const childPath = relative(parent, child);
  if (!childPath) return allowEqual;
  return childPath !== '..' && !childPath.startsWith(`..${sep}`) && !isAbsolute(childPath);
}

function samePath(left, right) {
  return resolve(left).toLowerCase() === resolve(right).toLowerCase();
}

function readArgument(registration, flag) {
  const indices = registration.args.flatMap((value, index) => value === flag ? [index] : []);
  const value = registration.args[indices[0] + 1];
  if (indices.length !== 1 || !value) throw new BootstrapError('registration_argument_unavailable');
  return value;
}

async function loadRegisteredBridgeOrigin() {
  if (!process.env.LOCALAPPDATA) throw new BootstrapError('local_app_data_unavailable');
  const configPath = join(process.env.LOCALAPPDATA, 'AkuSupervisor', 'services.json');
  const config = JSON.parse(await readFile(configPath, 'utf8'));
  const registration = config.services?.akusidecar;
  if (!registration || !Array.isArray(registration.args)) throw new BootstrapError('registered_service_missing');
  const origin = readArgument(registration, '--bridge-extension-origin');
  if (!/^chrome-extension:\/\/[a-p]{32}\/$/.test(origin)) throw new BootstrapError('registered_bridge_origin_invalid');
  return origin;
}

async function containedRealpath(parent, target, code) {
  const root = await realpath(parent);
  const path = await realpath(target);
  if (!isInside(root, path)) throw new BootstrapError(code);
  return path;
}

async function loadCandidate(candidateArgument) {
  const buildRoot = await realpath(browserBuildRoot);
  const buildStat = await lstat(buildRoot);
  if (buildStat.isSymbolicLink()) throw new BootstrapError('browser_build_root_reparse_point');
  const candidateRoot = await containedRealpath(buildRoot, candidateArgument, 'candidate_outside_browser_build');
  const candidateStat = await lstat(candidateRoot);
  if (candidateStat.isSymbolicLink() || !candidateStat.isDirectory()) throw new BootstrapError('candidate_root_invalid');
  const pointer = JSON.parse(await readFile(join(candidateRoot, 'runtime', 'current.json'), 'utf8'));
  if (!pointer || !/^[A-Za-z0-9._-]+$/.test(pointer.version)
      || pointer.manifestPath !== `runtime/versions/${pointer.version}/manifest.json`) {
    throw new BootstrapError('candidate_pointer_invalid');
  }
  const versionRoot = await containedRealpath(candidateRoot, join(candidateRoot, 'runtime', 'versions', pointer.version), 'candidate_version_outside_root');
  const manifest = JSON.parse(await readFile(join(versionRoot, 'manifest.json'), 'utf8'));
  const identity = manifest?.bridgeIdentity;
  const health = manifest?.health;
  if (manifest.version !== pointer.version || manifest.product !== 'AkuBrowser'
      || identity?.profile !== 'production-app' || identity?.environment !== 'production'
      || identity?.distribution !== 'installed-app' || identity?.runtimeLifecycle !== 'managed'
      || identity?.runtimeAcquisition !== 'bundled-installer'
      || !/^[a-p]{32}$/.test(identity?.extensionId || '')
      || identity.origin !== `chrome-extension://${identity.extensionId}/`
      || health?.host !== '127.0.0.1' || !Number.isInteger(health.port) || health.port < 1 || health.port > 65535
      || health.path !== '/api/health' || !Number.isInteger(health.timeoutMs) || health.timeoutMs < 1) {
    throw new BootstrapError('candidate_production_tuple_contract_invalid');
  }
  const sidecarExe = await containedRealpath(versionRoot, join(versionRoot, manifest.sidecarPath), 'candidate_sidecar_outside_version');
  const chromiumExe = await containedRealpath(versionRoot, join(versionRoot, manifest.chromiumPath), 'candidate_chromium_outside_version');
  if (!(await stat(sidecarExe)).isFile() || !(await stat(chromiumExe)).isFile()) throw new BootstrapError('candidate_payload_unavailable');
  return {candidateRoot, versionRoot, version: manifest.version, manifest, sidecarExe, chromiumExe};
}

function canonicalJSON(value) {
  if (Array.isArray(value)) return value.map(canonicalJSON);
  if (value && typeof value === 'object') {
    return Object.fromEntries(Object.keys(value).sort().map(key => [key, canonicalJSON(value[key])]));
  }
  return value;
}

function projectionDigest(settings) {
  return createHash('sha256').update(JSON.stringify(canonicalJSON(settings))).digest('hex');
}

async function readJSON(url, timeoutMs = 4_000, headers = undefined, method = 'GET') {
  const response = await fetch(url, {method, headers, signal: AbortSignal.timeout(timeoutMs)});
  if (!response.ok) throw new BootstrapError('local_api_unavailable');
  return response.json();
}

function canonicalBridgeOrigin(value) {
  if (typeof value !== 'string') return null;
  const match = /^chrome-extension:\/\/([a-p]{32})\/?$/.exec(value);
  return match ? `chrome-extension://${match[1]}/` : null;
}

export function validateOriginalProjection(settingsEnvelope, bridgeEnvelope, configuredOrigin) {
  const settings = settingsEnvelope?.settings;
  const bridge = bridgeEnvelope?.bridge;
  const observedOrigin = bridge?.actual?.extensionOrigin;
  const originAbsent = observedOrigin === undefined || observedOrigin === null || observedOrigin === '';
  const heartbeatOrigin = originAbsent ? null : canonicalBridgeOrigin(observedOrigin);
  const configuredBridgeOrigin = canonicalBridgeOrigin(configuredOrigin);
  if (!settings || typeof settings !== 'object' || Array.isArray(settings)
      || bridge?.compatible !== true || !bridge.actual || typeof bridge.actual !== 'object'
      || !configuredBridgeOrigin
      || (!originAbsent && (!heartbeatOrigin || heartbeatOrigin !== configuredBridgeOrigin))) {
    throw new BootstrapError('original_projection_unverified');
  }
  return {settingsSha256: projectionDigest(settings), configuredBridgeOrigin,
    heartbeatOrigin, identityEvidence: heartbeatOrigin ? 'heartbeat_and_registered_config_match' : 'registered_config_only'};
}

export function sameOriginalProjection(before, after) {
  return Boolean(before && after && before.settingsSha256 === after.settingsSha256
    && before.configuredBridgeOrigin === after.configuredBridgeOrigin
    && before.heartbeatOrigin === after.heartbeatOrigin);
}

async function originalProjection(registration) {
  const [settingsEnvelope, bridgeEnvelope] = await Promise.all([
    readJSON(`${HEALTH_ORIGIN}:11122/api/settings`),
    readJSON(`${HEALTH_ORIGIN}:11122/api/bridge/health`),
  ]);
  return validateOriginalProjection(settingsEnvelope, bridgeEnvelope, registration.bridgeOrigin);
}

export async function runStopTestRestore(ops) {
  const report = {runtimeStopIssued: false, originalStopped: false, candidateReleased: null,
    restored: false, restoreVerified: false, restoreBlocked: false, failureCode: null};
  let before = null;
  try {
    before = await ops.snapshotOriginal();
    report.before = before;
  } catch (error) {
    report.failureCode = safeCode(error, 'original_snapshot_failed');
    return report;
  }
  report.runtimeStopIssued = true;
  ops.onStopIssued?.();
  try {
    await ops.stopOriginal();
    report.originalStopped = await ops.waitOriginalStopped();
    if (!report.originalStopped) throw new BootstrapError('original_runtime_stop_unverified');
    report.candidateResult = await ops.runCandidate();
  } catch (error) {
    report.failureCode ||= safeCode(error, 'bootstrap_smoke_failed');
  } finally {
    try {
      report.candidateReleased = await ops.verifyCandidateReleased();
    } catch {
      report.candidateReleased = false;
    }
    if (!report.candidateReleased) {
      report.restoreBlocked = true;
      report.restoreReason = 'candidate_process_or_profile_owner_remains';
      report.failureCode ||= 'candidate_shutdown_unverified';
    } else {
      try {
        const restore = await ops.restoreOriginal();
        report.restored = restore?.restored === true;
        report.restoreBlocked = restore?.blocked === true;
        if (!report.restored) {
          report.restoreReason = restore?.reason || 'original_runtime_restore_unverified';
          report.failureCode ||= 'original_runtime_restore_unverified';
        } else {
          const after = await ops.verifyOriginalRestored();
          report.after = after;
          report.restoreVerified = sameOriginalProjection(before, after);
          if (!report.restoreVerified) report.failureCode ||= 'original_projection_restore_mismatch';
        }
      } catch (error) {
        report.restoreReason = 'original_runtime_restore_verification_failed';
        report.failureCode ||= safeCode(error, 'original_runtime_restore_verification_failed');
      }
    }
  }
  return report;
}

function safeCode(error, fallback) {
  return typeof error?.code === 'string' && /^[a-z0-9_]+$/.test(error.code) ? error.code : fallback;
}

function powershellEnv(profiles, candidate, launcherPid) {
  const chromiumExe = candidate.chromiumExe;
  const sidecarExe = candidate.sidecarExe;
  const script = String.raw`
$wantedProfiles = @($env:AKU_BOOTSTRAP_PROFILE, $env:AKU_BOOTSTRAP_UI_PROFILE) | ForEach-Object { [IO.Path]::GetFullPath($_).TrimEnd('\') }
$wantedSidecar = [IO.Path]::GetFullPath($env:AKU_BOOTSTRAP_SIDECAR)
$wantedChromium = [IO.Path]::GetFullPath($env:AKU_BOOTSTRAP_CHROMIUM)
$launcherPid = [int]$env:AKU_BOOTSTRAP_LAUNCHER_PID
$port = [int]$env:AKU_BOOTSTRAP_PORT
$all = @(Get-CimInstance Win32_Process)
$candidate = @($all | Where-Object {
  $_.Name -ieq 'AkuSidecar.exe' -and $_.ExecutablePath -and
  [IO.Path]::GetFullPath($_.ExecutablePath) -ieq $wantedSidecar -and
  $_.ParentProcessId -eq $launcherPid
} | ForEach-Object {
  [pscustomobject]@{ pid=$_.ProcessId; parentPid=$_.ParentProcessId; executable=$_.ExecutablePath;
    creationUtc=$_.CreationDate.ToUniversalTime().ToString('o'); commandLine=$_.CommandLine }
})
$owners = @($all | Where-Object {
  $_.Name -ieq 'chrome.exe' -and $_.CommandLine -notlike '*--type=*'
} | ForEach-Object {
  $m = [regex]::Match($_.CommandLine, '--user-data-dir=(?:"([^"]+)"|(\S+))')
  $p = $m.Groups[1].Value + $m.Groups[2].Value
  if ($p -and ($wantedProfiles -contains [IO.Path]::GetFullPath($p).TrimEnd('\'))) {
    [pscustomobject]@{ pid=$_.ProcessId; parentPid=$_.ParentProcessId; executable=$_.ExecutablePath }
  }
})
$launcher = @($all | Where-Object { $_.ProcessId -eq $launcherPid } | Select-Object -First 1 | ForEach-Object {
  [pscustomobject]@{ pid=$_.ProcessId; executable=$_.ExecutablePath; creationUtc=$_.CreationDate.ToUniversalTime().ToString('o') }
})
try {
  $listeners = @(Get-NetTCPConnection -State Listen -LocalPort $port -ErrorAction Stop | ForEach-Object {
    [pscustomobject]@{ pid=$_.OwningProcess; address=$_.LocalAddress }
  })
  $listenerInspectionAvailable = $true
} catch { $listeners = @(); $listenerInspectionAvailable = $false }
[ordered]@{ candidate=$candidate; profileOwners=$owners; launcher=$launcher; listeners=$listeners;
  listenerInspectionAvailable=$listenerInspectionAvailable } | ConvertTo-Json -Depth 5 -Compress
`;
  return {
    script,
    env: {...process.env, AKU_BOOTSTRAP_PROFILE: profiles[0], AKU_BOOTSTRAP_UI_PROFILE: profiles[1],
      AKU_BOOTSTRAP_SIDECAR: sidecarExe, AKU_BOOTSTRAP_CHROMIUM: chromiumExe,
      AKU_BOOTSTRAP_LAUNCHER_PID: String(launcherPid), AKU_BOOTSTRAP_PORT: String(candidate.manifest.health.port)},
  };
}

async function processSnapshot(runState) {
  if (!runState?.child?.pid) return {candidate: [], profileOwners: [], launcher: []};
  const {script, env} = powershellEnv(runState.profiles, runState.candidate, runState.child.pid);
  let stdout;
  try {
    ({stdout} = await execFile('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {
      windowsHide: true, timeout: 10_000, maxBuffer: 512 * 1024, env,
    }));
  } catch {
    throw new BootstrapError('candidate_process_inspection_unavailable');
  }
  const value = JSON.parse(stdout.trim());
  const asArray = item => Array.isArray(item) ? item : item ? [item] : [];
  return {candidate: asArray(value.candidate), profileOwners: asArray(value.profileOwners), launcher: asArray(value.launcher),
    listeners: asArray(value.listeners), listenerInspectionAvailable: value.listenerInspectionAvailable === true};
}

function oneArgument(commandLine, flag) {
  const escaped = flag.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const matches = [...String(commandLine || '').matchAll(new RegExp(`(?:^|\\s)${escaped}\\s+(?:"([^"]+)"|(\\S+))`, 'g'))];
  if (matches.length !== 1) return null;
  return matches[0][1] || matches[0][2] || null;
}

function verifiedCandidateProcess(snapshot, runState) {
  const launcher = snapshot.launcher.find(value => value.pid === runState.child.pid);
  if (!launcher || !samePath(launcher.executable || '', runState.launcherExe)
      || Date.parse(launcher.creationUtc) < runState.startedAt - 3_000) return null;
  const matches = snapshot.candidate.filter(value => value.parentPid === runState.child.pid
    && samePath(value.executable || '', runState.candidate.sidecarExe)
    && Number.isInteger(value.pid) && Date.parse(value.creationUtc) >= runState.startedAt - 3_000);
  if (matches.length !== 1) return null;
  const profile = oneArgument(matches[0].commandLine, '--browser-profile');
  if (!profile || !samePath(profile, runState.profile)) return null;
  return matches[0];
}

function controlToken(commandLine) {
  const value = oneArgument(commandLine, '--runtime-control-token');
  return value && /^[a-f0-9]{64}$/i.test(value) ? value : null;
}

async function candidateReleased(runState) {
  if (!runState?.child?.pid) return !runState?.candidate?.manifest?.health?.port
    || (await portIsFree(runState.candidate.manifest.health.port));
  const snapshot = await processSnapshot(runState);
  const candidate = snapshot.candidate.filter(value => samePath(value.executable || '', runState.candidate.sidecarExe));
  const launcherAlive = snapshot.launcher.some(value => value.pid === runState.child.pid
    && samePath(value.executable || '', runState.launcherExe));
  const foreignProfileOwner = snapshot.profileOwners.some(value => !samePath(value.executable || '', runState.candidate.chromiumExe));
  const portAvailable = snapshot.listenerInspectionAvailable && snapshot.listeners.length === 0
    && await portIsFree(runState.candidate.manifest.health.port);
  return candidate.length === 0 && snapshot.profileOwners.length === 0 && !launcherAlive && !foreignProfileOwner
    && portAvailable && (runState.child.exitCode !== null || runState.child.signalCode !== null);
}

async function waitForCandidateRelease(runState, timeoutMs) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    try { if (await candidateReleased(runState)) return true; } catch {}
    await pause(300);
  }
  return candidateReleased(runState).catch(() => false);
}

async function cooperativeCandidateStop(runState) {
  if (!runState.child?.pid) return {released: true, method: 'launcher_never_spawned'};
  const deadline = Date.now() + CANDIDATE_DRAIN_MS;
  let verified = null;
  const identifyDeadline = Math.min(deadline, Date.now() + 5_000);
  while (Date.now() < identifyDeadline) {
    let snapshot;
    try { snapshot = await processSnapshot(runState); } catch { break; }
    verified = verifiedCandidateProcess(snapshot, runState);
    if (verified) {
      const ownersSafe = snapshot.profileOwners.every(owner => samePath(owner.executable || '', runState.candidate.chromiumExe));
      if (!ownersSafe) return {released: false, reason: 'isolated_profile_owner_executable_mismatch'};
      if (!snapshot.listenerInspectionAvailable) return {released: false, reason: 'candidate_listener_inspection_unavailable'};
      const listenerPids = snapshot.listeners.map(value => Number(value.pid));
      if (listenerPids.some(pid => pid !== verified.pid)) return {released: false, reason: 'candidate_port_owned_by_other_process'};
      if (!listenerPids.includes(verified.pid)) { await pause(300); continue; }
      const token = controlToken(verified.commandLine);
      if (!token) return {released: false, reason: 'candidate_shutdown_token_unverifiable'};
      let accepted = false;
      try {
        const response = await fetch(`${HEALTH_ORIGIN}:${runState.candidate.manifest.health.port}/api/runtime/shutdown-if-idle`, {
          method: 'POST', headers: {'X-Aku-Runtime-Control-Token': token}, signal: AbortSignal.timeout(5_000),
        });
        accepted = response.status === 202;
        await response.body?.cancel();
      } catch {}
      if (accepted && await waitForCandidateRelease(runState, Math.max(0, deadline - Date.now()))) {
        return {released: true, method: 'cooperative_runtime_shutdown'};
      }
      return {released: await waitForCandidateRelease(runState, Math.max(0, deadline - Date.now())),
        reason: accepted ? 'candidate_process_drain_timeout' : 'cooperative_runtime_shutdown_not_accepted'};
    }
    if (await candidateReleased(runState).catch(() => false)) return {released: true, method: 'natural_exit'};
    await pause(300);
  }
  return {released: await waitForCandidateRelease(runState, Math.max(0, deadline - Date.now())),
    reason: 'candidate_process_identity_unverified'};
}

async function fetchHealth(candidate) {
  return readJSON(`${HEALTH_ORIGIN}:${candidate.manifest.health.port}${candidate.manifest.health.path}`, 2_500);
}

async function candidatePortReady(runState) {
  const snapshot = await processSnapshot(runState);
  if (!snapshot.listenerInspectionAvailable) throw new BootstrapError('candidate_listener_inspection_unavailable');
  const process = verifiedCandidateProcess(snapshot, runState);
  const listenerPids = snapshot.listeners.map(value => Number(value.pid));
  if (listenerPids.some(pid => !process || pid !== process.pid)) throw new BootstrapError('candidate_port_owner_unverified');
  return Boolean(process && listenerPids.includes(process.pid));
}

function validateSettingsEnvelope(value) {
  const settings = value?.settings;
  if (!settings || typeof settings !== 'object' || Array.isArray(settings)
      || typeof value.provider !== 'string' || !value.provider
      || !value.collectionRuntime || typeof value.collectionRuntime !== 'object') {
    throw new BootstrapError('candidate_settings_projection_invalid');
  }
  return {settingsSha256: projectionDigest(settings), fieldCount: Object.keys(settings).length};
}

export function validateCandidateBridge(value, expectedOrigin) {
  const bridge = value?.bridge;
  const actual = bridge?.actual;
  if (!actual) throw new BootstrapError('candidate_bridge_not_loaded');
  if (bridge.compatible !== true || !canonicalBridgeOrigin(expectedOrigin)
      || canonicalBridgeOrigin(actual.extensionOrigin) !== canonicalBridgeOrigin(expectedOrigin)) {
    throw new BootstrapError('candidate_production_bridge_identity_mismatch');
  }
  const access = actual.sourceAccess;
  if (!access || !Array.isArray(access.grantedSources)
      || (access.sources !== undefined && !Array.isArray(access.sources))) {
    throw new BootstrapError('candidate_source_grants_unverifiable');
  }
  const reportedGrant = access.grantedSources.length !== 0
    || (access.sources || []).some(source => source?.permissionGranted === true || source?.ready === true);
  if (reportedGrant) throw new BootstrapError('candidate_source_grants_not_empty');
  return {compatible: true, extensionOrigin: expectedOrigin, grantedSourceCount: 0};
}

async function smokeCandidate(candidate, plan) {
  await mkdir(plan.acceptanceRoot, {recursive: false});
  const launcherExe = await containedRealpath(candidate.candidateRoot,
    join(candidate.candidateRoot, 'AkuBrowserLauncher.exe'), 'candidate_launcher_outside_root');
  if (!(await stat(launcherExe)).isFile()) throw new BootstrapError('candidate_launcher_unavailable');
  const runState = {candidate, profile: plan.isolatedBrowserProfile,
    profiles: [plan.isolatedBrowserProfile, plan.isolatedUiProfile], launcherExe, startedAt: Date.now(), child: null};
  const env = {...process.env, LOCALAPPDATA: plan.isolatedLocalAppData,
    AKUBROWSER_ISOLATED_TEST_CREDENTIAL_NAMESPACE: plan.credentialNamespace};
  let child;
  try {
    plan.candidateLaunchIssued = true;
    plan.activeRunState = runState;
    child = spawn(launcherExe, ['--install-root', candidate.candidateRoot], {
      cwd: candidate.versionRoot, env, windowsHide: false, stdio: 'ignore',
    });
    runState.child = child;
    await new Promise((resolveSpawn, rejectSpawn) => {
      child.once('spawn', resolveSpawn);
      child.once('error', () => rejectSpawn(new BootstrapError('candidate_launcher_start_failed')));
    });
  } catch (error) {
    runState.child = child || null;
    throw error instanceof BootstrapError ? error : new BootstrapError('candidate_launcher_start_failed');
  }
  let smokeResult = null;
  let primaryError = null;
  try {
    const deadline = Date.now() + MAX_BOOTSTRAP_MS;
    while (Date.now() < deadline) {
      if (child.exitCode !== null || child.signalCode !== null) throw new BootstrapError('candidate_launcher_exited_before_health');
      try {
        if (!(await candidatePortReady(runState))) { await pause(500); continue; }
        const health = await fetchHealth(candidate);
        const deployment = health.deployment || {};
        if (health.status !== 'ok' || health.version !== candidate.version || health.runtime !== 'go'
            || health.database?.status !== 'healthy' || deployment.mode !== 'production-installed-app'
            || deployment.runtimeInstallKind !== 'installed' || deployment.bridgeIdentityProfile !== 'production-app'
            || (deployment.releaseVersion && deployment.releaseVersion !== candidate.version)) {
          throw new BootstrapError('candidate_production_health_projection_mismatch');
        }
        const [bridge, settings] = await Promise.all([
          readJSON(`${HEALTH_ORIGIN}:${candidate.manifest.health.port}/api/bridge/health`, 4_000),
          readJSON(`${HEALTH_ORIGIN}:${candidate.manifest.health.port}/api/settings`, 4_000),
        ]);
        const bridgeProjection = validateCandidateBridge(bridge, candidate.manifest.bridgeIdentity.origin);
        const settingsProjection = validateSettingsEnvelope(settings);
        smokeResult = {health: {status: health.status, version: health.version, deploymentMode: deployment.mode,
          runtimeInstallKind: deployment.runtimeInstallKind, bridgeIdentityProfile: deployment.bridgeIdentityProfile},
          bridge: bridgeProjection, settings: settingsProjection, authenticationParity: false, trustedReaderProof: false};
        break;
      } catch (error) {
        if (error instanceof BootstrapError && !['local_api_unavailable', 'candidate_bridge_not_loaded'].includes(error.code)) throw error;
      }
      await pause(500);
    }
    if (!smokeResult) throw new BootstrapError('candidate_bootstrap_timeout');
  } catch (error) {
    primaryError = error instanceof BootstrapError ? error : new BootstrapError('candidate_bootstrap_failed');
  }
  const shutdown = await cooperativeCandidateStop(runState);
  plan.lastCandidateStop = {released: shutdown.released === true, method: shutdown.method || null, reason: shutdown.reason || null};
  if (!shutdown.released) {
    const error = new BootstrapError('candidate_shutdown_unverified');
    throw error;
  }
  if (primaryError) throw primaryError;
  return smokeResult;
}

async function portIsFree(port) {
  return new Promise(resolvePort => {
    const socket = createConnection({host: '127.0.0.1', port});
    let settled = false;
    const finish = free => {
      if (settled) return;
      settled = true;
      socket.destroy();
      resolvePort(free);
    };
    socket.setTimeout(1_000, () => finish(false));
    socket.once('connect', () => finish(false));
    socket.once('error', error => finish(error.code === 'ECONNREFUSED'));
  });
}

async function stage(code, operation) {
  try { return await operation(); }
  catch (error) {
    if (error instanceof BootstrapError) throw error;
    throw new BootstrapError(code);
  }
}

async function prepare(args) {
  const candidate = await stage('candidate_load_failed', () => loadCandidate(args.candidate));
  await stage('candidate_tuple_verification_failed', () => verifyTuple(candidate.candidateRoot));
  const registration = await stage('registered_runtime_load_failed', async () => {
    const loaded = await loadRegistration();
    loaded.bridgeOrigin = await loadRegisteredBridgeOrigin();
    return loaded;
  });
  const before = await stage('registered_runtime_preflight_failed', () => preflight(registration));
  const projection = await stage('original_projection_failed', () => originalProjection(registration));
  const acceptanceRoot = join(browserBuildRoot, `headless-hybrid-bootstrap-${Date.now()}-${randomUUID()}`);
  if (isInside(candidate.candidateRoot, acceptanceRoot) || isInside(acceptanceRoot, candidate.candidateRoot)) {
    throw new BootstrapError('candidate_and_acceptance_roots_overlap');
  }
  const buildRoot = await realpath(browserBuildRoot);
  if (!isInside(buildRoot, acceptanceRoot) || await pathExists(acceptanceRoot)) throw new BootstrapError('acceptance_root_unavailable');
  const namespaceDigest = createHash('sha256').update(acceptanceRoot.toLowerCase()).digest('hex').slice(0, 16);
  const profileRelative = candidate.manifest.storage?.browserProfileRelativePath;
  if (typeof profileRelative !== 'string' || !profileRelative || profileRelative.includes('..') || isAbsolute(profileRelative)) {
    throw new BootstrapError('candidate_profile_contract_invalid');
  }
  const isolatedLocalAppData = join(acceptanceRoot, 'LocalAppData');
  const isolatedBrowserProfile = resolve(isolatedLocalAppData, profileRelative);
  const isolatedUiProfile = `${isolatedBrowserProfile}-ui-split-cft`;
  if (!isInside(isolatedLocalAppData, isolatedBrowserProfile)) throw new BootstrapError('isolated_profile_outside_local_app_data');
  if (!isInside(isolatedLocalAppData, isolatedUiProfile)) throw new BootstrapError('isolated_ui_profile_outside_local_app_data');
  const plan = {status: args.allowRuntimeStop ? 'ready_for_explicitly_approved_smoke' : 'preflight_only',
    candidate: candidate.candidateRoot, candidateVersion: candidate.version, candidateBridgeOrigin: candidate.manifest.bridgeIdentity.origin,
    acceptanceRoot, isolatedLocalAppData, isolatedBrowserProfile, isolatedUiProfile, credentialNamespace: `AkuBrowserTest-${namespaceDigest}`,
    currentRuntimePreflight: before, currentRuntimeProjection: projection,
    bounds: {healthAndBridgeCheckMs: MAX_BOOTSTRAP_MS, candidateDrainMs: CANDIDATE_DRAIN_MS},
    runtimeStopIssued: false, candidateLaunchIssued: false, grantsExpected: 0,
    requiresBothRuntimeStopAndForegroundApproval: true, liveAuthenticatedSourcesUsed: false,
    profileAuthCopied: false, permissionsGranted: false, bridgeReloaded: false};
  return {candidate, registration, plan};
}

async function pathExists(path) {
  try { await lstat(path); return true; } catch { return false; }
}

async function main() {
  if (process.platform !== 'win32') throw new BootstrapError('windows_required');
  const args = parseArguments(process.argv.slice(2));
  const prepared = await prepare(args);
  if (!args.allowRuntimeStop) {
    process.stdout.write(`${JSON.stringify(prepared.plan)}\n`);
    return;
  }
  const {candidate, registration, plan} = prepared;
  // Repeat all live read-only gates immediately before any stop or launch.
  plan.currentRuntimePreflight = await stage('registered_runtime_preflight_failed', () => preflight(registration));
  const port = 11122;
  const current = await stage('original_projection_failed', () => originalProjection(registration));
  plan.currentRuntimeProjection = current;
  const report = await runStopTestRestore({
    snapshotOriginal: async () => originalProjection(registration),
    onStopIssued: () => { plan.runtimeStopIssued = true; },
    stopOriginal: async () => supervisor(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'approved installed hybrid package bootstrap smoke', '--request-id', randomUUID()]),
    waitOriginalStopped: async () => {
      const stopped = await waitForStopped(registration.profile);
      if (!stopped.stopped || !(await portIsFree(port))) return false;
      return true;
    },
    runCandidate: async () => {
      return smokeCandidate(candidate, plan);
    },
    verifyCandidateReleased: async () => plan.activeRunState
      ? candidateReleased(plan.activeRunState) : !plan.candidateLaunchIssued,
    restoreOriginal: async () => restoreOriginal(registration, true, Date.now() + 45_000),
    verifyOriginalRestored: async () => {
      const afterPreflight = await preflight(registration);
      const projection = await originalProjection(registration);
      return {...projection, preflight: afterPreflight};
    },
  });
  const {activeRunState, lastCandidateStop, ...publicPlan} = plan;
  const output = {status: report.failureCode ? 'failed' : 'complete', plan: publicPlan, report,
    candidateStop: lastCandidateStop || null};
  process.stdout.write(`${JSON.stringify(output)}\n`);
  if (report.failureCode || !report.restored || !report.restoreVerified) process.exitCode = 1;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  void main().catch(error => {
    process.stderr.write(`${JSON.stringify({status: 'failed', code: safeCode(error, 'bootstrap_harness_error')})}\n`);
    process.exitCode = 1;
  });
}
