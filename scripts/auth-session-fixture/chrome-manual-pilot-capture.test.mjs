import test from 'node:test';
import assert from 'node:assert/strict';
import {captureManualFixtureDOM,fixtureDOMSnapshot} from './chrome-manual-pilot-capture.mjs';
import {manualChromeCommand} from './chrome-manual-pilot-driver.mjs';

test('capture wire accepts no source, URL, script, or diagnostic authority',()=>{
  assert.equal(manualChromeCommand({id:1,op:'capture_fixture',owner:1}).op,'capture_fixture');
  for(const key of ['source','url','script','canAuthorize','nativeQualified'])assert.throws(()=>manualChromeCommand({id:1,op:'capture_fixture',owner:1,[key]:'x'}));
});
test('DOM text and post identity pass through the production observation mapper',async()=>{
  const origin='http://127.0.0.1:12345';let closed=false;
  globalThis.document={title:'Synthetic feed',readyState:'complete',querySelectorAll:()=>[{
    getAttribute:()=> '1908123456789012346',querySelector:selector=>selector==='a[data-permalink]'?{getAttribute:()=> 'https://x.com/synthetic/status/1908123456789012346'}:{textContent:selector==='[data-text]'?'Actual DOM fixture text':'Synthetic author'}
  }]};globalThis.innerHeight=700;
  try {
    const snapshot=fixtureDOMSnapshot();
    const browser={role:'collector',page:async()=>({targetId:'feed-target',navigate:async path=>assert.equal(path,'/feed'),evaluate:async expression=>expression==='location.href'?origin+'/feed':snapshot}),send:async(method,args)=>{assert.equal(method,'Target.closeTarget');assert.equal(args.targetId,'feed-target');closed=true;}};
    const got=await captureManualFixtureDOM(browser,origin,{targetId:'proof-target'});
    assert.equal(got.observation.snapshots[0].blocks[0].text,'Actual DOM fixture text');
    assert.equal(got.observation.snapshots[0].blocks[0].platformId,'x:status:1908123456789012346');
    assert.equal(closed,true);
    await assert.rejects(captureManualFixtureDOM(browser,'https://x.com',{targetId:'proof-target'}));
  }finally{delete globalThis.document;delete globalThis.innerHeight;}
});
