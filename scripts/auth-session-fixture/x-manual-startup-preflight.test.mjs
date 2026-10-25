import test from 'node:test';
import assert from 'node:assert/strict';
import {collectorShortCheckpoint, executeManualReply} from './x-native-metadata-probe-driver.mjs';
import {createManualStartupOwner} from './x-manual-startup-owner.mjs';
import {ManualStartupCookieGuard} from './x-manual-startup-cookie-guard.mjs';
import {createManualV2BoundObservation} from './x-qa-bound-observation.mjs';
import {startupFailure, startupFailureLatch, startupDiagnostic, requireBoundStartup} from './x-manual-startup-diagnostic.mjs';
import {registerFreshStartupSession} from './x-manual-root-controls.mjs';

// Exercise the actual collector path with an accountless transport. It cannot
// spawn a browser, access a profile, transfer cookies or publish real evidence.
function fixture(fault, diagnostics = startupFailureLatch()) {
  const calls = [];
  let bindingAttempts = 0, unsubscribed = 0;
  const guard = new ManualStartupCookieGuard('private-root', fault === 'guard-session' ? 'private-other-session' : 'private-session',
    {beforeSeed: fault === 'guard-baseline'});
  if (fault === 'guard-replay') guard.claim('private-root', 'private-session');
  if (fault === 'guard-closed') guard.finish();
  const failure = (error, phase, reason) => diagnostics.record(error, 'collector', phase, reason);
  const transport = {
    ownership: () => ({pid: 2, profileId: 'private-profile'}),
    subscribe: () => () => {unsubscribed++;},
    async send(method, params, session, project) {
      calls.push(method);
      assert.equal(session, 'private-session');
      if (method === 'Page.navigate') {
        assert.deepEqual(params, {url: 'https://x.com/home'});
        if (fault === 'navigation-command') throw new Error('PRIVATE_TOKEN https://private.invalid');
        return project(fault === 'navigation-error' ? {frameId: 'private-root', errorText: 'PRIVATE_TOKEN'} :
          fault === 'navigation-shape' ? {} : {frameId: 'private-root'});
      }
      if (method === 'Runtime.evaluate') {
        assert.equal(params.userGesture, false);
        assert.equal(params.awaitPromise, false);
        if (fault === 'readiness-command') throw new Error('PRIVATE_TOKEN');
        return project({result: {type: 'boolean', value: fault !== 'readiness-deadline'}});
      }
      if (method === 'Page.getFrameTree') return project({frameTree: {frame: {
        id: fault === 'guard-frame' ? 'private-other-root' : 'private-root',
        loaderId: 'private-loader', url: 'https://x.com/home',
      }}});
      if (method === 'Network.disable') return project({});
      assert.fail('Unexpected operation after rejected preflight');
    },
  };
  const current = {manualStartupSelected: true, shortFeasibility: {
    transport, sessionId: 'private-session', targetId: 'private-target', manualCookieGuard: guard,
  }};
  registerFreshStartupSession(current.shortFeasibility,'private-session','collector');
  const owner = createManualStartupOwner({role: 'collector', diagnostics, run: async sink => {
    try {
      const result = await collectorShortCheckpoint(current, undefined, sink.onPrivateStartup, sink.beforeIdentityRead, failure,
        (...args) => {
          bindingAttempts++;
          if (fault === 'guard-factory') throw new Error('PRIVATE_TOKEN');
          return createManualV2BoundObservation(...args, () => {}, guard);
        });
      requireBoundStartup(result?.checkpoint?.observation, 'collector', diagnostics);
    } catch (error) { throw failure(error, 'identity'); }
  }});
  return {owner, diagnostics, calls, bindingAttempts: () => bindingAttempts, unsubscribed: () => unsubscribed};
}

for (const [fault, phase, reason] of [
  ['navigation-command', 'navigation', 'unavailable'],
  ['navigation-error', 'navigation', 'rejected'],
  ['navigation-shape', 'navigation', 'rejected'],
  ['readiness-command', 'page_ready', 'unavailable'],
  ['guard-frame', 'guard_bind', 'rejected'],
  ['guard-session', 'guard_bind', 'rejected'],
  ['guard-baseline', 'guard_bind', 'rejected'],
  ['guard-replay', 'guard_bind', 'rejected'],
  ['guard-closed', 'guard_bind', 'rejected'],
  ['guard-factory', 'guard_bind', 'rejected'],
]) {
  test(`collector ${fault} keeps its origin through owner PREPARE and private RPC`, async () => {
    const f = fixture(fault), replies = [];
    await executeManualReply({id: 1, op: 'manual_prepare'}, () => f.owner.prepare(), (...args) => replies.push(args));
    assert.deepEqual(replies, [[1, undefined, `manual_initial_collector_${phase}_${reason}_primary`]]);
    assert.deepEqual(startupDiagnostic(f.diagnostics.record(new Error('PRIVATE_TOKEN'), 'collector', 'identity')),
      {role: 'collector', phase, reason, peer: false});
    assert.equal(f.calls.includes('Debugger.evaluateOnCallFrame'), false);
    assert.equal(f.calls.includes('Page.reload'), false);
    assert.equal(f.bindingAttempts(), phase === 'guard_bind' ? 1 : 0);
    assert.equal(f.unsubscribed(), phase === 'guard_bind' ? 1 : 0);
    assert.equal(f.calls.includes('Network.disable'), phase === 'guard_bind');
    assert.equal(JSON.stringify(replies).includes('PRIVATE'), false);
    await f.owner.cancel();
  });
}

test('collector readiness expiration is a page deadline before identity or binding', async t => {
  let clockCalls = 0;
  t.mock.method(Date, 'now', () => clockCalls++ < 2 ? 0 : 15000);
  const f = fixture('readiness-deadline');
  await assert.rejects(f.owner.prepare(), error => {
    assert.deepEqual(startupDiagnostic(error), {role: 'collector', phase: 'page_ready', reason: 'deadline', peer: false});
    return true;
  });
  assert.deepEqual(f.calls, ['Page.navigate', 'Runtime.evaluate']);
  assert.equal(f.bindingAttempts(), 0);
  await f.owner.cancel();
});

test('preflight classification preserves an earlier branded guard cause and safe trace', async () => {
  const diagnostics = startupFailureLatch();
  diagnostics.record(startupFailure('reader', 'bound_finish', 'observed_cookie_write', false, undefined,
    {firstCause: 'guard_header_classification', guardHeaderReason: 'syntax'}), 'reader', 'bound_finish');
  const f = fixture('navigation-command', diagnostics);
  await assert.rejects(f.owner.prepare(), error => {
    const d = startupDiagnostic(error);
    assert.equal(d.role, 'reader'); assert.equal(d.phase, 'bound_finish'); assert.equal(d.reason, 'observed_cookie_write');
    assert.equal(d.peer, true); assert.equal(d.trace.firstCause, 'guard_header_classification');
    assert.equal(d.trace.guardHeaderReason, 'syntax');
    return true;
  });
  await f.owner.cancel();
});
