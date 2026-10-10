import test from 'node:test';
import assert from 'node:assert/strict';
import { PassThrough } from 'node:stream';
import { runWorker, GLOBAL_RPC_IDLE_TIMEOUT_MS } from '../worker.mjs';

function createFakeTimers() {
  let nextId = 0;
  const records = new Map();
  return {
    records,
    setTimer(callback, delay) {
      const id = ++nextId;
      records.set(id, { callback, delay, cleared: false, fired: false });
      return id;
    },
    clearTimer(id) {
      const timer = records.get(id);
      if (timer) timer.cleared = true;
    },
    active() {
      return [...records.entries()].filter(([, timer]) => !timer.cleared && !timer.fired);
    },
    async fire(id, { evenIfCleared = false } = {}) {
      const timer = records.get(id);
      if (!timer || timer.fired || (timer.cleared && !evenIfCleared)) return false;
      timer.fired = true;
      timer.callback();
      await new Promise(resolve => setImmediate(resolve));
      return true;
    },
  };
}

function createHarness(options = {}) {
  const input = new PassThrough();
  const output = new PassThrough();
  const errorOutput = new PassThrough();
  let lineBuffer = '';
  const responses = new Map();
  const waiters = new Map();
  output.setEncoding('utf8');
  output.on('data', chunk => {
    lineBuffer += chunk;
    let end;
    while ((end = lineBuffer.indexOf('\n')) >= 0) {
      const line = lineBuffer.slice(0, end);
      lineBuffer = lineBuffer.slice(end + 1);
      if (!line) continue;
      const response = JSON.parse(line);
      const waiter = waiters.get(response.id);
      if (waiter) {
        waiters.delete(response.id);
        waiter(response);
      } else responses.set(response.id, response);
    }
  });
  const running = runWorker({ input, output, errorOutput, ...options });
  return {
    input,
    running,
    async request(request) {
      const response = responses.has(request.id)
        ? Promise.resolve(responses.get(request.id))
        : new Promise(resolve => waiters.set(request.id, resolve));
      input.write(JSON.stringify(request) + '\n');
      const value = await response;
      await new Promise(resolve => setImmediate(resolve));
      return value;
    },
    async finish() {
      if (!input.destroyed) input.end();
      await running;
    },
  };
}

const ownedInit = id => ({
  id,
  type: 'init',
  chrome: 'C:\\Chrome\\chrome.exe',
  profile: 'C:\\Aku\\OwnedHeadlessProfile',
  bridgePath: 'C:\\Aku\\AkuBridge',
});

function fakeOwnedBrowser(id, close) {
  return {
    pid: 9000 + id,
    version: { product: 'Chrome fixture' },
    async close() { await close?.(id); },
  };
}

test('owned Chrome closes after global RPC idle and relaunches with retained options and assets', async () => {
  const timers = createFakeTimers();
  const launches = [];
  const captures = [];
  let assetLoads = 0;
  let closeCount = 0;
  let frontier = 0;
  const harness = createHarness({
    idleTimeoutMs: 500,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async (bridgePath, source) => { assetLoads++; return { bridgePath, source }; },
    launchChromeImpl: async options => {
      launches.push({ ...options });
      const browserId = launches.length;
      return fakeOwnedBrowser(browserId, async () => { closeCount++; });
    },
    captureImpl: async (browser, assets, source, payload) => {
      captures.push({ browser, assets, source, payload });
      frontier++;
      return { frontier };
    },
  });

  try {
    assert.equal((await harness.request(ownedInit('init'))).ok, true);
    assert.equal(launches.length, 1);
    assert.equal(assetLoads, 4);
    assert.equal((await harness.request({ id: 'first', type: 'capture', source: 'x', payload: {} })).result.frontier, 1);
    const timerId = timers.active()[0][0];
    assert.equal(timers.records.get(timerId).delay, 500);
    await timers.fire(timerId);
    assert.equal(closeCount, 1);

    const duplicateInit = await harness.request(ownedInit('duplicate'));
    assert.equal(duplicateInit.ok, false);
    assert.equal(duplicateInit.error.code, 'already_initialized');
    assert.equal(launches.length, 1);
    assert.equal(assetLoads, 4);

    assert.equal((await harness.request({ id: 'second', type: 'capture', source: 'x', payload: { continuation: true } })).result.frontier, 2);
    assert.equal(launches.length, 2);
    assert.deepEqual(launches[1], launches[0]);
    assert.strictEqual(captures[1].assets, captures[0].assets);
    assert.equal(captures[1].browser.pid, 9002);
    assert.equal(assetLoads, 4);
  } finally {
    await harness.finish();
  }
});

test('active and queued captures defeat stale idle callbacks without closing their browser', async () => {
  const timers = createFakeTimers();
  let markStarted;
  const started = new Promise(resolve => { markStarted = resolve; });
  let releaseFirst;
  const firstCapture = new Promise(resolve => { releaseFirst = resolve; });
  const launches = [];
  let closes = 0;
  let capturesStarted = 0;
  const browsers = [];
  const harness = createHarness({
    idleTimeoutMs: 300,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => {
      const browser = fakeOwnedBrowser(launches.length + 1, async () => { closes++; });
      browsers.push(browser);
      launches.push(browser);
      return browser;
    },
    captureImpl: async browser => {
      if (browser === browsers[0] && launches.length === 1 && capturesStarted++ === 0) {
        markStarted();
        await firstCapture;
      }
      return { browserPid: browser.pid };
    },
  });

  try {
    await harness.request(ownedInit('init'));
    const staleTimer = timers.active()[0][0];
    const firstReply = harness.request({ id: 'capture-1', type: 'capture', source: 'x', payload: {} });
    await started;
    const secondReply = harness.request({ id: 'capture-2', type: 'capture', source: 'facebook', payload: {} });
    await timers.fire(staleTimer, { evenIfCleared: true });
    assert.equal(closes, 0);
    assert.equal(launches.length, 1);

    releaseFirst();
    assert.equal((await firstReply).result.browserPid, browsers[0].pid);
    assert.equal((await secondReply).result.browserPid, browsers[0].pid);
    assert.equal(closes, 0);
  } finally {
    releaseFirst();
    await harness.finish();
  }
});

test('idle hold keeps the current browser and frontier, then release starts a fresh idle interval', async () => {
  const timers = createFakeTimers();
  let launches = 0;
  let closes = 0;
  let frontier = 0;
  const harness = createHarness({
    idleTimeoutMs: 400,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => {
      launches++;
      return fakeOwnedBrowser(launches, async () => { closes++; });
    },
    captureImpl: async browser => ({ browserPid: browser.pid, frontier: ++frontier }),
  });

  try {
    await harness.request(ownedInit('init'));
    const priorTimer = timers.active()[0][0];
    assert.deepEqual((await harness.request({ id: 'hold', type: 'setIdleHold', held: true })).result, { held: true });
    assert.equal(timers.active().length, 0);
    await timers.fire(priorTimer, { evenIfCleared: true });
    assert.equal(closes, 0);
    assert.deepEqual((await harness.request({ id: 'round-1', type: 'capture', source: 'facebook', payload: {} })).result,
      { browserPid: 9001, frontier: 1 });
    assert.deepEqual((await harness.request({ id: 'round-2', type: 'capture', source: 'facebook', payload: { continuation: true } })).result,
      { browserPid: 9001, frontier: 2 });
    assert.equal(launches, 1);

    assert.deepEqual((await harness.request({ id: 'release', type: 'setIdleHold', held: false })).result, { held: false });
    const releasedTimer = timers.active()[0][0];
    assert.notEqual(releasedTimer, priorTimer);
    assert.equal(timers.records.get(releasedTimer).delay, 400);
    await timers.fire(releasedTimer);
    assert.equal(closes, 1);
    assert.equal(launches, 1);
    await harness.request({ id: 'late-hold', type: 'setIdleHold', held: true });
    await harness.request({ id: 'late-release', type: 'setIdleHold', held: false });
    assert.equal(launches, 1, 'holding or releasing after idle cleanup must not resurrect Chrome');
    assert.equal(timers.active().length, 0);
  } finally {
    await harness.finish();
  }
});

test('borrowed browser_quiet_hidden is excluded from owned Chrome idle cleanup', async () => {
  const timers = createFakeTimers();
  const harness = createHarness({
    idleTimeoutMs: 100,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    captureImpl: async () => ({ ok: true }),
  });

  try {
    const init = await harness.request({
      id: 'borrowed-init', type: 'init', backend: 'browser_quiet_hidden',
      bridgePath: 'C:\\Aku\\AkuBridge', chromeVersion: { product: 'Host Chrome' },
    });
    assert.equal(init.ok, true);
    assert.equal(timers.active().length, 0);
    await harness.request({ id: 'borrowed-capture', type: 'capture', source: 'x', payload: {} });
    assert.equal(timers.active().length, 0);
  } finally {
    await harness.finish();
  }
});

test('shutdown clears idle expiry and cannot lazily relaunch Chrome', async () => {
  const timers = createFakeTimers();
  let launches = 0;
  let closes = 0;
  const harness = createHarness({
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => {
      launches++;
      return fakeOwnedBrowser(launches, async () => { closes++; });
    },
  });

  await harness.request(ownedInit('init'));
  const timerId = timers.active()[0][0];
  assert.deepEqual((await harness.request({ id: 'stop', type: 'shutdown' })).result, { stopped: true });
  await harness.running;
  await timers.fire(timerId, { evenIfCleared: true });
  assert.equal(launches, 1);
  assert.equal(closes, 1);
});

test('input disconnect clears idle expiry and closes the owned browser', async () => {
  const timers = createFakeTimers();
  let launches = 0;
  let closes = 0;
  const harness = createHarness({
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => {
      launches++;
      return fakeOwnedBrowser(launches, async () => { closes++; });
    },
  });

  await harness.request(ownedInit('init'));
  const timerId = timers.active()[0][0];
  await harness.finish();
  await timers.fire(timerId, { evenIfCleared: true });
  assert.equal(launches, 1);
  assert.equal(closes, 1);
});

test('unverified owned Chrome cleanup fails closed for later captures', async () => {
  const timers = createFakeTimers();
  let launches = 0;
  let captureCount = 0;
  const harness = createHarness({
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => {
      launches++;
      return fakeOwnedBrowser(launches, async () => { throw Object.assign(new Error('process still present'), { code: 'owned_chrome_cleanup_failed' }); });
    },
    captureImpl: async () => { captureCount++; return {}; },
  });

  try {
    await harness.request(ownedInit('init'));
    const timerId = timers.active()[0][0];
    await timers.fire(timerId);
    const capture = await harness.request({ id: 'after-failed-cleanup', type: 'capture', source: 'x', payload: {} });
    assert.equal(capture.ok, false);
    assert.equal(capture.error.code, 'owned_chrome_cleanup_unverified');
    assert.equal(launches, 1);
    assert.equal(captureCount, 0);
  } finally {
    await harness.finish();
  }
});

test('a lazy launch failure can be retried after verified idle cleanup', async () => {
  const timers = createFakeTimers();
  const launchOptions = [];
  let launchAttempt = 0;
  let closes = 0;
  let captures = 0;
  const harness = createHarness({
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async options => {
      launchOptions.push({ ...options });
      launchAttempt++;
      if (launchAttempt === 2) throw Object.assign(new Error('temporary launch failure'), { code: 'chrome_start_failed' });
      return fakeOwnedBrowser(launchAttempt, async () => { closes++; });
    },
    captureImpl: async () => { captures++; return { ok: true }; },
  });

  try {
    await harness.request(ownedInit('init'));
    await timers.fire(timers.active()[0][0]);
    assert.equal(closes, 1);
    const failed = await harness.request({ id: 'retry-1', type: 'capture', source: 'x', payload: {} });
    assert.equal(failed.ok, false);
    assert.equal(failed.error.code, 'chrome_start_failed');
    const recovered = await harness.request({ id: 'retry-2', type: 'capture', source: 'x', payload: {} });
    assert.deepEqual(recovered.result, { ok: true });
    assert.equal(launchAttempt, 3);
    assert.deepEqual(launchOptions[1], launchOptions[0]);
    assert.deepEqual(launchOptions[2], launchOptions[0]);
    assert.equal(captures, 1);
  } finally {
    await harness.finish();
  }
});

test('an unverified lazy launch cleanup blocks another launch for the same worker', async () => {
  const timers = createFakeTimers();
  let launchAttempt = 0;
  const harness = createHarness({
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => {
      launchAttempt++;
      if (launchAttempt === 2) throw Object.assign(new Error('launch cleanup was not verified'), { code: 'owned_chrome_cleanup_failed' });
      return fakeOwnedBrowser(launchAttempt);
    },
  });

  try {
    await harness.request(ownedInit('init'));
    await timers.fire(timers.active()[0][0]);
    const failed = await harness.request({ id: 'unverified-launch', type: 'capture', source: 'x', payload: {} });
    assert.equal(failed.ok, false);
    assert.equal(failed.error.code, 'owned_chrome_cleanup_failed');
    const refused = await harness.request({ id: 'refused-launch', type: 'capture', source: 'x', payload: {} });
    assert.equal(refused.ok, false);
    assert.equal(refused.error.code, 'owned_chrome_cleanup_unverified');
    assert.equal(launchAttempt, 2);
  } finally {
    await harness.finish();
  }
});

test('idle timer remains capped at the default global RPC timeout', async () => {
  const timers = createFakeTimers();
  const harness = createHarness({
    idleTimeoutMs: GLOBAL_RPC_IDLE_TIMEOUT_MS + 10_000,
    setTimer: timers.setTimer,
    clearTimer: timers.clearTimer,
    sourceAssetsImpl: async () => ({}),
    launchChromeImpl: async () => fakeOwnedBrowser(1),
  });
  try {
    await harness.request(ownedInit('init'));
    assert.equal(timers.records.get(timers.active()[0][0]).delay, GLOBAL_RPC_IDLE_TIMEOUT_MS);
  } finally {
    await harness.finish();
  }
});
