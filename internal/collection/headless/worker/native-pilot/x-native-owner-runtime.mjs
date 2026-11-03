import {createCDPResultProjector} from './shared/x-native-cdp-result-projection.mjs';
import {registerFreshStartupSession, startupRootControls, allowsStartupControl, mustRetireStartupOwner, markStartupControlAttempt} from './shared/x-manual-root-controls.mjs';
import {createStartupTiming} from './shared/x-manual-startup-timing.mjs';
import {createBatchBootstrap} from './shared/x-batch-bootstrap.mjs';
import {sourceTransientClass, SOURCE_RETRY_LIMIT} from './source-retry-policy.mjs';
import {cdpCommandFailureDetail} from './cdp-command-failure.mjs';
import {cleanupDailyBatch} from './shared/x-daily-batch-cleanup.mjs';
import {spawn} from 'node:child_process';
import {lstat, realpath} from 'node:fs/promises';
import {dirname, isAbsolute, join, relative, resolve, sep} from 'node:path';
import {performance} from 'node:perf_hooks';
import {parseStrictJSON} from './shared/x-evidence-ingress.mjs';
import {standardChromeArgs} from './shared/x-standard-chrome-args.mjs';

import {accountSettingsBodyMode} from './shared/x-account-settings-capture.mjs';
import {BOOTSTRAP_CURRENT_SELF_EXPRESSION} from './shared/x-bootstrap-current-self.mjs';
import {BootstrapHandoffCapture} from './shared/x-bootstrap-breakpoint-handoff.mjs';
import {qaCookieRequest} from './shared/x-qa-source-qualification.mjs';
import {qaGuestCommand} from './shared/x-qa-auth-control.mjs';
import {qaABCommand, projectQADirectBody} from './shared/x-qa-direct-ab.mjs';
import {qaWebCommand, QA_WEB_SNAPSHOT, QA_WEB_START, QA_WEB_DISPOSE} from './shared/x-qa-web-aba.mjs';

import {QABoundObservation, createManualV2BoundObservation, QA_BOUND_BOOTSTRAP_EXPRESSION, projectBoundBootstrap} from './shared/x-qa-bound-observation.mjs';
import {QACookieWriteCapture, finishQACookieWriteScope} from './shared/x-qa-cookie-writes.mjs';
import {shortQACheckpoint} from './shared/x-qa-short-feasibility.mjs';
import {QACheckpointRendezvous} from './shared/x-qa-checkpoint-rendezvous.mjs';
import {createQAFeasibilityCollector} from './shared/x-qa-feasibility-collector.mjs';
import {QA_LIVE_SELF_EXPRESSION} from './shared/x-qa-live-self.mjs';
import {qaLiveSelfCommand} from './shared/x-qa-live-self-transport.mjs';
import {createManualStartupOwner} from './shared/x-manual-startup-owner.mjs';
import {createXManualOwnerPort, manualOwnerOperation} from './shared/x-native-manual-owner-port.mjs';
import {createRetainedManualSetup, manualSeedFailure, manualSeedDiagnosticCode} from './shared/x-manual-retained-setup.mjs';
import {ManualEnrollmentCapture} from './shared/x-manual-enrollment-capture.mjs';
import {ManualStartupObservation} from './shared/x-manual-startup-cookie-guard.mjs';
import {startupFailure, startupFailureLatch, requireBoundStartup, startupInspectionFailure, startupPrepareFailure, startupDiagnostic} from './shared/x-manual-startup-diagnostic.mjs';
import {MANUAL_TIMING} from './shared/x-manual-timing-policy.mjs';
import {createApplicationSeedGrant} from './shared/x-application-seed-grant.mjs';
import {createApplicationReaderPost, applicationPostOperation} from './shared/x-application-reader-post.mjs';

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
const COMMAND_TIMEOUT_MS = MANUAL_TIMING.cdpCommandMS;
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
  'Target.detachFromTarget': 'target_detach',
  'Target.closeTarget': 'qa_ab_close_target',
  'Target.createBrowserContext': 'qa_guest_context',
  'Target.createTarget': 'qa_guest_target',
  'Target.disposeBrowserContext': 'qa_guest_dispose',
  'Page.navigate': 'qa_guest_home',
  'Page.enable': 'page_enable',
  'Page.reload': 'page_reload',
  'Page.addScriptToEvaluateOnNewDocument': 'qa_web_install',
  'Page.removeScriptToEvaluateOnNewDocument': 'qa_web_remove',
  'Runtime.evaluate': 'runtime_evaluate',
  'Debugger.evaluateOnCallFrame': 'early_evaluate_call_frame',
  'Network.enable': 'network_enable',
  'Network.disable': 'network_disable',
  'Network.getResponseBody': 'account_settings_body',
  'Network.getCookies': 'qa_cookie_metadata',
  'Network.setCacheDisabled': 'account_settings_cache',
  'Network.setBypassServiceWorker': 'account_settings_cache',
  'Browser.close': 'browser_close',
  'Browser.getWindowForTarget': 'qa_short_window',
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
// Closed, value-free startup diagnostics. Only the shared transport's private
// brand can distinguish its command failures; arbitrary exception text cannot.
export function startupCDPFailureCategory(code,error){
  if(code==='borrowed_reader_command_failed'){
    switch(cdpCommandFailureDetail(error)?.kind){
      case 'command_rejected':return 'cdp_command_rejected';
      case 'request_rejected':return 'cdp_request_rejected';
      case 'reply_invalid':return 'cdp_reply_invalid';
      case 'transport_closed':return 'cdp_transport_closed';
      case 'timeout':return 'cdp_timeout';
      default:return 'cdp_borrowed_unclassified';
    }
  }
  if(code==='borrowed_reader_event_failed')return 'cdp_event_failure';
  if(['cdp_pipe_error','cdp_pipe_closed','cdp_write_failed'].includes(code))return 'cdp_pipe_failure';
  if(['chrome_process_error','chrome_exited'].includes(code))return 'cdp_process_failure';
  if(['cdp_frame_count_exceeded','cdp_frame_too_large','cdp_input_invalid','cdp_json_invalid','cdp_utf8_invalid'].includes(code))return 'cdp_frame_failure';
  if(code==='application_post_timeout'||Object.values(CDP_METHODS).some(name=>code===`${name}_timeout`))return 'cdp_timeout';
  return 'cdp_other';
}
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

function isTargetURL(rawURL) {
  if (typeof rawURL !== 'string' || rawURL.length > 2_048) return false;
  try {
    const parsed = new URL(rawURL);
    return parsed.origin === X_ORIGIN && !parsed.username && !parsed.password;
  } catch { return false; }
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

export function manualSeedRPCCode(error){
  return manualSeedDiagnosticCode(error)??'manual_seed_preconditions_unavailable_unknown';
}

// Own the command binding across await: the ingress parser clears its local
// message reference as soon as this bounded concurrent operation is started.

// Own one process scope; production composition keeps one instance for both factories.
export function createNativeOwnerRuntime(){
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

  let executable = '';
  let applicationScope = null;
  // Only the sealed main transport factory can inject this Node-local port.
  // Go owns the existing Chrome lifecycle; this port never grants browser close.
  let borrowedMainTransport = null;
  const manualStartupAllowed=()=>applicationScope?applicationScope.seedGrant?.startupAllowed()===true:process.env.AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED==='1';
  const retainedTransferAllowed=()=>applicationScope?applicationScope.seedGrant?.transferAllowed()===true:process.env.AKU_X_MANUAL_RETAINED_TRANSFER_ONCE==='1';
  const shortTransferAllowed=()=>applicationScope?applicationScope.seedGrant?.transferAllowed()===true:process.env.AKU_X_QA_SHORT_TRANSFER_ALLOWED==='1';
  let qaRoot = '';
  let controlOrigin = '';
  let profilePath = '';
  let driverMode = 'single_phase';
  let twoPhaseState = null;
  let retainedQAScope = null;
  let child = null;
  let childExited = false;
  let cdpFailure = null;
  let cdpFirstCause = null;
  let protocolOwner = 0;
  let nextOwner = 0;
  let nextCommandId = 0;
  let active = null;
  let cdpFrames = new CDPFrameDecoder();
  const pending = new Map();
  // Resolve live state only when a reply is projected.
  const projectCDPResult = createCDPResultProjector({
    getActive: () => active, getDriverMode: () => driverMode,
    bodyMode, isHomeTarget, isObject, isTargetURL, own, probeError, qualificationMode, safeId,
  });

  function emit(value, maxBytes = MAX_CONTROL_LINE_BYTES) {
    if(applicationScope){if(value?.event==='manual_startup_progress')applicationScope.emitProgress(value);return;}
    let bytes;
    try { bytes = Buffer.from(`${JSON.stringify(value)}\n`, 'utf8'); }
    catch { process.exitCode = 2; return; }
    if (bytes.byteLength > maxBytes) {
      bytes = Buffer.from(`${JSON.stringify({ id: value?.id ?? 0, ok: false, code: 'output_too_large' })}\n`, 'utf8');
    }
    process.stdout.write(bytes);
  }

  function emitReply(id, result = undefined, code = undefined, maxBytes = MAX_CONTROL_LINE_BYTES, diagnostic = undefined) {
    if (!Number.isSafeInteger(id) || id <= 0) return;
    if (code) emit({ id, ok: false, code, ...(diagnostic?{diagnostic}:{}) });
    else emit({ id, ok: true, result }, maxBytes);
  }

  function safeCode(error) {
    const manual=manualSeedDiagnosticCode(error);if(manual)return manual;
    const value = typeof error?.message === 'string' ? error.message : '';
    if (/^xnp_[a-z0-9_]{1,80}$/u.test(value)) return value.slice(4);
    return 'probe_failed';
  }

  // This is the seed command's sole error-code path. A patterned xnp_* message
  // from any other exception is insufficient to supply a seed diagnostic.
  function cdpWrite(message) {
    if (cdpFailure) throw probeError(cdpFailure);
    if (borrowedMainTransport) {
      if (message.method === 'Browser.close' || !borrowedMainTransport.alive()) throw probeError('borrowed_reader_unavailable');
      void borrowedMainTransport.call(message.method,message.params,message.sessionId??'',COMMAND_TIMEOUT_MS)
        .then(result=>{
          if(message.method==='Target.createTarget')borrowedMainTransport.reserveTarget?.(result.targetId);
          if(message.method==='Target.attachToTarget')borrowedMainTransport.reserveSession?.(result.sessionId);
          if(message.method==='Target.closeTarget'&&result.success===true)borrowedMainTransport.releaseTarget?.(message.params.targetId);
          if(message.method==='Target.detachFromTarget')borrowedMainTransport.releaseSession?.(message.params.sessionId);
          dispatchCDP({id:message.id,result,...(message.sessionId?{sessionId:message.sessionId}:{})});
        })
        .catch(error=>failCDP('borrowed_reader_command_failed',error));
      return;
    }
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

  function isEarlyBootstrapCallFrameRead(params, capture, boundIdentity = false) {
    const keys = ['callFrameId', 'expression', 'returnByValue', 'throwOnSideEffect', 'timeout',
      'silent', 'includeCommandLineAPI', 'generatePreview'];
    return isObject(params) && Object.keys(params).length === keys.length &&
      Object.keys(params).every(key => keys.includes(key)) && capture?.canReadCallFrame(params.callFrameId) === true &&
      params.expression === (boundIdentity ? QA_BOUND_BOOTSTRAP_EXPRESSION : BOOTSTRAP_CURRENT_SELF_EXPRESSION) && params.returnByValue === true &&
      params.throwOnSideEffect === true && params.timeout === MANUAL_TIMING.bootstrapEvaluationMS && params.silent === true &&
      params.includeCommandLineAPI === false && params.generatePreview === false;
  }

  function send(method, params = {}, sessionId = undefined, purpose = null) {
    if (cdpFailure) return Promise.reject(probeError(cdpFailure));
    if (!Object.hasOwn(CDP_METHODS, method) || !isObject(params) || pending.size >= MAX_PENDING_COMMANDS) {
      return Promise.reject(probeError('cdp_command_invalid'));
    }
    if(method==='Network.setBypassServiceWorker'&&!allowsStartupControl(active,method,params,sessionId))return Promise.reject(probeError('manual_root_control_failed'));
    const manualRootControl=purpose==='manual_root_control'&&qualificationMode(driverMode)&&manualStartupAllowed()&&active?.manualStartupSelected===true&&allowsStartupControl(active,method,params,sessionId);
    const webState = active?.webABA;
    const manualProvenance=purpose==='manual_provenance'&&qualificationMode(driverMode)&&manualStartupAllowed()&&retainedTransferAllowed()&&!!active?.manualSetup&&!active.manualStartupPort&&sessionId===active.sessionId&&(
      method==='Page.getFrameTree'&&Object.keys(params).length===0||method==='Network.setCacheDisabled'&&!active.manualCacheControlReady&&JSON.stringify(params)==='{"cacheDisabled":true}'||method==='Network.enable'&&JSON.stringify(params)===JSON.stringify({maxTotalBufferSize:MAX_TOTAL_NETWORK_BUFFER_BYTES,maxResourceBufferSize:MAX_RESOURCE_NETWORK_BUFFER_BYTES,maxPostDataSize:0}));
    const manualEnrollment=purpose==='manual_enrollment'&&retainedTransferAllowed()&&qualificationMode(driverMode)&&active?.manualEnrollment?.command(method,params,sessionId)===true;
    if(method==='Target.detachFromTarget'&&!manualEnrollment)return Promise.reject(probeError('manual_enrollment_detach_denied'));
    const manualControl=!!active?.manualStartupPort&&purpose==='manual_owner'&&(
      method==='Target.getTargets'&&sessionId===undefined&&Object.keys(params).length===0||
      method==='Browser.getWindowForTarget'&&sessionId===undefined&&Object.keys(params).join(',')==='targetId'&&active.manualInventoryTargets?.has(params.targetId)||
      method==='Target.attachToTarget'&&sessionId===undefined&&Object.keys(params).sort().join(',')==='flatten,targetId'&&params.flatten===true&&params.targetId===active.targetId||
      method==='Target.createTarget'&&sessionId===undefined&&JSON.stringify(params)===JSON.stringify({url:HOME_URL,newWindow:true,background:true})||
      ['Page.enable','Runtime.enable'].includes(method)&&sessionId===active.manualObserver?.sessionId&&Object.keys(params).length===0||
      ['Page.getFrameTree','Runtime.disable'].includes(method)&&sessionId===active.sessionId&&Object.keys(params).length===0);
    const liveCommand = active?.liveSelf?.running === true;
    if (liveCommand && (!qualificationMode(driverMode) || process.env.AKU_X_QA_LIVE_SELF !== '1' ||
        sessionId !== active.sessionId || purpose !== null || !qaLiveSelfCommand(method, params, QA_LIVE_SELF_EXPRESSION))) return Promise.reject(probeError('qa_live_command_denied'));
    const shortCookieExport = method === 'Network.getCookies' && active?.shortFeasibility?.exporting === true;
    if (method === 'Browser.getWindowForTarget' && !manualControl && (!active?.shortFeasibility || sessionId !== undefined ||
        Object.keys(params).join(',') !== 'targetId' || params.targetId !== active.targetId)) return Promise.reject(probeError('qa_short_window_denied'));
    const bound = active?.bound;
    const boundNetwork = bound?.running && sessionId === active.sessionId && method.startsWith('Network.');
    if (boundNetwork) {
      const valid = method === 'Network.getResponseBody' ? Object.keys(params).join(',') === 'requestId' && bound.binder?.capture.consumeRead(params.requestId) :
        method === 'Network.enable' ? JSON.stringify(params) === JSON.stringify({ maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES, maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }) :
        method === 'Network.setCacheDisabled' ? JSON.stringify(params) === '{"cacheDisabled":true}'||allowsStartupControl(active,method,params,sessionId) : method === 'Network.setBypassServiceWorker' ? allowsStartupControl(active,method,params,sessionId) : method === 'Network.disable' && Object.keys(params).length === 0;
      if (!qualificationMode(driverMode) || !valid) return Promise.reject(probeError('qa_bound_command_not_authorized'));
    }
    const webMethod = ['Page.addScriptToEvaluateOnNewDocument', 'Page.removeScriptToEvaluateOnNewDocument'].includes(method);
    const webCommand = !!webState?.running && sessionId === active.sessionId && (method !== 'Runtime.evaluate' ||
      [QA_WEB_SNAPSHOT, QA_WEB_START, QA_WEB_DISPOSE].includes(params.expression));
    if (webMethod && !webCommand || webCommand && (!qualificationMode(driverMode) || !qaWebCommand(method, params, webState))) return Promise.reject(probeError('qa_web_command_not_authorized'));
    const guestMethod = ['Target.createBrowserContext', 'Target.createTarget', 'Target.disposeBrowserContext', 'Page.navigate'].includes(method);
    const guestSession = !!active?.guest?.sessionId && sessionId === active.guest.sessionId;
    const guestAttach = method === 'Target.attachToTarget' && active?.authControlAttempted;
    const guestCommand = guestMethod || guestSession || guestAttach;
    const abCommand = !!active?.directABRunning && (guestMethod || method === 'Target.attachToTarget' ||
      method === 'Target.closeTarget' || !!active.ab?.sessionId && sessionId === active.ab.sessionId);
    if (method === 'Target.closeTarget' && !abCommand) return Promise.reject(probeError('qa_ab_command_not_authorized'));
    if (abCommand && (!qualificationMode(driverMode) || !qaABCommand(method, params, sessionId, active.ab))) return Promise.reject(probeError('qa_ab_command_not_authorized'));
    if (!manualEnrollment && !manualControl && !abCommand && guestCommand && (!qualificationMode(driverMode) || !active?.authControlAttempted ||
        !qaGuestCommand(method, params, sessionId, active.guest))) return Promise.reject(probeError('qa_guest_command_not_authorized'));
    if (purpose !== null && !manualRootControl && !manualControl && !manualEnrollment && !manualProvenance && !(purpose === 'normal_remove' && method === 'Debugger.removeBreakpoint' &&
        active?.earlyCapture?.handoff.breakpoints.has(params.breakpointId))) return Promise.reject(probeError('early_command_not_authorized'));
    if (shortCookieExport && (!shortTransferAllowed() || active.shortFeasibility.exportIssued ||
        sessionId !== active.sessionId || JSON.stringify(params) !== JSON.stringify(qaCookieRequest('api_v1')))) return Promise.reject(probeError('qa_short_transfer_denied'));
    if (method === 'Network.getCookies' && !shortCookieExport && (!qualificationMode(driverMode) || !active?.qaCookieRequested || active.qaCookieIssued ||
        sessionId !== active.sessionId || JSON.stringify(params) !== JSON.stringify(qaCookieRequest(active.qaSourceRoute)))) {
      return Promise.reject(probeError('cookie_read_not_authorized'));
    }
    if (method === 'Network.getCookies') active.qaCookieIssued = true;
    if (shortCookieExport) active.shortFeasibility.exportIssued = true;
    if (method === 'Network.getResponseBody' && !boundNetwork && !webCommand && !abCommand && (!bodyMode(driverMode) || !active?.gate.networkEnabled ||
        sessionId !== active.sessionId || Object.keys(params).join(',') !== 'requestId' || !active.accountSettings?.consumeRead(params.requestId))) {
      return Promise.reject(probeError('account_body_not_authorized'));
    }
    if (method === 'Network.setCacheDisabled' && !allowsStartupControl(active,method,params,sessionId) && !webCommand && !abCommand && !guestCommand && (!bodyMode(driverMode) || sessionId !== active?.sessionId ||
        Object.keys(params).join(',') !== 'cacheDisabled' || params.cacheDisabled !== true)) return Promise.reject(probeError('account_cache_not_authorized'));
    if (!manualControl && !manualEnrollment && !manualProvenance && !liveCommand && !webCommand && !abCommand && !guestCommand && (EARLY_CDP_METHODS.has(method) || method === 'Page.reload' && (driverMode === 'retained_bootstrap' || bound?.running))) {
      const keys = Object.keys(params);
      let valid = keys.length === 0;
      if (method === 'Debugger.setInstrumentationBreakpoint') valid = keys.length === 1 && params.instrumentation === 'beforeScriptExecution';
      if (method === 'Debugger.removeBreakpoint') valid = keys.length === 1 && safeId(params.breakpointId) &&
        (purpose === 'normal_remove' ? active?.earlyCapture?.handoff.breakpoints.has(params.breakpointId) : params.breakpointId === active?.earlyInstrumentationId);
      if (method === 'Debugger.setBreakpoint') valid = keys.length === 1 && keys[0] === 'location' && active?.earlyCapture?.handoff.canInstall(params.location);
      if (method === 'Page.reload') valid = keys.length === 1 && params.ignoreCache === false;
      const qualificationFrame = qualificationMode(driverMode) && method === 'Page.getFrameTree' &&
        active?.qualificationAttempted && active.gate.observationStarted && keys.length === 0;
      if (!qualificationFrame && (!(driverMode === 'retained_bootstrap' || bound?.running) || !active?.earlyRunning || sessionId !== active.sessionId || !valid) ||
          qualificationFrame && sessionId !== active.sessionId) {
        return Promise.reject(probeError('early_command_not_authorized'));
      }
    }
    let projectionKind = webCommand ? 'qa_web' : abCommand ? 'qa_ab' : guestCommand ? 'qa_guest' : null;
    if(bound?.running && active?.manualStartupSelected && method==='Page.getFrameTree')projectionKind='manual_frame';
    if(manualRootControl)projectionKind='manual_root_control';
    if(manualProvenance&&method==='Page.getFrameTree')projectionKind='manual_frame';
    if(manualControl)projectionKind=method==='Page.getFrameTree'?'manual_frame':method==='Target.getTargets'?'manual_inventory':null;
    if(manualEnrollment)projectionKind='manual_enrollment_capture';
    if (liveCommand) projectionKind = 'qa_live_self';
    if (shortCookieExport) projectionKind = 'qa_short_cookies';
    if (boundNetwork && method === 'Network.getResponseBody') projectionKind = 'qa_bound';
    if (webCommand && method === 'Network.getResponseBody' && webState.qualify401 && webState.stage === 'b') projectionKind = 'qa_web_401';
    if (method === 'Debugger.evaluateOnCallFrame'&&!manualEnrollment) {
      if (!(driverMode === 'retained_bootstrap' || bound?.running) || !active?.earlyRunning || active.gate.observationStarted && !bound?.running ||
          sessionId !== active.sessionId || !isEarlyBootstrapCallFrameRead(params, active.earlyCapture, bound?.running === true)) {
        return Promise.reject(probeError('early_call_frame_not_authorized'));
      }
      projectionKind = 'bootstrap_current_self';
    }
    if (method === 'Runtime.evaluate' && webCommand) {
      projectionKind = params.expression === QA_WEB_SNAPSHOT ? 'qa_web_hook' : 'qa_web';
    } else if (method === 'Runtime.evaluate' && !liveCommand && !manualEnrollment) {
      const bootstrap = params.expression === BOOTSTRAP_CURRENT_SELF_EXPRESSION;
      const keys = bootstrap ? ['expression', 'returnByValue', 'awaitPromise', 'userGesture', 'throwOnSideEffect', 'timeout'] :
        ['expression', 'returnByValue', 'awaitPromise', 'userGesture'];
      if (Object.keys(params).length !== keys.length || Object.keys(params).some(key => !keys.includes(key)) ||
          params.returnByValue !== true || params.awaitPromise !== false || params.userGesture !== false || sessionId !== active?.sessionId ||
          bootstrap && (driverMode !== 'retained_bootstrap' ||
            (!active?.bootstrapAttempted || !active.gate.loginVisible || active.earlyRunning) ||
            active.gate.observationStarted || params.throwOnSideEffect !== true || params.timeout !== MANUAL_TIMING.bootstrapEvaluationMS) ||
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

  // This writer is received only by the fixed application-post recipe. No caller
  // can select a method/parameter envelope through the private owner protocol.
  function sendApplicationPost(method,params={},sessionId){
    if(!applicationScope||!['Target.getTargets','Browser.getWindowForTarget','Target.createTarget','Target.attachToTarget','Target.detachFromTarget','Page.enable','Page.getFrameTree','Page.navigate'].includes(method)||cdpFailure||pending.size>=MAX_PENDING_COMMANDS)return Promise.reject(probeError('application_post_unavailable'));
    const id=++nextCommandId;
    if(!Number.isSafeInteger(id)||id<1)return Promise.reject(probeError('application_post_unavailable'));
    return new Promise((accept,reject)=>{
      const timer=setTimeout(()=>{if(pending.has(id))failCDP('application_post_timeout');},COMMAND_TIMEOUT_MS);
      pending.set(id,{method,projectionKind:'application_post',sessionId:sessionId??null,resolve:accept,reject,timer});
      try{cdpWrite({id,method,params,...(sessionId?{sessionId}:{})});}
      catch{pending.delete(id);clearTimeout(timer);reject(probeError('application_post_unavailable'));}
    });
  }

  function rejectPending(code) {
    for (const [id, item] of pending) {
      clearTimeout(item.timer);
      pending.delete(id);
      item.reject(probeError(code));
    }
  }

  function failCDP(code,error) {
    if(!cdpFailure)cdpFirstCause=startupCDPFailureCategory(code,error);
    active?.manualEnrollment?.invalidate();
    active?.manualStartupPort?.invalidate(cdpFirstCause);
    active?.bound?.binder?.invalidate();
    if (!cdpFailure) cdpFailure = /^[a-z0-9_]{1,80}$/u.test(code) ? code : 'cdp_protocol_failed';
    cdpFrames.reset();
    rejectPending(cdpFailure);
    active?.accountSettings?.invalidate();
    active?.guest?.capture?.invalidate();
    active?.ab?.capture?.invalidate();
    active?.webABA?.capture?.invalidate();
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
      active?.manualEnrollment?.observe(raw);
      applicationScope?.readerPost?.observe(raw);
      active?.manualCookieGuard?.observe(raw);
      if(active?.manualObserver&&raw.sessionId===active.manualObserver.sessionId&&raw.method==='Runtime.executionContextCreated'){
        const context=raw.params?.context;
        if(context?.auxData?.isDefault&&context.auxData.frameId===active.manualObserver.frameId&&context.origin===X_ORIGIN)active.manualObserver.context=context.id;
      }
      active?.manualStartupPort?.observe('reader',raw);
      if (active?.liveSelf?.running) active.liveSelf.capture?.observe(raw);
      if (active?.bound?.sampling) active.bound.binder?.observe(raw, boundScope(active));
      if (active?.webABA?.running && raw.sessionId === active.sessionId && typeof raw.method === 'string' && raw.method.startsWith('Network.')) {
        const state = active.webABA;
        if (Date.now() <= state.endsAt) {
          if (++state.events > MAX_NETWORK_EVENTS) state.capture?.invalidate();
          else state.capture?.observe(raw.method, raw.params);
        }
        return;
      }
      if (active?.ab?.sessionId && raw.sessionId === active.ab.sessionId) {
        const ab = active.ab;
        if (!ab.closed && Date.now() <= ab.endsAt && typeof raw.method === 'string' && raw.method.startsWith('Network.')) {
          if (++ab.events > MAX_NETWORK_EVENTS) ab.capture?.invalidate();
          else ab.capture?.observe(raw.method, raw.params);
        }
        if (raw.method === 'Inspector.detached' && !ab.disposing) ab.capture?.invalidate();
        return;
      }
      if (raw.method === 'Target.detachedFromTarget' && raw.params?.sessionId === active?.ab?.sessionId && !active?.ab?.disposing) active?.ab?.capture?.invalidate();
      if (active?.guest?.sessionId && raw.sessionId === active.guest.sessionId) {
        const guest = active.guest;
        if (!guest.closed && Date.now() <= guest.endsAt && typeof raw.method === 'string' && raw.method.startsWith('Network.')) {
          if (++guest.events > MAX_NETWORK_EVENTS) guest.capture?.invalidate();
          else guest.capture?.observe(raw.method, raw.params);
        }
        if (raw.method === 'Inspector.detached') guest.capture?.invalidate();
        return;
      }
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

  async function initializeChromeSession(processChild, profileReal, borrowedTarget = null) {
    const targetDeadline = Date.now() + 10_000;
    let targetId = borrowedTarget;
    while (!targetId && Date.now() <= targetDeadline) {
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
    registerFreshStartupSession(active,active.sessionId,'reader');
    await send('Page.enable', {}, active.sessionId);
    return { owner: next, pid: processChild.pid, windowCount: 0, bootstrapCount: 0, nativeReaderStartup: false };
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

  function sanitizeEarlyHandoffDiagnostics(value) {
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

  function boundScope(current) {
    return { owner: current.manualStartupSelected ? current.pid : current.owner, profileId: current.profileId, sessionId: current.sessionId, targetId: current.targetId };
  }

  async function boundCheckpoint(current, cookieWriteScope = false, pointOptions, afterIdentity, beforeIdentityRead,initialFailure, observationFactory = (...args) => new QABoundObservation(...args), progress=()=>{}, waitForSettings) {
    current.earlyRunning = true;
    const bound = current.bound = { running: true, sampling: false, binder: null, networkDisabled: false };
    let detachSettings = () => {};
    const controls=current.manualStartupSelected?startupRootControls(current,send,'reader'):null;
    let checkpointFailure;
    try {
      const { frameId, loaderId: oldLoader } = await send('Page.getFrameTree', {}, current.sessionId);
      const writes = cookieWriteScope ? new QACookieWriteCapture(frameId) : null;
      bound.binder = observationFactory(boundScope(current), frameId,
        requestId => send('Network.getResponseBody', { requestId }, current.sessionId), () => performance.now(),
        writes ? (raw, phase, loader) => writes.observe(raw, phase, loader) : undefined, pointOptions);
      detachSettings = bound.binder.onSettingsWait(state => progress(undefined, state));
      bound.sampling = true;
      if(!current.manualCookieGuard)await send('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
        maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }, current.sessionId);
      if(controls)await controls.apply();else await send('Network.setCacheDisabled', { cacheDisabled: true }, current.sessionId);
      let synchronized = false;
      current.earlyCapture = new BootstrapHandoffCapture({ boundIdentity: true,
        beforeIdentityRead: contextId => beforeIdentityRead?.(bound.binder.startupPins(contextId)),
        send: async (method, params, purpose) => {
          if(method==='Page.reload' && current.manualStartupSelected)bound.binder.armManualReload(oldLoader);
          let result;
          if(method==='Debugger.evaluateOnCallFrame')progress('identity');
          if(['Runtime.disable','Debugger.disable','Debugger.removeBreakpoint'].includes(method))progress('cleanup');
          try{result=await send(method,params,current.sessionId,purpose);}
          catch(error){throw initialFailure?initialFailure(error,method==='Debugger.evaluateOnCallFrame'?'identity':['Runtime.disable','Debugger.disable','Debugger.removeBreakpoint'].includes(method)?'cleanup':'prepare'):error;}
          if(method==='Page.getFrameTree' && current.manualStartupSelected) {
            try{result=manualReloadFrameProjection(result,frameId,oldLoader);}
            catch(error){bound.binder.invalidate('scope_lost','navigation');throw error;}
          }
          if (afterIdentity && !synchronized && method === 'Debugger.evaluateOnCallFrame' && bound.binder.identityReady()) {
            synchronized = true; await afterIdentity();
          }
          return result;
        },
        beforeRuntimeDisable: async () => {
          progress('settings_wait', bound.binder.settingsWaitState());
          // Finish this one document's request before disabling context-loss
          // observation. No second reload, negative control or cookie inspection.
          let timer;
          try {
            if (waitForSettings) {
              const result = await waitForSettings(bound.binder.done);
              if (result?.status !== 'complete') throw startupFailure('reader','settings','source_unavailable');
            }
            else if (current.manualStartupSelected) throw probeError('manual_startup_budget_invalid');
            else await Promise.race([bound.binder.done, new Promise(resolve => { timer = setTimeout(resolve, MANUAL_TIMING.settingsFallbackMS); })]);
          }
          catch(error) { throw initialFailure ? initialFailure(error, 'settings') : error; }
          finally { clearTimeout(timer); bound.sampling = false; }
        },
      });
      const early = await current.earlyCapture.run();
      const handoff = current.earlyCapture.diagnostics();
      current.earlyCleanupVerified = early.cleanupVerified;
      current.earlyHandoffAccepted = !handoff.handoffRejected;
      bound.binder.earlyOutcome(early.stopReason,handoff.handoffRejected);
      const continuousObservation = current.manualCookieGuard ? await bound.binder.finish(early.cleanupVerified && !handoff.handoffRejected) : null;
      await send('Network.disable', {}, current.sessionId);
      bound.networkDisabled = true;
      const observation = continuousObservation ?? await bound.binder.finish(early.cleanupVerified && !handoff.handoffRejected);
      return writes ? finishQACookieWriteScope(observation, writes.finish()) : observation;
    } catch(error) { checkpointFailure=error;throw error;
    } finally {
      detachSettings();
      bound.sampling = false;
      if (!bound.networkDisabled) await send('Network.disable', {}, current.sessionId).catch(() => { bound.binder?.invalidate('scope_lost','cleanup_failed'); });
      let restoreFailure;
      if(controls){progress('root_restore');try{await controls.restore();current.manualControlsCleanup='verified';}catch(error){current.manualControlsCleanup='uncertain';initialFailure?.(error,'cleanup');restoreFailure=error;}}
      bound.running = false; current.earlyRunning = false; current.earlyCapture = null;
      current.gate.setLoginVisible(false);
      if(restoreFailure&&!checkpointFailure)throw restoreFailure;
    }
  }

  async function prepareShortConcurrency(message) {
    const current = requireOwner(message);
    if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !current.gate.loginVisible ||
        current.gate.observationStarted || current.shortFeasibility) throw probeError('qa_short_gate_closed');
    const transport = (applicationScope?.collectorFactory??createQAFeasibilityCollector)({ executable, root: process.env.AKU_CHROME_FIXTURE_ROOT,
      ...(applicationScope?{applicationProfile:applicationScope.collectorProfile,loadAssets:applicationScope.loadAssets}:{}),
      FrameDecoder: CDPFrameDecoder, allowedExpressions: [ACCOUNT_READY_EXPRESSION, QA_BOUND_BOOTSTRAP_EXPRESSION] });
    const state = current.shortFeasibility = { transport, exportIssued: false, exporting: false, contextId: null, closed: false, ran: false };
    const launched = await transport.launch();
    if (!Number.isSafeInteger(launched.pid) || launched.pid <= 0 || launched.pid === current.pid) throw probeError('qa_short_collector_invalid');
    const context = await transport.send('Target.createBrowserContext', { disposeOnDetach: true }, undefined, result => {
      if (!safeId(result.browserContextId)) throw probeError('qa_short_collector_invalid');
      return { id: result.browserContextId };
    });
    state.contextId = context.id;
    const target = await transport.send('Target.createTarget', { url: 'about:blank', browserContextId: context.id }, undefined, result => {
      if (!safeId(result.targetId)) throw probeError('qa_short_collector_invalid');
      return { id: result.targetId };
    });
    state.targetId = target.id;
    const attached = await transport.send('Target.attachToTarget', { targetId: target.id, flatten: true }, undefined, result => {
      if (!safeId(result.sessionId)) throw probeError('qa_short_collector_invalid');
      return { id: result.sessionId };
    });
    state.sessionId = attached.id;
    registerFreshStartupSession(state,state.sessionId,'collector');
    await transport.send('Page.enable', {}, state.sessionId, () => ({ acknowledged: true }));
    state.windowId = (await send('Browser.getWindowForTarget', { targetId: current.targetId })).windowId;
    return { collectorPID: launched.pid, headless: true, isolatedContext: true, distinctProfile: true, seedAttempted: false };
  }

  async function closeShortCollector(current) {
    const s = current.shortFeasibility;
    if (!s || s.closed) return true;
    let disposed = false;
    try {
      if (s.contextId) {
        await s.transport.send('Target.disposeBrowserContext', { browserContextId: s.contextId }, undefined, () => ({ acknowledged: true }));
        disposed = await s.transport.send('Target.getBrowserContexts', {}, undefined,
          r => Array.isArray(r.browserContextIds) && !r.browserContextIds.includes(s.contextId));
      } else disposed = true;
    } finally { await s.transport.close(); s.closed = true; }
    return disposed && s.closed;
  }

  async function collectorShortCheckpoint(current, afterIdentity, onPrivateStartup, beforeIdentityRead,initialFailure, observationFactory = (...args) => new QABoundObservation(...args), progress=()=>{}, waitForSettings) {
    const s = current.shortFeasibility;
    // Classify at origin, before the owner-level identity fallback. Never inspect
    // transport exception text or overwrite an already branded first cause.
    const failure = (error, phase, reason = 'unavailable') => initialFailure ? initialFailure(error, phase, reason) : error;
    if (!s.navigated) {
      s.navigated = true;
      try {
        await s.transport.send('Page.navigate', { url: HOME_URL }, s.sessionId, r => {
          if (r.errorText || !safeId(r.frameId)) throw failure(probeError('qa_short_collector_invalid'), 'navigation', 'rejected');
          return { acknowledged: true };
        });
      } catch (error) { throw failure(error, 'navigation'); }
      const until = Date.now() + MANUAL_TIMING.collectorHomeReadyMS;
      let ready = false;
      while (!ready && Date.now() < until) {
        try {
          ready = await s.transport.send('Runtime.evaluate', { expression: ACCOUNT_READY_EXPRESSION, returnByValue: true, awaitPromise: false, userGesture: false }, s.sessionId,
            r => r.result?.type === 'boolean' && r.result.value === true);
        } catch (error) { throw failure(error, 'page_ready'); }
        if (!ready) await new Promise(resolve => setTimeout(resolve, 200));
      }
      if (!ready) {
        if (initialFailure) throw failure(probeError('qa_short_collector_invalid'), 'page_ready', 'deadline');
        return { checkpoint: shortQACheckpoint(null), accountId: null, start: null, end: null };
      }
    }
    let binder, capture, instrumentationId, frameId, oldLoader, sampling = false, networkDisabled = false, synchronized = false, privateResult = {};
    let detachSettings = () => {};
    const owned = current.manualStartupSelected ? s.transport.ownership() : null;
    const scope = { owner: owned ? owned.pid : current.owner + 1, profileId: owned ? owned.profileId : 'derived-headless-owned-context', sessionId: s.sessionId, targetId: s.targetId };
    const project = (method, r) => {
      if (!isObject(r)) throw probeError('qa_short_collector_invalid');
      if (method === 'Page.getFrameTree') {
        if (!safeId(r.frameTree?.frame?.id) || !isHomeTarget(r.frameTree.frame.url)) throw probeError('qa_short_collector_invalid');
        if(current.manualStartupSelected && !safeId(r.frameTree.frame.loaderId))throw probeError('qa_short_collector_invalid');
        return { frameId: r.frameTree.frame.id, ...(current.manualStartupSelected?{loaderId:r.frameTree.frame.loaderId}:{}) };
      }
      if (method === 'Debugger.setInstrumentationBreakpoint') {
        if (!safeId(r.breakpointId)) throw probeError('qa_short_collector_invalid');
        instrumentationId = r.breakpointId; return { breakpointId: r.breakpointId };
      }
      if (method === 'Debugger.setBreakpoint') {
        capture.handoff.created(r.breakpointId);
        const requested = capture.handoff.installing, location = r.actualLocation;
        if (!requested || !location || location.scriptId !== requested.scriptId || !Number.isSafeInteger(location.lineNumber) ||
            !Number.isSafeInteger(location.columnNumber) || location.lineNumber < 0 || location.columnNumber < 0 ||
            location.lineNumber > 1000000 || location.columnNumber > 1000000) throw probeError('qa_short_collector_invalid');
        return { breakpointId: r.breakpointId, actualLocation: { scriptId: location.scriptId, lineNumber: location.lineNumber, columnNumber: location.columnNumber } };
      }
      if (method === 'Debugger.evaluateOnCallFrame') {
        if (r.exceptionDetails || r.result?.type !== 'object') throw probeError('qa_short_collector_invalid');
        return projectBoundBootstrap(r.result.value, (id, p) => binder.acceptIdentity(id, p));
      }
      if (method === 'Network.getResponseBody') return projectQADirectBody(r);
      return { acknowledged: true };
    };
    const command = async (method, params = {}, purpose) => {
      if((method==='Network.setBypassServiceWorker'||method==='Network.setCacheDisabled'&&params.cacheDisabled!==true)&&!allowsStartupControl(s,method,params,s.sessionId))throw probeError('manual_root_control_failed');
      if(method==='Page.reload' && current.manualStartupSelected)binder.armManualReload(oldLoader);
      if (method === 'Debugger.setBreakpoint' && !capture?.handoff.canInstall(params.location)) throw probeError('qa_short_collector_invalid');
      if (method === 'Network.getResponseBody' && !binder?.capture.consumeRead(params.requestId)) throw probeError('qa_short_collector_invalid');
      if (method === 'Debugger.evaluateOnCallFrame' && !isEarlyBootstrapCallFrameRead(params, capture, true)) throw probeError('qa_short_collector_invalid');
      if (method === 'Debugger.removeBreakpoint' && !(purpose === 'normal_remove' ? capture.handoff.breakpoints.has(params.breakpointId) : params.breakpointId === instrumentationId)) throw probeError('qa_short_collector_invalid');
      let result;
      if(method==='Debugger.evaluateOnCallFrame')progress('identity');
      if(['Runtime.disable','Debugger.disable','Debugger.removeBreakpoint'].includes(method))progress('cleanup');
      try{result=await s.transport.send(method,params,s.sessionId,r=>{if(purpose==='manual_root_control'&&(!isObject(r)||Reflect.ownKeys(r).length!==0))throw probeError('manual_root_control_ack');return project(method,r);});}
      catch(error){throw initialFailure?initialFailure(error,method==='Debugger.evaluateOnCallFrame'?'identity':['Runtime.disable','Debugger.disable','Debugger.removeBreakpoint'].includes(method)?'cleanup':method.startsWith('Network.')?'settings':'prepare'):error;}
      if(method==='Page.getFrameTree' && binder && current.manualStartupSelected) {
        try{result=manualReloadFrameProjection(result,frameId,oldLoader);}
        catch(error){binder.invalidate('scope_lost','navigation');throw error;}
      }
      if (afterIdentity && !synchronized && method === 'Debugger.evaluateOnCallFrame' && binder.identityReady()) {
        synchronized = true; await afterIdentity();
      }
      return result;
    };
    const controls=current.manualStartupSelected?startupRootControls(s,(method,params)=>command(method,params,'manual_root_control'),'collector'):null;
    let checkpointFailure;
    const unsubscribe = s.transport.subscribe(raw => {
      if (!sampling) return;
      try {
        binder.observe(raw, scope);
        const event = projectCDPEvent(raw);
        if (event?.type === 'early' && event.sessionId === s.sessionId) capture?.observe(event.event, raw.params);
      } catch { binder?.invalidate(); }
    });
    try {
      ({ frameId, loaderId:oldLoader } = await command('Page.getFrameTree'));
      const pointOptions = { deferCookieMutation: !onPrivateStartup, onPrivateResult: r => { privateResult = r; } };
      if (onPrivateStartup) pointOptions.onPrivateStartup = onPrivateStartup;
      try {
        binder = observationFactory(scope, frameId, requestId => command('Network.getResponseBody', { requestId }),
          () => performance.now(), undefined, pointOptions);
      } catch (error) { throw failure(error, 'guard_bind', 'rejected'); }
      detachSettings = binder.onSettingsWait(state => progress(undefined, state));
      sampling = true;
      if(!s.manualCookieGuard)await command('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES, maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 });
      if(controls)await controls.apply();else await command('Network.setCacheDisabled', { cacheDisabled: true });
      capture = new BootstrapHandoffCapture({ boundIdentity: true, send: command,
        beforeIdentityRead: contextId => beforeIdentityRead?.(binder.startupPins(contextId)), beforeRuntimeDisable: async () => {
        progress('settings_wait', binder.settingsWaitState());
        let timer;
        try {
          if (waitForSettings) {
            const result = await waitForSettings(binder.done);
            if (result?.status !== 'complete') throw startupFailure('collector','settings','source_unavailable');
          }
          else if (current.manualStartupSelected) throw probeError('manual_startup_budget_invalid');
          else await Promise.race([binder.done, new Promise(resolve => { timer = setTimeout(resolve, MANUAL_TIMING.settingsFallbackMS); })]);
        }
        catch(error) { throw initialFailure ? initialFailure(error, 'settings') : error; }
        finally { clearTimeout(timer); sampling = false; }
      } });
      const early = await capture.run();
      const rejected = capture.diagnostics().handoffRejected;
      binder.earlyOutcome(early.stopReason,rejected);
      const continuousObservation=s.manualCookieGuard?await binder.finish(early.cleanupVerified&&!rejected):null;
      await command('Network.disable');
      networkDisabled = true;
      const observation = continuousObservation ?? await binder.finish(early.cleanupVerified && !rejected);
      return { checkpoint: shortQACheckpoint(observation), ...privateResult };
    } catch(error) { checkpointFailure=error;throw error;
    } finally {
      detachSettings();
      sampling = false; unsubscribe();s.manualCookieUnsubscribe?.();
      if (!networkDisabled) await command('Network.disable').catch(() => {});
      if(controls){progress('root_restore');try{await controls.restore();s.manualControlsCleanup='verified';}catch(error){s.manualControlsCleanup='uncertain';initialFailure?.(error,'cleanup');if(!checkpointFailure)throw error;}}
    }
  }

  // Owner actor factory selected only by the guarded private QA role protocol.
  // No main provider selects it. The trusted bridge binds actual startup pins and
  // invokes both captures together. This hook never seeds cookies or adopts a
  // different document/context to make a requested binding pass.
  function manualReloadFrameProjection(value,frameId,oldLoader){
    if(!isObject(value)||Object.keys(value).sort().join(',')!=='frameId,loaderId'||!safeId(frameId)||!safeId(oldLoader))throw probeError('manual_startup_unavailable');
    const fields=Object.getOwnPropertyDescriptors(value);
    if(fields.frameId?.value!==frameId||fields.loaderId?.value!==oldLoader)throw probeError('manual_startup_unavailable');
    // EarlyBootstrapCapture deliberately accepts exactly one projected key.
    return {frameId};
  }
  function createOwnedManualStartupOwners(message,diagnostics=startupFailureLatch()) {
    if (!manualStartupAllowed()) throw probeError('manual_startup_disabled');
    const current = requireOwner(message), collector = current.shortFeasibility;
    if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !manualStartupStateReady(current)) throw probeError('manual_startup_unavailable');
    const rendezvous = new QACheckpointRendezvous(MANUAL_TIMING.rendezvousMS);
    const owner = role => createManualStartupOwner({role,diagnostics,requireFreshness:true,run: async sink => {
      let released=false;
      const beforeIdentity=async pins=>{await sink.beforeIdentityRead(pins);released=true;};
      const progress=(op,state)=>{
        const timing=current.manualTiming?.[role];
        if(op===undefined)timing?.settings(state);else timing?.step(op,state);
      };
      if (sink.signal?.aborted) throw probeError('manual_startup_unavailable');
      const abort = () => rendezvous.cancel();
      sink.signal?.addEventListener('abort', abort, { once: true });
      const failure=(error,phase,reason='unavailable',state)=>{
        const detail=startupDiagnostic(error);
        if(sink.observationDeadline)error=startupFailure(detail?.role??role,detail?.phase??phase,detail?.reason??reason,detail?.peer??false,undefined,{...detail?.trace,...sink.observationDeadline.snapshot()});
        const cause=diagnostics.record(phase==='prepare'?startupPrepareFailure(error,role,'driver_setup','command_failed'):error,role,phase,reason);
        current.manualTiming?.[role]?.failure(cause,state);
        return cause;
      };
      const manualObservation=(...args)=>createManualV2BoundObservation(...args,(reason,trace,state)=>{
        const phase=reason==='source_unavailable'?'settings':'bound_finish';
        failure(startupFailure(role,phase,reason,false,undefined,trace),phase,reason,state);
      },role==='reader'?current.manualCookieGuard:collector.manualCookieGuard,
      {role,cacheControl:(role==='reader'?current:collector).manualFreshnessCacheControl,observationDeadline:sink.observationDeadline});
      const together=async()=>{progress('rendezvous');try{await rendezvous.arrive(role);}catch{throw diagnostics.record(startupFailure('pair','rendezvous','deadline'),'pair','rendezvous');}};
      try {
        let observation;
        if (role === 'reader') {
          current.gate.beginObservation(); current.earlyAttempted = true;
          observation=await boundCheckpoint(current, false, { deferCookieMutation: false, onPrivateResult: () => {},
            onPrivateStartup: sink.onPrivateStartup },together,beforeIdentity,failure,manualObservation,progress,sink.waitForSettings);
        }else{
          const result=await collectorShortCheckpoint(current,together,sink.onPrivateStartup,beforeIdentity,failure,manualObservation,progress,sink.waitForSettings);
          observation=result?.checkpoint?.observation;
        }
        requireBoundStartup(observation,role,diagnostics);
      }catch(error){const cause=failure(error,released?'identity':'prepare');current.manualTiming?.[role]?.failure(cause);rendezvous.cancel();throw cause;
      } finally { sink.signal?.removeEventListener('abort', abort); }
    } });
    return Object.freeze({ reader: owner('reader'), collector: owner('collector') });
  }

  function manualStartupStateReady(current){
    const collector=current?.shortFeasibility;
    return !!(current?.gate?.loginVisible&&!current.gate.observationStarted&&!current.earlyAttempted&&!current.qualificationAttempted&&collector&&!collector.closed&&collector.contextId&&collector.sessionId&&current.manualSeeded);
  }

  async function manualReaderInventory(current,readerSend=send) {
    const {targetIds}=await readerSend('Target.getTargets',{},undefined,'manual_owner');
    const windows=new Set();let bootstrapWindow;
    for(const targetId of targetIds){
      const {windowId}=await readerSend('Browser.getWindowForTarget',{targetId},undefined,'manual_owner');
      windows.add(windowId);if(targetId===current.targetId)bootstrapWindow=windowId;
    }
    return {windowCount:windows.size,bootstrapCount:bootstrapWindow&&windows.has(bootstrapWindow)?1:0};
  }

  // The same snapshot path is exercised accountlessly with injected transports.
  // Wrapping command failures never publishes transport messages or source pins.
  async function manualOwnerSnapshot({current,collector,role,document,pins,phase='prepare',readerSend=send,readerAlive=()=>!childExited&&!cdpFailure}){
    const step=async(stage,run)=>{try{return await run();}catch(error){throw startupInspectionFailure(error,role,phase,stage,'command_failed');}};
    const reader=role==='reader',owned=reader?{pid:current.pid,profileId:current.profileId,alive:readerAlive()}:collector.transport.ownership();
    const targetId=reader?current.targetId:collector.targetId,state=reader?current:collector;
    if(document&&!state.manualObserver){
      const attached=await step('snapshot_attach',()=>reader?readerSend('Target.attachToTarget',{targetId,flatten:true},undefined,'manual_owner'):
        collector.transport.send('Target.attachToTarget',{targetId,flatten:true},undefined,r=>{
          if(!safeId(r.sessionId))throw probeError('manual_owner_unavailable');return{sessionId:r.sessionId};}));
      state.manualObserver={sessionId:attached.sessionId,context:null,frameId:pins.frameId};
      for(const method of ['Page.enable','Runtime.enable'])await step(method==='Page.enable'?'snapshot_page':'snapshot_runtime',()=>
        reader?readerSend(method,{},attached.sessionId,'manual_owner'):collector.transport.send(method,{},attached.sessionId,()=>({acknowledged:true})));
    }
    const observer=state.manualObserver;
    let frame;
    if(document)frame=await step('snapshot_frame',()=>reader?readerSend('Page.getFrameTree',{},current.sessionId,'manual_owner'):
      collector.transport.send('Page.getFrameTree',{},collector.sessionId,r=>{
        const value=r.frameTree?.frame;
        if(!value||!isHomeTarget(value.url)||!safeId(value.id)||!safeId(value.loaderId))throw probeError('manual_owner_unavailable');
        return{frameId:value.id,loaderId:value.loaderId};}));
    const inventory=reader?await step('snapshot_inventory',()=>manualReaderInventory(current,readerSend)):{windowCount:0,bootstrapCount:0};
    return{ownerId:String(owned.pid),profileId:owned.profileId,targetId,sessionId:observer?.sessionId??(reader?current.sessionId:collector.sessionId),
      alive:owned.alive,headless:!reader,...inventory,frameId:frame?.frameId,documentId:frame?.loaderId,contextId:String(observer?.context??''),
      lifecycleReady:!!observer&&Number.isSafeInteger(observer.context)&&observer.context>0};
  }

  // Fixed private owner RPCs are intentionally concurrent: the two actors meet at
  // the existing post-identity rendezvous. Serializing ACQUIRE would deadlock it.
  async function retireManualReader(current,preserveReader,close=closeChrome){
    if(borrowedMainTransport){if(mustRetireStartupOwner(current))current.manualControlsCleanup='uncertain';return;}
    if(!preserveReader||mustRetireStartupOwner(current)){const closed=await close();if(!closed.exited)throw probeError('chrome_exit_unverified');}
  }
  async function executeManualOwnerRPC(message) {
    if(!manualStartupAllowed()&&!(applicationScope&&['manual_abort','manual_capture_cancel'].includes(message?.op)))throw probeError('manual_startup_disabled');
    manualOwnerOperation(message);
    const current=requireOwner(message), collector=current.shortFeasibility;
    if(!current.manualStartupPort){
      current.manualStartupSelected=true;
      try{
        const diagnostics=current.manualCookieDiagnostics??startupFailureLatch();
        current.manualStartupPort=createXManualOwnerPort({enabled:true,owner:current.owner,diagnostics,
          createOwners:()=>createOwnedManualStartupOwners(message,diagnostics),
          snapshot:(role,document,pins,phase)=>manualOwnerSnapshot({current,collector,role,document,pins,phase}),
          capture:(request,options)=>collector.transport.captureWorker(request,{...options,browserContextId:collector.contextId}),
          retire:async(role,preserveReader)=>{
            if(role==='reader'){
              await retireManualReader(current,preserveReader);
              // A preserved reader keeps its observation session until its owned
              // tree exits; no unobserved restart or admission is permitted.
            }else{
              if(!await closeShortCollector(current))throw probeError('chrome_exit_unverified');
              current.manualCollectorUnsubscribe?.();
            }
          },
        });
        current.manualCollectorUnsubscribe=collector.transport.subscribe(raw=>{
          const observer=collector.manualObserver;
          if(observer&&raw.sessionId===observer.sessionId&&raw.method==='Runtime.executionContextCreated'){
            const context=raw.params?.context;
            if(context?.auxData?.isDefault&&context.auxData.frameId===observer.frameId&&context.origin===X_ORIGIN)observer.context=context.id;
          }
          current.manualStartupPort.observe('collector',raw);
        });
      }catch{current.manualStartupSelected=false;throw probeError('manual_owner_unavailable');}
    }
    const measured=['manual_acquire','manual_inspect','manual_inventory'].includes(message.op);
    const timing=measured?createStartupTiming(result=>emit({event:'manual_startup_progress',id:message.id,result})):null;
    current.manualTiming??={};
    if(timing){current.manualTiming[message.role]=timing;timing.step(message.op==='manual_inspect'?'inspect_rpc':message.op==='manual_inventory'?'inventory_rpc':'acquire_rpc');}
    let result;
    try{result=await current.manualStartupPort.execute(message);timing?.step('complete');}
    catch(error){timing?.failure(error);throw error;}
    finally{timing?.finish();if(current.manualTiming[message.role]===timing)delete current.manualTiming[message.role];}
    if(message.op==='manual_acquire'){
      current.manualAcquired??=new Set();current.manualAcquired.add(message.role);
    }
    return result;
  }

  // Begin passive request provenance before enrollment reload, and keep it through
  // seed acknowledgment and startup. No cookie read or write is added here.
  async function startManualCookieObservation(current) {
    const s=current.shortFeasibility;
    if(current.manualCookieGuard||s.manualCookieGuard)throw probeError('manual_setup_unavailable');
    const diagnostics=current.manualCookieDiagnostics=startupFailureLatch();
    const make=(frameId,sessionId,role)=>new ManualStartupObservation(frameId,sessionId,{beforeSeed:true,onRejection:(reason,trace,failureReason)=>{
      diagnostics.record(startupFailure(role,'bound_finish',failureReason,false,undefined,{firstCause:reason,...trace}),role,'bound_finish',failureReason);
    }});
    const reader=await send('Page.getFrameTree',{},current.sessionId,'manual_provenance');
    current.manualCookieGuard=make(reader.frameId,current.sessionId,'reader');
    const collector=await s.transport.send('Page.getFrameTree',{},s.sessionId,r=>{
      if(!safeId(r.frameTree?.frame?.id))throw probeError('manual_setup_unavailable');
      return{frameId:r.frameTree.frame.id};
    });
    s.manualCookieGuard=make(collector.frameId,s.sessionId,'collector');
    s.manualCookieUnsubscribe=s.transport.subscribe(raw=>s.manualCookieGuard.observe(raw));
    // Prepare the existing uncached envelope before enrollment traffic. This
    // neither bypasses service workers nor repairs an earlier provenance loss.
    current.manualFreshnessCacheControl={state:'requested',requestedAt:performance.now(),acknowledgedAt:'unknown'};
    markStartupControlAttempt(current);
    const readerCache=await send('Network.setCacheDisabled',{cacheDisabled:true},current.sessionId,'manual_provenance');
    if(readerCache?.acknowledged!==true)throw probeError('manual_setup_unavailable');
    current.manualFreshnessCacheControl=Object.freeze({...current.manualFreshnessCacheControl,state:'acknowledged',acknowledgedAt:performance.now()});
    s.manualFreshnessCacheControl={state:'requested',requestedAt:performance.now(),acknowledgedAt:'unknown'};
    markStartupControlAttempt(s);
    await s.transport.send('Network.setCacheDisabled',{cacheDisabled:true},s.sessionId,r=>{
      if(!isObject(r)||Object.keys(r).length!==0)throw probeError('manual_setup_unavailable');
      return{acknowledged:true};
    });
    s.manualFreshnessCacheControl=Object.freeze({...s.manualFreshnessCacheControl,state:'acknowledged',acknowledgedAt:performance.now()});
    current.manualCacheControlReady=true;
    const params={maxTotalBufferSize:MAX_TOTAL_NETWORK_BUFFER_BYTES,maxResourceBufferSize:MAX_RESOURCE_NETWORK_BUFFER_BYTES,maxPostDataSize:0};
    await send('Network.enable',params,current.sessionId,'manual_provenance');
    await s.transport.send('Network.enable',params,s.sessionId,()=>({acknowledged:true}));
  }

  async function seedRetainedManualPilot(message){
    exactKeys(message,['id','op','owner'],'manual_setup_unavailable');
    if(message.op!=='manual_seed_bootstrap'||!manualStartupAllowed()||!retainedTransferAllowed()||!shortTransferAllowed())throw manualSeedFailure('preconditions','disabled');
    const current=requireOwner(message),s=current.shortFeasibility;
    if(!qualificationMode(driverMode)||!retainedQAScope?.reuse||!current.gate.loginVisible||current.gate.observationStarted||
      !s||s.closed||s.ran||s.exportIssued||current.manualStartupPort||current.manualSetup)throw manualSeedFailure('preconditions',current.manualSetup?'replay':'rejected',current.manualSetup?'unknown':'not_attempted');
    const assertGuards=()=>{
      applicationScope?.seedGrant.assertCurrent();
      for(const [role,guard]of [['reader',current.manualCookieGuard],['collector',s.manualCookieGuard]]){
        if(!guard)throw manualSeedFailure('preconditions','rejected');
        const reason=guard.rejection();
        if(reason!=='unknown')throw current.manualCookieDiagnostics.record(
          startupFailure(role,'bound_finish',guard.failureReason(),false,undefined,{firstCause:reason,...guard.trace()}),role,'bound_finish',guard.failureReason());
      }
    };
    current.manualSetup=createRetainedManualSetup({enabled:true,transferAllowed:true,
      ownership:async()=>{
        if(current.manualEnrollment){await current.manualEnrollment.check();assertGuards();}
        const owned=s.transport.ownership();if(childExited||cdpFailure||!owned.alive)throw probeError('manual_setup_unavailable');
        return {readerPID:current.pid,readerProfile:current.profileId,collectorPID:owned.pid,collectorProfile:owned.profileId};
      },
      readEnrollment:async()=>{
        await startManualCookieObservation(current);
        assertGuards();
        current.manualEnrollment=new ManualEnrollmentCapture({
          scope:{pid:current.pid,profileId:current.profileId,targetId:current.targetId,sessionId:current.sessionId},
          send:(method,params,sessionId)=>send(method,params,sessionId,'manual_enrollment'),
          snapshot:async()=>({pid:current.pid,profileId:current.profileId,targetId:current.targetId,sessionId:current.sessionId,alive:!childExited&&!cdpFailure}),
          projectEvent:projectCDPEvent,
        });
        const account=await current.manualEnrollment.run();assertGuards();
        // Daily composition checks the process account before cookie access.
        if(applicationScope?.expectedAccount!==undefined&&applicationScope.expectedAccount!==account)throw manualSeedFailure('enrollment_project','rejected');
        return account;
      },
      readAllowedCookies:async()=>{
        assertGuards();await current.manualEnrollment.check();assertGuards();
        s.exporting=true;
        try{const selected=await send('Network.getCookies',qaCookieRequest('api_v1'),current.sessionId);assertGuards();await current.manualEnrollment.check();assertGuards();return selected;}
        finally{s.exporting=false;}
      },
      beforeWrite:async()=>{assertGuards();await current.manualEnrollment.check();assertGuards();},
      assertBeforeWrite:()=>{current.manualEnrollment.assertCurrent();assertGuards();},
      writeAllowedCookies:async(cookies,acknowledgeWrite)=>{
        try{await s.transport.send('Network.setCookies',{cookies},s.sessionId,()=>{
          // Receipt of the CDP success is distinct from post-ack guard health.
          acknowledgeWrite();
          // Set the baseline inside acknowledgment dispatch, before another event
          // in the same pipe batch or enrollment cleanup can be processed.
          current.manualCookieGuard.beginPostSeed();s.manualCookieGuard.beginPostSeed();
          assertGuards();
          return{acknowledged:true};
        });}catch(error){assertGuards();throw error;}
        assertGuards();return true;
      },
      cleanupEnrollment:async()=>{if(current.manualEnrollment){await current.manualEnrollment.close();assertGuards();}},
    });
    // homeReady retains the earlier fixed UI check's meaning; the enrollment
    // reload does not assert a new UI check or grant identity/login authority.
    const result=await current.manualSetup();
    try{assertGuards();}catch(error){throw manualSeedFailure('enrollment_cleanup','rejected','acknowledged',error);}
    current.manualSeeded=true;return result;
  }

  async function openRetainedManualReader(message){
    exactKeys(message,['id','op','owner'],'manual_owner_unavailable');
    const current=requireOwner(message);
    if(!manualStartupAllowed()||!current.manualStartupPort||current.manualAcquired?.size!==2||current.manualReaderOpened)throw probeError('manual_owner_unavailable');
    current.manualReaderOpened=true;
    await current.manualStartupPort.execute({id:message.id,op:'manual_inspect',owner:message.owner,role:'reader'});
    await send('Target.createTarget',{url:HOME_URL,newWindow:true,background:true},undefined,'manual_owner');
    const inventory=await manualReaderInventory(current);
    if(inventory.windowCount<2||inventory.bootstrapCount!==1)throw probeError('manual_owner_unavailable');
    return {opened:true,...inventory};
  }

  // Trusted application composition selects fixed paths once. The application
  // pipe never exposes the QA execute dispatcher, URL selection or CDP methods.
  // Application seed has its own private one-use grant. Legacy QA gates retain
  // their existing environment requirements and cannot enable application seed.
  async function createApplicationManualOwner({chromePath,readerProfile,collectorProfile,loadAssets,emitProgress=()=>{}}){
    if(applicationScope||child||protocolOwner||driverMode!=='single_phase'||typeof loadAssets!=='function'||typeof emitProgress!=='function')throw probeError('application_owner_unavailable');
    for(const path of [chromePath,readerProfile,collectorProfile])if(typeof path!=='string'||!isAbsolute(path)||resolve(path)!==path)throw probeError('application_owner_path_invalid');
    const chromeInfo=await lstat(chromePath);
    if(!chromeInfo.isFile()||chromeInfo.isSymbolicLink()||!sameWindowsPath(await realpath(chromePath),chromePath))throw probeError('application_owner_path_invalid');
    for(const [left,right]of [[readerProfile,collectorProfile],[collectorProfile,readerProfile]]){
      const rel=relative(left,right);
      if(!rel||!isAbsolute(rel)&&rel!=='..'&&!rel.startsWith('..'+sep))throw probeError('application_owner_profiles_overlap');
    }
    const readerIdentity=await ordinaryDirectoryIdentity(readerProfile),collectorIdentity=await ordinaryDirectoryIdentity(collectorProfile);
    const root=dirname(readerProfile),rootIdentity=await ordinaryDirectoryIdentity(root);
    executable=chromePath;profilePath=readerProfile;qaRoot=root;
    driverMode='retained_source_qualification';retainedQAScope={reuse:true};
    twoPhaseState={root,profile:readerProfile,rootIdentity,profileIdentity:readerIdentity,phase2Started:true};
    applicationScope={collectorProfile,loadAssets,emitProgress,seedGrant:createApplicationSeedGrant({runSeed:seedRetainedManualPilot})};
    let used=false,closed=false;
    const check=async()=>{
      if(closed||!await verifyTwoPhaseDirectories()||!sameDirectoryIdentity(await ordinaryDirectoryIdentity(collectorProfile),collectorIdentity))throw probeError('application_owner_scope_changed');
    };
    return Object.freeze({
      async initialize(){
        if(used)throw probeError('application_owner_replay');used=true;
        await check();
        const spawned=spawn(executable,twoPhaseChromeArgs(readerProfile,true),{windowsHide:false,stdio:['ignore','ignore','ignore','pipe','pipe']});
        child=spawned;childExited=false;bindChrome(spawned);
        if(!Number.isSafeInteger(spawned.pid)||spawned.pid<1)throw probeError('application_owner_launch_failed');
        const launched=await initializeChromeSession(spawned,readerProfile);
        const deadline=Date.now()+15000;
        while(!(await loginState({owner:launched.owner})).ready){
          await check();if(Date.now()>=deadline)throw probeError('application_owner_home_unavailable');
          await new Promise(accept=>setTimeout(accept,250));
        }
        const collector=await prepareShortConcurrency({owner:launched.owner});
        await check();
        return{owner:launched.owner,readerPID:launched.pid,collectorPID:collector.collectorPID};
      },
      async seed(message){
        exactKeys(message,['id','op','owner'],'manual_setup_unavailable');
        if(message.op!=='native_owner_seed')throw manualSeedFailure('preconditions','rejected');
        requireOwner(message);await check();
        return applicationScope.seedGrant.run({id:message.id,op:'manual_seed_bootstrap',owner:message.owner});
      },
      async execute(message){
        // Reserve the request in the post actor synchronously. A concurrent
        // cancel must be able to retire it while profile/CDP reads are pending.
        if(['manual_prepare_post','manual_complete_post','manual_cancel_post'].includes(message.op)){
          applicationPostOperation(message);requireOwner(message);
          if(active.manualAcquired?.size!==2)throw probeError('application_post_unavailable');
          applicationScope.readerPost??=createApplicationReaderPost({owner:active.owner,bootstrapTarget:active.targetId,
            command:async(method,params,session,beforeDispatch=()=>{})=>{await check();if(method!=='Target.detachFromTarget')applicationScope.seedGrant.assertCurrent();beforeDispatch();return sendApplicationPost(method,params,session);},
            assertCurrent:()=>applicationScope.seedGrant.assertCurrent()});
          return applicationScope.readerPost.execute(message);
        }
        await check();
        const cleanup=['manual_abort','manual_capture_cancel'].includes(message.op);
        if(!cleanup)applicationScope.seedGrant.assertCurrent();
        if(message.op==='manual_capture')applicationScope.seedGrant.assertCaptureDeadline(message.request?.deadlineMS);
        let result;
        try{
          if(message.op==='manual_open_reader')result=await openRetainedManualReader(message);
          else{manualOwnerOperation(message);result=await executeManualOwnerRPC(message);}
          if(!cleanup)applicationScope.seedGrant.assertCurrent();
          return result;
        }catch(error){if(Buffer.isBuffer(result))result.fill(0);throw error;}
      },
      async close(){
        if(closed)return;closed=true;
        try{await applicationScope.readerPost?.close();if(active?.shortFeasibility)await closeShortCollector(active);}
        finally{await closeChrome();}
      },
    });
  }

  // Daily composition has a persistent, exclusively owned reader and a new
  // actor/session/grant for every disposable collector. No consumed pilot grant
  // is reset, and the manual factory above retains its one-use semantics.
  async function createApplicationDailyOwner({chromePath,readerProfile,loadAssets,emitProgress=()=>{},borrowedReader=null,collectorFactory=createQAFeasibilityCollector}){
    if(applicationScope||child||protocolOwner||driverMode!=='single_phase'||typeof loadAssets!=='function')throw probeError('daily_owner_unavailable');
    for(const path of [chromePath,readerProfile])if(typeof path!=='string'||!isAbsolute(path)||resolve(path)!==path)throw probeError('daily_owner_path_invalid');
    const info=await lstat(chromePath);
    if(!info.isFile()||info.isSymbolicLink()||!sameWindowsPath(await realpath(chromePath),chromePath))throw probeError('daily_owner_path_invalid');
    const readerIdentity=await ordinaryDirectoryIdentity(readerProfile),root=dirname(readerProfile),rootIdentity=await ordinaryDirectoryIdentity(root);
    executable=chromePath;profilePath=readerProfile;qaRoot=root;driverMode='retained_source_qualification';retainedQAScope={reuse:true};
    twoPhaseState={root,profile:readerProfile,rootIdentity,profileIdentity:readerIdentity,phase2Started:true};
    applicationScope={loadAssets,emitProgress};
    let initialized=false,closed=false,failed=false,base,batch,generation=0,account;
    let transient='',retryCount=0,captureSucceeded=false;
    const posts=new Map();
    let borrowedTarget=null,unsubscribeBorrowed=null;
    let batchBootstrap;
    let lastBatchCleanup;
    if(borrowedReader){
      if(!Number.isSafeInteger(borrowedReader.pid)||borrowedReader.pid<1||typeof borrowedReader.profileDirectory!=='string'||!borrowedReader.profileDirectory||/[\\/\u0000-\u001f]/u.test(borrowedReader.profileDirectory)||
        typeof borrowedReader.transport?.call!=='function'||typeof borrowedReader.transport?.subscribe!=='function'||typeof borrowedReader.transport?.alive!=='function')throw probeError('borrowed_reader_rejected');
      borrowedMainTransport=borrowedReader.transport;
      batchBootstrap=createBatchBootstrap({call:(method,params,timeoutMS=10000)=>borrowedMainTransport.call(method,params,'',timeoutMS),
        reserveTarget:id=>borrowedMainTransport.reserveTarget?.(id),releaseTarget:id=>borrowedMainTransport.releaseTarget?.(id)});
      unsubscribeBorrowed=borrowedMainTransport.subscribe(raw=>{try{dispatchCDP(raw);}catch{failCDP('borrowed_reader_event_failed');}});
    }
    const check=async()=>{if(closed||childExited||cdpFailure||borrowedReader&&!borrowedMainTransport.alive()||!await verifyTwoPhaseDirectories())throw probeError('daily_reader_changed');return{readerPID:child.pid,readerProfile};};
    const home=async()=>{const end=Date.now()+MANUAL_TIMING.collectorHomeReadyMS;while(!(await loginState({owner:active.owner})).ready){await check();if(Date.now()>=end)throw probeError('daily_reader_login_unavailable');await new Promise(r=>setTimeout(r,200));}};
    const endBatch=async()=>{
      applicationScope?.seedGrant?.cancel();
      const current=batch;
      if(!current&&lastBatchCleanup)return lastBatchCleanup;
      const controlState=owner=>owner?.manualControlsCleanup??(owner&&mustRetireStartupOwner(owner)?'uncertain':'not_needed');
      const result=await cleanupDailyBatch({readerControls:controlState(current),collectorControls:controlState(current?.shortFeasibility),
        collector:current?async()=>{if(!await closeShortCollector(current))throw probeError('daily_collector_cleanup_uncertain');}:undefined,
        enrollment:current?async()=>{let first;for(const release of [()=>current.manualCollectorUnsubscribe?.(),()=>current.shortFeasibility?.manualCookieUnsubscribe?.(),()=>current.manualEnrollment?.close()])try{await release();}catch(error){first??=error;}if(first)throw first;}:undefined,
        attachments:current?async()=>{let first;for(const sessionId of new Set([current.manualObserver?.sessionId,current.sessionId].filter(Boolean)))try{await sendApplicationPost('Target.detachFromTarget',{sessionId});borrowedMainTransport?.releaseSession?.(sessionId);}catch(error){first??=error;}if(first)throw first;}:undefined,
        bootstrap:batchBootstrap?async()=>{await batchBootstrap.close();borrowedTarget=null;}:undefined});
      if(!result.closed){failed=true;transient='';}
      else if(failed&&transient&&retryCount<SOURCE_RETRY_LIMIT){failed=false;retryCount++;}
      else if(!failed&&captureSucceeded)retryCount=0;
      lastBatchCleanup=result;
      batch=null;active=base;protocolOwner=base.owner;applicationScope={loadAssets,emitProgress};
      return result;
    };
    return Object.freeze({
      async initialize(){
        if(initialized||closed)throw probeError('daily_reader_replay');initialized=true;
        if(borrowedReader){
          child={pid:borrowedReader.pid};childExited=false;
          base={owner:++nextOwner,pid:child.pid,profileId:readerProfile,gate:new LoginObservationGate(),metadata:null};active=base;protocolOwner=base.owner;
          await check();
          return{owner:base.owner,readerPID:child.pid};
        }else{
          child=spawn(executable,twoPhaseChromeArgs(readerProfile,true),{windowsHide:false,stdio:['ignore','ignore','ignore','pipe','pipe']});childExited=false;bindChrome(child);
        }
        if(!Number.isSafeInteger(child.pid)||child.pid<1)throw probeError('daily_reader_launch_failed');
        const launched=await initializeChromeSession(child,readerProfile,borrowedTarget);await home();base=active;
        return{owner:base.owner,readerPID:launched.pid};
      },
      async check(){if(!initialized)throw probeError('daily_reader_unavailable');return check();},
      async begin(message){
        if(!initialized||failed||batch||message.generation!==generation+1)throw probeError('daily_batch_unavailable');
        generation=message.generation;transient='';captureSucceeded=false;await check();
        lastBatchCleanup=null;
        const path=message.collectorProfile;
        if(typeof path!=='string'||!isAbsolute(path)||resolve(path)!==path)throw probeError('daily_collector_path_invalid');
        for(const [left,right]of [[readerProfile,path],[path,readerProfile]]){const rel=relative(left,right);if(!rel||!isAbsolute(rel)&&rel!=='..'&&!rel.startsWith('..'+sep))throw probeError('daily_profiles_overlap');}
        const collectorIdentity=await ordinaryDirectoryIdentity(path);
        if(batchBootstrap){
          try{
            borrowedTarget=await batchBootstrap.begin();
            await initializeChromeSession(child,readerProfile,borrowedTarget);
            batch=active;
          }catch(error){failed=true;transient='';throw error;}
        }
        const attached=batchBootstrap?{sessionId:batch.sessionId}:await sendApplicationPost('Target.attachToTarget',{targetId:base.targetId,flatten:true});
        if(!safeId(attached.sessionId))throw probeError('daily_reader_attachment_failed');
        // Fresh object and attachment: old actor capabilities remain consumed.
        if(!batchBootstrap)batch=active={owner:++nextOwner,pid:base.pid,profileId:base.profileId,targetId:base.targetId,sessionId:attached.sessionId,
          gate:new LoginObservationGate(),pageLoad:null,metadata:null,observationEndsAt:0,networkEventCount:0,eventLimitReached:false};protocolOwner=batch.owner;
        registerFreshStartupSession(batch,batch.sessionId,'reader');await send('Page.enable',{},batch.sessionId);await home();
        applicationScope={collectorProfile:path,loadAssets,emitProgress,collectorFactory,expectedAccount:account,seedGrant:createApplicationSeedGrant({runSeed:seedRetainedManualPilot})};
        try{const collector=await prepareShortConcurrency({owner:batch.owner});if(!sameDirectoryIdentity(await ordinaryDirectoryIdentity(path),collectorIdentity))throw probeError('daily_collector_changed');return{owner:batch.owner,readerPID:base.pid,collectorPID:collector.collectorPID};}
        catch(error){failed=true;transient='';throw error;}
      },
      async seed(message){
        if(!batch||failed||message.generation!==generation||message.owner!==batch.owner)throw probeError('daily_seed_unavailable');
        try{const receipt=await applicationScope.seedGrant.run({id:message.id,op:'manual_seed_bootstrap',owner:batch.owner});
          if(account!==undefined&&account!==receipt.accountId)throw probeError('daily_account_changed');account=receipt.accountId;return receipt;
        }catch(error){failed=true;transient='';throw error;}
      },
      async execute(message){
        if(!batch)throw probeError('daily_batch_unavailable');await check();
        const cleanup=['manual_abort','manual_capture_cancel'].includes(message.op);
        if(cleanup)applicationScope.seedGrant?.cancel();
        if(!cleanup){if(failed)throw probeError('daily_batch_failed');applicationScope.seedGrant.assertCurrent();}
        if(message.op==='manual_capture')applicationScope.seedGrant.assertCaptureDeadline(message.request?.deadlineMS);
        try{const result=await executeManualOwnerRPC(message);if(message.op==='manual_capture')captureSucceeded=true;return result;}
        catch(error){if(!cleanup){transient=failed?'':sourceTransientClass(error);failed=true;}throw error;}
      },
      async end(message){if(message.generation!==generation)throw probeError('daily_batch_stale');return endBatch();},
      async post(message){
        await check();applicationPostOperation(message,{allowMainOrigin:true});if(message.owner!==base.owner)throw probeError('daily_reader_scope');
        const key=message.request?.requestId??message.requestId;let record=posts.get(key);
        if(message.op==='manual_prepare_post'){
          if(record||posts.size>=32)throw probeError('daily_reader_busy');
          const actor=createApplicationReaderPost({owner:base.owner,bootstrapTarget:base.targetId,allowMainOrigin:true,processScopedOwner:Boolean(borrowedReader),
            command:async(method,params,session,before=()=>{})=>{await check();before();return sendApplicationPost(method,params,session);},
            assertCurrent:()=>{if(closed||childExited||cdpFailure)throw probeError('daily_reader_changed');}});
          record={actor,timer:setTimeout(()=>{posts.delete(key);void actor.close().catch(()=>{failed=true;transient='';});},35000)};posts.set(key,record);
        }
        if(!record)throw probeError('daily_reader_request_missing');
        try{return await record.actor.execute(message);}finally{if(message.op!=='manual_prepare_post'){clearTimeout(record.timer);posts.delete(key);await record.actor.close();}}
      },
      async close(){
        if(closed)return;closed=true;
        applicationScope?.seedGrant?.cancel();
        for(const p of posts.values()){clearTimeout(p.timer);await p.actor.close().catch(()=>{});}posts.clear();
        if(!borrowedReader){try{if(batch)await closeShortCollector(batch);}finally{await closeChrome();}return;}
        // Borrower cleanup may retire only its collector and explicit bootstrap
        // target. Main Chrome/profile/Job lifetime remains with the Go owner.
        let cleanupError=lastBatchCleanup?.closed===false?probeError('daily_batch_cleanup_uncertain'):undefined;
        try{if(batch&&!(await endBatch()).closed)throw probeError('daily_batch_cleanup_uncertain');}catch(error){cleanupError=error;}
        try{
          if(base?.sessionId){await borrowedMainTransport.call('Target.detachFromTarget',{sessionId:base.sessionId});borrowedMainTransport.releaseSession?.(base.sessionId);}
          await batchBootstrap?.close();borrowedTarget=null;
        }catch(error){cleanupError??=error;}
        unsubscribeBorrowed?.();
        if(cleanupError)throw cleanupError;
      },
    });
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


  // Trusted fixture composition receives the same guarded operations and live
  // state. Production entry points expose only the two owner factories.
  return Object.freeze({
    createApplicationManualOwner, createApplicationDailyOwner,
    fixture:Object.freeze({
      verifyTwoPhaseDirectories, emit, emitReply, safeCode, isEarlyBootstrapCallFrameRead,
      send, rejectPending, bindChrome, withTimeout, deferred,
      initializeChromeSession, requireOwner, loginState, sanitizeEarlyHandoffDiagnostics, boundCheckpoint,
      prepareShortConcurrency, closeShortCollector, collectorShortCheckpoint, manualReloadFrameProjection, createOwnedManualStartupOwners,
      manualStartupStateReady, manualOwnerSnapshot, executeManualOwnerRPC, seedRetainedManualPilot, openRetainedManualReader,
      closeChrome,
      state:Object.freeze({
        get active(){return active;}, set active(value){active=value;},
        get cdpFailure(){return cdpFailure;},
        get cdpFrames(){return cdpFrames;},
        get child(){return child;}, set child(value){child=value;},
        get childExited(){return childExited;}, set childExited(value){childExited=value;},
        get controlOrigin(){return controlOrigin;}, set controlOrigin(value){controlOrigin=value;},
        get driverMode(){return driverMode;}, set driverMode(value){driverMode=value;},
        get executable(){return executable;}, set executable(value){executable=value;},
        get nextOwner(){return nextOwner;}, set nextOwner(value){nextOwner=value;},
        get profilePath(){return profilePath;}, set profilePath(value){profilePath=value;},
        get protocolOwner(){return protocolOwner;}, set protocolOwner(value){protocolOwner=value;},
        get qaRoot(){return qaRoot;}, set qaRoot(value){qaRoot=value;},
        get retainedQAScope(){return retainedQAScope;}, set retainedQAScope(value){retainedQAScope=value;},
        get twoPhaseState(){return twoPhaseState;}, set twoPhaseState(value){twoPhaseState=value;},
      }),
    }),
  });
}

// Fixed definitions shared with the offline fixture adapter. No QA dispatcher is installed here.
export const nativeProbeDefinitions=Object.freeze({
  qualificationMode, bodyMode, retainedQAMode, HOME_URL, MAX_CONTROL_LINE_BYTES,
  MAX_CONTROL_BUFFER_BYTES, MAX_CONTROL_LINES_PER_CHUNK, MAX_CONTROL_LINES_TOTAL, MAX_CANDIDATES, MAX_ENCODED_BYTES,
  OBSERVATION_MS, MAX_TOTAL_NETWORK_BUFFER_BYTES, MAX_RESOURCE_NETWORK_BUFFER_BYTES, CACHE_KEYS, startupCDPFailureCategory,
  EARLY_COMMAND_LABELS, EARLY_LABELS, probeError, isObject, exactKeys,
  parseControlOrigin, parseJSONBytes, CDPFrameDecoder, classifyXOperation, isSafeOperationCode,
  projectNetworkEvent, MetadataWindow, LoginObservationGate, TwoPhaseWorkflowGrant, twoPhaseChromeArgs,
  sameWindowsPath, ordinaryDirectoryIdentity, sameDirectoryIdentity, projectCDPEvent, ACCOUNT_READY_EXPRESSION,
  manualSeedRPCCode,
});
