import test from 'node:test';
import assert from 'node:assert/strict';
import {createXManualOwnerPort,manualOwnerOperation} from './x-native-manual-owner-port.mjs';
import {offlineXManualOwnerPort} from './x-native-manual-owner-offline.mjs';
import {offlineStartupOwner} from './x-manual-startup-offline-fixture.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';
import {startupDiagnostic,startupRPCCode,startupFailure} from './x-manual-startup-diagnostic.mjs';
import {manualOwnerSnapshot} from './x-native-metadata-probe-driver.mjs';

let id=0;
const command=(op,role='reader',extra={})=>({id:++id,op,owner:1,role,...extra});
async function prepared(port){return Promise.all(['reader','collector'].map(role=>port.execute(command('manual_prepare',role))));}
const acquire=(port,role,ready)=>port.execute(command('manual_acquire',role,{handle:ready.handle,startupBudgetMS:2000,request:{challenge:'c'.repeat(48),role,...ready.pins}}));

test('the private acquire route validates and forwards only a bounded startup wait hint', async () => {
  const base=command('manual_acquire','reader',{handle:'h',startupBudgetMS:75,request:{challenge:'c'.repeat(48),role:'reader',...offlineStartupOwner('reader')}});
  assert.equal(manualOwnerOperation(base).startupBudgetMS,75);
  for(const startupBudgetMS of [undefined,null,0,-1,0.5,2001,'75',Infinity]){
    assert.throws(()=>manualOwnerOperation({...base,startupBudgetMS}),/manual_owner_unavailable/);
  }
  let received;
  const port=createXManualOwnerPort({enabled:true,owner:1,
    createOwners:()=>Object.fromEntries(['reader','collector'].map(role=>[role,{
      prepare:async()=>({handle:role,pins:offlineStartupOwner(role)}),
      acquire:async(_handle,_request,_signal,budget)=>{received=budget;return Buffer.from('{}');},abort:async()=>{},cancel:async()=>{},
    }])),
    snapshot:async role=>({...offlineStartupOwner(role),sessionId:'offline-'+role,alive:true,lifecycleReady:true,
      headless:role==='collector',windowCount:role==='reader'?1:0,bootstrapCount:role==='reader'?1:0}),retire:async()=>{},
  });
  const ready=await prepared(port);
  (await port.execute({...base,handle:ready[0].handle})).fill(0);
  assert.equal(received,75);
  await Promise.all(['reader','collector'].map(role=>port.execute(command('manual_abort',role,{handle:null,preserveReader:role==='reader'}))));
});
test('X private owner port defaults off and rejects routes, flags, accessors and arbitrary commands',()=>{
  assert.throws(()=>createXManualOwnerPort(),/manual_owner_unavailable/);
  for(const value of [command('manual_prepare','other'),command('login'),command('manual_prepare','reader',{nativeQualified:true}),
    command('manual_acquire','reader',{handle:'h',request:{}}),command('manual_abort','collector',{handle:null,preserveReader:true})]){
    assert.throws(()=>manualOwnerOperation(value),/manual_owner_unavailable/);
  }
  const value=command('manual_prepare');Object.defineProperty(value,'role',{get(){throw new Error('private');},enumerable:true});
  assert.throws(()=>manualOwnerOperation(value),/manual_owner_unavailable/);
});
test('one shared owner port acquires both roles concurrently on the same prepared documents',async()=>{
  const released=new Set();let release;
  const together=new Promise(resolve=>{release=resolve;});
  const port=offlineXManualOwnerPort({afterRelease:async role=>{released.add(role);if(released.size===2)release();await together;}});
  const ready=await prepared(port);
  const wires=await Promise.all(['reader','collector'].map((role,index)=>acquire(port,role,ready[index])));
  for(let index=0;index<2;index++){
    try{const receipt=JSON.parse(wires[index].toString('utf8'));assert.equal(receipt.documentId,ready[index].pins.documentId);assert.equal(Object.keys(receipt).length,16);}
    finally{wires[index].fill(0);}
  }
  await assert.rejects(acquire(port,'reader',ready[0]),/manual_owner_unavailable/);
});
test('both owners must be prepared; changed challenge scope never rebinds the held source',async()=>{
  const port=offlineXManualOwnerPort();const reader=await port.execute(command('manual_prepare'));
  await assert.rejects(acquire(port,'reader',reader),/manual_owner_unavailable/);
  await port.execute(command('manual_prepare','collector'));
  await assert.rejects(acquire(port,'reader',{...reader,pins:{...reader.pins,documentId:'wrong-document'}}),/manual_owner_unavailable/);
  await port.execute(command('manual_abort','reader',{handle:null,preserveReader:true}));
});
test('live document, process, profile or observer drift rejects inspection',async()=>{
  for(const field of ['ownerId','profileId','targetId','frameId','documentId','contextId','sessionId','lifecycleReady','alive']){
    let drift=false;const port=offlineXManualOwnerPort({changeSnapshot:(role,value)=>{if(drift&&role==='reader')value[field]=typeof value[field]==='boolean'?false:'changed';}});
    await prepared(port);drift=true;
    await assert.rejects(port.execute(command('manual_inspect')),/manual_owner_unavailable/);
    await port.execute(command('manual_abort','reader',{handle:null,preserveReader:true}));
  }
});
test('post-capture context loss is still observed and revokes subsequent operations',async()=>{
  const port=offlineXManualOwnerPort(),ready=await prepared(port);
  for(const [index,role]of ['reader','collector'].entries())(await acquire(port,role,ready[index])).fill(0);
  const pins=offlineStartupOwner('reader');
  port.observe('reader',{sessionId:'unrelated',method:'Runtime.executionContextsCleared'});
  assert.equal((await port.execute(command('manual_inspect'))).invalid,false);
  port.observe('reader',{sessionId:'offline-session-reader',method:'Runtime.executionContextDestroyed',params:{executionContextId:Number(pins.contextId)}});
  await assert.rejects(port.execute(command('manual_inspect')),/manual_owner_unavailable/);
});
test('ambiguous PREPARE can be cancelled without its handle and cannot deliver late evidence',async()=>{
  let entered;const started=new Promise(resolve=>{entered=resolve;});let reads=0;
  const actors=Object.fromEntries(['reader','collector'].map(role=>[role,createManualStartupOwner({run:async sink=>{
    entered();await new Promise(resolve=>sink.signal.addEventListener('abort',resolve,{once:true}));
    if(!sink.signal.aborted)reads++;
  }})]));
  const port=createXManualOwnerPort({enabled:true,owner:1,createOwners:()=>actors,snapshot:async()=>{},retire:async()=>{}});
  const pending=port.execute(command('manual_prepare'));await started;
  assert.deepEqual(await port.execute(command('manual_abort','reader',{handle:null,preserveReader:true})),{closed:true});
  await assert.rejects(pending,/manual_owner_unavailable/);assert.equal(reads,0);
});
test('cleanup may escalate from preserving a reader to retiring it on parent exit',async()=>{
  const retired=[],port=offlineXManualOwnerPort({onRetire:(role,preserve)=>retired.push([role,preserve])});
  await prepared(port);
  await port.execute(command('manual_abort','reader',{handle:null,preserveReader:true}));
  await port.execute(command('manual_abort','reader',{handle:null,preserveReader:false}));
  await port.execute(command('manual_abort','reader',{handle:null,preserveReader:true}));
  assert.deepEqual(retired,[['reader',true],['reader',false]]);
});

const inspectionFaults=[
  ['owner_dead',v=>{v.alive=false;}],['reference_invalid',v=>{v.ownerId='';}],
  ['headless_mismatch',v=>{v.headless=true;}],['window_count',v=>{v.windowCount=129;}],
  ['bootstrap_count',v=>{v.bootstrapCount=-1;}],['reader_windows',v=>{v.windowCount=0;v.bootstrapCount=0;}],
  ['reader_bootstrap',v=>{v.bootstrapCount=0;}],
  ...[['ownerId','owner_mismatch'],['profileId','profile_mismatch'],['targetId','target_mismatch'],['sessionId','session_mismatch'],
    ['frameId','frame_mismatch'],['documentId','document_mismatch'],['contextId','context_mismatch']].map(([field,condition])=>[condition,v=>{v[field]='PRIVATE_PIN';}]),
  ['lifecycle_unready',v=>{v.lifecycleReady=false;}],
];
for(const [condition,mutate]of inspectionFaults)test(`inspect_before identifies ${condition} and survives peer failure and cleanup`,async()=>{
  let drift=false;
  const port=offlineXManualOwnerPort({changeSnapshot:(role,v)=>{if(drift&&role==='reader')mutate(v);}});
  await prepared(port);drift=true;
  let first;
  await assert.rejects(port.execute(command('manual_inspect')),error=>{
    first=startupDiagnostic(error);assert.equal(first.role,'reader');assert.equal(first.phase,'inspect_before');assert.equal(first.reason,'unavailable');
    assert.equal(first.trace.firstCause,'inspection');assert.equal(first.trace.inspectStage,'node_validate');assert.equal(first.trace.inspectCondition,condition);
    assert.equal(JSON.stringify(first).includes('PRIVATE_PIN'),false);assert.ok(startupRPCCode(error).length<=80);return true;
  });
  await port.execute(command('manual_abort','reader',{handle:null,preserveReader:true}));
  await assert.rejects(port.execute(command('manual_inspect','collector')),error=>{
    const later=startupDiagnostic(error);assert.deepEqual({...later,peer:false},first);assert.equal(later.peer,true);return true;
  });
  await port.execute(command('manual_abort','collector',{handle:null,preserveReader:false}));
});
test('snapshot rejection and collector inventory have distinct closed inspection diagnostics',async()=>{
  for(const fault of ['command','collector']){
    let drift=false;const port=offlineXManualOwnerPort({changeSnapshot:(role,v)=>{
      if(!drift)return;
      if(fault==='command')throw new Error('PRIVATE_TOKEN auth_token=https://private.invalid');
      if(role==='collector')v.windowCount=1;
    }});
    await prepared(port);drift=true;
    await assert.rejects(port.execute(command('manual_inspect',fault==='command'?'reader':'collector')),error=>{
      const d=startupDiagnostic(error);assert.equal(d.trace.inspectStage,fault==='command'?'node_snapshot':'node_validate');
      assert.equal(d.trace.inspectCondition,fault==='command'?'command_failed':'collector_windows');
      assert.equal(JSON.stringify(d).includes('PRIVATE'),false);assert.equal(error.message,'xnp_manual_owner_unavailable');return true;
    });
    await Promise.all(['reader','collector'].map(role=>port.execute(command('manual_abort',role,{handle:null,preserveReader:role==='reader'}))));
  }
});
test('an earlier branded guard failure is never relabeled as an inspection command failure',async()=>{
  let drift=false;
  const port=offlineXManualOwnerPort({changeSnapshot:()=>{if(drift)throw startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,{firstCause:'cookie_candidate'});}});
  await prepared(port);drift=true;
  await assert.rejects(port.execute(command('manual_inspect')),error=>{
    const d=startupDiagnostic(error);assert.equal(d.phase,'bound_finish');assert.equal(d.reason,'observed_cookie_write');
    assert.equal(d.trace.firstCause,'cookie_candidate');assert.equal(d.trace.inspectCondition,'unknown');return true;
  });
  await Promise.all(['reader','collector'].map(role=>port.execute(command('manual_abort',role,{handle:null,preserveReader:role==='reader'}))));
});
test('context loss during an awaited snapshot survives a subsequently valid snapshot result',async()=>{
  let pending=false,entered,release;
  const started=new Promise(resolve=>{entered=resolve;}),held=new Promise(resolve=>{release=resolve;});
  const pins=offlineStartupOwner('reader');
  const port=createXManualOwnerPort({enabled:true,owner:1,
    createOwners:()=>Object.fromEntries(['reader','collector'].map(role=>[role,{prepare:async()=>({handle:role,pins:offlineStartupOwner(role)}),acquire:async()=>{},abort:async()=>{},cancel:async()=>{}}])),
    snapshot:async role=>{
      if(pending){entered();await held;}
      return{...offlineStartupOwner(role),sessionId:'offline-session-'+role,headless:role==='collector',windowCount:role==='reader'?1:0,bootstrapCount:role==='reader'?1:0,alive:true,lifecycleReady:true};
    },retire:async()=>{}});
  await prepared(port);pending=true;
  const result=port.execute(command('manual_inspect'));await started;
  port.observe('reader',{sessionId:'offline-session-reader',method:'Runtime.executionContextDestroyed',params:{executionContextId:Number(pins.contextId)}});
  release();
  await assert.rejects(result,error=>{const d=startupDiagnostic(error);assert.equal(d.role,'reader');assert.equal(d.phase,'joint');assert.equal(d.reason,'scope_lost');return true;});
  await Promise.all(['reader','collector'].map(role=>port.execute(command('manual_abort',role,{handle:null,preserveReader:role==='reader'}))));
});

function snapshotFixture(failMethod,role='reader'){
  const current={pid:1,profileId:'PRIVATE_PROFILE',targetId:'PRIVATE_TARGET',sessionId:'startup-session'},
    collector={targetId:'PRIVATE_COLLECTOR_TARGET',sessionId:'collector-startup'};
  const calls=[];
  const send=async(method,params,session,project)=>{
    calls.push([method,session]);
    if(method===failMethod)throw new Error('PRIVATE_CDP_URL_TOKEN');
    const state=role==='reader'?current:collector;
    if(method==='Target.attachToTarget')return{sessionId:'observer-session'};
    if(method==='Runtime.enable')state.manualObserver.context=7;
    if(method==='Page.getFrameTree')return role==='reader'?{frameId:'frame',loaderId:'doc'}:
      project({frameTree:{frame:{id:'frame',loaderId:'doc',url:'https://x.com/home'}}});
    if(method==='Target.getTargets')return{targetIds:['PRIVATE_TARGET','PRIVATE_OTHER_TAB']};
    if(method==='Browser.getWindowForTarget')return{windowId:1};
    return{acknowledged:true};
  };
  collector.transport={ownership:()=>({pid:2,profileId:'PRIVATE_COLLECTOR_PROFILE',alive:true}),send};
  return{calls,current,collector,read:()=>manualOwnerSnapshot({current,collector,role,document:true,pins:{frameId:'frame'},phase:'inspect_before',readerSend:send,readerAlive:()=>true})};
}
for(const [method,stage]of [['Target.attachToTarget','snapshot_attach'],['Page.enable','snapshot_page'],['Runtime.enable','snapshot_runtime'],
  ['Page.getFrameTree','snapshot_frame'],['Target.getTargets','snapshot_inventory'],['Browser.getWindowForTarget','snapshot_inventory']]){
  test(`real snapshot path identifies ${method} without exporting transport error`,async()=>{
    for(const role of method==='Target.getTargets'||method==='Browser.getWindowForTarget'?['reader']:['reader','collector']){
      await assert.rejects(snapshotFixture(method,role).read(),error=>{
        const d=startupDiagnostic(error);assert.equal(d.role,role);assert.equal(d.phase,'inspect_before');assert.equal(d.trace.inspectStage,stage);
        assert.equal(d.trace.inspectCondition,'command_failed');assert.equal(JSON.stringify(d).includes('PRIVATE'),false);return true;
      });
    }
  });
}
test('snapshot keeps observer and paused startup sessions separate and counts bootstrap window once',async()=>{
  for(const role of ['reader','collector']){
    const f=snapshotFixture(undefined,role),first=await f.read();
    assert.equal(first.sessionId,'observer-session');assert.equal(first.contextId,'7');assert.equal(first.lifecycleReady,true);
    assert.equal(first.windowCount,role==='reader'?1:0);assert.equal(first.bootstrapCount,role==='reader'?1:0);
    assert.ok(f.calls.some(([method,session])=>method==='Page.getFrameTree'&&session===(role==='reader'?'startup-session':'collector-startup')));
    const n=f.calls.filter(([method])=>method==='Target.attachToTarget').length;
    await f.read();assert.equal(f.calls.filter(([method])=>method==='Target.attachToTarget').length,n);
  }
});
