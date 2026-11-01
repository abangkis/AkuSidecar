// One-use, page-session-bound diagnostic capture. No native identity authority.
import { performance } from 'node:perf_hooks';

export const ACCOUNT_SETTINGS_CAPTURE_SCHEMA = 'aku.x.account-settings-capture.v1';
export const accountSettingsBodyMode = mode => mode === 'two_phase_account_settings' || mode === 'retained_account_settings';

function key(value) {
  return typeof value === 'string' && value.length > 0 && value.length <= 512 && !/[\u0000-\u001f\u007f]/u.test(value);
}

function route(value) {
  if (typeof value !== 'string' || value.length > 8192) return null;
  try {
    const url = new URL(value);
    if (url.username || url.password) return null;
    if (url.origin === 'https://x.com' && url.pathname === '/i/api/1.1/account/settings.json') return 'web';
    if (url.origin === 'https://api.x.com' && ['/1.1/account/settings.json', '/i/api/1.1/account/settings.json'].includes(url.pathname)) return 'api';
  } catch { /* Fixed diagnostic only; never expose the supplied URL. */ }
  return null;
}

export class AccountSettingsCapture {
  constructor({ readBody, now = () => performance.now() }) {
    if (typeof readBody !== 'function' || typeof now !== 'function') throw new TypeError('account_capture_config_invalid');
    this.readBody = readBody;
    this.now = now;
    this.chosen = null;
    this.matching = 0;
    this.attempts = 0;
    this.invalid = false;
    this.ambiguous = false;
    this.consumed = false;
    this.closed = false;
    this.pending = null;
    this.projection = null;
    this.diagnostic = 'not_observed';
    this.lastClock = null;
  }

  clock() {
    let value;
    try { value = this.now(); } catch { value = NaN; }
    if (!Number.isFinite(value) || value < 0 || this.lastClock !== null && value < this.lastClock) {
      this.invalid = true;
      return null;
    }
    this.lastClock = value;
    return value;
  }

  consumeRead(requestId) {
    const now = this.clock();
    if (this.closed || this.invalid || this.ambiguous || this.consumed || this.attempts !== 1 ||
        !this.chosen || requestId !== this.chosen.id || this.chosen.deadline === null || now === null || now >= this.chosen.deadline) return false;
    this.consumed = true;
    return true;
  }

  invalidate() {
    if (this.chosen) this.invalid = true;
  }

  observe(method, params) {
    if (this.closed || !params || typeof params !== 'object' || !key(params.requestId)) return;
    if (method === 'Network.requestWillBeSent') {
      if (this.chosen?.id === params.requestId) {
        this.ambiguous = true;
        return;
      }
      const source = route(params.request?.url);
      // CORS preflights are separate operations, not competing GET bodies.
      // Reuse of the already selected request ID remains ambiguous above.
      if (!source || params.request?.method !== 'GET') return;
      this.matching = Math.min(2, this.matching + 1);
      if (this.chosen) { this.ambiguous = true; return; }
      this.chosen = { id: params.requestId, source, response: false, deadline: null };
      if (params.redirectResponse) this.invalid = true;
      this.diagnostic = 'candidate_rejected';
      return;
    }
    if (!this.chosen || params.requestId !== this.chosen.id) return;
    if (method === 'Network.requestServedFromCache' || method === 'Network.loadingFailed') this.invalid = true;
    if (method === 'Network.responseReceived') {
      const response = params.response;
      if (this.chosen.response) this.ambiguous = true;
      this.chosen.response = true;
      if (!response || route(response.url) !== this.chosen.source || response.status !== 200 ||
          !['application/json', 'application/json;charset=utf-8', 'application/json; charset=utf-8'].includes(String(response.mimeType).toLowerCase()) ||
          response.fromDiskCache !== false || response.fromServiceWorker !== false || response.fromPrefetchCache !== false) this.invalid = true;
    }
    if (method !== 'Network.loadingFinished' || this.pending || this.attempts !== 0) return;
    if (this.invalid || this.ambiguous || !this.chosen.response || !Number.isSafeInteger(params.encodedDataLength) || params.encodedDataLength < 1 || params.encodedDataLength > 16384) return;
    const now = this.clock();
    if (now === null) return;
    this.chosen.deadline = now + 4000;
    this.attempts = 1;
    // readBody must resolve only a projected DTO from the CDP dispatch boundary.
    try {
      this.pending = Promise.resolve(this.readBody(this.chosen.id)).then(projection => {
        if (!this.consumed || !projection || projection.schema !== 'aku.x.account-settings-shape.v1' || projection.canAuthorize !== false) {
          this.diagnostic = 'read_failed';
          return;
        }
        this.projection = projection;
        this.diagnostic = projection.diagnostic === 'projected' ? 'projected' : 'body_projection_rejected';
      }, () => { this.diagnostic = 'read_failed'; });
    } catch { this.diagnostic = 'read_failed'; this.pending = Promise.resolve(); }
  }

  async finish() {
    this.closed = true;
    if (this.pending) await this.pending;
    const diagnostic = this.ambiguous ? 'ambiguous' : this.invalid ? 'candidate_rejected' : this.diagnostic;
    const projection = this.ambiguous || this.invalid ? null : this.projection;
    // Discard private correlation identifiers before producing the report.
    this.chosen = null;
    return Object.freeze({ schema: ACCOUNT_SETTINGS_CAPTURE_SCHEMA, diagnostic,
      matchingRequests: this.matching, bodyReadAttempts: this.attempts, projection, canAuthorize: false });
  }
}
