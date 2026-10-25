import test from 'node:test';
import assert from 'node:assert/strict';
import { createManualStartupCapture, MANUAL_STARTUP_SCHEMA } from './x-manual-startup-source.mjs';
import { offlineStartupCapture, offlineStartupOwner, runOfflineStartupRecipe } from './x-manual-startup-offline-fixture.mjs';
import { startupDiagnostic } from './x-manual-startup-diagnostic.mjs';

const request = (role = 'reader') => ({ challenge: '0123456789abcdef'.repeat(3), role, ...offlineStartupOwner(role) });
const decode = wire => { try { return JSON.parse(wire.toString('utf8')); } finally { wire.fill(0); } };

test('startup may exceed two seconds only while samples remain fresh at publication', async () => {
  for (const age of [1999, 2000]) {
    let clock = 0;
    const capture = createManualStartupCapture({now: () => clock, wallNow: () => 10000,
      run: async (input, sink) => {
        clock = 3000;
        await runOfflineStartupRecipe(input, sink, {now: () => clock});
        clock += age;
      }});
    if (age === 1999) {
      const result = decode(await capture(request()));
      assert.equal(result.elapsedMS, 4999);
      assert.equal(result.identityObservedAtMS, 13000);
      assert.equal(result.settingsObservedAtMS, 13000);
    } else {
      await assert.rejects(capture(request()), error => startupDiagnostic(error)?.reason === 'stale_observation');
    }
  }
});

test('fresh samples at the exact six-second startup limit are still rejected', async () => {
  let clock = 0;
  const capture = createManualStartupCapture({now: () => clock, wallNow: () => 10000,
    run: async (input, sink) => {
      clock = 5999;
      await runOfflineStartupRecipe(input, sink, {now: () => clock});
      clock = 6000;
    }});
  await assert.rejects(capture(request()), error => startupDiagnostic(error)?.reason === 'deadline');
});

test('actual fixed bootstrap/body recipe supplies both private role receipts without changing QA reports', async () => {
  for (const role of ['reader', 'collector']) {
    const capture = offlineStartupCapture(), input = request(role);
    const result = decode(await capture(input));
    assert.equal(result.schema, MANUAL_STARTUP_SCHEMA);
    assert.equal(result.accountId, '1908123456789012345');
    assert.equal(result.contextId, input.contextId); assert.equal(result.documentId, input.documentId);
    assert.equal(result.settingsRequestId, 'offline-settings-' + role);
    assert.equal(result.qualification, 'api_v1.cookie-auth-rejection.2026-10-20');
    assert.equal(Object.keys(result).length, 16);
    assert.ok(result.identityObservedAtMS <= result.settingsObservedAtMS);
    assert.ok(result.elapsedMS < 6000);
    await assert.rejects(capture(input), /manual_startup_unavailable/);
    let privateSample;
    const report = await runOfflineStartupRecipe(input, { onPrivateStartup: s => { privateSample = s; } });
    assert.ok(privateSample); assert.equal(report.diagnostic, 'bound');
    assert.equal(JSON.stringify(report).includes(result.accountId), false);
    assert.equal(report.canAuthorize, false); assert.equal(report.sessionTransferred, false);
  }
});

test('every actual scope pin is compared rather than reflected from the request', async () => {
  for (const field of ['ownerId', 'profileId', 'contextId', 'targetId', 'frameId', 'documentId']) {
    await assert.rejects(offlineStartupCapture()({ ...request(), [field]: 'wrong-' + field }), /manual_startup_unavailable/);
  }
});

test('cache, scope loss and observed writes never publish a manual startup receipt', async () => {
  for (const change of [
    ({ emit, requestId }) => emit('Network.requestServedFromCache', { requestId }),
    ({ emit }) => emit('Runtime.executionContextsCleared', {}),
    ({ emit }) => emit('Network.responseReceivedExtraInfo', { headers: { 'set-cookie': 'invented-private-secret' } }),
  ]) await assert.rejects(offlineStartupCapture({ change })(request()), /manual_startup_unavailable/);
});

test('old QA summaries and returned boolean assertions cannot replace the private sink', async () => {
  for (const result of [{ diagnostic: 'bound', identity: 'current_self_observed' }, { success: true }, true]) {
    await assert.rejects(createManualStartupCapture({ run: async () => result })(request()), /manual_startup_unavailable/);
  }
});

test('deadline, rollback and cancellation fence late callback completion', async () => {
  for (const scenario of ['deadline', 'rollback', 'cancel']) {
    let time = 10;
    const abort = new AbortController();
    const capture = createManualStartupCapture({ now: () => time, wallNow: () => 10000,
      run: async (input, sink) => {
        await runOfflineStartupRecipe(input, sink, { now: () => time });
        if (scenario === 'deadline') time += 6000;
        if (scenario === 'rollback') time--;
        if (scenario === 'cancel') abort.abort();
      } });
    await assert.rejects(capture(request(), abort.signal), /manual_startup_unavailable/);
  }
});

test('duplicate callback, extra fields and accessor inputs fail without exposing error text', async () => {
  const double = createManualStartupCapture({ run: async (input, sink) => {
    const deliver = sink.onPrivateStartup;
    await runOfflineStartupRecipe(input, { onPrivateStartup: sample => {
      deliver(sample); try { deliver(sample); } catch { /* Runner cannot swallow duplication. */ }
    } });
  } });
  await assert.rejects(double(request()), /manual_startup_unavailable/);
  await assert.rejects(offlineStartupCapture()({ ...request(), unexpected: 'secret' }), /manual_startup_unavailable/);
  let reads = 0;
  const accessor = request(); Object.defineProperty(accessor, 'ownerId', { enumerable: true, get() { reads++; return 'secret'; } });
  await assert.rejects(offlineStartupCapture()(accessor), /manual_startup_unavailable/);
  assert.equal(reads, 0);
  const throwing = createManualStartupCapture({ run: () => { throw new Error('private-error-secret'); } });
  await assert.rejects(throwing(request()), e => e.message === 'manual_startup_unavailable');
});
