// Project Set-Cookie names and declared scope only, before report/RPC queues.
// No cookie value, raw line, URL, unknown name or attribute value is retained.
import { qaSourceRoute } from './x-qa-source-qualification.mjs';

export const QA_COOKIE_NAMES = Object.freeze(['auth_token', 'ct0', 'twid', 'kdt', 'auth_multi', 'att', 'lang',
  'guest_id', 'guest_id_ads', 'guest_id_marketing', 'personalization_id', 'external_referer', '__cf_bm', 'cf_clearance']);
export const QA_COOKIE_PATHS = Object.freeze(['/', '/i', '/i/', '/1.1/account', '/1.1/account/', '/i/api/1.1/account', '/i/api/1.1/account/']);
const record = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const data = (v, k) => {
  if (!record(v)) return undefined;
  const d = Object.getOwnPropertyDescriptor(v, k);
  return d && Object.hasOwn(d, 'value') ? d.value : undefined;
};
const token = /^[!#$%&'*+.^_`|~0-9a-z-]+$/i;
function setterScope(url) {
  if (typeof url !== 'string' || url.length > 4096) return null;
  try {
    const u = new URL(url);
    if (u.protocol !== 'https:' || !['x.com', 'api.x.com'].includes(u.hostname) || u.port || u.username || u.password || u.hash) return null;
    // RFC default-path: omit the final path segment. Never retain the URL.
    const index = u.pathname.lastIndexOf('/');
    const path = index <= 0 ? '/' : u.pathname.slice(0, index);
    return { host: u.hostname, path: QA_COOKIE_PATHS.includes(path) ? path : 'other',
      role: qaSourceRoute(url) === 'api_v1' ? 'settings' : 'other_x' };
  } catch { return null; }
}
function projectLine(line, setter, phase, source) {
  if (line.length > 4096 || /[\u0000-\u0008\u000b-\u001f\u007f]/.test(line)) return null;
  const parts = line.split(';');
  if (parts.length > 32) return null;
  const first = parts.shift().trim(), equal = first.indexOf('=');
  if (equal < 1 || equal > 64 || !token.test(first.slice(0, equal)) || first.includes(',')) return null;
  const rawName = first.slice(0, equal);
  const name = QA_COOKIE_NAMES.includes(rawName) ? rawName : 'other';
  const attributes = new Map();
  for (const part of parts) {
    const text = part.trim(); if (!text) continue;
    const i = text.indexOf('='), key = (i < 0 ? text : text.slice(0, i)).trim().toLowerCase();
    if (!token.test(key)) return null;
    if (!['domain', 'path', 'secure', 'httponly', 'samesite', 'partitioned'].includes(key)) continue;
    if (attributes.has(key)) return null;
    const v = i < 0 ? null : text.slice(i + 1).trim();
    if (['secure', 'httponly', 'partitioned'].includes(key) && v !== null) return null;
    attributes.set(key, v);
  }
  let domain = setter?.host ?? 'unknown', hostOnly = setter ? true : null;
  if (attributes.has('domain')) {
    const v = attributes.get('domain');
    if (typeof v !== 'string' || !v || v.endsWith('.') || !/^[.a-z0-9-]+$/i.test(v)) return null;
    const normalized = v.toLowerCase().replace(/^\./, '');
    domain = ['x.com', 'api.x.com'].includes(normalized) ? normalized : 'other'; hostOnly = false;
  }
  let path = setter?.path ?? 'unknown';
  if (attributes.has('path')) {
    const v = attributes.get('path');
    if (typeof v !== 'string' || !v.startsWith('/')) return null;
    path = QA_COOKIE_PATHS.includes(v) ? v : 'other';
  }
  let sameSite = 'absent';
  if (attributes.has('samesite')) {
    const v = attributes.get('samesite');
    if (typeof v !== 'string' || !['strict', 'lax', 'none'].includes(v.toLowerCase())) return null;
    sameSite = v.toLowerCase();
  }
  return Object.freeze({ name, domain, hostOnly, path, secure: attributes.has('secure'), httpOnly: attributes.has('httponly'),
    sameSite, partitioned: attributes.has('partitioned'), phase, setter: setter?.host ?? 'unknown',
    requestRole: setter?.role ?? 'unbound', source });
}

export function projectQACookieHeaders(headers, setter, phase, source) {
  const writes = []; let unresolved = 0;
  if (!['before_identity', 'after_identity'].includes(phase) || !['response', 'extra_info'].includes(source)) throw new TypeError('qa_cookie_projection_invalid');
  if (!setter || !['x.com', 'api.x.com'].includes(setter.host) || ![...QA_COOKIE_PATHS, 'other'].includes(setter.path) ||
      !['settings', 'other_x'].includes(setter.role)) setter = null;
  if (!record(headers) || Reflect.ownKeys(headers).length > 128) return { present: true, unresolved: 1, writes };
  const keys = Reflect.ownKeys(headers).filter(k => typeof k === 'string' && k.toLowerCase() === 'set-cookie');
  for (const key of keys) {
    const value = data(headers, key);
    if (typeof value !== 'string' || value.length > 16384 || Buffer.byteLength(value, 'utf8') > 16384) { unresolved++; continue; }
    const lines = value.split(/\r?\n/);
    if (lines.length > 32) { unresolved++; continue; }
    for (const line of lines) {
      const write = projectLine(line, setter, phase, source);
      if (write) writes.push(write); else unresolved++;
      if (writes.length > 32) return { present: true, unresolved: 1, writes: [] };
    }
  }
  return { present: keys.length > 0, unresolved, writes };
}

function completeWrite(w) {
  return w.name !== 'other' && ['x.com', 'api.x.com'].includes(w.domain) && !['other', 'unknown'].includes(w.path) &&
    w.setter !== 'unknown' && w.requestRole !== 'unbound' &&
    (w.setter === w.domain || w.setter === 'api.x.com' && w.domain === 'x.com');
}
export class QACookieWriteCapture {
  #requests = new Map(); #writes = []; #headerEvents = 0; #unresolved = 0; #closed = false;
  constructor(frameId) {
    if (typeof frameId !== 'string' || !frameId || frameId.length > 256) throw new TypeError('qa_cookie_scope_invalid');
  }
  observe(raw, phase, loaderId) {
    if (this.#closed || !raw.method?.startsWith('Network.')) return;
    const p = raw.params ?? {};
    if (raw.method === 'Network.requestWillBeSent') {
      const scope = setterScope(p.request?.url);
      if (!scope) return; // Foreign assets cannot exhaust X setter correlation.
      if (typeof p.requestId !== 'string' || !p.requestId || p.requestId.length > 512) return;
      if (this.#requests.has(p.requestId)) { this.#requests.set(p.requestId, null); return; }
      // Missing/ambiguous request metadata makes an actual cookie header's
      // scope unknown. A request without Set-Cookie is not an unresolved write.
      if (this.#requests.size >= 128) return;
      // Diagnostic metadata covers already-observed requests in the owned
      // page session, including child-frame requests that can set jar cookies.
      // This does not bind those requests to the root current-self document.
      this.#requests.set(p.requestId, scope);
    }
    if (!loaderId || !['Network.responseReceived', 'Network.responseReceivedExtraInfo'].includes(raw.method)) return;
    const headers = raw.method === 'Network.responseReceived' ? p.response?.headers : p.headers;
    if (headers === undefined) return;
    const scope = this.#requests.get(p.requestId) ?? null;
    const projected = projectQACookieHeaders(headers, scope, phase, raw.method === 'Network.responseReceived' ? 'response' : 'extra_info');
    if (!projected.present) return;
    this.#headerEvents = Math.min(64, this.#headerEvents + 1);
    if (this.#headerEvents >= 64 || this.#writes.length + projected.writes.length > 32) { this.#unresolved = 64; return; }
    this.#unresolved = Math.min(64, this.#unresolved + projected.unresolved);
    this.#writes.push(...projected.writes);
  }
  finish() {
    if (this.#closed) throw new TypeError('qa_cookie_projection_replayed');
    this.#closed = true; this.#requests.clear();
    const partial = this.#unresolved > 0 || this.#writes.some(w => !completeWrite(w));
    const diagnostic = partial ? 'partial' : !this.#headerEvents ? 'not_observed' : 'projected';
    const candidate = this.#writes.some(w => ['auth_token', 'ct0'].includes(w.name));
    const qualification = partial ? 'mixed_or_unresolved' : candidate ? 'candidate_names_present' :
      this.#writes.length ? 'outside_candidate_names_only' : 'not_observed';
    return Object.freeze({ schema: 'aku.x.qa-cookie-write-projection.v1', diagnostic, headerEvents: this.#headerEvents,
      unresolved: this.#unresolved, writes: Object.freeze(this.#writes), qualification, acceptance: 'header_only' });
  }
}

export function finishQACookieWriteScope(observation, cookieWrites) {
  if (observation?.schema !== 'aku.x.qa-bound-observation.v1' || observation.canAuthorize !== false || observation.sessionTransferred !== false ||
      cookieWrites?.schema !== 'aku.x.qa-cookie-write-projection.v1') throw new TypeError('qa_cookie_scope_invalid');
  return Object.freeze({ schema: 'aku.x.qa-cookie-write-scope.v1', observation, cookieWrites, canAuthorize: false, sessionTransferred: false });
}
