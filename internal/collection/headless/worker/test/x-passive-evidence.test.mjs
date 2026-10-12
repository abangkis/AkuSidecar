import test from 'node:test';
import assert from 'node:assert/strict';
import vm from 'node:vm';
import {readFile} from 'node:fs/promises';
import {prepareXPassiveEvidence, resolveXPassiveEvidence} from '../x-passive-evidence.mjs';
import {capture} from '../capture.mjs';

const bridge = new URL('../../../../../../AkuBridge/', import.meta.url);
const names = ['x-response-evidence-adapter.js', 'x-media-evidence-runtime.js', 'capture-primitives.js',
  'source-adapter-runtime.js', 'adapters/x-adapter.js'];
const templates = await Promise.all(names.map(async name => ({relative: 'AkuBridge/' + name,
  execute: !name.startsWith('x-'), content: await readFile(new URL(name, bridge), 'utf8')})));
for (const name of ['x-quote-identity.js', 'x-extract.js']) templates.push({relative: 'worker/vendor/' + name,
  execute: true, content: await readFile(new URL('../vendor/' + name, import.meta.url), 'utf8')});
const ownID = '2106867448746021278', parentID = '2106867448746021279';
const native = `https://x.com/owner/status/${ownID}`;
const poster = 'https://pbs.twimg.com/ext_tw_video_thumb/12345/pu/img/poster.jpg';
const playback = 'https://video.twimg.com/ext_tw_video/12345/pu/vid/720x720/video.mp4';
const avatar = 'https://pbs.twimg.com/profile_images/12345/owner_normal.jpg';
const envelope = candidates => ({type: 'AKU_X_RESPONSE_MEDIA_EVIDENCE', runtimeRevision: 'x-response-evidence-v2', candidates});
const post = (extra = {}) => ({id: ownID, permalink: native, author: 'Owner', text: 'DOM caption',
  avatarEvidenceKeys: [`x:status:${ownID}`, 'x:user:owner'], mediaExpected: ['video'], media: [{kind: 'video_poster', url: poster}], ...extra});
const candidate = (extra = {}) => ({candidateId: `x:status:${ownID}`, media: [{kind: 'video', url: poster,
  posterUrl: poster, playbackUrl: playback}], avatarUrl: avatar, ...extra});
const assets = () => templates.map(value => ({...value}));

async function registered(extra = {}) {
  const value = assets();
  const state = {sends: 0, reads: 0, result: envelope([candidate()])};
  const page = {send: async () => {state.sends++; return {identifier: 'observer'};},
    evaluate: async () => {state.reads++; return state.result;}, ...extra};
  const browser = {backend: 'headless_worker'};
  assert.equal(await prepareXPassiveEvidence({browser, page, assets: value, deadlineAt: Date.now() + 1000}), 'registered');
  return {browser, page, value, state};
}

test('observer registration is once per owned target and never modifies borrowed pages', async () => {
  const {browser, page, value, state} = await registered();
  assert.equal(await prepareXPassiveEvidence({browser, page, assets: value, deadlineAt: Date.now() + 1000}), 'registered');
  assert.equal(state.sends, 1);
  assert.equal(await prepareXPassiveEvidence({browser: {backend: 'browser_quiet_hidden'}, page,
    assets: value, deadlineAt: Date.now() + 1000}), 'not_applicable');
  assert.equal(state.sends, 1);
});

test('hanging registration is bounded, failure-soft and cannot create duplicate late hooks', async () => {
  let sends = 0, finish;
  const page = {send: () => {sends++; return new Promise(resolve => {finish = resolve;});}};
  const parameters = {browser: {backend: 'headless_worker'}, page, assets: assets(), deadlineAt: Date.now() + 25};
  assert.equal(await prepareXPassiveEvidence(parameters), 'unavailable');
  assert.equal(await prepareXPassiveEvidence({...parameters, deadlineAt: Date.now() + 500}), 'unavailable');
  assert.equal(sends, 1);
  finish({identifier: 'late-observer'}); await Promise.resolve(); await Promise.resolve();
  assert.equal(await prepareXPassiveEvidence({...parameters, deadlineAt: Date.now() + 500}), 'registered');
});

test('passive video uses existing exact-owner, DOM-poster and MP4 admission', async () => {
  const {browser, page, value, state} = await registered();
  const input = post();
  const result = await resolveXPassiveEvidence({browser, page, assets: value, posts: [input], deadlineAt: Date.now() + 1000});
  assert.equal(result[0].media[0].playbackUrl, playback);
  assert.equal(result[0].avatar, avatar);
  assert.equal(result[0].media[0].provenance, 'x_response_graphql');
  assert.equal(result[0].mediaEvidence.passiveResponse.playbackUrlRecovered, true);
  assert.equal(input.media[0].kind, 'video_poster');
  state.result = envelope([]);
  const cached = await resolveXPassiveEvidence({browser, page, assets: value, posts: [post()], deadlineAt: Date.now() + 1000});
  assert.equal(cached[0].media[0].playbackUrl, playback);
  assert.equal(cached[0].mediaEvidence.passiveResponse.evidenceSource, 'worker_session_cache');
  const another = post({id: '2106867448746021280', permalink: 'https://x.com/owner/status/2106867448746021280', media: []});
  const next = await resolveXPassiveEvidence({browser, page, assets: value, posts: [another], deadlineAt: Date.now() + 1000});
  assert.equal(next[0].avatar, avatar, 'shared exact author key permits session avatar fallback');
});

test('foreign, duplicate, mismatched, unsafe and ambiguous passive evidence cannot upgrade media', async () => {
  for (const values of [
    [candidate({candidateId: `x:status:${parentID}`})],
    [candidate(), candidate()],
    [candidate({media: [{kind: 'video', posterUrl: 'https://pbs.twimg.com/ext_tw_video_thumb/999/pu/img/foreign.jpg', playbackUrl: playback}]})],
    [candidate({media: [{kind: 'video', posterUrl: poster, playbackUrl: 'https://video.twimg.com.evil.example/clip.mp4'}]})],
    [candidate({media: [{kind: 'video', posterUrl: poster, playbackUrl: 'https://video.twimg.com/ext_tw_video/12345/stream.m3u8'}]})],
  ]) {
    const {browser, page, value, state} = await registered(); state.result = envelope(values);
    const result = await resolveXPassiveEvidence({browser, page, assets: value, posts: [post()], deadlineAt: Date.now() + 1000});
    assert.equal(result[0].media[0].kind, 'video_poster');
    assert.equal(result[0].media[0].playbackUrl, undefined);
  }
  const {browser, page, value} = await registered();
  const result = await resolveXPassiveEvidence({browser, page, assets: value, posts: [post(), post()], deadlineAt: Date.now() + 1000});
  assert.ok(result.every(value => value.media[0].kind === 'video_poster'));
});

test('same response media-entity tuple can pair a generic poster without inventing its asset ID', async () => {
  const {browser, page, value, state} = await registered();
  const generic = 'https://pbs.twimg.com/media/generic-poster.jpg?name=large';
  state.result = envelope([candidate({media: [{kind: 'video', posterUrl: generic, playbackUrl: playback}]})]);
  const input = post({media: [{kind: 'video_poster', url: generic}]});
  const result = await resolveXPassiveEvidence({browser, page, assets: value, posts: [input], deadlineAt: Date.now() + 1000});
  assert.equal(result[0].media[0].playbackUrl, playback);
  assert.equal(result[0].media[0].pairing, 'same_video_media_entity');
  const unrelated = await resolveXPassiveEvidence({browser, page, assets: value,
    posts: [post({media: [{kind: 'video_poster', url: 'https://pbs.twimg.com/media/unrelated.jpg'}]})], deadlineAt: Date.now() + 1000});
  assert.equal(unrelated[0].media[0].kind, 'video_poster');
});

test('an existing inline video is not attributed to passive recovery of another unresolved poster', async () => {
  const {browser, page, value} = await registered();
  const original = {kind: 'video', playbackUrl: playback, provenance: 'x_main_structured_state'};
  const input = post({media: [original, {kind: 'video_poster', url: 'https://pbs.twimg.com/ext_tw_video_thumb/other/img/missing.jpg'}]});
  const result = await resolveXPassiveEvidence({browser, page, assets: value, posts: [input], deadlineAt: Date.now() + 1000});
  assert.equal(result[0].media[0].provenance, original.provenance);
  assert.equal(result[0].media[1].kind, 'video_poster');
  assert.equal(result[0].mediaEvidence.passiveResponse.playbackUrlRecovered, false);
});

test('expired deadline and borrowed backend cannot perform a passive evidence read', async () => {
  const {browser, page, value, state} = await registered();
  const input = [post()];
  await resolveXPassiveEvidence({browser, page, assets: value, posts: input, deadlineAt: Date.now() - 1});
  assert.equal(state.reads, 0);
  assert.equal(await resolveXPassiveEvidence({browser: {backend: 'browser_quiet_hidden'}, page, assets: value,
    posts: input, deadlineAt: Date.now() + 1000}), input);
  assert.equal(state.reads, 0);
});

test('capture grants expansion only to an owned round-one mutation-authorized page', async () => {
  for (const [backend, options, expected] of [
    ['headless_worker', {}, true], ['browser_quiet_hidden', {}, false],
    ['headless_worker', {sameTabMutationAllowed: false}, false],
    ['headless_worker', {pendingContentPolicy: 'detect_only'}, false],
  ]) {
    let policy;
    const page = {navigate: async () => ({}), evaluate: async expression => {
      if (expression.startsWith('globalThis.AkuHeadlessCapturePolicy=')) policy = expression;
      if (expression === 'location.href') return 'https://x.com/home';
      if (expression.includes('XHeadlessPoC.collect()')) return JSON.stringify({posts: [post({media: [], mediaExpected: []})], documentReady: true});
      return null;
    }};
    await capture({backend, forSource: async () => page}, {x: [{execute: false}, {execute: false}, {execute: false}]},
      'x', {scrolls: 0, captureTimeoutMs: 2000, sourceHydrationTimeoutMs: 1000, ...options});
    assert.ok(policy.includes(`allowContentExpansion:${expected}`));
  }
});

function domArticle() {
  const text = {nodeType: 1, tagName: 'DIV', innerText: 'DOM caption', childNodes: [{nodeType: 3, nodeValue: 'DOM caption'}],
    getAttribute: () => null, querySelector: () => null, querySelectorAll: () => [], closest: () => null};
  const time = {dateTime: '2026-10-12T00:00:00Z', closest: () => ({href: native})};
  return {innerText: 'DOM caption', getClientRects: () => [{}], getBoundingClientRect: () => ({height: 200, top: 0, bottom: 200}),
    querySelector: selector => selector === '[data-testid="User-Name"]' ? {innerText: 'Owner'} : selector.includes('tweetText') ? text : null,
    querySelectorAll: selector => selector === 'time' ? [time] : selector === '[data-testid="tweetText"]' ? [text] : []};
}

test('real GraphQL observer and shared parser carry a response-only reply through capture', async () => {
  const value = assets(), article = domArticle(), listeners = [], events = [];
  let context, boot;
  const graphURL = 'https://x.com/i/api/graphql/query/HomeTimeline';
  const payload = {data: {tweet: {__typename: 'Tweet', rest_id: ownID,
    legacy: {full_text: 'PRIVATE_GRAPHQL_BODY', in_reply_to_status_id_str: parentID},
    core: {user_results: {result: {avatar: {image_url: avatar}, core: {screen_name: 'Owner'}}}}}}};
  const page = {
    async send(method, parameters) {assert.equal(method, 'Page.addScriptToEvaluateOnNewDocument'); boot = parameters.source; events.push('register'); return {identifier: 'observer'};},
    async navigate(url) {
      events.push('navigate'); assert.ok(boot, 'observer must precede source navigation');
      context = vm.createContext({URL, Response, Headers, ReadableStream, ReadableStreamDefaultReader,
        TextDecoder, Uint8Array, setTimeout, clearTimeout, queueMicrotask, location: new URL(url),
        innerHeight: 900, innerWidth: 1200, scrollY: 0,
        MutationObserver: class {observe() {} disconnect() {}},
        document: {visibilityState: 'visible', readyState: 'complete', title: 'Fixture', scrollingElement: {scrollHeight: 1000},
          body: {innerText: ''}, querySelector: () => null,
          querySelectorAll: selector => selector === 'article[data-testid="tweet"]' || selector === 'main article' ? [article] : []},
        fetch: async () => {const response = new Response(JSON.stringify(payload), {headers: {'content-type': 'application/json'}});
          Object.defineProperty(response, 'url', {value: graphURL}); return response;},
        addEventListener: (type, handler) => {if (type === 'message') listeners.push(handler);}, removeEventListener: () => {},
        postMessage(data) {for (const listener of [...listeners]) listener({source: this, origin: 'https://x.com', data});},
      });
      context.window = context;
      vm.runInContext(boot, context);
      await vm.runInContext(`fetch(${JSON.stringify(graphURL)})`, context);
      await new Promise(resolve => setTimeout(resolve, 30));
      assert.equal(vm.runInContext(`AkuXMediaEvidenceRuntime.lookupReplyTo('x:status:${ownID}')`, context), parentID,
        JSON.stringify(vm.runInContext('AkuXMediaEvidenceRuntime.responseDiagnostics()', context)));
      return {};
    },
    async evaluate(expression) {return JSON.parse(JSON.stringify(await vm.runInContext(expression, context) ?? null));},
  };
  const browser = {backend: 'headless_worker', forSource: async () => page};
  const output = await capture(browser, {x: value}, 'x', {scrolls: 0, captureTimeoutMs: 3000, sourceHydrationTimeoutMs: 1000});
  const block = output.snapshots[0].blocks[0];
  assert.deepEqual(events.slice(0, 2), ['register', 'navigate']);
  assert.equal(block.directContext[0].target.id, parentID);
  assert.equal(block.directContext[0].provenance, 'observed_response');
  assert.equal(block.directContext[0].target.availability, 'reference_only');
  assert.equal(block.avatarUrl, avatar);
  assert.equal(block.text, 'DOM caption');
  assert.equal(JSON.stringify(output).includes('PRIVATE_GRAPHQL_BODY'), false);
});
