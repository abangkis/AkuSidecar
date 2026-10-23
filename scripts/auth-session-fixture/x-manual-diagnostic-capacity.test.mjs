import test from 'node:test';
import assert from 'node:assert/strict';
import { ManualStartupCookieGuard } from './x-manual-startup-cookie-guard.mjs';
import { QABoundObservation, createManualV2BoundObservation } from './x-qa-bound-observation.mjs';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';

// Invented offline metadata only. No transport, profile, browser or Store APIs.
const scope={owner:1,profileId:'PRIVATE_PROFILE',sessionId:'PRIVATE_SESSION',targetId:'PRIVATE_TARGET'};
const frame='PRIVATE_FRAME',loader='PRIVATE_LOADER',url='https://outside.invalid/PRIVATE_PATH';
const settingsURL='https://api.x.com/1.1/account/settings.json';
const projection=inspectBootstrapCurrentSelf({location:{origin:'https://x.com',pathname:'/home'},
  __INITIAL_STATE__:{session:{user_id:'12345'},entities:{users:{entities:{'12345':{id_str:'12345'}}}}}});
function fixture(factory=createManualV2BoundObservation){
  let receipts=0,binder;
  binder=factory(scope,frame,async id=>{
    assert.equal(binder.capture.consumeRead(id),true);
    return {schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};
  },()=>1,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>receipts++});
  const emit=(method,params,sessionId=scope.sessionId)=>binder.observe({method,params,sessionId},scope);
  const document=()=>{
    emit('Page.frameNavigated',{frame:{id:frame,loaderId:loader,url:'https://x.com/home'}});
    emit('Runtime.executionContextCreated',{context:{id:7,origin:'https://x.com',auxData:{isDefault:true,frameId:frame}}});
  };
  const request=(id,patch={})=>emit('Network.requestWillBeSent',{requestId:id,frameId:frame,loaderId:loader,type:'Image',request:{method:'GET',url},...patch});
  const response=(id,headers={},extra=false)=>emit('Network.responseReceived',{requestId:id,frameId:frame,loaderId:loader,hasExtraInfo:extra,response:{url,status:200,headers}});
  const extra=(id,headers={})=>emit('Network.responseReceivedExtraInfo',{requestId:id,statusCode:200,headers,blockedCookies:[]});
  const flood=(count,headers=false)=>{for(let i=0;i<count;i++){
    const id='PRIVATE_DIAGNOSTIC_'+i;request(id);
    if(headers){extra(id,{'Set-Cookie':'PRIVATE_OTHER=PRIVATE_VALUE; Path=/'});response(id,{},true);}
  }};
  const settings=()=>{
    request('PRIVATE_SETTINGS',{type:'XHR',request:{method:'GET',url:settingsURL}});
    emit('Network.responseReceived',{requestId:'PRIVATE_SETTINGS',frameId:frame,loaderId:loader,hasExtraInfo:false,
      response:{url:settingsURL,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
    emit('Network.loadingFinished',{requestId:'PRIVATE_SETTINGS',encodedDataLength:32});
  };
  return {binder,emit,document,request,response,extra,flood,settings,identity:()=>binder.acceptIdentity('12345',projection),receipts:()=>receipts};
}
const privateSummary=report=>{
  for(const raw of ['PRIVATE_', 'outside.invalid','12345','auth_token','ct0'])assert.equal(JSON.stringify(report).includes(raw),false,raw);
};

for(const timing of ['before_identity','after_identity'])for(const headers of [false,true])
test(`diagnostic requests and header saturation preserve independent private receipt: ${timing}/${headers}`,async()=>{
  const s=fixture();s.document();if(timing==='after_identity')s.identity();
  s.flood(1800,headers); // More than the former request and binder-event caps.
  if(timing==='before_identity')s.identity();s.settings();const r=await s.binder.finish(true);
  assert.equal(r.diagnostic,'bound');assert.equal(s.receipts(),1);assert.equal(r.canAuthorize,false);
  assert.equal(r.manualCookieGuard.diagnosticCoverage,'partial');assert.ok(r.manualCookieGuard.diagnosticRecordsDropped>0);
  assert.equal(r.manualCookieGuard.ambiguousAttempts,0);
  assert.equal(r.manualCookieGuard.diagnosticHeadersDropped>0,headers);privateSummary(r.manualCookieGuard);
});

for(const kind of ['candidate','blocked_deletion','ambiguous','malformed'])
test(`evicted diagnostic record still inspects every observed header: ${kind}`,async()=>{
  const s=fixture();s.document();s.flood(300,true);s.identity();
  const id='PRIVATE_DIAGNOSTIC_0';
  if(kind==='blocked_deletion')s.emit('Network.responseReceivedExtraInfo',{requestId:id,statusCode:200,headers:{},blockedCookies:[{cookieLine:'auth_token=; Max-Age=0'}]});
  else s.response(id,{'Set-Cookie':kind==='candidate'?'ct0=PRIVATE_TOKEN':kind==='ambiguous'?'PRIVATE_OTHER=x, ct0=PRIVATE_TOKEN':['PRIVATE_VALUE']});
  s.settings();const r=await s.binder.finish(true);assert.equal(s.receipts(),0);assert.equal(r.diagnostic,'observed_cookie_write');
  assert.ok(r.manualCookieGuard.candidateAttempts||r.manualCookieGuard.ambiguousAttempts);privateSummary(r.manualCookieGuard);
});

for(const adoption of ['settings','root','redirect'])test(`discarded diagnostic request cannot be rebound into ${adoption}`,async()=>{
  const s=fixture();s.document();s.flood(300);s.identity();
  s.request('PRIVATE_DIAGNOSTIC_0',adoption==='settings'?{type:'XHR',request:{method:'GET',url:settingsURL}}:
    adoption==='root'?{type:'Document',request:{method:'GET',url:'https://x.com/home'}}:
      {redirectHasExtraInfo:false,redirectResponse:{url,status:302,headers:{}}});
  assert.equal(s.binder.capture.chosen,null);s.settings();const r=await s.binder.finish(true);assert.equal(s.receipts(),1);assert.equal(r.diagnostic,'bound');
  assert.equal(r.manualCookieGuard.ambiguousAttempts,0);privateSummary(r.manualCookieGuard);
});

test('discarded and orphan ordinary headers remain partial and nonauthorizing',async()=>{
  const s=fixture();s.document();s.flood(300);s.response('PRIVATE_DIAGNOSTIC_0',{'Set-Cookie':'PRIVATE_OTHER=PRIVATE_VALUE'});
  s.identity();s.settings();const r=await s.binder.finish(true);assert.equal(r.diagnostic,'bound');assert.equal(s.receipts(),1);
  assert.equal(r.manualCookieGuard.diagnosticCoverage,'partial');assert.equal(r.manualCookieGuard.otherAttempts,1);
  assert.equal(r.manualCookieGuard.diagnosticRecordsDropped,45); // Late headers are not extra record evictions.
  const orphan=fixture();orphan.document();orphan.flood(300);orphan.response('NEVER_OBSERVED',{'Set-Cookie':'PRIVATE_OTHER=PRIVATE_VALUE'});
  orphan.identity();orphan.settings();const rejected=await orphan.binder.finish(true);assert.equal(orphan.receipts(),1);assert.equal(rejected.diagnostic,'bound');
});

test('diagnostic eviction cannot replace first guard cause or epoch with a later candidate or exhaustion',()=>{
  const g=new ManualStartupCookieGuard(frame,scope.sessionId),emit=(method,params)=>g.observe({method,params,sessionId:scope.sessionId});
  for(let i=0;i<300;i++)emit('Network.requestWillBeSent',{requestId:'diag-'+i,frameId:frame,loaderId:loader,type:'Image',request:{method:'GET',url}});
  emit('Network.responseReceivedExtraInfo',{requestId:'diag-0',statusCode:200,headers:{'Set-Cookie':'bad name=PRIVATE_VALUE'}});
  const first=g.trace();assert.equal(g.rejection(),'guard_header_classification');assert.equal(first.guardEpoch,'post_seed');
  emit('Network.responseReceivedExtraInfo',{requestId:'diag-0',statusCode:200,headers:{'Set-Cookie':'ct0=PRIVATE_TOKEN'}});
  g.armReload('old');for(let i=0;i<1281;i++)emit('Network.requestWillBeSent',{requestId:'security-'+i,frameId:frame,loaderId:loader,type:'Document',request:{method:'GET',url:'https://x.com/home'}});
  const r=g.finish();assert.deepEqual(g.trace(),first);assert.ok(r.candidateAttempts);privateSummary(r);
});

test('diagnostic history has a finite exact-ID coverage bound and cannot silently discard no-adoption markers',()=>{
  const g=new ManualStartupCookieGuard(frame,scope.sessionId),request=i=>g.observe({method:'Network.requestWillBeSent',sessionId:scope.sessionId,
    params:{requestId:'diag-'+i,frameId:frame,loaderId:loader,type:'Image',request:{method:'GET',url}}});
  for(let i=0;i<16384;i++)request(i);assert.equal(g.rejection(),'unknown');request(16384);
  assert.equal(g.rejection(),'guard_request_budget');assert.equal(g.finish().diagnostic,'ambiguous');
});

test('foreign sessions cannot consume Manual V2 event budget or supply selected source events',async()=>{
  const s=fixture();s.document();s.identity();s.settings();
  for(let i=0;i<5000;i++)s.emit('Network.requestWillBeSent',{requestId:'PRIVATE_SETTINGS',request:{method:'GET',url:settingsURL}},'FOREIGN_SESSION');
  const r=await s.binder.finish(true);assert.equal(r.diagnostic,'bound');assert.equal(s.receipts(),1);
});

test('Manual V2 identity/lifecycle budget and default QA total event budget still fail closed',async()=>{
  for(const factory of [createManualV2BoundObservation,(...args)=>new QABoundObservation(...args)]){
    const s=fixture(factory);s.document();s.identity();
    for(let i=0;i<4097;i++)s.emit('Page.loadEventFired',{});
    s.settings();const r=await s.binder.finish(true);assert.equal(r.diagnostic,'scope_lost');assert.equal(s.receipts(),0);
  }
});

for(const security of ['root','settings','redirect'])test(`diagnostic eviction preserves the distinct completion authority of ${security}`,()=>{
  const g=new ManualStartupCookieGuard(frame,scope.sessionId),emit=(method,params)=>g.observe({method,params,sessionId:scope.sessionId});
  if(security==='root')g.armReload('old');
  const sourceURL=security==='settings'?settingsURL:'https://x.com/PRIVATE_ASSET';
  emit('Network.requestWillBeSent',{requestId:'protected',frameId:frame,loaderId:loader,type:security==='root'?'Document':security==='settings'?'XHR':'Image',
    request:{method:'GET',url:security==='root'?'https://x.com/home':sourceURL}});
  if(security==='root')emit('Page.frameNavigated',{frame:{id:frame,loaderId:loader,url:'https://x.com/home'}});
  if(security==='redirect')emit('Network.requestWillBeSent',{requestId:'protected',frameId:frame,loaderId:loader,type:'Image',request:{method:'GET',url:'https://api.x.com/PRIVATE_ASSET'},
    redirectHasExtraInfo:false,redirectResponse:{url:sourceURL,status:302,headers:{}}});
  else emit('Network.responseReceived',{requestId:'protected',frameId:frame,loaderId:loader,hasExtraInfo:true,
    response:{url:security==='root'?'https://x.com/home':sourceURL,status:200,headers:{'Set-Cookie':'PRIVATE_OTHER=PRIVATE_VALUE'}}});
  for(let i=0;i<1800;i++)emit('Network.requestWillBeSent',{requestId:'diag-'+i,frameId:frame,loaderId:loader,type:'Image',request:{method:'GET',url}});
  const r=g.finish();assert.equal(g.rejection(),security==='root'?'guard_completion_missing':'unknown');assert.equal(r.diagnosticCoverage,'partial');assert.equal(Boolean(r.ambiguousAttempts),security==='root');privateSummary(r);
});
