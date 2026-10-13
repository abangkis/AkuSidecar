import test from 'node:test';
import assert from 'node:assert/strict';
import { ExclusiveFallbackController } from './exclusive-fallback.mjs';
import { FixtureCaptureQueue } from './capture-queue.mjs';
const full = { cookieObservation: 'verified', startupCoverage: 'verified', ownership: 'verified' };
const deferred = () => { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; };
function fixture(caps = full, timeoutMs = 100) {
  const s = { count: 0, authenticated: true, accountId: 'alpha', revision: 'private-one', pauses: 0, starts: 0, opens: 0 };
  const adapters = {
    readerCount: async () => s.count,
    verifySession: async () => ({ authenticated: s.authenticated, accountId: s.accountId, revision: s.revision }),
    pauseCollector: async () => { s.pauses++; return { stopped: true }; },
    resumeCollector: async v => { s.starts++; return { ready: true, accountId: v.accountId, revision: v.revision }; },
    openReader: async () => { s.opens++; s.count++; },
  };
  return { s, adapters, c: new ExclusiveFallbackController({ expectedAccountId: 'alpha', capabilities: caps, adapters, timeoutMs }) };
}
test('fallback fences active reads and retained queue resumes exactly once', async () => {
  const { c, s } = fixture(); await c.start(); const wait = deferred(); const entered = deferred(); let calls = 0;
  const q = new FixtureCaptureQueue({ session: c });
  q.enqueueRead({ id: 'one', read: async () => { calls++; entered.resolve(); if (calls === 1) await wait.promise; return { status: 200, body: { account: 'alpha', postId: 'fixture-post-1' } }; } });
  const running = q.run('one'); await entered.promise;
  s.count = 1; const fallback = c.setCapabilities({}); assert.equal(c.status().state, 'SWITCHING'); assert.equal(c.lease('collector'), null);
  wait.resolve(); await running; await fallback; assert.equal(q.status('one').state, 'pending'); await q.resume(); assert.equal(calls, 1);
  s.count = 0; await c.readerClosed(); await q.resume(); await q.resume(); assert.equal(calls, 2); assert.equal(q.status('one').state, 'completed');
  await c.setCapabilities(full); assert.equal(c.status().mode, 'exclusive'); assert.ok(!JSON.stringify(c.status()).includes('private-one'));
});
test('exclusive reader priority and inventory-confirmed resume', async () => {
  const { c, s } = fixture({}); await c.start(); await c.requestReader(); assert.equal(s.opens, 1); assert.equal(c.status().state, 'SWITCHING');
  await c.readerClosed(); assert.equal(s.starts, 1); s.count = 0; await c.readerClosed(); assert.equal(s.starts, 2);
});
for (const [name, change] of [['unknown inventory', s => { s.count = null; }], ['wrong account', s => { s.accountId = 'beta'; }], ['logged out', s => { s.authenticated = false; }]]) {
  test(name + ' fails closed without automatic recovery', async () => { const { c, s } = fixture({}); change(s); await c.start(); assert.equal(c.status().state, 'NEEDS_USER'); s.count = 0; s.accountId = 'alpha'; s.authenticated = true; await c.readerClosed(); assert.equal(c.status().state, 'NEEDS_USER'); });
}
test('revision change after read blocks publication; foreign lease rejected', async () => {
  const { c, s } = fixture(); await c.start(); const other = fixture().c; await other.start();
  assert.equal((await c.use('collector', other.lease('collector'), async () => 1)).code, 'LEASE_STALE');
  const result = await c.use('collector', c.lease('collector'), async () => { s.revision = 'private-two'; return 1; });
  assert.equal(result.ok, false); assert.equal(c.status().state, 'NEEDS_USER');
});
test('reader arriving during collector resume stops late start before opening reader', async () => {
  const { c, s, adapters } = fixture({}); const gate = deferred(); const entered = deferred();
  adapters.resumeCollector = async v => { s.starts++; entered.resolve(); await gate.promise; return { ready: true, ...v }; };
  const starting = c.start(); await entered.promise; const reader = c.requestReader(); gate.resolve(); await starting; await reader;
  assert.equal(c.status().state, 'SWITCHING'); assert.equal(s.opens, 1); assert.ok(s.pauses >= 2);
});
test('stop during control never publishes ready or opens reader', async () => {
  const { c, s, adapters } = fixture({}); const gate = deferred(); const entered = deferred();
  adapters.resumeCollector = async v => { entered.resolve(); await gate.promise; return { ready: true, ...v }; };
  const starting = c.start(); await entered.promise; const stopping = c.stop(); gate.resolve(); await starting; await stopping;
  assert.equal(c.status().state, 'REVOKED'); assert.equal(c.lease('collector'), null); assert.equal(s.opens, 0);
});
test('drain timeout prevents opening reader and rejects late old read', async () => {
  const { c, s } = fixture({}, 20); await c.start(); const gate = deferred(); const entered = deferred();
  const read = c.use('collector', c.lease('collector'), async () => { entered.resolve(); await gate.promise; return 7; }); await entered.promise;
  await c.requestReader(); gate.resolve(); assert.equal((await read).ok, false); assert.equal(s.opens, 0); assert.equal(c.status().state, 'NEEDS_USER');
});
for (const operation of ['pauseCollector', 'resumeCollector']) {
  test(operation + ' failure is fail closed', async () => { const { c, adapters } = fixture({}); if (operation === 'pauseCollector') await c.start(); adapters[operation] = async () => { throw new Error('secret'); }; await (operation === 'pauseCollector' ? c.requestReader() : c.start()); assert.equal(c.status().state, 'NEEDS_USER'); assert.ok(!JSON.stringify(c.status()).includes('secret')); });
}
test('partly started collector is stopped even when start rejects', async () => {
  const { c, adapters, s } = fixture({}); let physical = false;
  adapters.resumeCollector = async () => { physical = true; throw new Error('private-start-detail'); };
  adapters.pauseCollector = async () => { s.pauses++; physical = false; return { stopped: true }; };
  await c.start(); assert.equal(physical, false); assert.equal(s.pauses, 1); assert.equal(c.status().state, 'NEEDS_USER');
  assert.ok(!JSON.stringify(c.status()).includes('private-start-detail'));
});
for (const rejectLate of [false, true]) {
  test('timed-out start then stop exposes pending physical cleanup: ' + (rejectLate ? 'late rejection' : 'late success'), async () => {
    const { c, adapters } = fixture({}, 15); const gate = deferred(); let physical = false;
    adapters.resumeCollector = async v => { await gate.promise; physical = true; if (rejectLate) throw new Error('private'); return { ready: true, ...v }; };
    adapters.pauseCollector = async () => { physical = false; return { stopped: true }; };
    await c.start(); assert.equal(c.status().state, 'NEEDS_USER');
    const stopped = await c.stop(); assert.equal(stopped.state, 'REVOKED'); assert.equal(stopped.phase, 'cleanup_pending'); assert.equal(stopped.cleanupPending, true);
    gate.resolve(); await new Promise(resolve => setImmediate(resolve));
    const settled = await c.stop(); assert.equal(physical, false); assert.equal(settled.cleanupPending, false); assert.equal(settled.phase, 'stopped');
  });
}
