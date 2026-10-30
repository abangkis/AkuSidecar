import test from 'node:test';
import assert from 'node:assert/strict';
import {createNativeSourceAttempt, nativeSourceDiagnostic, settleNativeCleanup} from './native-source-kernel.mjs';
import {cleanupDailyBatch} from '../../../../../scripts/auth-session-fixture/x-daily-batch-cleanup.mjs';

test('cleanup executor preserves order and continues after release and diagnostic failures', async () => {
  const calls = [];
  const results = await settleNativeCleanup([
    ['collector', () => { calls.push('collector'); throw Error('PRIVATE_TOKEN'); }],
    ['absent', undefined],
    ['attachment', async () => { calls.push('attachment'); throw Error('PRIVATE_BODY'); }],
    ['target', async () => { calls.push('target'); }],
  ], () => { throw Error('diagnostic failure'); });
  assert.deepEqual(calls, ['collector', 'attachment', 'target']);
  assert.deepEqual(results, [
    {stage: 'collector', outcome: 'uncertain'}, {stage: 'absent', outcome: 'not_needed'},
    {stage: 'attachment', outcome: 'uncertain'}, {stage: 'target', outcome: 'verified'},
  ]);
  assert.equal(JSON.stringify(results).includes('PRIVATE'), false);
  assert(Object.isFrozen(results));
});

// Freeze the legacy X receipt across every combination of independent failures.
// This guards the existing Go wire consumer while X adopts the shared executor.
test('X cleanup receipt and release order remain compatible for all failure combinations', async () => {
  const stages = ['collector', 'enrollment', 'attachments', 'bootstrap'];
  for (let mask = 0; mask < 16; mask++) {
    const calls = [];
    const releases = Object.fromEntries(stages.map((stage, index) => [stage, async () => {
      calls.push(stage); if (mask & (1 << index)) throw Error('PRIVATE');
    }]));
    const receipt = await cleanupDailyBatch(releases);
    assert.deepEqual(calls, stages);
    assert.deepEqual(receipt, {
      closed: mask === 0,
      cleanup: {readerControls: 'not_needed', collectorControls: 'not_needed',
        ...Object.fromEntries(stages.map((stage, index) => [stage, mask & (1 << index) ? 'uncertain' : 'verified']))},
      ...(mask & 8 ? {bootstrapDiagnostic: {stage: 'unknown', condition: 'unavailable'}} : {}),
    });
  }
});

for (const source of ['x', 'instagram']) {
  const options = () => ({source, generation: 1, nonce: 'a'.repeat(48), profileScope: 'synthetic-profile', deadlineMS: Date.now() + 60000});
  test(`${source}: cleanup preserves first cause, reverse ownership order and idempotency`, async () => {
    const attempt = createNativeSourceAttempt(options());
    const calls = [];
    await attempt.own('collector', async () => 'process', async () => calls.push('process'));
    await attempt.own('page', async () => 'page', async () => { calls.push('page'); throw Error('PRIVATE'); });
    const original = attempt.fail('evidence', 'unavailable');
    const first = attempt.close();
    assert.equal(attempt.close(), first);
    const receipt = await first;
    assert.deepEqual(calls, ['page', 'process']);
    assert.equal(receipt.cleanup, 'uncertain');
    assert.deepEqual(nativeSourceDiagnostic(original), {stage: 'evidence', reason: 'unavailable', cleanup: 'uncertain'});
    await assert.rejects(attempt.run('capture', async () => 'forbidden'), error => error === original);
  });
  test(`${source}: late creation remains owned after close and cannot return to capture`, async () => {
    const attempt = createNativeSourceAttempt({...options(), cleanupMS: 10});
    let finish, started;
    const ready = new Promise(resolve => { started = resolve; });
    let releases = 0;
    const pending = attempt.own('collector', () => { started(); return new Promise(resolve => { finish = resolve; }); }, async () => { releases++; });
    const rejected = assert.rejects(pending);
    await ready;
    const receipt = await attempt.close();
    await rejected;
    assert.equal(receipt.cleanup, 'uncertain');
    finish('late-resource');
    await new Promise(resolve => setImmediate(resolve));
    assert.equal(releases, 1);
    assert.equal(attempt.status().state, 'closed');
    assert.equal(await attempt.close(), receipt);
  });
}
