import test from 'node:test';
import assert from 'node:assert/strict';
import { FixtureCaptureQueue } from './capture-queue.mjs';
const post = { status: 200, body: { postId: 'fixture-post-1', account: 'alpha' } };
function harness() {
  let state = 'READY', generation = 1, uses = 0;
  const session = {
    status: () => ({ state }), lease: () => state === 'READY' ? { generation } : null,
    async use(_id, lease, read) {
      uses++; const value = await read({ accountId: 'alpha' });
      return state === 'READY' && lease.generation === generation ? { ok: true, value } : { ok: false, code: 'LEASE_STALE' };
    },
  };
  return { queue: new FixtureCaptureQueue({ session, maxAttempts: 2 }), set: value => { state = value; generation++; }, uses: () => uses };
}
test('read lost across a handoff stays pending then is admitted once with a fresh lease', async () => {
  const h = harness(); let release;
  let calls = 0;
  const read = async () => { if (++calls === 1) await new Promise(r => { release = r; }); return post; };
  h.queue.enqueueRead({ id: 'post-1', read });
  const first = h.queue.run('post-1');
  assert.equal(h.queue.run('post-1'), first);
  h.set('SWITCHING'); release();
  assert.equal((await first).state, 'pending');
  assert.equal(h.queue.result('post-1'), null);
  await h.queue.resume(); assert.equal(h.uses(), 1);
  h.set('READY'); await h.queue.resume();
  assert.equal(h.queue.status('post-1').state, 'completed');
  assert.deepEqual(h.queue.result('post-1'), { postId: 'fixture-post-1', account: 'alpha' });
  await h.queue.run('post-1'); assert.equal(h.uses(), 2);
});
test('conflict never triggers automatic capture replay', async () => {
  const h = harness();
  h.queue.enqueueRead({ id: 'post-1', read: async () => { h.set('CONFLICT'); return post; } });
  assert.equal((await h.queue.run('post-1')).state, 'blocked');
  h.set('READY'); await h.queue.resume(); assert.equal(h.uses(), 1);
  assert.equal(h.queue.result('post-1'), null);
});
test('repeated handoff reaches explicit retry budget instead of an unbounded loop', async () => {
  const h = harness();
  h.queue.enqueueRead({ id: 'post-1', read: async () => { h.set('REFRESHING'); return post; } });
  await h.queue.run('post-1'); h.set('READY'); await h.queue.resume();
  assert.equal(h.queue.status('post-1').state, 'exhausted');
  h.set('READY'); await h.queue.resume(); assert.equal(h.uses(), 2);
});
test('conflicting duplicate and invalid callback output cannot overwrite accepted evidence', async () => {
  const h = harness(); const read = async () => post;
  h.queue.enqueueRead({ id: 'post-1', read }); await h.queue.run('post-1');
  assert.throws(() => h.queue.enqueueRead({ id: 'post-1', read: async () => post }), /READ_JOB_CONFLICT/);
  h.queue.enqueueRead({ id: 'post-2', read: async () => ({ status: 401, secret: 'synthetic-private-value' }) });
  assert.equal((await h.queue.run('post-2')).state, 'blocked');
  assert.equal(JSON.stringify(h.queue.status('post-2')).includes('synthetic-private-value'), false);
});

test('a payload from a different account is never admitted despite a valid session lease', async () => {
  const h = harness();
  h.queue.enqueueRead({ id: 'post-1', read: async () => ({ status: 200, body: { postId: 'fixture-post-1', account: 'beta' } }) });
  assert.equal((await h.queue.run('post-1')).code, 'READ_RESPONSE_INVALID');
  assert.equal(h.queue.result('post-1'), null);
});
