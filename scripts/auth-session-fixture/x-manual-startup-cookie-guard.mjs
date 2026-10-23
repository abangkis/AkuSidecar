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
const MAX_REQUEST_RECORDS = 1280;
// Diagnostic only. Acceptance uses private request-local frame correlation.
function requestFrameProjection(params, root) {
  const d = Object.getOwnPropertyDescriptor(params, 'frameId');
  if (!d) return 'absent';
  if (!Object.hasOwn(d, 'value') || !ref(d.value)) return 'malformed';
  return d.value === root ? 'matching' : 'different';
}
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
// Retain only scope constraints for a pending ExtraInfo line, never its value
// or unknown name. Candidate and malformed evidence is decided immediately.
function lineKind(line) {
  if (typeof line !== 'string' || !line) return 'syntax';
  if (line.length > 4096) return 'capacity';
  if (/[\x00-\x1f\x7f]/.test(line)) return 'syntax';
  const parts = line.split(';');
  if (parts.length > 32) return 'capacity';
  const first = parts.shift().trim(), i = first.indexOf('='), name = first.slice(0, i);
  // Candidate attempts revoke even when attributes or declared scope are bad.
  if (i > 0 && candidate.has(name)) return 'candidate';
  if (i < 1 || name.length > 64 || !token.test(name)) return 'syntax';
  if (!QA_COOKIE_NAMES.includes(name)) return 'name_unknown';
  let value = first.slice(i + 1);
  if (value.startsWith('"') && value.endsWith('"')) value = value.slice(1, -1);
  if (!/^[\x21\x23-\x2b\x2d-\x3a\x3c-\x5b\x5d-\x7e]*$/.test(value)) return 'syntax';
  const seen = new Set(); let path = null, domain = null;
  for (const part of parts) {
    const text = part.trim(), equal = text.indexOf('='), key = (equal < 0 ? text : text.slice(0, equal)).trim().toLowerCase();
    const v = equal < 0 ? null : text.slice(equal + 1).trim();
    if (!token.test(key) || seen.has(key)) return 'attribute_invalid';
    seen.add(key);
    if (['secure', 'httponly'].includes(key)) { if (v !== null) return 'attribute_invalid'; }
    else if (key === 'domain') {
      if (typeof v !== 'string' || !/^\.?[a-z0-9-]+(?:\.[a-z0-9-]+)*$/i.test(v)) return 'attribute_invalid';
      domain = v.toLowerCase().replace(/^\./, '');
      if (!['x.com', 'api.x.com'].includes(domain)) return 'scope_domain';
    } else if (key === 'path') { path = v; }
    else if (key === 'samesite') { if (typeof v !== 'string' || !['lax', 'strict', 'none'].includes(v.toLowerCase())) return 'attribute_invalid'; }
    else if (key === 'max-age') { if (typeof v !== 'string' || !/^-?[0-9]{1,12}$/.test(v)) return 'attribute_invalid'; }
    else if (key === 'expires') {
      if (typeof v !== 'string' || !/^[A-Za-z]{3}, [0-9]{2} [A-Za-z]{3} [0-9]{4} [0-9]{2}:[0-9]{2}:[0-9]{2} GMT$/.test(v) || !Number.isFinite(Date.parse(v))) return 'attribute_invalid';
    } else return 'attribute_unsupported'; // Includes unqualified partitioned/unknown attributes.
  }
  return path !== null && !QA_COOKIE_PATHS.includes(path) ? 'scope_path' : { path, domain };
}

const redirectStatuses = new Set([301, 302, 303, 307, 308]);
const resourceTypes = new Set(['Image', 'Script', 'Stylesheet', 'Font', 'Media', 'XHR', 'Fetch', 'Other', 'Manifest', 'TextTrack']);
const statusValid = v => Number.isInteger(v) && v >= 100 && v <= 599;
const unsafeCache = response => ['fromDiskCache', 'fromServiceWorker', 'fromPrefetchCache'].some(key => {
  const value = data(response, key); return value !== undefined && value !== false;
});
const cacheFields = Object.freeze([
  ['fromDiskCache', 'guardDiskCache'],
  ['fromServiceWorker', 'guardServiceWorker'],
  ['fromPrefetchCache', 'guardPrefetchCache'],
]);
function cacheProjection(response) {
  const result = {};
  for (const [key, field] of cacheFields) {
    let descriptor;
    try { descriptor = object(response) ? Object.getOwnPropertyDescriptor(response, key) : undefined; }
    catch { descriptor = invalid; }
    result[field] = descriptor === undefined ? 'absent' : descriptor === invalid || !Object.hasOwn(descriptor, 'value') ? 'malformed' :
      descriptor.value === true ? 'true' : descriptor.value === false ? 'false' : 'malformed';
  }
  return Object.freeze(result);
}
const blockedShape = value => Array.isArray(value) && value.length <= 32 && Array.from({length:value.length},(_,i)=>
  data(Object.getOwnPropertyDescriptor(value,String(i))?.value,'cookieLine')).every(line=>typeof line==='string');
function settingsScope(scope) {
  if (!scope) return false;
  const path = new URL(scope.url).pathname;
  return path === '/1.1/account/settings.json' || path === '/i/api/1.1/account/settings.json';
}

// Request provenance is browser metadata. JavaScript context belongs to the
// separate identity binder and never determines whether a request was seen.
export class ManualStartupCookieGuard {
  #frame; #session; #requests = new Map(); #candidate = 0; #other = 0; #ambiguous = 0;
  #events = 0; #lines = 0; #closed = false; #baseline; #claimed = false;
  #armed = false; #oldLoader; #rootId = null; #root = null; #commit = null; #records = 0;
  #rejection = 'unknown'; #trigger = 'unknown'; #provenance = 'unknown';
  #condition = 'unknown'; #epoch = 'unknown';
  #guardDiskCache = 'unknown'; #guardServiceWorker = 'unknown'; #guardPrefetchCache = 'unknown';
  #guardRequestFrame = 'unknown';
  #guardHeaderReason = 'unknown';
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
  #note(reason, trigger, provenance = 'unknown', condition = 'unknown', cache = null, frame = 'unknown', headerReason = 'unknown') {
    if (this.#rejection !== 'unknown') return;
    this.#rejection = reason; this.#trigger = trigger; this.#provenance = provenance;
    this.#condition = condition;
    this.#guardRequestFrame = frame;
    this.#guardHeaderReason = headerReason;
    if (cache) {
      this.#guardDiskCache = cache.guardDiskCache;
      this.#guardServiceWorker = cache.guardServiceWorker;
      this.#guardPrefetchCache = cache.guardPrefetchCache;
    }
    // Capture the observation epoch once, not the later reporting phase.
    this.#epoch = !this.#baseline ? 'before_seed' : this.#commit !== null ? 'document_committed' : this.#armed ? 'reload_armed' : 'post_seed';
    try{this.#onRejection(reason,this.trace());}catch{}
  }
  #fail(reason = 'guard_metadata_shape', trigger = 'metadata', provenance = 'unknown', condition = 'unknown', cache = null, frame = 'unknown', headerReason = 'unknown') {
    this.#note(reason,trigger,provenance,condition,cache,frame,headerReason); this.#ambiguous = Math.min(64,this.#ambiguous + 1);
  }
  rejection() { return this.#rejection; }
  trace() { return Object.freeze({guardTrigger:this.#trigger,guardProvenance:this.#provenance,guardCondition:this.#condition,guardEpoch:this.#epoch,
    guardDiskCache:this.#guardDiskCache,guardServiceWorker:this.#guardServiceWorker,guardPrefetchCache:this.#guardPrefetchCache,guardRequestFrame:this.#guardRequestFrame,guardHeaderReason:this.#guardHeaderReason}); }
  category() { return this.#candidate ? 'candidate_observed' : this.#ambiguous ? 'ambiguous' : this.#other ? 'other_only' : 'not_observed'; }
  #attempt(headers, blocked, request, trigger, provenance = request?.provenance ?? 'absent') {
    if (!this.#baseline) return null;
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
    if(!present&&!malformed)return null;
    if(request)request.cookie=true;
    const frame=request?.frameStatus??'unknown';
    if(++this.#events>64){this.#fail('guard_event_budget',trigger,provenance,'unknown',null,frame,'capacity');return null;}
    if(lines.length>32||this.#lines+lines.length>32){this.#fail('guard_header_budget',trigger,provenance,'unknown',null,frame,'capacity');return null;}
    this.#lines+=lines.length;
    const kinds=lines.map(line=>lineKind(line));
    if(kinds.includes('candidate'))this.#note('cookie_candidate',trigger,provenance,'unknown',null,frame);
    if(malformed||!lines.length)this.#fail(malformedReason,trigger,provenance,'unknown',null,frame,malformedReason==='guard_header_budget'?'capacity':'metadata_shape');
    const other=[];
    for(const kind of kinds){
      if(kind==='candidate')this.#candidate=Math.min(64,this.#candidate+1);
      else if(typeof kind==='string')this.#fail('guard_header_classification',trigger,provenance,'unknown',null,frame,kind);
      else other.push(kind);
    }
    return other;
  }
  #apply(other, request, trigger, provenance = request?.provenance ?? 'absent', condition = 'unknown', cache = null) {
    if(!other)return;
    for(const line of other){
      const scope=request?.scope;
      const frame=request?.frameStatus??'unknown';
      if(!scope||request.provenance!=='scoped')this.#fail('guard_request_unbound',trigger,provenance,condition,cache,frame);
      else if(line.domain!==null&&!(scope.host===line.domain||scope.host==='api.x.com'&&line.domain==='x.com'))this.#fail('guard_header_classification',trigger,provenance,condition,cache,frame,'scope_domain');
      else if(!QA_COOKIE_PATHS.includes(line.path??scope.path))this.#fail('guard_header_classification',trigger,provenance,condition,cache,frame,'scope_path');
      else if(request.bad)this.#fail('guard_response_correlation',trigger,provenance,condition,cache,frame);
      else this.#other=Math.min(64,this.#other+1);
    }
  }
  #reserve(trigger, provenance, frame = 'unknown') {
    // Hops and queued ExtraInfo projections share the lifetime record budget.
    // Reusing a request ID never replenishes it, including across the seed.
    if(this.#records>=MAX_REQUEST_RECORDS){this.#fail('guard_request_budget',trigger,provenance,'unknown',null,frame);return false;}
    this.#records++;return true;
  }
  #chainFailure(chain, trigger, provenance) {
    if(chain?.redirected||chain?.hops.some(r=>r.cookie||r.root))this.#fail('guard_response_correlation',trigger,provenance,'unknown',null,chain.hops.at(-1)?.frameStatus??'unknown');
  }
  #complete(chain) {
    const relevant=chain.redirected||chain.hops.some(r=>r.cookie||r.root);
    if(!relevant)return;
    const last=chain.hops.at(-1);
    for(const r of chain.hops){
      const cacheFailure=chain.redirected&&!chain.cache&&!chain.failed&&!r.bad&&!chain.bad&&r.cache;
      if(r.bad||chain.bad||chain.redirected&&(chain.cache||chain.failed||r.cache||r.provenance!=='scoped'||!r.resource||r.response&&r===last&&(r.status<200||r.status>=300&&r.status<400))){
        this.#fail('guard_response_correlation','finish',r.provenance,'unknown',cacheFailure?r.cacheProjection:null,r.frameStatus);return;
      }
      if(!r.response||r.root&&this.#commit!==r.loaderId){this.#fail('guard_completion_missing','finish',r.provenance,'unknown',null,r.frameStatus);return;}
    }
    const required=chain.hops.filter(r=>r.needsExtra===true);
    if(required.length!==chain.extras.length){
      this.#fail(chain.extras.length>required.length?'guard_response_correlation':'guard_completion_missing','finish',last.provenance,'unknown',null,last.frameStatus);return;
    }
    // CDP permits ExtraInfo before or after the normal response, but does not
    // promise a cross-hop order. Admit only a unique status-consistent pairing
    // after every hop has closed; repeated status slots remain ambiguous.
    const matched=new Set();
    for(const extra of chain.extras){
      const choices=required.filter(r=>r.status===extra.status);
      if(chain.redirected&&(!extra.headersShape||!extra.blockedShape)){
        this.#fail('guard_metadata_shape','finish',last.provenance,'unknown',null,last.frameStatus);return;
      }
      if(extra.bad||choices.length!==1||matched.has(choices[0])){
        this.#fail('guard_response_correlation','finish',last.provenance,'unknown',null,last.frameStatus);return;
      }
      matched.add(choices[0]);
    }
    for(const r of chain.hops)this.#apply(r.other,r,r===last?'response':'redirect');
    for(const extra of chain.extras)this.#apply(extra.other,required.find(r=>r.status===extra.status),'extra');
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
      const root=this.#root;
      if(root&&root.loaderId!==loader){root.bad=true;this.#fail('guard_response_correlation','document','root_scope','unknown',null,root.frameStatus);}
    }
    const id=data(p,'requestId');
    if(method==='Network.requestWillBeSent'){
      const redirect=data(p,'redirectResponse'),chain=this.#requests.get(id),previous=chain?.hops.at(-1);
      const other=redirect!==undefined?this.#attempt(data(redirect,'headers'),undefined,previous,'redirect','redirect'):null;
      if(!ref(id)){this.#fail('guard_metadata_shape','request');return true;}
      const frameStatus=requestFrameProjection(p,this.#frame);
      if(!this.#reserve('request',redirect!==undefined?'redirect':chain?'reused':'unknown',frameStatus))return true;
      const scope=setter(data(data(p,'request'),'url')),loader=data(p,'loaderId'),frame=data(p,'frameId');
      // Cookie provenance is local to the owned CDP session. A valid nonroot
      // request cannot supply account evidence, but its own response can be
      // correlated without inventing a relationship to the identity root.
      const frameScoped=ref(frame)&&(frame===this.#frame||ref(this.#session));
      const provenance=!scope?'origin_scope':!frameScoped?'frame_scope':!ref(loader)?'loader_missing':'scoped';
      const r={scope,provenance,frameStatus,frameId:ref(frame)?frame:null,loaderId:ref(loader)?loader:null,response:false,bad:false,cookie:false,status:null,needsExtra:null,root:false,cache:false,cacheProjection:null,other:null,
        resource:resourceTypes.has(data(p,'type'))&&data(data(p,'request'),'method')==='GET'&&!settingsScope(scope)};
      const current=chain??{hops:[],extras:[],redirected:false,bad:false,cache:false,failed:false,completed:false};
      if(chain&&redirect===undefined){r.provenance='reused';current.bad=true;this.#fail('guard_request_unbound','request','reused','unknown',null,frameStatus);}
      if(redirect!==undefined){
        current.redirected=true;
        if(!previous){r.provenance='redirect';current.bad=true;this.#apply(other,{provenance:'redirect',frameStatus},'redirect','redirect','predecessor_missing');this.#fail('guard_request_unbound','request','redirect','predecessor_missing',null,frameStatus);}
        else{
          const status=data(redirect,'status'),extra=data(p,'redirectHasExtraInfo');
          const valid=object(redirect)&&!previous.response&&!current.completed&&!current.failed&&!current.cache&&previous.resource&&r.resource&&
            previous.provenance==='scoped'&&provenance==='scoped'&&previous.frameId===frame&&previous.loaderId===loader&&data(redirect,'url')===previous.scope?.url&&redirectStatuses.has(status)&&typeof extra==='boolean'&&object(data(redirect,'headers'))&&!unsafeCache(redirect);
          if(!valid){
            // Report the first failed term using closed categories only.
            const condition=!object(redirect)?'redirect_shape':previous.response?'previous_response':current.completed?'chain_completed':current.failed?'chain_failed':current.cache?'chain_cache':
              !previous.resource?'previous_resource':!r.resource?'current_resource':
              previous.provenance==='origin_scope'?'previous_origin':previous.provenance==='frame_scope'?'previous_frame':previous.provenance!=='scoped'?'previous_loader':
              provenance==='origin_scope'?'current_origin':provenance==='frame_scope'?'current_frame':provenance!=='scoped'?'current_loader':
              previous.frameId!==frame?'frame_mismatch':previous.loaderId!==loader?'loader_mismatch':data(redirect,'url')!==previous.scope?.url?'redirect_url':!redirectStatuses.has(status)?'redirect_status':
              typeof extra!=='boolean'?'extra_flag':!object(data(redirect,'headers'))?'headers_shape':'response_cache';
            const cache=condition==='response_cache'?cacheProjection(redirect):null;
            // The predicate identifies which request frame failed. Header
            // attempts above retain their own predecessor snapshot instead.
            const failedFrame=condition==='previous_frame'?previous.frameStatus:frameStatus;
            previous.bad=true;current.bad=true;this.#apply(other,{provenance:'redirect',frameStatus:failedFrame},'redirect','redirect',condition,cache);this.#fail('guard_request_unbound','request','redirect',condition,cache,failedFrame);
          }
          else{previous.response=true;previous.status=status;previous.needsExtra=extra;previous.other=other;}
        }
      }
      current.hops.push(r);this.#requests.set(id,current);
      if(this.#armed&&data(p,'type')==='Document'&&data(p,'frameId')===this.#frame){
        const valid=this.#rootId===null&&!chain&&redirect===undefined&&provenance==='scoped'&&data(data(p,'request'),'method')==='GET'&&scope.url==='https://x.com/home'&&loader!==this.#oldLoader&&(!this.#commit||this.#commit===loader);
        if(!valid){r.provenance='root_scope';this.#fail('guard_request_unbound','request','root_scope','unknown',null,frameStatus);}
        else{r.root=true;this.#rootId=id;this.#root=r;}
      }
    }
    if(method==='Network.responseReceived'||method==='Network.responseReceivedExtraInfo'){
      const chain=this.#requests.get(id),r=chain?.hops.at(-1),response=data(p,'response');
      // Parse attempts before correlation failures so candidates/unknown syntax
      // are observed even when no hop can ever receive this metadata.
      const other=this.#attempt(method==='Network.responseReceived'?data(response,'headers'):data(p,'headers'),method==='Network.responseReceivedExtraInfo'?data(p,'blockedCookies'):undefined,r,method==='Network.responseReceived'?'response':'extra');
      if(method==='Network.responseReceived'&&r){
        const status=data(response,'status'),extra=data(p,'hasExtraInfo');
        r.bad ||= r.response||data(p,'loaderId')!==r.loaderId||data(p,'frameId')!==r.frameId||data(response,'url')!==r.scope?.url||!statusValid(status)||r.root&&status!==200||typeof extra!=='boolean';
        if(!r.cache&&unsafeCache(response)){r.cache=true;r.cacheProjection=cacheProjection(response);}
        if(chain.redirected&&!object(data(response,'headers'))){r.bad=true;this.#fail('guard_metadata_shape','response',r.provenance,'unknown',null,r.frameStatus);}
        r.response=true;r.status=statusValid(status)?status:null;r.needsExtra=typeof extra==='boolean'?extra:null;
        // A single hop can be checked immediately; a redirect chain waits for
        // all declarations, because a late ExtraInfo may belong to any hop.
        if(!chain.redirected&&chain.extras.length&&(chain.extras.length!==1||extra!==true||chain.extras[0].status!==status||chain.extras[0].bad))r.bad=true;
        if(chain.redirected)r.other=other;else this.#apply(other,r,'response');
      }else if(method==='Network.responseReceivedExtraInfo'&&r){
        const status=data(p,'statusCode');
        if(this.#reserve('extra',r.provenance,r.frameStatus))chain.extras.push({status:statusValid(status)?status:null,bad:!statusValid(status),other,headersShape:object(data(p,'headers')),blockedShape:blockedShape(data(p,'blockedCookies'))});
        if(!chain.redirected&&(chain.extras.length>1||!statusValid(status)||r.response&&(r.needsExtra!==true||status!==r.status)))r.bad=true;
        if(r.provenance!=='scoped')this.#apply(other,r,'extra');
      }else this.#apply(other,r,method==='Network.responseReceived'?'response':'extra');
      if(r?.bad)this.#chainFailure(chain,method==='Network.responseReceived'?'response':'extra',r.provenance);
    }
    if(['Network.requestServedFromCache','Network.loadingFinished','Network.loadingFailed'].includes(method)){
      const chain=this.#requests.get(id);
      if(chain){
        if(method==='Network.requestServedFromCache')chain.cache=true;
        else if(method==='Network.loadingFailed')chain.failed=true;
        else{if(chain.completed||chain.redirected&&!chain.hops.at(-1).response)chain.bad=true;chain.completed=true;}
        if(chain.redirected&&(chain.cache||chain.failed||chain.bad))this.#chainFailure(chain,'response',chain.hops.at(-1).provenance);
      }
    }
    return this.#candidate>0||this.#ambiguous>0;
  }
  finish() {
    if(this.#closed)throw new Error('manual_cookie_guard_replayed');
    this.#closed=true;
    if(!this.#baseline)this.#fail('guard_request_unbound','baseline');
    for(const chain of this.#requests.values())this.#complete(chain);
    this.#requests.clear();
    this.#root=null;
    return Object.freeze({schema:'aku.x.manual-startup-cookie-guard.v1',diagnostic:this.category(),candidateAttempts:this.#candidate,otherAttempts:this.#other,ambiguousAttempts:this.#ambiguous});
  }
}
