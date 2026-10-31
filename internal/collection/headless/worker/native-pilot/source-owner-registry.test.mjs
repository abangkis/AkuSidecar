import test from 'node:test';
import assert from 'node:assert/strict';
import {createSourceOwnerRegistry,sourceOwnerRegistryFailure} from './source-owner-registry.mjs';
const deferred=()=>{let resolve;const promise=new Promise(done=>{resolve=done;});return{promise,resolve};};
const tick=()=>new Promise(done=>setImmediate(done));

test('sealed source registry isolates factories and never accepts a provider from control params',async()=>{
  const calls=[];
  const registry=createSourceOwnerRegistry({instagram:async()=>({execute:async(op,p)=>{calls.push([op,p]);return{ready:true};},close:async()=>{}})});
  await assert.rejects(registry.execute('x','source_bootstrap',{},{}));
  await assert.rejects(registry.execute('__proto__','source_bootstrap',{},{}));
  await assert.rejects(registry.execute('instagram','source_initialize',{},{}));
  assert.deepEqual(await registry.execute('instagram','source_bootstrap',{},{}),{bound:true});
  assert.deepEqual(await registry.execute('instagram','source_initialize',{},{}),{ready:true});
  assert.equal(calls.length,1);
  await assert.rejects(registry.execute('instagram','source_bootstrap',{},{}),error=>sourceOwnerRegistryFailure(error)==='source_replay');
  await assert.rejects(registry.execute('instagram','Network.getCookies',{},{}));
  await registry.close();
});

test('a failed bootstrap remains consumed',async()=>{
  let attempts=0;
  const registry=createSourceOwnerRegistry({instagram:async()=>{attempts++;throw new Error('private-token');}});
  await assert.rejects(registry.execute('instagram','source_bootstrap',{},{}));
  await assert.rejects(registry.execute('instagram','source_bootstrap',{},{}));
  assert.equal(attempts,1);
  await assert.rejects(registry.close(),error=>sourceOwnerRegistryFailure(error)==='source_cleanup_uncertain');
});

test('manual retry is exact-empty, IG-only and cannot overlap an active operation or retired owner', async () => {
  const capture = deferred(), calls = [];
  const factory = async () => ({execute: async (op, params) => {
    calls.push([op, params]);
    return op === 'source_capture' ? capture.promise : {rearmed: true, retryCount: 1, retryLimit: 2};
  }, close: async () => {}});
  const registry = createSourceOwnerRegistry({instagram: factory, x: factory});
  await registry.execute('instagram', 'source_bootstrap', {}, {});
  await registry.execute('x', 'source_bootstrap', {}, {});
  await assert.rejects(registry.execute('x', 'source_retry', {}, {}));
  await assert.rejects(registry.execute('instagram', 'source_retry', {cleanup: 'verified'}, {}));
  const getter = {}; Object.defineProperty(getter, 'authority', {get() { assert.fail('must not read control getter'); }});
  await assert.rejects(registry.execute('instagram', 'source_retry', getter, {}));
  const pending = registry.execute('instagram', 'source_capture', {}, {});
  await assert.rejects(registry.execute('instagram', 'source_retry', {}, {}), error => sourceOwnerRegistryFailure(error) === 'source_busy');
  capture.resolve({}); await pending;
  assert.deepEqual(await registry.execute('instagram', 'source_retry', {}, {}), {rearmed: true, retryCount: 1, retryLimit: 2});
  assert.deepEqual(calls.map(call => call[0]), ['source_capture', 'source_retry']);
  await registry.execute('instagram', 'source_close', {}, {});
  await assert.rejects(registry.execute('instagram', 'source_retry', {}, {}));
  await registry.close();
});

test('terminal end and close supersede an unacknowledged retry receipt',async()=>{
  for(const terminal of ['source_end','source_close']){
    const retry=deferred();
    const registry=createSourceOwnerRegistry({instagram:async()=>({execute:async op=>op==='source_retry'?retry.promise:{closed:true},close:async()=>{}})});
    await registry.execute('instagram','source_bootstrap',{},{});
    const pending=registry.execute('instagram','source_retry',{},{});
    await registry.execute('instagram',terminal,{},{});
    retry.resolve({rearmed:true,retryCount:1,retryLimit:2,descriptor:{source:'instagram'}});
    await assert.rejects(pending,error=>sourceOwnerRegistryFailure(error)==='source_closed');
    await registry.close();
  }
});

test('terminal cleanup can interrupt capture and late capture cannot publish',async()=>{
  const capture=deferred();let closed=0;
  const registry=createSourceOwnerRegistry({instagram:async()=>({
    execute:async op=>op==='source_capture'?capture.promise:{closed:true},close:async()=>{closed++;},
  })});
  await registry.execute('instagram','source_bootstrap',{},{});
  const pending=registry.execute('instagram','source_capture',{},{});
  await assert.rejects(registry.execute('instagram','source_proof',{},{}),error=>sourceOwnerRegistryFailure(error)==='source_busy');
  await registry.execute('instagram','source_end',{},{});
  capture.resolve({observation:'must-not-publish'});
  await assert.rejects(pending,error=>sourceOwnerRegistryFailure(error)==='source_closed');
  await registry.close();assert.equal(closed,1);
});

test('late factory completion after shutdown is cleaned before it can publish binding',async()=>{
  const factory=deferred();let closed=0;
  const registry=createSourceOwnerRegistry({instagram:()=>factory.promise});
  const pending=registry.execute('instagram','source_bootstrap',{},{});
  await tick();const closing=registry.close();
  factory.resolve({execute:async()=>({}),close:async()=>{closed++;}});
  await assert.rejects(pending,error=>sourceOwnerRegistryFailure(error)==='source_closed');
  await closing;assert.equal(closed,1);
  await assert.rejects(registry.execute('instagram','source_bootstrap',{},{}));
});

test('cleanup uncertainty is retained independently of repeated close',async()=>{
  let closes=0;
  const registry=createSourceOwnerRegistry({instagram:async()=>({execute:async()=>({}),close:async()=>{closes++;throw new Error('secret');}})});
  await registry.execute('instagram','source_bootstrap',{},{});
  const closing=registry.close();
  await assert.rejects(closing,error=>sourceOwnerRegistryFailure(error)==='source_cleanup_uncertain');
  assert.equal(registry.close(),closing);assert.equal(closes,1);
});

test('factory getters are not invoked',()=>{
  let reads=0;const factories={};Object.defineProperty(factories,'instagram',{enumerable:true,get(){reads++;return()=>{};}});
  assert.throws(()=>createSourceOwnerRegistry(factories));assert.equal(reads,0);
});

test('explicit source close joins and retires a factory still starting',async()=>{
  const factory=deferred();let closed=0;
  const registry=createSourceOwnerRegistry({instagram:()=>factory.promise});
  const pending=registry.execute('instagram','source_bootstrap',{},{});
  await tick();const closing=registry.execute('instagram','source_close',{},{});
  factory.resolve({execute:async()=>({}),close:async()=>{closed++;}});
  await assert.rejects(pending,error=>sourceOwnerRegistryFailure(error)==='source_closed');
  assert.deepEqual(await closing,{closed:true});assert.equal(closed,1);await registry.close();
});
