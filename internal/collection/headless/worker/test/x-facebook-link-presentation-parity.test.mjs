import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { fileURLToPath } from 'node:url';
import { sourceAssets } from '../worker.mjs';
import { toObservation } from '../observation.mjs';

const bridgePath = fileURLToPath(new URL('../../../../../../AkuBridge/', import.meta.url));
const [xAssets, facebookAssets] = await Promise.all([
  sourceAssets(bridgePath, 'x'),
  sourceAssets(bridgePath, 'facebook'),
]);

function executeAssets(assets, globals) {
  const context = vm.createContext(globals);
  for (const asset of assets.filter(value => value.execute)) {
    vm.runInContext(asset.content, context, { filename: asset.relative });
  }
  return context;
}

function textLeaf(value) { return { nodeType: 3, nodeValue: value }; }

function textElement({ tagName = 'DIV', testId = null, text = '', anchors = [] } = {}) {
  return {
    nodeType: 1,
    tagName,
    childNodes: [textLeaf(text)],
    innerText: text,
    textContent: text,
    getAttribute(name) { return name === 'data-testid' ? testId : null; },
    querySelector() { return null; },
    querySelectorAll(selector) { return selector === 'a[href]' ? anchors : []; },
    contains(node) { return node === this || anchors.includes(node); },
    closest() { return null; },
  };
}

function anchor(href, text = '') {
  return {
    tagName: 'A', href, innerText: text, textContent: text, target: '',
    getAttribute(name) { return name === 'href' ? href : null; },
    hasAttribute() { return false; },
    querySelector() { return null; },
    getBoundingClientRect() { return { left: 1, right: 81, top: 30, bottom: 48, width: 80, height: 18 }; },
    closest() { return null; },
  };
}

function xFixture({ quoteId = '2222222222222222', ambiguous = false } = {}) {
  const primaryId = '1111111111111111';
  const permalink = `https://x.com/owner/status/${primaryId}`;
  const location = { href: permalink, hostname: 'x.com', pathname: `/owner/status/${primaryId}` };
  const primaryAnchor = anchor(permalink);
  const primaryTime = {
    dateTime: '2026-10-12T00:00:00Z', getAttribute: name => name === 'datetime' ? '2026-10-12T00:00:00Z' : null,
    closest: () => primaryAnchor,
  };
  const alternateTime = {
    dateTime: '2026-10-11T00:00:00Z', getAttribute: name => name === 'datetime' ? '2026-10-11T00:00:00Z' : null,
    closest: () => anchor('https://x.com/other/status/3333333333333333'),
  };

  const authoredLinks = [
    anchor('https://news.example/article?ref=post#first', 'Read this'),
    anchor('https://news.example/article?ref=post#duplicate', 'Duplicate'),
    anchor('https://news.example/second', 'Second link'),
    anchor('http://news.example/insecure', 'Insecure'),
    anchor('javascript:alert(1)', 'Script'),
    anchor('https://user:pass@news.example/private', 'Credentials'),
    anchor('https://news.example:8443/nonstandard-port', 'Nonstandard port'),
    anchor(`https://news.example/${'x'.repeat(2050)}`, 'Oversized URL'),
  ];
  const quoteLink = anchor('https://quoted.example/evidence#source', 'Quoted evidence link');
  const quoteStatusAnchor = anchor(`https://x.com/quoted/status/${quoteId}`);
  const quoteTime = {
    dateTime: '2026-10-10T00:00:00Z', getAttribute: name => name === 'datetime' ? '2026-10-10T00:00:00Z' : null,
    closest: () => quoteStatusAnchor,
  };
  const quoteText = textElement({ testId: 'tweetText', text: 'Quoted post body', anchors: [quoteLink] });
  const quoteAuthor = { innerText: 'Quoted author', textContent: 'Quoted author' };
  const quoteNodes = [quoteText, quoteTime, quoteStatusAnchor, quoteLink, quoteAuthor];
  const quote = {
    getAttribute(name) { return name === 'data-testid' ? 'quoteTweet' : name === 'role' ? 'link' : null; },
    querySelector(selector) {
      if (selector === '[data-testid="tweetText"]') return quoteText;
      if (selector === 'time') return quoteTime;
      if (selector === '[data-testid="User-Name"]') return quoteAuthor;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === 'a[href*="/status/"]') return [quoteStatusAnchor];
      return [];
    },
    contains(node) { return node === this || quoteNodes.includes(node) || quoteText.contains(node); },
  };
  const primaryTextRoot = textElement({ testId: 'tweetText', text: 'Primary authored post body', anchors: authoredLinks });
  const aiLabel = { innerText: 'AI-generated', textContent: 'AI-generated', getAttribute: () => null };
  const articleNodes = [primaryTextRoot, quote, ...quoteNodes, primaryAnchor, primaryTime, ...authoredLinks, aiLabel];
  const article = {
    getBoundingClientRect() { return { top: 10, bottom: 240, height: 230, left: 0, right: 500, width: 500 }; },
    contains(node) { return node === this || articleNodes.includes(node) || primaryTextRoot.contains(node) || quote.contains(node); },
    querySelector(selector) {
      if (selector === '[data-testid="quoteTweet"]') return quote;
      if (selector === '[data-testid="tweetText"], [data-testid="twitterArticleReadView"]') return quoteText;
      if (selector === '[data-testid="socialContext"]') return null;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === 'time') return [quoteTime, primaryTime, ...(ambiguous ? [alternateTime] : [])];
      if (selector.includes('tweetText') || selector.includes('twitterArticleReadView')) return [quoteText, primaryTextRoot];
      if (selector === '[data-testid="tweet-text-show-more-link"]') return [];
      if (selector === '[aria-label],[title],[role="button"],button,a') return [aiLabel];
      if (selector === 'video' || selector.includes('img')) return [];
      if (selector === 'a[href]') return [primaryAnchor, ...authoredLinks, quoteStatusAnchor, quoteLink];
      return [];
    },
  };
  const document = {
    title: 'Fixture', readyState: 'complete', visibilityState: 'visible',
    body: { innerText: '' }, scrollingElement: { scrollHeight: 1000 },
    querySelector() { return null; },
    querySelectorAll(selector) {
      if (selector === 'article[data-testid="tweet"]' || selector === 'main article') return [article];
      return [];
    },
  };
  const context = executeAssets(xAssets, {
    URL, setTimeout, clearTimeout, Date, location, window: { location }, document,
    innerHeight: 900, innerWidth: 1280, scrollY: 0,
    AkuHeadlessCapturePolicy: { allowContentExpansion: false, deadlineAt: Date.now() + 2000 },
  });
  context.AkuXMediaEvidence = { avatarKeyFromContainer: () => 'avatar:evidence:key' };
  const real = context.AkuSourceAdapters.get('x');
  const adapter = {
    ...real,
    matchesPage: () => true,
    discoverCandidates: () => ({ candidates: [article], strategy: 'fixture_native_permalink' }),
    findAuthor: () => 'Owner',
    findAvatar: () => null,
    extractSemantics: () => ({ contentKind: 'post', relationshipType: 'quote', parentPermalink: null, engagement: {} }),
    extractDirectContext: () => [],
    mediaAcquisition: { detectExpectedKinds: () => [] },
    loginRequired: () => false,
  };
  context.AkuSourceAdapters = { get: source => source === 'x' ? adapter : null };
  const extractor = xAssets.find(asset => asset.relative === 'worker/vendor/x-extract.js');
  vm.runInContext(extractor.content, context, { filename: extractor.relative });
  return { context, primaryId, permalink, article };
}

function facebookFixture() {
  const location = { href: 'https://www.facebook.com/', hostname: 'www.facebook.com', pathname: '/' };
  const rootSelector = 'div[aria-posinset], [role="article"]';
  let postRoot, quotedRoot, commentRoot;
  const makeAnchor = (href, text, owner) => {
    const value = anchor(href, text);
    value.closest = selector => selector === rootSelector ? owner
      : selector.includes('comments-comment-item') && owner === commentRoot ? commentRoot : null;
    return value;
  };
  const primaryLinks = [
    makeAnchor('https://news.example/facebook?ref=own#first', 'Facebook source', null),
    makeAnchor('https://news.example/facebook?ref=own#duplicate', 'Duplicate source', null),
    makeAnchor('http://news.example/insecure', 'Insecure source', null),
    makeAnchor('https://user:pass@news.example/private', 'Credentialed source', null),
  ];
  const quotedLink = makeAnchor('https://quoted.example/nested', 'Nested evidence', null);
  const commentLink = makeAnchor('https://comment.example/nested', 'Comment evidence', null);
  const messageRoot = {
    nodeType: 1, tagName: 'DIV', innerText: 'Facebook authored post', textContent: 'Facebook authored post',
    getAttribute: name => name === 'data-ad-preview' ? 'message' : null,
    closest: selector => selector === rootSelector ? postRoot : null,
    querySelectorAll(selector) { return selector === 'a[href]' ? [...primaryLinks, quotedLink, commentLink] : []; },
  };
  const quotedMessage = {
    nodeType: 1, tagName: 'DIV', getAttribute: name => name === 'data-ad-preview' ? 'message' : null,
    closest: selector => selector === rootSelector ? quotedRoot : null,
    querySelectorAll() { return [quotedLink]; },
  };
  const commentMessage = {
    nodeType: 1, tagName: 'DIV', getAttribute: name => name === 'data-ad-preview' ? 'message' : null,
    closest: selector => selector === rootSelector ? commentRoot : null,
    querySelectorAll() { return [commentLink]; },
  };
  const contentSelector = '[data-ad-preview="message"], [data-ad-comet-preview="message"]';
  function makeBoundaryRoot(label, message, isComment = false) {
    return {
      tagName: 'DIV',
      matches(selector) { return selector.includes('[role="article"]'); },
      getAttribute(name) { return name === 'role' ? 'article' : name === 'aria-label' ? label : null; },
      hasAttribute() { return false; },
      closest(selector) { return selector === rootSelector ? this : null; },
      querySelectorAll(selector) {
        if (selector === contentSelector || selector.includes('data-ad-preview="message"')) return [message];
        return [];
      },
      querySelector() { return null; },
    };
  }
  postRoot = makeBoundaryRoot('Post by Owner', messageRoot);
  quotedRoot = makeBoundaryRoot('Quoted post', quotedMessage);
  commentRoot = makeBoundaryRoot('Comment by Guest', commentMessage, true);
  for (const value of primaryLinks) value.closest = selector => selector === rootSelector ? postRoot : null;
  quotedLink.closest = selector => selector === rootSelector ? quotedRoot : null;
  commentLink.closest = selector => selector === rootSelector ? commentRoot : null;

  const ownAnchor = makeAnchor('https://www.facebook.com/owner/posts/222222222222222/', 'Post permalink', postRoot);
  const nativeTime = {
    getAttribute: name => name === 'datetime' ? '2026-10-12T00:00:00Z' : null,
    closest: () => ownAnchor,
    closestOwner: postRoot,
  };
  nativeTime.closest = selector => selector === rootSelector ? postRoot : ownAnchor;
  const candidate = {
    tagName: 'DIV', innerText: 'Facebook authored post', textContent: 'Facebook authored post',
    matches(selector) { return selector.includes('[role="article"]'); },
    getAttribute(name) { return name === 'role' ? 'article' : name === 'aria-label' ? 'Post by Owner' : null; },
    hasAttribute() { return false; },
    getBoundingClientRect() { return { top: 10, bottom: 240, height: 230, left: 0, right: 500, width: 500 }; },
    closest(selector) { return selector === rootSelector ? postRoot : null; },
    contains(node) { return node === this || [postRoot, quotedRoot, commentRoot, messageRoot, quotedMessage, commentMessage, nativeTime, ownAnchor,
      ...primaryLinks, quotedLink, commentLink].includes(node); },
    querySelector(selector) {
      if (selector === contentSelector) return messageRoot;
      if (selector === 'time') return nativeTime;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === rootSelector) return [quotedRoot, commentRoot];
      if (selector === contentSelector || selector.includes('data-ad-preview="message"')) return [messageRoot, quotedMessage, commentMessage];
      if (selector === 'a[href]') return [ownAnchor, ...primaryLinks, quotedLink, commentLink];
      if (selector === 'time[datetime]') return [nativeTime];
      if (selector === '[data-utime]') return [];
      if (selector.includes('Actions for this post')) return [];
      if (selector === 'img,video' || selector === 'video' || selector === 'img') return [];
      return [];
    },
  };
  postRoot = candidate;
  ownAnchor.closest = selector => selector === rootSelector ? candidate : null;
  const document = {
    title: 'Fixture', readyState: 'complete', visibilityState: 'visible', body: { innerText: '' },
    scrollingElement: { scrollHeight: 1000 },
    querySelector() { return null; }, querySelectorAll() { return []; },
  };
  const context = executeAssets(facebookAssets, {
    URL, setTimeout, clearTimeout, Date, location, window: { location }, document,
    innerHeight: 900, innerWidth: 1280, scrollY: 0,
    getComputedStyle: () => ({ display: 'block', visibility: 'visible', opacity: '1' }),
    AkuHeadlessCapturePolicy: { allowContentExpansion: false, deadlineAt: Date.now() + 2000 },
  });
  const real = context.AkuSourceAdapters.get('facebook');
  const adapter = {
    ...real,
    matchesPage: () => true,
    discoverCandidates: () => ({ candidates: [candidate], strategy: 'fixture_native_post' }),
    findPermalinkDetails: () => ({ url: 'https://www.facebook.com/owner/posts/222222222222222/', source: 'native_post_anchor' }),
    findAuthor: () => 'Owner',
    findAvatar: () => null,
    extractText: () => 'Facebook authored post',
    extractSemantics: () => ({ contentKind: 'post', relationshipType: 'original', parentPermalink: null, engagement: {} }),
    extractPresentation: () => ({ promoted: false, originSignals: [] }),
    mediaAcquisition: { detectExpectedKinds: () => [], extractCandidates: () => [] },
    availability: () => null,
    loginRequired: () => false,
  };
  context.AkuSourceAdapters = { get: source => source === 'facebook' ? adapter : null };
  const extractor = facebookAssets.find(asset => asset.relative === 'worker/vendor/facebook-extract.js');
  vm.runInContext(extractor.content, context, { filename: extractor.relative });
  return { context, candidate };
}

function observedBlock(source, post) {
  return toObservation({ source, requestedUrl: post.permalink, snapshots: [{ posts: [post], candidateCount: 1 }],
    provenance: {}, capturedAt: '2026-10-12T00:00:00Z' }).snapshots[0].blocks[0];
}

function plain(value) { return JSON.parse(JSON.stringify(value)); }

test('X forwards bounded primary authored links and adapter presentation through observation', async () => {
  const fixture = xFixture();
  const snapshot = await fixture.context.XHeadlessPoC.collect();
  assert.equal(snapshot.posts.length, 1);
  const post = snapshot.posts[0];
  assert.equal(post.text, 'Primary authored post body', 'a quote text root appearing first must not replace the primary text root');
  assert.deepEqual(Array.from(post.links, value => value.href), [
    'https://news.example/article?ref=post',
    'https://news.example/second',
  ]);
  assert.ok(post.links.every(value => value.text.length <= 300 && value.href.length <= 2048));
  assert.equal(post.presentation.originSignals[0].kind, 'platform_ai_label');
  assert.equal(post.presentation.originSignals[0].authority, 'platform');
  assert.ok(post.quotedPost.links.some(value => value.href === 'https://quoted.example/evidence#source'));
  assert.equal(post.links.some(value => value.href.includes('quoted.example')), false);
  assert.deepEqual(Array.from(post.avatarEvidenceKeys), ['x:status:1111111111111111', 'avatar:evidence:key']);

  const block = observedBlock('x', post);
  assert.deepEqual(plain(block.links), plain(post.links));
  assert.deepEqual(plain(block.presentation.originSignals), plain(post.presentation.originSignals));
  assert.equal(Object.hasOwn(block, 'avatarEvidenceKeys'), false, 'private avatar cache keys stay out of Block evidence');
});

test('X excludes a self-referential quote and rejects ambiguous primary identities', async () => {
  const self = xFixture({ quoteId: '1111111111111111' });
  const selfPost = (await self.context.XHeadlessPoC.collect()).posts[0];
  assert.equal(selfPost.quotedPost.identityStatus, 'conflicting_identity');
  assert.equal(selfPost.quotedPost.identityEvidence.conflictReason, 'primary_self_reference');
  assert.equal(selfPost.links.some(value => value.href.includes('quoted.example')), false);

  const ambiguous = xFixture({ ambiguous: true });
  const ambiguousSnapshot = await ambiguous.context.XHeadlessPoC.collect();
  assert.equal(ambiguousSnapshot.posts.length, 0);
  assert.equal(ambiguousSnapshot.rejectionReasons.ambiguous_identity, 1);
});

test('Facebook forwards only safe primary authored links, excluding nested posts and comments', async () => {
  const fixture = facebookFixture();
  const snapshot = await fixture.context.XHeadlessPoC.collect();
  assert.equal(snapshot.posts.length, 1, JSON.stringify(snapshot.rejectionReasons));
  const post = snapshot.posts[0];
  assert.deepEqual(Array.from(post.links, value => value.href), ['https://news.example/facebook?ref=own']);
  const block = observedBlock('facebook', post);
  assert.deepEqual(plain(block.links), plain(post.links));
});
