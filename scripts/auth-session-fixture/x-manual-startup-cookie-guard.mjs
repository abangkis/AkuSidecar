// Trusted Manual V2 startup only. Header attempts are not cookie-jar proof.
// Values, URLs, unknown names and attribute values never enter the summary.
import { QA_COOKIE_NAMES, QA_COOKIE_PATHS } from './x-qa-cookie-writes.mjs';

const object = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const invalid = Symbol();
const data = (v, k) => {
  const d = object(v) ? Object.getOwnPropertyDescriptor(v, k) : undefined;
  return d ? Object.hasOwn(d, 'value') ? d.value : invalid : undefined;
};
const ref = v => typeof v === 'string' && v.length > 0 && v.length <= 512;
const candidate = new Set(['auth_token', 'ct0']);
const token = /^[!#$%&'*+.^_`|~0-9a-z-]+$/i;
function setter(url) {
  if (typeof url !== 'string' || url.length > 4096) return null;
  try {
    const u = new URL(url);
    if (u.protocol !== 'https:' || !['x.com', 'api.x.com'].includes(u.hostname) || u.port || u.username || u.password || u.hash) return null;
    const i = u.pathname.lastIndexOf('/'), path = i <= 0 ? '/' : u.pathname.slice(0, i);
    return { host: u.hostname, path, url };
  } catch { return null; }
}
function lineKind(line, scope) {
  if (typeof line !== 'string' || !line || line.length > 4096 || /[\x00-\x1f\x7f]/.test(line)) return 'ambiguous';
  const parts = line.split(';');
  if (parts.length > 32) return 'ambiguous';
  const first = parts.shift().trim(), i = first.indexOf('='), name = first.slice(0, i);
  // Candidate attempts revoke even when attributes or declared scope are bad.
  if (i > 0 && candidate.has(name)) return 'candidate';
  if (i < 1 || name.length > 64 || !token.test(name) || !QA_COOKIE_NAMES.includes(name)) return 'ambiguous';
  let value = first.slice(i + 1);
  if (value.startsWith('"') && value.endsWith('"')) value = value.slice(1, -1);
  if (!/^[\x21\x23-\x2b\x2d-\x3a\x3c-\x5b\x5d-\x7e]*$/.test(value) || !scope) return 'ambiguous';
  const seen = new Set(); let path = scope.path;
  for (const part of parts) {
    const text = part.trim(), equal = text.indexOf('='), key = (equal < 0 ? text : text.slice(0, equal)).trim().toLowerCase();
    const v = equal < 0 ? null : text.slice(equal + 1).trim();
    if (!token.test(key) || seen.has(key)) return 'ambiguous';
    seen.add(key);
    if (['secure', 'httponly'].includes(key)) { if (v !== null) return 'ambiguous'; }
    else if (key === 'domain') {
      if (typeof v !== 'string' || !/^\.?[a-z0-9-]+(?:\.[a-z0-9-]+)*$/i.test(v)) return 'ambiguous';
      const domain = v.toLowerCase().replace(/^\./, '');
      if (!['x.com', 'api.x.com'].includes(domain) || !(scope.host === domain || scope.host === 'api.x.com' && domain === 'x.com')) return 'ambiguous';
    } else if (key === 'path') { path = v; }
    else if (key === 'samesite') { if (typeof v !== 'string' || !['lax', 'strict', 'none'].includes(v.toLowerCase())) return 'ambiguous'; }
    else if (key === 'max-age') { if (typeof v !== 'string' || !/^-?[0-9]{1,12}$/.test(v)) return 'ambiguous'; }
    else if (key === 'expires') {
      if (typeof v !== 'string' || !/^[A-Za-z]{3}, [0-9]{2} [A-Za-z]{3} [0-9]{4} [0-9]{2}:[0-9]{2}:[0-9]{2} GMT$/.test(v) || !Number.isFinite(Date.parse(v))) return 'ambiguous';
    } else return 'ambiguous'; // Includes unqualified partitioned/unknown attributes.
  }
  return QA_COOKIE_PATHS.includes(path) ? 'other' : 'ambiguous';
}

export class ManualStartupCookieGuard {
  #frame; #requests = new Map(); #candidate = 0; #other = 0; #ambiguous = 0; #events = 0; #lines = 0; #closed = false;
  constructor(frameId) {
    if (!ref(frameId)) throw new Error('manual_cookie_guard_invalid');
    this.#frame = frameId;
  }
  #fail() { this.#ambiguous = Math.min(64, this.#ambiguous + 1); }
  #attempt(headers, blocked, scope, correlated, request) {
    const lines = []; let malformed = false, present = false;
    if (headers !== undefined) {
      if (!object(headers) || Reflect.ownKeys(headers).length > 128 || Reflect.ownKeys(headers).some(k => typeof k !== 'string')) malformed = true;
      else {
        const keys = Reflect.ownKeys(headers).filter(k => k.toLowerCase() === 'set-cookie');
        if (keys.length > 1) malformed = true;
        for (const key of keys) {
          present = true;
          const value = data(headers, key);
          if (typeof value !== 'string' || Buffer.byteLength(value, 'utf8') > 16384 || /\r(?!\n)/.test(value)) malformed = true;
          else lines.push(...value.split(/\r?\n/));
        }
      }
    }
    if (blocked !== undefined) {
      if (!Array.isArray(blocked) || blocked.length > 32) malformed = true;
      else for (let i = 0; i < blocked.length; i++) {
        present = true;
        const item = Object.getOwnPropertyDescriptor(blocked, String(i))?.value;
        const line = data(item, 'cookieLine');
        if (typeof line !== 'string') malformed = true; else lines.push(line);
      }
    }
    if (!present && !malformed) return;
    if (++this.#events > 64 || lines.length > 32 || this.#lines + lines.length > 32) { this.#fail(); return; }
    this.#lines += lines.length;
    if (malformed || !lines.length) this.#fail();
    for (const line of lines.slice(0, 32)) {
      const kind = lineKind(line, scope);
      if (kind === 'candidate') this.#candidate = Math.min(64, this.#candidate + 1);
      else if (kind === 'ambiguous' || !correlated) this.#fail();
      else this.#other = Math.min(64, this.#other + 1);
    }
    if (request) request.cookie = true;
  }
  observe(raw, loaderId, contextId) {
    if (this.#closed) return true;
    const method = data(raw, 'method'), p = data(raw, 'params');
    if (!object(p)) { this.#fail(); return true; }
    const id = data(p, 'requestId');
    if (method === 'Network.requestWillBeSent') {
      const redirect = data(p, 'redirectResponse');
      if (redirect !== undefined) this.#attempt(data(redirect, 'headers'), undefined, null, false, null);
      if (!ref(id)) { this.#fail(); return true; }
      if (this.#requests.size >= 128) { this.#fail(); return true; }
      const scope = setter(data(data(p, 'request'), 'url'));
      const valid = redirect === undefined && !this.#requests.has(id) && scope && loaderId && contextId && data(p, 'loaderId') === loaderId && data(p, 'frameId') === this.#frame;
      if (this.#requests.get(id)?.cookie) this.#fail();
      this.#requests.set(id, valid ? { scope, loaderId, contextId, response: false, extra: false, bad: false, cookie: false, status: null, extraStatus: null, needsExtra: null } : null);
    }
    if (['Network.responseReceived', 'Network.responseReceivedExtraInfo'].includes(method)) {
      const r = this.#requests.get(id), response = data(p, 'response');
      let correlated = !!r && !!loaderId && !!contextId && r.loaderId === loaderId && r.contextId === contextId;
      if (method === 'Network.responseReceived') {
        correlated &&= data(p, 'loaderId') === loaderId && data(p, 'frameId') === this.#frame && data(response, 'url') === r.scope.url && !r.response;
        if (r) {
          const status = data(response, 'status'), extra = data(p, 'hasExtraInfo');
          r.bad ||= !correlated || !Number.isInteger(status) || status < 100 || status > 599 || typeof extra !== 'boolean';
          r.response = true; r.status = status; r.needsExtra = extra;
          if (r.extra && (extra !== true || r.extraStatus !== status)) r.bad = true;
        }
      } else if (r) {
        const status = data(p, 'statusCode');
        r.bad ||= r.extra || !Number.isInteger(status) || status < 100 || status > 599;
        r.extra = true; r.extraStatus = status;
        if (r.response && (r.needsExtra !== true || status !== r.status)) r.bad = true;
      }
      correlated &&= !r.bad;
      this.#attempt(method === 'Network.responseReceived' ? data(response, 'headers') : data(p, 'headers'),
        method === 'Network.responseReceivedExtraInfo' ? data(p, 'blockedCookies') : undefined, r?.scope, correlated, r);
      if (r?.bad && r.cookie) this.#fail();
    }
    return this.#candidate > 0 || this.#ambiguous > 0;
  }
  finish() {
    if (this.#closed) throw new Error('manual_cookie_guard_replayed');
    this.#closed = true;
    for (const r of this.#requests.values()) if (r?.cookie && (!r.response || r.bad || r.needsExtra === true && !r.extra)) this.#fail();
    this.#requests.clear();
    return Object.freeze({ schema: 'aku.x.manual-startup-cookie-guard.v1',
      diagnostic: this.#candidate ? 'candidate_observed' : this.#ambiguous ? 'ambiguous' : this.#other ? 'other_only' : 'not_observed',
      candidateAttempts: this.#candidate, otherAttempts: this.#other, ambiguousAttempts: this.#ambiguous });
  }
}
