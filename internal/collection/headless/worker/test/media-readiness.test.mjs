import test from 'node:test';
import assert from 'node:assert/strict';
import { capture } from '../capture.mjs';

const linkedinPermalink='https://www.linkedin.com/feed/update/urn:li:activity:1234567890/';

function readinessFixture(source, postForSample) {
  let samples=0;
  const permalink=source==='linkedin' ? linkedinPermalink : source==='instagram'
    ? 'https://www.instagram.com/reel/Fixture123/' : 'https://x.com/fixture/status/123';
  const id=source==='linkedin' ? 'linkedin:activity:1234567890' : source==='instagram'
    ? 'instagram:reel:Fixture123' : '123';
  const page={async navigate(){return {};},async send(){return {};},async evaluate(expression){
    if(expression.includes('prepareEvidenceTargets'))return [];
    if(expression.includes('XHeadlessPoC.collect()')){
      samples++;
      return JSON.stringify({documentReady:true,scroll:{y:0,viewportHeight:900},posts:[{
        id,permalink,author:'Fixture',text:'Media readiness',...postForSample(samples),
      }]});
    }
    if(expression==='location.href')return permalink;
    return undefined;
  }};
  const assets={[source]:['runtime','adapter','extractor'].map((relative,index)=>({relative,execute:false,sha256:String(index).repeat(64)}))};
  return {page,assets,permalink,get samples(){return samples;}};
}

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

test('LinkedIn and Instagram keep a poster-only video pending until playback appears',async()=>{
  for(const source of ['linkedin','instagram']){
    const host=source==='linkedin' ? 'https://media.licdn.com' : 'https://scontent.cdninstagram.com';
    const poster={kind:'video',url:`${host}/dms/image/poster.jpg`,
      posterUrl:`${host}/dms/image/poster.jpg`,playbackUrl:null};
    const playback={...poster,playbackUrl:`${host}/dms/video/clip.mp4`};
    const fixture=readinessFixture(source,samples=>({mediaExpected:['video'],media:[samples===1?poster:playback],
      mediaEvidence:{expectedWithoutUrl:[]}}));
    const result=await capture({async forSource(){return fixture.page;}},fixture.assets,source,{
      pageUrl:fixture.permalink,scrolls:0,sourceHydrationTimeoutMs:1500,captureTimeoutMs:3000,
    });
    assert.equal(fixture.samples,2,source);
    assert.equal(result.snapshots[0].blocks[0].media[0].playbackUrl,playback.playbackUrl,source);
  }
});

test('a persistent LinkedIn poster-only video remains bounded by the 3 second settle window',async()=>{
  const poster={kind:'video',url:'https://media.licdn.com/dms/image/poster.jpg',
    posterUrl:'https://media.licdn.com/dms/image/poster.jpg',playbackUrl:null};
  const fixture=readinessFixture('linkedin',()=>({mediaExpected:['video'],media:[poster],
    mediaEvidence:{expectedWithoutUrl:[]}}));
  const startedAt=Date.now();
  const result=await capture({async forSource(){return fixture.page;}},fixture.assets,'linkedin',{
    pageUrl:fixture.permalink,scrolls:0,sourceHydrationTimeoutMs:8000,captureTimeoutMs:10000,
  });
  const elapsed=Date.now()-startedAt;
  assert.ok(fixture.samples>1);
  assert.ok(fixture.samples<=12,`expected the media settle cap to stop sampling, got ${fixture.samples}`);
  assert.ok(elapsed<4500,`expected bounded settle, capture took ${elapsed}ms`);
  assert.equal(result.snapshots[0].blocks[0].media[0].playbackUrl,null);
});

test('resolved video and image-only LinkedIn posts skip extra media settling',async()=>{
  const cases=[
    {name:'direct video',post:{mediaExpected:['video'],media:[{kind:'video',url:'https://media.licdn.com/dms/video/clip.mp4',
      playbackUrl:'https://media.licdn.com/dms/video/clip.mp4'}],mediaEvidence:{expectedWithoutUrl:[]}}},
    {name:'image only',post:{mediaExpected:['image'],media:[{kind:'image',url:'https://media.licdn.com/dms/image/photo.jpg'}],
      mediaEvidence:{expectedWithoutUrl:[]}}},
  ];
  for(const item of cases){
    const fixture=readinessFixture('linkedin',()=>item.post);
    await capture({async forSource(){return fixture.page;}},fixture.assets,'linkedin',{
      pageUrl:fixture.permalink,scrolls:0,sourceHydrationTimeoutMs:1500,captureTimeoutMs:3000,
    });
    assert.equal(fixture.samples,1,item.name);
  }
});

test('X preserves poster-only readiness behavior',async()=>{
  const poster={kind:'video_poster',url:'https://pbs.twimg.com/fixture.jpg',loaded:true};
  const fixture=readinessFixture('x',()=>({mediaExpected:['video'],media:[poster],
    mediaEvidence:{expectedWithoutUrl:[]}}));
  await capture({async forSource(){return fixture.page;}},fixture.assets,'x',{
    pageUrl:fixture.permalink,scrolls:0,sourceHydrationTimeoutMs:1500,captureTimeoutMs:3000,
  });
  assert.equal(fixture.samples,1);
});
