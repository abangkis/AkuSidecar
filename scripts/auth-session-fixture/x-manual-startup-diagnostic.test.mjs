import test from 'node:test';
import assert from 'node:assert/strict';
import {STARTUP_ROLES,STARTUP_PHASES,STARTUP_REASONS,startupFailure,startupDiagnostic,startupRPCCode,startupFailureLatch,requireBoundStartup} from './x-manual-startup-diagnostic.mjs';
import {createManualStartupCapture} from './x-manual-startup-source.mjs';
import {offlineStartupOwner,runOfflineStartupRecipe} from './x-manual-startup-offline-fixture.mjs';
import {offlineXManualOwnerPort} from './x-native-manual-owner-offline.mjs';
import {executeManualReply} from './x-native-metadata-probe-driver.mjs';
const request=role=>({challenge:'c'.repeat(48),role,...offlineStartupOwner(role)});
const command=(op,role,extra={})=>({id:1,op,owner:1,role,...extra});

test('closed startup diagnostic codes are branded, bounded and cannot echo spoofed errors or getters',async()=>{
  let longest=0,touched=0;
  for(const role of STARTUP_ROLES)for(const phase of STARTUP_PHASES)for(const reason of STARTUP_REASONS)for(const peer of [false,true]){
    const code=startupRPCCode(startupFailure(role,phase,reason,peer));longest=Math.max(longest,code.length);assert.match(code,/^manual_initial_[a-z_]+$/);
  }
  assert.ok(longest<=80);
  const unsafe={get message(){touched++;throw new Error('PRIVATE_SECRET');},role:'reader',phase:'identity',reason:'PRIVATE_SECRET'};
  for(const error of [unsafe,new Error('xnp_manual_initial_reader_identity_PRIVATE_primary'),null])assert.equal(startupRPCCode(error),'manual_initial_unknown_acquire_rpc_unavailable_primary');
  assert.equal(touched,0);
  const replies=[];await executeManualReply(command('manual_acquire','reader'),async()=>{throw unsafe;},(...args)=>replies.push(args));
  assert.equal(replies[0][2],'manual_initial_unknown_acquire_rpc_unavailable_primary');assert.equal(touched,0);
});
for(const role of ['reader','collector']){
  test(`${role} negative bound result survives source and async RPC without publishing a wire`,async()=>{
    const diagnostics=startupFailureLatch();
    const capture=createManualStartupCapture({role,diagnostics,run:async(q,sink)=>{
      const observation=await runOfflineStartupRecipe(q,sink,{change:({binder})=>binder.invalidate('observed_cookie_write')});
      requireBoundStartup(observation,role,diagnostics);
    }});
    const replies=[];await executeManualReply(command('manual_acquire',role),()=>capture(request(role)),(...args)=>replies.push(args));
    assert.equal(replies[0][1],undefined);assert.equal(replies[0][2],`manual_initial_${role}_bound_finish_observed_cookie_write_primary`);
  });
}
for(const fault of ['missing','duplicate','scope','time']){
  test(`startup ${fault} is a closed category and never publishes a receipt`,async()=>{
    const role='collector',q=request(role),sample={accountId:'1908123456789012345',...offlineStartupOwner(role),settingsRequestId:'offline-settings',identityAt:0,settingsAt:0,qualification:'api_v1.cookie-auth-rejection.2026-10-20'};
    const capture=createManualStartupCapture({role,now:()=>0,wallNow:()=>0,run:async(_,sink)=>{
      if(fault==='missing')return;
      sink.onPrivateStartup({...sample,...(fault==='scope'?{documentId:'foreign'}:{}),...(fault==='time'?{settingsAt:1}:{})});
      if(fault==='duplicate')sink.onPrivateStartup(sample);
    }});
    await assert.rejects(capture(q),error=>{
      const d=startupDiagnostic(error);assert.equal(d.role,role);assert.equal(d.reason,{missing:'sample_missing',duplicate:'sample_duplicate',scope:'scope_mismatch',time:'time_invalid'}[fault]);return true;
    });
  });
}
test('first observed role loss is retained even when a peer failure is the first RPC reply',async()=>{
  const port=offlineXManualOwnerPort();await Promise.all(['reader','collector'].map(role=>port.execute(command('manual_prepare',role))));
  port.observe('reader',{sessionId:'offline-session-reader',method:'Runtime.executionContextDestroyed',params:{executionContextId:7}});
  await assert.rejects(port.execute(command('manual_inspect','collector')),error=>{assert.deepEqual(startupDiagnostic(error),{role:'reader',phase:'joint',reason:'scope_lost',peer:true});return true;});
  await assert.rejects(port.execute(command('manual_inspect','reader')),error=>{assert.deepEqual(startupDiagnostic(error),{role:'reader',phase:'joint',reason:'scope_lost',peer:false});return true;});
  await Promise.all(['reader','collector'].map(role=>port.execute(command('manual_abort',role,{handle:null,preserveReader:role==='reader'}))));
});
test('primary failure is immutable across secondary cancellation and unattributed joint deadlines stay pair-scoped',()=>{
  const latch=startupFailureLatch();
  latch.record(startupFailure('collector','settings','source_unavailable'),'collector','settings');
  const peer=latch.record(startupFailure('reader','joint','canceled'),'reader','joint');
  assert.deepEqual(startupDiagnostic(peer),{role:'collector',phase:'settings',reason:'source_unavailable',peer:true});
  assert.equal(startupDiagnostic(startupFailure('pair','joint','deadline')).role,'pair');
});
