import test from 'node:test';
import assert from 'node:assert/strict';
import {createApplicationSeedGrant} from './x-application-seed-grant.mjs';
import {createRetainedManualSetup,manualSeedDiagnostic,manualSeedFailure} from './x-manual-retained-setup.mjs';
import {manualSeedRPCCode} from './x-native-metadata-probe-driver.mjs';

const ownership={readerPID:101,readerProfile:'synthetic-reader',collectorPID:102,collectorProfile:'synthetic-collector'};
test('application grant defaults off and cannot come from QA environment flags',()=>{
  assert.throws(()=>createApplicationSeedGrant(),/xnp_manual_setup_unavailable/u);
  assert.throws(()=>createApplicationSeedGrant({runSeed:()=>{},budgetMS:60001}),/xnp_manual_setup_unavailable/u);
});
test('private application grant reuses guarded setup and preserves exact seven-field receipt',async()=>{
  let grant,writes=0,cleanups=0,clock=100;
  const synthetic=[{name:'auth_token',value:'SYNTHETIC_AUTH'},{name:'ct0',value:'SYNTHETIC_CSRF'}];
  const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>clock,
    ownership:async()=>ownership,readEnrollment:async()=>{assert(grant.transferAllowed());return'123';},
    readAllowedCookies:async()=>synthetic,beforeWrite:async()=>grant.assertCurrent(),assertBeforeWrite:()=>grant.assertCurrent(),
    writeAllowedCookies:async(cookies,ack)=>{assert.equal(cookies,synthetic);assert(grant.transferAllowed());writes++;ack();return true;},
    cleanupEnrollment:async()=>{cleanups++;},
  });
  grant=createApplicationSeedGrant({runSeed:run,now:()=>clock,wallNow:()=>100000});
  assert.equal(grant.transferAllowed(),false);assert.equal(grant.startupAllowed(),false);
  const receipt=await grant.run({id:1,op:'manual_seed_bootstrap',owner:1});
  assert.deepEqual(receipt,{accountId:'123',...ownership,seeded:true,homeReady:true});assert.equal(Object.keys(receipt).length,7);
  assert.equal(writes,1);assert.equal(cleanups,1);assert.equal(grant.transferAllowed(),false);assert.equal(grant.startupAllowed(),true);
  await assert.rejects(grant.run({id:2}),error=>manualSeedDiagnostic(error)?.reason==='replay');assert.equal(writes,1);
  assert.doesNotMatch(JSON.stringify(receipt),/SYNTHETIC_AUTH|SYNTHETIC_CSRF/u);
  clock+=59999;assert.equal(grant.startupAllowed(),true);grant.assertCaptureDeadline(160000);
  assert.throws(()=>grant.assertCaptureDeadline(160001),error=>manualSeedDiagnostic(error)?.reason==='deadline');
  clock++;assert.equal(grant.startupAllowed(),false);assert.equal(grant.transferAllowed(),false);
});
test('failed application enrollment/guard never gains startup or a second seed',async()=>{
  for(const failure of ['enrollment','guard']){
    let grant,writes=0,cleanup=0;
    const run=createRetainedManualSetup({enabled:true,transferAllowed:true,ownership:async()=>ownership,
      readEnrollment:async()=>{if(failure==='enrollment')throw new Error('auth_token=SECRET');return'123';},
      readAllowedCookies:async()=>[{name:'auth_token',value:'fake'},{name:'ct0',value:'fake'}],
      assertBeforeWrite:()=>{grant.assertCurrent();if(failure==='guard')throw manualSeedFailure('frame_after','document_changed');},
      writeAllowedCookies:async()=>{writes++;return true;},cleanupEnrollment:async()=>{cleanup++;},
    });
    grant=createApplicationSeedGrant({runSeed:run});
    await assert.rejects(grant.run({}),error=>{assert.doesNotMatch(manualSeedRPCCode(error),/SECRET|auth_token/u);return true;});
    await assert.rejects(grant.run({}),error=>manualSeedDiagnostic(error)?.reason==='replay');
    assert.equal(writes,0);assert.equal(cleanup,1);assert.equal(grant.startupAllowed(),false);assert.equal(grant.transferAllowed(),false);
  }
});
test('original seed deadline and clock regression revoke grant before a late write',async()=>{
  let clock=100,grant,writes=0;
  const run=createRetainedManualSetup({enabled:true,transferAllowed:true,now:()=>clock,ownership:async()=>ownership,
    readEnrollment:async()=>'123',readAllowedCookies:async()=>[{name:'auth_token',value:'fake'},{name:'ct0',value:'fake'}],
    beforeWrite:async()=>{clock+=60000;},assertBeforeWrite:()=>grant.assertCurrent(),writeAllowedCookies:async()=>{writes++;return true;},
  });
  grant=createApplicationSeedGrant({runSeed:run,now:()=>clock});
  await assert.rejects(grant.run({}),error=>manualSeedDiagnostic(error)?.reason==='deadline');assert.equal(writes,0);assert.equal(grant.startupAllowed(),false);
  clock=100;const regressed=createApplicationSeedGrant({runSeed:async()=>{clock--;return{};},now:()=>clock});
  await assert.rejects(regressed.run({}),error=>manualSeedDiagnostic(error)?.reason==='clock_regressed');assert.equal(regressed.transferAllowed(),false);
});
test('hanging seed has a bounded one-use failure and no late successful authority',async()=>{
  let release;const grant=createApplicationSeedGrant({runSeed:()=>new Promise(accept=>{release=accept;}),budgetMS:5});
  await assert.rejects(grant.run({}),error=>manualSeedDiagnostic(error)?.reason==='deadline');
  release({seeded:true});await new Promise(accept=>setImmediate(accept));
  assert.equal(grant.startupAllowed(),false);assert.equal(grant.transferAllowed(),false);
  await assert.rejects(grant.run({}),error=>manualSeedDiagnostic(error)?.reason==='replay');
});

test('cancel before seed consumes the grant without invoking the recipe',async()=>{
  let calls=0;
  const grant=createApplicationSeedGrant({runSeed:async()=>{calls++;}});
  grant.cancel();grant.cancel();
  await assert.rejects(grant.run({}),error=>manualSeedDiagnostic(error)?.reason==='replay');
  assert.equal(calls,0);assert.equal(grant.transferAllowed(),false);
});

test('cancel during seed rejects the wait and prevents a late cookie write',async()=>{
  let finish,started,writes=0,grant;
  const ready=new Promise(resolve=>{started=resolve;});
  grant=createApplicationSeedGrant({runSeed:async()=>{
    started();await new Promise(resolve=>{finish=resolve;});
    grant.assertCurrent();writes++;return{seeded:true};
  }});
  const pending=grant.run({});await ready;
  grant.cancel();
  await assert.rejects(pending,error=>manualSeedDiagnostic(error)?.reason==='rejected');
  finish();await new Promise(resolve=>setImmediate(resolve));
  assert.equal(writes,0);assert.equal(grant.startupAllowed(),false);
  await assert.rejects(grant.run({}),error=>manualSeedDiagnostic(error)?.reason==='replay');
});

test('cancel after seed revokes startup and capture without changing a later fresh grant',async()=>{
  const grant=createApplicationSeedGrant({runSeed:async()=>({seeded:true})});
  await grant.run({});assert.equal(grant.startupAllowed(),true);
  grant.cancel();assert.equal(grant.startupAllowed(),false);
  assert.throws(()=>grant.assertCaptureDeadline(Date.now()));
  const fresh=createApplicationSeedGrant({runSeed:async()=>({seeded:true})});
  await fresh.run({});assert.equal(fresh.startupAllowed(),true);
});

test('deadline first cause survives cancellation and a late rejected operation',async()=>{
  let rejectLate;
  const grant=createApplicationSeedGrant({budgetMS:5,runSeed:()=>new Promise((_,reject)=>{rejectLate=reject;})});
  let original;
  await assert.rejects(grant.run({}),error=>{original=error;return manualSeedDiagnostic(error)?.reason==='deadline';});
  grant.cancel();rejectLate(Error('PRIVATE'));
  await new Promise(resolve=>setImmediate(resolve));
  assert.throws(()=>grant.assertCaptureDeadline(Date.now()),error=>error===original);
  assert.equal(grant.startupAllowed(),false);
});
