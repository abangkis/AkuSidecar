import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { capture } from '../capture.mjs';
import { createXTextRecovery } from '../x-text-recovery.mjs';

const urlFor = id => `https://x.com/theo/status/${id}`;
const assets = [
  { relative: 'AkuBridge/capture-primitives.js', execute: true,
    content: readFileSync(new URL('../../../../../../AkuBridge/capture-primitives.js', import.meta.url), 'utf8') },
  { relative: 'adapter', execute: true, content: 'fixture adapter' },
  { relative: 'extractor', execute: true, content: 'fixture extractor' },
];

function feedPost(id, extra = {}) {
  return {
    id,
    permalink: urlFor(id),
    author: 'Theo',
    text: 'A'.repeat(280),
    textCollapsed: true,
    textStatus: 'requires_permalink_capture',
    limitations: ['visible_dom_only', 'text_may_be_collapsed'],
    media: [{ kind: 'image', url: 'https://pbs.twimg.com/media/feed.jpg' }],
    relationshipType: 'quote',
    quotedPost: { id: '1880000000000000000', permalink: 'https://x.com/other/status/1880000000000000000' },
    engagement: { replies: 4, reposts: 2 },
    ...extra,
  };
}

function makeHarness({ documents = {}, redirects = {}, navigateError = null, hangCollect = false, closeError = null } = {}) {
  const state = { createCount: 0, closeCount: 0, navigations: [], evaluations: [], collectCounts: new Map(), url: 'about:blank' };
  const page = {
    async navigate(url) {
      state.navigations.push(url);
      const error = typeof navigateError === 'function' ? navigateError(state.navigations.length) : navigateError;
      if (error) throw error;
      state.url = redirects[url] || url;
      return {};
    },
    async evaluate(expression) {
      state.evaluations.push(expression);
      if (expression === '({url:location.href,ready:document.readyState})') return { url: state.url, ready: 'complete' };
      if (expression === 'location.href') return state.url;
      if (expression.includes('globalThis.XHeadlessPoC.collect()')) {
        if (typeof hangCollect === 'function' ? hangCollect(state) : hangCollect) return new Promise(() => {});
        const id = new URL(state.url).pathname.match(/\/status\/(\d+)/)?.[1];
        const reads = state.collectCounts.get(id) || 0;
        state.collectCounts.set(id, reads + 1);
        const configured = documents[id];
        const item = Array.isArray(configured) ? configured[Math.min(reads, configured.length - 1)] : configured;
        const snapshot = typeof item === 'function' ? item(reads, state.url) : item;
        return JSON.stringify({ url: state.url, posts: [], ...(snapshot || {}) });
      }
      return undefined;
    },
    async close() {
      state.closeCount++;
      if (closeError) throw closeError;
    },
  };
  const browser = {
    backend: 'headless_worker',
    async createTemporaryPage(timeoutMs) {
      state.createCount++;
      state.setupTimeoutMs = timeoutMs;
      return page;
    },
  };
  return { browser, page, state };
}

function detail(id, text, extra = {}) {
  return {
    posts: [{ id, permalink: urlFor(id), author: 'Theo', text,
      textCollapsed: false, textStatus: 'expanded', ...extra }],
  };
}

test('recovers longer text only from a hydrated exact permalink and preserves feed evidence', async () => {
  const id = '2106847019319062819';
  const fullText = `${'Long post line.\n'.repeat(24)}🧵`;
  const { browser, state } = makeHarness({ documents: { [id]: [{ posts: [] }, detail(id, fullText)] } });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 5000, perTargetMs: 1500, totalMs: 2500 });
  const original = feedPost(id);
  const first = await recovery.recoverSnapshot({ posts: [original], scroll: { y: 900, height: 3000 } });
  const recovered = first.posts[0];
  assert.equal(recovered.text, fullText);
  assert.equal(recovered.textStatus, 'permalink_text_verified');
  assert.equal(recovered.textCompleteness, 'recovery_verified');
  assert.equal(recovered.textCollapsed, false);
  assert.deepEqual(recovered.media, original.media);
  assert.deepEqual(recovered.quotedPost, original.quotedPost);
  assert.deepEqual(recovered.engagement, original.engagement);
  assert.deepEqual(recovered.limitations, ['visible_dom_only']);
  assert.equal(state.collectCounts.get(id), 2, 'recovery should retry after an empty hydrated snapshot');
  await recovery.close();
  assert.equal(state.createCount, 1);
  assert.equal(state.closeCount, 1);
});

test('validates post identity before navigation and rejects a redirected status route', async () => {
  const id = '2106847019319062819';
  const { browser, state } = makeHarness({ redirects: { [urlFor(id)]: urlFor('2106847019319062820') } });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 100, totalMs: 100 });
  const mismatch = feedPost('2106847019319062821', { permalink: urlFor(id) });
  const rejected = (await recovery.recoverSnapshot({ posts: [mismatch] })).posts[0];
  assert.equal(rejected.textStatus, 'requires_permalink_capture');
  assert.ok(rejected.limitations.includes('permalink_identity_mismatch'));
  assert.equal(state.createCount, 0, 'a feed ID and permalink mismatch must be rejected before target creation');

  const redirected = feedPost(id);
  const result = (await recovery.recoverSnapshot({ posts: [redirected] })).posts[0];
  assert.equal(result.textStatus, 'requires_permalink_capture');
  assert.ok(result.limitations.includes('permalink_identity_mismatch'));
  assert.deepEqual(state.navigations, [urlFor(id)]);
  assert.equal(result.textRecovery.attempts.length, 1, 'identity mismatch must not retry');
  await recovery.close();
  assert.equal(state.closeCount, 1);
});

test('retries transient navigation once and preserves recovery stage diagnostics', async () => {
  const id = '2106847019319062819';
  const fullText = 'Recovered after transient navigation. ' + 'R'.repeat(320);
  const { browser, state } = makeHarness({ documents: { [id]: detail(id, fullText) },
    navigateError: attempt => attempt === 1 ? new Error('temporary navigation failure') : null });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 400, totalMs: 600 });
  const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
  assert.equal(result.text, fullText);
  assert.equal(result.textStatus, 'permalink_text_verified');
  assert.equal(result.textRecovery.attempts.length, 2);
  assert.equal(result.textRecovery.attempts[0].stage, 'navigation');
  assert.equal(result.textRecovery.attempts[0].limitation, 'permalink_capture_failed');
  assert.equal(result.textRecovery.attempts[1].outcome, 'verified');
  await recovery.recoverSnapshot({ posts: [feedPost(id)] });
  assert.equal(state.navigations.length, 2, 'successful retry is cached');
  await recovery.close();
  assert.equal(state.closeCount, 1);
});

test('retries a timed-out collection within the shared per-target deadline', async () => {
  const id = '2106847019319062819';
  const { browser, state } = makeHarness({ documents: { [id]: detail(id, 'Recovered text ' + 'T'.repeat(320)) },
    hangCollect: state => state.navigations.length === 1 });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 200, totalMs: 300 });
  const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
  assert.equal(result.textStatus, 'permalink_text_verified');
  assert.equal(result.textRecovery.attempts.length, 2);
  assert.equal(result.textRecovery.attempts[0].stage, 'text_collection');
  assert.equal(result.textRecovery.attempts[0].limitation, 'permalink_capture_timeout');
  assert.equal(state.createCount, 1);
  await recovery.close();
});

test('persistent failures stop after two attempts and capture expiry prevents retry', async () => {
  const id = '2106847019319062819';
  const failing = makeHarness({ navigateError: new Error('persistent failure') });
  const recovery = createXTextRecovery({ browser: failing.browser, assets, deadlineAt: Date.now() + 1000,
    perTargetMs: 200, totalMs: 300 });
  const first = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
  assert.equal(first.textRecovery.attempts.length, 2);
  assert.equal(first.text, feedPost(id).text);
  await recovery.recoverSnapshot({ posts: [feedPost(id)] });
  assert.equal(failing.state.navigations.length, 2, 'failed retry cannot repeat across feed snapshots');
  await recovery.close();

  const hung = makeHarness({ hangCollect: true });
  const bounded = createXTextRecovery({ browser: hung.browser, assets, deadlineAt: Date.now() + 30,
    perTargetMs: 200, totalMs: 300 });
  const expired = (await bounded.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
  assert.equal(expired.textRecovery.attempts.length, 1);
  assert.equal(hung.state.navigations.length, 1);
  assert.ok(expired.limitations.includes('permalink_capture_timeout'));
  await bounded.close();
});

test('bounds target count, caches success and failure across snapshots, and closes one owned page', async () => {
  const ids = ['2106847019319062819', '2106847019319062820', '2106847019319062821', '2106847019319062822'];
  const documents = Object.fromEntries(ids.slice(0, 3).map(id => [id, detail(id, `Full text for ${id} with more than 280 characters. ${'B'.repeat(300)}`)]));
  const { browser, state } = makeHarness({ documents });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 5000, perTargetMs: 1000, totalMs: 4000 });
  const posts = ids.map(id => feedPost(id));
  const first = await recovery.recoverSnapshot({ posts });
  const second = await recovery.recoverSnapshot({ posts });
  assert.deepEqual(first.posts.slice(0, 3).map(post => post.textStatus), Array(3).fill('permalink_text_verified'));
  assert.ok(first.posts[3].limitations.includes('permalink_capture_limit_reached'));
  assert.deepEqual(second.posts.map(post => post.textStatus), first.posts.map(post => post.textStatus));
  assert.deepEqual(state.navigations, ids.slice(0, 3).map(urlFor));
  assert.equal(state.createCount, 1);
  await recovery.close();
  assert.equal(state.closeCount, 1);
});

test('cached outcomes preserve later longer or complete feed text', async t => {
  const id = '2106847019319062819';
  await t.test('verified text does not replace a longer unresolved snapshot', async () => {
    const recoveredText = `Verified permalink text. ${'R'.repeat(320)}`;
    const { browser } = makeHarness({ documents: { [id]: detail(id, recoveredText) } });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 300, totalMs: 300 });
    await recovery.recoverSnapshot({ posts: [feedPost(id)] });
    const later = feedPost(id, { text: 'L'.repeat(600) });
    const observed = (await recovery.recoverSnapshot({ posts: [later] })).posts[0];
    assert.deepEqual(observed, later);
    await recovery.close();
  });
  await t.test('truncated text does not turn a later complete feed post into partial', async () => {
    const { browser } = makeHarness({ documents: { [id]: detail(id, '🧵'.repeat(4001)) } });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 300, totalMs: 300 });
    await recovery.recoverSnapshot({ posts: [feedPost(id)] });
    const later = feedPost(id, { text: 'A complete later feed observation.', textCollapsed: false,
      textStatus: 'visible_text_no_collapse_control', limitations: ['visible_dom_only'] });
    assert.deepEqual((await recovery.recoverSnapshot({ posts: [later] })).posts[0], later);
    await recovery.close();
  });
  await t.test('failed text recovery does not degrade a later complete post', async () => {
    const { browser } = makeHarness({ navigateError: new Error('navigation failed') });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 300, totalMs: 300 });
    await recovery.recoverSnapshot({ posts: [feedPost(id)] });
    const later = feedPost(id, { text: 'A complete later feed observation.', textCollapsed: false,
      textStatus: 'visible_text_no_collapse_control', limitations: ['visible_dom_only'] });
    assert.deepEqual((await recovery.recoverSnapshot({ posts: [later] })).posts[0], later);
    await recovery.close();
  });
});

test('keeps timeout, navigation failure, challenge, and unresolved collapse partial', async t => {
  const id = '2106847019319062819';
  await t.test('collect timeout', async () => {
    const { browser, state } = makeHarness({ hangCollect: true });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 500, perTargetMs: 30, totalMs: 30 });
    const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
    assert.equal(result.textStatus, 'requires_permalink_capture');
    assert.ok(result.limitations.includes('permalink_capture_timeout'));
    await recovery.close();
    assert.equal(state.closeCount, 1);
  });
  await t.test('navigation failure', async () => {
    const { browser, state } = makeHarness({ navigateError: new Error('navigation failed') });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 500, perTargetMs: 100, totalMs: 100 });
    const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
    assert.ok(result.limitations.includes('permalink_capture_failed'));
    await recovery.close();
    assert.equal(state.closeCount, 1);
  });
  await t.test('challenge snapshot', async () => {
    const { browser } = makeHarness({ documents: { [id]: { challengeDetected: true } } });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 500, perTargetMs: 100, totalMs: 100 });
    const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
    assert.ok(result.limitations.includes('permalink_capture_unavailable'));
    assert.equal(result.textRecovery.attempts.length, 1, 'challenge must not retry');
    await recovery.close();
  });
  await t.test('detail remains collapsed', async () => {
    const { browser } = makeHarness({ documents: { [id]: detail(id, 'C'.repeat(500), { textCollapsed: true, textStatus: 'requires_permalink_capture' }) } });
    const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 500, perTargetMs: 30, totalMs: 30 });
    const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
    assert.equal(result.textStatus, 'requires_permalink_capture');
    assert.equal(result.text, 'A'.repeat(280));
    assert.ok(result.limitations.includes('permalink_text_unresolved'));
    await recovery.close();
  });
});

test('clips at the observation limit while marking text partial', async () => {
  const id = '2106847019319062819';
  const longText = '🧵'.repeat(4001);
  const { browser } = makeHarness({ documents: { [id]: detail(id, longText) } });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 300, totalMs: 300 });
  const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
  assert.equal(Array.from(result.text).length, 4000);
  assert.equal(result.textStatus, 'requires_permalink_capture');
  assert.ok(result.limitations.includes('text_may_be_truncated'));
  assert.ok(result.limitations.includes('text_truncated'));
  assert.equal(result.textCompleteness, 'truncated');
  await recovery.close();
});

test('Quiet backend keeps permalink-required text explicitly partial', async () => {
  const id = '2106847019319062819';
  const browser = { backend: 'browser_quiet_hidden' };
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000 });
  const result = (await recovery.recoverSnapshot({ posts: [feedPost(id)] })).posts[0];
  assert.equal(result.textStatus, 'requires_permalink_capture');
  assert.ok(result.limitations.includes('permalink_capture_unavailable'));
  await recovery.close();
});

test('propagates temporary page cleanup failures', async () => {
  const id = '2106847019319062819';
  const { browser } = makeHarness({ documents: { [id]: detail(id, 'Full text ' + 'D'.repeat(300)) }, closeError: new Error('target still exists') });
  const recovery = createXTextRecovery({ browser, assets, deadlineAt: Date.now() + 1000, perTargetMs: 300, totalMs: 300 });
  await recovery.recoverSnapshot({ posts: [feedPost(id)] });
  await assert.rejects(recovery.close(), { code: 'temporary_target_cleanup_failed' });
});

test('capture preserves the retained feed page and its frontier while using a separate permalink target', async () => {
  const id = '2106847019319062819';
  const feedPostValue = feedPost(id);
  const { browser: recoveryBrowser, page: recoveryPage, state: recoveryState } = makeHarness({
    documents: { [id]: detail(id, `Recovered from the permalink. ${'E'.repeat(320)}`) },
  });
  const feedState = { url: 'about:blank', y: 0, navigations: [] };
  const feedPage = {
    async navigate(url) { feedState.url = url; feedState.navigations.push(url); feedState.y = 0; return {}; },
    async send() { return {}; },
    async evaluate(expression) {
      if (expression === 'location.href') return feedState.url;
      if (expression === 'globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []') return [];
      const restore = /window\.scrollTo\(\{top:(\d+)/.exec(expression);
      if (restore) { feedState.y = Number(restore[1]); return; }
      if (expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({
        posts: [feedPostValue], scroll: { y: feedState.y, height: 2400, viewportHeight: 900 }, documentReady: true,
      });
      return undefined;
    },
  };
  const browser = {
    backend: 'headless_worker',
    async forSource(source) { assert.equal(source, 'x'); return feedPage; },
    async createTemporaryPage(timeoutMs) {
      recoveryState.setupTimeoutMs = timeoutMs;
      return recoveryPage;
    },
  };
  const workerAssets = { x: [
    assets[0],
    { relative: 'runtime', sha256: '1'.repeat(64), execute: false },
    { relative: 'adapter', sha256: '2'.repeat(64), execute: false },
    { relative: 'extractor', sha256: '3'.repeat(64), execute: false },
  ] };
  const observation = await capture(browser, workerAssets, 'x', {
    scrolls: 0, sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 5000,
  });
  assert.equal(observation.snapshots[0].blocks[0].captureQuality.textStatus, 'permalink_text_verified');
  assert.equal(observation.snapshots[0].blocks[0].captureQuality.textRecovery.attempts[0].outcome, 'verified');
  assert.equal(observation.snapshots[0].blocks[0].captureQuality.textCompleteness, 'recovery_verified');
  assert.equal(observation.coverage.frontier.scrollY, 0);
  assert.deepEqual(observation.coverage.frontier.anchorKeys, [`x:status:${id}`]);
  assert.deepEqual(feedState.navigations, ['https://x.com/home']);
  assert.equal(feedState.url, 'https://x.com/home');
  assert.equal(feedState.y, 0);
  assert.deepEqual(recoveryState.navigations, [urlFor(id)]);
  assert.equal(recoveryState.closeCount, 1);
});
