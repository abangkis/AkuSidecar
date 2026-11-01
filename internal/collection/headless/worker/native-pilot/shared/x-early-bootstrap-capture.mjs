import { validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';

const RESULT_SCHEMA = 'aku.x.early-bootstrap-capture.v1';
const OBSERVATION_MS = MANUAL_TIMING.bootstrapObservationMS;
const MAX_PAUSES = 64;
const MAX_SCRIPT_BINDINGS = 64;
const MAX_QUEUED_PAUSES = 64;
const MAX_OVERFLOW_RESUMES = 8;
const MAX_CLEANUP_RESUMES = 4;
// Let the driver's CDP command deadline settle before racing cleanup against a
// still-pending enable.
const COMMAND_TIMEOUT_MS = MANUAL_TIMING.earlyCommandMS;
const FRAME_ID_MAX_LENGTH = 256;
const STOP_REASONS = new Set([
  'captured',
  'deadline',
  'script_limit',
  'wrong_page',
  'unexpected_pause',
  'evaluation_failed',
  'setup_failed',
]);

const CAPTURE_ERROR = 'early_bootstrap_capture_failed';
const REPLAY_ERROR = 'early_bootstrap_capture_replay_denied';

function exactRecord(value, keys) {
  try {
    if (value === null || typeof value !== 'object' || Array.isArray(value)) return null;
    const prototype = Object.getPrototypeOf(value);
    if (prototype !== null && prototype !== Object.prototype) return null;
    const ownKeys = Reflect.ownKeys(value);
    if (
      ownKeys.length !== keys.length ||
      ownKeys.some((key) => typeof key !== 'string' || !keys.includes(key))
    ) {
      return null;
    }

    const result = Object.create(null);
    for (const key of keys) {
      const descriptor = Object.getOwnPropertyDescriptor(value, key);
      if (descriptor === undefined || !Object.hasOwn(descriptor, 'value')) return null;
      result[key] = descriptor.value;
    }
    return result;
  } catch {
    return null;
  }
}

function ownDataValue(value, key) {
  try {
    if (value === null || typeof value !== 'object' || Array.isArray(value)) return undefined;
    const descriptor = Object.getOwnPropertyDescriptor(value, key);
    if (descriptor === undefined || !Object.hasOwn(descriptor, 'value')) return undefined;
    return descriptor.value;
  } catch {
    return undefined;
  }
}

function validFrameId(value) {
  return (
    typeof value === 'string' &&
    value.length > 0 &&
    value.length <= FRAME_ID_MAX_LENGTH &&
    !/[\u0000-\u001f\u007f]/.test(value)
  );
}

function projectEvent(value) {
  const type = ownDataValue(value, 'type');
  if (typeof type !== 'string') return null;

  if (type === 'frame_navigated') {
    const event = exactRecord(value, ['type', 'frameId', 'isHome']);
    if (event === null || !validFrameId(event.frameId) || typeof event.isHome !== 'boolean') return null;
    return event;
  }
  if (type === 'context_created') {
    const event = exactRecord(value, ['type', 'contextId', 'frameId', 'isDefault']);
    if (
      event === null ||
      !Number.isSafeInteger(event.contextId) ||
      event.contextId <= 0 ||
      !validFrameId(event.frameId) ||
      event.isDefault !== true
    ) {
      return null;
    }
    return event;
  }
  if (type === 'contexts_cleared') {
    return exactRecord(value, ['type']);
  }
  if (type === 'context_destroyed') {
    const event = exactRecord(value, ['type', 'contextId']);
    if (event === null || !Number.isSafeInteger(event.contextId) || event.contextId <= 0) return null;
    return event;
  }
  if (type === 'paused') {
    const event = exactRecord(value, ['type', 'reason', 'callFrameId', 'scriptId']);
    if (event === null || !['instrumentation', 'other'].includes(event.reason) ||
        !(event.callFrameId === null || validFrameId(event.callFrameId)) ||
        !(event.scriptId === null || validFrameId(event.scriptId))) return null;
    return event;
  }
  if (type === 'script_parsed') {
    const event = exactRecord(value, ['type', 'scriptId', 'contextId', 'frameId', 'isDefault']);
    if (event === null || !validFrameId(event.scriptId) || !Number.isSafeInteger(event.contextId) ||
        event.contextId <= 0 || !validFrameId(event.frameId) || event.isDefault !== true) return null;
    return event;
  }
  if (type === 'resumed') return exactRecord(value, ['type']);
  return null;
}

function withTimeout(factory, timeoutMs = COMMAND_TIMEOUT_MS) {
  return new Promise((resolve) => {
    let settled = false;
    let timer;
    const finish = (result) => {
      if (settled) return;
      settled = true;
      if (timer !== undefined) clearTimeout(timer);
      resolve(result);
    };

    timer = setTimeout(() => finish({ ok: false }), timeoutMs);
    Promise.resolve()
      .then(factory)
      .then(
        (value) => finish({ ok: true, value }),
        () => finish({ ok: false }),
      );
  });
}

function validProjectedIdResult(value, key) {
  const result = exactRecord(value, [key]);
  if (result === null || !validFrameId(result[key])) return null;
  return result[key];
}

function hasAcknowledged(value) {
  const result = exactRecord(value, ['acknowledged']);
  return result !== null && result.acknowledged === true;
}

export class EarlyBootstrapCapture {
  #send;
  #beforeRuntimeDisable;
  #readBootstrap;
  #now;
  #state = 'idle';
  #runUsed = false;
  #runPromise = null;
  #stopReason = null;
  #pauseCount = 0;
  #readAttempts = 0;
  #bootstrapSeen = false;
  #snapshot = null;
  #cleanupVerified = false;
  #cleanupCertain = true;
  #rootFrameId = null;
  #breakpointId = null;
  #currentContextId = null;
  #contextVersion = 0;
  #navigationSeen = false;
  #reloadIssued = false;
  #runtimeEnableAttempted = false;
  #debuggerEnableAttempted = false;
  #breakpointAttempted = false;
  #debuggerDisabling = false;
  #debuggerDisabled = false;
  #breakpointRemovalInProgress = false;
  #cleanupSuppressDrain = false;
  #activeReadContextId = null;
  #activeReadVersion = null;
  #activeReadCallFrameId = null;
  #scripts = new Map();
  #pauseQueue = [];
  #overflowResumes = 0;
  #outstandingPauses = 0;
  #pauseDrainPromise = null;
  #waiters = [];
  #observationStart = null;
  #lastClock = null;
  #deadlineTimer = null;

  constructor({ send, readBootstrap, now = () => performance.now(), beforeRuntimeDisable = async () => {} } = {}) {
    if (typeof send !== 'function' || typeof readBootstrap !== 'function' || typeof now !== 'function' || typeof beforeRuntimeDisable !== 'function') {
      throw new TypeError('early_bootstrap_capture_invalid_dependencies');
    }
    this.#send = send;
    this.#beforeRuntimeDisable = beforeRuntimeDisable;
    this.#readBootstrap = readBootstrap;
    this.#now = now;
  }

  run() {
    if (this.#runUsed) return Promise.reject(new Error(REPLAY_ERROR));
    this.#runUsed = true;
    this.#state = 'running';
    this.#runPromise = this.#execute();
    return this.#runPromise;
  }

  observe(event) {
    if (this.#state === 'idle' || this.#state === 'complete') return false;

    const projected = projectEvent(event);
    if (projected === null) {
      if (ownDataValue(event, 'type') === 'paused' && !this.#debuggerDisabled) {
        this.#enqueuePause({ reason: 'other', contextId: null, contextVersion: null }, true);
      }
      this.#setStop('evaluation_failed', true, true);
      return false;
    }

    if (this.#state === 'running' && this.#reloadIssued) this.#checkDeadline();
    if (this.#state === 'complete') return false;

    switch (projected.type) {
      case 'frame_navigated':
        return this.#observeFrameNavigation(projected);
      case 'context_created':
        return this.#observeContextCreated(projected);
      case 'script_parsed':
        return this.#observeScriptParsed(projected);
      case 'resumed':
        if (this.#activeReadCallFrameId !== null) {
          this.#activeReadCallFrameId = null;
          this.#setStop('evaluation_failed', true, true);
        }
        return true;
      case 'contexts_cleared':
        if (!this.#reloadIssued || this.#debuggerDisabled) return false;
        this.#invalidateContext();
        return true;
      case 'context_destroyed':
        if (!this.#reloadIssued || this.#debuggerDisabled) return false;
        if (projected.contextId === this.#currentContextId) this.#invalidateContext();
        return true;
      case 'paused':
        if (this.#debuggerDisabled) return false;
        this.#enqueuePause(
          {
            reason: projected.reason,
            contextId: this.#currentContextId,
            contextVersion: this.#contextVersion,
            callFrameId: projected.callFrameId,
            scriptId: projected.scriptId,
          },
          false,
        );
        return true;
      default:
        return false;
    }
  }

  canRead(contextId) {
    return (
      this.#state === 'running' &&
      this.#activeReadContextId === contextId &&
      this.#activeReadVersion === this.#contextVersion &&
      this.#currentContextId === contextId &&
      this.#activeReadCallFrameId !== null &&
      this.#navigationSeen &&
      this.#stopReason === null
    );
  }

  canReadCallFrame(callFrameId) {
    return validFrameId(callFrameId) && this.#activeReadCallFrameId === callFrameId &&
      this.canRead(this.#activeReadContextId);
  }

  async #execute() {
    try {
      const frameTree = await this.#command('Page.getFrameTree', {});
      this.#rootFrameId = validProjectedIdResult(frameTree, 'frameId');
      if (this.#rootFrameId === null) throw new Error(CAPTURE_ERROR);

      this.#runtimeEnableAttempted = true;
      await this.#command('Runtime.enable', {});

      this.#debuggerEnableAttempted = true;
      await this.#command('Debugger.enable', {});

      this.#breakpointAttempted = true;
      const breakpointResult = await this.#command('Debugger.setInstrumentationBreakpoint', {
        instrumentation: 'beforeScriptExecution',
      });
      this.#breakpointId = validProjectedIdResult(breakpointResult, 'breakpointId');
      if (this.#breakpointId === null) throw new Error(CAPTURE_ERROR);

      const start = this.#readMonotonicClock();
      if (start === null) {
        this.#setStop('evaluation_failed', true);
        throw new Error(CAPTURE_ERROR);
      }
      this.#observationStart = start;
      this.#reloadIssued = true;
      this.#deadlineTimer = setTimeout(() => {
        if (this.#state === 'running' && this.#reloadIssued) this.#setStop('deadline');
      }, OBSERVATION_MS);

      await this.#command('Page.reload', { ignoreCache: false });
    } catch {
      this.#setStop('setup_failed');
    }

    if (this.#stopReason === null) await this.#waitForStop();
    this.#state = 'cleaning';
    this.#clearDeadlineTimer();
    await this.#cleanup();
    this.#state = 'complete';
    this.#clearPrivateCorrelation();
    return this.#safeResult();
  }

  async #waitForStop() {
    while (this.#stopReason === null) {
      await new Promise((resolve) => this.#waiters.push(resolve));
      if (this.#state !== 'running') break;
      if (this.#reloadIssued) this.#checkDeadline();
    }
  }

  async #command(method, params) {
    const result = await withTimeout(() => this.#send(method, params));
    if (!result.ok) throw new Error(CAPTURE_ERROR);
    if (method !== 'Page.getFrameTree' && method !== 'Debugger.setInstrumentationBreakpoint' && !hasAcknowledged(result.value)) {
      throw new Error(CAPTURE_ERROR);
    }
    return result.value;
  }

  #readMonotonicClock() {
    try {
      const value = this.#now();
      if (typeof value !== 'number' || !Number.isFinite(value)) return null;
      if (this.#lastClock !== null && value < this.#lastClock) return null;
      this.#lastClock = value;
      return value;
    } catch {
      return null;
    }
  }

  #checkDeadline() {
    if (this.#state !== 'running' || !this.#reloadIssued || this.#observationStart === null) return;
    const now = this.#readMonotonicClock();
    if (now === null) {
      this.#setStop('evaluation_failed', true);
      return;
    }
    if (now - this.#observationStart >= OBSERVATION_MS) this.#setStop('deadline');
  }

  #observeFrameNavigation(event) {
    if (!this.#reloadIssued || this.#debuggerDisabled) return false;
    if (event.frameId !== this.#rootFrameId || !event.isHome) {
      this.#invalidateContext();
      this.#setStop('wrong_page', true, true);
      return true;
    }

    this.#navigationSeen = true;
    this.#invalidateContext();
    return true;
  }

  #observeContextCreated(event) {
    if (
      !this.#reloadIssued ||
      !this.#navigationSeen ||
      this.#debuggerDisabled ||
      event.frameId !== this.#rootFrameId
    ) {
      return false;
    }
    if (this.#currentContextId !== event.contextId) {
      this.#scripts.clear();
      this.#currentContextId = event.contextId;
      this.#contextVersion += 1;
      this.#snapshot = null;
      if (this.#stopReason === 'captured') this.#setStop('evaluation_failed', true, true);
    }
    return true;
  }

  #observeScriptParsed(event) {
    if (!this.#reloadIssued || !this.#navigationSeen || this.#debuggerDisabled ||
        event.frameId !== this.#rootFrameId || event.contextId !== this.#currentContextId) return false;
    if (!this.#scripts.has(event.scriptId) && this.#scripts.size >= MAX_SCRIPT_BINDINGS) {
      this.#setStop('script_limit');
      return false;
    }
    this.#scripts.set(event.scriptId, { contextId: event.contextId, contextVersion: this.#contextVersion });
    return true;
  }

  #invalidateContext() {
    const invalidatesPublishedCapture = this.#stopReason === 'captured';
    this.#currentContextId = null;
    this.#scripts.clear();
    this.#activeReadCallFrameId = null;
    this.#contextVersion += 1;
    this.#snapshot = null;
    if (invalidatesPublishedCapture) this.#setStop('evaluation_failed', true, true);
    this.#signal();
  }

  #enqueuePause(pause, malformed) {
    if (this.#debuggerDisabled) return;

    if (this.#pauseCount < MAX_PAUSES && !malformed) {
      this.#pauseCount += 1;
      this.#pauseQueue.push(pause);
      this.#outstandingPauses += 1;
    } else {
      this.#setStop(malformed ? 'evaluation_failed' : 'script_limit', malformed, malformed);
      if (this.#overflowResumes < MAX_OVERFLOW_RESUMES) {
        this.#overflowResumes += 1;
        this.#outstandingPauses += 1;
      }
    }
    if (this.#pauseQueue.length > MAX_QUEUED_PAUSES) {
      this.#pauseQueue.length = MAX_QUEUED_PAUSES;
      this.#setStop('script_limit', true);
    }
    this.#signal();
    this.#ensurePauseDrain();
  }

  #ensurePauseDrain() {
    if (
      this.#pauseDrainPromise !== null ||
      this.#debuggerDisabling ||
      this.#debuggerDisabled ||
      this.#breakpointRemovalInProgress ||
      this.#cleanupSuppressDrain ||
      (this.#state === 'running' && this.#stopReason !== null)
    ) {
      return;
    }
    this.#pauseDrainPromise = this.#drainPauses()
      .catch(() => {
        this.#cleanupCertain = false;
        this.#setStop('evaluation_failed', true);
      })
      .finally(() => {
        this.#pauseDrainPromise = null;
        this.#signal();
        if (this.#pauseQueue.length > 0 || this.#overflowResumes > 0) this.#ensurePauseDrain();
      });
  }

  async #drainPauses() {
    while (this.#pauseQueue.length > 0 || this.#overflowResumes > 0) {
      if (this.#debuggerDisabling || this.#debuggerDisabled) return;
      const pause = this.#pauseQueue.length > 0 ? this.#pauseQueue.shift() : null;
      if (pause === null) {
        this.#overflowResumes -= 1;
        await this.#resumeOne();
        continue;
      }

      if (
        pause.reason === 'other' &&
        this.#state === 'running' &&
        this.#stopReason === null
      ) {
        this.#setStop('unexpected_pause', true);
      }

      const binding = this.#scripts.get(pause.scriptId);
      const eligible =
        pause.reason === 'instrumentation' &&
        this.#state === 'running' &&
        this.#stopReason === null &&
        this.#navigationSeen &&
        validFrameId(pause.callFrameId) &&
        binding?.contextId === pause.contextId &&
        binding?.contextVersion === pause.contextVersion &&
        pause.contextId !== null &&
        pause.contextId === this.#currentContextId &&
        pause.contextVersion === this.#contextVersion;

      if (eligible) await this.#readDuringPause(pause.contextId, pause.contextVersion, pause.callFrameId);
      else await this.#resumeOne();

      if (this.#state === 'running' && this.#stopReason !== null) return;
      if (
        this.#pauseCount >= MAX_PAUSES &&
        this.#pauseQueue.length === 0 &&
        this.#overflowResumes === 0 &&
        this.#stopReason === null
      ) {
        this.#setStop('script_limit');
      }
    }
  }

  async #readDuringPause(contextId, contextVersion, callFrameId) {
    if (
      this.#state !== 'running' ||
      this.#stopReason !== null ||
      this.#currentContextId !== contextId ||
      this.#contextVersion !== contextVersion
    ) {
      await this.#resumeOne();
      return;
    }

    this.#activeReadContextId = contextId;
    this.#activeReadVersion = contextVersion;
    this.#activeReadCallFrameId = callFrameId;
    this.#readAttempts += 1;
    let readResult;
    try {
      readResult = await withTimeout(() => this.#readBootstrap(contextId, callFrameId));
      if (!readResult.ok) {
        this.#setStop('evaluation_failed');
      } else {
        this.#checkDeadline();
        let dto;
        try {
          dto = validateBootstrapCurrentSelf(readResult.value);
        } catch {
          this.#setStop('evaluation_failed');
        }

        if (
          dto !== undefined &&
          this.#activeReadContextId === contextId &&
          this.#activeReadVersion === contextVersion &&
          this.#activeReadCallFrameId === callFrameId &&
          this.#currentContextId === contextId &&
          this.#contextVersion === contextVersion &&
          this.#stopReason === null
        ) {
          if (dto.diagnostic === 'wrong_page') {
            this.#setStop('wrong_page', true, true);
          } else {
            if (dto.bootstrapState === 'object') this.#bootstrapSeen = true;
            if (dto.diagnostic !== 'bootstrap_missing' || this.#snapshot === null) this.#snapshot = dto;

            if (dto.bootstrapState === 'object' && dto.stableIdStringCandidates > 0) {
              this.#setStop('captured');
            }
          }
        }
      }
    } catch {
      this.#setStop('evaluation_failed');
    } finally {
      this.#activeReadContextId = null;
      this.#activeReadVersion = null;
      this.#activeReadCallFrameId = null;
      await this.#resumeOne();
    }
  }

  async #resumeOne() {
    const result = await withTimeout(() => this.#send('Debugger.resume', {}));
    if (result.ok && hasAcknowledged(result.value)) {
      if (this.#outstandingPauses > 0) this.#outstandingPauses -= 1;
      return true;
    }

    this.#cleanupCertain = false;
    if (this.#state === 'running') this.#setStop('evaluation_failed');
    return false;
  }

  #setStop(reason, clearSnapshot = false, override = false) {
    if (!STOP_REASONS.has(reason)) reason = 'evaluation_failed';
    if (clearSnapshot) this.#snapshot = null;
    if (this.#stopReason === null || override || (reason === 'wrong_page' && this.#stopReason === 'captured')) {
      this.#stopReason = reason;
    }
    this.#signal();
  }

  #signal() {
    const waiters = this.#waiters;
    this.#waiters = [];
    for (const resolve of waiters) resolve();
  }

  async #waitForPauseDrain() {
    while (this.#pauseDrainPromise !== null) await this.#pauseDrainPromise;
  }

  async #cleanup() {
    let certain = this.#cleanupCertain;
    this.#cleanupSuppressDrain = true;
    let breakpointRemoved = !this.#breakpointAttempted;
    if (this.#breakpointAttempted && this.#breakpointId !== null) {
      this.#breakpointRemovalInProgress = true;
      const removal = await withTimeout(() =>
        this.#send('Debugger.removeBreakpoint', { breakpointId: this.#breakpointId }),
      );
      this.#breakpointRemovalInProgress = false;
      breakpointRemoved = removal.ok && hasAcknowledged(removal.value);
      if (!breakpointRemoved) certain = false;
    } else if (this.#breakpointAttempted) {
      certain = false;
    }

    if (breakpointRemoved) {
      this.#cleanupSuppressDrain = false;
      this.#ensurePauseDrain();
      await this.#waitForPauseDrain();
      const resumeAttempts = Math.min(this.#outstandingPauses, MAX_CLEANUP_RESUMES);
      for (let index = 0; index < resumeAttempts; index += 1) {
        if (!(await this.#resumeOne())) certain = false;
      }

      if (this.#debuggerEnableAttempted) {
        this.#debuggerDisabling = true;
        const disabled = await withTimeout(() => this.#send('Debugger.disable', {}));
        if (disabled.ok && hasAcknowledged(disabled.value)) {
          this.#debuggerDisabled = true;
          this.#pauseQueue.length = 0;
          this.#overflowResumes = 0;
          this.#outstandingPauses = 0;
        } else {
          certain = false;
          this.#debuggerDisabling = false;
          this.#ensurePauseDrain();
          await this.#waitForPauseDrain();
        }
      }
    } else if (this.#debuggerEnableAttempted) {
      this.#debuggerDisabling = true;
      const disabled = await withTimeout(() => this.#send('Debugger.disable', {}));
      if (disabled.ok && hasAcknowledged(disabled.value)) {
        this.#debuggerDisabled = true;
        this.#pauseQueue.length = 0;
        this.#overflowResumes = 0;
        this.#outstandingPauses = 0;
      } else {
        certain = false;
        this.#debuggerDisabling = false;
        this.#cleanupSuppressDrain = false;
        this.#ensurePauseDrain();
        await this.#waitForPauseDrain();
        const resumeAttempts = Math.min(this.#outstandingPauses, MAX_CLEANUP_RESUMES);
        for (let index = 0; index < resumeAttempts; index += 1) {
          if (!(await this.#resumeOne())) certain = false;
        }
      }
    } else {
      this.#cleanupSuppressDrain = false;
      this.#ensurePauseDrain();
      await this.#waitForPauseDrain();
      const resumeAttempts = Math.min(this.#outstandingPauses, MAX_CLEANUP_RESUMES);
      for (let index = 0; index < resumeAttempts; index += 1) {
        if (!(await this.#resumeOne())) certain = false;
      }
    }

    if (this.#runtimeEnableAttempted) {
      // A composite observation can finish its bounded response while Runtime
      // context-loss events are still enabled. Always perform real cleanup.
      const drained = await withTimeout(this.#beforeRuntimeDisable);
      if (!drained.ok) certain = false;
      const disabled = await withTimeout(() => this.#send('Runtime.disable', {}));
      if (!disabled.ok || !hasAcknowledged(disabled.value)) certain = false;
    }

    if (this.#outstandingPauses > 0 || this.#pauseQueue.length > 0 || this.#overflowResumes > 0) certain = false;
    this.#cleanupCertain = certain && this.#cleanupCertain;
    this.#cleanupVerified = this.#cleanupCertain;
  }

  #clearDeadlineTimer() {
    if (this.#deadlineTimer !== null) {
      clearTimeout(this.#deadlineTimer);
      this.#deadlineTimer = null;
    }
  }

  #clearPrivateCorrelation() {
    this.#rootFrameId = null;
    this.#breakpointId = null;
    this.#currentContextId = null;
    this.#contextVersion += 1;
    this.#activeReadContextId = null;
    this.#activeReadVersion = null;
    this.#activeReadCallFrameId = null;
    this.#scripts.clear();
    this.#pauseQueue.length = 0;
    this.#overflowResumes = 0;
    this.#waiters.length = 0;
  }

  #safeResult() {
    let snapshot = null;
    if (this.#snapshot !== null) {
      try {
        snapshot = validateBootstrapCurrentSelf(this.#snapshot);
      } catch {
        snapshot = null;
        this.#stopReason = 'evaluation_failed';
      }
    }

    return {
      schema: RESULT_SCHEMA,
      stopReason: STOP_REASONS.has(this.#stopReason) ? this.#stopReason : 'evaluation_failed',
      pauseCount: Math.max(0, Math.min(MAX_PAUSES, this.#pauseCount)),
      readAttempts: Math.max(0, Math.min(MAX_PAUSES, this.#readAttempts)),
      bootstrapSeen: snapshot?.bootstrapState === 'object',
      snapshot,
      cleanupVerified: this.#cleanupVerified === true,
      canAuthorize: false,
      continuityProven: false,
    };
  }
}
