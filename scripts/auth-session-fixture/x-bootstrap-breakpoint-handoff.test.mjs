import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { BootstrapHandoffCapture, BootstrapBreakpointHandoff } from './x-bootstrap-breakpoint-handoff.mjs';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION, validateBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { QA_BOUND_BOOTSTRAP_EXPRESSION, projectBoundBootstrap } from './x-qa-bound-observation.mjs';

function fixture(fault, boundIdentity = false) {
  let capture;
  let index = 0;
  let phase = 'instrumentation';
  let instrumentation = true;
  let normal = null;
  let evaluations = 0;
  const context = vm.createContext({ location: { origin: 'https://x.com', pathname: '/home' } });
  const scripts = ['globalThis.__INITIAL_STATE__={session:{user_id_str:"123456"}}', 'void 0', 'delete globalThis.__INITIAL_STATE__', 'void 0'];
  if (boundIdentity) scripts[0] = 'globalThis.__INITIAL_STATE__={session:{user_id:"123456"},entities:{users:{entities:{"123456":{id_str:"123456"}}}}}';
  const observe = (event, raw = {}) => capture.observe(event, raw);
  const pause = (ordinary = false) => {
    phase = ordinary ? 'normal' : 'instrumentation';
    observe({ type: 'script_parsed', scriptId: `private-script-${index}`, contextId: 1, frameId: 'private-root', isDefault: true });
    observe({ type: 'paused', reason: ordinary ? 'other' : 'instrumentation', callFrameId: 'private-call', scriptId: `private-script-${index}` }, {
      hitBreakpoints: ordinary ? [fault === 'wrong_hit' ? 'private-wrong-id' : normal] : [],
      callFrames: [{ location: { scriptId: `private-script-${index}`, lineNumber: 0, columnNumber: 0 } }],
    });
  };
  const execute = () => {
    if (index < scripts.length) vm.runInContext(scripts[index++], context);
    if (index < scripts.length && instrumentation) pause();
    else while (index < scripts.length) vm.runInContext(scripts[index++], context);
  };
  capture = new BootstrapHandoffCapture({
    boundIdentity,
    send: async (method, params, purpose) => {
      if (method === 'Page.getFrameTree') return { frameId: 'private-root' };
      if (method === 'Debugger.setInstrumentationBreakpoint') return { breakpointId: 'private-instrumentation' };
      if (method === 'Page.reload') {
        observe({ type: 'frame_navigated', frameId: 'private-root', isHome: true });
        observe({ type: 'context_created', frameId: 'private-root', contextId: 1, isDefault: true });
        pause();
      }
      if (method === 'Debugger.setBreakpoint') {
        assert.equal(phase, 'instrumentation');
        assert.equal(capture.canReadCallFrame('private-call'), false);
        assert.equal(capture.handoff.canInstall(params.location), true);
        normal = `private-normal-${index}`;
        capture.handoff.created(normal);
        return { breakpointId: normal, actualLocation: { ...params.location } };
      }
      if (method === 'Debugger.removeBreakpoint') {
        if (purpose === 'normal_remove') {
          if (fault === 'cleanup') throw new Error('PRIVATE-ERROR');
          normal = null;
        } else instrumentation = false;
      }
      if (method === 'Debugger.resume') {
        observe({ type: 'resumed' });
        if (normal && phase === 'instrumentation') {
          if (fault === 'stale') observe({ type: 'context_destroyed', contextId: 1 });
          pause(true);
        } else execute();
      }
      if (method === 'Debugger.evaluateOnCallFrame') {
        assert.equal(phase, 'normal');
        assert.equal(capture.canReadCallFrame(params.callFrameId), true);
        assert.equal(params.expression, boundIdentity ? QA_BOUND_BOOTSTRAP_EXPRESSION : BOOTSTRAP_CURRENT_SELF_EXPRESSION);
        assert.equal(params.throwOnSideEffect, true);
        assert.equal(params.timeout, 1500);
        assert.equal(params.returnByValue, true);
        assert.equal(params.silent, true);
        assert.equal(params.includeCommandLineAPI, false);
        assert.equal(params.generatePreview, false);
        evaluations += 1;
        const raw = vm.runInContext(params.expression, context);
        // CDP JSON dispatch removes VM prototypes before either projection.
        const projected = JSON.parse(JSON.stringify(raw));
        const dto = boundIdentity ? projectBoundBootstrap(projected, id => {
          assert.ok(id === null || id === '123456');
        }) : validateBootstrapCurrentSelf(projected);
        if (fault === 'replay') pause(true);
        return dto;
      }
      return { acknowledged: true };
    },
  });
  return { capture, evaluations: () => evaluations };
}

test('native handoff adapter reads only verified normal hits with the fixed guarded projector', async () => {
  const { capture, evaluations } = fixture();
  const result = await capture.run();
  assert.equal(result.stopReason, 'captured');
  assert.equal(result.snapshot.stableIdStringCandidates, 1);
  assert.equal(result.cleanupVerified, true);
  assert.equal(evaluations(), 2);
  const diagnostics = capture.diagnostics();
  assert.equal(diagnostics.handoff.installed, 2);
  assert.equal(diagnostics.handoff.matched, 2);
  assert.equal(diagnostics.handoff.removed, 2);
  assert.equal(diagnostics.handoff.normalCleanupVerified, true);
  await assert.rejects(capture.run(), /early_bootstrap_capture_replay_denied/u);
  for (const value of ['private-root', 'private-script', 'private-call', 'private-normal', '123456']) assert.equal(JSON.stringify([result, diagnostics]).includes(value), false);
});

test('bound private ID projection retains the same verified normal-hit and side-effect guards', async () => {
  const { capture } = fixture(undefined, true);
  const result = await capture.run();
  assert.equal(result.stopReason, 'captured'); assert.equal(result.cleanupVerified, true);
  assert.equal(result.snapshot.entityChecks.normalizedUsers, 'match');
  assert.equal(JSON.stringify(result).includes('123456'), false);
});

for (const fault of ['wrong_hit', 'stale', 'replay', 'cleanup']) {
  test(`native handoff adapter rejects ${fault} and preserves cleanup evidence`, async () => {
    const { capture, evaluations } = fixture(fault);
    const result = await capture.run();
    const diagnostics = capture.diagnostics();
    assert.notEqual(result.stopReason, 'captured');
    assert.equal(result.snapshot?.stableIdStringCandidates ?? 0, 0);
    if (fault === 'wrong_hit') { assert.equal(diagnostics.handoff.unexpected, 1); assert.equal(evaluations(), 0); }
    if (fault === 'stale') { assert.equal(diagnostics.handoff.stale, 1); assert.equal(evaluations(), 0); }
    if (fault === 'replay') assert.equal(diagnostics.handoff.replayed, 1);
    assert.equal(result.cleanupVerified, fault !== 'cleanup');
    assert.equal(diagnostics.handoff.normalCleanupVerified, fault !== 'cleanup');
    assert.equal(JSON.stringify([result, diagnostics]).includes('PRIVATE-ERROR'), false);
  });
}

test('normal breakpoint cap reports partial coverage without calling it a bad hit or evaluating', async () => {
  const records = [];
  const handoff = new BootstrapBreakpointHandoff({ record: (...args) => records.push(args), capture: { canReadCallFrame: () => true }, send: () => assert.fail('no ninth installation') });
  handoff.root = 'root';
  handoff.observe({ type: 'frame_navigated', frameId: 'root', isHome: true });
  handoff.observe({ type: 'context_created', frameId: 'root', contextId: 1, isDefault: true });
  handoff.observe({ type: 'script_parsed', frameId: 'root', contextId: 1, scriptId: 'script', isDefault: true });
  handoff.observe({ type: 'paused', reason: 'instrumentation', callFrameId: 'call', scriptId: 'script' }, { callFrames: [{ location: { scriptId: 'script', lineNumber: 0, columnNumber: 0 } }] });
  handoff.stats.installAttempts = 8;
  assert.equal(await handoff.beforeRead(1, 'call'), false);
  assert.equal(await handoff.beforeRead(1, 'call'), false);
  assert.equal(handoff.snapshot().outcome, 'normal_breakpoint_limit');
  assert.equal(handoff.snapshot().mismatched, 0);
  assert.equal(handoff.snapshot().normalCleanupVerified, true);
  assert.equal(records.filter(row => row[1] === 'handoffLimitReached').length, 1);
});
