import test from 'node:test';
import assert from 'node:assert/strict';
import { awaitIssuedAtCatchUp } from './x-evidence-request-clock.mjs';

test('issued time catches up within the bounded wait without extending the deadline', async () => {
  let now = 1_000;
  let elapsed = 0;
  const timing = await awaitIssuedAtCatchUp({
    issuedAt: '1970-01-01T00:00:01.000000500Z',
    clock: () => now,
    elapsedClock: () => elapsed,
    sleep: async milliseconds => { now += milliseconds; elapsed += milliseconds; },
    ttlMs: 5_000,
  });
  assert.equal(timing.nowMs, 1_001);
  assert.equal(timing.issuedAtMs, 1_000.0005);
  assert.equal(timing.deadlineMs, 6_000);
});

test('a still-future request is rejected when the catch-up budget is exhausted', async () => {
  let elapsed = 0;
  let waits = 0;
  await assert.rejects(awaitIssuedAtCatchUp({
    issuedAt: '1970-01-01T00:00:01.000000500Z',
    clock: () => 1_000,
    elapsedClock: () => elapsed,
    sleep: async milliseconds => { waits++; elapsed += milliseconds; },
  }), /request_future/u);
  assert.equal(waits, 50);
});

test('a request at or beyond its original TTL is expired', async () => {
  await assert.rejects(awaitIssuedAtCatchUp({
    issuedAt: '1970-01-01T00:00:01Z',
    clock: () => 6_000,
    elapsedClock: () => 0,
    sleep: async () => assert.fail('expired request must not wait'),
  }), /request_expired/u);
});

test('wall-clock rollback during catch-up is rejected', async () => {
  let reads = 0;
  let elapsed = 0;
  await assert.rejects(awaitIssuedAtCatchUp({
    issuedAt: '1970-01-01T00:00:01.000000500Z',
    clock: () => (++reads === 1 ? 1_000 : 999),
    elapsedClock: () => elapsed,
    sleep: async milliseconds => { elapsed += milliseconds; },
  }), /request_clock_rollback/u);
});
