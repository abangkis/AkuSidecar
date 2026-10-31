import test from 'node:test';
import assert from 'node:assert/strict';
import {resolve} from 'node:path';
import {runInNewContext} from 'node:vm';
import {fileURLToPath} from 'node:url';
import {sourceAssets} from '../worker.mjs';
import {captureIsolated} from '../capture.mjs';
import {createInstagramVideoRecovery} from '../additional-source-media.mjs';
import {resolveInstagramStructuredMediaInMainWorld} from '../../../../../../AkuBridge/instagram-main-world-media-resolver.js';
import {resolveInstagramStructuredFeedInMainWorld} from '../../../../../../AkuBridge/instagram-main-world-feed-resolver.js';
import {createInstagramDailyOwner, INSTAGRAM_NATIVE_RECIPE} from './instagram-native-provider.mjs';
import {createNativeSourceAttempt, createNativeOwnedPage, nativeSourceDiagnostic, nativeSourceFailure} from './native-source-kernel.mjs';
import {projectInstagramCookies, instagramAccountExpression, verifyInstagramPair} from './instagram-native-session.mjs';
import {fixture as bootstrapFixture} from './instagram-bootstrap-fixture.mjs';

const nonce = 'a'.repeat(48), profileScope = 'reader-profile-scope';
const params = {chromePath: resolve('offline/chrome.exe'), readerProfile: resolve('offline/reader'), readerPID: 100,
  profileDirectory: 'Default', acknowledgeLimitations: true, allowProcessSessionDerivation: true};
const cookies = () => ['csrftoken', 'ds_user_id', 'sessionid'].map(name => ({name, value: name === 'ds_user_id' ? '123456789' : name + '-private-secret',
  domain: '.instagram.com', path: '/', secure: true, httpOnly: name === 'sessionid', expires: -1, sameSite: 'Lax'}));
const binding = {generation: 1, nonce, profileScope};

function fixture(options = {}) {
  const calls = [], targets = new Set(), pins = new Set(), sessions = new Set();
  let next = 0, seedCount = 0, collectorClosed = false;
  const readerCookies = cookies();
  const bootstrapOptions={bodyID:options.accountId??'123456789',domID:options.accountId??'123456789',cookieID:'123456789',login:options.loggedOut,changeContext:options.changedDocument};
  const readerBootstrap=bootstrapFixture(bootstrapOptions);
  const projection = expression => {
    assert.ok(!expression.includes('current_user'),'production must not request the legacy endpoint');
    return true;
  };
  const transport = {
    subscribe:listener=>readerBootstrap.raw.subscribe(event=>listener({...event,sessionId:'session-'+next})),
    alive: () => true,
    reserveTarget: id => pins.add(id), releaseTarget: id => pins.delete(id),
    reserveSession: id => sessions.add(id), releaseSession: id => sessions.delete(id),
    async call(method, values, session) {
      calls.push({side: 'reader', method});
      if (method === 'Target.createTarget') { const targetId = 'target-' + ++next; targets.add(targetId); return {targetId}; }
      if (method === 'Target.attachToTarget') return {sessionId: 'session-' + next};
      if (method === 'Target.closeTarget') { if (!options.keepTarget) targets.delete(values.targetId); return {success: true}; }
      if (method === 'Target.getTargets') return {targetInfos: [...targets].map(targetId => ({targetId}))};
      if (method === 'Network.setCacheDisabled' && options.failCache) throw new Error('PRIVATE response body must not escape');
      if (method === 'Network.getCookies') return {cookies: structuredClone(readerCookies)};
      return readerBootstrap.raw.call(method,values,'owned-session',1000);
    },
  };
  const pages = [];
  const createCollector = async configuration => {
    calls.push({side: 'collector', method: 'launch'});
    let jar = [];
    return {pid: 200, profilePath: configuration.profilePath,
      async createTemporaryPage() {
        const bootstrap=bootstrapFixture(bootstrapOptions);
        const page = {closed: false, url: 'about:blank',
          on:listener=>bootstrap.raw.subscribe(listener),
          async send(method, values) {
            calls.push({side: 'collector', method});
            if (method === 'Network.setCookies') { seedCount++; jar = structuredClone(values.cookies).map(cookie => ({...cookie, expires: cookie.expires ?? -1})); return {}; }
            if (method === 'Network.getCookies') {
              const selected = structuredClone(jar);
              if (options.collectorMismatch) selected.find(cookie => cookie.name === 'ds_user_id').value = '222';
              return {cookies: selected};
            }
            return bootstrap.raw.call(method,values,'owned-session',1000);
          },
          async navigate(url) { page.url = url; return {}; },
          async evaluate(expression) {
            if (options.mediaFixture && expression.includes('function resolveInstagramStructured')) {
              const native = {code: 'Fixture', user: {username: 'fixture.author'}, caption: {text: 'Offline media'}, media_type: 2,
                image_versions2: {candidates: [{url: 'https://s.cdninstagram.com/poster.jpg'}]}, video_versions: [{url: 'https://s.cdninstagram.com/playback.mp4'}]};
              return runInNewContext(expression, {document: {querySelectorAll: () => [{textContent: JSON.stringify({require: [{data: [native]}]})}]}, URL, TextEncoder});
            }
            if (options.mediaFixture && expression === '({url:location.href,ready:document.readyState})') return {url: page.url, ready: 'complete'};
            if (options.mediaFixture && expression === 'location.href') return page.url;
            if (options.mediaFixture && !expression.includes('current_user') && !expression.startsWith("location.origin==='https://www.instagram.com'")) return undefined;
            if (options.fullPipeline && !expression.includes('current_user')) {
              if (expression === 'location.href') return page.url;
              if (expression === '({url:location.href,ready:document.readyState})') return {url: page.url, ready: 'complete'};
              if (expression.includes('prepareEvidenceTargets')) return [];
              if (expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({posts: [{id: 'instagram:p:Fixture', permalink: 'https://www.instagram.com/p/Fixture/',
                author: 'fixture.author', text: 'Offline native transport IG post', media: []}], documentReady: true, authenticatedUiObserved: true,
                scroll: {y: 0, height: 1000, viewportHeight: 900}});
              if (expression.startsWith("location.origin==='https://www.instagram.com'")) return true;
              return undefined;
            }
            return projection(expression);
          },
          async close() { page.closed = true; if (options.pageCloseFailure) throw new Error('PRIVATE close error'); },
        };
        pages.push(page); return page;
      },
      async close() { collectorClosed = true; },
    };
  };
  const capture = options.runCapture ?? (async (browser, assets, source) => {
    assert.equal(source, 'instagram'); assert.ok(assets.instagram);
    await browser.forSource(source);
    const media = await browser.createTemporaryPage(); await media.navigate('https://www.instagram.com/p/ABC123/'); await media.close();
    return {source: 'instagram', posts: [], coverage: {}};
  });
  return {transport, createCollector, capture, calls, targets, pins, sessions, pages, readerCookies,
    get seedCount() { return seedCount; }, get collectorClosed() { return collectorClosed; }};
}
async function start(options) {
  const f = fixture(options);
  f.owner = await createInstagramDailyOwner(params, f.transport, {createCollector: f.createCollector,
    loadAssets: options?.loadAssets ?? (async () => ['sealed-ig-asset']), runCapture: f.capture});
  await f.owner.execute('source_initialize', {});
  f.announcement = await f.owner.execute('source_begin', {...binding, collectorProfile: resolve('offline/collector'), deadlineMS: Date.now() + 60000});
  return f;
}

test('native recipe stays unqualified and has no speculative optional cookie import', () => {
  assert.equal(INSTAGRAM_NATIVE_RECIPE.nativeQualified, false);
  assert.deepEqual(INSTAGRAM_NATIVE_RECIPE.optionalCookies, []);
  assert.throws(() => { INSTAGRAM_NATIVE_RECIPE.nativeQualified = true; });
});

test('post-capture verification navigates the separate evidence page and preserves capture document',async()=>{
 const url='https://www.instagram.com/p/ABC123/';
 const f=await start({runCapture:async browser=>{
  await (await browser.forSource('instagram')).navigate(url);
  return {source:'instagram',posts:[],coverage:{}};
 }});
 await f.owner.execute('source_seed',binding);
 await f.owner.execute('source_capture',{...binding,payload:{scrolls:0}});
 assert.equal(f.pages[0].url,url);assert.equal(f.pages[0].closed,false);
 assert.equal(f.pages[1].closed,false);
 await f.owner.close();assert.ok(f.pages.every(page=>page.closed));
});

test('IG retries use a fresh collector and proof after verified cleanup, then stop at the bound',async()=>{
  const f=await start({runCapture:async()=>{throw nativeSourceFailure('capture','deadline');}});
  for(let generation=1;generation<=3;generation++){
    const q={...binding,generation,nonce:generation===1?nonce:generation.toString().repeat(48)};
    if(generation>1)await f.owner.execute('source_begin',{...q,collectorProfile:resolve('offline/collector-'+generation),deadlineMS:Date.now()+60000});
    await f.owner.execute('source_seed',q);
    await assert.rejects(f.owner.execute('source_capture',{...q,payload:{scrolls:0}}));
    assert.deepEqual(await f.owner.execute('source_end',q),{closed:true});
  }
  assert.equal(f.seedCount,3);assert.equal(f.targets.size,0);
  await assert.rejects(f.owner.execute('source_begin',{...binding,generation:4,collectorProfile:resolve('offline/collector-4'),deadlineMS:Date.now()+60000}));
  await f.owner.close();
});

test('two-phase native provider reads no session until seed; fresh bound proof and injected capture seam', async () => {
  const f = await start();
  assert.equal(f.seedCount, 0);
  assert.ok(!f.calls.some(call => call.method === 'Network.getCookies'));
  assert.equal(f.announcement.collectorPID, 200);
  const pair = await f.owner.execute('source_seed', binding);
  assert.equal(f.seedCount, 1);
  assert.equal(pair.reader.ownerID, '100'); assert.equal(pair.collector.ownerID, '200');
  assert.equal(pair.reader.sessionRevision, pair.collector.sessionRevision);
  assert.equal(pair.reader.accountKey, pair.collector.accountKey);
  assert.equal(pair.reader.domain, 'native-browser');
  assert.equal(pair.reader.nonce, nonce);
  assert.equal(pair.reader.profileScope, profileScope);
  assert.ok(!JSON.stringify(pair).includes('123456789'));
  assert.ok(!JSON.stringify(pair).includes('private-secret'));
  const challenge = await f.owner.execute('source_proof', {...binding, nonce: 'b'.repeat(48)});
  assert.equal(challenge.reader.nonce, 'b'.repeat(48));
  const result = await f.owner.execute('source_capture', {...binding, payload: {scrolls: 0}});
  assert.equal(result.source, 'instagram');
  assert.deepEqual(await f.owner.execute('source_end', binding), {closed: true});
  assert.equal(f.collectorClosed, true); assert.equal(f.targets.size, 0);
  assert.equal(f.pins.size, 0); assert.equal(f.sessions.size, 0);
  assert.ok(f.pages.every(page => page.closed)); await f.owner.close();
});

test('native provider uses real IG sourceAssets and captureIsolated over offline CDP fixtures', async () => {
  const f = await start({fullPipeline: true, runCapture: captureIsolated,
    loadAssets: source => sourceAssets(fileURLToPath(new URL('../../../../../../AkuBridge/', import.meta.url)), source)});
  await f.owner.execute('source_seed', binding);
  const result = await f.owner.execute('source_capture', {...binding, payload: {scrolls: 0, sourceHydrationTimeoutMs: 1000, captureTimeoutMs: 5000}});
  assert.equal(result.source, 'instagram');
  assert.equal(result.snapshots[0].blocks[0].text, 'Offline native transport IG post');
  assert.equal(f.pages[0].url, INSTAGRAM_NATIVE_RECIPE.home);
  assert.deepEqual(await f.owner.execute('source_end', binding), {closed: true});
  assert.ok(f.pages.every(page => page.closed)); assert.equal(f.targets.size, 0); await f.owner.close();
});

test('native provider reuses IG structured video recovery and closes its owned media page', async () => {
  let recovered;
  const f = await start({mediaFixture: true, runCapture: async browser => {
    const recovery = createInstagramVideoRecovery({browser, deadlineAt: Date.now() + 5000,
      resolver: {available: true, functionSource: resolveInstagramStructuredMediaInMainWorld.toString(), runtimeRevision: 'instagram-main-world-media-resolver-v2'},
      feedResolver: {available: true, functionSource: resolveInstagramStructuredFeedInMainWorld.toString(), runtimeRevision: 'instagram-main-world-feed-resolver-v1'},
      inspectSnapshot: async () => ({posts: [], documentReady: true}),
    });
    recovered = await recovery.recoverSnapshot({posts: [{id: 'instagram:p:Fixture', permalink: 'https://www.instagram.com/p/Fixture/', author: 'fixture.author',
      text: '', contentKind: 'video', media: [{kind: 'video_poster', url: 'https://s.cdninstagram.com/poster.jpg'}], mediaExpected: ['video']}]});
    await recovery.close(); return {source: 'instagram', snapshots: [], coverage: {}};
  }});
  await f.owner.execute('source_seed', binding);
  await f.owner.execute('source_capture', {...binding, payload: {}});
  assert.equal(recovered.posts[0].mediaEvidence.nativeVideoFallback.outcome, 'owned_playback_url_observed');
  assert.equal(f.pages.length, 3); assert.equal(f.pages[2].closed, true);
  assert.deepEqual(await f.owner.execute('source_end', binding), {closed: true});
  assert.ok(f.pages.every(page => page.closed)); await f.owner.close();
});

test('ordinary cookies are omitted, required authentication material is mandatory', () => {
  const selected = projectInstagramCookies({cookies: [...cookies(), {name: 'theme', domain: '.instagram.com', value: 'dark'}]}, Date.now());
  assert.equal(selected.length, 3);
  assert.throws(() => projectInstagramCookies({cookies: cookies().filter(cookie => cookie.name !== 'sessionid')}, Date.now()));
});

for (const [name, change] of [
  ['cross-source', cookie => { cookie.domain = '.x.com'; }],
  ['subdomain', cookie => { cookie.domain = '.evil.instagram.com'; }],
  ['path', cookie => { cookie.path = '/api'; }],
  ['insecure', cookie => { cookie.secure = false; }],
  ['partitioned', cookie => { cookie.partitionKey = {topLevelSite: 'https://evil.test'}; }],
  ['expired', cookie => { cookie.expires = 1; }],
]) test(`session manifest rejects ${name} cookies`, () => {
  const selected = cookies(); change(selected[0]); assert.throws(() => projectInstagramCookies({cookies: selected}, Date.now()));
});

test('session manifest rejects duplicate auth names', () => {
  assert.throws(() => projectInstagramCookies({cookies: [...cookies(), cookies()[0]]}, Date.now()));
});

for (const [name, options] of [['logged out', {loggedOut: true}], ['bootstrap/cookie account mismatch', {accountId: '999'}],
  ['collector cookie mismatch', {collectorMismatch: true}], ['root document replacement', {changedDocument: true}], ['cache control rejection', {failCache: true}]]) {
  test(`native evidence rejects ${name} and keeps diagnostics private`, async () => {
    const f = await start(options);
    await assert.rejects(f.owner.execute('source_seed', binding), error => {
      assert.equal(error.message, 'native_source_failed');
      const detail = nativeSourceDiagnostic(error); assert.ok(detail); assert.equal(detail.cleanup, 'verified');
      assert.ok(!JSON.stringify(detail).includes('PRIVATE')); assert.ok(!JSON.stringify(detail).includes('123456789')); return true;
    });
    assert.equal(f.collectorClosed, true); assert.equal(f.targets.size, 0); await f.owner.close();
  });
}

test('session drift after seed is rejected before capture', async () => {
  const f = await start(); await f.owner.execute('source_seed', binding);
  f.readerCookies.find(cookie => cookie.name === 'sessionid').value = 'rotated';
  await assert.rejects(f.owner.execute('source_capture', {...binding, payload: {}}));
  assert.equal(f.collectorClosed, true); await f.owner.close();
});

test('cross-source and stale generation cannot consume seed authority', async () => {
  const f = await start(); await assert.rejects(f.owner.execute('source_seed', {...binding, source: 'x'}));
  assert.equal(f.seedCount, 0); await f.owner.close();
  const g = await start(); await assert.rejects(g.owner.execute('source_seed', {...binding, generation: 2}));
  assert.equal(g.seedCount, 0); await g.owner.close();
});

test('collector page cleanup failure remains independent of the original capture cause', async () => {
  const f = await start({pageCloseFailure: true, runCapture: async () => { throw new Error('PRIVATE capture failed'); }});
  await f.owner.execute('source_seed', binding);
  await assert.rejects(f.owner.execute('source_capture', {...binding, payload: {}}), error => {
    assert.deepEqual(nativeSourceDiagnostic(error), {stage: 'capture', reason: 'unavailable', cleanup: 'uncertain'}); return true;
  });
  assert.equal(f.collectorClosed, true); await assert.rejects(f.owner.close());
});

test('end uncertainty stays latched across owner close and repeated close', async () => {
  const f = await start({pageCloseFailure: true}); await f.owner.execute('source_seed', binding);
  assert.deepEqual(await f.owner.execute('source_end', binding), {closed: false});
  await assert.rejects(f.owner.execute('source_close', {}));
  await assert.rejects(f.owner.close());
  assert.equal(f.collectorClosed, true);
});

test('exact target pin is retained when absence cannot be verified', async () => {
  const f = fixture(); const page = await createNativeOwnedPage(f.transport, 1000);
  const original = f.transport.call;
  f.transport.call = async (method, ...args) => { if (method === 'Target.getTargets') throw new Error('PRIVATE inventory'); return original(method, ...args); };
  await assert.rejects(page.close()); assert.equal(f.pins.size, 1); assert.equal(f.sessions.size, 1);
});

test('native attempt rejects oversized deadlines, expiry and clock rollback', async () => {
  assert.throws(() => createNativeSourceAttempt({source: 'instagram', ...binding, now: () => 100, deadlineMS: 60101}));
  let time = 100;
  const a = createNativeSourceAttempt({source: 'instagram', ...binding, now: () => time, deadlineMS: 200});
  time = 99; assert.throws(() => a.remaining()); await a.close();
  const b = createNativeSourceAttempt({source: 'instagram', ...binding, now: () => time, deadlineMS: 200});
  time = 200; assert.throws(() => b.remaining()); await b.close();
});

test('late-created resources retain their cleanup obligation and cannot publish', async () => {
  let complete, released = false;
  const attempt = createNativeSourceAttempt({source: 'instagram', ...binding, deadlineMS: Date.now() + 60000, cleanupMS: 5});
  const pending = attempt.own('target', () => new Promise(resolve => { complete = resolve; }), async () => { released = true; });
  await new Promise(resolve => setImmediate(resolve));
  const rejected = assert.rejects(pending);
  const receipt = await attempt.close(); assert.equal(receipt.closed, false);
  complete({}); await rejected; await new Promise(resolve => setImmediate(resolve));
  assert.equal(released, true); assert.equal(attempt.status().cleanup, 'uncertain');
});

test('partial target attachment failure closes only its owned target and releases its pin', async () => {
  const f = fixture(), call = f.transport.call;
  f.targets.add('user-reader');
  f.transport.call = async (method, ...args) => { if (method === 'Target.attachToTarget') throw new Error('PRIVATE'); return call(method, ...args); };
  await assert.rejects(createNativeOwnedPage(f.transport, 1000));
  assert.deepEqual([...f.targets], ['user-reader']); assert.equal(f.pins.size, 0);
});

test('bounded account expression streams only one fixed endpoint, no raw body projection', async () => {
  let request;
  const body = JSON.stringify({status: 'ok', user: {pk: '123456789', username: 'PRIVATE username', email: 'PRIVATE email'}});
  const sandbox = {location: {origin: 'https://www.instagram.com', pathname: '/'}, document: {querySelector: () => null},
    setTimeout, clearTimeout, AbortController, TextDecoder, Uint8Array, URL,
    fetch: async (url, options) => { request = {url, options}; return {
      status: 200, url: 'https://www.instagram.com/api/v1/accounts/current_user/?edit=true',
      headers: new Headers({'content-type': 'application/json'}), body: new Response(body).body,
    }; },
  };
  const result = await runInNewContext(instagramAccountExpression(nonce), sandbox);
  assert.equal(request.url, '/api/v1/accounts/current_user/?edit=true'); assert.equal(request.options.redirect, 'error');
  assert.equal(result.authenticated, true); assert.deepEqual(Object.keys(result).sort(), ['accountId', 'authenticated', 'nonce', 'rejection']);
  assert.ok(!JSON.stringify(result).includes('PRIVATE'));
  sandbox.fetch = async () => ({status: 200, url: 'https://www.instagram.com/api/v1/accounts/current_user/?edit=true', headers: new Headers({'content-type': 'application/json'}), body: new Response('x'.repeat(65537)).body});
  assert.equal((await runInNewContext(instagramAccountExpression(nonce), sandbox)).authenticated, false);
});

test('proof verifier rejects stale and mismatched role/nonce/source evidence', () => {
  const base = {source: 'instagram', domain: 'native-browser', recipe: 'instagram-native-v1', ...binding,
    accountKey: 'a'.repeat(64), sessionRevision: 'b'.repeat(64), authenticated: true, observedAtMS: 10000};
  const reader = {...base, role: 'reader', ownerID: '100', profileID: 'reader'}, collector = {...base, role: 'collector', ownerID: '200', profileID: 'collector'};
  assert.ok(verifyInstagramPair(reader, collector, binding, 10000));
  for (const change of [{source: 'x'}, {role: 'reader'}, {nonce: 'b'.repeat(48)}, {generation: 2}, {observedAtMS: 1}]) {
    assert.throws(() => verifyInstagramPair(reader, {...collector, ...change}, binding, 10000));
  }
});
