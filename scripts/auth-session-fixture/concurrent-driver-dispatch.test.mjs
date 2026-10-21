import test from 'node:test';
import assert from 'node:assert/strict';
import { createConcurrentDriverDispatch } from './concurrent-driver-dispatch.mjs';

test('held collection permits probes, denies duplicate, and drains failures', async () => {
  let release, collecting = false, probes = 0;
  const held = new Promise(resolve => { release = resolve; });
  const dispatch = createConcurrentDriverDispatch(async message => {
    if (message.op === 'collect') { collecting = true; await held; collecting = false; throw Error('held_failure'); }
    if (message.op === 'probe') { assert.equal(collecting, true); probes++; return probes; }
    if (message.op === 'bad') throw Error('control_failure');
    return 'drained';
  });
  const first = dispatch({ op: 'collect' });
  const failed = assert.rejects(first, /held_failure/);
  assert.equal(await dispatch({ op: 'probe' }), 1);
  await assert.rejects(dispatch({ op: 'collect' }), /fixture_concurrent_collect_busy/);
  await assert.rejects(dispatch({ op: 'bad' }), /control_failure/);
  assert.equal(await dispatch({ op: 'probe' }), 2);
  release(); await failed;
  assert.equal(await dispatch({ op: 'inventory' }), 'drained');
});

test('controls stay serialized and failed collection admits its successor', async () => {
  const order = [];
  const dispatch = createConcurrentDriverDispatch(async message => {
    order.push(message.id); await Promise.resolve();
    if (message.op === 'collect') throw Error('collection_failure');
    return message.id;
  });
  assert.deepEqual(await Promise.all([dispatch({ op: 'probe', id: 1 }), dispatch({ op: 'probe', id: 2 })]), [1, 2]);
  await assert.rejects(dispatch({ op: 'collect', id: 3 }), /collection_failure/);
  await new Promise(resolve => setImmediate(resolve));
  await assert.rejects(dispatch({ op: 'collect', id: 4 }), /collection_failure/);
  assert.deepEqual(order, [1, 2, 3, 4]);
});

test('control backlog is bounded and drains after rejection', async () => {
  let release;
  const held = new Promise(resolve => { release = resolve; });
  const dispatch = createConcurrentDriverDispatch(async message => { if (message.id === 0) await held; return message.id; });
  const queued = Array.from({ length: 64 }, (_, id) => dispatch({ op: 'probe', id }));
  await assert.rejects(dispatch({ op: 'probe', id: 65 }), /fixture_concurrent_control_busy/);
  release();
  assert.equal((await Promise.all(queued)).length, 64);
  assert.equal(await dispatch({ op: 'probe', id: 66 }), 66);
});
