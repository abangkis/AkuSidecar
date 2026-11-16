import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { access } from 'node:fs/promises';
import { resolve } from 'node:path';
import { launchChrome, profile } from './chrome.mjs';
import { loadScripts, inject, collect, parseOptions } from './capture.mjs';
import { matchesQuote, applyQuoteRecovery } from './quote-navigation.mjs';

// Real browser + real AkuBridge adapter, synthetic content only. No X account.
const page = `<!doctype html><html><head><meta charset="utf-8"></head><body><main>
<article data-testid="tweet">
  <div data-testid="User-Name">Alice @alice</div>
  <a href="https://x.com/alice/status/123456789"><time datetime="2026-10-01T00:00:00Z">Now</time></a>
  <div data-testid="tweetText">Headless fixture: primary post.</div>
  <button data-testid="tweet-text-show-more-link" onclick="this.previousElementSibling.innerHTML='Headless fixture: primary post.<br>Expanded paragraph <img alt=🙂>';this.remove()">Show more</button>
  <button data-testid="like" aria-label="12 Likes"></button>
  <div data-testid="tweetPhoto"><img width="20" height="20" src="https://pbs.twimg.com/media/fixture-primary.png"></div>
  <div data-testid="quoteTweet"><div data-testid="User-Name">Bob @bob</div>
    <a href="https://x.com/bob/status/987654321"><time datetime="2026-09-30T00:00:00Z">Yesterday</time></a>
    <div data-testid="tweetText">Quoted text must not become primary text.</div>
    <button data-testid="tweet-text-show-more-link">Show more</button>
    <div data-testid="tweetPhoto"><img width="20" height="20" alt="Quoted image" src="https://pbs.twimg.com/media/fixture-quote.png"></div>
    <div data-testid="videoPlayer"><video width="20" height="20" poster="https://pbs.twimg.com/media/fixture-video.png"></video>
      <img width="20" height="20" src="https://pbs.twimg.com/media/fixture-video.png"></div>
  </div>
</article>
<article data-testid="tweet"><div data-testid="User-Name">Carol @carol</div>
  <a href="https://x.com/carol/status/456"><time datetime="2026-10-01T00:00:00Z">Now</time></a>
  <div data-testid="tweetText">Still truncated.</div><button data-testid="tweet-text-show-more-link">Show more</button>
  <div data-testid="tweetPhoto"><img width="20" height="20" src="https://example.invalid/untrusted.png"></div></article>
<article data-testid="tweet"><div data-testid="User-Name">No identity</div><div data-testid="tweetText">Reject me.</div></article>
<article data-testid="tweet"><div data-testid="User-Name">Dana @dana</div>
  <a href="https://x.com/dana/status/789123"><time>Now</time></a>
  <div data-testid="quoteTweet"><div data-testid="User-Name">Bob @bob</div>
  <span><time>Now</time></span>
  <div data-testid="tweetText">Quote only, primary has no text.</div></div></article>
<article data-testid="tweet" style="position:absolute;top:5000px"><div data-testid="User-Name">Offscreen</div>
  <a href="https://x.com/offscreen/status/555"><time>Now</time></a><div data-testid="tweetText">Not visible.</div></article>
</main></body></html>`;
const server = createServer((request, response) => {
  response.setHeader('Content-Type', 'text/html; charset=utf-8');
  response.end(page);
});
await new Promise(r => server.listen(0, '127.0.0.1', r));
let browser;
try {
  const quoted = { text: 'Known quoted text long enough for matching.' };
  assert.ok(matchesQuote(quoted, { text: quoted.text + ' Extra detail.' }));
  assert.equal(matchesQuote(quoted, { text: 'Different unrelated post text.' }), false);
  const recovery = { primaryId: '123456789', quoteId: '987654321', permalink: 'https://x.com/bob/status/987654321',
    quotedText: quoted.text, identityStatus: 'identified', identityProvenance: 'observed_quote_navigation' };
  const recovered = { posts: [{ id: '123456789', quotedPost: { ...quoted, permalink: null }, limitations: ['quoted_identity_unresolved'] }] };
  applyQuoteRecovery(recovered, recovery);
  assert.equal(recovered.posts[0].parentPermalink, recovery.permalink);
  assert.deepEqual(recovered.posts[0].limitations, []);
  const conflicting = { posts: [{ id: '123456789', quotedPost: { ...quoted, permalink: 'https://x.com/other/status/111111' }, limitations: [] }] };
  applyQuoteRecovery(conflicting, recovery);
  assert.equal(conflicting.posts[0].quotedPost.permalink, null);
  assert.equal(conflicting.posts[0].quotedPost.identityStatus, 'conflicting_identity');
  assert.throws(() => parseOptions(['--url', 'https://example.com/']), /x.com/);
  assert.throws(() => parseOptions(['--max-posts', '0']), /Invalid/);
  assert.throws(() => parseOptions(['--max-scrolls', '999']), /Invalid/);
  browser = await launchChrome();
  // Synthetic media URLs are never fetched from the internet by the fixture.
  await browser.send('Network.setBlockedURLs', { urls: ['*://pbs.twimg.com/*', '*://example.invalid/*'] });
  await assert.rejects(launchChrome(), /already in use/);
  const navigation = await browser.send('Page.navigate', { url: `http://127.0.0.1:${server.address().port}/` });
  assert.equal(navigation.errorText, undefined);
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    if (await browser.evaluate('document.readyState === "complete" && Boolean(document.querySelector("article"))')) break;
    await new Promise(r => setTimeout(r, 100));
  }
  await inject(browser, await loadScripts());
  const identityChecks = await browser.evaluate(`(() => {
    const article = [...document.querySelectorAll('article')].find(a => a.querySelector('a[href*="/789123"]'));
    article.__reactProps$fixture = { tweet: { rest_id: '789123', legacy: { quoted_status_id_str: '987654321' } } };
    const element = document.createElement('div');
    let getterCalls = 0;
    const props = { tweet: { rest_id: '123456789', legacy: { quoted_status_id_str: '987654321' } } };
    Object.defineProperty(props, 'secretGetter', { get() { getterCalls++; throw new Error('Getter must never run'); } });
    props.cycle = props;
    element.__reactProps$fixture = props;
    const resolver = globalThis.XHeadlessQuoteIdentity;
    const resolved = resolver.resolve(element, '123456789', null);
    const conflict = resolver.resolve(element, '123456789', 'https://x.com/bob/status/111111');
    const unrelated = resolver.resolve(element, '555555', null);
    props.tweet.quoted_status_result = { result: { rest_id: '222222' } };
    const ambiguous = resolver.resolve(element, '123456789', null);
    return { resolved, conflict, unrelated, ambiguous, getterCalls };
  })()`);
  assert.equal(identityChecks.resolved.permalink, 'https://x.com/i/status/987654321');
  assert.equal(identityChecks.resolved.provenance, 'main_structured_quote_relation');
  assert.equal(identityChecks.conflict.status, 'conflicting_identity');
  assert.equal(identityChecks.conflict.permalink, null);
  assert.equal(identityChecks.unrelated.permalink, null);
  assert.equal(identityChecks.ambiguous.permalink, null);
  assert.equal(identityChecks.getterCalls, 0);
  const result = await collect(browser);
  assert.equal(result.posts.length, 3);
  assert.equal(result.posts[0].id, '123456789');
  assert.equal(result.posts[0].text, 'Headless fixture: primary post.\nExpanded paragraph 🙂');
  assert.equal(result.posts[0].textStatus, 'expanded');
  assert.equal(result.posts[0].textCollapsed, false);
  assert.equal(result.posts[0].quotedPost.text, 'Quoted text must not become primary text.');
  assert.equal(result.posts[0].quotedPost.author, 'Bob @bob');
  assert.equal(result.posts[0].quotedPost.permalink, 'https://x.com/bob/status/987654321');
  assert.equal(result.posts[0].quotedPost.textCollapsed, true);
  assert.deepEqual(result.posts[0].media.map(m => m.url), ['https://pbs.twimg.com/media/fixture-primary.png']);
  assert.deepEqual(result.posts[0].quotedPost.media.map(m => m.kind), ['video_poster', 'image']);
  assert.equal(result.posts[0].quotedPost.media[0].url, 'https://pbs.twimg.com/media/fixture-video.png');
  assert.equal(result.posts[0].quotedPost.media[0].loadedMeaning, 'video_metadata_ready');
  assert.equal(result.posts[0].quotedPost.media[0].loaded, false);
  assert.equal(result.posts[0].quotedPost.media[1].alt, 'Quoted image');
  assert.deepEqual(result.posts[0].quotedPost.mediaExpected.sort(), ['image', 'video']);
  assert.equal(result.posts[0].quotedPost.mediaEvidence.status, 'observed_urls_partial');
  assert.deepEqual(result.posts[1].media, []);
  assert.deepEqual(result.posts[1].mediaEvidence.expectedWithoutUrl, ['image']);
  assert.equal(result.posts[1].textStatus, 'expand_failed');
  assert.equal(result.posts[1].textCollapsed, true);
  assert.equal(result.posts[2].text, '');
  assert.equal(result.posts[2].quotedPost.text, 'Quote only, primary has no text.');
  assert.equal(result.posts[2].quotedPost.permalink, 'https://x.com/i/status/987654321');
  assert.equal(result.posts[2].parentPermalink, 'https://x.com/i/status/987654321');
  assert.equal(result.posts[0].author, 'Alice @alice');
  assert.equal(result.posts[0].publishedAt, '2026-10-01T00:00:00Z');
  assert.equal(result.posts[0].relationshipType, 'quote');
  assert.equal(result.posts[0].parentPermalink, 'https://x.com/bob/status/987654321');
  assert.equal(result.posts[0].engagement.like, '12');
  assert.equal(result.rejected, 1);
  assert.equal(result.loginRequired, false);
  await browser.close();
  await browser.close();
  await assert.rejects(access(resolve(profile, 'poc-owner.lock')), { code: 'ENOENT' });
  console.log(JSON.stringify({ status: 'passed', test: 'real_headless_browser_with_existing_x_adapter', browser: browser.version.product, adapter: result.adapterVersion, visibility: result.visibility, assertions: 'primary_identity_and_text, quote_text_and_media_separation, poster_deduplication, media_host_filter, missing_expected_url, engagement, rejection, viewport, input_bounds, profile_lock, idempotent_cleanup' }));
} finally {
  if (browser) await browser.close();
  await new Promise(r => server.close(r));
}
