import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { launchChrome } from './chrome.mjs';
import { loadScripts, inject, collect, parseOptions } from './capture.mjs';
const page = `<!doctype html><meta charset="utf-8"><div aria-label="Account"></div><main role="main"><div role="feed">
<div role="article" aria-posinset="1"><button aria-label="Actions for this post by Alice"></button>
<h2><a role="link" href="https://www.facebook.com/alice">Alice</a></h2>
<a href="https://www.facebook.com/alice/posts/pfbidFixtureAlpha/?tracking=removed" target="_blank">1h</a>
<div data-ad-preview="message"><span>Short original Facebook fixture text.</span><div role="button" onclick="const s=this.previousElementSibling;if(this.textContent==='See more'){s.innerHTML='Short original Facebook fixture text.<br>Expanded paragraph <img alt=🙂>';this.textContent='See less'}else{s.textContent='Short original Facebook fixture text.';this.textContent='See more'}">See more</div></div>
<a role="link" href="https://www.facebook.com/alice"><img width="40" height="40" src="https://test.fbcdn.net/avatar.jpg"></a>
<a href="https://www.facebook.com/photo/?fbid=777"><img width="150" height="80" src="https://test.fbcdn.net/media.jpg"></a>
<div role="button" aria-label="12 reactions"></div><div role="button" aria-label="3 comments"></div>
<div role="article"><div data-ad-preview="message">Nested content excluded.</div></div></div>
<div role="article" aria-posinset="2"><button aria-label="Actions for this post by Bob"></button>
<a href="https://www.facebook.com/bob/videos/888888/" target="_blank">2h</a><div data-ad-preview="message">A video fixture with a poster only.</div>
<video width="120" height="60" poster="https://video.fbcdn.net/poster.jpg"></video></div>
<div role="article"><button aria-label="Actions for this post by Unknown"></button><div data-ad-preview="message">No stable permalink must be rejected.</div></div>
<div aria-label="Reels"><div role="article"><button aria-label="Actions for this post by Reel"></button></div></div>
</div></main>`;
const server = createServer((req,res) => { res.setHeader('Content-Type','text/html; charset=utf-8'); res.end(page); });
await new Promise(resolve => server.listen(0,'127.0.0.1',resolve));
let browser;
try {
  assert.equal(parseOptions(['--source','facebook']).url,'https://www.facebook.com/');
  assert.throws(() => parseOptions(['--source','facebook','--url','https://www.facebook.com.evil.test/']),/HTTPS/);
  browser = await launchChrome();
  await browser.send('Network.setBlockedURLs',{urls:['*://*.fbcdn.net/*']});
  await browser.send('Page.navigate',{url:`http://127.0.0.1:${server.address().port}/`});
  for (let n=0;n<50;n++) { if(await browser.evaluate('document.readyState === "complete"')) break; await new Promise(r=>setTimeout(r,100)); }
  await inject(browser,await loadScripts('facebook'));
  const result = await collect(browser);
  assert.equal(result.posts.length,2);
  const first=result.posts[0];
  assert.equal(first.id,'facebook:post:pfbidFixtureAlpha');
  assert.equal(first.permalink,'https://www.facebook.com/alice/posts/pfbidFixtureAlpha/');
  assert.equal(first.author,'Alice');
  assert.equal(first.text,'Short original Facebook fixture text.\nExpanded paragraph 🙂');
  assert.equal(first.textStatus,'expanded');
  assert.equal(first.textCollapsed,false);
  assert.equal(first.timestampSource,'relative_text_estimate');
  assert.ok(Number.isFinite(Date.parse(first.publishedAt)));
  assert.equal(first.engagement.like,'12');
  assert.equal(first.media.length,1);
  assert.equal(first.media[0].url,'https://test.fbcdn.net/media.jpg');
  assert.equal(result.posts[1].media[0].kind,'video_poster');
  assert.equal(result.posts[1].media[0].loaded,false);
  assert.equal(result.rejectionReasons.missing_identity,1);
  assert.ok(result.candidateDiagnostics.rejectedReasons.nested_article>0);
  assert.ok(result.candidateDiagnostics.rejectedReasons.non_feed_surface>0);
  assert.equal(result.authenticatedUiObserved,true);
  assert.equal(await browser.evaluate('document.querySelector("[data-ad-preview=message]").innerText.includes("Expanded paragraph")'),false);
  await browser.evaluate(`document.body.innerHTML = \`<div aria-label="Account"></div><main><div role="feed"><div role="article">
    <button aria-label="Actions for this post by Hover Author"></button>
    <a href="#" onmouseenter="this.href='https://www.facebook.com/hover/posts/999991/';this.setAttribute('aria-describedby','clock-tip');document.getElementById('clock-tip').hidden=false">Timestamp</a>
    <div data-ad-preview="message">Scoped hover hydration must retain this native post identity.</div>
    </div></div></main><aside id="clock-tip" role="tooltip" hidden>3 hours ago</aside>\``);
  const hovered = await collect(browser);
  assert.equal(hovered.posts[0].id,'facebook:post:999991');
  assert.equal(hovered.posts[0].hoverEvidence.permalinkRecoveredByHover,true);
  assert.equal(hovered.posts[0].timestampSource,'relative_text_estimate');
  assert.equal(hovered.posts[0].timestampEstimated,true);
  await browser.evaluate(`document.querySelector('[role="article"] a').href='https://www.facebook.com/hover/posts/999992/'`);
  assert.equal((await collect(browser)).posts[0].publishedAt,null);
  await browser.evaluate(`document.querySelector('[role="article"] a').href='https://www.facebook.com/hover/posts/999991/'`);
  await browser.evaluate(`document.querySelector('[role="article"]').insertAdjacentHTML('beforeend','<abbr data-utime="1770000000"></abbr>')`);
  assert.equal((await collect(browser)).posts[0].timestampSource,'native_unix_attribute');
  await browser.evaluate(`(() => {
    const root=document.querySelector('[role="article"]');
    root.insertAdjacentHTML('beforeend','<abbr data-utime="1770000200"></abbr>');
    root.querySelector('a').textContent='Unparseable date';root.querySelector('a').removeAttribute('aria-describedby');
    globalThis.FacebookHeadlessEvidenceState.cache.delete(root);
  })()`);
  assert.equal((await collect(browser)).posts[0].publishedAt,null);
  await browser.evaluate(`document.body.innerHTML = \`<div aria-label="Account"></div><main><div role="feed"><div aria-posinset="1"><div role="article">
    <button aria-label="Actions for this post by Wrapper Author"></button><a href="https://www.facebook.com/wrapper/posts/555551/">Timestamp</a>
    <div data-ad-preview="message">Primary article inside a feed position wrapper.</div><abbr data-utime="1770000000"></abbr>
    <div role="article"><abbr data-utime="1770000200"></abbr></div>
    </div></div></div></main>\``);
  const wrapped = await collect(browser);
  assert.ok(wrapped.posts.length>0);
  assert.ok(wrapped.posts.every(post=>post.publishedAt==='2026-02-02T02:40:00.000Z'));
  await browser.evaluate(`document.body.innerHTML = \`<div aria-label="Account"></div><main><div role="feed"><div role="article">
    <button aria-label="Actions for this post by Structured Author"></button><a href="https://www.facebook.com/story-author/posts/999995/">Native post</a>
    <div data-ad-preview="message">Timestamp must be bound to this exact native post.</div></div></div></main>
    <script type="application/json" data-sjs>{"stories":[{"url":"https://www.facebook.com/story-author/posts/999995/","id":"opaque-story-1","comet_sections":{"timestamp":{"story":{"id":"opaque-story-1","creation_time":1770000000}}}},{"url":"https://www.facebook.com/other/posts/111112/","creation_time":1770000200}]}</script>\``);
  const structured = await collect(browser);
  assert.equal(structured.posts[0].timestampSource,'native_structured_story');
  assert.equal(structured.posts[0].timestampEstimated,false);
  assert.equal(structured.posts[0].publishedAt,'2026-02-02T02:40:00.000Z');
  await browser.evaluate(`const script=document.querySelector('script[data-sjs]');const data=JSON.parse(script.textContent);data.stories.push({url:'https://www.facebook.com/story-author/posts/999995/',creation_time:1770000300});script.textContent=JSON.stringify(data);`);
  const conflictingTime = await collect(browser);
  assert.equal(conflictingTime.posts[0].publishedAt,null);
  assert.equal(conflictingTime.posts[0].timestampSource,'conflicting_structured_time');
  // Reproduce a broad position wrapper borrowing its nested target's permalink.
  await browser.evaluate(`document.body.innerHTML=\`<div aria-label="Account"></div><main><div role="feed">
    <div aria-posinset="1"><button aria-label="Actions for this post by Foreign background"></button>
    <a href="#">Unknown date</a><div data-ad-preview="message">Foreign background body.</div>
    <abbr data-utime="1770000200"></abbr><a href="/photo/?fbid=777"><img width="240" height="120" src="https://test.fbcdn.net/foreign.jpg"></a>
    <div role="article"><button aria-label="Actions for this post by Correct target"></button>
    <a href="https://www.facebook.com/target/posts/666661/" target="_blank">1h</a>
    <div data-ad-preview="message">Correct target body.</div><a href="/photo/?fbid=778"><img width="240" height="120" src="https://test.fbcdn.net/target.jpg"></a>
    </div></div></div></main>\``);
  const bound=await collect(browser);
  assert.equal(bound.posts.length,1);assert.equal(bound.posts[0].id,'facebook:post:666661');
  assert.equal(bound.posts[0].author,'Correct target');assert.equal(bound.posts[0].text,'Correct target body.');
  assert.equal(bound.posts[0].media.length,1);assert.equal(bound.posts[0].media[0].url,'https://test.fbcdn.net/target.jpg');
  assert.equal(bound.posts[0].timestampSource,'relative_text_estimate');
  assert.equal(bound.rejectionReasons.missing_identity,1);
  await browser.evaluate(`document.body.innerHTML=\`<div aria-label="Account"></div><main><div role="feed">
    <div role="article"><button aria-label="Actions for this post by One owner"></button><a href="https://www.facebook.com/one/posts/666662/">1h</a><div data-ad-preview="message">One body.</div></div>
    <div role="article"><button aria-label="Actions for this post by Other owner"></button><a href="https://www.facebook.com/other/posts/666662/">1h</a><div data-ad-preview="message">Other body.</div></div>
    </div></main>\``);
  const duplicate=await collect(browser);assert.equal(duplicate.posts.length,0);
  assert.equal(duplicate.rejectionReasons.conflicting_post_binding,2);
  await browser.evaluate(`document.body.innerHTML=\`<div aria-label="Account"></div><main><div role="feed">
    <div role="article"><button aria-label="Actions for this post by Primary owner"></button>
    <a href="https://www.facebook.com/primary/posts/666663/">1h</a><div data-ad-preview="message">Primary comment.
    <div role="article"><button aria-label="Actions for this post by Embedded owner"></button>
    <a href="https://www.facebook.com/embedded/posts/666664/">2h</a><div data-ad-preview="message">Embedded foreign body.</div>
    <a href="/photo/?fbid=779"><img width="240" height="120" src="https://test.fbcdn.net/embedded.jpg"></a></div></div>
    </div></div></main>\``);
  const embedded=await collect(browser);assert.equal(embedded.posts.length,1);
  assert.equal(embedded.posts[0].id,'facebook:post:666663');assert.equal(embedded.posts[0].text,'Primary comment.');
  assert.equal(embedded.posts[0].media.length,0);
  await browser.evaluate(`document.body.innerHTML=\`<div aria-label="Account"></div><main><div role="feed">
    <div role="article"><button aria-label="Actions for this post by Hydrated owner"></button>
    <a href="https://www.facebook.com/other/posts/777772/" target="_blank" onmouseenter="this.href='https://www.facebook.com/hydrated/posts/777771/'">1h</a>
    <div data-ad-preview="message">Hydration must replace a valid but wrong initial permalink.</div><abbr data-utime="1770000000"></abbr>
    <div role="article"><a href="/photo/?fbid=880"><img width="240" height="120" src="https://test.fbcdn.net/article-media.jpg"></a></div></div>
    <div role="article"><button aria-label="Actions for this post by Other owner"></button>
    <a href="https://www.facebook.com/other/posts/777772/" target="_blank">2h</a><div data-ad-preview="message">Other post remains distinct.</div></div>
    </div></main>\``);
  const hydrated=await collect(browser);assert.equal(hydrated.posts.length,2);
  const hydratedPost=hydrated.posts.find(p=>p.id==='facebook:post:777771');assert.ok(hydratedPost);
  assert.equal(hydratedPost.hoverEvidence.permalinkChangedByHover,true);
  assert.equal(hydratedPost.timestampSource,'native_unix_attribute');
  assert.equal(hydratedPost.media[0].url,'https://test.fbcdn.net/article-media.jpg');
  await browser.evaluate(`document.body.innerHTML=\`<div aria-label="Account"></div><main><div role="feed">
    <div role="article"><button aria-label="Actions for this post by Fragment owner"></button><a href="#">1h</a>
    <div data-ad-preview="message">A fragment is not independent native post identity.</div></div>
    </div></main>\`;
    Object.defineProperty(document.querySelector('a'), 'href', {get(){return 'https://www.facebook.com/other/posts/777773/#';}});`);
  const fragment=await collect(browser);assert.equal(fragment.posts.length,0);
  assert.equal(fragment.rejectionReasons.missing_identity,1);
  await browser.evaluate('document.body.innerHTML = \'<form action="/login"><input name="email"><input name="pass"></form>\'');
  assert.equal((await collect(browser)).loginRequired,true);
  await browser.evaluate('document.body.innerHTML = \'<h1>Account temporarily unavailable</h1><p>Unavailable due to a site issue</p>\'');
  assert.equal((await collect(browser)).sourceUnavailable.code,'site_outage');
  console.log(JSON.stringify({status:'passed',source:'facebook',adapter:result.adapterVersion,
    checks:'identity, hover_permalink_recovery, scoped_tooltip, native_unix_time, timestamp_conflict_unknown, expanded_text, restoration, media_scope, admission, login_outage_detection, nested_container_ownership, duplicate_identity_fail_closed, embedded_body_exclusion, known_permalink_hydration, article_media_preserved, fragment_permalink_rejected'}));
} finally { if(browser) await browser.close(); await new Promise(resolve=>server.close(resolve)); }
