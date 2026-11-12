import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { sourceAssets } from '../worker.mjs';
import { sourceProvenance } from '../provenance.mjs';
import { toObservation } from '../observation.mjs';
import { resolveStructuredMedia } from '../structured-media.mjs';
import { captureIsolated, NATIVE_CAPTURE_CAPABILITIES } from '../capture.mjs';

const bridge = new URL('../../../../../../AkuBridge/', import.meta.url);
const bridgePath = fileURLToPath(bridge);
const scripts = Object.fromEntries(await Promise.all([
  'capture-primitives.js', 'bounded-capture-policy.js', 'source-adapter-runtime.js',
  'adapters/x-adapter.js', 'content-script.js',
].map(async name => [name, await readFile(new URL(name, bridge), 'utf8')])));
const extractor = await readFile(new URL('../vendor/x-extract.js', import.meta.url), 'utf8');
const quoteIdentity = await readFile(new URL('../vendor/x-quote-identity.js', import.meta.url), 'utf8');
const ownID = '2106867448746021278';
const quoteID = '2106867448746021279';
const permalink = `https://x.com/owner/status/${ownID}`;
const poster = 'https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/own.jpg';
const quotePoster = 'https://pbs.twimg.com/ext_tw_video_thumb/67890/pu/img/quote.jpg';
const master = 'https://video.twimg.com/ext_tw_video/12345/pu/pl/master.m3u8?tag=12';
const quoteMaster = 'https://video.twimg.com/ext_tw_video/67890/pu/pl/master.m3u8';
const mp4 = 'https://video.twimg.com/ext_tw_video/12345/pu/vid/720x720/own.mp4';
const capturedAt = '2026-11-12T00:00:00.000Z';
const plain = value => JSON.parse(JSON.stringify(value));
const entry = (name, responseStatus = 200) => ({ name, responseStatus });

function video(posterUrl, direct = 'blob:https://x.com/owned-video', { sources = [] } = {}) {
  return {
    tagName: 'VIDEO', poster: posterUrl, currentSrc: direct, src: '',
    videoWidth: 640, videoHeight: 360, naturalWidth: 640, naturalHeight: 360,
    readyState: 1, paused: true, currentTime: 0, duration: 10,
    getAttribute: name => name === 'poster' ? posterUrl : null,
    querySelectorAll: selector => selector === 'source[src]' ? sources.map(src => ({ src })) : [],
    getBoundingClientRect: () => ({ width: 640, height: 360, top: 0, bottom: 360 }),
    closest: () => null,
  };
}

// Identity/discovery/text are controlled seams. Acquisition uses the real X adapter's
// media selectors/rendering, actual Windowed findMedia and actual Headless extractor.
function fixture({ ownVideos = [video(poster)], quoteVideos = [], resources = [], retainQuote = false } = {}) {
  const text = 'Owned fixture text with a video, independent of playback availability.';
  const textRoot = { nodeType: 1, tagName: 'DIV', innerText: text,
    childNodes: [{ nodeType: 3, nodeValue: text }], querySelectorAll: () => [] };
  const time = { dateTime: capturedAt, closest: () => ({ href: permalink }) };
  const quote = quoteVideos.length ? {
    contains: node => quoteVideos.includes(node),
    querySelectorAll: selector => selector === 'video' ? quoteVideos : [],
    querySelector: () => null,
    getBoundingClientRect: () => ({ width: 640, height: 360, top: 0, bottom: 360 }),
  } : null;
  const container = {
    querySelectorAll: selector => selector === 'video' ? [...ownVideos, ...quoteVideos]
      : selector === 'time' ? [time] : selector === 'text' ? [textRoot] : [],
    querySelector: selector => selector === 'text' ? textRoot : null,
    contains: node => ownVideos.includes(node) || quoteVideos.includes(node) || node === textRoot,
    getBoundingClientRect: () => ({ width: 640, height: 360, top: 0, bottom: 360 }),
  };
  const location = { href: 'https://x.com/home', hostname: 'x.com', pathname: '/home' };
  const document = {
    querySelector: () => null, querySelectorAll: () => [],
    readyState: 'complete', visibilityState: 'visible', title: 'Owned media fixture',
    scrollingElement: { scrollHeight: 1000 }, body: { innerText: '', childNodes: [] },
  };
  const context = vm.createContext({ URL, setTimeout, clearTimeout, location, document,
    innerHeight: 900, innerWidth: 1200, scrollY: 0,
    performance: { getEntriesByType: kind => kind === 'resource' ? resources : [] },
    getComputedStyle: () => ({ backgroundImage: 'none' }),
  });
  context.window = context;
  for (const name of ['capture-primitives.js', 'bounded-capture-policy.js',
    'source-adapter-runtime.js', 'adapters/x-adapter.js']) vm.runInContext(scripts[name], context);
  const real = context.AkuSourceAdapters.get('x');
  const adapter = { ...real,
    version: real.version,
    discoverCandidates: () => ({ candidates: [container], strategy: 'controlled_owned_post' }),
    findQuotedRoot: () => quote, resolveIdentity: () => undefined,
    findAuthor: () => 'Owner', findAvatar: () => null,
    contentRootSelector: 'text', extractText: () => text,
    extractSemantics: () => ({ contentKind: 'video', relationshipType: 'original' }),
    extractPresentation: () => ({}),
    extractQuotedPost: (_, { findMedia }) => retainQuote && quote ? {
      author: 'Quoted owner', text: 'Quoted fixture text',
      permalink: `https://x.com/quoted/status/${quoteID}`, media: findMedia(quote),
    } : null,
    contentExpansion: {}, loginRequired: () => false, feedRootPresent: () => true,
    mediaAcquisition: { ...real.mediaAcquisition,
      detectExpectedKinds: root => root.querySelectorAll('video').length ? ['video'] : [] },
  };
  const adapters = { get: source => source === 'x' ? adapter : null };
  context.AkuSourceAdapters = adapters;
  context.sourceAdapters = adapters;
  context.capturePolicy = context.AkuBoundedCapturePolicy;
  context.normalizeHttpUrl = value => {
    try { const url = new URL(value); return /^https?:$/.test(url.protocol) ? url.href : null; }
    catch { return null; }
  };
  context.renderedBackgroundUrl = () => null;
  context.readImageCandidate = image => ({ url: image.currentSrc || image.src, source: 'current_src' });
  const start = scripts['content-script.js'].indexOf('  function findMedia(');
  const end = scripts['content-script.js'].indexOf('\n  function findPermalink(', start);
  assert.ok(start > 0 && end > start, 'actual Windowed findMedia boundary must exist');
  vm.runInContext(scripts['content-script.js'].slice(start, end), context);
  vm.runInContext(quoteIdentity, context);
  vm.runInContext(extractor, context);
  return { context, adapter, container, quote, location,
    windowed: () => plain(context.findMedia(container, 'x', { excludeRoot: quote })),
    snapshot: async () => plain(await context.XHeadlessPoC.collect()),
  };
}

function observation(snapshot, policy) {
  return plain(toObservation({ source: 'x', requestedUrl: 'https://x.com/home',
    snapshots: [snapshot], capturedAt, stopReason: 'completed', performedScrolls: 0,
    scrollStopReason: 'budget', provenance: {}, xMediaPolicy: policy }));
}

function mediaURLs(values) {
  return values.map(value => ({ posterUrl: value.posterUrl || value.url,
    playbackUrl: value.playbackUrl || null, playbackMode: value.playbackMode || 'native' }));
}

test('actual Windowed and Headless acquisition preserve the same owned primary HLS master through Observation JSON', async () => {
  const f = fixture({ resources: [entry(master)] });
  const snapshot = await f.snapshot();
  assert.equal(snapshot.posts[0].media[0].kind, 'video_poster', 'internal poster remains eligible for MP4 enrichment');
  const block = observation(snapshot, f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
  assert.equal(block.media[0].kind, 'video');
  assert.deepEqual(mediaURLs(block.media), mediaURLs(f.windowed()));
  assert.equal(block.media[0].playbackUrl, master);
  assert.equal(block.captureQuality.status, 'unverified');
  assert.equal(block.mediaRecovery.outcome, 'owned_playback_url_observed');
  assert.equal(block.mediaRecovery.limitation, 'video_playback_unverified');
});

test('the same fixture detects the acquisition gap when the shared playback resolver is absent', async () => {
  const f = fixture({ resources: [entry(master)] });
  const policy = f.context.AkuBoundedCapturePolicy;
  // Reproduce the previous missing shared-policy composition in memory only.
  f.context.AkuBoundedCapturePolicy = { ...policy, xPlaybackUrl: undefined };
  const snapshot = await f.snapshot();
  const block = observation(snapshot, policy).snapshots[0].blocks[0];
  assert.equal(f.windowed()[0].playbackUrl, master);
  assert.equal(block.media[0].playbackUrl, undefined);
  assert.throws(() => assert.deepEqual(mediaURLs(block.media), mediaURLs(f.windowed())),
    { code: 'ERR_ASSERTION' });
});

test('progressive DOM MP4 takes precedence over an observed HLS master in both consumers', async () => {
  for (const ownVideo of [video(poster, mp4), video(poster, 'blob:https://x.com/owned-video', { sources: [mp4] })]) {
    const f = fixture({ ownVideos: [ownVideo], resources: [entry(master)] });
    const block = observation(await f.snapshot(), f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
    assert.deepEqual(mediaURLs(block.media), mediaURLs(f.windowed()));
    assert.equal(block.media[0].playbackUrl, mp4);
  }
});

test('structured own MP4 can still upgrade an internal HLS poster before final projection', async () => {
  const f = fixture({ resources: [entry(master)] });
  const snapshot = await f.snapshot();
  const resolved = await resolveStructuredMedia({ source: 'x', posts: snapshot.posts,
    page: { evaluate: async () => ({ runtimeRevision: 'x-main-world-media-resolver-v1',
      candidates: [{ candidateId: `x:status:${ownID}`, media: [{ kind: 'video', url: poster,
        posterUrl: poster, playbackUrl: mp4, playbackMode: 'inline', width: 640, height: 360 }] }] }) },
    resolver: { available: true, functionSource: '() => null', runtimeRevision: 'x-main-world-media-resolver-v1' },
    deadlineAt: Date.now() + 1000 });
  const block = observation({ ...snapshot, posts: resolved.posts }, f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
  assert.equal(block.media[0].playbackUrl, mp4);
  assert.equal(block.media[0].kind, 'video');
});

test('quote posters and resource entries cannot become primary playback evidence', async () => {
  const f = fixture({ quoteVideos: [video(quotePoster)], resources: [entry(quoteMaster), entry(master)] });
  const snapshot = await f.snapshot();
  const block = observation(snapshot, f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
  assert.equal(block.media.length, 1);
  assert.equal(block.media[0].posterUrl || block.media[0].url, poster);
  assert.equal(block.media[0].playbackUrl, master);
  assert.deepEqual(mediaURLs(block.media), mediaURLs(f.windowed()));
  assert.equal(JSON.stringify(block.media).includes(quotePoster), false);
  const quoteOnly = fixture({ ownVideos: [], quoteVideos: [video(quotePoster)], resources: [entry(quoteMaster)] });
  assert.deepEqual(observation(await quoteOnly.snapshot(), quoteOnly.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0].media, []);
  assert.deepEqual(quoteOnly.windowed(), []);
});

test('retained quote media uses its own poster/stream pair and cannot inherit primary playback', async () => {
  const f = fixture({ quoteVideos: [video(quotePoster)], retainQuote: true,
    resources: [entry(master), entry(quoteMaster)] });
  const block = observation(await f.snapshot(), f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
  const windowedQuote = plain(f.context.findMedia(f.quote, 'x'));
  assert.equal(block.media[0].playbackUrl, master);
  assert.equal(block.quotedPost.media[0].playbackUrl, quoteMaster);
  assert.deepEqual(mediaURLs(block.quotedPost.media), mediaURLs(windowedQuote));
  assert.equal(block.quotedPost.permalink, `https://x.com/quoted/status/${quoteID}`);
  const primaryOnlyStream = fixture({ quoteVideos: [video(quotePoster)], retainQuote: true,
    resources: [entry(master)] });
  const isolated = observation(await primaryOnlyStream.snapshot(), primaryOnlyStream.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
  assert.equal(isolated.media[0].playbackUrl, master);
  assert.ok(isolated.quotedPost.media.every(value => !value.playbackUrl));
});

test('variant-only, denied, foreign, wrong-video and unsafe playlist evidence leaves a truthful native poster', async t => {
  const examples = {
    'variant only': [entry('https://video.twimg.com/ext_tw_video/12345/pu/pl/avc1/variant.m3u8')],
    'HTTP 403': [entry(master, 403)],
    'HTTP 500': [entry(master, 500)],
    'no observed stream': [],
    'foreign host': [entry('https://video.twimg.com.evil.example/ext_tw_video/12345/pu/pl/master.m3u8')],
    'quote video ID': [entry(quoteMaster)],
    'wrong video family': [entry('https://video.twimg.com/amplify_video/12345/pu/pl/master.m3u8')],
    credentials: [entry(master.replace('https://', 'https://user:secret@'))],
    'encoded slash': [entry('https://video.twimg.com/ext_tw_video/12345/pu%2fpl/master.m3u8')],
    'encoded backslash': [entry('https://video.twimg.com/ext_tw_video/12345/pu%5cpl/master.m3u8')],
    'overlong URL': [entry(master + '&padding=' + 'a'.repeat(2100))],
  };
  for (const [name, resources] of Object.entries(examples)) await t.test(name, async () => {
    const f = fixture({ resources });
    const block = observation(await f.snapshot(), f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
    assert.deepEqual(mediaURLs(block.media), mediaURLs(f.windowed()));
    assert.ok(block.media.every(value => !value.playbackUrl));
    assert.notEqual(block.mediaRecovery.outcome, 'owned_playback_url_observed');
    assert.notEqual(block.mediaRecovery.outcome, 'verified_owned_playback');
  });
});

test('projection validates the poster/playlist pair again instead of trusting an injected playback field', async () => {
  for (const playbackUrl of [quoteMaster, master.replace('https://', 'https://user:secret@'), master + '&padding=' + 'a'.repeat(2100)]) {
    const f = fixture();
    const snapshot = await f.snapshot();
    snapshot.posts[0].media[0].playbackUrl = playbackUrl;
    snapshot.posts[0].media[0].playbackMode = 'inline';
    const block = observation(snapshot, f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
    assert.ok(block.media.every(value => !value.playbackUrl));
  }
});

test('unsafe or unrelated posters cannot acquire an HLS stream solely from page resources', async () => {
  for (const posterUrl of [
    quotePoster,
    poster.replace('https://', 'https://user:secret@'),
    'https://pbs.twimg.com/ext_tw_video_thumb/12345/pu%2fimg/own.jpg',
    poster + '?padding=' + 'a'.repeat(2100),
  ]) {
    const f = fixture({ ownVideos: [video(posterUrl)], resources: [entry(master)] });
    const block = observation(await f.snapshot(), f.context.AkuBoundedCapturePolicy).snapshots[0].blocks[0];
    assert.ok(block.media.every(value => !value.playbackUrl));
    assert.ok(f.windowed().every(value => !value.playbackUrl));
  }
});

test('shared direct-MP4 admission still requires a trusted bounded DOM poster', () => {
  const policy = fixture().context.AkuBoundedCapturePolicy;
  assert.equal(policy.xPlaybackUrl(poster, mp4), mp4);
  for (const value of [poster.replace('https://', 'https://user:secret@'),
    poster.replace('pbs.twimg.com', 'pbs.twimg.com.evil.example'),
    poster + '?padding=' + 'a'.repeat(2100)]) {
    assert.equal(policy.xPlaybackUrl(value, mp4), null);
  }
});

test('real X source assets inject the identical bounded policy and record its content hash', async () => {
  const assets = await sourceAssets(bridgePath, 'x');
  const policy = assets.find(value => value.relative === 'AkuBridge/bounded-capture-policy.js');
  assert.ok(policy, 'X source composition must stage the shared media policy');
  assert.equal(policy.execute, true);
  assert.equal(policy.content, scripts['bounded-capture-policy.js']);
  assert.equal(policy.sha256, createHash('sha256').update(policy.content).digest('hex'));
  assert.ok(assets.indexOf(policy) < assets.findIndex(value => value.relative === 'worker/vendor/x-extract.js'));
  assert.ok(JSON.stringify(sourceProvenance(assets)).includes(policy.sha256));
});

test('native-capability capture composition consumes the real staged policy without granting expansion or navigation authority', async () => {
  const f = fixture({ resources: [entry(master)] });
  const assets = await sourceAssets(bridgePath, 'x');
  const staged = [];
  const page = {
    navigate: async url => { f.location.href = url; return {}; },
    send: async () => ({}),
    evaluate: async expression => {
      const asset = assets.find(value => value.execute && value.content === expression);
      if (asset) {
        staged.push(asset.relative);
        // The fixture already loads the actual acquisition scripts. Retain the
        // controlled identity/discovery seam when the composition injects assets.
        return undefined;
      }
      if (expression.includes('AkuXResponseMediaEvidence') || expression.includes('AKU_X_RESPONSE_MEDIA_EVIDENCE')) return null;
      if (expression.includes('resolveXStructuredMediaInMainWorld')) return { runtimeRevision: 'x-main-world-media-resolver-v1', candidates: [] };
      return vm.runInContext(expression, f.context);
    },
  };
  const browser = { backend: 'headless_worker', captureCapabilities: NATIVE_CAPTURE_CAPABILITIES.x,
    forSource: async source => { assert.equal(source, 'x'); return page; } };
  const result = plain(await captureIsolated(browser, { x: assets }, 'x', {
    acquisitionRound: 1, scrolls: 0, captureTimeoutMs: 1500, sourceHydrationTimeoutMs: 1000,
    pendingContentPolicy: 'reveal_if_present', sameTabMutationAllowed: true,
  }));
  assert.ok(staged.includes('AkuBridge/bounded-capture-policy.js'));
  assert.equal(result.snapshots[0].blocks[0].media[0].playbackUrl, master);
  assert.equal(f.context.AkuHeadlessCapturePolicy.allowContentExpansion, false);
  assert.equal(browser.createTemporaryPage, undefined);
  assert.equal(result.snapshots[0].blocks[0].mediaRecovery.outcome, 'owned_playback_url_observed');
});
