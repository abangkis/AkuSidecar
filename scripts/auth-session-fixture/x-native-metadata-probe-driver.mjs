// Opt-in, headful X QA. Default modes export metadata only. The fixed
// account-settings mode may read one completed response body and immediately
// project it to value-free shape diagnostics before resolving the CDP promise.
// Bounded frames/body strings are transient; no raw body is persisted/exported.
// A separate retained-source mode can project scoped cookie attributes at the
// private dispatch boundary. It never returns or transfers cookie values.
// No mode retrieves headers, page text, input values or screenshots.
import { spawn } from 'node:child_process';
import { mkdir, lstat, realpath, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { performance } from 'node:perf_hooks';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { parseStrictJSON } from './x-evidence-ingress.mjs';
import { standardChromeArgs } from './x-standard-chrome-control.mjs';
import { projectAccountSettingsBody } from './x-account-settings-body.mjs';
import { AccountSettingsCapture, accountSettingsBodyMode } from './x-account-settings-capture.mjs';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { BootstrapHandoffCapture } from './x-bootstrap-breakpoint-handoff.mjs';
import { QASourceCapture, projectQASourceBody, projectQACookieMetadata, qaCookieRequest } from './x-qa-source-qualification.mjs';

const qualificationMode = mode => mode === 'retained_source_qualification';
const bodyMode = mode => accountSettingsBodyMode(mode) || qualificationMode(mode);
const retainedQAMode = mode => mode === 'retained_account_settings' || mode === 'retained_bootstrap' || qualificationMode(mode);

const HOME_URL = 'https://x.com/home';
const X_ORIGIN = 'https://x.com';
const MAX_CONTROL_LINE_BYTES = 65_536;
const MAX_CONTROL_BUFFER_BYTES = 131_072;
const MAX_CONTROL_LINES_PER_CHUNK = 8;
const MAX_CONTROL_LINES_TOTAL = 256;
const MAX_CDP_FRAME_BYTES = 262_144;
const MAX_CDP_INPUT_CHUNK_BYTES = 1_048_576;
const MAX_CDP_FRAMES_PER_CHUNK = 128;
const MAX_PENDING_COMMANDS = 32;
const MAX_CANDIDATES = 64;
const MAX_NETWORK_EVENTS = 4_096;
const MAX_ENCODED_BYTES = 100_000_000;
const OBSERVATION_MS = 20_000;
const MAX_TOTAL_NETWORK_BUFFER_BYTES = 1_000_000;
const MAX_RESOURCE_NETWORK_BUFFER_BYTES = 100_000;
const COMMAND_TIMEOUT_MS = 4_000;
const CACHE_KEYS = Object.freeze(['servedFromCache', 'fromDiskCache', 'fromServiceWorker', 'fromPrefetchCache']);
const SAFE_GRAPHQL_OPERATIONS = new Set([
  'Account', 'CurrentUser', 'HomeLatestTimeline', 'HomeTimeline', 'UserByRestId',
  'UserByScreenName', 'UsersByRestIds', 'Viewer',
]);
const SAFE_REST_OPERATIONS = new Set([
  'rest:account-settings', 'rest:verify-credentials', 'rest:account-multi-list', 'rest:other',
]);
const CDP_METHODS = Object.freeze({
  'Target.getTargets': 'target_get_targets',
  'Target.attachToTarget': 'target_attach',
  'Page.enable': 'page_enable',
  'Page.reload': 'page_reload',
  'Runtime.evaluate': 'runtime_evaluate',
  'Debugger.evaluateOnCallFrame': 'early_evaluate_call_frame',
  'Network.enable': 'network_enable',
  'Network.disable': 'network_disable',
  'Network.getResponseBody': 'account_settings_body',
  'Network.getCookies': 'qa_cookie_metadata',
  'Network.setCacheDisabled': 'account_settings_cache',
  'Browser.close': 'browser_close',
  'Page.getFrameTree': 'early_frame_tree',
  'Runtime.enable': 'early_runtime_enable',
  'Runtime.disable': 'early_runtime_disable',
  'Debugger.enable': 'early_debugger_enable',
  'Debugger.disable': 'early_debugger_disable',
  'Debugger.setInstrumentationBreakpoint': 'early_instrumentation',
  'Debugger.setBreakpoint': 'early_normal_breakpoint',
  'Debugger.removeBreakpoint': 'early_remove_breakpoint',
  'Debugger.resume': 'early_resume',
});
const EARLY_CDP_METHODS = new Set(['Page.getFrameTree', 'Runtime.enable', 'Runtime.disable', 'Debugger.enable',
  'Debugger.disable', 'Debugger.setInstrumentationBreakpoint', 'Debugger.setBreakpoint', 'Debugger.removeBreakpoint', 'Debugger.resume']);
const EARLY_COMMAND_LABELS = Object.freeze({ 'Page.getFrameTree': 'frameTree', 'Runtime.enable': 'runtimeEnable',
  'Debugger.enable': 'debuggerEnable', 'Debugger.setInstrumentationBreakpoint': 'breakpointSet', 'Page.reload': 'reload',
  'Debugger.resume': 'resume', 'Debugger.removeBreakpoint': 'breakpointRemove', 'Debugger.disable': 'debuggerDisable',
  'Runtime.disable': 'runtimeDisable', 'Debugger.setBreakpoint': 'normalBreakpointSet', 'Debugger.evaluateOnCallFrame': 'pausedRead' });
const EARLY_LABELS = Object.freeze([...Object.values(EARLY_COMMAND_LABELS), 'normalBreakpointRemove']);
const CDP_FRAME_LIMITS = Object.freeze({ maxJSONDepth: 20, maxJSONTokens: 8_192 });
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });

function probeError(code) {
  return new Error(`xnp_${code}`);
}

function isObject(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function own(value, key) {
  return Object.prototype.hasOwnProperty.call(value, key);
}

function safeId(value, max = 256) {
  return typeof value === 'string' && value.length > 0 && value.length <= max &&
    !/[\u0000-\u001f\u007f]/u.test(value);
}

function exactKeys(value, expected, code) {
  if (!isObject(value)) throw probeError(code);
  const keys = Object.keys(value);
  if (keys.length !== expected.length || keys.some(key => !expected.includes(key))) throw probeError(code);
}

function parseControlOrigin(value) {
  if (typeof value !== 'string' || value.length > 128) throw probeError('origin_invalid');
  let parsed;
  try { parsed = new URL(value); } catch { throw probeError('origin_invalid'); }
  if (parsed.protocol !== 'http:' || parsed.hostname !== '127.0.0.1' || !parsed.port ||
      parsed.username || parsed.password || parsed.pathname !== '/' || parsed.search || parsed.hash ||
      parsed.origin !== value) throw probeError('origin_invalid');
  return parsed.origin;
}

function parseJSONBytes(bytes, maxFrameBytes = MAX_CDP_FRAME_BYTES) {
  if (!(bytes instanceof Uint8Array) || bytes.byteLength < 1 || bytes.byteLength > maxFrameBytes) {
    throw probeError('cdp_frame_size_invalid');
  }
  let text;
  try { text = decoder.decode(bytes); } catch { throw probeError('cdp_utf8_invalid'); }
  try {
    const value = parseStrictJSON(text, CDP_FRAME_LIMITS);
    if (!isObject(value)) throw probeError('cdp_json_invalid');
    return value;
  } catch (error) {
    if (error?.message?.startsWith('xnp_')) throw error;
    throw probeError('cdp_json_invalid');
  }
}

export class CDPFrameDecoder {
  constructor({ maxFrameBytes = MAX_CDP_FRAME_BYTES, maxInputBytes = MAX_CDP_INPUT_CHUNK_BYTES,
    maxFramesPerChunk = MAX_CDP_FRAMES_PER_CHUNK } = {}) {
    if (!Number.isSafeInteger(maxFrameBytes) || maxFrameBytes < 1 || maxFrameBytes > MAX_CDP_FRAME_BYTES ||
        !Number.isSafeInteger(maxInputBytes) || maxInputBytes < 1 || maxInputBytes > MAX_CDP_INPUT_CHUNK_BYTES ||
        !Number.isSafeInteger(maxFramesPerChunk) || maxFramesPerChunk < 1 || maxFramesPerChunk > MAX_CDP_FRAMES_PER_CHUNK) {
      throw probeError('cdp_decoder_limits_invalid');
    }
    this.maxFrameBytes = maxFrameBytes;
    this.maxInputBytes = maxInputBytes;
    this.maxFramesPerChunk = maxFramesPerChunk;
    this.buffer = Buffer.allocUnsafe(maxFrameBytes);
    this.length = 0;
  }

  push(chunk, onFrame) {
    if (!(chunk instanceof Uint8Array) || chunk.byteLength > this.maxInputBytes || typeof onFrame !== 'function') {
      throw probeError('cdp_input_invalid');
    }
    let frames = 0;
    for (const byte of chunk) {
      if (byte === 0) {
        if (++frames > this.maxFramesPerChunk) throw probeError('cdp_frame_count_exceeded');
        if (this.length === 0) throw probeError('cdp_frame_empty');
        const frame = Buffer.from(this.buffer.subarray(0, this.length));
        this.buffer.fill(0, 0, this.length);
        this.length = 0;
        let message;
        try {
          message = parseJSONBytes(frame, this.maxFrameBytes);
          onFrame(message);
        } finally {
          frame.fill(0);
          message = null;
        }
      } else {
        if (this.length >= this.maxFrameBytes) {
          this.reset();
          throw probeError('cdp_frame_too_large');
        }
        this.buffer[this.length++] = byte;
      }
    }
  }

  finish() {
    if (this.length !== 0) throw probeError('cdp_frame_unterminated');
  }

  reset() {
    this.buffer.fill(0);
    this.length = 0;
  }
}

export function classifyXOperation(rawURL) {
  if (typeof rawURL !== 'string' || rawURL.length > 8_192) return null;
  let parsed;
  try { parsed = new URL(rawURL); } catch { return null; }
  if (parsed.protocol !== 'https:' || parsed.username || parsed.password ||
      !['x.com', 'api.x.com'].includes(parsed.hostname)) return null;
  let apiPath = parsed.pathname;
  if (parsed.hostname === 'x.com') {
    if (!apiPath.startsWith('/i/api/')) return null;
    apiPath = apiPath.slice('/i/api'.length);
  } else if (apiPath.startsWith('/i/api/')) {
    apiPath = apiPath.slice('/i/api'.length);
  }
  if (apiPath.includes('/graphql/')) {
    const match = apiPath.match(/(?:^|\/)graphql\/[^/]+\/([^/]+)$/u);
    if (!match || !SAFE_GRAPHQL_OPERATIONS.has(match[1])) return 'graphql:other';
    return `graphql:${match[1]}`;
  }
  if (apiPath === '/1.1/account/settings.json') return 'rest:account-settings';
  if (apiPath === '/1.1/account/verify_credentials.json') return 'rest:verify-credentials';
  if (apiPath === '/1.1/account/multi/list.json') return 'rest:account-multi-list';
  return 'rest:other';
}

function safeMethod(value) {
  if (typeof value !== 'string') return 'OTHER';
  const method = value.toUpperCase();
  return ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD'].includes(method) ? method : 'OTHER';
}

function finiteNonnegative(value, max = Number.MAX_SAFE_INTEGER) {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= max ? value : null;
}

function optionalBoolean(value) {
  return typeof value === 'boolean' ? value : null;
}

function isSafeOperationCode(value) {
  if (SAFE_REST_OPERATIONS.has(value)) return true;
  if (value === 'graphql:other') return true;
  return typeof value === 'string' && value.startsWith('graphql:') &&
    SAFE_GRAPHQL_OPERATIONS.has(value.slice('graphql:'.length));
}

// This projection is the only place raw Network event fields enter probe logic.
// Its result deliberately excludes URLs, query/hash text, headers, bodies, and
// server-provided error strings. requestKey is retained only for local joining.
export function projectNetworkEvent(method, params) {
  if (!isObject(params)) return null;
  if (method === 'Network.requestWillBeSent') {
    const request = params.request;
    if (!isObject(request) || !safeId(params.requestId, 512)) return null;
    const operation = classifyXOperation(request.url);
    if (operation === null) return null;
    return Object.freeze({
      type: 'request', requestKey: params.requestId, operation,
      method: safeMethod(request.method), timestamp: finiteNonnegative(params.timestamp),
      redirected: isObject(params.redirectResponse),
    });
  }
  if (!safeId(params.requestId, 512)) return null;
  if (method === 'Network.responseReceived') {
    const response = params.response;
    if (!isObject(response)) return null;
    return Object.freeze({
      type: 'response', requestKey: params.requestId,
      status: Number.isInteger(response.status) && response.status >= 100 && response.status <= 599 ? response.status : null,
      servedFromCache: null,
      hasExtraInfo: optionalBoolean(params.hasExtraInfo),
      fromDiskCache: optionalBoolean(response.fromDiskCache),
      fromServiceWorker: optionalBoolean(response.fromServiceWorker),
      fromPrefetchCache: optionalBoolean(response.fromPrefetchCache),
      timestamp: finiteNonnegative(params.timestamp),
    });
  }
  if (method === 'Network.requestServedFromCache') {
    return Object.freeze({ type: 'cache', requestKey: params.requestId, servedFromCache: true });
  }
  if (method === 'Network.loadingFinished') {
    const bytes = finiteNonnegative(params.encodedDataLength, MAX_ENCODED_BYTES);
    return Object.freeze({
      type: 'finished', requestKey: params.requestId,
      bytes: bytes === null ? null : Math.floor(bytes), timestamp: finiteNonnegative(params.timestamp),
    });
  }
  if (method === 'Network.loadingFailed') {
    return Object.freeze({ type: 'failed', requestKey: params.requestId, timestamp: finiteNonnegative(params.timestamp) });
  }
  return null;
}

function coarseTiming(start, end) {
  if (start === null || end === null) return null;
  const elapsed = Math.round((end - start) * 1_000);
  if (!Number.isSafeInteger(elapsed) || elapsed < 0 || elapsed > OBSERVATION_MS) return null;
  return Math.round(elapsed / 250) * 250;
}

export class MetadataWindow {
  constructor(maxCandidates = MAX_CANDIDATES) {
    if (!Number.isSafeInteger(maxCandidates) || maxCandidates < 1 || maxCandidates > MAX_CANDIDATES) {
      throw probeError('candidate_limit_invalid');
    }
    this.maxCandidates = maxCandidates;
    this.requests = new Map();
    this.nextAlias = 0;
    this.dropped = 0;
  }

  observe(event) {
    if (!isObject(event) || !safeId(event.requestKey, 512)) return;
    const current = this.requests.get(event.requestKey);
    if (event.type === 'request') {
      if (!isSafeOperationCode(event.operation) ||
          !['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD', 'OTHER'].includes(event.method)) return;
      if (current) {
        current.correlationAmbiguous = true;
        current.redirected ||= !!event.redirected;
        current.status = null;
        return;
      }
      if (this.requests.size >= this.maxCandidates) {
        this.dropped = Math.min(10_000, this.dropped + 1);
        return;
      }
      const cache = Object.create(null);
      for (const key of CACHE_KEYS) cache[key] = null;
      this.requests.set(event.requestKey, {
        alias: `r${++this.nextAlias}`, operation: event.operation, method: event.method,
        start: event.timestamp, response: null, finish: null, failed: false,
        redirected: !!event.redirected, correlationAmbiguous: false,
        hasExtraInfo: null, status: null, cache, bytes: null,
      });
      return;
    }
    if (!current) return;
    if (event.type === 'response') {
      current.hasExtraInfo = event.hasExtraInfo;
      current.status = current.redirected || current.correlationAmbiguous ? null : event.status;
      current.response = event.timestamp;
      for (const key of CACHE_KEYS) {
        if (event[key] !== null) current.cache[key] = event[key];
      }
    } else if (event.type === 'cache') {
      current.cache.servedFromCache = true;
    } else if (event.type === 'finished') {
      current.finish = event.timestamp;
      current.bytes = event.bytes;
    } else if (event.type === 'failed') {
      current.failed = true;
      current.finish = event.timestamp;
    }
  }

  finish() {
    const candidates = [];
    for (const item of this.requests.values()) {
      candidates.push({
        alias: item.alias,
        operation: item.operation,
        method: item.method,
        status: item.status,
        ambiguous: item.redirected || item.correlationAmbiguous,
        hasExtraInfo: item.hasExtraInfo,
        cache: { ...item.cache },
        completion: item.failed ? 'failed' : item.finish === null ? 'incomplete' : 'complete',
        bytes: item.bytes,
        timingMs: {
          response: coarseTiming(item.start, item.response),
          completion: coarseTiming(item.start, item.finish),
        },
      });
    }
    this.requests.clear();
    return { candidates, dropped: this.dropped };
  }
}

export class LoginObservationGate {
  constructor() {
    this.loginVisible = false;
    this.observationStarted = false;
    this.networkEnabled = false;
    this.reloadCount = 0;
  }

  setLoginVisible(value) {
    if (typeof value !== 'boolean') throw probeError('login_state_invalid');
    this.loginVisible = value;
  }

  beginObservation() {
    if (!this.loginVisible || this.observationStarted || this.networkEnabled || this.reloadCount !== 0) {
      throw probeError('observation_gate_closed');
    }
    this.observationStarted = true;
  }

  markNetworkEnabled() {
    if (!this.observationStarted || this.networkEnabled || this.reloadCount !== 0) {
      throw probeError('network_enable_order_invalid');
    }
    this.networkEnabled = true;
  }

  markReloaded() {
    if (!this.networkEnabled || this.reloadCount !== 0) throw probeError('reload_order_invalid');
    this.reloadCount = 1;
  }
}

function exactCDPKeys(message, keys) {
  const actual = Object.keys(message);
  if (actual.length !== keys.length || actual.some(key => !keys.includes(key))) throw probeError('cdp_message_invalid');
}

export class TwoPhaseWorkflowGrant {
  constructor({ owner, pid, now = () => performance.now() }) {
    if (!Number.isSafeInteger(owner) || owner <= 0 || !Number.isSafeInteger(pid) || pid <= 0 || typeof now !== 'function') {
      throw probeError('workflow_state_invalid');
    }
    this.owner = owner;
    this.pid = pid;
    this.now = now;
    this.attempted = false;
    this.authorized = false;
    this.consumed = false;
    this.expiresAt = 0;
    this.lastNow = null;
  }

  readClock() {
    let now;
    try {
      now = this.now();
    } catch {
      this.consumed = true;
      this.authorized = false;
      throw probeError('workflow_clock_invalid');
    }
    if (!Number.isFinite(now) || now < 0) {
      this.consumed = true;
      this.authorized = false;
      throw probeError('workflow_clock_invalid');
    }
    if (this.lastNow !== null && now < this.lastNow) {
      this.consumed = true;
      this.authorized = false;
      throw probeError('workflow_clock_regressed');
    }
    this.lastNow = now;
    return now;
  }

  authorize({ owner, oldPID, nonce, exitCode, identityUnchanged }) {
    if (this.attempted || this.authorized || this.consumed) throw probeError('workflow_grant_replay');
    this.attempted = true;
    if (owner !== this.owner || oldPID !== this.pid || typeof nonce !== 'string' || !/^[a-f0-9]{64}$/u.test(nonce)) {
      throw probeError('workflow_ticket_invalid');
    }
    if (exitCode !== 0) throw probeError('phase1_exit_required');
    if (identityUnchanged !== true) throw probeError('owned_profile_changed');
    this.expiresAt = this.readClock() + 10_000;
    this.authorized = true;
    return { authorized: true };
  }

  consume({ identityUnchanged }) {
    if (!this.authorized || this.consumed) throw probeError('workflow_grant_replay');
    if (this.readClock() >= this.expiresAt) {
      this.consumed = true;
      throw probeError('workflow_grant_expired');
    }
    if (identityUnchanged !== true) {
      this.consumed = true;
      throw probeError('owned_profile_changed');
    }
    this.consumed = true;
    return true;
  }
}

export function twoPhaseChromeArgs(profile, remoteDebugging) {
  if (typeof remoteDebugging !== 'boolean') throw probeError('chrome_args_invalid');
  const args = standardChromeArgs(profile);
  if (remoteDebugging) args.splice(args.length - 1, 0, '--remote-debugging-pipe');
  return args;
}

function sameWindowsPath(left, right) {
  return typeof left === 'string' && typeof right === 'string' &&
    resolve(left).toLowerCase() === resolve(right).toLowerCase();
}

async function ordinaryDirectoryIdentity(path) {
  const item = await lstat(path, { bigint: true }).catch(() => null);
  const real = await realpath(path).catch(() => '');
  if (!item?.isDirectory() || item.isSymbolicLink() || !real || !sameWindowsPath(real, path)) {
    throw probeError('owned_directory_invalid');
  }
  const identity = {
    real: resolve(real).toLowerCase(),
    dev: item.dev.toString(),
    ino: item.ino.toString(),
    birthtimeNs: item.birthtimeNs.toString(),
  };
  if (identity.dev === '0' || identity.ino === '0' || identity.birthtimeNs === '0') {
    throw probeError('owned_directory_identity_unavailable');
  }
  return Object.freeze(identity);
}

function sameDirectoryIdentity(left, right) {
  return !!left && !!right && left.real === right.real && left.dev === right.dev &&
    left.ino === right.ino && left.birthtimeNs === right.birthtimeNs;
}

async function verifyTwoPhaseDirectories() {
  if (!twoPhaseState) return false;
  try {
    const rootIdentity = await ordinaryDirectoryIdentity(twoPhaseState.root);
    const profileIdentity = await ordinaryDirectoryIdentity(twoPhaseState.profile);
    return sameDirectoryIdentity(rootIdentity, twoPhaseState.rootIdentity) &&
      sameDirectoryIdentity(profileIdentity, twoPhaseState.profileIdentity);
  } catch {
    return false;
  }
}

function isTargetURL(rawURL) {
  if (typeof rawURL !== 'string' || rawURL.length > 2_048) return false;
  try {
    const parsed = new URL(rawURL);
    return parsed.origin === X_ORIGIN && !parsed.username && !parsed.password;
  } catch { return false; }
}

function projectCDPResult(method, result, projectionKind = null) {
  if (!isObject(result)) throw probeError('cdp_result_invalid');
  switch (method) {
    case 'Page.getFrameTree': {
      const frame = result.frameTree?.frame;
      if (!isObject(frame) || !safeId(frame.id) || !isHomeTarget(frame.url)) throw probeError('early_frame_invalid');
      return { frameId: frame.id };
    }
    case 'Debugger.setInstrumentationBreakpoint':
      if (!safeId(result.breakpointId)) throw probeError('early_breakpoint_invalid');
      active.earlyInstrumentationId = result.breakpointId;
      return { breakpointId: result.breakpointId };
    case 'Debugger.setBreakpoint': {
      const handoff = active?.earlyCapture?.handoff;
      if (!handoff) throw probeError('early_normal_breakpoint_invalid');
      handoff.created(result.breakpointId);
      const location = result.actualLocation;
      const requested = handoff.installing;
      if (!requested || !isObject(location) || location.scriptId !== requested.scriptId ||
          !Number.isSafeInteger(location.lineNumber) || location.lineNumber < 0 || location.lineNumber > 1000000 ||
          !Number.isSafeInteger(location.columnNumber) || location.columnNumber < 0 || location.columnNumber > 1000000) throw probeError('early_normal_breakpoint_invalid');
      return { breakpointId: result.breakpointId, actualLocation: { scriptId: location.scriptId, lineNumber: location.lineNumber, columnNumber: location.columnNumber } };
    }
    case 'Network.getResponseBody':
      if (!bodyMode(driverMode)) throw probeError('body_mode_required');
      return qualificationMode(driverMode) ? projectQASourceBody(result) : projectAccountSettingsBody(result);
    case 'Network.getCookies':
      if (!qualificationMode(driverMode) || !active?.qaCookieIssued) throw probeError('cookie_mode_required');
      return projectQACookieMetadata(result);
    case 'Target.getTargets': {
      if (!Array.isArray(result.targetInfos) || result.targetInfos.length > 128) throw probeError('target_inventory_invalid');
      const matches = [];
      for (const target of result.targetInfos) {
        if (isObject(target) && target.type === 'page' && isTargetURL(target.url) && safeId(target.targetId)) {
          matches.push(target.targetId);
        }
      }
      return { targetIds: matches };
    }
    case 'Target.attachToTarget':
      if (!safeId(result.sessionId)) throw probeError('target_session_invalid');
      return { sessionId: result.sessionId };
    case 'Debugger.evaluateOnCallFrame':
    case 'Runtime.evaluate': {
      if (projectionKind === 'bootstrap_current_self') {
        if (driverMode !== 'retained_bootstrap' || own(result, 'exceptionDetails') ||
            !isObject(result.result) || result.result.type !== 'object' || !isObject(result.result.value)) {
          throw probeError('bootstrap_runtime_result_invalid');
        }
        try { return validateBootstrapCurrentSelf(result.result.value); }
        catch { throw probeError('bootstrap_projection_invalid'); }
      }
      if (own(result, 'exceptionDetails') || !isObject(result.result) || result.result.type !== 'boolean' ||
          typeof result.result.value !== 'boolean') throw probeError('runtime_result_invalid');
      return { value: result.result.value };
    }
    case 'Page.enable':
    case 'Page.reload':
    case 'Network.enable':
    case 'Network.disable':
    case 'Network.setCacheDisabled':
    case 'Browser.close':
    case 'Runtime.enable':
    case 'Runtime.disable':
    case 'Debugger.enable':
    case 'Debugger.disable':
    case 'Debugger.removeBreakpoint':
    case 'Debugger.resume':
      return { acknowledged: true };
    default:
      throw probeError('cdp_method_invalid');
  }
}

export function projectCDPEvent(message) {
  if (typeof message.method !== 'string' || message.method.length > 128 || !isObject(message.params)) {
    throw probeError('cdp_event_invalid');
  }
  switch (message.method) {
    case 'Page.frameNavigated': {
      const frame = message.params.frame;
      if (!isObject(frame) || own(frame, 'parentId') || !safeId(frame.id)) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'frame_navigated', frameId: frame.id, isHome: isHomeTarget(frame.url) } };
    }
    case 'Runtime.executionContextCreated': {
      const context = message.params.context;
      if (!isObject(context) || !Number.isSafeInteger(context.id) || context.id <= 0 ||
          context.auxData?.isDefault !== true || !safeId(context.auxData.frameId)) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'context_created', contextId: context.id, frameId: context.auxData.frameId, isDefault: true } };
    }
    case 'Runtime.executionContextsCleared':
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null, event: { type: 'contexts_cleared' } };
    case 'Runtime.executionContextDestroyed':
      if (!Number.isSafeInteger(message.params.executionContextId) || message.params.executionContextId <= 0) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'context_destroyed', contextId: message.params.executionContextId } };
    case 'Debugger.scriptParsed': {
      const params = message.params;
      if (!safeId(params.scriptId) || !Number.isSafeInteger(params.executionContextId) || params.executionContextId <= 0 ||
          params.executionContextAuxData?.isDefault !== true || !safeId(params.executionContextAuxData.frameId)) return null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'script_parsed', scriptId: params.scriptId, contextId: params.executionContextId,
          frameId: params.executionContextAuxData.frameId, isDefault: true } };
    }
    case 'Debugger.paused': {
      // Correlate only the top frame's opaque IDs. Never project its locals, scopes, URLs or source.
      const top = Array.isArray(message.params.callFrames) ? message.params.callFrames[0] : null;
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: { type: 'paused', reason: message.params.reason === 'instrumentation' ? 'instrumentation' : 'other',
          callFrameId: safeId(top?.callFrameId) ? top.callFrameId : null,
          scriptId: safeId(top?.location?.scriptId) ? top.location.scriptId : null } };
    }
    case 'Debugger.resumed':
      return { type: 'early', sessionId: safeId(message.sessionId) ? message.sessionId : null, event: { type: 'resumed' } };
    case 'Page.loadEventFired':
      return { type: 'page_load', sessionId: safeId(message.sessionId) ? message.sessionId : null };
    case 'Network.requestWillBeSent':
    case 'Network.responseReceived':
    case 'Network.requestServedFromCache':
    case 'Network.loadingFinished':
    case 'Network.loadingFailed':
      return {
        type: 'network', sessionId: safeId(message.sessionId) ? message.sessionId : null,
        event: projectNetworkEvent(message.method, message.params),
      };
    default:
      return null;
  }
}

const ACCOUNT_READY_EXPRESSION = `(()=>{const visible=e=>!!e&&e.getClientRects().length>0&&getComputedStyle(e).visibility!=='hidden'&&getComputedStyle(e).display!=='none';return location.origin==='https://x.com'&&location.pathname==='/home'&&visible(document.querySelector('[data-testid="SideNav_AccountSwitcher_Button"]'))&&visible(document.querySelector('a[data-testid="AppTabBar_Profile_Link"]'))})()`;

function isHomeTarget(value) {
  if (!isTargetURL(value)) return false;
  try { return new URL(value).pathname === '/home'; } catch { return false; }
}

let executable = '';
let qaRoot = '';
let controlOrigin = '';
let profilePath = '';
let driverMode = 'single_phase';
let twoPhaseState = null;
let retainedQAScope = null;
let child = null;
let childExited = false;
let cdpFailure = null;
let protocolOwner = 0;
let nextOwner = 0;
let nextCommandId = 0;
let controlBytes = Buffer.alloc(0);
let controlLines = 0;
let controlChain = Promise.resolve();
let active = null;
let bootstrapDone = false;
let shuttingDown = false;
let cdpFrames = new CDPFrameDecoder();
const pending = new Map();

function emit(value) {
  let bytes;
  try { bytes = Buffer.from(`${JSON.stringify(value)}\n`, 'utf8'); }
  catch { process.exitCode = 2; return; }
  if (bytes.byteLength > MAX_CONTROL_LINE_BYTES) {
    bytes = Buffer.from(`${JSON.stringify({ id: value?.id ?? 0, ok: false, code: 'output_too_large' })}\n`, 'utf8');
  }
  process.stdout.write(bytes);
}

function emitReply(id, result = undefined, code = undefined) {
  if (!Number.isSafeInteger(id) || id <= 0) return;
  if (code) emit({ id, ok: false, code });
  else emit({ id, ok: true, result });
}

function safeCode(error) {
  const value = typeof error?.message === 'string' ? error.message : '';
  if (/^xnp_[a-z0-9_]{1,80}$/u.test(value)) return value.slice(4);
  return 'probe_failed';
}

function cdpWrite(message) {
  if (cdpFailure) throw probeError(cdpFailure);
  if (!child || childExited || !child.stdio?.[3] || pending.size >= MAX_PENDING_COMMANDS) throw probeError('cdp_unavailable');
  let line;
  try { line = Buffer.from(`${JSON.stringify(message)}\0`, 'utf8'); } catch { throw probeError('cdp_encode_failed'); }
  if (line.byteLength > MAX_CDP_FRAME_BYTES) { line.fill(0); throw probeError('cdp_command_too_large'); }
  try {
    child.stdio[3].write(line, error => {
      line.fill(0);
      if (error) failCDP('cdp_write_failed');
    });
  } catch {
    line.fill(0);
    throw probeError('cdp_write_failed');
  }
}

export function isEarlyBootstrapCallFrameRead(params, capture) {
  const keys = ['callFrameId', 'expression', 'returnByValue', 'throwOnSideEffect', 'timeout',
    'silent', 'includeCommandLineAPI', 'generatePreview'];
  return isObject(params) && Object.keys(params).length === keys.length &&
    Object.keys(params).every(key => keys.includes(key)) && capture?.canReadCallFrame(params.callFrameId) === true &&
    params.expression === BOOTSTRAP_CURRENT_SELF_EXPRESSION && params.returnByValue === true &&
    params.throwOnSideEffect === true && params.timeout === 1500 && params.silent === true &&
    params.includeCommandLineAPI === false && params.generatePreview === false;
}

function send(method, params = {}, sessionId = undefined, purpose = null) {
  if (cdpFailure) return Promise.reject(probeError(cdpFailure));
  if (!Object.hasOwn(CDP_METHODS, method) || !isObject(params) || pending.size >= MAX_PENDING_COMMANDS) {
    return Promise.reject(probeError('cdp_command_invalid'));
  }
  if (purpose !== null && !(purpose === 'normal_remove' && method === 'Debugger.removeBreakpoint' &&
      active?.earlyCapture?.handoff.breakpoints.has(params.breakpointId))) return Promise.reject(probeError('early_command_not_authorized'));
  if (method === 'Network.getCookies' && (!qualificationMode(driverMode) || !active?.qaCookieRequested || active.qaCookieIssued ||
      sessionId !== active.sessionId || JSON.stringify(params) !== JSON.stringify(qaCookieRequest(active.qaSourceRoute)))) {
    return Promise.reject(probeError('cookie_read_not_authorized'));
  }
  if (method === 'Network.getCookies') active.qaCookieIssued = true;
  if (method === 'Network.getResponseBody' && (!bodyMode(driverMode) || !active?.gate.networkEnabled ||
      sessionId !== active.sessionId || Object.keys(params).join(',') !== 'requestId' || !active.accountSettings?.consumeRead(params.requestId))) {
    return Promise.reject(probeError('account_body_not_authorized'));
  }
  if (method === 'Network.setCacheDisabled' && (!bodyMode(driverMode) || sessionId !== active?.sessionId ||
      Object.keys(params).join(',') !== 'cacheDisabled' || params.cacheDisabled !== true)) return Promise.reject(probeError('account_cache_not_authorized'));
  if (EARLY_CDP_METHODS.has(method) || method === 'Page.reload' && driverMode === 'retained_bootstrap') {
    const keys = Object.keys(params);
    let valid = keys.length === 0;
    if (method === 'Debugger.setInstrumentationBreakpoint') valid = keys.length === 1 && params.instrumentation === 'beforeScriptExecution';
    if (method === 'Debugger.removeBreakpoint') valid = keys.length === 1 && safeId(params.breakpointId) &&
      (purpose === 'normal_remove' ? active?.earlyCapture?.handoff.breakpoints.has(params.breakpointId) : params.breakpointId === active?.earlyInstrumentationId);
    if (method === 'Debugger.setBreakpoint') valid = keys.length === 1 && keys[0] === 'location' && active?.earlyCapture?.handoff.canInstall(params.location);
    if (method === 'Page.reload') valid = keys.length === 1 && params.ignoreCache === false;
    const qualificationFrame = qualificationMode(driverMode) && method === 'Page.getFrameTree' &&
      active?.qualificationAttempted && active.gate.observationStarted && keys.length === 0;
    if (!qualificationFrame && (driverMode !== 'retained_bootstrap' || !active?.earlyRunning || sessionId !== active.sessionId || !valid) ||
        qualificationFrame && sessionId !== active.sessionId) {
      return Promise.reject(probeError('early_command_not_authorized'));
    }
  }
  let projectionKind = null;
  if (method === 'Debugger.evaluateOnCallFrame') {
    if (driverMode !== 'retained_bootstrap' || !active?.earlyRunning || active.gate.observationStarted ||
        sessionId !== active.sessionId || !isEarlyBootstrapCallFrameRead(params, active.earlyCapture)) {
      return Promise.reject(probeError('early_call_frame_not_authorized'));
    }
    projectionKind = 'bootstrap_current_self';
  }
  if (method === 'Runtime.evaluate') {
    const bootstrap = params.expression === BOOTSTRAP_CURRENT_SELF_EXPRESSION;
    const keys = bootstrap ? ['expression', 'returnByValue', 'awaitPromise', 'userGesture', 'throwOnSideEffect', 'timeout'] :
      ['expression', 'returnByValue', 'awaitPromise', 'userGesture'];
    if (Object.keys(params).length !== keys.length || Object.keys(params).some(key => !keys.includes(key)) ||
        params.returnByValue !== true || params.awaitPromise !== false || params.userGesture !== false || sessionId !== active?.sessionId ||
        bootstrap && (driverMode !== 'retained_bootstrap' ||
          (!active?.bootstrapAttempted || !active.gate.loginVisible || active.earlyRunning) ||
          active.gate.observationStarted || params.throwOnSideEffect !== true || params.timeout !== 1500) ||
        !bootstrap && (params.expression !== ACCOUNT_READY_EXPRESSION || active?.earlyRunning)) return Promise.reject(probeError('runtime_expression_not_authorized'));
    if (bootstrap) projectionKind = 'bootstrap_current_self';
  }
  const id = ++nextCommandId;
  if (!Number.isSafeInteger(id) || id <= 0) return Promise.reject(probeError('cdp_command_limit'));
  return new Promise((resolvePromise, rejectPromise) => {
    const timer = setTimeout(() => {
      if (pending.has(id)) failCDP(`${CDP_METHODS[method]}_timeout`);
    }, COMMAND_TIMEOUT_MS);
    pending.set(id, { method, projectionKind, sessionId: sessionId ?? null, resolve: resolvePromise, reject: rejectPromise, timer });
    try { cdpWrite({ id, method, params, ...(sessionId ? { sessionId } : {}) }); }
    catch (error) {
      pending.delete(id);
      clearTimeout(timer);
      rejectPromise(error);
    }
  });
}

function rejectPending(code) {
  for (const [id, item] of pending) {
    clearTimeout(item.timer);
    pending.delete(id);
    item.reject(probeError(code));
  }
}

function failCDP(code) {
  if (!cdpFailure) cdpFailure = /^[a-z0-9_]{1,80}$/u.test(code) ? code : 'cdp_protocol_failed';
  cdpFrames.reset();
  rejectPending(cdpFailure);
  active?.accountSettings?.invalidate();
  if (active?.pageLoad) {
    active.pageLoad.reject(probeError(cdpFailure));
    active.pageLoad = null;
  }
}

function handleNetworkEvent(event) {
  if (!active || !active.gate.networkEnabled || !active.metadata || !event) return;
  if (Date.now() > active.observationEndsAt) return;
  active.metadata.observe(event);
}

function dispatchCDP(raw) {
  if (own(raw, 'method')) {
    exactCDPKeys(raw, raw.sessionId === undefined ? ['method', 'params'] : ['method', 'params', 'sessionId']);
    if (typeof raw.method === 'string' && raw.method.startsWith('Network.') && active?.metadata &&
        active.gate.networkEnabled && Date.now() <= active.observationEndsAt) {
      if (active.networkEventCount >= MAX_NETWORK_EVENTS) {
        active.eventLimitReached = true;
        active.accountSettings?.invalidate();
        return;
      }
      active.networkEventCount++;
    }
    const event = projectCDPEvent(raw);
    if (!event || !active) return;
    if (event.type === 'early' && event.sessionId === active.sessionId && active.earlyRunning) {
      active.earlyCapture?.observe(event.event, raw.params);
    } else if (event.type === 'page_load' && event.sessionId === active.sessionId && active.pageLoad) {
      active.pageLoad.resolve(true);
      active.pageLoad = null;
    } else if (event.type === 'network' && event.sessionId === active.sessionId) {
      if (active.gate.networkEnabled && active.metadata && Date.now() <= active.observationEndsAt) {
        active.accountSettings?.observe(raw.method, raw.params);
      }
      handleNetworkEvent(event.event);
    }
    return;
  }
  const hasResult = own(raw, 'result');
  const hasError = own(raw, 'error');
  exactCDPKeys(raw, raw.sessionId === undefined ? ['id', ...(hasResult ? ['result'] : ['error'])] :
    ['id', 'sessionId', ...(hasResult ? ['result'] : ['error'])]);
  if (!Number.isSafeInteger(raw.id) || raw.id <= 0 || hasResult === hasError) throw probeError('cdp_response_invalid');
  const item = pending.get(raw.id);
  if (!item) throw probeError('cdp_response_unexpected');
  if ((safeId(raw.sessionId) ? raw.sessionId : null) !== item.sessionId) throw probeError('cdp_session_mismatch');
  pending.delete(raw.id);
  clearTimeout(item.timer);
  if (hasError) {
    item.reject(probeError(`${CDP_METHODS[item.method]}_failed`));
    return;
  }
  try { item.resolve(projectCDPResult(item.method, raw.result, item.projectionKind)); }
  catch (error) { item.reject(/^xnp_[a-z0-9_]{1,80}$/u.test(error?.message ?? '') ? error : probeError('cdp_result_invalid')); }
}

function onCDPData(chunk) {
  if (cdpFailure) return;
  try { cdpFrames.push(chunk, dispatchCDP); }
  catch (error) { failCDP(safeCode(error)); }
}

function bindChrome(processChild) {
  processChild.on('error', () => { childExited = true; failCDP('chrome_process_error'); });
  processChild.on('exit', () => {
    childExited = true;
    failCDP('chrome_exited');
    if (active?.owner) emit({ event: 'chrome_exit', owner: active.owner });
  });
  processChild.stdio[3]?.on('error', () => failCDP('cdp_pipe_error'));
  processChild.stdio[4]?.on('error', () => failCDP('cdp_pipe_error'));
  processChild.stdio[4]?.on('data', onCDPData);
  processChild.stdio[3]?.on('close', () => { if (!childExited) failCDP('cdp_pipe_closed'); });
  processChild.stdio[4]?.on('close', () => { if (!childExited) failCDP('cdp_pipe_closed'); });
}

function withTimeout(promise, code, ms) {
  let timer;
  return Promise.race([
    promise,
    new Promise((_, reject) => { timer = setTimeout(() => reject(probeError(code)), ms); }),
  ]).finally(() => clearTimeout(timer));
}

function deferred() {
  let resolvePromise;
  let rejectPromise;
  const promise = new Promise((resolve, reject) => {
    resolvePromise = resolve;
    rejectPromise = reject;
  });
  promise.catch(() => {});
  return { promise, resolve: resolvePromise, reject: rejectPromise };
}

async function validatePaths(mode = 'single_phase', allowOwnedProfile = false) {
  if (!isAbsolute(executable) || !isAbsolute(qaRoot)) throw probeError('arguments_invalid');
  const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
  const resolvedProject = await realpath(projectRoot).catch(() => '');
  const expectedParent = resolvedProject ? resolve(resolvedProject, 'runtime', 'qa', 'auth-session-sidecar') : '';
  const expectedParentReal = expectedParent ? await realpath(expectedParent).catch(() => '') : '';
  const rootStat = await lstat(qaRoot).catch(() => null);
  const rootReal = await realpath(qaRoot).catch(() => '');
  const rel = rootReal && expectedParentReal ? relative(expectedParentReal, rootReal) : '';
  const executableStat = await lstat(executable).catch(() => null);
  const rootName = rootReal.split(/[\\/]/u).at(-1);
  const rootNameValid = mode === 'two_phase'
    ? /^run-x-two-phase-[a-f0-9]{16,64}$/u.test(rootName)
    : mode === 'single_phase' && /^run-x-native-metadata-[A-Za-z0-9-]{1,80}$/u.test(rootName);
  if (!resolvedProject || !expectedParentReal || !rootStat?.isDirectory() || rootStat.isSymbolicLink() ||
      !rootReal || resolve(rootReal) !== resolve(qaRoot) || !rel || rel === '..' || rel.startsWith(`..${sep}`) ||
      isAbsolute(rel) || !rootNameValid ||
      !executableStat?.isFile() || executableStat.isSymbolicLink()) throw probeError('paths_invalid');
  const origin = parseControlOrigin(controlOrigin);
  const profile = join(rootReal, 'shared');
  if (allowOwnedProfile) {
    if (!retainedQAMode(driverMode) || mode !== 'two_phase' ||
        !retainedQAScope?.reuse || !sameWindowsPath(dirname(rootReal), expectedParentReal)) {
      throw probeError('retained_scope_required');
    }
    const identity = await ordinaryDirectoryIdentity(profile);
    const rootIdentity = await ordinaryDirectoryIdentity(rootReal);
    if (!sameDirectoryIdentity(identity, retainedQAScope.profileIdentity) ||
        !sameDirectoryIdentity(rootIdentity, retainedQAScope.rootIdentity)) throw probeError('owned_profile_changed');
  } else if (await lstat(profile).then(() => true, () => false)) throw probeError('profile_not_new');
  return { root: rootReal, profile, origin };
}

async function acceptRetainedQAScope(message) {
  if (!retainedQAMode(driverMode) || retainedQAScope || child || protocolOwner || twoPhaseState ||
      typeof message.nonce !== 'string' || !/^[a-f0-9]{64}$/u.test(message.nonce) || typeof message.reuse !== 'boolean') {
    throw probeError('retained_scope_invalid');
  }
  if ((driverMode === 'retained_bootstrap' || qualificationMode(driverMode)) && !message.reuse) throw probeError('retained_profile_required');
  // This private pipe message arrives only after Node is inside its owner's Job.
  // Identity is pinned by the Go lease; no profile path comes from the message.
  const rootIdentity = await ordinaryDirectoryIdentity(qaRoot);
  const project = await realpath(resolve(dirname(fileURLToPath(import.meta.url)), '..', '..'));
  const parent = await realpath(join(project, 'runtime', 'qa', 'auth-session-sidecar'));
  if (!sameWindowsPath(dirname(qaRoot), parent) || !/^run-x-two-phase-[a-f0-9]{32}$/u.test(qaRoot.split(/[\\/]/u).at(-1))) {
    throw probeError('retained_scope_invalid');
  }
  const profileIdentity = message.reuse ? await ordinaryDirectoryIdentity(join(qaRoot, 'shared')) : null;
  if (!message.reuse && await lstat(join(qaRoot, 'shared')).then(() => true, () => false)) throw probeError('profile_not_new');
  retainedQAScope = { reuse: message.reuse, rootIdentity, profileIdentity, consumed: false };
  return { scoped: true };
}

async function launchRetainedQAChrome(message) {
  const scope = retainedQAScope;
  if (!scope || scope.consumed || message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw probeError('retained_scope_required');
  }
  if (!scope.reuse) {
    if (!sameDirectoryIdentity(await ordinaryDirectoryIdentity(qaRoot), scope.rootIdentity)) throw probeError('owned_profile_changed');
    return launchTwoPhaseChrome(message);
  }
  scope.consumed = true;
  if (child || protocolOwner || cdpFailure || twoPhaseState) throw probeError('collector_launch_invalid');
  const paths = await validatePaths('two_phase', true);
  qaRoot = paths.root;
  profilePath = paths.profile;
  controlOrigin = paths.origin;
  twoPhaseState = { root: qaRoot, profile: profilePath, rootIdentity: scope.rootIdentity,
    profileIdentity: scope.profileIdentity, phase2Started: true };
  if (!(await verifyTwoPhaseDirectories())) throw probeError('owned_profile_changed');
  const processChild = spawn(executable, twoPhaseChromeArgs(profilePath, true),
    { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  child = processChild;
  childExited = false;
  bindChrome(processChild);
  if (!Number.isSafeInteger(processChild.pid) || processChild.pid <= 0) throw probeError('chrome_spawn_failed');
  return initializeChromeSession(processChild, profilePath);
}

async function initializeChromeSession(processChild, profileReal) {
  const targetDeadline = Date.now() + 10_000;
  let targetId = null;
  while (Date.now() <= targetDeadline) {
    const targets = await send('Target.getTargets');
    if (!Array.isArray(targets.targetIds)) throw probeError('target_inventory_invalid');
    if (targets.targetIds.length > 1) throw probeError('target_inventory_ambiguous');
    if (targets.targetIds.length === 1) {
      targetId = targets.targetIds[0];
      break;
    }
    await new Promise(resolvePromise => setTimeout(resolvePromise, 100));
  }
  if (!targetId) throw probeError('target_inventory_not_ready');
  const attached = await send('Target.attachToTarget', { targetId, flatten: true });
  const next = ++nextOwner;
  protocolOwner = next;
  active = {
    owner: next,
    pid: processChild.pid,
    sessionId: attached.sessionId,
    targetId,
    profileId: profileReal,
    gate: new LoginObservationGate(),
    pageLoad: null,
    metadata: null,
    observationEndsAt: 0,
    networkEventCount: 0,
    eventLimitReached: false,
  };
  await send('Page.enable', {}, active.sessionId);
  return { owner: next, pid: processChild.pid, windowCount: 0, bootstrapCount: 0, nativeReaderStartup: false };
}

async function launchSinglePhaseChrome(message) {
  if (protocolOwner || child || cdpFailure || message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw probeError('collector_launch_invalid');
  }
  const paths = await validatePaths();
  qaRoot = paths.root;
  profilePath = paths.profile;
  controlOrigin = paths.origin;
  try {
    await mkdir(profilePath, { recursive: false, mode: 0o700 });
    const defaultProfile = join(profilePath, 'Default');
    await mkdir(defaultProfile, { recursive: false, mode: 0o700 });
    await writeFile(join(defaultProfile, 'Preferences'), JSON.stringify({ profile: { name: 'AkuBrowser X Native QA' } }),
      { flag: 'wx', mode: 0o600 });
  } catch { throw probeError('profile_create_failed'); }
  const profileReal = await realpath(profilePath).catch(() => '');
  if (!profileReal || resolve(profileReal) !== resolve(profilePath)) throw probeError('profile_path_invalid');
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
  const processChild = spawn(executable, flags, { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  if (!processChild.pid) throw probeError('chrome_spawn_failed');
  child = processChild;
  childExited = false;
  bindChrome(processChild);
  return initializeChromeSession(processChild, profileReal);
}

async function launchTwoPhaseChrome(message) {
  if (message.role !== 'collector' || message.nativeReaderStartup !== false) {
    throw probeError('collector_launch_invalid');
  }
  if (!twoPhaseState) {
    if (protocolOwner || child || cdpFailure) throw probeError('collector_launch_invalid');
    const paths = await validatePaths('two_phase');
    qaRoot = paths.root;
    profilePath = paths.profile;
    controlOrigin = paths.origin;
    const rootIdentity = await ordinaryDirectoryIdentity(qaRoot);
    try {
      await mkdir(profilePath, { recursive: false, mode: 0o700 });
      const defaultProfile = join(profilePath, 'Default');
      await mkdir(defaultProfile, { recursive: false, mode: 0o700 });
      await writeFile(join(defaultProfile, 'Preferences'), JSON.stringify({ profile: { name: 'AkuBrowser X QA Two Phase' } }),
        { flag: 'wx', mode: 0o600 });
    } catch { throw probeError('profile_create_failed'); }
    const profileIdentity = await ordinaryDirectoryIdentity(profilePath);
    const owner = ++nextOwner;
    const processChild = spawn(executable, twoPhaseChromeArgs(profilePath, false),
      { windowsHide: false, stdio: 'ignore' });
    let resolveExit;
    const exitPromise = new Promise(resolvePromise => { resolveExit = resolvePromise; });
    const phase1 = { owner, pid: 0, exitCode: null, spawnError: false, exitPromise };
    child = processChild;
    childExited = false;
    processChild.on('error', () => { phase1.spawnError = true; });
    processChild.once('exit', code => {
      phase1.exitCode = Number.isInteger(code) ? code : null;
      childExited = true;
      resolveExit(phase1.exitCode);
    });
    await new Promise((resolvePromise, rejectPromise) => {
      processChild.once('spawn', resolvePromise);
      processChild.once('error', () => rejectPromise(probeError('chrome_spawn_failed')));
    });
    if (!Number.isSafeInteger(processChild.pid) || processChild.pid <= 0) throw probeError('chrome_spawn_failed');
    phase1.pid = processChild.pid;
    twoPhaseState = {
      root: qaRoot, profile: profilePath, rootIdentity, profileIdentity,
      phase1, grant: new TwoPhaseWorkflowGrant({ owner, pid: phase1.pid }), phase2Started: false,
    };
    return { owner, pid: processChild.pid, windowCount: 0, bootstrapCount: 0, nativeReaderStartup: false };
  }

  const state = twoPhaseState;
  if (state.phase2Started || !state.grant.authorized || !childExited || state.phase1.exitCode !== 0 ||
      state.phase1.spawnError || cdpFailure) throw probeError('phase2_not_authorized');
  const identityUnchanged = await verifyTwoPhaseDirectories();
  state.grant.consume({ identityUnchanged });
  state.phase2Started = true;
  const profileReal = await realpath(state.profile).catch(() => '');
  if (!profileReal || !sameWindowsPath(profileReal, state.profile)) throw probeError('owned_profile_changed');
  if (!(await verifyTwoPhaseDirectories())) throw probeError('owned_profile_changed');
  const processChild = spawn(executable, twoPhaseChromeArgs(profileReal, true),
    { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  child = processChild;
  childExited = false;
  bindChrome(processChild);
  if (!Number.isSafeInteger(processChild.pid) || processChild.pid <= 0) throw probeError('chrome_spawn_failed');
  return initializeChromeSession(processChild, profileReal);
}

async function launchChrome(message) {
  if (retainedQAMode(driverMode)) return launchRetainedQAChrome(message);
  return driverMode === 'two_phase' || accountSettingsBodyMode(driverMode)
    ? launchTwoPhaseChrome(message)
    : launchSinglePhaseChrome(message);
}

async function authorizeObservation(message) {
  if (driverMode !== 'two_phase' && !accountSettingsBodyMode(driverMode) || !twoPhaseState || twoPhaseState.phase2Started) {
    throw probeError('workflow_not_available');
  }
  const state = twoPhaseState;
  if (!childExited) {
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

function requireOwner(message) {
  if (!active || message.owner !== active.owner || childExited || cdpFailure) throw probeError('owner_unavailable');
  return active;
}

async function loginState(message) {
  const current = requireOwner(message);
  if (current.earlyRunning || current.earlyAttempted && !current.earlyCleanupVerified) throw probeError('early_cleanup_required');
  if (current.gate.observationStarted) throw probeError('login_poll_closed');
  const result = await send('Runtime.evaluate', {
    expression: ACCOUNT_READY_EXPRESSION,
    returnByValue: true,
    awaitPromise: false,
    userGesture: false,
  }, current.sessionId);
  current.gate.setLoginVisible(result.value);
  return { ready: result.value };
}

async function bootstrapCurrentSelf(message) {
  if (driverMode !== 'retained_bootstrap' || !retainedQAScope?.reuse) throw probeError('bootstrap_mode_required');
  const current = requireOwner(message);
  if (!current.gate.loginVisible || current.gate.observationStarted || current.bootstrapAttempted || current.earlyRunning ||
      current.earlyAttempted && (!current.earlyCleanupVerified || !current.earlyHandoffAccepted)) throw probeError('bootstrap_gate_closed');
  current.bootstrapAttempted = true;
  return send('Runtime.evaluate', { expression: BOOTSTRAP_CURRENT_SELF_EXPRESSION, returnByValue: true,
    awaitPromise: false, userGesture: false, throwOnSideEffect: true, timeout: 1500 }, current.sessionId);
}

async function earlyBootstrapCurrentSelf(message) {
  if (driverMode !== 'retained_bootstrap' || !retainedQAScope?.reuse) throw probeError('bootstrap_mode_required');
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
  if (driverMode !== 'retained_bootstrap' || !active || message.owner !== active.owner ||
      !active.earlyAttempted || active.earlyRunning || !active.earlyCommandStatus) throw probeError('early_diagnostics_unavailable');
  const commands = {};
  for (const label of EARLY_LABELS) {
    const status = active.earlyCommandStatus[label];
    if (!['not_attempted', 'sent', 'acknowledged', 'failed'].includes(status)) throw probeError('early_diagnostics_invalid');
    commands[label] = status;
  }
  const failureCategory = !cdpFailure ? 'none' : cdpFailure === 'cdp_frame_count_exceeded' ? 'frame_count_limit' :
    ['cdp_frame_too_large', 'cdp_input_invalid', 'cdp_json_invalid', 'cdp_utf8_invalid'].includes(cdpFailure) ? 'frame_invalid' :
    cdpFailure.endsWith('_timeout') ? 'command_timeout' : 'other';
  return { schema: 'aku.x.early-bootstrap-transport.v2', commands, failureCategory,
    capture: sanitizeEarlyHandoffDiagnostics(active.earlyHandoffDiagnostics), canAuthorize: false };
}

export function sanitizeEarlyHandoffDiagnostics(value) {
  // Reconstruct fixed fields only; no private correlator objects cross RPC.
  if (!value || value.mode !== 'instrumentation_to_normal_breakpoint' || value.pauseAdapter !== 'verified_normal_breakpoint_as_capture_eligible') throw probeError('early_diagnostics_invalid');
  const handoff = {};
  for (const key of ['installAttempts', 'installed', 'matched', 'unexpected', 'mismatched', 'stale', 'replayed', 'removed']) {
    const count = value.handoff?.[key];
    if (!Number.isSafeInteger(count) || count < 0 || count > (['installAttempts', 'installed', 'matched', 'removed'].includes(key) ? 8 : 64)) throw probeError('early_diagnostics_invalid');
    handoff[key] = count;
  }
  if (typeof value.handoff.normalCleanupVerified !== 'boolean' || !['not_started', 'waiting_for_normal', 'normal_hit_verified', 'install_failed', 'hit_rejected', 'cleanup_failed', 'no_normal_hit', 'normal_breakpoint_limit'].includes(value.handoff.outcome)) throw probeError('early_diagnostics_invalid');
  handoff.normalCleanupVerified = value.handoff.normalCleanupVerified;
  handoff.outcome = value.handoff.outcome;
  if (typeof value.handoffRejected !== 'boolean') throw probeError('early_diagnostics_invalid');
  const result = { mode: value.mode, pauseAdapter: value.pauseAdapter, handoffRejected: value.handoffRejected };
  for (const key of ['actualInstrumentationPauses', 'actualOtherPauses', 'pausedEvaluationAttempts']) {
    if (!Number.isSafeInteger(value[key]) || value[key] < 0 || value[key] > (key === 'pausedEvaluationAttempts' ? 8 : 64)) throw probeError('early_diagnostics_invalid');
    result[key] = value[key];
  }
  return { ...result, handoff };
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
  if (driverMode === 'retained_bootstrap') throw probeError('network_observation_not_available');
  if (qualificationMode(driverMode) && !active?.qualificationAttempted) throw probeError('qualification_mode_required');
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
  if (bodyMode(driverMode)) {
    await send('Network.setCacheDisabled', { cacheDisabled: true }, current.sessionId);
    const readBody = requestId => send('Network.getResponseBody', { requestId }, current.sessionId);
    if (qualificationMode(driverMode)) {
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
  return bodyMode(driverMode) ? { metadata: report, accountSettings } : report;
}

async function qualifySource(message) {
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse) throw probeError('qualification_mode_required');
  const current = requireOwner(message);
  if (!current.gate.loginVisible || current.qualificationAttempted) throw probeError('qualification_gate_closed');
  current.qualificationAttempted = true;
  const { accountSettings: source } = await startObservation(message);
  let cookies = null;
  const home = await loginState(message);
  if (home.ready && source.diagnostic === 'projected' && source.route && !cdpFailure) {
    current.qaSourceRoute = source.route;
    current.qaCookieRequested = true;
    cookies = await send('Network.getCookies', qaCookieRequest(source.route), current.sessionId);
  }
  return { schema: 'aku.x.qa-source-qualification.v1', source, cookies,
    canAuthorize: false, sessionTransferred: false };
}

async function closeChrome() {
  if (!child || childExited) return { exited: true };
  if ((driverMode === 'two_phase' || accountSettingsBodyMode(driverMode)) && !twoPhaseState?.phase2Started) return { exited: false };
  try { await send('Browser.close'); } catch { /* The enclosing Job remains the owner. */ }
  for (let attempt = 0; attempt < 200 && !childExited; attempt++) {
    await new Promise(resolvePromise => setTimeout(resolvePromise, 25));
  }
  return { exited: childExited };
}

async function execute(message) {
  if (!isObject(message) || !Number.isSafeInteger(message.id) || message.id <= 0 || typeof message.op !== 'string') {
    throw probeError('rpc_invalid');
  }
  switch (message.op) {
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
      const result = await closeChrome();
      if (!result.exited) throw probeError('chrome_exit_unverified');
      active = null;
      protocolOwner = 0;
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
    try { emitReply(id, await execute(message)); }
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
  cdpFrames.reset();
  rejectPending('control_closed');
  void closeChrome().finally(() => { process.exitCode = 0; });
}

export async function runNativeMetadataProbe(mode = 'single_phase') {
  if (!['single_phase', 'two_phase', 'two_phase_account_settings', 'retained_account_settings', 'retained_bootstrap', 'retained_source_qualification'].includes(mode) || driverMode !== 'single_phase' || child || twoPhaseState) {
    throw probeError('driver_mode_invalid');
  }
  driverMode = mode;
  const args = process.argv.slice(2);
  if (process.argv.length !== 5 || args.length !== 3 || args.some(value => !value)) {
    process.exitCode = 2;
    return;
  }
  executable = resolve(args[0]);
  qaRoot = resolve(args[1]);
  controlOrigin = parseControlOrigin(args[2]);
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
