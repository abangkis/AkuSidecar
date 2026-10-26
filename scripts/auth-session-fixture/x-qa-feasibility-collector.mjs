import { spawn } from 'node:child_process';
import { lstat, mkdir, realpath } from 'node:fs/promises';
import { basename, dirname, isAbsolute, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import {captureNativeXTarget} from './x-native-manual-capture.mjs';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';

const MODULE_DIR = dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = resolve(MODULE_DIR, '..', '..');
const QA_ROOT = join(PROJECT_ROOT, 'runtime', 'qa', 'auth-session-sidecar');
const PROFILE_NAME = 'derived-headless';
const ROOT_NAME = /^run-x-qa-source-[0-9a-f]{32}$/;
const MAX_PENDING = 32;
const COMMAND_TIMEOUT_MS = MANUAL_TIMING.cdpCommandMS;
const EXIT_TIMEOUT_MS = MANUAL_TIMING.collectorExitMS;
const MAX_COOKIE_BYTES = 8 * 1024;
const MAX_COOKIE_ARRAY_BYTES = 16 * 1024;
const MAX_FIXED_EXPRESSION_CHARS = 16 * 1024;

const ROOT_METHODS = new Set([
  'Target.getTargets',
  'Target.getBrowserContexts',
  'Target.createBrowserContext',
  'Target.disposeBrowserContext',
  'Target.createTarget',
  'Target.attachToTarget',
  'Browser.close',
]);
const SESSION_METHODS = new Set([
  'Page.enable',
  'Page.getFrameTree',
  'Page.navigate',
  'Page.reload',
  'Runtime.enable',
  'Runtime.disable',
  'Runtime.evaluate',
  'Debugger.enable',
  'Debugger.disable',
  'Debugger.setInstrumentationBreakpoint',
  'Debugger.setBreakpoint',
  'Debugger.removeBreakpoint',
  'Debugger.resume',
  'Debugger.evaluateOnCallFrame',
  'Network.enable',
  'Network.disable',
  'Network.setCacheDisabled',
  'Network.setBypassServiceWorker',
  'Network.getResponseBody',
  'Network.setCookies',
]);
const SAFE_ENV_NAMES = new Set([
  'SystemRoot', 'WINDIR', 'PATH', 'TEMP', 'TMP', 'USERPROFILE',
  'LOCALAPPDATA', 'APPDATA', 'PROGRAMDATA', 'HOMEDRIVE', 'HOMEPATH',
]);
const SAFE_ENV_NAMES_LOWER = new Set([...SAFE_ENV_NAMES].map((name) => name.toLowerCase()));
const COOKIE_FIELDS = [
  'name', 'value', 'domain', 'path', 'secure', 'httpOnly', 'sameSite',
  'expires', 'priority', 'sameParty', 'sourceScheme', 'sourcePort',
];

function fixedError(message = 'qa_command_rejected') {
  return new Error(message);
}

function isRecord(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function readDataRecord(value, allowed, required = []) {
  try {
    if (!isRecord(value)) return null;
    const prototype = Object.getPrototypeOf(value);
    if (prototype !== Object.prototype && prototype !== null) return null;
    const keys = Reflect.ownKeys(value);
    if (keys.some((key) => typeof key !== 'string' || !allowed.includes(key))) return null;
    const copied = Object.create(null);
    for (const key of keys) {
      const descriptor = Object.getOwnPropertyDescriptor(value, key);
      if (!descriptor || !Object.hasOwn(descriptor, 'value') || descriptor.enumerable !== true) return null;
      copied[key] = descriptor.value;
    }
    if (required.some((key) => !Object.hasOwn(copied, key))) return null;
    return copied;
  } catch {
    return null;
  }
}

function readDataArray(value, expectedLength) {
  try {
    if (!Array.isArray(value) || Object.getPrototypeOf(value) !== Array.prototype) return null;
    if (value.length !== expectedLength) return null;
    const keys = Reflect.ownKeys(value);
    if (keys.length !== expectedLength + 1 || !keys.includes('length')) return null;
    const copied = [];
    for (let index = 0; index < expectedLength; index += 1) {
      if (!keys.includes(String(index))) return null;
      const descriptor = Object.getOwnPropertyDescriptor(value, String(index));
      if (!descriptor || !Object.hasOwn(descriptor, 'value') || descriptor.enumerable !== true) return null;
      copied.push(descriptor.value);
    }
    return copied;
  } catch {
    return null;
  }
}

function safeIdentifier(value) {
  return typeof value === 'string'
    && value.length > 0
    && value.length <= 512
    && !/[\u0000-\u001f\u007f]/u.test(value);
}

function exactEmptyRecord(value) {
  const copied = readDataRecord(value, []);
  return copied;
}

function commandParams(method, params, sessionId, allowedExpressions, cookieSeedState) {
  const empty = params === undefined ? Object.create(null) : params;
  const isRoot = ROOT_METHODS.has(method);
  if (!isRoot && !SESSION_METHODS.has(method)) return null;
  if (isRoot ? sessionId !== undefined : !safeIdentifier(sessionId)) return null;

  if (method === 'Target.getTargets' || method === 'Target.getBrowserContexts' || method === 'Browser.close'
    || method === 'Page.enable' || method === 'Page.getFrameTree' || method === 'Runtime.enable'
    || method === 'Runtime.disable' || method === 'Debugger.enable' || method === 'Debugger.disable'
    || method === 'Debugger.resume' || method === 'Network.disable') {
    return exactEmptyRecord(empty);
  }

  if (method === 'Target.createBrowserContext') {
    const row = readDataRecord(empty, ['disposeOnDetach'], ['disposeOnDetach']);
    return row && row.disposeOnDetach === true ? { disposeOnDetach: true } : null;
  }
  if (method === 'Target.disposeBrowserContext') {
    const row = readDataRecord(empty, ['browserContextId'], ['browserContextId']);
    return row && safeIdentifier(row.browserContextId) ? { browserContextId: row.browserContextId } : null;
  }
  if (method === 'Target.createTarget') {
    const row = readDataRecord(empty, ['url', 'browserContextId'], ['url']);
    if (!row || row.url !== 'about:blank') return null;
    if (Object.hasOwn(row, 'browserContextId') && !safeIdentifier(row.browserContextId)) return null;
    return Object.hasOwn(row, 'browserContextId')
      ? { url: 'about:blank', browserContextId: row.browserContextId }
      : { url: 'about:blank' };
  }
  if (method === 'Target.attachToTarget') {
    const row = readDataRecord(empty, ['targetId', 'flatten'], ['targetId', 'flatten']);
    return row && safeIdentifier(row.targetId) && row.flatten === true
      ? { targetId: row.targetId, flatten: true }
      : null;
  }
  if (method === 'Page.navigate') {
    const row = readDataRecord(empty, ['url'], ['url']);
    return row && row.url === 'https://x.com/home' ? { url: row.url } : null;
  }
  if (method === 'Page.reload') {
    const row = readDataRecord(empty, ['ignoreCache'], ['ignoreCache']);
    return row && row.ignoreCache === false ? { ignoreCache: false } : null;
  }
  if (method === 'Runtime.evaluate') {
    const row = readDataRecord(empty, [
      'expression', 'returnByValue', 'awaitPromise', 'userGesture', 'throwOnSideEffect',
      'timeout', 'disableBreaks', 'silent', 'includeCommandLineAPI', 'generatePreview',
    ], ['expression']);
    if (!row || !allowedExpressions.has(row.expression)) return null;
    if (Object.hasOwn(row, 'returnByValue') && typeof row.returnByValue !== 'boolean') return null;
    if (Object.hasOwn(row, 'awaitPromise') && typeof row.awaitPromise !== 'boolean') return null;
    if (Object.hasOwn(row, 'userGesture') && row.userGesture !== false) return null;
    if (Object.hasOwn(row, 'throwOnSideEffect') && row.throwOnSideEffect !== true) return null;
    if (Object.hasOwn(row, 'timeout') && row.timeout !== MANUAL_TIMING.bootstrapEvaluationMS) return null;
    if (Object.hasOwn(row, 'disableBreaks') && row.disableBreaks !== true) return null;
    if (Object.hasOwn(row, 'silent') && typeof row.silent !== 'boolean') return null;
    if (Object.hasOwn(row, 'includeCommandLineAPI') && row.includeCommandLineAPI !== false) return null;
    if (Object.hasOwn(row, 'generatePreview') && row.generatePreview !== false) return null;
    return { ...row };
  }
  if (method === 'Debugger.setInstrumentationBreakpoint') {
    const row = readDataRecord(empty, ['instrumentation'], ['instrumentation']);
    return row && row.instrumentation === 'beforeScriptExecution'
      ? { instrumentation: 'beforeScriptExecution' }
      : null;
  }
  if (method === 'Debugger.setBreakpoint') {
    const row = readDataRecord(empty, ['location'], ['location']);
    const location = row && readDataRecord(row.location, ['scriptId', 'lineNumber', 'columnNumber'], ['scriptId', 'lineNumber']);
    if (!location || !safeIdentifier(location.scriptId)
      || !Number.isSafeInteger(location.lineNumber) || location.lineNumber < 0 || location.lineNumber > 1_000_000
      || (Object.hasOwn(location, 'columnNumber')
        && (!Number.isSafeInteger(location.columnNumber) || location.columnNumber < 0 || location.columnNumber > 1_000_000))) return null;
    return { location: Object.hasOwn(location, 'columnNumber')
      ? { scriptId: location.scriptId, lineNumber: location.lineNumber, columnNumber: location.columnNumber }
      : { scriptId: location.scriptId, lineNumber: location.lineNumber } };
  }
  if (method === 'Debugger.removeBreakpoint') {
    const row = readDataRecord(empty, ['breakpointId'], ['breakpointId']);
    return row && safeIdentifier(row.breakpointId) ? { breakpointId: row.breakpointId } : null;
  }
  if (method === 'Debugger.evaluateOnCallFrame') {
    const row = readDataRecord(empty, [
      'callFrameId', 'expression', 'returnByValue', 'throwOnSideEffect', 'timeout',
      'silent', 'includeCommandLineAPI', 'generatePreview',
    ], ['callFrameId', 'expression']);
    if (!row || !safeIdentifier(row.callFrameId) || !allowedExpressions.has(row.expression)) return null;
    if (Object.hasOwn(row, 'returnByValue') && typeof row.returnByValue !== 'boolean') return null;
    if (Object.hasOwn(row, 'throwOnSideEffect') && row.throwOnSideEffect !== true) return null;
    if (Object.hasOwn(row, 'timeout') && row.timeout !== MANUAL_TIMING.bootstrapEvaluationMS) return null;
    if (Object.hasOwn(row, 'silent') && typeof row.silent !== 'boolean') return null;
    if (Object.hasOwn(row, 'includeCommandLineAPI') && row.includeCommandLineAPI !== false) return null;
    if (Object.hasOwn(row, 'generatePreview') && row.generatePreview !== false) return null;
    return { ...row };
  }
  if (method === 'Network.enable') {
    const row = readDataRecord(empty, ['maxTotalBufferSize', 'maxResourceBufferSize', 'maxPostDataSize']);
    if (!row) return null;
    if (Object.keys(row).length === 0) return {};
    return row.maxTotalBufferSize === 1_000_000
      && row.maxResourceBufferSize === 100_000
      && row.maxPostDataSize === 0
      && Object.keys(row).length === 3
      ? { maxTotalBufferSize: 1_000_000, maxResourceBufferSize: 100_000, maxPostDataSize: 0 }
      : null;
  }
  if(method==='Network.setBypassServiceWorker'){const row=readDataRecord(empty,['bypass'],['bypass']);return row&&typeof row.bypass==='boolean'?{bypass:row.bypass}:null;}
  if (method === 'Network.setCacheDisabled') {
    const row = readDataRecord(empty, ['cacheDisabled'], ['cacheDisabled']);
    return row && typeof row.cacheDisabled === 'boolean' ? { cacheDisabled: row.cacheDisabled } : null;
  }
  if (method === 'Network.getResponseBody') {
    const row = readDataRecord(empty, ['requestId'], ['requestId']);
    return row && safeIdentifier(row.requestId) ? { requestId: row.requestId } : null;
  }
  if (method === 'Network.setCookies') {
    if (sessionId === undefined || cookieSeedState.used) return null;
    const row = readDataRecord(empty, ['cookies'], ['cookies']);
    const inputCookies = row && readDataArray(row.cookies, 2);
    if (!inputCookies) return null;
    const cookies = [];
    for (const inputCookie of inputCookies) {
      const cookie = readDataRecord(inputCookie, COOKIE_FIELDS, ['name', 'value', 'domain', 'path', 'secure']);
      if (!cookie
        || typeof cookie.name !== 'string' || typeof cookie.value !== 'string'
        || cookie.domain !== '.x.com' || cookie.path !== '/' || cookie.secure !== true) return null;
      if (!['auth_token', 'ct0'].includes(cookie.name)) return null;
      if (Object.hasOwn(cookie, 'httpOnly') && typeof cookie.httpOnly !== 'boolean') return null;
      if (Object.hasOwn(cookie, 'sameSite') && !['Strict', 'Lax', 'None'].includes(cookie.sameSite)) return null;
      if (Object.hasOwn(cookie, 'expires') && (!Number.isFinite(cookie.expires) || cookie.expires < 0)) return null;
      if (Object.hasOwn(cookie, 'priority') && !['Low', 'Medium', 'High'].includes(cookie.priority)) return null;
      if (Object.hasOwn(cookie, 'sameParty') && typeof cookie.sameParty !== 'boolean') return null;
      if (Object.hasOwn(cookie, 'sourceScheme') && !['Unset', 'NonSecure', 'Secure'].includes(cookie.sourceScheme)) return null;
      if (Object.hasOwn(cookie, 'sourcePort')
        && (!Number.isSafeInteger(cookie.sourcePort) || cookie.sourcePort < -1 || cookie.sourcePort > 65535)) return null;
      const copied = Object.create(null);
      for (const key of COOKIE_FIELDS) if (Object.hasOwn(cookie, key)) copied[key] = cookie[key];
      if (Buffer.byteLength(JSON.stringify(copied), 'utf8') > MAX_COOKIE_BYTES) return null;
      cookies.push(copied);
    }
    if (cookies[0].name !== 'auth_token' || cookies[1].name !== 'ct0'
      || Buffer.byteLength(JSON.stringify(cookies), 'utf8') > MAX_COOKIE_ARRAY_BYTES) return null;
    cookieSeedState.used = true;
    return { cookies };
  }
  return null;
}

function safeEnvironment() {
  const env = Object.create(null);
  const copiedNames = new Set();
  for (const [name, value] of Object.entries(process.env)) {
    const lowerName = name.toLowerCase();
    if (SAFE_ENV_NAMES_LOWER.has(lowerName) && !copiedNames.has(lowerName) && typeof value === 'string') {
      env[name] = value;
      copiedNames.add(lowerName);
    }
  }
  return env;
}

async function validateAndCreateProfile(root, executable) {
  if (typeof root !== 'string' || !isAbsolute(root) || resolve(root) !== root) throw fixedError('qa_root_rejected');
  if (typeof executable !== 'string' || !isAbsolute(executable) || resolve(executable) !== executable) throw fixedError('qa_executable_rejected');

  let canonicalParent;
  let rootInfo;
  let executableInfo;
  try {
    canonicalParent = await realpath(QA_ROOT);
    rootInfo = await lstat(root);
    executableInfo = await lstat(executable);
  } catch {
    throw fixedError('qa_root_or_executable_unavailable');
  }
  if (basename(root).match(ROOT_NAME) === null || dirname(root) !== canonicalParent
    || !rootInfo.isDirectory() || rootInfo.isSymbolicLink()) throw fixedError('qa_root_rejected');
  if (await realpath(root).catch(() => '') !== root) throw fixedError('qa_root_rejected');
  if (!executableInfo.isFile() || executableInfo.isSymbolicLink()
    || !/\.exe$/iu.test(basename(executable))) throw fixedError('qa_executable_rejected');
  if (await realpath(executable).catch(() => '') !== executable) throw fixedError('qa_executable_rejected');

  const profile = join(root, PROFILE_NAME);
  try {
    await lstat(profile);
    throw fixedError('qa_profile_already_exists');
  } catch (error) {
    if (error?.message === 'qa_profile_already_exists') throw error;
    if (error?.code !== 'ENOENT') throw fixedError('qa_profile_rejected');
  }
  try {
    await mkdir(profile, { mode: 0o700 });
    const profileInfo = await lstat(profile);
    if (!profileInfo.isDirectory() || profileInfo.isSymbolicLink() || await realpath(profile) !== profile) {
      throw fixedError('qa_profile_rejected');
    }
  } catch (error) {
    if (error?.message?.startsWith('qa_')) throw error;
    throw fixedError('qa_profile_rejected');
  }
  return profile;
}

function exactKeys(value, expected) {
  if (!isRecord(value)) return false;
  try {
    const keys = Reflect.ownKeys(value);
    return keys.length === expected.length
      && keys.every((key) => typeof key === 'string' && expected.includes(key))
      && keys.every((key) => {
        const descriptor = Object.getOwnPropertyDescriptor(value, key);
        return descriptor && Object.hasOwn(descriptor, 'value') && descriptor.enumerable === true;
      });
  } catch {
    return false;
  }
}

function safeEventEnvelope(message) {
  if (!isRecord(message) || Object.hasOwn(message, 'id')) return null;
  const hasSession = Object.hasOwn(message, 'sessionId');
  const expected = hasSession ? ['method', 'params', 'sessionId'] : ['method', 'params'];
  if (!exactKeys(message, expected) || typeof message.method !== 'string'
    || !/^[A-Za-z][A-Za-z0-9]*\.[A-Za-z][A-Za-z0-9]*$/u.test(message.method)
    || !isRecord(message.params)
    || (hasSession && !safeIdentifier(message.sessionId))) return null;
  return message;
}

function validateErrorPayload(value) {
  if (!isRecord(value)) return false;
  const hasData = Object.hasOwn(value, 'data');
  if (!exactKeys(value, hasData ? ['code', 'message', 'data'] : ['code', 'message'])) return false;
  return Number.isSafeInteger(value.code)
    && typeof value.message === 'string' && value.message.length <= 1024
    && (!hasData || (typeof value.data === 'string' && value.data.length <= 8192));
}

export function createQAFeasibilityCollector({
  executable,
  root,
  applicationProfile,
  loadAssets,
  spawnChrome = spawn,
  FrameDecoder,
  allowedExpressions = [],
} = {}) {
  if (typeof spawnChrome !== 'function' || typeof FrameDecoder !== 'function'
    || !Array.isArray(allowedExpressions) || allowedExpressions.length > 32) {
    throw fixedError('qa_configuration_rejected');
  }
  const expressions = new Set();
  for (const expression of allowedExpressions) {
    if (typeof expression !== 'string' || expression.length === 0 || expression.length > MAX_FIXED_EXPRESSION_CHARS
      || /[\u0000\u0001\u0002\u0003\u0004\u0005\u0006\u0007\u0008\u000b\u000c\u000e\u000f\u0010-\u001f\u007f]/u.test(expression)) {
      throw fixedError('qa_configuration_rejected');
    }
    expressions.add(expression);
  }

  let state = 'new';
  let child;
  let decoder;
  let nextId = 1;
  let transportFailed = false;
  let processExited = false;
  let cookieSeedUsed = false;
  let closePromise;
  let ownedProfile;
  const pending = new Map();
  const listeners = new Set();

  const rejectPending = (error) => {
    for (const entry of pending.values()) {
      clearTimeout(entry.timer);
      entry.reject(error);
    }
    pending.clear();
  };

  const markTransportFailed = () => {
    if (transportFailed) return;
    transportFailed = true;
    try { decoder?.reset(); } catch { /* Fixed failure below is sufficient. */ }
    rejectPending(fixedError('qa_transport_failed'));
  };

  const onProcessExit = () => {
    if (processExited) return;
    processExited = true;
    state = 'exited';
    try { decoder?.reset(); } catch { /* No process output is retained. */ }
    rejectPending(fixedError('qa_chrome_exited'));
  };

  const deliverMessage = (message) => {
    if (!isRecord(message)) throw fixedError('qa_protocol_invalid');
    if (Object.hasOwn(message, 'id')) {
      if (!Number.isSafeInteger(message.id) || message.id < 1) throw fixedError('qa_protocol_invalid');
      const entry = pending.get(message.id);
      if (!entry) throw fixedError('qa_protocol_invalid');
      const hasResult = Object.hasOwn(message, 'result');
      const hasError = Object.hasOwn(message, 'error');
      if (hasResult === hasError) throw fixedError('qa_protocol_invalid');
      const keys = entry.sessionId === undefined
        ? ['id', hasResult ? 'result' : 'error']
        : ['id', 'sessionId', hasResult ? 'result' : 'error'];
      if (!exactKeys(message, keys) || (entry.sessionId !== undefined && message.sessionId !== entry.sessionId)) {
        throw fixedError('qa_protocol_invalid');
      }
      if (hasResult && !isRecord(message.result)) throw fixedError('qa_protocol_invalid');
      if (hasError && !validateErrorPayload(message.error)) throw fixedError('qa_protocol_invalid');
      pending.delete(message.id);
      clearTimeout(entry.timer);
      if (hasError) {
        entry.reject(fixedError('qa_cdp_command_failed'));
        return;
      }
      let projected;
      try {
        projected = entry.project(message.result);
        if (projected !== null && (typeof projected === 'object' || typeof projected === 'function')
          && typeof projected.then === 'function') throw fixedError('qa_project_must_be_synchronous');
      } catch {
        entry.reject(fixedError('qa_project_failed'));
        return;
      }
      entry.resolve(projected);
      return;
    }

    const event = safeEventEnvelope(message);
    if (!event) throw fixedError('qa_protocol_invalid');
    for (const listener of [...listeners]) {
      try { listener(event); } catch { /* Raw CDP event failures are not logged or propagated. */ }
    }
  };

  const onData = (chunk) => {
    if (transportFailed || processExited) return;
    try {
      decoder.push(chunk, deliverMessage);
    } catch {
      markTransportFailed();
    }
  };

  const launch = async () => {
    if (state !== 'new') throw fixedError('qa_launch_unavailable');
    state = 'validating';
    let profile;
    try {
      // Only private application composition supplies an already leased profile.
      // QA callers retain the new-profile-only validation path.
      if(applicationProfile!==undefined){
        if(typeof applicationProfile!=='string'||!isAbsolute(applicationProfile)||resolve(applicationProfile)!==applicationProfile)throw fixedError('qa_profile_rejected');
        const info=await lstat(applicationProfile);
        if(!info.isDirectory()||info.isSymbolicLink()||await realpath(applicationProfile)!==applicationProfile)throw fixedError('qa_profile_rejected');
        profile=applicationProfile;
      }else profile = await validateAndCreateProfile(root, executable);
      ownedProfile = profile;
    } catch (error) {
      state = 'failed';
      if (error?.message?.startsWith('qa_')) throw error;
      throw fixedError('qa_launch_validation_failed');
    }

    const args = [
      `--user-data-dir=${profile}`,
      '--headless=new',
      '--remote-debugging-pipe',
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-extensions',
      '--disable-logging',
      '--disable-breakpad',
      '--disable-crash-reporter',
      '--disable-background-mode',
      '--disable-sync',
      'about:blank',
    ];
    try {
      decoder = new FrameDecoder();
      if (!decoder || typeof decoder.push !== 'function' || typeof decoder.reset !== 'function') {
        throw fixedError('qa_decoder_rejected');
      }
      child = spawnChrome(executable, args, {
        windowsHide: true,
        stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'],
        env: safeEnvironment(),
      });
      if (!child || !Number.isSafeInteger(child.pid) || child.pid < 1 || !Array.isArray(child.stdio)
        || !child.stdio[3] || !child.stdio[4] || typeof child.once !== 'function') {
        throw fixedError('qa_spawn_failed');
      }
      child.once('exit', onProcessExit);
      child.once('error', () => {
        if (state === 'launching') state = 'failed';
        markTransportFailed();
      });
      child.stdio[4].on('data', onData);
      state = 'launching';
      await new Promise((resolveSpawn, rejectSpawn) => {
        let settled = false;
        child.once('spawn', () => {
          if (settled) return;
          settled = true;
          resolveSpawn();
        });
        child.once('exit', () => {
          if (settled) return;
          settled = true;
          rejectSpawn(fixedError('qa_spawn_failed'));
        });
        child.once('error', () => {
          if (settled) return;
          settled = true;
          rejectSpawn(fixedError('qa_spawn_failed'));
        });
      });
      if (processExited || transportFailed) throw fixedError('qa_spawn_failed');
      state = 'launched';
      return Object.freeze({ pid: child.pid });
    } catch (error) {
      state = 'failed';
      markTransportFailed();
      if (error?.message?.startsWith('qa_')) throw error;
      throw fixedError('qa_spawn_failed');
    }
  };

  const send = (method, params = {}, sessionId = undefined, project = (result) => result) => {
    if (!['launched', 'closing'].includes(state) || transportFailed || processExited) {
      return Promise.reject(fixedError('qa_transport_unavailable'));
    }
    if (typeof project !== 'function') return Promise.reject(fixedError('qa_project_rejected'));
    const cookieState = { used: cookieSeedUsed };
    const cleanParams = commandParams(method, params, sessionId, expressions, cookieState);
    if (cleanParams === null || cleanParams === undefined || pending.size >= MAX_PENDING
      || nextId > Number.MAX_SAFE_INTEGER) return Promise.reject(fixedError('qa_command_rejected'));
    if (method === 'Network.setCookies') cookieSeedUsed = cookieState.used;
	return writeCommand(method,cleanParams,sessionId,project);
  };

  // Only trusted worker composition receives this private writer. The public
  // startup send retains its fixed-expression and home-only restrictions.
  const writeCommand=(method,cleanParams,sessionId,project=result=>result,timeoutMs=COMMAND_TIMEOUT_MS)=>{
    if(!['launched','closing'].includes(state)||transportFailed||processExited||pending.size>=MAX_PENDING) return Promise.reject(fixedError('qa_transport_unavailable'));

    const id = nextId;
    nextId += 1;
    const request = Object.create(null);
    request.id = id;
    request.method = method;
    request.params = cleanParams;
    if (sessionId !== undefined) request.sessionId = sessionId;
    let wire;
    try {
      wire = Buffer.from(`${JSON.stringify(request)}\0`, 'utf8');
      if(wire.length>262144)throw fixedError('qa_command_rejected');
    } catch {
      return Promise.reject(fixedError('qa_command_rejected'));
    }

    return new Promise((resolveResponse, rejectResponse) => {
      const timer = setTimeout(() => {
        if (!pending.has(id)) return;
        pending.delete(id);
        rejectResponse(fixedError('qa_command_timeout'));
      }, Math.max(1,Math.min(MANUAL_TIMING.attemptAuthorizationMaxMS,timeoutMs)));
      pending.set(id, { sessionId, project, resolve: resolveResponse, reject: rejectResponse, timer });
      try {
        child.stdio[3].write(wire, (error) => {
          wire.fill(0);
          if (error) markTransportFailed();
        });
      } catch {
        wire.fill(0);
        pending.delete(id);
        clearTimeout(timer);
        rejectResponse(fixedError('qa_transport_write_failed'));
        markTransportFailed();
      }
    });
  };

  const subscribe = (listener) => {
    if (typeof listener !== 'function') throw fixedError('qa_listener_rejected');
    listeners.add(listener);
    return () => listeners.delete(listener);
  };

  const waitForRootExit = () => {
    if (processExited) return Promise.resolve(true);
    return new Promise((resolveExit) => {
      let settled = false;
      const finish = (exited) => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        child.off('exit', onExit);
        resolveExit(exited);
      };
      const onExit = () => finish(true);
      const timer = setTimeout(() => finish(false), EXIT_TIMEOUT_MS);
      child.once('exit', onExit);
    });
  };

  const close = () => {
    if (closePromise) return closePromise;
    closePromise = (async () => {
      if (!child || processExited) return;
      let exitPromise;
      if (state === 'launched' && !transportFailed) {
        const closeRequest = send('Browser.close', {});
        state = 'closing';
        void closeRequest.catch(() => {});
      }
      exitPromise = waitForRootExit();
      const exited = await exitPromise;
      if (!exited) throw fixedError('qa_chrome_exit_unverified');
    })();
    return closePromise;
  };

  // Private owner binding only. No credentials, body or account projection.
  const ownership = () => {
    if(!child || !ownedProfile) throw fixedError('qa_transport_unavailable');
    return Object.freeze({pid:child.pid,profileId:ownedProfile,alive:state==='launched'&&!transportFailed&&!processExited});
  };
  let captureBusy=false;
  const captureWorker=async(request,{browserContextId,proofSession,signal,onUnsafe}={})=>{
    if((applicationProfile===undefined&&process.env.AKU_X_NATIVE_MANUAL_CAPTURE_ALLOWED!=='1')||captureBusy||request?.pins?.ownerId!==String(child?.pid)||request?.pins?.profileId!==ownedProfile)throw fixedError('qa_capture_unavailable');
    captureBusy=true;
    try{return await captureNativeXTarget({request,bridgePath:resolve(PROJECT_ROOT,'..','AkuBridge'),loadAssets,browserContextId,proofSession,signal,onUnsafe,subscribe,
      command:(method,params,sessionId,timeoutMs)=>writeCommand(method,params,sessionId,result=>result,timeoutMs)});}
    finally{captureBusy=false;}
  };
  return Object.freeze({ launch, send, subscribe, close, ownership, captureWorker });
}

