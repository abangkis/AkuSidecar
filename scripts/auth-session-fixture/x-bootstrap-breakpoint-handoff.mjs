// Shared private QA breakpoint correlation. No browser/profile operations or source reads.
import { EarlyBootstrapCapture } from './x-early-bootstrap-capture.mjs';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QA_BOUND_BOOTSTRAP_EXPRESSION } from './x-qa-bound-observation.mjs';
const fail = () => new Error('bootstrap_handoff_failed');
const object = value => value !== null && typeof value === 'object' && !Array.isArray(value);
const safeId = value => typeof value === 'string' && value.length > 0 && value.length <= 256 && !/[\u0000-\u001f\u007f]/u.test(value);
const keysAre = (value, keys) => object(value) && Object.keys(value).length === keys.length && Object.keys(value).every(key => keys.includes(key));
const validPosition = location => object(location) && safeId(location.scriptId) && Number.isSafeInteger(location.lineNumber) && location.lineNumber >= 0 && location.lineNumber <= 1000000 && Number.isSafeInteger(location.columnNumber) && location.columnNumber >= 0 && location.columnNumber <= 1000000;

// Private local correlation only. No IDs or source positions enter snapshot().
export class BootstrapBreakpointHandoff {
  constructor(transport) {
    this.transport = transport;
    this.root = null;
    this.context = null;
    this.version = 0;
    this.navigated = false;
    this.scripts = new Map();
    this.pauses = new Map();
    this.breakpoints = new Map();
    this.installing = null;
    this.rejected = false;
    this.limitReached = false;
    this.cleanupCertain = true;
    this.stats = { installAttempts: 0, installed: 0, matched: 0, unexpected: 0, mismatched: 0, stale: 0, replayed: 0,
      removed: 0, normalCleanupVerified: true, outcome: 'not_started' };
  }

  invalidate() { this.context = null; this.version += 1; this.scripts.clear(); this.pauses.clear(); }
  current(binding) { return binding && binding.contextId === this.context && binding.version === this.version && this.context !== null; }
  reject(category = 'mismatched') {
    this.stats[category] = Math.min(64, this.stats[category] + 1);
    this.stats.outcome = 'hit_rejected';
    if (!this.rejected) this.transport.record('event', 'handoffHitRejected', 'error');
    this.rejected = true;
  }

  observe(event, rawParams) {
    if (event.type === 'frame_navigated') {
      this.invalidate();
      this.navigated = event.frameId === this.root && event.isHome;
    } else if (event.type === 'contexts_cleared' || event.type === 'context_destroyed' && event.contextId === this.context) {
      this.invalidate();
    } else if (event.type === 'context_created' && this.navigated && event.frameId === this.root) {
      if (this.context !== event.contextId) { this.invalidate(); this.context = event.contextId; }
    } else if (event.type === 'script_parsed' && this.navigated && event.frameId === this.root && event.contextId === this.context) {
      if (!this.scripts.has(event.scriptId) && this.scripts.size >= 64) { this.reject(); return event; }
      this.scripts.set(event.scriptId, { contextId: this.context, version: this.version });
    } else if (event.type === 'resumed') {
      this.pauses.clear();
    } else if (event.type === 'paused') {
      const top = rawParams?.callFrames?.[0];
      const location = top?.location;
      const binding = this.scripts.get(event.scriptId);
      if (event.reason === 'instrumentation') {
        if (this.current(binding) && validPosition(location) && location.scriptId === event.scriptId && safeId(event.callFrameId)) {
          this.pauses.set(event.callFrameId, { ...binding, ...location, kind: 'instrumentation' });
        }
      } else {
        const hits = rawParams?.hitBreakpoints;
        const candidate = Array.isArray(hits) && hits.length === 1 ? this.breakpoints.get(hits[0]) : null;
        if (!candidate) this.reject('unexpected');
        else if (!this.current(candidate)) this.reject('stale');
        else if (candidate && candidate.state !== 'pending') this.reject('replayed');
        else if (!this.current(binding) || !validPosition(location) || !safeId(event.callFrameId) ||
            location.scriptId !== candidate.scriptId || location.lineNumber !== candidate.lineNumber || location.columnNumber !== candidate.columnNumber ||
            binding.contextId !== candidate.contextId || binding.version !== candidate.version || this.rejected) this.reject();
        else {
          candidate.state = 'matched';
          this.stats.matched += 1;
          this.stats.outcome = 'normal_hit_verified';
          this.pauses.set(event.callFrameId, { ...candidate, kind: 'normal', breakpointId: hits[0] });
          this.transport.record('event', 'handoffHitMatched', 'observed');
          return { ...event, reason: 'instrumentation' };
        }
      }
    }
    return event;
  }

  canInstall(location) {
    const token = this.installing;
    return token && this.current(token) && this.transport.capture?.canReadCallFrame(token.callFrameId) === true &&
      keysAre(location, ['scriptId', 'lineNumber', 'columnNumber']) && validPosition(location) &&
      ['scriptId', 'lineNumber', 'columnNumber'].every(key => location[key] === token[key]);
  }

  created(id) {
    if (!safeId(id) || this.breakpoints.has(id) || this.breakpoints.size >= 8) { this.cleanupCertain = false; throw fail(); }
    this.breakpoints.set(id, { state: 'invalid', removed: false });
    this.stats.installed += 1;
  }

  async beforeRead(contextId, callFrameId) {
    const pause = this.pauses.get(callFrameId);
    if (!this.current(pause) || pause.contextId !== contextId || this.rejected) { this.reject('stale'); throw fail(); }
    if (this.limitReached && pause.kind === 'instrumentation') return false;
    if (pause.kind === 'normal') {
      const breakpoint = this.breakpoints.get(pause.breakpointId);
      if (breakpoint?.state !== 'matched') { this.reject('replayed'); throw fail(); }
      breakpoint.state = 'reading';
      return true;
    }
    if ([...this.breakpoints.values()].some(value => !value.removed)) {
      this.stats.outcome = 'no_normal_hit'; this.rejected = true;
      this.transport.record('event', 'handoffHitMissing', 'error'); throw fail();
    }
    if (this.stats.installAttempts >= 8) {
      this.limitReached = true;
      this.stats.outcome = 'normal_breakpoint_limit';
      this.transport.record('event', 'handoffLimitReached', 'observed');
      return false;
    }
    this.stats.installAttempts += 1;
    this.installing = { ...pause, callFrameId };
    try {
      const result = await this.transport.send('Debugger.setBreakpoint', { location: {
        scriptId: pause.scriptId, lineNumber: pause.lineNumber, columnNumber: pause.columnNumber } });
      if (!this.current(pause)) { this.reject('stale'); throw fail(); }
      const candidate = this.breakpoints.get(result.breakpointId);
      Object.assign(candidate, pause, result.actualLocation, { state: 'pending' });
      this.stats.outcome = 'waiting_for_normal';
      this.transport.record('event', 'handoffInstalled', 'observed');
      return false;
    } catch {
      this.cleanupCertain = false;
      if (!this.rejected) this.stats.outcome = 'install_failed';
      throw fail();
    } finally { this.installing = null; }
  }

  canEvaluate(callFrameId) {
    const pause = this.pauses.get(callFrameId);
    return !this.rejected && pause?.kind === 'normal' && this.current(pause) &&
      this.breakpoints.get(pause.breakpointId)?.state === 'reading';
  }

  async remove(id) {
    const candidate = this.breakpoints.get(id);
    if (!candidate || candidate.removed) return true;
    try {
      const result = await this.transport.send('Debugger.removeBreakpoint', { breakpointId: id }, 'normal_remove');
      if (result.acknowledged !== true) throw fail();
      candidate.removed = true;
      candidate.state = 'removed';
      this.stats.removed += 1;
      return true;
    } catch { this.cleanupCertain = false; this.stats.outcome = 'cleanup_failed'; return false; }
  }

  async afterRead(callFrameId) {
    const pause = this.pauses.get(callFrameId);
    if (!pause || !this.current(pause) || this.rejected) throw fail();
    if (!(await this.remove(pause.breakpointId))) throw fail();
  }

  async cleanup() {
    for (const [id, candidate] of this.breakpoints) {
      if (!candidate.removed && candidate.state === 'pending' && !this.rejected) {
        this.stats.outcome = 'no_normal_hit'; this.rejected = true;
        this.transport.record('event', 'handoffHitMissing', 'error');
      }
      if (!candidate.removed) await this.remove(id);
    }
    return this.snapshot().normalCleanupVerified;
  }

  snapshot() {
    return { ...this.stats, normalCleanupVerified: this.cleanupCertain && [...this.breakpoints.values()].every(value => value.removed) };
  }
}

// Native QA adapter. Instrumentation only installs; verified normal hits alone
// receive the fixed projector with all original evaluation guards.
export class BootstrapHandoffCapture {
  constructor({ send, record = () => {}, boundIdentity = false, beforeRuntimeDisable, beforeIdentityRead = async () => {} } = {}) {
    if (typeof send !== 'function' || typeof record !== 'function' || typeof boundIdentity !== 'boolean' || typeof beforeIdentityRead !== 'function') throw fail();
    this.instrumentationPauses = 0;
    this.otherPauses = 0;
    this.evaluationAttempts = 0;
    this.transport = { send, record, capture: null };
    this.handoff = new BootstrapBreakpointHandoff(this.transport);
    this.capture = new EarlyBootstrapCapture({
      beforeRuntimeDisable,
      send: async (method, params) => {
        if (method === 'Debugger.removeBreakpoint') {
          let result;
          try { result = await send(method, params); }
          finally { await this.handoff.cleanup(); }
          return this.handoff.snapshot().normalCleanupVerified ? result : { acknowledged: false };
        }
        const result = await send(method, params);
        if (method === 'Page.getFrameTree') this.handoff.root = result.frameId;
        return result;
      },
      readBootstrap: async (contextId, callFrameId) => {
        if (!this.capture.canRead(contextId) || !this.capture.canReadCallFrame(callFrameId)) throw fail();
        if (!(await this.handoff.beforeRead(contextId, callFrameId))) {
          return inspectBootstrapCurrentSelf({ location: { origin: 'https://x.com', pathname: '/home' } });
        }
        this.evaluationAttempts += 1;
        try {
          await beforeIdentityRead(contextId);
          if (!this.capture.canRead(contextId) || !this.capture.canReadCallFrame(callFrameId) || !this.handoff.canEvaluate(callFrameId)) throw fail();
          return await send('Debugger.evaluateOnCallFrame', { callFrameId, expression: boundIdentity ? QA_BOUND_BOOTSTRAP_EXPRESSION : BOOTSTRAP_CURRENT_SELF_EXPRESSION,
            returnByValue: true, throwOnSideEffect: true, timeout: 1500, silent: true,
            includeCommandLineAPI: false, generatePreview: false });
        } finally { await this.handoff.afterRead(callFrameId); }
      },
    });
    this.transport.capture = this.capture;
  }

  observe(event, rawParams) {
    if (event.type === 'paused') {
      if (event.reason === 'instrumentation') this.instrumentationPauses = Math.min(64, this.instrumentationPauses + 1);
      else this.otherPauses = Math.min(64, this.otherPauses + 1);
    }
    return this.capture.observe(this.handoff.observe(event, rawParams));
  }
  canRead(contextId) { return this.capture.canRead(contextId); }
  canReadCallFrame(callFrameId) { return this.capture.canReadCallFrame(callFrameId) && this.handoff.canEvaluate(callFrameId); }
  async run() {
    const result = await this.capture.run();
    await this.handoff.cleanup();
    return { ...result, cleanupVerified: result.cleanupVerified && this.handoff.snapshot().normalCleanupVerified };
  }
  diagnostics() {
    return { mode: 'instrumentation_to_normal_breakpoint', pauseAdapter: 'verified_normal_breakpoint_as_capture_eligible',
      actualInstrumentationPauses: this.instrumentationPauses, actualOtherPauses: this.otherPauses,
      pausedEvaluationAttempts: this.evaluationAttempts, handoffRejected: this.handoff.rejected, handoff: this.handoff.snapshot() };
  }
}
