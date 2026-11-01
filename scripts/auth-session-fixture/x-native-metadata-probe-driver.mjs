import {spawn} from 'node:child_process';
import {mkdir, lstat, realpath, writeFile} from 'node:fs/promises';
import {dirname, isAbsolute, join, relative, resolve, sep} from 'node:path';
import {performance} from 'node:perf_hooks';
import {fileURLToPath, pathToFileURL} from 'node:url';
import {parseStrictJSON} from './x-evidence-ingress.mjs';
import {AccountSettingsCapture, accountSettingsBodyMode} from './x-account-settings-capture.mjs';
import {BOOTSTRAP_CURRENT_SELF_EXPRESSION} from './x-bootstrap-current-self.mjs';
import {BootstrapHandoffCapture} from './x-bootstrap-breakpoint-handoff.mjs';
import {QASourceCapture, qaCookieRequest, finishQASourceQualification} from './x-qa-source-qualification.mjs';
import {QAGuestCapture, QA_GUEST_HOME, finishQAAuthControl} from './x-qa-auth-control.mjs';
import {QADirectCapture, QA_AB_URL, finishQADirectAB} from './x-qa-direct-ab.mjs';
import {QA_WEB_INSTALL, QA_WEB_SNAPSHOT, QA_WEB_START, QA_WEB_DISPOSE, finishQAWebABA} from './x-qa-web-aba.mjs';
import {finishQAWeb401} from './x-qa-401-projection.mjs';
import {shortQACheckpoint, runQAShortFeasibility} from './x-qa-short-feasibility.mjs';
import {QACheckpointRendezvous, synchronizedQAReport} from './x-qa-checkpoint-rendezvous.mjs';
import {QA_LIVE_SELF_EXPRESSION} from './x-qa-live-self.mjs';
import {QALiveSelfTransport, finishQALiveSelf} from './x-qa-live-self-transport.mjs';
import {manualCaptureRPCCode, manualCaptureRPCDiagnostic} from './x-native-manual-capture.mjs';
import {startupRPCCode} from './x-manual-startup-diagnostic.mjs';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';

// QA workflows and stdin control remain separate from production owner composition.
import {createNativeOwnerRuntime,nativeProbeDefinitions} from '../../internal/collection/headless/worker/native-pilot/x-native-owner-runtime.mjs';
const fixture=createNativeOwnerRuntime().fixture;
const runtimeState=fixture.state;
const {
  qualificationMode, bodyMode, retainedQAMode, HOME_URL, MAX_CONTROL_LINE_BYTES,
  MAX_CONTROL_BUFFER_BYTES, MAX_CONTROL_LINES_PER_CHUNK, MAX_CONTROL_LINES_TOTAL, MAX_CANDIDATES, MAX_ENCODED_BYTES,
  OBSERVATION_MS, MAX_TOTAL_NETWORK_BUFFER_BYTES, MAX_RESOURCE_NETWORK_BUFFER_BYTES, CACHE_KEYS, startupCDPFailureCategory,
  EARLY_COMMAND_LABELS, EARLY_LABELS, probeError, isObject, exactKeys,
  parseControlOrigin, parseJSONBytes, CDPFrameDecoder, classifyXOperation, isSafeOperationCode,
  projectNetworkEvent, MetadataWindow, LoginObservationGate, TwoPhaseWorkflowGrant, twoPhaseChromeArgs,
  sameWindowsPath, ordinaryDirectoryIdentity, sameDirectoryIdentity, projectCDPEvent, ACCOUNT_READY_EXPRESSION,
  manualSeedRPCCode,
}=nativeProbeDefinitions;
const {
  verifyTwoPhaseDirectories, emit, emitReply, safeCode, isEarlyBootstrapCallFrameRead,
  send, rejectPending, bindChrome, withTimeout, deferred,
  initializeChromeSession, requireOwner, loginState, sanitizeEarlyHandoffDiagnostics, boundCheckpoint,
  prepareShortConcurrency, closeShortCollector, collectorShortCheckpoint, manualReloadFrameProjection, createOwnedManualStartupOwners,
  manualStartupStateReady, manualOwnerSnapshot, executeManualOwnerRPC, seedRetainedManualPilot, openRetainedManualReader,
  closeChrome,
}=fixture;
export {startupCDPFailureCategory, CDPFrameDecoder, classifyXOperation, projectNetworkEvent, MetadataWindow, LoginObservationGate, TwoPhaseWorkflowGrant, twoPhaseChromeArgs, projectCDPEvent, manualSeedRPCCode, isEarlyBootstrapCallFrameRead, sanitizeEarlyHandoffDiagnostics, collectorShortCheckpoint, manualReloadFrameProjection, createOwnedManualStartupOwners, manualStartupStateReady, manualOwnerSnapshot, executeManualOwnerRPC, seedRetainedManualPilot};

let controlBytes = Buffer.alloc(0);
let controlLines = 0;
let controlChain = Promise.resolve();
let bootstrapDone = false;
let shuttingDown = false;
export async function executeManualReply(message,executeCommand=execute,reply=emitReply){
  const id=message.id,seed=message.op==='manual_seed_bootstrap';
  let result;
  try{
    result=await executeCommand(message);
    reply(id,Buffer.isBuffer(result)?parseStrictJSON(result.toString('utf8'),{maxJSONDepth:4,maxJSONTokens:96}):result,undefined,message.op==='manual_capture'?800*1024:MAX_CONTROL_LINE_BYTES);
  }catch(error){
    if(message.op==='manual_capture')reply(id,undefined,manualCaptureRPCCode(error),undefined,manualCaptureRPCDiagnostic(error));
    else reply(id,undefined,seed?manualSeedRPCCode(error):startupRPCCode(error));
  }
  finally{if(Buffer.isBuffer(result))result.fill(0);}
}

async function validatePaths(mode = 'single_phase', allowOwnedProfile = false) {
  if (!isAbsolute(runtimeState.executable) || !isAbsolute(runtimeState.qaRoot)) throw probeError('arguments_invalid');
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
  const resolvedProject = await realpath(projectRoot).catch(() => '');
  const expectedParent = resolvedProject ? resolve(resolvedProject, 'runtime', 'qa', 'auth-session-sidecar') : '';
  const expectedParentReal = expectedParent ? await realpath(expectedParent).catch(() => '') : '';
  const rootStat = await lstat(runtimeState.qaRoot).catch(() => null);
  const rootReal = await realpath(runtimeState.qaRoot).catch(() => '');
  const rel = rootReal && expectedParentReal ? relative(expectedParentReal, rootReal) : '';
  const executableStat = await lstat(runtimeState.executable).catch(() => null);
  const rootName = rootReal.split(/[\\/]/u).at(-1);
  const rootNameValid = mode === 'two_phase'
    ? /^run-x-two-phase-[a-f0-9]{16,64}$/u.test(rootName)
    : mode === 'single_phase' && /^run-x-native-metadata-[A-Za-z0-9-]{1,80}$/u.test(rootName);
  if (!resolvedProject || !expectedParentReal || !rootStat?.isDirectory() || rootStat.isSymbolicLink() ||
      !rootReal || resolve(rootReal) !== resolve(runtimeState.qaRoot) || !rel || rel === '..' || rel.startsWith(`..${sep}`) ||
      isAbsolute(rel) || !rootNameValid ||
      !executableStat?.isFile() || executableStat.isSymbolicLink()) throw probeError('paths_invalid');
  const origin = parseControlOrigin(runtimeState.controlOrigin);
  const profile = join(rootReal, 'shared');
  if (allowOwnedProfile) {
    if (!retainedQAMode(runtimeState.driverMode) || mode !== 'two_phase' ||
        !runtimeState.retainedQAScope?.reuse || !sameWindowsPath(dirname(rootReal), expectedParentReal)) {
      throw probeError('retained_scope_required');
    }
    const identity = await ordinaryDirectoryIdentity(profile);
    const rootIdentity = await ordinaryDirectoryIdentity(rootReal);
    if (!sameDirectoryIdentity(identity, runtimeState.retainedQAScope.profileIdentity) ||
        !sameDirectoryIdentity(rootIdentity, runtimeState.retainedQAScope.rootIdentity)) throw probeError('owned_profile_changed');
  } else if (await lstat(profile).then(() => true, () => false)) throw probeError('profile_not_new');
  return { root: rootReal, profile, origin };
}

async function acceptRetainedQAScope(message) {
  if (!retainedQAMode(runtimeState.driverMode) || runtimeState.retainedQAScope || runtimeState.child || runtimeState.protocolOwner || runtimeState.twoPhaseState ||
      typeof message.nonce !== 'string' || !/^[a-f0-9]{64}$/u.test(message.nonce) || typeof message.reuse !== 'boolean') {
    throw probeError('retained_scope_invalid');
  }
  if ((runtimeState.driverMode === 'retained_bootstrap' || qualificationMode(runtimeState.driverMode)) && !message.reuse) throw probeError('retained_profile_required');
  // This private pipe message arrives only after Node is inside its owner's Job.
  // Identity is pinned by the Go lease; no profile path comes from the message.
  const rootIdentity = await ordinaryDirectoryIdentity(runtimeState.qaRoot);
  const project = await realpath(resolve(dirname(fileURLToPath(import.meta.url)), '..', '..'));
  const parent = await realpath(join(project, 'runtime', 'qa', 'auth-session-sidecar'));
  if (!sameWindowsPath(dirname(runtimeState.qaRoot), parent) || !/^run-x-two-phase-[a-f0-9]{32}$/u.test(runtimeState.qaRoot.split(/[\\/]/u).at(-1))) {
    throw probeError('retained_scope_invalid');
  }
  const profileIdentity = message.reuse ? await ordinaryDirectoryIdentity(join(runtimeState.qaRoot, 'shared')) : null;
  if (!message.reuse && await lstat(join(runtimeState.qaRoot, 'shared')).then(() => true, () => false)) throw probeError('profile_not_new');
  runtimeState.retainedQAScope = { reuse: message.reuse, rootIdentity, profileIdentity, consumed: false };
  return { scoped: true };
}

async function launchRetainedQAChrome(message) {
  const scope = runtimeState.retainedQAScope;
  if (!scope || scope.consumed || message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw probeError('retained_scope_required');
  }
  if (!scope.reuse) {
    if (!sameDirectoryIdentity(await ordinaryDirectoryIdentity(runtimeState.qaRoot), scope.rootIdentity)) throw probeError('owned_profile_changed');
    return launchTwoPhaseChrome(message);
  }
  scope.consumed = true;
  if (runtimeState.child || runtimeState.protocolOwner || runtimeState.cdpFailure || runtimeState.twoPhaseState) throw probeError('collector_launch_invalid');
  const paths = await validatePaths('two_phase', true);
  runtimeState.qaRoot = paths.root;
  runtimeState.profilePath = paths.profile;
  runtimeState.controlOrigin = paths.origin;
  runtimeState.twoPhaseState = { root: runtimeState.qaRoot, profile: runtimeState.profilePath, rootIdentity: scope.rootIdentity,
    profileIdentity: scope.profileIdentity, phase2Started: true };
  if (!(await verifyTwoPhaseDirectories())) throw probeError('owned_profile_changed');
  const processChild = spawn(runtimeState.executable, twoPhaseChromeArgs(runtimeState.profilePath, true),
    { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  runtimeState.child = processChild;
  runtimeState.childExited = false;
  bindChrome(processChild);
  if (!Number.isSafeInteger(processChild.pid) || processChild.pid <= 0) throw probeError('chrome_spawn_failed');
  return initializeChromeSession(processChild, runtimeState.profilePath);
}

async function launchSinglePhaseChrome(message) {
  if (runtimeState.protocolOwner || runtimeState.child || runtimeState.cdpFailure || message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw probeError('collector_launch_invalid');
  }
  const paths = await validatePaths();
  runtimeState.qaRoot = paths.root;
  runtimeState.profilePath = paths.profile;
  runtimeState.controlOrigin = paths.origin;
  try {
    await mkdir(runtimeState.profilePath, { recursive: false, mode: 0o700 });
    const defaultProfile = join(runtimeState.profilePath, 'Default');
    await mkdir(defaultProfile, { recursive: false, mode: 0o700 });
    await writeFile(join(defaultProfile, 'Preferences'), JSON.stringify({ profile: { name: 'AkuBrowser X Native QA' } }),
      { flag: 'wx', mode: 0o600 });
  } catch { throw probeError('profile_create_failed'); }
  const profileReal = await realpath(runtimeState.profilePath).catch(() => '');
  if (!profileReal || resolve(profileReal) !== resolve(runtimeState.profilePath)) throw probeError('profile_path_invalid');
  const flags = [
    `--user-data-dir=${profileReal}`,
    '--profile-directory=Default',
    '--remote-debugging-pipe',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-extensions',
    '--disable-background-networking',
    '--disable-component-update',
    '--disable-sync',
    '--disable-background-mode',
    '--disable-default-apps',
    '--disable-logging',
    '--disable-breakpad',
    '--disable-crash-reporter',
    '--metrics-recording-only',
    '--new-window',
    HOME_URL,
  ];
  const processChild = spawn(runtimeState.executable, flags, { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  if (!processChild.pid) throw probeError('chrome_spawn_failed');
  runtimeState.child = processChild;
  runtimeState.childExited = false;
  bindChrome(processChild);
  return initializeChromeSession(processChild, profileReal);
}

async function launchTwoPhaseChrome(message) {
  if (message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw probeError('collector_launch_invalid');
  }
  if (!runtimeState.twoPhaseState) {
    if (runtimeState.protocolOwner || runtimeState.child || runtimeState.cdpFailure) throw probeError('collector_launch_invalid');
    const paths = await validatePaths('two_phase');
    runtimeState.qaRoot = paths.root;
    runtimeState.profilePath = paths.profile;
    runtimeState.controlOrigin = paths.origin;
    const rootIdentity = await ordinaryDirectoryIdentity(runtimeState.qaRoot);
    try {
      await mkdir(runtimeState.profilePath, { recursive: false, mode: 0o700 });
      const defaultProfile = join(runtimeState.profilePath, 'Default');
      await mkdir(defaultProfile, { recursive: false, mode: 0o700 });
      await writeFile(join(defaultProfile, 'Preferences'), JSON.stringify({ profile: { name: 'AkuBrowser X QA Two Phase' } }),
        { flag: 'wx', mode: 0o600 });
    } catch { throw probeError('profile_create_failed'); }
    const profileIdentity = await ordinaryDirectoryIdentity(runtimeState.profilePath);
    const owner = ++runtimeState.nextOwner;
    const processChild = spawn(runtimeState.executable, twoPhaseChromeArgs(runtimeState.profilePath, false),
      { windowsHide: false, stdio: 'ignore' });
    let resolveExit;
    const exitPromise = new Promise(resolvePromise => { resolveExit = resolvePromise; });
    const phase1 = { owner, pid: 0, exitCode: null, spawnError: false, exitPromise };
    runtimeState.child = processChild;
    runtimeState.childExited = false;
    processChild.on('error', () => { phase1.spawnError = true; });
    processChild.once('exit', code => {
      phase1.exitCode = Number.isInteger(code) ? code : null;
      runtimeState.childExited = true;
      resolveExit(phase1.exitCode);
    });
    await new Promise((resolvePromise, rejectPromise) => {
      processChild.once('spawn', resolvePromise);
      processChild.once('error', () => rejectPromise(probeError('chrome_spawn_failed')));
    });
    if (!Number.isSafeInteger(processChild.pid) || processChild.pid <= 0) throw probeError('chrome_spawn_failed');
    phase1.pid = processChild.pid;
    runtimeState.twoPhaseState = {
      root: runtimeState.qaRoot, profile: runtimeState.profilePath, rootIdentity, profileIdentity,
      phase1, grant: new TwoPhaseWorkflowGrant({ owner, pid: phase1.pid }), phase2Started: false,
    };
    return { owner, pid: processChild.pid, windowCount: 0, bootstrapCount: 0, nativeReaderStartup: false };
  }

  const state = runtimeState.twoPhaseState;
  if (state.phase2Started || !state.grant.authorized || !runtimeState.childExited || state.phase1.exitCode !== 0 ||
      state.phase1.spawnError || runtimeState.cdpFailure) throw probeError('phase2_not_authorized');
  const identityUnchanged = await verifyTwoPhaseDirectories();
  state.grant.consume({ identityUnchanged });
  state.phase2Started = true;
  const profileReal = await realpath(state.profile).catch(() => '');
  if (!profileReal || !sameWindowsPath(profileReal, state.profile)) throw probeError('owned_profile_changed');
  if (!(await verifyTwoPhaseDirectories())) throw probeError('owned_profile_changed');
  const processChild = spawn(runtimeState.executable, twoPhaseChromeArgs(profileReal, true),
    { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  runtimeState.child = processChild;
  runtimeState.childExited = false;
  bindChrome(processChild);
  if (!Number.isSafeInteger(processChild.pid) || processChild.pid <= 0) throw probeError('chrome_spawn_failed');
  return initializeChromeSession(processChild, profileReal);
}

async function launchChrome(message) {
  if (retainedQAMode(runtimeState.driverMode)) return launchRetainedQAChrome(message);
  return runtimeState.driverMode === 'two_phase' || accountSettingsBodyMode(runtimeState.driverMode)
    ? launchTwoPhaseChrome(message)
    : launchSinglePhaseChrome(message);
}

async function authorizeObservation(message) {
  if (runtimeState.driverMode !== 'two_phase' && !accountSettingsBodyMode(runtimeState.driverMode) || !runtimeState.twoPhaseState || runtimeState.twoPhaseState.phase2Started) {
    throw probeError('workflow_not_available');
  }
  const state = runtimeState.twoPhaseState;
  if (!runtimeState.childExited) {
    await withTimeout(state.phase1.exitPromise, 'phase1_exit_not_observed', 2_000);
  }
  const identityUnchanged = await verifyTwoPhaseDirectories();
  return state.grant.authorize({
    owner: message.owner,
    oldPID: message.oldPID,
    nonce: message.nonce,
    exitCode: state.phase1.exitCode,
    identityUnchanged: identityUnchanged && !state.phase1.spawnError,
  });
}

async function bootstrapCurrentSelf(message) {
  if (runtimeState.driverMode !== 'retained_bootstrap' || !runtimeState.retainedQAScope?.reuse) throw probeError('bootstrap_mode_required');
  const current = requireOwner(message);
  if (!current.gate.loginVisible || current.gate.observationStarted || current.bootstrapAttempted || current.earlyRunning ||
      current.earlyAttempted && (!current.earlyCleanupVerified || !current.earlyHandoffAccepted)) throw probeError('bootstrap_gate_closed');
  current.bootstrapAttempted = true;
  return send('Runtime.evaluate', { expression: BOOTSTRAP_CURRENT_SELF_EXPRESSION, returnByValue: true,
    awaitPromise: false, userGesture: false, throwOnSideEffect: true, timeout: MANUAL_TIMING.bootstrapEvaluationMS }, current.sessionId);
}

async function earlyBootstrapCurrentSelf(message) {
  if (runtimeState.driverMode !== 'retained_bootstrap' || !runtimeState.retainedQAScope?.reuse) throw probeError('bootstrap_mode_required');
  const current = requireOwner(message);
  if (!current.gate.loginVisible || current.gate.observationStarted || current.bootstrapAttempted || current.earlyAttempted) {
    throw probeError('early_bootstrap_gate_closed');
  }
  current.earlyAttempted = true;
  current.earlyRunning = true;
  current.earlyCommandStatus = Object.fromEntries(EARLY_LABELS.map(label => [label, 'not_attempted']));
  current.earlyCapture = new BootstrapHandoffCapture({
    send: async (method, params, purpose = null) => {
      const label = purpose === 'normal_remove' ? 'normalBreakpointRemove' : EARLY_COMMAND_LABELS[method];
      if (!label) throw probeError('early_command_not_authorized');
      if (current.earlyCommandStatus[label] !== 'failed') current.earlyCommandStatus[label] = 'sent';
      try {
        const result = await send(method, params, current.sessionId, purpose);
        if (current.earlyCommandStatus[label] !== 'failed') current.earlyCommandStatus[label] = 'acknowledged';
        return result;
      } catch (error) { current.earlyCommandStatus[label] = 'failed'; throw error; }
    },
  });
  try {
    const result = await current.earlyCapture.run();
    current.earlyHandoffDiagnostics = current.earlyCapture.diagnostics();
    current.earlyHandoffAccepted = current.earlyHandoffDiagnostics.handoffRejected === false;
    current.earlyCleanupVerified = result.cleanupVerified === true;
    return result;
  } finally {
    current.earlyRunning = false;
    current.earlyCapture = null;
    current.gate.setLoginVisible(false); // The reload requires a fresh Home check.
  }
}

function earlyBootstrapDiagnostics(message) {
  if (runtimeState.driverMode !== 'retained_bootstrap' || !runtimeState.active || message.owner !== runtimeState.active.owner ||
      !runtimeState.active.earlyAttempted || runtimeState.active.earlyRunning || !runtimeState.active.earlyCommandStatus) throw probeError('early_diagnostics_unavailable');
  const commands = {};
  for (const label of EARLY_LABELS) {
    const status = runtimeState.active.earlyCommandStatus[label];
    if (!['not_attempted', 'sent', 'acknowledged', 'failed'].includes(status)) throw probeError('early_diagnostics_invalid');
    commands[label] = status;
  }
  const failureCategory = !runtimeState.cdpFailure ? 'none' : runtimeState.cdpFailure === 'cdp_frame_count_exceeded' ? 'frame_count_limit' :
    ['cdp_frame_too_large', 'cdp_input_invalid', 'cdp_json_invalid', 'cdp_utf8_invalid'].includes(runtimeState.cdpFailure) ? 'frame_invalid' :
    runtimeState.cdpFailure.endsWith('_timeout') ? 'command_timeout' : 'other';
  return { schema: 'aku.x.early-bootstrap-transport.v2', commands, failureCategory,
    capture: sanitizeEarlyHandoffDiagnostics(runtimeState.active.earlyHandoffDiagnostics), canAuthorize: false };
}

function validCandidate(candidate) {
  if (!isObject(candidate) || !/^r(?:[1-9]|[1-5][0-9]|6[0-4])$/u.test(candidate.alias) ||
      !isSafeOperationCode(candidate.operation) ||
      !['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD', 'OTHER'].includes(candidate.method) ||
      !(candidate.status === null || Number.isInteger(candidate.status) && candidate.status >= 100 && candidate.status <= 599) ||
      typeof candidate.ambiguous !== 'boolean' ||
      !(candidate.hasExtraInfo === null || typeof candidate.hasExtraInfo === 'boolean') ||
      !['complete', 'failed', 'incomplete'].includes(candidate.completion) ||
      !(candidate.bytes === null || Number.isSafeInteger(candidate.bytes) && candidate.bytes >= 0 && candidate.bytes <= MAX_ENCODED_BYTES) ||
      !isObject(candidate.cache) || !isObject(candidate.timingMs)) return false;
  if (CACHE_KEYS.some(key => candidate.cache[key] !== null && typeof candidate.cache[key] !== 'boolean')) return false;
  return ['response', 'completion'].every(key => candidate.timingMs[key] === null ||
    Number.isSafeInteger(candidate.timingMs[key]) && candidate.timingMs[key] >= 0 &&
    candidate.timingMs[key] <= OBSERVATION_MS && candidate.timingMs[key] % 250 === 0);
}

function safeReport(metadata, eventLimitReached) {
  if (!isObject(metadata) || !Array.isArray(metadata.candidates) || metadata.candidates.length > MAX_CANDIDATES ||
      !Number.isSafeInteger(metadata.dropped) || metadata.dropped < 0 || metadata.dropped > 10_000 ||
      typeof eventLimitReached !== 'boolean' || !metadata.candidates.every(validCandidate)) {
    throw probeError('metadata_projection_invalid');
  }
  return {
    schema: 'aku.x.native-metadata-observation.v1',
    coverage: 'partial_single_page_home_reload',
    extraInfoCoverage: 'not_observed',
    observationWindowMs: OBSERVATION_MS,
    eventLimitReached,
    candidateCount: metadata.candidates.length,
    droppedCandidates: metadata.dropped,
    candidates: metadata.candidates,
    canAuthorize: false,
  };
}

async function startObservation(message) {
  if (runtimeState.driverMode === 'retained_bootstrap') throw probeError('network_observation_not_available');
  if (qualificationMode(runtimeState.driverMode) && !runtimeState.active?.qualificationAttempted) throw probeError('qualification_mode_required');
  const current = requireOwner(message);
  if (!current.gate.loginVisible) throw probeError('login_required');
  current.gate.beginObservation();
  current.networkEventCount = 0;
  current.eventLimitReached = false;
  await send('Network.enable', {
    maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
    maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES,
    maxPostDataSize: 0,
  }, current.sessionId);
  current.gate.markNetworkEnabled();
  if (bodyMode(runtimeState.driverMode)) {
    await send('Network.setCacheDisabled', { cacheDisabled: true }, current.sessionId);
    const readBody = requestId => send('Network.getResponseBody', { requestId }, current.sessionId);
    if (qualificationMode(runtimeState.driverMode)) {
      const { frameId } = await send('Page.getFrameTree', {}, current.sessionId);
      current.accountSettings = new QASourceCapture({ readBody, frameId });
    } else current.accountSettings = new AccountSettingsCapture({ readBody });
  }
  current.metadata = new MetadataWindow();
  current.observationEndsAt = Date.now() + OBSERVATION_MS;
  current.pageLoad = deferred();
  const loadPromise = current.pageLoad.promise;
  await send('Page.reload', { ignoreCache: false }, current.sessionId);
  current.gate.markReloaded();
  await withTimeout(loadPromise, 'home_reload_timeout', OBSERVATION_MS);
  current.pageLoad = null;
  const remaining = Math.max(0, current.observationEndsAt - Date.now());
  if (remaining > 0) await new Promise(resolvePromise => setTimeout(resolvePromise, remaining));
  const metadata = current.metadata.finish();
  current.metadata = null;
  const accountSettings = current.accountSettings ? await current.accountSettings.finish() : null;
  await send('Network.disable', {}, current.sessionId);
  const report = safeReport(metadata, current.eventLimitReached);
  return bodyMode(runtimeState.driverMode) ? { metadata: report, accountSettings } : report;
}

async function qualifySource(message, typesOnly = false) {
  if (!qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse) throw probeError('qualification_mode_required');
  const current = requireOwner(message);
  if (!current.gate.loginVisible || current.qualificationAttempted) throw probeError('qualification_gate_closed');
  current.qualificationAttempted = true;
  const { accountSettings: source } = await startObservation(message);
  return finishQASourceQualification(source, {
    inspectCookies: !typesOnly,
    readHome: async () => {
      requireOwner(message);
      const result = await send('Runtime.evaluate', { expression: ACCOUNT_READY_EXPRESSION,
        returnByValue: true, awaitPromise: false, userGesture: false }, current.sessionId);
      return result.value;
    },
    readCookies: async route => {
      requireOwner(message);
      current.qaSourceRoute = route;
      current.qaCookieRequested = true;
      return send('Network.getCookies', qaCookieRequest(route), current.sessionId);
    },
  });
}

async function qualifyAuthControl(message) {
  const current = requireOwner(message);
  if (!qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse || current.authControlAttempted) throw probeError('qa_auth_control_gate_closed');
  current.authControlAttempted = true;
  const signedIn = await qualifySource(message, true);
  const home = async () => {
    requireOwner(message);
    return (await send('Runtime.evaluate', { expression: ACCOUNT_READY_EXPRESSION,
      returnByValue: true, awaitPromise: false, userGesture: false }, current.sessionId)).value;
  };
  const before = await home();
  if (!before || signedIn.source.route !== 'api_v1' || signedIn.source.diagnostic !== 'projected' || signedIn.source.bodyContractMatches !== true) throw probeError('qa_positive_control_required');
  const guest = current.guest = { events: 0, navigated: false, closed: false };
  let report, disposed = false;
  try {
    guest.contextId = (await send('Target.createBrowserContext', { disposeOnDetach: true })).browserContextId;
    guest.targetId = (await send('Target.createTarget', { url: 'about:blank', browserContextId: guest.contextId })).targetId;
    guest.sessionId = (await send('Target.attachToTarget', { targetId: guest.targetId, flatten: true })).sessionId;
    if (guest.sessionId === current.sessionId) throw probeError('qa_guest_session_collision');
    await send('Page.enable', {}, guest.sessionId);
    guest.frameId = (await send('Page.getFrameTree', {}, guest.sessionId)).frameId;
    guest.capture = new QAGuestCapture('api_v1', guest.frameId);
    await send('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
      maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }, guest.sessionId);
    await send('Network.setCacheDisabled', { cacheDisabled: true }, guest.sessionId);
    guest.endsAt = Date.now() + OBSERVATION_MS;
    await send('Page.navigate', { url: QA_GUEST_HOME }, guest.sessionId);
    guest.navigated = true;
    await new Promise(resolvePromise => setTimeout(resolvePromise, Math.max(0, guest.endsAt - Date.now())));
    report = guest.capture.finish();
    await send('Network.disable', {}, guest.sessionId);
  } finally {
    if (guest.contextId) {
      await send('Target.disposeBrowserContext', { browserContextId: guest.contextId });
      disposed = true;
    }
    guest.closed = true;
  }
  return finishQAAuthControl(signedIn, report, before, await home(), disposed);
}

async function runDirectABPage(current, role) {
  const state = current.ab = { role, navigated: false, closed: false, disposing: false, events: 0 };
  let report;
  try {
    if (role === 'b') state.contextId = (await send('Target.createBrowserContext', { disposeOnDetach: true })).browserContextId;
    state.targetId = (await send('Target.createTarget', role === 'a' ? { url: 'about:blank' } :
      { url: 'about:blank', browserContextId: state.contextId })).targetId;
    state.sessionId = (await send('Target.attachToTarget', { targetId: state.targetId, flatten: true })).sessionId;
    if (state.sessionId === current.sessionId) throw probeError('qa_ab_session_collision');
    await send('Page.enable', {}, state.sessionId);
    state.frameId = (await send('Page.getFrameTree', {}, state.sessionId)).frameId;
    state.capture = new QADirectCapture(role, state.frameId, requestId => send('Network.getResponseBody', { requestId }, state.sessionId));
    await send('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
      maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }, state.sessionId);
    await send('Network.setCacheDisabled', { cacheDisabled: true }, state.sessionId);
    state.endsAt = Date.now() + 10000;
    await send('Page.navigate', { url: QA_AB_URL }, state.sessionId);
    state.navigated = true;
    let timer;
    try { await Promise.race([state.capture.done, new Promise(resolve => { timer = setTimeout(resolve, Math.max(0, state.endsAt - Date.now())); })]); }
    finally { clearTimeout(timer); }
    report = await state.capture.finish();
    await send('Network.disable', {}, state.sessionId);
  } finally {
    state.disposing = true;
    if (role === 'a' && state.targetId) await send('Target.closeTarget', { targetId: state.targetId });
    if (role === 'b' && state.contextId) await send('Target.disposeBrowserContext', { browserContextId: state.contextId });
    state.closed = true;
  }
  return report;
}

async function qualifyDirectAB(message) {
  const current = requireOwner(message);
  if (!qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse || !current.gate.loginVisible ||
      current.directABAttempted || current.gate.observationStarted) throw probeError('qa_direct_ab_gate_closed');
  current.directABAttempted = true;
  current.gate.beginObservation();
  const home = async () => {
    requireOwner(message);
    return (await send('Runtime.evaluate', { expression: ACCOUNT_READY_EXPRESSION,
      returnByValue: true, awaitPromise: false, userGesture: false }, current.sessionId)).value;
  };
  const before = await home();
  if (!before) throw probeError('qa_direct_ab_home_required');
  current.directABRunning = true;
  try {
    const a = await runDirectABPage(current, 'a');
    // Both observations are useful diagnostics even if A fails. Such a pair
    // must retain positive_control_failed, never an authentication claim.
    const b = await runDirectABPage(current, 'b');
    return finishQADirectAB(a, b, before, await home(), true, true);
  } finally { current.directABRunning = false; }
}

async function qualifyLiveSelf(message) {
  const current = requireOwner(message);
  if (process.env.AKU_X_QA_LIVE_SELF !== '1' || !qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse ||
      !current.gate.loginVisible || current.gate.observationStarted || current.earlyAttempted || current.qualificationAttempted || current.liveSelf) throw probeError('qa_live_gate_closed');
  current.gate.beginObservation();
  const state = current.liveSelf = { running: true, capture: null };
  try {
    const before = await send('Page.getFrameTree', {}, current.sessionId);
    state.capture = new QALiveSelfTransport(current.sessionId, before.frameId, before.loaderId);
    await send('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
      maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }, current.sessionId);
    const start = performance.now();
    const observation = await send('Runtime.evaluate', { expression: QA_LIVE_SELF_EXPRESSION,
      returnByValue: true, awaitPromise: true, userGesture: false }, current.sessionId);
    const after = await send('Page.getFrameTree', {}, current.sessionId);
    return finishQALiveSelf(observation, state.capture.finish(after.frameId, after.loaderId), performance.now() - start);
  } finally {
    try { await send('Network.disable', {}, current.sessionId); }
    finally { state.running = false; }
  }
}

async function qualifyBoundObservation(message, cookieWriteScope = false) {
  const current = requireOwner(message);
  if (!qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse || !current.gate.loginVisible ||
      current.gate.observationStarted || current.earlyAttempted || current.qualificationAttempted) throw probeError('qa_bound_gate_closed');
  current.gate.beginObservation(); current.earlyAttempted = true; current.earlyRunning = true;
  return boundCheckpoint(current, cookieWriteScope);
}

async function shortReaderCheckpoint(message) {
  const current = requireOwner(message);
  if (!qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse || !current.gate.loginVisible || current.gate.observationStarted) throw probeError('qa_short_gate_closed');
  current.gate.beginObservation(); current.earlyAttempted = true;
  const observation = await boundCheckpoint(current, false, { deferCookieMutation: true, onPrivateResult: () => {} });
  return shortQACheckpoint(observation);
}

async function runShortConcurrency(message, synchronize = false) {
  const current = requireOwner(message), s = current.shortFeasibility;
  if (!qualificationMode(runtimeState.driverMode) || !s || s.closed || s.ran || !current.gate.loginVisible ||
      process.env.AKU_X_QA_SHORT_TRANSFER_ALLOWED !== '1' || synchronize && process.env.AKU_X_QA_SHORT_SYNCHRONIZED !== '1') throw probeError('qa_short_transfer_denied');
  s.ran = true; current.gate.beginObservation(); current.earlyAttempted = true;
  const rendezvous = synchronize ? new QACheckpointRendezvous() : null;
  const counts = { reader: 0, collector: 0 };
  let readerWindow = 'not_checked';
  const experiment = await runQAShortFeasibility({
    prepareCollector: async () => {},
    check: async role => {
      const afterIdentity = rendezvous && counts[role]++ === 1 ? () => rendezvous.arrive(role) : undefined;
      if (role === 'collector') return collectorShortCheckpoint(current, afterIdentity);
      let privateResult = {};
      const observation = await boundCheckpoint(current, false, { deferCookieMutation: true, onPrivateResult: r => { privateResult = r; } }, afterIdentity);
      return { checkpoint: shortQACheckpoint(observation), ...privateResult };
    },
    seedOnce: async () => {
      let selected;
      try {
        s.exporting = true;
        selected = await send('Network.getCookies', qaCookieRequest('api_v1'), current.sessionId);
        s.exporting = false;
        await s.transport.send('Network.setCookies', { cookies: selected }, s.sessionId, () => ({ acknowledged: true }));
      } finally { s.exporting = false; selected = null; }
    },
    readerPreserved: async () => {
      const targets = await send('Target.getTargets');
      const preserved = targets.targetIds.length === 1 && targets.targetIds[0] === current.targetId &&
        (await send('Browser.getWindowForTarget', { targetId: current.targetId })).windowId === s.windowId;
      readerWindow = preserved ? 'preserved' : 'changed';
      return preserved;
    },
    closeCollector: () => closeShortCollector(current),
  });
  return rendezvous ? synchronizedQAReport(experiment, rendezvous, readerWindow) : experiment;
}

async function webABAPhase(current, stage, frameId, action) {
  const state = current.webABA;
  state.stage = stage; state.reloaded = false; state.events = 0;
  state.capture = new QADirectCapture(stage === 'b' ? 'b' : 'a', frameId,
    requestId => send('Network.getResponseBody', { requestId }, current.sessionId));
  state.endsAt = Date.now() + 10000;
  await action();
  await new Promise(resolve => setTimeout(resolve, Math.max(0, state.endsAt - Date.now())));
  return state.capture.finish();
}

async function qualifyWebABA(message, qualify401 = false) {
  const current = requireOwner(message);
  if (!qualificationMode(runtimeState.driverMode) || !runtimeState.retainedQAScope?.reuse || !current.gate.loginVisible ||
      current.gate.observationStarted || current.webABAAttempted) throw probeError('qa_web_aba_gate_closed');
  current.webABAAttempted = true; current.gate.beginObservation();
  const home = async () => (await send('Runtime.evaluate', { expression: ACCOUNT_READY_EXPRESSION,
    returnByValue: true, awaitPromise: false, userGesture: false }, current.sessionId)).value;
  const readHook = () => send('Runtime.evaluate', { expression: QA_WEB_SNAPSHOT,
    returnByValue: true, awaitPromise: false, userGesture: false }, current.sessionId);
  const before = await home();
  if (!before) throw probeError('qa_web_home_required');
  const state = current.webABA = { running: true, stage: 'setup', reloaded: false, bStarted: false, qualify401, rejection: null };
  let a, b = null, aAgain, hook, removed = false;
  try {
    state.scriptId = (await send('Page.addScriptToEvaluateOnNewDocument', { source: QA_WEB_INSTALL }, current.sessionId)).identifier;
    await send('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
      maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }, current.sessionId);
    await send('Network.setCacheDisabled', { cacheDisabled: true }, current.sessionId);
    const { frameId } = await send('Page.getFrameTree', {}, current.sessionId);
    const reload = async () => { await send('Page.reload', { ignoreCache: false }, current.sessionId); state.reloaded = true; };
    a = await webABAPhase(current, 'a', frameId, reload);
    hook = await readHook();
    if (a.status === 200 && a.body?.category === 'settings_match' && a.diagnostic === 'http_200_unqualified' &&
        hook.candidates === 1 && hook.prepared && !hook.attempted && !hook.rejected) {
      b = await webABAPhase(current, 'b', frameId, async () => {
        const started = await send('Runtime.evaluate', { expression: QA_WEB_START, returnByValue: true,
          awaitPromise: false, userGesture: false }, current.sessionId);
        state.bStarted = true;
        if (!started.value) throw probeError('qa_web_b_not_started');
      });
    }
    const disposed = await send('Runtime.evaluate', { expression: QA_WEB_DISPOSE,
      returnByValue: true, awaitPromise: false, userGesture: false }, current.sessionId);
    hook = await readHook();
    await send('Page.removeScriptToEvaluateOnNewDocument', { identifier: state.scriptId }, current.sessionId);
    state.scriptId = null; removed = disposed.value && hook.disposed;
    aAgain = await webABAPhase(current, 'a_again', frameId, reload);
    const base = finishQAWebABA(a, b, aAgain, hook, before, await home(), removed);
    return qualify401 ? finishQAWeb401(base, state.rejection) : base;
  } finally {
    // The owned Job remains the final cleanup authority on any partial failure.
    if (state.scriptId) {
      await send('Page.removeScriptToEvaluateOnNewDocument', { identifier: state.scriptId }, current.sessionId).catch(() => {});
    }
    await send('Network.disable', {}, current.sessionId).catch(() => {});
    state.running = false;
  }
}

async function execute(message) {
  if (!isObject(message) || !Number.isSafeInteger(message.id) || message.id <= 0 || typeof message.op !== 'string') {
    throw probeError('rpc_invalid');
  }
  if (process.env.AKU_X_QA_LIVE_SELF === '1' && !['qa_profile_scope', 'launch', 'login_state', 'qualify_live_self', 'release'].includes(message.op)) throw probeError('qa_live_mode_operation_denied');
  if(message.op==='manual_seed_bootstrap')return seedRetainedManualPilot(message);
  if(message.op==='manual_open_reader')return openRetainedManualReader(message);
  if(message.op.startsWith('manual_'))return executeManualOwnerRPC(message);
  if(runtimeState.active?.manualStartupPort&&message.op!=='release')throw probeError('manual_owner_operation_denied');
  switch (message.op) {
    case 'qualify_live_self':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyLiveSelf(message);
    case 'prepare_short_concurrency':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return prepareShortConcurrency(message);
    case 'qualify_short_reader_checkpoint':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return shortReaderCheckpoint(message);
    case 'qualify_short_concurrency':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return runShortConcurrency(message);
    case 'qualify_synchronized_short_concurrency':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return runShortConcurrency(message, true);
    case 'close_short_collector':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return { closed: await closeShortCollector(requireOwner(message)) };
    case 'qualify_bound_cookie_writes':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyBoundObservation(message, true);
    case 'qualify_bound_observation':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyBoundObservation(message);
    case 'qualify_web_401':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyWebABA(message, true);
    case 'qualify_web_aba':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyWebABA(message);
    case 'qualify_direct_ab':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyDirectAB(message);
    case 'qualify_auth_control':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifyAuthControl(message);
    case 'qualify_settings_types':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifySource(message, true);
    case 'qualify_source':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return qualifySource(message);
    case 'bootstrap_early_diagnostics':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return earlyBootstrapDiagnostics(message);
    case 'bootstrap_early_current_self':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return earlyBootstrapCurrentSelf(message);
    case 'bootstrap_current_self':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return bootstrapCurrentSelf(message);
    case 'qa_profile_scope':
      exactKeys(message, ['id', 'op', 'nonce', 'reuse'], 'rpc_fields_invalid');
      return acceptRetainedQAScope(message);
    case 'launch':
      exactKeys(message, ['id', 'op', 'role', 'nativeReaderStartup'], 'rpc_fields_invalid');
      return launchChrome(message);
    case 'login_state':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return loginState(message);
    case 'start_observation':
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      return startObservation(message);
    case 'authorize_observation':
      exactKeys(message, ['id', 'op', 'owner', 'oldPID', 'nonce'], 'rpc_fields_invalid');
      return authorizeObservation(message);
    case 'release': {
      exactKeys(message, ['id', 'op', 'owner'], 'rpc_fields_invalid');
      requireOwner(message);
      if (runtimeState.active.shortFeasibility && !runtimeState.active.shortFeasibility.closed) await closeShortCollector(runtimeState.active);
      const result = await closeChrome();
      if (!result.exited) throw probeError('chrome_exit_unverified');
      runtimeState.active = null;
      runtimeState.protocolOwner = 0;
      return result;
    }
    default:
      throw probeError('rpc_operation_unsupported');
  }
}

async function handleControlLine(raw) {
  let message;
  try {
    try { message = parseJSONBytes(raw, MAX_CONTROL_LINE_BYTES); }
    catch { throw probeError('control_json_invalid'); }
    if (!bootstrapDone) {
      exactKeys(message, ['op', 'withholdReady'], 'bootstrap_invalid');
      if (message.op !== 'fixture_bootstrap' || message.withholdReady !== false) throw probeError('bootstrap_invalid');
      bootstrapDone = true;
      emit({ ready: true });
      return;
    }
    const id = message.id;
    try {
      if(typeof message.op==='string'&&message.op.startsWith('manual_')){
        // Only the bounded fixed manual operations bypass the serialized QA
        // chain. Per-role state rejects replay and other modes remain serial.
        void executeManualReply(message);
      }else emitReply(id,await execute(message));
    }
    catch (error) { emitReply(id, undefined, safeCode(error)); }
  } finally {
    message = null;
    raw.fill(0);
  }
}

function onControlData(chunk) {
  if (!(chunk instanceof Uint8Array) || chunk.byteLength > MAX_CONTROL_BUFFER_BYTES ||
      controlBytes.byteLength + chunk.byteLength > MAX_CONTROL_BUFFER_BYTES) {
    shutdown('control_input_too_large');
    return;
  }
  controlBytes = Buffer.concat([controlBytes, Buffer.from(chunk)]);
  let lines = 0;
  for (let end; (end = controlBytes.indexOf(0x0a)) >= 0;) {
    if (++lines > MAX_CONTROL_LINES_PER_CHUNK) { shutdown('control_too_many_lines'); return; }
    const line = Buffer.from(controlBytes.subarray(0, end));
    const previous = controlBytes;
    controlBytes = Buffer.from(previous.subarray(end + 1));
    previous.fill(0);
    if (line.byteLength < 1 || line.byteLength > MAX_CONTROL_LINE_BYTES || ++controlLines > MAX_CONTROL_LINES_TOTAL) {
      line.fill(0);
      shutdown('control_line_invalid');
      return;
    }
    controlChain = controlChain.then(() => handleControlLine(line)).catch(() => shutdown('control_protocol_failed'));
  }
}

function shutdown() {
  if (shuttingDown) return;
  shuttingDown = true;
  runtimeState.cdpFrames.reset();
  rejectPending('control_closed');
  void closeChrome().finally(() => { process.exitCode = 0; });
}

export async function runNativeMetadataProbe(mode = 'single_phase') {
  if (!['single_phase', 'two_phase', 'two_phase_account_settings', 'retained_account_settings', 'retained_bootstrap', 'retained_source_qualification'].includes(mode) || runtimeState.driverMode !== 'single_phase' || runtimeState.child || runtimeState.twoPhaseState) {
    throw probeError('driver_mode_invalid');
  }
  runtimeState.driverMode = mode;
  const args = process.argv.slice(2);
  if (process.argv.length !== 5 || args.length !== 3 || args.some(value => !value)) {
    process.exitCode = 2;
    return;
  }
  runtimeState.executable = resolve(args[0]);
  runtimeState.qaRoot = resolve(args[1]);
  runtimeState.controlOrigin = parseControlOrigin(args[2]);
  process.stdin.on('data', onControlData);
  process.stdin.on('end', shutdown);
  process.stdin.on('error', shutdown);
  process.on('SIGTERM', shutdown);
  process.on('SIGINT', shutdown);
  await new Promise(() => {});
}

const invokedPath = process.argv[1] ? resolve(process.argv[1]) : '';
if (invokedPath && pathToFileURL(invokedPath).href === import.meta.url) {
  void runNativeMetadataProbe('single_phase').catch(() => { process.exitCode = 2; process.stdin.destroy(); });
}
