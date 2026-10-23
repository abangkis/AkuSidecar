import test from 'node:test';
import assert from 'node:assert/strict';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {inspectBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';
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


test('ordinary redirect headers are classified independently of hop metadata and completion',()=>{
  for(const fault of ['orphan','reused','frame','loader','url','status','cache','duplicate-extra','missing-response','missing-extra']){
    const s=fixture();
    if(fault!=='orphan')s.request();
    if(fault==='reused')s.request();
    s.redirect(1,{headers:other,extra:true,...(fault==='frame'?{frameId:null}:fault==='loader'?{loaderId:''}:
      fault==='url'?{request:{method:'PRIVATE_METHOD',url:'not-a-url'}}:
      fault==='cache'?{redirectResponse:{url:urls[0],status:302,headers:other,fromServiceWorker:true}}:{})});
    if(fault!=='missing-extra')s.extra(fault==='status'?999:302);
    if(fault==='duplicate-extra')s.extra(302);
    if(fault!=='missing-response')s.response();
    assert.equal(s.g.canSelectRequest('chain'),false,fault);accepted(s,1);
  }
});

test('orphan and reused IDs remain permanently ineligible across seed and diagnostic eviction',()=>{
  for(const orphan of [false,true])for(const evicted of [false,true]){
    const s=fixture({beforeSeed:true});
    if(orphan)s.extra(200,other);else s.request();
    s.g.beginPostSeed();
    if(evicted)for(let i=0;i<300;i++)s.request(0,{requestId:'other-'+i});
    s.request();assert.equal(s.g.canSelectRequest('chain'),false);
    s.extra(200,other);accepted(s,1);
  }
});

test('pinned root redirects and reused IDs fail independently of cookie content',()=>{
  for(const redirect of [false,true])for(const headers of [{},other]){
    const s=fixture();s.g.armReload('old');
    s.request(0,{type:'Document',request:{method:'GET',url:'https://x.com/home'}});
    s.request(1,redirect?{redirectResponse:{url:'https://x.com/home',status:302,headers}}:{});
    assert.equal(s.g.rejection(),'guard_request_unbound');assert.equal(s.g.failureReason(),'scope_lost');s.g.finish();
  }
});

test('descriptor-only diagnostics never execute optional metadata accessors',()=>{
  for(const key of ['frameId','loaderId','request','type']){
    const s=fixture();let reads=0;
    const p={requestId:'chain'};Object.defineProperty(p,key,{get(){reads++;throw Error('PRIVATE');}});
    s.emit('Network.requestWillBeSent',p);s.extra(200,other);accepted(s,1);assert.equal(reads,0);
  }
});

test('header inspection failures and exact-ID exhaustion remain sticky across seed and later candidates',()=>{
  for(const first of ['envelope','history']){
    const s=fixture({beforeSeed:true});
    if(first==='envelope')s.emit('Network.responseReceived',null);
    else for(let i=0;i<16385;i++)s.request(0,{requestId:'record-'+i});
    const trace=s.g.trace();assert.equal(trace.guardEpoch,'before_seed');
    s.g.beginPostSeed();s.extra(200,{'Set-Cookie':'ct0=PRIVATE'});
    assert.equal(s.g.finish().diagnostic,'candidate_observed');assert.deepEqual(s.g.trace(),trace);
  }
});

const settingsURL='https://api.x.com/1.1/account/settings.json';
function boundFixture(){
  const scope={owner:1,profileId:'private-profile',sessionId:'session',targetId:'private-target'};
  let binder,receipts=0,reads=0;
  binder=createManualV2BoundObservation(scope,'frame',async id=>{
    reads++;assert.equal(id,'selected');assert.equal(binder.capture.consumeRead(id),true);
    return {schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};
  },()=>1,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>receipts++});
  const emit=(method,params)=>binder.observe({method,params,sessionId:'session'},scope);
  emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'loader',url:'https://x.com/home'}});
  emit('Runtime.executionContextCreated',{context:{id:7,origin:'https://x.com',auxData:{isDefault:true,frameId:'frame'}}});
  const identity=()=>binder.acceptIdentity('12345',inspectBootstrapCurrentSelf({location:{origin:'https://x.com',pathname:'/home'},
    __INITIAL_STATE__:{session:{user_id:'12345'},entities:{users:{entities:{'12345':{id_str:'12345'}}}}}}));
  const request=(id='selected',patch={})=>emit('Network.requestWillBeSent',{requestId:id,frameId:'frame',loaderId:'loader',type:'XHR',request:{method:'GET',url:settingsURL},...patch});
  const response=(id='selected',patch={})=>{
    emit('Network.responseReceived',{requestId:id,frameId:'frame',loaderId:'loader',hasExtraInfo:false,
      response:{url:settingsURL,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false},...patch});
    emit('Network.loadingFinished',{requestId:id,encodedDataLength:32});
  };
  return {binder,emit,identity,request,response,receipts:()=>receipts,reads:()=>reads};
}

const unqualified=[
  {frameId:undefined},{frameId:null},{frameId:'child'},{loaderId:''},{loaderId:'old'},
  {type:'Document',frameId:undefined,request:{method:'GET',url:'https://x.com/home'}},
  {request:{method:'POST',url:settingsURL}}, {request:{method:'GET',url:'not-a-url'}},
  {requestId:null},{requestId:undefined},{request:Object.assign([],{method:'GET',url:settingsURL})},
];
for(const withProof of [false,true])test(`unqualified protected-source headers cannot supply or interfere with independent proof: ${withProof}`,async()=>{
  for(const beforeIdentity of [false,true])for(const patch of unqualified){
    const s=boundFixture();if(!beforeIdentity)s.identity();
    s.request('unqualified',patch);const id=Object.hasOwn(patch,'requestId')?patch.requestId:'unqualified';
    s.emit('Network.responseReceivedExtraInfo',{requestId:id,statusCode:200,headers:other});
    s.emit('Network.responseReceived',{requestId:id,frameId:'frame',loaderId:'loader',hasExtraInfo:false,response:{url:settingsURL,status:200,headers:{}}});
    s.emit('Network.loadingFinished',{requestId:id,encodedDataLength:32});
    assert.equal(s.binder.capture.chosen,null);assert.equal(s.reads(),0);
    if(beforeIdentity)s.identity();
    if(withProof){s.request();s.response();}
    const r=await s.binder.finish(true);
    assert.equal(r.diagnostic,withProof?'bound':'source_unavailable',JSON.stringify(patch)+' '+JSON.stringify(r.manualCookieGuard));assert.equal(s.receipts(),withProof?1:0);
    assert.equal(r.manualCookieGuard.diagnostic,'other_only');assert.equal(r.canAuthorize,false);
  }
});

test('unqualified settings flood cannot consume the independent lifecycle/account budget',async()=>{
  const s=boundFixture();s.identity();
  for(let i=0;i<5000;i++)s.request('unqualified-'+i,{frameId:undefined});
  s.request();s.response();assert.equal((await s.binder.finish(true)).diagnostic,'bound');assert.equal(s.receipts(),1);
});

test('ordinary metadata never repairs a selected account failure',async()=>{
  for(const fault of ['frame','loader','extra','duplicate','cache','status','context']){
    const s=boundFixture();s.identity();s.request();
    if(fault==='duplicate')s.request();
    s.response('selected',fault==='frame'?{frameId:'child'}:fault==='loader'?{loaderId:'other'}:fault==='extra'?{hasExtraInfo:true}:
      ['cache','status'].includes(fault)?{response:{url:settingsURL,status:fault==='status'?401:200,headers:{},mimeType:'application/json',fromDiskCache:fault==='cache',fromServiceWorker:false,fromPrefetchCache:false}}:{});
    if(fault==='context')s.emit('Runtime.executionContextDestroyed',{executionContextId:7});
    s.request('ordinary',{frameId:undefined});s.emit('Network.responseReceivedExtraInfo',{requestId:'ordinary',headers:other,statusCode:200});
    const r=await s.binder.finish(true);assert.notEqual(r.diagnostic,'bound',fault);assert.equal(s.receipts(),0,fault);
  }
});

test('an orphan or previously ignored settings ID cannot be adopted even with a later complete tuple',async()=>{
  for(const orphan of [false,true])for(const evicted of [false,true]){
    const s=boundFixture();s.identity();
    if(orphan)s.emit('Network.responseReceivedExtraInfo',{requestId:'selected',headers:other,statusCode:200});
    else s.request('selected',{frameId:undefined});
    if(evicted)for(let i=0;i<300;i++)s.request('ignored-'+i,{frameId:undefined});
    s.request();s.response();assert.equal(s.reads(),0);assert.equal(s.binder.capture.chosen,null);
    assert.equal((await s.binder.finish(true)).diagnostic,'source_unavailable');assert.equal(s.receipts(),0);
  }
});

test('root proof and header safety retain distinct sticky public reasons through the receipt gate',async()=>{
  for(const first of ['root','header']){
    const scope={owner:1,profileId:'p',sessionId:'session',targetId:'t'},g=new ManualStartupCookieGuard('frame','session');
    const emit=(method,params)=>g.observe({method,params,sessionId:'session'});
    g.armReload('old');
    if(first==='header')emit('Network.responseReceivedExtraInfo',{requestId:'orphan',headers:{'Set-Cookie':'ct0=PRIVATE'},statusCode:200});
    emit('Network.requestWillBeSent',{requestId:'root',type:'Document',frameId:'frame',loaderId:'loader',request:{method:'POST',url:'https://x.com/home'}});
    const cause=g.rejection(),trace=g.trace();
    if(first==='root')emit('Network.responseReceivedExtraInfo',{requestId:'orphan',headers:{'Set-Cookie':'ct0=PRIVATE'},statusCode:200});
    const binder=createManualV2BoundObservation(scope,'frame',async()=>assert.fail('body unavailable'),()=>1,undefined,
      {deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>assert.fail('receipt unavailable')},()=>{},g);
    binder.checkCookieGuard();const report=await binder.finish(true);
    const expected=first==='root'?'scope_lost':'observed_cookie_write';
    assert.equal(report.diagnostic,expected);assert.deepEqual(g.trace(),trace);
    assert.throws(()=>requireBoundStartup(report,'reader',startupFailureLatch()),error=>{
      const d=startupDiagnostic(error);assert.equal(d.reason,expected);assert.equal(d.trace.firstCause,cause);return true;
    });
  }
});
