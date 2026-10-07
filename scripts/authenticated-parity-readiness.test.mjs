import test from 'node:test';
import assert from 'node:assert/strict';
import {validateCollectionReadiness} from './test-authenticated-headless-parity.mjs';
const headless = {available:true,state:'ready',pending:false,activeLeases:0,effective:'headless',headlessAvailable:true,authorizedSources:['x']};
const offline = {compatible:false,authorizedSources:[]};
test('idle authorized headless accepts an absent browser heartbeat',()=>{
  assert.equal(validateCollectionReadiness(headless,offline,['x']).effective,'headless');
});
test('headless rejects missing permissions, unknown state, active leases and reader ownership',()=>{
  assert.throws(()=>validateCollectionReadiness(headless,offline,['facebook']),/source_access_unconfirmed/);
  for(const change of [{available:false},{state:'unknown'},{pending:true},{activeLeases:1},{activeLeases:undefined},{nativeReaderOnly:true},{headlessAvailable:false},{effective:'unknown'}]){
    assert.throws(()=>validateCollectionReadiness({...headless,...change},offline,['x']));
  }
});
test('browser retains heartbeat and source-permission checks',()=>{
  const browser={...headless,effective:'browser'};
  assert.throws(()=>validateCollectionReadiness(browser,offline),/bridge_incompatible/);
  assert.throws(()=>validateCollectionReadiness(browser,{compatible:true,authorizedSources:[]},['x']),/source_access_unconfirmed/);
  assert.equal(validateCollectionReadiness(browser,{compatible:true,authorizedSources:['x']},['x']).effective,'browser');
});
test('restore cannot accept a different effective backend',()=>{
  assert.throws(()=>validateCollectionReadiness(headless,offline,[],'browser'),/collection_runtime_not_idle_ready/);
});
