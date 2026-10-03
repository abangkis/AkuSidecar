import test from 'node:test';
import assert from 'node:assert/strict';
import { compareReport, comparisonScope, permalinkEvidence } from './authenticated-parity-comparison.mjs';
import { selectTarget, makeCapturePayload, validateBaseline, parseArguments } from './test-authenticated-headless-parity.mjs';
import { sourceDefinition } from '../../AkuBridge/source-catalog.js';

const target = {source:'x',platformId:'x:status:123',permalink:'https://x.com/fixture/status/123',author:'Fixture',text:'Original text'};
const report = blocks => ({baseline:{targets:[target]},captures:[{source:'x',kind:'target',ok:true,result:{snapshots:[{blocks}]}}]});

test('feed-only qualification rejects conflicting scope before runtime access',()=>{
  const args=['--artifact',import.meta.filename,'--baseline',import.meta.filename,'--source','instagram','--feed-only'];
  assert.equal(parseArguments(args).feedOnly,true);
  assert.throws(()=>parseArguments([...args,'--targets-only']),{code:'conflicting_capture_scopes'});
  assert.throws(()=>parseArguments([...args,'--feed-only']),{code:'invalid_arguments'});
});

test('new-source qualification is explicit, native-bound and does not require Facebook',()=>{
  const rows=[{source:'instagram',platformId:'instagram:p:AbC_123',permalink:'https://www.instagram.com/p/AbC_123/',author:'Fixture',text:'Caption'},
    {source:'linkedin',platformId:'linkedin:activity:1234567890',permalink:'https://www.linkedin.com/feed/update/urn:li:activity:1234567890/',author:'Fixture',text:'Body'}];
  const baseline={scope:'new_source_headless_qualification_baseline',targets:rows};
  assert.equal(validateBaseline(baseline),baseline);
  for (const row of rows) assert.equal(selectTarget(baseline,row.source),row);
  assert.throws(()=>validateBaseline({...baseline,targets:[{...rows[0],platformId:'instagram:p:Other'}]}),{code:'baseline_identity_permalink_mismatch'});
  assert.throws(()=>validateBaseline({...baseline,targets:[]}),{code:'invalid_baseline'});
  assert.throws(()=>validateBaseline({targets:rows}),{code:'baseline_requires_two_targets_per_source'});
  const result=compareReport({baseline,captures:rows.map(row=>({source:row.source,ok:true,result:{snapshots:[{blocks:[row]}]}}))});
  assert.ok(result.cases.every(row=>row.status==='native_identity_and_author_match'));
  assert.equal(result.scope,'new_source_browser_targets_vs_live_headless_sequential');
  assert.equal(result.fullParityVerified,false);
});

test('X author normalization requires the same display name and URL-bound handle',()=>{
  const baseline={...target,author:'Fixture @fixture · 5h'};
  const compare=author=>compareReport({...report([{...target,author}]),baseline:{targets:[baseline]}}).cases[0];
  const matched=compare('Fixture @fixture');
  assert.equal(matched.status,'native_identity_and_author_match');
  assert.equal(matched.authorRawEqual,false);
  assert.equal(matched.authorIdentityNormalizedMatch,true);
  for (const author of ['Fixture @other','Other @fixture','Fixture @fixture · promoted']) {
    assert.equal(compare(author).status,'author_binding_mismatch');
  }
});

test('X image format equivalence preserves raw differences and rejects unsupported identities',()=>{
  const compare=(before,after,kind='image')=>compareReport({...report([{...target,media:[{kind,url:after}]}]),
    baseline:{targets:[{...target,media:[{kind,url:before}]}]}}).cases[0];
  const matched=compare('https://pbs.twimg.com/media/asset.png','https://pbs.twimg.com/media/asset?format=png&name=large');
  assert.equal(matched.sameMediaAssetFormatSets,true);
  assert.equal(matched.exactMediaSetsEqual,false);
  assert.equal(matched.sameHostPathMediaSets,false);
  for (const [before,after] of [
    ['https://pbs.twimg.com/media/asset.png','https://pbs.twimg.com/media/other?format=png'],
    ['https://pbs.twimg.com/media/asset.png','https://pbs.twimg.com/media/asset?format=jpg'],
    ['https://pbs.twimg.com/media/asset.png','https://pbs.twimg.com/media/asset.png?format=jpg'],
    ['https://pbs.twimg.com/media/asset?format=gif','https://pbs.twimg.com/media/asset?format=bmp'],
    ['https://pbs.twimg.com/media/asset.png','https://pbs.twimg.com.evil.test/media/asset?format=png'],
  ]) assert.equal(compare(before,after).sameMediaAssetFormatSets,false);
  assert.equal(compare('https://pbs.twimg.com/media/asset.png','https://pbs.twimg.com/media/asset?format=png','video').sameMediaAssetFormatSets,false);
});

const photoReport=()=>{
  const baseline={source:'facebook',platformId:'facebook:post:123',permalink:'https://www.facebook.com/photo/?fbid=123',
    author:'Fixture Page',text:'Caption',media:[{kind:'image',url:'https://cdn.example.test/image.jpg'}]};
  const block={...baseline,platformId:'facebook:post:pfbidABC',permalink:'https://www.facebook.com/fixture/posts/pfbidABC/'};
  return {baseline:{scope:'fresh_browser_feed_acquisition_baseline',targets:[baseline]},captures:[{source:'facebook',ok:true,
    result:{pageUrl:block.permalink,coverage:{photoParentResolution:{status:'verified',photoId:'123',
      parentPlatformId:block.platformId,provenance:'structured_photo_parent_and_matching_native_post'}},snapshots:[{blocks:[block]}]}}]};
};

test('Facebook photo comparison retains separate identities only with verified parent evidence',()=>{
  const result=compareReport(photoReport());
  const row=result.cases[0];
  assert.equal(row.status,'verified_photo_parent_and_author_match');
  assert.equal(row.platformId,'facebook:post:123');
  assert.equal(row.photoId,'123');
  assert.equal(row.parentPlatformId,'facebook:post:pfbidABC');
  assert.equal(row.textEqual,true);
  assert.equal(row.exactMediaSetsEqual,true);
  assert.equal(row.qualityStatus,'unknown');
  assert.equal(result.scope,'fresh_browser_feed_targets_vs_live_headless_sequential');
  assert.equal(result.fullParityVerified,false);
});

test('Facebook photo similarity cannot bypass missing or conflicting native binding',()=>{
  for (const change of [
    r=>delete r.captures[0].result.coverage.photoParentResolution,
    r=>r.captures[0].result.coverage.photoParentResolution.photoId='456',
    r=>r.captures[0].result.coverage.photoParentResolution.provenance='caption_similarity',
    r=>r.captures[0].result.pageUrl='https://www.facebook.com/fixture/posts/pfbidOTHER/',
    r=>r.captures[0].result.snapshots[0].blocks[0].platformId='facebook:post:pfbidOTHER',
    r=>r.captures[0].result.snapshots[0].blocks[0].permalink='https://www.facebook.com/fixture/posts/pfbidOTHER/',
  ]) {
    const report=photoReport();change(report);
    assert.equal(compareReport(report).cases[0].status,'photo_parent_binding_unverified');
  }
  const ambiguous=photoReport();
  const other=structuredClone(ambiguous.captures[0]);
  other.result.pageUrl='https://www.facebook.com/fixture/posts/pfbidOTHER/';
  other.result.coverage.photoParentResolution.parentPlatformId='facebook:post:pfbidOTHER';
  other.result.snapshots[0].blocks[0].platformId='facebook:post:pfbidOTHER';
  other.result.snapshots[0].blocks[0].permalink=other.result.pageUrl;
  ambiguous.captures.push(other);
  assert.equal(compareReport(ambiguous).cases[0].status,'ambiguous_photo_parent_binding');
  const invalid=photoReport();invalid.baseline.targets[0].platformId='facebook:post:456';
  assert.equal(compareReport(invalid).cases[0].status,'baseline_photo_identity_unverified');
});

test('Facebook inferred, observed and unknown permalinks remain distinct even when ID and URL agree',()=>{
  const fb={source:'facebook',platformId:'facebook:post:123',permalink:'https://www.facebook.com/fixture/posts/123/'};
  for (const [source,expected] of [['media_parent_id','inferred_media_parent'],['post_anchor','observed_anchor'],['direct_anchor','observed_anchor'],['unavailable','unknown'],[undefined,'unknown']]) {
    const baseline={...fb,presentation:{permalinkSource:source}};
    assert.equal(permalinkEvidence(baseline),expected);
    const result=compareReport({baseline:{targets:[baseline]},captures:[]});
    assert.equal(result.cases[0].status,'not_observed');
    assert.equal(result.cases[0].baselinePermalinkEvidence,expected);
    assert.equal(result.fullParityVerified,false);
    // Inferred links remain usable as diagnostic cases; they are not silently
    // discarded or relabeled as invalid IDs.
    assert.equal(selectTarget({targets:[baseline]},'facebook'),baseline);
  }
  assert.equal(permalinkEvidence(target),'not_applicable');
});

test('read-only capture uses each source hydration default without increasing total capture time',()=>{
  for (const source of ['x','facebook','instagram','linkedin']) for (const kind of ['feed','target']) {
    const payload=makeCapturePayload(kind,{permalink:'https://fixture.invalid/'},source);
    assert.equal(payload.sourceHydrationTimeoutMs,sourceDefinition(source).hydration.defaultTimeoutMs);
    assert.equal(payload.captureTimeoutMs,45_000);
    assert.equal(payload.sameTabMutationAllowed,false);
    assert.equal(payload.pendingContentPolicy,'detect_only');
  }
  assert.throws(()=>makeCapturePayload('feed',null,'unknown'),{code:'unsupported_capture_source'});
});

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
test('foreground Reel media Recapture retains exact native binding and its narrower scope',()=>{
  const reel={source:'facebook',platformId:'facebook:post:1639349817600268',
    permalink:'https://www.facebook.com/reel/1639349817600268/',author:'Physics Girl',text:'Caption'};
  const baseline={scope:'native_bridge_media_recapture_foreground_baseline',targets:[reel]};
  assert.equal(selectTarget(baseline,'facebook'),reel);
  const result=compareReport({baseline,captures:[{source:'facebook',ok:true,result:{snapshots:[{blocks:[reel]}]}}]});
  assert.equal(result.cases[0].status,'native_identity_and_author_match');
  assert.equal(result.scope,'native_bridge_media_recapture_vs_live_headless_sequential');
  assert.equal(comparisonScope(baseline,true),'native_bridge_media_recapture_vs_live_quiet_sequential');
  assert.equal(result.fullParityVerified,false);
  const forged={...reel,platformId:'facebook:post:999'};
  assert.throws(()=>selectTarget({targets:[forged]},'facebook'),{code:'selected_target_identity_unverified'});
  assert.equal(compareReport({baseline:{...baseline,targets:[forged]},captures:[]}).cases[0].status,'baseline_native_id_url_unverified');
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
