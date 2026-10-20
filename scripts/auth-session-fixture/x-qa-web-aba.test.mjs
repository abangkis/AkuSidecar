import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import { QA_WEB_INSTALL, QA_WEB_START, QA_WEB_SNAPSHOT, QA_WEB_DISPOSE, validateQAWebHook, qaWebCommand, finishQAWebABA } from './x-qa-web-aba.mjs';

function fixture(body = '{"errors":[]}', origin = 'https://x.com') {
  const traffic = [], xhrTraffic = [];
  const nativeFetch = (input, init) => {
    traffic.push(new Request(input, init));
    return Promise.resolve(new Response(body));
  };
  class XHR {
    open(method, url) { this.url = url; this.method = method; this.headers = []; }
    setRequestHeader(k, v) { this.headers.push([k, v]); }
    send(value) { xhrTraffic.push({ method: this.method, value, withCredentials: this.withCredentials, headers: this.headers }); return 'original-return'; }
  }
  const context = vm.createContext({ location: { origin, pathname: '/home', href: origin + '/home' },
    fetch: nativeFetch, Request, Headers, XMLHttpRequest: XHR, URL, AbortController, setTimeout, clearTimeout });
  vm.runInContext(QA_WEB_INSTALL, context);
  return { context, traffic, xhrTraffic, nativeFetch, run: expression => vm.runInContext(expression, context),
    snapshot() { return validateQAWebHook(vm.runInContext(QA_WEB_SNAPSHOT, context)); } };
}
const URL_SETTINGS = 'https://api.x.com/1.1/account/settings.json';
async function settle(f) {
  for (let i = 0; i < 40 && f.snapshot().outcome === 'started'; i++) await new Promise(resolve => setImmediate(resolve));
}
test('Fetch A passes through; exactly one B keeps opaque application headers in-page and omits cookies', async () => {
  const f = fixture();
  const response = await f.context.fetch(URL_SETTINGS, { credentials: 'include', headers: { 'x-private-fixture': 'SYNTHETIC_ONLY' } });
  assert.equal(await response.text(), '{"errors":[]}'); assert.equal(f.traffic.length, 1);
  assert.equal(f.traffic[0].credentials, 'include'); assert.equal(f.snapshot().transport, 'fetch');
  assert.equal(f.run(QA_WEB_START), true); await settle(f);
  assert.equal(f.traffic.length, 2); assert.equal(f.traffic[1].credentials, 'omit');
  assert.equal(f.traffic[1].headers.get('x-private-fixture'), 'SYNTHETIC_ONLY');
  assert.equal(f.snapshot().outcome, 'completed'); assert.equal(f.run(QA_WEB_START), false);
  assert.equal(JSON.stringify(f.snapshot()).includes('SYNTHETIC_ONLY'), false);
  assert.equal(f.run(QA_WEB_DISPOSE), true); assert.equal(f.context.fetch, f.nativeFetch);
  assert.equal(f.snapshot().disposed, true); assert.equal(f.snapshot().prepared, false);
});
test('XHR A preserves callbacks/credentials and B uses only its captured application request inside the page', async () => {
  const f = fixture(); const xhr = new f.context.XMLHttpRequest();
  xhr.open('GET', URL_SETTINGS); xhr.withCredentials = true; xhr.setRequestHeader('x-private-fixture', 'SYNTHETIC_ONLY');
  assert.equal(xhr.send(), 'original-return'); assert.equal(f.xhrTraffic[0].withCredentials, true);
  assert.equal(f.snapshot().transport, 'xhr'); assert.equal(f.run(QA_WEB_START), true); await settle(f);
  assert.equal(f.traffic.length, 1); assert.equal(f.xhrTraffic.length, 1);
  assert.equal(f.traffic[0].credentials, 'omit'); assert.equal(f.traffic[0].headers.get('x-private-fixture'), 'SYNTHETIC_ONLY');
  assert.equal(JSON.stringify(f.snapshot()).includes('SYNTHETIC_ONLY'), false);
  f.run(QA_WEB_DISPOSE);
});
test('second candidate, disallowed origins/methods, unsafe XHR state and disposed hook cannot issue B', async () => {
  for (const [url, method] of [['https://api.x.com.evil.test/1.1/account/settings.json', 'GET'], [URL_SETTINGS, 'POST']]) {
    const f = fixture(); await f.context.fetch(url, { method }); assert.equal(f.snapshot().candidates, 0); assert.equal(f.run(QA_WEB_START), false); f.run(QA_WEB_DISPOSE);
  }
  const f = fixture(); await f.context.fetch(URL_SETTINGS); await f.context.fetch(URL_SETTINGS);
  assert.equal(f.snapshot().rejected, true); assert.equal(f.run(QA_WEB_START), false); assert.equal(f.traffic.length, 2); f.run(QA_WEB_DISPOSE);
  const x = fixture(); const xhr = new x.context.XMLHttpRequest(); xhr.open('GET', URL_SETTINGS);
  xhr.setRequestHeader('x-private', 'a'.repeat(8193)); xhr.send(); assert.equal(x.run(QA_WEB_START), false); x.run(QA_WEB_DISPOSE);
  const y = fixture(); await y.context.fetch(URL_SETTINGS); y.run(QA_WEB_DISPOSE); assert.equal(y.run(QA_WEB_START), false);
  assert.equal(vm.runInContext('globalThis.__akuQaWebABA_v1', fixture('', 'https://example.test').context), undefined);
});
test('B body drain is bounded and B failures never change A return or enter XHR callbacks', async () => {
  const f = fixture('a'.repeat(16385)); const a = await f.context.fetch(URL_SETTINGS);
  assert.equal((await a.text()).length, 16385); assert.equal(f.run(QA_WEB_START), true); await settle(f);
  assert.equal(f.snapshot().outcome, 'body_limit'); f.run(QA_WEB_DISPOSE);
});
test('closed hook DTO and command gate reject header/script/URL injection and body read replay', () => {
  const f = fixture(); const hook = f.snapshot();
  assert.throws(() => validateQAWebHook({ ...hook, headers: {} })); assert.throws(() => validateQAWebHook({ ...hook, candidates: 3 }));
  const s = { running: true, stage: 'a', reloaded: false };
  assert.equal(qaWebCommand('Page.addScriptToEvaluateOnNewDocument', { source: QA_WEB_INSTALL }, s), true);
  assert.equal(qaWebCommand('Page.addScriptToEvaluateOnNewDocument', { source: 'PRIVATE' }, s), false);
  for (const method of ['Network.getCookies', 'Network.setCookies', 'Network.setExtraHTTPHeaders', 'Page.navigate']) assert.equal(qaWebCommand(method, {}, s), false);
  const params = { expression: QA_WEB_START, returnByValue: true, awaitPromise: false, userGesture: false };
  assert.equal(qaWebCommand('Runtime.evaluate', params, s), false); s.stage = 'b'; assert.equal(qaWebCommand('Runtime.evaluate', params, s), true);
  s.bStarted = true; assert.equal(qaWebCommand('Runtime.evaluate', params, s), false);
  f.run(QA_WEB_DISPOSE);
});
test('A and A again are mandatory; cookie dependency remains a non-authorizing candidate', () => {
  const f = fixture(); f.run(QA_WEB_DISPOSE); const hook = f.snapshot();
  const a = { status: 200, responseFresh: true, completed: true, diagnostic: 'http_200_unqualified', body: { category: 'settings_match' } };
  const b = { status: 401, responseFresh: true, completed: true, diagnostic: 'authentication_rejection_candidate', body: { category: 'error_envelope' } };
  const r = finishQAWebABA(a, b, a, hook, true, true, true);
  assert.equal(r.outcome, 'cookie_dependency_candidate'); assert.equal(r.authentication, 'unknown'); assert.equal(r.canAuthorize, false);
  assert.equal(finishQAWebABA(a, b, { ...a, status: 403 }, hook, true, true, true).outcome, 'positive_control_failed');
  assert.equal(finishQAWebABA(a, null, a, hook, true, true, true).outcome, 'comparison_not_run');
  assert.equal(finishQAWebABA(a, { ...b, status: 403 }, a, hook, true, true, true).outcome, 'comparison_inconclusive');
});
