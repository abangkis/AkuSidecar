import test from 'node:test';
import assert from 'node:assert/strict';
import { QAGuestCapture, qaGuestCommand, QA_GUEST_HOME, finishQAAuthControl } from './x-qa-auth-control.mjs';
const URL = 'https://api.x.com/1.1/account/settings.json';
const request = (id = 'r1', method = 'GET', frameId = 'guest') => ({ requestId: id, frameId, request: { method, url: URL } });
const response = (status = 401) => ({ requestId: 'r1', response: { url: URL, status,
  fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false, headers: { private: 'PRIVATE' } } });
function complete(c, status = 401) {
  c.observe('Network.responseReceived', response(status));
  c.observe('Network.loadingFinished', { requestId: 'r1' });
}
test('empty context without a settings request is inconclusive, never a negative authentication proof', () => {
  const c = new QAGuestCapture('api_v1', 'guest');
  assert.equal(c.finish().diagnostic, 'not_observed');
  c.observe('Network.requestWillBeSent', request());
  assert.equal(c.finish().matchingRequests, 0);
});
test('401, success, CSRF, rate limit and challenge remain distinct non-authorizing metadata', () => {
  for (const [status, expected] of [[401, 'authentication_rejection_candidate'], [200, 'http_200_unqualified'],
    [403, 'other_response'], [429, 'other_response'], [500, 'other_response']]) {
    const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', request()); complete(c, status);
    const result = c.finish(); assert.equal(result.diagnostic, expected);
    assert.equal(result.canAuthorize, false); assert.equal(result.bodyReadAttempts, 0);
    assert.equal(JSON.stringify(result).includes('PRIVATE'), false);
  }
});
test('duplicate IDs, frames, redirects, cache, failures and missing freshness cannot qualify', () => {
  for (const mutate of [c => c.observe('Network.requestWillBeSent', request('r2')),
    c => c.observe('Network.requestWillBeSent', request()),
    c => c.observe('Network.requestServedFromCache', { requestId: 'r1' }),
    c => c.observe('Network.loadingFailed', { requestId: 'r1' }),
    c => c.invalidate()]) {
    const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', request()); complete(c); mutate(c);
    assert.equal(c.finish().diagnostic, 'candidate_rejected');
  }
  for (const patch of [{ frameId: 'foreign' }, { redirectResponse: {} }]) {
    const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', { ...request(), ...patch }); complete(c);
    assert.equal(c.finish().diagnostic, 'candidate_rejected');
  }
  const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', request());
  const r = response(); delete r.response.fromServiceWorker; c.observe('Network.responseReceived', r); c.observe('Network.loadingFinished', { requestId: 'r1' });
  assert.equal(c.finish().diagnostic, 'candidate_rejected');
});
test('separate preflight is ignored but collision with selected ID invalidates', () => {
  const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', request('preflight', 'OPTIONS'));
  c.observe('Network.requestWillBeSent', request()); complete(c);
  assert.equal(c.finish().matchingRequests, 1);
  const other = new QAGuestCapture('api_v1', 'guest'); other.observe('Network.requestWillBeSent', request());
  other.observe('Network.requestWillBeSent', request('r1', 'OPTIONS'));
  assert.equal(other.finish().diagnostic, 'candidate_rejected');
});
test('partial response and unpinned route are not a successful comparison', () => {
  assert.throws(() => new QAGuestCapture('api_i_api', 'guest'));
  const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', request());
  assert.equal(c.finish().diagnostic, 'incomplete');
});
test('guest command gate only permits an empty context and one fixed natural Home navigation', () => {
  const g = {};
  assert.equal(qaGuestCommand('Target.createBrowserContext', { disposeOnDetach: true }, undefined, g), true);
  assert.equal(qaGuestCommand('Target.createBrowserContext', { disposeOnDetach: true, proxyServer: 'PRIVATE' }, undefined, g), false);
  g.contextId = 'c'; g.targetId = 't'; g.sessionId = 's';
  for (const method of ['Network.getCookies', 'Network.setCookies', 'Network.getResponseBody', 'Runtime.evaluate', 'Page.reload']) {
    assert.equal(qaGuestCommand(method, {}, 's', g), false);
  }
  assert.equal(qaGuestCommand('Page.navigate', { url: URL }, 's', g), false);
  assert.equal(qaGuestCommand('Page.navigate', { url: QA_GUEST_HOME }, 'owner', g), false);
  assert.equal(qaGuestCommand('Page.navigate', { url: QA_GUEST_HOME }, 's', g), true);
  g.navigated = true;
  assert.equal(qaGuestCommand('Page.navigate', { url: QA_GUEST_HOME }, 's', g), false);
  assert.equal(qaGuestCommand('Target.disposeBrowserContext', { browserContextId: 'owner' }, undefined, g), false);
  assert.equal(qaGuestCommand('Target.disposeBrowserContext', { browserContextId: 'c' }, undefined, g), true);
  g.closed = true;
  assert.equal(qaGuestCommand('Target.disposeBrowserContext', { browserContextId: 'c' }, undefined, g), false);
});
test('comparison requires verified disposal and preserves unknown authentication even for 401', () => {
  const positive = { schema: 'aku.x.qa-source-qualification.v3', cookies: null, canAuthorize: false, sessionTransferred: false };
  const c = new QAGuestCapture('api_v1', 'guest'); c.observe('Network.requestWillBeSent', request()); complete(c);
  const guest = c.finish();
  const result = finishQAAuthControl(positive, guest, true, true, true);
  assert.equal(result.authentication, 'unknown'); assert.equal(result.canAuthorize, false);
  assert.throws(() => finishQAAuthControl(positive, guest, true, true, false));
  assert.throws(() => finishQAAuthControl({ ...positive, cookies: {} }, guest, true, true, true));
});
