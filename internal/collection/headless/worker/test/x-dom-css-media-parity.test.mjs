import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { sourceAssets } from '../worker.mjs';
import { sourceProvenance } from '../provenance.mjs';
import { toObservation } from '../observation.mjs';

const bridge = new URL('../../../../../../AkuBridge/', import.meta.url);
const scripts = Object.fromEntries(await Promise.all([
  'capture-primitives.js', 'bounded-capture-policy.js', 'source-adapter-runtime.js',
  'adapters/x-adapter.js', 'content-script.js',
].map(async name => [name, await readFile(new URL(name, bridge), 'utf8')])));
const extractor = await readFile(new URL('../vendor/x-extract.js', import.meta.url), 'utf8');
const quoteIdentity = await readFile(new URL('../vendor/x-quote-identity.js', import.meta.url), 'utf8');
const id = '2106867448746021278';
const quotedID = '2106867448746021279';
const native = `https://x.com/fixture/status/${id}`;
const photo = 'https://pbs.twimg.com/media/owned-photo.jpg';
const poster = 'https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg';
const master = 'https://video.twimg.com/ext_tw_video/12345/pu/pl/master.m3u8';
const mp4 = 'https://video.twimg.com/ext_tw_video/12345/pu/vid/720x720/movie.mp4';
const capturedAt = '2026-11-12T00:00:00.000Z';
const plain = value => JSON.parse(JSON.stringify(value));

// Synthetic DOM implementing only the tag, attribute, descendant and comma
// selector forms used by the real adapter's media contract. No CSS selectors
// are copied into the worker or substituted for the adapter's selector lists.
function matchesSimple(node, selector) {
  if (selector === '*') return true;
  const tag = selector.match(/^[A-Za-z][\w-]*/)?.[0];
  if (tag && node.tagName !== tag.toUpperCase()) return false;
  let consumed = tag || '';
  for (const match of selector.matchAll(/\[([\w-]+)(?:(\^=|\*=|=)"([^"]*)"(?:\s+(i))?)?\]/g)) {
    consumed += match[0];
    const actual = node.getAttribute(match[1]);
    if (actual === null) return false;
    if (!match[2]) continue;
    const value = match[4] ? actual.toLowerCase() : actual;
    const expected = match[4] ? match[3].toLowerCase() : match[3];
    if (match[2] === '=' && value !== expected || match[2] === '^=' && !value.startsWith(expected)
        || match[2] === '*=' && !value.includes(expected)) return false;
  }
  return consumed === selector;
}

function matches(node, selector) {
  return selector.split(',').some(part => {
    const chain = part.trim().match(/(?:[^\s\[\]]|\[[^\]]*\])+/g) || [];
    if (!chain.length || !matchesSimple(node, chain.at(-1))) return false;
    let parent = node.parentElement;
    for (let index = chain.length - 2; index >= 0; index--) {
      while (parent && !matchesSimple(parent, chain[index])) parent = parent.parentElement;
      if (!parent) return false;
      parent = parent.parentElement;
    }
    return true;
  });
}

function element(tag, attrs = {}, children = [], properties = {}) {
  const node = {
    nodeType: 1, tagName: tag.toUpperCase(), parentElement: null, children,
    childNodes: children, innerText: '', textContent: '',
    style: { backgroundImage: attrs.backgroundImage || '' },
    computedBackground: attrs.computedBackground || 'none',
    currentSrc: '', src: attrs.src || '', srcset: attrs.srcset || '', poster: attrs.poster || '',
    complete: true, naturalWidth: 640, naturalHeight: 360,
    videoWidth: 640, videoHeight: 360, readyState: 1,
    paused: true, currentTime: 0, duration: 12,
    getAttribute(name) {
      if (name === 'style' && node.style.backgroundImage) return `background-image:${node.style.backgroundImage}`;
      return attrs[name] === undefined ? null : String(attrs[name]);
    },
    matches(selector) { return matches(node, selector); },
    closest(selector) {
      for (let current = node; current; current = current.parentElement) if (matches(current, selector)) return current;
      return null;
    },
    contains(other) { return other === node || children.some(child => child.contains?.(other)); },
    querySelectorAll(selector) {
      const descendants = children.flatMap(child => [child, ...child.querySelectorAll('*')]);
      return descendants.filter(child => matches(child, selector));
    },
    querySelector(selector) { return node.querySelectorAll(selector)[0] || null; },
    getBoundingClientRect() { return { top: 0, bottom: node.height ?? 360,
      left: 0, right: node.width ?? 640, width: node.width ?? 640, height: node.height ?? 360 }; },
    ...properties,
  };
  for (const child of children) child.parentElement = node;
  return node;
}

const img = (attrs = {}, properties = {}) => element('img', attrs, [], { alt: attrs.alt || '', ...properties });
const photoRoot = children => element('div', { 'data-testid': 'tweetPhoto' }, children);
const videoRoot = children => element('div', { 'data-testid': 'videoPlayer' }, children);
const background = (url, properties = {}) => element('div', { backgroundImage: `url("${url}")` }, [], properties);

function fixture({ own = [], quoted = [], resources = [], retainQuote = false, skipImage } = {}) {
  const text = 'Owned fixture text keeps media availability independent from candidate admission.';
  const textRoot = element('fixturetext', {}, [], { innerText: text,
    childNodes: [{ nodeType: 3, nodeValue: text }] });
  const quote = quoted.length ? element('aside', { 'data-testid': 'quoteTweet' }, quoted) : null;
  const container = element('article', {}, [textRoot, ...own, ...(quote ? [quote] : [])]);
  element('div', { role: 'link', 'data-href': native }, [container]);
  const location = { href: 'https://x.com/home', hostname: 'x.com', pathname: '/home' };
  const document = { body: element('body'), readyState: 'complete', visibilityState: 'visible',
    title: 'CSS media fixture', scrollingElement: { scrollHeight: 1000 },
    querySelector: () => null, querySelectorAll: () => [] };
  const context = vm.createContext({ URL, setTimeout, clearTimeout, location, document,
    innerHeight: 900, innerWidth: 1200, scrollY: 0,
    performance: { getEntriesByType: kind => kind === 'resource' ? resources : [] },
    getComputedStyle: node => ({ backgroundImage: node.computedBackground }),
  });
  context.window = context;
  for (const name of ['capture-primitives.js', 'bounded-capture-policy.js',
    'source-adapter-runtime.js', 'adapters/x-adapter.js']) vm.runInContext(scripts[name], context);
  const real = context.AkuSourceAdapters.get('x');
  const adapter = { ...real,
    discoverCandidates: () => ({ candidates: [container], strategy: 'controlled_owned_identity' }),
    findQuotedRoot: () => quote, resolveIdentity: () => undefined,
    findAuthor: () => 'Fixture owner', findAvatar: () => null,
    contentRootSelector: 'fixturetext', resolveContentRoot: () => textRoot,
    extractText: () => text, extractSemantics: () => ({ contentKind: 'post', relationshipType: 'original' }),
    extractPresentation: () => ({}), contentExpansion: real.contentExpansion, loginRequired: () => false,
    feedRootPresent: () => true,
    extractQuotedPost: (_, { findMedia }) => retainQuote && quote ? {
      author: 'Quoted owner', text: 'Quoted fixture text',
      permalink: `https://x.com/quoted/status/${quotedID}`, media: findMedia(quote),
    } : null,
    ...(skipImage ? { shouldSkipImage: skipImage } : {}),
  };
  const adapters = { get: source => source === 'x' ? adapter : null };
  context.AkuSourceAdapters = adapters;
  context.sourceAdapters = adapters;
  context.capturePolicy = context.AkuBoundedCapturePolicy;
  const start = scripts['content-script.js'].indexOf('  function findMedia(');
  const end = scripts['content-script.js'].indexOf('\n  function findPermalink(', start);
  assert.ok(start > 0 && end > start, 'actual Windowed function boundaries must remain identifiable');
  vm.runInContext(scripts['content-script.js'].slice(start, end), context);
  vm.runInContext(quoteIdentity, context);
  vm.runInContext(extractor, context);
  // Time identity is outside media acquisition and does not require a full DOM.
  const query = container.querySelectorAll.bind(container);
  container.querySelectorAll = selector => selector === 'time'
    ? [{ dateTime: capturedAt, closest: () => ({ href: native }) }] : query(selector);
  return {
    context, adapter, container, quote,
    windowed: root => plain(context.findMedia(root || container, 'x', { excludeRoot: root ? null : quote })),
    async capture() {
      const snapshot = plain(await context.XHeadlessPoC.collect());
      const observation = plain(toObservation({ source: 'x', requestedUrl: location.href,
        snapshots: [snapshot], capturedAt, provenance: {}, stopReason: 'completed',
        performedScrolls: 0, scrollStopReason: 'budget', xMediaPolicy: context.AkuBoundedCapturePolicy }));
      return { snapshot, block: observation.snapshots[0].blocks[0] };
    },
  };
}

function mediaContract(values) {
  return values.map(value => ({ kind: value.kind === 'video_poster' ? 'video' : value.kind,
    url: value.posterUrl || value.url, playbackUrl: value.playbackUrl || null,
    playbackMode: value.kind === 'image' ? null : value.playbackMode || 'native',
    width: value.width, height: value.height }));
}

async function parity(f) {
  const result = await f.capture();
  assert.ok(result.block, 'controlled candidate identity must produce an Observation block');
  assert.deepEqual(mediaContract(result.block.media), mediaContract(f.windowed()));
  assert.equal(result.block.captureQuality.status, 'unverified');
  return result;
}

test('inline CSS photo and computed CSS video use the same shared collector through Observation JSON', async () => {
  const cssPhoto = background(photo);
  const cssVideo = element('div', { 'data-testid': 'videoComponent',
    computedBackground: `url("${poster}")` });
  const f = fixture({ own: [photoRoot([cssPhoto]), cssVideo] });
  const { block } = await parity(f);
  assert.equal(block.media.length, 2);
  assert.deepEqual(block.media.map(value => value.url), [photo, poster]);
  for (const value of block.media) {
    assert.equal(value.loaded, false, 'CSS geometry is not image decode or video metadata evidence');
    assert.equal(value.loadedMeaning, 'css_background_observed');
    assert.equal(value.urlSource, 'css_background');
    assert.ok(!value.playbackUrl);
  }
  assert.notEqual(block.mediaRecovery.outcome, 'verified_owned_playback');
});

test('legacy, wrapped and article image selectors preserve actual DOM decode evidence', async t => {
  const examples = {
    legacy: () => photoRoot([img({ src: photo })]),
    wrapped: () => img({ src: photo }),
    article: () => element('div', { 'data-testid': 'twitterArticleReadView' }, [img({ 'data-src': photo })]),
    'photo permalink': () => element('a', { href: `${native}/photo/1` }, [img({ src: photo })]),
  };
  for (const [name, make] of Object.entries(examples)) await t.test(name, async () => {
    const { block } = await parity(fixture({ own: [make()] }));
    assert.equal(block.media.length, 1);
    assert.equal(block.media[0].url, photo);
    assert.equal(block.media[0].loaded, name !== 'article', 'a data-src alternative alone does not prove that URL decoded');
    assert.equal(block.media[0].loadedMeaning, 'image_decoded');
  });
});

test('currentSrc, src property, src attribute, srcset and lazy image alternatives share one priority order', async t => {
  const alternate = 'https://pbs.twimg.com/media/alternate.jpg';
  const examples = [
    ['current_src', { src: alternate }, { currentSrc: photo }],
    ['src_property', { src: alternate }, { src: photo }],
    ['src_attribute', { src: photo }, { src: '' }],
    ['srcset', { srcset: `${photo} 1x, ${alternate} 2x` }, {}],
    ['data_src', { 'data-src': photo }, {}],
    ['data_original', { 'data-original': photo }, {}],
  ];
  for (const [name, attrs, properties] of examples) await t.test(name, async () => {
    const { block } = await parity(fixture({ own: [photoRoot([img(attrs, properties)])] }));
    assert.equal(block.media.length, 1);
    assert.equal(block.media[0].url, photo);
    assert.equal(block.media[0].urlSource, name);
    assert.equal(block.media[0].loaded, ['current_src', 'src_property', 'src_attribute'].includes(name));
  });
});

test('undecoded and loaded DOM images retain their observed states instead of inheriting CSS state', async () => {
  const f = fixture({ own: [photoRoot([
    img({ src: photo }, { complete: false, naturalWidth: 0, naturalHeight: 0 }),
    img({ src: 'https://pbs.twimg.com/media/decoded.jpg' }),
  ])] });
  const { block } = await parity(f);
  assert.deepEqual(block.media.map(value => value.loaded), [false, true]);
});

test('an alternate valid image URL cannot inherit decode evidence from a different currentSrc', async () => {
  const { block } = await parity(fixture({ own: [photoRoot([
    img({ src: photo }, { currentSrc: 'https://unrelated.example/decoded-placeholder.jpg' }),
  ])] }));
  assert.equal(block.media[0].url, photo);
  assert.equal(block.media[0].urlSource, 'src_property');
  assert.equal(block.media[0].loaded, false);
});

test('video poster CSS fallback and nested source MP4 preserve observed video metadata', async () => {
  const node = element('video', { backgroundImage: `url("${poster}")` },
    [element('source', { src: mp4 })], { currentSrc: 'blob:https://x.com/player' });
  const { block } = await parity(fixture({ own: [videoRoot([node])] }));
  assert.equal(block.media.length, 1);
  assert.equal(block.media[0].playbackUrl, mp4);
  assert.equal(block.media[0].loaded, true);
  assert.equal(block.media[0].loadedMeaning, 'video_metadata_ready');
  assert.equal(block.mediaRecovery.outcome, 'owned_playback_url_observed');
  assert.equal(block.mediaRecovery.limitation, 'video_playback_unverified');
});

test('wrapped video-poster images acquire matching A1 HLS without claiming decode or playback', async () => {
  const node = img({ src: poster }, { complete: false, naturalWidth: 0, naturalHeight: 0 });
  const { snapshot, block } = await parity(fixture({ own: [node], resources: [{ name: master, responseStatus: 200 }] }));
  assert.equal(snapshot.posts[0].media[0].kind, 'video_poster');
  assert.equal(block.media[0].kind, 'video');
  assert.equal(block.media[0].playbackUrl, master);
  assert.equal(block.media[0].loaded, false);
  assert.equal(block.mediaRecovery.limitation, 'video_playback_unverified');
});

test('CSS poster and DOM video duplicates retain MP4 precedence over HLS and native-only evidence', async () => {
  const node = element('video', { poster }, [], { currentSrc: mp4 });
  const duplicate = element('div', { 'data-testid': 'videoComponent', backgroundImage: `url("${poster}")` });
  const { block } = await parity(fixture({ own: [node, duplicate], resources: [{ name: master, responseStatus: 200 }] }));
  assert.equal(block.media.length, 1);
  assert.equal(block.media[0].playbackUrl, mp4);
  assert.equal(block.media[0].loadedMeaning, 'video_metadata_ready');
});

test('avatar URLs and the adapter image-skip policy cannot promote profile images into post media', async () => {
  const avatar = 'https://pbs.twimg.com/profile_images/42/avatar.jpg';
  const skipped = img({ src: 'https://pbs.twimg.com/media/skipped.jpg' });
  const f = fixture({ own: [photoRoot([img({ src: avatar }), img({ src: photo }), skipped])],
    skipImage: node => node === skipped });
  const { block } = await parity(f);
  assert.deepEqual(block.media.map(value => value.url), [photo]);
});

test('quote CSS media stays separate and cannot add quote-only media to the primary post', async () => {
  const quotedPhoto = 'https://pbs.twimg.com/media/quoted-photo.jpg';
  const f = fixture({ own: [photoRoot([background(photo)])],
    quoted: [photoRoot([background(quotedPhoto)])], retainQuote: true });
  const { block } = await parity(f);
  assert.deepEqual(block.media.map(value => value.url), [photo]);
  assert.deepEqual(mediaContract(block.quotedPost.media), mediaContract(f.windowed(f.quote)));
  assert.deepEqual(block.quotedPost.media.map(value => value.url), [quotedPhoto]);
});

test('duplicate URLs collapse and the existing 20-media ceiling applies to CSS and DOM together', async () => {
  const nodes = Array.from({ length: 25 }, (_, index) => photoRoot([
    img({ src: `https://pbs.twimg.com/media/owned-${index}.jpg` }),
    background(`https://pbs.twimg.com/media/owned-${index}.jpg`),
  ]));
  const { block } = await parity(fixture({ own: nodes }));
  assert.equal(block.media.length, 20);
  assert.equal(new Set(block.media.map(value => value.url)).size, 20);
});

test('trusted CSS roots preserve unknown geometry without claiming decode', async () => {
  const { block } = await parity(fixture({ own: [
    photoRoot([background(photo, { width: 0, height: 0 })]),
    photoRoot([background('https://pbs.twimg.com/media/partial.jpg', { width: 1, height: 0 })]),
  ] }));
  assert.equal(block.media.length, 1);
  assert.equal(block.media[0].width, 0);
  assert.equal(block.media[0].height, 0);
  assert.equal(block.media[0].loaded, false);
});

test('large DOM candidate sets record the collection bound and retain bounded output', async () => {
  const f = fixture({ own: Array.from({ length: 140 }, (_, index) =>
    photoRoot([img({ src: `https://pbs.twimg.com/media/bounded-${index}.jpg` })])) });
  const shared = f.context.AkuBoundedCapturePolicy.extractXDOMMedia(f.container, f.adapter, { excludeRoot: f.quote });
  assert.equal(shared.bounded, true);
  assert.equal(shared.media.length, 20);
  assert.equal(shared.observations.length, shared.media.length);
  await parity(f);
});

test('foreign, non-HTTPS and undersized candidates remain rejected by shared normalization', async () => {
  const { block } = await parity(fixture({ own: [
    photoRoot([img({ src: 'https://unrelated.example/photo.jpg' })]),
    photoRoot([background('https://unrelated.example/background.jpg')]),
    photoRoot([img({ src: 'http://pbs.twimg.com/media/insecure.jpg' })]),
    photoRoot([img({ src: 'https://user:secret@pbs.twimg.com/media/credential.jpg' })]),
    photoRoot([background('https://user:secret@pbs.twimg.com/media/credential-background.jpg')]),
    photoRoot([img({ src: 'https://pbs.twimg.com:8443/media/port.jpg' })]),
    photoRoot([img({ src: photo + '?padding=' + 'a'.repeat(2100) })]),
    photoRoot([img({ src: 'https://pbs.twimg.com/media/tiny.jpg' }, { width: 100, height: 50 })]),
    photoRoot([background(photo, { width: 100, height: 50 })]),
  ] }));
  assert.deepEqual(block.media, []);
});

test('actual Windowed and Headless collectors invoke the same shared policy entry point', async () => {
  const f = fixture({ own: [photoRoot([background(photo)])] });
  const shared = f.context.AkuBoundedCapturePolicy;
  assert.equal(typeof shared.extractXDOMMedia, 'function');
  let calls = 0;
  const wrapped = { ...shared, extractXDOMMedia(...args) { calls++; return shared.extractXDOMMedia(...args); } };
  f.context.AkuBoundedCapturePolicy = wrapped;
  f.context.capturePolicy = wrapped;
  f.windowed();
  const afterWindowed = calls;
  await f.capture();
  assert.ok(afterWindowed > 0);
  assert.ok(calls > afterWindowed, 'Headless must invoke the same collector rather than duplicate CSS acquisition');
  const before = calls;
  f.context.AkuBoundedCapturePolicy = { ...shared, extractXDOMMedia: () => ({ media: [], observations: [] }) };
  const control = await f.capture();
  assert.equal(control.block.media.length, 0);
  assert.throws(() => assert.deepEqual(mediaContract(control.block.media), mediaContract(f.windowed())),
    { code: 'ERR_ASSERTION' }, 'equivalent-input comparison detects a missing shared Headless acquisition result');
  assert.ok(calls > before);
});

test('the executable X policy asset and recorded provenance use the exact shared collector bytes', async () => {
  const assets = await sourceAssets(fileURLToPath(bridge), 'x');
  const policy = assets.find(value => value.relative === 'AkuBridge/bounded-capture-policy.js');
  assert.ok(policy?.execute);
  assert.equal(policy.content, scripts['bounded-capture-policy.js']);
  assert.match(policy.content, /function extractXDOMMedia\(/);
  assert.equal(policy.sha256, createHash('sha256').update(policy.content).digest('hex'));
  assert.ok(sourceProvenance(assets).sources.some(value => value.path === policy.relative && value.sha256 === policy.sha256));
  assert.ok(assets.indexOf(policy) < assets.findIndex(value => value.relative === 'worker/vendor/x-extract.js'));
});
