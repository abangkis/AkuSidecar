import fs from 'node:fs/promises';
import test from 'node:test';
import assert from 'node:assert/strict';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createRetainedManualSetup,manualSeedFailure} from './x-manual-retained-setup.mjs';
import vm from 'node:vm';
import {pathToFileURL} from 'node:url';
import {resolve} from 'node:path';
import {createManualV2BoundObservation,projectBoundBootstrap} from './x-qa-bound-observation.mjs';
import {requireBoundStartup,startupDiagnostic,startupFailureLatch} from './x-manual-startup-diagnostic.mjs';
const path=resolve('scripts/auth-session-fixture/x-native-metadata-probe-driver.mjs');
let code=await fs.readFile(path,'utf8');
code=code.replace(/from '(\.\/[^']+)'/g,(_,p)=>`from '${new URL(p,pathToFileURL(path)).href}'`);
code+=`\nexport function offlineHarness(current,transport){active=current;send=transport;return {dispatch:dispatchCDP,start:()=>startManualCookieObservation(current),run:(options,factory)=>boundCheckpoint(current,false,options,undefined,()=>{},undefined,factory),collector:factory=>collectorShortCheckpoint(current,undefined,()=>{},()=>{},undefined,factory)};}\n`;
const driver=await import('data:text/javascript;base64,'+Buffer.from(code).toString('base64'));
// Expose only the production transport's event dispatcher in memory. No launch,
// profile validation, native send or process creation is invoked by these tests.
const collectorPath=resolve('scripts/auth-session-fixture/x-qa-feasibility-collector.mjs');
let collectorCode=await fs.readFile(collectorPath,'utf8');
collectorCode=collectorCode.replace('fileURLToPath(import.meta.url)',JSON.stringify(collectorPath))
  .replace('return Object.freeze({ launch, send, subscribe, close, ownership });','return Object.freeze({ subscribe, deliver:deliverMessage });');
const collectorModule=await import('data:text/javascript;base64,'+Buffer.from(collectorCode).toString('base64'));

async function collectorRepeatScenario(mode){
  const transport=collectorModule.createQAFeasibilityCollector({FrameDecoder:driver.CDPFrameDecoder}),decoder=new driver.CDPFrameDecoder();
  let guard,binder,received=0,observed=0,maxCalls=0,atDispatch,afterLate;
  const counts=new WeakMap(),commands=[],original=ManualStartupCookieGuard.prototype.observe;
  ManualStartupCookieGuard.prototype.observe=function(raw){
    if(this===guard&&raw.method==='Network.requestWillBeSent'){
      observed++;const n=(counts.get(raw.params)||0)+1;counts.set(raw.params,n);maxCalls=Math.max(maxCalls,n);
    }
    return original.call(this,raw);
  };
  const emit=(method,params)=>{
    if(method==='Network.requestWillBeSent')received++;
    decoder.push(Buffer.from(JSON.stringify({method,params,sessionId:'cs'})+'\0'),transport.deliver);
  };
  const request=(id,loader,type='Image',extra={})=>emit('Network.requestWillBeSent',{requestId:id,frameId:'cf',loaderId:loader,type,request:{method:'GET',url:type==='Document'?'https://x.com/home':'https://x.com/asset'},...extra});
  const current={owner:1,sessionId:'reader',manualStartupSelected:true,shortFeasibility:{sessionId:'cs',targetId:'ct'}};
  current.shortFeasibility.transport={subscribe:transport.subscribe,ownership:()=>({pid:2,profileId:'collector',alive:true}),send:async(method,params,session,project)=>{
    commands.push(method);
    if(method==='Page.getFrameTree')return project({frameTree:{frame:{id:'cf',loaderId:'old',url:'https://x.com/home'}}});
    if(method==='Page.navigate'){request('navigation','old','Document');return project({frameId:'cf'});}
    if(method==='Runtime.evaluate')return project({result:{type:'boolean',value:true}});
    if(method==='Debugger.setInstrumentationBreakpoint')return project({breakpointId:'instrumentation'});
    if(method==='Page.reload'){
      request('root','new','Document');request('asset','new');
      if(mode!=='single'){
        const headers=mode==='candidate'?{'Set-Cookie':'ct0=; Path=/'}:mode==='unknown'?{'Set-Cookie':'unknown_private=x; Path=/'}:mode==='other'?{'Set-Cookie':'lang=en; Path=/'}:{};
        request('asset','new','Image',mode==='undeclared'?{}:{redirectResponse:mode==='malformed'?null:{url:'https://x.com/asset',status:302,headers}});
      }
      atDispatch={reason:guard.rejection(),...guard.trace()};
      // Later complete, consistent response metadata cannot repair ID reuse.
      emit('Network.responseReceived',{requestId:'asset',frameId:'cf',loaderId:'new',hasExtraInfo:false,response:{url:'https://x.com/asset',status:200,headers:{}}});
      afterLate={reason:guard.rejection(),...guard.trace()};
      // Stop after the production dispatch path; this is not a login-success fixture.
      throw new Error('offline_stop_after_dispatch');
    }
    return project({});
  }};
  try{
    const h=driver.offlineHarness(current,async method=>method==='Page.getFrameTree'?{frameId:'rf'}:{acknowledged:true});
    await h.start();guard=current.shortFeasibility.manualCookieGuard;
    current.manualCookieGuard.beginPostSeed();guard.beginPostSeed();
    await h.collector((...args)=>{binder=createManualV2BoundObservation(...args,()=>{},guard);return binder;});
    return{atDispatch,afterLate,received,observed,maxCalls,commands,binder,final:{reason:guard.rejection(),...guard.trace()},first:current.manualCookieDiagnostics.current()};
  }finally{ManualStartupCookieGuard.prototype.observe=original;}
}
for(const [mode,reason,trigger,provenance]of [
  ['single','unknown','unknown','unknown'],
  ['declared','guard_request_unbound','request','redirect'],
  ['undeclared','guard_request_unbound','request','reused'],
  ['malformed','guard_request_unbound','request','redirect'],
  ['candidate','cookie_candidate','redirect','redirect'],
  ['unknown','guard_header_classification','redirect','redirect'],
  ['other','guard_request_unbound','redirect','redirect'],
])test(`actual collector transport delivers once and keeps repeat rejection closed: ${mode}`,async()=>{
  const r=await collectorRepeatScenario(mode),expected={reason,guardTrigger:trigger,guardProvenance:provenance,
    guardCondition:['declared','other'].includes(mode)?'extra_flag':mode==='malformed'?'redirect_shape':'unknown',
    guardEpoch:mode==='single'?'unknown':'reload_armed'};
  assert.ok(r.binder);assert.equal(r.received,mode==='single'?3:4);assert.equal(r.observed,r.received);assert.equal(r.maxCalls,1);
  assert.equal(r.commands.filter(m=>m==='Network.enable').length,1);
  assert.deepEqual(r.atDispatch,expected);assert.deepEqual(r.afterLate,expected);
  if(mode!=='single'){
    assert.deepEqual(r.final,expected);assert.equal(r.first.role,'collector');
    assert.equal(r.first.trace.firstCause,reason);assert.equal(r.first.trace.guardTrigger,trigger);assert.equal(r.first.trace.guardProvenance,provenance);
  }
});
async function driverScenario(scenario, continuous=false, injectedCookie='lang=en; Path=/') {
  let normal=null,instrumentation=true,index=0,phase='instrumentation',evaluations=0,receipts=0,first=null,settings=false;
  const current={owner:1,pid:1,profileId:'p',sessionId:'s',targetId:'t',manualStartupSelected:true,gate:{networkEnabled:false,setLoginVisible(){}}};
  const context=vm.createContext({location:{origin:'https://x.com',pathname:'/home'}});
  const scripts=['globalThis.__INITIAL_STATE__={session:{user_id:"123456"},entities:{users:{entities:{"123456":{id_str:"123456"}}}}}','void 0','delete globalThis.__INITIAL_STATE__','void 0'];
  let harness; const commands=[];
  current.shortFeasibility={sessionId:'collector-session',transport:{send:async(method,params,session,project)=>project(method==='Page.getFrameTree'?{frameTree:{frame:{id:'collector-frame'}}}:{}),subscribe:()=>()=>{}}};
  const emit=(method,params,sessionId='s')=>harness.dispatch({method,params,sessionId});
  const request=(id,url='https://x.com/asset',loader='new',type='Image',patch={})=>emit('Network.requestWillBeSent',{requestId:id,frameId:'f',loaderId:loader,type,request:{method:'GET',url},...patch});
  const response=(id,url='https://x.com/asset',loader='new',cookie=injectedCookie,sessionId='s')=>{
   emit('Network.responseReceivedExtraInfo',{requestId:id,statusCode:200,headers:cookie?{'Set-Cookie':cookie}:{},blockedCookies:[]},sessionId);
   emit('Network.responseReceived',{requestId:id,frameId:'f',loaderId:loader,hasExtraInfo:true,response:{url,status:200,headers:{}}},sessionId);
  };
  const pause=(ordinary=false)=>{
   phase=ordinary?'normal':'instrumentation';
   emit('Debugger.scriptParsed',{scriptId:'script-'+index,executionContextId:1,executionContextAuxData:{isDefault:true,frameId:'f'}});
   emit('Debugger.paused',{reason:ordinary?'other':'instrumentation',hitBreakpoints:ordinary?[normal]:[],callFrames:[{callFrameId:'call',location:{scriptId:'script-'+index,lineNumber:0,columnNumber:0}}]});
  };
  const execute=()=>{if(index<scripts.length)vm.runInContext(scripts[index++],context);if(index<scripts.length&&instrumentation)pause();else while(index<scripts.length)vm.runInContext(scripts[index++],context);};
  const emitSettings=()=>{
   if(settings)return;settings=true;const url='https://api.x.com/1.1/account/settings.json';request('settings',url,'new','XHR');
   emit('Network.responseReceived',{requestId:'settings',frameId:'f',loaderId:'new',hasExtraInfo:true,response:{url,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
   emit('Network.responseReceivedExtraInfo',{requestId:'settings',statusCode:200,headers:{}});
   emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
  };
  harness=driver.offlineHarness(current,async(method,params,sessionId,purpose)=>{
   commands.push(method);
   if(method==='Page.getFrameTree')return{frameId:'f',loaderId:'old'};
   if(method==='Network.enable'&&scenario==='after-enable-old-loader')request('pending',undefined,'old');
   if(method==='Page.reload'){
    request('root','https://x.com/home','new','Document');response('root','https://x.com/home','new',null);
    if(scenario==='precommit-resource')request('asset');
    emit('Page.frameNavigated',{frame:{id:'f',loaderId:'new',url:'https://x.com/home'}});
    if(scenario==='postcommit-resource')request('asset');
    emit('Runtime.executionContextCreated',{context:{id:1,origin:'https://x.com',auxData:{isDefault:true,frameId:'f'}}});
    if(['before-observer-late-response','after-enable-old-loader','observed-before-seed'].includes(scenario))response('pending',undefined,'old');
    if(['precommit-resource','postcommit-resource'].includes(scenario))response('asset');
    if(scenario==='foreign-origin'){request('foreign','https://static.invalid/asset');response('foreign','https://static.invalid/asset');}
    if(scenario==='resource-redirect'){request('redirect');request('redirect','https://x.com/asset2','new','Image',{redirectResponse:{url:'https://x.com/asset',status:302,headers:{'Set-Cookie':'lang=en; Path=/'}}});}
    if(scenario.startsWith('qualified-redirect')){
      const headers={'Set-Cookie':scenario==='qualified-redirect-candidate'?'ct0=; Max-Age=0; Path=/':'lang=en; Path=/'};
      request('redirect');
      if(scenario==='qualified-redirect-early')emit('Network.responseReceivedExtraInfo',{requestId:'redirect',statusCode:302,headers,blockedCookies:[]});
      request('redirect','https://x.com/asset2','new','Image',{redirectHasExtraInfo:true,redirectResponse:{url:'https://x.com/asset',status:302,headers:{}}});
      if(scenario!=='qualified-redirect-missing'&&scenario!=='qualified-redirect-early')emit('Network.responseReceivedExtraInfo',{requestId:'redirect',statusCode:302,headers,blockedCookies:[]});
      response('redirect','https://x.com/asset2','new','lang=en; Path=/');
    }
    if(scenario==='foreign-session-response')response('unseen',undefined,'new','lang=en; Path=/','observer-session');
    pause();
   }
   if(method==='Debugger.setInstrumentationBreakpoint')return{breakpointId:'instrumentation'};
   if(method==='Debugger.setBreakpoint'){normal='normal-'+index;current.earlyCapture.handoff.created(normal);return{breakpointId:normal,actualLocation:{...params.location}};}
   if(method==='Debugger.removeBreakpoint'){if(purpose==='normal_remove')normal=null;else instrumentation=false;}
   if(method==='Debugger.resume'){emit('Debugger.resumed',{});if(normal&&phase==='instrumentation')pause(true);else execute();}
   if(method==='Debugger.evaluateOnCallFrame'){evaluations++;return projectBoundBootstrap(JSON.parse(JSON.stringify(vm.runInContext(params.expression,context))),(id,p)=>current.bound.binder.acceptIdentity(id,p));}
   if(method==='Debugger.disable')emitSettings();
   if(method==='Network.getResponseBody'){if(!current.bound.binder.capture.consumeRead(params.requestId))throw Error('bad-body-read');return{schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};}
   return{acknowledged:true};
  });
  if(scenario==='before-observer-late-response')request('pending',undefined,'old');
  if(continuous){await harness.start();if(scenario==='observed-before-seed')request('pending',undefined,'old');current.manualCookieGuard.beginPostSeed();current.shortFeasibility.manualCookieGuard.beginPostSeed();}
  const observation=await harness.run({deferCookieMutation:false,onPrivateResult(){},onPrivateStartup(){receipts++;commands.push('receipt');}},(...args)=>createManualV2BoundObservation(...args,(reason,trace)=>{first={reason,...trace};},current.manualCookieGuard));
  let trace=null;try{requireBoundStartup(observation,'reader',startupFailureLatch());}catch(e){trace=startupDiagnostic(e)?.trace;}
  return {scenario,evaluations,receipts,diagnostic:observation.diagnostic,first,trace,commands,observation};
}

for(const scenario of ['clean','precommit-resource','postcommit-resource','observed-before-seed','foreign-session-response','qualified-redirect-late','qualified-redirect-early'])test(`actual driver retains bounded network provenance: ${scenario}`,async()=>{
  const r=await driverScenario(scenario,true);assert.equal(r.diagnostic,'bound');assert.equal(r.receipts,1);assert.equal(r.evaluations,2);
  assert.equal(r.commands.filter(v=>v==='Network.enable').length,1);
  assert.ok(r.commands.indexOf('Network.disable')>r.commands.indexOf('receipt'));
});
test('actual reader dispatcher rejects a pending redirect chain before publishing and candidates before identity reads',async()=>{
  const missing=await driverScenario('qualified-redirect-missing',true);assert.equal(missing.receipts,0);assert.equal(missing.evaluations,2);
  assert.equal(missing.trace.firstCause,'guard_completion_missing');assert.equal(missing.observation.manualCookieGuard.diagnostic,'ambiguous');
  const candidate=await driverScenario('qualified-redirect-candidate',true);assert.equal(candidate.receipts,0);assert.equal(candidate.evaluations,0);
  assert.equal(candidate.trace.firstCause,'cookie_candidate');assert.equal(candidate.observation.manualCookieGuard.diagnostic,'candidate_observed');
});

async function collectorRedirectScenario(mode){
  const transport=collectorModule.createQAFeasibilityCollector({FrameDecoder:driver.CDPFrameDecoder}),decoder=new driver.CDPFrameDecoder();
  const commands=[],context=vm.createContext({location:{origin:'https://x.com',pathname:'/home'}});
  const scripts=['globalThis.__INITIAL_STATE__={session:{user_id:"123456"},entities:{users:{entities:{"123456":{id_str:"123456"}}}}}','void 0','delete globalThis.__INITIAL_STATE__','void 0'];
  let guard,binder,normal=null,instrumentation=true,index=0,phase='instrumentation',evaluations=0,receipts=0,settings=false,received=0,observed=0,maxCalls=0;
  const counts=new WeakMap(),original=ManualStartupCookieGuard.prototype.observe;
  ManualStartupCookieGuard.prototype.observe=function(raw){
    if(this===guard&&raw.method.startsWith('Network.')){observed++;const n=(counts.get(raw.params)||0)+1;counts.set(raw.params,n);maxCalls=Math.max(maxCalls,n);}
    return original.call(this,raw);
  };
  const emit=(method,params)=>{
    if(method.startsWith('Network.'))received++;
    decoder.push(Buffer.from(JSON.stringify({method,params,sessionId:'cs'})+'\0'),transport.deliver);
  };
  const request=(id,url='https://x.com/asset',type='Image',patch={})=>emit('Network.requestWillBeSent',{requestId:id,frameId:'cf',loaderId:'new',type,request:{method:'GET',url},...patch});
  const extra=(id,status=200,headers={},patch={})=>emit('Network.responseReceivedExtraInfo',{requestId:id,statusCode:status,headers,blockedCookies:[],...patch});
  const response=(id,url='https://x.com/asset2',hasExtraInfo=false)=>emit('Network.responseReceived',{requestId:id,frameId:'cf',loaderId:'new',hasExtraInfo,response:{url,status:200,headers:{}}});
  const pause=(ordinary=false)=>{
    phase=ordinary?'normal':'instrumentation';
    emit('Debugger.scriptParsed',{scriptId:'script-'+index,executionContextId:1,executionContextAuxData:{isDefault:true,frameId:'cf'}});
    emit('Debugger.paused',{reason:ordinary?'other':'instrumentation',hitBreakpoints:ordinary?[normal]:[],callFrames:[{callFrameId:'call',location:{scriptId:'script-'+index,lineNumber:0,columnNumber:0}}]});
  };
  const execute=()=>{if(index<scripts.length)vm.runInContext(scripts[index++],context);if(index<scripts.length&&instrumentation)pause();else while(index<scripts.length)vm.runInContext(scripts[index++],context);};
  const emitSettings=()=>{
    if(settings)return;settings=true;const url='https://api.x.com/1.1/account/settings.json';request('settings',url,'XHR');
    emit('Network.responseReceived',{requestId:'settings',frameId:'cf',loaderId:'new',hasExtraInfo:true,response:{url,status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false}});
    extra('settings');emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
  };
  const current={owner:1,sessionId:'reader',manualStartupSelected:true,shortFeasibility:{sessionId:'cs',targetId:'ct',navigated:true}};
  current.shortFeasibility.transport={subscribe:transport.subscribe,ownership:()=>({pid:2,profileId:'collector',alive:true}),send:async(method,params,session,project)=>{
    commands.push(method);let result={};
    if(method==='Page.getFrameTree')result={frameTree:{frame:{id:'cf',loaderId:'old',url:'https://x.com/home'}}};
    if(method==='Page.reload'){
      request('root','https://x.com/home','Document');response('root','https://x.com/home');
      request('asset');
      if(mode==='early')extra('asset',302,{'Set-Cookie':'lang=en; Path=/'});
      request('asset','https://x.com/asset2','Image',{redirectHasExtraInfo:true,redirectResponse:{url:'https://x.com/asset',status:302,headers:{}}});
      if(!['early','missing','reordered'].includes(mode))extra('asset',302,mode==='candidate'?{}:{'Set-Cookie':'lang=en; Path=/'},mode==='candidate'?{blockedCookies:[{cookieLine:'ct0=; Max-Age=0; Path=/'}]}:{});
      extra('asset',200,{'Set-Cookie':'lang=en; Path=/'});response('asset','https://x.com/asset2',true);
      if(mode==='reordered')extra('asset',302,{'Set-Cookie':'lang=en; Path=/'});
      emit('Page.frameNavigated',{frame:{id:'cf',loaderId:'new',url:'https://x.com/home'}});
      emit('Runtime.executionContextCreated',{context:{id:1,origin:'https://x.com',auxData:{isDefault:true,frameId:'cf'}}});pause();
    }
    if(method==='Debugger.setInstrumentationBreakpoint')result={breakpointId:'instrumentation'};
    if(method==='Debugger.setBreakpoint'){normal='normal-'+index;result={breakpointId:normal,actualLocation:{...params.location}};}
    if(method==='Debugger.removeBreakpoint'){if(params.breakpointId===normal)normal=null;else instrumentation=false;}
    if(method==='Debugger.resume'){emit('Debugger.resumed',{});if(normal&&phase==='instrumentation')pause(true);else execute();}
    if(method==='Debugger.evaluateOnCallFrame'){evaluations++;result={result:{type:'object',value:JSON.parse(JSON.stringify(vm.runInContext(params.expression,context)))}};}
    if(method==='Debugger.disable')emitSettings();
    if(method==='Network.getResponseBody')result={body:JSON.stringify({language:'en',time_zone:null,protected:false,always_use_https:true,discoverable_by_email:false,discoverable_by_mobile_phone:false}),base64Encoded:false};
    return project(result);
  }};
  try{
    const h=driver.offlineHarness(current,async method=>method==='Page.getFrameTree'?{frameId:'rf'}:{acknowledged:true});
    await h.start();guard=current.shortFeasibility.manualCookieGuard;
    current.manualCookieGuard.beginPostSeed();guard.beginPostSeed();
    const result=await h.collector((...args)=>{args[5]={...args[5],onPrivateStartup:()=>{receipts++;commands.push('receipt');}};binder=createManualV2BoundObservation(...args,()=>{},guard);return binder;});
    return{observation:result.checkpoint.observation,evaluations,receipts,commands,received,observed,maxCalls,first:current.manualCookieDiagnostics.current()};
  }finally{ManualStartupCookieGuard.prototype.observe=original;}
}
for(const mode of ['early','late','reordered','missing','candidate'])test(`actual collector transport preserves hop provenance through final receipt gate: ${mode}`,async()=>{
  const r=await collectorRedirectScenario(mode);assert.equal(r.received,r.observed);assert.equal(r.maxCalls,1);
  assert.equal(r.commands.filter(m=>m==='Network.enable').length,1);
  if(['missing','candidate'].includes(mode)){
    assert.equal(r.receipts,0);assert.equal(r.evaluations,mode==='candidate'?0:2);
    assert.equal(r.first.role,'collector');assert.equal(r.first.trace.firstCause,mode==='candidate'?'cookie_candidate':'guard_completion_missing');
    assert.equal(r.observation.manualCookieGuard.diagnostic,mode==='candidate'?'candidate_observed':'ambiguous');
  }else{
    assert.equal(r.observation.diagnostic,'bound');assert.equal(r.receipts,1);assert.equal(r.evaluations,2);assert.equal(r.observation.manualCookieGuard.otherAttempts,2);
    assert.ok(r.commands.indexOf('Network.disable')>r.commands.indexOf('receipt'));assert.equal(r.observation.continuity,'unknown');assert.equal(r.observation.canAuthorize,false);
  }
});
for(const [scenario,provenance,trigger]of [['before-observer-late-response','absent','extra'],['foreign-origin','origin_scope','extra'],['resource-redirect','redirect','redirect']])test(`actual driver keeps unqualified provenance closed: ${scenario}`,async()=>{
  const r=await driverScenario(scenario,true);assert.equal(r.receipts,0);assert.equal(r.evaluations,0);
  assert.equal(r.trace.firstCause,'guard_request_unbound');assert.equal(r.trace.guardProvenance,provenance);assert.equal(r.trace.guardTrigger,trigger);
  assert.equal(r.trace.early,'evaluation_failed');assert.equal(r.trace.handoff,'accepted');assert.equal(r.trace.extra,'complete');
});
test('actual driver candidate attempts from observed old-loader requests remain immediate and immutable',async()=>{
  for(const line of ['auth_token=; Path=/','ct0=x; Max-Age=0; Path=/','unknown_private=x; Path=/']){
    const r=await driverScenario('observed-before-seed',true,line);assert.equal(r.receipts,0);assert.equal(r.evaluations,0);
    assert.equal(r.trace.firstCause,line.startsWith('unknown')?'guard_header_classification':'cookie_candidate');assert.equal(r.trace.guardProvenance,'scoped');assert.equal(r.trace.guardTrigger,'extra');
  }
});

function passive(){
  const g=new ManualStartupCookieGuard('f','s',{beforeSeed:true});
  const emit=(method,params)=>g.observe({method,params,sessionId:'s'});
  const request=(id='r',patch={})=>emit('Network.requestWillBeSent',{requestId:id,frameId:'f',loaderId:'old',request:{method:'POST',url:'https://x.com/asset'},...patch});
  const response=(line,id='r',patch={})=>emit('Network.responseReceived',{requestId:id,frameId:'f',loaderId:'old',hasExtraInfo:false,response:{url:'https://x.com/asset',status:200,headers:line?{'Set-Cookie':line}:{}},...patch});
  return{g,emit,request,response};
}
test('pre-seed headers do not poison the epoch; structural coverage loss cannot be reset',()=>{
  for(const line of ['ct0=x; Path=/','unknown_private=x; Path=/']){
    const s=passive();s.request();s.response(line);assert.equal(s.g.rejection(),'unknown');s.g.beginPostSeed();assert.equal(s.g.finish().diagnostic,'not_observed');
  }
  const capped=passive();for(let i=0;i<129;i++)capped.request('r'+i);capped.g.beginPostSeed();assert.equal(capped.g.rejection(),'guard_request_budget');assert.equal(capped.g.finish().diagnostic,'ambiguous');
  const malformed=passive();malformed.emit('Network.requestWillBeSent',{});malformed.g.beginPostSeed();assert.equal(malformed.g.rejection(),'guard_metadata_shape');
});
test('post-seed missing predecessor and seen rejected scope remain distinct, including blocked candidates',()=>{
  const absent=passive();absent.g.beginPostSeed();absent.response('lang=en; Path=/');assert.deepEqual(absent.g.trace(),{guardTrigger:'response',guardProvenance:'absent',guardCondition:'unknown',guardEpoch:'post_seed'});
  const foreign=passive();foreign.request('r',{frameId:'child'});foreign.g.beginPostSeed();foreign.response('lang=en; Path=/');assert.deepEqual(foreign.g.trace(),{guardTrigger:'response',guardProvenance:'frame_scope',guardCondition:'unknown',guardEpoch:'post_seed'});
  for(const scope of ['absent','scoped','foreign']){
    const s=passive();if(scope!=='absent')s.request('r',scope==='foreign'?{request:{method:'GET',url:'https://foreign.invalid/asset'}}:{});s.g.beginPostSeed();
    s.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers:{},blockedCookies:[{cookieLine:'ct0=; Max-Age=0; Path=/'}]});
    assert.equal(s.g.rejection(),'cookie_candidate');assert.equal(s.g.category(),'candidate_observed');
  }
});
test('provenance cannot be replayed, rebound, or repaired by later events',()=>{
  const s=passive();s.request();s.g.beginPostSeed();s.g.claim('f','s');assert.throws(()=>s.g.claim('f','s'));assert.throws(()=>s.g.beginPostSeed());
  const bad=passive();bad.request();bad.g.beginPostSeed();bad.response('lang=en; Path=/', 'r',{frameId:'child'});const first=bad.g.trace();bad.response('ct0=x; Path=/');assert.deepEqual(bad.g.trace(),first);assert.equal(bad.g.rejection(),'guard_response_correlation');
  assert.equal(bad.g.finish().diagnostic,'candidate_observed');assert.throws(()=>bad.g.finish());
});

// Execute the exact production seed function with an enrollment test double.
// All transport functions are stubs; no native helper, browser or cookie API runs.
const originalDriver=await fs.readFile(path,'utf8');
const seedSource=originalDriver.slice(originalDriver.indexOf('export async function seedRetainedManualPilot('),originalDriver.indexOf('\nasync function openRetainedManualReader(')).replace('export async','async');
test('actual seed acknowledgment dispatch starts both baselines before same-batch and cleanup events',async()=>{
  const names=['AKU_X_NATIVE_MANUAL_STARTUP_ALLOWED','AKU_X_MANUAL_RETAINED_TRANSFER_ONCE','AKU_X_QA_SHORT_TRANSFER_ALLOWED'],previous=names.map(k=>process.env[k]);
  try{
    for(const key of names)process.env[key]='1';
    for(const when of ['ack-batch','cleanup','none']){
      let collectorObserver,seedCalls=0;const methods=[];
      const current={pid:1,owner:1,profileId:'reader',sessionId:'s',targetId:'t',gate:{loginVisible:true,observationStarted:false}};
      const emit=(id,line)=>harness.dispatch({method:'Network.responseReceivedExtraInfo',sessionId:'s',params:{requestId:id,statusCode:200,headers:{'Set-Cookie':line}}});
      const request=id=>harness.dispatch({method:'Network.requestWillBeSent',sessionId:'s',params:{requestId:id,frameId:'f',loaderId:'old',request:{method:'GET',url:'https://x.com/asset'}}});
      current.shortFeasibility={sessionId:'cs',transport:{ownership:()=>({alive:true,pid:2,profileId:'collector'}),subscribe:callback=>{collectorObserver=callback;return()=>{};},send:async(method,params,session,project)=>{
        methods.push('collector:'+method);
        if(method==='Network.setCookies'){seedCalls++;const ack=project({});if(when==='ack-batch')emit('pending','ct0=; Path=/');return ack;}
        return project(method==='Page.getFrameTree'?{frameTree:{frame:{id:'cf'}}}:{});
      }}};
      const transport=async(method)=>{methods.push('reader:'+method);if(method==='Page.getFrameTree')return{frameId:'f',loaderId:'old'};if(method==='Network.getCookies')return[{name:'auth_token'},{name:'ct0'}];return{acknowledged:true};};
      const harness=driver.offlineHarness(current,transport);
      class Enrollment{
        observe(){}
        async run(){assert.ok(methods.includes('reader:Network.enable'));assert.ok(methods.includes('collector:Network.enable'));assert.equal(methods.some(v=>v.endsWith('Page.navigate')),false);request('preseed');emit('preseed','ct0=x; Path=/');request('pending');return'123456';}
        async check(){} assertCurrent(){} async close(){if(when==='cleanup')emit('pending','ct0=; Path=/');}
      }
      const seed=new Function('requireOwner','qualificationMode','driverMode','retainedQAScope','createRetainedManualSetup','ManualEnrollmentCapture','startManualCookieObservation','send','exactKeys','manualSeedFailure','childExited','cdpFailure','projectCDPEvent','qaCookieRequest',seedSource+';return seedRetainedManualPilot;')(
        ()=>current,()=>true,'retained_source_qualification',{reuse:true},createRetainedManualSetup,Enrollment,()=>harness.start(),transport,()=>{},manualSeedFailure,false,null,()=>{},()=>({urls:['https://api.x.com/1.1/account/settings.json']}));
      await seed({id:1,op:'manual_seed_bootstrap',owner:1});assert.equal(seedCalls,1);assert.equal(methods.some(v=>v.endsWith('Network.disable')),false);
      assert.equal(current.manualCookieGuard.rejection(),when==='none'?'unknown':'cookie_candidate');
      assert.equal(current.manualCookieDiagnostics.current()?.trace.guardProvenance,when==='none'?undefined:'scoped');
      collectorObserver({method:'Network.responseReceivedExtraInfo',sessionId:'cs',params:{requestId:'unseen',statusCode:200,headers:{'Set-Cookie':'ct0=; Path=/'}}});
      assert.equal(current.shortFeasibility.manualCookieGuard.rejection(),'cookie_candidate');
    }
  }finally{names.forEach((key,i)=>{if(previous[i]===undefined)delete process.env[key];else process.env[key]=previous[i];});}
});
