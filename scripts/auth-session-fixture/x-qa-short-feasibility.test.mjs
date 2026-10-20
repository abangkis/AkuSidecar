import test from 'node:test';
import assert from 'node:assert/strict';
import { QA_SHORT_MANIFEST, selectQAShortCookies, shortQACheckpoint, runQAShortFeasibility } from './x-qa-short-feasibility.mjs';

const cookies = () => QA_SHORT_MANIFEST.tuples.map(t => ({ ...t, value: 'SYNTHETIC_PRIVATE', session: false,
  expires: 4102444800.125, priority: 'Medium', sourceScheme: 'Secure', sourcePort: 443, size: 17 }));
const observation = () => ({ schema: 'aku.x.qa-bound-observation.v1', diagnostic: 'observed_cookie_write',
  identity: 'current_self_observed', login: 'protected_response_observed', binding: 'unknown', freshness: 'within_5s',
  cleanupVerified: true, continuity: 'unknown', canAuthorize: false, sessionTransferred: false,
  source: { diagnostic: 'http_200_unqualified', status: 200, responseFresh: true, completed: true,
    matchingRequests: 1, bodyReadAttempts: 1, body: { category: 'settings_match', settingsMatch: true } } });

test('exact two tuple import preserves flags and expiry without touching incidental values', () => {
  const incidental = { name: '__cf_bm' };
  Object.defineProperty(incidental, 'value', { get() { throw new Error('must never read'); } });
  const selected = selectQAShortCookies({ cookies: [cookies()[1], incidental, cookies()[0]] });
  assert.deepEqual(selected.map(t => t.name), ['auth_token', 'ct0']);
  assert.equal(selected[0].expires, 4102444800.125); assert.equal(selected[1].sameSite, 'Lax');
  assert.equal(selected[0].httpOnly, true); assert.equal(selected[1].httpOnly, false);
});
test('transfer selection rejects changed scope, duplicates, getters, partitioning, expiry and size', () => {
  for (const mutate of [c => c.pop(), c => c.push(c[0]), c => { c[0].domain = 'x.com'; },
    c => { c[0].secure = false; }, c => { c[1].httpOnly = true; }, c => { c[0].sameSite = 'Lax'; },
    c => { c[0].expires = 0; }, c => { c[0].expires = Infinity; }, c => { c[0].value = 'x'.repeat(9000); },
    c => { c[0].partitionKey = {}; }, c => { c[0].raw = 'PRIVATE'; }, c => { c[0].sourcePort = 80; },
    c => { Object.defineProperty(c[0], 'value', { get() { throw new Error('PRIVATE'); } }); }]) {
    const c = cookies(); mutate(c); assert.throws(() => selectQAShortCookies({ cookies: c }), /qa_short_feasibility_rejected/);
  }
  assert.throws(() => selectQAShortCookies({ cookies: cookies(), raw: 'PRIVATE' }));
});
test('mutation deferral never becomes continuity or Store authority and does not defer other failures', () => {
  const p = shortQACheckpoint(observation()); assert.equal(p.passed, true);
  assert.equal(p.continuity, 'unknown'); assert.equal(p.canAuthorize, false);
  for (const mutate of [o => { o.diagnostic = 'scope_lost'; }, o => { o.diagnostic = 'stale_observation'; },
    o => { o.login = 'unknown'; }, o => { o.cleanupVerified = false; }, o => { o.source.status = 401; },
    o => { o.source.body.settingsMatch = false; }, o => { o.source.completed = false; }]) {
    const o = observation(); mutate(o); assert.equal(shortQACheckpoint(o).passed, false);
  }
  assert.equal(shortQACheckpoint(null).passed, false);
});
function harness(patch = {}) {
  let time = 1000, n = 0;
  const wallBase = Date.now() - 1000;
  const calls = [];
  const a = { prepareCollector: async () => { calls.push('prepare'); },
    check: async role => { calls.push(role); const step = n++; time += 100;
      return { accountId: '12345', start: time - 150, end: time, networkInterval: { start: wallBase + time - 1150, end: wallBase + time - 1000 }, checkpoint: shortQACheckpoint(observation()), step }; },
    seedOnce: async () => { calls.push('seed'); }, readerPreserved: async () => true,
    closeCollector: async () => { calls.push('close'); return true; }, ...patch };
  // Ensure each response interval is within the experiment and the pair overlaps.
  const original = a.check;
  a.check = async role => { const s = await original(role); const sample = { ...s, start: Math.max(1000, s.start) };
    patch.mutateSample?.(sample); return sample; };
  return { run: () => runQAShortFeasibility(a, () => time), calls };
}
test('four matching checkpoints with request overlap produce only a short non-authorizing result', async () => {
  const h = harness(); const r = await h.run();
  assert.equal(r.outcome, 'short_overlap_observed'); assert.equal(r.completedCheckpoints, 4);
  assert.equal(r.overlapMS, 50); assert.equal(r.accountMatch, 'match'); assert.equal(r.sessionTransferred, true);
  assert.equal(r.continuity, 'unknown'); assert.equal(r.refresh, 'not_qualified'); assert.equal(r.nativeQualified, false);
  assert.deepEqual(h.calls, ['prepare', 'reader', 'seed', 'collector', 'reader', 'collector', 'close']);
  assert.equal(JSON.stringify(r).includes('12345'), false); assert.equal(Object.isFrozen(r.checkpoints), true);
});
test('failed precheck never seeds; mismatch, lack of overlap, cleanup and uncertain seed are explicit', async () => {
  const before = harness({ check: async () => ({ checkpoint: shortQACheckpoint(null) }) });
  assert.equal((await before.run()).sessionTransferred, false); assert.equal(before.calls.includes('seed'), false);
  const mismatch = harness({ mutateSample: s => { if (s.step > 0) s.accountId = '54321'; } });
  assert.equal((await mismatch.run()).outcome, 'account_mismatch');
  const noOverlap = harness({ mutateSample: s => { s.networkInterval.start = s.networkInterval.end - 10; } });
  assert.equal((await noOverlap.run()).outcome, 'overlap_not_observed');
  const lostAck = await harness({ seedOnce: async () => { throw new Error('PRIVATE_COOKIE'); } }).run();
  assert.equal(lostAck.seedAttempted, true); assert.equal(lostAck.sessionTransferred, null);
  assert.equal(JSON.stringify(lostAck).includes('PRIVATE_COOKIE'), false);
  assert.equal((await harness({ readerPreserved: async () => false }).run()).outcome, 'reader_window_changed');
  assert.equal((await harness({ closeCollector: async () => false }).run()).outcome, 'cleanup_unverified');
});
