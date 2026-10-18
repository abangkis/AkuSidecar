// A dedicated, headless, loopback-only Chrome driver for synthetic X body
// evidence. This module deliberately does not import the older Chrome driver.
import { spawn } from 'node:child_process';
import { mkdir, lstat, realpath } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createFixtureXEvidenceIngress, parseStrictJSON } from './x-evidence-ingress.mjs';
import { readQualifiedFixtureXBody, canonicalizeFixtureXBody } from './x-evidence-body.mjs';
import { awaitIssuedAtCatchUp } from './x-evidence-request-clock.mjs';

const MAX_CONTROL_LINE_BYTES = 65_536;
const MAX_CONTROL_BUFFER_BYTES = 131_072;
const MAX_CONTROL_LINES_PER_CHUNK = 8;
const MAX_CDP_FRAME_BYTES = 262_144;
const MAX_CDP_INPUT_CHUNK_BYTES = 1_048_576;
const MAX_CDP_FRAMES_PER_CHUNK = 128;
const MAX_PENDING_COMMANDS = 32;
const MAX_PENDING_BODY_READS = 1;
const PHASE_WINDOW_MS = 5_000;
const COMMAND_TIMEOUT_MS = 4_000;
const BODY_WAIT_TIMEOUT_MS = 3_500;
const MAX_TRIGGER_DRAIN_BYTES = 65_536;
const OBSERVATION_SCHEMA = 'aku.x.browser-self.observation.v1';
const OPERATION = 'x.loopback-self.fixture/v1';
const OPERATION_PATH = '/x-fixture-operation';
const NETWORK_FAILURE_CODES = new Map([
  ['net::ERR_ABORTED', 'aborted'], ['net::ERR_FAILED', 'failed'],
  ['net::ERR_BLOCKED_BY_ORB', 'blocked_by_orb'],
  ['net::ERR_INCOMPLETE_CHUNKED_ENCODING', 'incomplete_chunked_encoding'],
  ['net::ERR_CONTENT_LENGTH_MISMATCH', 'content_length_mismatch'],
  ['net::ERR_CONNECTION_RESET', 'connection_reset'],
  ['net::ERR_EMPTY_RESPONSE', 'empty_response'],
  ['net::ERR_INVALID_HTTP_RESPONSE', 'invalid_http_response'],
]);
const REQUEST_KEYS = Object.freeze([
  'Version', 'Source', 'Operation', 'Phase', 'Challenge', 'RequestID', 'Origin', 'Path',
  'ProfileID', 'OwnerID', 'OwnerEpoch', 'OwnerGeneration', 'IssuedAt',
]);
const CDP_METHOD_CODES = Object.freeze({
  'Target.createBrowserContext': 'target_create_context',
  'Target.createTarget': 'target_create_target',
  'Target.attachToTarget': 'target_attach_to_target',
  'Target.disposeBrowserContext': 'target_dispose_context',
  'Page.enable': 'page_enable',
  'Page.navigate': 'page_navigate',
  'Runtime.enable': 'runtime_enable',
  'Runtime.evaluate': 'runtime_evaluate',
  'Network.enable': 'network_enable',
  'Network.setCacheDisabled': 'network_cache_bypass',
  'Network.setBypassServiceWorker': 'network_service_worker_bypass',
  'Network.getResponseBody': 'network_get_response_body',
  'Performance.enable': 'performance_enable',
  'Performance.getMetrics': 'performance_get_metrics',
  'Browser.close': 'browser_close',
});

const methodSet = new Set(Object.keys(CDP_METHOD_CODES));
const cdpFrameDecoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
const controlFrameDecoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });

let executable = '';
let qaRoot = '';
let origin = '';
let ownedProfile = '';
let child = null;
let childExited = false;
let childClosePromise = null;
let owner = 0;
let nextOwner = 0;
let nextCommandId = 0;
let observationSequence = 0;
let active = null;
let bootstrapDone = false;
let shuttingDown = false;
let lastClockMs = 0;
let controlBytes = Buffer.alloc(0);
let controlLines = 0;
let controlChain = Promise.resolve();
let cdpFrame = Buffer.allocUnsafe(MAX_CDP_FRAME_BYTES);
let cdpFrameLength = 0;
let cdpProtocolFailure = null;
const pending = new Map();
const usedChallenges = new Set();
const usedRequestIDs = new Set();
const MAX_USED_REQUESTS = 64;

function fixedError(code) {
  return new Error(`xev_${code}`);
}

function object(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function own(value, key) {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function safeIdentifier(value, limit = 256) {
  return typeof value === 'string' && value.length > 0 && value.length <= limit &&
    !/[\u0000-\u001f\u007f]/u.test(value);
}

function samePath(left, right) {
  return resolve(left).toLocaleLowerCase('en-US') === resolve(right).toLocaleLowerCase('en-US');
}

function monotonicWallClock() {
  const value = Date.now();
  if (!Number.isSafeInteger(value) || value < lastClockMs) throw fixedError('clock_invalid');
  lastClockMs = value;
  return value;
}

function parseOrigin(value) {
  if (typeof value !== 'string' || value.length > 128) throw fixedError('origin_invalid');
  let parsed;
  try { parsed = new URL(value); } catch { throw fixedError('origin_invalid'); }
  if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port ||
      parsed.username || parsed.password || parsed.pathname !== '/' || parsed.search || parsed.hash ||
      parsed.origin !== value) throw fixedError('origin_invalid');
  return parsed.origin;
}

async function validateDriverPaths() {
  if (!isAbsolute(executable) || !isAbsolute(qaRoot)) throw fixedError('arguments_invalid');
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
  const resolvedProject = await realpath(projectRoot).catch(() => '');
  const expectedQAParent = resolvedProject
    ? resolve(resolvedProject, 'runtime', 'qa', 'auth-session-sidecar') : '';
  const resolvedExpectedQA = await realpath(expectedQAParent).catch(() => '');
  const rootStat = await lstat(qaRoot).catch(() => null);
  const rootReal = await realpath(qaRoot).catch(() => '');
  const rootRel = rootReal && resolvedExpectedQA ? relative(resolvedExpectedQA, rootReal) : '';
  if (!resolvedProject || !resolvedExpectedQA || !rootStat?.isDirectory() || rootStat.isSymbolicLink() ||
      !samePath(rootReal, qaRoot) || !rootRel || rootRel === '..' || rootRel.startsWith(`..${sep}`) ||
      isAbsolute(rootRel) ||
      !/^run-[A-Za-z0-9._-]{1,80}$/u.test(rootReal.split(/[\\/]/u).at(-1))) {
    throw fixedError('qa_root_invalid');
  }
  const rel = relative(resolvedProject, rootReal);
  if (!rel || rel === '..' || rel.startsWith(`..${sep}`) || isAbsolute(rel)) throw fixedError('qa_root_invalid');

  const chromeStat = await lstat(executable).catch(() => null);
  const chromeReal = await realpath(executable).catch(() => '');
  if (!chromeStat?.isFile() || !chromeReal || !samePath(chromeReal, executable)) throw fixedError('chrome_path_invalid');
  executable = chromeReal;
  qaRoot = rootReal;
  ownedProfile = join(qaRoot, 'shared');
  const profileStat = await lstat(ownedProfile).catch(() => null);
  if (profileStat) throw fixedError('profile_must_be_new');
}

function emit(value) {
  let encoded;
  try { encoded = Buffer.from(`${JSON.stringify(value)}\n`, 'utf8'); }
  catch { return; }
  if (encoded.byteLength > MAX_CONTROL_LINE_BYTES) {
    encoded = Buffer.from(`${JSON.stringify({ id: value?.id ?? 0, ok: false, code: 'fixture_output_too_large' })}\n`, 'utf8');
  }
  process.stdout.write(encoded);
}

function emitReply(id, result = undefined, code = undefined) {
  if (!Number.isSafeInteger(id) || id <= 0) return;
  if (code) emit({ id, ok: false, code });
  else emit({ id, ok: true, result });
}

function safeErrorCode(error) {
  const value = typeof error?.message === 'string' ? error.message : '';
  if (/^xev_[a-z0-9_]{1,80}$/u.test(value)) return value.slice(4);
  if (/^body_[a-z0-9_]{1,80}$/u.test(value)) return value;
  return 'fixture_bridge_failed';
}

function validateFrameObject(value) {
  if (!object(value)) throw fixedError('protocol_message_invalid');
  return value;
}

function parseJSONFrame(bytes, decoder, limitSet) {
  let text;
  try { text = decoder.decode(bytes); } catch { throw fixedError('invalid_utf8'); }
  try { return validateFrameObject(parseStrictJSON(text, limitSet)); }
  catch (error) {
    if (error?.message?.startsWith('xev_')) throw error;
    throw fixedError('malformed_json');
  }
}

function cdpWrite(message) {
  if (cdpProtocolFailure) throw fixedError(cdpProtocolFailure);
  if (!child || childExited || !child.stdio?.[3] || pending.size >= MAX_PENDING_COMMANDS) {
    throw fixedError('cdp_unavailable');
  }
  const line = Buffer.from(`${JSON.stringify(message)}\0`, 'utf8');
  if (line.byteLength > MAX_CDP_FRAME_BYTES) throw fixedError('cdp_command_too_large');
  try { child.stdio[3].write(line); } catch { throw fixedError('cdp_write_failed'); }
}

function projectCDPResult(method, result) {
  if (!object(result)) throw fixedError('cdp_result_invalid');
  switch (method) {
    case 'Target.createBrowserContext':
      if (!safeIdentifier(result.browserContextId)) throw fixedError('target_context_invalid');
      return { browserContextId: result.browserContextId };
    case 'Target.createTarget':
      if (!safeIdentifier(result.targetId)) throw fixedError('target_id_invalid');
      return { targetId: result.targetId };
    case 'Target.attachToTarget':
      if (!safeIdentifier(result.sessionId)) throw fixedError('target_session_invalid');
      return { sessionId: result.sessionId };
    case 'Page.navigate':
      if (own(result, 'errorText')) throw fixedError('page_navigation_failed');
      return { navigated: true };
    case 'Performance.getMetrics': {
      if (!Array.isArray(result.metrics) || result.metrics.length > 128) throw fixedError('performance_metrics_invalid');
      const timestamps = result.metrics.filter(item => object(item) && item.name === 'Timestamp');
      if (timestamps.length !== 1 || typeof timestamps[0].value !== 'number' ||
          !Number.isFinite(timestamps[0].value) || timestamps[0].value <= 0) {
        throw fixedError('performance_timestamp_invalid');
      }
      return { timestamp: timestamps[0].value };
    }
    case 'Runtime.evaluate': {
      if (own(result, 'exceptionDetails')) throw fixedError('runtime_evaluation_failed');
      const remote = result.result;
      if (!object(remote) || remote.type !== 'boolean' || typeof remote.value !== 'boolean') {
        throw fixedError('runtime_result_invalid');
      }
      return { value: remote.value };
    }
    case 'Network.getResponseBody':
      return canonicalizeFixtureXBody(result);
    case 'Target.disposeBrowserContext':
    case 'Page.enable':
    case 'Runtime.enable':
    case 'Network.enable':
    case 'Network.setCacheDisabled':
    case 'Network.setBypassServiceWorker':
    case 'Performance.enable':
    case 'Browser.close':
      return Object.freeze({ acknowledged: true });
    default:
      throw fixedError('cdp_method_invalid');
  }
}

function send(method, params = {}, sessionId = undefined, projectResult = undefined) {
  if (cdpProtocolFailure) return Promise.reject(fixedError(cdpProtocolFailure));
  if (!methodSet.has(method)) return Promise.reject(fixedError('cdp_method_invalid'));
  if (!object(params)) return Promise.reject(fixedError('cdp_params_invalid'));
  if (pending.size >= MAX_PENDING_COMMANDS) return Promise.reject(fixedError('cdp_pending_limit'));
  const id = ++nextCommandId;
  if (!Number.isSafeInteger(id) || id <= 0) return Promise.reject(fixedError('cdp_id_limit'));
  const promise = new Promise((resolvePromise, rejectPromise) => {
    const timer = setTimeout(() => {
      if (!pending.has(id)) return;
      failCDPProtocol(`${CDP_METHOD_CODES[method]}_timeout`);
    }, COMMAND_TIMEOUT_MS);
    pending.set(id, { method, sessionId, resolve: resolvePromise, reject: rejectPromise, timer,
      projectResult: projectResult ?? (value => projectCDPResult(method, value)) });
    try {
      cdpWrite({ id, method, params, ...(sessionId ? { sessionId } : {}) });
    } catch (error) {
      if (error?.message === 'xev_cdp_write_failed') failCDPProtocol('cdp_write_failed');
      clearTimeout(timer);
      pending.delete(id);
      rejectPromise(error);
    }
  });
  return promise;
}

function rejectPending(code) {
  for (const [id, item] of pending) {
    clearTimeout(item.timer);
    pending.delete(id);
    item.reject(fixedError(code));
  }
}

function markIngressUnavailable(reason) {
  active?.ingress?.loss();
  active?.ingress?.close();
  if (active && !active.failureCode) active.failureCode = /^([a-z0-9_]{1,80})$/u.test(reason) ? reason : 'source_lost';
}

function rejectActiveObservation(code) {
  if (active && !active.failureCode) active.failureCode = code;
  active?.body?.reject(fixedError(code));
}

function failCDPProtocol(code) {
  if (!cdpProtocolFailure) {
    cdpProtocolFailure = /^xev_[a-z0-9_]{1,80}$/u.test(code) ? code.slice(4) :
      /^[a-z0-9_]{1,80}$/u.test(code) ? code : 'cdp_protocol_failed';
  }
  cdpFrame.fill(0);
  cdpFrameLength = 0;
  if (active) {
    active.protocolFailed = true;
    active.protocolFailureCode = cdpProtocolFailure;
    if (!active.failureCode) active.failureCode = cdpProtocolFailure;
    active.ingress?.loss();
    active.ingress?.close();
    active.body?.reject(fixedError(cdpProtocolFailure));
  }
  rejectPending(cdpProtocolFailure);
}

function createDeferred() {
  let resolve;
  let reject;
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
  promise.catch(() => {});
  return { promise, resolve, reject };
}

function dispatchEvent(message, rawFrame) {
  const method = message.method;
  if (typeof method !== 'string' || method.length > 128 || !object(message.params)) {
    throw fixedError('cdp_event_invalid');
  }
  if (method === 'Page.loadEventFired') {
    if (active?.pageLoad && message.sessionId === active.sessionId) {
      active.pageLoad.resolve(true);
      active.pageLoad = null;
    }
    return;
  }
  if (!active?.ingress) return;
  const networkish = method.startsWith('Network.') ||
    method === 'Target.targetDestroyed' || method === 'Target.detachedFromTarget';
  if (!networkish) return;

  if (message.sessionId === active.sessionId && object(message.params)) {
    if (method === 'Network.requestWillBeSent' && object(message.params.request) &&
        safeIdentifier(message.params.requestId) &&
        message.params.request.url === origin + OPERATION_PATH &&
        message.params.request.method === 'POST' && !active.operationRequestId) {
      active.operationRequestId = message.params.requestId;
    }
  }
  if (method.startsWith('Network.') && message.sessionId !== active.sessionId) {
    throw fixedError('cdp_event_session_mismatch');
  }

  const joined = Buffer.allocUnsafe(rawFrame.byteLength + 1);
  rawFrame.copy(joined, 0);
  joined[joined.length - 1] = 0;
  const status = active.ingress.push(joined);
  joined.fill(0);
  if (status.unavailable) {
    if (['malformed_json', 'invalid_utf8', 'scope_mismatch', 'target_lost', 'source_detached'].includes(status.reason)) {
      failCDPProtocol(`ingress_${status.reason}`);
      return;
    }
    rejectActiveObservation(`ingress_${status.reason ?? 'unavailable'}`);
    return;
  }
  if (method === 'Network.loadingFailed' && message.sessionId === active.sessionId &&
      message.params?.requestId === active.operationRequestId) {
    // Reduce exact known network errors immediately. Never retain or export
    // the protocol's raw error text or blocked URL.
    const code = NETWORK_FAILURE_CODES.get(message.params.errorText) ?? 'unknown';
    rejectActiveObservation(`request_failed_${code}`);
    return;
  }

  if (method.startsWith('Network.') && message.sessionId === active.sessionId &&
      object(message.params) && safeIdentifier(message.params.requestId)) {
    const ticket = active.ingress.claimCompletedBodyRequest(message.params.requestId);
    if (ticket) {
      if (active.bodyReadCount >= MAX_PENDING_BODY_READS || active.bodyReady) {
        rejectActiveObservation('multiple_body_requests');
        return;
      }
      active.bodyReadCount++;
      const current = active;
      current.bodyCommandPending = true;
      void readQualifiedFixtureXBody({
        ingress: current.ingress,
        ticket,
        getResponseBody(requestId, projectBody) {
          return send('Network.getResponseBody', { requestId }, current.sessionId,
            value => projectBody(value));
        },
      }).then(body => {
        if (active !== current || current.failureCode) return;
        current.bodyReady = true;
        current.body.resolve(body);
      }).catch(error => {
        if (active === current) rejectActiveObservation(safeErrorCode(error));
      }).finally(() => { current.bodyCommandPending = false; });
    }
  }
}

function dispatchCDP(message, rawFrame) {
  if (own(message, 'method')) {
    dispatchEvent(message, rawFrame);
    return;
  }
  const responseKeys = Object.keys(message);
  if (!Number.isSafeInteger(message.id) || message.id <= 0 ||
      !own(message, 'result') && !own(message, 'error') ||
      own(message, 'result') === own(message, 'error') ||
      responseKeys.some(key => !['id', 'sessionId', 'result', 'error'].includes(key))) {
    throw fixedError('cdp_message_invalid');
  }
  const item = pending.get(message.id);
  if (!item) throw fixedError('cdp_response_unexpected');
  if (message.sessionId !== item.sessionId) {
    throw fixedError('cdp_session_mismatch');
  }
  pending.delete(message.id);
  clearTimeout(item.timer);
  if (own(message, 'error')) {
    item.reject(fixedError(`${CDP_METHOD_CODES[item.method]}_failed`));
    return;
  }
  try { item.resolve(item.projectResult(message.result)); }
  catch (error) {
    const messageText = typeof error?.message === 'string' ? error.message : '';
    item.reject(/^xev_[a-z0-9_]{1,80}$/u.test(messageText) || /^body_[a-z0-9_]{1,80}$/u.test(messageText)
      ? error : fixedError('cdp_result_invalid'));
  }
}

function acceptCDPFrame() {
  if (cdpFrameLength === 0) throw fixedError('cdp_frame_empty');
  const rawFrame = Buffer.from(cdpFrame.subarray(0, cdpFrameLength));
  let message;
  try { message = parseJSONFrame(rawFrame, cdpFrameDecoder, { maxJSONDepth: 32, maxJSONTokens: 16_384 }); }
  catch (error) {
    cdpFrame.fill(0, 0, cdpFrameLength);
    cdpFrameLength = 0;
    throw error;
  }
  cdpFrame.fill(0, 0, cdpFrameLength);
  cdpFrameLength = 0;
  try { dispatchCDP(message, rawFrame); }
  finally { rawFrame.fill(0); message = null; }
}

function onCDPData(chunk) {
  if (cdpProtocolFailure) return;
  if (!(chunk instanceof Uint8Array) || chunk.byteLength > MAX_CDP_INPUT_CHUNK_BYTES) {
    failCDPProtocol('cdp_input_too_large');
    return;
  }
  let frames = 0;
  for (const byte of chunk) {
    if (byte === 0) {
      if (++frames > MAX_CDP_FRAMES_PER_CHUNK) {
        failCDPProtocol('cdp_too_many_frames');
        return;
      }
      try { acceptCDPFrame(); }
      catch (error) {
        failCDPProtocol(safeErrorCode(error));
        return;
      }
    } else {
      if (cdpFrameLength >= MAX_CDP_FRAME_BYTES) {
        cdpFrame.fill(0);
        cdpFrameLength = 0;
        failCDPProtocol('cdp_frame_too_large');
        return;
      }
      cdpFrame[cdpFrameLength++] = byte;
    }
  }
}

function bindChromeProcess(processChild) {
  processChild.on('error', () => {
    childExited = true;
    failCDPProtocol('chrome_process_error');
  });
  processChild.on('exit', () => {
    childExited = true;
    failCDPProtocol('chrome_exited');
    if (active?.owner) emit({ event: 'chrome_exit', owner: active.owner });
  });
  processChild.stdio[3]?.on('error', () => {
    failCDPProtocol('cdp_pipe_error');
  });
  processChild.stdio[4]?.on('error', () => {
    failCDPProtocol('cdp_pipe_error');
  });
  processChild.stdio[4]?.on('data', onCDPData);
  processChild.stdio[3]?.on('close', () => {
    if (!childExited) failCDPProtocol('cdp_pipe_closed');
  });
  processChild.stdio[4]?.on('close', () => {
    if (!childExited) failCDPProtocol('cdp_pipe_closed');
  });
}

function awaitWithTimeout(promise, code, durationMs) {
  let timer;
  return Promise.race([
    promise,
    new Promise((_, reject) => { timer = setTimeout(() => reject(fixedError(code)), durationMs); }),
  ]).finally(() => clearTimeout(timer));
}

async function waitForPageLoad(sessionId) {
  if (active?.pageLoad) throw fixedError('page_load_pending');
  const deferred = createDeferred();
  active.pageLoad = deferred;
  await awaitWithTimeout(deferred.promise, 'page_load_timeout', COMMAND_TIMEOUT_MS);
  if (active?.sessionId !== sessionId) throw fixedError('target_session_changed');
}

async function startChrome(message) {
  if (cdpProtocolFailure) throw fixedError(cdpProtocolFailure);
  if (owner || child || message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw fixedError('collector_launch_invalid');
  }
  await validateDriverPaths();
  try { await mkdir(ownedProfile, { recursive: false, mode: 0o700 }); }
  catch { throw fixedError('profile_create_failed'); }
  const profileStat = await lstat(ownedProfile).catch(() => null);
  const profileReal = await realpath(ownedProfile).catch(() => '');
  const profileRel = relative(qaRoot, profileReal);
  if (!profileStat?.isDirectory() || profileStat.isSymbolicLink() || !samePath(profileReal, ownedProfile) ||
      profileRel !== 'shared') throw fixedError('profile_path_invalid');

  const flags = [
    `--user-data-dir=${profileReal}`,
    '--remote-debugging-pipe', '--no-first-run', '--no-default-browser-check',
    '--disable-extensions', '--disable-background-networking', '--disable-component-update',
    '--disable-sync', '--disable-background-mode', '--disable-default-apps', '--no-proxy-server',
    '--metrics-recording-only',
    '--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE localhost, EXCLUDE 127.0.0.1',
    '--headless=new', '--no-startup-window',
  ];
  const processChild = spawn(executable, flags, {
    windowsHide: true,
    stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'],
  });
  if (!processChild.pid) throw fixedError('chrome_spawn_failed');
  child = processChild;
  childExited = false;
  bindChromeProcess(processChild);
  const created = await send('Target.createBrowserContext', { disposeOnDetach: true });
  const target = await send('Target.createTarget', { url: 'about:blank', browserContextId: created.browserContextId });
  const attached = await send('Target.attachToTarget', { targetId: target.targetId, flatten: true });
  const newOwner = ++nextOwner;
  owner = newOwner;
  active = {
    owner: newOwner,
    pid: processChild.pid,
    browserContextId: created.browserContextId,
    targetId: target.targetId,
    sessionId: attached.sessionId,
    profileId: profileReal,
    cacheBypassConfigured: false,
    loginDone: false,
    ingress: null,
    pageLoad: null,
    body: null,
    bodyReady: false,
    bodyReadCount: 0,
    bodyCommandPending: false,
    failureCode: null,
    protocolFailed: false,
    protocolFailureCode: null,
    profileFailure: false,
  };
  await send('Page.enable', {}, active.sessionId);
  await send('Runtime.enable', {}, active.sessionId);
  await send('Network.enable', {}, active.sessionId);
  await send('Network.setCacheDisabled', { cacheDisabled: true }, active.sessionId);
  await send('Network.setBypassServiceWorker', { bypass: true }, active.sessionId);
  active.cacheBypassConfigured = true;
  await send('Performance.enable', {}, active.sessionId);
  return { owner: newOwner, pid: processChild.pid, windowCount: 0, bootstrapCount: 0, nativeReaderStartup: false };
}

function requireOwner(message, { allowProtocolFailure = false } = {}) {
  if (!active || message.owner !== active.owner || childExited) throw fixedError('owner_unavailable');
  if (!allowProtocolFailure && active.protocolFailed) {
    throw fixedError(active.protocolFailureCode ?? 'cdp_protocol_failed');
  }
  return active;
}

async function evaluateBoolean(sessionId, expression, { allowFalse = false } = {}) {
  const result = await send('Runtime.evaluate', {
    expression,
    awaitPromise: true,
    returnByValue: true,
    userGesture: false,
  }, sessionId);
  if (result.value === false && allowFalse) return false;
  if (result.value !== true) throw fixedError('runtime_result_invalid');
  return true;
}

async function login(message) {
  const current = requireOwner(message);
  if (current.loginDone) throw fixedError('login_already_completed');
  current.pageLoad = createDeferred();
  const loaded = current.pageLoad;
  const navigation = send('Page.navigate', { url: origin + '/' }, current.sessionId);
  await navigation;
  await awaitWithTimeout(loaded.promise, 'page_load_timeout', COMMAND_TIMEOUT_MS);
  current.pageLoad = null;
  if (current.failureCode) throw fixedError(current.failureCode);
  const expression = `(async()=>{await fetch(${JSON.stringify(origin + '/login')},{method:'POST',credentials:'include',cache:'no-store'});return true})()`;
  await evaluateBoolean(current.sessionId, expression);
  current.loginDone = true;
  return { loggedIn: true };
}

async function validateRequest(rawRequest, current) {
  if (!object(rawRequest)) throw fixedError('request_invalid');
  const keys = Object.keys(rawRequest);
  if (keys.length !== REQUEST_KEYS.length || keys.some(key => !REQUEST_KEYS.includes(key))) {
    throw fixedError('request_invalid');
  }
  const request = Object.assign(Object.create(null), rawRequest);
  if (request.Version !== 1 || request.Source !== 'x' || request.Operation !== OPERATION ||
      !['initial', 'recheck'].includes(request.Phase) || !safeIdentifier(request.Challenge, 128) ||
      !safeIdentifier(request.RequestID, 128) || request.Origin !== origin || request.Path !== OPERATION_PATH ||
      request.ProfileID !== current.profileId || request.OwnerID !== `fixture-owner-${current.pid}` ||
      !safeIdentifier(request.OwnerEpoch, 128) || !Number.isSafeInteger(request.OwnerGeneration) ||
      request.OwnerGeneration <= 0 || typeof request.IssuedAt !== 'string' ||
      !/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/u.test(request.IssuedAt)) {
    throw fixedError('request_scope_invalid');
  }
  let timing;
  try {
    timing = await awaitIssuedAtCatchUp({
      issuedAt: request.IssuedAt,
      clock() {
        try { return monotonicWallClock(); }
        catch { throw new Error('request_clock_rollback'); }
      },
      maxCatchUpMs: 50,
      ttlMs: PHASE_WINDOW_MS,
    });
  } catch (error) {
    const code = error?.message;
    if (['request_future', 'request_expired', 'request_clock_invalid', 'request_clock_rollback'].includes(code)) {
      throw fixedError(code);
    }
    throw fixedError('request_clock_failed');
  }
  const deadlineMs = timing.deadlineMs;
  if (usedChallenges.has(request.Challenge) || usedRequestIDs.has(request.RequestID)) throw fixedError('request_replayed');
  if (usedChallenges.size >= MAX_USED_REQUESTS || usedRequestIDs.size >= MAX_USED_REQUESTS) throw fixedError('request_limit');
  usedChallenges.add(request.Challenge);
  usedRequestIDs.add(request.RequestID);
  return { request, deadlineMs };
}

function createIngress(current, request, timestamp, openedAtMs, deadlineMs) {
  const remainingMs = deadlineMs - openedAtMs;
  if (!Number.isFinite(remainingMs) || remainingMs <= 0 || remainingMs > PHASE_WINDOW_MS) {
    throw fixedError('request_stale');
  }
  const endTimestamp = timestamp + remainingMs / 1000;
  const window = {
    id: `phase-${request.Phase}-${++observationSequence}`,
    startTimestamp: timestamp,
    endTimestamp,
    openedAtMs,
    expiresAtMs: deadlineMs,
  };
  return createFixtureXEvidenceIngress({
    scope: {
      ownerId: request.OwnerID,
      contextId: current.browserContextId,
      sessionId: current.sessionId,
      targetId: current.targetId,
      origin,
    },
    window,
    clock: monotonicWallClock,
    limits: {
      maxFrameBytes: MAX_CDP_FRAME_BYTES,
      maxInputBytes: MAX_CDP_FRAME_BYTES + 1,
      maxFramesPerPush: 1,
      maxPending: 16,
      maxOrphans: 16,
      maxAliases: 64,
      maxOutputRecords: 4,
      maxAgeMs: PHASE_WINDOW_MS,
      maxWindowMs: PHASE_WINDOW_MS,
      maxJSONDepth: 32,
      maxJSONTokens: 16_384,
    },
  });
}

function classifyBodyTimeout(current) {
  if (current.bodyCommandPending) return 'body_timeout_body_command_pending';
  const ingress = current.ingress;
  if (!ingress) return 'body_timeout_ingress_absent';
  const before = ingress.status();
  if (before.unavailable) {
    return before.reason === 'window_expired'
      ? 'body_timeout_window_expired' : 'body_timeout_ingress_unavailable';
  }
  const final = ingress.finish();
  if (final.reason === 'response_missing') return 'body_timeout_response_missing';
  if (final.reason === 'extra_info_missing') return 'body_timeout_extra_info_missing';
  if (final.reason === 'incomplete_operation') return 'body_timeout_terminal_missing';
  if (final.reason === 'window_expired') return 'body_timeout_window_expired';
  const records = ingress.drain();
  if (records.length === 1 && records[0].phase === 'completed') {
    return records[0].cache === 'unreported'
      ? 'body_timeout_completed_without_ticket' : 'body_timeout_cache_not_eligible';
  }
  if (records.length === 1 && records[0].phase === 'failed') return 'body_timeout_request_failed';
  return before.pending === 0 ? 'body_timeout_no_matching_request' : 'body_timeout_no_completed_request';
}

function checkBodyStatus(statusCode, body) {
  const expected = {
    authenticated: 200,
    logged_out: 401,
    challenge: 403,
    unknown: 200,
  }[body.state];
  if (expected === undefined || statusCode !== expected) throw fixedError('body_status_conflict');
}

async function observeSelf(message) {
  const current = requireOwner(message);
  if (!current.loginDone || !current.cacheBypassConfigured) throw fixedError('session_not_ready');
  const validated = await validateRequest(message.request, current);
  const { request, deadlineMs } = validated;
  let success = false;
  try {
    const metric = await send('Performance.getMetrics', {}, current.sessionId);
    const openedAt = monotonicWallClock();
    current.ingress = createIngress(current, request, metric.timestamp, openedAt, deadlineMs);
    if (current.protocolFailed || cdpProtocolFailure) {
      throw fixedError(current.protocolFailureCode ?? cdpProtocolFailure ?? 'cdp_protocol_failed');
    }
    if (!current.protocolFailed) current.failureCode = null;
    current.body = createDeferred();
    current.bodyReady = false;
    current.bodyReadCount = 0;
    current.bodyCommandPending = false;
    current.operationRequestId = null;
    const bodyWait = current.body.promise;
    // Allocation here is bounded by the controlled fixture server's enforced
    // wire cap and exact Content-Length, not by a post-allocation client check.
    // Avoid the direct stream-close/cancellation path on affected Chrome builds.
    const expression = `(async()=>{const response=await fetch(${JSON.stringify(origin + OPERATION_PATH)},{method:'POST',credentials:'include',cache:'no-store',headers:{'content-type':'application/json'},body:${JSON.stringify(JSON.stringify(request))}});const declared=response.headers.get('content-length');if(declared===null||!/^([0-9]{1,6})$/.test(declared))throw Error('fixture_length_contract');const expected=Number(declared);if(expected>${MAX_TRIGGER_DRAIN_BYTES})return false;const discarded=await response.arrayBuffer();if(discarded.byteLength!==expected)throw Error('fixture_length_contract');return discarded.byteLength<=${MAX_TRIGGER_DRAIN_BYTES}})()`;
    const triggerComplete = await evaluateBoolean(current.sessionId, expression, { allowFalse: true });
    if (!triggerComplete) throw fixedError('body_oversized');
    const remainingMs = Math.max(1, deadlineMs - monotonicWallClock());
    let body;
    try {
      body = await awaitWithTimeout(bodyWait, 'body_timeout', Math.min(BODY_WAIT_TIMEOUT_MS, remainingMs));
    } catch (error) {
      if (error?.message === 'xev_body_timeout') throw fixedError(classifyBodyTimeout(current));
      throw error;
    }
    if (current.failureCode) throw fixedError(current.failureCode);
    checkBodyStatus((() => {
      const status = current.ingress.finish();
      if (status.unavailable || status.state !== 'finished') throw fixedError(`ingress_${status.reason ?? 'incomplete'}`);
      const records = current.ingress.drain();
      if (records.length !== 1) throw fixedError('operation_count_invalid');
      current.record = records[0];
      return records[0].statusCode;
    })(), body);
    const record = current.record;
    if (!record || record.phase !== 'completed' || record.requestOffsetMs <= 0 ||
        record.cache !== 'unreported' || record.authorizing !== false) {
      throw fixedError(record?.cache !== 'unreported' ? 'cached_response_rejected' : 'ingress_evidence_invalid');
    }
    if (!current.cacheBypassConfigured || !body.body || body.bodyBytes > 4_096 || body.decodedBodyBytes > 4_096) {
      throw fixedError('body_evidence_invalid');
    }
    const observedAtMs = monotonicWallClock();
    if (observedAtMs >= deadlineMs) throw fixedError('request_stale');
    const evidenceId = `xev-${request.Phase}-${observationSequence}`;
    current.ingress.close();
    current.ingress = null;
    current.body = null;
    current.record = null;
    success = true;
    return {
      schema: OBSERVATION_SCHEMA,
      request,
      statusCode: record.statusCode,
      observedAt: new Date(observedAtMs).toISOString(),
      body: body.body,
      transport: {
        completed: true,
        cache: record.cache,
        cacheBypassConfigured: true,
        requestAfterWindow: true,
        ownerBound: true,
        bodyBound: true,
        bodyBytes: body.bodyBytes,
        decodedBodyBytes: body.decodedBodyBytes,
        authorizing: false,
      },
      evidenceId,
      productionQualified: false,
      nativeQualified: false,
      captureAuthorized: false,
    };
  } finally {
    if (!success) {
      current.ingress?.close();
      current.ingress = null;
      current.body = null;
      current.record = null;
    }
  }
}

async function closeChrome() {
  if (!child || childExited) return { exited: true };
  if (childClosePromise) return childClosePromise;
  childClosePromise = (async () => {
    try { await send('Browser.close', {}); } catch { /* Job cleanup remains the final owner. */ }
    for (let attempt = 0; attempt < 120 && !childExited; attempt++) {
      await new Promise(resolvePromise => setTimeout(resolvePromise, 25));
    }
    return { exited: childExited };
  })();
  return childClosePromise;
}

async function execute(message) {
  if (!object(message) || !Number.isSafeInteger(message.id) || message.id <= 0 ||
      typeof message.op !== 'string') throw fixedError('rpc_invalid');
  switch (message.op) {
    case 'launch':
      assertExactKeys(message, ['id', 'op', 'role', 'nativeReaderStartup']);
      return startChrome(message);
    case 'fixture_login':
      assertExactKeys(message, ['id', 'op', 'owner']);
      return login(message);
    case 'observe_self':
      assertExactKeys(message, ['id', 'op', 'owner', 'request']);
      return observeSelf(message);
    case 'release': {
      assertExactKeys(message, ['id', 'op', 'owner']);
      const current = requireOwner(message, { allowProtocolFailure: true });
      const result = await closeChrome();
      if (!result.exited) throw fixedError('chrome_exit_unverified');
      active = null;
      return result;
    }
    default:
      throw fixedError('rpc_operation_unsupported');
  }
}

function assertExactKeys(value, expected) {
  const keys = Object.keys(value);
  if (keys.length !== expected.length || keys.some(key => !expected.includes(key))) {
    throw fixedError('rpc_fields_invalid');
  }
}

async function handleControlLine(rawBytes) {
  if (!bootstrapDone) {
    const message = parseJSONFrame(rawBytes, controlFrameDecoder, { maxJSONDepth: 8, maxJSONTokens: 64 });
    assertExactKeys(message, ['op', 'withholdReady']);
    if (message.op !== 'fixture_bootstrap' || message.withholdReady !== false) {
      throw fixedError('bootstrap_invalid');
    }
    bootstrapDone = true;
    emit({ ready: true });
    return;
  }
  const message = parseJSONFrame(rawBytes, controlFrameDecoder, { maxJSONDepth: 32, maxJSONTokens: 4_096 });
  const id = message.id;
  try {
    const result = await execute(message);
    emitReply(id, result);
  } catch (error) {
    emitReply(id, undefined, safeErrorCode(error));
  } finally {
    message.id = 0;
  }
}

function onControlData(chunk) {
  if (!(chunk instanceof Uint8Array) || chunk.byteLength > MAX_CONTROL_BUFFER_BYTES ||
      controlBytes.byteLength + chunk.byteLength > MAX_CONTROL_BUFFER_BYTES) {
    shutdown('control_input_too_large');
    return;
  }
  controlBytes = Buffer.concat([controlBytes, Buffer.from(chunk)]);
  let newLines = 0;
  for (let end; (end = controlBytes.indexOf(0x0a)) >= 0;) {
    if (++newLines > MAX_CONTROL_LINES_PER_CHUNK) { shutdown('control_too_many_lines'); return; }
    const line = Buffer.from(controlBytes.subarray(0, end));
    const previous = controlBytes;
    controlBytes = Buffer.from(previous.subarray(end + 1));
    previous.fill(0);
    if (line.byteLength === 0 || line.byteLength > MAX_CONTROL_LINE_BYTES) { shutdown('control_line_invalid'); return; }
    controlLines++;
    if (controlLines > 256) { shutdown('control_line_limit'); return; }
    const raw = line;
    controlChain = controlChain.then(() => handleControlLine(raw)).catch(() => shutdown('control_protocol_failed'))
      .finally(() => raw.fill(0));
  }
}

function shutdown(reason) {
  if (shuttingDown) return;
  shuttingDown = true;
  if (active?.ingress) active.ingress.loss();
  rejectPending(reason);
  const current = active;
  void closeChrome(current).finally(() => process.exitCode = 0);
}

async function main() {
  const args = process.argv.slice(2);
  if (process.argv.length !== 5 || args.length !== 3 || !args[0] || !args[1] || !args[2]) {
    process.exitCode = 2;
    return;
  }
  executable = resolve(args[0]);
  qaRoot = resolve(args[1]);
  origin = parseOrigin(args[2]);
  lastClockMs = Date.now();
  process.stdin.on('data', onControlData);
  process.stdin.on('end', () => shutdown('control_eof'));
  process.stdin.on('error', () => shutdown('control_input_failed'));
  process.on('SIGTERM', () => shutdown('signal_terminated'));
  process.on('SIGINT', () => shutdown('signal_interrupted'));
  await new Promise(() => {});
}

void main().catch(() => {
  process.exitCode = 2;
  process.stdin.destroy();
});
