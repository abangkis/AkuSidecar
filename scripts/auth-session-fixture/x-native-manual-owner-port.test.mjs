import test from 'node:test';
import assert from 'node:assert/strict';
import {createXManualOwnerPort,manualOwnerOperation} from './x-native-manual-owner-port.mjs';
import {offlineXManualOwnerPort} from './x-native-manual-owner-offline.mjs';
import {offlineStartupOwner} from './x-manual-startup-offline-fixture.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';

let id=0;
const command=(op,role='reader',extra={})=>({id:++id,op,owner:1,role,...extra});
async function prepared(port){return Promise.all(['reader','collector'].map(role=>port.execute(command('manual_prepare',role))));}
const acquire=(port,role,ready)=>port.execute(command('manual_acquire',role,{handle:ready.handle,request:{challenge:'c'.repeat(48),role,...ready.pins}}));
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
