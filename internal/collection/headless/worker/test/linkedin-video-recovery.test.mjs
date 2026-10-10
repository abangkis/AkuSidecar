import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {resolveAdditionalSourceMedia} from '../additional-source-media.mjs';
import {toObservation} from '../observation.mjs';

const readerSource=await readFile(new URL('../../../../../../AkuBridge/linkedin-main-world-media-resolver.js',import.meta.url),'utf8');
const reader=vm.runInNewContext(readerSource.replace('export function','function')+';resolveLinkedInStructuredMediaInMainWorld');
const id='linkedin:ugcpost:7514519298036576256';
const native='https://www.linkedin.com/feed/update/urn:li:ugcPost:7514519298036576256/';
const poster='https://dms.licdn.com/playlist/vid/v2/fixture/thumbnail-low/0/frame?token=PRIVATE';
const playback='https://dms.licdn.com/playlist/vid/v2/fixture/mp4-720p-30fp/0/video?token=PRIVATE';
const resolver={available:true,runtimeRevision:'linkedin-main-world-media-resolver-v1',functionSource:reader.toString()};
const post=()=>({id,permalink:native,author:'Fixture',text:'Fixture native post',contentKind:'video',
  media:[{kind:'video',url:poster,posterUrl:poster,playbackUrl:null,playbackMode:'native'}],
  mediaExpected:['video'],mediaEvidence:{expectedWithoutUrl:[],playerIds:['fixture-player']}});
const candidate={candidateId:id,media:[{kind:'video',url:poster,posterUrl:poster,playbackUrl:playback}]};

test('LinkedIn uses the shared real reader again after hydration and persists counters without signed URLs',async()=>{
  let hydrated=false,calls=0,waits=0;
  const container={getAttribute:key=>key==='data-urn'?'urn:li:ugcPost:7514519298036576256':null,querySelectorAll:()=>[]};
  const video={videoWidth:550,videoHeight:309,getBoundingClientRect:()=>({width:550,height:309})};
  const root={id:'fixture-player',matches:()=>true,closest:()=>container,querySelector:()=>video};
  const document={querySelectorAll:selector=>selector==='[data-vjs-player]'?[root]:[]};
  const player={source:playback,poster};
  const vjsForDebug={getPlayer:()=>hydrated?player:null};
  const page={evaluate:async(expression)=>{
    if(expression.includes('setTimeout')) {waits++;hydrated=true;return null;}
    calls++;
    return vm.runInNewContext(expression,{document,vjsForDebug,URL});
  }};
  const result=await resolveAdditionalSourceMedia({page,source:'linkedin',posts:[post()],resolver,deadlineAt:Date.now()+5000});
  assert.equal(calls,2);assert.equal(waits,1);
  assert.equal(result.posts[0].media[0].playbackUrl,playback);
  const diagnostic=result.posts[0].mediaEvidence.additionalStructuredMedia;
  assert.equal(diagnostic.outcome,'retry_recovered');
  assert.equal(diagnostic.attempts[0].resolverDiagnostics.playerRootCount,1);
  assert.equal(diagnostic.attempts[0].resolverDiagnostics.resolvedPlayerCount,0);
  assert.equal(diagnostic.attempts[1].resolverDiagnostics.resolvedPlayerCount,1);
  const observation=toObservation({source:'linkedin',requestedUrl:native,
    snapshots:[{posts:result.posts,structuredMediaResolution:result.summary}],provenance:{},capturedAt:'2026-10-10T00:00:00Z'});
  const coverage=observation.coverage.structuredMediaResolution.snapshots[0];
  // The shared reader reports hitting its one-candidate ceiling even when
  // that exact candidate was successfully recovered.
  assert.equal(coverage.status,'bounded');
  assert.equal(coverage.resolverBounded,true);
  assert.equal(coverage.additionalMediaRetryRecoveredCount,1);
  assert.equal(coverage.additionalMediaPlayerRootCount,2);
  assert.equal(coverage.additionalMediaResolvedPlayerCount,1);
  assert.equal(coverage.additionalMediaDirectPlaybackURLCount,1);
  for(const safe of [coverage,diagnostic]) {
    assert.equal(JSON.stringify(safe).includes('PRIVATE'),false);
    assert.equal(JSON.stringify(safe).includes('fixture-player'),false);
  }
  assert.equal(observation.snapshots[0].blocks[0].mediaRecovery.outcome,'owned_playback_url_observed');
  assert.ok(result.posts[0].limitations.includes('video_playback_unverified'));
});

test('dimensioned LinkedIn poster survives the real shared reader, headless guard and observation',async()=>{
  const dimensionedPoster=poster.replace('thumbnail-low','thumbnail-shrink_720_1280');
  const container={getAttribute:key=>key==='data-urn'?'urn:li:ugcPost:7514519298036576256':null,querySelectorAll:()=>[]};
  const root={id:'fixture-player',matches:()=>true,closest:()=>container,querySelector:()=>null};
  const document={querySelectorAll:selector=>selector==='[data-vjs-player]'?[root]:[]};
  const player={source:playback,poster:dimensionedPoster,
    noise:Array.from({length:100},()=>Array.from({length:100},()=>({unrelated:true})))};
  const vjsForDebug={getPlayer:()=>player};
  const p=post();p.media[0]={...p.media[0],url:dimensionedPoster,posterUrl:dimensionedPoster};
  p.mediaEvidence.expectedWithoutUrl=['video'];
  let calls=0;
  const result=await resolveAdditionalSourceMedia({source:'linkedin',posts:[p],resolver,deadlineAt:Date.now()+5000,
    page:{evaluate:async expression=>{calls++;return vm.runInNewContext(expression,{document,vjsForDebug,URL});}}});
  assert.equal(calls,1);
  assert.equal(result.posts[0].media.length,1);
  assert.equal(result.posts[0].media[0].posterUrl,dimensionedPoster);
  assert.equal(result.posts[0].media[0].playbackUrl,playback);
  assert.deepEqual(result.posts[0].mediaEvidence.expectedWithoutUrl,[]);
  const diagnostic=result.posts[0].mediaEvidence.additionalStructuredMedia;
  assert.equal(diagnostic.status,'observed_owned_urls');
  assert.equal(diagnostic.retryCount,0);
  assert.equal(diagnostic.attempts[0].resolverDiagnostics.traversedNodeCount,3000);
  assert.equal(JSON.stringify(diagnostic).includes('PRIVATE'),false);
  const observation=toObservation({source:'linkedin',requestedUrl:native,
    snapshots:[{posts:result.posts,structuredMediaResolution:result.summary}],provenance:{},capturedAt:'2026-10-10T00:00:00Z'});
  assert.equal(observation.snapshots[0].blocks[0].mediaRecovery.outcome,'owned_playback_url_observed');
  assert.ok(result.posts[0].limitations.includes('video_playback_unverified'));
});

test('headless LinkedIn poster guard independently rejects unsafe dimensioned candidates',async()=>{
  const path='/playlist/vid/v2/fixture/thumbnail-shrink_720_1280/0/frame';
  for(const unsafe of [
    `https://evil.test${path}`,`https://dms.licdn.com.evil.test${path}`,
    `http://dms.licdn.com${path}`,`https://user@dms.licdn.com${path}`,`https://dms.licdn.com:444${path}`,
    'https://dms.licdn.com/other/thumbnail-shrink_720_1280/0/frame',playback,
    'https://dms.licdn.com/playlist/vid/v2/fixture/thumbnail-shrink_720_1280_extra/0/frame',
  ]) {
    const result=await resolveAdditionalSourceMedia({source:'linkedin',posts:[post()],resolver,deadlineAt:Date.now()+5000,
      page:{evaluate:async()=>({runtimeRevision:resolver.runtimeRevision,
        candidates:[{candidateId:id,media:[{kind:'video',url:unsafe,posterUrl:unsafe,playbackUrl:playback}]}],diagnostics:{}})}});
    assert.equal(result.posts[0].media[0].playbackUrl,null,unsafe);
    assert.equal(result.posts[0].mediaEvidence.additionalStructuredMedia.status,'no_safe_media',unsafe);
    assert.equal(result.summary.additionalMediaRetryAttemptCount,0);
  }
});

test('LinkedIn retry stays within candidate cap and rejects foreign, ambiguous or unsafe results',async()=>{
  const cases=[
    {candidates:[{...candidate,candidateId:'linkedin:ugcpost:9999999999'}]},
    {candidates:[candidate,candidate]},
    {candidates:[{...candidate,media:[{...candidate.media[0],playbackUrl:'https://evil.test/video.mp4'}]}]},
    {throws:true,candidates:[]},
    {candidates:[],image:true},
    {candidates:[],alreadyPlayable:true},
    {candidates:[],invalidRevision:true},
  ];
  for(const item of cases) {
    let calls=0,waits=0;
    const p=post();
    if(item.image) Object.assign(p,{contentKind:'post',media:[],mediaExpected:['image']});
    if(item.alreadyPlayable) p.media[0].playbackUrl=playback;
    const result=await resolveAdditionalSourceMedia({source:'linkedin',posts:[p],resolver,deadlineAt:Date.now()+5000,
      page:{evaluate:async expression=>{
        if(expression.includes('setTimeout')) {waits++;return null;}
        calls++;if(item.throws) throw Error('PRIVATE signed URL');
        return {runtimeRevision:item.invalidRevision?'wrong':resolver.runtimeRevision,candidates:item.candidates,
          diagnostics:{playerRootCount:1,privateURL:playback,bounded:false}};
      }}});
    assert.equal(calls,1);assert.equal(waits,0);
    assert.equal(result.summary.additionalMediaRetryAttemptCount,0);
    assert.equal(JSON.stringify(result.posts[0].mediaEvidence.additionalStructuredMedia).includes('PRIVATE'),false);
  }
  let calls=0,waits=0;
  const posts=Array.from({length:6},(_,i)=>({...post(),id:`linkedin:ugcpost:751451929803657625${i}`,
    permalink:`https://www.linkedin.com/feed/update/urn:li:ugcPost:751451929803657625${i}/`}));
  const capped=await resolveAdditionalSourceMedia({source:'linkedin',posts,resolver,deadlineAt:Date.now()+5000,
    page:{evaluate:async expression=>{
      if(expression.includes('setTimeout')) {waits++;return null;}
      calls++;return {runtimeRevision:resolver.runtimeRevision,candidates:[],diagnostics:{playerRootCount:0,bounded:false}};
    }}});
  assert.equal(calls,10);assert.equal(waits,4);
  assert.equal(capped.summary.additionalMediaRetrySkippedCandidateCapCount,2);
});

test('LinkedIn deadline and unknown diagnostic values stay distinct from zero',async()=>{
  let calls=0;
  const result=await resolveAdditionalSourceMedia({source:'linkedin',posts:[post()],resolver,deadlineAt:Date.now()+80,
    page:{evaluate:async()=>{
      calls++;return {runtimeRevision:resolver.runtimeRevision,candidates:[],diagnostics:{playerRootCount:0,
        resolvedPlayerCount:-1,directPlaybackURLCount:'0',bounded:false}};
    }}});
  assert.equal(calls,1);
  assert.equal(result.summary.additionalMediaRetrySkippedDeadlineCount,1);
  const coverage=toObservation({source:'linkedin',requestedUrl:native,snapshots:[{posts:result.posts,structuredMediaResolution:result.summary}],
    provenance:{},capturedAt:'2026-10-10T00:00:00Z'}).coverage.structuredMediaResolution.snapshots[0];
  assert.equal(coverage.additionalMediaPlayerRootCount,0);
  assert.equal(coverage.additionalMediaResolvedPlayerCount,null);
  assert.equal(coverage.additionalMediaDirectPlaybackURLCount,null);
  assert.equal(coverage.resolverBounded,false);
});
