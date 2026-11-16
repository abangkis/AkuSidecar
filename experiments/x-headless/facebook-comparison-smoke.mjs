import assert from 'node:assert/strict';
import {createServer} from 'node:http';
import {launchChrome} from './chrome.mjs';
import {loadScripts,inject,collect} from './capture.mjs';
import {productionScripts,productionContext,collectProduction,comparePosts} from './facebook-comparison.mjs';

const longText='Expanded paragraph 🙂 '.repeat(230).trim();
const page=`<!doctype html><meta charset="utf-8"><style>[role=article]{height:230px}img{display:inline-block}</style>
<div aria-label="Account"></div><main><div role="feed">
<div role="article" aria-posinset="1"><button aria-label="Actions for this post by Fixture author"></button>
<a href="https://www.facebook.com/fixture/posts/111111/" target="_blank">1h</a>
<div data-ad-preview="message"><span>Short text</span><div role="button" onclick="if(this.textContent==='See more'){this.previousElementSibling.textContent=${JSON.stringify(longText).replace(/"/g,'&quot;')};this.textContent='See less'}else{this.previousElementSibling.textContent='Short text';this.textContent='See more'}">See more</div></div>
<a href="https://www.facebook.com/photo/?fbid=777"><img width="240" height="120" src="https://test.fbcdn.net/one.jpg"></a>
<a href="https://www.facebook.com/photo/?fbid=778"><img width="240" height="120" src="https://test.fbcdn.net/two.jpg"></a></div>
<div role="article" aria-posinset="2"><button aria-label="Actions for this post by Fixture video"></button>
<a href="https://www.facebook.com/fixture/videos/222222/" target="_blank">2h</a>
<div data-ad-preview="message">A video fixture.</div><video width="240" height="120" poster="https://test.fbcdn.net/poster.jpg"></video></div>
<div role="article" aria-posinset="3"><button aria-label="Actions for this post by Fixture shared"></button>
<a href="https://www.facebook.com/fixture/posts/333333/" target="_blank">3h</a>
<h2>Fixture shared shared a post</h2><div data-ad-preview="message">Shared wrapper commentary.</div>
<div role="article"><div data-ad-preview="message">Nested shared original body.</div></div></div>
</div></main><script type="application/json" data-sjs>{"video":{"id":"222222","width":640,"height":360,
"browser_native_sd_url":"https://test.fbcdn.net/video.mp4","first_frame_thumbnail":"https://test.fbcdn.net/poster.jpg"}}</script>`;
const server=createServer((_req,res)=>{res.setHeader('Content-Type','text/html; charset=utf-8');res.end(page);});
await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));let browser;
try{
  browser=await launchChrome({timeoutMs:60000});
  await browser.send('Emulation.setDeviceMetricsOverride',{width:1280,height:900,deviceScaleFactor:1,mobile:false});
  await browser.send('Network.setBlockedURLs',{urls:['*://*.fbcdn.net/*']});
  await browser.send('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/`});
  for(let n=0;n<50;n++){if(await browser.evaluate('document.readyState==="complete"'))break;await new Promise(r=>setTimeout(r,100));}
  await inject(browser,await loadScripts('facebook'));
  const scripts=await productionScripts();
  // Fixture-only hostname gate override BEFORE content-script captures its registry.
  // Real comparisons never include this script; extraction/policy/resolver stay unchanged.
  scripts.splice(scripts.length-1,0,{file:'fixture/localhost-gate',content:`const actual=globalThis.AkuSourceAdapters;
    globalThis.AkuSourceAdapters=Object.freeze({...actual,get(source){return {...actual.get(source),matchesPage:()=>true};}});`});
  const context=await productionContext(browser,scripts);
  const production=await collectProduction(browser,context),poc=await collect(browser);
  const long=comparePosts(poc,production,'facebook:post:111111');
  assert.ok(long.comparable);assert.equal(long.authorEqual,true);
  assert.equal(long.productionTextCharacters,4000);assert.ok(long.pocTextCharacters>4000);
  assert.equal(long.productionTextIsPocPrefix,true);
  assert.equal(long.pocMedia,2);assert.equal(long.productionMedia,2);assert.equal(long.exactMediaSetsEqual,true);
  assert.equal(await browser.evaluate('document.querySelector("[data-ad-preview=message] span").textContent'),'Short text');
  const video=comparePosts(poc,production,'facebook:post:222222');
  assert.ok(video.comparable);assert.equal(video.pocPlaybackUrls,0);assert.equal(video.productionPlaybackUrls,1);
  assert.equal(video.pocMedia,1);assert.equal(video.productionMedia,1);
  const shared=comparePosts(poc,production,'facebook:post:333333');
  assert.ok(shared.comparable);assert.equal(shared.pocRelationship,'repost');assert.equal(shared.productionRelationship,'repost');
  assert.equal(poc.posts.find(p=>p.id==='facebook:post:333333').limitations.includes('shared_post_body_not_separated'),true);
  const unmatched=comparePosts(poc,production,'facebook:post:999999');assert.equal(unmatched.comparable,false);
  const original=poc.posts.find(p=>p.id==='facebook:post:111111');
  const foreign={...original,author:'Foreign author',text:'Unrelated recommendation'};
  const ambiguousPoc={...poc,posts:[foreign,...poc.posts]};
  const duplicateGuard=comparePosts(ambiguousPoc,{snapshots:[{blocks:[{...foreign,platformId:original.id}]}]},original.id,original);
  assert.equal(duplicateGuard.comparable,false);assert.equal(duplicateGuard.pocBinding.ambiguous,true);
  assert.equal(duplicateGuard.targetMatchedPoc,true);assert.equal(duplicateGuard.targetMatchedProduction,false);
  console.log(JSON.stringify({passed:true,scope:'synthetic_fixture',longTextTruncationObserved:true,
    samePostMultiImageSetsEqual:true,productionStructuredVideoPlaybackObserved:true,sharedPostLimitationPreserved:true,
    unrelatedSameIdBindingRejected:true}));
}finally{if(browser)await browser.close();await new Promise(resolve=>server.close(resolve));}
