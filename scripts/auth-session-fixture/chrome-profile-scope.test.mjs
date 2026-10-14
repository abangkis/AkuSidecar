import test from 'node:test';
import assert from 'node:assert/strict';
import { mkdtemp, mkdir, writeFile, rename, realpath, rm } from 'node:fs/promises';
import { join, dirname, relative, isAbsolute, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { scopeProcessFixtureProfile, launchFixtureChrome } from './chrome.mjs';

const parent = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'runtime', 'qa', 'auth-session-sidecar');
const nonce = 'a'.repeat(64);
async function ownedScope(t) {
  await mkdir(parent, { recursive: true });
  const base = await realpath(parent);
  const root = await mkdtemp(join(base, 'run-profile-scope-'));
  t.after(async () => {
    const resolved = await realpath(root);
    const rel = relative(base, resolved);
    assert.equal(resolved, root);
    assert.ok(rel && !isAbsolute(rel) && !rel.startsWith('..') && basename(root).startsWith('run-profile-scope-'));
    await rm(root, { recursive: true, force: true });
  });
  return root;
}

test('ordinary fresh scope still rejects an existing empty profile', async t => {
  const root = await ownedScope(t);
  await mkdir(join(root, 'shared'));
  await assert.rejects(scopeProcessFixtureProfile(root, nonce, false), /fixture_process_profile_not_new/);
});

test('prepared scope accepts only an empty directory and cannot be replayed', async t => {
  const root = await ownedScope(t);
  await mkdir(join(root, 'shared'));
  await scopeProcessFixtureProfile(root, nonce, false, true);
  await assert.rejects(scopeProcessFixtureProfile(root, nonce, false, true), /fixture_process_scope_invalid/);
});

test('prepared scope rejects a populated profile', async t => {
  const root = await ownedScope(t);
  await mkdir(join(root, 'shared'));
  await writeFile(join(root, 'shared', 'synthetic-existing-state'), 'synthetic');
  await assert.rejects(scopeProcessFixtureProfile(root, nonce, false, true), /fixture_prepared_profile_not_empty/);
});

test('prepared scope is invalid for reopen and rejects a missing directory', async t => {
  const root = await ownedScope(t);
  await assert.rejects(scopeProcessFixtureProfile(root, nonce, true, true), /fixture_process_scope_invalid/);
  await assert.rejects(scopeProcessFixtureProfile(root, nonce, false, true), { code: 'ENOENT' });
});

test('replacing a prepared directory is rejected before process spawn', async t => {
  const root = await ownedScope(t);
  await mkdir(join(root, 'shared'));
  await scopeProcessFixtureProfile(root, nonce, false, true);
  await rename(join(root, 'shared'), join(root, 'retired-shared'));
  await mkdir(join(root, 'shared'));
  await assert.rejects(launchFixtureChrome(join(root, 'nonexistent-chrome.exe'), root, 'collector', 'shared'), /fixture_process_profile_identity_invalid/);
});
