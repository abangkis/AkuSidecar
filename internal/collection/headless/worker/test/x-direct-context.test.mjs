import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {toObservation} from '../observation.mjs';
import {mergeXReplyDirectContext} from '../x-passive-evidence.mjs';
import {contentContextKnownTypes} from '../../../../httpapi/web/timeline-content-context-state.js';

const bridge = new URL('../../../../../../AkuBridge/', import.meta.url);
const sources = await Promise.all([
  ...['capture-primitives.js', 'source-adapter-runtime.js', 'adapters/x-adapter.js'].map(name => readFile(new URL(name, bridge), 'utf8')),
  ...['x-quote-identity.js', 'x-extract.js'].map(name => readFile(new URL('../vendor/' + name, import.meta.url), 'utf8')),
]);
const ownID = '2106867448746021278';
const parentID = '2106867448746021279';
const url = id => `https://x.com/owner/status/${id}`;
const plain = value => JSON.parse(JSON.stringify(value));
const textNode = (text, testId = 'tweetText') => ({nodeType: 1, tagName: 'DIV', innerText: text,
  childNodes: [{nodeType: 3, nodeValue: text}], getAttribute: name => name === 'data-testid' ? testId : null, closest: () => null,
  querySelector: () => null, querySelectorAll: () => []});

function marker(links = [], label = 'Replying to @parent') {
  return {innerText: label, closest: () => null, querySelector: () => null,
    querySelectorAll: () => links.map(href => ({href}))};
}
function image(src) {
  return {tagName: 'IMG', currentSrc: src, src, alt: 'Photo', complete: true, naturalWidth: 640, naturalHeight: 480,
    getAttribute: name => name === 'data-testid' ? 'tweetPhoto' : null, closest: () => null,
    querySelectorAll: () => []};
}
function article(id, body, {markers = [], quote = null, media = [], surfaceText} = {}) {
  const text = body === null ? null : textNode(body), author = textNode('Owner', 'User-Name');
  const time = {dateTime: '2026-10-12T00:00:00Z', closest: () => ({href: url(id)}),
    getAttribute: () => '2026-10-12T00:00:00Z'};
  const root = {
    innerText: surfaceText ?? [body, '12 likes', 'Reply'].filter(Boolean).join('\n'),
    getBoundingClientRect: () => ({height: 200, top: 0, bottom: 200}), getClientRects: () => [{}],
    contains: target => target === root || target === text || target === time || markers.includes(target) || media.includes(target) || Boolean(quote?.contains(target)),
    getAttribute: name => name === 'data-testid' ? (quote ? 'quoteTweet' : 'tweet') : null,
    querySelector(selector) {
      if (selector === '[data-testid="tweetText"]' || selector.startsWith('[data-testid="tweetText"],') ||
          selector === '[data-testid="twitterArticleReadView"]') {
        return text || quote?.querySelector(selector) || null;
      }
      if (selector === '[data-testid="User-Name"]') return author;
      if (selector === 'time') return time;
      if (selector === '[data-testid="quoteTweet"]') return quote;
      return null;
    },
    querySelectorAll(selector) {
      if (selector === 'time') return [time, ...(quote?.querySelectorAll('time') ?? [])];
      if (selector.includes('tweetPhoto')) return [...media, ...(quote?.querySelectorAll(selector) ?? [])];
      if (selector.includes('tweetText')) {
        return [...(text ? [text] : []), ...(quote?.querySelectorAll(selector) ?? [])];
      }
      if (selector.includes('video')) return quote?.querySelectorAll(selector) ?? [];
      if (selector.includes('replyingTo')) return markers;
      return [];
    },
  };
  return root;
}
async function collect({markers = [], responseID, parents = [], quote = null, body = 'Reply body', media = [], surfaceText} = {}) {
  const child = article(ownID, body, {markers, quote, media, surfaceText});
  const articles = [child, ...parents];
  const location = new URL('https://x.com/home');
  const context = vm.createContext({URL, setTimeout, location, window: {location},
    innerHeight: 900, innerWidth: 1200, scrollY: 0,
    AkuHeadlessCapturePolicy: {allowContentExpansion: false},
    ...(responseID ? {AkuXMediaEvidenceRuntime: {lookupReplyTo: candidate => candidate === `x:status:${ownID}` ? responseID : null}} : {}),
    document: {visibilityState: 'visible', readyState: 'complete', body: {innerText: ''}, querySelector: () => null,
      querySelectorAll: selector => selector === 'article[data-testid="tweet"]' || selector === 'main article' ? articles : []},
  });
  for (const source of sources) vm.runInContext(source, context);
  const snapshot = plain(await context.XHeadlessPoC.collect());
  const post = snapshot.posts.find(value => value.id === ownID);
  const block = toObservation({source: 'x', snapshots: [snapshot], provenance: {},
    capturedAt: '2026-10-12T00:00:00Z', stopReason: 'fixture'}).snapshots[0].blocks.find(value => value.platformId === `x:status:${ownID}`);
  return {post, block};
}

test('shared X parser carries an exact DOM reply and visible parent through headless Observation', async () => {
  const {post, block} = await collect({markers: [marker([url(parentID)])], parents: [article(parentID, 'Original parent')]});
  assert.equal(post.relationshipType, 'reply');
  assert.equal(post.directContext[0].target.id, parentID);
  assert.equal(post.directContext[0].target.text, 'Original parent');
  assert.equal(block.directContext[0].kind, 'replies_to');
  assert.equal(block.directContext[0].provenance, 'observed_dom');
  assert.deepEqual(block.directContext, post.directContext);
  assert.deepEqual(contentContextKnownTypes({source: 'x', directContext: block.directContext}), ['Reply to post']);
  assert.equal(block.text, 'Reply body');
});

test('shared parser preserves available response identity without requiring parent DOM', async () => {
  // A supplied shared-runtime lookup tests parser compatibility, not installation
  // of the windowed passive response observer in headless.
  const {block} = await collect({responseID: parentID});
  assert.equal(block.directContext[0].target.id, parentID);
  assert.equal(block.directContext[0].target.permalink, `https://x.com/i/status/${parentID}`);
  assert.equal(block.directContext[0].target.availability, 'reference_only');
  assert.equal(block.directContext[0].provenance, 'observed_response');
});

test('response identity can pair with an exact bounded DOM-captured parent body', async () => {
  const {post} = await collect({responseID: parentID, parents: [article(parentID, 'Original parent')]});
  const relation = post.directContext.find(value => value.kind === 'replies_to');
  assert.equal(relation.provenance, 'observed_response');
  assert.equal(relation.target.availability, 'captured');
  assert.equal(relation.target.text, 'Original parent');

  const owner = {...post, relationshipType: 'original', parentPermalink: '', directContext: []};
  const enriched = mergeXReplyDirectContext(owner, [relation]);
  assert.equal(enriched.directContext[0].target.text, 'Original parent');
  assert.equal(enriched.directContext[0].target.availability, 'captured');
  assert.equal(enriched.relationshipType, 'reply');
  assert.equal(enriched.parentPermalink, relation.target.permalink);

  const oversized = {...relation, target: {...relation.target, text: 'P'.repeat(4001)}};
  const rejected = mergeXReplyDirectContext(owner, [oversized]);
  assert.deepEqual(rejected.directContext, []);
  assert.equal(rejected.presentation.directContextStatus, 'ambiguous');
});

test('headless does not invent reply identity from body, self or ambiguous marker links', async () => {
  for (const options of [{body: 'Replying to @parent in authored prose'}, {responseID: ownID}]) {
    assert.deepEqual((await collect(options)).block.directContext, []);
  }
  for (const links of [[], [url(parentID), url('2106867448746021280')], [url(ownID)]]) {
    const {block} = await collect({markers: [marker(links)]});
    assert.equal(block.directContext[0].target.id, '');
    assert.equal(block.directContext[0].target.permalink, '');
    assert.equal(block.directContext[0].target.availability, 'reference_only');
  }
});

test('ambiguous parent DOM never supplies an arbitrary parent body', async () => {
  const {block} = await collect({markers: [marker([url(parentID)])],
    parents: [article(parentID, 'First copy'), article(parentID, 'Conflicting copy')]});
  assert.equal(block.directContext[0].target.id, parentID);
  assert.equal(block.directContext[0].target.text, '');
  assert.equal(block.directContext[0].target.availability, 'reference_only');
});

test('quote and reply remain separate captured relations for the existing Related Context projection', async () => {
  const quote = article('2106867448746021281', 'Quoted body');
  const {post, block} = await collect({markers: [marker([url(parentID)])], quote});
  assert.equal(post.relationshipType, 'quote');
  assert.deepEqual(block.directContext.map(value => value.kind), ['quotes', 'replies_to']);
  assert.notEqual(block.directContext[0].target.id, block.directContext[1].target.id);
  assert.equal(block.quotedPost.text, 'Quoted body');
  assert.equal(block.text, 'Reply body');
  assert.deepEqual(contentContextKnownTypes({source: 'x', directContext: block.directContext}), ['Reply to post']);
});

test('headless uses the real X adapter for image-only posts, captions and quoted media', async () => {
  const ownImage = image('https://pbs.twimg.com/media/own-photo.jpg');
  const quotedImage = image('https://pbs.twimg.com/media/quoted-photo.jpg');
  const imageOnly = await collect({body: null, media: [ownImage], surfaceText: 'Owner\n12 likes\nReply\n1 repost'});
  assert.equal(imageOnly.post.text, '');
  assert.equal(imageOnly.block.text, '');
  assert.deepEqual(imageOnly.post.media.map(value => value.url), [ownImage.src]);

  const caption = 'My 2026 photo has 42 blue doors.';
  const quotedText = 'The original says 9 doors were painted.';
  const mediaOnlyQuote = article(parentID, null, {
    surfaceText: 'Quoted author\n5 likes\nReply', media: [quotedImage],
  });
  const quoteOnly = await collect({body: null, surfaceText: 'Owner\n8 likes\nReply', quote: mediaOnlyQuote});
  assert.equal(quoteOnly.post.text, '');
  assert.equal(quoteOnly.post.quotedPost.text, '');
  assert.deepEqual(quoteOnly.post.quotedPost.media.map(value => value.url), [quotedImage.src]);

  const quotedPost = article(parentID, quotedText, {media: [quotedImage]});
  const ownAndQuoted = await collect({body: caption, media: [ownImage], quote: quotedPost});
  assert.equal(ownAndQuoted.post.text, caption);
  assert.deepEqual(ownAndQuoted.post.media.map(value => value.url), [ownImage.src]);
  assert.equal(ownAndQuoted.post.quotedPost.text, quotedText);
  assert.deepEqual(ownAndQuoted.post.quotedPost.media.map(value => value.url), [quotedImage.src]);
  assert.equal(ownAndQuoted.block.text, caption);
});

test('X relation forwarding is bounded and cloned while unsupported sources remain excluded', () => {
  const relation = {kind: 'replies_to', provenance: 'observed_dom', target: {kind: 'post', id: parentID, availability: 'reference_only'}};
  const post = {id: ownID, permalink: url(ownID), author: 'Owner', text: 'Reply body', directContext: Array.from({length: 12}, () => structuredClone(relation))};
  const block = toObservation({source: 'x', snapshots: [{posts: [post]}], provenance: {}, capturedAt: '2026-10-12T00:00:00Z'}).snapshots[0].blocks[0];
  assert.equal(block.directContext.length, 8);
  post.directContext[0].target.id = ownID;
  assert.equal(block.directContext[0].target.id, parentID);
  const facebook = {...post, id: 'facebook:post:12345', permalink: 'https://www.facebook.com/owner/posts/12345'};
  assert.equal(toObservation({source: 'facebook', snapshots: [{posts: [facebook]}], provenance: {}, capturedAt: '2026-10-12T00:00:00Z'}).snapshots[0].blocks[0].directContext, undefined);
});
