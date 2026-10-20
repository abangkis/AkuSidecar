import test from 'node:test';
import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { QASourceCapture, qaSourceRoute, qaCookieRequest, projectQASourceBody, projectQACookieMetadata, finishQASourceQualification } from './x-qa-source-qualification.mjs';
import { LoginObservationGate } from './x-native-metadata-probe-driver.mjs';

const URL = 'https://api.x.com/1.1/account/settings.json';
const body = () => ({ body: JSON.stringify({ language: 'en', time_zone: null, protected: false,
  always_use_https: true, discoverable_by_email: false, discoverable_by_mobile_phone: false,
  screen_name: 'PRIVATE_HANDLE' }), base64Encoded: false });
const cookie = (name = 'auth_token') => ({ name, value: 'PRIVATE_COOKIE_VALUE', domain: '.x.com', path: '/',
  expires: 4102444800.125, size: 100, httpOnly: true, secure: true, session: false,
  sameSite: 'Lax', priority: 'Medium', sourceScheme: 'Secure', sourcePort: 443 });
const request = (id = 'r1', url = URL, method = 'GET', frameId = 'main') => ({ requestId: id,
  frameId, request: { method, url } });
function response(id = 'r1', url = URL) {
  return { requestId: id, response: { url, status: 200, mimeType: 'application/json',
    fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false } };
}
function capture() {
  let selected, reads = 0;
  selected = new QASourceCapture({ frameId: 'main', now: () => 1000,
    readBody(id) { reads++; assert.equal(selected.consumeRead(id), true); return projectQASourceBody(body()); } });
  return { selected, reads: () => reads };
}
function complete(selected) {
  selected.observe('Network.responseReceived', response());
  selected.observe('Network.loadingFinished', { requestId: 'r1', encodedDataLength: 500 });
}
test('source route is exact and query/credentials/foreign origins never escape into metadata', () => {
  assert.equal(qaSourceRoute(URL + '?secret=PRIVATE'), 'api_v1');
  assert.deepEqual(qaCookieRequest('api_v1'), { urls: ['https://x.com/home', URL] });
  for (const value of ['https://api.x.com.evil.test/1.1/account/settings.json', URL + '/other',
    'https://user:PRIVATE@api.x.com/1.1/account/settings.json', URL + '#private']) assert.equal(qaSourceRoute(value), null);
  assert.throws(() => qaCookieRequest('https://evil.test'));
});
test('bounded settings projection checks types but never claims authentication or exports values', () => {
  const projected = projectQASourceBody(body());
  assert.equal(projected.contractMatches, true);
  assert.equal(projected.canAuthorize, false);
  assert.equal(JSON.stringify(projected).includes('PRIVATE_HANDLE'), false);
  for (const patch of [{ protected: 'false' }, { authenticated: false }, { errors: [] }, { language: 7 }]) {
    const value = JSON.parse(body().body); Object.assign(value, patch);
    assert.equal(projectQASourceBody({ body: JSON.stringify(value), base64Encoded: false }).contractMatches, false);
  }
  for (const text of ['{', ' '.repeat(16385), '{"language":1,"language":2}']) {
    assert.equal(projectQASourceBody({ body: text, base64Encoded: false }).contractMatches, null);
  }
});
test('six fixed field kinds identify mismatches without values, object keys or contract relaxation', () => {
  const value = JSON.parse(body().body);
  value.time_zone = { PRIVATE_TIMEZONE_KEY: 'PRIVATE_TIMEZONE_VALUE' };
  value.protected = 'PRIVATE_STRING';
  delete value.discoverable_by_email;
  value.discoverable_by_mobile_phone = ['PRIVATE_ARRAY_ITEM'];
  const result = projectQASourceBody({ body: JSON.stringify(value), base64Encoded: false });
  assert.deepEqual(result.settingsFields, {
    language: { kind: 'string', matches: true }, time_zone: { kind: 'object', matches: true },
    protected: { kind: 'string', matches: false }, always_use_https: { kind: 'boolean', matches: true },
    discoverable_by_email: { kind: 'absent', matches: false }, discoverable_by_mobile_phone: { kind: 'array', matches: false },
  });
  assert.equal(result.contractMatches, false);
  assert.equal(JSON.stringify(result).includes('PRIVATE'), false);
  const encoded = projectQASourceBody({ body: Buffer.from(JSON.stringify(value)).toString('base64'), base64Encoded: true });
  assert.deepEqual(encoded.settingsFields, result.settingsFields);
  const malformed = projectQASourceBody({ body: '{', base64Encoded: false });
  assert.equal(malformed.settingsFields, null);
});
test('timezone object string and null match without exposing nested data; other types fail', () => {
  for (const [value, matches] of [[{ PRIVATE: ['PRIVATE'] }, true], ['PRIVATE', true], [null, true],
    [[], false], [true, false], [42, false]]) {
    const text = JSON.parse(body().body); text.time_zone = value;
    const result = projectQASourceBody({ body: JSON.stringify(text), base64Encoded: false });
    assert.equal(result.contractMatches, matches); assert.equal(result.settingsFields.time_zone.matches, matches);
    assert.equal(JSON.stringify(result).includes('PRIVATE'), false);
  }
  const absent = JSON.parse(body().body); delete absent.time_zone;
  assert.equal(projectQASourceBody({ body: JSON.stringify(absent), base64Encoded: false }).contractMatches, false);
});
test('one same-frame natural response pins the exact route, ignores separate OPTIONS and consumes one read', async () => {
  const { selected, reads } = capture();
  selected.observe('Network.requestWillBeSent', request('options', URL, 'OPTIONS'));
  selected.observe('Network.requestWillBeSent', request('r1', URL + '?private=QUERY'));
  complete(selected);
  const result = await selected.finish();
  assert.equal(reads(), 1); assert.equal(result.route, 'api_v1');
  assert.equal(result.diagnostic, 'projected'); assert.equal(result.bodyContractMatches, true);
  assert.equal(result.authentication, 'unknown'); assert.equal(result.negativeControl, 'not_run');
  assert.equal(result.canAuthorize, false); assert.equal(JSON.stringify(result).includes('QUERY'), false);
  assert.equal(selected.consumeRead('r1'), false);
});
test('cross-frame requests, route changes, collisions, cache, ambiguity and invalidation reject source evidence', async () => {
  for (const mutate of [c => c.observe('Network.requestWillBeSent', request('foreign', URL, 'GET', 'iframe')),
    c => c.observe('Network.responseReceived', response('r1', 'https://api.x.com/i/api/1.1/account/settings.json')),
    c => c.observe('Network.requestWillBeSent', request('r1', URL, 'OPTIONS')),
    c => c.observe('Network.requestWillBeSent', request('r2')),
    c => c.observe('Network.requestServedFromCache', { requestId: 'r1' }), c => c.invalidate()]) {
    const { selected } = capture(); selected.observe('Network.requestWillBeSent', request()); mutate(selected); complete(selected);
    const result = await selected.finish(); assert.notEqual(result.diagnostic, 'projected');
    assert.equal(result.bodyContractMatches, null);
  }
});
test('metadata keeps only two approved tuples and exact attributes, never values or credential hashes', () => {
  const result = projectQACookieMetadata({ cookies: [cookie(), cookie('ct0'), cookie('unlisted')] }, 2000000000);
  assert.deepEqual(result.tuples.map(t => [t.name, t.status]), [['auth_token', 'candidate'], ['ct0', 'candidate']]);
  assert.equal(result.tuples[0].attributes.expires, 4102444800.125);
  assert.equal(result.tuples[0].attributes.hostOnly, false);
  for (const value of ['PRIVATE_COOKIE_VALUE', 'unlisted', '"value"']) assert.equal(JSON.stringify(result).includes(value), false);
  assert.equal(result.sessionTransferred, false); assert.equal(result.valuesPersisted, false); assert.equal(result.canAuthorize, false);
});
test('post-observation fixed Home read precedes one scoped cookie read without reopening initial polling', async () => {
  const { selected } = capture(); selected.observe('Network.requestWillBeSent', request()); complete(selected);
  const source = await selected.finish();
  const gate = new LoginObservationGate(); gate.setLoginVisible(true); gate.beginObservation(); gate.markNetworkEnabled(); gate.markReloaded();
  const calls = [];
  const result = await finishQASourceQualification(source, {
    readHome: async () => { assert.equal(gate.observationStarted, true); calls.push('fixed_home'); return true; },
    readCookies: async route => { calls.push(route); return projectQACookieMetadata({ cookies: [cookie(), cookie('ct0')] }); },
  });
  assert.deepEqual(calls, ['fixed_home', 'api_v1']); assert.notEqual(result.cookies, null);
  assert.throws(() => gate.beginObservation(), /observation_gate_closed/);
  assert.equal(result.sessionTransferred, false); assert.equal(result.canAuthorize, false);
  for (const ready of [false, null]) {
    let reads = 0;
    const operation = finishQASourceQualification(source, { readHome: async () => ready, readCookies: async () => { reads++; } });
    if (ready === false) assert.equal((await operation).cookies, null);
    else await assert.rejects(operation, /qa_source_finalization_failed/);
    assert.equal(reads, 0);
  }
  await assert.rejects(finishQASourceQualification(source, { readHome: async () => { throw new Error('PRIVATE'); }, readCookies() {} }),
    error => error.message === 'qa_source_finalization_failed');
});
test('settings-only finalization cannot invoke a cookie reader even when the source and Home succeed', async () => {
  const { selected } = capture(); selected.observe('Network.requestWillBeSent', request()); complete(selected);
  const source = await selected.finish(); let reads = 0;
  const result = await finishQASourceQualification(source, { inspectCookies: false, readHome: async () => true,
    readCookies() { reads++; throw new Error('cookie read forbidden'); } });
  assert.equal(reads, 0); assert.equal(result.cookies, null);
  assert.equal(result.schema, 'aku.x.qa-source-qualification.v3');
  assert.deepEqual(result.source.settingsFields, projectQASourceBody(body()).settingsFields);
  assert.equal(result.canAuthorize, false); assert.equal(result.sessionTransferred, false);
});
test('cookie absence, scope shadows, expiry and unsupported state remain separate named outcomes', () => {
  const cases = [[[], 'missing'], [[cookie(), cookie()], 'ambiguous'],
    [[{ ...cookie(), domain: 'x.com' }], 'scope_mismatch'], [[{ ...cookie(), path: '/other' }], 'scope_mismatch'],
    [[{ ...cookie(), expires: 100 }], 'expired'], [[{ ...cookie(), partitionKey: {} }], 'unsupported'],
    [[{ ...cookie(), secure: false }], 'unsupported'], [[{ ...cookie(), value: 'a'.repeat(8192) }], 'unsupported'],
    [[{ ...cookie(), unknown: 'PRIVATE' }], 'unsupported'], [Array(65).fill(cookie()), 'unsupported']];
  for (const [cookies, expected] of cases) {
    const result = projectQACookieMetadata({ cookies }, 2000000000);
    assert.equal(result.tuples[0].status, expected); assert.equal(result.tuples[0].attributes, null);
  }
  const absentSameSite = cookie(); delete absentSameSite.sameSite;
  assert.equal(projectQACookieMetadata({ cookies: [absentSameSite] }, 2000000000).tuples[0].attributes.sameSite, 'Unspecified');
  assert.equal(projectQACookieMetadata({ cookies: [{ ...cookie(), session: true, expires: -1 }] }).tuples[0].status, 'candidate');
});
test('getters are never invoked, including incidental nonallowlisted cookie values', () => {
  let reads = 0;
  const accessor = c => Object.defineProperty(c, 'value', { get() { reads++; throw new Error('PRIVATE'); }, enumerable: true });
  const result = projectQACookieMetadata({ cookies: [accessor(cookie()), accessor(cookie('unlisted'))] });
  assert.equal(result.tuples[0].status, 'unsupported'); assert.equal(reads, 0);
  const raw = body(); Object.defineProperty(raw, 'body', { get() { reads++; return 'PRIVATE'; }, enumerable: true });
  assert.equal(projectQASourceBody(raw).contractMatches, null); assert.equal(reads, 0);
});
test('dedicated entrypoint denies arbitrary commands, new profiles and pre-owner reads without touching Chrome', () => {
  const messages = [{ op: 'fixture_bootstrap', withholdReady: false },
    { id: 1, op: 'qa_profile_scope', nonce: 'a'.repeat(64), reuse: false },
    { id: 2, op: 'launch', role: 'collector', nativeReaderStartup: false },
    { id: 3, op: 'qualify_source', owner: 1 }, { id: 4, op: 'Network.getCookies' },
    { id: 5, op: 'qualify_source', owner: 1, url: 'https://evil.test' },
    { id: 6, op: 'start_observation', owner: 1 }, { id: 7, op: 'Storage.setCookies', cookies: [] }];
  const replies = invokeDriver(messages);
  assert.deepEqual(replies.slice(1).map(r => r.code), ['retained_profile_required', 'retained_scope_required',
    'qualification_mode_required', 'rpc_operation_unsupported', 'rpc_fields_invalid', 'qualification_mode_required', 'rpc_operation_unsupported']);
});
function invokeDriver(messages) {
  const result = spawnSync(process.execPath, [fileURLToPath(new globalThis.URL('./x-retained-source-qualification-driver.mjs', import.meta.url)),
    process.execPath, process.cwd(), 'http://127.0.0.1:12345'], { input: messages.map(m => JSON.stringify(m)).join('\n') + '\n',
    encoding: 'utf8', timeout: 3000, maxBuffer: 65536, windowsHide: true });
  assert.equal(result.error, undefined); assert.equal(result.status, 0); assert.equal(result.stderr, '');
  return result.stdout.trim().split('\n').map(line => JSON.parse(line));
}
test('authentication control denies pre-owner calls, caller URLs and direct context commands', () => {
  const replies = invokeDriver([{ op: 'fixture_bootstrap', withholdReady: false },
    { id: 1, op: 'qualify_auth_control', owner: 1 },
    { id: 2, op: 'qualify_auth_control', owner: 1, url: 'https://evil.test' },
    { id: 3, op: 'Target.createBrowserContext' },
    { id: 4, op: 'qualify_direct_ab', owner: 1 },
    { id: 5, op: 'qualify_direct_ab', owner: 1, url: 'https://evil.test' },
    { id: 6, op: 'qualify_direct_ab', owner: 1, headers: {} }]);
  assert.deepEqual(replies.slice(1).map(r => r.code), ['owner_unavailable', 'rpc_fields_invalid', 'rpc_operation_unsupported',
    'owner_unavailable', 'rpc_fields_invalid', 'rpc_fields_invalid']);
});
