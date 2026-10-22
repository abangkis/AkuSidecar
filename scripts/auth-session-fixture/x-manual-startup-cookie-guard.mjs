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

// Request provenance is browser metadata. JavaScript context belongs to the
// separate identity binder and never determines whether a request was seen.
export class ManualStartupCookieGuard {
  #frame; #session; #requests = new Map(); #candidate = 0; #other = 0; #ambiguous = 0;
  #events = 0; #lines = 0; #closed = false; #baseline; #claimed = false;
  #armed = false; #oldLoader; #rootId = null; #commit = null;
  #rejection = 'unknown'; #trigger = 'unknown'; #provenance = 'unknown';
  #onRejection;
  constructor(frameId, sessionId, { beforeSeed = false, onRejection = () => {} } = {}) {
    if (!ref(frameId) || typeof beforeSeed !== 'boolean' || beforeSeed && !ref(sessionId)) throw new Error('manual_cookie_guard_invalid');
    this.#frame = frameId; this.#session = sessionId; this.#baseline = !beforeSeed;
    if(typeof onRejection!=='function')throw new Error('manual_cookie_guard_invalid');
    this.#onRejection=onRejection;
  }
  beginPostSeed() {
    if (this.#closed || this.#baseline) { this.#fail('guard_request_unbound','baseline'); throw new Error('manual_cookie_guard_invalid'); }
    // Pre-seed header attempts were never counted. Structural loss is retained.
    this.#baseline = true;
  }
  claim(frameId, sessionId) {
    if (this.#closed || this.#claimed || !this.#baseline || frameId !== this.#frame || sessionId !== this.#session) throw new Error('manual_cookie_guard_invalid');
    this.#claimed = true; return this;
  }
  armReload(oldLoader) {
    if (this.#closed || this.#armed || !this.#baseline || !ref(oldLoader) || !ref(this.#session)) {
      this.#fail('guard_request_unbound','arm'); throw new Error('manual_cookie_guard_invalid');
    }
    this.#armed = true; this.#oldLoader = oldLoader;
  }
  #note(reason, trigger, provenance = 'unknown') {
    if (this.#rejection !== 'unknown') return;
    this.#rejection = reason; this.#trigger = trigger; this.#provenance = provenance;
    try{this.#onRejection(reason,this.trace());}catch{}
  }
  #fail(reason = 'guard_metadata_shape', trigger = 'metadata', provenance = 'unknown') {
    this.#note(reason,trigger,provenance); this.#ambiguous = Math.min(64,this.#ambiguous + 1);
  }
  rejection() { return this.#rejection; }
  trace() { return Object.freeze({guardTrigger:this.#trigger,guardProvenance:this.#provenance}); }
  category() { return this.#candidate ? 'candidate_observed' : this.#ambiguous ? 'ambiguous' : this.#other ? 'other_only' : 'not_observed'; }
  #attempt(headers, blocked, request, trigger) {
    if (!this.#baseline) return;
    const scope=request?.scope, provenance=request?.provenance ?? 'absent';
    const lines=[]; let malformed=false,present=false,malformedReason='guard_metadata_shape';
    if (headers !== undefined) {
      if (!object(headers) || Reflect.ownKeys(headers).length > 128 || Reflect.ownKeys(headers).some(k=>typeof k!=='string')) {
        malformed=true; if(object(headers)&&Reflect.ownKeys(headers).length>128)malformedReason='guard_header_budget';
      } else {
        const keys=Reflect.ownKeys(headers).filter(k=>k.toLowerCase()==='set-cookie');
        if(keys.length>1)malformed=true;
        for(const key of keys){
          present=true; const value=data(headers,key);
          if(typeof value!=='string'||Buffer.byteLength(value,'utf8')>16384||/\r(?!\n)/.test(value)){
            malformed=true;if(typeof value==='string'&&Buffer.byteLength(value,'utf8')>16384)malformedReason='guard_header_budget';
          }else lines.push(...value.split(/\r?\n/));
        }
      }
    }
    if(blocked!==undefined){
      if(!Array.isArray(blocked)||blocked.length>32){malformed=true;if(Array.isArray(blocked)&&blocked.length>32)malformedReason='guard_header_budget';}
      else for(let i=0;i<blocked.length;i++){
        present=true; const line=data(Object.getOwnPropertyDescriptor(blocked,String(i))?.value,'cookieLine');
        if(typeof line!=='string')malformed=true;else lines.push(line);
      }
    }
    if(!present&&!malformed)return;
    if(++this.#events>64){this.#fail('guard_event_budget',trigger,provenance);return;}
    if(lines.length>32||this.#lines+lines.length>32){this.#fail('guard_header_budget',trigger,provenance);return;}
    this.#lines+=lines.length;
    const kinds=lines.map(line=>lineKind(line,scope));
    if(kinds.includes('candidate'))this.#note('cookie_candidate',trigger,provenance);
    if(malformed||!lines.length)this.#fail(malformedReason,trigger,provenance);
    for(const kind of kinds){
      if(kind==='candidate')this.#candidate=Math.min(64,this.#candidate+1);
      else if(kind==='ambiguous')this.#fail('guard_header_classification',trigger,provenance);
      else if(kind==='unbound'||provenance!=='scoped')this.#fail('guard_request_unbound',trigger,provenance);
      else if(request.bad)this.#fail('guard_response_correlation',trigger,provenance);
      else this.#other=Math.min(64,this.#other+1);
    }
    if(request)request.cookie=true;
  }
  observe(raw) {
    if(this.#closed)return true;
    const method=data(raw,'method');
    if(typeof method!=='string'||!(method.startsWith('Network.')||method==='Page.frameNavigated'))return this.#candidate>0||this.#ambiguous>0;
    if(ref(this.#session)&&data(raw,'sessionId')!==this.#session)return this.#candidate>0||this.#ambiguous>0;
    const p=data(raw,'params');if(!object(p)){this.#fail();return true;}
    if(this.#armed&&method==='Page.frameNavigated'&&data(data(p,'frame'),'id')===this.#frame){
      const frame=data(p,'frame'),loader=data(frame,'loaderId');
      if(this.#commit!==null||!ref(loader)||loader===this.#oldLoader||data(frame,'url')!=='https://x.com/home'||data(frame,'parentId')!==undefined)this.#fail('guard_response_correlation','document','root_scope');
      else this.#commit=loader;
      const root=this.#requests.get(this.#rootId);
      if(root&&root.loaderId!==loader){root.bad=true;this.#fail('guard_response_correlation','document','root_scope');}
    }
    const id=data(p,'requestId');
    if(method==='Network.requestWillBeSent'){
      const redirect=data(p,'redirectResponse'),previous=this.#requests.get(id);
      if(redirect!==undefined)this.#attempt(data(redirect,'headers'),undefined,{scope:previous?.scope,provenance:'redirect',bad:true},'redirect');
      if(!ref(id)){this.#fail('guard_metadata_shape','request');return true;}
      if(this.#requests.size>=128&&!this.#requests.has(id)){this.#fail('guard_request_budget','request');return true;}
      const scope=setter(data(data(p,'request'),'url')),loader=data(p,'loaderId');
      const provenance=this.#requests.has(id)?'reused':redirect!==undefined?'redirect':!scope?'origin_scope':data(p,'frameId')!==this.#frame?'frame_scope':!ref(loader)?'loader_missing':'scoped';
      const r={scope,provenance,loaderId:loader,response:false,extra:false,bad:false,cookie:false,status:null,extraStatus:null,needsExtra:null,root:false};
      this.#requests.set(id,r);
      // A declared redirect still rejects ID reuse; distinguish its presence
      // without accepting a hop or replacing an earlier header rejection.
      if(provenance==='reused')this.#fail('guard_request_unbound','request',redirect!==undefined?'redirect':'reused');
      if(this.#armed&&data(p,'type')==='Document'&&data(p,'frameId')===this.#frame){
        const valid=this.#rootId===null&&provenance==='scoped'&&data(data(p,'request'),'method')==='GET'&&scope.url==='https://x.com/home'&&loader!==this.#oldLoader&&(!this.#commit||this.#commit===loader);
        if(!valid){r.provenance='root_scope';this.#fail('guard_request_unbound','request','root_scope');}
        else{r.root=true;this.#rootId=id;}
      }
    }
    if(method==='Network.responseReceived'||method==='Network.responseReceivedExtraInfo'){
      const r=this.#requests.get(id),response=data(p,'response');
      if(method==='Network.responseReceived'&&r){
        const status=data(response,'status'),extra=data(p,'hasExtraInfo');
        r.bad ||= r.response||data(p,'loaderId')!==r.loaderId||data(p,'frameId')!==this.#frame||data(response,'url')!==r.scope?.url||!Number.isInteger(status)||status<100||status>599||r.root&&status!==200||typeof extra!=='boolean';
        r.response=true;r.status=status;r.needsExtra=extra;
        if(r.extra&&(extra!==true||r.extraStatus!==status))r.bad=true;
      }else if(method==='Network.responseReceivedExtraInfo'&&r){
        const status=data(p,'statusCode');r.bad ||= r.extra||!Number.isInteger(status)||status<100||status>599;
        r.extra=true;r.extraStatus=status;if(r.response&&(r.needsExtra!==true||status!==r.status))r.bad=true;
      }
      this.#attempt(method==='Network.responseReceived'?data(response,'headers'):data(p,'headers'),method==='Network.responseReceivedExtraInfo'?data(p,'blockedCookies'):undefined,r,method==='Network.responseReceived'?'response':'extra');
      if(r?.bad&&r.cookie)this.#fail('guard_response_correlation',method==='Network.responseReceived'?'response':'extra',r.provenance);
    }
    return this.#candidate>0||this.#ambiguous>0;
  }
  finish() {
    if(this.#closed)throw new Error('manual_cookie_guard_replayed');
    this.#closed=true;
    if(!this.#baseline)this.#fail('guard_request_unbound','baseline');
    for(const r of this.#requests.values())if((r.cookie||r.root)&&(!r.response||r.bad||r.root&&this.#commit!==r.loaderId||r.needsExtra===true&&!r.extra))this.#fail(r.bad?'guard_response_correlation':'guard_completion_missing','finish',r.provenance);
    this.#requests.clear();
    return Object.freeze({schema:'aku.x.manual-startup-cookie-guard.v1',diagnostic:this.category(),candidateAttempts:this.#candidate,otherAttempts:this.#other,ambiguousAttempts:this.#ambiguous});
  }
}
