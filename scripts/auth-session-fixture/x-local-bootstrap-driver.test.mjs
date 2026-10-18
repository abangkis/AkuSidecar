import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { BOOTSTRAP_CURRENT_SELF_EXPRESSION } from './x-bootstrap-current-self.mjs';
import { LocalBootstrapTransport, localBootstrapChromeArgs, localBootstrapDocuments,
  localBootstrapExpression, localBootstrapOrigin, localBootstrapCase, runLocalBootstrapCapture } from './x-local-bootstrap-driver.mjs';

const ORIGIN = 'http://127.0.0.1:43123';
const PRIVATE = 'PRIVATE-LOCAL-BOOTSTRAP-MARKER';

function fakeChrome(kind, { stall, nestedPause = false, initialFramePending = false, handoffFault } = {}) {
  const context = vm.createContext({ location: { origin: ORIGIN, pathname: '/home' } });
  const documents = localBootstrapDocuments(kind);
  const ordinary = kind === 'ordinary_full';
  const scripts = [...documents.home.matchAll(/<script(?: src="([^"]+)")?>([\s\S]*?)<\/script>/gu)].map(match =>
    match[1] === '/middle.js' ? documents.middle : match[1] === '/late.js' ? documents.late : match[2]);
  let index = 0;
  let breakpoint = true;
  let normalBreakpoint = null;
  let pausedKind = 'instrumentation';
  const commands = [];
  let transport;
  const frame = () => event('Debugger.scriptParsed', { scriptId: `private-script-${index}`, executionContextId: 701,
    executionContextAuxData: { isDefault: true, frameId: 'private-root' }, url: PRIVATE, hash: PRIVATE });
  const event = (method, params) => transport.push(Buffer.from(`${JSON.stringify({ method, params, sessionId: 'private-session' })}\0`));
  const paused = (normal = false) => {
    frame();
    pausedKind = normal ? 'normal' : 'instrumentation';
    event('Debugger.paused', { reason: normal || ordinary ? 'other' : 'instrumentation',
      hitBreakpoints: normal ? [handoffFault === 'unexpected_hit' ? 'private-unexpected-breakpoint' : normalBreakpoint.id] : [],
      callFrames: [{ callFrameId: `private-call-${index}`,
      location: { scriptId: `private-script-${index}`, lineNumber: 0, columnNumber: normal && handoffFault === 'mismatched_hit' ? 1 : 0 }, scopeChain: [{ object: { value: PRIVATE } }] }] });
  };
  const execute = () => {
    if (index < scripts.length) vm.runInContext(scripts[index++], context);
    if (index < scripts.length && breakpoint && !ordinary) paused();
    else {
      while (index < scripts.length) vm.runInContext(scripts[index++], context);
      event('Page.loadEventFired', {});
    }
  };
  transport = new LocalBootstrapTransport({ origin: ORIGIN, kind, timeoutMs: 30,
    write: (bytes, callback) => {
      const message = JSON.parse(bytes.subarray(0, -1).toString());
      commands.push({ ...message, observedPauseKind: pausedKind });
      callback();
      queueMicrotask(() => {
        const reply = result => transport.push(Buffer.from(`${JSON.stringify({ id: message.id, result,
          ...(message.sessionId ? { sessionId: message.sessionId } : {}) })}\0`));
        if (message.method === stall) {
          if (nestedPause) paused();
          return;
        }
        if (message.method === 'Page.getFrameTree') {
          reply({ frameTree: { frame: { id: 'private-root', url: initialFramePending ? 'about:blank' : `${ORIGIN}/home` } } });
          if (initialFramePending) queueMicrotask(() => {
            initialFramePending = false;
            event('Page.frameNavigated', { frame: { id: 'private-root', url: `${ORIGIN}/home` } });
          });
          return;
        }
        if (message.method === 'Debugger.setInstrumentationBreakpoint') { reply({ breakpointId: 'private-breakpoint' }); return; }
        if (message.method === 'Debugger.setBreakpoint') {
          normalBreakpoint = { id: `private-normal-${index}`, location: message.params.location };
          reply({ breakpointId: normalBreakpoint.id, actualLocation: { ...message.params.location } });
          return;
        }
        if (message.method === 'Debugger.removeBreakpoint') {
          if (message.params.breakpointId === 'private-breakpoint') breakpoint = false;
          else if (handoffFault === 'cleanup_failure') {
            transport.push(Buffer.from(`${JSON.stringify({ id: message.id, sessionId: message.sessionId, error: { code: -32000, message: PRIVATE } })}\0`));
            return;
          } else normalBreakpoint = null;
        }
        if (message.method === 'Page.reload') {
          event('Runtime.executionContextsCleared', {});
          event('Page.frameNavigated', { frame: { id: 'private-root', url: `${ORIGIN}/home` } });
          event('Runtime.executionContextCreated', { context: { id: 701, auxData: { isDefault: true, frameId: 'private-root' } } });
          paused();
        }
        if (message.method === 'Debugger.resume') {
          event('Debugger.resumed', {});
          if (normalBreakpoint && pausedKind === 'instrumentation') {
            if (handoffFault === 'stale_context') {
              event('Runtime.executionContextDestroyed', { executionContextId: 701 });
              event('Runtime.executionContextCreated', { context: { id: 702, auxData: { isDefault: true, frameId: 'private-root' } } });
            }
            paused(true);
          } else execute();
        }
        if (message.method === 'Debugger.evaluateOnCallFrame' || message.method === 'Runtime.evaluate') {
          const value = vm.runInContext(message.params.expression, context);
          if (handoffFault === 'replay' && message.method === 'Debugger.evaluateOnCallFrame') paused(true);
          reply({ result: { type: typeof value, value } });
          return;
        }
        reply({});
      });
    },
  });
  transport.sessionId = 'private-session';
  return { transport, commands };
}

test('local adapter changes exactly the origin comparisons and keeps a real local location check', () => {
  const expression = localBootstrapExpression(ORIGIN);
  assert.equal(expression, BOOTSTRAP_CURRENT_SELF_EXPRESSION.replaceAll("'https://x.com'", JSON.stringify(ORIGIN)));
  const good = vm.runInNewContext(expression, { location: { origin: ORIGIN, pathname: '/home' } });
  assert.equal(good.diagnostic, 'bootstrap_missing');
  const wrong = vm.runInNewContext(expression, { location: { origin: 'https://x.com', pathname: '/home' } });
  assert.equal(wrong.diagnostic, 'wrong_page');
  for (const origin of ['https://127.0.0.1:43123', 'http://localhost:43123', `${ORIGIN}/home`, `${ORIGIN}?a=b`, 'https://x.com']) {
    assert.throws(() => localBootstrapOrigin(origin));
  }
});

test('fresh-profile Chrome flags bind only loopback and expose no debug socket or profile reuse mode', () => {
  const flags = localBootstrapChromeArgs('C:\\project\\runtime\\qa\\fresh\\shared', ORIGIN);
  assert.ok(flags.includes('--remote-debugging-pipe'));
  assert.ok(flags.includes('--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE 127.0.0.1'));
  assert.equal(flags.at(-1), `${ORIGIN}/home`);
  assert.equal(flags.some(flag => /remote-debugging-port|disable-web-security|ignore-certificate-errors|x\.com/u.test(flag)), false);
});

test('full projector captures transient inline bootstrap before local external script and observes late absence', async () => {
  const { transport, commands } = fakeChrome('projection');
  const result = await runLocalBootstrapCapture(transport);
  assert.equal(result.early.stopReason, 'captured');
  assert.equal(result.early.cleanupVerified, true);
  assert.equal(result.early.snapshot.stableIdStringCandidates, 1);
  assert.equal(result.late.diagnostic, 'bootstrap_missing');
  assert.equal(result.firstFailure, null);
  assert.equal(result.constantReads, 0);
  assert.ok(commands.some(message => message.method === 'Debugger.evaluateOnCallFrame'));
  const serialized = JSON.stringify(result);
  for (const value of [PRIVATE, 'private-session', 'private-root', 'private-script', 'private-call', '123456', ORIGIN]) assert.equal(serialized.includes(value), false);
});

test('target URL before frame commit waits for root Home navigation and rechecks the committed frame before capture', async () => {
  const { transport } = fakeChrome('projection', { initialFramePending: true });
  const result = await runLocalBootstrapCapture(transport);
  assert.equal(result.early.stopReason, 'captured');
  assert.equal(result.firstFailure, null);
  assert.equal(result.early.cleanupVerified, true);
  const entries = result.chronology;
  assert.equal(entries.filter(entry => entry.label === 'initialFrame' && entry.status === 'acknowledged').length, 2);
  assert.ok(entries.findIndex(entry => entry.label === 'initialHomePending') < entries.findIndex(entry => entry.label === 'rootHome'));
  assert.ok(entries.findIndex(entry => entry.label === 'initialHomeReady') < entries.findIndex(entry => entry.label === 'frameTree' && entry.status === 'sent'));
  assert.equal(entries.some(entry => entry.label === 'frameTree' && entry.status === 'error'), false);
});

test('tiny fixed control reads through every local script with the same pause and cleanup state machine', async () => {
  const { transport, commands } = fakeChrome('constant');
  const result = await runLocalBootstrapCapture(transport);
  assert.equal(result.constantReads, 4);
  assert.equal(result.early.stopReason, 'deadline');
  assert.equal(result.early.cleanupVerified, true);
  assert.equal(result.early.snapshot.diagnostic, 'bootstrap_missing');
  assert.equal(result.late.diagnostic, 'bootstrap_missing');
  assert.equal(result.firstFailure, null);
  assert.ok(commands.filter(message => message.method === 'Debugger.evaluateOnCallFrame').every(message => message.params.expression === 'true'));
});

test('chronology identifies the first paused-read timeout even with nested pause and failed cleanup', async () => {
  const { transport } = fakeChrome('projection', { stall: 'Debugger.evaluateOnCallFrame', nestedPause: true });
  const result = await runLocalBootstrapCapture(transport);
  assert.equal(result.early.stopReason, 'evaluation_failed');
  assert.equal(result.early.cleanupVerified, false);
  assert.equal(result.late, null);
  assert.deepEqual(result.firstFailure, { sequence: result.firstFailure.sequence, kind: 'command', label: 'pausedRead', status: 'timeout', phase: 'paused_read' });
  assert.equal(result.chronology.some(entry => entry.label === 'breakpointRemove' && entry.status === 'rejected'), true);
  assert.equal(result.chronology.filter(entry => entry.label === 'instrumentationPause').length, 2);
  assert.equal(JSON.stringify(result).includes(PRIVATE), false);
});

test('transport rejects broader reads and refuses a frame from outside the active paused capture', async () => {
  const { transport, commands } = fakeChrome('projection');
  await assert.rejects(transport.send('Network.getResponseBody', { requestId: 'private' }));
  await assert.rejects(transport.send('Debugger.getScriptSource', { scriptId: 'private' }));
  await assert.rejects(transport.send('Runtime.evaluate', { expression: 'true' }));
  await assert.rejects(transport.send('Debugger.evaluateOnCallFrame', { callFrameId: 'private' }));
  assert.equal(commands.length, 0);
  assert.equal(transport.firstFailure, null);
});

for (const kind of ['instrumentation_resume_only', 'instrumentation_minimal', 'ordinary_full', 'instrumentation_runtime']) {
  test(`closed mechanism variant ${kind} keeps actual pause cause, evaluation and cleanup separate`, async () => {
    const { transport, commands } = fakeChrome(kind);
    const result = await runLocalBootstrapCapture(transport);
    assert.equal(result.schema, 'aku.x.local-bootstrap-repro.v3');
    assert.equal(result.firstFailure, null);
    assert.equal(result.early.cleanupVerified, true);
    assert.equal(result.mechanism.lateConstantResult, true);
    assert.equal(result.late, null);
    const reads = commands.filter(message => message.method === 'Debugger.evaluateOnCallFrame' ||
      message.method === 'Runtime.evaluate' && Object.hasOwn(message.params, 'contextId'));
    assert.equal(result.mechanism.pausedEvaluationAttempts, reads.length);
    assert.ok(reads.every(message => message.params.expression === 'true'));
    assert.equal(result.mechanism.actualOtherPauses, kind === 'ordinary_full' ? 1 : 0);
    assert.equal(result.mechanism.actualInstrumentationPauses, kind === 'ordinary_full' ? 0 : 4);
    if (kind === 'instrumentation_resume_only') {
      assert.equal(reads.length, 0);
      assert.equal(result.constantReads, 0);
      assert.equal(result.early.readAttempts, 4);
    } else assert.equal(result.constantReads, kind === 'ordinary_full' ? 1 : 4);
    if (kind === 'instrumentation_minimal') {
      assert.ok(reads.every(message => Object.keys(message.params).sort().join(',') === 'callFrameId,expression,returnByValue'));
    }
    if (kind === 'ordinary_full') {
      assert.equal(commands.some(message => message.method === 'Debugger.setInstrumentationBreakpoint'), false);
      assert.equal(commands.some(message => message.method === 'Debugger.removeBreakpoint'), false);
      assert.equal(result.mechanism.pauseAdapter, 'ordinary_other_as_capture_eligible');
      assert.ok(result.chronology.some(entry => entry.label === 'otherPause'));
      assert.equal(result.chronology.some(entry => entry.label === 'instrumentationPause'), false);
      assert.ok(reads.every(message => message.params.throwOnSideEffect === true && message.params.timeout === 1500));
    }
    if (kind === 'instrumentation_runtime') assert.ok(reads.every(message => message.method === 'Runtime.evaluate' && message.params.disableBreaks === true));
    for (const secret of [PRIVATE, ORIGIN, 'private-call', 'private-root', '123456']) assert.equal(JSON.stringify(result).includes(secret), false);
  });
}

test('mechanism case inputs stay closed and production projection guards are retained', async () => {
  assert.throws(() => localBootstrapCase('arbitrary_eval'));
  const { transport, commands } = fakeChrome('instrumentation_minimal');
  transport.capture = { canReadCallFrame: () => true };
  await assert.rejects(transport.send('Debugger.evaluateOnCallFrame', { callFrameId: 'private', expression: 'globalThis', returnByValue: true }));
  await assert.rejects(transport.send('Debugger.evaluateOnCallFrame', { callFrameId: 'private', expression: 'true', returnByValue: true, throwOnSideEffect: false }));
  assert.equal(commands.length, 0);
  const { transport: projector } = fakeChrome('projection');
  projector.capture = { canReadCallFrame: () => true };
  assert.equal(projector.validCommand('Debugger.evaluateOnCallFrame', { callFrameId: 'private', expression: projector.expression, returnByValue: true }), false);
});

for (const kind of ['handoff_constant', 'handoff_projection']) {
  test(`${kind} installs only during instrumentation and evaluates only the verified normal breakpoint`, async () => {
    const { transport, commands } = fakeChrome(kind);
    const result = await runLocalBootstrapCapture(transport);
    assert.equal(result.firstFailure, null);
    assert.equal(result.early.cleanupVerified, true);
    assert.equal(result.mechanism.handoff.normalCleanupVerified, true);
    assert.equal(result.mechanism.handoff.installed, result.mechanism.handoff.removed);
    assert.equal(result.mechanism.handoff.installed, result.mechanism.handoff.matched);
    assert.ok(result.mechanism.handoff.matched > 0);
    assert.equal(result.mechanism.handoff.unexpected + result.mechanism.handoff.mismatched + result.mechanism.handoff.stale + result.mechanism.handoff.replayed, 0);
    assert.equal(result.late.diagnostic, 'bootstrap_missing');
    assert.equal(result.mechanism.pauseAdapter, 'verified_normal_breakpoint_as_capture_eligible');
    assert.ok(result.chronology.some(entry => entry.label === 'lateFrame' && entry.status === 'acknowledged'));
    const reads = commands.filter(message => message.method === 'Debugger.evaluateOnCallFrame');
    assert.ok(reads.length > 0);
    assert.ok(reads.every(message => message.observedPauseKind === 'normal' && message.params.returnByValue === true &&
      message.params.throwOnSideEffect === true && message.params.timeout === 1500 && message.params.silent === true &&
      message.params.includeCommandLineAPI === false && message.params.generatePreview === false));
    assert.ok(commands.filter(message => message.method === 'Debugger.setBreakpoint').every(message => message.observedPauseKind === 'instrumentation'));
    if (kind === 'handoff_projection') {
      assert.equal(result.early.stopReason, 'captured');
      assert.equal(result.early.snapshot.diagnostic, 'projected');
      assert.equal(result.early.snapshot.bootstrapState, 'object');
      assert.equal(result.early.snapshot.sessionState, 'object');
      assert.deepEqual(result.early.snapshot.idFields, { user_id: 'string', user_id_str: 'string' });
      assert.equal(result.early.snapshot.stableIdStringCandidates, 2);
      assert.equal(result.early.snapshot.sessionIdsAgree, true);
      assert.deepEqual(result.early.snapshot.entityChecks, { normalizedUsers: 'match', rootUsers: 'match' });
      assert.ok(Object.values(result.early.snapshot.authFields).every(value => value === 'absent'));
      assert.ok(reads.every(message => message.params.expression === localBootstrapExpression(ORIGIN)));
    } else assert.equal(result.constantReads, result.mechanism.handoff.matched);
    assert.equal(localBootstrapDocuments(kind).home.includes('debugger;'), false);
    for (const secret of [PRIVATE, ORIGIN, 'private-call', 'private-root', 'private-normal', '123456']) assert.equal(JSON.stringify(result).includes(secret), false);
  });
}

for (const fault of ['unexpected_hit', 'mismatched_hit', 'stale_context', 'replay', 'cleanup_failure']) {
  test(`handoff fails closed on ${fault} and preserves separate normal cleanup evidence`, async () => {
    const { transport } = fakeChrome('handoff_projection', { handoffFault: fault });
    const result = await runLocalBootstrapCapture(transport);
    assert.notEqual(result.early.stopReason, 'captured');
    assert.equal(result.early.snapshot?.stableIdStringCandidates ?? 0, 0);
    assert.equal(result.late, null);
    assert.notEqual(result.firstFailure, null);
    if (fault === 'unexpected_hit') {
      assert.equal(result.mechanism.handoff.unexpected, 1);
      assert.equal(result.mechanism.pausedEvaluationAttempts, 0);
    }
    if (fault === 'mismatched_hit') {
      assert.equal(result.mechanism.handoff.mismatched, 1);
      assert.equal(result.mechanism.pausedEvaluationAttempts, 0);
    }
    if (fault === 'stale_context') {
      assert.equal(result.mechanism.handoff.stale, 1);
      assert.equal(result.mechanism.pausedEvaluationAttempts, 0);
    }
    if (fault === 'replay') assert.equal(result.mechanism.handoff.replayed, 1);
    if (fault === 'cleanup_failure') {
      assert.equal(result.mechanism.handoff.normalCleanupVerified, false);
      assert.equal(result.early.cleanupVerified, false);
      assert.equal(result.firstFailure.label, 'normalBreakpointRemove');
    } else assert.equal(result.mechanism.handoff.normalCleanupVerified, true);
    for (const secret of [PRIVATE, 'private-normal', 'private-script', '123456']) assert.equal(JSON.stringify(result).includes(secret), false);
  });
}

test('bounded chronology retains the first failure without arbitrary status, phase or error strings', () => {
  const { transport } = fakeChrome('constant');
  for (let i = 0; i < 600; i += 1) transport.record('event', 'scriptParsed', 'observed');
  transport.terminate('pausedRead', 'timeout');
  assert.equal(transport.chronology.length, 512);
  assert.equal(transport.chronologyTruncated, true);
  assert.equal(transport.firstFailure.label, 'pausedRead');
  assert.equal(transport.firstFailure.status, 'timeout');
  assert.throws(() => transport.record('event', PRIVATE, 'observed'));
  assert.throws(() => transport.record('event', 'protocol', PRIVATE));
});
