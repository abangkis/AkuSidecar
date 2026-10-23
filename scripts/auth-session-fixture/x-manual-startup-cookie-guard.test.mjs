import test from 'node:test';
import assert from 'node:assert/strict';
import { ManualStartupCookieGuard } from './x-manual-startup-cookie-guard.mjs';
import { QABoundObservation, createManualV2BoundObservation } from './x-qa-bound-observation.mjs';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QA_AB_URL } from './x-qa-direct-ab.mjs';
import { createManualStartupOwner } from './x-manual-startup-owner.mjs';
import { requireBoundStartup, startupDiagnostic, startupFailureLatch } from './x-manual-startup-diagnostic.mjs';

const projection = inspectBootstrapCurrentSelf({ location: { origin: 'https://x.com', pathname: '/home' },
  __INITIAL_STATE__: { session: { user_id: '12345' }, entities: { users: { entities: { '12345': { id_str: '12345' } } } } } });
function sample(factory = createManualV2BoundObservation, options) {
  const scope = { owner: 1, profileId: 'private-profile', sessionId: 'private-session', targetId: 'private-target' };
  let now = 0, published = 0, binder;
  binder = factory(scope, 'frame', async id => {
    assert.equal(binder.capture.consumeRead(id), true);
    return { schema: 'aku.x.qa-direct-body.v1', category: 'settings_match', settingsMatch: true };
  }, () => now, undefined, options ?? { deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: () => { published++; } });
  const emit = (method, params, binding = scope) => binder.observe({ method, params, sessionId: scope.sessionId }, binding);
  const document = () => {
    emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'loader', url: 'https://x.com/home' } });
    emit('Runtime.executionContextCreated', { context: { id: 7, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'frame' } } });
  };
  const request = (id = 'other', patch = {}) => emit('Network.requestWillBeSent', { requestId: id, frameId: 'frame', loaderId: 'loader', request: { method: 'GET', url: 'https://x.com/asset' }, ...patch });
  const response = (id = 'other', headers = {}, patch = {}) => emit('Network.responseReceived', { requestId: id, frameId: 'frame', loaderId: 'loader', hasExtraInfo: false, response: { url: 'https://x.com/asset', status: 200, headers }, ...patch });
  const extra = (id = 'other', headers = {}, patch = {}) => emit('Network.responseReceivedExtraInfo', { requestId: id, statusCode: 200, headers, ...patch });
  const settings = (status = 200) => {
    request('settings', { request: { method: 'GET', url: QA_AB_URL } });
    now++;
    response('settings', {}, { response: { url: QA_AB_URL, status, headers: {}, mimeType: 'application/json', fromDiskCache: false, fromServiceWorker: false, fromPrefetchCache: false } });
    emit('Network.loadingFinished', { requestId: 'settings', encodedDataLength: 32 });
  };
  return { binder, scope, emit, document, request, response, extra, settings, time: value => { now = value; },
    identity: () => binder.acceptIdentity('12345', projection), published: () => published };
}
async function finish(s) { s.identity(); s.settings(); return s.binder.finish(true); }
function guard() {
  const g = new ManualStartupCookieGuard('frame');
  return { g, emit: (method, params, loader = 'loader', context = 7) => g.observe({ method, params }, loader, context),
    request: (id = 'r', patch = {}) => g.observe({ method: 'Network.requestWillBeSent', params: { requestId: id, loaderId: 'loader', frameId: 'frame', request: { url: 'https://x.com/asset' }, ...patch } }, 'loader', 7) };
}

const rejectedHeaders=[
  ['syntax','lang'],['syntax','bad name=PRIVATE_VALUE'],['syntax','lang =PRIVATE_VALUE'],
  ['syntax','lang=PRIVATE_VALUE,OTHER'],['syntax','lang=x, ct0=PRIVATE_TOKEN'],
  ['syntax','lang=x; Priority=Low, auth_token=PRIVATE_TOKEN'],
  ['capacity','lang='+ 'x'.repeat(4097)],
  ['capacity','lang=PRIVATE_VALUE;'+Array(33).fill('Secure').join(';')],
];
for(const [index,[reason,line]]of rejectedHeaders.entries())test(`closed header reason ${reason} preserves rejection and privacy: ${index}`,()=>{
  const s=guard();s.request();s.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers:{'Set-Cookie':line}});
  assert.equal(s.g.rejection(),'guard_header_classification');assert.equal(s.g.trace().guardHeaderReason,reason);
  const first=s.g.trace();s.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers:{'Set-Cookie':'ct0=PRIVATE_TOKEN; Path=/'}});
  const summary=s.g.finish();assert.deepEqual(s.g.trace(),first);assert.equal(summary.diagnostic,'candidate_observed');
  assert.equal(Object.hasOwn(summary,'guardHeaderReason'),false);
  for(const raw of ['PRIVATE_NAME','PRIVATE_VALUE','PRIVATE_ATTRIBUTE','PRIVATE_PATH','PRIVATE_TOKEN','foreign.invalid'])assert.equal(JSON.stringify({trace:first,summary}).includes(raw),false);
});

test('metadata shape and capacity reasons never invoke header or blocked-cookie accessors',()=>{
  let reads=0;
  const accessor=Object.defineProperty({},'Set-Cookie',{get(){reads++;throw Error('PRIVATE_HEADER_GETTER');}});
  const blocked=Object.defineProperty({},'cookieLine',{get(){reads++;throw Error('PRIVATE_BLOCKED_GETTER');}});
  for(const [headers,blockedCookies,reason,firstCause]of [
    [{'Set-Cookie':['PRIVATE_VALUE']},undefined,'metadata_shape','guard_metadata_shape'],
    [accessor,undefined,'metadata_shape','guard_metadata_shape'],
    [{'Set-Cookie':'lang=x','set-cookie':'lang=y'},undefined,'metadata_shape','guard_metadata_shape'],
    [{},[blocked],'metadata_shape','guard_metadata_shape'],
    [Object.fromEntries(Array.from({length:129},(_,i)=>['h'+i,'PRIVATE_VALUE'])),undefined,'capacity','guard_header_budget'],
    [{'Set-Cookie':'lang='+ 'x'.repeat(16385)},undefined,'capacity','guard_header_budget'],
    [{'Set-Cookie':Array(33).fill('lang=x').join('\n')},undefined,'capacity','guard_header_budget'],
    [{},Array(33).fill({cookieLine:'lang=x'}),'capacity','guard_header_budget'],
  ]){
    const s=guard();s.request();s.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers,blockedCookies});
    assert.equal(s.g.rejection(),firstCause);assert.equal(s.g.trace().guardHeaderReason,reason);s.g.finish();
  }
  assert.equal(reads,0);
});

test('unfamiliar names, domains, paths and attributes remain other through normal and deferred responses',()=>{
  for(const source of ['response','extra']){
    const s=guard(),url='https://x.com/PRIVATE_PATH/asset';
    s.request('r',{request:{url}});
    const headers={'Set-Cookie':'PRIVATE_NAME=PRIVATE_VALUE; Domain=foreign.invalid; Path=/PRIVATE_PATH; Priority=High; Partitioned; SameSite=PRIVATE_ATTRIBUTE; Max-Age=odd; Expires=unusual'};
    if(source==='extra')s.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers});
    s.emit('Network.responseReceived',{requestId:'r',frameId:'frame',loaderId:'loader',hasExtraInfo:source==='extra',response:{url,status:200,headers:source==='response'?headers:{}}});
    const summary=s.g.finish();assert.equal(summary.diagnostic,'other_only');assert.equal(summary.otherAttempts,1);
    assert.equal(s.g.rejection(),'unknown');assert.equal(s.g.trace().guardHeaderReason,'unknown');
    assert.equal(JSON.stringify({summary,trace:s.g.trace()}).includes('PRIVATE'),false);
  }
});

test('candidate precedence and earlier structural failures cannot acquire a later header reason',()=>{
  for(const [reason,line]of rejectedHeaders){
    const s=guard();s.request();s.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers:{'Set-Cookie':line+'\nct0=PRIVATE_TOKEN; Path=/'}});
    assert.equal(s.g.rejection(),'cookie_candidate',reason);assert.equal(s.g.trace().guardHeaderReason,'unknown');s.g.finish();
  }
  const mixed=guard();mixed.request();mixed.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers:{'Set-Cookie':[]},blockedCookies:[{cookieLine:'auth_token=PRIVATE_TOKEN; Path=/'}]});
  assert.equal(mixed.g.rejection(),'cookie_candidate');assert.equal(mixed.g.trace().guardHeaderReason,'unknown');mixed.g.finish();
  const prior=guard();prior.request();prior.request();const first=prior.g.trace();
  prior.emit('Network.responseReceivedExtraInfo',{requestId:'r',statusCode:200,headers:{'Set-Cookie':'PRIVATE_NAME=PRIVATE_VALUE'}});
  prior.g.finish();assert.deepEqual(prior.g.trace(),first);assert.equal(first.guardHeaderReason,'unknown');
});

test('header reason survives the real binder and failure latch without authorizing or exposing raw data',async()=>{
  for(const [reason,line]of rejectedHeaders.slice(0,14)){
    const s=sample();s.document();s.request();s.response('other',{'Set-Cookie':line});
    const report=await finish(s),latch=startupFailureLatch();let detail;
    assert.throws(()=>requireBoundStartup(report,'reader',latch),error=>{detail=startupDiagnostic(error);return true;});
    assert.equal(detail.trace.firstCause,'guard_header_classification');assert.equal(detail.trace.guardHeaderReason,reason);
    assert.equal(s.published(),0);assert.equal(JSON.stringify(detail).includes('PRIVATE'),false);
  }
});

test('Manual V2 clearly classified other cookies are diagnostic only before and after identity', async () => {
  const lines=[
    'lang=en; Path=/; Secure; SameSite=Lax',
    'PRIVATE_NAME=PRIVATE_VALUE; Domain=foreign.invalid; Path=/private; Priority=High; Partitioned; Expires=Thu, 01 Jan 1970 00:00:00 GMT',
    'lang=x; auth_token=attribute-only; ct0=attribute-only; note=auth_token and ct0',
    'lang=PRIVATE VALUE; Path=/; Path=/; Secure=true; SameSite=invalid; Max-Age=odd; Expires=not-a-date',
  ];
  for (const before of [true, false]) for(const line of lines) {
    const s = sample(); s.document(); if (!before) s.identity();
    s.request(); s.response('other', { 'Set-Cookie': line });
    const r = await finish(s);
    assert.equal(r.diagnostic, 'bound'); assert.equal(s.published(), 1);
    assert.equal(r.manualCookieGuard.diagnostic, 'other_only');
    assert.equal(r.manualCookieGuard.otherAttempts, 1);
    for (const privateValue of ['lang', 'en;', 'https://x.com', 'private-profile','PRIVATE_NAME','PRIVATE_VALUE','foreign.invalid','auth_token','ct0']) assert.equal(JSON.stringify(r.manualCookieGuard).includes(privateValue), false);
  }
});

test('other-cookie effective default paths do not use the former QA path allowlist', async () => {
  const s=sample();s.document();
  s.request('other',{request:{method:'GET',url:'https://x.com/private/asset'}});
  s.response('other',{'Set-Cookie':'PRIVATE_NAME=PRIVATE_VALUE'},
    {response:{url:'https://x.com/private/asset',status:200,headers:{'Set-Cookie':'PRIVATE_NAME=PRIVATE_VALUE'}}});
  const r=await finish(s);
  assert.equal(r.diagnostic,'bound');assert.equal(r.manualCookieGuard.diagnostic,'other_only');assert.equal(r.manualCookieGuard.otherAttempts,1);
  assert.equal(JSON.stringify(r).includes('PRIVATE_NAME'),false);assert.equal(JSON.stringify(r).includes('PRIVATE_VALUE'),false);
});

test('Manual V2 candidate attempts revoke before identity, after identity, mixed and blocked', async () => {
  for (const before of [true, false]) for (const name of ['auth_token', 'ct0']) for (const blocked of [false, true]) {
    const s = sample(); s.document(); if (!before) s.identity(); s.request();
    const candidate = `${name}=PRIVATE; Domain=wrong.invalid; Path=/private; Priority=High; Partitioned; SameSite=odd; Max-Age=not-a-number; Expires=not-a-date`;
    if (blocked) s.extra('other', {}, { blockedCookies: [{ cookieLine: candidate, blockedReasons: ['arbitrary-private-reason'] }] });
    else s.response('other', { 'Set-Cookie': `lang=en; Path=/\n${candidate}` });
    assert.equal(s.binder.identityReady(), false); assert.throws(() => s.binder.startupPins(7));
    const r = await finish(s);
    assert.equal(r.diagnostic, 'observed_cookie_write'); assert.equal(s.published(), 0);
    assert.equal(r.manualCookieGuard.diagnostic, 'candidate_observed'); assert.ok(r.manualCookieGuard.candidateAttempts > 0);
    assert.equal(JSON.stringify(r).includes('PRIVATE'), false);
  }
});

test('candidate empty, deletion and expiration attempts revoke regardless of the resulting jar state', async () => {
  for (const name of ['auth_token', 'ct0']) for (const suffix of ['=; Path=/', '=PRIVATE; Path=/; Max-Age=0', '=; Path=/; Expires=Thu, 01 Jan 1970 00:00:00 GMT']) {
    const s = sample(); s.document(); s.request(); s.response('other', { 'Set-Cookie': name + suffix });
    const r = await finish(s);
    assert.equal(r.diagnostic, 'observed_cookie_write'); assert.equal(s.published(), 0);
    assert.equal(r.manualCookieGuard.diagnostic, 'candidate_observed');
  }
});

test('no-header manual startup reports no observed attempt and preserves unknown continuity', async () => {
  const s = sample(); s.document(); const r = await finish(s);
  assert.equal(r.diagnostic, 'bound'); assert.equal(s.published(), 1);
  assert.equal(r.manualCookieGuard.diagnostic, 'not_observed'); assert.equal(r.continuity, 'unknown');
  assert.equal(r.canAuthorize, false); assert.equal(r.sessionTransferred, false);
});

test('an other-cookie diagnostic never bypasses login, cleanup, freshness or owner binding', async () => {
  for (const fault of ['login', 'cleanup', 'stale', 'owner']) {
    const s = sample(); s.document(); s.identity(); s.request(); s.response('other', { 'Set-Cookie': 'lang=en; Path=/' });
    if (fault === 'owner') s.emit('Page.loadEventFired', {}, { ...s.scope, owner: 2 });
    if (fault === 'stale') s.time(6000);
    s.settings(fault === 'login' ? 401 : 200);
    const r = await s.binder.finish(fault !== 'cleanup');
    assert.notEqual(r.diagnostic, 'bound'); assert.equal(s.published(), 0);
    assert.equal(r.manualCookieGuard.diagnostic, 'other_only');
  }
});

test('malformed or ambiguous names and folded cookie pairs reject without raw output', async () => {
  const headers = [
    { 'Set-Cookie': 'lang' }, { 'Set-Cookie': 'bad name=x' },
    { 'Set-Cookie': 'lang=x, ct0=b' }, { 'Set-Cookie': 'lang=x, auth_token=y' },
    { 'Set-Cookie': 'lang=x; Priority=Low, ct0=y' },
    { 'Set-Cookie': ['lang=x'] }, { 'Set-Cookie': '' }, { 'Set-Cookie': 'lang=x\rct0=y' },
    { 'Set-Cookie': 'lang=x; Path=/', 'set-cookie': 'lang=y; Path=/' },
    { 'Set-Cookie': 'lang=' + 'x'.repeat(16385) },
  ];
  let getterReads = 0;
  headers.push(Object.defineProperty({}, 'Set-Cookie', { enumerable: true, get() { getterReads++; return 'private'; } }));
  for (const h of headers) {
    const s = sample(); s.document(); s.request(); s.response('other', h);
    const r = await finish(s);
    assert.equal(r.diagnostic, 'observed_cookie_write'); assert.equal(s.published(), 0);
    assert.equal(r.manualCookieGuard.diagnostic, 'ambiguous');
  }
  assert.equal(getterReads, 0);
});

test('identity readiness remains independent of header provenance and rejected request scope never admits a receipt', async () => {
  for (const fault of ['loader', 'context', 'frame', 'request-loader', 'foreign-url', 'reused-id']) {
    const s = sample();
    if (fault !== 'loader') {
      s.emit('Page.frameNavigated', { frame: { id: 'frame', loaderId: 'loader', url: 'https://x.com/home' } });
      if (fault !== 'context') s.emit('Runtime.executionContextCreated', { context: { id: 7, origin: 'https://x.com', auxData: { isDefault: true, frameId: 'frame' } } });
    }
    s.request('other', fault === 'frame' ? { frameId: 'child' } : fault === 'request-loader' ? { loaderId: 'old' } : fault === 'foreign-url' ? { request: { url: 'https://foreign.invalid/asset' } } : {});
    if (fault === 'reused-id') s.request();
    s.response('other', { 'Set-Cookie': 'lang=en; Path=/' });
    const r = await s.binder.finish(true);
    assert.equal(r.diagnostic, ['loader','context'].includes(fault) ? 'identity_unavailable' : 'observed_cookie_write'); assert.equal(s.published(), 0);
  }
});

test('ExtraInfo before response requires a known request and final exact response, never late rebinding', async () => {
  const good = sample(); good.document(); good.request(); good.extra('other', { 'Set-Cookie': 'lang=en; Path=/' });
  good.response('other', {}, { hasExtraInfo: true });
  assert.equal((await finish(good)).diagnostic, 'bound');
  for (const fault of ['before-request', 'missing-response', 'duplicate-extra', 'status', 'extra-false', 'response-url', 'response-frame']) {
    const s = sample(); s.document();
    if (fault !== 'before-request') s.request();
    if (fault === 'response-url') s.response('other', {}, { response: { url: 'https://x.com/different', status: 200 }, hasExtraInfo: true });
    if (fault === 'response-frame') s.response('other', {}, { frameId: 'child', hasExtraInfo: true });
    s.extra('other', { 'Set-Cookie': 'lang=en; Path=/' }, fault === 'status' ? { statusCode: 201 } : {});
    if (fault === 'before-request') s.request();
    if (fault === 'duplicate-extra') s.extra();
    if (!['missing-response', 'response-url', 'response-frame'].includes(fault)) s.response('other', {}, { hasExtraInfo: fault !== 'extra-false' });
    const r = await finish(s);
    assert.equal(r.diagnostic, 'observed_cookie_write', fault); assert.equal(s.published(), 0);
  }
});

test('response cookie waits for declared ExtraInfo and later metadata contradictions stay rejected', async () => {
  for (const fault of ['missing-extra', 'duplicate-response', 'late-status', 'extra-after-false']) {
    const s = sample(); s.document(); s.request();
    s.response('other', { 'Set-Cookie': 'lang=en; Path=/' }, { hasExtraInfo: fault !== 'extra-after-false' });
    if (fault === 'duplicate-response') s.response('other', {}, { hasExtraInfo: true });
    if (fault !== 'missing-extra') s.extra('other', {}, fault === 'late-status' ? { statusCode: 201 } : {});
    assert.equal((await finish(s)).diagnostic, 'observed_cookie_write');
  }
});

test('redirect response and blocked-cookie attempts cannot bypass candidate or ambiguity revocation', async () => {
  for (const name of ['auth_token', 'ct0', 'lang']) {
    const s = sample(); s.document();
    s.request('redirect', { redirectResponse: { headers: { 'Set-Cookie': `${name}=PRIVATE; Path=/` } } });
    const r = await finish(s); assert.equal(r.diagnostic, 'observed_cookie_write');
    assert.equal(r.manualCookieGuard.diagnostic, name === 'lang' ? 'ambiguous' : 'candidate_observed');
  }
  for (const blocked of [[{}], {}, new Array(33).fill({ cookieLine: 'lang=x' })]) {
    const s = sample(); s.document(); s.request(); s.extra('other', {}, { blockedCookies: blocked });
    assert.equal((await finish(s)).diagnostic, 'observed_cookie_write');
  }
});

test('bounded request/header/line metadata and accessor rejection stay fail closed', () => {
  for (const kind of ['requests', 'headers', 'lines', 'events', 'getter']) {
    const s = guard(); s.request();
    if (kind === 'requests') for (let i = 0; i < 1281; i++) s.request('r' + i);
    else {
      let headers = { 'Set-Cookie': 'lang=x; Path=/' };
      if (kind === 'headers') headers = Object.fromEntries(Array.from({ length: 129 }, (_, i) => ['h' + i, 'v']));
      if (kind === 'lines') headers['Set-Cookie'] = new Array(33).fill('lang=x; Path=/').join('\n');
      if (kind === 'getter') headers = Object.defineProperty({}, 'Set-Cookie', { get() { assert.fail('getter invoked'); } });
      for (let i = 0; i < (kind === 'events' ? 65 : 1); i++) s.emit('Network.responseReceivedExtraInfo', { requestId: 'r', statusCode: 200, headers });
    }
    const r = s.g.finish(); assert.equal(r.diagnostic, 'ambiguous'); assert.ok(r.ambiguousAttempts <= 64);
    assert.throws(() => s.g.finish());
  }
});

test('default strict and deferred QA policies are unchanged and cannot select Manual V2 using JSON options', async () => {
  const strict = sample((...args) => new QABoundObservation(...args)); strict.document(); strict.identity(); strict.request(); strict.response('other', { 'Set-Cookie': 'lang=en; Path=/' });
  assert.equal((await finish(strict)).diagnostic, 'observed_cookie_write');
  const deferred = sample((...args) => new QABoundObservation(...args), { deferCookieMutation: true, onPrivateResult: () => {}, onPrivateStartup: () => assert.fail('deferred receipt') });
  deferred.document(); deferred.identity(); deferred.request(); deferred.response('other', { 'Set-Cookie': 'auth_token=PRIVATE; Path=/' });
  const r = await finish(deferred); assert.equal(r.diagnostic, 'observed_cookie_write'); assert.equal(r.login, 'protected_response_observed');
  assert.equal(Object.hasOwn(r, 'manualCookieGuard'), false);
  assert.throws(() => sample(createManualV2BoundObservation, { deferCookieMutation: true, onPrivateResult: () => {} }));
  assert.throws(() => sample((...args) => new QABoundObservation(...args), { deferCookieMutation: false, onPrivateResult: () => {}, manualCookiePolicy: true }));
  // The trusted factory also works with omitted optional arguments.
  assert.ok(createManualV2BoundObservation({ owner: 1, profileId: 'p', sessionId: 's', targetId: 't' }, 'f', async () => {}));
});

test('both private owner roles preserve cookie rejection attribution and cannot publish a startup wire', async () => {
  for (const role of ['reader', 'collector']) {
    const diagnostics = startupFailureLatch();
    const owner = createManualStartupOwner({ role, diagnostics, run: async sink => {
      const s = sample(createManualV2BoundObservation, { deferCookieMutation: false, onPrivateResult: () => {}, onPrivateStartup: sink.onPrivateStartup });
      s.document(); await sink.beforeIdentityRead(s.binder.startupPins(7));
      s.request(); s.response('other', { 'Set-Cookie': 'ct0=PRIVATE; Path=/' });
      const r = await finish(s); requireBoundStartup(r, role, diagnostics);
    } });
    const ready = await owner.prepare();
    await assert.rejects(owner.acquire(ready.handle, { challenge: 'b'.repeat(48), role, ...ready.pins }), error => {
      const d = startupDiagnostic(error); return d?.role === role && d.reason === 'observed_cookie_write';
    });
    await owner.abort(ready.handle);
  }
});
