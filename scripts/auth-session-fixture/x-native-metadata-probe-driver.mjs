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
import { QASourceCapture, projectQASourceBody, projectQACookieMetadata, qaCookieRequest, finishQASourceQualification } from './x-qa-source-qualification.mjs';
import { QAGuestCapture, qaGuestCommand, QA_GUEST_HOME, finishQAAuthControl } from './x-qa-auth-control.mjs';
import { QADirectCapture, qaABCommand, QA_AB_URL, projectQADirectBody, finishQADirectAB } from './x-qa-direct-ab.mjs';
import { qaWebCommand, QA_WEB_INSTALL, QA_WEB_SNAPSHOT, QA_WEB_START, QA_WEB_DISPOSE, validateQAWebHook, finishQAWebABA } from './x-qa-web-aba.mjs';
import { projectQA401Body, finishQAWeb401 } from './x-qa-401-projection.mjs';
import { QABoundObservation, createManualV2BoundObservation, QA_BOUND_BOOTSTRAP_EXPRESSION, projectBoundBootstrap } from './x-qa-bound-observation.mjs';
import { QACookieWriteCapture, finishQACookieWriteScope } from './x-qa-cookie-writes.mjs';
import { shortQACheckpoint, selectQAShortCookies, runQAShortFeasibility } from './x-qa-short-feasibility.mjs';
import { QACheckpointRendezvous, synchronizedQAReport } from './x-qa-checkpoint-rendezvous.mjs';
import { createQAFeasibilityCollector } from './x-qa-feasibility-collector.mjs';
import { QA_LIVE_SELF_EXPRESSION, projectQALiveSelf } from './x-qa-live-self.mjs';
import { QALiveSelfTransport, finishQALiveSelf, qaLiveSelfCommand } from './x-qa-live-self-transport.mjs';
import { createManualStartupOwner } from './x-manual-startup-owner.mjs';
import { createXManualOwnerPort, manualOwnerOperation } from './x-native-manual-owner-port.mjs';
import { createRetainedManualSetup, manualSeedFailure, manualSeedDiagnosticCode } from './x-manual-retained-setup.mjs';
import { ManualEnrollmentCapture } from './x-manual-enrollment-capture.mjs';
import { ManualStartupCookieGuard } from './x-manual-startup-cookie-guard.mjs';
import {startupFailure,startupFailureLatch,startupRPCCode,requireBoundStartup} from './x-manual-startup-diagnostic.mjs';

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
  if(projectionKind==='manual_enrollment_capture')return active.manualEnrollment.project(method,result);
  switch (method) {
    case 'Page.addScriptToEvaluateOnNewDocument':
      if (!safeId(result.identifier)) throw probeError('qa_web_identifier_invalid');
      return { identifier: result.identifier };
    case 'Page.getFrameTree': {
      const frame = result.frameTree?.frame;
      if (!isObject(frame) || !safeId(frame.id) || !(['qa_guest', 'qa_ab'].includes(projectionKind) ? frame.url === 'about:blank' : isHomeTarget(frame.url))) throw probeError('early_frame_invalid');
      if (projectionKind === 'qa_live_self' || projectionKind === 'manual_frame') {
        if (!safeId(frame.loaderId)) throw probeError('qa_live_document_invalid');
        return { frameId: frame.id, loaderId: frame.loaderId };
      }
      return { frameId: frame.id };
    }
    case 'Target.createBrowserContext':
      if (!safeId(result.browserContextId)) throw probeError('qa_guest_context_invalid');
      return { browserContextId: result.browserContextId };
    case 'Target.createTarget':
      if (!safeId(result.targetId)) throw probeError('qa_guest_target_invalid');
      return { targetId: result.targetId };
    case 'Page.navigate':
      if (projectionKind === 'qa_ab') {
        if (!safeId(result.frameId) || result.frameId !== active?.ab?.frameId ||
            own(result, 'errorText') && result.errorText !== 'net::ERR_HTTP_RESPONSE_CODE_FAILURE') throw probeError('qa_ab_navigation_failed');
        return { acknowledged: true };
      }
      if (own(result, 'errorText') || !safeId(result.frameId) || result.frameId !== active?.guest?.frameId) throw probeError('qa_guest_navigation_invalid');
      return { acknowledged: true };
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
      if (projectionKind === 'qa_web_401') {
        if (!active?.webABA?.qualify401 || active.webABA.rejection !== null) throw probeError('qa_rejection_read_replayed');
        const projected = projectQA401Body(result);
        active.webABA.rejection = projected.rejection;
        return projected.body;
      }
      if (projectionKind === 'qa_bound') return projectQADirectBody(result);
      if (projectionKind === 'qa_ab' || projectionKind === 'qa_web') return projectQADirectBody(result);
      return qualificationMode(driverMode) ? projectQASourceBody(result) : projectAccountSettingsBody(result);
    case 'Network.getCookies':
      if (projectionKind === 'qa_short_cookies') {
        try{return selectQAShortCookies(result);}
        catch(error){if(active?.manualSetup)throw manualSeedFailure('cookie_select','rejected');throw error;}
      }
      if (!qualificationMode(driverMode) || !active?.qaCookieIssued) throw probeError('cookie_mode_required');
      return projectQACookieMetadata(result);
    case 'Target.getTargets': {
      if (!Array.isArray(result.targetInfos) || result.targetInfos.length > 128) throw probeError('target_inventory_invalid');
      const matches = [];
      for (const target of result.targetInfos) {
        if (isObject(target) && target.type === 'page' && (projectionKind === 'manual_inventory' || isTargetURL(target.url)) && safeId(target.targetId)) {
          matches.push(target.targetId);
        }
      }
      if(projectionKind==='manual_inventory')active.manualInventoryTargets=new Set(matches);
      return { targetIds: matches };
    }
    case 'Browser.getWindowForTarget':
      if (!Number.isSafeInteger(result.windowId) || result.windowId < 1) throw probeError('qa_short_window_invalid');
      return { windowId: result.windowId };
    case 'Target.attachToTarget':
      if (!safeId(result.sessionId)) throw probeError('target_session_invalid');
      return { sessionId: result.sessionId };
    case 'Target.closeTarget':
      if (result.success !== true) throw probeError('qa_ab_close_failed');
      return { acknowledged: true };
    case 'Debugger.evaluateOnCallFrame':
    case 'Runtime.evaluate': {
      if (projectionKind === 'qa_live_self') {
        if (!active?.liveSelf?.running || own(result, 'exceptionDetails') || !isObject(result.result) || result.result.type !== 'object') throw probeError('qa_live_result_invalid');
        try { return projectQALiveSelf(result.result.value); } catch { throw probeError('qa_live_projection_invalid'); }
      }
      if (projectionKind === 'qa_web_hook') {
        if (own(result, 'exceptionDetails') || !isObject(result.result) || !isObject(result.result.value)) throw probeError('qa_web_hook_unavailable');
        try { return validateQAWebHook(result.result.value); } catch { throw probeError('qa_web_hook_invalid'); }
      }
      if (projectionKind === 'bootstrap_current_self') {
        if (!(driverMode === 'retained_bootstrap' || active?.bound?.running) || own(result, 'exceptionDetails') ||
            !isObject(result.result) || result.result.type !== 'object' || !isObject(result.result.value)) {
          throw probeError('bootstrap_runtime_result_invalid');
        }
        if (active?.bound?.running) {
          return projectBoundBootstrap(result.result.value, (id, projection) => active.bound.binder.acceptIdentity(id, projection));
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
    case 'Page.removeScriptToEvaluateOnNewDocument':
    case 'Network.enable':
    case 'Network.disable':
    case 'Network.setCacheDisabled':
    case 'Browser.close':
    case 'Target.disposeBrowserContext':
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
  const manual=manualSeedDiagnosticCode(error);if(manual)return manual;
  const value = typeof error?.message === 'string' ? error.message : '';
  if (/^xnp_[a-z0-9_]{1,80}$/u.test(value)) return value.slice(4);
  return 'probe_failed';
}

// This is the seed command's sole error-code path. A patterned xnp_* message
// from any other exception is insufficient to supply a seed diagnostic.
export function manualSeedRPCCode(error){
  return manualSeedDiagnosticCode(error)??'manual_seed_preconditions_unavailable_unknown';
}

// Own the command binding across await: the ingress parser clears its local
// message reference as soon as this bounded concurrent operation is started.
export async function executeManualReply(message,executeCommand=execute,reply=emitReply){
  const id=message.id,seed=message.op==='manual_seed_bootstrap';
  let result;
  try{
    result=await executeCommand(message);
    reply(id,Buffer.isBuffer(result)?parseStrictJSON(result.toString('utf8'),{maxJSONDepth:4,maxJSONTokens:96}):result);
  }catch(error){reply(id,undefined,seed?manualSeedRPCCode(error):startupRPCCode(error));}
  finally{if(Buffer.isBuffer(result))result.fill(0);}
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

export function isEarlyBootstrapCallFrameRead(params, capture, boundIdentity = false) {
  const keys = ['callFrameId', 'expression', 'returnByValue', 'throwOnSideEffect', 'timeout',
    'silent', 'includeCommandLineAPI', 'generatePreview'];
  return isObject(params) && Object.keys(params).length === keys.length &&
    Object.keys(params).every(key => keys.includes(key)) && capture?.canReadCallFrame(params.callFrameId) === true &&
    params.expression === (boundIdentity ? QA_BOUND_BOOTSTRAP_EXPRESSION : BOOTSTRAP_CURRENT_SELF_EXPRESSION) && params.returnByValue === true &&
    params.throwOnSideEffect === true && params.timeout === 1500 && params.silent === true &&
    params.includeCommandLineAPI === false && params.generatePreview === false;
}

function send(method, params = {}, sessionId = undefined, purpose = null) {
  if (cdpFailure) return Promise.reject(probeError(cdpFailure));
  if (!Object.hasOwn(CDP_METHODS, method) || !isObject(params) || pending.size >= MAX_PENDING_COMMANDS) {
    return Promise.reject(probeError('cdp_command_invalid'));
  }
  const webState = active?.webABA;
  const manualProvenance=purpose==='manual_provenance'&&qualificationMode(driverMode)&&process.env.AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED==='1'&&process.env.AKU_X_MANUAL_RETAINED_TRANSFER_ONCE==='1'&&!!active?.manualSetup&&!active.manualStartupPort&&sessionId===active.sessionId&&(
    method==='Page.getFrameTree'&&Object.keys(params).length===0||method==='Network.setCacheDisabled'&&!active.manualCacheControlReady&&JSON.stringify(params)==='{"cacheDisabled":true}'||method==='Network.enable'&&JSON.stringify(params)===JSON.stringify({maxTotalBufferSize:MAX_TOTAL_NETWORK_BUFFER_BYTES,maxResourceBufferSize:MAX_RESOURCE_NETWORK_BUFFER_BYTES,maxPostDataSize:0}));
  const manualEnrollment=purpose==='manual_enrollment'&&process.env.AKU_X_MANUAL_RETAINED_TRANSFER_ONCE==='1'&&qualificationMode(driverMode)&&active?.manualEnrollment?.command(method,params,sessionId)===true;
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
      method === 'Network.setCacheDisabled' ? JSON.stringify(params) === '{"cacheDisabled":true}' : method === 'Network.disable' && Object.keys(params).length === 0;
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
  if (purpose !== null && !manualControl && !manualEnrollment && !manualProvenance && !(purpose === 'normal_remove' && method === 'Debugger.removeBreakpoint' &&
      active?.earlyCapture?.handoff.breakpoints.has(params.breakpointId))) return Promise.reject(probeError('early_command_not_authorized'));
  if (shortCookieExport && (process.env.AKU_X_QA_SHORT_TRANSFER_ALLOWED !== '1' || active.shortFeasibility.exportIssued ||
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
  if (method === 'Network.setCacheDisabled' && !webCommand && !abCommand && !guestCommand && (!bodyMode(driverMode) || sessionId !== active?.sessionId ||
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
  active?.manualEnrollment?.invalidate();
  active?.manualStartupPort?.invalidate();
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

async function qualifySource(message, typesOnly = false) {
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse) throw probeError('qualification_mode_required');
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
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || current.authControlAttempted) throw probeError('qa_auth_control_gate_closed');
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
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !current.gate.loginVisible ||
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

function boundScope(current) {
  return { owner: current.manualStartupSelected ? current.pid : current.owner, profileId: current.profileId, sessionId: current.sessionId, targetId: current.targetId };
}

async function qualifyLiveSelf(message) {
  const current = requireOwner(message);
  if (process.env.AKU_X_QA_LIVE_SELF !== '1' || !qualificationMode(driverMode) || !retainedQAScope?.reuse ||
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
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !current.gate.loginVisible ||
      current.gate.observationStarted || current.earlyAttempted || current.qualificationAttempted) throw probeError('qa_bound_gate_closed');
  current.gate.beginObservation(); current.earlyAttempted = true; current.earlyRunning = true;
  return boundCheckpoint(current, cookieWriteScope);
}

async function boundCheckpoint(current, cookieWriteScope = false, pointOptions, afterIdentity, beforeIdentityRead,initialFailure, observationFactory = (...args) => new QABoundObservation(...args)) {
  current.earlyRunning = true;
  const bound = current.bound = { running: true, sampling: false, binder: null, networkDisabled: false };
  try {
    const { frameId, loaderId: oldLoader } = await send('Page.getFrameTree', {}, current.sessionId);
    const writes = cookieWriteScope ? new QACookieWriteCapture(frameId) : null;
    bound.binder = observationFactory(boundScope(current), frameId,
      requestId => send('Network.getResponseBody', { requestId }, current.sessionId), () => performance.now(),
      writes ? (raw, phase, loader) => writes.observe(raw, phase, loader) : undefined, pointOptions);
    bound.sampling = true;
    if(!current.manualCookieGuard)await send('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES,
      maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 }, current.sessionId);
    await send('Network.setCacheDisabled', { cacheDisabled: true }, current.sessionId);
    let synchronized = false;
    current.earlyCapture = new BootstrapHandoffCapture({ boundIdentity: true,
      beforeIdentityRead: contextId => beforeIdentityRead?.(bound.binder.startupPins(contextId)),
      send: async (method, params, purpose) => {
        if(method==='Page.reload' && current.manualStartupSelected)bound.binder.armManualReload(oldLoader);
        let result;
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
        // Finish this one document's request before disabling context-loss
        // observation. No second reload, negative control or cookie inspection.
        let timer;
        try { await Promise.race([bound.binder.done, new Promise(resolve => { timer = setTimeout(resolve, 2500); })]); }
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
  } finally {
    bound.sampling = false;
    if (!bound.networkDisabled) await send('Network.disable', {}, current.sessionId).catch(() => { bound.binder?.invalidate('scope_lost','cleanup_failed'); });
    bound.running = false; current.earlyRunning = false; current.earlyCapture = null;
    current.gate.setLoginVisible(false);
  }
}

async function shortReaderCheckpoint(message) {
  const current = requireOwner(message);
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !current.gate.loginVisible || current.gate.observationStarted) throw probeError('qa_short_gate_closed');
  current.gate.beginObservation(); current.earlyAttempted = true;
  const observation = await boundCheckpoint(current, false, { deferCookieMutation: true, onPrivateResult: () => {} });
  return shortQACheckpoint(observation);
}

async function prepareShortConcurrency(message) {
  const current = requireOwner(message);
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !current.gate.loginVisible ||
      current.gate.observationStarted || current.shortFeasibility) throw probeError('qa_short_gate_closed');
  const transport = createQAFeasibilityCollector({ executable, root: process.env.AKU_CHROME_FIXTURE_ROOT,
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

export async function collectorShortCheckpoint(current, afterIdentity, onPrivateStartup, beforeIdentityRead,initialFailure, observationFactory = (...args) => new QABoundObservation(...args)) {
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
    const until = Date.now() + 15000;
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
    if(method==='Page.reload' && current.manualStartupSelected)binder.armManualReload(oldLoader);
    if (method === 'Debugger.setBreakpoint' && !capture?.handoff.canInstall(params.location)) throw probeError('qa_short_collector_invalid');
    if (method === 'Network.getResponseBody' && !binder?.capture.consumeRead(params.requestId)) throw probeError('qa_short_collector_invalid');
    if (method === 'Debugger.evaluateOnCallFrame' && !isEarlyBootstrapCallFrameRead(params, capture, true)) throw probeError('qa_short_collector_invalid');
    if (method === 'Debugger.removeBreakpoint' && !(purpose === 'normal_remove' ? capture.handoff.breakpoints.has(params.breakpointId) : params.breakpointId === instrumentationId)) throw probeError('qa_short_collector_invalid');
    let result;
    try{result=await s.transport.send(method,params,s.sessionId,r=>project(method,r));}
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
    sampling = true;
    if(!s.manualCookieGuard)await command('Network.enable', { maxTotalBufferSize: MAX_TOTAL_NETWORK_BUFFER_BYTES, maxResourceBufferSize: MAX_RESOURCE_NETWORK_BUFFER_BYTES, maxPostDataSize: 0 });
    await command('Network.setCacheDisabled', { cacheDisabled: true });
    capture = new BootstrapHandoffCapture({ boundIdentity: true, send: command,
      beforeIdentityRead: contextId => beforeIdentityRead?.(binder.startupPins(contextId)), beforeRuntimeDisable: async () => {
      let timer;
      try { await Promise.race([binder.done, new Promise(resolve => { timer = setTimeout(resolve, 2500); })]); }
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
  } finally {
    sampling = false; unsubscribe();s.manualCookieUnsubscribe?.();
    if (!networkDisabled) await command('Network.disable').catch(() => {});
  }
}

// Owner actor factory selected only by the guarded private QA role protocol.
// No main provider selects it. The trusted bridge binds actual startup pins and
// invokes both captures together. This hook never seeds cookies or adopts a
// different document/context to make a requested binding pass.
export function manualReloadFrameProjection(value,frameId,oldLoader){
  if(!isObject(value)||Object.keys(value).sort().join(',')!=='frameId,loaderId'||!safeId(frameId)||!safeId(oldLoader))throw probeError('manual_startup_unavailable');
  const fields=Object.getOwnPropertyDescriptors(value);
  if(fields.frameId?.value!==frameId||fields.loaderId?.value!==oldLoader)throw probeError('manual_startup_unavailable');
  // EarlyBootstrapCapture deliberately accepts exactly one projected key.
  return {frameId};
}
export function createOwnedManualStartupOwners(message,diagnostics=startupFailureLatch()) {
  if (process.env.AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED !== '1') throw probeError('manual_startup_disabled');
  const current = requireOwner(message), collector = current.shortFeasibility;
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !manualStartupStateReady(current)) throw probeError('manual_startup_unavailable');
  const rendezvous = new QACheckpointRendezvous();
  const owner = role => createManualStartupOwner({role,diagnostics,run: async sink => {
    if (sink.signal?.aborted) throw probeError('manual_startup_unavailable');
    const abort = () => rendezvous.cancel();
    sink.signal?.addEventListener('abort', abort, { once: true });
    const failure=(error,phase,reason='unavailable')=>diagnostics.record(error,role,phase,reason);
    const manualObservation=(...args)=>createManualV2BoundObservation(...args,(reason,trace)=>{
      diagnostics.record(startupFailure(role,'bound_finish',reason,false,undefined,trace),role,'bound_finish',reason);
    },role==='reader'?current.manualCookieGuard:collector.manualCookieGuard);
    const together=async()=>{try{await rendezvous.arrive(role);}catch{throw diagnostics.record(startupFailure('pair','rendezvous','deadline'),'pair','rendezvous');}};
    try {
      let observation;
      if (role === 'reader') {
        current.gate.beginObservation(); current.earlyAttempted = true;
        observation=await boundCheckpoint(current, false, { deferCookieMutation: false, onPrivateResult: () => {},
          onPrivateStartup: sink.onPrivateStartup },together,sink.beforeIdentityRead,failure,manualObservation);
      }else{
        const result=await collectorShortCheckpoint(current,together,sink.onPrivateStartup,sink.beforeIdentityRead,failure,manualObservation);
        observation=result?.checkpoint?.observation;
      }
      requireBoundStartup(observation,role,diagnostics);
    }catch(error){const cause=failure(error,'identity');rendezvous.cancel();throw cause;
    } finally { sink.signal?.removeEventListener('abort', abort); }
  } });
  return Object.freeze({ reader: owner('reader'), collector: owner('collector') });
}

export function manualStartupStateReady(current){
  const collector=current?.shortFeasibility;
  return !!(current?.gate?.loginVisible&&!current.gate.observationStarted&&!current.earlyAttempted&&!current.qualificationAttempted&&collector&&!collector.closed&&collector.contextId&&collector.sessionId&&current.manualSeeded);
}

async function manualReaderInventory(current) {
  const {targetIds}=await send('Target.getTargets',{},undefined,'manual_owner');
  const windows=new Set();let bootstrapWindow;
  for(const targetId of targetIds){
    const {windowId}=await send('Browser.getWindowForTarget',{targetId},undefined,'manual_owner');
    windows.add(windowId);if(targetId===current.targetId)bootstrapWindow=windowId;
  }
  return {windowCount:windows.size,bootstrapCount:bootstrapWindow&&windows.has(bootstrapWindow)?1:0};
}

// Fixed private owner RPCs are intentionally concurrent: the two actors meet at
// the existing post-identity rendezvous. Serializing ACQUIRE would deadlock it.
export async function executeManualOwnerRPC(message) {
  if(process.env.AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED!=='1')throw probeError('manual_startup_disabled');
  manualOwnerOperation(message);
  const current=requireOwner(message), collector=current.shortFeasibility;
  if(!current.manualStartupPort){
    current.manualStartupSelected=true;
    try{
      const diagnostics=current.manualCookieDiagnostics??startupFailureLatch();
      current.manualStartupPort=createXManualOwnerPort({enabled:true,owner:current.owner,diagnostics,
        createOwners:()=>createOwnedManualStartupOwners(message,diagnostics),
        snapshot:async(role,document,pins)=>{
          const reader=role==='reader', owned=reader?{pid:current.pid,profileId:current.profileId,alive:!childExited&&!cdpFailure}:collector.transport.ownership();
          const targetId=reader?current.targetId:collector.targetId;
          if(document){
            const state=reader?current:collector;
            if(!state.manualObserver){
              const attached=reader?await send('Target.attachToTarget',{targetId,flatten:true},undefined,'manual_owner'):
                await collector.transport.send('Target.attachToTarget',{targetId,flatten:true},undefined,r=>{
                  if(!safeId(r.sessionId))throw probeError('manual_owner_unavailable');return{sessionId:r.sessionId};});
              state.manualObserver={sessionId:attached.sessionId,context:null,frameId:pins.frameId};
              for(const method of ['Page.enable','Runtime.enable']){
                if(reader)await send(method,{},attached.sessionId,'manual_owner');
                else await collector.transport.send(method,{},attached.sessionId,()=>({acknowledged:true}));
              }
            }
          }
          const observer=(reader?current:collector).manualObserver;
          let frame;
          if(document){
            if(reader)frame=await send('Page.getFrameTree',{},current.sessionId,'manual_owner');
            else frame=await collector.transport.send('Page.getFrameTree',{},collector.sessionId,r=>{
              const value=r.frameTree?.frame;
              if(!value||!isHomeTarget(value.url)||!safeId(value.id)||!safeId(value.loaderId))throw probeError('manual_owner_unavailable');
              return{frameId:value.id,loaderId:value.loaderId};});
          }
          return {ownerId:String(owned.pid),profileId:owned.profileId,targetId,
            sessionId:observer?.sessionId??(reader?current.sessionId:collector.sessionId),
            alive:owned.alive,headless:!reader,...(reader?await manualReaderInventory(current):{windowCount:0,bootstrapCount:0}),
            frameId:frame?.frameId,documentId:frame?.loaderId,contextId:String(observer?.context??''),
            lifecycleReady:!!observer&&Number.isSafeInteger(observer.context)&&observer.context>0};
        },
        retire:async(role,preserveReader)=>{
          if(role==='reader'){
            if(!preserveReader){const closed=await closeChrome();if(!closed.exited)throw probeError('chrome_exit_unverified');}
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
  const result=await current.manualStartupPort.execute(message);
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
  const make=(frameId,sessionId,role)=>new ManualStartupCookieGuard(frameId,sessionId,{beforeSeed:true,onRejection:(reason,trace,failureReason)=>{
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
  const readerCache=await send('Network.setCacheDisabled',{cacheDisabled:true},current.sessionId,'manual_provenance');
  if(readerCache?.acknowledged!==true)throw probeError('manual_setup_unavailable');
  await s.transport.send('Network.setCacheDisabled',{cacheDisabled:true},s.sessionId,r=>{
    if(!isObject(r)||Object.keys(r).length!==0)throw probeError('manual_setup_unavailable');
    return{acknowledged:true};
  });
  current.manualCacheControlReady=true;
  const params={maxTotalBufferSize:MAX_TOTAL_NETWORK_BUFFER_BYTES,maxResourceBufferSize:MAX_RESOURCE_NETWORK_BUFFER_BYTES,maxPostDataSize:0};
  await send('Network.enable',params,current.sessionId,'manual_provenance');
  await s.transport.send('Network.enable',params,s.sessionId,()=>({acknowledged:true}));
}

export async function seedRetainedManualPilot(message){
  exactKeys(message,['id','op','owner'],'manual_setup_unavailable');
  if(message.op!=='manual_seed_bootstrap'||process.env.AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED!=='1'||
    process.env.AKU_X_MANUAL_RETAINED_TRANSFER_ONCE!=='1'||process.env.AKU_X_QA_SHORT_TRANSFER_ALLOWED!=='1')throw manualSeedFailure('preconditions','disabled');
  const current=requireOwner(message),s=current.shortFeasibility;
  if(!qualificationMode(driverMode)||!retainedQAScope?.reuse||!current.gate.loginVisible||current.gate.observationStarted||
    !s||s.closed||s.ran||s.exportIssued||current.manualStartupPort||current.manualSetup)throw manualSeedFailure('preconditions',current.manualSetup?'replay':'rejected',current.manualSetup?'unknown':'not_attempted');
  const assertGuards=()=>{
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
      const account=await current.manualEnrollment.run();assertGuards();return account;
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
  if(process.env.AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED!=='1'||!current.manualStartupPort||current.manualAcquired?.size!==2||current.manualReaderOpened)throw probeError('manual_owner_unavailable');
  current.manualReaderOpened=true;
  await current.manualStartupPort.execute({id:message.id,op:'manual_inspect',owner:message.owner,role:'reader'});
  await send('Target.createTarget',{url:HOME_URL,newWindow:true,background:true},undefined,'manual_owner');
  const inventory=await manualReaderInventory(current);
  if(inventory.windowCount<2||inventory.bootstrapCount!==1)throw probeError('manual_owner_unavailable');
  return {opened:true,...inventory};
}

async function runShortConcurrency(message, synchronize = false) {
  const current = requireOwner(message), s = current.shortFeasibility;
  if (!qualificationMode(driverMode) || !s || s.closed || s.ran || !current.gate.loginVisible ||
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
  if (!qualificationMode(driverMode) || !retainedQAScope?.reuse || !current.gate.loginVisible ||
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
  if (process.env.AKU_X_QA_LIVE_SELF === '1' && !['qa_profile_scope', 'launch', 'login_state', 'qualify_live_self', 'release'].includes(message.op)) throw probeError('qa_live_mode_operation_denied');
  if(message.op==='manual_seed_bootstrap')return seedRetainedManualPilot(message);
  if(message.op==='manual_open_reader')return openRetainedManualReader(message);
  if(message.op.startsWith('manual_'))return executeManualOwnerRPC(message);
  if(active?.manualStartupPort&&message.op!=='release')throw probeError('manual_owner_operation_denied');
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
      if (active.shortFeasibility && !active.shortFeasibility.closed) await closeShortCollector(active);
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
