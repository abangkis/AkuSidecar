import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {createInstagramVideoRecovery,resolveAdditionalSourceMedia} from '../additional-source-media.mjs';
import {capture} from '../capture.mjs';
import {resolveInstagramStructuredMediaInMainWorld} from '../../../../../../AkuBridge/instagram-main-world-media-resolver.js';
import {resolveInstagramStructuredFeedInMainWorld} from '../../../../../../AkuBridge/instagram-main-world-feed-resolver.js';

const resolver={available:true,functionSource:resolveInstagramStructuredMediaInMainWorld.toString(),runtimeRevision:'instagram-main-world-media-resolver-v2'};
const feedResolver={available:true,functionSource:resolveInstagramStructuredFeedInMainWorld.toString(),runtimeRevision:'instagram-main-world-feed-resolver-v1'};
const urlFor=code=>`https://www.instagram.com/p/${code}/`;
const image=index=>`https://s.cdninstagram.com/slide${index}.jpg`;
const playback='https://s.cdninstagram.com/slide2.mp4';
function feedPost(code='Fixture') {
  return {id:`instagram:p:${code}`,permalink:urlFor(code),author:'fixture.author',text:'',contentKind:'video',
    media:[{kind:'image',url:image(1)},{kind:'image',url:image(2)}],mediaExpected:['image','video'],
    mediaEvidence:{expectedWithoutUrl:['video'],videoStreamStatus:'unknown'},limitations:['video_stream_not_resolved']};
}
function nativeObject(code='Fixture') {
  return {code,user:{username:'fixture.author'},caption:{text:'髪のメンテナンスしてきたよ〜✂︎'},media_type:8,
    image_versions2:{candidates:[{url:image(1),width:1000,height:1000}]},carousel_media:[1,2,3,4].map(index=>({
      code:`${code}${index}`,image_versions2:{candidates:[{url:image(index),width:1000,height:1000}]},
      ...(index===2 ? {media_type:2,video_versions:[{url:playback,width:1000,height:1000}]} : {media_type:1}),
    }))};
}
function harness({objects=[nativeObject()],actualUrl,flags={},delaySetup=0,closeError=false}={}) {
  const state={url:'about:blank',navigations:[],created:0,closed:0,policies:[]};
  const page={
    async navigate(url) {state.url=actualUrl || url;state.navigations.push(url);return {};},
    async evaluate(expression) {
      if(expression==='location.href') return state.url;
      if(expression==='({url:location.href,ready:document.readyState})') return {url:state.url,ready:'complete'};
      if(expression.includes('AkuHeadlessCapturePolicy')) {state.policies.push(expression);return;}
      const script={textContent:JSON.stringify({require:[{data:objects}]})};
      return vm.runInNewContext(expression,{document:{querySelectorAll:()=>[script]},URL,TextEncoder});
    },
    async close() {state.closed++;if(closeError) throw new Error('target remains');},
  };
  const browser={backend:'headless_worker',async createTemporaryPage() {
    state.created++;if(delaySetup) await new Promise(resolve=>setTimeout(resolve,delaySetup));return page;
  }};
  const inspectSnapshot=async()=>({posts:[],...flags});
  const recovery=(options={})=>createInstagramVideoRecovery({browser,resolver,feedResolver,inspectSnapshot,deadlineAt:Date.now()+5000,...options});
  return {state,page,browser,inspectSnapshot,recovery};
}

test('native fallback reuses both real resolvers, binds identity and recovers mixed carousel/caption without duplicate poster',async()=>{
  const h=harness(),recovery=h.recovery();
  try {
    const result=await recovery.recoverSnapshot({posts:[feedPost()],scroll:{y:520}});
    assert.equal(result.posts[0].text,'髪のメンテナンスしてきたよ〜✂︎');
    assert.deepEqual(result.posts[0].media.map(m=>m.kind),['image','video','image','image']);
    assert.equal(result.posts[0].media[1].playbackUrl,playback);
    assert.equal(result.posts[0].media[1].loaded,false);
    assert.equal(result.posts[0].mediaEvidence.nativeVideoFallback.playbackVerified,false);
    assert.equal(result.posts[0].mediaEvidence.additionalStructuredMedia.status,'observed_owned_urls');
    assert.deepEqual(result.posts[0].mediaEvidence.expectedWithoutUrl,[]);
    assert.deepEqual(result.posts[0].limitations,['video_playback_unverified']);
    assert.deepEqual(result.scroll,{y:520});
    const cached=await recovery.recoverSnapshot({posts:[feedPost()]});
    assert.equal(cached.structuredMediaResolution.nativeVideoFallbackAttemptCount,0);
    assert.equal(cached.posts[0].media.length,4);
    assert.deepEqual(h.state.navigations,[urlFor('Fixture')]);
    assert.ok(h.state.policies.every(value=>value.includes('allowContentExpansion:false')));
  } finally {await recovery.close();}
  assert.equal(h.state.closed,1);
});

test('rejects redirect, foreign shortcode/author/poster and challenged native target',async()=>{
  const cases=[
    {options:{actualUrl:urlFor('Other')},outcome:'identity_mismatch'},
    {options:{objects:[nativeObject('Other')]},outcome:'unresolved'},
    {options:{objects:[{...nativeObject(),user:{username:'foreign.author'}}]},outcome:'identity_mismatch'},
    {options:{flags:{challengeDetected:true}},outcome:'source_unavailable'},
    {options:{},post:{...feedPost(),media:[{kind:'image',url:image(99)}]},outcome:'poster_mismatch'},
  ];
  for(const item of cases) {
    const h=harness(item.options),recovery=h.recovery();
    try {
      const original=item.post || feedPost();
      const result=(await recovery.recoverSnapshot({posts:[original]})).posts[0];
      assert.deepEqual(result.media,original.media);
      assert.equal(result.text,original.text);
      assert.equal(result.mediaEvidence.nativeVideoFallback.outcome,item.outcome);
      assert.equal(result.limitations.includes('video_stream_not_resolved'),true);
    } finally {await recovery.close();}
    assert.equal(h.state.closed,1);
  }
});

test('skips resolved/image-only/invalid identities, borrowed browser, target cap and exhausted capture deadline',async()=>{
  const h=harness(),recovery=h.recovery();
  const complete={...feedPost('Resolved'),media:[{kind:'video',url:image(2),playbackUrl:playback}]};
  const still={...feedPost('Image'),contentKind:'post',mediaExpected:['image'],mediaEvidence:{},media:[{kind:'image',url:image(1)}]};
  try {
    const result=await recovery.recoverSnapshot({posts:[complete,still,{...feedPost(),id:'instagram:p:Wrong'}]});
    assert.deepEqual(result.posts.slice(0,2),[complete,still]);
    assert.equal(h.state.created,0);
  } finally {await recovery.close();}
  const bounded=harness({objects:[]}),limited=bounded.recovery({maxTargets:1});
  try {
    const result=await limited.recoverSnapshot({posts:[feedPost('One'),feedPost('Two')]});
    assert.equal(result.structuredMediaResolution.nativeVideoFallbackAttemptCount,1);
    assert.equal(result.posts[1].mediaEvidence.nativeVideoFallback.outcome,'candidate_cap');
    assert.equal(bounded.state.navigations.length,1);
  } finally {await limited.close();}
  for(const options of [{deadlineAt:Date.now()+300},{browser:{...h.browser,backend:'browser_quiet_hidden'}}]) {
    const denied=h.recovery(options);
    const result=await denied.recoverSnapshot({posts:[feedPost()]});
    assert.equal(result.structuredMediaResolution.nativeVideoFallbackAttemptCount,0);
    await denied.close();
  }
  assert.equal(h.state.created,0);
});

test('cleans a late-created target after timeout and propagates cleanup failures',async()=>{
  const h=harness({delaySetup:40}),recovery=h.recovery({perTargetMs:10});
  const result=await recovery.recoverSnapshot({posts:[feedPost()]});
  assert.equal(result.posts[0].mediaEvidence.nativeVideoFallback.outcome,'deadline');
  await recovery.close();await recovery.close();
  assert.equal(h.state.closed,1);
  const failure=harness({closeError:true}),failed=failure.recovery();
  await failed.recoverSnapshot({posts:[feedPost()]});
  await assert.rejects(failed.close(),{code:'temporary_target_cleanup_failed'});
});

test('same-post structured caption can fill empty DOM caption while rejecting a foreign author',async()=>{
  const h=harness();await h.page.navigate(urlFor('Fixture'));
  const result=await resolveAdditionalSourceMedia({page:h.page,source:'instagram',posts:[feedPost()],feedResolver,deadlineAt:Date.now()+2000});
  assert.equal(result.posts[0].text,'髪のメンテナンスしてきたよ〜✂︎');
  const foreign=await resolveAdditionalSourceMedia({page:h.page,source:'instagram',posts:[{...feedPost(),author:'other'}],feedResolver,deadlineAt:Date.now()+2000});
  assert.equal(foreign.posts[0].text,'');
});

test('capture pipeline preserves feed frontier and emits native recovery diagnostics with playback still unverified',async()=>{
  const h=harness(),feedState={url:'about:blank',navigations:[],y:0};
  const feedPage={
    async navigate(url) {feedState.url=url;feedState.navigations.push(url);return {};},
    async evaluate(expression) {
      if(expression==='location.href') return feedState.url;
      if(expression.includes('prepareEvidenceTargets')) return [];
      if(expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({posts:[feedPost()],documentReady:true,
        scroll:{y:0,height:1800,viewportHeight:900}});
      if(expression.includes('function resolveInstagramStructured')) return {runtimeRevision:expression.includes('FeedInMainWorld')?feedResolver.runtimeRevision:resolver.runtimeRevision,candidates:[]};
      if(expression.includes('setTimeout')) return null;
      return undefined;
    },
  };
  const originalEvaluate=h.page.evaluate;
  h.page.evaluate=async expression=>{
    if(expression.includes('prepareEvidenceTargets')) return [];
    if(expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({posts:[],documentReady:true});
    return originalEvaluate(expression);
  };
  const assets=Array.from({length:3},(_,index)=>({relative:`asset${index}`,sha256:String(index).repeat(64),execute:false}));
  assets.structuredMediaResolver=resolver;assets.structuredFeedResolver=feedResolver;
  const observation=await capture({...h.browser,forSource:async()=>feedPage},{instagram:assets},'instagram',{
    scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:5000,
  });
  const block=observation.snapshots[0].blocks[0];
  assert.equal(block.mediaRecovery.outcome,'owned_playback_url_observed');
  assert.equal(block.mediaRecovery.status,'partial');
  assert.equal(block.mediaRecovery.unknownVideo,'playback_unverified');
  assert.equal(block.media.length,4);
  assert.equal(observation.coverage.structuredMediaResolution.snapshots[0].nativeVideoFallbackAttemptCount,1);
  assert.equal(observation.coverage.structuredMediaResolution.snapshots[0].nativeVideoFallbackRecoveredCount,1);
  assert.deepEqual(feedState.navigations,['https://www.instagram.com/']);
  assert.equal(feedState.url,'https://www.instagram.com/');
  assert.equal(observation.coverage.frontier.scrollY,0);
  assert.deepEqual(observation.coverage.frontier.anchorKeys,['instagram:p:Fixture']);
  assert.equal(h.state.closed,1);
});
