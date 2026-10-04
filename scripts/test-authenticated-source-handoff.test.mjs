import test from 'node:test';
import assert from 'node:assert/strict';
import {parseJourneyArguments,registeredBridgeIdentity} from './test-authenticated-source-handoff.mjs';
import {createHash} from 'node:crypto';
const base=['--artifact',import.meta.filename,'--baseline',import.meta.filename,'--source','linkedin'];
test('authenticated source window requires both scoped approvals before any runtime work',()=>{
  assert.equal(parseJourneyArguments(base).allowStop,false);
  assert.throws(()=>parseJourneyArguments([...base,'--allow-runtime-stop']),{code:'both_runtime_and_foreground_approval_required'});
  assert.throws(()=>parseJourneyArguments([...base,'--allow-source-window']),{code:'both_runtime_and_foreground_approval_required'});
  const approved=parseJourneyArguments([...base,'--allow-runtime-stop','--allow-source-window']);
  assert.equal(approved.allowStop,true);assert.equal(approved.allowSourceWindow,true);
  assert.equal(approved.allowBridgeReload,false);
  assert.throws(()=>parseJourneyArguments([...base,'--allow-bridge-reload']),{code:'bridge_reload_requires_runtime_and_foreground_approval'});
  assert.equal(parseJourneyArguments([...base,'--allow-runtime-stop','--allow-source-window','--allow-bridge-reload']).allowBridgeReload,true);
});
test('registered Bridge must bind its manifest identity to exactly one registered origin',()=>{
  const key=Buffer.from('fixture-public-key').toString('base64');
  const id=createHash('sha256').update(Buffer.from(key,'base64')).digest('hex').slice(0,32)
    .replace(/[0-9a-f]/g,v=>String.fromCharCode(97+parseInt(v,16)));
  const args=['--bridge-extension-path',import.meta.filename,'--bridge-extension-origin',`chrome-extension://${id}/`];
  assert.equal(registeredBridgeIdentity({args},{key}).id,id);
  assert.throws(()=>registeredBridgeIdentity({args:args.slice(0,2).concat(['--bridge-extension-origin','chrome-extension://wrong'])},{key}),{code:'registered_bridge_origin_mismatch'});
  assert.throws(()=>registeredBridgeIdentity({args:[...args,...args]},{key}),{code:'registered_bridge_identity_unavailable'});
});
test('operator scope rejects unsupported sources, relative paths and duplicate flags',()=>{
  for(const args of [base.map(v=>v==='linkedin'?'facebook':v),['--artifact','relative',...base.slice(2)],
    [...base,'--source','instagram'],[...base,'--allow-runtime-stop','--allow-runtime-stop'],[...base,'--unknown']]) {
    assert.throws(()=>parseJourneyArguments(args),{code:'invalid_arguments'});
  }
});
