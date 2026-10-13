import test from 'node:test';
import assert from 'node:assert/strict';
import { observeSessionMutations } from './observer.mjs';

const origin = 'http://127.0.0.1:43127';

function fakePage(pageOrigin = origin) {
  const listeners = new Set();
  const calls = [];
  let sequence = 0;
  let barriers = 0;
  function emit(method, params = {}) {
    for (const listener of [...listeners]) listener.onEvent({ method, params });
  }
  const page = {
    origin: pageOrigin,
    async subscribeNetwork(onEvent, onUnavailable) {
      const listener = { onEvent, onUnavailable };
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
    async networkBarrier() { barriers++; },
    request(path, method = 'GET', options = {}) {
      const requestId = `fixture-request-${++sequence}`;
      let resolve;
      let reject;
      const promise = new Promise((res, rej) => { resolve = res; reject = rej; });
      const call = { path, method, requestId, options, resolve, reject, promise };
      calls.push(call);
      emit('Network.requestWillBeSent', {
        requestId,
        request: {
          url: `${origin}${path}`,
          method,
          headers: options.refreshNonce ? { 'x-aku-fixture-refresh-nonce': options.refreshNonce } : {},
        },
      });
      return promise;
    },
  };
  return {
    page,
    calls,
    emit,
    barriers: () => barriers,
    unavailable() { for (const listener of [...listeners]) listener.onUnavailable(); },
    respond(call, { setCookie, blockedCookies, extraFirst = false, includeExtraInfo = true } = {}) {
      const headers = setCookie ? { 'set-cookie': setCookie } : {};
      const extra = () => {
        if (!includeExtraInfo) return;
        emit('Network.responseReceivedExtraInfo', {
          requestId: call.requestId,
          headers,
          blockedCookies,
        });
      };
      const response = () => emit('Network.responseReceived', {
        requestId: call.requestId,
        response: { headers },
        hasExtraInfo: includeExtraInfo,
      });
      if (extraFirst) { extra(); response(); }
      else { response(); extra(); }
      emit('Network.loadingFinished', { requestId: call.requestId });
      call.resolve({ status: 200, body: { ok: true } });
    },
  };
}

async function makeObserver(fake, overrides = {}) {
  const signals = [];
  const observer = await observeSessionMutations(fake.page, {
    origin,
    sessionId: 'fixture-session-1',
    clientId: 'collector',
    flushTimeoutMs: 25,
    onUnexpected: signal => { signals.push(signal); },
    ...overrides,
  });
  return { observer, signals };
}

test('nonce ties the refresh exemption to one request and still catches an overlapping reader mutation', async () => {
  const fake = fakePage();
  const { observer, signals } = await makeObserver(fake);
  const refresh = observer.authorizeRefresh(request => request('/rotate', 'POST'));
  const controlled = fake.calls[0];
  assert.equal(controlled.path, '/rotate');
  assert.match(controlled.options.refreshNonce, /^[a-f0-9]{64}$/);

  const unrelated = fake.page.request('/reader-rotate', 'GET');
  unrelated.catch(() => {});
  const readerCall = fake.calls[1];
  assert.equal(readerCall.options.refreshNonce, undefined);
  fake.respond(readerCall, { setCookie: 'fixture_session=reader-secret; Path=/' });
  assert.deepEqual(signals, [{
    kind: 'unexpected_session_mutation',
    sessionId: 'fixture-session-1',
    clientId: 'collector',
  }]);

  fake.respond(controlled, { setCookie: 'fixture_session=refresh-secret; Path=/' });
  const result = await refresh;
  assert.equal(result.body.ok, true);
  const status = observer.status();
  assert.equal(status.controlledRefreshRequests, 1);
  assert.equal(status.controlledRefreshMutations, 1);
  assert.equal(status.unexpectedMutations, 1);
  assert.equal(status.pendingExtraInfo, 0);
  assert.equal(status.state, 'active');
  assert.equal(fake.barriers(), 1);
  const evidence = JSON.stringify({ signals, status });
  for (const secret of ['reader-secret', 'refresh-secret', '/reader-rotate', '200', origin]) {
    assert.equal(evidence.includes(secret), false);
  }
  observer.close();
});

test('response extra info can arrive before responseReceived and still pairs at flush', async () => {
  const fake = fakePage();
  const { observer, signals } = await makeObserver(fake);
  const request = fake.page.request('/reader-rotate', 'GET');
  request.catch(() => {});
  fake.respond(fake.calls[0], { setCookie: 'fixture_session=early-secret; Path=/', extraFirst: true });
  assert.equal(signals.length, 1);
  const status = await observer.flush();
  assert.equal(status.pendingExtraInfo, 0);
  assert.equal(status.unexpectedMutations, 1);
  assert.equal(status.state, 'active');
  observer.close();
});

test('blocked session cookies are evidence and cannot be hidden inside a controlled refresh', async () => {
  const fake = fakePage();
  const { observer, signals } = await makeObserver(fake);
  const refresh = observer.authorizeRefresh(request => request('/rotate', 'POST'));
  const call = fake.calls[0];
  fake.respond(call, {
    blockedCookies: [{ cookieLine: 'fixture_session=blocked-secret; Secure' }],
  });
  assert.equal(signals[0]?.kind, 'observation_lost');
  assert.equal(observer.status().blockedSessionCookieResponses, 1);
  assert.equal(observer.status().state, 'faulted');
  await assert.rejects(refresh, /fixture_observer_unavailable/);
  observer.close();
});

test('redirect ambiguity, detach, and missing ExtraInfo fail closed without exposing protocol data', async () => {
  const fake = fakePage();
  const { observer, signals } = await makeObserver(fake);
  const refresh = observer.authorizeRefresh(request => request('/rotate', 'POST'));
  const call = fake.calls[0];
  fake.emit('Network.requestWillBeSent', {
    requestId: call.requestId,
    redirectResponse: { headers: { 'set-cookie': 'fixture_session=redirect-secret; Path=/' } },
    redirectHasExtraInfo: false,
    request: { url: `${origin}/after-redirect`, method: 'GET', headers: {} },
  });
  assert.equal(signals[0]?.kind, 'observation_lost');
  assert.equal(observer.status().state, 'faulted');
  call.resolve({ status: 200, body: { ok: true } });
  await assert.rejects(refresh, /fixture_observer_unavailable/);
  const exposed = JSON.stringify({ signals, status: observer.status() });
  for (const secret of ['redirect-secret', '/after-redirect', origin]) assert.equal(exposed.includes(secret), false);
  observer.close();

  const detachedFake = fakePage();
  const detached = await makeObserver(detachedFake);
  detachedFake.unavailable();
  assert.equal(detached.signals[0]?.kind, 'observation_lost');
  assert.equal(detached.observer.status().state, 'faulted');
  detached.observer.close();

  const missingFake = fakePage();
  const missing = await makeObserver(missingFake);
  const pending = missingFake.page.request('/probe', 'GET');
  pending.catch(() => {});
  missingFake.emit('Network.responseReceived', {
    requestId: missingFake.calls[0].requestId,
    response: { headers: {} },
    hasExtraInfo: true,
  });
  await assert.rejects(missing.observer.flush(), /fixture_observer_extra_info_incomplete/);
  assert.equal(missing.observer.status().state, 'faulted');
  missing.observer.close();
});

test('observer accepts only an exact loopback fixture origin and synthetic session cookie', async () => {
  const fake = fakePage('http://localhost:43127');
  await assert.rejects(observeSessionMutations(fake.page, {
    origin: 'http://localhost:43127',
    sessionId: 'fixture-session-1',
    clientId: 'collector',
    onUnexpected() {},
  }), /fixture_observer_invalid_origin/);

  const local = fakePage();
  await assert.rejects(observeSessionMutations(local.page, {
    origin,
    sessionId: 'fixture-session-1',
    clientId: 'collector',
    sessionCookieName: 'real_auth',
    onUnexpected() {},
  }), /fixture_observer_invalid_cookie_name/);
});
