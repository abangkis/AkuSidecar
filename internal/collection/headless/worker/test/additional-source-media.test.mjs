import test from 'node:test';
import assert from 'node:assert/strict';
import {resolveAdditionalSourceMedia,resolveInstagramNativeTarget} from '../additional-source-media.mjs';
import {toObservation} from '../observation.mjs';

const fixtures=[
  {source:'instagram',id:'instagram:p:AbC_123',candidateId:'instagram:post:AbC_123',permalink:'https://www.instagram.com/p/AbC_123/',
    poster:'https://s.cdninstagram.com/post.jpg',playback:'https://s.cdninstagram.com/owned.mp4'},
  {source:'linkedin',id:'linkedin:ugcpost:1234567890',candidateId:'linkedin:ugcpost:1234567890',permalink:'https://www.linkedin.com/feed/update/urn:li:ugcPost:1234567890/',
    poster:'https://media.licdn.com/dms/image/post',playback:'https://dms.licdn.com/playlist/vid/owned/mp4-720p/file'},
];
test('Instagram structured target fallback requires actual route and exact unique shortcode evidence',async()=>{
  const f=fixtures[0];const snapshot={posts:[],authenticatedUiObserved:true};
  const candidate={candidateId:f.candidateId,platformId:f.id,permalink:f.permalink,author:'Fixture',text:'Caption',media:[{kind:'image',url:f.poster}]};
  const resolver={available:true,functionSource:'function(){}',runtimeRevision:'fixture'};
  const run=(candidates,actual=f.permalink)=>resolveInstagramNativeTarget({requestedUrl:f.permalink,snapshot,resolver,deadlineAt:Date.now()+3000,
    page:{evaluate:async expression=>expression==='location.href' ? actual : {runtimeRevision:'fixture',candidates}}});
  const observed=await run([candidate]);
  assert.equal(observed.posts[0].id,f.id);
  const block=toObservation({source:'instagram',requestedUrl:f.permalink,snapshots:[observed],provenance:{},capturedAt:'2026-10-03T00:00:00Z'}).snapshots[0].blocks[0];
  assert.equal(block.captureQuality.mode,'headless_native_target_structured_observation');
  assert.equal(block.mediaRecovery.outcome,'observed_native_target_structured');
  assert.equal((await run([candidate,candidate])).posts.length,0);
  assert.equal((await run([{...candidate,platformId:'instagram:p:Other'}])).posts.length,0);
  assert.equal((await run([candidate],'https://www.instagram.com/')).posts.length,0);
});
test('additional media requires native identity, source-safe playback and unambiguous own candidate',async()=>{
  for(const f of fixtures) {
    const post={id:f.id,permalink:f.permalink,author:'Fixture',text:'Body',media:[],mediaEvidence:{playerIds:['own-player']}};
    const resolver={available:true,functionSource:'function(){}',runtimeRevision:'fixture'};
    const candidate={candidateId:f.candidateId,media:[{kind:'video',url:f.poster,posterUrl:f.poster,playbackUrl:f.playback}]};
    const run=async(candidates,p=post)=>resolveAdditionalSourceMedia({source:f.source,posts:[p],resolver,deadlineAt:Date.now()+5000,
      page:{evaluate:async expression=>{assert.ok(expression.includes(f.candidateId));return {runtimeRevision:'fixture',candidates};}}});
    const observed=(await run([candidate])).posts[0];
    assert.equal(observed.media[0].playbackUrl,f.playback);
    assert.equal(observed.mediaEvidence.additionalStructuredMedia.status,'observed_owned_urls');
    assert.equal(observed.media[0].loaded,false);
    const block=toObservation({source:f.source,requestedUrl:f.permalink,snapshots:[{posts:[observed]}],provenance:{},capturedAt:'2026-10-03T00:00:00Z'}).snapshots[0].blocks[0];
    assert.equal(block.mediaRecovery.outcome,'owned_playback_url_observed');
    assert.equal(block.mediaRecovery.status,'partial');
    assert.equal(block.captureQuality.status,'unverified');
    assert.equal((await run([candidate,candidate])).posts[0].media.length,0);
    assert.equal((await run([{...candidate,candidateId:'foreign'}])).posts[0].media.length,0);
    assert.equal((await run([{...candidate,media:[{...candidate.media[0],playbackUrl:'https://evil.test/owned.mp4'}]}])).posts[0].media.length,0);
    assert.deepEqual((await run([candidate],{...post,id:'wrong'})).posts[0],{...post,id:'wrong'});
    const unavailable=await resolveAdditionalSourceMedia({source:f.source,posts:[post],deadlineAt:Date.now()+1000,page:{evaluate:()=>{throw Error('must not evaluate');}}});
    assert.equal(unavailable.posts[0].mediaEvidence.additionalStructuredMedia.status,'unavailable');
    if(f.source==='instagram') {
      const unavailableObservation=toObservation({source:f.source,requestedUrl:f.permalink,
        snapshots:[{posts:unavailable.posts,structuredMediaResolution:unavailable.summary}],provenance:{},capturedAt:'2026-10-03T00:00:00Z'});
      const unavailableCoverage=unavailableObservation.coverage.structuredMediaResolution.snapshots[0];
      assert.equal(unavailableCoverage.additionalMediaSkippedPostCount,1);
      assert.equal(unavailableCoverage.additionalMediaRetryAttemptCount,0);
      assert.equal(unavailableCoverage.additionalMediaTraversedNodeCount,null);
      assert.equal(unavailableCoverage.bounded,null);
    }
  }
});

test('Instagram retries one exact video no-match and carries bounded diagnostics into the observation',async()=>{
  const f=fixtures[0];
  const post={id:f.id,permalink:f.permalink,author:'Fixture',text:'Video post',contentKind:'video',media:[],mediaExpected:['video'],
    mediaEvidence:{expectedWithoutUrl:['video']}};
  const candidate={candidateId:f.candidateId,media:[{kind:'video',url:f.poster,posterUrl:f.poster,playbackUrl:f.playback}]};
  const resolver={available:true,functionSource:'function mediaReader(){}',runtimeRevision:'instagram-fixture-v1'};
  const resolverCalls=[];let waitCalls=0;
  const page={evaluate:async(expression,timeout)=>{
    assert.ok(timeout<=1200);
    if(expression.includes('setTimeout')) {waitCalls++;return null;}
    resolverCalls.push(expression);
    if(resolverCalls.length===1) return {runtimeRevision:resolver.runtimeRevision,candidates:[],diagnostics:{
      documentScriptCount:48,mediaScriptCount:48,inspectedScriptCount:48,parsedScriptCount:48,rejectedScriptCount:0,
      inspectedBytes:524288,traversedNodeCount:6000,matchedMediaObjectCount:0,candidateCount:0,bounded:true}};
    return {runtimeRevision:resolver.runtimeRevision,candidates:[candidate],diagnostics:{
      documentScriptCount:3,mediaScriptCount:2,inspectedScriptCount:2,parsedScriptCount:2,rejectedScriptCount:0,
      inspectedBytes:2_000_000,traversedNodeCount:87,matchedMediaObjectCount:1,candidateCount:1,bounded:false}};
  }};
  const result=await resolveAdditionalSourceMedia({page,source:'instagram',posts:[post],resolver,deadlineAt:Date.now()+5000});
  const observed=result.posts[0];
  assert.equal(resolverCalls.length,2);
  assert.equal(waitCalls,1);
  assert.match(resolverCalls[1],/"maxTotalBytes":2000000/);
  assert.match(resolverCalls[1],/"maxTraversalNodes":20000/);
  assert.equal(observed.media[0].playbackUrl,f.playback);
  const diagnostic=observed.mediaEvidence.additionalStructuredMedia;
  assert.equal(diagnostic.status,'observed_owned_urls');
  assert.equal(diagnostic.outcome,'retry_recovered');
  assert.equal(diagnostic.attemptCount,2);
  assert.equal(diagnostic.retryCount,1);
  assert.equal(diagnostic.retryOutcome,'recovered');
  assert.equal(diagnostic.attempts[0].resolverDiagnostics.bounded,true);
  assert.equal(diagnostic.attempts[1].resolverDiagnostics.traversedNodeCount,87);
  assert.equal(JSON.stringify(diagnostic).includes(f.poster),false);
  assert.equal(JSON.stringify(diagnostic).includes(f.playback),false);

  const observation=toObservation({source:'instagram',requestedUrl:f.permalink,
    snapshots:[{posts:result.posts,structuredMediaResolution:result.summary}],provenance:{},capturedAt:'2026-10-08T00:00:00Z'});
  const coverage=observation.coverage.structuredMediaResolution.snapshots[0];
  assert.equal(coverage.additionalMediaAttemptedPostCount,1);
  assert.equal(coverage.additionalMediaRetryAttemptCount,1);
  assert.equal(coverage.additionalMediaRetryRecoveredCount,1);
  assert.equal(coverage.additionalMediaResolverBoundedAttemptCount,1);
  assert.equal(coverage.additionalMediaTraversedNodeCount,6087);
  assert.equal(coverage.additionalMediaInspectedBytes,2_524_288);
  assert.equal(JSON.stringify(coverage).includes(f.poster),false);
  assert.equal(JSON.stringify(coverage).includes(f.playback),false);
});

test('Instagram coverage keeps a known bound hit when a later retry omits diagnostics',async()=>{
  const f=fixtures[0];
  const post={id:f.id,permalink:f.permalink,author:'Fixture',text:'Video post',contentKind:'video',media:[],mediaExpected:['video']};
  const resolver={available:true,functionSource:'function mediaReader(){}',runtimeRevision:'instagram-fixture-v1'};
  let resolverCalls=0;
  const result=await resolveAdditionalSourceMedia({source:'instagram',posts:[post],resolver,deadlineAt:Date.now()+3000,
    page:{evaluate:async(expression)=>{
      if(expression.includes('setTimeout')) return null;
      resolverCalls++;
      return resolverCalls===1
        ? {runtimeRevision:resolver.runtimeRevision,candidates:[],diagnostics:{candidateCount:0,traversedNodeCount:10,bounded:true}}
        : {runtimeRevision:resolver.runtimeRevision,candidates:[]};
    }}});
  const coverage=toObservation({source:'instagram',requestedUrl:f.permalink,
    snapshots:[{posts:result.posts,structuredMediaResolution:result.summary}],provenance:{},capturedAt:'2026-10-08T00:00:00Z'})
    .coverage.structuredMediaResolution.snapshots[0];
  assert.equal(resolverCalls,2);
  assert.equal(coverage.bounded,true);
  assert.equal(coverage.resolverBounded,true);
  assert.equal(coverage.additionalMediaResolverBoundedAttemptCount,null);
  assert.equal(coverage.additionalMediaTraversedNodeCount,null);
});

test('Instagram retry is limited by candidate cap and deadline',async()=>{
  const resolver={available:true,functionSource:'function mediaReader(){}',runtimeRevision:'instagram-fixture-v1'};
  const posts=Array.from({length:6},(_,index)=>({id:`instagram:p:Video${index}`,
    permalink:`https://www.instagram.com/p/Video${index}/`,author:'Fixture',text:'Video post',contentKind:'video',media:[],
    mediaExpected:['video'],mediaEvidence:{expectedWithoutUrl:['video']}}));
  let calls=0,waits=0;
  const boundedPage={evaluate:async(expression)=>{
    if(expression.includes('setTimeout')) {waits++;return null;}
    calls++;
    return {runtimeRevision:resolver.runtimeRevision,candidates:[],diagnostics:{candidateCount:0,traversedNodeCount:20,bounded:false}};
  }};
  const bounded=await resolveAdditionalSourceMedia({page:boundedPage,source:'instagram',posts,resolver,deadlineAt:Date.now()+5000});
  assert.equal(calls,10);
  assert.equal(waits,4);
  assert.equal(bounded.summary.additionalMediaRetryCandidateCount,6);
  assert.equal(bounded.summary.additionalMediaRetryAttemptCount,4);
  assert.equal(bounded.summary.additionalMediaRetryNoMatchCount,4);
  assert.equal(bounded.summary.additionalMediaRetrySkippedCandidateCapCount,2);
  assert.deepEqual(bounded.posts.slice(4).map(post=>post.mediaEvidence.additionalStructuredMedia.retryOutcome),['candidate_cap','candidate_cap']);

  let deadlineCalls=0,deadlineWaits=0;
  const deadlinePage={evaluate:async(expression)=>{
    if(expression.includes('setTimeout')) {deadlineWaits++;return null;}
    deadlineCalls++;
    await new Promise(resolve=>setTimeout(resolve,35));
    return {runtimeRevision:resolver.runtimeRevision,candidates:[],diagnostics:{candidateCount:0,bounded:false}};
  }};
  const deadlinePost={...posts[0]};
  const deadline=await resolveAdditionalSourceMedia({page:deadlinePage,source:'instagram',posts:[deadlinePost],resolver,deadlineAt:Date.now()+20});
  assert.equal(deadlineCalls,1);
  assert.equal(deadlineWaits,0);
  assert.equal(deadline.summary.additionalMediaRetryAttemptCount,0);
  assert.equal(deadline.summary.additionalMediaRetrySkippedDeadlineCount,1);
  assert.equal(deadline.posts[0].mediaEvidence.additionalStructuredMedia.retryOutcome,'deadline');
});

test('Instagram skips retries for images, foreign candidates, ambiguous candidates and unsafe media',async()=>{
  const f=fixtures[0];
  const resolver={available:true,functionSource:'function mediaReader(){}',runtimeRevision:'instagram-fixture-v1'};
  const exact={candidateId:f.candidateId,media:[{kind:'video',url:f.poster,posterUrl:f.poster,playbackUrl:f.playback}]};
  const cases=[
    {name:'image-only',post:{id:f.id,permalink:f.permalink,author:'Fixture',text:'Image',media:[],mediaExpected:['image'],mediaEvidence:{expectedWithoutUrl:['image']}},candidates:[]},
    {name:'foreign',post:{id:f.id,permalink:f.permalink,author:'Fixture',text:'Video',contentKind:'video',media:[],mediaExpected:['video']},candidates:[{...exact,candidateId:'instagram:post:Foreign'}]},
    {name:'ambiguous',post:{id:f.id,permalink:f.permalink,author:'Fixture',text:'Video',contentKind:'video',media:[],mediaExpected:['video']},candidates:[exact,exact]},
    {name:'unsafe-url',post:{id:f.id,permalink:f.permalink,author:'Fixture',text:'Video',contentKind:'video',media:[],mediaExpected:['video']},
      candidates:[{...exact,media:[{...exact.media[0],playbackUrl:'https://attacker.example/video.mp4'}]}]},
    {name:'resolver-error',post:{id:f.id,permalink:f.permalink,author:'Fixture',text:'Video',contentKind:'video',media:[],mediaExpected:['video']},candidates:[],throws:true},
  ];
  for(const item of cases) {
    let resolverCalls=0,waitCalls=0;
    const result=await resolveAdditionalSourceMedia({source:'instagram',posts:[item.post],resolver,deadlineAt:Date.now()+3000,
      page:{evaluate:async(expression)=>{
        if(expression.includes('setTimeout')) {waitCalls++;return null;}
        resolverCalls++;
        if(item.throws) throw new Error('resolver failed with https://private.invalid/payload');
        return {runtimeRevision:resolver.runtimeRevision,candidates:item.candidates,diagnostics:{candidateCount:item.candidates.length,bounded:false}};
      }}});
    assert.equal(resolverCalls,1,`${item.name} should use one resolver call`);
    assert.equal(waitCalls,0,`${item.name} should not wait for hydration`);
    assert.equal(result.summary.additionalMediaRetryAttemptCount,0,`${item.name} should not retry`);
    if(item.throws) {
      assert.equal(result.summary.additionalMediaErrorPostCount,1);
      assert.equal(result.summary.status,'unavailable');
      assert.equal(result.posts[0].mediaEvidence.additionalStructuredMedia.outcome,'error');
      assert.equal(JSON.stringify(result.posts[0].mediaEvidence.additionalStructuredMedia).includes('private.invalid'),false);
    }
  }
});

test('Instagram caption enrichment requires same native author and a compatible visible prefix',async()=>{
  const f=fixtures[0];const post={id:f.id,permalink:f.permalink,author:'Fixture',text:'Visible caption...',media:[]};
  const candidate={platformId:f.id,permalink:f.permalink,author:'Fixture',text:'Visible caption with more detail.'};
  const run=async candidates=>resolveAdditionalSourceMedia({source:'instagram',posts:[post],deadlineAt:Date.now()+3000,
    feedResolver:{available:true,functionSource:'function captionReader(){}',runtimeRevision:'caption-fixture'},
    page:{evaluate:async()=>({runtimeRevision:'caption-fixture',candidates})}});
  assert.equal((await run([candidate])).posts[0].text,candidate.text);
  for(const candidates of [[{...candidate,author:'Another'}],[{...candidate,text:'Different story'}],[candidate,candidate],
    [{...candidate,platformId:'instagram:p:Foreign'}]]) assert.equal((await run(candidates)).posts[0].text,post.text);
});
