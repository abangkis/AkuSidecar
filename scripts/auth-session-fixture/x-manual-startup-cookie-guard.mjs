// Trusted Manual V2 startup only. Header attempts are not cookie-jar proof.
// Cookie names, values, URLs and attribute values never enter the summary.

const object = v => v !== null && typeof v === 'object' && !Array.isArray(v);
const invalid = Symbol();
const data = (v, k) => {
  const d = object(v) ? Object.getOwnPropertyDescriptor(v, k) : undefined;
  return d ? Object.hasOwn(d, 'value') ? d.value : invalid : undefined;
};
const ref = v => typeof v === 'string' && v.length > 0 && v.length <= 512;
const MAX_DIAGNOSTIC_RECORDS = 256;
// Exact no-adoption markers survive diagnostic eviction. Losing even these
// bounded markers is coverage loss, not a successful truncated observation.
const MAX_DIAGNOSTIC_IDS = 16384;
const increment = value => Math.min(Number.MAX_SAFE_INTEGER, value + 1);
// Closed projections never invoke accessors or retain browser-owned values.
const cacheProjection = response => Object.fromEntries([
  ['guardDiskCache','fromDiskCache'],['guardServiceWorker','fromServiceWorker'],['guardPrefetchCache','fromPrefetchCache']
].map(([key,field])=>{const value=data(response,field);return [key,value===undefined?'absent':value===true?'true':value===false?'false':'malformed'];}));
const firstCondition = checks => checks.find(([failed])=>failed)?.[1];
// Diagnostic only. Acceptance uses private request-local frame correlation.
function requestFrameProjection(params, root) {
  const d = Object.getOwnPropertyDescriptor(params, 'frameId');
  if (!d) return 'absent';
  if (!Object.hasOwn(d, 'value') || !ref(d.value)) return 'malformed';
  return d.value === root ? 'matching' : 'different';
}
const candidate = new Set(['auth_token', 'ct0']);
const token = /^[!#$%&'*+.^_`|~0-9a-z-]+$/i;
const otherCookie = Object.freeze({});
// Retain only a marker for a pending noncandidate line, never its name or value.
// Candidate and ambiguous name evidence is decided immediately.
function lineKind(line) {
  if (typeof line !== 'string' || !line) return 'syntax';
  if (line.length > 4096) return 'capacity';
  if (/[\x00-\x1f\x7f]/.test(line)) return 'syntax';
  const parts = line.split(';');
  if (parts.length > 32) return 'capacity';
  const first = parts.shift().trim(), i = first.indexOf('='), name = first.slice(0, i);
  // Candidate attempts revoke even when attributes or declared scope are bad.
  if (i > 0 && candidate.has(name)) return 'candidate';
  // The name must be an unambiguous token before anything is classified as
  // noncandidate. A comma in the pair is folded-header ambiguity; commas in
  // attributes are allowed unless they introduce another cookie-pair shape
  // (Expires dates contain an ordinary comma but no following name=value).
  if (i < 1 || name.length > 64 || !token.test(name) || first.slice(i + 1).includes(',')) return 'syntax';
  if (parts.some(part => /,\s*[!#$%&'*+.^_`|~0-9a-z-]+\s*=/i.test(part))) return 'syntax';
  return otherCookie;
}

// Header safety and account eligibility are separate. Request metadata below
// is bounded diagnostic context, except the explicitly armed root document.
export class ManualStartupCookieGuard {
  #frame; #session; #candidate=0; #other=0; #ambiguous=0;
  #closed=false; #baseline; #claimed=false;
  #armed=false; #oldLoader; #root=null; #commit=null;
  #history=new Map(); #diagnosticRequests=new Map();
  #diagnosticDropped=0; #diagnosticHeaderDropped=0; #diagnosticEvents=0; #diagnosticLines=0;
  #diagnosticUnframed=0; #diagnosticPartial=false;
  #rejection='unknown'; #trigger='unknown'; #provenance='unknown';
  #condition='unknown'; #epoch='unknown';
  #guardDiskCache='unknown'; #guardServiceWorker='unknown'; #guardPrefetchCache='unknown';
  #guardRequestFrame='unknown'; #guardHeaderReason='unknown'; #onRejection;
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
      this.#fail('guard_request_unbound','arm','unknown',firstCondition([[this.#closed,'root_arm_closed'],[this.#armed,'root_arm_repeated'],[!this.#baseline,'root_arm_before_seed'],[!ref(oldLoader),'root_arm_loader'],[!ref(this.#session),'root_arm_session']])); throw new Error('manual_cookie_guard_invalid');
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
    try{this.#onRejection(reason,this.trace(),this.failureReason());}catch{}
  }
  #fail(reason = 'guard_metadata_shape', trigger = 'metadata', provenance = 'unknown', condition = 'unknown', cache = null, frame = 'unknown', headerReason = 'unknown') {
    this.#note(reason,trigger,provenance,condition,cache,frame,headerReason); this.#ambiguous = Math.min(64,this.#ambiguous + 1);
  }
  rejection() { return this.#rejection; }
  // Preserve the first failure domain without changing the private trace wire.
  failureReason() { return this.#provenance==='root_scope'?'scope_lost':'observed_cookie_write'; }
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
    this.#diagnosticPartial=true; // Header classification does not claim response provenance.
    if(request)request.cookie=true;
    const frame=request?.frameStatus??'unknown';
    const kinds=lines.map(line=>lineKind(line));
    if(kinds.includes('candidate'))this.#note('cookie_candidate',trigger,provenance,'unknown',null,frame);
    // Per-message bounds protect inspection itself. Cumulative diagnostic
    // counters must never short-circuit candidate or ambiguous inspection.
    if(lines.length>32){this.#fail('guard_header_budget',trigger,provenance,'unknown',null,frame,'capacity');return null;}
    // Every observed header is inspected. Saturating diagnostic totals never
    // weakens per-message inspection or turns ordinary names into mutations.
    if(this.#diagnosticEvents>=64||this.#diagnosticLines+lines.length>32)this.#diagnosticHeaderDropped=increment(this.#diagnosticHeaderDropped);
    this.#diagnosticEvents=Math.min(64,this.#diagnosticEvents+1);
    this.#diagnosticLines=Math.min(32,this.#diagnosticLines+lines.length);
    if(malformed||!lines.length)this.#fail(malformedReason,trigger,provenance,'unknown',null,frame,malformedReason==='guard_header_budget'?'capacity':'metadata_shape');
    const other=[];
    for(const kind of kinds){
      if(kind==='candidate')this.#candidate=Math.min(64,this.#candidate+1);
      else if(typeof kind==='string')this.#fail('guard_header_classification',trigger,provenance,'unknown',null,frame,kind);
      else { other.push(kind); this.#other=Math.min(64,this.#other+1); }
    }
    return other;
  }
  // Called only at request selection, after this guard observed that event.
  // A prior response, redirect, or request permanently prevents ID adoption.
  canSelectRequest(id) { return !this.#closed && ref(this.#session) && this.#history.get(id) === 'first_request'; }
  #remember(id, request, redirect) {
    if(!ref(id)){this.#diagnosticPartial=true;return false;}
    if(this.#history.has(id)){
      if(request)this.#history.set(id,'ineligible');
      return false;
    }
    if(this.#history.size>=MAX_DIAGNOSTIC_IDS){this.#fail('guard_request_budget','request');return false;}
    this.#history.set(id,request&&!redirect?'first_request':'ineligible');
    return request&&!redirect;
  }
  #rootFailure(reason, trigger, condition, response = null, frame = this.#root?.frameStatus??'unknown') {
    this.#fail(reason,trigger,'root_scope',condition,response===null?this.#root?.cache??null:cacheProjection(response),frame);
  }
  observe(raw) {
    if(this.#closed)return true;
    const method=data(raw,'method');
    if(typeof method!=='string'||!(method.startsWith('Network.')||method==='Page.frameNavigated'))return this.#candidate>0||this.#ambiguous>0;
    if(ref(this.#session)&&data(raw,'sessionId')!==this.#session)return this.#candidate>0||this.#ambiguous>0;
    const p=data(raw,'params');if(!object(p)){this.#fail();return true;}
    const id=data(p,'requestId');
    const context=this.#diagnosticRequests.get(id);
    // Inspection precedes every correlation/adoption decision, including an
    // orphan, redirect, reused ID, or diagnostic record that was evicted.
    if(method==='Network.requestWillBeSent'&&data(p,'redirectResponse')!==undefined){
      const redirect=data(p,'redirectResponse');
      if(!object(redirect))this.#fail('guard_metadata_shape','redirect');
      else this.#attempt(data(redirect,'headers'),undefined,context,'redirect','redirect');
    }
    if(method==='Network.responseReceived'){
      const response=data(p,'response');
      if(!object(response))this.#fail('guard_metadata_shape','response');
      else this.#attempt(data(response,'headers'),undefined,context,'response');
    }
    if(method==='Network.responseReceivedExtraInfo')this.#attempt(data(p,'headers'),data(p,'blockedCookies'),context,'extra');
    if(method==='Page.frameNavigated'&&this.#armed&&data(data(p,'frame'),'id')===this.#frame){
      const frame=data(p,'frame'),loader=data(frame,'loaderId');
      const condition=firstCondition([[this.#commit!==null,'root_commit_duplicate'],[!ref(loader),'root_commit_loader_invalid'],[loader===this.#oldLoader,'root_commit_old_loader'],[data(frame,'url')!=='https://x.com/home','root_commit_url'],[data(frame,'parentId')!==undefined,'root_commit_parent']]);
      if(condition)this.#rootFailure('guard_response_correlation','document',condition);
      else this.#commit=loader;
      if(this.#root&&this.#root.loaderId!==loader)this.#rootFailure('guard_response_correlation','document','root_commit_loader_mismatch');
    }
    if(method==='Network.requestWillBeSent'){
      const redirect=data(p,'redirectResponse')!==undefined;
      const first=this.#remember(id,true,redirect);
      const frameStatus=requestFrameProjection(p,this.#frame);
      const requestURL=data(data(p,'request'),'url');
      let qualified=false;
      try { if(typeof requestURL!=='string')throw invalid; const u=new URL(requestURL); qualified=typeof requestURL==='string'&&requestURL.length<=4096&&u.protocol==='https:'&&['x.com','api.x.com'].includes(u.hostname)&&!u.port&&!u.username&&!u.password&&!u.hash; } catch {}
      const provenance=!qualified?'origin_scope':!ref(data(p,'frameId'))?'frame_scope':!ref(data(p,'loaderId'))?'loader_missing':'scoped';
      const r={frameStatus,provenance};
      // These projections are explanatory only. Missing/contradictory browser
      // metadata cannot change an unambiguous ordinary header into a mutation.
      if(frameStatus==='absent')this.#diagnosticUnframed=increment(this.#diagnosticUnframed);
      if(frameStatus!=='matching'||!ref(data(p,'loaderId'))||!first)this.#diagnosticPartial=true;
      if(ref(id)){
        if(!this.#diagnosticRequests.has(id)&&this.#diagnosticRequests.size>=MAX_DIAGNOSTIC_RECORDS){
          this.#diagnosticRequests.delete(this.#diagnosticRequests.keys().next().value);
          this.#diagnosticDropped=increment(this.#diagnosticDropped);
        }
        this.#diagnosticRequests.set(id,r);
      }
      const pinned=this.#armed&&data(p,'type')==='Document'&&data(p,'frameId')===this.#frame;
      if(pinned){
        const request=data(p,'request'),loader=data(p,'loaderId');
        const condition=firstCondition([[!!this.#root,'root_request_duplicate'],[!first,redirect?'root_request_redirect':'root_request_history'],[data(request,'method')!=='GET','root_request_method'],[data(request,'url')!=='https://x.com/home','root_request_url'],[!ref(loader),'root_request_loader_invalid'],[loader===this.#oldLoader,'root_request_old_loader'],[!!this.#commit&&this.#commit!==loader,'root_request_commit_mismatch']]);
        if(condition)this.#rootFailure('guard_request_unbound','request',condition,null,frameStatus);
        else this.#root={id,loaderId:loader,frameStatus,response:false,extra:null,needsExtra:null};
      }else if(this.#root&&this.#root.id===id)this.#rootFailure('guard_request_unbound','request','root_request_reused');
    }else if(method.startsWith('Network.')&&id!==undefined){
      const known=this.#history.has(id);
      this.#remember(id,false,false);
      if(!known||!context)this.#diagnosticPartial=true;
    }
    const root=this.#root;
    if(root&&id===root.id){
      if(method==='Network.responseReceived'){
        const response=data(p,'response'),extra=data(p,'hasExtraInfo');
        // Retain only closed cache projections for subsequent root failures.
        // The sticky first trace is never enriched from a later response.
        root.cache??=cacheProjection(response);
        const frame=requestFrameProjection(p,this.#frame);
        const condition=firstCondition([[root.response,'root_response_duplicate'],[data(p,'frameId')!==this.#frame,`root_response_frame_${frame}`],[data(p,'loaderId')!==root.loaderId,'root_response_loader'],[data(response,'url')!=='https://x.com/home','root_response_url'],[data(response,'status')!==200,'root_response_status'],[typeof extra!=='boolean','root_response_extra_flag'],...['fromDiskCache','fromServiceWorker','fromPrefetchCache'].map((k,i)=>[data(response,k)!==undefined&&data(response,k)!==false,['root_response_disk_cache','root_response_service_worker','root_response_prefetch_cache'][i]])]);
        if(condition)this.#rootFailure('guard_response_correlation','response',condition,response);
        root.response=true;root.needsExtra=extra;
      }
      if(method==='Network.responseReceivedExtraInfo'){
        const condition=firstCondition([[root.extra!==null,'root_extra_duplicate'],[data(p,'statusCode')!==200,'root_extra_status']]);
        if(condition)this.#rootFailure('guard_response_correlation','extra',condition);
        root.extra=data(p,'statusCode');
      }
      if(method==='Network.loadingFailed'||method==='Network.requestServedFromCache')this.#rootFailure('guard_response_correlation','response',method==='Network.loadingFailed'?'root_loading_failed':'root_served_from_cache');
      if(root.response&&root.extra!==null&&root.needsExtra!==true)this.#rootFailure('guard_response_correlation','extra','root_extra_unexpected');
    }
    return this.#candidate>0||this.#ambiguous>0;
  }
  finish() {
    if(this.#closed)throw new Error('manual_cookie_guard_replayed');
    this.#closed=true;
    if(!this.#baseline)this.#fail('guard_request_unbound','baseline');
    const root=this.#root;
    if(this.#armed&&!root)this.#rootFailure('guard_completion_missing','finish','root_missing');
    if(root){const condition=firstCondition([[!root.response,'root_response_missing'],[this.#commit!==root.loaderId,'root_commit_missing'],[root.needsExtra===true&&root.extra===null,'root_extra_missing']]);if(condition)this.#rootFailure('guard_completion_missing','finish',condition);}
    // Correlation of ordinary headers is deliberately not claimed. An absence
    // of cookie candidates is bounded observed-header evidence only.
    this.#history.clear();this.#diagnosticRequests.clear();this.#root=null;
    return Object.freeze({schema:'aku.x.manual-startup-cookie-guard.v1',diagnostic:this.category(),candidateAttempts:this.#candidate,otherAttempts:this.#other,ambiguousAttempts:this.#ambiguous,
      diagnosticCoverage:this.#diagnosticDropped||this.#diagnosticHeaderDropped||this.#diagnosticUnframed||this.#diagnosticPartial?'partial':'complete',diagnosticRecordsDropped:this.#diagnosticDropped,diagnosticHeadersDropped:this.#diagnosticHeaderDropped,
      diagnosticMissingFrameRequests:this.#diagnosticUnframed});
  }
}
