import assert from 'node:assert/strict';
import { createCipheriv, createDecipheriv, randomBytes } from 'node:crypto';
import { mkdtemp, mkdir, readFile, realpath, rm, symlink, writeFile } from 'node:fs/promises';
import { join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { after, before, test } from 'node:test';
import { FIXTURE_VAULT_TTL_MS, FixtureVaultStore } from './vault.mjs';

const projectRoot = resolve(fileURLToPath(new URL('../..', import.meta.url)));
const fixtureParent = join(projectRoot, 'runtime', 'qa', 'auth-session-fixture');
const origin = 'http://127.0.0.1:47821';
const secret = 'a'.repeat(64);
let runRoot;

async function removeRun(directory) {
  const parent = await realpath(fixtureParent), target = await realpath(directory);
  const rel = relative(parent, target);
  assert.equal(!isAbsolute(rel) && /^run-vault-[^\\/]+$/.test(rel), true);
  await rm(target, { recursive: true, force: true });
}

function mockProtector() {
  const key = randomBytes(32);
  return {
    async protect(plaintext) {
      const iv = randomBytes(12);
      const cipher = createCipheriv('aes-256-gcm', key, iv);
      const ciphertext = Buffer.concat([cipher.update(plaintext), cipher.final()]);
      return Buffer.concat([iv, cipher.getAuthTag(), ciphertext]);
    },
    async unprotect(blob) {
      if (!Buffer.isBuffer(blob) || blob.length < 29) throw new Error('mock_failure');
      const decipher = createDecipheriv('aes-256-gcm', key, blob.subarray(0, 12));
      decipher.setAuthTag(blob.subarray(12, 28));
      return Buffer.concat([decipher.update(blob.subarray(28)), decipher.final()]);
    },
  };
}

function cookie(nowMs = Date.now(), name = 'fixture_session') {
  return {
    name,
    value: secret,
    domain: '127.0.0.1',
    path: '/',
    expires: Math.floor(nowMs / 1000) + 3600,
    httpOnly: true,
    secure: true,
    sameSite: 'Lax',
  };
}

function makeStore(options = {}) {
  return new FixtureVaultStore({
    runRoot,
    origin,
    expectedAccountId: 'alpha',
    protector: mockProtector(),
    ...options,
  });
}

before(async () => {
  try { await mkdir(fixtureParent); }
  catch (error) {
    if (error.code !== 'EEXIST') throw error;
  }
  runRoot = await mkdtemp(join(fixtureParent, 'run-vault-test-'));
});

after(async () => {
  if (runRoot) await removeRun(runRoot);
});

test('saves only encrypted bytes and restores a normalized synthetic ticket', async () => {
  const protector = mockProtector();
  const now = Date.now();
  const store = makeStore({ protector, now: () => now });
  const saved = await store.save({ origin, accountId: 'alpha', cookies: [cookie(now)] });
  assert.deepEqual(Object.keys(saved).sort(), ['expiresAt', 'ok']);
  assert.equal(saved.ok, true);
  assert.equal(saved.expiresAt - now, FIXTURE_VAULT_TTL_MS);

  const file = await readFile(join(runRoot, 'fixture-session.vault'));
  assert.equal(file.includes(Buffer.from(secret)), false);
  assert.equal(file.toString('utf8').includes(secret), false);

  const restored = await store.restore({ origin, expectedAccountId: 'alpha' });
  assert.equal(restored.origin, origin);
  assert.equal(restored.accountId, 'alpha');
  assert.equal(restored.cookies.length, 1);
  assert.equal(restored.cookies[0].name, 'fixture_session');
  assert.equal(restored.cookies[0].value, secret);
  assert.equal(restored.cookies[0].httpOnly, true);
  assert.equal(restored.cookies[0].secure, true);
});

test('rejects ciphertext tampering without exposing the ticket value', async () => {
  const store = makeStore();
  await store.save({ origin, accountId: 'alpha', cookies: [cookie()] });
  const filePath = join(runRoot, 'fixture-session.vault');
  const blob = await readFile(filePath);
  blob[blob.length - 1] ^= 0x01;
  await writeFile(filePath, blob);

  await assert.rejects(store.restore({ origin, expectedAccountId: 'alpha' }), error => {
    assert.equal(error.code, 'VAULT_UNPROTECT_FAILED');
    assert.equal(error.message.includes(secret), false);
    return true;
  });
});

test('rejects a ticket bound to another invented account', async () => {
  const protector = mockProtector();
  const beta = new FixtureVaultStore({ runRoot, origin, expectedAccountId: 'beta', protector });
  await beta.save({ origin, accountId: 'beta', cookies: [cookie()] });
  const alpha = makeStore({ protector });

  await assert.rejects(alpha.restore({ origin, expectedAccountId: 'alpha' }), error => {
    assert.equal(error.code, 'VAULT_ACCOUNT_MISMATCH');
    assert.equal(error.message.includes(secret), false);
    return true;
  });
});

test('reports a missing vault with a fixed safe error', async () => {
  const missingRoot = await mkdtemp(join(fixtureParent, 'run-vault-missing-'));
  try {
    const store = new FixtureVaultStore({
      runRoot: missingRoot,
      origin,
      expectedAccountId: 'alpha',
      protector: mockProtector(),
    });
    await assert.rejects(store.restore({ origin, expectedAccountId: 'alpha' }), error => {
      assert.equal(error.code, 'VAULT_NOT_FOUND');
      assert.equal(error.message, 'VAULT_NOT_FOUND');
      assert.equal(error.message.includes(missingRoot), false);
      return true;
    });
  } finally {
    await removeRun(missingRoot);
  }
});

test('rejects tickets after the fixed 15 minute lifetime', async () => {
  let now = 1_800_000_000_000;
  const store = makeStore({ now: () => now });
  await store.save({ origin, accountId: 'alpha', cookies: [cookie(now)] });
  now += FIXTURE_VAULT_TTL_MS;

  await assert.rejects(store.restore({ origin, expectedAccountId: 'alpha' }), error => {
    assert.equal(error.code, 'VAULT_EXPIRED');
    return true;
  });
});

test('accepts only the exact loopback origin, invented accounts, and fixture cookie', async () => {
  assert.throws(() => makeStore({ origin: 'http://localhost:47821' }), { code: 'FIXTURE_ORIGIN_NOT_ALLOWED' });
  assert.throws(() => makeStore({ expectedAccountId: 'real-user' }), { code: 'ACCOUNT_MISMATCH' });
  const store = makeStore();
  await assert.rejects(store.save({ origin, accountId: 'alpha', cookies: [cookie(Date.now(), 'auth')] }), {
    code: 'FIXTURE_COOKIE_NOT_ALLOWED',
  });
  await assert.rejects(store.save({ origin: 'http://127.0.0.1', accountId: 'alpha', cookies: [cookie()] }), {
    code: 'FIXTURE_ORIGIN_NOT_ALLOWED',
  });
});

test('rejects roots outside the existing fixture run directory', () => {
  const outside = join(projectRoot, 'runtime', 'qa', 'other-fixture', 'run-vault-test');
  assert.throws(() => new FixtureVaultStore({
    runRoot: outside,
    origin,
    expectedAccountId: 'alpha',
    protector: mockProtector(),
  }), { code: 'FIXTURE_VAULT_ROOT_INVALID' });
});

test('rejects a reparse-point run root before reading or writing', async t => {
  const linkPath = join(fixtureParent, `run-vault-link-${process.pid}`);
  try {
    await symlink(runRoot, linkPath, 'junction');
  } catch (error) {
    if (['EPERM', 'EACCES', 'ENOTSUP'].includes(error.code)) {
      t.skip('Windows junction creation is unavailable in this environment');
      return;
    }
    throw error;
  }
  try {
    const linked = new FixtureVaultStore({
      runRoot: linkPath,
      origin,
      expectedAccountId: 'alpha',
      protector: mockProtector(),
    });
    await assert.rejects(linked.save({ origin, accountId: 'alpha', cookies: [cookie()] }), {
      code: 'FIXTURE_VAULT_ROOT_INVALID',
    });
  } finally {
    await rm(linkPath, { recursive: false, force: true });
  }
});
