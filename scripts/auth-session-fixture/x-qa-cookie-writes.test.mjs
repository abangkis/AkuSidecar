import test from 'node:test';
import assert from 'node:assert/strict';
import { projectQACookieHeaders, QACookieWriteCapture, finishQACookieWriteScope } from './x-qa-cookie-writes.mjs';
import { QABoundObservation } from './x-qa-bound-observation.mjs';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';

const setter = { host: 'api.x.com', path: '/1.1/account', role: 'settings' };
const project = (value, scope = setter) => projectQACookieHeaders({ 'Set-Cookie': value }, scope, 'after_identity', 'extra_info');
test('projects names and normalized declared scope without cookie, expiry or unrelated attribute values', () => {
  const raw = 'ct0=PRIVATE_COOKIE_VALUE; Domain=.x.com; Path=/; Secure; HttpOnly; SameSite=None; Partitioned; Expires=Wed, 21 Oct 2037 07:28:00 GMT; Other=PRIVATE_ATTRIBUTE';
  const p = project(raw);
  assert.deepEqual(p, { present: true, unresolved: 0, writes: [{ name: 'ct0', domain: 'x.com', hostOnly: false, path: '/',
    secure: true, httpOnly: true, sameSite: 'none', partitioned: true, phase: 'after_identity', setter: 'api.x.com', requestRole: 'settings', source: 'extra_info' }] });
  for (const secret of ['PRIVATE_COOKIE_VALUE', 'PRIVATE_ATTRIBUTE', '2037', raw]) assert.equal(JSON.stringify(p).includes(secret), false);
});
test('host-only/default-path and newline-separated headers remain separate from explicit domain/root scope', () => {
  const p = project('lang=PRIVATE\nguest_id=PRIVATE2; Domain=X.COM; Path=/; SameSite=Lax');
  assert.equal(p.writes.length, 2);
  assert.equal(p.writes[0].hostOnly, true); assert.equal(p.writes[0].domain, 'api.x.com'); assert.equal(p.writes[0].path, '/1.1/account');
  assert.equal(p.writes[1].hostOnly, false); assert.equal(p.writes[1].domain, 'x.com'); assert.equal(p.writes[1].sameSite, 'lax');
});
test('malformed, duplicate, combined, oversized and accessor header lines never reflect values or guess scope', () => {
  for (const value of ['ct0=PRIVATE; Path=/; Path=/i', 'ct0=PRIVATE; Domain=x.com; Domain=x.com',
    'ct0=PRIVATE; Secure=1', 'ct0=PRIVATE; SameSite=PRIVATE', 'ct0=PRIVATE; Domain=',
    'ct0=PRIVATE; Path=PRIVATE', 'ct0=PRIVATE, auth_token=PRIVATE2', 'ct0=PRIVATE\rhidden',
    'ct0=PRIVATE;' + ' a=x;'.repeat(40), 'x'.repeat(16385)]) {
    const p = project(value); assert.ok(p.unresolved > 0); assert.equal(p.writes.length, 0);
    assert.equal(JSON.stringify(p).includes('PRIVATE'), false);
  }
  let reads = 0;
  const headers = {}; Object.defineProperty(headers, 'Set-Cookie', { enumerable: true, get() { reads++; return 'PRIVATE'; } });
  assert.equal(projectQACookieHeaders(headers, setter, 'after_identity', 'response').unresolved, 1); assert.equal(reads, 0);
});
test('unknown names/domains/paths collapse to fixed labels and unknown setter cannot export supplied strings', () => {
  const p = project('PRIVATE_NAME=PRIVATE_VALUE; Domain=PRIVATE-DOMAIN.test; Path=/PRIVATE_PATH');
  assert.equal(p.writes[0].name, 'other'); assert.equal(p.writes[0].domain, 'other'); assert.equal(p.writes[0].path, 'other');
  assert.equal(JSON.stringify(p).includes('PRIVATE'), false);
  const unbound = project('ct0=PRIVATE', { host: 'PRIVATE_HOST', path: 'PRIVATE_PATH', role: 'PRIVATE_ROLE' });
  assert.equal(unbound.writes[0].domain, 'unknown'); assert.equal(unbound.writes[0].hostOnly, null);
  assert.equal(unbound.writes[0].setter, 'unknown'); assert.equal(JSON.stringify(unbound).includes('PRIVATE'), false);
});

function fixture() {
  const c = new QACookieWriteCapture('frame');
  const emit = (method, params, phase = 'after_identity', loader = 'loader') => c.observe({ method, params }, phase, loader);
  const request = (url = 'https://api.x.com/1.1/account/settings.json') => emit('Network.requestWillBeSent', {
    requestId: 'request', frameId: 'frame', loaderId: 'loader', request: { method: 'GET', url } });
  const header = (line, phase) => emit('Network.responseReceivedExtraInfo', { requestId: 'request', headers: { 'Set-Cookie': line } }, phase);
  return { c, emit, request, header };
}
test('request binding retains only scope categories, preserves phase and never grants storage/session authority', () => {
  const s = fixture(); s.request('https://api.x.com/1.1/account/settings.json?PRIVATE_QUERY');
  s.header('guest_id=PRIVATE; Domain=.x.com; Path=/; Secure', 'before_identity');
  s.header('ct0=PRIVATE2; Domain=.x.com; Path=/; Secure', 'after_identity');
  const p = s.c.finish();
  assert.equal(p.diagnostic, 'projected'); assert.equal(p.qualification, 'candidate_names_present');
  assert.equal(p.acceptance, 'header_only'); assert.equal(p.headerEvents, 2); assert.equal(p.writes[0].phase, 'before_identity');
  assert.equal(p.writes[1].phase, 'after_identity'); assert.equal(p.writes[1].requestRole, 'settings');
  assert.equal(JSON.stringify(p).includes('PRIVATE'), false); assert.throws(() => s.c.finish());
});
test('outside candidate names remain diagnostic only; unknown or uncorrelated writes stay partial', () => {
  const known = fixture(); known.request(); known.header('personalization_id=PRIVATE; Domain=.x.com; Path=/');
  assert.equal(known.c.finish().qualification, 'outside_candidate_names_only');
  for (const line of ['PRIVATE_NAME=PRIVATE; Domain=.x.com; Path=/', 'ct0=PRIVATE; Domain=foreign.test; Path=/']) {
    const s = fixture(); s.request(); s.header(line); const p = s.c.finish();
    assert.equal(p.diagnostic, 'partial'); assert.equal(p.qualification, 'mixed_or_unresolved'); assert.equal(JSON.stringify(p).includes('PRIVATE'), false);
  }
  const missing = fixture(); missing.header('ct0=PRIVATE');
  assert.equal(missing.c.finish().qualification, 'mixed_or_unresolved');
  const absent = fixture(); assert.equal(absent.c.finish().diagnostic, 'not_observed');
});
test('default path strips the final URL segment; unmatched requests and limits cannot produce complete metadata', () => {
  const home = fixture(); home.request('https://x.com/home'); home.header('lang=PRIVATE');
  const p = home.c.finish(); assert.equal(p.writes[0].path, '/'); assert.equal(p.writes[0].domain, 'x.com');
  const unmatched = fixture();
  unmatched.emit('Network.responseReceivedExtraInfo', { requestId: 'missing', headers: { 'Set-Cookie': 'ct0=PRIVATE' } });
  assert.equal(unmatched.c.finish().diagnostic, 'partial');
  const capped = fixture(); capped.request(); for (let i = 0; i < 70; i++) capped.header('ct0=PRIVATE; Path=/');
  const limit = capped.c.finish(); assert.equal(limit.diagnostic, 'partial'); assert.ok(limit.writes.length <= 32); assert.ok(limit.headerEvents <= 64);
});
test('already-observed child-frame X setters correlate without treating them as root identity evidence', () => {
  const s = fixture();
  for (let i = 0; i < 200; i++) s.emit('Network.requestWillBeSent', { requestId: `asset${i}`, frameId: 'frame', request: { url: 'https://foreign.test/PRIVATE' } });
  s.emit('Network.requestWillBeSent', { requestId: 'child', frameId: 'child-frame', loaderId: 'child-loader', request: { url: 'https://x.com/i/PRIVATE_ACTION', method: 'GET' } });
  s.emit('Network.responseReceivedExtraInfo', { requestId: 'child', headers: { 'Set-Cookie': '__cf_bm=PRIVATE_VALUE; Domain=.x.com; Path=/; Secure; HttpOnly; SameSite=None' } });
  const p = s.c.finish();
  assert.equal(p.diagnostic, 'projected'); assert.equal(p.unresolved, 0); assert.equal(p.writes[0].setter, 'x.com');
  assert.equal(p.writes[0].requestRole, 'other_x'); assert.equal(p.qualification, 'outside_candidate_names_only');
  assert.equal(JSON.stringify(p).includes('PRIVATE'), false);
});
test('request ambiguity without a cookie header is not an unresolved cookie write; ambiguous setters remain partial', () => {
  const without = fixture(); without.request(); without.request();
  const empty = without.c.finish(); assert.equal(empty.diagnostic, 'not_observed'); assert.equal(empty.unresolved, 0);
  const withHeader = fixture(); withHeader.request(); withHeader.request(); withHeader.header('__cf_bm=PRIVATE; Domain=.x.com; Path=/');
  const partial = withHeader.c.finish(); assert.equal(partial.diagnostic, 'partial');
  assert.equal(partial.writes[0].setter, 'unknown'); assert.equal(partial.qualification, 'mixed_or_unresolved');
});
test('diagnostic observer leaves the original post-identity cookie-write fence intact', async () => {
  const c = new QACookieWriteCapture('frame');
  const scope = { owner: 1, targetId: 'private-target', sessionId: 'private-session', profileId: 'private-profile' };
  const b = new QABoundObservation(scope, 'frame', () => assert.fail('fenced response must not read its body'), () => 100,
    (raw, phase, loader) => c.observe(raw, phase, loader));
  const emit = (method, params) => b.observe({ method, params, sessionId: scope.sessionId }, scope);
  emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'loader', url: 'https://x.com/home' } });
  emit('Runtime.executionContextCreated', { context: { id: 1, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'frame' } } });
  b.acceptIdentity('12345', inspectBootstrapCurrentSelf({ location: { origin: 'https://x.com', pathname: '/home' }, __INITIAL_STATE__: {
    session: { user_id: '12345' }, entities: { users: { entities: { '12345': { id_str: '12345' } } } } } }));
  emit('Network.requestWillBeSent', { requestId: 'request', frameId: 'frame', loaderId: 'loader', request: { method: 'GET', url: 'https://api.x.com/1.1/account/settings.json' } });
  emit('Network.responseReceivedExtraInfo', { requestId: 'request', statusCode: 200, headers: { 'Set-Cookie': 'personalization_id=PRIVATE; Domain=.x.com; Path=/' } });
  emit('Network.responseReceived', { requestId: 'request', frameId: 'frame', loaderId: 'loader', hasExtraInfo: true,
    response: { url: 'https://api.x.com/1.1/account/settings.json', status: 200, mimeType: 'application/json', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false } });
  emit('Network.loadingFinished', { requestId: 'request', encodedDataLength: 512 });
  const result = finishQACookieWriteScope(await b.finish(true), c.finish());
  assert.equal(result.observation.diagnostic, 'observed_cookie_write'); assert.equal(result.observation.binding, 'unknown');
  assert.equal(result.cookieWrites.qualification, 'outside_candidate_names_only');
  assert.equal(result.canAuthorize, false); assert.equal(result.sessionTransferred, false);
});
