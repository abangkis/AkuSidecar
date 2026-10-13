import test from 'node:test';
import assert from 'node:assert/strict';
import { CentralSession } from './central.mjs';

const ORIGIN = 'http://127.0.0.1:43127';
const ACCOUNT = 'fixture-account-1';
const SECRET = 'never-print-fixture-cookie-secret';
const cookie = value => ({
  name: 'sid', value, domain: '127.0.0.1', path: '/', expires: -1,
  session: true, httpOnly: true, secure: false, sameSite: 'Lax',
});
const deferred = () => {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
};

function adapter(initial = cookie(SECRET)) {
  const state = {
    cookies: [structuredClone(initial)], accountId: ACCOUNT, authenticated: true,
    replaceCalls: 0, clearCalls: 0, refreshCalls: 0, failReplace: false,
    rotateOnRefresh: cookie('rotated-fixture-cookie'), refreshGate: null,
  };
  const api = {
    async probe() {
      return { authenticated: state.authenticated,
        identity: state.authenticated ? { accountId: state.accountId } : null };
    },
    async exportCookies() { return { cookies: structuredClone(state.cookies) }; },
    async replaceCookies({ cookies }) {
      state.replaceCalls += 1;
      if (state.failReplace) throw new Error(`private value ${SECRET}`);
      state.cookies = structuredClone(cookies);
      if (state.cookies.length > 0) state.authenticated = true;
    },
    async clear() { state.clearCalls += 1; state.cookies = []; state.authenticated = false; },
    async refresh() {
      state.refreshCalls += 1;
      if (state.refreshGate) await state.refreshGate.promise;
      state.cookies = [structuredClone(state.rotateOnRefresh)];
    },
  };
  return { api, state };
}

function makeSession({ reader = adapter(), collector = adapter(), timeoutMs = 200,
  bootstrapClientId = 'reader' } = {}) {
  return {
    reader, collector,
    session: new CentralSession({
      origin: ORIGIN, expectedAccountId: ACCOUNT,
      authority: collector.api, refreshClientId: 'collector', bootstrapClientId,
      clients: { reader: reader.api, collector: collector.api }, timeoutMs,
    }),
  };
}

async function ready(session, cookies = [cookie(SECRET)]) {
  const result = await session.initialize({ cookies, accountId: ACCOUNT });
  assert.equal(result.ok, true, JSON.stringify(result));
}

test('initialize publishes a loopback-only in-memory session and admits a managed use', async () => {
  const { session } = makeSession();
  await ready(session);
  const lease = session.lease('reader');
  assert.ok(lease);
  assert.deepEqual(await session.use('reader', lease, () => 'fixture response'), {
    ok: true, value: 'fixture response',
  });
  assert.equal(session.status().state, 'READY');
  assert.equal(JSON.stringify(session.status()).includes(SECRET), false);
});

test('bootstrap reader seeds an initially unauthenticated collector before authority verification', async () => {
  const reader = adapter();
  const collector = adapter();
  collector.state.authenticated = false;
  collector.state.cookies = [];
  const { session } = makeSession({ reader, collector, bootstrapClientId: 'reader' });
  await ready(session);
  assert.equal(collector.state.authenticated, true);
  assert.ok(session.lease('collector'));
  assert.equal(session.status().state, 'READY');
});

test('parallel refreshes coalesce, drain admitted work, and reject old or stale leases', async () => {
  const { session, collector } = makeSession();
  await ready(session);
  const oldLease = session.lease('reader');
  const useGate = deferred();
  const work = session.use('reader', oldLease, () => useGate.promise);
  const refreshGate = deferred();
  collector.state.refreshGate = refreshGate;
  const before = session.status();
  const first = session.refresh({ expectedVersion: before.version, expectedEpoch: before.epoch });
  const second = session.refresh({ expectedVersion: before.version, expectedEpoch: before.epoch });
  assert.equal(session.status().state, 'REFRESHING');
  assert.equal(collector.state.refreshCalls, 0);
  useGate.resolve('work finished');
  assert.deepEqual(await work, { ok: false, code: 'LEASE_STALE' });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(collector.state.refreshCalls, 1);
  refreshGate.resolve();
  const [a, b] = await Promise.all([first, second]);
  assert.equal(a.ok, true);
  assert.equal(b, a);
  assert.equal(session.status().version, before.version + 1);
  let called = false;
  assert.deepEqual(await session.use('reader', oldLease, () => { called = true; }), {
    ok: false, code: 'LEASE_STALE',
  });
  assert.equal(called, false);
  assert.deepEqual(await session.refresh({ expectedVersion: before.version, expectedEpoch: before.epoch }), {
    ok: false, code: 'STALE_UPDATE',
  });
});

test('rotation from an admitted request wins over a concurrently requested refresh', async () => {
  const { session, reader, collector } = makeSession();
  await ready(session);
  const lease = session.lease('reader');
  const gate = deferred();
  const work = session.use('reader', lease, async () => {
    await gate.promise;
    reader.state.cookies = [cookie('response-rotated-cookie')];
    return 'rotated response';
  });
  const version = session.status();
  const refresh = session.refresh({ expectedVersion: version.version, expectedEpoch: version.epoch });
  gate.resolve();
  assert.deepEqual(await work, { ok: false, code: 'UNEXPECTED_ROTATION' });
  assert.deepEqual(await refresh, { ok: false, code: 'AUTH_EPOCH_CHANGED' });
  assert.equal(collector.state.refreshCalls, 0);
  assert.equal(session.status().state, 'CONFLICT');
});

test('foreign session leases cannot run callbacks', async () => {
  const first = makeSession();
  const second = makeSession();
  await ready(first.session);
  await ready(second.session);
  let called = false;
  assert.deepEqual(await second.session.use('reader', first.session.lease('reader'), () => { called = true; }), {
    ok: false, code: 'LEASE_STALE',
  });
  assert.equal(called, false);
});

test('revoke fences synchronously while a managed request is still running', async () => {
  const { session, reader, collector } = makeSession();
  await ready(session);
  const lease = session.lease('reader');
  const gate = deferred();
  const work = session.use('reader', lease, () => gate.promise);
  const revoked = session.revoke();
  assert.equal(session.status().state, 'REVOKED');
  assert.equal(session.status().epoch, lease.epoch + 1);
  gate.resolve('late result');
  assert.deepEqual(await work, { ok: false, code: 'LEASE_STALE' });
  assert.equal((await revoked).ok, true);
  assert.ok(reader.state.clearCalls > 0);
  assert.ok(collector.state.clearCalls > 0);
  assert.equal(session.lease('reader'), null);
});

test('failed fanout admits no client and never exposes adapter exception text', async () => {
  const { session, reader } = makeSession();
  await ready(session);
  reader.state.failReplace = true;
  const old = session.status();
  const result = await session.refresh({ expectedVersion: old.version, expectedEpoch: old.epoch });
  assert.equal(result.ok, false);
  assert.equal(session.status().state, 'NEEDS_USER');
  assert.equal(session.lease('collector'), null);
  const statusText = JSON.stringify(session.status());
  assert.equal(statusText.includes(SECRET), false);
  assert.equal(statusText.includes('private value'), false);
  assert.equal(JSON.stringify(result).includes(SECRET), false);
  assert.equal(JSON.stringify(result).includes('private value'), false);
});

test('detached reader preserves the vault and reattaches at the newest version', async () => {
  const { session, reader } = makeSession();
  await ready(session);
  const stale = session.lease('reader');
  assert.deepEqual(await session.detach('reader'), { ok: true });
  assert.equal(session.lease('reader'), null);
  assert.equal(reader.state.clearCalls, 1);
  const before = session.status();
  assert.equal((await session.refresh({ expectedVersion: before.version, expectedEpoch: before.epoch })).ok, true);
  assert.equal(session.status().clients.reader.attached, false);
  assert.equal((await session.attach('reader')).ok, true);
  const current = session.lease('reader');
  assert.equal(current.version, before.version + 1);
  assert.notDeepEqual(current, stale);
  assert.deepEqual(reader.state.cookies.map(item => item.value), ['rotated-fixture-cookie']);
});

test('managed cookie rotation fails closed and rejects the triggering result', async () => {
  const { session, reader } = makeSession();
  await ready(session);
  const lease = session.lease('reader');
  const result = await session.use('reader', lease, () => {
    reader.state.cookies = [cookie('opaque-new-token')];
    return { status: 200 };
  });
  assert.deepEqual(result, { ok: false, code: 'UNEXPECTED_ROTATION' });
  assert.equal(session.status().state, 'CONFLICT');
  assert.equal(session.lease('collector'), null);
  assert.equal(JSON.stringify(session.status()).includes('opaque-new-token'), false);
});

test('callback throws are sanitized after cookie observation, including a response rotation', async t => {
  await t.test('stable cookies', async () => {
    const { session } = makeSession();
    await ready(session);
    const result = await session.use('reader', session.lease('reader'), () => {
      throw new Error(`exception contains ${SECRET}`);
    });
    assert.deepEqual(result, { ok: false, code: 'USE_FAILED' });
    assert.equal(session.status().state, 'READY');
    assert.equal(JSON.stringify(result).includes(SECRET), false);
  });
  await t.test('rotation before exception', async () => {
    const { session, reader } = makeSession();
    await ready(session);
    const result = await session.use('reader', session.lease('reader'), () => {
      reader.state.cookies = [cookie('rotated-before-throw')];
      throw new Error(`exception contains ${SECRET}`);
    });
    assert.deepEqual(result, { ok: false, code: 'UNEXPECTED_ROTATION' });
    assert.equal(session.status().state, 'CONFLICT');
  });
});

test('notification of an unowned browser rotation fences every lease immediately', async () => {
  const { session } = makeSession();
  await ready(session);
  assert.deepEqual(session.notifyUnexpectedRotation('reader', { cookies: [cookie('do-not-log')] }), {
    ok: false, code: 'UNEXPECTED_ROTATION',
  });
  assert.equal(session.status().state, 'CONFLICT');
  assert.equal(session.lease('reader'), null);
  assert.equal(JSON.stringify(session.status()).includes('do-not-log'), false);
});

test('authentication loss and identity mismatch after a managed request fail closed', async t => {
  await t.test('401 probe', async () => {
    const { session, reader } = makeSession();
    await ready(session);
    const lease = session.lease('reader');
    const result = await session.use('reader', lease, () => {
      reader.state.authenticated = false;
      return 'must not return';
    });
    assert.deepEqual(result, { ok: false, code: 'CLIENT_AUTH_REQUIRED' });
    assert.equal(session.status().state, 'NEEDS_USER');
    assert.equal(session.lease('reader'), null);
  });
  await t.test('different account', async () => {
    const { session, reader } = makeSession();
    await ready(session);
    const lease = session.lease('reader');
    const result = await session.use('reader', lease, () => {
      reader.state.accountId = 'other-fixture-account';
      return 'must not return';
    });
    assert.deepEqual(result, { ok: false, code: 'CLIENT_IDENTITY_MISMATCH' });
    assert.equal(session.status().state, 'NEEDS_USER');
    assert.equal(session.lease('collector'), null);
  });
});

test('invalid fixture cookies and non-loopback origins fail without exposing cookie material', async () => {
  assert.throws(() => new CentralSession({ origin: 'http://localhost:43127',
    expectedAccountId: ACCOUNT, authority: adapter().api, refreshClientId: 'reader',
    clients: { reader: adapter().api } }), /FIXTURE_ORIGIN_NOT_ALLOWED/);
  const { session } = makeSession();
  const result = await session.initialize({ accountId: ACCOUNT, cookies: [cookie(SECRET), {
    ...cookie('private-outside'), domain: 'example.com',
  }] });
  assert.equal(result.ok, false);
  assert.equal(result.code, 'COOKIE_OUTSIDE_FIXTURE');
  assert.equal(session.status().state, 'NEEDS_USER');
  assert.equal(JSON.stringify(result).includes(SECRET), false);
  assert.equal(JSON.stringify(session.status()).includes('private-outside'), false);
});

test('a refresh request with stale compare-and-swap fields does not call authority', async () => {
  const { session, collector } = makeSession();
  await ready(session);
  const calls = collector.state.refreshCalls;
  assert.deepEqual(await session.refresh({ expectedVersion: 99, expectedEpoch: 0 }), {
    ok: false, code: 'STALE_UPDATE',
  });
  assert.equal(collector.state.refreshCalls, calls);
});

test('partitioned cookie fingerprints ignore export order while retaining partition identity', async () => {
  const first = { ...cookie('partition-one'), partitionKey: {
    topLevelSite: 'http://127.0.0.1', hasCrossSiteAncestor: false,
  } };
  const second = { ...cookie('partition-two'), partitionKey: {
    topLevelSite: 'http://localhost', hasCrossSiteAncestor: false,
  } };
  const reader = adapter(first);
  const collector = adapter(first);
  reader.state.cookies = [first, second];
  collector.state.cookies = [first, second];
  const { session } = makeSession({ reader, collector });
  await ready(session, [first, second]);
  const lease = session.lease('reader');
  assert.deepEqual(await session.use('reader', lease, () => reader.state.cookies.reverse().length), {
    ok: true, value: 2,
  });
});

test('timed-out mutation blocks cleanup completion until settled, then revoke retries clears', async () => {
  const { session, collector, reader } = makeSession({ timeoutMs: 30 });
  await ready(session);
  const gate = deferred();
  collector.state.refreshGate = gate;
  const before = session.status();
  const refresh = await session.refresh({ expectedVersion: before.version, expectedEpoch: before.epoch });
  assert.equal(refresh.ok, false);
  assert.equal(session.status().state, 'NEEDS_USER');

  const firstRevoke = await session.revoke();
  assert.deepEqual(firstRevoke, { ok: false, code: 'PENDING_OPERATION_TIMEOUT' });
  assert.equal(collector.state.clearCalls, 0);
  assert.equal(reader.state.clearCalls, 0);

  gate.resolve();
  await new Promise(resolve => setImmediate(resolve));
  const secondRevoke = await session.revoke();
  assert.equal(secondRevoke.ok, true);
  assert.ok(collector.state.clearCalls > 0);
  assert.ok(reader.state.clearCalls > 0);
  assert.equal(session.status().state, 'REVOKED');
});

test('revoke reports a failed clear and retries cleanup instead of claiming success', async () => {
  const { session, reader } = makeSession();
  await ready(session);
  const clear = reader.api.clear;
  let fail = true;
  reader.api.clear = async (...args) => {
    if (fail) {
      fail = false;
      throw new Error(`clear failed with ${SECRET}`);
    }
    return clear(...args);
  };
  const first = await session.revoke();
  assert.deepEqual(first, { ok: false, code: 'CLEAR_FAILED' });
  assert.equal(session.status().state, 'REVOKED');
  assert.equal(JSON.stringify(first).includes(SECRET), false);
  const second = await session.revoke();
  assert.equal(second.ok, true);
  assert.ok(reader.state.clearCalls > 0);
});
