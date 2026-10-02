import test from 'node:test';
import assert from 'node:assert/strict';
import { compareReport, comparisonScope } from './authenticated-parity-comparison.mjs';
import { selectTarget } from './test-authenticated-headless-parity.mjs';

const target = {source:'x',platformId:'x:status:123',permalink:'https://x.com/fixture/status/123',author:'Fixture',text:'Original text'};
const report = blocks => ({baseline:{targets:[target]},captures:[{source:'x',kind:'target',ok:true,result:{snapshots:[{blocks}]}}]});

test('keeps the Quiet driver scope distinct from a headless runtime',()=>{
  const result=compareReport({...report([target]),baseline:{targets:[target],scope:'saved_browser_timeline_vs_live_headless_sequential'},execution:{scope:'saved_timeline_vs_live_quiet_sequential'}});
  assert.equal(result.scope,'saved_timeline_vs_live_quiet_sequential');
  assert.equal(result.fullParityVerified,false);
});

test('a prior headless or unverified baseline cannot be labeled legacy Timeline evidence',()=>{
  const prior={scope:'previous_headless_observed_video_target_not_legacy_parity',targets:[target]};
  assert.equal(comparisonScope(prior),'previous_headless_vs_live_headless_sequential');
  assert.equal(compareReport({...report([target]),baseline:prior,execution:{scope:'saved_timeline_vs_live_headless_sequential'}}).scope,
    'previous_headless_vs_live_headless_sequential');
  assert.equal(comparisonScope({}),'unverified_baseline_vs_live_headless_sequential');
});

test('selects the second target explicitly and rejects mismatched native IDs before execution',()=>{
  const second={...target,platformId:'x:status:456',permalink:'https://x.com/fixture/status/456'};
  const fb={source:'facebook',platformId:'facebook:post:123',permalink:'https://www.facebook.com/watch/?v=123'};
  const baseline={targets:[target,second,fb,{...fb,platformId:'facebook:post:456'}]};
  assert.equal(selectTarget(baseline,'x',1),second);
  assert.equal(selectTarget(baseline,'facebook'),fb);
  assert.throws(()=>selectTarget(baseline,'facebook',1),{code:'selected_target_identity_unverified'});
  assert.throws(()=>selectTarget(baseline,'x',2),{code:'invalid_target_selection'});
});
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

test('video parity includes playback identity, not just a matching poster',()=>{
  const media={kind:'video',url:'https://pbs.twimg.com/amplify_video_thumb/1/p.jpg',
    posterUrl:'https://pbs.twimg.com/amplify_video_thumb/1/p.jpg',playbackUrl:'https://video.twimg.com/amplify_video/1/vid.mp4?token=old',playbackMode:'inline'};
  const baseline={...target,media:[media]};
  const compare=playbackUrl=>compareReport({...report([{...target,media:[{...media,playbackUrl}]}]),baseline:{targets:[baseline]}}).cases[0];
  const other=compare('https://video.twimg.com/amplify_video/2/vid.mp4');
  assert.equal(other.exactMediaSetsEqual,false);
  assert.equal(other.sameHostPathMediaSets,false);
  const rotated=compare('https://video.twimg.com/amplify_video/1/vid.mp4?token=new');
  assert.equal(rotated.exactMediaSetsEqual,false);
  assert.equal(rotated.sameHostPathMediaSets,true);
  assert.equal(compare(undefined).exactMediaSetsEqual,false);
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
test('binds Facebook watch video identities across canonical URL variants',()=>{
  const facebookTarget={source:'facebook',platformId:'facebook:post:12345',
    permalink:'https://www.facebook.com/watch/?v=12345',author:'Fixture Page',text:'Video post'};
  const observed={...facebookTarget,permalink:'https://www.facebook.com/video.php?v=12345'};
  const result=compareReport({baseline:{targets:[facebookTarget]},captures:[{source:'facebook',kind:'target',ok:true,
    result:{snapshots:[{blocks:[observed]}]}}]});
  assert.equal(result.cases[0].status,'native_identity_and_author_match');
  const mismatchedBaseline={...facebookTarget,platformId:'facebook:post:54321'};
  const invalid=compareReport({baseline:{targets:[mismatchedBaseline]},captures:[]});
  assert.equal(invalid.cases[0].status,'baseline_native_id_url_unverified');
});
