import test from 'node:test';
import assert from 'node:assert/strict';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {inspectBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';

// Accountless metadata only. No Chrome, profile, transport or Store capability.
const scope={owner:1,profileId:'PRIVATE_PROFILE',sessionId:'PRIVATE_SESSION',targetId:'PRIVATE_TARGET'};
const root='PRIVATE_ROOT',loader='PRIVATE_LOADER',settings='https://api.x.com/1.1/account/settings.json';
const projection=inspectBootstrapCurrentSelf({location:{origin:'https://x.com',pathname:'/home'},
  __INITIAL_STATE__:{session:{user_id:'12345'},entities:{users:{entities:{'12345':{id_str:'12345'}}}}}});
function fixture(){
  let receipts=0,binder;
  binder=createManualV2BoundObservation(scope,root,async id=>{
    assert.equal(binder.capture.consumeRead(id),true);
    return {schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};
  },()=>1,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>receipts++});
  const emit=(method,params)=>binder.observe({method,params,sessionId:scope.sessionId},scope);
  const document=()=>{
    emit('Page.frameNavigated',{frame:{id:root,loaderId:loader,url:'https://x.com/home'}});
    emit('Runtime.executionContextCreated',{context:{id:7,origin:'https://x.com',auxData:{isDefault:true,frameId:root}}});
  };
  const request=(id,url,patch={})=>emit('Network.requestWillBeSent',{requestId:id,loaderId:loader,type:'Image',request:{method:'GET',url},...patch});
  const response=(id,url,headers={},extra=false,patch={})=>emit('Network.responseReceived',{
    requestId:id,loaderId:loader,hasExtraInfo:extra,response:{url,status:200,headers},...patch});
  const extra=(id,headers={},patch={})=>emit('Network.responseReceivedExtraInfo',{requestId:id,statusCode:200,headers,blockedCookies:[],...patch});
  const login=(status=200)=>{
    request('settings',settings,{type:'XHR',frameId:root});
    response('settings',settings,{},false,{frameId:root,response:{url:settings,status,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
    emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
  };
  return {binder,emit,document,request,response,extra,login,identity:()=>binder.acceptIdentity('12345',projection),receipts:()=>receipts};
}
const privateSummary=r=>{
  for(const raw of ['PRIVATE_', '12345','x.com','outside.invalid','auth_token','ct0'])assert.equal(JSON.stringify(r).includes(raw),false,raw);
};
const other={'Set-Cookie':'PRIVATE_OTHER=PRIVATE_VALUE; Priority=High'};

test('known unframed resources produce partial diagnostics without changing strict private startup proof',async()=>{
  for(const url of ['https://x.com/PRIVATE_ASSET','https://outside.invalid/PRIVATE_ASSET'])
    for(const order of ['response','extra-before','extra-after','blocked'])for(const before of [true,false])for(const responseFrame of [undefined,root,'PRIVATE_CHILD']){
      const s=fixture();s.document();if(!before)s.identity();s.request('ordinary',url);
      if(order==='extra-before')s.extra('ordinary',other);
      if(order==='blocked')s.extra('ordinary',{}, {blockedCookies:[{cookieLine:'PRIVATE_OTHER=PRIVATE_VALUE'}]});
      s.response('ordinary',url,order==='response'?other:{},order!=='response',responseFrame===undefined?{}:{frameId:responseFrame});
      if(order==='extra-after')s.extra('ordinary',other);
      if(before)s.identity();s.login();const r=await s.binder.finish(true);
      assert.equal(r.diagnostic,'bound');assert.equal(s.receipts(),1);assert.equal(r.canAuthorize,false);
      assert.equal(r.manualCookieGuard.diagnostic,'other_only');assert.equal(r.manualCookieGuard.otherAttempts,1);
      assert.equal(r.manualCookieGuard.diagnosticCoverage,'partial');assert.equal(r.manualCookieGuard.diagnosticMissingFrameRequests,1);
      assert.equal(r.manualCookieGuard.diagnosticRecordsDropped,0);privateSummary(r.manualCookieGuard);
    }
});

for(const kind of ['candidate','deletion','blocked','ambiguous','malformed'])test(`unframed ${kind} remains rejected before or after diagnostic eviction`,async()=>{
  for(const evicted of [false,true])for(const name of ['auth_token','ct0']){
    const s=fixture();s.document();
    for(let i=0;i<(evicted?300:1);i++)s.request('ordinary-'+i,'https://outside.invalid/PRIVATE_ASSET');
    if(kind==='blocked')s.extra('ordinary-0',{}, {blockedCookies:[{cookieLine:name+'=; Max-Age=0'}]});
    else s.response('ordinary-0','https://outside.invalid/PRIVATE_ASSET',{'Set-Cookie':kind==='candidate'?name+'=PRIVATE_TOKEN':
      kind==='deletion'?name+'=; Max-Age=0':kind==='ambiguous'?'PRIVATE_OTHER=x, '+name+'=PRIVATE_TOKEN':['PRIVATE_VALUE']});
    s.identity();s.login();const r=await s.binder.finish(true);
    assert.equal(r.diagnostic,'observed_cookie_write');assert.equal(s.receipts(),0);
    assert.ok(r.manualCookieGuard.candidateAttempts||r.manualCookieGuard.ambiguousAttempts);privateSummary(r.manualCookieGuard);
  }
});

test('missing frame cannot downgrade Home, Document, settings, unsupported method, missing loader or orphan metadata',async()=>{
  for(const fault of ['home','document','settings','foreign-settings','post','loader','orphan','malformed-frame','malformed-response-frame']){
    const s=fixture();s.document();
    const url=fault==='home'?'https://x.com/home':fault==='settings'?settings:
      fault==='foreign-settings'?'https://outside.invalid/1.1/account/settings.json':'https://outside.invalid/PRIVATE_ASSET';
    if(fault!=='orphan')s.request('ordinary',url,fault==='document'?{type:'Document'}:fault==='post'?{request:{method:'POST',url}}:
      fault==='loader'?{loaderId:undefined}:fault==='malformed-frame'?{frameId:undefined}:{});
    s.response('ordinary',url,other,false,fault==='malformed-response-frame'?{frameId:42}:{});
    s.identity();s.login();const r=await s.binder.finish(true);
    assert.notEqual(r.diagnostic,'bound',fault);assert.equal(s.receipts(),0,fault);privateSummary(r.manualCookieGuard);
  }
});

test('unframed diagnostic IDs never become new root/settings or redirect provenance, including after eviction',async()=>{
  for(const evicted of [false,true])for(const role of ['root','settings','redirect']){
    const s=fixture();s.document();for(let i=0;i<(evicted?300:1);i++)s.request('ordinary-'+i,'https://outside.invalid/PRIVATE_ASSET');
    s.request('ordinary-0',role==='settings'?settings:role==='root'?'https://x.com/home':'https://outside.invalid/next',
      role==='settings'?{type:'XHR',frameId:root}:role==='root'?{type:'Document',frameId:root}:
        {frameId:root,redirectHasExtraInfo:false,redirectResponse:{url:'https://outside.invalid/PRIVATE_ASSET',status:302,headers:{}}});
    s.identity();s.login();const r=await s.binder.finish(true);assert.notEqual(r.diagnostic,'bound');assert.equal(s.receipts(),0);
  }
});

test('partial unframed diagnostics cannot repair failed identity, login, context or cleanup',async()=>{
  for(const fault of ['identity','wrong-identity','login','context','cleanup']){
    const s=fixture();s.document();s.request('ordinary','https://outside.invalid/PRIVATE_ASSET');s.response('ordinary','https://outside.invalid/PRIVATE_ASSET',other);
    if(fault!=='identity')s.identity();if(fault==='wrong-identity')s.binder.acceptIdentity('99999',{});
    s.login(fault==='login'?401:200);if(fault==='context')s.emit('Runtime.executionContextDestroyed',{executionContextId:7});
    const r=await s.binder.finish(fault!=='cleanup');assert.notEqual(r.diagnostic,'bound');assert.equal(s.receipts(),0);
    assert.equal(r.manualCookieGuard.diagnostic,'other_only');assert.equal(r.manualCookieGuard.diagnosticCoverage,'partial');
  }
});

test('unframed diagnostics retain exact response URL/loader/status, declared ExtraInfo and frame-accessor rejection',async()=>{
  let reads=0;
  for(const fault of ['url','loader','status','missing-extra','duplicate-extra','missing-response','frame-getter']){
    const s=fixture();s.document();const url='https://outside.invalid/PRIVATE_ASSET';s.request('ordinary',url);
    if(fault!=='missing-response'){
      const patch=fault==='url'?{response:{url:'https://outside.invalid/other',status:200,headers:other}}:fault==='loader'?{loaderId:'different'}:
        fault==='frame-getter'?Object.defineProperty({},'frameId',{enumerable:true,get(){reads++;throw new Error('PRIVATE');}}):{};
      // Preserve accessor descriptors: spreading here would invoke the getter.
      if(fault==='frame-getter')s.emit('Network.responseReceived',Object.defineProperty({requestId:'ordinary',loaderId:loader,hasExtraInfo:false,response:{url,status:200,headers:other}},'frameId',Object.getOwnPropertyDescriptor(patch,'frameId')));
      else s.response('ordinary',url,other,true,patch);
    }
    if(fault!=='missing-extra'&&fault!=='frame-getter')s.extra('ordinary',fault==='missing-response'?other:{},fault==='status'?{statusCode:201}:{});
    if(fault==='duplicate-extra')s.extra('ordinary');s.identity();s.login();const r=await s.binder.finish(true);
    assert.notEqual(r.diagnostic,'bound',fault);assert.equal(s.receipts(),0,fault);
  }
  assert.equal(reads,0);
});

test('without an owned-session binding a missing frame remains unavailable',()=>{
  const g=new ManualStartupCookieGuard(root);
  g.observe({method:'Network.requestWillBeSent',params:{requestId:'ordinary',type:'Image',loaderId:loader,request:{method:'GET',url:'https://x.com/asset'}}});
  g.observe({method:'Network.responseReceivedExtraInfo',params:{requestId:'ordinary',statusCode:200,headers:other}});
  assert.equal(g.rejection(),'guard_request_unbound');g.finish();
});

test('selected account settings keep mandatory root frame binding even without cookie headers',async()=>{
  for(const fault of ['request','response']){
    const s=fixture();s.document();s.identity();
    s.request('ordinary','https://outside.invalid/PRIVATE_ASSET');s.response('ordinary','https://outside.invalid/PRIVATE_ASSET',other);
    s.request('settings',settings,{type:'XHR',...(fault==='request'?{}:{frameId:root})});
    s.response('settings',settings,{},false,{...(fault==='response'?{}:{frameId:root}),
      response:{url:settings,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
    s.emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
    const r=await s.binder.finish(true);assert.notEqual(r.diagnostic,'bound');assert.equal(s.receipts(),0);
    assert.equal(r.manualCookieGuard.diagnostic,'other_only');assert.equal(r.manualCookieGuard.diagnosticMissingFrameRequests,1);
  }
});

test('later unframed ordinary metadata cannot clear the first session-cookie rejection',()=>{
  const g=new ManualStartupCookieGuard(root,scope.sessionId),emit=(method,params)=>g.observe({method,params,sessionId:scope.sessionId});
  emit('Network.requestWillBeSent',{requestId:'first',type:'Image',loaderId:loader,request:{method:'GET',url:'https://outside.invalid/asset'}});
  emit('Network.responseReceivedExtraInfo',{requestId:'first',statusCode:200,headers:{'Set-Cookie':'ct0=PRIVATE_TOKEN'}});
  const first=g.trace();assert.equal(g.rejection(),'cookie_candidate');assert.equal(first.guardRequestFrame,'absent');
  for(let i=0;i<300;i++)emit('Network.requestWillBeSent',{requestId:'later-'+i,type:'Image',loaderId:loader,request:{method:'GET',url:'https://outside.invalid/asset'}});
  emit('Network.responseReceivedExtraInfo',{requestId:'later-0',statusCode:200,headers:other});
  const r=g.finish();assert.deepEqual(g.trace(),first);assert.equal(g.rejection(),'cookie_candidate');assert.ok(r.candidateAttempts);privateSummary(r);
});
