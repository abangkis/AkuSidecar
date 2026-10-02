import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import { resolveStructuredMedia, structuredMediaRequest } from '../structured-media.mjs';
import { toObservation } from '../observation.mjs';
import { sourceAssets } from '../worker.mjs';
import { sourceProvenance } from '../provenance.mjs';
import { resolveXStructuredMediaInMainWorld } from '../../../../../../AkuBridge/x-main-world-media-resolver.js';
import { resolveFacebookStructuredMediaInMainWorld } from '../../../../../../AkuBridge/facebook-main-world-media-resolver.js';

const here = dirname(fileURLToPath(import.meta.url));
const bridgePath = resolve(here, '../../../../../../AkuBridge');
const xResolver = {
  available: true,
  functionSource: resolveXStructuredMediaInMainWorld.toString(),
  runtimeRevision: 'x-main-world-media-resolver-v1',
};
const facebookResolver = {
  available: true,
  functionSource: resolveFacebookStructuredMediaInMainWorld.toString(),
  runtimeRevision: 'facebook-main-world-media-resolver-v1',
};

function xArticle(id, poster, playback) {
  const permalink = `https://x.com/fixture/status/${id}`;
  const anchor = { href: permalink, getAttribute: () => permalink, closest: () => null };
  const article = {
    querySelectorAll(selector) {
      if (selector === 'time') return [];
      if (selector.includes('a[')) return [anchor];
      return [];
    },
  };
  Object.defineProperty(article, '__reactProps$fixture', {
    value: { tweet: { rest_id: id, extended_entities: { media: [{
      media_url_https: poster,
      video_info: { variants: [{ videoUrl: playback }] },
    }] } } },
  });
  return article;
}

function withDocument(document, action) {
  const previous = globalThis.document;
  globalThis.document = document;
  return Promise.resolve().then(action).finally(() => {
    if (previous === undefined) delete globalThis.document;
    else globalThis.document = previous;
  });
}

function fakePage(evaluate) {
  return {
    async evaluate(expression, timeoutMs) {
      assert.ok(timeoutMs > 0 && timeoutMs <= 1400);
      if (evaluate) return evaluate(expression, timeoutMs);
      return Function(`return ${expression}`)();
    },
  };
}

function xPost(id, posters) {
  return {
    id: `x:status:${id}`,
    permalink: `https://x.com/fixture/status/${id}`,
    author: 'Fixture author',
    text: 'Fixture video post',
    mediaExpected: ['video'],
    media: posters.map(url => ({ kind: 'video_poster', url, loaded: true })),
    mediaEvidence: { status: 'missing_expected_url', expectedWithoutUrl: ['video'] },
    limitations: ['visible_dom_only', 'video_stream_not_resolved'],
  };
}

function facebookScript(id, poster, playback) {
  return { textContent: JSON.stringify({
    id: `facebook:post:${id}`,
    first_frame_thumbnail: poster,
    videoDeliveryResponseFragment: {
      videoDeliveryResponseResult: { progressive_urls: [{ progressive_url: playback }] },
    },
  }) };
}

test('sourceAssets hashes the original Bridge resolver and keeps the derived function separate', async () => {
  const bridgeResolverPath = resolve(bridgePath, 'x-main-world-media-resolver.js');
  const original = await readFile(bridgeResolverPath);
  const expectedHash = createHash('sha256').update(original).digest('hex');
  const assets = await sourceAssets(bridgePath, 'x');
  const sourceAsset = assets.find(asset => asset.relative === 'AkuBridge/x-main-world-media-resolver.js');
  assert.equal(sourceAsset.sha256, expectedHash);
  assert.equal(sourceProvenance(assets).sources.find(asset => asset.path === sourceAsset.relative).sha256, expectedHash);
  assert.equal(assets.structuredMediaResolver.available, true);
  assert.equal(assets.structuredMediaResolver.sha256, expectedHash);
  assert.match(assets.structuredMediaResolver.functionSource, /^function resolveXStructuredMediaInMainWorld/);
  assert.ok(assets.some(asset => asset.relative === 'worker/structured-media.mjs'));
  assert.equal(sourceProvenance(assets).sources.some(asset => asset.path.includes('derived')), false);
  const facebookPath = resolve(bridgePath, 'facebook-main-world-media-resolver.js');
  const facebookHash = createHash('sha256').update(await readFile(facebookPath)).digest('hex');
  const facebookAssets = await sourceAssets(bridgePath, 'facebook');
  assert.equal(facebookAssets.structuredMediaResolver.available, true);
  assert.equal(facebookAssets.structuredMediaResolver.sha256, facebookHash);
  assert.equal(sourceProvenance(facebookAssets).sources.find(asset => asset.path === 'AkuBridge/facebook-main-world-media-resolver.js').sha256, facebookHash);
});

test('X resolver returns only the requested own ID and enriches matched DOM video posters', async () => {
  const ownId = '1890000000000000001';
  const foreignId = '1890000000000000002';
  const ownPoster = 'https://pbs.twimg.com/ext_tw_video_thumb/own/pu/img/frame.jpg?name=small';
  const playback = 'https://video.twimg.com/ext_tw_video/own/pu/vid/avc1/clip.mp4?tag=12';
  const foreignPoster = 'https://pbs.twimg.com/ext_tw_video_thumb/foreign/pu/img/frame.jpg';
  const ownArticle = xArticle(ownId, ownPoster, playback);
  const foreignArticle = xArticle(foreignId, foreignPoster,
    'https://video.twimg.com/ext_tw_video/foreign/pu/vid/avc1/clip.mp4');
  const document = { querySelectorAll: () => [ownArticle, foreignArticle] };

  await withDocument(document, async () => {
    const result = await resolveStructuredMedia({
      page: fakePage(), source: 'x', posts: [xPost(ownId, [ownPoster])], resolver: xResolver,
      deadlineAt: Date.now() + 2000,
    });
    assert.equal(result.summary.resolvedPosts, 1);
    assert.equal(result.posts[0].media[0].kind, 'video');
    assert.equal(result.posts[0].media[0].url, ownPoster);
    assert.equal(result.posts[0].media[0].playbackUrl, playback);
    assert.equal(result.posts[0].mediaEvidence.structuredMediaResolution.status, 'resolved');
  });
});

test('X mixed posters stay unresolved, while fully accounted owned playback keeps quality unverified', async () => {
  const id = '1890000000000000003';
  const poster = 'https://pbs.twimg.com/amplify_video_thumb/own/img/frame.jpg';
  const playback = 'https://video.twimg.com/amplify_video/own/vid/avc1/clip.mp4';
  const article = xArticle(id, poster, playback);
  await withDocument({ querySelectorAll: () => [article] }, async () => {
    const mixed = await resolveStructuredMedia({
      page: fakePage(), source: 'x', posts: [xPost(id, [poster, 'https://pbs.twimg.com/ext_tw_video_thumb/unknown/img/other.jpg'])],
      resolver: xResolver, deadlineAt: Date.now() + 2000,
    });
    assert.equal(mixed.posts[0].mediaEvidence.structuredMediaResolution.status, 'partial');
    assert.equal(mixed.posts[0].media.filter(item => item.kind === 'video_poster').length, 1);
    const mixedBlock = toObservation({ source: 'x', requestedUrl: 'https://x.com/home', snapshots: [{ posts: mixed.posts }],
      provenance: {}, capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture' }).snapshots[0].blocks[0];
    assert.equal(mixedBlock.mediaRecovery.unknownVideo, 'unresolved');

    const complete = await resolveStructuredMedia({
      page: fakePage(), source: 'x', posts: [xPost(id, [poster])], resolver: xResolver,
      deadlineAt: Date.now() + 2000,
    });
    const block = toObservation({ source: 'x', requestedUrl: 'https://x.com/home', snapshots: [{ posts: complete.posts }],
      provenance: {}, capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture' }).snapshots[0].blocks[0];
    assert.equal(block.mediaRecovery.status, 'resolved');
    assert.equal(block.mediaRecovery.outcome, 'verified_owned_playback');
    assert.equal(block.captureQuality.status, 'unverified');
    const wrongSource = structuredClone(complete.posts[0]);
    wrongSource.mediaEvidence.structuredMediaResolution.resolverVersion = 'facebook-structured-video-v1';
    const rejected = toObservation({ source: 'x', requestedUrl: 'https://x.com/home', snapshots: [{ posts: [wrongSource] }],
      provenance: {}, capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture' }).snapshots[0].blocks[0];
    assert.equal(rejected.mediaRecovery.unknownVideo, 'unresolved');
  });
});

test('Facebook resolver upgrades only an exact own image poster and preserves other images', async () => {
  const id = '123456789012345';
  const poster = 'https://scontent.xx.fbcdn.net/v/t39.30808-6/12345_n.jpg?stp=dst-jpg_s320x320';
  const playback = 'https://video.xx.fbcdn.net/o1/v/t2/f2/m412/fixture.mp4?ccb=9';
  const foreign = facebookScript('123456789012346', 'https://scontent.xx.fbcdn.net/foreign/frame.jpg',
    'https://video.xx.fbcdn.net/foreign/clip.mp4');
  const own = facebookScript(id, poster, playback);
  const pageDocument = { querySelectorAll: () => [own, foreign] };
  const post = {
    id: `facebook:post:${id}`,
    permalink: `https://www.facebook.com/watch/?v=${id}`,
    author: 'Fixture author', text: 'Fixture video post', mediaExpected: ['video'],
    media: [
      { kind: 'image', url: poster, loaded: true },
      { kind: 'image', url: 'https://scontent.xx.fbcdn.net/other/photo.jpg', loaded: true },
    ],
    mediaEvidence: { status: 'missing_expected_url', expectedWithoutUrl: ['video'] },
    limitations: ['visible_dom_only', 'no_structured_media_resolver', 'video_stream_not_resolved'],
  };
  await withDocument(pageDocument, async () => {
    const result = await resolveStructuredMedia({
      page: fakePage(), source: 'facebook', posts: [post], resolver: facebookResolver,
      deadlineAt: Date.now() + 2000,
    });
    assert.equal(result.posts[0].media[0].kind, 'video');
    assert.equal(result.posts[0].media[0].url, poster);
    assert.equal(result.posts[0].media[0].playbackUrl, playback);
    assert.equal(result.posts[0].media[1].kind, 'image');
    assert.equal(result.posts[0].mediaEvidence.structuredMediaResolution.status, 'resolved');
    assert.deepEqual(result.posts[0].limitations, ['visible_dom_only']);
  });
});

test('unsafe or missing Facebook media remains unknown and is never synthesized', async () => {
  const id = '123456789012347';
  const poster = 'https://scontent.xx.fbcdn.net/fixture/frame.jpg';
  const unsafe = facebookScript(id, poster, 'https://evil.example/fixture.mp4');
  const post = {
    id: `facebook:post:${id}`, permalink: `https://www.facebook.com/watch/?v=${id}`,
    author: 'Fixture author', text: 'Fixture video post', mediaExpected: ['video'],
    media: [{ kind: 'video_poster', url: poster }], mediaEvidence: {},
  };
  await withDocument({ querySelectorAll: () => [unsafe] }, async () => {
    const result = await resolveStructuredMedia({ page: fakePage(), source: 'facebook', posts: [post],
      resolver: facebookResolver, deadlineAt: Date.now() + 2000 });
    assert.equal(result.posts[0].media[0].kind, 'video_poster');
    assert.equal(result.posts[0].mediaEvidence.structuredMediaResolution.status, 'no_match');
  });
  await withDocument({ querySelectorAll: () => [] }, async () => {
    const result = await resolveStructuredMedia({ page: fakePage(), source: 'facebook', posts: [post],
      resolver: facebookResolver, deadlineAt: Date.now() + 2000 });
    assert.equal(result.posts[0].media.length, 1);
    assert.equal(result.posts[0].media[0].kind, 'video_poster');
  });
});

test('exact native Facebook video URLs retain video expectation when DOM exposes an image only', async () => {
  const id='123456789012349';
  const post={id:`facebook:post:${id}`,permalink:`https://www.facebook.com/watch/?v=${id}`,
    author:'Fixture',text:'Video shell',mediaExpected:['image'],media:[{kind:'image',url:'https://scontent.xx.fbcdn.net/fixture/frame.jpg'}]};
  const result=await resolveStructuredMedia({page:fakePage(),source:'facebook',posts:[post],resolver:null,deadlineAt:Date.now()+2000});
  assert.deepEqual(result.posts[0].mediaExpected,['image','video']);
  assert.equal(result.posts[0].mediaEvidence.nativeVideoPermalink,true);
  assert.equal(result.posts[0].media[0].kind,'image');
  const block=toObservation({source:'facebook',requestedUrl:'https://www.facebook.com/',snapshots:[{posts:result.posts}],
    provenance:{},capturedAt:'2026-10-02T00:00:00.000Z',stopReason:'fixture'}).snapshots[0].blocks[0];
  assert.equal(block.mediaRecovery.unknownVideo,'unresolved');
  assert.equal(block.captureQuality.status,'unverified');
  const alias={...post,id:'facebook:post:123456789012350'};
  assert.equal(structuredMediaRequest('facebook',[alias]).request.candidateIds.length,0);
  const imagePost={...post,permalink:`https://www.facebook.com/fixture/posts/${id}`};
  assert.equal(structuredMediaRequest('facebook',[imagePost]).request.candidateIds.length,0);
});

test('duplicate candidate bindings fail closed and more than sixteen requested IDs are bounded', async () => {
  const id = '1890000000000000004';
  const poster = 'https://pbs.twimg.com/ext_tw_video_thumb/own/pu/img/frame.jpg';
  const playback = 'https://video.twimg.com/ext_tw_video/own/pu/vid/avc1/clip.mp4';
  const duplicate = {
    runtimeRevision: xResolver.runtimeRevision,
    resolverVersion: 'x-main-world-structured-v1',
    candidates: [
      { candidateId: `x:status:${id}`, media: [{ kind: 'video', url: poster, posterUrl: poster, playbackUrl: playback }] },
      { candidateId: `x:status:${id}`, media: [{ kind: 'video', url: poster, posterUrl: poster, playbackUrl: playback }] },
    ],
  };
  const result = await resolveStructuredMedia({ page: fakePage(() => duplicate), source: 'x', posts: [xPost(id, [poster])],
    resolver: xResolver, deadlineAt: Date.now() + 2000 });
  assert.equal(result.posts[0].media[0].kind, 'video_poster');
  assert.equal(result.posts[0].mediaEvidence.structuredMediaResolution.status, 'ambiguous');

  const many = Array.from({ length: 18 }, (_, index) => {
    const postId = `189000000000000${String(index).padStart(4, '0')}`;
    return xPost(postId, [`https://pbs.twimg.com/ext_tw_video_thumb/${index}/pu/img/frame.jpg`]);
  });
  const built = structuredMediaRequest('x', many);
  assert.equal(built.request.candidateIds.length, 16);
  assert.equal(built.bounded, true);
  assert.equal(built.request.maxTraversalNodes, 1500);
  assert.equal(built.request.maxDepth, 16);
});

test('unavailable optional resolver leaves posts intact and records unavailability', async () => {
  const id = '1890000000000000005';
  const post = xPost(id, ['https://pbs.twimg.com/ext_tw_video_thumb/own/pu/img/frame.jpg']);
  const result = await resolveStructuredMedia({ page: fakePage(), source: 'x', posts: [post], resolver: null,
    deadlineAt: Date.now() + 2000 });
  assert.equal(result.posts[0].media[0].kind, 'video_poster');
  assert.equal(result.posts[0].mediaEvidence.structuredMediaResolution.status, 'unavailable');
  assert.equal(result.summary.available, false);
  const observation = toObservation({ source: 'x', requestedUrl: 'https://x.com/home',
    snapshots: [{ posts: result.posts, structuredMediaResolution: result.summary }], provenance: {},
    capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture' });
  const diagnostics = observation.coverage.structuredMediaResolution.snapshots[0];
  assert.equal(diagnostics.returnedExactCandidateCount, null);
  assert.equal(diagnostics.resolverBounded, null);
  assert.equal(diagnostics.ownSafePairCount, null);
  assert.equal(diagnostics.domVideoPosterCount, 1);
});

test('aggregate diagnostics distinguish absent candidates, unsafe pairs, and DOM poster path mismatch', async () => {
  const id = '1890000000000000006';
  const post = xPost(id, ['https://pbs.twimg.com/ext_tw_video_thumb/dom/pu/img/frame.jpg']);
  const diagnostics = { candidateCount: 0, traversedNodeCount: 19, matchedStructuredNodeCount: 0, bounded: false };
  const noCandidate = await resolveStructuredMedia({ page: fakePage(() => ({
    runtimeRevision: xResolver.runtimeRevision, resolverVersion: 'x-main-world-structured-v1', candidates: [], diagnostics,
  })), source: 'x', posts: [post], resolver: xResolver, deadlineAt: Date.now() + 2000 });
  assert.equal(noCandidate.summary.noExactReturnedCandidateCount, 1);
  assert.equal(noCandidate.summary.resolverNoSafePairCandidateCount, 0);
  assert.equal(noCandidate.summary.resolverTraversedNodeCount, 19);
  assert.equal(noCandidate.summary.resolverMatchedStructuredNodeCount, 0);
  assert.equal(noCandidate.summary.resolverBounded, false);
  assert.equal(noCandidate.summary.returnedCandidates, 0);

  const candidateId = `x:status:${id}`;
  const noSafePair = await resolveStructuredMedia({ page: fakePage(() => ({
    runtimeRevision: xResolver.runtimeRevision, resolverVersion: 'x-main-world-structured-v1',
    candidates: [{ candidateId, media: [{ kind: 'video', url: 'https://video.twimg.com/ext_tw_video/own/clip.mp4',
      playbackUrl: 'https://video.twimg.com/ext_tw_video/own/clip.mp4', posterUrl: null }] }],
    diagnostics: { candidateCount: 1, traversedNodeCount: 23, matchedStructuredNodeCount: 1, bounded: false },
  })), source: 'x', posts: [post], resolver: xResolver, deadlineAt: Date.now() + 2000 });
  assert.equal(noSafePair.summary.noExactReturnedCandidateCount, 0);
  assert.equal(noSafePair.summary.resolverNoSafePairCandidateCount, 1);
  assert.equal(noSafePair.summary.ownSafePairCount, 0);
  assert.equal(noSafePair.posts[0].mediaEvidence.structuredMediaResolution.status, 'no_match');

  const poster = 'https://pbs.twimg.com/ext_tw_video_thumb/structured/pu/img/frame.jpg';
  const playback = 'https://video.twimg.com/ext_tw_video/structured/pu/vid/clip.mp4';
  const mismatch = await resolveStructuredMedia({ page: fakePage(() => ({
    runtimeRevision: xResolver.runtimeRevision, resolverVersion: 'x-main-world-structured-v1',
    candidates: [{ candidateId, media: [{ kind: 'video', url: poster, posterUrl: poster, playbackUrl: playback }] }],
    diagnostics: { candidateCount: 1, traversedNodeCount: 31, matchedStructuredNodeCount: 2, bounded: true },
  })), source: 'x', posts: [post], resolver: xResolver, deadlineAt: Date.now() + 2000 });
  assert.equal(mismatch.summary.resolverNoSafePairCandidateCount, 0);
  assert.equal(mismatch.summary.ownSafePairCount, 1);
  assert.equal(mismatch.summary.domVideoPosterCount, 1);
  assert.equal(mismatch.summary.matchedPosterPathCount, 0);
  assert.equal(mismatch.summary.resolverDomVideoPosterPathMismatchCount, 1);
  assert.equal(mismatch.summary.resolverBounded, true);
  assert.equal(mismatch.posts[0].mediaEvidence.structuredMediaResolution.status, 'partial');

  const observation = toObservation({ source: 'x', requestedUrl: 'https://x.com/home',
    snapshots: [{ posts: mismatch.posts, structuredMediaResolution: mismatch.summary }], provenance: {},
    capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture' });
  const coverage = observation.coverage.structuredMediaResolution;
  assert.equal(coverage.schema, 'aku.headless-structured-media-diagnostics.v1');
  assert.equal(coverage.snapshots[0].resolverDomVideoPosterPathMismatchCount, 1);
  assert.equal(coverage.snapshots[0].resolverTraversedNodeCount, 31);
  assert.equal(JSON.stringify(coverage).includes(id), false);
  assert.equal(JSON.stringify(coverage).includes('pbs.twimg.com'), false);
  assert.equal(JSON.stringify(coverage).includes('Fixture author'), false);

  const capped = toObservation({ source: 'x', requestedUrl: 'https://x.com/home',
    snapshots: Array.from({ length: 9 }, () => ({ posts: [], structuredMediaResolution: mismatch.summary })),
    provenance: {}, capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture' }).coverage.structuredMediaResolution;
  assert.equal(capped.snapshotCount, 9);
  assert.equal(capped.includedSnapshotCount, 8);
  assert.equal(capped.truncated, true);
  assert.equal(capped.snapshots.length, 8);
});
