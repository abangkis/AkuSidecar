import test from 'node:test';
import assert from 'node:assert/strict';
import {createRetainedManualSetup,manualSeedDiagnostic,manualSeedDiagnosticCode} from './x-manual-retained-setup.mjs';
import {startupFailure} from './x-manual-startup-diagnostic.mjs';
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

test('failure phase and write state stay closed before, during and after the single write',async()=>{
  const cases=[
    ['account-shape','enrollment_project','candidate_absent','not_attempted'],
    ['deadline','cookie_read','deadline','not_attempted'],
    ['write-failed','seed_write','unavailable','unknown'],
    ['owner-drift','owner_after','ownership_changed','acknowledged'],
  ];
  for(const [fault,phase,reason,write] of cases){
    const f=fixture(fault);
    await assert.rejects(f.run(),error=>{
      assert.deepEqual(manualSeedDiagnostic(error),{phase,reason,write});
      assert.equal(manualSeedDiagnosticCode(error),`manual_seed_${phase}_${reason}_${write}`);
      assert.equal(JSON.stringify(error),'{}');assert.equal('cause' in error,false);return true;
    });
    await assert.rejects(f.run(),error=>{assert.deepEqual(manualSeedDiagnostic(error),{phase:'preconditions',reason:'replay',write});return true;});
  }
});

test('callback exceptions cannot inject messages, getters or diagnostic properties',async()=>{
  for(const failurePhase of ['owner_before','enrollment_evaluate','cookie_read','seed_write','owner_after']){
    let ownerCalls=0,touched=0;
    const unsafe={get message(){touched++;throw new Error('PRIVATE_SECRET');},phase:'seed_write',reason:'acknowledged',write:'acknowledged'};
    const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>0,
      ownership:async()=>{ownerCalls++;if(failurePhase===(ownerCalls===1?'owner_before':'owner_after'))throw unsafe;return {readerPID:1,readerProfile:'r',collectorPID:2,collectorProfile:'c'};},
      readEnrollment:async()=>{if(failurePhase==='enrollment_evaluate')throw unsafe;return '123';},
      readAllowedCookies:async()=>{if(failurePhase==='cookie_read')throw unsafe;return [{},{}];},
      writeAllowedCookies:async()=>{if(failurePhase==='seed_write')throw unsafe;return true;},
    });
    await assert.rejects(run(),error=>{
      assert.deepEqual(manualSeedDiagnostic(error),{phase:failurePhase,reason:'unavailable',write:failurePhase==='seed_write'?'unknown':failurePhase==='owner_after'?'acknowledged':'not_attempted'});
      assert.equal(touched,0);return true;
    });
  }
  for(const error of [new Error('xnp_manual_seed_seed_write_rejected_acknowledged'),{phase:'seed_write',reason:'rejected',write:'acknowledged'},null])assert.equal(manualSeedDiagnosticCode(error),null);
});

test('false write acknowledgement remains unknown and a post-ack deadline remains acknowledged',async()=>{
  for(const outcome of ['false','post-ack-deadline','selection']){
    let clock=0,writes=0;
    const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>clock,
      ownership:async()=>({readerPID:1,readerProfile:'r',collectorPID:2,collectorProfile:'c'}),
      readEnrollment:async()=> '123',readAllowedCookies:async()=>outcome==='selection'?[]:[{},{}],
      writeAllowedCookies:async()=>{writes++;if(outcome==='post-ack-deadline')clock=60000;return outcome!=='false';},
    });
    await assert.rejects(run(),error=>{
      assert.deepEqual(manualSeedDiagnostic(error),outcome==='selection'?{phase:'cookie_select',reason:'rejected',write:'not_attempted'}:{phase:'seed_write',reason:outcome==='false'?'unavailable':'deadline',write:outcome==='false'?'unknown':'acknowledged'});return true;
    });
    assert.equal(writes,outcome==='selection'?0:1);
  }
});

test('observer cleanup still runs but cannot publish seed success at the original sixty-second boundary',async()=>{
  let clock=0,cleaned=0;
  const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>clock,
    ownership:async()=>({readerPID:1,readerProfile:'r',collectorPID:2,collectorProfile:'c'}),
    readEnrollment:async()=> '123',readAllowedCookies:async()=>[{},{}],writeAllowedCookies:async()=>true,
    cleanupEnrollment:async()=>{await Promise.resolve();cleaned++;clock=60000;},
  });
  await assert.rejects(run(),error=>{assert.deepEqual(manualSeedDiagnostic(error),{phase:'enrollment_cleanup',reason:'deadline',write:'acknowledged'});return true;});
  assert.equal(cleaned,1);await assert.rejects(run(),error=>manualSeedDiagnostic(error).reason==='replay');
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

test('synchronous final assertion prevents dispatch while acknowledgment survives later failure and cleanup',async()=>{
  for(const stage of ['assert','async-assert','pending','ack','cleanup','none']){
    let writes=0,cleaned=0,lateAck;
    const failure=()=>startupFailure('reader','bound_finish','observed_cookie_write',false,undefined,
      {firstCause:'guard_request_unbound',guardCondition:'response_cache',guardEpoch:'before_seed',guardDiskCache:'true',guardServiceWorker:'absent',guardPrefetchCache:'false'});
    const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>0,
      ownership:async()=>({readerPID:1,readerProfile:'r',collectorPID:2,collectorProfile:'c'}),readEnrollment:async()=> '123',readAllowedCookies:async()=>[{},{}],
      assertBeforeWrite:()=>{if(stage==='assert')throw failure();if(stage==='async-assert')return Promise.resolve();},
      writeAllowedCookies:async(_,ack)=>{writes++;lateAck=ack;if(stage==='pending')throw failure();ack();if(stage==='ack')throw failure();return true;},
      cleanupEnrollment:async()=>{cleaned++;if(stage==='cleanup')throw failure();},
    });
    if(stage==='none')await run();
    else await assert.rejects(run(),error=>{
      const write=['assert','async-assert'].includes(stage)?'not_attempted':stage==='pending'?'unknown':'acknowledged';
      assert.equal(manualSeedDiagnostic(error).write,write);
      const code=manualSeedDiagnosticCode(error);assert.ok(code.length<=80);assert.equal(code.includes('PRIVATE'),false);
      if(stage!=='async-assert')assert.match(code,/^manual_seed_v2_/);return true;
    });
    assert.equal(writes,['assert','async-assert'].includes(stage)?0:1);assert.equal(cleaned,1);
    if(lateAck)assert.throws(lateAck,error=>manualSeedDiagnostic(error)?.reason==='replay');
  }
});
