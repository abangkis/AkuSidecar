import test from 'node:test';
import assert from 'node:assert/strict';
import { compareReport } from './authenticated-parity-comparison.mjs';

const target = {source:'x',platformId:'x:status:123',permalink:'https://x.com/fixture/status/123',author:'Fixture',text:'Original text'};
const report = blocks => ({baseline:{targets:[target]},captures:[{source:'x',kind:'target',ok:true,result:{snapshots:[{blocks}]}}]});
test('rejects same text without explicit native identity and rejects conflicting bindings',()=>{
  assert.equal(compareReport(report([{...target,platformId:'x:status:456'}])).cases[0].status,'not_observed');
  assert.equal(compareReport(report([target,{...target,author:'Other'}])).cases[0].status,'ambiguous_identity_binding');
  assert.equal(compareReport(report([{...target,permalink:'https://x.com/fixture/status/456'}])).cases[0].status,'native_permalink_mismatch');
  const invalid = {...target,permalink:'https://x.com/fixture/status/456'};
  assert.equal(compareReport({...report([invalid]),baseline:{targets:[invalid]}}).cases[0].status,'baseline_native_id_url_unverified');
});
test('separately reports CDN query differences without claiming full parity',()=>{
  const baseline={...target,media:[{kind:'image',url:'https://media.example.test/photo.jpg?token=before'}]};
  const observed={...target,media:[{kind:'image',url:'https://media.example.test/photo.jpg?token=after'}]};
  const result=compareReport({...report([observed]),baseline:{targets:[baseline]}});
  assert.equal(result.cases[0].exactMediaSetsEqual,false);
  assert.equal(result.cases[0].sameHostPathMediaSets,true);
  assert.equal(result.fullParityVerified,false);
});
test('preserves unavailable media and quality separately from observed empty media',()=>{
  const missing = compareReport(report([target])).cases[0];
  assert.equal(missing.exactMediaSetsEqual,null);
  assert.equal(missing.observedMediaCount,null);
  assert.equal(missing.qualityStatus,'unknown');
  const empty = compareReport({...report([{...target,media:[]}]),baseline:{targets:[{...target,media:[]}]}}).cases[0];
  assert.equal(empty.exactMediaSetsEqual,true);
  assert.equal(empty.observedMediaCount,0);
  assert.equal(compareReport(report([target])).fullParityVerified,false);
});
test('URL token replacement isolates prose while retaining an exact text mismatch',()=>{
  const baseline={...target,text:'Status is live https://example.test/story/expanded-display-token'};
  const observed={...target,text:'Status is live https://t.co/short'};
  const result=compareReport({...report([observed]),baseline:{targets:[baseline]}});
  assert.equal(result.cases[0].textEqual,false);
  assert.equal(result.cases[0].proseEqualWithUrlTokensReplaced,true);
  assert.equal(result.fullParityVerified,false);
});
test('changed prose remains unequal after URL tokens are replaced',()=>{
  const baseline={...target,text:'Status is live https://example.test/story/expanded-display-token'};
  const observed={...target,text:'A different status https://t.co/short'};
  const result=compareReport({...report([observed]),baseline:{targets:[baseline]}});
  assert.equal(result.cases[0].textEqual,false);
  assert.equal(result.cases[0].proseEqualWithUrlTokensReplaced,false);
});
test('unchanged text remains equal in both comparisons',()=>{
  const result=compareReport(report([target]));
  assert.equal(result.cases[0].textEqual,true);
  assert.equal(result.cases[0].proseEqualWithUrlTokensReplaced,true);
});
