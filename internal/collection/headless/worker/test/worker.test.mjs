import test from 'node:test';
import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { createHash } from 'node:crypto';
import { canonicalSourceURL, evidenceKey, toObservation } from '../observation.mjs';
import { capture, continuationMatches, validateCapture } from '../capture.mjs';
import { runWorker, validateInit, validateRequest } from '../worker.mjs';

test('accepts only canonical native X and Facebook recapture URLs', () => {
  assert.equal(canonicalSourceURL('x', 'https://x.com/example/status/1890000000000000000/photo/1?x=1'),
    'https://x.com/example/status/1890000000000000000');
  assert.equal(canonicalSourceURL('x', 'https://evil.example/example/status/1890000000000000000'), null);
  assert.equal(canonicalSourceURL('facebook', 'https://www.facebook.com/example/posts/12345'),
    'https://www.facebook.com/example/posts/12345');
  assert.equal(canonicalSourceURL('facebook', 'https://www.facebook.com/story.php?id=1'), null);
  assert.equal(canonicalSourceURL('facebook', 'https://attacker.example/example/posts/12345'), null);
  assert.equal(validateCapture('x', {}).pageUrl, 'https://x.com/home');
  assert.throws(() => validateCapture('x', { pageUrl: 'https://example.com/post/1' }), { code: 'invalid_page_url' });
  assert.throws(() => validateCapture('facebook', { pageUrl: 'https://www.facebook.com/' }), { code: 'invalid_page_url' });
});

test('canonicalizes exact Facebook watch and video.php URLs with bounded numeric identities', () => {
  const canonical = 'https://www.facebook.com/watch/?v=12345678901234567890';
  for (const raw of [
    'https://www.facebook.com/watch/?v=12345678901234567890',
    'https://facebook.com/video.php?v=12345678901234567890&ref=watch',
    'https://m.facebook.com/watch/?v=12345678901234567890',
  ]) assert.equal(canonicalSourceURL('facebook', raw), canonical);

  const invalid = [
    'https://attacker.example/watch/?v=123',
    'http://www.facebook.com/watch/?v=123',
    'https://user@www.facebook.com/watch/?v=123',
    'https://www.facebook.com:8443/watch/?v=123',
    'https://www.facebook.com/watch/',
    'https://www.facebook.com/video.php?v=not-numeric',
    'https://www.facebook.com/watch/?v=123&v=456',
    `https://www.facebook.com/watch/?v=${'1'.repeat(33)}`,
  ];
  for (const raw of invalid) assert.equal(canonicalSourceURL('facebook', raw), null, raw);
  assert.equal(canonicalSourceURL('facebook', 'https://www.facebook.com/watch?v=123'), null);
});

test('requires a Facebook watch permalink ID to match its numeric v identity', () => {
  const post = {
    id: 'facebook:post:12345',
    permalink: 'https://www.facebook.com/video.php?v=12345',
    author: 'Example Page',
    text: 'A bounded Facebook video post.',
  };
  const observation = toObservation({source: 'facebook', requestedUrl: post.permalink,
    snapshots: [{posts: [post]}], provenance: {}, capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture'});
  assert.equal(observation.snapshots[0].blocks[0].platformId, post.id);
  assert.equal(observation.snapshots[0].blocks[0].permalink, post.permalink);
  assert.throws(() => toObservation({source: 'facebook', requestedUrl: post.permalink,
    snapshots: [{posts: [{...post, id: 'facebook:post:54321'}]}], provenance: {},
    capturedAt: '2026-10-02T00:00:00.000Z', stopReason: 'fixture'}), {code: 'invalid_observation'});
});

test('validates Bridge production capture fields and enforces the worker deadline ceiling', () => {
  const first = validateCapture('facebook', {
    mode: 'catch_up', sourceHydrationTimeoutMs: 29000, scrolls: 3, scrollFraction: 0.75,
    scrollSettleMs: 900, captureTimeoutMs: 45000, pendingContentPolicy: 'reveal_if_present',
    sameTabMutationAllowed: true, sourceFreshnessPolicy: 'wake_and_reveal', maxBlocksPerSnapshot: 20,
    restoreScroll: true, acquisitionRound: 1, continuation: null,
  });
  assert.equal(first.pageUrl, 'https://www.facebook.com/');
  assert.equal(first.hydrationMs, 29000);
  assert.equal(first.scrolls, 3);
  assert.equal(first.captureTimeoutMs, 45000);
  assert.equal(first.maxBlocksPerSnapshot, 20);
  const next = validateCapture('facebook', {
    sourceHydrationTimeoutMs: 29000, scrolls: 3, captureTimeoutMs: 90000,
    pendingContentPolicy: 'detect_only', sameTabMutationAllowed: false,
    sourceFreshnessPolicy: 'preserve_frontier', acquisitionRound: 2,
    continuation: { startScrollY: 900, anchorKeys: ['facebook:post:12345'], settleMs: 900 },
  });
  assert.equal(next.acquisitionRound, 2);
  assert.equal(next.explicitPageUrl, false);
  assert.throws(() => validateCapture('x', { acquisitionRound: 2, continuation: { cursor: 'opaque' } }), { code: 'unsupported_continuation' });
  assert.throws(() => validateCapture('x', { captureTimeoutMs: 90001 }), { code: 'invalid_payload' });
});

test('round 2 resumes its per-source page frontier after another source is captured', async () => {
  class FakePage {
    constructor(source) { this.source = source; this.url = 'about:blank'; this.y = 0; this.navigations = 0; }
    async navigate(url) { this.url = url; this.y = 0; this.navigations++; return {}; }
    async send() { return {}; }
    async evaluate(expression) {
      if (expression === 'location.href') return this.url;
      const to = /window\.scrollTo\(\{top:(\d+)/.exec(expression);
      if (to) { this.y = Number(to[1]); return; }
      const by = /window\.scrollBy\(0,Math\.round\(innerHeight\*([0-9.]+)\)\)/.exec(expression);
      if (by) { this.y += Math.round(900 * Number(by[1])); return; }
      if (expression === 'globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []') return [];
      if (expression.includes('XHeadlessPoC.collect()')) {
        const isX = this.source === 'x';
        const id = isX ? '1890000000000000000' : 'facebook:post:12345';
        return JSON.stringify({ posts: [{ id, permalink: isX
          ? 'https://x.com/example/status/1890000000000000000' : 'https://www.facebook.com/example/posts/12345',
        author: 'Example', text: 'Fixture post' }], scroll: { y: this.y, viewportHeight: 900 }, documentReady: true });
      }
      return undefined;
    }
  }
  const pages = new Map();
  const browser = { async forSource(source) { if (!pages.has(source)) pages.set(source, new FakePage(source)); return pages.get(source); } };
  const makeAssets = () => ['runtime', 'adapter', 'extractor'].map((name, i) => ({ relative: name, sha256: String(i).repeat(64), execute: false }));
  const assets = { x: makeAssets(), facebook: makeAssets() };
  const first = await capture(browser, assets, 'facebook', { acquisitionRound: 1, scrolls: 0, sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 3000 });
  const checkpoint = first.coverage.frontier;
  assert.deepEqual(checkpoint.anchorKeys, ['facebook:post:12345']);
  assert.equal(checkpoint.scrollY, 0);
  assert.equal(first.coverage.performedScrolls, 0);
  assert.equal(first.coverage.scrollStopReason, 'not_requested');
  assert.equal(checkpoint.hasMoreCandidateSignal, null, 'missing document height must remain unknown');
  assert.equal(checkpoint.continuationReady, true);
  assert.equal(first.coverage.freshness.workerStatus, 'not_verified');
  const x = await capture(browser, assets, 'x', { acquisitionRound: 1, scrolls: 3, scrollSettleMs: 100,
    sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 3000 });
  assert.equal(x.source, 'x');
  assert.equal(x.snapshots.length, 4, 'maxBlocksPerSnapshot must not be treated as a total post limit');
  assert.equal(x.coverage.performedScrolls, 3);
  assert.equal(x.coverage.scrollStopReason, 'budget_exhausted');
  assert.equal(x.coverage.frontier.hasMoreCandidateSignal, null, 'missing document height must remain unknown');
  const continuation = { startScrollY: checkpoint.scrollY, anchorKeys: checkpoint.anchorKeys, settleMs: 0 };
  assert.equal(continuationMatches({ source: 'facebook', pageUrl: 'facebook://www.facebook.com/', frontier: checkpoint }, 'facebook', 'https://www.facebook.com/', continuation), true);
  const next = await capture(browser, assets, 'facebook', {
    acquisitionRound: 2, continuation, scrolls: 0, sourceHydrationTimeoutMs: 1000,
    captureTimeoutMs: 3000, pendingContentPolicy: 'detect_only', sameTabMutationAllowed: false,
    sourceFreshnessPolicy: 'preserve_frontier', restoreScroll: true,
  });
  assert.equal(next.source, 'facebook');
  assert.equal(pages.get('facebook').navigations, 1, 'round 2 must not navigate or reload the source tab');
  assert.equal(pages.get('x').navigations, 1, 'capturing X must use its own retained tab');
  assert.equal(next.coverage.performedScrolls, 0);
  assert.equal(next.coverage.scrollStopReason, 'not_requested');
  await assert.rejects(() => capture(browser, assets, 'facebook', {
    acquisitionRound: 2, continuation: { ...continuation, startScrollY: 42 }, scrolls: 0,
    sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 3000,
  }), { code: 'unsupported_continuation' });
});

test('capture counts only observed scroll movement and records no_movement', async () => {
  const page = {
    url: 'about:blank',
    async navigate(url) { this.url = url; return {}; },
    async send() { return {}; },
    async evaluate(expression) {
      if (expression === 'location.href') return this.url;
      if (expression === 'globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []') return [];
      if (expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({
        posts: [{ id: '1890000000000000000', permalink: 'https://x.com/example/status/1890000000000000000', author: 'Example', text: 'Fixture post' }],
        scroll: { y: 0, viewportHeight: 900, height: 1800 }, documentReady: true,
      });
      return undefined;
    },
  };
  const assets = { x: ['runtime', 'adapter', 'extractor'].map((name, i) => ({ relative: name, sha256: String(i).repeat(64), execute: false })) };
  const observation = await capture(page, assets, 'x', { scrolls: 2, scrollSettleMs: 100, sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 3000 });
  assert.equal(observation.coverage.performedScrolls, 0);
  assert.equal(observation.coverage.scrollStopReason, 'no_movement');
  assert.equal(observation.coverage.frontier.hasMoreCandidateSignal, true);
});

test('capture leaves scroll count and continuation unknown when the post-scroll position is unavailable', async () => {
  let collectCount = 0;
  const page = {
    url: 'about:blank',
    async navigate(url) { this.url = url; return {}; },
    async send() { return {}; },
    async evaluate(expression) {
      if (expression === 'location.href') return this.url;
      if (expression === 'globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []') return [];
      if (expression.includes('XHeadlessPoC.collect()')) {
        collectCount++;
        return JSON.stringify({
          posts: [{ id: '1890000000000000000', permalink: 'https://x.com/example/status/1890000000000000000', author: 'Example', text: 'Fixture post' }],
          ...(collectCount === 1 ? { scroll: { y: 0, viewportHeight: 900, height: 1800 } } : {}),
          documentReady: true,
        });
      }
      return undefined;
    },
  };
  const assets = { x: ['runtime', 'adapter', 'extractor'].map((name, i) => ({ relative: name, sha256: String(i).repeat(64), execute: false })) };
  const observation = await capture(page, assets, 'x', { scrolls: 2, scrollSettleMs: 100, sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 3000 });
  assert.equal(observation.coverage.performedScrolls, undefined);
  assert.equal(observation.coverage.scrollStopReason, 'scroll_position_unavailable');
  assert.equal(observation.coverage.frontier.hasMoreCandidateSignal, null);
  assert.equal(observation.coverage.frontier.continuationReady, null);
});

test('maps X evidence to the canonical Observation shape without changing source IDs', () => {
  const post = {
    id: '1890000000000000000',
    permalink: 'https://x.com/example/status/1890000000000000000',
    author: 'Example',
    avatar: 'https://pbs.twimg.com/profile_images/example.jpg',
    text: 'A source-backed X post.',
    publishedAt: '2026-09-30T10:20:30.000Z',
    contentKind: 'video',
    relationshipType: 'quote',
    parentPermalink: 'https://x.com/other/status/1880000000000000000',
    quotedPost: { id: '1880000000000000000', permalink: null, identityStatus: 'unknown', media: [{ kind: 'video_poster' }] },
    engagement: { replies: 4, reposts: 2 },
    media: [{ kind: 'video_poster', url: 'https://video.twimg.com/poster.jpg', loaded: true }],
    mediaExpected: ['video'],
    mediaEvidence: { status: 'missing_expected_url', expectedWithoutUrl: ['video'] },
    limitations: ['video_stream_not_resolved'],
    textStatus: 'visible_text_no_collapse_control',
  };
  const provenance = { schema: 'aku.headless-source-provenance.v1', algorithm: 'sha256', sources: [{ path: 'AkuBridge/adapters/x-adapter.js', sha256: 'a'.repeat(64) }] };
  const observation = toObservation({ source: 'x', requestedUrl: 'https://x.com/example/status/1890000000000000000',
    snapshots: [{ posts: [post], adapterVersion: 'x-dom-v23', discoveryStrategy: 'tweet_testid', candidateCount: 1,
      scroll: { y: 0, viewportHeight: 900 }, documentReady: true }], provenance,
    capturedAt: '2026-10-01T00:00:00.000Z', stopReason: 'scroll_limit' });
  const block = observation.snapshots[0].blocks[0];
  const expectedDigest = createHash('sha256').update('x\0x:status:' + post.id, 'utf8').digest('hex').slice(0, 24);
  assert.equal(observation.source, 'x');
  assert.equal(observation.coverage.status, 'partial');
  assert.equal(observation.coverage.provenance.sources[0].sha256, 'a'.repeat(64));
  assert.equal(block.platformId, `x:status:${post.id}`);
  assert.equal(block.captureQuality.headlessSourceId, post.id);
  assert.equal(block.permalink, post.permalink);
  assert.equal(block.publishedAt, post.publishedAt);
  assert.equal(block.relationshipType, 'quote');
  assert.deepEqual(block.quotedPost, post.quotedPost);
  assert.equal(block.evidenceKey, `x:${expectedDigest}`);
  assert.equal(block.mediaRecovery.unknownVideo, 'unresolved');
  assert.equal(block.media[0].url, post.media[0].url);
});

test('unavailable Facebook target notice is scoped to the exact requested native page', async () => {
  const native='https://www.facebook.com/example/posts/12345';
  const assets={facebook:['runtime','adapter','extractor'].map((name,i)=>({relative:name,sha256:String(i).repeat(64),execute:false}))};
  for (const [pageUrl,actualUrl,want] of [[native,native,'target_unavailable'],[native,'https://www.facebook.com/','empty_unverified'],[undefined,'https://www.facebook.com/','empty_unverified']]) {
    let noticeCalls=0;
    const page={async navigate(){return {};},async evaluate(expression){
      if(expression==='location.href')return actualUrl;
      if(expression==='globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []')return [];
      if(expression.includes('XHeadlessPoC.collect()'))return JSON.stringify({posts:[],documentReady:true,scroll:{y:0}});
      if(expression.includes('function facebookTargetUnavailable')){noticeCalls++;return true;}
    }};
    await assert.rejects(capture({forSource:async()=>page},assets,'facebook',{pageUrl,scrolls:0,sourceHydrationTimeoutMs:1000,captureTimeoutMs:3000}),{code:want});
    assert.equal(noticeCalls,want==='target_unavailable'?1:0);
  }
});

test('maps Facebook timestamp and video uncertainty without promoting unknowns', () => {
  const post = {
    id: 'facebook:post:12345',
    permalink: 'https://www.facebook.com/example/posts/12345',
    author: 'Example Page',
    text: 'A bounded Facebook post.',
    publishedAt: null,
    timestampSource: 'unavailable',
    timestampEstimated: false,
    timestampText: '',
    timestampEvidence: { diagnostics: { scannedScripts: 3 }, publishedAt: null },
    contentKind: 'video',
    relationshipType: 'original',
    parentPermalink: null,
    presentation: { timestampAvailability: 'unavailable' },
    media: [{ kind: 'video_poster', url: 'https://video.xx.fbcdn.net/poster.jpg', loaded: true }],
    mediaExpected: ['video'],
    mediaEvidence: { status: 'missing_expected_url', expectedWithoutUrl: ['video'] },
    limitations: ['video_stream_not_resolved'],
    textStatus: 'visible_text_no_collapse_control',
  };
  const observation = toObservation({ source: 'facebook', requestedUrl: post.permalink,
    snapshots: [{ posts: [post], adapterVersion: 'facebook-dom-v18', discoveryStrategy: 'aria_posinset', candidateCount: 2,
      rejected: 1, rejectionReasons: { missing_identity: 1 }, scroll: { y: 0, viewportHeight: 900 } }],
    provenance: { schema: 'aku.headless-source-provenance.v1', algorithm: 'sha256', sources: [] },
    capturedAt: '2026-10-01T00:00:00.000Z', stopReason: 'post_limit' });
  const block = observation.snapshots[0].blocks[0];
  assert.equal(block.platformId, post.id);
  assert.equal(block.publishedAt, null);
  assert.equal(block.presentation.timestampAvailability, 'unavailable');
  assert.equal(block.presentation.timestampSource, 'unavailable');
  assert.equal(block.mediaRecovery.unknownVideo, 'unresolved');
  assert.equal(observation.coverage.status, 'partial');
  assert.equal(observation.snapshots[0].candidateDiagnostics, undefined);
  assert.equal(observation.snapshots[0].qualityReports[0].status, 'unverified');
});

test('JSONL protocol rejects capture before init and shuts down without starting Chrome', async () => {
  const input = new PassThrough();
  const output = new PassThrough();
  let transcript = '';
  output.setEncoding('utf8');
  output.on('data', chunk => { transcript += chunk; });
  const running = runWorker({ input, output, errorOutput: new PassThrough() });
  input.write(JSON.stringify({ id: 'before-init', type: 'capture', source: 'x', payload: {} }) + '\n');
  input.write(JSON.stringify({ id: 'stop', type: 'shutdown' }) + '\n');
  await running;
  const replies = transcript.trim().split('\n').map(line => JSON.parse(line));
  assert.deepEqual(replies.map(reply => reply.id), ['before-init', 'stop']);
  assert.equal(replies[0].ok, false);
  assert.equal(replies[0].error.code, 'not_initialized');
  assert.deepEqual(replies[1], { id: 'stop', ok: true, result: { stopped: true } });
  assert.deepEqual(validateRequest({ id: 7, type: 'init', chrome: 'C:\\Chrome\\chrome.exe' }).type, 'init');
  assert.deepEqual(validateInit({ type: 'init', chrome: 'C:\\Chrome\\chrome.exe', profile: 'C:\\Profile', bridgePath: 'C:\\AkuBridge' }), {
    chromePath: 'C:\\Chrome\\chrome.exe', profilePath: 'C:\\Profile', bridgePath: 'C:\\AkuBridge', profileDirectory: undefined,
  });
});
