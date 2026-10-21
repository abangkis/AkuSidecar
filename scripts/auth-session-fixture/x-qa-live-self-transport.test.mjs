import test from 'node:test';
import assert from 'node:assert/strict';
import { QALiveSelfTransport, finishQALiveSelf, qaLiveSelfCommand } from './x-qa-live-self-transport.mjs';
const frames = () => [
  { method: 'Network.requestWillBeSent', sessionId: 's', params: { requestId: 'r', frameId: 'f', loaderId: 'l', type: 'Fetch', request: { method: 'GET', url: 'https://x.com/home' } } },
  { method: 'Network.responseReceived', sessionId: 's', params: { requestId: 'r', frameId: 'f', loaderId: 'l', type: 'Fetch', response: { url: 'https://x.com/home', status: 200, mimeType: 'text/html', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false } } },
  { method: 'Network.loadingFinished', sessionId: 's', params: { requestId: 'r' } },
];
test('fresh response stays pinned to the original document and role channel', () => {
  const capture = new QALiveSelfTransport('s', 'f', 'l'); frames().forEach(v => capture.observe(v));
  const report = finishQALiveSelf({ identity: 'candidate', outcome: 'identity_candidate' }, capture.finish('f', 'l'), 1500);
  assert.equal(report.outcome, 'fresh_identity_observed'); assert.equal(report.canAuthorize, false);
});
for (const kind of ['cache', 'sw', 'prefetch', 'frame', 'loader', 'method', 'redirect', 'duplicate', 'lost', 'navigation', 'missing_complete']) {
  test(`reject ${kind}`, () => {
    const capture = new QALiveSelfTransport('s', 'f', 'l'); const rows = frames();
    if (kind === 'cache') rows[1].params.response.fromDiskCache = true;
    if (kind === 'sw') rows[1].params.response.fromServiceWorker = true;
    if (kind === 'prefetch') delete rows[1].params.response.fromPrefetchCache;
    if (kind === 'frame') rows[0].params.frameId = 'other';
    if (kind === 'loader') rows[1].params.loaderId = 'other';
    if (kind === 'method') rows[0].params.request.method = 'POST';
    if (kind === 'redirect') rows[0].params.redirectResponse = {};
    if (kind === 'duplicate') rows.push(rows[0]);
    if (kind === 'lost') rows.push({ method: 'Target.detachedFromTarget', params: { sessionId: 's' } });
    if (kind === 'navigation') rows.push({ method: 'Page.frameNavigated', sessionId: 's', params: { frame: { id: 'f' } } });
    if (kind === 'missing_complete') rows.pop();
    rows.forEach(v => capture.observe(v)); assert.equal(capture.finish('f', 'l').responseFresh, false);
  });
}
test('foreign role events cannot supply a proof and exact two seconds is late', () => {
  const capture = new QALiveSelfTransport('s', 'f', 'l'); frames().forEach(v => capture.observe({ ...v, sessionId: 'foreign' }));
  assert.equal(capture.finish('f', 'l').responseFresh, false);
  const view = finishQALiveSelf({ identity: 'candidate' }, { documentPreserved: true, responseFresh: true }, 2000);
  assert.equal(view.outcome, 'deadline'); assert.equal(view.nativeQualified, false);
});
test('oversize and stale bootstrap never become a fresh identity', () => {
  const transport = { documentPreserved: true, responseFresh: true };
  assert.equal(finishQALiveSelf({ identity: 'unavailable', outcome: 'body_limit' }, transport, 1).outcome, 'body_limit');
  assert.equal(finishQALiveSelf({ identity: 'unavailable', outcome: 'source_absent' }, transport, 1).outcome, 'identity_unavailable');
});
test('probe command boundary forbids reload, navigation, debugger and cookie access', () => {
  for (const method of ['Page.reload', 'Page.navigate', 'Debugger.enable', 'Debugger.evaluateOnCallFrame', 'Network.getCookies', 'Network.setCookies', 'Network.getResponseBody']) {
    assert.equal(qaLiveSelfCommand(method, {}, 'fixed'), false);
  }
  assert.equal(qaLiveSelfCommand('Runtime.evaluate', { expression: 'other', returnByValue: true, awaitPromise: true, userGesture: false }, 'fixed'), false);
  assert.equal(qaLiveSelfCommand('Runtime.evaluate', { expression: 'fixed', returnByValue: true, awaitPromise: true, userGesture: false }, 'fixed'), true);
});
test('v3 separates parser and initializer limits and rejects oversized result envelopes', () => {
  const transport = { documentPreserved: true, responseFresh: false };
  for (const outcome of ['initializer_limit', 'parser_rejected']) {
    const report = finishQALiveSelf({ identity: 'unavailable', outcome }, transport, 500);
    assert.equal(report.outcome, outcome);
    assert.equal(report.schema, 'aku.x.qa-live-self-run.v3');
    assert.equal(report.canAuthorize, false);
  }
  assert.equal(finishQALiveSelf({ identity: 'unavailable', outcome: 'timeout' }, transport, 1999).outcome, 'deadline');
  assert.throws(() => finishQALiveSelf({ padding: 'x'.repeat(4096) }, transport, 1), /qa_live_projection_limit/);
});
