import test from 'node:test';
import assert from 'node:assert/strict';
import { createStartupWaitBudget } from './x-manual-startup-wait-budget.mjs';
import { startupDiagnostic, startupFailure } from './x-manual-startup-diagnostic.mjs';

function fixture(milliseconds = 100) {
  let clock = 0, timer, cleared = 0;
  const controller = new AbortController();
  const budget = createStartupWaitBudget(milliseconds, { role: 'reader', signal: controller.signal,
    now: () => clock, setTimer: (callback, delay) => { timer = { callback, delay }; return timer; },
    clearTimer: value => { assert.equal(value, timer); cleared++; } });
  return { budget, controller, at: value => { clock = value; }, timer: () => timer, cleared: () => cleared };
}

test('settings waits use the remaining acquisition budget and never reset it', async () => {
  const f = fixture();
  f.at(70);
  await f.budget.wait(Promise.resolve());
  assert.equal(f.timer().delay, 30);
  f.at(95);
  await f.budget.wait(Promise.resolve());
  assert.equal(f.timer().delay, 5);
  assert.equal(f.cleared(), 2);
  f.at(100);
  await assert.rejects(f.budget.wait(Promise.resolve()), error => startupDiagnostic(error).reason === 'deadline');
  assert.equal(f.cleared(), 2);
});

test('six-second startup waits preserve the remaining budget after two seconds', async () => {
  const f = fixture(6000);
  f.at(2500);
  await f.budget.wait(Promise.resolve());
  assert.equal(f.timer().delay, 3500);
  f.at(5999);
  await f.budget.wait(Promise.resolve());
  assert.equal(f.timer().delay, 1);
  f.at(6000);
  await assert.rejects(f.budget.wait(Promise.resolve()), error => startupDiagnostic(error).reason === 'deadline');
});

test('a stalled settings source fails at the remaining deadline with no successful fallback', async () => {
  const f = fixture(); f.at(80);
  const pending = f.budget.wait(new Promise(() => {}));
  const checked = assert.rejects(pending, error => {
    const d = startupDiagnostic(error);
    return d.role === 'reader' && d.phase === 'settings' && d.reason === 'deadline';
  });
  assert.equal(f.timer().delay, 20);
  f.at(100); f.timer().callback();
  await checked;
  assert.equal(f.cleared(), 1);
});

test('a completed source cannot publish after the deadline or a reversed clock', async () => {
  for (const end of [100, -1]) {
    const f = fixture();
    const pending = f.budget.wait(Promise.resolve()); f.at(end);
    await assert.rejects(pending, error => startupDiagnostic(error).reason === (end === 100 ? 'deadline' : 'time_invalid'));
    assert.equal(f.cleared(), 1);
  }
});

test('cancellation and the original source failure survive wait cleanup', async () => {
  const f = fixture();
  const pending = f.budget.wait(new Promise(() => {}));
  f.controller.abort();
  await assert.rejects(pending, error => startupDiagnostic(error).reason === 'canceled');
  assert.equal(f.cleared(), 1);
  const g = fixture(), first = startupFailure('reader', 'bound_finish', 'observed_cookie_write');
  await assert.rejects(g.budget.wait(Promise.reject(first)), error => error === first);
  assert.equal(g.cleared(), 1);
});

test('invalid or enlarged private budgets are rejected', () => {
  for (const milliseconds of [undefined, null, 0, -1, 0.5, 6001, Infinity, '100']) {
    assert.throws(() => createStartupWaitBudget(milliseconds, { role: 'reader' }), /budget_invalid/);
  }
});

test('bounded waits preserve a failed terminal result instead of converting it to completion', async () => {
  const f = fixture(), result = Object.freeze({status:'failed',reason:'network_failed'});
  assert.equal(await f.budget.wait(Promise.resolve(result)), result);
});
