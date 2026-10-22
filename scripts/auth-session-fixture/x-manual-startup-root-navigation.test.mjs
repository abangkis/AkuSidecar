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
test('one armed root document may classify known-other headers before commit or before default context',()=>{
  for(const order of ['before-commit','before-context','after-context'])for(const extraFirst of [true,false]){
    const s=guardFixture();s.arm();
    if(order!=='before-commit')s.commit();if(order==='after-context')s.context();
    assert.equal(s.request(),false);
    if(extraFirst)assert.equal(s.extra({'Set-Cookie':'lang=en; Path=/'}),false);
    assert.equal(s.response(extraFirst?{}:{'Set-Cookie':'lang=en; Path=/'}),false);
    if(!extraFirst)assert.equal(s.extra(),false);
    if(order==='before-commit')s.commit();if(order!=='after-context')s.context();
    const r=s.g.finish();assert.equal(r.diagnostic,'other_only');assert.equal(r.otherAttempts,1);
    for(const raw of [home,'lang','en;',frame,next])assert.equal(JSON.stringify(r).includes(raw),false);
  }
});
test('candidate attempts revoke immediately at every provisional phase, including blocked/delete attempts',()=>{
  for(const phase of ['before-commit','before-context','after-context'])for(const name of ['auth_token','ct0'])for(const suffix of ['=PRIVATE; Path=/','=; Max-Age=0; Path=/','=; Expires=Thu, 01 Jan 1970 00:00:00 GMT; Path=/'])for(const blocked of [false,true]){
    const s=guardFixture();s.arm();if(phase!=='before-commit')s.commit();if(phase==='after-context')s.context();s.request();
    const rejected=blocked?s.extra({}, {blockedCookies:[{cookieLine:name+suffix,blockedReasons:['private']}]}) : s.response({'Set-Cookie':name+suffix});
    assert.equal(rejected,true);assert.equal(s.g.finish().diagnostic,'candidate_observed');
  }
});
test('ambiguous syntax, unknown names and declared scopes never wait for provisional binding',()=>{
  for(const line of ['unknown_private=x; Path=/','lang=x, ct0=y; Path=/','lang=x; Domain=foreign.invalid; Path=/','lang=x; Path=/unknown','lang=x; Path=/; Path=/']){
    const s=guardFixture();s.arm();s.request();assert.equal(s.extra({'Set-Cookie':line}),true);assert.equal(s.g.finish().diagnostic,'ambiguous');
  }
});
test('provisional root must resolve commit, context, response and required ExtraInfo before finish',()=>{
  for(const missing of ['commit','context','response','extra']){
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
test('arming is single use, requires a predecessor and cannot follow adopted document state',()=>{
  const repeated=guardFixture();repeated.arm();assert.throws(()=>repeated.arm());assert.equal(repeated.g.finish().diagnostic,'ambiguous');
  const missing=guardFixture();assert.throws(()=>missing.g.armReload(''));
  const adopted=guardFixture();adopted.commit();assert.throws(()=>adopted.arm());
  const noParams=guardFixture();noParams.arm();
  assert.equal(noParams.emit('Debugger.resumed'),false);assert.equal(noParams.emit('Runtime.executionContextsCleared'),false);
  assert.equal(noParams.g.finish().diagnostic,'not_observed');
});

async function handoffFixture({role='reader',order='before-commit',cookie='lang=en; Path=/',fault}={}){
  const scope={owner:role==='reader'?1:2,profileId:'private-profile-'+role,sessionId:session,targetId:'private-target-'+role},diagnostics=startupFailureLatch();
  let binder,capture,index=0,phase='instrumentation',normal=null,instrumentation=true,evaluations=0,receipts=0,settings=false;
  const context=vm.createContext({location:{origin:'https://x.com',pathname:'/home'}});
  const scripts=['globalThis.__INITIAL_STATE__={session:{user_id:"123456"},entities:{users:{entities:{"123456":{id_str:"123456"}}}}}','void 0','delete globalThis.__INITIAL_STATE__','void 0'];
  const emit=(method,params,binding=scope)=>binder.observe({method,params,sessionId:session},binding);
  const commit=()=>{emit('Page.frameNavigated',{frame:{id:frame,loaderId:next,url:home}});capture.observe({type:'frame_navigated',frameId:frame,isHome:true});};
  const created=()=>{emit('Runtime.executionContextCreated',{context:{id:1,origin:'https://x.com',auxData:{isDefault:true,frameId:frame}}});capture.observe({type:'context_created',frameId:frame,contextId:1,isDefault:true});};
  const root=()=>{emit('Network.requestWillBeSent',rootRequest());emit('Network.responseReceivedExtraInfo',rootExtra(cookie?{'Set-Cookie':cookie}:{}));emit('Network.responseReceived',rootResponse());};
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
      if(order==='before-commit')root();commit();if(order==='before-context')root();created();if(order==='after-context')root();pause();
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
test('actual handoff no-header control passes, candidates and observed scope/context loss never publish',async()=>{
  const none=await handoffFixture({cookie:null});assert.equal(none.observation.diagnostic,'bound');assert.equal(none.observation.manualCookieGuard.diagnostic,'not_observed');
  for(const options of [{cookie:'ct0=PRIVATE; Path=/'},{cookie:'unknown_private=x; Path=/'},{fault:'context'},{fault:'owner'}]){
    const r=await handoffFixture(options);assert.notEqual(r.observation.diagnostic,'bound');assert.equal(r.receipts,0);
    if(options.cookie)assert.equal(r.evaluations,0);
    assert.throws(()=>requireBoundStartup(r.observation,'reader',r.diagnostics),error=>{assert.ok(startupDiagnostic(error));return true;});
  }
});
