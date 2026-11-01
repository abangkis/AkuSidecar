// Fixed read-only browser navigations. No cookie, header or credential API.
import { performance } from 'node:perf_hooks';
import { QAGuestCapture } from './x-qa-auth-control.mjs';
import { projectQASourceBody } from './x-qa-source-qualification.mjs';

export const QA_AB_URL = 'https://api.x.com/1.1/account/settings.json';
const exact = (a, b) => JSON.stringify(a) === JSON.stringify(b);
export function qaABCommand(method, params, sessionId, state) {
  if (!state || state.closed || !['a', 'b'].includes(state.role)) return false;
  const root = sessionId === undefined;
  switch (method) {
    case 'Target.createBrowserContext': return root && state.role === 'b' && !state.contextId && exact(params, { disposeOnDetach: true });
    case 'Target.createTarget': return root && !state.targetId && (state.role === 'a' ? exact(params, { url: 'about:blank' }) :
      !!state.contextId && exact(params, { url: 'about:blank', browserContextId: state.contextId }));
    case 'Target.attachToTarget': return root && !!state.targetId && !state.sessionId && exact(params, { targetId: state.targetId, flatten: true });
    case 'Target.closeTarget': return root && state.role === 'a' && !!state.targetId && exact(params, { targetId: state.targetId });
    case 'Target.disposeBrowserContext': return root && state.role === 'b' && !!state.contextId && exact(params, { browserContextId: state.contextId });
    default: if (!state.sessionId || sessionId !== state.sessionId) return false;
  }
  switch (method) {
    case 'Page.enable': case 'Page.getFrameTree': case 'Network.disable': return exact(params, {});
    case 'Page.navigate': return !state.navigated && exact(params, { url: QA_AB_URL });
    case 'Network.enable': return exact(params, { maxTotalBufferSize: 1000000, maxResourceBufferSize: 100000, maxPostDataSize: 0 });
    case 'Network.setCacheDisabled': return exact(params, { cacheDisabled: true });
    case 'Network.getResponseBody': return Object.keys(params).join(',') === 'requestId' && state.capture?.consumeRead(params.requestId) === true;
    default: return false;
  }
}

export function projectQADirectBody(result) {
  const source = projectQASourceBody(result);
  const category = source.diagnostic !== 'projected' ? 'unreadable' : source.fields.challenge !== 'absent' ? 'challenge' :
    source.fields.error !== 'absent' || source.fields.errors !== 'absent' ? 'error_envelope' :
    source.contractMatches === true ? 'settings_match' : 'other_json';
  return Object.freeze({ schema: 'aku.x.qa-direct-body.v1', category, settingsMatch: category === 'settings_match' });
}

export class QADirectCapture extends QAGuestCapture {
  #settled = false; #listeners = new Set();
  constructor(role, frameId, readBody, now = () => performance.now()) {
    super('api_v1', frameId);
    if (!['a', 'b'].includes(role) || typeof readBody !== 'function' || typeof now !== 'function') throw new TypeError('qa_direct_scope_invalid');
    this.role = role; this.readBody = readBody; this.now = now; this.attempts = 0;
    this.consumed = false; this.body = null; this.pending = null; this.mime = false;
    this.networkFailed = false; this.bodyState = 'unread';
    this.done = new Promise(resolve => { this.resolveDone = resolve; });
  }
  completion() {
    // Network metadata only. Body projection and account authority are separate.
    return Object.freeze(this.invalid || this.closed && !this.completed
      ? {status: 'failed', reason: this.networkFailed ? 'network_failed' : 'candidate_rejected'}
      : {status: this.completed ? 'complete' : 'pending', reason: this.completed ? 'metadata_complete' : 'completion_pending'});
  }
  onStateChange(listener) {
    if (typeof listener !== 'function') throw new TypeError('qa_direct_listener_invalid');
    this.#listeners.add(listener);
    return () => this.#listeners.delete(listener);
  }
  #publish(notify = true) {
    const state = this.completion();
    if (!this.#settled && state.status !== 'pending') { this.#settled = true; this.resolveDone(state); }
    if (notify) for (const listener of this.#listeners) { try { listener(); } catch {} }
  }
  invalidate() { super.invalidate(); this.#publish(); }
  consumeRead(id) {
    const now = this.now();
    if (this.closed || this.invalid || this.consumed || this.attempts !== 1 || id !== this.chosen ||
        !Number.isFinite(now) || now < this.readStartedAt || now >= this.readDeadline) return false;
    this.consumed = true; return true;
  }
  observe(method, params) {
    if (this.closed) return;
    super.observe(method, params);
    if (!params || params.requestId !== this.chosen) return;
    if (method === 'Network.loadingFailed') this.networkFailed = true;
    if (method === 'Network.responseReceived') this.mime = ['application/json', 'application/json;charset=utf-8', 'application/json; charset=utf-8'].includes(String(params.response?.mimeType).toLowerCase());
    if (method === 'Network.loadingFinished' && !this.invalid && this.matching === 1 && this.mime && this.fresh === true &&
        this.attempts === 0 && [200, 401, 403, 429].includes(this.status) && Number.isSafeInteger(params.encodedDataLength) &&
        params.encodedDataLength > 0 && params.encodedDataLength <= 16384) {
      this.attempts = 1; this.readStartedAt = this.now(); this.readDeadline = this.readStartedAt + 4000;
      this.bodyState = 'pending';
      try {
        this.pending = Promise.resolve(this.readBody(this.chosen)).then(value => {
          if (this.consumed && value?.schema === 'aku.x.qa-direct-body.v1') { this.body = value; this.bodyState = 'complete'; }
          else this.bodyState = 'failed';
          this.#publish();
        }, () => { this.bodyState = 'failed'; this.#publish(); });
      } catch { this.bodyState = 'failed'; this.pending = Promise.resolve(); }
    }
    this.#publish();
  }
  async finish() {
    if (this.pending) await this.pending;
    const metadata = super.finish();
    this.#publish(false);
    return Object.freeze({ ...metadata, schema: 'aku.x.qa-direct-response.v1', role: this.role,
      bodyReadAttempts: this.attempts, body: !this.invalid && this.consumed ? this.body : null });
  }
}

export function finishQADirectAB(a, b, before, after, tabsDisposed, contextDisposed) {
  if (a?.schema !== 'aku.x.qa-direct-response.v1' || a.role !== 'a' || b?.schema !== a.schema || b.role !== 'b' ||
      before !== true || typeof after !== 'boolean' || tabsDisposed !== true || contextDisposed !== true) throw new TypeError('qa_direct_result_invalid');
  const positive = after && a.status === 200 && a.responseFresh === true && a.completed && a.diagnostic === 'http_200_unqualified' && a.body?.category === 'settings_match';
  const negative = b.status === 401 && b.responseFresh === true && b.completed && b.diagnostic === 'authentication_rejection_candidate' && b.body?.category === 'error_envelope';
  const outcome = !positive ? 'positive_control_failed' : negative ? 'negative_response_candidate' :
    b.status === 200 && b.body?.category === 'settings_match' ? 'guest_settings_match' : 'comparison_inconclusive';
  return Object.freeze({ schema: 'aku.x.qa-direct-ab.v1', navigation: 'direct_api_v1', a, b,
    ownerHomeBefore: before, ownerHomeAfter: after, tabsDisposed, guestContextDisposed: contextDisposed,
    outcome, authentication: 'unknown', canAuthorize: false, sessionTransferred: false });
}
