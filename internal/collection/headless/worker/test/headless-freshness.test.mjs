import test from 'node:test';
import assert from 'node:assert/strict';
import { recoverHeadlessFreshness } from '../headless-freshness.mjs';

const home = 'https://x.com/home';
const homeWithQuery = 'https://x.com/home?tab=for-you#feed';
const urlFor = id => `https://x.com/owner/status/${id}`;

function post(id, text, extra = {}) {
  return { id, permalink: urlFor(id), text, ...extra };
}

function snapshot(posts, extra = {}) {
  return { url: home, posts, ...extra };
}

const xContract = {
  source: 'x', enabled: true, version: 'synthetic-x-v1',
  matchesFeedURL(value) {
    if (typeof value !== 'string' || value.length > 2048) return false;
    try {
      const url = new URL(value);
      return url.protocol === 'https:' && url.hostname.toLowerCase() === 'x.com'
        && !url.username && !url.password && !url.port && url.pathname === '/home';
    } catch { return false; }
  },
  primaryIdentity(post) {
    const id = typeof post?.id === 'string' && /^\d{1,30}$/.test(post.id) ? post.id : null;
    if (!id || typeof post?.permalink !== 'string') return null;
    let permalinkId;
    try { permalinkId = new URL(post.permalink).pathname.match(/^\/[^/]+\/status\/(\d+)$/)?.[1] ?? null; }
    catch { return null; }
    return permalinkId === id ? { id, permalink: post.permalink } : null;
  },
};

function makePage({ probe = { pendingContentDetected: true, pendingContentLabel: 'Show new posts', strategy: 'pending_button' },
  activation = { activated: true, label: 'Show new posts' }, url = homeWithQuery, hangProbe = false,
  sourceName = 'x' } = {}) {
  const probeCall = `runtime.probe(${JSON.stringify(sourceName)})`;
  const activationCall = `runtime.activatePending(${JSON.stringify(sourceName)}, `;
  const state = { url, sourceName, probeCalls: 0, activationCalls: 0, evaluations: [], activationArgs: [] };
  const page = {
    async evaluate(expression, timeoutMs) {
      state.evaluations.push({ expression, timeoutMs });
      if (expression === 'location.href') return state.url;
      if (expression.includes(probeCall)) {
        state.probeCalls++;
        if (hangProbe) return new Promise(() => {});
        return typeof probe === 'function' ? probe(state) : probe;
      }
      if (expression.includes(activationCall)) {
        const start = expression.indexOf(activationCall) + activationCall.length;
        const end = expression.indexOf(') };', start);
        const args = start >= 0 && end > start ? JSON.parse(expression.slice(start, end)) : null;
        state.activationCalls++;
        state.activationArgs.push(args);
        const result = typeof activation === 'function' ? await activation(state, args) : activation;
        return { available: true, result };
      }
      return undefined;
    },
  };
  return { page, state };
}

function options(overrides = {}) {
  return {
    source: 'x', pageUrl: home, acquisitionRound: 1, explicitPageUrl: false,
    pendingContentPolicy: 'reveal_if_present', sourceFreshnessPolicy: 'wake_and_reveal',
    sameTabMutationAllowed: true, ...overrides,
  };
}

function invoke({ page, initial = snapshot([post('10001', 'Original post')]), fresh,
  collectSnapshot, captureMs = 1600, opts = options(), backend = 'headless_worker',
  source = opts.source ?? 'x', contract = xContract }) {
  return recoverHeadlessFreshness({ page, snapshot: initial, options: opts,
    deadlineAt: Date.now() + captureMs,
    collectSnapshot: collectSnapshot || (async () => fresh), backend, source, contract });
}

test('accepts one new primary native post and retains the verified feed snapshot', async () => {
  const { page, state } = makePage();
  const retained = snapshot([post('10001', 'Original post'), post('10002', 'New post')]);
  const result = await invoke({ page, fresh: retained });
  assert.equal(result.freshness.status, 'verified');
  assert.equal(result.freshness.workerStatus, 'verified');
  assert.equal(result.freshness.comparisonScope, 'same_capture_primary_posts');
  assert.equal(result.freshness.addedPrimaryCount, 1);
  assert.equal(result.freshness.changedPrimaryCount, 0);
  assert.equal(result.freshness.verifiedPrimaryCount, 1);
  assert.equal(result.snapshot, retained);
  assert.equal(state.activationCalls, 1);
  assert.equal(result.freshness.activationAttempts, 1);
  assert.equal(result.freshness.activationCount, 1);
  assert.equal(state.activationArgs[0].expectedPageUrl, homeWithQuery);
  const activationEvaluation = state.evaluations.find(e => e.expression.includes('runtime.activatePending'));
  assert.ok(state.activationArgs[0].deadlineAt <= Date.now() + activationEvaluation.timeoutMs,
    'the DOM mutation must expire no later than the host activation wait');
});

test('accepts normalized content edits on the same exact primary identity', async () => {
  const { page } = makePage();
  const initial = snapshot([post('10001', 'A short post\nwith stable words')]);
  const fresh = snapshot([post('10001', 'A short post with\nstable words and a new sentence')]);
  const result = await invoke({ page, initial, fresh });
  assert.equal(result.freshness.status, 'verified');
  assert.equal(result.freshness.addedPrimaryCount, 0);
  assert.equal(result.freshness.changedPrimaryCount, 1);
  assert.equal(result.snapshot, fresh);
});

test('native ID and normalized text remain the comparison key across permalink changes', async () => {
  const { page } = makePage();
  const initial = snapshot([post('10001', 'Original text')]);
  const fresh = snapshot([post('10001', 'Updated text', {
    permalink: 'https://x.com/renamed_owner/status/10001',
  })]);
  const result = await invoke({ page, initial, fresh });
  assert.equal(result.freshness.status, 'verified');
  assert.equal(result.freshness.addedPrimaryCount, 0);
  assert.equal(result.freshness.changedPrimaryCount, 1);
  assert.equal(result.snapshot, fresh);
});

test('reordering, engagement, author, relative age, and quote-only changes do not qualify', async () => {
  const { page, state } = makePage();
  const initial = snapshot([
    post('10001', 'First primary', { author: 'A', publishedAt: '2m', engagement: { likes: 1 },
      quotedPost: { id: '70001', text: 'quoted text' } }),
    post('10002', 'Second primary', { author: 'B', publishedAt: '3m', engagement: { likes: 2 } }),
  ]);
  const fresh = snapshot([
    post('10002', 'Second primary', { author: 'B renamed', publishedAt: '4m', engagement: { likes: 30 } }),
    post('10001', 'First primary', { author: 'A renamed', publishedAt: '5m', engagement: { likes: 20 },
      quotedPost: { id: '70002', text: 'different quoted text' } }),
  ]);
  const result = await invoke({ page, initial, fresh, captureMs: 1450 });
  assert.equal(result.snapshot, initial, 'unverified reveal must preserve the initial feed evidence');
  assert.equal(result.freshness.status, 'reveal_unverified');
  assert.equal(result.freshness.addedPrimaryCount, 0);
  assert.equal(result.freshness.changedPrimaryCount, 0);
  assert.equal(state.activationCalls, 1, 'the reveal action is attempted exactly once');
});

test('returns checked_no_pending without attempting activation', async () => {
  const { page, state } = makePage({ probe: { pendingContentDetected: false, strategy: 'no_pending_control' } });
  const initial = snapshot([post('10001', 'Original post')]);
  const result = await invoke({ page, initial, fresh: null });
  assert.equal(result.snapshot, initial);
  assert.equal(result.freshness.status, 'checked_no_pending');
  assert.equal(result.freshness.pendingContentDetected, false);
  assert.equal(result.freshness.pendingContentScope, 'visible_page_controls');
  assert.equal(state.activationCalls, 0);
});

test('rechecks the route after a no-pending probe before admitting the initial snapshot', async () => {
  const { page, state } = makePage({ probe: current => {
    current.url = 'https://x.com/i/flow/login';
    return { pendingContentDetected: false };
  } });
  await assert.rejects(invoke({ page }), { code: 'source_route_changed' });
  assert.equal(state.activationCalls, 0);
});

test('rechecks the route when a probe is unavailable or fails', async t => {
  await t.test('unavailable result after login navigation', async () => {
    const { page } = makePage({ probe: current => {
      current.url = 'https://x.com/i/flow/login';
      return null;
    } });
    await assert.rejects(invoke({ page }), { code: 'source_route_changed' });
  });
  await t.test('failed probe after route change', async () => {
    const { page } = makePage({ probe: current => {
      current.url = 'https://x.com/i/flow/login';
      throw new Error('probe interrupted');
    } });
    await assert.rejects(invoke({ page }), { code: 'source_route_changed' });
  });
});

test('a pending control that disappears before activation stays unverified', async () => {
  const { page, state } = makePage({ activation: { activated: false, label: 'Show new posts' } });
  const initial = snapshot([post('10001', 'Original post')]);
  const result = await invoke({ page, initial, fresh: snapshot([post('10001', 'Changed post')]) });
  assert.equal(result.snapshot, initial);
  assert.equal(result.freshness.status, 'reveal_unverified');
  assert.equal(result.freshness.limitation, 'pending_control_unavailable');
  assert.equal(state.activationCalls, 1);
});

test('policy, target, round, and quiet-backend constraints prevent mutation', async t => {
  const pending = { pendingContentDetected: true, pendingContentLabel: 'Show new posts' };
  const cases = [
    ['round 2', options({ acquisitionRound: 2 }), 'headless_worker', 'preserved', 0],
    ['explicit target', options({ explicitPageUrl: true, pageUrl: 'https://x.com/owner/status/10001' }), 'headless_worker', 'not_applicable', 0],
    ['quiet backend', options(), 'browser_quiet_hidden', 'not_applicable', 0],
    ['detect only', options({ pendingContentPolicy: 'detect_only' }), 'headless_worker', 'pending_not_revealed', 0],
    ['preserve frontier', options({ sourceFreshnessPolicy: 'preserve_frontier' }), 'headless_worker', 'preserved', 0],
    ['preserve target', options({ sourceFreshnessPolicy: 'preserve_target' }), 'headless_worker', 'preserved', 0],
    ['same-tab mutation denied', options({ sameTabMutationAllowed: false }), 'headless_worker', 'preserved', 0],
  ];
  for (const [name, opts, backend, expectedStatus, expectedActivations] of cases) {
    await t.test(name, async () => {
      const { page, state } = makePage({ probe: pending });
      const result = await invoke({ page, opts, backend, fresh: null });
      assert.equal(result.freshness.status, expectedStatus);
      assert.equal(state.activationCalls, expectedActivations);
      if (name === 'detect only' || name === 'preserve frontier' || name === 'preserve target') {
        assert.equal(state.probeCalls, 1, 'read-only pending detection remains available');
      }
      if (name === 'round 2' || name === 'explicit target' || name === 'quiet backend') {
        assert.equal(state.probeCalls, 0);
      }
      if (name === 'explicit target') {
        assert.equal(result.freshness.limitation, 'explicit_target');
        assert.equal(state.evaluations.length, 0, 'an explicit native target exits before feed-route evaluation');
      }
    });
  }
});

test('rejects route changes before activation and after activation', async t => {
  await t.test('route changes after probe and before activation', async () => {
    const { page, state } = makePage({ probe: current => {
      current.url = 'https://x.com/i/flow/login';
      return { pendingContentDetected: true };
    } });
    await assert.rejects(invoke({ page, fresh: snapshot([post('10002', 'New')]) }), { code: 'source_route_changed' });
    assert.equal(state.activationCalls, 0);
  });
  await t.test('route changes during activation', async () => {
    const { page, state } = makePage({ activation: current => {
      current.url = 'https://x.com/i/flow/login';
      return { activated: true };
    } });
    await assert.rejects(invoke({ page, fresh: snapshot([post('10002', 'New')]) }), { code: 'source_route_changed' });
    assert.equal(state.activationCalls, 1);
  });
  await t.test('shared helper route error survives a wrapped message', async () => {
    const { page } = makePage({ activation: () => { throw new Error('Evaluation failed: freshness_route_changed'); } });
    await assert.rejects(invoke({ page, fresh: snapshot([post('10002', 'New')]) }), { code: 'source_route_changed' });
  });
});

test('maps shared helper stale-control and unsupported errors to bounded outcomes', async t => {
  await t.test('stale pending control', async () => {
    const { page } = makePage({ activation: () => { throw new Error('freshness_control_unavailable'); } });
    const initial = snapshot([post('10001', 'Original')]);
    const result = await invoke({ page, initial, fresh: snapshot([post('10001', 'Changed')]) });
    assert.equal(result.snapshot, initial);
    assert.equal(result.freshness.status, 'reveal_unverified');
    assert.equal(result.freshness.limitation, 'pending_control_unavailable');
    assert.equal(result.freshness.activationAttempts, 1);
    assert.equal(result.freshness.activationCount, 0);
  });
  await t.test('unsupported activation', async () => {
    const { page } = makePage({ activation: () => { throw new Error('freshness_reveal_unsupported'); } });
    const result = await invoke({ page, fresh: snapshot([post('10002', 'New')]) });
    assert.equal(result.freshness.status, 'reveal_failed');
    assert.equal(result.freshness.limitation, 'activation_unsupported');
  });
  await t.test('shared deadline error', async () => {
    const { page } = makePage({ activation: () => { throw new Error('freshness_deadline'); } });
    const result = await invoke({ page, fresh: snapshot([post('10002', 'New')]) });
    assert.equal(result.freshness.status, 'reveal_failed');
    assert.equal(result.freshness.limitation, 'phase_timeout');
    assert.equal(result.freshness.activationCount, 0);
  });
  await t.test('transport failure leaves physical activation unknown', async () => {
    const { page } = makePage({ activation: () => { throw new Error('CDP connection lost'); } });
    const result = await invoke({ page, fresh: snapshot([post('10002', 'New')]) });
    assert.equal(result.freshness.status, 'reveal_failed');
    assert.equal(result.freshness.activationAttempts, 1);
    assert.equal(result.freshness.activationCount, null);
  });
});

test('propagates login and challenge states instead of admitting old feed evidence', async t => {
  await t.test('login is already required on the initial snapshot', async () => {
    const { page, state } = makePage();
    await assert.rejects(invoke({ page, initial: snapshot([post('10001', 'Original')], { loginRequired: true }) }),
      { code: 'login_required' });
    assert.equal(state.probeCalls, 0);
  });
  await t.test('probe reports an access challenge', async () => {
    const { page } = makePage({ probe: { pendingContentDetected: true, challengeDetected: true } });
    await assert.rejects(invoke({ page, fresh: snapshot([post('10002', 'New')]) }), { code: 'challenge_required' });
  });
  await t.test('challenge appears in the post-activation snapshot', async () => {
    const { page } = makePage();
    await assert.rejects(invoke({ page, collectSnapshot: async () => snapshot([], { challengeDetected: true }) }),
      { code: 'challenge_required' });
  });
});

test('only direct posts with matching native IDs and canonical permalinks count', async () => {
  const { page } = makePage();
  const initial = snapshot([post('10001', 'Original', { quotedPost: { id: '70001', permalink: urlFor('70001') } })]);
  const fresh = snapshot([
    post('10001', 'Original', { quotedPost: { id: '70002', permalink: urlFor('70002') } }),
    post('10003', 'Mismatch', { permalink: urlFor('10004') }),
  ]);
  const result = await invoke({ page, initial, fresh, captureMs: 1450 });
  assert.equal(result.snapshot, initial);
  assert.equal(result.freshness.status, 'reveal_unverified');
  assert.equal(result.freshness.addedPrimaryCount, 0);
  assert.equal(result.freshness.changedPrimaryCount, 0);
});

test('bounds hanging probes and collectors, and keeps legacy undefined results harmless', async t => {
  await t.test('hanging probe becomes an unavailable unknown result', async () => {
    const { page, state } = makePage({ hangProbe: true });
    const initial = snapshot([post('10001', 'Original')]);
    const started = Date.now();
    const result = await invoke({ page, initial, fresh: null, captureMs: 1500 });
    assert.equal(result.snapshot, initial);
    assert.equal(result.freshness.status, 'unavailable');
    assert.equal(result.freshness.probeStatus, 'unavailable');
    assert.ok(Date.now() - started < 1000);
    assert.equal(state.activationCalls, 0);
  });
  await t.test('hanging post-activation collection remains bounded and preserves source snapshot', async () => {
    const { page } = makePage();
    const initial = snapshot([post('10001', 'Original')]);
    const started = Date.now();
    const result = await invoke({ page, initial, collectSnapshot: () => new Promise(() => {}), captureMs: 1700 });
    assert.equal(result.snapshot, initial);
    assert.equal(result.freshness.status, 'reveal_failed');
    assert.equal(result.freshness.limitation, 'phase_timeout');
    assert.ok(Date.now() - started < 1200);
  });
  await t.test('undefined probe and snapshot do not crash legacy fake pages', async () => {
    const { page } = makePage({ probe: null });
    const missingProbe = await invoke({ page, fresh: null });
    assert.equal(missingProbe.freshness.status, 'unavailable');
    const missingSnapshot = await recoverHeadlessFreshness({ page, snapshot: undefined, options: options(),
      deadlineAt: Date.now() + 1500, collectSnapshot: async () => undefined, backend: 'headless_worker',
      source: 'x', contract: xContract });
    assert.equal(missingSnapshot.snapshot, undefined);
    assert.equal(missingSnapshot.freshness.status, 'unavailable');
  });
});

test('rejects a captured post from another route even when its own post identity looks new', async () => {
  const { page } = makePage();
  await assert.rejects(invoke({ page, collectSnapshot: async () => snapshot([post('10002', 'New')], {
    url: 'https://x.com/owner/status/10002',
  }) }), { code: 'source_route_changed' });
});

test('a synthetic non-X adapter activates and compares opaque primary identities', async () => {
  const source = 'photo"feed';
  const feedUrl = 'https://photos.example/feed';
  const permalinkFor = id => `https://photos.example/items/${encodeURIComponent(id)}`;
  const nonXContract = {
    source, enabled: true, version: 'synthetic-photo-v3',
    matchesFeedURL(value) {
      try {
        const url = new URL(value);
        return url.protocol === 'https:' && url.hostname === 'photos.example' && url.pathname === '/feed';
      } catch { return false; }
    },
    primaryIdentity(value) {
      const id = typeof value?.id === 'string' ? value.id : null;
      if (!id || typeof value?.permalink !== 'string') return null;
      try {
        const url = new URL(value.permalink);
        const permalinkId = decodeURIComponent(url.pathname.slice('/items/'.length));
        return url.origin === 'https://photos.example' && url.pathname.startsWith('/items/') && permalinkId === id
          ? { id, permalink: value.permalink } : null;
      } catch { return null; }
    },
  };
  const nonXPost = (id, text) => ({ id, permalink: permalinkFor(id), text });
  const initial = { url: feedUrl, posts: [nonXPost('opaque:alpha', 'Original')] };
  const fresh = { url: feedUrl, posts: [nonXPost('opaque:alpha', 'Original'), nonXPost('opaque:beta', 'New')] };
  const { page, state } = makePage({ sourceName: source, url: `${feedUrl}?sort=recent` });
  const result = await invoke({ page, initial, fresh, source, contract: nonXContract,
    opts: options({ source, pageUrl: feedUrl }), captureMs: 1600 });
  assert.equal(result.freshness.schema, 'aku.headless-source-freshness.v1');
  assert.equal(result.freshness.source, source);
  assert.equal(result.freshness.adapterFreshnessVersion, 'synthetic-photo-v3');
  assert.equal(result.freshness.status, 'verified');
  assert.equal(result.freshness.addedPrimaryCount, 1);
  assert.equal(result.snapshot, fresh);
  assert.equal(state.probeCalls, 1);
  assert.equal(state.activationCalls, 1);
  assert.ok(state.evaluations.some(({ expression }) => expression.includes(`runtime.probe(${JSON.stringify(source)})`)));
  assert.ok(state.evaluations.some(({ expression }) => expression.includes(`runtime.activatePending(${JSON.stringify(source)}, `)));
});

test('disabled, mismatched, and malformed contracts return not_verified before page evaluation', async t => {
  const cases = [
    ['disabled', { ...xContract, enabled: false }],
    ['mismatched source', { ...xContract, source: 'other-source' }],
    ['mismatched route', { ...xContract, matchesFeedURL: () => false }],
    ['malformed identity', { ...xContract, primaryIdentity: () => ({ id: 123, permalink: '' }) }],
  ];
  for (const [name, contract] of cases) {
    await t.test(name, async () => {
      const { page, state } = makePage();
      const initial = snapshot([post('10001', 'Original')]);
      const result = await invoke({ page, initial, contract, fresh: snapshot([post('10002', 'New')]) });
      assert.equal(result.snapshot, initial);
      assert.equal(result.freshness.status, 'not_verified');
      assert.equal(result.freshness.workerStatus, 'not_verified');
      assert.equal(result.freshness.limitation, 'source_contract_unsupported');
      assert.equal(state.evaluations.length, 0);
      assert.equal(state.probeCalls, 0);
      assert.equal(state.activationCalls, 0);
    });
  }
});
