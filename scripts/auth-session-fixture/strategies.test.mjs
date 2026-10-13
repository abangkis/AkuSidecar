import test from 'node:test';
import assert from 'node:assert/strict';
import { SessionStrategyController } from './strategies.mjs';

const ORIGIN = 'http://127.0.0.1:43187';
const ACCOUNT = 'fixture-account-strategy';
const SECRET = 'strategy-fixture-cookie-secret-never-report';

function cookie(value = SECRET) {
  return {
    name: 'sid', value, domain: '127.0.0.1', path: '/', expires: -1,
    session: true, httpOnly: true, secure: false, sameSite: 'Lax',
  };
}

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function harness({ timeoutMs = 300 } = {}) {
  const stats = { refreshOwners: [], readerProbeGate: null, replaceGate: null, replaceEntered: null,
    failClear: new Set(), clearCalls: [], replaceCalls: [] };
  const states = {};
  const clients = {};
  for (const id of ['reader', 'collector']) {
    states[id] = { cookies: [cookie()], accountId: ACCOUNT, probeGate: null };
    clients[id] = {
      async probe() {
        const gate = states[id].probeGate ?? (id === 'reader' ? stats.readerProbeGate : null);
        if (gate) await gate.promise;
        const authenticated = states[id].cookies.length > 0;
        return { authenticated, identity: authenticated ? { accountId: states[id].accountId } : null };
      },
      async exportCookies() { return { cookies: structuredClone(states[id].cookies) }; },
      async replaceCookies({ cookies: next }) {
        stats.replaceCalls.push(id);
        if (stats.replaceEntered && !stats.replaceEntered.done) {
          stats.replaceEntered.done = true;
          stats.replaceEntered.resolve(id);
        }
        if (stats.replaceGate) await stats.replaceGate.promise;
        states[id].cookies = structuredClone(next);
      },
      async clear() {
        stats.clearCalls.push(id);
        if (stats.failClear.has(id)) {
          stats.failClear.delete(id);
          throw new Error(`adapter failure contains ${SECRET}`);
        }
        states[id].cookies = [];
      },
      async refresh() {
        stats.refreshOwners.push(id);
        states[id].cookies = [cookie(`${id}-rotated-fixture-cookie`)];
      },
    };
  }
  const controller = new SessionStrategyController({
    origin: ORIGIN, expectedAccountId: ACCOUNT, clients, timeoutMs,
  });
  return { controller, clients, states, stats };
}

async function verifyPair(controller) {
  const results = await Promise.all(['reader', 'collector'].map(client =>
    controller.use(client, controller.lease(client), async context => ({
      status: 200, account: context.accountId, client,
    })),
  ));
  assert.equal(results.every(result => result.ok && result.value.status === 200 &&
    result.value.account === ACCOUNT), true);
}

test('uses the real broker and central session wrappers for bidirectional switching and owned refresh', async () => {
  const { controller, stats } = harness();
  try {
    assert.equal((await controller.start('reader-owned')).ok, true);
    await verifyPair(controller);
    assert.equal((await controller.refresh()).ok, true);
    assert.equal(stats.refreshOwners.at(-1), 'reader');

    assert.equal((await controller.switchTo('central-vault')).ok, true);
    await verifyPair(controller);
    assert.equal((await controller.refresh()).ok, true);
    assert.equal(stats.refreshOwners.at(-1), 'collector');

    assert.equal((await controller.switchTo('reader-owned')).ok, true);
    await verifyPair(controller);
    assert.equal(stats.refreshOwners.length, 2);
    assert.equal(JSON.stringify(controller.status()).includes(SECRET), false);
  } finally {
    await controller.revoke();
  }
});

test('switch fences new work synchronously, drains admitted work, and rejects its old result', async () => {
  const { controller } = harness();
  try {
    assert.equal((await controller.start('reader-owned')).ok, true);
    const entered = deferred();
    const finish = deferred();
    const work = controller.use('collector', controller.lease('collector'), async () => {
      entered.resolve();
      await finish.promise;
      return 'late fixture response';
    });
    await entered.promise;

    const transition = controller.switchTo('central-vault');
    assert.equal(controller.status().state, 'SWITCHING');
    assert.deepEqual(await controller.use('reader', controller.lease('reader'), () => 'blocked'), {
      ok: false, code: 'LEASE_STALE',
    });
    finish.resolve();
    const [oldResult, switched] = await Promise.all([work, transition]);
    assert.equal(switched.ok, true);
    assert.deepEqual(oldResult, { ok: false, code: 'LEASE_STALE' });
    await verifyPair(controller);
  } finally {
    await controller.revoke();
  }
});

test('unknown and same-strategy selections leave the current lease intact', async () => {
  const { controller } = harness();
  try {
    assert.equal((await controller.start('central-vault')).ok, true);
    const lease = controller.lease('reader');
    assert.deepEqual(await controller.switchTo('unregistered'), { ok: false, code: 'STRATEGY_UNKNOWN' });
    assert.equal(controller.status().activeStrategy, 'central-vault');
    assert.equal((await controller.use('reader', lease, () => 'still-admitted')).value, 'still-admitted');
    assert.deepEqual(await controller.switchTo('central-vault'), { ok: true, code: 'UNCHANGED' });
    assert.equal((await controller.use('reader', lease, () => 'still-admitted')).ok, true);
  } finally {
    await controller.revoke();
  }
});

test('prototype property names are not strategy IDs and cannot pass the allowlist', async () => {
  const { controller } = harness();
  assert.deepEqual(await controller.start('toString'), { ok: false, code: 'STRATEGY_UNKNOWN' });
  assert.deepEqual(await controller.start('constructor'), { ok: false, code: 'STRATEGY_UNKNOWN' });
  assert.deepEqual(await controller.start('__proto__'), { ok: false, code: 'STRATEGY_UNKNOWN' });

  assert.equal((await controller.start('reader-owned')).ok, true);
  const lease = controller.lease('reader');
  for (const value of ['toString', 'constructor', '__proto__']) {
    assert.deepEqual(await controller.switchTo(value), { ok: false, code: 'STRATEGY_UNKNOWN' });
  }
  assert.equal((await controller.use('reader', lease, () => 'still-active')).value, 'still-active');
  await controller.revoke();
});

test('failed old-strategy cleanup blocks publication and a retry completes the handoff', async () => {
  const { controller, stats } = harness();
  try {
    assert.equal((await controller.start('reader-owned')).ok, true);
    stats.failClear.add('collector');
    const blocked = await controller.switchTo('central-vault');
    assert.deepEqual(blocked, { ok: false, code: 'CONTEXT_DISPOSAL_FAILED' });
    assert.equal(controller.status().ready, false);
    assert.equal(controller.status().activeStrategy, null);
    assert.equal(controller.status().pendingStrategy, null);
    assert.equal(controller.status().cleanupPending, true);

    assert.equal((await controller.switchTo('central-vault')).ok, true);
    await verifyPair(controller);
    assert.equal(controller.status().cleanupPending, false);
  } finally {
    await controller.revoke();
  }
});

test('revoke fences a queued start before it can allocate or publish a strategy', async () => {
  const { controller, stats } = harness();
  stats.readerProbeGate = deferred();
  const starting = controller.start('reader-owned');
  assert.equal(controller.status().state, 'SWITCHING');
  assert.equal((await controller.revoke()).ok, true);
  stats.readerProbeGate.resolve();
  assert.deepEqual(await starting, { ok: false, code: 'AUTH_EPOCH_CHANGED' });
  assert.equal(controller.status().state, 'REVOKED');
  assert.equal(controller.lease('reader'), null);
  assert.equal(stats.replaceCalls.length, 0);
});

test('revoke waits for target fanout and clears any late cookie writes before it completes', async () => {
  const { controller, states, stats } = harness();
  stats.replaceGate = deferred();
  stats.replaceEntered = deferred();
  const starting = controller.start('central-vault');
  await stats.replaceEntered.promise;
  const revoking = controller.revoke();
  stats.replaceGate.resolve();
  assert.equal((await revoking).ok, true);
  assert.equal((await starting).ok, false);
  assert.equal(controller.status().state, 'REVOKED');
  assert.equal(controller.lease('reader'), null);
  assert.deepEqual(states.reader.cookies, []);
  assert.deepEqual(states.collector.cookies, []);
});

test('wrong source account blocks handoff and status/results never contain fixture cookie material', async () => {
  const { controller, states } = harness();
  try {
    assert.equal((await controller.start('reader-owned')).ok, true);
    states.reader.accountId = 'different-fixture-account';
    const switched = await controller.switchTo('central-vault');
    assert.deepEqual(switched, { ok: false, code: 'ACCOUNT_MISMATCH' });
    assert.equal(controller.status().ready, false);
    assert.equal(JSON.stringify(controller.status()).includes(SECRET), false);
    assert.equal(JSON.stringify(switched).includes(SECRET), false);
  } finally {
    await controller.revoke();
  }
});

test('central revoke reports failed clear, denies leases immediately, and succeeds on retry', async () => {
  const { controller, stats } = harness();
  assert.equal((await controller.start('central-vault')).ok, true);
  stats.failClear.add('reader');
  const first = await controller.revoke();
  assert.deepEqual(first, { ok: false, code: 'CLEAR_FAILED' });
  assert.equal(controller.status().state, 'REVOKED');
  assert.equal(controller.lease('reader'), null);
  assert.equal(JSON.stringify(controller.status()).includes(SECRET), false);
  assert.equal((await controller.revoke()).ok, true);
  assert.equal(controller.status().cleanupPending, false);
});

test('central use conflict makes controller status not ready and fences every lease', async () => {
  const { controller, states } = harness();
  try {
    assert.equal((await controller.start('central-vault')).ok, true);
    const result = await controller.use('reader', controller.lease('reader'), () => {
      states.reader.cookies = [cookie('unexpected-rotation')];
      return 'must be rejected';
    });
    assert.deepEqual(result, { ok: false, code: 'UNEXPECTED_ROTATION' });
    assert.equal(controller.status().state, 'CONFLICT');
    assert.equal(controller.status().ready, false);
    assert.equal(controller.lease('reader'), null);
    assert.equal(controller.lease('collector'), null);
  } finally {
    await controller.revoke();
  }
});

test('reader-owned collector detach can recover through attach', async () => {
  const { controller } = harness();
  try {
    assert.equal((await controller.start('reader-owned')).ok, true);
    assert.deepEqual(await controller.detach('collector'), { ok: true, code: 'DETACHED' });
    assert.equal(controller.lease('reader'), null);
    assert.equal((await controller.attach('collector')).ok, true);
    await verifyPair(controller);
  } finally {
    await controller.revoke();
  }
});

test('central refresh remains available when the independent reader client is detached', async () => {
  const { controller, stats } = harness();
  try {
    assert.equal((await controller.start('central-vault')).ok, true);
    assert.equal((await controller.detach('reader')).ok, true);
    assert.equal(controller.lease('reader'), null);
    assert.ok(controller.lease('collector'));
    assert.equal((await controller.refresh()).ok, true);
    assert.equal(stats.refreshOwners.at(-1), 'collector');
    assert.equal(controller.status().ready, true);
    assert.ok(controller.lease('collector'));
    assert.equal((await controller.attach('reader')).ok, true);
    await verifyPair(controller);
  } finally {
    await controller.revoke();
  }
});

test('only configured loopback fixture origins and the two built-in strategies are accepted', async () => {
  assert.throws(() => new SessionStrategyController({
    origin: 'https://example.com', expectedAccountId: ACCOUNT,
    clients: harness().clients,
  }), /FIXTURE_ORIGIN_NOT_ALLOWED/);
  const { controller } = harness();
  assert.deepEqual(controller.listStrategies().map(item => item.id), ['reader-owned', 'central-vault']);
  assert.equal(controller.status().activeStrategy, null);
  assert.equal((await controller.start('../arbitrary-module')).ok, false);
});

test('drain timeout never publishes a target and a settled operation allows an explicit retry', async () => {
  const { controller } = harness({ timeoutMs: 20 });
  const finish = deferred();
  let work;
  try {
    assert.equal((await controller.start('reader-owned')).ok, true);
    const entered = deferred();
    work = controller.use('collector', controller.lease('collector'), async () => {
      entered.resolve();
      await finish.promise;
      return 'expired result';
    });
    await entered.promise;
    assert.deepEqual(await controller.switchTo('central-vault'), { ok: false, code: 'DRAIN_TIMEOUT' });
    assert.equal(controller.status().activeStrategy, 'reader-owned');
    assert.equal(controller.status().pendingStrategy, null);
    assert.equal(controller.status().ready, false);
    assert.equal(controller.lease('collector'), null);
    finish.resolve();
    assert.equal((await work).ok, false);
    assert.equal((await controller.switchTo('central-vault')).ok, true);
    await verifyPair(controller);
  } finally {
    finish.resolve();
    await work;
    await controller.revoke();
  }
});
