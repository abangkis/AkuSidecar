import test from 'node:test';
import assert from 'node:assert/strict';
import { qaABCommand, QA_AB_URL, QADirectCapture, projectQADirectBody, finishQADirectAB } from './x-qa-direct-ab.mjs';
const settings = { language: 'en', time_zone: { PRIVATE: 'PRIVATE' }, protected: false, always_use_https: true,
  discoverable_by_email: false, discoverable_by_mobile_phone: false };
const project = body => projectQADirectBody({ body: JSON.stringify(body), base64Encoded: false });
const request = (id = 'r') => ({ requestId: id, frameId: 'f', request: { url: QA_AB_URL, method: 'GET' } });
const response = (status = 200) => ({ requestId: 'r', response: { url: QA_AB_URL, status, mimeType: 'application/json',
  fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false } });
function capture(role = 'a', value = settings) {
  let c, reads = 0;
  c = new QADirectCapture(role, 'f', id => { reads++; assert.equal(c.consumeRead(id), true); return project(value); }, () => 100);
  return { c, reads: () => reads };
}
function complete(c, status = 200) {
  c.observe('Network.requestWillBeSent', request()); c.observe('Network.responseReceived', response(status));
  c.observe('Network.loadingFinished', { requestId: 'r', encodedDataLength: 512 });
}
test('fixed browser command guard separates retained A and empty B; no cookie/header APIs', () => {
  const a = { role: 'a' }, b = { role: 'b' };
  assert.equal(qaABCommand('Target.createTarget', { url: 'about:blank' }, undefined, a), true);
  assert.equal(qaABCommand('Target.createBrowserContext', { disposeOnDetach: true }, undefined, a), false);
  assert.equal(qaABCommand('Target.createTarget', { url: 'about:blank' }, undefined, b), false);
  assert.equal(qaABCommand('Target.createBrowserContext', { disposeOnDetach: true }, undefined, b), true);
  b.contextId = 'empty'; b.targetId = 'tab'; b.sessionId = 's';
  assert.equal(qaABCommand('Target.createTarget', { url: 'about:blank', browserContextId: 'owner' }, undefined, b), false);
  assert.equal(qaABCommand('Page.navigate', { url: QA_AB_URL }, 's', b), true);
  for (const params of [{ url: QA_AB_URL + '?private=1' }, { url: 'https://x.com/home' }, { url: QA_AB_URL, referrer: 'PRIVATE' }]) {
    assert.equal(qaABCommand('Page.navigate', params, 's', b), false);
  }
  for (const method of ['Network.getCookies', 'Network.setCookies', 'Network.setExtraHTTPHeaders', 'Runtime.evaluate']) assert.equal(qaABCommand(method, {}, 's', b), false);
  assert.equal(qaABCommand('Page.navigate', { url: QA_AB_URL }, 'owner', b), false);
  b.navigated = true; assert.equal(qaABCommand('Page.navigate', { url: QA_AB_URL }, 's', b), false);
  assert.equal(qaABCommand('Target.closeTarget', { targetId: 'tab' }, undefined, b), false);
  assert.equal(qaABCommand('Target.disposeBrowserContext', { browserContextId: 'empty' }, undefined, b), true);
  b.closed = true; assert.equal(qaABCommand('Target.disposeBrowserContext', { browserContextId: 'empty' }, undefined, b), false);
});
test('body projection discards settings values, arbitrary error details and malformed/oversized JSON', () => {
  for (const [body, category] of [[settings, 'settings_match'], [{ errors: [{ code: 32, message: 'PRIVATE' }] }, 'error_envelope'],
    [{ challenge: 'PRIVATE', errors: [] }, 'challenge'], [{ PRIVATE: 'PRIVATE' }, 'other_json']]) {
    const result = project(body); assert.equal(result.category, category); assert.equal(JSON.stringify(result).includes('PRIVATE'), false);
  }
  for (const body of ['{', ' '.repeat(16385), '{"errors":[],"errors":[]}']) assert.equal(projectQADirectBody({ body, base64Encoded: false }).category, 'unreadable');
});
test('completed response permits one deadline-bound projected body read only', async () => {
  const { c, reads } = capture(); complete(c); const report = await c.finish();
  assert.equal(report.body.category, 'settings_match'); assert.equal(reads(), 1); assert.equal(report.bodyReadAttempts, 1);
  assert.equal(c.consumeRead('r'), false); assert.equal(report.canAuthorize, false);
  assert.equal(JSON.stringify(report).includes('PRIVATE'), false);
});
test('ambiguous, redirected, cross-frame and cached responses cannot retain a body', async () => {
  for (const mutate of [c => c.observe('Network.requestWillBeSent', request('other')),
    c => c.observe('Network.requestServedFromCache', { requestId: 'r' }), c => c.invalidate()]) {
    const { c } = capture(); complete(c); mutate(c); const r = await c.finish();
    assert.equal(r.body, null); assert.equal(r.diagnostic, 'candidate_rejected');
  }
  for (const patch of [{ frameId: 'other' }, { redirectResponse: {} }]) {
    const { c, reads } = capture(); c.observe('Network.requestWillBeSent', { ...request(), ...patch });
    c.observe('Network.responseReceived', response()); c.observe('Network.loadingFinished', { requestId: 'r', encodedDataLength: 512 });
    assert.equal((await c.finish()).body, null); assert.equal(reads(), 0);
  }
});
test('oversized, HTML, failed and incomplete responses remain metadata only', async () => {
  for (const [mime, size, failed] of [['text/html', 512, false], ['application/json', 16385, false], ['application/json', 512, true]]) {
    const { c, reads } = capture(); c.observe('Network.requestWillBeSent', request()); const r = response(401); r.response.mimeType = mime;
    c.observe('Network.responseReceived', r); c.observe(failed ? 'Network.loadingFailed' : 'Network.loadingFinished', { requestId: 'r', encodedDataLength: size });
    assert.equal((await c.finish()).body, null); assert.equal(reads(), 0);
  }
  const { c } = capture(); c.observe('Network.requestWillBeSent', request()); assert.equal((await c.finish()).diagnostic, 'incomplete');
});
test('deadline expiry, wrong ID, rollback and repeated body consumption are denied', async () => {
  for (const clock of [NaN, 99, 4100]) {
    let time = 100, c;
    c = new QADirectCapture('a', 'f', id => { time = clock; assert.equal(c.consumeRead(id), false); return project(settings); }, () => time);
    complete(c); assert.equal((await c.finish()).body, null);
  }
});
test('two 401s fail the positive control; a candidate rejection never becomes authentication authority', async () => {
  const a = capture('a', { errors: [] }), b = capture('b', { errors: [] }); complete(a.c, 401); complete(b.c, 401);
  const ar = await a.c.finish(), br = await b.c.finish();
  assert.equal(finishQADirectAB(ar, br, true, true, true, true).outcome, 'positive_control_failed');
  const positive = capture(); complete(positive.c); const pr = await positive.c.finish();
  const result = finishQADirectAB(pr, br, true, true, true, true);
  assert.equal(result.outcome, 'negative_response_candidate'); assert.equal(result.authentication, 'unknown');
  assert.equal(result.canAuthorize, false); assert.equal(result.sessionTransferred, false);
  assert.equal(finishQADirectAB(pr, br, true, false, true, true).outcome, 'positive_control_failed');
  assert.throws(() => finishQADirectAB(pr, br, true, true, false, true));
});
test('observed 403 aborted A and completed 400 B fail positive control without any body reads', async () => {
  const a = capture('a'), b = capture('b');
  a.c.observe('Network.requestWillBeSent', request()); a.c.observe('Network.responseReceived', response(403));
  a.c.observe('Network.loadingFailed', { requestId: 'r' }); complete(b.c, 400);
  const ar = await a.c.finish(), br = await b.c.finish();
  assert.equal(ar.status, 403); assert.equal(ar.completed, false); assert.equal(ar.diagnostic, 'candidate_rejected');
  assert.equal(br.status, 400); assert.equal(br.completed, true); assert.equal(br.diagnostic, 'other_response');
  assert.equal(a.reads() + b.reads(), 0);
  assert.equal(finishQADirectAB(ar, br, true, true, true, true).outcome, 'positive_control_failed');
});
