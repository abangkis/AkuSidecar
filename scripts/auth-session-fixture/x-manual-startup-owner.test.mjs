import test from 'node:test';
import assert from 'node:assert/strict';
import { createManualStartupOwner } from './x-manual-startup-owner.mjs';
import { offlineStartupOwnerActor, offlineStartupOwner, runOfflineStartupRecipe } from './x-manual-startup-offline-fixture.mjs';

const request = (role, pins) => ({ challenge: 'b'.repeat(48), role, ...pins });
const decode = buffer => { try { return JSON.parse(buffer.toString('utf8')); } finally { buffer.fill(0); } };

test('both roles prepare actual startup pins before any identity acquisition, then release the same document', async () => {
  let identityReads = 0;
  const owners = ['reader', 'collector'].map(role => offlineStartupOwnerActor(role, { afterRelease: () => { identityReads++; } }));
  const prepared = await Promise.all(owners.map(owner => owner.prepare()));
  assert.equal(identityReads, 0);
  assert.notEqual(prepared[0].handle, prepared[1].handle);
  assert.deepEqual({ ...prepared[0].pins }, offlineStartupOwner('reader'));
  const results = await Promise.all(owners.map(async (owner, i) => decode(await owner.acquire(prepared[i].handle, request(i ? 'collector' : 'reader', prepared[i].pins)))));
  assert.equal(identityReads, 2);
  for (let i = 0; i < 2; i++) {
    assert.equal(results[i].documentId, prepared[i].pins.documentId);
    assert.equal(results[i].contextId, prepared[i].pins.contextId);
    assert.equal(results[i].accountId, '1908123456789012345');
    await owners[i].abort(prepared[i].handle);
  }
});

test('wrong handle, scope drift and replay cannot release or rebind a prepared document', async () => {
  for (const fault of ['handle', 'document', 'context', 'replay']) {
    let reads = 0;
    const owner = offlineStartupOwnerActor('reader', { afterRelease: () => { reads++; } });
    const ready = await owner.prepare(), input = request('reader', ready.pins);
    if (fault === 'document') input.documentId = 'new-document';
    if (fault === 'context') input.contextId = '8';
    if (fault === 'replay') {
      (await owner.acquire(ready.handle, input)).fill(0);
      await assert.rejects(owner.acquire(ready.handle, input), /manual_startup_owner_unavailable/);
      assert.equal(reads, 1);
    } else {
      await assert.rejects(owner.acquire(fault === 'handle' ? 'wrong' : ready.handle, input), /manual_startup_owner_unavailable/);
      assert.equal(reads, 0);
    }
    await owner.abort(ready.handle);
  }
});

test('document/context loss after broker binding prevents identity reads and receipt publication', async () => {
  for (const method of ['Runtime.executionContextsCleared', 'Inspector.detached']) {
    const owner = offlineStartupOwnerActor('reader', { afterRelease: ({ emit }) => emit(method, {}) });
    const ready = await owner.prepare();
    await assert.rejects(owner.acquire(ready.handle, request('reader', ready.pins)), /manual_startup_owner_unavailable/);
    await owner.abort(ready.handle);
  }
});

test('abort before acquisition releases the held runner without emitting identity', async () => {
  let released = false;
  const owner = offlineStartupOwnerActor('reader', { afterRelease: () => { released = true; } });
  const ready = await owner.prepare();
  await owner.abort(ready.handle);
  await owner.abort(ready.handle);
  assert.equal(released, false);
  await assert.rejects(owner.acquire(ready.handle, request('reader', ready.pins)), /manual_startup_owner_unavailable/);
});

test('cancelled prepare, early completion and private errors produce only closed diagnostics', async () => {
  const abort = new AbortController(); abort.abort();
  await assert.rejects(offlineStartupOwnerActor('reader').prepare(abort.signal), /manual_startup_owner_unavailable/);
  for (const run of [async () => {}, async () => { throw new Error('private-error-secret'); }]) {
    await assert.rejects(createManualStartupOwner({ run }).prepare(), e => e.message === 'manual_startup_owner_unavailable');
  }
});

test('normal-pause owner may retry empty bootstrap only in the same frozen scope', async () => {
  let calls = 0;
  const owner = createManualStartupOwner({ run: sink => runOfflineStartupRecipe({ role: 'reader' }, sink, {
    beforeIdentityRead: async p => { await sink.beforeIdentityRead(p); await sink.beforeIdentityRead(p); calls++; },
  }) });
  const ready = await owner.prepare();
  (await owner.acquire(ready.handle, request('reader', ready.pins))).fill(0);
  assert.equal(calls, 1);
  await owner.abort(ready.handle);
});

test('a valid private sample cannot escape an owner whose restoration fails afterward',async()=>{
 for(const role of ['reader','collector']){let sampled=false;const owner=createManualStartupOwner({role,run:async sink=>{await runOfflineStartupRecipe({role},{onPrivateStartup:value=>{sampled=true;sink.onPrivateStartup(value);}},{beforeIdentityRead:sink.beforeIdentityRead});throw new Error('manual_root_control_failed');}});const ready=await owner.prepare();await assert.rejects(owner.acquire(ready.handle,request(role,ready.pins)));assert.equal(sampled,true);await owner.abort(ready.handle);}
});
