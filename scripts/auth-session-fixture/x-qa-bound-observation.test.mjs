import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QABoundObservation, QA_BOUND_BOOTSTRAP_EXPRESSION, projectBoundBootstrap } from './x-qa-bound-observation.mjs';
import { QA_AB_URL } from './x-qa-direct-ab.mjs';

const scope = { owner: 1, profileId: 'PRIVATE_PROFILE', sessionId: 'private-session', targetId: 'private-target' };
function page(id = '12345') {
  return { location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: {
    session: { user_id: id }, entities: { users: { entities: { [id]: { id_str: id } } } },
  } };
}
function setup(pointOptions, readDelay) {
  let now = 0, bodyReads = 0, b;
  b = new QABoundObservation(scope, 'frame', async id => {
    bodyReads++; assert.equal(b.capture.consumeRead(id), true);
    if (readDelay) now += readDelay;
    return { schema: 'aku.x.qa-direct-body.v1', category: 'settings_match', settingsMatch: true };
  }, () => now, undefined, pointOptions);
  const emit = (method, params, binding = scope) => b.observe({ method, params, sessionId: scope.sessionId }, binding);
  const document = () => {
    emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'loader', url: 'https://x.com/home' } });
    emit('Runtime.executionContextCreated', { context: { id: 7, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'frame' } } });
  };
  const identity = (id = '12345') => projectBoundBootstrap({ projection: inspectBootstrapCurrentSelf(page(id)), accountId: id },
    (value, projection) => b.acceptIdentity(value, projection));
  const request = (patch = {}) => emit('Network.requestWillBeSent', { requestId: 'request', frameId: 'frame', loaderId: 'loader',
    request: { method: 'GET', url: QA_AB_URL }, ...patch });
  const response = (patch = {}, finishedPatch = {}) => {
    emit('Network.responseReceived', { requestId: 'request', frameId: 'frame', loaderId: 'loader', hasExtraInfo: true, response: {
      url: QA_AB_URL, status: 200, mimeType: 'application/json', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false }, ...patch });
    if (patch.hasExtraInfo !== false) emit('Network.responseReceivedExtraInfo', { requestId: 'request', statusCode: 200, headers: {} });
    emit('Network.loadingFinished', { requestId: 'request', encodedDataLength: 512, ...finishedPatch });
  };
  return { b, emit, document, identity, request, response, time: v => { now = v; }, reads: () => bodyReads };
}

test('explicit short QA defers only the mutation gate and keeps private identity out of the observation', async () => {
  let privateResult;
  const s = setup({ deferCookieMutation: true, onPrivateResult: r => { privateResult = r; } });
  s.document(); s.time(100); s.identity(); s.request(); s.time(200);
  assert.equal(s.b.identityReady(), true);
  s.emit('Network.responseReceivedExtraInfo', { headers: { 'Set-Cookie': 'PRIVATE_SECRET' } });
  s.response();
  const r = await s.b.finish(true);
  assert.equal(s.b.identityReady(), false);
  assert.equal(r.diagnostic, 'observed_cookie_write'); assert.equal(r.binding, 'unknown');
  assert.equal(r.login, 'protected_response_observed'); assert.equal(s.reads(), 1);
  assert.deepEqual(privateResult, { accountId: '12345', start: 100, end: 200, networkInterval: null });
  assert.equal(r.continuity, 'unknown'); assert.equal(r.canAuthorize, false);
  assert.equal(JSON.stringify(r).includes('12345'), false); assert.equal(JSON.stringify(r).includes('PRIVATE_SECRET'), false);
  const lost = setup({ deferCookieMutation: true, onPrivateResult: () => {} });
  lost.document(); lost.identity(); lost.emit('Runtime.executionContextsCleared', {}); lost.request(); lost.response();
  assert.equal(lost.b.identityReady(), false);
  assert.equal((await lost.b.finish(true)).diagnostic, 'scope_lost'); assert.equal(lost.reads(), 0);
});

test('freshness is checked again after asynchronous body projection', async () => {
  const s = setup(undefined, 5100); s.document(); s.identity(); s.time(100); s.request(); s.time(200); s.response();
  assert.equal((await s.b.finish(true)).diagnostic, 'stale_observation');
});

test('private request interval uses Chrome wallTime plus monotonic duration, not pipe delivery overlap', async () => {
  let captured;
  const s = setup({ deferCookieMutation: true, onPrivateResult: r => { captured = r; } });
  const wall = Date.now() / 1000;
  s.document(); s.identity();
  s.request({ wallTime: wall, timestamp: 100 });
  s.response({}, { timestamp: 100.2 });
  const r = await s.b.finish(true);
  assert.ok(Math.abs(captured.networkInterval.start - wall * 1000) < 1);
  assert.ok(Math.abs(captured.networkInterval.end - captured.networkInterval.start - 200) < 1);
  assert.equal(Object.hasOwn(r, 'networkInterval'), false);
});

test('one reload binds current-self and protected settings without exporting any private scope or ID', async () => {
  const s = setup(); s.document(); s.time(100); s.identity(); s.request(); s.time(200); s.response();
  const report = await s.b.finish(true);
  assert.equal(report.diagnostic, 'bound'); assert.equal(report.binding, 'same_document_observed');
  assert.equal(report.identity, 'current_self_observed'); assert.equal(report.login, 'protected_response_observed');
  assert.equal(report.freshness, 'within_5s'); assert.equal(s.reads(), 1);
  assert.equal(report.enrollment, 'not_verified'); assert.equal(report.continuity, 'unknown');
  assert.equal(report.canAuthorize, false); assert.equal(report.sessionTransferred, false);
  for (const value of [...Object.values(scope).filter(v => typeof v === 'string'), '12345', 'loader', 'accountId']) {
    assert.equal(JSON.stringify(report).includes(value), false);
  }
  await assert.rejects(s.b.finish(true));
});
test('fixed paused expression uses direct string self ID and rejects conflicting or accessor sources', () => {
  for (const [modify, expected] of [[() => {}, '12345'], [p => { p.__INITIAL_STATE__.session.user_id_str = '54321'; }, null],
    [p => { p.__INITIAL_STATE__.session.user_id = 12345; }, null],
    [p => { p.__INITIAL_STATE__.entities.users.entities['12345'].id = '54321'; }, null],
    [p => { p.__INITIAL_STATE__.session.authenticated = false; }, null]]) {
    const p = page(); modify(p);
    const result = vm.runInNewContext(QA_BOUND_BOOTSTRAP_EXPRESSION, p);
    assert.equal(result.accountId, expected);
  }
  let reads = 0; const p = page();
  Object.defineProperty(p.__INITIAL_STATE__.session, 'user_id', { get() { reads++; return '12345'; } });
  assert.equal(vm.runInNewContext(QA_BOUND_BOOTSTRAP_EXPRESSION, p).accountId, null); assert.equal(reads, 0);
});
test('private dispatch rejects reflected fields, numeric ID, inconsistent projection and accessors', () => {
  const projection = inspectBootstrapCurrentSelf(page());
  for (const input of [{ projection, accountId: 12345 }, { projection, accountId: '01' },
    { projection, accountId: '12345', body: 'PRIVATE' },
    { projection: inspectBootstrapCurrentSelf({ location: page().location }), accountId: '12345' }]) {
    assert.throws(() => projectBoundBootstrap(input, () => {}));
  }
  let reads = 0;
  const input = { projection }; Object.defineProperty(input, 'accountId', { enumerable: true, get() { reads++; return 'PRIVATE'; } });
  assert.throws(() => projectBoundBootstrap(input, () => {})); assert.equal(reads, 0);
});
test('context, document, loader, owner, target and session drift cannot yield a bound observation', async () => {
  const failures = [
    s => s.emit('Runtime.executionContextsCleared', {}),
    s => s.emit('Runtime.executionContextDestroyed', { executionContextId: 7 }),
    s => s.emit('Runtime.executionContextCreated', { context: { id: 8, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'frame' } } }),
    s => s.emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'next', url: 'https://x.com/home' } }),
    s => s.emit('Inspector.detached', {}), s => s.emit('Page.frameDetached', { frameId: 'frame' }),
    ...['owner', 'profileId', 'targetId', 'sessionId'].map(k => s => s.emit('Page.loadEventFired', {}, { ...scope, [k]: 'OTHER' })),
    s => s.emit('Target.detachedFromTarget', { sessionId: scope.sessionId }),
    s => s.identity('54321'),
  ];
  for (const fail of failures) {
    const s = setup(); s.document(); s.identity(); fail(s); s.request(); s.response();
    const r = await s.b.finish(true); assert.equal(r.binding, 'unknown'); assert.equal(r.diagnostic, 'scope_lost');
  }
  const s = setup(); s.document(); s.identity(); s.request({ loaderId: 'old' }); s.response();
  assert.equal((await s.b.finish(true)).diagnostic, 'scope_lost'); assert.equal(s.reads(), 0);
});
test('freshness, clock rollback, overflow, observed cookie write and failed cleanup stay non-authorizing', async () => {
  for (const [change, expected] of [
    [s => s.time(5100), 'stale_observation'], [s => s.time(-1), 'stale_observation'],
    [s => { for (let i = 0; i < 4097; i++) s.emit('Page.loadEventFired', {}); }, 'scope_lost'],
    [s => s.emit('Network.responseReceivedExtraInfo', { headers: { 'Set-Cookie': 'PRIVATE_SECRET' } }), 'observed_cookie_write'],
  ]) {
    const s = setup(); s.document(); s.identity(); change(s); s.request(); s.response();
    const r = await s.b.finish(true); assert.equal(r.binding, 'unknown'); assert.equal(r.diagnostic, expected);
    assert.equal(JSON.stringify(r).includes('PRIVATE_SECRET'), false);
  }
  const s = setup(); s.document(); s.identity(); s.request(); s.response();
  assert.equal((await s.b.finish(false)).diagnostic, 'cleanup_unverified');
});
test('startup writes precede the identity baseline; post-baseline writes and pre-identity settings still fence', async () => {
  const s = setup(); s.document();
  s.emit('Network.responseReceivedExtraInfo', { headers: { 'set-cookie': 'PRIVATE_STARTUP_VALUE' } });
  s.identity(); s.request(); s.response();
  const r = await s.b.finish(true);
  assert.equal(r.diagnostic, 'bound'); assert.equal(r.startupCookieWriteObserved, true);
  assert.equal(JSON.stringify(r).includes('PRIVATE_STARTUP_VALUE'), false);
  const early = setup(); early.document(); early.request(); early.identity(); early.response();
  assert.equal((await early.b.finish(true)).diagnostic, 'scope_lost');
  const late = setup(); late.document(); late.identity(); late.request(); late.response();
  late.emit('Network.responseReceivedExtraInfo', { headers: { 'Set-Cookie': 'PRIVATE_LATE_VALUE' } });
  assert.equal((await late.b.finish(true)).diagnostic, 'observed_cookie_write');
  const query = setup(); query.document(); query.request({ request: { method: 'GET', url: QA_AB_URL + '?fixed=1' } }); query.identity(); query.response();
  assert.equal((await query.b.finish(true)).diagnostic, 'scope_lost');
});
test('required ExtraInfo must be complete and consistent before the observation can close', async () => {
  const valid = setup(); valid.document(); valid.identity(); valid.request(); valid.response({ hasExtraInfo: false });
  assert.equal((await valid.b.finish(true)).diagnostic, 'bound');
  const missing = setup(); missing.document(); missing.identity(); missing.request(); missing.response({ hasExtraInfo: undefined });
  assert.equal((await missing.b.finish(true)).diagnostic, 'scope_lost');
  const conflicting = setup(); conflicting.document(); conflicting.identity(); conflicting.request();
  conflicting.emit('Network.responseReceivedExtraInfo', { requestId: 'request', statusCode: 304, headers: {} });
  conflicting.response();
  assert.equal((await conflicting.b.finish(true)).diagnostic, 'scope_lost');
});
test('old requests, OPTIONS, missing self, duplicate requests, cache and non-200 do not become bound login', async () => {
  const old = setup(); old.request(); old.response(); old.document(); old.identity();
  assert.equal((await old.b.finish(true)).diagnostic, 'source_unavailable'); assert.equal(old.reads(), 0);
  const missing = setup(); missing.document();
  assert.equal((await missing.b.finish(true)).diagnostic, 'identity_unavailable');
  for (const extra of [s => s.request(), s => s.emit('Network.requestServedFromCache', { requestId: 'request' })]) {
    const s = setup(); s.document(); s.identity(); s.request(); extra(s); s.response();
    assert.equal((await s.b.finish(true)).binding, 'unknown');
  }
  const s = setup(); s.document(); s.identity(); s.request({ request: { method: 'OPTIONS', url: QA_AB_URL } }); s.response();
  assert.equal((await s.b.finish(true)).binding, 'unknown'); assert.equal(s.reads(), 0);
});
