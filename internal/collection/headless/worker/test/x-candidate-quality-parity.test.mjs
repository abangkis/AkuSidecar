import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile,writeFile,mkdir,mkdtemp} from 'node:fs/promises';
import {fileURLToPath,pathToFileURL} from 'node:url';
import {join} from 'node:path';
import {spawnSync} from 'node:child_process';
import {sourceAssets} from '../worker.mjs';
import {toObservation} from '../observation.mjs';
import {xCandidateQualityPolicyFor} from '../capture-primitives.mjs';
import {captureIsolated,NATIVE_CAPTURE_CAPABILITIES} from '../capture.mjs';

const bridge=new URL('../../../../../../AkuBridge/',import.meta.url);
const assets=await sourceAssets(fileURLToPath(bridge),'x');
const context=vm.createContext({URL});
for(const name of ['capture-primitives.js','source-adapter-runtime.js','adapters/x-adapter.js','capture-quality-policy.js'])
  vm.runInContext(await readFile(new URL(name,bridge),'utf8'),context,{filename:name});
const source=await readFile(new URL('content-script.js',bridge),'utf8');
const evaluate=source.match(/^  function evaluateBlockQuality\([\s\S]*?^  }/m)?.[0];
const extractBlock=source.match(/^  function extractBlock\([\s\S]*?^  }/m)?.[0];
assert.ok(evaluate);
assert.ok(extractBlock);
const adapter=context.AkuSourceAdapters.get('x');
context.sourceAdapters=context.AkuSourceAdapters;
context.qualityPolicy=context.AkuCaptureQualityPolicy;
context.compactText=value=>String(value??'').replace(/\s+/g,' ').trim();
vm.runInContext(evaluate,context);
const contract=xCandidateQualityPolicyFor(assets);
const id='1234500000000000022';
const base={id,permalink:`https://x.com/fixture/status/${id}`,author:'Fixture',
  text:'An authored source post with sufficient text and stable native identity.',avatar:'',media:[],
  mediaExpected:[],publishedAt:null,limitations:['visible_dom_only'],textCompleteness:'no_collapse_observed'};
const project=post=>toObservation({source:'x',requestedUrl:'https://x.com/home',snapshots:[{posts:[post]}],
  capturedAt:'2026-11-15T00:00:00Z',xCandidateQualityPolicy:contract});
const plain=value=>JSON.parse(JSON.stringify(value));

// This fixture supplies normalized facts; the actual DOM/owned-media observer
// is exercised separately by the opt-in real-DOM fixture.
function observedContainer({content=true,media=false,avatar=false,timestamp=false}={}) {
  const node={};
  return {querySelector:()=>null,querySelectorAll(selector){
    if(selector===adapter.qualitySelectors.content)return content?[node]:[];
    if(selector===adapter.qualitySelectors.author)return [node];
    if(selector===adapter.qualitySelectors.avatar)return avatar?[node]:[];
    if(selector===adapter.qualitySelectors.timestamp)return timestamp?[node]:[];
    if(selector===adapter.qualitySelectors.media)return media?[node]:[];
    return [];
  }};
}

test('actual Windowed quality call and final Headless projection agree at equal terminal budget',async t=>{
  const cases=[
    {name:'ready text',changes:{},expected:'complete'},
    {name:'media only',changes:{text:'',media:[{kind:'image',url:'https://pbs.twimg.com/media/fixture.jpg'}]},media:true,expected:'complete'},
    {name:'missing owned image',changes:{mediaExpected:['image']},media:true,expected:'usable_degraded'},
    {name:'unresolved video',changes:{media:[{kind:'video_poster',url:'https://pbs.twimg.com/ext_tw_video_thumb/123/pu/img/poster.jpg'}],
      mediaExpected:['video'],limitations:['video_stream_not_resolved']},media:true,expected:'complete'},
    {name:'collapsed text',changes:{textStatus:'requires_permalink_capture',textCompleteness:'partial',textCollapsed:true,
      limitations:['text_may_be_collapsed']},expected:'complete'},
    {name:'exhausted empty evidence',changes:{text:'',mediaExpected:['image']},media:true,expected:'invalid'},
  ];
  for(const spec of cases)await t.test(spec.name,()=>{
    const post={...base,...spec.changes};
    const container=observedContainer(spec);
    const candidate={...post,platformId:'x:status:'+id,avatarUrl:post.avatar,
      media:post.media.map(item=>({...item,kind:item.kind==='video_poster'?'video':item.kind})),presentation:{}};
    const mediaEvidence={signal:spec.media?'owned':'absent',expectedKinds:post.mediaExpected};
    const facts=context.qualityPolicy.observeCandidateFacts({container,adapter,candidate,mediaEvidence});
    post.qualityFacts={schema:'aku.x.candidate-quality-facts.v1',...plain(facts)};
    const windowed=context.evaluateBlockQuality(container,'x',candidate,null,0,0,mediaEvidence);
    const observation=project(post);
    const block=observation.snapshots[0].blocks[0];
    const report=block.captureQuality.assessment;
    assert.equal(windowed.verdict,spec.expected);
    for(const field of ['profile','contentFamily','evidenceModalities','identitySource','candidateKey','verdict','score','attempt','issues'])
      assert.deepEqual(report[field],plain(windowed[field]),spec.name+' '+field);
    assert.equal(report.textCompleteness,post.textCompleteness);
    assert.equal(block.captureQuality.status,'unverified');
    assert.equal(Object.hasOwn(block.captureQuality,'verdict'),false);
    assert.equal(Object.hasOwn(observation.coverage.captureQuality??{},'verdict'),false);
    assert.equal(observation.snapshots[0].qualityReports[0].status,'unverified');
    assert.equal(report.retriesRemaining,0);
    if(spec.name==='collapsed text')assert.ok(block.captureQuality.limitations.includes('text_may_be_collapsed'));
    if(spec.name==='unresolved video'){
      assert.equal(block.mediaRecovery.unknownVideo,'unresolved');
      assert.ok(block.captureQuality.limitations.includes('video_stream_not_resolved'));
      assert.equal(typeof adapter.mediaAcquisition.isComplete,'undefined');
    }
    if(spec.name==='exhausted empty evidence')assert.equal(context.evaluateBlockQuality(container,'x',candidate,null,0,1,mediaEvidence).verdict,'retryable');
  });
});

test('final recovered text/media are assessed again and supplied assessments cannot override policy',()=>{
  const facts={schema:'aku.x.candidate-quality-facts.v1',contentRootDetected:true,authorRootDetected:true,
    mediaRootDetected:true,mediaEvidence:true,mediaPlaybackMissing:true,stableTextIdentity:false,verdict:'invalid',retriesRemaining:99};
  const post={...base,qualityFacts:facts,media:[{kind:'image',url:'https://pbs.twimg.com/media/recovered.jpg'}],
    textCompleteness:'recovery_verified',textStatus:'permalink_text_verified',
    captureQuality:{assessment:{verdict:'invalid'}},candidateQuality:{verdict:'invalid'}};
  const report=project(post).snapshots[0].blocks[0].captureQuality.assessment;
  assert.equal(report.verdict,'complete');
  assert.equal(report.textCompleteness,'recovery_verified');
  assert.deepEqual(report.evidenceModalities,['text','image']);
  assert.equal(report.candidateKey,null);
  assert.equal(report.retriesRemaining,0);
  const oldShape=toObservation({source:'x',snapshots:[{posts:[post]}],capturedAt:'2026-11-15T00:00:00Z'});
  assert.equal(Object.hasOwn(oldShape.snapshots[0].blocks[0].captureQuality,'assessment'),false);
  const unknownFacts={schema:'aku.x.candidate-quality-facts.v1',contentRootDetected:'true',mediaRootDetected:1};
  Object.defineProperty(unknownFacts,'authorRootDetected',{get(){throw Error('getter must not execute');}});
  assert.deepEqual(Object.values(plain(context.qualityPolicy.normalizeFacts(unknownFacts))),Array(9).fill(false));
});

test('actual asset loading pins the policy hash and fails closed for missing shared contract',()=>{
  const asset=assets.find(value=>value.relative==='AkuBridge/capture-quality-policy.js');
  assert.equal(asset.execute,true);
  assert.match(asset.sha256,/^[0-9a-f]{64}$/);
  assert.equal(xCandidateQualityPolicyFor(assets),contract);
  assert.throws(()=>xCandidateQualityPolicyFor(assets.filter(value=>value!==asset)),/candidate quality asset is required/);
  assert.throws(()=>xCandidateQualityPolicyFor(assets.map(value=>value===asset?{...value,content:'globalThis.AkuCaptureQualityPolicy={evaluateCandidate(){}};'}:value)),/contract is unavailable/);
  assert.equal(project({...base,qualityFacts:{schema:'unexpected',verdict:'complete'}}).snapshots[0].blocks[0].captureQuality.assessment,undefined);
});

function qualityBrowserFixture(spec) {
  const location=new URL('https://x.com/home'),window={location};
  for(const code of spec.assets)new Function('window','location',code)(window,location);
  const adapter=globalThis.AkuSourceAdapters.get('x');
  const policy=globalThis.AkuCaptureQualityPolicy;
  const compactText=value=>String(value?.innerText??value??'').replace(/\s+/g,' ').trim();
  const evaluate=new Function('sourceAdapters','qualityPolicy',spec.evaluate+'\nreturn evaluateBlockQuality;')(
    globalThis.AkuSourceAdapters,policy);
  const extractBlock=new Function('sourceAdapters','structuredText','compactText',spec.extractBlock+'\nreturn extractBlock;')(
    globalThis.AkuSourceAdapters,globalThis.AkuCapturePrimitives.structuredText,compactText);
  const main=document.querySelector('main');
  const wrap=(body,extra='')=>`<div role="link" data-href="/fixture/status/${spec.id}"><article class="group/post">
    <div class="x-middot-group"><a href="https://x.com/fixture">Fixture</a></div>${body}${extra}</article></div>`;
  const text=body=>`<div dir="auto" class="whitespace-pre-wrap">${body}</div>`;
  const ownPhoto='<div data-testid="tweetPhoto"><img width="240" height="135" src="https://pbs.twimg.com/media/fixture.jpg"></div>';
  const emptyPhoto='<div data-testid="tweetPhoto"></div>';
  const quote=`<div role="link" data-href="/quote/status/1234500000000000023"><article class="group/post">
    <div class="x-middot-group"><a href="https://x.com/quote">Quoted author</a></div>${text('Quoted caption')}${ownPhoto}</article></div>`;
  const cases=[
    ['ready',wrap(text('Ready authored evidence with a stable native identity.'))],
    ['media_only',wrap(ownPhoto)],
    ['missing_owned_media',wrap(text('Ready authored evidence with a stable native identity.'),emptyPhoto)],
    ['unresolved_video',wrap(text('Ready authored evidence with a stable native identity.'),
      '<div data-testid="videoPlayer"><video poster="https://pbs.twimg.com/ext_tw_video_thumb/123/pu/img/poster.jpg"></video></div>')],
    ['collapsed',wrap(text('Ready authored evidence with a stable native identity.<button type="button">Show more</button>'))],
    ['empty_evidence',wrap('',emptyPhoto)],
    ['quote_only_media',wrap(text('Ready authored evidence with a stable native identity.'),quote)],
    ['foreign_photo_root',wrap(text('Ready authored evidence with a stable native identity.'),
      '<a href="https://x.com/other/status/1234500000000000024/photo/1">'+emptyPhoto+'</a>')],
    ['fallback_geometry',wrap(text('Ready authored evidence with a stable native identity.'),
      '<img width="240" height="135" src="https://example.test/unowned.png">')],
    ['ambiguous',wrap(text('Ready authored evidence with a stable native identity.'),
      '<a href="https://x.com/other/status/1234500000000000024"><time datetime="2026-11-15T00:00:00Z">Now</time></a>')],
  ];
  return (async()=>{
    const rows=[];
    for(const [name,html]of cases){
      main.innerHTML=html;
      globalThis.AkuHeadlessCapturePolicy={allowContentExpansion:false};
      const article=main.querySelector('article');
      const identity=adapter.resolveIdentity(article);
      const snapshot=await globalThis.XHeadlessPoC.collect();
      if(identity.status!=='identified'){
        rows.push({name,identity:identity.status,snapshot,windowedRejected:extractBlock(article,'x',4000,null)===null});continue;
      }
      const candidate={text:adapter.extractText(article,{compactText,structuredText:globalThis.AkuCapturePrimitives.structuredText}),
        author:adapter.findAuthor(article,{compactText}),platformId:'x:status:'+spec.id,permalink:identity.permalink,
        avatarUrl:'',media:globalThis.AkuBoundedCapturePolicy.extractXDOMMedia(article,adapter,{excludeRoot:adapter.findQuotedRoot(article)}).media,
        publishedAt:null,contentKind:'post',presentation:{},
        quotedPost:adapter.extractQuotedPost(article,{compactText,structuredText:globalThis.AkuCapturePrimitives.structuredText,
          normalizeHttpUrl:value=>value,capturedAt:'2026-11-15T00:00:00Z',
          findMedia:root=>globalThis.AkuBoundedCapturePolicy.extractXDOMMedia(root,adapter).media})};
      const mediaEvidence=globalThis.AkuMediaAcquisitionEngine.observe({source:'x',container:article,candidateId:candidate.platformId,
        excludeRoot:adapter.findQuotedRoot(article),initialMedia:candidate.media});
      const facts=policy.observeCandidateFacts({container:article,adapter,candidate,mediaEvidence});
      rows.push({name,snapshot,facts,signal:mediaEvidence.signal,
        nullFacts:policy.observeCandidateFacts({container:article,adapter,candidate,mediaEvidence:null}),
        geometryFallbackFacts:policy.observeCandidateFacts({container:article,
          adapter:{...adapter,qualitySelectors:{...adapter.qualitySelectors,media:null},imageSelector:'img'},candidate,mediaEvidence:null}),
        windowed:evaluate(article,'x',candidate,null,0,0,mediaEvidence),
        retryable:evaluate(article,'x',candidate,null,0,1,mediaEvidence)});
    }
    return rows;
  })();
}

test('actual X DOM observer preserves ownership and matches actual Windowed media observation',
  {skip:!process.env.AKU_OFFLINE_CHROME&&'Set AKU_OFFLINE_CHROME for isolated real-DOM quality parity.'},async()=>{
    const workspace=fileURLToPath(new URL('../../../../../../',import.meta.url));
    const root=join(workspace,'AkuSidecar','build','a4-x-quality-20261115');
    await mkdir(root,{recursive:true});
    const scratch=await mkdtemp(join(root,'quality-dom-'));
    const codes=assets.filter(value=>value.execute).map(value=>value.content);
    for(const name of ['media-post-processor.js','media-acquisition-engine.js'])codes.push(await readFile(new URL(name,bridge),'utf8'));
    const script=`(${qualityBrowserFixture.toString()})(${JSON.stringify({assets:codes,evaluate,extractBlock,id})}).then(rows=>{
      document.getElementById('result').textContent=JSON.stringify({rows});
    }).catch(error=>document.getElementById('result').textContent=JSON.stringify({error:String(error.stack||error)}));`;
    const fixture=join(scratch,'fixture.html');
    await writeFile(fixture,'<!doctype html><meta charset="utf-8"><meta http-equiv="Content-Security-Policy" content="default-src \'none\'; script-src \'unsafe-inline\' \'unsafe-eval\'; style-src \'unsafe-inline\'">'+
      '<base href="https://x.com/home"><style>article{min-height:32px}</style><main></main><pre id="result"></pre><script>'+script.replaceAll('</script','<\\/script')+'</script>');
    const run=spawnSync(process.env.AKU_OFFLINE_CHROME,['--headless','--dump-dom','--virtual-time-budget=15000',
      '--disable-background-networking','--disable-component-update','--no-first-run','--no-default-browser-check',
      '--host-resolver-rules=MAP * ~NOTFOUND','--window-size=1200,1000','--user-data-dir='+join(scratch,'profile'),pathToFileURL(fixture).href],
      {encoding:'utf8',timeout:30000,maxBuffer:8*1024*1024,windowsHide:true});
    assert.equal(run.status,0,String(run.error??run.stderr).slice(-1200));
    const encoded=run.stdout.match(/<pre id="result">([\s\S]*?)<\/pre>/)?.[1];
    assert.ok(encoded,run.stdout.slice(-1500));
    const result=JSON.parse(encoded.replaceAll('&lt;','<').replaceAll('&gt;','>').replaceAll('&amp;','&'));
    assert.ok(!result.error,result.error);
    await writeFile(join(scratch,'results.json'),JSON.stringify(result,null,2));
    for(const row of result.rows){
      if(row.name==='ambiguous'){
        assert.equal(row.identity,'conflicting');assert.equal(row.snapshot.posts.length,0);assert.equal(row.windowedRejected,true);continue;
      }
      const post=row.snapshot.posts[0];
      assert.ok(post,row.name);
      const {schema,...facts}=post.qualityFacts;
      assert.equal(schema,'aku.x.candidate-quality-facts.v1');
      assert.deepEqual(facts,row.facts,row.name+' normalized facts');
      const block=project(post).snapshots[0].blocks[0];
      const report=block.captureQuality.assessment;
      for(const field of ['verdict','score','issues','evidenceModalities'])assert.deepEqual(report[field],row.windowed[field],row.name+' '+field);
      if(['foreign_photo_root','quote_only_media','fallback_geometry'].includes(row.name))assert.equal(row.facts.mediaRootDetected,false,row.name);
      if(row.name==='foreign_photo_root')assert.equal(row.signal,'ambiguous');
      if(row.name==='fallback_geometry'){
        // Actual X's imageSelector excludes arbitrary foreign images, even in
        // its null-evidence fallback. A test-only narrowed selector exercises
        // the preserved generic geometry branch separately.
        assert.equal(row.nullFacts.mediaRootDetected,false);
        assert.equal(row.geometryFallbackFacts.mediaRootDetected,true);
      }
      if(row.name==='collapsed')assert.equal(block.captureQuality.textCompleteness,'partial');
      if(row.name==='unresolved_video')assert.equal(block.mediaRecovery.unknownVideo,'unresolved');
      if(row.name==='empty_evidence'){
        assert.equal(report.verdict,'invalid');assert.equal(row.retryable.verdict,'retryable');
      }
    }
    const collapsed=result.rows.find(row=>row.name==='collapsed').snapshot;
    let temporaryPages=0,quoteClicks=0,mutationGrants=0;
    const page={navigate:async()=>({}),send:async()=>({}),evaluate:async expression=>{
      if(expression.startsWith('globalThis.XHeadlessPoC.clickQuote(')){quoteClicks++;throw Error('native quote action attempted');}
      if(expression.startsWith('globalThis.AkuHeadlessCapturePolicy=')&&expression.includes('allowContentExpansion:true'))mutationGrants++;
      if(expression.includes('XHeadlessPoC.collect()'))return JSON.stringify(collapsed);
      if(expression==='location.href')return 'https://x.com/home';
      return undefined;
    }};
    const native=await captureIsolated({backend:'headless_worker',captureCapabilities:NATIVE_CAPTURE_CAPABILITIES.x,
      forSource:async()=>page,createTemporaryPage:async()=>{temporaryPages++;throw Error('native detail page attempted');}},
      {x:assets},'x',{scrolls:0,captureTimeoutMs:3000,sourceHydrationTimeoutMs:1000,
        sameTabMutationAllowed:true,pendingContentPolicy:'reveal_if_present',sourceFreshnessPolicy:'wake_and_reveal'});
    const nativeQuality=native.snapshots[0].blocks[0].captureQuality;
    assert.equal(nativeQuality.status,'unverified');
    assert.equal(nativeQuality.textCompleteness,'partial');
    assert.equal(nativeQuality.assessment.verdict,'complete');
    assert.ok(nativeQuality.limitations.includes('permalink_capture_unavailable'));
    assert.deepEqual({temporaryPages,quoteClicks,mutationGrants},{temporaryPages:0,quoteClicks:0,mutationGrants:0});
    assert.ok(native.coverage.provenance.sources.some(value=>value.path==='AkuBridge/capture-quality-policy.js'));
    assert.equal(Object.hasOwn(native.coverage.captureQuality??{},'verdict'),false);
  });
