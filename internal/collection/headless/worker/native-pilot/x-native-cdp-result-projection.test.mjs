import test from 'node:test';
import assert from 'node:assert/strict';
import {EventEmitter} from 'node:events';
import {createNativeOwnerRuntime} from './x-native-owner-runtime.mjs';
import {BOOTSTRAP_CURRENT_SELF_EXPRESSION} from './shared/x-bootstrap-current-self.mjs';
import {qaCookieRequest} from './shared/x-qa-source-qualification.mjs';
import {MANUAL_TIMING} from './shared/x-manual-timing-policy.mjs';

const ownerPort = () => ({observe() {}, invalidate() {}});
function harness(t, {active = {}, mode = 'single_phase'} = {}) {
  // Use the real runtime/dispatcher. Flattening its closure would hide the bug.
  const fixture = createNativeOwnerRuntime().fixture;
  const writes = [], input = new EventEmitter(), output = new EventEmitter();
  output.write = (bytes, callback) => {
    writes.push(JSON.parse(bytes.subarray(0, bytes.length - 1).toString('utf8')));
    callback();
    return true;
  };
  const child = new EventEmitter();
  child.stdio = [null, null, null, output, input];
  fixture.state.child = child;
  fixture.state.active = {owner: 1, targetId: 'home', sessionId: 'reader', ...active};
  fixture.state.driverMode = mode;
  fixture.bindChrome(child);
  const send = (...args) => {
    const promise = fixture.send(...args);
    promise.catch(() => {});
    return promise;
  };
  const reply = (result, overrides = {}) => {
    const command = writes.at(-1);
    assert.ok(command, 'an admitted command must have been written');
    input.emit('data', Buffer.from(JSON.stringify({id: command.id, result,
      ...(command.sessionId ? {sessionId: command.sessionId} : {}), ...overrides}) + '\0'));
  };
  t.after(() => {
    input.emit('close');
    child.removeAllListeners(); input.removeAllListeners(); output.removeAllListeners();
    fixture.state.cdpFrames.reset();
  });
  return {fixture, state: fixture.state, writes, send, reply, input};
}

test('CDP result characterization: ordinary inventory retains its exact projection and cap', async t => {
  for (const [name, result, expected] of [
    ['filtered', {targetInfos: [{targetId: 'home', type: 'page', url: 'https://x.com/home?SECRET'},
      {targetId: 'other', type: 'page', url: 'https://example.com/private'},
      {targetId: 'worker', type: 'worker', url: 'https://x.com/home'}]}, {targetIds: ['home']}],
    ['empty', {targetInfos: []}, {targetIds: []}],
    ['missing-array', {}, 'xnp_target_inventory_invalid'],
    ['cap', {targetInfos: Array.from({length: 129}, () => ({}))}, 'xnp_target_inventory_invalid'],
    ['non-object', [], 'xnp_cdp_result_invalid'],
  ]) await t.test(name, async t => {
    const h = harness(t), pending = h.send('Target.getTargets'); h.reply(result);
    if (typeof expected === 'string') await assert.rejects(pending, {message: expected});
    else assert.deepEqual(await pending, expected);
    assert.equal(h.state.cdpFailure, null, 'projection rejection does not fail the transport');
  });
});

test('CDP result characterization: frame/document and acknowledgment shapes stay unchanged', async t => {
  for (const [name, method, params, result, expected] of [
    ['frame', 'Page.getFrameTree', {}, {frameTree: {frame: {id: 'frame', loaderId: 'document', url: 'https://x.com/home'}}}, {frameId: 'frame', loaderId: 'document'}],
    ['foreign-frame', 'Page.getFrameTree', {}, {frameTree: {frame: {id: 'frame', loaderId: 'document', url: 'https://example.com/home'}}}, 'xnp_early_frame_invalid'],
    ['missing-document', 'Page.getFrameTree', {}, {frameTree: {frame: {id: 'frame', url: 'https://x.com/home'}}}, 'xnp_qa_live_document_invalid'],
    ['ack', 'Page.enable', {}, {ignored: 'PRIVATE'}, {acknowledged: true}],
  ]) await t.test(name, async t => {
    const h = harness(t, {active: {manualStartupPort: ownerPort(), manualObserver: {sessionId: 'reader'}}});
    const pending = h.send(method, params, 'reader', 'manual_owner'); h.reply(result);
    if (typeof expected === 'string') await assert.rejects(pending, {message: expected});
    else assert.deepEqual(await pending, expected);
  });
});

test('CDP result regression: manual inventory binds to the live owner and admits only its windows', async t => {
  const h = harness(t, {active: {manualStartupPort: ownerPort()}});
  const previous = h.state.active, pending = h.send('Target.getTargets', {}, undefined, 'manual_owner');
  const current = {owner: 2, sessionId: 'reader', targetId: 'replacement', manualStartupPort: ownerPort()};
  h.state.active = current;
  h.reply({targetInfos: [{targetId: 'replacement', type: 'page', url: 'https://example.com/private'},
    {targetId: 'worker', type: 'worker', url: 'https://x.com/home'}]});
  assert.deepEqual(await pending, {targetIds: ['replacement']});
  assert.equal(previous.manualInventoryTargets, undefined);
  assert.deepEqual([...current.manualInventoryTargets], ['replacement']);
  const count = h.writes.length;
  await assert.rejects(h.send('Browser.getWindowForTarget', {targetId: 'foreign'}, undefined, 'manual_owner'), /xnp_qa_short_window_denied/);
  assert.equal(h.writes.length, count);
  const window = h.send('Browser.getWindowForTarget', {targetId: 'replacement'}, undefined, 'manual_owner');
  h.reply({windowId: 7, privateField: 'SECRET'});
  assert.deepEqual(await window, {windowId: 7});
});

test('CDP result regression: separate runtime instances never share projection state', async t => {
  const left = harness(t, {active: {manualStartupPort: ownerPort()}});
  const right = harness(t, {active: {manualStartupPort: ownerPort()}});
  const a = left.send('Target.getTargets', {}, undefined, 'manual_owner');
  const b = right.send('Target.getTargets', {}, undefined, 'manual_owner');
  right.reply({targetInfos: [{type: 'page', targetId: 'right'}]});
  left.reply({targetInfos: [{type: 'page', targetId: 'left'}]});
  assert.deepEqual(await a, {targetIds: ['left']}); assert.deepEqual(await b, {targetIds: ['right']});
  assert.deepEqual([...left.state.active.manualInventoryTargets], ['left']);
  assert.deepEqual([...right.state.active.manualInventoryTargets], ['right']);
});

test('CDP result regression: instrumentation updates the owning runtime', async t => {
  const h = harness(t, {mode: 'retained_bootstrap', active: {earlyRunning: true}});
  const pending = h.send('Debugger.setInstrumentationBreakpoint', {instrumentation: 'beforeScriptExecution'}, 'reader');
  h.reply({breakpointId: 'opaque-breakpoint'});
  assert.deepEqual(await pending, {breakpointId: 'opaque-breakpoint'});
  assert.equal(h.state.active.earlyInstrumentationId, 'opaque-breakpoint');
});

test('CDP result regression: body mode is read at reply time and preserves value-free projection', async t => {
  const h = harness(t, {mode: 'retained_account_settings', active: {
    gate: {networkEnabled: true}, accountSettings: {consumeRead: () => true, invalidate() {}},
  }});
  const pending = h.send('Network.getResponseBody', {requestId: 'body'}, 'reader');
  h.state.driverMode = 'single_phase';
  h.reply({body: '{"screen_name":"PRIVATE"}', base64Encoded: false});
  await assert.rejects(pending, {message: 'xnp_body_mode_required'});
  assert.equal(h.state.cdpFailure, null);
  h.state.driverMode = 'retained_account_settings';
  const valid = h.send('Network.getResponseBody', {requestId: 'body'}, 'reader');
  h.reply({body: '{"screen_name":"PRIVATE"}', base64Encoded: false});
  const value = await valid;
  assert.equal(value.diagnostic, 'projected');
  assert.doesNotMatch(JSON.stringify(value), /PRIVATE/);
});

test('CDP result regression: cookie selection reads qualification state without exposing values', async t => {
  const h = harness(t, {mode: 'retained_source_qualification', active: {qaCookieRequested: true, qaSourceRoute: 'api_v1'}});
  const pending = h.send('Network.getCookies', qaCookieRequest('api_v1'), 'reader');
  h.reply({cookies: []});
  const value = await pending;
  assert.equal(value.schema, 'aku.x.qa-cookie-metadata.v1');
  assert.equal(value.canAuthorize, false); assert.equal(value.valuesPersisted, false);
  assert.deepEqual(value.tuples.map(item => item.status), ['missing', 'missing']);
  const count = h.writes.length;
  await assert.rejects(h.send('Network.getCookies', qaCookieRequest('api_v1'), 'reader'), /xnp_cookie_read_not_authorized/);
  assert.equal(h.writes.length, count);
});

test('CDP result regression: enrollment delegates to the current actor after admission', async t => {
  const key = 'AKU_X_MANUAL_RETAINED_TRANSFER_ONCE', previous = process.env[key];
  process.env[key] = '1';
  t.after(() => { if (previous === undefined) delete process.env[key]; else process.env[key] = previous; });
  const calls = [], h = harness(t, {mode: 'retained_source_qualification', active: {
    manualEnrollment: {command: () => true, invalidate() {}, project: () => assert.fail('old actor used')},
  }});
  const pending = h.send('Target.detachFromTarget', {sessionId: 'observer'}, undefined, 'manual_enrollment');
  h.state.active = {manualEnrollment: {invalidate() {}, project: (method, result) => {calls.push([method, result]); return {acknowledged: true};}}};
  h.reply({});
  assert.deepEqual(await pending, {acknowledged: true});
  assert.equal(calls.length, 1); assert.equal(calls[0][0], 'Target.detachFromTarget');
  assert.equal(Object.getPrototypeOf(calls[0][1]), null, 'strict JSON replies retain their null prototype');
  assert.deepEqual(Object.keys(calls[0][1]), []);
});

test('CDP result regression: bootstrap rejection uses the owning runtime mode', async t => {
  const h = harness(t, {mode: 'retained_bootstrap', active: {
    bootstrapAttempted: true, gate: {loginVisible: true, observationStarted: false},
  }});
  const pending = h.send('Runtime.evaluate', {expression: BOOTSTRAP_CURRENT_SELF_EXPRESSION,
    returnByValue: true, awaitPromise: false, userGesture: false, throwOnSideEffect: true, timeout: MANUAL_TIMING.bootstrapEvaluationMS}, 'reader');
  h.state.driverMode = 'single_phase';
  h.reply({result: {type: 'object', value: {}}});
  await assert.rejects(pending, {message: 'xnp_bootstrap_runtime_result_invalid'});
});

test('CDP result characterization: denied commands never reach the writer', async t => {
  for (const [name, args, code] of [
    ['unknown', ['Arbitrary.execute'], 'cdp_command_invalid'],
    ['detach', ['Target.detachFromTarget', {sessionId: 'reader'}], 'manual_enrollment_detach_denied'],
    ['body', ['Network.getResponseBody', {requestId: 'body'}, 'reader'], 'account_body_not_authorized'],
    ['evaluate', ['Runtime.evaluate', {expression: 'PRIVATE'}, 'reader'], 'runtime_expression_not_authorized'],
    ['purpose', ['Page.enable', {}, 'reader', 'untrusted'], 'early_command_not_authorized'],
  ]) await t.test(name, async t => {
    const h = harness(t);
    await assert.rejects(h.send(...args), {message: 'xnp_' + code});
    assert.deepEqual(h.writes, []);
  });
});

test('CDP result characterization: remote errors retain closed codes without failing the pipe', async t => {
  const h = harness(t), pending = h.send('Page.enable');
  h.reply(undefined, {error: {code: 17, message: 'PRIVATE TOKEN'}});
  await assert.rejects(pending, {message: 'xnp_page_enable_failed'});
  assert.equal(h.state.cdpFailure, null);
  const next = h.send('Page.enable'); h.reply({});
  assert.deepEqual(await next, {acknowledged: true});
});

test('CDP result characterization: original timeout rejects all pending commands and fences late replies', async t => {
  t.mock.timers.enable({apis: ['setTimeout']});
  const invalidations = [];
  const h = harness(t, {active: {manualStartupPort: {invalidate: cause => invalidations.push(cause)}}});
  const a = h.send('Page.enable'), b = h.send('Page.enable');
  t.mock.timers.tick(MANUAL_TIMING.cdpCommandMS);
  await assert.rejects(a, {message: 'xnp_page_enable_timeout'});
  await assert.rejects(b, {message: 'xnp_page_enable_timeout'});
  assert.equal(h.state.cdpFailure, 'page_enable_timeout');
  assert.ok(invalidations.length > 0 && invalidations.every(value => value === 'cdp_timeout'));
  h.reply({});
  const count = h.writes.length;
  await assert.rejects(h.send('Page.enable'), {message: 'xnp_page_enable_timeout'});
  assert.equal(h.writes.length, count); assert.equal(h.state.cdpFrames.length, 0);
});

test('CDP result characterization: reply fencing and pipe cleanup retain their errors', async t => {
  for (const [name, overrides, code] of [
    ['session', {sessionId: 'foreign'}, 'cdp_session_mismatch'],
    ['id', {id: 99}, 'cdp_response_unexpected'],
    ['extra-field', {private: 'SECRET'}, 'cdp_message_invalid'],
  ]) await t.test(name, async t => {
    const h = harness(t), pending = h.send('Page.enable', {}, 'reader'); h.reply({}, overrides);
    await assert.rejects(pending, {message: 'xnp_' + code});
    assert.equal(h.state.cdpFailure, code);
  });
  await t.test('pipe-close-rejects-all-pending', async t => {
    const h = harness(t), a = h.send('Page.enable'), b = h.send('Page.enable');
    h.input.emit('close');
    await assert.rejects(a, {message: 'xnp_cdp_pipe_closed'}); await assert.rejects(b, {message: 'xnp_cdp_pipe_closed'});
    const count = h.writes.length;
    await assert.rejects(h.send('Page.enable'), {message: 'xnp_cdp_pipe_closed'});
    assert.equal(h.writes.length, count); assert.equal(h.state.cdpFrames.length, 0);
  });
});
