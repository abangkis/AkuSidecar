import test from 'node:test';
import assert from 'node:assert/strict';
import { capture } from '../capture.mjs';

test('X retains a hydrated attachment instead of accepting the first empty media shell',async()=>{
  let samples=0;
  const page={async navigate(){return {};},async send(){return {};},async evaluate(expression){
    if(expression.includes('prepareEvidenceTargets'))return [];
    if(expression.includes('XHeadlessPoC.collect()')){
      samples++;
      return JSON.stringify({documentReady:true,scroll:{y:0,viewportHeight:900},posts:[{
        id:'123',permalink:'https://x.com/fixture/status/123',author:'Fixture',text:'Hydrating video',
        mediaExpected:['video'],media:samples===1?[]:[{kind:'video_poster',url:'https://pbs.twimg.com/fixture.jpg',loaded:true}],
        mediaEvidence:{status:samples===1?'missing_expected_url':'observed_urls_partial',expectedWithoutUrl:samples===1?['video']:[]},
      }]});
    }
    return undefined;
  }};
  const assets={x:['runtime','adapter','extractor'].map((relative,index)=>({relative,execute:false,sha256:String(index).repeat(64)}))};
  const result=await capture({async forSource(){return page;}},assets,'x',{
    pageUrl:'https://x.com/fixture/status/123',scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:3000});
  assert.equal(samples,2);
  assert.equal(result.snapshots[0].blocks[0].media[0].kind,'video_poster');
  assert.equal(result.snapshots[0].blocks[0].mediaRecovery.unknownVideo,'unresolved');
});

test('invalid native post retains bounded structural diagnostics without copying private evidence',async()=>{
  const page={async navigate(){return {};},async send(){return {};},async evaluate(expression){
    if(expression.includes('prepareEvidenceTargets'))return [];
    if(expression.includes('XHeadlessPoC.collect()'))return JSON.stringify({candidateCount:2,rejected:1,
      authenticatedUiObserved:true,posts:[{id:'facebook:post:123',permalink:'https://www.facebook.com/private-profile',
        author:'Private author',text:'Private body'}]});
    return undefined;
  }};
  const assets={facebook:['runtime','adapter','extractor'].map((relative,index)=>({relative,execute:false,sha256:String(index).repeat(64)}))};
  await assert.rejects(capture({async forSource(){return page;}},assets,'facebook',{
    scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:3000}),error=>{
    assert.equal(error.code,'invalid_observation');
    assert.equal(error.message,'post permalink is outside the native source URL contract');
    assert.equal(error.diagnostics.samples[0].candidateCount,2);
    assert.equal(JSON.stringify(error.diagnostics).includes('Private'),false);
    assert.equal(JSON.stringify(error.diagnostics).includes('private-profile'),false);
    return true;
  });
});
