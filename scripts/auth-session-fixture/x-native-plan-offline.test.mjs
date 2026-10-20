import test from 'node:test';
import assert from 'node:assert/strict';
import { OFFLINE_X_PLAN_SCHEMA, OFFLINE_X_INPUT_SCHEMA, validateXOfflineContract,
  createXOfflineEvidenceCombiner } from './x-native-plan-offline.mjs';

let sequence = 0;
const SOURCE = { method: 'GET', origin: 'https://api.x.com', path: '/1.1/account/settings.json' };
const ID = '12345678901234567890';
const NOW = 10000;
const UNIX = 2000000000;
function binding(role) {
  return Object.fromEntries(['ownerId', 'profileId', 'contextId', 'targetId', 'frameId', 'generation']
    .map(field => [field, `${role}-${field}`]));
}
function plan() {
  return { schema: OFFLINE_X_PLAN_SCHEMA, provenance: 'synthetic-fixture', attemptId: `synthetic-attempt-${++sequence}`,
    expectedAccountId: ID, reader: binding('reader'), collector: binding('collector'), source: { ...SOURCE },
    cookies: ['auth_token', 'ct0'].map(name => ({ name, domain: '.x.com', path: '/', hostOnly: false,
      secure: true, httpOnly: name === 'auth_token', sameSite: 'Lax', session: false,
      expires: UNIX + 1000.125, partitionKey: null })),
    qualification: { version: 'account-settings-protected.fixture.v2', source: { ...SOURCE },
      signedIn: 'settings_success', signedOut: 'authentication_required' } };
}
function settings(patch = {}) {
  return JSON.stringify({ language: 'en', time_zone: null, protected: false, always_use_https: true,
    discoverable_by_email: false, discoverable_by_mobile_phone: false, ...patch });
}
function page(account = ID) {
  return { location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: {
    session: { user_id: account }, entities: { users: { entities: { [account]: { id_str: account } } } } } };
}
function role(scope) {
  return { self: { binding: { ...scope }, observedAtMs: NOW - 200, page: page() },
    login: { binding: { ...scope }, requestId: 'synthetic-request', startedAtMs: NOW - 100,
      completedAtMs: NOW - 10, source: { ...SOURCE }, status: 200, mimeType: 'application/json',
      fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false, servedFromCache: false,
      redirected: false, ambiguous: false, complete: true, body: settings() },
    coverage: { binding: { ...scope }, fromMs: NOW - 500, throughMs: NOW, mutation: false,
      accountChange: false, challenge: false, gap: 'none', detached: false, overflow: false } };
}
function evidence(p) {
  return { schema: OFFLINE_X_INPUT_SCHEMA, provenance: 'synthetic-fixture', attemptId: p.attemptId,
    nowMs: NOW, unixSeconds: UNIX,
    cookies: p.cookies.map(c => ({ ...c, value: `synthetic:private_${c.name}` })),
    reader: role(p.reader), collector: role(p.collector) };
}
function evaluate(changeInput = () => {}, changePlan = () => {}) {
  const p = plan(); changePlan(p);
  const input = evidence(p); changeInput(input);
  return createXOfflineEvidenceCombiner(p).evaluate(input);
}
function noAuthority(result) {
  assert.equal(result.canAuthorize, false);
  assert.equal(result.nativeQualified, false);
  assert.equal(result.realAuthTested, false);
  assert.equal(Object.isFrozen(result), true);
}

test('closed contract and positive synthetic composite remain non-authorizing', () => {
  const p = plan();
  assert.equal(validateXOfflineContract(p).reason, 'contract_valid');
  const result = createXOfflineEvidenceCombiner(p).evaluate(evidence(p));
  assert.equal(result.outcome, 'synthetic_ready');
  assert.equal(result.identity, 'match');
  assert.equal(result.authentication, 'authenticated');
  assert.equal(result.continuity, 'synthetic_covered');
  noAuthority(result);
});
test('timezone metadata accepts object string and null but rejects absent array boolean and number', () => {
  for (const [value, valid] of [[{ PRIVATE: 'PRIVATE' }, true], ['PRIVATE', true], [null, true],
    [[], false], [true, false], [42, false]]) {
    const result = evaluate(e => e.reader.login.body = settings({ time_zone: value }));
    assert.equal(result.reason, valid ? 'synthetic_evidence_qualified' : 'body_invalid');
    assert.equal(JSON.stringify(result).includes('PRIVATE'), false); noAuthority(result);
  }
  const result = evaluate(e => { const b = JSON.parse(settings()); delete b.time_zone; e.reader.login.body = JSON.stringify(b); });
  assert.equal(result.reason, 'body_invalid');
});

test('all three exact existing source alternatives can be pinned individually', () => {
  for (const [origin, path] of [['https://x.com', '/i/api/1.1/account/settings.json'],
    ['https://api.x.com', '/1.1/account/settings.json'], ['https://api.x.com', '/i/api/1.1/account/settings.json']]) {
    const p = plan(); p.source = { method: 'GET', origin, path }; p.qualification.source = { ...p.source };
    const input = evidence(p);
    for (const name of ['reader', 'collector']) input[name].login.source = { ...p.source };
    assert.equal(createXOfflineEvidenceCombiner(p).evaluate(input).outcome, 'synthetic_ready');
  }
});

test('native provenance, authority fields and wrong schema cannot enter the offline contract', () => {
  for (const mutate of [p => p.provenance = 'native', p => p.schema = 'aku.x.native-transfer-plan.v1',
    p => p.canAuthorize = true, p => p.cookies[0].value = 'real-token', p => p.expectedAccountId = 123,
    p => p.qualification.version = 'account-settings-protected.fixture.v1']) {
    const p = plan(); mutate(p);
    assert.equal(validateXOfflineContract(p).reason, 'contract_invalid');
    assert.throws(() => createXOfflineEvidenceCombiner(p), error => error.message === 'offline_x_contract_invalid');
  }
  for (const mutate of [e => e.provenance = 'native', e => e.schema = OFFLINE_X_PLAN_SCHEMA,
    e => e.canAuthorize = true, e => e.attemptId = 'other']) {
    assert.equal(evaluate(mutate).reason, 'evidence_invalid');
  }
});

test('cookie ceiling rejects extra, absent, duplicate and wrong-scope tuples', () => {
  for (const mutate of [p => p.cookies.push({ ...p.cookies[0], name: 'twid' }), p => p.cookies.pop(),
    p => p.cookies[1].name = 'auth_token', p => p.cookies[0].domain = '.twitter.com',
    p => p.cookies[0].hostOnly = true, p => p.cookies[0].path = '/home',
    p => p.cookies[0].partitionKey = { topLevelSite: 'https://x.com' },
    p => p.cookies[0].secure = false, p => p.cookies[0].priority = 'High']) {
    const p = plan(); mutate(p); assert.equal(validateXOfflineContract(p).reason, 'contract_invalid');
  }
  for (const mutate of [e => e.cookies.push({ ...e.cookies[0] }), e => e.cookies[1] = { ...e.cookies[0] },
    e => e.cookies[0].value = 'unmarked-native-like-token', e => e.cookies[0].value = '',
    e => e.cookies[0].value = 'synthetic:' + 'a'.repeat(8192),
    e => e.cookies[0].value = 'synthetic:line\nbreak', e => e.cookies[0].domain = 'x.com']) {
    assert.equal(evaluate(mutate).reason, 'evidence_invalid');
  }
});

test('exact expiry and SameSite representation survive validation without widening', () => {
  for (const sameSite of ['Strict', 'Lax', 'None', 'Unspecified']) {
    assert.equal(evaluate(() => {}, p => p.cookies[1].sameSite = sameSite).outcome, 'synthetic_ready');
  }
  for (const expires of [null, -1]) {
    assert.equal(evaluate(() => {}, p => { p.cookies[0].session = true; p.cookies[0].expires = expires; }).outcome, 'synthetic_ready');
  }
  for (const mutate of [e => e.cookies[0].expires = Math.floor(e.cookies[0].expires),
    e => e.cookies[0].httpOnly = false, e => e.cookies[0].sameSite = 'Unspecified']) {
    assert.equal(evaluate(mutate).reason, 'evidence_invalid');
  }
  assert.equal(evaluate(e => e.unixSeconds = UNIX + 2000).reason, 'evidence_invalid');
  for (const expires of [Infinity, NaN, 0, -2, Number.MAX_SAFE_INTEGER + 1]) {
    const p = plan(); p.cookies[0].expires = expires;
    assert.equal(validateXOfflineContract(p).reason, 'contract_invalid');
  }
});

test('8 KiB applies to each serialized tuple and the 16 KiB pair without truncation', () => {
  const p = plan(), input = evidence(p);
  for (const c of input.cookies) {
    c.value = 'synthetic:';
    c.value += 'a'.repeat(8192 - Buffer.byteLength(JSON.stringify(c), 'utf8'));
    assert.equal(Buffer.byteLength(JSON.stringify(c), 'utf8'), 8192);
  }
  assert.equal(createXOfflineEvidenceCombiner(p).evaluate(input).outcome, 'synthetic_ready');
  const q = plan(), oversized = evidence(q);
  oversized.cookies[0].value = input.cookies[0].value + 'a';
  assert.equal(createXOfflineEvidenceCombiner(q).evaluate(oversized).reason, 'evidence_invalid');
});

test('route pinning rejects fallback, OPTIONS, extra query and unqualified origin', () => {
  for (const mutate of [p => p.source.method = 'OPTIONS', p => p.source.origin = 'https://evil.test',
    p => p.source.path += '?private=value', p => p.qualification.source.path = '/i/api/1.1/account/settings.json']) {
    const p = plan(); mutate(p); assert.equal(validateXOfflineContract(p).reason, 'contract_invalid');
  }
  assert.equal(evaluate(e => e.collector.login.source.path = '/i/api/1.1/account/settings.json').reason, 'evidence_invalid');
});

test('HTTP 200, a success shape and even explicit true never replace protected semantics', () => {
  for (const mutate of [p => p.qualification.version = 'unqualified', p => p.qualification.signedIn = 'unknown',
    ...['unknown', 'no_request', 'network_error', 'rate_limited', 'csrf_error', 'challenge']
      .map(value => p => p.qualification.signedOut = value)]) {
    const result = evaluate(e => e.reader.login.body = settings({ authenticated: true }), mutate);
    assert.equal(result.reason, 'login_source_unqualified');
    assert.equal(result.authentication, 'unknown');
    noAuthority(result);
  }
});

test('private account comparison catches distinct valid users, numeric IDs and contradictory maps', () => {
  assert.equal(evaluate(e => e.collector.self.page = page('999')).reason, 'identity_mismatch');
  for (const mutate of [page => page.__INITIAL_STATE__.session.user_id = 123,
    page => page.__INITIAL_STATE__.session.user_id_str = '999',
    page => page.__INITIAL_STATE__.entities.users.entities[ID].id = '999',
    page => page.__INITIAL_STATE__.users = { entities: { [ID]: { id_str: '999' } } },
    page => { page.__INITIAL_STATE__.session = { user_id_str: ID }; },
    page => delete page.__INITIAL_STATE__, page => page.location.pathname = '/other']) {
    const result = evaluate(e => mutate(e.collector.self.page));
    assert.equal(result.reason, 'identity_unavailable');
    assert.equal(result.authentication, 'unknown');
  }
});

test('isActive and auth flags cannot override contradictory login observations', () => {
  assert.equal(evaluate(e => e.reader.self.page.__INITIAL_STATE__.session.isActive = false).outcome, 'synthetic_ready');
  for (const field of ['loggedIn', 'isAuthenticated', 'authenticated']) {
    const result = evaluate(e => e.collector.self.page.__INITIAL_STATE__.session[field] = false);
    assert.equal(result.reason, 'login_conflict');
    assert.equal(result.authentication, 'unknown');
  }
  assert.equal(evaluate(e => e.reader.login.body = settings({ authenticated: false })).reason, 'body_invalid');
});

test('unknown cache, service workers, redirects, ambiguity and partial responses fail closed', () => {
  for (const patch of [{ status: 401 }, { status: 429 }, { mimeType: 'text/html' }, { complete: false },
    { fromDiskCache: true }, { fromServiceWorker: true }, { fromPrefetchCache: true },
    { servedFromCache: null }, { redirected: true }, { ambiguous: true }]) {
    const result = evaluate(e => Object.assign(e.collector.login, patch));
    assert.equal(result.reason, 'response_unqualified');
    assert.equal(result.authentication, 'unknown');
  }
});

test('settings body is bounded, strict JSON and checked for drift, challenges and identity conflict', () => {
  for (const body of ['not json', '[]', '{}', settings({ language: false }), settings({ protected: 'false' }),
    settings({ errors: [] }), settings({ challenge: null }), settings({ authenticated: 'true' }),
    settings().replace('"language":"en"', '"language":"en","language":"en"'),
    '{"unknown":' + '['.repeat(10) + '0' + ']'.repeat(10) + '}',
    '{"unknown":[' + Array(1100).fill('0').join(',') + ']}',
    ' '.repeat(16385), settings({ private_note: '🧪'.repeat(5000) }),
    settings().replace('"en"', '"' + String.fromCharCode(0xd800) + '"')]) {
    assert.equal(evaluate(e => e.reader.login.body = body).reason, 'body_invalid');
  }
  assert.equal(evaluate(e => e.reader.login.body = settings({ user_id: '999' })).reason, 'identity_mismatch');
  assert.equal(evaluate(e => e.reader.login.body = settings({ user_id: ID })).outcome, 'synthetic_ready');
});

test('every role and witness binds exact owner/profile/context/target/frame/generation', () => {
  for (const witness of ['self', 'login', 'coverage']) {
    for (const field of Object.keys(binding('reader'))) {
      assert.equal(evaluate(e => e.reader[witness].binding[field] = 'foreign').reason, 'evidence_invalid');
    }
  }
  for (const field of ['ownerId', 'profileId']) {
    const p = plan(); p.collector[field] = p.reader[field];
    assert.equal(validateXOfflineContract(p).reason, 'contract_invalid');
  }
});

test('five-second freshness covers both observations and the whole request interval', () => {
  for (const mutate of [e => e.reader.self.observedAtMs = NOW - 5001,
    e => e.collector.login.startedAtMs = NOW - 5001, e => e.collector.login.completedAtMs = NOW + 1,
    e => e.reader.self.observedAtMs = NaN, e => e.reader.login.startedAtMs = NOW]) {
    assert.equal(evaluate(mutate).reason, 'evidence_stale');
  }
  assert.equal(evaluate(e => { e.reader.self.observedAtMs = NOW - 5000; e.reader.coverage.fromMs = NOW - 5000; }).outcome, 'synthetic_ready');
});

test('mutation, startup/worker/transport gaps and stale coverage cannot be repaired by matching IDs', () => {
  for (const patch of [{ mutation: true }, { accountChange: true }, { challenge: true }, { detached: true },
    { overflow: true }, { gap: 'startup' }, { gap: 'worker' }, { gap: 'transport' }, { gap: 'timeout' },
    { mutation: null }, { throughMs: NOW - 1 }, { throughMs: NOW + 1 }, { fromMs: NOW }]) {
    const result = evaluate(e => Object.assign(e.reader.coverage, patch));
    assert.equal(result.reason, 'coverage_unqualified');
    assert.equal(result.continuity, 'unknown'); noAuthority(result);
  }
});

test('one-use attempt, close and duplicate factory prohibit replay; caller mutation cannot change frozen plan', () => {
  const p = plan(), input = evidence(p), combiner = createXOfflineEvidenceCombiner(p);
  assert.throws(() => createXOfflineEvidenceCombiner(p), /offline_x_attempt_unavailable/);
  p.expectedAccountId = '999'; p.source.origin = 'https://evil.test'; p.cookies[0].secure = false;
  assert.equal(combiner.evaluate(input).outcome, 'synthetic_ready');
  assert.equal(combiner.evaluate(input).reason, 'attempt_consumed');
  const q = plan(), closed = createXOfflineEvidenceCombiner(q); closed.close(); closed.close();
  assert.equal(closed.evaluate(evidence(q)).reason, 'attempt_consumed');
  const r = plan(), failed = createXOfflineEvidenceCombiner(r);
  assert.equal(failed.evaluate(null).reason, 'evidence_invalid');
  assert.equal(failed.evaluate(evidence(r)).reason, 'attempt_consumed');
});

test('getters/toJSON are never invoked and thrown private text never reaches receipts', () => {
  let invoked = 0;
  const accessor = (object, field) => Object.defineProperty(object, field, { enumerable: true,
    get() { invoked++; throw new Error('private sentinel'); } });
  const p = plan(); accessor(p, 'expectedAccountId');
  assert.equal(validateXOfflineContract(p).reason, 'contract_invalid');
  for (const mutate of [e => accessor(e.cookies[0], 'value'), e => accessor(e, 'nowMs'),
    e => accessor(e.reader.self.page.__INITIAL_STATE__.session, 'user_id'),
    e => accessor(e.cookies, '0'), e => e.cookies[0].toJSON = () => { invoked++; return 'private sentinel'; }]) {
    const result = evaluate(mutate); assert.notEqual(result.outcome, 'synthetic_ready');
    assert.equal(JSON.stringify(result).includes('private sentinel'), false);
  }
  const hostile = new Proxy({}, { getPrototypeOf() { throw new Error('private sentinel'); } });
  assert.equal(validateXOfflineContract(hostile).reason, 'contract_invalid');
  assert.equal(invoked, 0);
});

test('successful and rejected reports contain no raw account, cookie, body or binding values', () => {
  for (const mutate of [() => {}, e => e.collector.self.page = page('999'),
    e => e.reader.login.body = 'private malformed body', e => e.reader.coverage.gap = 'private-error']) {
    const result = evaluate(mutate), serialized = JSON.stringify(result);
    for (const secret of [ID, 'synthetic:private_auth_token', 'synthetic:private_ct0',
      'reader-profileId', 'collector-contextId', 'private malformed body', 'private-error', 'synthetic-attempt-']) {
      assert.equal(serialized.includes(secret), false);
    }
    noAuthority(result);
    assert.deepEqual(Object.keys(result).sort(), ['schema', 'provenance', 'contract', 'transfer', 'identity',
      'authentication', 'continuity', 'refresh', 'outcome', 'reason', 'canAuthorize', 'nativeQualified', 'realAuthTested'].sort());
  }
});
