import test from 'node:test';
import assert from 'node:assert/strict';
import { collectProtectedFixture } from './concurrent-protected-collection.mjs';

function fakePage(values) {
  const calls = [];
  return { calls, async evaluate(expression) { calls.push(expression); if (expression.startsWith('delete ')) return true; const next = values.shift(); if (next instanceof Error) throw next; return next; } };
}
const fast = { now: () => 0, pause: async () => {} };

test('protected collection polls pending browser fetch and clears state after completion', async () => {
  const page = fakePage([true, {finished:false,status:0,item:false}, {finished:false,status:0,item:false}, {finished:true,status:200,item:true}]);
  assert.deepEqual(await collectProtectedFixture(page, fast), {status:200,item:true});
  assert.equal(page.calls.length, 5);
  assert.match(page.calls.at(-1), /^delete /);
  assert.match(page.calls[0], /fetch\('\/collect'/);
});

test('invalid/failed polling clears owned state and failed browser fetch returns bounded result', async () => {
  for (const value of [null, {finished:true,status:500,item:false}, Error('poll_failure')]) {
    const page = fakePage([true, value]);
    await assert.rejects(collectProtectedFixture(page, fast));
    assert.match(page.calls.at(-1), /^delete /);
  }
  const page = fakePage([true, {finished:true,status:0,item:false}]);
  assert.deepEqual(await collectProtectedFixture(page, fast), {status:0,item:false});
});

test('polling has a fixed duration bound and does not clear another active collection', async () => {
  const page = fakePage([true]); let ticks = 0;
  await assert.rejects(collectProtectedFixture(page, { now: () => ++ticks*30001, pause: async () => {} }), /fixture_concurrent_collection_timeout/);
  assert.equal(page.calls.length, 2);
  const occupied = fakePage([Error('fixture_concurrent_collect_busy')]);
  await assert.rejects(collectProtectedFixture(occupied, fast), /fixture_concurrent_collect_busy/);
  assert.equal(occupied.calls.length, 1);
});
