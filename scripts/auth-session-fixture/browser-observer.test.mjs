import test from 'node:test';
import assert from 'node:assert/strict';
import { observeBrowserSessionMutations } from './browser-observer.mjs';

const origin = 'http://127.0.0.1:43127';

function deferred() {
  let resolve, reject;
  const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
  return { promise, resolve, reject };
}

function fakeBrowser({ targets = [], rootAttachments = [], blockedSession = null, failAutoAttach = false } = {}) {
  const listeners = new Set();
  const calls = [];
  const primary = new Map();
  let sessionSequence = 0;
  let requestSequence = 0;
  let closed = false;
  const pendingRequests = new Map();
  const blocked = new Map();
  if (blockedSession) blocked.set(blockedSession.sessionId, blockedSession.promise);

  function emit(event) {
    for (const listener of [...listeners]) {
      try { listener.onEvent(event); } catch { listener.onUnavailable(); }
    }
  }
  function attach(info, { sessionId = `session-${++sessionSequence}`, parentSessionId, waitingForDebugger = false } = {}) {
    const event = {
      method: 'Target.attachedToTarget',
      ...(parentSessionId ? { sessionId: parentSessionId } : {}),
      params: { sessionId, targetInfo: info, waitingForDebugger },
    };
    if (!primary.has(info.targetId)) primary.set(info.targetId, sessionId);
    emit(event);
    return sessionId;
  }
  function network(sessionId, method, params) {
    emit({ method, sessionId, params });
  }
  function request(page, path, method = 'GET', options = {}) {
    const id = `request-${++requestSequence}`;
    const headers = options.refreshNonce ? { 'x-aku-fixture-refresh-nonce': options.refreshNonce } : {};
    network(primary.get(page.targetId), 'Network.requestWillBeSent', {
      requestId: id,
      request: { url: `${origin}${path}`, method, headers },
    });
    const result = deferred();
    pendingRequests.set(id, result);
    return Object.assign(result.promise, { requestId: id });
  }
  function respond(id, setCookie) {
    network(primary.get('page-1'), 'Network.responseReceived', {
      requestId: id, response: { headers: {} }, hasExtraInfo: true,
    });
    network(primary.get('page-1'), 'Network.responseReceivedExtraInfo', {
      requestId: id, headers: setCookie ? { 'set-cookie': setCookie } : {},
    });
    network(primary.get('page-1'), 'Network.loadingFinished', { requestId: id });
    pendingRequests.get(id)?.resolve({ status: 200, body: { ok: true } });
  }

  const browser = {
    calls,
    primary,
    emit,
    attach,
    network,
    request,
    respond,
    async send(method, params = {}) {
      if (closed) throw new Error('fixture_chrome_closed');
      calls.push({ method, params, sessionId: null });
      if (method === 'Target.setAutoAttach' && params.autoAttach === true) {
        if (failAutoAttach) throw new Error('raw CDP details must not escape');
        for (const item of rootAttachments) attach(item.targetInfo, item);
        return {};
      }
      if (method === 'Target.getTargets') return { targetInfos: targets };
      if (method === 'Target.attachToTarget') {
        const info = targets.find(item => item.targetId === params.targetId) ?? { targetId: params.targetId, type: 'page' };
        const sessionId = attach(info);
        return { sessionId };
      }
      if (method === 'Target.closeTarget') {
        emit({ method: 'Target.targetDestroyed', params: { targetId: params.targetId } });
        return { success: true };
      }
      return {};
    },
    async sendSession(sessionId, method, params = {}) {
      if (closed) throw new Error('fixture_chrome_closed');
      calls.push({ method, params, sessionId });
      if (method === 'Network.enable' && blocked.has(sessionId)) await blocked.get(sessionId);
      return {};
    },
    subscribeProtocol(onEvent, onUnavailable) {
      if (closed) throw new Error('fixture_chrome_closed');
      const listener = { onEvent, onUnavailable };
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    disconnect() {
      if (closed) return;
      closed = true;
      for (const listener of [...listeners]) listener.onUnavailable();
    },
    page(targetId = 'page-1') {
      return {
        targetId,
        origin,
        request(path, method = 'GET', options = {}) { return request({ targetId }, path, method, options); },
      };
    },
  };
  return browser;
}

function pageInfo(targetId = 'page-1') { return { targetId, type: 'page', title: 'fixture', url: 'about:blank' }; }

async function start(browser) {
  const signals = [];
  const watcher = await observeBrowserSessionMutations(browser, {
    origin,
    clientId: 'reader',
    onUnexpected(signal) { signals.push(signal); },
  });
  return { watcher, signals };
}

async function startWorkerFamily() {
  const browser = fakeBrowser({ targets: [pageInfo()] });
  const { watcher, signals } = await start(browser);
  const parentSessionId = browser.primary.get('page-1');
  browser.attach({ targetId: 'worker-split', type: 'worker', url: `${origin}/worker` },
    { sessionId: 'split-worker-session', parentSessionId, waitingForDebugger: true });
  await watcher.flush();
  return { browser, watcher, signals, parentSessionId, workerSessionId: 'split-worker-session' };
}

test('split worker request and response route one cookie mutation to the response target', async () => {
  const { browser, watcher, signals, parentSessionId, workerSessionId } = await startWorkerFamily();
  browser.network(parentSessionId, 'Network.requestWillBeSent', {
    requestId: 'split-1', request: { url: `${origin}/worker`, method: 'GET', headers: {} },
  });
  browser.network(workerSessionId, 'Network.responseReceived', {
    requestId: 'split-1', response: { headers: {} }, hasExtraInfo: true,
  });
  browser.network(parentSessionId, 'Network.responseReceivedExtraInfo', {
    requestId: 'split-1', headers: { 'set-cookie': 'fixture_session=split; Path=/' },
  });
  browser.network(workerSessionId, 'Network.loadingFinished', { requestId: 'split-1' });
  assert.equal((await watcher.flush()).state, 'active');
  assert.deepEqual(signals.map(s => [s.kind, s.targetType]), [['unexpected_session_mutation', 'worker']]);
  await watcher.close();
});

test('parent ExtraInfo arriving before worker request is correlated without lost coverage', async () => {
  const { browser, watcher, signals, parentSessionId, workerSessionId } = await startWorkerFamily();
  browser.network(parentSessionId, 'Network.responseReceivedExtraInfo', { requestId: 'early-1', headers: {} });
  browser.network(workerSessionId, 'Network.requestWillBeSent', {
    requestId: 'early-1', request: { url: `${origin}/worker`, method: 'GET', headers: {} },
  });
  browser.network(workerSessionId, 'Network.responseReceived', {
    requestId: 'early-1', response: { headers: {} }, hasExtraInfo: true,
  });
  browser.network(workerSessionId, 'Network.loadingFinished', { requestId: 'early-1' });
  assert.equal((await watcher.flush()).state, 'active');
  assert.deepEqual(signals, []);
  await watcher.close();
});

test('sibling request ID collision is rejected instead of sharing response evidence', async () => {
  const { browser, watcher, signals, parentSessionId, workerSessionId } = await startWorkerFamily();
  browser.attach({ targetId: 'worker-sibling', type: 'worker', url: `${origin}/worker` },
    { sessionId: 'sibling-session', parentSessionId, waitingForDebugger: true });
  await watcher.flush();
  for (const sessionId of [workerSessionId, 'sibling-session']) {
    browser.network(sessionId, 'Network.requestWillBeSent', {
      requestId: 'collision', request: { url: `${origin}/worker`, method: 'GET', headers: {} },
    });
  }
  assert.equal(watcher.status().state, 'unavailable');
  assert.ok(signals.some(s => s.evidenceKind === 'network_request_id_collision'));
  await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
  await watcher.close();
});

test('duplicate nested attachment establishes ancestry for a root-attached authoritative worker', async () => {
  const worker = { targetId: 'root-worker', type: 'worker', url: `${origin}/worker` };
  const browser = fakeBrowser({ targets: [pageInfo(), worker] });
  const { watcher, signals } = await start(browser);
  const parentSessionId = browser.primary.get('page-1');
  const workerSessionId = browser.primary.get(worker.targetId);
  browser.attach(worker, { sessionId: 'nested-worker-duplicate', parentSessionId, waitingForDebugger: true });
  await watcher.flush();
  browser.network(parentSessionId, 'Network.requestWillBeSent', {
    requestId: 'root-nested', request: { url: `${origin}/worker`, method: 'GET', headers: {} },
  });
  browser.network(parentSessionId, 'Network.responseReceivedExtraInfo', {
    requestId: 'root-nested', headers: { 'set-cookie': 'fixture_session=worker; Path=/' },
  });
  browser.network(workerSessionId, 'Network.responseReceived', {
    requestId: 'root-nested', response: { headers: {} }, hasExtraInfo: true,
  });
  browser.network(workerSessionId, 'Network.loadingFinished', { requestId: 'root-nested' });
  assert.equal((await watcher.flush()).state, 'active');
  assert.deepEqual(signals.map(s => [s.kind, s.targetType]), [['unexpected_session_mutation', 'worker']]);
  assert.equal(browser.calls.some(c => c.method === 'Network.enable' && c.sessionId === 'nested-worker-duplicate'), false);
  await watcher.close();
});

test('contradictory or cyclic duplicate target ancestry cannot authorize correlation', async t => {
  for (const topology of ['contradictory', 'cyclic']) {
    await t.test(topology, async () => {
      const worker = { targetId: 'root-worker', type: 'worker', url: `${origin}/worker` };
      const browser = fakeBrowser({ targets: [pageInfo(), pageInfo('page-2'), worker] });
      const { watcher } = await start(browser);
      const parentSessionId = browser.primary.get('page-1');
      const workerSessionId = browser.primary.get(worker.targetId);
      browser.attach(worker, { sessionId: 'nested-worker', parentSessionId });
      if (topology === 'contradictory') {
        browser.attach(worker, { sessionId: 'other-parent-worker', parentSessionId: browser.primary.get('page-2') });
      } else {
        browser.attach(pageInfo(), { sessionId: 'cyclic-page', parentSessionId: workerSessionId });
      }
      await watcher.flush();
      browser.network(parentSessionId, 'Network.requestWillBeSent', {
        requestId: 'bad-topology', request: { url: `${origin}/worker`, method: 'GET', headers: {} },
      });
      browser.network(workerSessionId, 'Network.responseReceived', {
        requestId: 'bad-topology', response: { headers: {} }, hasExtraInfo: false,
      });
      assert.equal(watcher.status().state, 'unavailable');
      await watcher.close();
    });
  }
});

test('refresh authorization cannot absorb related-target ExtraInfo or response', async t => {
  for (const transferred of ['extra-before-response', 'extra-after-response', 'response']) {
    await t.test(transferred, async () => {
      const { browser, watcher, signals, parentSessionId, workerSessionId } = await startWorkerFamily();
      let id;
      const refresh = watcher.authorizeRefresh(browser.page(), request => {
        const pending = request('/rotate', 'POST'); id = pending.requestId; return pending;
      });
      const rejected = assert.rejects(refresh, /fixture_(?:browser_)?observer_/);
      if (transferred === 'extra-after-response') {
        browser.network(parentSessionId, 'Network.responseReceived', {
          requestId: id, response: { headers: {} }, hasExtraInfo: true,
        });
      }
      browser.network(workerSessionId,
        transferred === 'response' ? 'Network.responseReceived' : 'Network.responseReceivedExtraInfo',
        { requestId: id, response: { headers: {} }, hasExtraInfo: true,
          headers: { 'set-cookie': 'fixture_session=must-not-exempt; Path=/' } });
      browser.respond(id, 'fixture_session=own; Path=/');
      await rejected;
      assert.equal(watcher.status().state, 'unavailable');
      assert.ok(signals.some(s => s.evidenceKind === 'network_cross_session_refresh_or_extra_info'));
      await watcher.close();
    });
  }
});

test('only explicit internal response URLs recover requests predating observation', async t => {
  for (const url of ['chrome://settings/main.js', 'chrome-extension://fixture/background.js', `${origin}/rotate`, undefined]) {
    await t.test(String(url), async () => {
      const browser = fakeBrowser({ targets: [pageInfo()] });
      const { watcher } = await start(browser);
      const sessionId = browser.primary.get('page-1');
      browser.network(sessionId, 'Network.responseReceived', {
        requestId: 'already-started', response: { url, headers: {} }, hasExtraInfo: false,
      });
      if (url?.startsWith('chrome')) {
        browser.network(sessionId, 'Network.loadingFinished', { requestId: 'already-started' });
        assert.equal((await watcher.flush()).state, 'active');
      } else {
        assert.equal(watcher.status().state, 'unavailable');
      }
      await watcher.close();
    });
  }
});

test('unowned ExtraInfo remains incomplete at flush and retirement', async t => {
  for (const action of ['flush', 'retire']) {
    await t.test(action, async () => {
      const browser = fakeBrowser({ targets: [pageInfo()] });
      const { watcher } = await start(browser);
      browser.network(browser.primary.get('page-1'), 'Network.responseReceivedExtraInfo', {
        requestId: 'orphan', headers: {},
      });
      await assert.rejects(action === 'flush' ? watcher.flush() : watcher.retireTarget('page-1'),
        /fixture_browser_observer_unavailable/);
      assert.equal(browser.calls.some(c => c.method === 'Target.closeTarget'), false);
      await watcher.close();
    });
  }
});

test('terminal-only internal browser UI lifecycle tails remain diagnostic after coverage starts', async t => {
  for (const method of ['Network.loadingFinished', 'Network.loadingFailed']) {
    await t.test(method, async () => {
      const browser = fakeBrowser({ targets: [{ targetId: 'ui-prime', type: 'browser_ui', url: 'chrome://settings/' }] });
      const { watcher, signals } = await start(browser);
      const sessionId = browser.primary.get('ui-prime');
      browser.network(sessionId, method, { requestId: 'predates-observation' });
      const status = await watcher.flush();
      assert.equal(status.state, 'active');
      assert.equal(status.evidenceByKind.internal_ui_terminal_without_request, 1);
      assert.deepEqual(signals, []);
      browser.network(sessionId, method, { requestId: 'after-observation' });
      const activeStatus = await watcher.flush();
      assert.equal(activeStatus.state, 'active');
      assert.equal(activeStatus.evidenceByKind.internal_ui_terminal_without_request, 2);
      assert.deepEqual(signals, []);
      await watcher.close();
    });
  }
});

test('internal UI terminal exception never covers HTTP pages or orphan response evidence', async t => {
  for (const variant of ['page', 'http-browser-ui', 'extra-info']) {
    await t.test(variant, async () => {
      const info = variant === 'page' ? pageInfo() : {
        targetId: 'ui-prime', type: 'browser_ui', url: variant === 'http-browser-ui' ? origin : 'chrome://settings/',
      };
      const browser = fakeBrowser({ targets: [info] });
      const { watcher } = await start(browser);
      const sessionId = browser.primary.get(info.targetId);
      if (variant === 'extra-info') browser.network(sessionId, 'Network.responseReceivedExtraInfo', {
        requestId: 'unknown', headers: { 'set-cookie': 'fixture_session=must-not-ignore' },
      });
      browser.network(sessionId, 'Network.loadingFinished', { requestId: 'unknown' });
      assert.equal(watcher.status().state, 'unavailable');
      await watcher.close();
    });
  }
});

test('authoritative initialization events replay in order after Network observer subscription', async () => {
  const browser = fakeBrowser({ targets: [pageInfo()] });
  const { watcher, signals } = await start(browser);
  const blocker = deferred();
  const enableEntered = deferred();
  const originalSend = browser.sendSession.bind(browser);
  browser.sendSession = async (id, method, params) => {
    if (id === 'initializing-ui' && method === 'Network.enable') {
      enableEntered.resolve();
      await blocker.promise;
    }
    return originalSend(id, method, params);
  };
  browser.attach({ targetId: 'initializing-ui-target', type: 'browser_ui', url: 'chrome://settings/' },
    { sessionId: 'initializing-ui', waitingForDebugger: true });
  await enableEntered.promise;
  assert.deepEqual(watcher.status().initializingSetupStages, [{
    targetType: 'browser_ui', stage: 'network_enable', count: 1,
  }]);
  browser.network('initializing-ui', 'Network.responseReceived', {
    requestId: 'internal-startup', response: { url: 'chrome://settings/resource.js', headers: {} }, hasExtraInfo: false,
  });
  browser.network('initializing-ui', 'Network.loadingFinished', { requestId: 'internal-startup' });
  browser.network('initializing-ui', 'Network.requestWillBeSent', {
    requestId: 'fixture-startup', request: { url: `${origin}/rotate`, method: 'GET', headers: {} },
  });
  browser.network('initializing-ui', 'Network.responseReceivedExtraInfo', {
    requestId: 'fixture-startup', headers: { 'set-cookie': 'fixture_session=queued-cookie; Path=/' },
  });
  browser.network('initializing-ui', 'Network.responseReceived', {
    requestId: 'fixture-startup', response: { headers: {} }, hasExtraInfo: true,
  });
  browser.network('initializing-ui', 'Network.loadingFinished', { requestId: 'fixture-startup' });
  assert.deepEqual(signals, []);
  blocker.resolve();
  assert.equal((await watcher.flush()).state, 'active');
  assert.deepEqual(watcher.status().initializingSetupStages, []);
  assert.deepEqual(signals.map(s => [s.kind, s.targetType]), [['unexpected_session_mutation', 'browser_ui']]);
  await watcher.close();
});

test('initialization event queues fail closed on setup rejection and overflow', async t => {
  for (const failure of ['setup', 'overflow']) {
    await t.test(failure, async () => {
      const browser = fakeBrowser({ targets: [pageInfo()] });
      const { watcher, signals } = await start(browser);
      const blocker = deferred();
      const originalSend = browser.sendSession.bind(browser);
      browser.sendSession = async (id, method, params) => {
        if (id === 'queued-session' && method === 'Network.enable') await blocker.promise;
        return originalSend(id, method, params);
      };
      browser.attach({ targetId: 'queued-target', type: 'browser_ui', url: 'chrome://settings/' },
        { sessionId: 'queued-session', waitingForDebugger: true });
      // Let initialization reach Network.enable before rejecting its promise.
      await Promise.resolve(); await Promise.resolve();
      const total = failure === 'overflow' ? 129 : 1;
      for (let index = 0; index < total; index++) browser.network('queued-session', 'Network.loadingFinished', {
        requestId: `queued-${index}`,
      });
      if (failure === 'setup') blocker.reject(new Error('synthetic enable failure'));
      else blocker.resolve();
      await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
      assert.ok(signals.some(s => s.evidenceKind === (failure === 'setup' ? 'target_setup_failed' : 'initializing_network_queue_full')));
      await watcher.close();
    });
  }
});

test('existing shared worker denies admission without Network events and resumes its pause', async () => {
  const worker = { targetId: 'existing-shared', type: 'shared_worker', url: `${origin}/shared-worker` };
  const browser = fakeBrowser({ targets: [worker], rootAttachments: [{
    targetInfo: worker, sessionId: 'existing-shared-session', waitingForDebugger: true,
  }] });
  const signals = [];
  await assert.rejects(observeBrowserSessionMutations(browser, {
    origin, clientId: 'reader', onUnexpected(signal) { signals.push(signal); },
  }), error => {
    assert.equal(error.message, 'fixture_browser_observer_start_failed');
    assert.equal(error.observationStatus.targetsByType.shared_worker.unavailable, 1);
    return true;
  });
  assert.ok(signals.some(s => s.evidenceKind === 'shared_worker_network_coverage_unavailable'));
  assert.ok(browser.calls.some(c => c.method === 'Runtime.runIfWaitingForDebugger' && c.sessionId === 'existing-shared-session'));
  assert.equal(browser.calls.some(c => c.method === 'Network.enable' && c.sessionId === 'existing-shared-session'), false);
});

test('new paused shared worker revokes browser coverage without waiting for network evidence', async () => {
  const browser = fakeBrowser({ targets: [pageInfo()] });
  const { watcher, signals } = await start(browser);
  browser.attach({ targetId: 'new-shared', type: 'shared_worker', url: `${origin}/shared-worker` }, {
    sessionId: 'new-shared-session', parentSessionId: browser.primary.get('page-1'), waitingForDebugger: true,
  });
  await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
  assert.ok(signals.some(s => s.evidenceKind === 'shared_worker_network_coverage_unavailable'));
  assert.equal(watcher.status().targetsByType.shared_worker.unavailable, 1);
  assert.ok(browser.calls.some(c => c.method === 'Runtime.runIfWaitingForDebugger' && c.sessionId === 'new-shared-session'));
  await watcher.close();
});

test('new service worker authority belongs to its paused auto-attachment instead of discovery', async () => {
  const browser = fakeBrowser({ targets: [pageInfo()] });
  const { watcher } = await start(browser);
  const worker = { targetId: 'dynamic-service-worker', type: 'service_worker', url: 'chrome-extension://fixture/sw.js' };
  browser.emit({ method: 'Target.targetCreated', params: { targetInfo: worker } });
  assert.equal(browser.calls.some(c => c.method === 'Target.attachToTarget' && c.params.targetId === worker.targetId), false);
  assert.deepEqual(watcher.status().initializingSetupStages, [{
    targetType: 'service_worker', stage: 'awaiting_attachment', count: 1,
  }]);
  browser.attach(worker, { sessionId: 'paused-service-worker', parentSessionId: browser.primary.get('page-1'), waitingForDebugger: true });
  assert.equal((await watcher.flush()).targetsByType.service_worker.ready, 1);
  const enableIndex = browser.calls.findIndex(c => c.method === 'Network.enable' && c.sessionId === 'paused-service-worker');
  const resumeIndex = browser.calls.findIndex(c => c.method === 'Runtime.runIfWaitingForDebugger' && c.sessionId === 'paused-service-worker');
  assert.ok(enableIndex >= 0 && resumeIndex > enableIndex);
  await watcher.close();
});

test('new target lacking auto-attachment fails coverage instead of falling back to manual attachment', async () => {
  const browser = fakeBrowser({ targets: [pageInfo()] });
  const { watcher, signals } = await start(browser);
  const worker = { targetId: 'missing-auto-attachment', type: 'service_worker', url: 'chrome-extension://fixture/sw.js' };
  browser.emit({ method: 'Target.targetCreated', params: { targetInfo: worker } });
  await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
  assert.ok(signals.some(s => s.evidenceKind === 'target_not_ready'));
  assert.equal(browser.calls.some(c => c.method === 'Target.attachToTarget' && c.params.targetId === worker.targetId), false);
  await watcher.close();
});

test('HTTP and unknown service-worker targets deny admission without waiting for Network.enable', async t => {
  for (const url of [`${origin}/service-worker`, 'https://fixture.invalid/sw.js', undefined]) {
    await t.test(String(url), async () => {
      const worker = { targetId: 'unsupported-sw', type: 'service_worker', url };
      const browser = fakeBrowser({ targets: [worker], rootAttachments: [{
        targetInfo: worker, sessionId: 'unsupported-sw-session', waitingForDebugger: true,
      }] });
      await assert.rejects(start(browser), /fixture_browser_observer_start_failed/);
      assert.equal(browser.calls.some(c => c.method === 'Network.enable' && c.sessionId === 'unsupported-sw-session'), false);
      assert.ok(browser.calls.some(c => c.method === 'Runtime.runIfWaitingForDebugger' && c.sessionId === 'unsupported-sw-session'));
    });
  }
});

test('service-worker discovery synchronously fences an in-progress controlled refresh', async () => {
  const browser = fakeBrowser({ targets: [pageInfo()] });
  const { watcher, signals } = await start(browser);
  let id;
  const refresh = watcher.authorizeRefresh(browser.page(), request => {
    const pending = request('/rotate', 'POST'); id = pending.requestId; return pending;
  });
  const rejected = assert.rejects(refresh, /fixture_browser_observer_unavailable/);
  const worker = { targetId: 'new-http-sw', type: 'service_worker', url: `${origin}/service-worker` };
  browser.emit({ method: 'Target.targetCreated', params: { targetInfo: worker } });
  assert.equal(watcher.status().state, 'unavailable');
  assert.ok(signals.some(s => s.evidenceKind === 'service_worker_network_coverage_unavailable'));
  browser.attach(worker, { sessionId: 'new-http-sw-session', waitingForDebugger: true });
  browser.respond(id, 'fixture_session=controlled; Path=/');
  await rejected;
  assert.ok(browser.calls.some(c => c.method === 'Runtime.runIfWaitingForDebugger' && c.sessionId === 'new-http-sw-session'));
  await watcher.close();
});

test('internal service worker HTTP request fences coverage before any response exists', async () => {
  const browser = fakeBrowser({ targets: [{ targetId: 'extension-sw', type: 'service_worker', url: 'chrome-extension://fixture/sw.js' }] });
  const { watcher, signals } = await start(browser);
  browser.network(browser.primary.get('extension-sw'), 'Network.requestWillBeSent', {
    requestId: 'http-from-internal-sw', request: { url: `${origin}/rotate`, method: 'POST', headers: {} },
  });
  assert.equal(watcher.status().state, 'unavailable');
  assert.ok(signals.some(s => s.evidenceKind === 'service_worker_http_request_unobservable'));
  await watcher.close();
});

test('browser UI data resources are explicit non-network requests while HTTP remains strict', async () => {
  const browser = fakeBrowser({ targets: [{ targetId: 'data-ui', type: 'browser_ui', url: 'chrome://settings/' }] });
  const { watcher, signals } = await start(browser);
  const id = browser.primary.get('data-ui');
  browser.network(id, 'Network.requestWillBeSent', {
    requestId: 'data-resource', request: { url: 'data:text/plain,fixture', method: 'GET', headers: {} },
  });
  browser.network(id, 'Network.responseReceived', {
    requestId: 'data-resource', response: { url: 'data:text/plain,fixture', headers: {} }, hasExtraInfo: false,
  });
  browser.network(id, 'Network.loadingFinished', { requestId: 'data-resource' });
  assert.equal((await watcher.flush()).state, 'active');
  assert.deepEqual(signals, []);
  browser.network(id, 'Network.requestWillBeSent', {
    requestId: 'http-resource', request: { url: 'https://foreign.invalid/resource', method: 'GET', headers: {} },
  });
  assert.equal(watcher.status().state, 'unavailable');
  await watcher.close();
});

test('startup failure exposes only sanitized stage and evidence status', async () => {
  const browser = fakeBrowser({ failAutoAttach: true });
  await assert.rejects(
    observeBrowserSessionMutations(browser, {
      origin,
      clientId: 'reader',
      onUnexpected() {},
    }),
    error => {
      assert.equal(error.message, 'fixture_browser_observer_start_failed');
      assert.deepEqual(error.observationStatus.lastFailure, {
        stage: 'root_auto_attach',
        targetType: 'browser',
        evidenceKind: 'startup_failed',
      });
      assert.equal(error.observationStatus.state, 'unavailable');
      assert.equal(error.observationStatus.evidenceByKind.startup_failed, 1);
      assert.equal(JSON.stringify(error.observationStatus).includes('raw CDP details'), false);
      return true;
    },
  );
});

test('unsupported target diagnostics retain only bounded type and URL scheme counts', async () => {
  const browser = fakeBrowser({ targets: [{
    targetId: 'unknown-1',
    type: 'chrome_internal_thing',
    url: 'chrome-extension://private-extension-id/secret/path.html?token=secret',
  }] });
  await assert.rejects(
    observeBrowserSessionMutations(browser, {
      origin,
      clientId: 'reader',
      onUnexpected() {},
    }),
    error => {
      const status = error.observationStatus;
      assert.equal(status.state, 'unavailable');
      assert.deepEqual(status.unsupportedTargetDiagnostics, [{
        type: 'chrome_internal_thing',
        urlScheme: 'chrome-extension',
        count: 1,
      }]);
      assert.equal(JSON.stringify(status).includes('private-extension-id'), false);
      assert.equal(JSON.stringify(status).includes('secret/path.html'), false);
      return true;
    },
  );
});

test('browser_ui and background_page targets receive full Network coverage', async () => {
  const targets = [
    { targetId: 'background-1', type: 'background_page', url: 'chrome-extension://fixture/background.html' },
    { targetId: 'browser-ui-1', type: 'browser_ui', url: 'chrome://settings/' },
  ];
  const browser = fakeBrowser({ targets });
  const { watcher } = await start(browser);
  await watcher.flush();
  for (const target of targets) {
    const sessionId = browser.primary.get(target.targetId);
    assert.ok(sessionId);
    assert.ok(browser.calls.some(call => call.method === 'Network.enable' && call.sessionId === sessionId));
    assert.equal(watcher.status().targetsByType[target.type].ready, 1);
  }
  assert.equal(watcher.status().state, 'active');
  await watcher.close();
});

test('browser-managed internal requests are request-scoped while fixture and foreign HTTP stay monitored', async () => {
  const targets = [
    pageInfo(),
    { targetId: 'background-1', type: 'background_page', url: 'chrome-extension://fixture/background.html' },
    { targetId: 'browser-ui-1', type: 'browser_ui', url: 'chrome://settings/' },
    { targetId: 'extension-sw-1', type: 'service_worker', url: 'chrome-extension://fixture/sw.js' },
  ];
  const browser = fakeBrowser({ targets });
  const { watcher, signals } = await start(browser);
  await watcher.flush();

  for (const target of targets) {
    const sessionId = browser.primary.get(target.targetId);
    const url = target.type === 'browser_ui' ? 'chrome://settings/resources.js' :
      'chrome-extension://fixture/internal.js';
    browser.network(sessionId, 'Network.requestWillBeSent', {
      requestId: `internal-${target.targetId}`,
      request: { url, method: 'GET', headers: {} },
    });
    browser.network(sessionId, 'Network.responseReceived', {
      requestId: `internal-${target.targetId}`,
      response: { headers: { 'set-cookie': 'fixture_session=internal-only; Path=/' } },
      hasExtraInfo: true,
    });
    browser.network(sessionId, 'Network.responseReceivedExtraInfo', {
      requestId: `internal-${target.targetId}`,
      headers: { 'set-cookie': 'fixture_session=internal-only; Path=/' },
    });
    browser.network(sessionId, 'Network.loadingFinished', { requestId: `internal-${target.targetId}` });
  }
  await watcher.flush();
  assert.deepEqual(signals, []);
  assert.equal(watcher.status().state, 'active');

  const uiSession = browser.primary.get('browser-ui-1');
  browser.network(uiSession, 'Network.requestWillBeSent', {
    requestId: 'ui-fixture-mutation',
    request: { url: `${origin}/browser-ui-rotate`, method: 'GET', headers: {} },
  });
  browser.network(uiSession, 'Network.responseReceived', {
    requestId: 'ui-fixture-mutation', response: { headers: {} }, hasExtraInfo: true,
  });
  browser.network(uiSession, 'Network.responseReceivedExtraInfo', {
    requestId: 'ui-fixture-mutation',
    headers: { 'set-cookie': 'fixture_session=ui-mutation; Path=/' },
  });
  browser.network(uiSession, 'Network.loadingFinished', { requestId: 'ui-fixture-mutation' });
  assert.equal(signals[0]?.kind, 'unexpected_session_mutation');
  assert.equal(signals[0]?.targetType, 'browser_ui');
  assert.equal(watcher.status().state, 'active');

  browser.network(uiSession, 'Network.requestWillBeSent', {
    requestId: 'ui-foreign-request',
    request: { url: 'https://unrelated.invalid/secret-path', method: 'GET', headers: {} },
  });
  assert.equal(signals[1]?.kind, 'observation_lost');
  await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
  const exposed = JSON.stringify({ signals, status: watcher.status() });
  assert.equal(exposed.includes('secret-path'), false);
  await watcher.close();
});

test('startup covers existing pages and resumes only after Network.enable; duplicate sessions share one observer', async () => {
  const info = pageInfo();
  const browser = fakeBrowser({
    targets: [info],
    rootAttachments: [
      { targetInfo: info, sessionId: 'auto-page', waitingForDebugger: true },
      { targetInfo: info, sessionId: 'manual-page', waitingForDebugger: false },
    ],
  });
  const { watcher } = await start(browser);
  const enable = browser.calls.findIndex(call => call.method === 'Network.enable' && call.sessionId === 'auto-page');
  const resume = browser.calls.findIndex(call => call.method === 'Runtime.runIfWaitingForDebugger' && call.sessionId === 'auto-page');
  assert.ok(enable >= 0 && resume > enable);
  assert.equal(browser.calls.filter(call => call.method === 'Network.enable').length, 1);
  assert.equal(browser.calls.some(call => call.method === 'Target.setAutoAttach' && call.sessionId === 'manual-page'), false);
  assert.equal(watcher.status().targetsByType.page.ready, 1);
  assert.equal(watcher.status().targetsByType.page.initializing, 0);
  await watcher.close();
});

test('nested workers are observed separately and cannot borrow the page refresh nonce exemption', async () => {
  const info = pageInfo();
  const browser = fakeBrowser({ targets: [info] });
  const { watcher, signals } = await start(browser);
  const pageSession = browser.primary.get('page-1');
  const workerInfo = { targetId: 'worker-1', type: 'worker', title: '', url: '' };
  browser.attach(workerInfo, { sessionId: 'worker-session', parentSessionId: pageSession, waitingForDebugger: true });
  await watcher.flush();
  assert.equal(watcher.status().targetsByType.worker.ready, 1);
  const networkEnable = browser.calls.findIndex(call => call.method === 'Network.enable' && call.sessionId === 'worker-session');
  const workerResume = browser.calls.findIndex(call => call.method === 'Runtime.runIfWaitingForDebugger' && call.sessionId === 'worker-session');
  assert.ok(networkEnable >= 0 && workerResume > networkEnable);

  const page = browser.page();
  let controlledId;
  const refresh = watcher.authorizeRefresh(page, request => {
    const promise = request('/rotate', 'POST');
    controlledId = promise.requestId;
    return promise;
  });
  assert.match(controlledId, /^request-/);
  browser.network('worker-session', 'Network.requestWillBeSent', {
    requestId: 'worker-request',
    request: { url: `${origin}/worker-rotate`, method: 'GET', headers: {} },
  });
  browser.network('worker-session', 'Network.responseReceived', {
    requestId: 'worker-request', response: { headers: {} }, hasExtraInfo: true,
  });
  browser.network('worker-session', 'Network.responseReceivedExtraInfo', {
    requestId: 'worker-request', headers: { 'set-cookie': 'fixture_session=worker-secret; Path=/' },
  });
  browser.network('worker-session', 'Network.loadingFinished', { requestId: 'worker-request' });
  browser.respond(controlledId, 'fixture_session=controlled-secret; Path=/');
  assert.deepEqual(await refresh, { status: 200, body: { ok: true } });
  assert.deepEqual(signals.map(signal => [signal.kind, signal.targetType]), [['unexpected_session_mutation', 'worker']]);
  const status = await watcher.flush();
  assert.equal(status.evidenceByKind.unexpected_session_mutation, 1);
  const exposed = JSON.stringify({ signals, status });
  for (const secret of ['worker-secret', 'controlled-secret', origin, '/worker-rotate']) assert.equal(exposed.includes(secret), false);
  await watcher.close();
});

test('confirmed owned target retirement records retired and keeps remaining coverage active', async () => {
  const info = pageInfo();
  const browser = fakeBrowser({ targets: [info] });
  const { watcher } = await start(browser);
  const tabInfo = { targetId: 'tab-page', type: 'page', title: '', url: '' };
  browser.emit({ method: 'Target.targetCreated', params: { targetInfo: tabInfo } });
  browser.attach(tabInfo, { sessionId: 'auto-tab-page', waitingForDebugger: true });
  await watcher.flush();
  await watcher.retireTarget(tabInfo.targetId);
  const status = await watcher.flush();
  assert.equal(status.targetsByType.page.retired, 1);
  assert.equal(status.targetsByType.page.ready, 1);
  assert.equal(status.state, 'active');
  await watcher.close();
});

test('retired target tombstones ignore late type information without weakening live type checks', async t => {
  await t.test('destroyed target late info and discovery', async () => {
    const info = { targetId: 'retired-ui', type: 'browser_ui', url: 'chrome://settings/' };
    const browser = fakeBrowser({ targets: [pageInfo(), info] });
    const { watcher, signals } = await start(browser);
    browser.emit({ method: 'Target.targetDestroyed', params: { targetId: info.targetId } });
    for (const method of ['Target.targetInfoChanged', 'Target.targetCreated']) {
      browser.emit({ method, params: { targetInfo: { ...info, type: 'unsupported-late-type' } } });
    }
    const status = await watcher.flush();
    assert.equal(status.state, 'active');
    assert.equal(status.targetsByType.browser_ui.retired, 1);
    assert.equal(status.targetsByType.page.ready, 1);
    assert.deepEqual(signals, []);
    await watcher.close();
  });
  await t.test('live non-internal type change remains unavailable before destruction', async () => {
    const info = { targetId: 'live-ui', type: 'browser_ui', url: origin };
    const browser = fakeBrowser({ targets: [info] });
    const { watcher, signals } = await start(browser);
    browser.emit({ method: 'Target.targetInfoChanged', params: { targetInfo: { ...info, type: 'page' } } });
    assert.equal(watcher.status().state, 'unavailable');
    assert.ok(signals.some(s => s.evidenceKind === 'target_type_changed'));
    browser.emit({ method: 'Target.targetDestroyed', params: { targetId: info.targetId } });
    assert.equal(watcher.status().state, 'unavailable');
    await watcher.close();
  });
});

test('internal UI type change during explicit retirement requires confirmed disappearance', async t => {
  for (const outcome of ['absent', 'destroyed', 'alive', 'malformed']) {
    await t.test(outcome, async () => {
      const ui = { targetId: 'closing-ui', type: 'browser_ui', url: 'chrome://settings/' };
      const browser = fakeBrowser({ targets: [pageInfo(), ui] });
      const { watcher, signals } = await start(browser);
      const originalSend = browser.send.bind(browser);
      browser.send = async (method, params) => {
        if (method === 'Target.getTargets') {
          if (outcome === 'malformed') return {};
          return { targetInfos: outcome === 'alive' ? [ui] : [] };
        }
        if (method === 'Target.closeTarget') {
          browser.emit({ method: 'Target.targetInfoChanged', params: { targetInfo: { ...ui, type: 'page' } } });
          // Cookie response evidence is still observed while absence is pending.
          const sessionId = browser.primary.get(ui.targetId);
          browser.network(sessionId, 'Network.requestWillBeSent', {
            requestId: 'closing-cookie', request: { url: `${origin}/rotate`, method: 'GET', headers: {} },
          });
          browser.network(sessionId, 'Network.responseReceived', {
            requestId: 'closing-cookie', response: { headers: { 'set-cookie': 'fixture_session=closing' } }, hasExtraInfo: false,
          });
          browser.network(sessionId, 'Network.loadingFinished', { requestId: 'closing-cookie' });
          if (outcome === 'destroyed') browser.emit({ method: 'Target.targetDestroyed', params: { targetId: ui.targetId } });
        }
        return originalSend(method, params);
      };
      if (outcome === 'absent' || outcome === 'destroyed') {
        const status = await watcher.retireTarget('page-1');
        assert.equal(status.state, 'active');
        assert.equal(status.targetsByType.browser_ui.retired, 1);
        assert.deepEqual(status.retirementTypeChanges, [{ targetType: 'page', count: 1 }]);
      } else {
        await assert.rejects(watcher.retireTarget('page-1'), /fixture_browser_observer_unavailable/);
        assert.ok(signals.some(s => s.evidenceKind === 'retiring_ui_disappearance_unconfirmed'));
      }
      assert.ok(signals.some(s => s.kind === 'unexpected_session_mutation'));
      await watcher.close();
    });
  }
});

test('internal UI type change after retireTarget returns still requires disappearance verification', async t => {
  for (const present of [false, true]) {
    await t.test(present ? 'still-present' : 'confirmed-absent', async () => {
      const ui = { targetId: 'late-closing-ui', type: 'browser_ui', url: 'chrome://settings/' };
      const browser = fakeBrowser({ targets: [pageInfo(), ui] });
      const { watcher, signals } = await start(browser);
      await watcher.retireTarget('page-1');
      const originalSend = browser.send.bind(browser);
      browser.send = async (method, params) => method === 'Target.getTargets'
        ? { targetInfos: present ? [ui] : [] } : originalSend(method, params);
      browser.emit({ method: 'Target.targetInfoChanged', params: { targetInfo: { ...ui, type: 'page' } } });
      if (present) {
        await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
        assert.ok(signals.some(s => s.evidenceKind === 'retiring_ui_disappearance_unconfirmed'));
      } else {
        const status = await watcher.flush();
        assert.equal(status.state, 'active');
        assert.equal(status.targetsByType.browser_ui.retired, 1);
        assert.deepEqual(signals, []);
      }
      await watcher.close();
    });
  }
});

test('pending flush barrier rejection is harmless only after complete confirmed retirement', async t => {
  for (const outcome of ['retired', 'still-live', 'retired-incomplete']) {
    await t.test(outcome, async () => {
      const ui = { targetId: 'flush-race-ui', type: 'browser_ui', url: 'chrome://settings/' };
      const browser = fakeBrowser({ targets: [ui] });
      const { watcher, signals } = await start(browser);
      const sessionId = browser.primary.get(ui.targetId);
      const barrierEntered = deferred();
      const barrier = deferred();
      const originalSend = browser.sendSession.bind(browser);
      browser.sendSession = async (id, method, params) => {
        if (id === sessionId && method === 'Runtime.evaluate') {
          barrierEntered.resolve();
          return barrier.promise;
        }
        return originalSend(id, method, params);
      };
      if (outcome === 'retired-incomplete') {
        browser.network(sessionId, 'Network.requestWillBeSent', {
          requestId: 'pending', request: { url: `${origin}/rotate`, method: 'GET', headers: {} },
        });
        browser.network(sessionId, 'Network.responseReceived', {
          requestId: 'pending', response: { headers: {} }, hasExtraInfo: true,
        });
      }
      const flushing = watcher.flush();
      const rejected = outcome === 'retired' ? null : assert.rejects(flushing, /fixture_browser_observer_unavailable/);
      await barrierEntered.promise;
      if (outcome !== 'still-live') browser.emit({ method: 'Target.targetDestroyed', params: { targetId: ui.targetId } });
      barrier.reject(new Error('synthetic target disappeared'));
      if (outcome === 'retired') {
        assert.equal((await flushing).state, 'active');
        assert.deepEqual(signals, []);
      } else {
        await rejected;
        assert.equal(watcher.status().state, 'unavailable');
      }
      await watcher.close();
    });
  }
});

test('unexpected detach, transport loss, and pending ExtraInfo fail closed', async t => {
  await t.test('unexpected detach', async () => {
    const browser = fakeBrowser({ targets: [pageInfo()] });
    const { watcher, signals } = await start(browser);
    browser.emit({ method: 'Target.detachedFromTarget', params: { sessionId: browser.primary.get('page-1') } });
    assert.equal(watcher.status().state, 'unavailable');
    assert.equal(signals[0]?.kind, 'observation_lost');
    await assert.rejects(watcher.flush(), /fixture_browser_observer_unavailable/);
    await watcher.close();
  });

  await t.test('transport loss', async () => {
    const browser = fakeBrowser({ targets: [pageInfo()] });
    const { watcher, signals } = await start(browser);
    browser.disconnect();
    assert.equal(watcher.status().state, 'unavailable');
    assert.equal(signals[0]?.evidenceKind, 'transport_unavailable');
    await watcher.close();
  });

  await t.test('retire refuses incomplete response extra info', async () => {
    const browser = fakeBrowser({ targets: [pageInfo()] });
    const { watcher, signals } = await start(browser);
    const sessionId = browser.primary.get('page-1');
    browser.network(sessionId, 'Network.requestWillBeSent', {
      requestId: 'pending-extra', request: { url: `${origin}/rotate`, method: 'GET', headers: {} },
    });
    browser.network(sessionId, 'Network.responseReceived', {
      requestId: 'pending-extra', response: { headers: {} }, hasExtraInfo: true,
    });
    await assert.rejects(watcher.retireTarget('page-1'), /fixture_browser_observer_unavailable/);
    assert.equal(watcher.status().targetsByType.page.unavailable, 1);
    assert.ok(signals.some(signal => signal.kind === 'observation_lost'));
    await watcher.close();
  });
});

test('close during a pending attachment disables coverage and resumes only the paused owned target', async () => {
  const info = pageInfo();
  const blocker = deferred();
  const browser = fakeBrowser({ targets: [info] });
  const { watcher } = await start(browser);
  const workerInfo = { targetId: 'worker-pending', type: 'worker', title: '', url: '' };
  // The fake transport's attachment event is enough to start target setup; hold its
  // Network.enable through a small wrapper around the existing sendSession method.
  const sendSession = browser.sendSession.bind(browser);
  browser.sendSession = async (sessionId, method, params) => {
    if (sessionId === 'worker-pending-session' && method === 'Network.enable') await blocker.promise;
    return sendSession(sessionId, method, params);
  };
  browser.emit({ method: 'Target.attachedToTarget', sessionId: browser.primary.get('page-1'), params: {
    sessionId: 'worker-pending-session', targetInfo: workerInfo, waitingForDebugger: true,
  } });
  await Promise.resolve(); await Promise.resolve();
  const closing = watcher.close();
  browser.emit({ method: 'Target.attachedToTarget', sessionId: browser.primary.get('page-1'), params: {
    sessionId: 'late-paused-target', targetInfo: { targetId: 'late-worker', type: 'worker', title: '', url: '' },
    waitingForDebugger: true,
  } });
  const status = await closing;
  assert.equal(status.state, 'closed');
  assert.ok(browser.calls.some(call => call.method === 'Runtime.runIfWaitingForDebugger' && call.sessionId === 'worker-pending-session'));
  assert.ok(browser.calls.some(call => call.method === 'Runtime.runIfWaitingForDebugger' && call.sessionId === 'late-paused-target'));
  blocker.resolve();
});
