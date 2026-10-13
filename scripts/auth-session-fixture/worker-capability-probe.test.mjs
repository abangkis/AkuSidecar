import test from 'node:test';
import assert from 'node:assert/strict';
import { createWorkerCapabilityProbe, fixtureCookieCheckpoint } from './worker-capability-probe.mjs';

function fakeBrowser() {
  let onEvent, onUnavailable;
  const calls = [];
  return {
    calls, failNetwork: false,
    subscribeProtocol(event, unavailable) { onEvent = event; onUnavailable = unavailable; return () => { onEvent = null; }; },
    async send(method, params) { calls.push({ method, params }); return {}; },
    async sendSession(id, method) {
      calls.push({ id, method });
      if (method === 'Network.enable' && this.failNetwork) throw new Error('fixture_cdp_Network.enable_timeout');
      return {};
    },
    emit(event) { onEvent(event); },
    unavailable() { onUnavailable(); },
  };
}
const origin = 'http://127.0.0.1:12345';
const attach = (id, targetId = 'worker-one') => ({ method: 'Target.attachedToTarget', params: {
  sessionId: id, targetInfo: { targetId, type: 'service_worker', url: `${origin}/service-worker` }, waitingForDebugger: true,
} });
test('single root authority enables Network before resuming; duplicate never enables Network', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  assert.deepEqual(browser.calls[0].params.filter, [{ type: 'service_worker', exclude: false }, { exclude: true }]);
  browser.emit(attach('primary'));
  browser.emit(attach('duplicate'));
  await probe.settle();
  const primary = browser.calls.filter(call => call.id === 'primary').map(call => call.method);
  assert.deepEqual(primary, ['Network.enable', 'Runtime.runIfWaitingForDebugger']);
  assert.deepEqual(browser.calls.filter(call => call.id === 'duplicate').map(call => call.method), ['Runtime.runIfWaitingForDebugger']);
  assert.equal(probe.status().duplicateSessions, 1);
  assert.equal(probe.status().readyTargets, 1);
  await probe.close();
});
test('resume-first is explicitly post-resume and never qualifies startup request coverage', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin, resumeOrder: 'before-network' });
  browser.emit(attach('primary')); await probe.settle();
  assert.deepEqual(browser.calls.filter(call => call.id).map(call => call.method), ['Runtime.runIfWaitingForDebugger', 'Network.enable']);
  assert.equal(probe.status().observationWindow, 'post_resume_only');
  assert.equal(probe.status().startupRequestCoverageQualified, false);
  await probe.close();
});
test('worker restart retains historical authority but excludes detached sessions from readiness', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  browser.emit(attach('first')); await probe.settle();
  browser.emit({ method: 'Target.detachedFromTarget', params: { sessionId: 'first' } });
  browser.emit(attach('second', 'worker-two')); await probe.settle();
  assert.equal(probe.status().authoritativeTargets, 2);
  assert.equal(probe.status().detachedSessions, 1);
  assert.equal(probe.status().readyTargets, 1);
  await probe.close();
});
test('nonfixture service workers do not contribute network setup or fixture readiness', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  const internal = attach('internal', 'other-worker'); internal.params.targetInfo.url = 'chrome-extension://invented/worker.js';
  browser.emit(internal); browser.emit(attach('fixture')); await probe.settle();
  assert.deepEqual(browser.calls.filter(call => call.id === 'internal').map(call => call.method), ['Runtime.runIfWaitingForDebugger']);
  assert.equal(probe.status().ignoredNonFixtureTargets, 1);
  assert.equal(probe.status().authoritativeTargets, 1);
  assert.equal(probe.status().readyTargets, 1);
  await probe.close();
});
test('Network setup failure is sanitized, resumes owned pause, and does not report readiness', async () => {
  const browser = fakeBrowser(); browser.failNetwork = true;
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  browser.emit(attach('primary')); await probe.settle();
  assert.deepEqual(probe.status().setupErrors, ['fixture_cdp_Network.enable_timeout']);
  assert.equal(probe.status().resumed, 1);
  assert.equal(probe.status().readyTargets, 0);
  await probe.close();
});
test('rotation request can lack ExtraInfo; other sessions and URLs cannot count as worker evidence', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  browser.emit(attach('primary')); await probe.settle();
  browser.emit({ sessionId: 'other', method: 'Network.requestWillBeSent', params: { requestId: 'excluded', request: { url: `${origin}/reader-rotate` } } });
  browser.emit({ sessionId: 'primary', method: 'Network.requestWillBeSent', params: { requestId: 'unrelated', request: { url: `${origin}/probe` } } });
  browser.emit({ sessionId: 'primary', method: 'Network.requestWillBeSent', params: { requestId: 'rotation', request: { url: `${origin}/reader-rotate` } } });
  browser.emit({ sessionId: 'primary', method: 'Network.responseReceived', params: { requestId: 'rotation', hasExtraInfo: false, response: { status: 200, headers: {} } } });
  const status = probe.status();
  assert.equal(status.rotateRequests, 1); assert.equal(status.firstRotateStatus, 200);
  assert.equal(status.responseExtraInfo, false); assert.equal(status.responseSetCookie, false);
  assert.equal(JSON.stringify(status).includes(origin), false);
  browser.unavailable(); assert.equal(probe.status().unavailable, true);
  await probe.close();
});
test('fixture cookie checkpoint detects rotation without exposing values, excludes expiry churn', () => {
  const cookie = { domain: '127.0.0.1', name: 'fixture_session', value: 'invented-a', path: '/', httpOnly: true, expires: 123 };
  const first = fixtureCookieCheckpoint([cookie]);
  assert.equal(first.length, 64); assert.equal(first.includes(cookie.value), false);
  assert.equal(first, fixtureCookieCheckpoint([{ ...cookie, expires: 124 }]));
  assert.notEqual(first, fixtureCookieCheckpoint([{ ...cookie, value: 'invented-b' }]));
  assert.throws(() => fixtureCookieCheckpoint([{ ...cookie, domain: '.example.test' }]), /fixture_checkpoint_scope_invalid/);
  assert.throws(() => fixtureCookieCheckpoint([]), /fixture_checkpoint_session_invalid/);
  assert.throws(() => fixtureCookieCheckpoint([cookie, cookie]), /fixture_checkpoint_session_invalid/);
});
test('early ExtraInfo is associated with exact rotation request using boolean metadata only', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  browser.emit(attach('primary')); await probe.settle();
  const secret = 'invented-cookie-never-retained';
  browser.emit({ sessionId: 'primary', method: 'Network.responseReceivedExtraInfo', params: { requestId: 'rotation', headers: { 'Set-Cookie': secret } } });
  browser.emit({ sessionId: 'primary', method: 'Network.requestWillBeSentExtraInfo', params: { requestId: 'rotation', headers: { Cookie: secret } } });
  assert.equal(probe.status().pendingEarlyMetadata, 1);
  assert.equal(probe.status().responseSetCookie, false);
  browser.emit({ sessionId: 'primary', method: 'Network.requestWillBeSent', params: { requestId: 'rotation', request: { url: `${origin}/reader-rotate` } } });
  const status = probe.status();
  assert.equal(status.pendingEarlyMetadata, 0);
  assert.equal(status.requestExtraInfo, true);
  assert.equal(status.responseExtraInfo, true);
  assert.equal(status.responseSetCookie, true);
  assert.equal(status.unavailable, false);
  assert.equal(JSON.stringify(status).includes(secret), false);
  await probe.close();
});
test('early ExtraInfo on unrelated requests is discarded instead of becoming rotation evidence', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  browser.emit(attach('primary')); await probe.settle();
  browser.emit({ sessionId: 'primary', method: 'Network.responseReceivedExtraInfo', params: { requestId: 'unrelated', headers: { 'Set-Cookie': 'synthetic' } } });
  browser.emit({ sessionId: 'primary', method: 'Network.requestWillBeSent', params: { requestId: 'unrelated', request: { url: `${origin}/probe` } } });
  assert.equal(probe.status().pendingEarlyMetadata, 0);
  assert.equal(probe.status().responseSetCookie, false);
  await probe.close();
});
test('early metadata overflow is bounded and marks evidence unavailable', async () => {
  const browser = fakeBrowser();
  const probe = await createWorkerCapabilityProbe(browser, { targetType: 'service_worker', origin });
  browser.emit(attach('primary')); await probe.settle();
  for (let i = 0; i < 65; i++) {
    browser.emit({ sessionId: 'primary', method: 'Network.responseReceivedExtraInfo', params: { requestId: `early-${i}`, headers: {} } });
  }
  assert.equal(probe.status().pendingEarlyMetadata, 64);
  assert.equal(probe.status().earlyMetadataOverflow, true);
  assert.equal(probe.status().unavailable, true);
  await probe.close();
});
test('probe rejects nonlocal origins and invalid modes before enabling observation', async () => {
  for (const candidate of ['https://example.test', 'http://127.0.0.1:12/path', 'http://secret@127.0.0.1:12']) {
    await assert.rejects(createWorkerCapabilityProbe(fakeBrowser(), { targetType: 'service_worker', origin: candidate }), /fixture_invalid_worker_probe/);
  }
});
