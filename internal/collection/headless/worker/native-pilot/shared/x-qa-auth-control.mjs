// Metadata-only comparison on a new empty BrowserContext. No body/cookie API.
import { qaSourceRoute } from './x-qa-source-qualification.mjs';

export const QA_GUEST_HOME = 'https://x.com/home';
const exact = (params, expected) => JSON.stringify(params) === JSON.stringify(expected);
export function qaGuestCommand(method, params, sessionId, guest) {
  if (!guest || guest.closed) return false;
  const root = sessionId === undefined;
  switch (method) {
    case 'Target.createBrowserContext': return root && !guest.contextId && exact(params, { disposeOnDetach: true });
    case 'Target.createTarget': return root && !!guest.contextId && !guest.targetId && exact(params, { url: 'about:blank', browserContextId: guest.contextId });
    case 'Target.attachToTarget': return root && !!guest.targetId && !guest.sessionId && exact(params, { targetId: guest.targetId, flatten: true });
    case 'Target.disposeBrowserContext': return root && !!guest.contextId && exact(params, { browserContextId: guest.contextId });
    default: if (!guest.sessionId || sessionId !== guest.sessionId) return false;
  }
  switch (method) {
    case 'Page.enable': case 'Page.getFrameTree': case 'Network.disable': return exact(params, {});
    case 'Page.navigate': return !guest.navigated && exact(params, { url: QA_GUEST_HOME });
    case 'Network.setCacheDisabled': return exact(params, { cacheDisabled: true });
    case 'Network.enable': return exact(params, { maxTotalBufferSize: 1000000, maxResourceBufferSize: 100000, maxPostDataSize: 0 });
    default: return false;
  }
}

export class QAGuestCapture {
  constructor(route, frameId) {
    if (route !== 'api_v1' || typeof frameId !== 'string' || !frameId || frameId.length > 256) throw new TypeError('qa_guest_scope_invalid');
    this.route = route; this.frameId = frameId; this.chosen = null; this.matching = 0;
    this.invalid = false; this.closed = false; this.status = null; this.fresh = null; this.completed = false;
  }
  invalidate() { this.invalid = true; }
  observe(method, params) {
    if (this.closed || !params) return;
    if (method === 'Network.requestWillBeSent') {
      if (this.chosen === params.requestId) { this.invalid = true; return; }
      if (qaSourceRoute(params.request?.url) !== this.route || params.request.method !== 'GET') return;
      this.matching = Math.min(2, this.matching + 1);
      if (this.chosen || params.frameId !== this.frameId || params.redirectResponse ||
          typeof params.requestId !== 'string' || !params.requestId || params.requestId.length > 512) this.invalid = true;
      this.chosen ??= params.requestId;
      return;
    }
    if (!this.chosen || params.requestId !== this.chosen) return;
    if (method === 'Network.requestServedFromCache' || method === 'Network.loadingFailed') this.invalid = true;
    if (method === 'Network.responseReceived') {
      const r = params.response;
      if (this.status !== null || !r || qaSourceRoute(r.url) !== this.route || !Number.isInteger(r.status) || r.status < 100 || r.status > 599) { this.invalid = true; return; }
      this.status = r.status;
      this.fresh = r.fromDiskCache === false && r.fromServiceWorker === false && r.fromPrefetchCache === false;
      if (!this.fresh) this.invalid = true;
    }
    if (method === 'Network.loadingFinished') {
      if (this.completed || this.status === null) this.invalid = true;
      this.completed = true;
    }
  }
  finish() {
    this.closed = true;
    const diagnostic = this.invalid ? 'candidate_rejected' : !this.matching ? 'not_observed' :
      !this.completed ? 'incomplete' : this.status === 200 ? 'http_200_unqualified' :
      this.status === 401 ? 'authentication_rejection_candidate' : 'other_response';
    this.chosen = null;
    return Object.freeze({ schema: 'aku.x.qa-guest-source.v1', route: this.route, diagnostic,
      matchingRequests: this.matching, status: this.status, responseFresh: this.fresh, completed: this.completed,
      bodyReadAttempts: 0, canAuthorize: false, sessionTransferred: false });
  }
}

export function finishQAAuthControl(signedIn, guest, before, after, disposed) {
  if (signedIn?.schema !== 'aku.x.qa-source-qualification.v3' || signedIn.cookies !== null ||
      signedIn.canAuthorize !== false || signedIn.sessionTransferred !== false || guest?.schema !== 'aku.x.qa-guest-source.v1' ||
      guest.canAuthorize !== false || guest.sessionTransferred !== false || before !== true || typeof after !== 'boolean' || disposed !== true) {
    throw new TypeError('qa_auth_control_invalid');
  }
  // A 401 alone is insufficient: this comparison intentionally reads no guest
  // body and cannot distinguish an account rejection from every anti-abuse gate.
  return Object.freeze({ schema: 'aku.x.qa-auth-control.v1', signedIn, guest,
    isolation: 'fresh_browser_context', navigation: 'home', ownerHomeBefore: before, ownerHomeAfter: after,
    contextDisposed: disposed, authentication: 'unknown', canAuthorize: false, sessionTransferred: false });
}
