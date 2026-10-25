import test from 'node:test';
import assert from 'node:assert/strict';
import { QACheckpointRendezvous, synchronizedQAReport } from './x-qa-checkpoint-rendezvous.mjs';
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';

test('both distinct roles must arrive before either private paused checkpoint continues', async () => {
  const r = new QACheckpointRendezvous(); let resumed = false;
  const reader = r.arrive('reader').then(() => { resumed = true; });
  await Promise.resolve(); assert.equal(resumed, false); assert.equal(r.snapshot().state, 'waiting');
  await assert.rejects(r.arrive('reader'), /qa_rendezvous_closed/);
  await Promise.all([reader, r.arrive('collector')]); assert.equal(resumed, true);
  assert.deepEqual(r.snapshot(), { schema: 'aku.x.qa-checkpoint-rendezvous.v1', state: 'released', arrivals: 2, waitLimitMS: 1000, canAuthorize: false });
  await assert.rejects(r.arrive('collector'), /qa_rendezvous_closed/);
});
test('manual startup can use its centralized rendezvous window while the historical default stays fixed', async () => {
  const r = new QACheckpointRendezvous(MANUAL_TIMING.rendezvousMS);
  const pending = r.arrive('reader');
  assert.equal(r.snapshot().waitLimitMS, MANUAL_TIMING.rendezvousMS);
  r.cancel();
  await assert.rejects(pending, /qa_rendezvous_closed/);
  assert.equal(new QACheckpointRendezvous().snapshot().waitLimitMS, 1000);
});
test('one missing role times out with fixed diagnostics and cannot join later', async () => {
  const r = new QACheckpointRendezvous();
  await assert.rejects(r.arrive('reader'), /qa_rendezvous_closed/);
  assert.equal(r.snapshot().state, 'timed_out'); assert.equal(r.snapshot().arrivals, 1);
  await assert.rejects(r.arrive('collector'), /qa_rendezvous_closed/);
});
test('cleanup cancels a pending rendezvous; wrapper separates account match from overlap', async () => {
  const r = new QACheckpointRendezvous(); const pending = r.arrive('collector');
  const result = synchronizedQAReport({ completedCheckpoints: 2, outcome: 'reader_checkpoint_rejected' }, r, 'not_checked');
  await assert.rejects(pending, /qa_rendezvous_closed/);
  assert.equal(result.rendezvous.state, 'aborted'); assert.equal(result.accountComparison, 'unknown');
  assert.equal(result.canAuthorize, false); assert.equal(result.readerWindow, 'not_checked');
  const released = new QACheckpointRendezvous();
  await Promise.all([released.arrive('reader'), released.arrive('collector')]);
  assert.equal(synchronizedQAReport({ completedCheckpoints: 4, outcome: 'overlap_not_observed' }, released, 'preserved').accountComparison, 'match');
  const empty = new QACheckpointRendezvous();
  await assert.rejects(empty.arrive('PRIVATE'), /qa_rendezvous_closed/);
  assert.equal(synchronizedQAReport({ completedCheckpoints: 0 }, empty, 'not_checked').rendezvous.state, 'not_started');
});
