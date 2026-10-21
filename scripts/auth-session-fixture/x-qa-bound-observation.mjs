// One retained-QA document only. Private account/scope values never enter reports.
import { performance } from 'node:perf_hooks';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QADirectCapture } from './x-qa-direct-ab.mjs';
import { qaSourceRoute } from './x-qa-source-qualification.mjs';

const own = (v, k) => Object.getOwnPropertyDescriptor(v, k)?.value;
const stableId = v => typeof v === 'string' && /^[1-9][0-9]{0,19}$/.test(v);
const safe = v => typeof v === 'string' && v.length > 0 && v.length <= 4096;
const error = () => new Error('qa_bound_observation_invalid');

export function identityProjectionMatches(p) {
  return p?.diagnostic === 'projected' && p.bootstrapState === 'object' && p.sessionState === 'object' &&
    p.idFields.user_id === 'string' && ['absent', 'string'].includes(p.idFields.user_id_str) && p.sessionIdsAgree !== false &&
    [p.entityChecks.normalizedUsers, p.entityChecks.rootUsers].includes('match') &&
    [p.entityChecks.normalizedUsers, p.entityChecks.rootUsers].every(v => ['match', 'absent'].includes(v)) &&
    Object.values(p.authFields).every(v => ['absent', 'true'].includes(v));
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
  #needsExtra = null; #extraStatus = null; #resolveDone;
  #observeCookieWrites;
  #pointOptions; #requestAt = null; #networkRequest = null; #networkInterval = null;
  constructor(scope, frameId, readBody, now = () => performance.now(), observeCookieWrites = () => {}, pointOptions = { deferCookieMutation: false, onPrivateResult: () => {} }) {
    if (!scope || Object.keys(scope).sort().join(',') !== 'owner,profileId,sessionId,targetId' ||
        !Number.isSafeInteger(scope.owner) || scope.owner <= 0 || !['profileId', 'sessionId', 'targetId'].every(k => safe(scope[k])) ||
        !safe(frameId) || typeof now !== 'function' || typeof observeCookieWrites !== 'function' ||
        !pointOptions || !['deferCookieMutation,onPrivateResult', 'deferCookieMutation,onPrivateResult,onPrivateStartup'].includes(Object.keys(pointOptions).sort().join(',')) ||
        Object.hasOwn(pointOptions, 'onPrivateStartup') && typeof pointOptions.onPrivateStartup !== 'function' ||
        typeof pointOptions.deferCookieMutation !== 'boolean' || typeof pointOptions.onPrivateResult !== 'function') throw error();
    this.#scope = Object.freeze({ ...scope }); this.#frame = frameId; this.#now = now;
    this.#observeCookieWrites = observeCookieWrites;
    this.#pointOptions = Object.freeze({ ...pointOptions });
    this.#start = this.#last = now();
    if (!Number.isFinite(this.#start)) throw error();
    this.capture = new QADirectCapture('a', frameId, readBody, now);
    this.done = new Promise(resolve => { this.#resolveDone = resolve; });
  }
  #tick() {
    const t = this.#now();
    if (!Number.isFinite(t) || t < this.#last || t - this.#start > 10000) this.invalidate('stale_observation');
    this.#last = t; return t;
  }
  invalidate(reason = 'scope_lost') {
    if (!this.#failure || this.#failure === 'observed_cookie_write' && reason !== 'observed_cookie_write') this.#failure = reason;
    // Only the separately gated short-QA mode can retain a source observation
    // across cookie writes. The V1 binding remains rejected and non-authorizing.
    if (!(reason === 'observed_cookie_write' && this.#pointOptions.deferCookieMutation)) this.capture.invalidate();
  }
  acceptIdentity(id, projection) {
    const t = this.#tick();
    if (this.#closed || !this.#context || !this.#loader) { this.invalidate(); return; }
    if (id === null) return;
    if (!stableId(id) || !identityProjectionMatches(projection)) { this.invalidate(); return; }
    if (this.#id !== null && this.#id !== id) { this.invalidate(); return; }
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
    if (this.#closed || this.#failure || this.#id !== null || !this.#loader ||
        !this.#context || contextId !== this.#context) throw error();
    return Object.freeze({ ownerId: String(this.#scope.owner), profileId: this.#scope.profileId,
      contextId: String(this.#context), targetId: this.#scope.targetId,
      frameId: this.#frame, documentId: this.#loader });
  }
  observe(raw, scope) {
    if (this.#closed) return;
    this.#tick();
    if (!scope || Object.keys(this.#scope).some(k => scope[k] !== this.#scope[k])) { this.invalidate(); return; }
    if (++this.#events > 4096) { this.invalidate(); return; }
    const p = raw.params ?? {};
    if (raw.method === 'Target.detachedFromTarget' && p.sessionId === this.#scope.sessionId ||
        raw.method === 'Target.targetDestroyed' && p.targetId === this.#scope.targetId) this.invalidate();
    if (raw.sessionId !== this.#scope.sessionId) return;
    if (['Inspector.detached', 'Inspector.targetCrashed'].includes(raw.method) ||
        raw.method === 'Page.frameDetached' && p.frameId === this.#frame) this.invalidate();
    if (raw.method === 'Page.frameNavigated' && p.frame?.id === this.#frame) {
      if (this.#loader !== null || p.frame.url !== 'https://x.com/home' || !safe(p.frame.loaderId) || p.frame.parentId) this.invalidate();
      else this.#loader = p.frame.loaderId;
    }
    if (raw.method === 'Runtime.executionContextsCleared' && this.#context !== null ||
        raw.method === 'Runtime.executionContextDestroyed' && p.executionContextId === this.#context) this.invalidate();
    if (raw.method === 'Runtime.executionContextCreated' && p.context?.auxData?.isDefault === true && p.context.auxData.frameId === this.#frame) {
      if (!this.#loader) return; // Runtime.enable reports the pre-reload context.
      if (this.#context !== null || p.context.origin !== 'https://x.com' || !Number.isSafeInteger(p.context.id) || p.context.id <= 0) this.invalidate();
      else this.#context = p.context.id;
    }
    if (!raw.method?.startsWith('Network.')) return;
    this.#observeCookieWrites(raw, this.#identityAt === null ? 'before_identity' : 'after_identity', this.#loader);
    if (!this.#loader) return; // Do not adopt old-document requests.
    // The baseline begins at the first valid identity. Startup writes can
    // precede it; the selected request MUST begin after it. Every observed
    // write thereafter fences this sample. This is not worker-wide coverage.
    const headers = raw.method === 'Network.responseReceivedExtraInfo' ? p.headers : p.response?.headers;
    if (headers && Object.keys(headers).some(k => k.toLowerCase() === 'set-cookie')) {
      if (this.#identityAt === null) this.#startupCookieWriteObserved = true;
      else this.invalidate('observed_cookie_write');
    }
    if (raw.method === 'Network.requestWillBeSent' && p.request?.method === 'GET' && qaSourceRoute(p.request.url) === 'api_v1' &&
        (!this.#loader || !this.#context || this.#identityAt === null || p.loaderId !== this.#loader || p.frameId !== this.#frame)) this.invalidate();
    if (raw.method === 'Network.responseReceived' && p.requestId === this.capture.chosen &&
        (p.loaderId !== this.#loader || p.frameId !== this.#frame)) this.invalidate();
    if (raw.method === 'Network.responseReceived' && p.requestId === this.capture.chosen) {
      if (typeof p.hasExtraInfo !== 'boolean') this.invalidate();
      else this.#needsExtra = p.hasExtraInfo;
    }
    if (raw.method === 'Network.responseReceivedExtraInfo' && p.requestId === this.capture.chosen) {
      if (this.#extraStatus !== null || !Number.isInteger(p.statusCode)) this.invalidate();
      else this.#extraStatus = p.statusCode;
    }
    this.capture.observe(raw.method, p);
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
  async finish(cleanupVerified) {
    if (this.#closed) throw error();
    this.#tick();
    if (this.capture.matching && (this.#needsExtra === null || this.#needsExtra === true && this.#extraStatus !== this.capture.status ||
        this.#needsExtra === false && this.#extraStatus !== null)) this.invalidate();
    // The ordinary report deliberately discards this private request reference.
    const settingsRequestId = this.capture.chosen;
    const source = await this.capture.finish();
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
    return Object.freeze({ schema: 'aku.x.qa-bound-observation.v1', sourceQualification: QA_BOUND_QUALIFICATION,
      diagnostic, identity: identity ? 'current_self_observed' : 'unknown', login: login ? 'protected_response_observed' : 'unknown',
      binding: diagnostic === 'bound' ? 'same_document_observed' : 'unknown', freshness: fresh ? 'within_5s' : 'unknown',
      bootstrap: this.#projection, source, cleanupVerified: cleanupVerified === true,
      startupCookieWriteObserved: this.#startupCookieWriteObserved,
      enrollment: 'not_verified', continuity: 'unknown', canAuthorize: false, sessionTransferred: false });
  }
}
