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
