import assert from 'node:assert/strict';
import test from 'node:test';

import { EarlyBootstrapCapture } from './x-early-bootstrap-capture.mjs';
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';

const ROOT_FRAME = 'root-frame';
const CHILD_FRAME = 'child-frame';
const PRIVATE_MARKER = 'PRIVATE-EARLY-CAPTURE-MARKER';
const testContexts = new WeakMap();
let nextPause = 0;

function missingDto() {
  return inspectBootstrapCurrentSelf({
    location: { origin: 'https://x.com', pathname: '/home' },
  });
}

function oneIdDto(id = '123456') {
  return inspectBootstrapCurrentSelf({
    location: { origin: 'https://x.com', pathname: '/home' },
    __INITIAL_STATE__: { session: { user_id_str: id } },
  });
}

function twoIdDto(id = '987654') {
  return inspectBootstrapCurrentSelf({
    location: { origin: 'https://x.com', pathname: '/home' },
    __INITIAL_STATE__: { session: { user_id: id, user_id_str: id } },
  });
}

function harness({ onSend, readBootstrap, now } = {}) {
  const calls = [];
  let capture;
  const send = async (method, params) => {
    calls.push({ method, params: structuredClone(params) });
    if (onSend !== undefined) {
      const result = await onSend(method, params, capture);
      if (result !== undefined) return result;
    }
    if (method === 'Page.getFrameTree') return { frameId: ROOT_FRAME };
    if (method === 'Debugger.setInstrumentationBreakpoint') return { breakpointId: 'private-breakpoint-id' };
    return { acknowledged: true };
  };
  capture = new EarlyBootstrapCapture({
    send,
    readBootstrap: readBootstrap ?? (async () => missingDto()),
    now,
  });
  return { capture, calls };
}

function home(capture, frameId = ROOT_FRAME) {
  return capture.observe({ type: 'frame_navigated', frameId, isHome: true });
}

function context(capture, contextId, frameId = ROOT_FRAME) {
  if (frameId === ROOT_FRAME) testContexts.set(capture, contextId);
  return capture.observe({ type: 'context_created', contextId, frameId, isDefault: true });
}

function pause(capture, reason = 'instrumentation') {
  const scriptId = `script-${++nextPause}`;
  capture.observe({ type: 'script_parsed', scriptId, contextId: testContexts.get(capture), frameId: ROOT_FRAME, isDefault: true });
  return capture.observe({ type: 'paused', reason, callFrameId: `call-${nextPause}`, scriptId });
}

function methods(calls) {
  return calls.map(({ method }) => method);
}

test('waits for delayed enable and disable acknowledgments instead of abandoning pending commands', async () => {
  const { capture } = harness({
    async onSend(method, _params, current) {
      if (method === 'Runtime.enable' || method === 'Debugger.disable') await new Promise(resolve => setTimeout(resolve, 300));
      if (method === 'Page.reload') { home(current); context(current, 1); pause(current); }
    },
    readBootstrap: async () => oneIdDto(),
  });
  const result = await capture.run();
  assert.equal(result.stopReason, 'captured');
  assert.equal(result.cleanupVerified, true);
});

test('keeps the latest missing read until a current self projection is available, then cleans up', async () => {
  let resultIndex = 0;
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 11);
        pause(current);
        pause(current);
      }
      return undefined;
    },
    readBootstrap: async (contextId, callFrameId) => {
      assert.equal(contextId, 11);
      assert.equal(capture.canRead(contextId), true);
      assert.equal(capture.canReadCallFrame(callFrameId), true);
      assert.equal(capture.canReadCallFrame('unbound-call'), false);
      resultIndex += 1;
      return resultIndex === 1 ? missingDto() : oneIdDto();
    },
  });

  const result = await capture.run();

  assert.equal(result.schema, 'aku.x.early-bootstrap-capture.v1');
  assert.equal(result.stopReason, 'captured');
  assert.equal(result.pauseCount, 2);
  assert.equal(result.readAttempts, 2);
  assert.equal(result.bootstrapSeen, true);
  assert.equal(result.snapshot.stableIdStringCandidates, 1);
  assert.equal(result.cleanupVerified, true);
  assert.equal(result.canAuthorize, false);
  assert.equal(result.continuityProven, false);
  assert.equal(JSON.stringify(result).includes('123456'), false);
  assert.deepEqual(methods(calls), [
    'Page.getFrameTree',
    'Runtime.enable',
    'Debugger.enable',
    'Debugger.setInstrumentationBreakpoint',
    'Page.reload',
    'Debugger.resume',
    'Debugger.resume',
    'Debugger.removeBreakpoint',
    'Debugger.disable',
    'Runtime.disable',
  ]);
  assert.deepEqual(calls.find(({ method }) => method === 'Page.reload').params, { ignoreCache: false });
  assert.deepEqual(calls.find(({ method }) => method === 'Debugger.setInstrumentationBreakpoint').params, {
    instrumentation: 'beforeScriptExecution',
  });
  assert.deepEqual(calls.find(({ method }) => method === 'Debugger.removeBreakpoint').params, {
    breakpointId: 'private-breakpoint-id',
  });
  assert.equal(capture.canRead(11), false);
  assert.equal(capture.canReadCallFrame('call-1'), false);
});

test('requires the top paused script to be bound to the new root default context', async (t) => {
  for (const scenario of ['unknown_script', 'child_frame', 'old_context', 'old_navigation', 'missing_top']) {
    await t.test(scenario, async () => {
      let clock = 0;
      const { capture } = harness({
        now: () => clock,
        onSend(method, _params, current) {
          if (method === 'Page.reload') {
            home(current);
            context(current, 201);
            if (scenario !== 'unknown_script' && scenario !== 'missing_top') {
              current.observe({ type: 'script_parsed', scriptId: 'bound-script', contextId: 201,
                frameId: scenario === 'child_frame' ? CHILD_FRAME : ROOT_FRAME, isDefault: true });
            }
            if (scenario === 'old_context') {
              current.observe({ type: 'context_destroyed', contextId: 201 });
              context(current, 202);
            }
            if (scenario === 'old_navigation') { home(current); context(current, 202); }
            current.observe({ type: 'paused', reason: 'instrumentation',
              callFrameId: scenario === 'missing_top' ? null : 'private-call',
              scriptId: scenario === 'missing_top' ? null : 'bound-script' });
          }
          if (method === 'Debugger.resume') {
            clock = 5000;
            current.observe({ type: 'contexts_cleared' });
          }
        },
        readBootstrap: async () => { assert.fail('unbound call frame must never be evaluated'); },
      });
      const result = await capture.run();
      assert.equal(result.readAttempts, 0);
      assert.equal(result.snapshot, null);
      assert.equal(result.cleanupVerified, true);
      assert.equal(JSON.stringify(result).includes('private-call'), false);
    });
  }
});

test('discards a paused read if the VM resumes before its result returns', async () => {
  const { capture } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') { home(current); context(current, 203); pause(current); }
    },
    readBootstrap: async (_contextId, callFrameId) => {
      assert.equal(capture.canReadCallFrame(callFrameId), true);
      capture.observe({ type: 'resumed' });
      assert.equal(capture.canReadCallFrame(callFrameId), false);
      return oneIdDto();
    },
  });
  const result = await capture.run();
  assert.equal(result.stopReason, 'evaluation_failed');
  assert.equal(result.snapshot, null);
  assert.equal(result.cleanupVerified, true);
});

test('removes instrumentation before draining pauses caused by resume', async () => {
  let breakpointRemoved = false;
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 13);
        pause(current);
      }
      if (method === 'Debugger.resume' && !breakpointRemoved) pause(current);
      if (method === 'Debugger.removeBreakpoint') breakpointRemoved = true;
      return undefined;
    },
    readBootstrap: async () => oneIdDto(),
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'captured');
  assert.equal(result.pauseCount, 2);
  assert.equal(result.readAttempts, 1);
  assert.equal(result.cleanupVerified, true);
  const names = methods(calls);
  assert.ok(names.indexOf('Debugger.removeBreakpoint') < names.lastIndexOf('Debugger.resume'));
});

test('ignores contexts from before navigation and from child frames', async () => {
  const readContexts = [];
  const { capture } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        context(current, 21);
        home(current);
        context(current, 22, CHILD_FRAME);
        pause(current);
        context(current, 23);
        pause(current);
      }
      return undefined;
    },
    readBootstrap: async (contextId) => {
      readContexts.push(contextId);
      return oneIdDto();
    },
  });

  const result = await capture.run();

  assert.deepEqual(readContexts, [23]);
  assert.equal(result.stopReason, 'captured');
  assert.equal(result.pauseCount, 2);
  assert.equal(result.readAttempts, 1);
});

test('discards a read when its context is destroyed and reads only from the replacement context', async () => {
  const readContexts = [];
  const { capture } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 31);
        pause(current);
      }
      return undefined;
    },
    readBootstrap: async (contextId) => {
      readContexts.push(contextId);
      if (contextId === 31) {
        capture.observe({ type: 'context_destroyed', contextId: 31 });
        context(capture, 32);
        pause(capture);
        return oneIdDto('111');
      }
      return twoIdDto('222');
    },
  });

  const result = await capture.run();

  assert.deepEqual(readContexts, [31, 32]);
  assert.equal(result.stopReason, 'captured');
  assert.equal(result.pauseCount, 2);
  assert.equal(result.readAttempts, 2);
  assert.equal(result.snapshot.stableIdStringCandidates, 2);
  assert.equal(result.snapshot.sessionIdsAgree, true);
});

test('invalidates a read when the root navigates away during evaluation', async () => {
  const { capture } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 41);
        pause(current);
      }
      return undefined;
    },
    readBootstrap: async () => {
      capture.observe({ type: 'frame_navigated', frameId: ROOT_FRAME, isHome: false });
      return oneIdDto();
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'wrong_page');
  assert.equal(result.snapshot, null);
  assert.equal(result.bootstrapSeen, false);
  assert.equal(result.cleanupVerified, true);
});

test('stops at the fixed pause cap and attempts one read and resume per accepted pause', async () => {
  let breakpointRemoved = false;
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 51);
        pause(current);
      }
      if (method === 'Debugger.resume' && !breakpointRemoved) pause(current);
      if (method === 'Debugger.removeBreakpoint') breakpointRemoved = true;
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'script_limit');
  assert.equal(result.pauseCount, 64);
  assert.equal(result.readAttempts, 64, JSON.stringify(result));
  assert.ok(methods(calls).filter((method) => method === 'Debugger.resume').length >= 64);
  assert.ok(methods(calls).indexOf('Debugger.removeBreakpoint') < methods(calls).lastIndexOf('Debugger.resume'));
  assert.equal(result.snapshot.diagnostic, 'bootstrap_missing');
  assert.equal(result.cleanupVerified, true);
});

test('checks the monotonic deadline as projected events arrive', async () => {
  let clock = 100;
  const { capture } = harness({
    now: () => clock,
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 61);
        clock = 5100;
        current.observe({ type: 'contexts_cleared' });
      }
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'deadline');
  assert.equal(result.pauseCount, 0);
  assert.equal(result.readAttempts, 0);
  assert.equal(result.snapshot, null);
  assert.equal(result.cleanupVerified, true);
});

test('rejects replay after the one-use capture promise completes', async () => {
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') current.observe({ type: 'frame_navigated', frameId: ROOT_FRAME, isHome: false });
      return undefined;
    },
  });

  const first = await capture.run();
  const callsAfterFirstRun = calls.length;

  assert.equal(first.stopReason, 'wrong_page');
  await assert.rejects(capture.run(), { message: 'early_bootstrap_capture_replay_denied' });
  assert.equal(calls.length, callsAfterFirstRun);
});

test('resumes an unexpected pause and stops without reading bootstrap state', async () => {
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 71);
        pause(current, 'other');
      }
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'unexpected_pause');
  assert.equal(result.pauseCount, 1);
  assert.equal(result.readAttempts, 0);
  assert.equal(methods(calls).filter((method) => method === 'Debugger.resume').length, 1);
  assert.equal(result.cleanupVerified, true);
});

test('rejects event projections with extra private fields and emits no supplied strings', async () => {
  const { capture } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        current.observe({
          type: 'frame_navigated',
          frameId: ROOT_FRAME,
          isHome: true,
          privateValue: PRIVATE_MARKER,
        });
      }
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'evaluation_failed');
  assert.equal(result.snapshot, null);
  assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
});

test('rejects an invalid bootstrap projection without returning its extra fields', async () => {
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 81);
        pause(current);
      }
      return undefined;
    },
    readBootstrap: async () => ({ ...oneIdDto(), privateValue: PRIVATE_MARKER }),
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'evaluation_failed');
  assert.equal(result.readAttempts, 1);
  assert.equal(methods(calls).includes('Debugger.resume'), true);
  assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
  assert.equal(result.cleanupVerified, true);
});

test('cleans up after a reload setup failure', async () => {
  const { capture, calls } = harness({
    onSend(method) {
      if (method === 'Page.reload') throw new Error(PRIVATE_MARKER);
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'setup_failed');
  assert.equal(result.cleanupVerified, true);
  assert.deepEqual(methods(calls).slice(-3), [
    'Debugger.removeBreakpoint',
    'Debugger.disable',
    'Runtime.disable',
  ]);
  assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
});

test('keeps cleanup unverified when breakpoint setup returns an unprojected ID', async () => {
  const { capture, calls } = harness({
    onSend(method) {
      if (method === 'Debugger.setInstrumentationBreakpoint') {
        return { breakpointId: 'private-breakpoint-id', privateValue: PRIVATE_MARKER };
      }
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'setup_failed');
  assert.equal(result.cleanupVerified, false);
  assert.equal(methods(calls).includes('Debugger.removeBreakpoint'), false);
  assert.equal(methods(calls).includes('Debugger.disable'), true);
  assert.equal(methods(calls).includes('Runtime.disable'), true);
  assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
});

test('read failures resume the page and hide error text', async () => {
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 91);
        pause(current);
      }
      return undefined;
    },
    readBootstrap: async () => {
      throw new Error(PRIVATE_MARKER);
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'evaluation_failed');
  assert.equal(result.readAttempts, 1);
  assert.equal(methods(calls).includes('Debugger.resume'), true);
  assert.equal(result.cleanupVerified, true);
  assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
});

test('reports unverified cleanup after a resume failure while still disabling both domains', async () => {
  let resumeAttempts = 0;
  const { capture, calls } = harness({
    onSend(method, _params, current) {
      if (method === 'Page.reload') {
        home(current);
        context(current, 101);
        pause(current, 'other');
      }
      if (method === 'Debugger.resume') {
        resumeAttempts += 1;
        throw new Error(PRIVATE_MARKER);
      }
      return undefined;
    },
  });

  const result = await capture.run();

  assert.equal(result.stopReason, 'unexpected_pause');
  assert.equal(resumeAttempts, 2);
  assert.equal(result.cleanupVerified, false);
  assert.equal(methods(calls).slice(-2).join(','), 'Debugger.disable,Runtime.disable');
  assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
});

test('continues cleanup after breakpoint removal or domain disable failures', async (t) => {
  for (const failedMethod of ['Debugger.removeBreakpoint', 'Debugger.disable', 'Runtime.disable']) {
    await t.test(failedMethod, async () => {
      const { capture, calls } = harness({
        onSend(method, _params, current) {
          if (method === 'Page.reload') {
            home(current);
            context(current, failedMethod === 'Debugger.disable' ? 111 : 112);
            pause(current);
          }
          if (method === failedMethod) return { acknowledged: false };
          return undefined;
        },
        readBootstrap: async () => oneIdDto(),
      });

      const result = await capture.run();

      assert.equal(result.stopReason, 'captured');
      assert.equal(result.cleanupVerified, false);
      assert.equal(methods(calls).includes('Debugger.removeBreakpoint'), true);
      assert.equal(methods(calls).includes('Debugger.disable'), true);
      assert.equal(methods(calls).includes('Runtime.disable'), true);
      assert.equal(JSON.stringify(result).includes(PRIVATE_MARKER), false);
    });
  }
});

test('fails closed on invalid and backward-moving clocks', async (t) => {
  await t.test('invalid value', async () => {
    const { capture, calls } = harness({ now: () => Number.NaN });

    const result = await capture.run();

    assert.equal(result.stopReason, 'evaluation_failed');
    assert.equal(methods(calls).includes('Page.reload'), false);
    assert.equal(result.cleanupVerified, true);
  });

  await t.test('clock rollback', async () => {
    const values = [10, 9];
    const { capture } = harness({
      now: () => values.shift() ?? 9,
      onSend(method, _params, current) {
        if (method === 'Page.reload') {
          home(current);
          context(current, 121);
          pause(current);
        }
        return undefined;
      },
    });

    const result = await capture.run();

    assert.equal(result.stopReason, 'evaluation_failed');
    assert.equal(result.readAttempts, 0);
    assert.equal(result.snapshot, null);
  });
});
