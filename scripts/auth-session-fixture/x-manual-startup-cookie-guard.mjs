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
const resourceTypes = new Set(['Image','Script','Stylesheet','Font','Media','TextTrack','Manifest','XHR','Fetch','Other']);
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
  if (!/^[\x21\x23-\x2b\x2d-\x3a\x3c-\x5b\x5d-\x7e]*$/.test(value)) return 'ambiguous';
  if (!scope) return 'unbound';
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
  #session; #armed = false; #oldLoader; #rootId = null; #commit = null; #context = null; #precommit = false;
  #rejection = 'unknown';
  constructor(frameId, sessionId) {
    if (!ref(frameId)) throw new Error('manual_cookie_guard_invalid');
    this.#frame = frameId;
    this.#session = sessionId;
  }
  armReload(oldLoader) {
    if (this.#closed || this.#armed || this.#precommit || !ref(oldLoader) || !ref(this.#session)) { this.#fail('guard_request_unbound'); throw new Error('manual_cookie_guard_invalid'); }
    this.#armed = true; this.#oldLoader = oldLoader;
  }
  #bindRoot(loaderId, contextId) {
    for(const r of this.#requests.values()){
      if(!r?.provisional)continue;
      if (loaderId && loaderId !== r.loaderId || contextId && this.#context && contextId !== this.#context) r.bad = true;
      r.bound = !r.bad && this.#commit === r.loaderId && Number.isSafeInteger(this.#context) && this.#context > 0;
      if (r.bound) r.contextId = this.#context;
      if (r.bad) this.#fail('guard_response_correlation');
    }
  }
  #note(reason) { if(this.#rejection==='unknown')this.#rejection=reason; }
  #fail(reason='guard_metadata_shape') { this.#note(reason);this.#ambiguous = Math.min(64, this.#ambiguous + 1); }
  rejection() { return this.#rejection; }
  category() { return this.#candidate ? 'candidate_observed' : this.#ambiguous ? 'ambiguous' : this.#other ? 'other_only' : 'not_observed'; }
  #attempt(headers, blocked, scope, correlated, request) {
    const lines = []; let malformed = false, present = false, malformedReason='guard_metadata_shape';
    if (headers !== undefined) {
      if (!object(headers) || Reflect.ownKeys(headers).length > 128 || Reflect.ownKeys(headers).some(k => typeof k !== 'string')) {malformed = true;if(object(headers)&&Reflect.ownKeys(headers).length>128)malformedReason='guard_header_budget';}
      else {
        const keys = Reflect.ownKeys(headers).filter(k => k.toLowerCase() === 'set-cookie');
        if (keys.length > 1) malformed = true;
        for (const key of keys) {
          present = true;
          const value = data(headers, key);
          if (typeof value !== 'string' || Buffer.byteLength(value, 'utf8') > 16384 || /\r(?!\n)/.test(value)) {malformed = true;if(typeof value==='string'&&Buffer.byteLength(value,'utf8')>16384)malformedReason='guard_header_budget';}
          else lines.push(...value.split(/\r?\n/));
        }
      }
    }
    if (blocked !== undefined) {
      if (!Array.isArray(blocked) || blocked.length > 32) {malformed = true;if(Array.isArray(blocked)&&blocked.length>32)malformedReason='guard_header_budget';}
      else for (let i = 0; i < blocked.length; i++) {
        present = true;
        const item = Object.getOwnPropertyDescriptor(blocked, String(i))?.value;
        const line = data(item, 'cookieLine');
        if (typeof line !== 'string') malformed = true; else lines.push(line);
      }
    }
    if (!present && !malformed) return;
    if (++this.#events > 64) {this.#fail('guard_event_budget');return;}
    if (lines.length > 32 || this.#lines + lines.length > 32) { this.#fail('guard_header_budget'); return; }
    this.#lines += lines.length;
    const kinds=lines.slice(0,32).map(line=>lineKind(line,scope));
    if(kinds.includes('candidate'))this.#note('cookie_candidate');
    if (malformed || !lines.length) this.#fail(malformedReason);
    for (const kind of kinds) {
      if (kind === 'candidate') this.#candidate = Math.min(64, this.#candidate + 1);
      else if (kind === 'ambiguous') this.#fail('guard_header_classification');
      else if (kind === 'unbound' || !request) this.#fail('guard_request_unbound');
      else if (!correlated) this.#fail('guard_response_correlation');
      else this.#other = Math.min(64, this.#other + 1);
    }
    if (request) request.cookie = true;
  }
  observe(raw, loaderId, contextId) {
    if (this.#closed) return true;
    const method = data(raw, 'method');
    if(typeof method!=='string' || !(method.startsWith('Network.') || ['Page.frameNavigated','Runtime.executionContextCreated'].includes(method)))return this.#candidate > 0 || this.#ambiguous > 0;
    const p = data(raw, 'params');
    if (!object(p)) { this.#fail('guard_metadata_shape'); return true; }
    if (this.#armed && data(raw,'sessionId') !== this.#session) return this.#candidate > 0 || this.#ambiguous > 0;
    if (!this.#armed && method === 'Page.frameNavigated' && data(data(p,'frame'),'id') === this.#frame)this.#precommit=true;
    if (this.#armed && method === 'Page.frameNavigated' && data(data(p,'frame'),'id') === this.#frame) {
      const frame=data(p,'frame'), loader=data(frame,'loaderId');
      if (this.#commit !== null || !ref(loader) || loader === this.#oldLoader || data(frame,'url') !== 'https://x.com/home' || data(frame,'parentId') !== undefined) this.#fail('guard_response_correlation');
      else this.#commit=loader;
    }
    if (this.#armed && method === 'Runtime.executionContextCreated' && data(data(data(p,'context'),'auxData'),'isDefault') === true && data(data(data(p,'context'),'auxData'),'frameId') === this.#frame && this.#commit !== null) {
      const context=data(p,'context'),id=data(context,'id');
      if(this.#context !== null || data(context,'origin') !== 'https://x.com' || !Number.isSafeInteger(id) || id<=0)this.#fail('guard_response_correlation');
      else this.#context=id;
    }
    if(this.#armed)this.#bindRoot(loaderId,contextId);
    const id = data(p, 'requestId');
    if (method === 'Network.requestWillBeSent') {
      const redirect = data(p, 'redirectResponse');
      if (redirect !== undefined) this.#attempt(data(redirect, 'headers'), undefined, null, false, null);
      if (!ref(id)) { this.#fail('guard_metadata_shape'); return true; }
      if(this.#rootId!==null && id===this.#rootId)this.#fail('guard_request_unbound');
      if (this.#requests.size >= 128) { this.#fail('guard_request_budget'); return true; }
      const scope = setter(data(data(p, 'request'), 'url'));
      const valid = redirect === undefined && !this.#requests.has(id) && scope && loaderId && contextId && data(p, 'loaderId') === loaderId && data(p, 'frameId') === this.#frame;
      if (this.#requests.get(id)?.cookie || this.#requests.get(id)?.provisional) this.#fail('guard_request_unbound');
      const root = this.#armed && data(p,'type') === 'Document' && data(p,'frameId') === this.#frame;
      const rootProvisional = root && this.#rootId === null && !this.#requests.has(id) && redirect === undefined &&
        data(data(p,'request'),'method') === 'GET' && scope?.url === 'https://x.com/home' && ref(data(p,'loaderId')) &&
        data(p,'loaderId') !== this.#oldLoader && (!this.#commit || this.#commit === data(p,'loaderId'));
      const resourceProvisional=this.#armed && this.#commit!==null && this.#context===null && !contextId && !this.#requests.has(id) && redirect===undefined && scope &&
        data(p,'frameId')===this.#frame && data(p,'loaderId')===this.#commit && loaderId===this.#commit && data(p,'loaderId')!==this.#oldLoader &&
        data(data(p,'request'),'method')==='GET' && resourceTypes.has(data(p,'type'));
      const provisional=rootProvisional||resourceProvisional;
      if(root && !rootProvisional)this.#fail('guard_request_unbound');
      const r = valid || provisional ? { scope, loaderId:provisional?data(p,'loaderId'):loaderId, contextId,
        provisional, root:rootProvisional, bound:!provisional, response: false, extra: false, bad: false, cookie: false, status: null, extraStatus: null, needsExtra: null } : null;
      this.#requests.set(id,r);
      if(rootProvisional)this.#rootId=id;
      if(provisional)this.#bindRoot(loaderId,contextId);
    }
    if (['Network.responseReceived', 'Network.responseReceivedExtraInfo'].includes(method)) {
      const r = this.#requests.get(id), response = data(p, 'response');
      let correlated = !!r && (r.provisional ? !r.bad && (!loaderId || loaderId === r.loaderId) && (!contextId || !r.bound || contextId === r.contextId) : !!loaderId && !!contextId && r.loaderId === loaderId && r.contextId === contextId);
      if (method === 'Network.responseReceived') {
        correlated &&= data(p, 'loaderId') === r.loaderId && data(p, 'frameId') === this.#frame && data(response, 'url') === r.scope.url && !r.response;
        if (r) {
          const status = data(response, 'status'), extra = data(p, 'hasExtraInfo');
          r.bad ||= !correlated || !Number.isInteger(status) || status < 100 || status > 599 || r.root && status !== 200 || r.provisional && status>=300 && status<400 || typeof extra !== 'boolean';
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
      if (r?.bad && r.cookie) this.#fail('guard_response_correlation');
    }
    return this.#candidate > 0 || this.#ambiguous > 0;
  }
  finish() {
    if (this.#closed) throw new Error('manual_cookie_guard_replayed');
    this.#closed = true;
    for (const r of this.#requests.values()) if ((r?.cookie || r?.provisional) && (!r.response || r.bad || r.provisional && !r.bound || r.needsExtra === true && !r.extra)) this.#fail(r.bad?'guard_response_correlation':'guard_completion_missing');
    this.#requests.clear();
    return Object.freeze({ schema: 'aku.x.manual-startup-cookie-guard.v1',
      diagnostic: this.category(),
      candidateAttempts: this.#candidate, otherAttempts: this.#other, ambiguousAttempts: this.#ambiguous });
  }
}
