import test from 'node:test';
import assert from 'node:assert/strict';
import { AuthSessionBroker } from './broker.mjs';

const origin = 'http://127.0.0.1:43129';
const now = Date.UTC(2026, 9, 13, 12, 0, 0);
const secret = 'fixture-cookie-secret-do-not-report';

function cookie(overrides = {}) {
  return {
    name: 'session_id',
    value: secret,
    domain: '127.0.0.1',
    path: '/',
    expires: -1,
    session: true,
    httpOnly: true,
    secure: true,
    sameSite: 'Lax',
    partitionKey: { topLevelSite: 'http://127.0.0.1', hasCrossSiteAncestor: false },
    partitionKeyOpaque: false,
    size: 32,
    priority: 'Medium',
    sourceScheme: 'Secure',
    sourcePort: 443,
    ...overrides,
  };
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function harness(options = {}) {
  const calls = {
    ownerProbes: 0,
    exports: 0,
    creates: [],
    seeds: [],
    probes: [],
    disposals: [],
    order: [],
  };
  let accountId = options.accountId ?? 'fixture-account-a';
  let ownerAuthenticated = options.ownerAuthenticated ?? true;
  let derivedAccountId = options.derivedAccountId;
  let ownerProbeHook = options.ownerProbe;
  let seedHook = options.seed;
  let disposeHook = options.dispose;
  let cookieSnapshot = options.cookies ?? [cookie()];
  let contextId = 0;
  const owner = {
    async probe(args) {
      calls.ownerProbes += 1;
      calls.order.push('owner.probe');
      if (ownerProbeHook) return ownerProbeHook(args, calls.ownerProbes);
      return { authenticated: ownerAuthenticated, identity: { accountId } };
    },
    async exportCookies(args) {
      calls.exports += 1;
      calls.order.push('owner.exportCookies');
      assert.equal(args.accountId, accountId);
      return { cookies: cookieSnapshot };
    },
  };
  const derived = {
    async create(args) {
      calls.order.push('derived.create');
      const context = { id: ++contextId, origin: args.origin };
      calls.creates.push(context);
      return context;
    },
    async seed(args) {
      calls.order.push('derived.seed');
      calls.seeds.push({ context: args.context, origin: args.origin, cookies: args.cookies });
      if (seedHook) return seedHook(args);
    },
    async probe(args) {
      calls.order.push('derived.probe');
      calls.probes.push(args.context);
      return {
        authenticated: true,
        identity: { accountId: derivedAccountId ?? accountId },
      };
    },
    async dispose(args) {
      calls.order.push('derived.dispose');
      calls.disposals.push(args.context);
      if (disposeHook) return disposeHook(args);
    },
  };
  const broker = new AuthSessionBroker({
    fixtureOrigins: [origin],
    owner,
    derived,
    now: options.now ?? (() => now),
  });
  return {
    broker,
    calls,
    setAccountId(value) { accountId = value; },
    setOwnerAuthenticated(value) { ownerAuthenticated = value; },
    setDerivedAccountId(value) { derivedAccountId = value; },
    setOwnerProbe(value) { ownerProbeHook = value; },
    setDispose(value) { disposeHook = value; },
    setCookieSnapshot(value) { cookieSnapshot = value; },
  };
}

test('bootstrap verifies the same account and copies only faithful fixture CookieParam fields', async () => {
  const { broker, calls } = harness();
  const result = await broker.bootstrap({ origin });

  assert.equal(result.ok, true);
  assert.equal(result.code, 'VERIFIED');
  assert.equal(result.lease.accountId, 'fixture-account-a');
  assert.equal(broker.admitResult(result.lease), true);
  assert.equal(broker.getContext(result.lease), calls.creates[0]);
  assert.deepEqual(calls.seeds[0].cookies[0], {
    name: 'session_id',
    value: secret,
    domain: '127.0.0.1',
    path: '/',
    expires: -1,
    httpOnly: true,
    secure: true,
    sameSite: 'Lax',
    partitionKey: { topLevelSite: 'http://127.0.0.1', hasCrossSiteAncestor: false },
  });
  assert.equal('session' in calls.seeds[0].cookies[0], false);
  assert.equal('size' in calls.seeds[0].cookies[0], false);
  assert.equal('sourcePort' in calls.seeds[0].cookies[0], false);
  assert.deepEqual(calls.order, [
    'owner.probe', 'owner.exportCookies', 'derived.create', 'derived.seed',
    'derived.probe', 'owner.probe',
  ]);
  assert.deepEqual(broker.status(), {
    state: 'VERIFIED', generation: 1, authEpoch: 1, code: null,
    hasContext: true, undisposedContextCount: 0,
  });
});

test('fixture origins must be exact HTTP loopback origins and cookie domains stay on that host', async () => {
  for (const disallowed of ['https://127.0.0.1:43129', 'http://example.com', 'http://127.0.0.1:43129/path']) {
    assert.throws(() => new AuthSessionBroker({ fixtureOrigins: [disallowed] }), {
      code: 'FIXTURE_ORIGIN_NOT_ALLOWED',
    });
  }
  assert.throws(() => new AuthSessionBroker({
    fixtureOrigins: ['https://example.com'],
    owner: { probe() {}, exportCookies() {} },
    derived: { create() {}, seed() {}, probe() {}, dispose() {} },
  }), { code: 'FIXTURE_ORIGIN_NOT_ALLOWED' });

  const foreignCookie = harness({ cookies: [cookie({ domain: 'example.com' })] });
  const rejected = await foreignCookie.broker.bootstrap({ origin });
  assert.equal(rejected.ok, false);
  assert.equal(rejected.code, 'COOKIE_OUTSIDE_FIXTURE');
  assert.equal(foreignCookie.calls.creates.length, 0);

  const exactAllowlist = harness();
  const otherPort = await exactAllowlist.broker.bootstrap({ origin: 'http://127.0.0.1:43130' });
  assert.deepEqual(otherPort, { ok: false, code: 'FIXTURE_ORIGIN_NOT_ALLOWED' });
  assert.equal(exactAllowlist.broker.status().state, 'UNSEEDED');
});

test('expired cookies and corrupt cookie metadata fail closed before derived context creation', async () => {
  const expired = harness({ cookies: [cookie({ expires: now / 1000 - 1, session: false })] });
  const expiredResult = await expired.broker.bootstrap({ origin });
  assert.deepEqual(expiredResult, { ok: false, code: 'COOKIE_EXPIRED' });
  assert.equal(expired.broker.status().state, 'NEEDS_USER');
  assert.equal(expired.calls.creates.length, 0);

  const corruptions = [
    { expires: 'later' },
    { path: 'relative' },
    { httpOnly: 1 },
    { sameSite: 'Untrusted' },
    { session: true, expires: now / 1000 + 1000 },
    { secure: false, sameSite: 'None' },
  ];
  for (const change of corruptions) {
    const item = harness({ cookies: [cookie(change)] });
    const result = await item.broker.bootstrap({ origin });
    assert.equal(result.ok, false, JSON.stringify(change));
    assert.equal(result.code, 'COOKIE_SNAPSHOT_INVALID', JSON.stringify(change));
    assert.equal(item.calls.creates.length, 0);
  }
});

test('partition metadata is preserved only for allowlisted loopback schemeful sites', async () => {
  const accepted = harness({ cookies: [cookie({
    partitionKey: { topLevelSite: 'http://localhost', hasCrossSiteAncestor: true },
  })] });
  const acceptedResult = await accepted.broker.bootstrap({ origin });
  assert.equal(acceptedResult.ok, true);
  assert.deepEqual(accepted.calls.seeds[0].cookies[0].partitionKey, {
    topLevelSite: 'http://localhost', hasCrossSiteAncestor: true,
  });

  for (const partitionChange of [
    { partitionKey: 'https://attacker.example' },
    { partitionKey: { topLevelSite: 'http://127.0.0.1:43129', hasCrossSiteAncestor: false } },
    { partitionKeyOpaque: true },
  ]) {
    const rejected = harness({ cookies: [cookie(partitionChange)] });
    const result = await rejected.broker.bootstrap({ origin });
    assert.equal(result.ok, false);
    assert.equal(result.code, 'PARTITION_KEY_OUTSIDE_FIXTURE');
    assert.equal(rejected.calls.creates.length, 0);
  }
});

test('bootstrap fails when owner is unauthenticated or the derived account differs', async () => {
  const ownerUnauthenticated = harness({ ownerAuthenticated: false });
  assert.deepEqual(await ownerUnauthenticated.broker.bootstrap({ origin }), {
    ok: false, code: 'OWNER_AUTH_REQUIRED',
  });
  assert.equal(ownerUnauthenticated.calls.exports, 0);

  const mismatch = harness({ derivedAccountId: 'fixture-account-b' });
  const result = await mismatch.broker.bootstrap({ origin });
  assert.deepEqual(result, { ok: false, code: 'DERIVED_IDENTITY_MISMATCH' });
  assert.equal(mismatch.broker.status().state, 'NEEDS_USER');
  assert.equal(mismatch.broker.status().hasContext, false);
  assert.deepEqual(mismatch.calls.disposals, [mismatch.calls.creates[0]]);
});

test('same-account bootstrap fences old results immediately and replaces the old context', async () => {
  const { broker, calls } = harness();
  const first = await broker.bootstrap({ origin });
  const secondPending = broker.bootstrap({ origin, expectedAccountId: first.lease.accountId });

  assert.equal(broker.admitResult(first.lease), false);
  assert.equal(broker.getContext(first.lease), null);
  const second = await secondPending;
  assert.equal(second.ok, true);
  assert.ok(second.lease.generation > first.lease.generation);
  assert.ok(second.lease.authEpoch > first.lease.authEpoch);
  assert.equal(broker.admitResult(first.lease), false);
  assert.equal(broker.admitResult(second.lease), true);
  assert.deepEqual(calls.disposals, [calls.creates[0]]);
});

test('broker instance identity prevents a fresh instance from admitting an old lease', async () => {
  const previous = harness();
  const priorLogin = await previous.broker.bootstrap({ origin });
  const fresh = harness();
  const freshLogin = await fresh.broker.bootstrap({ origin });

  assert.equal(priorLogin.lease.generation, freshLogin.lease.generation);
  assert.equal(priorLogin.lease.authEpoch, freshLogin.lease.authEpoch);
  assert.equal(priorLogin.lease.accountId, freshLogin.lease.accountId);
  assert.equal(priorLogin.lease.origin, freshLogin.lease.origin);
  assert.notEqual(priorLogin.lease.brokerId, freshLogin.lease.brokerId);
  assert.equal(previous.broker.admitResult(priorLogin.lease), true);
  assert.equal(fresh.broker.admitResult(priorLogin.lease), false);
});

test('a newer bootstrap cancels an older pending probe before export or context creation', async () => {
  const firstProbe = deferred();
  const firstProbeStarted = deferred();
  const h = harness({ ownerProbe: (_args, count) => {
    if (count === 1) {
      firstProbeStarted.resolve();
      return firstProbe.promise;
    }
    return { authenticated: true, identity: { accountId: 'fixture-account-a' } };
  } });
  const earlier = h.broker.bootstrap({ origin });
  await firstProbeStarted.promise;
  const later = h.broker.bootstrap({ origin });
  firstProbe.resolve({ authenticated: true, identity: { accountId: 'fixture-account-a' } });

  const [earlierResult, laterResult] = await Promise.all([earlier, later]);
  assert.deepEqual(earlierResult, { ok: false, code: 'AUTH_EPOCH_CHANGED' });
  assert.equal(laterResult.ok, true);
  assert.equal(h.calls.exports, 1);
  assert.equal(h.calls.creates.length, 1);
});

test('revocation fences an in-flight bootstrap and disposes any context created after the fence', async () => {
  const gate = deferred();
  const seedStarted = deferred();
  const h = harness({ seed: async () => { seedStarted.resolve(); await gate.promise; } });
  const pending = h.broker.bootstrap({ origin });
  await seedStarted.promise;
  const revoking = h.broker.revoke();
  assert.equal(h.broker.status().state, 'REVOKED');
  gate.resolve();

  const [bootstrapResult, revokeResult] = await Promise.all([pending, revoking]);
  assert.deepEqual(bootstrapResult, { ok: false, code: 'AUTH_EPOCH_CHANGED' });
  assert.deepEqual(revokeResult, { ok: true, code: 'REVOKED' });
  assert.equal(h.broker.status().state, 'REVOKED');
  assert.equal(h.broker.status().hasContext, false);
  assert.deepEqual(h.calls.disposals, [h.calls.creates[0]]);
});

test('revocation invalidates already admitted same-account results before disposal completes', async () => {
  const { broker, calls } = harness();
  const result = await broker.bootstrap({ origin });
  const revoking = broker.revoke();
  assert.equal(broker.admitResult(result.lease), false);
  assert.equal(broker.getContext(result.lease), null);
  await revoking;
  assert.deepEqual(calls.disposals, [calls.creates[0]]);
  assert.equal(broker.status().state, 'REVOKED');
});

test('stale recovery reseeds once per episode and requires the original owner account', async () => {
  const h = harness();
  const first = await h.broker.bootstrap({ origin });
  const episode = h.broker.markStale(first.lease);
  assert.ok(episode);
  assert.equal(h.broker.admitResult(first.lease), false);
  assert.equal(h.broker.markStale(first.lease), null);

  const recovered = await h.broker.recoverStale({ episode });
  assert.equal(recovered.ok, true);
  assert.equal(h.calls.creates.length, 2);
  assert.equal(h.broker.admitResult(first.lease), false);
  assert.equal(h.broker.admitResult(recovered.lease), true);
  assert.deepEqual(await h.broker.recoverStale({ episode }), {
    ok: false, code: 'RECOVERY_NOT_AVAILABLE',
  });
  assert.equal(h.calls.creates.length, 2);

  const h2 = harness();
  const original = await h2.broker.bootstrap({ origin });
  const nextEpisode = h2.broker.markStale(original.lease);
  h2.setAccountId('fixture-account-b');
  const switched = await h2.broker.recoverStale({ episode: nextEpisode });
  assert.deepEqual(switched, { ok: false, code: 'ACCOUNT_MISMATCH' });
  assert.equal(h2.broker.status().state, 'NEEDS_USER');
  assert.equal(h2.calls.creates.length, 1);
});

test('public results and status never include cookie values or adapter exception text', async () => {
  const h = harness({ ownerProbe: async () => { throw new Error(`failure ${secret}`); } });
  const result = await h.broker.bootstrap({ origin });
  const publicData = JSON.stringify({ result, status: h.broker.status() });
  assert.equal(result.code, 'OWNER_PROBE_FAILED');
  assert.equal(publicData.includes(secret), false);
  assert.equal(publicData.includes('failure'), false);
  assert.deepEqual(h.broker.status(), {
    state: 'NEEDS_USER', generation: 1, authEpoch: 1,
    code: 'OWNER_PROBE_FAILED', hasContext: false, undisposedContextCount: 0,
  });
});

test('failed revoke reports cleanup failure, fences leases, and retries orphan disposal later', async () => {
  const h = harness();
  const boot = await h.broker.bootstrap({ origin });
  h.setDispose(async () => { throw new Error(`disposal ${secret}`); });

  const failed = await h.broker.revoke();
  assert.deepEqual(failed, { ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
  assert.equal(h.broker.admitResult(boot.lease), false);
  assert.deepEqual(h.broker.status(), {
    state: 'NEEDS_USER', generation: 2, authEpoch: 2,
    code: 'CONTEXT_DISPOSAL_FAILED', hasContext: false, undisposedContextCount: 1,
  });

  h.setDispose(undefined);
  assert.deepEqual(await h.broker.revoke(), { ok: true, code: 'REVOKED' });
  assert.deepEqual(h.broker.status(), {
    state: 'REVOKED', generation: 3, authEpoch: 3,
    code: 'REVOKED', hasContext: false, undisposedContextCount: 0,
  });
  assert.deepEqual(h.calls.disposals, [h.calls.creates[0], h.calls.creates[0]]);
});

test('failed old-context disposal blocks bootstrap allocation until a later cleanup pass succeeds', async () => {
  const h = harness();
  const first = await h.broker.bootstrap({ origin });
  h.setDispose(async () => { throw new Error(secret); });

  const blocked = await h.broker.bootstrap({ origin });
  assert.deepEqual(blocked, { ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
  assert.equal(h.calls.creates.length, 1);
  assert.equal(h.broker.status().state, 'NEEDS_USER');
  assert.equal(h.broker.status().undisposedContextCount, 1);
  assert.equal(h.broker.admitResult(first.lease), false);

  h.setDispose(undefined);
  const retried = await h.broker.bootstrap({ origin });
  assert.equal(retried.ok, true);
  assert.equal(h.calls.creates.length, 2);
  assert.equal(h.broker.status().undisposedContextCount, 0);
  assert.equal(h.broker.admitResult(retried.lease), true);
  assert.deepEqual(h.calls.disposals, [h.calls.creates[0], h.calls.creates[0]]);
});

test('failed stale-reseed disposal blocks context allocation and a foreign broker cannot reuse an episode', async () => {
  const h = harness();
  const first = await h.broker.bootstrap({ origin });
  const episode = h.broker.markStale(first.lease);
  h.setDispose(async () => { throw new Error(secret); });

  const blocked = await h.broker.recoverStale({ episode });
  assert.deepEqual(blocked, { ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
  assert.equal(h.calls.creates.length, 1);
  assert.equal(h.broker.status().state, 'NEEDS_USER');
  assert.equal(h.broker.status().undisposedContextCount, 1);

  h.setDispose(undefined);
  const retried = await h.broker.bootstrap({ origin, expectedAccountId: first.lease.accountId });
  assert.equal(retried.ok, true);
  assert.equal(h.calls.creates.length, 2);
  assert.equal(h.broker.status().undisposedContextCount, 0);

  const other = harness();
  const otherBoot = await other.broker.bootstrap({ origin });
  const foreignEpisode = other.broker.markStale(otherBoot.lease);
  const local = harness();
  const localBoot = await local.broker.bootstrap({ origin });
  const localEpisode = local.broker.markStale(localBoot.lease);
  assert.notEqual(foreignEpisode.brokerId, localEpisode.brokerId);
  assert.deepEqual(await local.broker.recoverStale({ episode: foreignEpisode }), {
    ok: false, code: 'RECOVERY_NOT_AVAILABLE',
  });
  assert.equal(local.calls.creates.length, 1);
  assert.equal((await local.broker.recoverStale({ episode: localEpisode })).ok, true);
});
