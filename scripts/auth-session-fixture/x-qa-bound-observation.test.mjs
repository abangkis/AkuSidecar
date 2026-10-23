import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QABoundObservation, QA_BOUND_BOOTSTRAP_EXPRESSION, createManualV2BoundObservation, projectBoundBootstrap } from './x-qa-bound-observation.mjs';
import { QA_AB_URL } from './x-qa-direct-ab.mjs';

const scope = { owner: 1, profileId: 'PRIVATE_PROFILE', sessionId: 'private-session', targetId: 'private-target' };
function page(id = '12345') {
  return { location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: {
    session: { user_id: id }, entities: { users: { entities: { [id]: { id_str: id } } } },
  } };
}
function setup(pointOptions, readDelay, manual = false, observeCookieWrites = () => {}) {
  let now = 0, bodyReads = 0, b;
  const readBody = async id => {
    bodyReads++; assert.equal(b.capture.consumeRead(id), true);
    if (readDelay) now += readDelay;
    return { schema: 'aku.x.qa-direct-body.v1', category: 'settings_match', settingsMatch: true };
  };
  b = manual
    ? createManualV2BoundObservation(scope, 'frame', readBody, () => now, observeCookieWrites, pointOptions)
    : new QABoundObservation(scope, 'frame', readBody, () => now, observeCookieWrites, pointOptions);
  const emit = (method, params, binding = scope) => b.observe({ method, params, sessionId: scope.sessionId }, binding);
  const document = () => {
    emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'loader', url: 'https://x.com/home' } });
    emit('Runtime.executionContextCreated', { context: { id: 7, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'frame' } } });
  };
  const identity = (id = '12345') => projectBoundBootstrap({ projection: inspectBootstrapCurrentSelf(page(id)), accountId: id },
    (value, projection) => b.acceptIdentity(value, projection));
  const request = (patch = {}, requestId = 'request') => emit('Network.requestWillBeSent', { requestId, frameId: 'frame', loaderId: 'loader',
    request: { method: 'GET', url: QA_AB_URL }, ...patch });
  const response = (patch = {}, finishedPatch = {}, requestId = 'request') => {
    emit('Network.responseReceived', { requestId, frameId: 'frame', loaderId: 'loader', hasExtraInfo: true, response: {
      url: QA_AB_URL, status: 200, mimeType: 'application/json', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false }, ...patch });
    if (patch.hasExtraInfo !== false) emit('Network.responseReceivedExtraInfo', { requestId, statusCode: 200, headers: {} });
    emit('Network.loadingFinished', { requestId, encodedDataLength: 512, ...finishedPatch });
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
test('startup owner pins come from the new document before self acquisition and reject stale contexts', () => {
  const s = setup();
  assert.throws(() => s.b.startupPins(7));
  s.document();
  assert.deepEqual(s.b.startupPins(7), { ownerId: '1', profileId: scope.profileId, contextId: '7',
    targetId: scope.targetId, frameId: 'frame', documentId: 'loader' });
  assert.throws(() => s.b.startupPins(8));
  s.identity();
  assert.throws(() => s.b.startupPins(7));
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

function nonRootSettings(s, requestId = 'child-request', frameId = 'child-frame', loaderId = 'child-loader') {
  s.emit('Network.requestWillBeSent', { requestId, frameId, loaderId, request: { method: 'GET', url: QA_AB_URL } });
}
function nonRootSettingsResponse(s, requestId = 'child-request', frameId = 'child-frame', loaderId = 'child-loader', patch = {}) {
  s.emit('Network.responseReceived', { requestId, frameId, loaderId, hasExtraInfo: false, response: {
    url: QA_AB_URL, status: 200, headers: {}, ...patch,
  } });
  s.emit('Network.loadingFinished', { requestId, encodedDataLength: 128 });
}

test('Manual V2 excludes a well-formed nonroot settings request before or after root identity selection', async () => {
  for (const order of ['before_identity', 'after_identity', 'after_root_selection']) {
    let receipts = 0;
    const s = setup({ deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => { receipts++; } }, undefined, true);
    s.document();
    if (order === 'before_identity') nonRootSettings(s);
    s.identity();
    if (order === 'after_identity') nonRootSettings(s);
    nonRootSettingsResponse(s);
    assert.equal(s.b.capture.chosen, null);
    assert.equal(s.b.capture.matching, 0);
    s.request({}, 'root-request');
    assert.equal(s.b.capture.chosen, 'root-request');
    assert.equal(s.b.capture.matching, 1);
    if (order === 'after_root_selection') {
      nonRootSettings(s, 'later-child-request');
      nonRootSettingsResponse(s, 'later-child-request');
      assert.equal(s.b.capture.chosen, 'root-request');
      assert.equal(s.b.capture.matching, 1);
    }
    s.response({}, {}, 'root-request');
    const report = await s.b.finish(true);
    assert.equal(report.diagnostic, 'bound');
    assert.equal(receipts, 1);
    assert.equal(s.reads(), 1);
  }
});

test('Manual V2 cannot issue a receipt from nonroot settings evidence or a nonroot execution context', async () => {
  let receipts = 0;
  const onlyNonRoot = setup({ deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => { receipts++; } }, undefined, true);
  onlyNonRoot.document(); onlyNonRoot.identity(); nonRootSettings(onlyNonRoot); nonRootSettingsResponse(onlyNonRoot);
  const noSource = await onlyNonRoot.b.finish(true);
  assert.equal(noSource.diagnostic, 'source_unavailable');
  assert.equal(onlyNonRoot.b.capture.matching, 0);
  assert.equal(receipts, 0);

  const childContext = setup({ deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => { receipts++; } }, undefined, true);
  childContext.emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'loader', url: 'https://x.com/home' } });
  childContext.emit('Runtime.executionContextCreated', { context: { id: 8, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'child-frame' } } });
  childContext.identity(); nonRootSettings(childContext);
  const noRootIdentity = await childContext.b.finish(true);
  assert.equal(noRootIdentity.diagnostic, 'scope_lost');
  assert.equal(receipts, 0);
});

test('Manual V2 keeps selected request ID reuse and root response-frame conflicts fail-closed', async () => {
  for (const conflict of ['chosen_id', 'ignored_id', 'response_frame']) {
    let receipts = 0;
    const s = setup({ deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => { receipts++; } }, undefined, true);
    s.document(); s.identity();
    if (conflict === 'ignored_id') nonRootSettings(s, 'same-id');
    if (conflict !== 'ignored_id') s.request({}, 'same-id');
    if (conflict === 'chosen_id') nonRootSettings(s, 'same-id');
    if (conflict === 'ignored_id') s.request({}, 'same-id');
    if (conflict === 'response_frame') s.response({ frameId: 'child-frame' }, {}, 'same-id');
    else s.response({}, {}, 'same-id');
    const report = await s.b.finish(true);
    assert.equal(report.diagnostic, conflict === 'ignored_id' ? 'observed_cookie_write' : 'scope_lost', conflict);
    assert.equal(receipts, 0);
  }
});

test('Manual V2 leaves malformed request tuples, stale root loaders, and pre-identity root requests closed', async () => {
  for (const fault of ['missing_frame', 'malformed_frame', 'missing_loader', 'malformed_request_id', 'root_loader', 'before_identity']) {
    let receipts = 0;
    const s = setup({ deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => { receipts++; } }, undefined, true);
    s.document();
    if (fault !== 'before_identity') s.identity();
    const patch = fault === 'missing_frame' ? { frameId: undefined } :
      fault === 'malformed_frame' ? { frameId: 7 } :
        fault === 'missing_loader' ? { frameId: 'child-frame', loaderId: undefined } :
          fault === 'malformed_request_id' ? { frameId: 'child-frame', loaderId: 'child-loader' } :
        fault === 'root_loader' ? { loaderId: 'stale-loader' } : {};
    s.request(patch, fault === 'malformed_request_id' ? 7 : 'fault-request');
    s.response({}, {}, fault === 'malformed_request_id' ? 7 : 'fault-request');
    const report = await s.b.finish(true);
    assert.equal(report.diagnostic, 'scope_lost', fault);
    assert.equal(receipts, 0);
  }
});

test('Manual V2 nonroot exclusion still observes cookie events, blocked candidates, and unknown headers', async () => {
  for (const evidence of ['blocked_candidate', 'unknown_header']) {
    const observed = [];
    const s = setup({ deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => assert.fail('cookie evidence cannot produce a receipt') },
      undefined, true, raw => observed.push(raw.method));
    s.document(); s.identity(); nonRootSettings(s);
    if (evidence === 'blocked_candidate') {
      s.emit('Network.responseReceivedExtraInfo', { requestId: 'child-request', statusCode: 200, headers: {},
        blockedCookies: [{ cookieLine: 'auth_token=PRIVATE_BLOCKED_VALUE; Path=/' }] });
    } else {
      s.emit('Network.responseReceived', { requestId: 'child-request', frameId: 'child-frame', loaderId: 'child-loader',
        hasExtraInfo: false, response: { url: QA_AB_URL, status: 200, headers: { 'Set-Cookie': 'unknown_private=value; Path=/' } } });
    }
    const report = await s.b.finish(true);
    assert.equal(report.diagnostic, 'observed_cookie_write');
    assert.equal(report.manualCookieGuard[evidence === 'blocked_candidate' ? 'candidateAttempts' : 'ambiguousAttempts'], 1);
    assert.ok(observed.includes('Network.requestWillBeSent'));
    assert.ok(observed.includes(evidence === 'blocked_candidate' ? 'Network.responseReceivedExtraInfo' : 'Network.responseReceived'));
    assert.equal(JSON.stringify(report).includes('PRIVATE_BLOCKED_VALUE'), false);
  }
});

test('ordinary QA retains nonroot settings request rejection and direct-capture selection', async () => {
  const s = setup(); s.document(); nonRootSettings(s); s.identity();
  assert.equal(s.b.capture.chosen, 'child-request');
  assert.equal(s.b.capture.matching, 1);
  assert.equal((await s.b.finish(true)).diagnostic, 'scope_lost');
});
