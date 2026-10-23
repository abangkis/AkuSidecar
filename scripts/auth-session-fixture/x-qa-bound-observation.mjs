// One retained-QA document only. Private account/scope values never enter reports.
import { performance } from 'node:perf_hooks';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QADirectCapture } from './x-qa-direct-ab.mjs';
import { qaSourceRoute } from './x-qa-source-qualification.mjs';
import { ManualStartupCookieGuard } from './x-manual-startup-cookie-guard.mjs';
import { closedStartupTrace, rememberBoundStartup, STARTUP_GUARD_CAUSES } from './x-manual-startup-diagnostic.mjs';

const own = (v, k) => Object.getOwnPropertyDescriptor(v, k)?.value;
const stableId = v => typeof v === 'string' && /^[1-9][0-9]{0,19}$/.test(v);
const safe = v => typeof v === 'string' && v.length > 0 && v.length <= 4096;
const requestRef = v => typeof v === 'string' && v.length > 0 && v.length <= 512;
const error = () => new Error('qa_bound_observation_invalid');
const manualV2CookiePolicy = Symbol();
// Selected by trusted Manual V2 owner code only, never by a JSON option.
export function createManualV2BoundObservation(scope, frameId, readBody, now, observeCookieWrites, pointOptions, onFailure = () => {}, provenance) {
  return new QABoundObservation(scope, frameId, readBody, now, observeCookieWrites, pointOptions, manualV2CookiePolicy, onFailure, provenance);
}

export function identityProjectionMatches(p) {
  return p?.diagnostic === 'projected' && p.bootstrapState === 'object' && p.sessionState === 'object' &&
    p.idFields.user_id === 'string' && ['absent', 'string'].includes(p.idFields.user_id_str) && p.sessionIdsAgree !== false &&
    [p.entityChecks.normalizedUsers, p.entityChecks.rootUsers].includes('match') &&
    [p.entityChecks.normalizedUsers, p.entityChecks.rootUsers].every(v => ['match', 'absent'].includes(v)) &&
    Object.values(p.authFields).every(v => ['absent', 'true'].includes(v));
}

// Manual V2 can ignore a request as identity evidence only when its valid
// CDP request tuple positively identifies a separate frame. Ordinary QA keeps
// the existing fail-closed selection path.
function nonRootSettingsRequest(params, rootFrame) {
  try {
    const value = (object, key) => {
      const descriptor = Object.getOwnPropertyDescriptor(object, key);
      return descriptor && Object.hasOwn(descriptor, 'value') ? descriptor.value : undefined;
    };
    const requestId = value(params, 'requestId');
    const frameId = value(params, 'frameId');
    const loaderId = value(params, 'loaderId');
    const request = value(params, 'request');
    if (!requestRef(requestId) || !requestRef(frameId) || frameId === rootFrame || !requestRef(loaderId) ||
        !request || typeof request !== 'object' || Array.isArray(request) ||
        Object.hasOwn(params, 'redirectResponse') || value(request, 'method') !== 'GET' ||
        qaSourceRoute(value(request, 'url')) !== 'api_v1') return null;
    return Object.freeze({ requestId, frameId, loaderId });
  } catch { return null; }
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
  #scope; #now; #last; #start; #frame; #loader = null; #context = null; #id = null;
  #identityAt = null; #sourceAt = null; #projection = null; #failure = null; #events = 0; #closed = false;
  #startupCookieWriteObserved = false;
  #manualCookieGuard;
  #externalCookieGuard = false;
  #firstCause = 'unknown'; #hasFirst = false; #onManualFailure; #early = 'unknown'; #handoff = 'unknown';
  #needsExtra = null; #extraStatus = null; #resolveDone;
  #observeCookieWrites;
  #pointOptions; #requestAt = null; #networkRequest = null; #networkInterval = null;
  constructor(scope, frameId, readBody, now = () => performance.now(), observeCookieWrites = () => {}, pointOptions = { deferCookieMutation: false, onPrivateResult: () => {} }, cookiePolicy, onManualFailure, provenance) {
    if (!scope || Object.keys(scope).sort().join(',') !== 'owner,profileId,sessionId,targetId' ||
        !Number.isSafeInteger(scope.owner) || scope.owner <= 0 || !['profileId', 'sessionId', 'targetId'].every(k => safe(scope[k])) ||
        !safe(frameId) || typeof now !== 'function' || typeof observeCookieWrites !== 'function' ||
        !pointOptions || !['deferCookieMutation,onPrivateResult', 'deferCookieMutation,onPrivateResult,onPrivateStartup'].includes(Object.keys(pointOptions).sort().join(',')) ||
        Object.hasOwn(pointOptions, 'onPrivateStartup') && typeof pointOptions.onPrivateStartup !== 'function' ||
        typeof pointOptions.deferCookieMutation !== 'boolean' || typeof pointOptions.onPrivateResult !== 'function' ||
        cookiePolicy !== undefined && cookiePolicy !== manualV2CookiePolicy || cookiePolicy === manualV2CookiePolicy && (pointOptions.deferCookieMutation || typeof onManualFailure !== 'function') ||
        cookiePolicy === undefined && (onManualFailure !== undefined || provenance !== undefined) || provenance !== undefined && !(provenance instanceof ManualStartupCookieGuard)) throw error();
    this.#scope = Object.freeze({ ...scope }); this.#frame = frameId; this.#now = now;
    this.#observeCookieWrites = observeCookieWrites;
    this.#pointOptions = Object.freeze({ ...pointOptions });
    this.#manualCookieGuard = cookiePolicy === manualV2CookiePolicy ? provenance?.claim(frameId,scope.sessionId) ?? new ManualStartupCookieGuard(frameId,scope.sessionId) : null;
    this.#externalCookieGuard = provenance !== undefined;
    this.#onManualFailure = onManualFailure;
    this.#start = this.#last = now();
    if (!Number.isFinite(this.#start)) throw error();
    this.capture = new QADirectCapture('a', frameId, readBody, now);
    this.done = new Promise(resolve => { this.#resolveDone = resolve; });
  }
  #tick() {
    const t = this.#now();
    if (!Number.isFinite(t) || t < this.#last || t - this.#start > 10000) this.invalidate('stale_observation','stale_observation');
    this.#last = t; return t;
  }
  #noteCause(reason, cause) {
    if (!this.#hasFirst) {
      this.#hasFirst = true; this.#firstCause = closedStartupTrace({ firstCause: cause }).firstCause;
      // Trusted manual callback records the originating event before peers can
      // report cancellation. Callback failure never repairs this observation.
      try { this.#onManualFailure?.(reason, { firstCause: this.#firstCause, ...this.#manualCookieGuard?.trace() }); } catch {}
    }
  }
  invalidate(reason = 'scope_lost', cause = 'unknown') {
    this.#noteCause(reason,cause);
    if (!this.#failure || this.#failure === 'observed_cookie_write' && reason !== 'observed_cookie_write') this.#failure = reason;
    // Only the separately gated short-QA mode can retain a source observation
    // across cookie writes. The V1 binding remains rejected and non-authorizing.
    if (!(reason === 'observed_cookie_write' && this.#pointOptions.deferCookieMutation)) this.capture.invalidate();
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
    catch { this.invalidate('observed_cookie_write',this.#manualCookieGuard.rejection()); throw error(); }
  }
  acceptIdentity(id, projection) {
    const t = this.#tick();
    if (this.#closed || !this.#context || !this.#loader) { this.invalidate('scope_lost','identity_order'); return; }
    if (id === null) return;
    if (!stableId(id) || !identityProjectionMatches(projection)) { this.invalidate('scope_lost','identity_mismatch'); return; }
    if (this.#id !== null && this.#id !== id) { this.invalidate('scope_lost','identity_mismatch'); return; }
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
    if(this.#manualCookieGuard?.rejection() !== undefined && this.#manualCookieGuard.rejection() !== 'unknown')this.invalidate('observed_cookie_write',this.#manualCookieGuard.rejection());
  }
  observe(raw, scope) {
    if (this.#closed) return;
    this.#tick();
    if (!scope || Object.keys(this.#scope).some(k => scope[k] !== this.#scope[k])) { this.invalidate('scope_lost','owner_scope'); return; }
    const p = raw.params ?? {};
    // Manual V2's guard inspects every owned-session network header separately.
    // Generic network diagnostics cannot consume the identity/lifecycle/source
    // event budget. Ordinary and deferred QA retain their original total cap.
    const ownedTargetEvent = raw.method === 'Target.detachedFromTarget' && p.sessionId === this.#scope.sessionId ||
      raw.method === 'Target.targetDestroyed' && p.targetId === this.#scope.targetId;
    const securityEvent = !this.#manualCookieGuard || ownedTargetEvent || raw.sessionId === this.#scope.sessionId &&
      (!raw.method?.startsWith('Network.') || p.requestId === this.capture.chosen ||
        raw.method === 'Network.requestWillBeSent' && qaSourceRoute(p.request?.url) === 'api_v1');
    if (securityEvent && ++this.#events > 4096) { this.invalidate('scope_lost','event_cap'); return; }
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
      else this.#context = p.context.id;
    }
    if(!this.#externalCookieGuard)this.#manualCookieGuard?.observe(raw);
    this.checkCookieGuard();
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
    let bypassCandidate = false;
    if (raw.method === 'Network.requestWillBeSent' && this.#manualCookieGuard) {
      const candidate = nonRootSettingsRequest(p, this.#frame);
      // The current candidate ID remains on the ordinary capture path so its
      // duplicate-ID rejection runs before any nonroot exclusion.
      if (candidate && candidate.requestId !== this.capture.chosen) bypassCandidate = true;
    }
    if (!bypassCandidate && raw.method === 'Network.requestWillBeSent' && p.request?.method === 'GET' && qaSourceRoute(p.request.url) === 'api_v1' &&
        (!this.#loader || !this.#context || this.#identityAt === null || p.loaderId !== this.#loader || p.frameId !== this.#frame)) this.invalidate('scope_lost','settings_order');
    if (raw.method === 'Network.responseReceived' && p.requestId === this.capture.chosen &&
        (p.loaderId !== this.#loader || p.frameId !== this.#frame)) this.invalidate('scope_lost','response_frame');
    if (raw.method === 'Network.responseReceived' && p.requestId === this.capture.chosen) {
      if (typeof p.hasExtraInfo !== 'boolean') this.invalidate('scope_lost','extra_shape');
      else this.#needsExtra = p.hasExtraInfo;
    }
    if (raw.method === 'Network.responseReceivedExtraInfo' && p.requestId === this.capture.chosen) {
      if (this.#extraStatus !== null || !Number.isInteger(p.statusCode)) this.invalidate('scope_lost','extra_status');
      else this.#extraStatus = p.statusCode;
    }
    if (!bypassCandidate) this.capture.observe(raw.method, p);
    // Diagnose observed contradictions at event time; rejection still uses the
    // existing finish checks, so this note does not alter capture acceptance.
    if (p.requestId === this.capture.chosen && this.#extraStatus !== null) {
      if (this.#needsExtra === false) this.#noteCause('scope_lost','extra_unexpected');
      else if (this.#needsExtra === true && this.capture.status !== null && this.#extraStatus !== this.capture.status) this.#noteCause('scope_lost','extra_status');
    }
    if (raw.method === 'Network.requestWillBeSent' && p.requestId === this.capture.chosen && this.#requestAt === null) {
      this.#requestAt = this.#tick();
      if (Number.isFinite(p.timestamp) && Number.isFinite(p.wallTime) && p.timestamp > 0 && p.wallTime > 0 &&
          Math.abs(Date.now() - p.wallTime * 1000) <= 5000) this.#networkRequest = { timestamp: p.timestamp, wallTime: p.wallTime };
    }
    if (raw.method === 'Network.loadingFinished' && p.requestId === this.capture.chosen) {
      this.#sourceAt = this.#tick();
      const n = this.#networkRequest;
      if (n && Number.isFinite(p.timestamp) && p.timestamp > n.timestamp && p.timestamp - n.timestamp <= 5) {
        this.#networkInterval = { start: n.wallTime * 1000, end: (n.wallTime + p.timestamp - n.timestamp) * 1000 };
      }
    }
    if (this.capture.completed && (this.#needsExtra === false || this.#needsExtra === true && this.#extraStatus !== null)) this.#resolveDone();
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
    if (cookieGuard && (cookieGuard.candidateAttempts || cookieGuard.ambiguousAttempts)) this.invalidate('observed_cookie_write',this.#manualCookieGuard.rejection());
    const t = this.#tick();
    this.#closed = true;
    const identity = this.#id !== null && identityProjectionMatches(this.#projection);
    const fresh = this.#identityAt !== null && this.#sourceAt !== null &&
      Math.abs(this.#sourceAt - this.#identityAt) <= 5000 && t - Math.min(this.#identityAt, this.#sourceAt) <= 5000;
    const login = source.diagnostic === 'http_200_unqualified' && source.status === 200 && source.matchingRequests === 1 &&
      source.responseFresh === true && source.completed && source.bodyReadAttempts === 1 && source.body?.category === 'settings_match';
    const diagnostic = this.#failure ?? (!cleanupVerified ? 'cleanup_unverified' : !identity ? 'identity_unavailable' :
      !login ? 'source_unavailable' : !fresh ? 'stale_observation' : 'bound');
    try {
      this.#pointOptions.onPrivateResult({ accountId: this.#id, start: this.#requestAt, end: this.#sourceAt, networkInterval: this.#networkInterval });
      // Only this fixed validated recipe can supply the optional private startup
      // sink. Public QA output and deferred-mutation samples cannot supply it.
      if (diagnostic === 'bound' && !this.#pointOptions.deferCookieMutation) {
        this.#pointOptions.onPrivateStartup?.(Object.freeze({
          accountId: this.#id, ownerId: String(this.#scope.owner), profileId: this.#scope.profileId,
          contextId: String(this.#context), targetId: this.#scope.targetId,
          frameId: this.#frame, documentId: this.#loader, settingsRequestId,
          identityAt: this.#identityAt, settingsAt: this.#sourceAt,
          qualification: QA_BOUND_QUALIFICATION,
        }));
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
      early:this.#early,handoff:this.#handoff,cookie:STARTUP_GUARD_CAUSES.includes(this.#manualCookieGuard.rejection())?this.#manualCookieGuard.rejection():cookieGuard.diagnostic,extra:this.#extraSummary(),...this.#manualCookieGuard.trace()}) : report;
  }
}
