import test from 'node:test';
import assert from 'node:assert/strict';
import {ManualObservationDeadline} from './x-manual-observation-deadline.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';
import {runOfflineStartupRecipe} from './x-manual-startup-offline-fixture.mjs';
import {startupDiagnostic,startupRPCCode,closedStartupTrace,startupFailure,startupFailureLatch} from './x-manual-startup-diagnostic.mjs';
import {IdentityFreshnessLedger} from './x-identity-freshness-ledger.mjs';
import {ManualStartupObservation} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';

test('actual prepared actor and binder survive an 11s held pause under shared deadline without resetting age',async()=>{
  let time=0,reads=0,authority;
  const owner=createManualStartupOwner({role:'reader',now:()=>time,wallNow:()=>1000,run:sink=>{
    authority=sink.observationDeadline;
    return runOfflineStartupRecipe({role:'reader'},sink,{now:()=>time,beforeIdentityRead:async pins=>{await sink.beforeIdentityRead(pins);reads++;time+=100;}});
  }});
  const ready=await owner.prepare(undefined,19000);time=11000;
  const wire=await owner.acquire(ready.handle,{challenge:'c'.repeat(48),role:'reader',...ready.pins});wire.fill(0);
  authority.remaining();assert.equal(reads,1);assert.equal(authority.expiresAt,18000);
  assert.deepEqual(authority.snapshot(),{observationAgeMS:11100,prepareAgeMS:0,pausedAgeMS:11000,acquireAgeMS:100});
  await owner.abort(ready.handle);
});

test('expired prepared actor rejects before releasing identity and reports bounded private-free ages',async()=>{
  let time=0,reads=0;
  const owner=createManualStartupOwner({role:'reader',now:()=>time,wallNow:()=>1000,run:sink=>runOfflineStartupRecipe({role:'reader'},sink,{now:()=>time,beforeIdentityRead:async pins=>{await sink.beforeIdentityRead(pins);reads++;}})});
  const ready=await owner.prepare(undefined,19000);time=18000;
  await assert.rejects(owner.acquire(ready.handle,{challenge:'c'.repeat(48),role:'reader',...ready.pins}),error=>{
    const d=startupDiagnostic(error);assert.equal(d.trace.deadlineCondition,'prepared_expired');assert.equal(d.trace.pausedAgeMS,18000);assert.match(startupRPCCode(error),/^manual_initial_v10_[0-9_]+$/);return true;
  });assert.equal(reads,0);await owner.abort(ready.handle);
});

test('ledger shares observation deadline while retaining strict five-second identity freshness',()=>{
  let time=0;const deadline=new ManualObservationDeadline(19000,{now:()=>time,wallNow:()=>1000});
  const guard=new ManualStartupObservation('frame','session');
  const ledger=new IdentityFreshnessLedger({owner:1,profileId:'profile',sessionId:'session',targetId:'target',frameId:'frame',role:'reader'},0,guard,deadline);
  const call=(method,...args)=>ledger[method](ledger.revision,ledger.generation,...args);
  call('bindDocument',{documentId:'doc',contextId:7},0);time=11000;call('observeIdentity','12345',time);guard.finish();call('observeRoot',guard,time);call('update',{identityAt:time,settingsAt:time,settingsRequestId:'settings',login:true},time);
  assert.equal(ledger.read(time).state,'ready');assert.equal(ledger.read(16001).state,'stale');assert.equal(ledger.read(18000).state,'expired');
});

test('clock regression and diagnostic accessors cannot introduce raw clock values or secrets',()=>{
  let time=10;const deadline=new ManualObservationDeadline(2000,{now:()=>time,wallNow:()=>1000,role:'reader'});time=9;
  assert.throws(()=>deadline.remaining(),error=>startupDiagnostic(error).trace.deadlineCondition==='clock_regressed');
  const input=Object.defineProperty({prepareAgeMS:Infinity,pausedAgeMS:60001},'observationAgeMS',{get(){throw Error('secret');}});
  const trace=closedStartupTrace(input);assert.equal(trace.observationAgeMS,-1);assert.equal(trace.prepareAgeMS,-1);assert.equal(trace.pausedAgeMS,-1);
});

test('production binder classifies clock failures before generic stale invalidation',()=>{
 for(const [value,condition]of [[9,'clock_regressed'],[NaN,'clock_nonfinite']]){
  let time=10;const deadline=new ManualObservationDeadline(2000,{now:()=>time,wallNow:()=>1000,role:'reader'}),seen=[];
  const binder=createManualV2BoundObservation({owner:1,profileId:'profile',sessionId:'session',targetId:'target'},'frame',async()=>{},()=>time,undefined,{deferCookieMutation:false,onPrivateResult:()=>{}},(reason,trace)=>seen.push({reason,trace}),undefined,{role:'reader',observationDeadline:deadline});
  time=value;assert.throws(()=>binder.startupPins(7));assert.equal(seen[0].reason,'time_invalid');assert.equal(seen[0].trace.deadlineCondition,condition);assert.equal(seen.length,1);
 }
});

test('same originating failure can gain missing ages without peer contamination or replacing frozen ages',()=>{
 const latch=startupFailureLatch();
 latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'navigation'}),'reader','bound_finish');
 latch.record(startupFailure('collector','bound_finish','scope_lost',false,undefined,{firstCause:'navigation',observationAgeMS:999}),'collector','bound_finish');
 assert.equal(latch.current().trace.observationAgeMS,-1);
 latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'navigation',observationAgeMS:11000,prepareAgeMS:10,pausedAgeMS:10900,acquireAgeMS:90}),'reader','bound_finish');
 assert.equal(latch.current().trace.observationAgeMS,11000);
 latch.record(startupFailure('reader','bound_finish','scope_lost',false,undefined,{firstCause:'navigation',observationAgeMS:12000}),'reader','bound_finish');
 assert.equal(latch.current().trace.observationAgeMS,11000);
});

test('invalid prepare deadline detaches listener and consumes actor without starting runner',async()=>{
 let listeners=0,runs=0;
 const signal={aborted:false,addEventListener(){listeners++;},removeEventListener(){listeners--;}};
 const owner=createManualStartupOwner({role:'reader',now:()=>0,wallNow:()=>1000,run:async()=>{runs++;}});
 await assert.rejects(owner.prepare(signal,1000));assert.equal(listeners,0);assert.equal(runs,0);
 await assert.rejects(owner.prepare(signal,2000));assert.equal(listeners,0);assert.equal(runs,0);
 await owner.cancel();
});

test('expired observation event is a nonthrowing domain failure and cannot authorize evidence',async()=>{
 let time=0,published=false;const seen=[];
 const deadline=new ManualObservationDeadline(2000,{now:()=>time,wallNow:()=>1000,role:'reader'});
 const scope={owner:1,profileId:'profile',sessionId:'session',targetId:'target'};
 const binder=createManualV2BoundObservation(scope,'frame',async()=>{},()=>time,undefined,{deferCookieMutation:false,onPrivateResult:()=>{},onPrivateStartup:()=>{published=true;}},(reason,trace)=>seen.push({reason,trace}),undefined,{role:'reader',observationDeadline:deadline});
 time=1000;
 assert.doesNotThrow(()=>binder.observe({method:'Runtime.executionContextsCleared',params:{},sessionId:'session'},scope));
 assert.equal(seen.length,1);assert.equal(seen[0].reason,'deadline');assert.equal(seen[0].trace.deadlineCondition,'observation_expired');
 assert.equal(binder.identityReady(),false);assert.equal((await binder.done).status,'failed');
 await assert.rejects(binder.finish(true));assert.equal(published,false);
});
