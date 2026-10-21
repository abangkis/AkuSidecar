import test from 'node:test';
import assert from 'node:assert/strict';
import { manualChromeCommand,runManualChromeDriver,boundedFixtureAcquire } from './chrome-manual-pilot-driver.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';
test('manual Chrome fixture rejects extra fields, arbitrary sites and unowned roles',()=>{
  for(const value of [{op:'fixture_bootstrap',withholdReady:true},{id:1,op:'prepare',owner:2},
    {id:1,op:'prepare',owner:1,url:'https://x.com'}, {id:1,op:'launch',role:'owner',nativeReaderStartup:false},
    {id:1,op:'seed_fixture',owner:1,value:'real-cookie-value'},{id:1,op:'abort',owner:1,handle:'h',preserveReader:'true'}])
    assert.throws(()=>manualChromeCommand(value),/fixture_manual_rejected/);
  assert.equal(manualChromeCommand({id:1,op:'prepare',owner:1}).op,'prepare');
  assert.equal(manualChromeCommand(parseStrictJSON('{"op":"fixture_bootstrap","withholdReady":false}')).op,'fixture_bootstrap');
});
test('blocked fixture acquisition reports a closed failure before owner deadline',async()=>{
  await assert.rejects(boundedFixtureAcquire(new Promise(()=>{}),10),/fixture_manual_rejected/);
  assert.equal(await boundedFixtureAcquire(Promise.resolve('synthetic'),10),'synthetic');
  let release;
  const pending=new Promise(resolve=>{release=resolve;});
  await assert.rejects(boundedFixtureAcquire(pending,10),/fixture_manual_rejected/);
  const late=Buffer.from('synthetic');release(late);
  await new Promise(resolve=>setImmediate(resolve));
  assert.ok(late.every(value=>value===0));
});
test('native Chrome fixture cannot start without explicit test process opt-in',async()=>{
  const previous=process.env.AKU_NATIVE_MANUAL_CHROME_FIXTURE;
  try{delete process.env.AKU_NATIVE_MANUAL_CHROME_FIXTURE;await assert.rejects(runManualChromeDriver('','',''),/fixture_manual_rejected/);}
  finally{if(previous===undefined)delete process.env.AKU_NATIVE_MANUAL_CHROME_FIXTURE;else process.env.AKU_NATIVE_MANUAL_CHROME_FIXTURE=previous;}
});
