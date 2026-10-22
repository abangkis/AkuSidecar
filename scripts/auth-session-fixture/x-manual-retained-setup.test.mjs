import test from 'node:test';
import assert from 'node:assert/strict';
import {createRetainedManualSetup} from './x-manual-retained-setup.mjs';
function fixture(fault){
  let reads=0,writes=0,clock=0,ownReads=0;
  const state={readerPID:1,readerProfile:'qa-reader',collectorPID:2,collectorProfile:'qa-collector'};
  const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>clock,
    ownership:async()=>{ownReads++;return {...state,...(fault==='owner-drift'&&ownReads>1?{collectorPID:3}:{})};},
    readEnrollment:async()=>fault==='account-shape'?1:'1908123456789012345',
    readAllowedCookies:async()=>{reads++;if(fault==='deadline')clock=60000;return [{name:'auth_token',value:'private-synthetic-a'},{name:'ct0',value:'private-synthetic-b'}];},
    writeAllowedCookies:async()=>{writes++;if(fault==='write-failed')throw new Error('private-cookie-value');return true;},
  });
  return{run,counts:()=>({reads,writes})};
}
test('retained manual setup requires separate enabled and one-transfer gates',()=>{
  for(const args of [{},{enabled:true},{enabled:true,transferAllowed:true}])assert.throws(()=>createRetainedManualSetup(args),/manual_setup_unavailable/);
});
test('one setup emits only private enrollment/owner metadata and cannot transfer twice',async()=>{
  const f=fixture(),reply=await f.run();
  assert.equal(reply.seeded,true);assert.equal(reply.homeReady,true);
  assert.equal(Object.keys(reply).length,7);
  assert.equal(JSON.stringify(reply).includes('private-synthetic'),false);
  await assert.rejects(f.run(),/manual_setup_unavailable/);
  assert.deepEqual(f.counts(),{reads:1,writes:1});
});
test('failed, expired or owner-changing setup stays one-shot and has no raw errors',async()=>{
  for(const fault of ['account-shape','deadline','owner-drift','write-failed']){
    const f=fixture(fault);
    await assert.rejects(f.run(),e=>e.message==='xnp_manual_setup_unavailable');
    await assert.rejects(f.run(),e=>e.message==='xnp_manual_setup_unavailable');
    if(fault==='account-shape')assert.deepEqual(f.counts(),{reads:0,writes:0});
    if(fault==='deadline')assert.deepEqual(f.counts(),{reads:1,writes:0});
  }
});
