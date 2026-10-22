import test from 'node:test';
import assert from 'node:assert/strict';
import {STARTUP_TRACE,closedStartupTrace,startupFailure,startupFailureLatch,startupDiagnostic,startupRPCCode,requireBoundStartup} from './x-manual-startup-diagnostic.mjs';
import {offlineFirstCausePair,awaitOfflineWallTick} from './x-manual-startup-first-cause-offline.mjs';
import {offlineStartupOwner} from './x-manual-startup-offline-fixture.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {inspectBootstrapCurrentSelf} from './x-bootstrap-current-self.mjs';
const q=(op,role,extra={})=>({id:1,owner:1,op,role,...extra});
test('offline diagnostic clock waits past a same-ms challenge and accepts an already advanced real tick',async()=>{
  for(const ticks of [[1000,1000,1000,1001],[1000,1001]]){
    let reads=0,waits=0;
    await awaitOfflineWallTick(()=>ticks[reads++],async()=>{waits++;});
    assert.equal(reads,ticks.length);assert.equal(waits,ticks.length-1);
    // A nanosecond-resolution Go challenge inside the original ms is earlier
    // only after the actual wall tick, never through a forged future timestamp.
    assert.ok(ticks.at(-1)>1000.9);
  }
  await assert.rejects(awaitOfflineWallTick(()=>1000,async()=>{}),/offline_clock_stalled/);
  let n=0;await assert.rejects(awaitOfflineWallTick(()=>n++?999:1000,async()=>{}),/offline_clock_invalid/);
});
async function run(fault,role){
  const f=offlineFirstCausePair({fault,faultRole:role});
  const ready=await Promise.all(['reader','collector'].map(r=>f.port.execute(q('manual_prepare',r))));
  let failure;
  try{await f.port.execute(q('manual_acquire',role,{handle:ready[role==='reader'?0:1].handle,request:{challenge:'b'.repeat(48),role,...offlineStartupOwner(role)}}));}catch(error){failure=error;}
  assert.ok(failure);
  await Promise.all(['reader','collector'].map(r=>f.port.execute(q('manual_abort',r,{handle:null,preserveReader:r==='reader'}))));
  return {f,d:startupDiagnostic(failure),failure};
}
for(const role of ['reader','collector'])test(`${role} first observed cookie or ambiguity survives early fallback and cleanup failure`,async()=>{
  for(const fault of ['cookie','ambiguous','cookie-context','cleanup']){
    const {f,d}=await run(fault,role);
    assert.equal(d.role,role);assert.equal(d.reason,'observed_cookie_write');
    assert.equal(d.trace.firstCause,fault==='ambiguous'?'guard_header_classification':'cookie_candidate');
    assert.equal(d.trace.terminal,'scope_lost');assert.equal(d.trace.early,'evaluation_failed');assert.equal(d.trace.handoff,'rejected');
    assert.equal(d.trace.cookie,fault==='ambiguous'?'guard_header_classification':'candidate_observed');
    assert.equal(f.reports[role].diagnostic,'scope_lost');assert.equal(f.reports[role].canAuthorize,false);
    assert.equal(f.reports[role].cleanupVerified,fault!=='cleanup');
    assert.equal(JSON.stringify(d).includes('PRIVATE'),false);
  }
});
test('real context loss remains first even when a later candidate and generic fallback are observed',async()=>{
  const {d}=await run('context-first','reader');assert.equal(d.trace.firstCause,'context_lost');assert.equal(d.reason,'scope_lost');assert.equal(d.trace.cookie,'candidate_observed');
});
test('settings order, ExtraInfo completeness and early outcomes remain separate closed fields',async()=>{
  for(const [fault,first,extra,early]of [['settings-order','settings_order','not_required','evaluation_failed'],['extra-missing','generic_early','missing','evaluation_failed'],['extra-status','extra_status','status_mismatch','evaluation_failed'],['early-deadline','generic_early','not_selected','deadline'],['early-rejected','generic_early','not_selected','captured']]){
    const {d}=await run(fault,'collector');assert.equal(d.trace.firstCause,first,fault);assert.equal(d.trace.extra,extra,fault);assert.equal(d.trace.early,early,fault);
  }
});
test('pair latch cannot borrow peer summaries or rewrite generic/unknown first cause',()=>{
  const latch=startupFailureLatch();
  latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'unknown'}),'reader','bound_finish');
  const peer=latch.record(startupFailure('collector','bound_finish','scope_lost',false,undefined,{firstCause:'unknown',terminal:'cleanup_unverified',cookie:'candidate_observed'}),'collector','bound_finish');
  assert.equal(startupDiagnostic(peer).trace.terminal,'unknown');assert.equal(startupDiagnostic(peer).trace.cookie,'unknown');
  const later=latch.record(startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,{firstCause:'cookie_candidate',cookie:'candidate_observed'}),'reader','bound_finish');
  assert.equal(startupDiagnostic(later).trace.firstCause,'unknown');assert.equal(startupDiagnostic(later).reason,'scope_lost');
  const same=latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'unknown',terminal:'scope_lost',early:'deadline'}),'reader','bound_finish');
  assert.equal(startupDiagnostic(same).trace.early,'deadline');assert.ok(Object.isFrozen(startupDiagnostic(same).trace));
});
test('event-time first cause survives peer-first delivery without importing peer terminal state',async()=>{
  const f=offlineFirstCausePair();await Promise.all(['reader','collector'].map(r=>f.port.execute(q('manual_prepare',r))));
  f.fail('reader');f.port.invalidate('collector');
  await assert.rejects(f.port.execute(q('manual_inspect','collector')),error=>{const d=startupDiagnostic(error);assert.equal(d.role,'reader');assert.equal(d.peer,true);assert.equal(d.trace.firstCause,'cookie_candidate');assert.equal(d.trace.terminal,'unknown');return true;});
  await Promise.all(['reader','collector'].map(r=>f.port.execute(q('manual_abort',r,{handle:null,preserveReader:r==='reader'}))));
});
test('trace wire uses closed append-only canonical indices and never invokes spoofed getters',()=>{
  let reads=0;
  const unsafe=Object.defineProperty({terminal:'PRIVATE',extra:'x'.repeat(20000)},'firstCause',{get(){reads++;throw new Error('PRIVATE');}});
  assert.deepEqual(closedStartupTrace(unsafe),{firstCause:'unknown',terminal:'unknown',early:'unknown',handoff:'unknown',cookie:'unknown',extra:'unknown',guardTrigger:'unknown',guardProvenance:'unknown',guardCondition:'unknown',guardEpoch:'unknown',guardDiskCache:'unknown',guardServiceWorker:'unknown',guardPrefetchCache:'unknown'});
  const codes=[];
  const guarded=Object.defineProperties({}, {
    guardCondition:{get(){reads++;throw new Error('PRIVATE');}},
    guardEpoch:{get(){reads++;throw new Error('PRIVATE');}},
    guardDiskCache:{get(){reads++;throw new Error('PRIVATE');}},
    guardServiceWorker:{get(){reads++;throw new Error('PRIVATE');}},
    guardPrefetchCache:{get(){reads++;throw new Error('PRIVATE');}},
  });
  assert.deepEqual([closedStartupTrace(guarded).guardCondition,closedStartupTrace(guarded).guardEpoch,closedStartupTrace(guarded).guardDiskCache,closedStartupTrace(guarded).guardServiceWorker,closedStartupTrace(guarded).guardPrefetchCache],['unknown','unknown','unknown','unknown','unknown']);
  for(const [key,values]of Object.entries(STARTUP_TRACE))for(const value of values){const code=startupRPCCode(startupFailure('collector','inspect_before','observed_cookie_write',true,undefined,{[key]:value}));assert.match(code,/^manual_initial_v5_(?:0|[1-9][0-9]*)(?:_(?:0|[1-9][0-9]*)){16}$/);assert.ok(code.length<=80);codes.push(code);}
  const maximum=Object.fromEntries(Object.entries(STARTUP_TRACE).map(([key,values])=>[key,values.at(-1)]));
  const maximumCode=startupRPCCode(startupFailure('collector','inspect_before','observed_cookie_write',true,undefined,maximum));
  assert.match(maximumCode,/^manual_initial_v5_(?:0|[1-9][0-9]*)(?:_(?:0|[1-9][0-9]*)){16}$/);assert.ok(maximumCode.length<=80);
  const report=Object.defineProperty({},'diagnostic',{get(){reads++;return 'bound';}});
  assert.throws(()=>requireBoundStartup(report,'reader',startupFailureLatch()));assert.equal(reads,0);
  assert.equal(startupRPCCode({trace:unsafe}),'manual_initial_unknown_acquire_rpc_unavailable_primary');assert.ok(codes.length>40);
});
test('later cache observations cannot enrich a first failure trace',()=>{
  const latch=startupFailureLatch();
  const first=latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'guard_request_unbound',guardCondition:'unknown'}),'reader','bound_finish');
  const later=latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'guard_request_unbound',guardCondition:'response_cache',guardDiskCache:'true',guardServiceWorker:'false',guardPrefetchCache:'absent'}),'reader','bound_finish');
  const trace=startupDiagnostic(later).trace;
  assert.equal(trace.guardCondition,'unknown');
  assert.deepEqual([trace.guardDiskCache,trace.guardServiceWorker,trace.guardPrefetchCache],['unknown','unknown','unknown']);
  assert.deepEqual([startupDiagnostic(first).trace.guardDiskCache,trace.guardDiskCache],['unknown','unknown']);
  assert.ok(Object.isFrozen(trace));
});

function directSample(readBody,document=true) {
  const scope={owner:1,profileId:'private-profile',sessionId:'private-session',targetId:'private-target'},diagnostics=startupFailureLatch();let binder;
  binder=createManualV2BoundObservation(scope,'frame',async id=>{
    assert.equal(binder.capture.consumeRead(id),true);
    if(readBody)await readBody();
    return {schema:'aku.x.qa-direct-body.v1',category:'settings_match',settingsMatch:true};
  },()=>1,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>assert.fail('failed receipt')},
  (reason,trace)=>diagnostics.record(startupFailure('reader','bound_finish',reason,false,undefined,trace),'reader','bound_finish',reason));
  const emit=(method,params,binding=scope)=>binder.observe({method,params,sessionId:scope.sessionId},binding);
  if(document){
    emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'loader',url:'https://x.com/home'}});
    emit('Runtime.executionContextCreated',{context:{id:7,origin:'https://x.com',auxData:{isDefault:true,frameId:'frame'}}});
  }
  const identity=()=>binder.acceptIdentity('12345',inspectBootstrapCurrentSelf({location:{origin:'https://x.com',pathname:'/home'},__INITIAL_STATE__:{session:{user_id:'12345'},entities:{users:{entities:{'12345':{id_str:'12345'}}}}}}));
  const request=()=>emit('Network.requestWillBeSent',{requestId:'settings',frameId:'frame',loaderId:'loader',request:{method:'GET',url:'https://api.x.com/1.1/account/settings.json'}});
  const response=(patch={})=>emit('Network.responseReceived',{requestId:'settings',frameId:'frame',loaderId:'loader',hasExtraInfo:true,response:{url:'https://api.x.com/1.1/account/settings.json',status:200,headers:{},mimeType:'application/json',fromDiskCache:false,fromServiceWorker:false,fromPrefetchCache:false},...patch});
  return {binder,scope,diagnostics,emit,identity,request,response};
}
test('specific invalidation sites remain distinct and a generic early first cause is never replaced',async()=>{
  const cases=[
    ['target_lost',s=>s.emit('Target.detachedFromTarget',{sessionId:'private-session'})],
    ['context_lost',s=>s.emit('Runtime.executionContextDestroyed',{executionContextId:7})],
    ['navigation',s=>s.emit('Page.frameNavigated',{frame:{id:'frame',loaderId:'next',url:'https://x.com/home'}})],
    ['owner_scope',s=>s.emit('Page.loadEventFired',{}, {...s.scope,owner:2})],
    ['event_cap',s=>{for(let i=0;i<4097;i++)s.emit('Page.loadEventFired',{});}],
    ['identity_order',s=>s.identity()],
    ['settings_order',s=>s.request()],
    ['response_frame',s=>{s.identity();s.request();s.response({frameId:'child'});}],
    ['extra_shape',s=>{s.identity();s.request();s.response({hasExtraInfo:undefined});}],
    ['extra_status',s=>{s.identity();s.request();s.response();s.emit('Network.responseReceivedExtraInfo',{requestId:'settings',headers:{},statusCode:201});}],
    ['extra_unexpected',s=>{s.identity();s.request();s.response({hasExtraInfo:false});s.emit('Network.responseReceivedExtraInfo',{requestId:'settings',headers:{},statusCode:200});}],
    ['generic_early',s=>s.binder.earlyOutcome('evaluation_failed',true)],
    ['unknown',s=>s.binder.invalidate()],
  ];
  for(const [cause,change]of cases){
    const s=directSample(undefined,cause!=='identity_order');change(s);s.binder.invalidate('observed_cookie_write','cookie_candidate');s.binder.earlyOutcome('evaluation_failed',true);
    const report=await s.binder.finish(false);
    assert.throws(()=>requireBoundStartup(report,'reader',s.diagnostics),error=>{assert.equal(startupDiagnostic(error).trace.firstCause,cause);return true;});
    assert.equal(report.canAuthorize,false);assert.equal(report.cleanupVerified,false);
  }
});
test('ExtraInfo summary freezes final metadata after awaited body projection without repairing rejection',async()=>{
  let release;
  const body=new Promise(resolve=>{release=resolve;}),s=directSample(()=>body);
  s.identity();s.request();s.response();s.emit('Network.loadingFinished',{requestId:'settings',encodedDataLength:32});
  s.binder.earlyOutcome('captured',false);
  const pending=s.binder.finish(true); // Existing predicate rejects missing ExtraInfo now.
  s.emit('Network.responseReceivedExtraInfo',{requestId:'settings',headers:{},statusCode:200});release();
  const report=await pending;
  assert.equal(report.diagnostic,'scope_lost');
  let final;
  assert.throws(()=>requireBoundStartup(report,'reader',s.diagnostics),error=>{final=startupDiagnostic(error);return true;});
  assert.equal(final.trace.firstCause,'extra_missing');assert.equal(final.trace.extra,'complete');
  s.emit('Runtime.executionContextsCleared',{});assert.equal(final.trace.firstCause,'extra_missing');assert.ok(Object.isFrozen(final.trace));
});
test('guard subreason survives fallback and remains visible after a prior non-guard invalidation',async()=>{
  for(const [fault,first,cookie]of [['guard-priority','guard_header_classification','guard_header_classification'],['guard-budget','guard_request_budget','guard_request_budget'],['guard-unbound','guard_request_unbound','guard_request_unbound'],['guard-response','guard_response_correlation','guard_response_correlation'],['guard-metadata','guard_metadata_shape','guard_metadata_shape'],['guard-completion','generic_early','guard_completion_missing'],['context-priority','context_lost','guard_header_classification'],['generic-priority','generic_early','guard_header_classification'],['budget-candidate','guard_request_budget','guard_request_budget'],['candidate-budget','cookie_candidate','candidate_observed']]){
    const {d}=await run(fault,'reader');assert.equal(d.trace.firstCause,first,fault);assert.equal(d.trace.cookie,cookie,fault);assert.equal(d.trace.terminal,'scope_lost');
  }
});
test('peer-first delivery preserves the specific first guard reason without borrowing peer summaries',async()=>{
  const f=offlineFirstCausePair();await Promise.all(['reader','collector'].map(r=>f.port.execute(q('manual_prepare',r))));
  f.fail('reader','guard-priority');f.port.invalidate('collector');
  await assert.rejects(f.port.execute(q('manual_inspect','collector')),error=>{const d=startupDiagnostic(error);assert.equal(d.role,'reader');assert.equal(d.peer,true);assert.equal(d.trace.firstCause,'guard_header_classification');assert.equal(d.trace.cookie,'unknown');return true;});
  await Promise.all(['reader','collector'].map(r=>f.port.execute(q('manual_abort',r,{handle:null,preserveReader:r==='reader'}))));
});
