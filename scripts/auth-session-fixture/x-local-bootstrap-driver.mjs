import { BootstrapBreakpointHandoff as LocalBreakpointHandoff } from './x-bootstrap-breakpoint-handoff.mjs';
export { BootstrapBreakpointHandoff as LocalBreakpointHandoff } from './x-bootstrap-breakpoint-handoff.mjs';
// Accountless loopback-only timeout reproduction. The enclosing Go Job owns
// Node and its fresh Chrome child. No retained profile or production mode exists.
import { spawn } from 'node:child_process';
import { lstat, mkdir, realpath, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import { EarlyBootstrapCapture } from './x-early-bootstrap-capture.mjs';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, inspectBootstrapCurrentSelf, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { CDPFrameDecoder, projectCDPEvent } from './x-native-metadata-probe-driver.mjs';
import { decodeControl } from './x-standard-chrome-control.mjs';

const COMMAND_LABELS = Object.freeze({
  'Target.getTargets': 'targets', 'Target.attachToTarget': 'attach', 'Page.enable': 'pageEnable',
  'Page.getFrameTree': 'frameTree', 'Runtime.enable': 'runtimeEnable', 'Runtime.disable': 'runtimeDisable',
  'Debugger.enable': 'debuggerEnable', 'Debugger.disable': 'debuggerDisable',
  'Debugger.setInstrumentationBreakpoint': 'breakpointSet', 'Debugger.removeBreakpoint': 'breakpointRemove',
  'Debugger.setBreakpoint': 'normalBreakpointSet',
  'Page.reload': 'reload', 'Debugger.resume': 'resume', 'Debugger.evaluateOnCallFrame': 'pausedRead',
  'Runtime.evaluate': 'lateRead', 'Browser.close': 'browserClose',
});
const EVENT_LABELS = new Set(['rootHome', 'rootOther', 'contextCreated', 'contextsCleared', 'contextDestroyed',
  'scriptParsed', 'instrumentationPause', 'otherPause', 'resumed', 'pageLoaded', 'protocol',
  'initialHomePending', 'initialHomeReady', 'initialHomeUnavailable', 'instrumentationOmitted', 'breakpointRemovalOmitted',
  'handoffInstalled', 'handoffHitMatched', 'handoffHitRejected', 'handoffHitMissing', 'handoffLimitReached', 'lateHomeUnavailable']);
const ALL_COMMAND_LABELS = new Set([...Object.values(COMMAND_LABELS), 'initialFrame', 'pausedReadRuntime', 'lateFrame', 'normalBreakpointRemove']);
const CASES = Object.freeze({
  constant: ['instrumentation_breakpoint', 'constant_callframe_full', 'none'],
  projection: ['instrumentation_breakpoint', 'projection_callframe_full', 'none'],
  instrumentation_resume_only: ['instrumentation_breakpoint', 'resume_only', 'none'],
  instrumentation_minimal: ['instrumentation_breakpoint', 'constant_callframe_minimal', 'none'],
  ordinary_full: ['debugger_statement', 'constant_callframe_full', 'ordinary_other_as_capture_eligible'],
  instrumentation_runtime: ['instrumentation_breakpoint', 'constant_runtime_context', 'none'],
  handoff_constant: ['instrumentation_to_normal_breakpoint', 'constant_callframe_full', 'verified_normal_breakpoint_as_capture_eligible'],
  handoff_projection: ['instrumentation_to_normal_breakpoint', 'projection_callframe_full', 'verified_normal_breakpoint_as_capture_eligible'],
});
export function localBootstrapCase(kind) {
  if (!Object.hasOwn(CASES, kind)) throw fail();
  const [pauseSource, evaluation, pauseAdapter] = CASES[kind];
  return { pauseSource, evaluation, pauseAdapter };
}
const PHASES = new Set(['setup', 'observation', 'handoff_install', 'paused_read', 'cleanup', 'late_read', 'complete']);
const MAX_CHRONOLOGY = 512;
const fail = () => new Error('local_bootstrap_failed');
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const safeId = value => typeof value === 'string' && value.length > 0 && value.length <= 256 && !/[\u0000-\u001f\u007f]/u.test(value);
const keysAre = (value, keys) => object(value) && Object.keys(value).length === keys.length && Object.keys(value).every(key => keys.includes(key));
const validPosition = location => object(location) && safeId(location.scriptId) &&
  Number.isSafeInteger(location.lineNumber) && location.lineNumber >= 0 && location.lineNumber <= 1000000 &&
  Number.isSafeInteger(location.columnNumber) && location.columnNumber >= 0 && location.columnNumber <= 1000000;

export function localBootstrapOrigin(origin) {
  if (typeof origin !== 'string' || origin.length > 128) throw fail();
  const url = new URL(origin);
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port || url.origin !== origin ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash) throw fail();
  return origin;
}

export function localBootstrapExpression(origin) {
  localBootstrapOrigin(origin);
  // Only the two origin comparisons change. The inspected location is real;
  // this adapter does not pretend that localhost has an X origin.
  if (BOOTSTRAP_CURRENT_SELF_EXPRESSION.split("'https://x.com'").length !== 3) throw fail();
  return BOOTSTRAP_CURRENT_SELF_EXPRESSION.replaceAll("'https://x.com'", JSON.stringify(origin));
}

export function localBootstrapDocuments(kind = 'projection') {
  const ordinary = localBootstrapCase(kind).pauseSource === 'debugger_statement';
  const handoff = ['handoff_constant', 'handoff_projection'].includes(kind);
  const initialState = handoff ? '{session:{user_id:"123456",user_id_str:"123456"},entities:{users:{entities:{"123456":{id_str:"123456"}}}},users:{entities:{"123456":{id_str:"123456"}}}}' :
    '{session:{user_id_str:"123456"}}';
  return {
    home: '<!doctype html><html lang="en"><meta charset="utf-8"><title>Accountless bootstrap reproduction</title><h1>Local synthetic bootstrap</h1>' +
      `<script>${ordinary ? 'debugger;' : ''}globalThis.__INITIAL_STATE__=${initialState};</script>` +
      '<script src="/middle.js"></script><script>delete globalThis.__INITIAL_STATE__;</script>' +
      '<script src="/late.js"></script></html>',
    middle: 'void 0;',
    late: 'void 0;',
  };
}

export function localBootstrapChromeArgs(profile, origin) {
  if (!isAbsolute(profile)) throw fail();
  localBootstrapOrigin(origin);
  return [`--user-data-dir=${profile}`, '--profile-directory=Default', '--remote-debugging-pipe',
    '--no-first-run', '--no-default-browser-check', '--disable-extensions', '--disable-background-networking',
    '--disable-component-update', '--disable-sync', '--disable-background-mode', '--disable-default-apps',
    '--disable-logging', '--disable-breakpad', '--disable-crash-reporter', '--metrics-recording-only',
    '--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1', '--no-proxy-server', '--new-window', `${origin}/home`];
}

export class LocalBootstrapTransport {
  constructor({ origin, kind, write, timeoutMs = 4000 } = {}) {
    this.origin = localBootstrapOrigin(origin);
    if (!Object.hasOwn(CASES, kind) || typeof write !== 'function' ||
        !Number.isSafeInteger(timeoutMs) || timeoutMs < 1 || timeoutMs > 4000) throw fail();
    this.kind = kind;
    this.mechanism = localBootstrapCase(kind);
    this.projection = this.mechanism.evaluation === 'projection_callframe_full';
    this.handoff = this.mechanism.pauseSource === 'instrumentation_to_normal_breakpoint' ? new LocalBreakpointHandoff(this) : null;
    this.variant = !['constant', 'projection'].includes(kind) && !this.handoff;
    this.actualInstrumentationPauses = 0;
    this.actualOtherPauses = 0;
    this.expression = localBootstrapExpression(origin);
    this.write = write;
    this.timeoutMs = timeoutMs;
    this.decoder = new CDPFrameDecoder();
    this.pending = new Map();
    this.nextId = 0;
    this.sessionId = null;
    this.capture = null;
    this.lateAllowed = false;
    this.phase = 'setup';
    this.failure = false;
    this.firstFailure = null;
    this.chronology = [];
    this.chronologyTruncated = false;
    this.sequence = 0;
    this.pageLoaded = false;
    this.loadWaiters = [];
    this.rootHomeSeen = false;
    this.homeWaiters = [];
  }

  record(kind, label, status) {
    if (!['command', 'event'].includes(kind) || !(kind === 'command' ? ALL_COMMAND_LABELS.has(label) : EVENT_LABELS.has(label)) ||
        !['sent', 'acknowledged', 'error', 'timeout', 'rejected', 'observed'].includes(status) || !PHASES.has(this.phase)) throw fail();
    const entry = { sequence: ++this.sequence, kind, label, status, phase: this.phase };
    if (this.firstFailure === null && ['error', 'timeout'].includes(status)) this.firstFailure = { ...entry };
    if (this.chronology.length < MAX_CHRONOLOGY) this.chronology.push(entry);
    else this.chronologyTruncated = true;
  }

  terminate(label = 'protocol', status = 'error') {
    if (!this.failure) {
      this.record(label === 'protocol' ? 'event' : 'command', label, status);
      this.failure = true;
    }
    this.decoder.reset();
    for (const item of this.pending.values()) { clearTimeout(item.timer); item.reject(fail()); }
    this.pending.clear();
    for (const resolveWait of this.loadWaiters.splice(0)) resolveWait(false);
    for (const resolveWait of this.homeWaiters.splice(0)) resolveWait(false);
  }

  validCommand(method, params, purpose = null) {
    if (!Object.hasOwn(COMMAND_LABELS, method) || !object(params)) return false;
    if (method === 'Target.attachToTarget') return keysAre(params, ['targetId', 'flatten']) && safeId(params.targetId) && params.flatten === true;
    if (method === 'Debugger.setInstrumentationBreakpoint') return keysAre(params, ['instrumentation']) && params.instrumentation === 'beforeScriptExecution';
    if (method === 'Debugger.setBreakpoint') return this.handoff && keysAre(params, ['location']) && this.handoff.canInstall(params.location);
    if (method === 'Debugger.removeBreakpoint') return keysAre(params, ['breakpointId']) && safeId(params.breakpointId);
    if (method === 'Page.reload') return keysAre(params, ['ignoreCache']) && params.ignoreCache === false;
    if (method === 'Debugger.evaluateOnCallFrame') {
      if (this.mechanism.evaluation === 'constant_callframe_minimal') {
        return keysAre(params, ['callFrameId', 'expression', 'returnByValue']) &&
          this.capture?.canReadCallFrame(params.callFrameId) === true && params.expression === 'true' && params.returnByValue === true;
      }
      return keysAre(params, ['callFrameId', 'expression', 'returnByValue', 'throwOnSideEffect', 'timeout', 'silent', 'includeCommandLineAPI', 'generatePreview']) &&
        this.capture?.canReadCallFrame(params.callFrameId) === true &&
        (!this.handoff || this.handoff.canEvaluate(params.callFrameId)) &&
        ['constant_callframe_full', 'projection_callframe_full'].includes(this.mechanism.evaluation) &&
        params.expression === (this.projection ? this.expression : 'true') && params.returnByValue === true &&
        params.throwOnSideEffect === true && params.timeout === 1500 && params.silent === true &&
        params.includeCommandLineAPI === false && params.generatePreview === false;
    }
    if (method === 'Runtime.evaluate') {
      if (purpose === 'paused_runtime') return this.mechanism.evaluation === 'constant_runtime_context' &&
        this.capture?.canRead(params.contextId) === true &&
        keysAre(params, ['expression', 'returnByValue', 'awaitPromise', 'userGesture', 'throwOnSideEffect', 'timeout', 'disableBreaks', 'contextId']) &&
        params.expression === 'true' && params.returnByValue === true && params.awaitPromise === false && params.userGesture === false &&
        params.throwOnSideEffect === true && params.timeout === 1500 && params.disableBreaks === true;
      return this.lateAllowed && keysAre(params, ['expression', 'returnByValue', 'awaitPromise', 'userGesture', 'throwOnSideEffect', 'timeout']) &&
        params.expression === (this.variant ? 'true' : this.expression) && params.returnByValue === true && params.awaitPromise === false &&
        params.userGesture === false && params.throwOnSideEffect === true && params.timeout === 1500;
    }
    return Object.keys(params).length === 0;
  }

  send(method, params = {}, purpose = null) {
    if (purpose !== null && !(purpose === 'initial_home' && method === 'Page.getFrameTree') &&
        !(purpose === 'late_home' && method === 'Page.getFrameTree' && this.handoff) &&
        !(purpose === 'normal_remove' && method === 'Debugger.removeBreakpoint' && this.handoff?.breakpoints.has(params.breakpointId)) &&
        !(purpose === 'paused_runtime' && method === 'Runtime.evaluate')) return Promise.reject(fail());
    const label = purpose === 'initial_home' ? 'initialFrame' : purpose === 'late_home' ? 'lateFrame' : purpose === 'normal_remove' ? 'normalBreakpointRemove' : purpose === 'paused_runtime' ? 'pausedReadRuntime' : COMMAND_LABELS[method];
    if (!label || !this.validCommand(method, params, purpose)) return Promise.reject(fail());
    if (this.failure) { this.record('command', label, 'rejected'); return Promise.reject(fail()); }
    if (this.pending.size >= 32) { this.terminate(); return Promise.reject(fail()); }
    if (method === 'Page.reload') { this.phase = 'observation'; this.pageLoaded = false; }
    else if (method === 'Debugger.setBreakpoint') this.phase = 'handoff_install';
    else if (method === 'Debugger.evaluateOnCallFrame' || purpose === 'paused_runtime') this.phase = 'paused_read';
    else if (method === 'Debugger.removeBreakpoint') this.phase = 'cleanup';
    else if (method === 'Runtime.evaluate') this.phase = 'late_read';
    else if (purpose === 'late_home') this.phase = 'late_read';
    this.record('command', label, 'sent');
    const id = ++this.nextId;
    return new Promise((accept, reject) => {
      const timer = setTimeout(() => { if (this.pending.has(id)) this.terminate(label, 'timeout'); }, this.timeoutMs);
      const sessionId = method.startsWith('Target.') || method === 'Browser.close' ? null : this.sessionId;
      this.pending.set(id, { method, label, purpose, params, sessionId, accept, reject, timer });
      const bytes = Buffer.from(`${JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) })}\0`);
      try { this.write(bytes, error => { bytes.fill(0); if (error) this.terminate(); }); }
      catch { bytes.fill(0); this.terminate(); }
    });
  }

  push(bytes) {
    if (this.failure) return;
    try { this.decoder.push(bytes, message => this.dispatch(message)); }
    catch { this.terminate(); }
  }

  dispatch(message) {
    if (Object.hasOwn(message, 'method')) {
      if (!keysAre(message, message.sessionId === undefined ? ['method', 'params'] : ['method', 'params', 'sessionId']) || !object(message.params)) throw fail();
      if (message.sessionId !== this.sessionId) return;
      let event;
      if (message.method === 'Page.frameNavigated') {
        const frame = message.params.frame;
        if (!object(frame) || Object.hasOwn(frame, 'parentId') || !safeId(frame.id)) return;
        event = { type: 'frame_navigated', frameId: frame.id, isHome: frame.url === `${this.origin}/home` };
        this.rootHomeSeen = event.isHome;
        if (event.isHome) for (const resolveWait of this.homeWaiters.splice(0)) resolveWait(true);
      } else if (message.method === 'Page.loadEventFired') {
        this.record('event', 'pageLoaded', 'observed');
        this.pageLoaded = true;
        for (const resolveWait of this.loadWaiters.splice(0)) resolveWait(true);
        return;
      } else event = projectCDPEvent(message)?.event;
      if (!event) return;
      const label = { frame_navigated: event.isHome ? 'rootHome' : 'rootOther', context_created: 'contextCreated',
        contexts_cleared: 'contextsCleared', context_destroyed: 'contextDestroyed', script_parsed: 'scriptParsed',
        paused: event.reason === 'instrumentation' ? 'instrumentationPause' : 'otherPause', resumed: 'resumed' }[event.type];
      if (label) this.record('event', label, 'observed');
      if (event.type === 'paused') {
        if (event.reason === 'instrumentation') this.actualInstrumentationPauses = Math.min(64, this.actualInstrumentationPauses + 1);
        else this.actualOtherPauses = Math.min(64, this.actualOtherPauses + 1);
        // The shared capture only admits instrumentation pauses. This local
        // adapter is explicit in the report; the actual CDP cause stays above.
        if (this.mechanism.pauseAdapter === 'ordinary_other_as_capture_eligible' && event.reason === 'other') {
          event = { ...event, reason: 'instrumentation' };
        }
      }
      if (this.handoff) event = this.handoff.observe(event, message.params);
      this.capture?.observe(event);
      return;
    }
    const hasResult = Object.hasOwn(message, 'result');
    if (!Number.isSafeInteger(message.id) || !keysAre(message, ['id', ...(message.sessionId === undefined ? [] : ['sessionId']), hasResult ? 'result' : 'error'])) throw fail();
    const item = this.pending.get(message.id);
    if (!item || (message.sessionId ?? null) !== item.sessionId) throw fail();
    this.pending.delete(message.id);
    clearTimeout(item.timer);
    if (!hasResult) { this.record('command', item.label, 'error'); item.reject(fail()); return; }
    try {
      const result = this.project(item.method, message.result, item.purpose, item.params);
      this.record('command', item.label, 'acknowledged');
      item.accept(result);
    } catch { this.record('command', item.label, 'error'); item.reject(fail()); }
  }

  project(method, result, purpose = null, params = null) {
    if (!object(result)) throw fail();
    if (method === 'Target.getTargets') {
      if (!Array.isArray(result.targetInfos) || result.targetInfos.length > 128) throw fail();
      const ids = result.targetInfos.filter(target => target?.type === 'page' && target.url === `${this.origin}/home` && safeId(target.targetId)).map(target => target.targetId);
      return { targetIds: ids };
    }
    if (method === 'Target.attachToTarget') { if (!safeId(result.sessionId)) throw fail(); return { sessionId: result.sessionId }; }
    if (method === 'Page.getFrameTree') {
      const frame = result.frameTree?.frame;
      if (!safeId(frame?.id) || typeof frame.url !== 'string' || frame.url.length > 2048) throw fail();
      if (purpose === 'initial_home' || purpose === 'late_home') return { ready: frame.url === `${this.origin}/home` };
      if (frame.url !== `${this.origin}/home`) throw fail();
      if (this.handoff) this.handoff.root = frame.id;
      return { frameId: frame.id };
    }
    if (method === 'Debugger.setBreakpoint') {
      this.handoff.created(result.breakpointId);
      if (!validPosition(result.actualLocation) || result.actualLocation.scriptId !== params.location.scriptId) throw fail();
      return { breakpointId: result.breakpointId, actualLocation: { scriptId: result.actualLocation.scriptId,
        lineNumber: result.actualLocation.lineNumber, columnNumber: result.actualLocation.columnNumber } };
    }
    if (method === 'Debugger.setInstrumentationBreakpoint') { if (!safeId(result.breakpointId)) throw fail(); return { breakpointId: result.breakpointId }; }
    if (method === 'Debugger.evaluateOnCallFrame' || method === 'Runtime.evaluate') {
      if (Object.hasOwn(result, 'exceptionDetails') || !object(result.result)) throw fail();
      if (purpose === 'paused_runtime' || (method === 'Debugger.evaluateOnCallFrame' && !this.projection) ||
          (method === 'Runtime.evaluate' && this.variant)) {
        if (result.result.type !== 'boolean' || result.result.value !== true) throw fail();
        return { constant: true };
      }
      if (result.result.type !== 'object') throw fail();
      return validateBootstrapCurrentSelf(result.result.value);
    }
    return { acknowledged: true };
  }

  async waitForLoad() {
    if (this.pageLoaded) return true;
    if (this.failure) return false;
    let timer;
    return new Promise(accept => {
      const finish = value => { clearTimeout(timer); this.loadWaiters = this.loadWaiters.filter(item => item !== finish); accept(value); };
      timer = setTimeout(() => finish(false), 5000);
      this.loadWaiters.push(finish);
    });
  }

  async waitForInitialHome() {
    // Target.targetInfo.url may name the requested URL before its root frame
    // commits. Query the committed frame first; a pending frame is not an error.
    const initial = await this.send('Page.getFrameTree', {}, 'initial_home');
    if (initial.ready) { this.record('event', 'initialHomeReady', 'observed'); return true; }
    this.record('event', 'initialHomePending', 'observed');
    let committed = this.rootHomeSeen;
    if (!committed && !this.failure) {
      committed = await new Promise(accept => {
        let timer;
        const finish = value => { clearTimeout(timer); this.homeWaiters = this.homeWaiters.filter(item => item !== finish); accept(value); };
        timer = setTimeout(() => finish(false), 5000);
        this.homeWaiters.push(finish);
      });
    }
    if (committed && !this.failure && (await this.send('Page.getFrameTree', {}, 'initial_home')).ready) {
      this.record('event', 'initialHomeReady', 'observed');
      return true;
    }
    this.record('event', 'initialHomeUnavailable', 'error');
    return false;
  }

  async captureCommand(method, params) {
    if (this.handoff && method === 'Debugger.removeBreakpoint') {
      let result;
      try { result = await this.send(method, params); }
      finally { await this.handoff.cleanup(); }
      return this.handoff.snapshot().normalCleanupVerified ? result : { acknowledged: false };
    }
    if (this.mechanism.pauseSource === 'debugger_statement') {
      if (method === 'Debugger.setInstrumentationBreakpoint') {
        this.record('event', 'instrumentationOmitted', 'observed');
        return Promise.resolve({ breakpointId: 'local-ordinary-no-instrumentation' });
      }
      if (method === 'Debugger.removeBreakpoint' && params.breakpointId === 'local-ordinary-no-instrumentation') {
        this.phase = 'cleanup';
        this.record('event', 'breakpointRemovalOmitted', 'observed');
        return Promise.resolve({ acknowledged: true });
      }
    }
    return this.send(method, params);
  }
}

export async function runLocalBootstrapCapture(transport) {
  let constantReads = 0;
  let pausedEvaluationAttempts = 0;
  let initialHomeReady = false;
  try { initialHomeReady = await transport.waitForInitialHome(); }
  catch { /* The first failing readiness command remains in the chronology. */ }
  const capture = new EarlyBootstrapCapture({
    send: (method, params) => transport.captureCommand(method, params),
    readBootstrap: async (contextId, callFrameId) => {
      if (!capture.canRead(contextId) || !capture.canReadCallFrame(callFrameId)) throw fail();
      const missing = () => inspectBootstrapCurrentSelf({ location: { origin: 'https://x.com', pathname: '/home' } });
      if (transport.mechanism.evaluation === 'resume_only') return missing();
      if (transport.handoff && !(await transport.handoff.beforeRead(contextId, callFrameId))) return missing();
      pausedEvaluationAttempts += 1;
      let result;
      try {
      if (transport.mechanism.evaluation === 'constant_runtime_context') {
        result = await transport.send('Runtime.evaluate', { expression: 'true', returnByValue: true, awaitPromise: false,
          userGesture: false, throwOnSideEffect: true, timeout: 1500, disableBreaks: true, contextId }, 'paused_runtime');
      } else if (transport.mechanism.evaluation === 'constant_callframe_minimal') {
        result = await transport.send('Debugger.evaluateOnCallFrame', { callFrameId, expression: 'true', returnByValue: true });
      } else {
        result = await transport.send('Debugger.evaluateOnCallFrame', { callFrameId,
          expression: transport.projection ? transport.expression : 'true', returnByValue: true,
          throwOnSideEffect: true, timeout: 1500, silent: true, includeCommandLineAPI: false, generatePreview: false });
      }
      } finally { if (transport.handoff) await transport.handoff.afterRead(callFrameId); }
      if (transport.projection) return result;
      if (result.constant !== true) throw fail();
      constantReads += 1;
      // The control reports only execution success; its neutral missing DTO
      // intentionally never satisfies the capture's positive stop condition.
      return missing();
    },
  });
  transport.capture = capture;
  const early = initialHomeReady ? await capture.run() : {
    schema: 'aku.x.early-bootstrap-capture.v1', stopReason: 'setup_failed', pauseCount: 0,
    readAttempts: 0, bootstrapSeen: false, snapshot: null, cleanupVerified: true,
    canAuthorize: false, continuityProven: false,
  };
  transport.capture = null;
  if (transport.handoff) await transport.handoff.cleanup();
  let late = null;
  let lateConstantResult = null;
  let lateReady = initialHomeReady && early.cleanupVerified && !transport.failure &&
    (!transport.handoff || transport.handoff.snapshot().normalCleanupVerified && !transport.handoff.rejected && transport.handoff.stats.matched > 0) &&
    await transport.waitForLoad();
  if (lateReady && transport.handoff) {
    try { lateReady = (await transport.send('Page.getFrameTree', {}, 'late_home')).ready; }
    catch { lateReady = false; }
    if (!lateReady && !transport.firstFailure) transport.record('event', 'lateHomeUnavailable', 'error');
  }
  if (lateReady) {
    transport.lateAllowed = true;
    try {
      const result = await transport.send('Runtime.evaluate', { expression: transport.variant ? 'true' : transport.expression,
        returnByValue: true, awaitPromise: false, userGesture: false, throwOnSideEffect: true, timeout: 1500 });
      if (transport.variant) lateConstantResult = result.constant === true;
      else late = result;
    }
    catch { /* Fixed chronology identifies the failure without source strings. */ }
    transport.lateAllowed = false;
  }
  transport.phase = 'complete';
  return { schema: 'aku.x.local-bootstrap-repro.v3', case: transport.kind, adapter: 'loopback_origin_comparisons_only',
    mechanism: { ...transport.mechanism, actualInstrumentationPauses: transport.actualInstrumentationPauses,
      actualOtherPauses: transport.actualOtherPauses, pausedEvaluationAttempts, lateConstantResult, handoff: transport.handoff?.snapshot() ?? null },
    early, late, constantReads, firstFailure: transport.firstFailure,
    chronology: transport.chronology.map(entry => ({ ...entry })), chronologyTruncated: transport.chronologyTruncated, canAuthorize: false };
}

async function main() {
  const [executable, root, origin, ...extra] = process.argv.slice(2);
  if (extra.length || !isAbsolute(executable ?? '') || !isAbsolute(root ?? '')) throw fail();
  localBootstrapOrigin(origin);
  const project = await realpath(resolve(dirname(fileURLToPath(import.meta.url)), '..', '..'));
  const parent = await realpath(join(project, 'runtime', 'qa', 'auth-session-sidecar'));
  const actualRoot = await realpath(root);
  const rel = relative(parent, actualRoot);
  const stat = await lstat(root);
  if (!stat.isDirectory() || stat.isSymbolicLink() || actualRoot.toLowerCase() !== resolve(root).toLowerCase() ||
      !rel || isAbsolute(rel) || rel === '..' || rel.startsWith(`..${sep}`) ||
      !/^run-x-local-bootstrap-(?:constant|projection|instrumentation_resume_only|instrumentation_minimal|ordinary_full|instrumentation_runtime|handoff_constant|handoff_projection)-[a-f0-9]{16,64}$/u.test(actualRoot.split(/[\\/]/u).at(-1))) throw fail();
  const kind = actualRoot.split(/[\\/]/u).at(-1).split('-').at(-2);
  let bootstrapped = false;
  let documentsProvided = false;
  let launched = false;
  let captureUsed = false;
  let child;
  let exited = false;
  let transport;
  const emit = value => process.stdout.write(`${JSON.stringify(value)}\n`);
  const handle = async bytes => {
    const message = decodeControl(bytes);
    if (!bootstrapped) {
      if (!keysAre(message, ['op', 'withholdReady']) || message.op !== 'fixture_bootstrap' || message.withholdReady !== false) throw fail();
      bootstrapped = true; emit({ ready: true }); return;
    }
    if (!Number.isSafeInteger(message.id) || message.id < 1) throw fail();
    const id = message.id;
    if (message.op === 'local_bootstrap_documents' && !documentsProvided && !launched && keysAre(message, ['id', 'op'])) {
      documentsProvided = true; emit({ id, ok: true, result: localBootstrapDocuments(kind) }); return;
    }
    if (message.op === 'launch' && documentsProvided && !launched && keysAre(message, ['id', 'op', 'role', 'nativeReaderStartup']) &&
        message.role === 'collector' && message.nativeReaderStartup === false) {
      launched = true;
      const profile = join(actualRoot, 'shared');
      await mkdir(profile, { recursive: false, mode: 0o700 });
      await mkdir(join(profile, 'Default'), { mode: 0o700 });
      await writeFile(join(profile, 'Default', 'Preferences'), JSON.stringify({ profile: { name: 'Accountless bootstrap QA' } }), { flag: 'wx', mode: 0o600 });
      child = spawn(executable, localBootstrapChromeArgs(profile, origin), { windowsHide: false, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
      transport = new LocalBootstrapTransport({ origin, kind, write: (bytes, callback) => child.stdio[3].write(bytes, callback) });
      child.on('error', () => { exited = true; transport.terminate(); });
      child.once('exit', () => { exited = true; transport.terminate(); emit({ event: 'chrome_exit', owner: 1 }); });
      child.stdio[3].on('error', () => transport.terminate());
      child.stdio[4].on('error', () => transport.terminate());
      child.stdio[4].on('data', chunk => transport.push(chunk));
      await new Promise((accept, reject) => { child.once('spawn', accept); child.once('error', () => reject(fail())); });
      let ids = [];
      for (let attempt = 0; attempt < 10; attempt += 1) {
        ids = (await transport.send('Target.getTargets')).targetIds;
        if (ids.length > 0) break;
        await new Promise(accept => setTimeout(accept, 100));
      }
      if (ids.length !== 1) throw fail();
      transport.sessionId = (await transport.send('Target.attachToTarget', { targetId: ids[0], flatten: true })).sessionId;
      await transport.send('Page.enable');
      emit({ id, ok: true, result: { owner: 1, pid: child.pid, windowCount: 0, bootstrapCount: 0, nativeReaderStartup: false } });
      return;
    }
    if (message.op === 'local_bootstrap_capture' && launched && !exited && !captureUsed && keysAre(message, ['id', 'op', 'owner']) && message.owner === 1) {
      captureUsed = true;
      emit({ id, ok: true, result: await runLocalBootstrapCapture(transport) });
      return;
    }
    emit({ id, ok: false, code: 'local_operation_denied' });
  };
  let pending = Buffer.alloc(0);
  let lines = 0;
  let chain = Promise.resolve();
  process.stdin.on('data', chunk => {
    if (chunk.length > 8192 || pending.length + chunk.length > 8192) { process.stdin.destroy(); return; }
    pending = Buffer.concat([pending, chunk]);
    let end;
    while ((end = pending.indexOf(10)) >= 0) {
      if (++lines > 5 || end < 1 || end > 4096) { process.stdin.destroy(); return; }
      const line = Buffer.from(pending.subarray(0, end));
      pending = Buffer.from(pending.subarray(end + 1));
      chain = chain.then(() => handle(line)).catch(() => { emit({ ready: false }); process.stdin.destroy(); }).finally(() => line.fill(0));
    }
  });
  // EOF never abandons a live child; the enclosing Go Job closes the tree.
  process.stdin.on('end', () => { if (!child || exited) process.exit(0); });
  process.stdin.on('error', () => { if (!child || exited) process.exit(2); });
}

if (process.argv[1] && pathToFileURL(resolve(process.argv[1])).href === import.meta.url) {
  void main().catch(() => { process.exitCode = 2; });
}
