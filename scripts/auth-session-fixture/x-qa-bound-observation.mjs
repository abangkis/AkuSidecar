// One retained-QA document only. Private account/scope values never enter reports.
import { performance } from 'node:perf_hooks';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QADirectCapture } from './x-qa-direct-ab.mjs';
import { qaSourceRoute } from './x-qa-source-qualification.mjs';
import { ManualStartupObservation } from './x-manual-startup-cookie-guard.mjs';
import { IdentityFreshnessLedger } from './x-identity-freshness-ledger.mjs';
import { closedStartupTrace, rememberBoundStartup, STARTUP_GUARD_CAUSES, startupDiagnostic } from './x-manual-startup-diagnostic.mjs';

const own = (v, k) => Object.getOwnPropertyDescriptor(v, k)?.value;
const stableId = v => typeof v === 'string' && /^[1-9][0-9]{0,19}$/.test(v);
const safe = v => typeof v === 'string' && v.length > 0 && v.length <= 4096;
const requestRef = v => typeof v === 'string' && v.length > 0 && v.length <= 512;
const error = () => new Error('qa_bound_observation_invalid');
const manualV2CookiePolicy = Symbol();
// Selected by trusted Manual V2 owner code only, never by a JSON option.
export function createManualV2BoundObservation(scope, frameId, readBody, now, observeCookieWrites, pointOptions, onFailure = () => {}, provenance, freshnessOptions) {
  return new QABoundObservation(scope, frameId, readBody, now, observeCookieWrites, pointOptions, manualV2CookiePolicy, onFailure, provenance, freshnessOptions);
}

export function identityProjectionMatches(p) {
  return p?.diagnostic === 'projected' && p.bootstrapState === 'object' && p.sessionState === 'object' &&
    p.idFields.user_id === 'string' && ['absent', 'string'].includes(p.idFields.user_id_str) && p.sessionIdsAgree !== false &&
    [p.entityChecks.normalizedUsers, p.entityChecks.rootUsers].includes('match') &&
    [p.entityChecks.normalizedUsers, p.entityChecks.rootUsers].every(v => ['match', 'absent'].includes(v)) &&
    Object.values(p.authFields).every(v => ['absent', 'true'].includes(v));
}

// Manual V2 selects account evidence only from a complete pinned tuple.
// Ineligible traffic remains header-observed but cannot create or repair proof.
function manualSettingsEligible(params, frame, loader, ready, guard) {
  const request=own(params,'request');
  return ready && requestRef(own(params,'requestId')) &&
    guard.canSelectRequest(own(params,'requestId')) &&
    own(params,'frameId')===frame && own(params,'loaderId')===loader &&
    !Object.hasOwn(params,'redirectResponse') && request && typeof request==='object' && !Array.isArray(request) &&
    own(request,'method')==='GET' && qaSourceRoute(own(request,'url'))==='api_v1';
}

// This fixed extra return field is consumed only by private CDP dispatch. The
// guarded descriptor reads never search users, coerce IDs or invoke accessors.
export const QA_BOUND_BOOTSTRAP_EXPRESSION = `(() => {
  const projection = (${BOOTSTRAP_CURRENT_SELF_EXPRESSION});
  const own = (v, k) => Object.getOwnPropertyDescriptor(v, k)?.value;
  const matches = (${identityProjectionMatches.toString()});
  let accountId = null;
  try {
    if (matches(projection)) {
      const state = own(globalThis, '__INITIAL_STATE__');
      const session = own(state, 'session');
      const id = own(session, 'user_id');
      if (typeof id === 'string' && /^[1-9][0-9]{0,19}$/.test(id)) accountId = id;
    }
  } catch { accountId = null; }
  return { projection, accountId };
})()`;

export function projectBoundBootstrap(value, accept) {
  if (!value || Object.keys(value).sort().join(',') !== 'accountId,projection' || typeof accept !== 'function') throw error();
  // Inspect descriptors before any values to reject accessors, including tests.
  for (const k of ['projection', 'accountId']) if (!Object.hasOwn(Object.getOwnPropertyDescriptor(value, k) ?? {}, 'value')) throw error();
  const projection = validateBootstrapCurrentSelf(own(value, 'projection'));
  const id = own(value, 'accountId');
  if (id !== null && (!stableId(id) || !identityProjectionMatches(projection))) throw error();
  accept(id, projection);
  return projection;
}

export const QA_BOUND_QUALIFICATION = 'api_v1.cookie-auth-rejection.2026-10-20';
export class QABoundObservation {
  #scope; #now; #last; #start; #frame; #loader = null; #context = null; #id = null; #observationDeadline;
  #identityAt = null; #sourceAt = null; #projection = null; #failure = null; #events = 0; #closed = false;
  #startupCookieWriteObserved = false;
  #manualCookieGuard;
  #freshness;
  #externalCookieGuard = false;
  #firstCause = 'unknown'; #hasFirst = false; #onManualFailure; #early = 'unknown'; #handoff = 'unknown';
  #needsExtra = null; #extraStatus = null; #resolveDone;
  #waitFailure = null; #waitSettled = false; #waitReason = 'request_pending';
  #waitListeners = new Set(); #waitLast = 'request_pending'; #detachCapture;
  #observeCookieWrites;
  #pointOptions; #requestAt = null; #networkRequest = null; #networkInterval = null;
  constructor(scope, frameId, readBody, now = () => performance.now(), observeCookieWrites = () => {}, pointOptions = { deferCookieMutation: false, onPrivateResult: () => {} }, cookiePolicy, onManualFailure, provenance, freshnessOptions) {
    if (!scope || Object.keys(scope).sort().join(',') !== 'owner,profileId,sessionId,targetId' ||
        !Number.isSafeInteger(scope.owner) || scope.owner <= 0 || !['profileId', 'sessionId', 'targetId'].every(k => safe(scope[k])) ||
        !safe(frameId) || typeof now !== 'function' || typeof observeCookieWrites !== 'function' ||
        !pointOptions || !['deferCookieMutation,onPrivateResult', 'deferCookieMutation,onPrivateResult,onPrivateStartup'].includes(Object.keys(pointOptions).sort().join(',')) ||
        Object.hasOwn(pointOptions, 'onPrivateStartup') && typeof pointOptions.onPrivateStartup !== 'function' ||
        typeof pointOptions.deferCookieMutation !== 'boolean' || typeof pointOptions.onPrivateResult !== 'function' ||
        cookiePolicy !== undefined && cookiePolicy !== manualV2CookiePolicy || cookiePolicy === manualV2CookiePolicy && (pointOptions.deferCookieMutation || typeof onManualFailure !== 'function') ||
        cookiePolicy === undefined && (onManualFailure !== undefined || provenance !== undefined) || provenance !== undefined && !(provenance instanceof ManualStartupObservation)) throw error();
    this.#scope = Object.freeze({ ...scope }); this.#frame = frameId; this.#now = now;
    this.#observeCookieWrites = observeCookieWrites;
    this.#pointOptions = Object.freeze({ ...pointOptions });
    this.#manualCookieGuard = cookiePolicy === manualV2CookiePolicy ? provenance?.claim(frameId,scope.sessionId) ?? new ManualStartupObservation(frameId,scope.sessionId,{now}) : null;
    this.#externalCookieGuard = provenance !== undefined;
    this.#onManualFailure = onManualFailure;
    this.#start = this.#last = now();
    this.#observationDeadline=freshnessOptions?.observationDeadline;
    if (!Number.isFinite(this.#start)) throw error();
    if(this.#manualCookieGuard){
      this.#freshness=new IdentityFreshnessLedger({...scope,frameId,role:freshnessOptions?.role??'unknown'},this.#start,this.#manualCookieGuard,this.#observationDeadline);
      if(freshnessOptions?.cacheControl!==undefined)this.#freshness.observeCacheControl(this.#freshness.revision,this.#freshness.generation,freshnessOptions.cacheControl,this.#start);
    }
    this.capture = new QADirectCapture('a', frameId, readBody, now);
    this.done = new Promise(resolve => { this.#resolveDone = resolve; });
    this.#detachCapture = this.capture.onStateChange(() => {
      if (!this.#closed && this.capture.completion().status === 'failed' && !this.#failure) this.invalidate('source_unavailable', 'settings_capture');
      this.#notifyWait();
    });
  }
  settingsWaitState() {
    if (this.#waitFailure) return this.#waitFailure;
    if (!requestRef(this.capture.chosen)) return this.#waitReason;
    if (this.capture.status === null) return 'response_pending';
    if (!this.capture.completed) return 'completion_pending';
    if (this.#needsExtra === true && this.#extraStatus === null) return 'extra_pending';
    if (this.#needsExtra === null) return 'response_pending';
    return {pending:'body_pending',complete:'body_complete',failed:'body_failed'}[this.capture.bodyState] ?? 'metadata_complete';
  }
  onSettingsWait(listener) {
    if (typeof listener !== 'function') throw error();
    this.#waitListeners.add(listener);
    try { listener(this.settingsWaitState()); } catch {}
    return () => this.#waitListeners.delete(listener);
  }
  #notifyWait() {
    const state = this.settingsWaitState();
    if (!this.#waitSettled && (this.#waitFailure || this.capture.completed &&
        (this.#needsExtra === false || this.#needsExtra === true && this.#extraStatus !== null))) {
      this.#waitSettled = true;
      this.#resolveDone(Object.freeze({status:this.#waitFailure?'failed':'complete',reason:this.#waitFailure??'metadata_complete'}));
    }
    if (state !== this.#waitLast) {
      this.#waitLast = state;
      for (const listener of this.#waitListeners) { try { listener(state); } catch {} }
    }
  }
  #tick(reject = true) {
    const t = this.#now();
    if(this.#observationDeadline)try{this.#observationDeadline.remaining('observation_expired');}catch(error){const d=startupDiagnostic(error);if(!this.#hasFirst){this.#hasFirst=true;try{this.#onManualFailure?.(d.reason,d.trace,this.settingsWaitState());}catch{}}this.invalidate(d.reason);if(reject)throw error;return this.#last;}
    else if (!Number.isFinite(t) || t < this.#last || t - this.#start > 10000) this.invalidate('stale_observation','stale_observation');
    this.#last = t; return t;
  }
  #noteCause(reason, cause) {
    if (!this.#hasFirst) {
      this.#hasFirst = true; this.#firstCause = closedStartupTrace({ firstCause: cause }).firstCause;
      // Trusted manual callback records the originating event before peers can
      // report cancellation. Callback failure never repairs this observation.
      try { this.#onManualFailure?.(reason, { firstCause: this.#firstCause, ...this.#manualCookieGuard?.trace() }, this.settingsWaitState()); } catch {}
    }
  }
  invalidate(reason = 'scope_lost', cause = 'unknown') {
    const deferred = reason === 'observed_cookie_write' && this.#pointOptions.deferCookieMutation;
    if (!deferred) this.#waitFailure ??= this.capture.completion().status === 'failed' ? this.capture.completion().reason : 'scope_lost';
    this.#noteCause(reason,cause);
    if (!this.#failure || this.#failure === 'observed_cookie_write' && reason !== 'observed_cookie_write') this.#failure = reason;
    if(this.#freshness)try{this.#freshness.invalidate(this.#freshness.revision,this.#freshness.generation,reason,this.#last);}catch{}
    // Only the separately gated short-QA mode can retain a source observation
    // across cookie writes. The V1 binding remains rejected and non-authorizing.
    if (!deferred) {
      this.capture.invalidate();
      this.#notifyWait();
    }
  }
  earlyOutcome(stopReason, handoffRejected) {
    const trace = closedStartupTrace({ early: stopReason, handoff: handoffRejected === true ? 'rejected' : handoffRejected === false ? 'accepted' : 'unknown' });
    this.#early = trace.early; this.#handoff = trace.handoff;
    if (stopReason !== 'captured' || handoffRejected !== false) this.invalidate('scope_lost','generic_early');
  }
  armManualReload(oldLoader) {
    this.#tick();
    if(!this.#manualCookieGuard || this.#closed || this.#failure || this.#loader !== null || this.#context !== null || this.#id !== null) {
      this.invalidate('scope_lost','navigation'); throw error();
    }
    try { this.#manualCookieGuard.armReload(oldLoader); }
    catch { this.invalidate(this.#manualCookieGuard.failureReason(),this.#manualCookieGuard.rejection()); throw error(); }
  }
  acceptIdentity(id, projection) {
    const t = this.#tick();
    if (this.#closed || !this.#context || !this.#loader) { this.invalidate('scope_lost','identity_order'); return; }
    if (id === null) return;
    if (!stableId(id) || !identityProjectionMatches(projection)) { this.invalidate('scope_lost','identity_mismatch'); return; }
    if (this.#id !== null && this.#id !== id) { this.invalidate('scope_lost','identity_mismatch'); return; }
    if(this.#freshness)try{this.#freshness.observeIdentity(this.#freshness.revision,this.#freshness.generation,id,t);}catch{this.invalidate('scope_lost','identity_mismatch');return;}
    this.#id = id; this.#identityAt ??= t; this.#projection = projection;
  }
  identityReady() {
    return !this.#closed && this.#id !== null && (!this.#failure ||
      this.#failure === 'observed_cookie_write' && this.#pointOptions.deferCookieMutation);
  }
  // Called only at a verified normal startup pause, before identity evaluation.
  // These are observed pins, not requested pins or a public QA projection.
  startupPins(contextId) {
    this.#tick();
    this.checkCookieGuard();

    if (this.#closed || this.#failure || this.#id !== null || !this.#loader ||
        !this.#context || contextId !== this.#context) throw error();
    return Object.freeze({ ownerId: String(this.#scope.owner), profileId: this.#scope.profileId,
      contextId: String(this.#context), targetId: this.#scope.targetId,
      frameId: this.#frame, documentId: this.#loader });
  }
  checkCookieGuard() {
    if(this.#manualCookieGuard?.rejection() !== undefined && this.#manualCookieGuard.rejection() !== 'unknown')this.invalidate(this.#manualCookieGuard.failureReason(),this.#manualCookieGuard.rejection());
  }
  observe(raw, scope) {
    if (this.#closed) return;
    this.#tick(false);
    // Domain expiry must not escape the CDP event subscriber and poison the
    // transport needed to restore controls and retire owned resources.
    if(this.#failure==='deadline'||this.#failure==='time_invalid')return;
    if (!scope || Object.keys(this.#scope).some(k => scope[k] !== this.#scope[k])) { this.invalidate('scope_lost','owner_scope'); return; }
    const p = raw.params ?? {};
    // Manual V2's guard inspects every owned-session network header separately.
    // Generic network diagnostics cannot consume the identity/lifecycle/source
    // event budget. Ordinary and deferred QA retain their original total cap.
    if (!this.#manualCookieGuard && ++this.#events > 4096) { this.invalidate('scope_lost','event_cap'); return; }
    const ownedTargetEvent = raw.method === 'Target.detachedFromTarget' && p.sessionId === this.#scope.sessionId ||
      raw.method === 'Target.targetDestroyed' && p.targetId === this.#scope.targetId;
    if (raw.method === 'Target.detachedFromTarget' && p.sessionId === this.#scope.sessionId ||
        raw.method === 'Target.targetDestroyed' && p.targetId === this.#scope.targetId) this.invalidate('scope_lost','target_lost');
    if (raw.sessionId !== this.#scope.sessionId) return;
    if (['Inspector.detached', 'Inspector.targetCrashed'].includes(raw.method) ||
        raw.method === 'Page.frameDetached' && p.frameId === this.#frame) this.invalidate('scope_lost','target_lost');
    if (raw.method === 'Page.frameNavigated' && p.frame?.id === this.#frame) {
      if (this.#loader !== null || p.frame.url !== 'https://x.com/home' || !safe(p.frame.loaderId) || p.frame.parentId) this.invalidate('scope_lost','navigation');
      else this.#loader = p.frame.loaderId;
    }
    if (raw.method === 'Runtime.executionContextsCleared' && this.#context !== null ||
        raw.method === 'Runtime.executionContextDestroyed' && p.executionContextId === this.#context) this.invalidate('scope_lost','context_lost');
    if (raw.method === 'Runtime.executionContextCreated' && p.context?.auxData?.isDefault === true && p.context.auxData.frameId === this.#frame) {
      if (!this.#loader) return; // Runtime.enable reports the pre-reload context.
      if (this.#context !== null || p.context.origin !== 'https://x.com' || !Number.isSafeInteger(p.context.id) || p.context.id <= 0) this.invalidate('scope_lost','context_lost');
      else {
        this.#context = p.context.id;
        if(this.#freshness)try{this.#freshness.bindDocument(this.#freshness.revision,this.#freshness.generation,{documentId:this.#loader,contextId:this.#context},this.#last);}catch{this.invalidate('scope_lost','context_lost');}
      }
    }
    if(!this.#externalCookieGuard)this.#manualCookieGuard?.observe(raw);
    this.checkCookieGuard();
    const securityEvent = this.#manualCookieGuard && (ownedTargetEvent || raw.sessionId === this.#scope.sessionId &&
      (!raw.method?.startsWith('Network.') || requestRef(this.capture.chosen) && p.requestId === this.capture.chosen ||
        raw.method === 'Network.requestWillBeSent' && manualSettingsEligible(p,this.#frame,this.#loader,
          this.#loader!==null&&this.#context!==null&&this.#identityAt!==null,this.#manualCookieGuard)));
    if (securityEvent && ++this.#events > 4096) { this.invalidate('scope_lost','event_cap'); return; }

    if (!raw.method?.startsWith('Network.')) return;
    this.#observeCookieWrites(raw, this.#identityAt === null ? 'before_identity' : 'after_identity', this.#loader);
    if (!this.#loader) return; // Do not adopt old-document requests.
    // Ordinary QA starts its write baseline at the first valid identity;
    // the selected request MUST begin after it. Manual V2 instead uses its
    // post-seed startup guard above, including attempts before identity.
    // Neither policy claims worker-wide or full manual-window coverage.
    const headers = raw.method === 'Network.responseReceivedExtraInfo' ? p.headers : p.response?.headers;
    if (!this.#manualCookieGuard && headers && Object.keys(headers).some(k => k.toLowerCase() === 'set-cookie')) {
      if (this.#identityAt === null) this.#startupCookieWriteObserved = true;
      else this.invalidate('observed_cookie_write');
    }
    let bypassCandidate = Boolean(this.#manualCookieGuard) && raw.method !== 'Network.requestWillBeSent' &&
      !(requestRef(this.capture.chosen) && p.requestId === this.capture.chosen);
    if (raw.method === 'Network.requestWillBeSent' && this.#manualCookieGuard) {
      const diagnosticRequest = own(p, 'request');
      if (!requestRef(this.capture.chosen) && diagnosticRequest && typeof diagnosticRequest === 'object' &&
          own(diagnosticRequest, 'method') === 'GET' && qaSourceRoute(own(diagnosticRequest, 'url')) === 'api_v1') {
        this.#waitReason = this.#identityAt === null ? 'request_before_identity' :
          own(p, 'frameId') !== this.#frame ? 'request_frame' : own(p, 'loaderId') !== this.#loader ? 'request_document' :
          !this.#manualCookieGuard.canSelectRequest(own(p, 'requestId')) ? 'request_history' : 'request_pending';
      }
      // Once selected, every event on the exact ID stays strict. Other
      // requests cannot compete until their independent tuple qualifies.
      bypassCandidate = !(requestRef(this.capture.chosen) && p.requestId === this.capture.chosen) &&
        !manualSettingsEligible(p,this.#frame,this.#loader,
          this.#loader!==null&&this.#context!==null&&this.#identityAt!==null,this.#manualCookieGuard);
    }
    if (!bypassCandidate && raw.method === 'Network.requestWillBeSent' && p.request?.method === 'GET' && qaSourceRoute(p.request.url) === 'api_v1' &&
        (!this.#loader || !this.#context || this.#identityAt === null || p.loaderId !== this.#loader || p.frameId !== this.#frame)) this.invalidate('scope_lost','settings_order');
    if (raw.method === 'Network.responseReceived' && requestRef(this.capture.chosen) && p.requestId === this.capture.chosen &&
        (p.loaderId !== this.#loader || p.frameId !== this.#frame)) this.invalidate('scope_lost','response_frame');
    if (raw.method === 'Network.responseReceived' && requestRef(this.capture.chosen) && p.requestId === this.capture.chosen) {
      if (typeof p.hasExtraInfo !== 'boolean') this.invalidate('scope_lost','extra_shape');
      else this.#needsExtra = p.hasExtraInfo;
    }
    if (raw.method === 'Network.responseReceivedExtraInfo' && requestRef(this.capture.chosen) && p.requestId === this.capture.chosen) {
      if (this.#extraStatus !== null || !Number.isInteger(p.statusCode)) this.invalidate('scope_lost','extra_status');
      else this.#extraStatus = p.statusCode;
    }
    if (!bypassCandidate) this.capture.observe(raw.method, p);
    // Known contradictions retire the metadata waiter immediately. The final
    // account/body/cleanup checks remain mandatory before any publication.
    if (requestRef(this.capture.chosen) && p.requestId === this.capture.chosen && this.#extraStatus !== null) {
      if (this.#needsExtra === false) this.invalidate('scope_lost','extra_unexpected');
      else if (this.#needsExtra === true && this.capture.status !== null && this.#extraStatus !== this.capture.status) this.invalidate('scope_lost','extra_status');
    }
    if (raw.method === 'Network.requestWillBeSent' && requestRef(this.capture.chosen) && p.requestId === this.capture.chosen && this.#requestAt === null) {
      this.#requestAt = this.#tick(false);
      if (Number.isFinite(p.timestamp) && Number.isFinite(p.wallTime) && p.timestamp > 0 && p.wallTime > 0 &&
          Math.abs(Date.now() - p.wallTime * 1000) <= 5000) this.#networkRequest = { timestamp: p.timestamp, wallTime: p.wallTime };
    }
    if (raw.method === 'Network.loadingFinished' && requestRef(this.capture.chosen) && p.requestId === this.capture.chosen) {
      this.#sourceAt = this.#tick(false);
      const n = this.#networkRequest;
      if (n && Number.isFinite(p.timestamp) && p.timestamp > n.timestamp && p.timestamp - n.timestamp <= 5) {
        this.#networkInterval = { start: n.wallTime * 1000, end: (n.wallTime + p.timestamp - n.timestamp) * 1000 };
      }
    }
    this.#notifyWait();
  }
  #extraSummary() {
    return !this.capture.matching ? 'not_selected' : this.#needsExtra === null ? 'invalid' :
      this.#needsExtra ? this.#extraStatus === null ? 'missing' : this.#extraStatus !== this.capture.status ? 'status_mismatch' : 'complete' :
        this.#extraStatus !== null ? 'unexpected' : 'not_required';
  }
  async finish(cleanupVerified) {
    if (this.#closed) throw error();
    this.#tick();
    const extra = this.#extraSummary();
    if (['invalid','missing','status_mismatch','unexpected'].includes(extra)) this.invalidate('scope_lost',
      {invalid:'extra_shape',missing:'extra_missing',status_mismatch:'extra_status',unexpected:'extra_unexpected'}[extra]);
    // The ordinary report deliberately discards this private request reference.
    const settingsRequestId = this.capture.chosen;
    const source = await this.capture.finish();
    const cookieGuard = this.#manualCookieGuard?.finish();
    if (cookieGuard && this.#manualCookieGuard.rejection()!=='unknown') this.invalidate(this.#manualCookieGuard.failureReason(),this.#manualCookieGuard.rejection());
    const t = this.#tick();
    this.#closed = true;
    this.#detachCapture(); this.#waitListeners.clear();
    const identity = this.#id !== null && identityProjectionMatches(this.#projection);
    const login = source.diagnostic === 'http_200_unqualified' && source.status === 200 && source.matchingRequests === 1 &&
      source.responseFresh === true && source.completed && source.bodyReadAttempts === 1 && source.body?.category === 'settings_match';
    let freshness=null;
    if(this.#freshness){
      try{
        this.#freshness.observeRoot(this.#freshness.revision,this.#freshness.generation,this.#manualCookieGuard,t);
        if(this.#sourceAt!==null)this.#freshness.update(this.#freshness.revision,this.#freshness.generation,{identityAt:this.#identityAt,settingsAt:this.#sourceAt,settingsRequestId,login},t);
      }catch{if(!this.#failure)this.invalidate('scope_lost','owner_scope');}
      freshness=this.#freshness.read(t);
    }
    const fresh = freshness?freshness.withinIdentitySettingsWindow:this.#identityAt !== null && this.#sourceAt !== null &&
      Math.abs(this.#sourceAt - this.#identityAt) <= 5000 && t - Math.min(this.#identityAt, this.#sourceAt) <= 5000;
    const diagnostic = this.#failure ?? (!cleanupVerified ? 'cleanup_unverified' : !identity ? 'identity_unavailable' :
      !login ? 'source_unavailable' : !fresh ? 'stale_observation' : freshness&&!freshness.readyUnderPolicy?'scope_lost':'bound');
    if (!this.#waitSettled) {
      this.#waitFailure ??= this.capture.completion().reason;
      this.#notifyWait();
    }
    try {
      this.#pointOptions.onPrivateResult({ accountId: this.#id, start: this.#requestAt, end: this.#sourceAt, networkInterval: this.#networkInterval });
      // Only this fixed validated recipe can supply the optional private startup
      // sink. Public QA output and deferred-mutation samples cannot supply it.
      if (diagnostic === 'bound' && !this.#pointOptions.deferCookieMutation) {
        const publication=Object.freeze({
          accountId: this.#id, ownerId: String(this.#scope.owner), profileId: this.#scope.profileId,
          contextId: String(this.#context), targetId: this.#scope.targetId,
          frameId: this.#frame, documentId: this.#loader, settingsRequestId,
          identityAt: this.#identityAt, settingsAt: this.#sourceAt,
          qualification: QA_BOUND_QUALIFICATION,
        });
        this.#pointOptions.onPrivateStartup?.(this.#freshness?this.#freshness.publish(publication,t,this.#now):publication);
      }
    }
    finally { this.#id = null; this.#scope = null; this.#networkRequest = null; this.#networkInterval = null; }
    const report = Object.freeze({ schema: 'aku.x.qa-bound-observation.v1', sourceQualification: QA_BOUND_QUALIFICATION,
      diagnostic, identity: identity ? 'current_self_observed' : 'unknown', login: login ? 'protected_response_observed' : 'unknown',
      binding: diagnostic === 'bound' ? 'same_document_observed' : 'unknown', freshness: fresh ? 'within_5s' : 'unknown',
      bootstrap: this.#projection, source, cleanupVerified: cleanupVerified === true,
      startupCookieWriteObserved: this.#startupCookieWriteObserved,
      ...(cookieGuard ? { manualCookieGuard: cookieGuard } : {}),
      enrollment: 'not_verified', continuity: 'unknown', canAuthorize: false, sessionTransferred: false });
    return this.#manualCookieGuard ? rememberBoundStartup(report,{firstCause:this.#firstCause,terminal:diagnostic,
      early:this.#early,handoff:this.#handoff,cookie:['header','coverage'].includes(this.#manualCookieGuard.failureDomain())&&STARTUP_GUARD_CAUSES.includes(this.#manualCookieGuard.rejection())?this.#manualCookieGuard.rejection():cookieGuard.diagnostic,extra:this.#extraSummary(),...this.#manualCookieGuard.trace()}) : report;
  }
}
