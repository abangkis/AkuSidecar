import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation,QA_BOUND_BOOTSTRAP_EXPRESSION,projectBoundBootstrap} from './x-qa-bound-observation.mjs';
import {BootstrapHandoffCapture} from './x-bootstrap-breakpoint-handoff.mjs';
import {manualReloadFrameProjection} from './x-native-metadata-probe-driver.mjs';
import {startupFailureLatch,startupFailure,requireBoundStartup,startupDiagnostic} from './x-manual-startup-diagnostic.mjs';

const frame='private-root',session='private-session',oldLoader='old-loader',next='new-loader',home='https://x.com/home';
const rootRequest=(patch={})=>({requestId:'root',frameId:frame,loaderId:next,type:'Document',request:{method:'GET',url:home},...patch});
const rootResponse=(headers={},patch={})=>({requestId:'root',frameId:frame,loaderId:next,hasExtraInfo:true,response:{url:home,status:200,headers},...patch});
const rootExtra=(headers={},patch={})=>({requestId:'root',statusCode:200,headers,...patch});
function guardFixture(){
  const g=new ManualStartupCookieGuard(frame,session);let loader=null,context=null;
  const emit=(method,params,source=session)=>g.observe({method,params,sessionId:source},loader,context);
  return {g,emit,arm:()=>g.armReload(oldLoader),request:patch=>emit('Network.requestWillBeSent',rootRequest(patch)),
    response:(headers,patch)=>emit('Network.responseReceived',rootResponse(headers,patch)),extra:(headers,patch)=>emit('Network.responseReceivedExtraInfo',rootExtra(headers,patch)),
    commit:(value=next)=>{loader=value;return emit('Page.frameNavigated',{frame:{id:frame,loaderId:value,url:home}});},
    context:()=>{context=1;return emit('Runtime.executionContextCreated',{context:{id:1,origin:'https://x.com',auxData:{isDefault:true,frameId:frame}}});}};
}
test('one armed root document accepts bounded noncandidate names and attributes before commit or context',()=>{
  const lines=['lang=en; Path=/','unknown_private=PRIVATE_VALUE; Domain=foreign.invalid; Path=/PRIVATE_PATH; Priority=High; Partitioned; Expires=unusual',
    'lang=x; auth_token=attribute-text; ct0=attribute-text'];
  for(const line of lines)for(const order of ['before-commit','before-context','after-context'])for(const extraFirst of [true,false]){
    const s=guardFixture();s.arm();
    if(order!=='before-commit')s.commit();if(order==='after-context')s.context();
    assert.equal(s.request(),false);
    if(extraFirst)assert.equal(s.extra({'Set-Cookie':line}),false);
    assert.equal(s.response(extraFirst?{}:{'Set-Cookie':line}),false);
    if(!extraFirst)assert.equal(s.extra(),false);
    if(order==='before-commit')s.commit();if(order!=='after-context')s.context();
    const r=s.g.finish();assert.equal(r.diagnostic,'other_only');assert.equal(r.otherAttempts,1);
    for(const raw of [home,'lang','en','unknown_private','PRIVATE_VALUE','foreign.invalid','PRIVATE_PATH','Priority','Partitioned','auth_token','ct0',frame,next])assert.equal(JSON.stringify(r).includes(raw),false);
  }
});
test('candidate attempts revoke immediately at every provisional phase, including blocked/delete attempts',()=>{
  for(const phase of ['before-commit','before-context','after-context'])for(const name of ['auth_token','ct0'])for(const suffix of ['=PRIVATE; Path=/','=; Max-Age=0; Path=/','=; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Path=/'])for(const blocked of [false,true]){
    const s=guardFixture();s.arm();if(phase!=='before-commit')s.commit();if(phase==='after-context')s.context();s.request();
    const rejected=blocked?s.extra({}, {blockedCookies:[{cookieLine:name+suffix,blockedReasons:['private']}]}) : s.response({'Set-Cookie':name+suffix});
    assert.equal(rejected,true);assert.equal(s.g.finish().diagnostic,'candidate_observed');
  }
});
test('comma-folded cookie pairs never wait for provisional binding',()=>{
  for(const line of ['lang=x, ct0=y; Path=/','lang=x; Priority=Low, auth_token=y']){
    const s=guardFixture();s.arm();s.request();assert.equal(s.extra({'Set-Cookie':line}),true);assert.equal(s.g.finish().diagnostic,'ambiguous');
  }
});
test('root network provenance requires commit, response and required ExtraInfo independently of JS context',()=>{
  for(const missing of ['commit','response','extra']){
    const s=guardFixture();s.arm();s.request();
    if(missing!=='response')s.response({'Set-Cookie':'lang=en; Path=/'});
    if(missing!=='extra')s.extra(missing==='response'?{'Set-Cookie':'lang=en; Path=/'}:{});
    if(missing!=='commit')s.commit();if(missing!=='context')s.context();
    assert.equal(s.g.finish().diagnostic,'ambiguous',missing);
  }
});
test('old, pre-arm, reused, redirected, child, foreign and contradictory root metadata is never adopted',()=>{
  for(const fault of ['old-loader','pre-arm','extra-before-request','duplicate','redirect','child','url','foreign-session','second-root','response-url','response-frame','status','extra-status','duplicate-extra','extra-false','loader-drift']){
    const s=guardFixture();if(fault==='pre-arm')s.request();s.arm();
    if(fault==='extra-before-request')assert.equal(s.extra({'Set-Cookie':'lang=en; Path=/'}),true);
    const patch=fault==='old-loader'?{loaderId:oldLoader}:fault==='redirect'?{redirectResponse:{headers:{'Set-Cookie':'lang=en; Path=/'}}}:fault==='child'?{frameId:'child'}:fault==='url'?{request:{method:'GET',url:'https://x.com/home?private=1'}}:{};
    if(fault==='foreign-session')s.emit('Network.requestWillBeSent',rootRequest(), 'foreign-session');else s.request(patch);
    if(fault==='duplicate')s.request({type:'XHR'});if(fault==='second-root')s.request({requestId:'second'});
    s.response({'Set-Cookie':'lang=en; Path=/'},fault==='response-url'?{response:{url:'https://x.com/other',status:200,headers:{'Set-Cookie':'lang=en; Path=/'}}}:fault==='response-frame'?{frameId:'child'}:fault==='status'?{response:{url:home,status:302,headers:{'Set-Cookie':'lang=en; Path=/'}}}:fault==='extra-false'?{hasExtraInfo:false}:{});
    s.extra({},fault==='extra-status'?{statusCode:201}:{});if(fault==='duplicate-extra')s.extra();s.commit(fault==='loader-drift'?'different-loader':next);s.context();
    assert.equal(s.g.finish().diagnostic,'ambiguous',fault);
  }
});
test('arming is single use and requires a predecessor; earlier observed documents remain network provenance',()=>{
  const repeated=guardFixture();repeated.arm();assert.throws(()=>repeated.arm());assert.equal(repeated.g.finish().diagnostic,'ambiguous');
  const missing=guardFixture();assert.throws(()=>missing.g.armReload(''));
  const adopted=guardFixture();adopted.commit();assert.doesNotThrow(()=>adopted.arm());
  const noParams=guardFixture();noParams.arm();
  assert.equal(noParams.emit('Debugger.resumed'),false);assert.equal(noParams.emit('Runtime.executionContextsCleared'),false);
  assert.equal(noParams.g.finish().diagnostic,'not_observed');
});

async function handoffFixture({role='reader',order='before-commit',cookie='lang=en; Path=/',fault,resourceMode}={}){
  const scope={owner:role==='reader'?1:2,profileId:'private-profile-'+role,sessionId:session,targetId:'private-target-'+role},diagnostics=startupFailureLatch();
  let binder,capture,index=0,phase='instrumentation',normal=null,instrumentation=true,evaluations=0,receipts=0,settings=false;
  const context=vm.createContext({location:{origin:'https://x.com',pathname:'/home'}});
  const scripts=['globalThis.__INITIAL_STATE__={session:{user_id:"123456"},entities:{users:{entities:{"123456":{id_str:"123456"}}}}}','void 0','delete globalThis.__INITIAL_STATE__','void 0'];
  const emit=(method,params,binding=scope)=>binder.observe({method,params,sessionId:session},binding);
  const commit=()=>{emit('Page.frameNavigated',{frame:{id:frame,loaderId:next,url:home}});capture.observe({type:'frame_navigated',frameId:frame,isHome:true});};
  const created=()=>{emit('Runtime.executionContextCreated',{context:{id:1,origin:'https://x.com',auxData:{isDefault:true,frameId:frame}}});capture.observe({type:'context_created',frameId:frame,contextId:1,isDefault:true});};
  const root=(line=cookie)=>{emit('Network.requestWillBeSent',rootRequest());emit('Network.responseReceivedExtraInfo',rootExtra(line?{'Set-Cookie':line}:{}));emit('Network.responseReceived',rootResponse());};
  const assetRequest=()=>emit('Network.requestWillBeSent',{requestId:'asset',frameId:frame,loaderId:next,type:'Image',request:{method:'GET',url:'https://x.com/asset'}});
  const assetResponse=()=>{
    emit('Network.responseReceived',{requestId:'asset',frameId:frame,loaderId:next,hasExtraInfo:true,response:{url:'https://x.com/asset',status:200,headers:cookie?{'Set-Cookie':cookie}:{}}});
    emit('Network.responseReceivedExtraInfo',{requestId:'asset',statusCode:200,headers:{}});
  };
  const source=()=>{
    const url='https://api.x.com/1.1/account/settings.json';
    emit('Network.requestWillBeSent',{requestId:'settings',frameId:frame,loaderId:next,request:{method:'GET',url}});
    emit('Network.responseReceived',{requestId:'settings',frameId:frame,loaderId:next,hasExtraInfo:false,response:{url,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
    emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
  };
  binder=createManualV2BoundObservation(scope,frame,async id=>{assert.equal(binder.capture.consumeRead(id),true);return{schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};},undefined,undefined,
    {deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>{receipts++;}},
    (reason,trace)=>diagnostics.record(startupFailure(role,'bound_finish',reason,false,undefined,trace),role,'bound_finish',reason));
  const pause=(ordinary=false)=>{
    phase=ordinary?'normal':'instrumentation';
    capture.observe({type:'script_parsed',scriptId:'private-script-'+index,contextId:1,frameId:frame,isDefault:true});
    capture.observe({type:'paused',reason:ordinary?'other':'instrumentation',callFrameId:'private-call',scriptId:'private-script-'+index},
      {hitBreakpoints:ordinary?[normal]:[],callFrames:[{location:{scriptId:'private-script-'+index,lineNumber:0,columnNumber:0}}]});
  };
  const execute=()=>{if(index<scripts.length)vm.runInContext(scripts[index++],context);if(index<scripts.length&&instrumentation)pause();else while(index<scripts.length)vm.runInContext(scripts[index++],context);};
  capture=new BootstrapHandoffCapture({boundIdentity:true,beforeIdentityRead:id=>{
    if(fault==='context')emit('Runtime.executionContextDestroyed',{executionContextId:id});
    if(fault==='owner')emit('Page.loadEventFired',{}, {...scope,targetId:'foreign-target'});
    return binder.startupPins(id);
  },send:async(method,params,purpose)=>{
    if(method==='Page.getFrameTree')return manualReloadFrameProjection({frameId:frame,loaderId:oldLoader},frame,oldLoader);
    if(method==='Page.reload'){
      binder.armManualReload(oldLoader);
      if(resourceMode){
        root(null);commit();
        if(fault==='resource-budget')for(let i=0;i<1280;i++)emit('Network.requestWillBeSent',{requestId:'foreign-'+i,frameId:frame,loaderId:next,type:'Image',request:{method:'GET',url:'https://foreign.invalid/asset'}});
        if(resourceMode!=='after-context')assetRequest();if(resourceMode==='response-before-context')assetResponse();created();
        if(resourceMode==='after-context')assetRequest();if(resourceMode!=='response-before-context')assetResponse();
      }else{if(order==='before-commit')root();commit();if(order==='before-context')root();created();if(order==='after-context')root();}
      pause();
    }
    if(method==='Debugger.setInstrumentationBreakpoint')return{breakpointId:'private-instrumentation'};
    if(method==='Debugger.setBreakpoint'){normal='private-normal-'+index;capture.handoff.created(normal);return{breakpointId:normal,actualLocation:{...params.location}};}
    if(method==='Debugger.removeBreakpoint'){if(purpose==='normal_remove')normal=null;else instrumentation=false;}
    if(method==='Debugger.resume'){capture.observe({type:'resumed'});if(normal&&phase==='instrumentation')pause(true);else execute();}
    if(method==='Debugger.evaluateOnCallFrame'){
      assert.equal(phase,'normal');assert.equal(params.expression,QA_BOUND_BOOTSTRAP_EXPRESSION);assert.equal(params.throwOnSideEffect,true);evaluations++;
      const projected=projectBoundBootstrap(JSON.parse(JSON.stringify(vm.runInContext(params.expression,context))),(id,p)=>binder.acceptIdentity(id,p));
      if(binder.identityReady()&&!settings){settings=true;source();}return projected;
    }
    return{acknowledged:true};
  }});
  const early=await capture.run(),handoff=capture.diagnostics();binder.earlyOutcome(early.stopReason,handoff.handoffRejected);
  const observation=await binder.finish(early.cleanupVerified&&!handoff.handoffRejected);
  return{early,observation,receipts,evaluations,diagnostics};
}
for(const role of ['reader','collector'])test(`${role} actual handoff accepts root other-cookie ordering and exact capture-facing frame projection`,async()=>{
  for(const order of ['before-commit','before-context','after-context']){
    const r=await handoffFixture({role,order});assert.equal(r.early.stopReason,'captured');assert.equal(r.early.cleanupVerified,true);
    assert.equal(r.observation.diagnostic,'bound');assert.equal(r.observation.manualCookieGuard.diagnostic,'other_only');assert.equal(r.receipts,1);assert.ok(r.evaluations>0);
    assert.equal(r.observation.continuity,'unknown');assert.equal(r.observation.canAuthorize,false);
  }
});

function resourceFixture(){
  const s=guardFixture();
  const request=(patch={})=>s.emit('Network.requestWillBeSent',{requestId:'asset',frameId:frame,loaderId:next,type:'Image',request:{method:'GET',url:'https://x.com/asset'},...patch});
  const response=(headers={},patch={})=>s.emit('Network.responseReceived',{requestId:'asset',frameId:frame,loaderId:next,hasExtraInfo:true,response:{url:'https://x.com/asset',status:200,headers},...patch});
  const extra=(headers={},patch={})=>s.emit('Network.responseReceivedExtraInfo',{requestId:'asset',statusCode:200,headers,...patch});
  const ready=()=>{s.arm();s.request();s.response({}, {hasExtraInfo:false});s.commit();};
  return {...s,assetRequest:request,assetResponse:response,assetExtra:extra,ready};
}
test('same-document provisional GET resource may finish before or after its matching default context',()=>{
  for(const contextFirst of [false,true])for(const extraFirst of [false,true]){
    const s=resourceFixture();s.ready();assert.equal(s.assetRequest(),false);if(contextFirst)s.context();
    if(extraFirst)assert.equal(s.assetExtra({'Set-Cookie':'lang=en; Path=/'}),false);
    assert.equal(s.assetResponse(extraFirst?{}:{'Set-Cookie':'lang=en; Path=/'}),false);if(!extraFirst)s.assetExtra();
    if(!contextFirst)s.context();assert.equal(s.g.finish().diagnostic,'other_only');assert.equal(s.g.rejection(),'unknown');
  }
});
test('network cookie attempt without response or ExtraInfo fails closed with completion reason',()=>{
  for(const missing of ['response','extra']){
    const s=resourceFixture();s.ready();s.assetRequest();if(missing!=='response')s.assetResponse({'Set-Cookie':'lang=en; Path=/'});
    if(missing!=='extra')s.assetExtra(missing==='response'?{'Set-Cookie':'lang=en; Path=/'}:{});if(missing!=='context')s.context();
    assert.equal(s.g.finish().diagnostic,'ambiguous');assert.equal(s.g.rejection(),'guard_completion_missing');
  }
});
test('network provenance preserves rejected scope and redirects without context-based admission islands',()=>{
  for(const fault of ['frame','foreign-session','foreign-url','reuse','redirect']){
    const s=resourceFixture();s.ready();
    if(fault==='foreign-session')s.emit('Network.requestWillBeSent',{requestId:'asset',frameId:frame,loaderId:next,request:{method:'GET',url:'https://x.com/asset'}},'foreign');
    else s.assetRequest(fault==='frame'?{frameId:'child'}:fault==='foreign-url'?{request:{method:'GET',url:'https://foreign.invalid/asset'}}:fault==='redirect'?{redirectResponse:{headers:{}}}:{});
    if(fault==='reuse')s.assetRequest();s.context();assert.equal(s.assetResponse({'Set-Cookie':'lang=en; Path=/'}),true);s.assetExtra();
    assert.equal(s.g.rejection(),fault==='frame'?'guard_response_correlation':'guard_request_unbound',fault);assert.equal(s.g.finish().diagnostic,'ambiguous');
  }
});

test('guard first rejection distinguishes unbound, ambiguous syntax, correlation, shape and unchanged budgets',()=>{
  const unbound=resourceFixture();unbound.ready();unbound.assetResponse({'Set-Cookie':'lang=en; Path=/'});assert.equal(unbound.g.rejection(),'guard_request_unbound');
  const unusual=resourceFixture();unusual.ready();unusual.assetRequest();unusual.assetResponse({'Set-Cookie':'unknown_private=x; Path=/; Priority=High; Partitioned'});unusual.assetExtra();assert.equal(unusual.g.rejection(),'unknown');assert.equal(unusual.g.finish().diagnostic,'other_only');
  const folded=resourceFixture();folded.ready();folded.assetRequest();folded.assetResponse({'Set-Cookie':'lang=x; Priority=Low, auth_token=y'});assert.equal(folded.g.rejection(),'guard_header_classification');assert.equal(folded.g.trace().guardHeaderReason,'syntax');
  const wrong=resourceFixture();wrong.ready();wrong.assetRequest();wrong.assetResponse({'Set-Cookie':'lang=en; Path=/'},{frameId:'other'});assert.equal(wrong.g.rejection(),'guard_response_correlation');
  const shape=resourceFixture();shape.ready();shape.assetRequest();shape.assetResponse({'Set-Cookie':['lang=x']});assert.equal(shape.g.rejection(),'guard_metadata_shape');
  const budget=resourceFixture();budget.ready();for(let i=0;i<1280;i++)budget.emit('Network.requestWillBeSent',{requestId:'foreign-'+i,frameId:frame,loaderId:next,type:'Image',request:{method:'GET',url:'https://foreign.invalid/asset'}});
  assert.equal(budget.g.rejection(),'guard_request_budget');const report=budget.g.finish();assert.equal(report.candidateAttempts,0);assert.equal(report.otherAttempts,0);
  for(const mode of ['header','event']){
    const s=resourceFixture();s.ready();s.assetRequest();
    if(mode==='header')s.assetResponse({'Set-Cookie':new Array(33).fill('lang=x; Path=/').join('\n')});
    else for(let i=0;i<65;i++)s.assetExtra({'Set-Cookie':'ct0=x; Path=/'});
    // A candidate in an earlier event remains the first rejection across later budgets.
    assert.equal(s.g.rejection(),mode==='header'?'guard_header_budget':'cookie_candidate');
  }
});
for(const role of ['reader','collector'])test(`${role} actual handoff accepts committed resource-before-context ordering only after full resolution`,async()=>{
  for(const resourceMode of ['response-after-context','response-before-context','after-context']){
    const r=await handoffFixture({role,resourceMode});assert.equal(r.early.stopReason,'captured');assert.equal(r.observation.diagnostic,'bound');assert.equal(r.observation.manualCookieGuard.diagnostic,'other_only');assert.equal(r.receipts,1);
  }
  const none=await handoffFixture({role,resourceMode:'response-after-context',cookie:null});assert.equal(none.observation.diagnostic,'bound');assert.equal(none.observation.manualCookieGuard.diagnostic,'not_observed');
  for(const [cookie,cause]of [['ct0=; Path=/; Max-Age=0','cookie_candidate'],['lang=x, ct0=y','guard_header_classification']]){
    const r=await handoffFixture({role,resourceMode:'response-after-context',cookie});assert.equal(r.receipts,0);assert.equal(r.evaluations,0);
    assert.throws(()=>requireBoundStartup(r.observation,role,r.diagnostics),error=>{assert.equal(startupDiagnostic(error).trace.firstCause,cause);return true;});
  }
  const capped=await handoffFixture({role,resourceMode:'response-after-context',cookie:null,fault:'resource-budget'});assert.equal(capped.receipts,0);assert.equal(capped.observation.manualCookieGuard.candidateAttempts,0);assert.equal(capped.observation.manualCookieGuard.otherAttempts,0);
  assert.throws(()=>requireBoundStartup(capped.observation,role,capped.diagnostics),error=>{const d=startupDiagnostic(error);assert.equal(d.trace.firstCause,'guard_request_budget');assert.equal(d.trace.cookie,'guard_request_budget');return true;});
});
test('actual handoff no-header control passes, candidates and observed scope/context loss never publish',async()=>{
  const none=await handoffFixture({cookie:null});assert.equal(none.observation.diagnostic,'bound');assert.equal(none.observation.manualCookieGuard.diagnostic,'not_observed');
  const other=await handoffFixture({cookie:'unknown_private=PRIVATE_VALUE; Domain=foreign.invalid; Path=/private; Priority=High; Partitioned'});
  assert.equal(other.observation.diagnostic,'bound');assert.equal(other.observation.manualCookieGuard.diagnostic,'other_only');assert.equal(other.receipts,1);
  assert.equal(JSON.stringify(other.observation).includes('unknown_private'),false);assert.equal(JSON.stringify(other.observation).includes('PRIVATE_VALUE'),false);
  for(const options of [{cookie:'ct0=PRIVATE; Path=/'},{fault:'context'},{fault:'owner'}]){
    const r=await handoffFixture(options);assert.notEqual(r.observation.diagnostic,'bound');assert.equal(r.receipts,0);
    if(options.cookie)assert.equal(r.evaluations,0);
    assert.throws(()=>requireBoundStartup(r.observation,'reader',r.diagnostics),error=>{assert.ok(startupDiagnostic(error));return true;});
  }
});
