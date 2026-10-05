import test from 'node:test';
import assert from 'node:assert/strict';
import {canonicalSourceURL,toObservation} from '../observation.mjs';
import {validateCapture,capture} from '../capture.mjs';
import {sourceAssets} from '../worker.mjs';
import {fileURLToPath} from 'node:url';
import vm from 'node:vm';

const fixtures=[
  {source:'instagram',id:'instagram:p:AbC_123',url:'https://www.instagram.com/p/AbC_123/',home:'https://www.instagram.com/'},
  {source:'linkedin',id:'linkedin:activity:1234567890',url:'https://www.linkedin.com/feed/update/urn:li:activity:1234567890/',home:'https://www.linkedin.com/feed/'},
];
test('additional sources load their own Bridge adapter and media runtime with hashed provenance',async()=>{
  const bridge=fileURLToPath(new URL('../../../../../../AkuBridge/',import.meta.url));
  for (const {source} of fixtures) {
    const assets=await sourceAssets(bridge,source);
    assert.deepEqual(assets.filter(a=>a.execute).map(a=>a.relative),[
      'AkuBridge/capture-primitives.js','AkuBridge/source-adapter-runtime.js','AkuBridge/bounded-capture-policy.js','AkuBridge/media-post-processor.js',
      ...(source==='linkedin' ? ['AkuBridge/linkedin-permalink-policy.js','AkuBridge/linkedin-timestamp-policy.js'] : []),
      `AkuBridge/adapters/${source}-adapter.js`,'worker/vendor/adapter-extract.js']);
    assert.ok(assets.every(a=>/^[a-f0-9]{64}$/.test(a.sha256)));
    assert.equal(assets.structuredMediaResolver.available,true);
    assert.equal(typeof assets.structuredMediaResolver.functionSource,'string');
    const context=vm.createContext({URL,console,setTimeout,clearTimeout,location:new URL(fixtures.find(f=>f.source===source).home),
      document:{body:{innerText:''},querySelector:()=>null,querySelectorAll:()=>[]},innerHeight:800,innerWidth:1200,scrollY:0});
    for (const asset of assets.filter(a=>a.execute)) vm.runInContext(asset.content,context,{filename:asset.relative});
    assert.equal(typeof context.XHeadlessPoC.collect,'function');
    const snapshot=await context.XHeadlessPoC.collect();
    assert.equal(snapshot.sourceUnavailable,false);
    assert.equal(snapshot.posts.length,0);
  }
  await assert.rejects(sourceAssets(bridge,'unknown'),{code:'unsupported_source'});
});
test('new source targets require native URL/identity agreement and retain owned attachments',()=>{
  for (const f of fixtures) {
    assert.equal(validateCapture(f.source,{}).pageUrl,f.home);
    assert.equal(canonicalSourceURL(f.source,f.url+'?tracking=1'),f.url);
    const post={id:f.id,permalink:f.url,author:'Fixture',text:'Body',attachments:[{kind:'link_preview',title:'Story',url:'https://example.test/story'}],links:[{url:'https://example.test/story'}]};
    const convert=p=>toObservation({source:f.source,requestedUrl:f.url,snapshots:[{posts:[p]}],provenance:{},capturedAt:'2026-10-03T00:00:00Z'});
    const block=convert(post).snapshots[0].blocks[0];
    assert.equal(block.platformId,f.id);
    assert.deepEqual(block.attachments,post.attachments);
    assert.equal(block.captureQuality.status,'unverified');
    assert.throws(()=>convert({...post,id:f.id+'wrong'}),{code:'invalid_observation'});
    for (const url of [f.url.replace('https:','http:'),f.url.replace('www.','www.evil.'),f.home,'https://evil.test/p/AbC_123/']) {
      assert.equal(canonicalSourceURL(f.source,url),null);
      assert.throws(()=>validateCapture(f.source,{pageUrl:url}),{code:'invalid_page_url'});
    }
  }
  assert.equal(canonicalSourceURL('linkedin','https://www.linkedin.com/posts/fixture-topic-activity-1234567890-AbCd?tracking=1'),fixtures[1].url);
  assert.equal(canonicalSourceURL('instagram','https://instagram.com/reel/CaseSensitive_1/?igsh=x'),'https://www.instagram.com/reel/CaseSensitive_1/');
  assert.equal(canonicalSourceURL('linkedin','https://www.linkedin.com/posts/no-native-identity/'),null);
});

test('each added source flows through capture without Facebook media resolver assumptions',async()=>{
  for (const f of fixtures) {
    let navigated;
    const page={navigate:async url=>{navigated=url;return {};},send:async()=>({}),evaluate:async expression=>{
      if (expression==='location.href') return navigated;
      if (expression.includes('prepareEvidenceTargets')) return [];
      if (expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({posts:[{id:f.id,permalink:f.url,author:'Fixture',text:'Body',media:[]}],documentReady:true,authenticatedUiObserved:true,scroll:{y:0,viewportHeight:900}});
    }};
    const result=await capture({forSource:async source=>{assert.equal(source,f.source);return page;}},
      {[f.source]:[{execute:false},{execute:false},{execute:false}]},f.source,{scrolls:0,sourceHydrationTimeoutMs:1000});
    assert.equal(navigated,f.home);
    assert.equal(result.source,f.source);
    assert.equal(result.snapshots[0].blocks[0].platformId,f.id);
  }
});
