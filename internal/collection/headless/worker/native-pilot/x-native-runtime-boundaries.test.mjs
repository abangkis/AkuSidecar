import test from 'node:test';
import assert from 'node:assert/strict';
import {mkdir, mkdtemp, writeFile, rm} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import {join, relative, sep} from 'node:path';
import * as runtime from './x-native-owner-runtime.mjs';

const d = runtime.nativeProbeDefinitions;
test('native runtime keeps its supported exports, fixture identities and class contracts', () => {
  assert.deepEqual(Object.keys(runtime).sort(), ['CDPFrameDecoder', 'LoginObservationGate', 'MetadataWindow',
    'TwoPhaseWorkflowGrant', 'classifyXOperation', 'createNativeOwnerRuntime', 'manualSeedRPCCode',
    'nativeProbeDefinitions', 'projectCDPEvent', 'projectNetworkEvent', 'startupCDPFailureCategory', 'twoPhaseChromeArgs'].sort());
  assert.equal(Object.isFrozen(d), true);
  for (const name of ['CDPFrameDecoder', 'LoginObservationGate', 'MetadataWindow', 'TwoPhaseWorkflowGrant',
    'classifyXOperation', 'projectCDPEvent', 'projectNetworkEvent', 'startupCDPFailureCategory', 'twoPhaseChromeArgs']) {
    assert.equal(runtime[name], d[name]);
  }
  const decoder = new runtime.CDPFrameDecoder();
  assert.ok(decoder instanceof d.CDPFrameDecoder);
  assert.equal(Object.getOwnPropertyDescriptor(runtime.CDPFrameDecoder.prototype, 'push').enumerable, false);
  decoder.reset();
});

test('framing preserves strict JSON, UTF-8 and bounded input rejection', async t => {
  const cases = [
    ['empty', Buffer.alloc(0), 'cdp_frame_size_invalid'],
    ['array', Buffer.from('[]'), 'cdp_json_invalid'],
    ['primitive', Buffer.from('true'), 'cdp_json_invalid'],
    ['duplicate-key', Buffer.from('{"a":1,"a":2}'), 'cdp_json_invalid'],
    ['invalid-utf8', Buffer.from([0xff]), 'cdp_utf8_invalid'],
    ['bom', Buffer.from([0xef, 0xbb, 0xbf, 123, 125]), 'cdp_json_invalid'],
    ['depth', Buffer.from('{"v":'.repeat(21) + '0' + '}'.repeat(21)), 'cdp_json_invalid'],
  ];
  for (const [name, bytes, code] of cases) await t.test(name, () => {
    assert.throws(() => d.parseJSONBytes(bytes), {message: 'xnp_' + code});
  });
  const value = d.parseJSONBytes(Buffer.from('{"id":7}'));
  assert.equal(Object.getPrototypeOf(value), null); assert.equal(value.id, 7);
  assert.throws(() => d.parseJSONBytes(Buffer.from('{}'), 1), {message: 'xnp_cdp_frame_size_invalid'});
});

test('framing constructor limits and per-chunk budgets keep their exact boundaries', async t => {
  for (const [name, options] of [
    ['zero-frame', {maxFrameBytes: 0}], ['large-frame', {maxFrameBytes: 262145}],
    ['fraction', {maxFrameBytes: 1.5}], ['input-zero', {maxInputBytes: 0}],
    ['input-large', {maxInputBytes: 1048577}], ['count-zero', {maxFramesPerChunk: 0}],
    ['count-large', {maxFramesPerChunk: 129}],
  ]) await t.test(name, () => assert.throws(() => new runtime.CDPFrameDecoder(options), {message: 'xnp_cdp_decoder_limits_invalid'}));
  const decoder = new runtime.CDPFrameDecoder({maxFrameBytes: 2, maxInputBytes: 16, maxFramesPerChunk: 2});
  const seen = [];
  decoder.push(Buffer.from('{}\0{}\0'), value => seen.push(value));
  decoder.push(Buffer.from('{}\0'), value => seen.push(value));
  assert.equal(seen.length, 3, 'frame-count budget resets for the next input chunk');
  assert.throws(() => decoder.push(Buffer.from('{}\0{}\0{}\0'), () => {}), {message: 'xnp_cdp_frame_count_exceeded'});
  assert.equal(decoder.length, 2, 'standalone codec leaves the rejected frame for owner cleanup');
  decoder.reset(); assert.ok(decoder.buffer.every(byte => byte === 0));
  assert.throws(() => decoder.push(Buffer.alloc(17), () => {}), {message: 'xnp_cdp_input_invalid'});
  assert.throws(() => decoder.push([], () => {}), {message: 'xnp_cdp_input_invalid'});
});

test('framing clears completed bytes before callbacks and retains finish/reset semantics', () => {
  const decoder = new runtime.CDPFrameDecoder({maxFrameBytes: 64});
  const marker = new Error('callback sentinel');
  assert.throws(() => decoder.push(Buffer.from('{"private":1}\0'), () => {
    assert.equal(decoder.length, 0);
    assert.ok(decoder.buffer.subarray(0, 13).every(byte => byte === 0));
    throw marker;
  }), error => error === marker);
  decoder.finish();
  decoder.push(Buffer.from('{'), () => assert.fail('unterminated frame delivered'));
  assert.throws(() => decoder.finish(), {message: 'xnp_cdp_frame_unterminated'});
  decoder.reset(); decoder.finish(); assert.ok(decoder.buffer.every(byte => byte === 0));
  let count = 0; decoder.push(Buffer.from('{}\0'), () => count++); assert.equal(count, 1);
});

test('value contracts retain loopback origin, exact-key and error identity rules', async t => {
  assert.equal(d.parseControlOrigin('http://127.0.0.1:7341'), 'http://127.0.0.1:7341');
  for (const value of ['http://localhost:7341', 'https://127.0.0.1:7341', 'http://127.0.0.1',
    'http://127.0.0.1:7341/', 'http://127.0.0.1:7341/path', 'http://u:p@127.0.0.1:7341', 'http://127.0.0.1:7341?secret=1']) {
    await t.test(value, () => assert.throws(() => d.parseControlOrigin(value), {message: 'xnp_origin_invalid'}));
  }
  assert.doesNotThrow(() => d.exactKeys({a: 1}, ['a'], 'exact'));
  assert.throws(() => d.exactKeys({a: 1, b: 2}, ['a'], 'exact'), {message: 'xnp_exact'});
  assert.throws(() => d.exactKeys([], [], 'exact'), {message: 'xnp_exact'});
  assert.equal(d.probeError('fixed').message, 'xnp_fixed');
  assert.equal(d.isObject(Object.create(null)), true); assert.equal(d.isObject([]), false);
});

test('metadata retains privacy, cap, ambiguity and destructive finish boundaries', () => {
  const window = new runtime.MetadataWindow(1), key = 'PRIVATE_REQUEST';
  const request = runtime.projectNetworkEvent('Network.requestWillBeSent', {requestId: key, timestamp: 1,
    request: {url: 'https://x.com/i/api/graphql/hash/Viewer?PRIVATE', method: 'GET', postData: 'PRIVATE_BODY'}});
  assert.equal(Object.isFrozen(request), true); window.observe(request);
  window.observe({type: 'request', requestKey: 'dropped', operation: 'rest:other', method: 'GET', timestamp: 1});
  window.observe(runtime.projectNetworkEvent('Network.responseReceived', {requestId: key, timestamp: 1.126,
    response: {status: 200, headers: {secret: 'PRIVATE'}, fromDiskCache: false}}));
  window.observe(runtime.projectNetworkEvent('Network.loadingFinished', {requestId: key, timestamp: 1.375, encodedDataLength: 9.9}));
  const result = window.finish();
  assert.equal(result.dropped, 1); assert.equal(result.candidates.length, 1);
  assert.equal(result.candidates[0].alias, 'r1'); assert.equal(result.candidates[0].bytes, 9);
  assert.deepEqual(result.candidates[0].timingMs, {response: 250, completion: 500});
  assert.doesNotMatch(JSON.stringify(result), /PRIVATE|requestKey|postData|headers/);
  assert.equal(window.requests.size, 0); assert.deepEqual(window.finish(), {candidates: [], dropped: 1});
  window.observe(request); window.observe(request);
  window.observe({type: 'response', requestKey: key, status: 200, timestamp: 2, hasExtraInfo: true});
  assert.equal(window.finish().candidates[0].ambiguous, true);
});

test('event projection retains default-root filtering, opaque IDs and unknown-event behavior', () => {
  assert.equal(runtime.projectCDPEvent({method: 'Page.frameNavigated', params: {frame: {id: 'sub', parentId: 'root'}}}), null);
  assert.equal(runtime.projectCDPEvent({method: 'Unknown.event', params: {private: 'PRIVATE'}}), null);
  assert.throws(() => runtime.projectCDPEvent({method: 'Page.loadEventFired', params: []}), {message: 'xnp_cdp_event_invalid'});
  const event = runtime.projectCDPEvent({method: 'Debugger.paused', sessionId: 'session', params: {
    reason: 'instrumentation', callFrames: [{callFrameId: 'top', location: {scriptId: 'script'}, url: 'PRIVATE', scopeChain: [{secret: 'PRIVATE'}]}, {callFrameId: 'ignored'}],
  }});
  assert.deepEqual(event, {type: 'early', sessionId: 'session', event: {type: 'paused', reason: 'instrumentation', callFrameId: 'top', scriptId: 'script'}});
  assert.doesNotMatch(JSON.stringify(event), /PRIVATE|scopeChain|ignored/);
  const navigated = runtime.projectCDPEvent({method: 'Page.frameNavigated', params: {frame: {id: 'root', url: 'https://x.com/home?private'}}});
  assert.equal(navigated.event.isHome, true);
});

test('observation and workflow policy keep state per instance and consumed deadlines', () => {
  const a = new runtime.LoginObservationGate(), b = new runtime.LoginObservationGate();
  assert.throws(() => a.beginObservation(), {message: 'xnp_observation_gate_closed'});
  a.setLoginVisible(true); a.beginObservation(); a.markNetworkEnabled(); a.markReloaded();
  assert.throws(() => a.markReloaded(), {message: 'xnp_reload_order_invalid'}); assert.equal(b.networkEnabled, false);
  let now = 100;
  const grant = new runtime.TwoPhaseWorkflowGrant({owner: 1, pid: 2, now: () => now});
  grant.authorize({owner: 1, oldPID: 2, nonce: 'a'.repeat(64), exitCode: 0, identityUnchanged: true});
  now = 10100; assert.throws(() => grant.consume({identityUnchanged: true}), {message: 'xnp_workflow_grant_expired'});
  assert.throws(() => grant.consume({identityUnchanged: true}), {message: 'xnp_workflow_grant_replay'});
});

test('directory identity and Chrome argument policy remain independent of runtime state', async () => {
  const root = fileURLToPath(new URL('../../../../../build/cl5-characterization/', import.meta.url));
  await mkdir(root, {recursive: true}); const directory = await mkdtemp(join(root, 'scope-'));
  try {
    const identity = await d.ordinaryDirectoryIdentity(directory);
    assert.equal(Object.isFrozen(identity), true);
    assert.equal(d.sameDirectoryIdentity(identity, await d.ordinaryDirectoryIdentity(directory)), true);
    for (const key of ['real', 'dev', 'ino', 'birthtimeNs']) assert.equal(d.sameDirectoryIdentity(identity, {...identity, [key]: 'different'}), false);
    const file = join(directory, 'file'); await writeFile(file, 'fixture');
    await assert.rejects(d.ordinaryDirectoryIdentity(file), {message: 'xnp_owned_directory_invalid'});
    const standard = runtime.twoPhaseChromeArgs(directory, false), pipe = runtime.twoPhaseChromeArgs(directory, true);
    assert.deepEqual(pipe, [...standard.slice(0, -1), '--remote-debugging-pipe', standard.at(-1)]);
    assert.equal(standard.at(-1), 'https://x.com/home');
  } finally {
    const child = relative(root, directory); assert.ok(child && !child.startsWith('..') && !child.includes(sep));
    await rm(directory, {recursive: true, force: true});
  }
});

test('startup failure categories stay closed and never trust arbitrary exception messages', () => {
  assert.equal(runtime.startupCDPFailureCategory('borrowed_reader_command_failed', Error('timeout PRIVATE')), 'cdp_borrowed_unclassified');
  for (const [code, expected] of [['cdp_pipe_closed', 'cdp_pipe_failure'], ['chrome_exited', 'cdp_process_failure'],
    ['cdp_utf8_invalid', 'cdp_frame_failure'], ['page_enable_timeout', 'cdp_timeout'], ['arbitrary_PRIVATE', 'cdp_other']]) {
    assert.equal(runtime.startupCDPFailureCategory(code), expected);
  }
});
