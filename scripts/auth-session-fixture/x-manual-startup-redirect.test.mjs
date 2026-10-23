import test from 'node:test';
import assert from 'node:assert/strict';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {startupFailureLatch,startupFailure,requireBoundStartup,startupDiagnostic} from './x-manual-startup-diagnostic.mjs';

const urls=['https://x.com/asset','https://api.x.com/asset2','https://x.com/asset3'];
const other={'Set-Cookie':'lang=PRIVATE_VALUE; Path=/'};
function fixture({beforeSeed=false}={}){
  const g=new ManualStartupCookieGuard('frame','session',{beforeSeed});
  const emit=(method,params,sessionId='session')=>g.observe({method,params,sessionId});
  const request=(index=0,patch={})=>emit('Network.requestWillBeSent',{
    requestId:'chain',frameId:'frame',loaderId:'loader',type:'Image',request:{method:'GET',url:urls[index]},...patch});
  const redirect=(index=1,{status=302,headers={},extra=false,...patch}={})=>request(index,{
    redirectHasExtraInfo:extra,redirectResponse:{url:urls[index-1],status,headers},...patch});
  const response=(index=1,{status=200,headers={},extra=false,...patch}={})=>emit('Network.responseReceived',{
    requestId:'chain',frameId:'frame',loaderId:'loader',hasExtraInfo:extra,response:{url:urls[index],status,headers},...patch});
  const extra=(status=302,headers={},patch={})=>emit('Network.responseReceivedExtraInfo',{requestId:'chain',statusCode:status,headers,blockedCookies:[],...patch});
  return{g,emit,request,redirect,response,extra};
}
function accepted(s,count){
  const result=s.g.finish();assert.equal(s.g.rejection(),'unknown');assert.equal(result.diagnostic,count?'other_only':'not_observed');
  assert.equal(result.otherAttempts,count);assert.equal(result.ambiguousAttempts,0);
  for(const raw of [...urls,'PRIVATE_VALUE','lang','frame','session','loader'])assert.equal(JSON.stringify(result).includes(raw),false);
}
function rejected(s,reason){
  const result=s.g.finish();assert.notEqual(s.g.rejection(),'unknown');assert.equal(result.diagnostic,'ambiguous');
  if(reason)assert.equal(s.g.rejection(),reason);return result;
}

const frameCases=[['absent','absent'],['undefined','malformed'],['empty','malformed'],['number','malformed'],
  ['object','malformed'],['oversized','malformed'],['accessor','malformed'],['different','different'],['matching','matching']];
function frameParams(kind,onRead=()=>{}){
  const params={requestId:'chain',loaderId:'loader',type:'Image',request:{method:'GET',url:urls[0]}};
  if(kind==='accessor')Object.defineProperty(params,'frameId',{get(){onRead();throw new Error('PRIVATE_FRAME_GETTER');}});
  else if(kind!=='absent')params.frameId={undefined:undefined,empty:'',number:42,object:{private:'PRIVATE_FRAME_VALUE'},
    oversized:'PRIVATE_FRAME_VALUE'.repeat(40),different:'PRIVATE_CHILD_FRAME',matching:'frame'}[kind];
  return params;
}
for(const [kind,state]of frameCases)test(`request frame metadata separates valid session-local provenance from invalid references: ${kind}`,()=>{
  const s=fixture();let reads=0;
  s.emit('Network.requestWillBeSent',frameParams(kind,()=>reads++));
  const valid=['absent','matching','different'].includes(kind);
  assert.equal(s.extra(200,other),!valid);
  if(valid){s.response(0,{extra:true,frameId:kind==='different'?'PRIVATE_CHILD_FRAME':'frame'});accepted(s,1);assert.equal(s.g.trace().guardRequestFrame,'unknown');}
  else{
    assert.equal(s.g.rejection(),'guard_request_unbound');
    assert.equal(s.g.trace().guardProvenance,'frame_scope');assert.equal(s.g.trace().guardRequestFrame,state);
    const first=s.g.trace();s.request();s.extra(200,{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'});
    for(let i=0;i<130;i++)s.request();
    s.g.finish();assert.deepEqual(s.g.trace(),first);assert.equal(s.g.category(),'candidate_observed');
  }
  assert.equal(reads,0);
  for(const raw of ['PRIVATE_FRAME_VALUE','PRIVATE_FRAME_GETTER','PRIVATE_CHILD_FRAME',...urls])assert.equal(JSON.stringify(s.g.trace()).includes(raw),false);
});

test('unseen request remains unknown and a late matching request cannot repair its first frame diagnostic',()=>{
  const s=fixture();s.extra(200,other);assert.equal(s.g.rejection(),'guard_request_unbound');
  const first=s.g.trace();assert.equal(first.guardProvenance,'absent');assert.equal(first.guardRequestFrame,'unknown');
  s.request();s.response(0,{extra:true});s.g.finish();assert.deepEqual(s.g.trace(),first);
});

test('matching request with missing or different response frame remains response correlation, never request frame scope',()=>{
  for(const kind of ['absent','different','accessor']){
    const s=fixture();s.request();let reads=0;
    const params={requestId:'chain',loaderId:'loader',hasExtraInfo:false,response:{url:urls[0],status:200,headers:other}};
    if(kind==='different')params.frameId='PRIVATE_RESPONSE_FRAME';
    if(kind==='accessor')Object.defineProperty(params,'frameId',{get(){reads++;throw Error('PRIVATE_FRAME_GETTER');}});
    s.emit('Network.responseReceived',params);rejected(s,'guard_response_correlation');
    assert.equal(s.g.trace().guardProvenance,'scoped');assert.equal(s.g.trace().guardRequestFrame,'matching');assert.equal(reads,0);
  }
});

test('frame diagnostics cannot change candidate, blocked, mixed or ambiguous-header precedence',()=>{
  for(const [kind,state]of frameCases)for(const source of ['candidate','blocked','mixed','ambiguous','malformed']){
    const s=fixture();s.emit('Network.requestWillBeSent',frameParams(kind));
    const candidate='ct0=PRIVATE_VALUE; Path=/';
    s.extra(200,source==='blocked'?{}:{'Set-Cookie':source==='candidate'?candidate:source==='mixed'?`lang=x; Path=/\n${candidate}`:source==='ambiguous'?'lang=x, ct0=y':['lang=x']},
      source==='blocked'?{blockedCookies:[{cookieLine:candidate}]}:{});
    assert.equal(s.g.rejection(),['candidate','blocked','mixed'].includes(source)?'cookie_candidate':source==='ambiguous'?'guard_header_classification':'guard_metadata_shape');
    assert.equal(s.g.trace().guardRequestFrame,state);s.g.finish();
  }
});

test('redirect frame diagnostic follows the failed predicate and header attempts keep their predecessor snapshot',()=>{
  for(const [kind,state]of frameCases.filter(([kind])=>!['matching','different'].includes(kind))){
    const previous=fixture();previous.emit('Network.requestWillBeSent',frameParams(kind));previous.redirect();
    assert.equal(previous.g.trace().guardCondition,'previous_frame');assert.equal(previous.g.trace().guardRequestFrame,state);rejected(previous,'guard_request_unbound');
    const current=fixture();current.request();const params=frameParams(kind);
    params.request.url=urls[1];params.redirectHasExtraInfo=false;params.redirectResponse={url:urls[0],status:302,headers:{}};
    current.emit('Network.requestWillBeSent',params);
    assert.equal(current.g.trace().guardCondition,'current_frame');assert.equal(current.g.trace().guardRequestFrame,state);rejected(current,'guard_request_unbound');
    const attempt=fixture();attempt.request();params.redirectResponse.headers={'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'};
    attempt.emit('Network.requestWillBeSent',params);assert.equal(attempt.g.rejection(),'cookie_candidate');assert.equal(attempt.g.trace().guardRequestFrame,'matching');attempt.g.finish();
  }
});

test('root frame pin, old-loader requests and cookie-free unsupported traffic keep their original behavior',()=>{
  const s=fixture();s.g.armReload('old');
  s.emit('Page.frameNavigated',{frame:{id:'PRIVATE_CHILD_FRAME',loaderId:'other',url:'https://x.com/home'}});
  s.request(0,{frameId:'PRIVATE_CHILD_FRAME'});s.extra(200,other);s.response(0,{extra:true,frameId:'PRIVATE_CHILD_FRAME'});
  assert.throws(()=>s.g.claim('PRIVATE_CHILD_FRAME','session'));assert.doesNotThrow(()=>s.g.claim('frame','session'));accepted(s,1);
  const old=fixture();old.g.armReload('old');old.request(0,{loaderId:'old'});old.response(0,{loaderId:'old',headers:other});accepted(old,1);
  for(const [kind]of frameCases){const clean=fixture();clean.emit('Network.requestWillBeSent',frameParams(kind));accepted(clean,0);}
});

test('declared resource redirects classify each exact hop without replacing the predecessor',()=>{
  const s=fixture();s.request();assert.equal(s.redirect(1,{headers:other}),false);assert.equal(s.g.category(),'not_observed');s.response(1,{headers:other});assert.equal(s.g.category(),'not_observed');accepted(s,2);
  const clean=fixture();clean.request();clean.redirect();clean.response();accepted(clean,0);
});

test('nonroot resource chains bind to their own frame and loader without acquiring root authority',()=>{
  for(const frameId of ['PRIVATE_NONROOT_A','PRIVATE_NONROOT_B']){
    const s=fixture();s.request(0,{frameId,loaderId:'nonroot-loader'});
    s.redirect(1,{frameId,loaderId:'nonroot-loader',headers:other,extra:true});s.extra(302);
    s.response(1,{frameId,loaderId:'nonroot-loader',headers:other});accepted(s,2);
  }
  for(const fault of ['redirect-frame','redirect-loader','response-frame','response-loader','missing-response','missing-extra','mismatched-extra','duplicate-extra']){
    const s=fixture();s.request(0,{frameId:'nonroot',loaderId:'nonroot-loader'});
    s.redirect(1,{frameId:fault==='redirect-frame'?'another':'nonroot',loaderId:fault==='redirect-loader'?'another':'nonroot-loader',extra:true,headers:other});
    if(fault!=='missing-extra')s.extra(fault==='mismatched-extra'?307:302);
    if(fault==='duplicate-extra')s.extra(302);
    if(fault!=='missing-response')s.response(1,{frameId:fault==='response-frame'?'frame':'nonroot',loaderId:fault==='response-loader'?'loader':'nonroot-loader'});
    rejected(s);if(fault==='redirect-frame')assert.equal(s.g.trace().guardCondition,'frame_mismatch');
  }
});

test('nonroot correlation requires the exact observed session and a valid loader',()=>{
  for(const fault of ['foreign-session','missing-loader','malformed-loader','missing-predecessor']){
    const s=fixture();
    if(fault!=='missing-predecessor'){
      const p={requestId:'chain',frameId:'nonroot',loaderId:fault==='missing-loader'?undefined:fault==='malformed-loader'?42:'nonroot-loader',request:{method:'GET',url:urls[0]}};
      s.emit('Network.requestWillBeSent',p,fault==='foreign-session'?'foreign':'session');
    }
    s.extra(200,other);rejected(s,'guard_request_unbound');
  }
  const unbound=new ManualStartupCookieGuard('frame');
  unbound.observe({method:'Network.requestWillBeSent',params:{requestId:'r',frameId:'nonroot',loaderId:'loader',request:{url:urls[0]}}});
  unbound.observe({method:'Network.responseReceivedExtraInfo',params:{requestId:'r',statusCode:200,headers:other}});
  assert.equal(unbound.rejection(),'guard_request_unbound');unbound.finish();
  const ignored=fixture();ignored.emit('Network.responseReceivedExtraInfo',{requestId:'r',headers:{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'}},'foreign');accepted(ignored,0);
});

test('ExtraInfo before redirect closure, after the next request, and around final response can resolve uniquely',()=>{
  for(const prior of ['before-close','after-next','after-final'])for(const final of ['before-response','after-response']){
    const s=fixture();s.request();
    if(prior==='before-close')assert.equal(s.extra(302,other),false);
    s.redirect(1,{extra:true});
    if(prior==='after-next')s.extra(302,other);
    if(final==='before-response')s.extra(200,other);
    s.response(1,{extra:true});
    if(final==='after-response')s.extra(200,other);
    if(prior==='after-final')s.extra(302,other);
    accepted(s,2);
  }
});

test('multiple distinct redirect statuses resolve without an ExtraInfo FIFO assumption',()=>{
  for(const order of [[301,302,200],[200,302,301],[302,200,301]]){
    const s=fixture();s.request();s.redirect(1,{status:301,extra:true});s.redirect(2,{status:302,extra:true});s.response(2,{extra:true});
    for(const status of order)s.extra(status,other);accepted(s,3);
  }
});

test('false declarations reserve no ExtraInfo slot, including a later hop event arriving before final response',()=>{
  const s=fixture();s.request();s.redirect(1,{headers:other,extra:false});s.extra(200,other);s.response(1,{extra:true});accepted(s,2);
  const mixed=fixture();mixed.request();mixed.extra(301,other);mixed.redirect(1,{status:301,extra:true});mixed.redirect(2,{status:302,extra:false});mixed.extra(200,other);mixed.response(2,{extra:true});accepted(mixed,2);
});

test('pending ExtraInfo retains only a noncandidate marker and binds to its own hop',()=>{
  const line='PRIVATE_NAME=PRIVATE_VALUE; Domain=foreign.invalid; Path=/PRIVATE_PATH; Priority=High; Partitioned; SameSite=odd; Max-Age=unbounded';
  const good=fixture();good.request();good.redirect(1,{extra:false});good.extra(200,{'Set-Cookie':line});good.response(1,{extra:true});accepted(good,1);
  const wrong=fixture();wrong.request();wrong.extra(301,{'Set-Cookie':line});wrong.redirect(1,{extra:true});wrong.response();rejected(wrong,'guard_response_correlation');
});

test('same-status hop slots, duplicate extras, mismatches, unexpected and missing extras remain closed',()=>{
  for(const fault of ['same-status','duplicate','mismatch','unexpected','missing','pending-final','duplicate-response']){
    const s=fixture();s.request();s.redirect(1,{status:302,extra:fault!=='unexpected'});
    if(fault==='same-status'){s.redirect(2,{status:302,extra:true});s.extra(302,other);s.extra(302,other);s.response(2);}
    else{
      if(fault!=='missing')s.extra(fault==='mismatch'?307:302,other);
      if(fault==='duplicate')s.extra(302);
      if(fault!=='pending-final')s.response();
      if(fault==='duplicate-response')s.response();
    }
    rejected(s,['missing','pending-final'].includes(fault)?'guard_completion_missing':'guard_response_correlation');
  }
});

test('multiple ExtraInfo events before first redirect declaration stay conservatively rejected without future rebinding',()=>{
  const s=fixture();s.request();s.extra(302,other);s.extra(200,other);s.redirect(1,{extra:true});s.response(1,{extra:true});rejected(s,'guard_response_correlation');
});

test('all candidate attempts revoke immediately in declared headers and early or late blocked ExtraInfo',()=>{
  for(const name of ['auth_token','ct0'])for(const source of ['redirect','final','early-extra','late-extra','blocked-extra']){
    const s=fixture();s.request();const line=`${name}=PRIVATE_VALUE; Max-Age=0; Domain=wrong.invalid`;
    if(source==='early-extra')assert.equal(s.extra(302,{'Set-Cookie':line}),true);
    assert.equal(s.redirect(1,{headers:source==='redirect'?{'Set-Cookie':line}:{},extra:['early-extra','late-extra','blocked-extra'].includes(source)}),['redirect','early-extra'].includes(source));
    if(source==='late-extra')assert.equal(s.extra(302,{'Set-Cookie':line}),true);
    if(source==='blocked-extra')assert.equal(s.extra(302,{}, {blockedCookies:[{cookieLine:line}]}),true);
    s.response(1,{headers:source==='final'?{'Set-Cookie':line}:{}});
    const first=s.g.rejection(),trace=s.g.trace();assert.equal(first,'cookie_candidate');
    const result=s.g.finish();assert.equal(result.diagnostic,'candidate_observed');assert.equal(s.g.rejection(),first);assert.deepEqual(s.g.trace(),trace);
    assert.equal(JSON.stringify(result).includes('PRIVATE_VALUE'),false);
  }
});

test('unfamiliar pending names and attributes pass only with exact hop correlation',()=>{
  for(const line of ['unknown_private=PRIVATE_VALUE; Path=/','lang=x; Path=/; Priority=High; Partitioned']){
    const s=fixture();s.request();s.redirect(1,{extra:true});
    assert.equal(s.extra(302,{'Set-Cookie':line}),false);
    s.response(1,{extra:false});accepted(s,1);
  }
  const ambiguous=fixture();ambiguous.request();
  assert.equal(ambiguous.extra(302,{'Set-Cookie':'lang=x, ct0=y'}),true);
  const first=ambiguous.g.rejection();assert.equal(first,'guard_header_classification');
  ambiguous.redirect(1,{extra:true});ambiguous.response();rejected(ambiguous,first);
  const malformed=fixture();malformed.request();
  assert.equal(malformed.extra(302,{'Set-Cookie':['lang=x']}),true);
  const malformedFirst=malformed.g.rejection();assert.equal(malformedFirst,'guard_metadata_shape');
  malformed.redirect(1,{extra:true});malformed.response();rejected(malformed,malformedFirst);
});

test('missing or accessor-owned required header evidence and blockedCookies shape cannot establish a complete hop',()=>{
  for(const fault of ['redirect-absent','redirect-getter','final-absent','final-getter','extra-absent','extra-getter','blocked-absent','blocked-object','blocked-hole','blocked-getter']){
    const s=fixture();s.request();let reads=0;
    const accessor=Object.defineProperty({},'headers',{get(){reads++;throw Error('private_getter_read');}});
    s.redirect(1,fault==='redirect-absent'?{redirectResponse:{url:urls[0],status:302}}:fault==='redirect-getter'?{redirectResponse:Object.assign(accessor,{url:urls[0],status:302})}:{extra:fault.startsWith('extra')||fault.startsWith('blocked')});
    if(fault.startsWith('extra')||fault.startsWith('blocked')){
      const extra={requestId:'chain',statusCode:302,headers:{},blockedCookies:[]};
      if(fault==='extra-absent')delete extra.headers;
      if(fault==='extra-getter')Object.defineProperty(extra,'headers',{get(){reads++;throw Error('private_getter_read');}});
      if(fault==='blocked-absent')delete extra.blockedCookies;
      if(fault==='blocked-object')extra.blockedCookies={};
      if(fault==='blocked-hole')extra.blockedCookies=new Array(1);
      if(fault==='blocked-getter')Object.defineProperty(extra,'blockedCookies',{get(){reads++;throw Error('private_getter_read');}});
      s.emit('Network.responseReceivedExtraInfo',extra);
    }
    s.response(1,fault==='final-absent'?{response:{url:urls[1],status:200}}:fault==='final-getter'?{response:Object.assign(accessor,{url:urls[1],status:200})}:{});
    rejected(s);assert.equal(reads,0,fault);
  }
});

test('missing predecessor, undeclared reuse, malformed redirect and unsupported request provenance never repair',()=>{
  for(const fault of ['predecessor','reuse','null','flag','url','status','frame','loader','origin','method','type','old-type','old-method','old-origin','old-frame','old-loader']){
    const s=fixture();
    if(fault!=='predecessor')s.request(0,fault==='old-type'?{type:undefined}:fault==='old-method'?{request:{method:'POST',url:urls[0]}}:fault==='old-origin'?{request:{method:'GET',url:'https://foreign.invalid/a'}}:fault==='old-frame'?{frameId:'foreign'}:fault==='old-loader'?{loaderId:''}:{});
    if(fault==='reuse')s.request(1);
    else s.redirect(1,fault==='null'?{redirectResponse:null}:fault==='flag'?{redirectHasExtraInfo:undefined}:fault==='url'?{redirectResponse:{url:urls[2],status:302,headers:{}}}:fault==='status'?{status:304}:fault==='frame'?{frameId:'foreign'}:fault==='loader'?{loaderId:'different'}:fault==='origin'?{request:{method:'GET',url:'https://foreign.invalid/a'}}:fault==='method'?{request:{method:'POST',url:urls[1]}}:fault==='type'?{type:undefined}:{});
    const first=s.g.rejection(),trace=s.g.trace();assert.notEqual(first,'unknown',fault);s.response();rejected(s,first);assert.deepEqual(s.g.trace(),trace);
  }
});

test('document and account-settings redirects cannot become resource or identity authority in either direction',()=>{
  for(const endpoint of ['https://x.com/home','https://api.x.com/1.1/account/settings.json','https://x.com/i/api/1.1/account/settings.json','https://api.x.com/i/api/1.1/account/settings.json'])for(const direction of ['from','to']){
    const s=fixture();const document=endpoint.endsWith('/home');
    s.request(0,direction==='from'?{type:document?'Document':'XHR',request:{method:'GET',url:endpoint}}:{});
    s.redirect(1,direction==='from'?{redirectResponse:{url:endpoint,status:302,headers:other}}:{type:document?'Document':'XHR',request:{method:'GET',url:endpoint}});
    s.response();rejected(s);
  }
  const root=fixture();root.g.armReload('old');root.request(0,{type:'Document',request:{method:'GET',url:'https://x.com/home'}});
  root.redirect(1,{redirectResponse:{url:'https://x.com/home',status:302,headers:other}});root.response();rejected(root);
});

test('cache, serviceworker, failed/completed requests, and final redirect statuses are outside the envelope',()=>{
  for(const fault of ['redirect-cache','redirect-worker','final-cache','final-worker','cache-event','failed','completed-before','completed-before-response','duplicate-completion','final-redirect','final-informational']){
    const s=fixture();s.request();
    if(fault==='completed-before')s.emit('Network.loadingFinished',{requestId:'chain'});
    if(fault==='cache-event')s.emit('Network.requestServedFromCache',{requestId:'chain'});
    s.redirect(1,['redirect-cache','redirect-worker'].includes(fault)?{redirectResponse:{url:urls[0],status:302,headers:{},[fault==='redirect-cache'?'fromDiskCache':'fromServiceWorker']:true}}:{});
    if(fault==='completed-before-response')s.emit('Network.loadingFinished',{requestId:'chain'});
    s.response(1,{status:fault==='final-redirect'?302:fault==='final-informational'?103:200,...(['final-cache','final-worker'].includes(fault)?{response:{url:urls[1],status:200,headers:{},[fault==='final-cache'?'fromDiskCache':'fromServiceWorker']:true}}:{})});
    if(fault==='failed')s.emit('Network.loadingFailed',{requestId:'chain'});
    if(fault==='duplicate-completion'){s.emit('Network.loadingFinished',{requestId:'chain'});s.emit('Network.loadingFinished',{requestId:'chain'});}
    rejected(s);
  }
});

test('each redirect cache attribute is summarized by descriptor category without invoking accessors',()=>{
  const attributes=[['fromDiskCache','guardDiskCache'],['fromServiceWorker','guardServiceWorker'],['fromPrefetchCache','guardPrefetchCache']];
  for(const [key,field]of attributes)for(const category of ['true','absent','false','malformed','accessor']){
    const s=fixture();s.request();let reads=0;
    const response={url:urls[0],status:302,headers:{}};
    const trigger=attributes.find(([other])=>other!==key)[0];
    if(category==='true')response[key]=true;
    else if(category==='absent')response[trigger]=true;
    else if(category==='false'){response[key]=false;response[trigger]=true;}
    else if(category==='malformed')response[key]='PRIVATE_CACHE_VALUE';
    else Object.defineProperty(response,key,{get(){reads++;throw new Error('PRIVATE_CACHE_GETTER');}});
    s.redirect(1,{redirectResponse:response});
    const trace=s.g.trace();
    assert.equal(s.g.rejection(),'guard_request_unbound',`${key} ${category}`);
    assert.equal(trace.guardCondition,'response_cache',`${key} ${category}`);
    const states=Object.fromEntries(attributes.map(([attribute,name])=>[name,
      attribute===key?(category==='true'?'true':category==='absent'?'absent':category==='false'?'false':'malformed'):
        attribute===trigger&&['absent','false'].includes(category)?'true':'absent']));
    assert.deepEqual({guardDiskCache:trace.guardDiskCache,guardServiceWorker:trace.guardServiceWorker,guardPrefetchCache:trace.guardPrefetchCache},states);
    assert.equal(reads,0,`${key} accessor`);
    for(const raw of ['PRIVATE_CACHE_VALUE','PRIVATE_CACHE_GETTER',...urls])assert.equal(JSON.stringify(trace).includes(raw),false);
  }
});

test('cache projection survives an earlier known-other latch and stays immutable across seed',()=>{
  const otherCookie={'Set-Cookie':'lang=x; Domain=api.x.com; Path=/'};
  const known=fixture();known.request();
  known.redirect(1,{headers:otherCookie,redirectResponse:{url:urls[0],status:302,headers:otherCookie,fromDiskCache:true}});
  assert.equal(known.g.rejection(),'guard_request_unbound');
  assert.deepEqual(known.g.trace(),{guardTrigger:'redirect',guardProvenance:'redirect',guardCondition:'response_cache',guardEpoch:'post_seed',guardDiskCache:'true',guardServiceWorker:'absent',guardPrefetchCache:'absent',guardRequestFrame:'matching',guardHeaderReason:'unknown'});

  const before=fixture({beforeSeed:true});before.request();
  before.redirect(1,{redirectResponse:{url:urls[0],status:302,headers:{},fromServiceWorker:true}});
  const first=before.g.trace();assert.equal(first.guardEpoch,'before_seed');assert.equal(first.guardServiceWorker,'true');
  before.g.beginPostSeed();before.extra(302,{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'});before.response();
  assert.equal(before.g.rejection(),'guard_request_unbound');assert.deepEqual(before.g.trace(),first);
  assert.equal(JSON.stringify(first).includes('PRIVATE'),false);
});

test('final redirect response cache flags are retained when completion rejects the chain',()=>{
  const s=fixture();s.request();s.redirect(1);
  s.response(1,{response:{url:urls[1],status:200,headers:{},fromDiskCache:false,fromServiceWorker:true,fromPrefetchCache:'malformed-private'}});
  assert.equal(s.g.rejection(),'unknown');
  rejected(s,'guard_response_correlation');
  assert.deepEqual(s.g.trace(),{guardTrigger:'finish',guardProvenance:'scoped',guardCondition:'unknown',guardEpoch:'post_seed',guardDiskCache:'false',guardServiceWorker:'true',guardPrefetchCache:'malformed',guardRequestFrame:'matching',guardHeaderReason:'unknown'});
  assert.equal(JSON.stringify(s.g.trace()).includes('malformed-private'),false);
});

test('a prior candidate or metadata failure is never relabeled by a later cache redirect',()=>{
  const candidate=fixture();candidate.request();
  candidate.redirect(1,{redirectResponse:{url:urls[0],status:302,headers:{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'},fromDiskCache:true}});
  assert.equal(candidate.g.rejection(),'cookie_candidate');
  assert.deepEqual(candidate.g.trace(),{guardTrigger:'redirect',guardProvenance:'redirect',guardCondition:'unknown',guardEpoch:'post_seed',guardDiskCache:'unknown',guardServiceWorker:'unknown',guardPrefetchCache:'unknown',guardRequestFrame:'matching',guardHeaderReason:'unknown'});

  const shape=fixture();shape.request();shape.extra(302,{'Set-Cookie':'lang=x, ct0=y'});
  shape.redirect(1,{redirectResponse:{url:urls[0],status:302,headers:{},fromPrefetchCache:true}});
  assert.equal(shape.g.rejection(),'guard_header_classification');
  assert.deepEqual(shape.g.trace(),{guardTrigger:'extra',guardProvenance:'scoped',guardCondition:'unknown',guardEpoch:'post_seed',guardDiskCache:'unknown',guardServiceWorker:'unknown',guardPrefetchCache:'unknown',guardRequestFrame:'matching',guardHeaderReason:'syntax'});
});

test('pre-seed projections stay outside the attempt epoch while late prior-hop candidates revoke post-seed',()=>{
  const clean=fixture({beforeSeed:true});clean.request();clean.extra(302,{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'});clean.redirect(1,{extra:true});clean.g.beginPostSeed();clean.response();accepted(clean,0);
  for(const blocked of [false,true]){
    const s=fixture({beforeSeed:true});s.request();s.redirect(1,{extra:true});s.g.beginPostSeed();
    assert.equal(s.extra(302,blocked?{}:{'Set-Cookie':'ct0=; Path=/'},blocked?{blockedCookies:[{cookieLine:'auth_token=; Path=/'}]}:{}),true);
    s.response();assert.equal(s.g.finish().diagnostic,'candidate_observed');assert.equal(s.g.rejection(),'cookie_candidate');
  }
});

test('same-ID hop chains and pending metadata consume the lifetime record budget',()=>{
  const s=fixture();s.request();for(let i=1;i<1280;i++)s.redirect(1,{redirectResponse:{url:urls[1],status:302,headers:{}}});
  assert.equal(s.g.rejection(),'guard_request_unbound'); // First closure deliberately disagrees with predecessor.
  const capped=fixture();capped.request();
  for(let i=1;i<1280;i++)capped.request(1,{redirectHasExtraInfo:false,redirectResponse:{url:i===1?urls[0]:urls[1],status:302,headers:{}}});
  assert.equal(capped.g.rejection(),'unknown');capped.redirect(1,{redirectResponse:{url:urls[1],status:302,headers:{}}});assert.equal(capped.g.rejection(),'guard_request_budget');rejected(capped,'guard_request_budget');
  const pending=fixture();pending.request();for(let i=0;i<1280;i++)pending.extra(302);assert.equal(pending.g.rejection(),'guard_request_budget');
});

test('1280 security request records are available across the seed boundary; record 1281 rejects without resetting the first failure',()=>{
  const s=fixture({beforeSeed:true});
  const request=i=>s.request(0,{requestId:'record-'+i,type:'Document'});
  for(let i=0;i<640;i++)request(i);
  assert.equal(s.g.rejection(),'unknown');s.g.beginPostSeed();
  for(let i=640;i<1280;i++)request(i);
  assert.equal(s.g.rejection(),'unknown');request(1280);
  assert.equal(s.g.rejection(),'guard_request_budget');
  const first=s.g.trace();assert.equal(first.guardEpoch,'post_seed');assert.equal(first.guardTrigger,'request');
  s.extra(200,{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'});s.g.finish();
  assert.deepEqual(s.g.trace(),first);assert.equal(s.g.category(),'candidate_observed');
});

test('security requests and queued ExtraInfo share the exact 1280-record capacity',()=>{
  const s=fixture();
  for(let i=0;i<640;i++)s.request(0,{requestId:'record-'+i,type:'Document'});
  for(let i=0;i<640;i++)s.emit('Network.responseReceivedExtraInfo',{requestId:'record-'+i,statusCode:200,headers:{},blockedCookies:[]});
  assert.equal(s.g.rejection(),'unknown');
  s.emit('Network.responseReceivedExtraInfo',{requestId:'record-0',statusCode:200,headers:{},blockedCookies:[]});
  assert.equal(s.g.rejection(),'guard_request_budget');assert.equal(s.g.trace().guardTrigger,'extra');
  rejected(s,'guard_request_budget');
});

test('candidate attempts beyond the former 128-record ceiling still revoke immediately',()=>{
  for(const name of ['auth_token','ct0']){
    const s=fixture();for(let i=0;i<200;i++)s.request(0,{requestId:'record-'+i});
    s.request();assert.equal(s.extra(200,{'Set-Cookie':`${name}=PRIVATE_VALUE; Path=/`}),true);
    assert.equal(s.g.rejection(),'cookie_candidate');assert.equal(s.g.finish().diagnostic,'candidate_observed');
  }
});

test('reader and collector guard instances each retain their own 1280 security-record capacity',()=>{
  const reader=fixture(),collector=fixture();
  for(let i=0;i<1280;i++)reader.request(0,{requestId:'reader-'+i,type:'Document'});
  for(let i=0;i<200;i++)collector.request(0,{requestId:'collector-'+i});
  reader.request(0,{type:'Document'});assert.equal(reader.g.rejection(),'guard_request_budget');
  collector.request();collector.response(0,{headers:other});accepted(collector,1);rejected(reader,'guard_request_budget');
});

test('a later candidate cannot replace the first structural rejection or revive an abandoned chain',()=>{
  const s=fixture();s.request();s.request(1);const first=s.g.trace();s.extra(302,{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'});
  assert.equal(s.g.rejection(),'guard_request_unbound');assert.deepEqual(s.g.trace(),first);assert.equal(s.g.finish().diagnostic,'candidate_observed');
});

// Every case changes one term of the existing redirect envelope. These are
// invented accountless events; passing them does not qualify native X traffic.
for(const condition of ['predecessor_missing','redirect_shape','previous_response','chain_completed','chain_failed','chain_cache','previous_resource','current_resource','previous_origin','previous_frame','previous_loader','current_origin','current_frame','current_loader','loader_mismatch','redirect_url','redirect_status','extra_flag','headers_shape','response_cache','frame_mismatch']){
  test(`first failed redirect condition is ${condition} without changing rejection`,()=>{
    const s=fixture();
    const old=condition==='previous_resource'?{type:'Document'}:condition==='previous_origin'?{request:{method:'GET',url:'https://PRIVATE.invalid/old'}}:condition==='previous_frame'?{frameId:''}:condition==='previous_loader'?{loaderId:''}:{};
    if(condition!=='predecessor_missing')s.request(0,old);
    if(condition==='previous_response')s.response(0);
    if(condition==='chain_completed')s.emit('Network.loadingFinished',{requestId:'chain'});
    if(condition==='chain_failed')s.emit('Network.loadingFailed',{requestId:'chain'});
    if(condition==='chain_cache')s.emit('Network.requestServedFromCache',{requestId:'chain'});
    const patch=condition==='redirect_shape'?{redirectResponse:null}:condition==='current_resource'?{request:{method:'POST',url:urls[1]}}:condition==='current_origin'?{request:{method:'GET',url:'https://PRIVATE.invalid/new'}}:condition==='current_frame'?{frameId:''}:condition==='current_loader'?{loaderId:''}:condition==='frame_mismatch'?{frameId:'PRIVATE_FRAME'}:condition==='loader_mismatch'?{loaderId:'PRIVATE_LOADER'}:condition==='redirect_url'?{redirectResponse:{url:urls[2],status:302,headers:{}}}:condition==='redirect_status'?{status:304}:condition==='extra_flag'?{redirectHasExtraInfo:undefined}:condition==='headers_shape'?{redirectResponse:{url:urls[0],status:302}}:condition==='response_cache'?{redirectResponse:{url:urls[0],status:302,headers:{},fromServiceWorker:true}}:{};
    assert.equal(s.redirect(1,patch),true);
    assert.equal(s.g.rejection(),'guard_request_unbound');
    const trace=s.g.trace();assert.equal(trace.guardCondition,condition);assert.equal(trace.guardEpoch,'post_seed');
    s.extra(302,{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'});s.response();
    assert.equal(s.g.finish().diagnostic,'candidate_observed');assert.deepEqual(s.g.trace(),trace);
    for(const raw of [...urls,'PRIVATE','ct0','https://','loader='])assert.equal(JSON.stringify(trace).includes(raw),false);
  });
}

test('first guard epoch is immutable across seed, reload, commit and finish',()=>{
  for(const epoch of ['before_seed','post_seed','reload_armed','document_committed']){
    let callback;
    const g=new ManualStartupCookieGuard('frame','session',{beforeSeed:true,onRejection:(reason,trace)=>{callback={reason,trace};}});
    const emit=(method,params)=>g.observe({method,params,sessionId:'session'});
    if(epoch!=='before_seed')g.beginPostSeed();
    if(['reload_armed','document_committed'].includes(epoch))g.armReload('old');
    if(epoch==='document_committed')emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'new',url:'https://x.com/home'}});
    emit('Network.requestWillBeSent',{requestId:'private',frameId:'frame',loaderId:'new',type:'Image',request:{method:'GET',url:urls[1]},redirectHasExtraInfo:false,redirectResponse:{url:urls[0],status:302,headers:{}}});
    const trace=g.trace();assert.equal(trace.guardEpoch,epoch);assert.equal(trace.guardCondition,'predecessor_missing');assert.deepEqual(callback,{reason:'guard_request_unbound',trace});
    if(epoch==='before_seed')g.beginPostSeed();
    if(['before_seed','post_seed'].includes(epoch))g.armReload('old');
    if(epoch!=='document_committed')emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'new',url:'https://x.com/home'}});
    g.finish();assert.deepEqual(g.trace(),trace);assert.deepEqual(callback.trace,trace);
  }
});

test('earlier candidate or ambiguous header rejection keeps its epoch and is not relabeled as a redirect predicate',()=>{
  for(const headers of [{'Set-Cookie':'ct0=PRIVATE_VALUE; Path=/'},{'Set-Cookie':'lang=x, ct0=y'}]){
    const s=fixture();s.request();s.redirect(1,{headers,request:{method:'POST',url:urls[1]}});
    assert.equal(s.g.trace().guardCondition,'unknown');assert.equal(s.g.trace().guardEpoch,'post_seed');
    assert.equal(s.g.rejection(),headers['Set-Cookie'].startsWith('ct0')?'cookie_candidate':'guard_header_classification');
  }
  const s=fixture();s.request(0,{type:'Document',frameId:'foreign'});s.redirect(1,{request:{method:'POST',url:urls[1]},status:304});
  assert.equal(s.g.trace().guardCondition,'previous_resource'); // Predicate order, not a guess at causal importance.
});

test('pre-seed no-cookie redirect failure survives the real binder and diagnostic latch with its original epoch',async()=>{
  const diagnostics=startupFailureLatch();let receipts=0,bodyReads=0;
  const g=new ManualStartupCookieGuard('frame','session',{beforeSeed:true,onRejection:(reason,trace)=>diagnostics.record(startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,{firstCause:reason,...trace}),'reader','bound_finish','observed_cookie_write')});
  const emit=(method,params)=>g.observe({method,params,sessionId:'session'});
  emit('Network.requestWillBeSent',{requestId:'PRIVATE_ID',frameId:'frame',loaderId:'old',type:'Image',request:{method:'GET',url:'https://PRIVATE.invalid/a'}});
  emit('Network.requestWillBeSent',{requestId:'PRIVATE_ID',frameId:'frame',loaderId:'old',type:'Image',request:{method:'GET',url:'https://PRIVATE.invalid/b'},redirectHasExtraInfo:false,redirectResponse:{url:'https://PRIVATE.invalid/a',status:302,headers:{}}});
  g.beginPostSeed();
  const scope={owner:1,profileId:'PRIVATE_PROFILE',sessionId:'session',targetId:'PRIVATE_TARGET'};
  const binder=createManualV2BoundObservation(scope,'frame',async()=>{bodyReads++;throw Error('forbidden');},()=>100,()=>{},
    {deferCookieMutation:false,onPrivateResult(){},onPrivateStartup(){receipts++;}},
    (reason,trace)=>diagnostics.record(startupFailure('reader','bound_finish',reason,false,undefined,trace),'reader','bound_finish',reason),g);
  binder.checkCookieGuard();assert.throws(()=>binder.armManualReload('old'));binder.earlyOutcome('setup_failed',false);
  const report=await binder.finish(true);let detail;
  assert.throws(()=>requireBoundStartup(report,'reader',diagnostics),error=>{detail=startupDiagnostic(error);return true;});
  assert.equal(detail.trace.firstCause,'guard_request_unbound');assert.equal(detail.trace.guardCondition,'previous_origin');assert.equal(detail.trace.guardEpoch,'before_seed');
  assert.equal(detail.trace.terminal,'scope_lost');assert.equal(detail.trace.early,'setup_failed');
  assert.equal(report.manualCookieGuard.candidateAttempts,0);assert.equal(report.manualCookieGuard.otherAttempts,0);
  assert.equal(receipts,0);assert.equal(bodyReads,0);assert.equal(JSON.stringify(detail).includes('PRIVATE'),false);
});
