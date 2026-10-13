import { spawn } from 'node:child_process';
import { randomUUID } from 'node:crypto';
import { lstat, open, readFile, rename, unlink } from 'node:fs/promises';
import { basename, dirname, join, parse, relative, resolve, sep } from 'node:path';
import { fileURLToPath } from 'node:url';
import { normalizeFixtureCookies } from './broker.mjs';

const MODULE_DIR = dirname(fileURLToPath(import.meta.url));
const PROJECT_ROOT = resolve(MODULE_DIR, '../..');
const VAULT_PARENT = ['runtime', 'qa', 'auth-session-fixture'];
const VAULT_BASENAME = 'fixture-session.vault';
const TEMP_PREFIX = `${VAULT_BASENAME}.tmp-`;
const MAGIC = Buffer.from('AKU-AUTH-FIXTURE-VAULT\0v1\n', 'ascii');
const FORMAT_VERSION = 1;
const MAX_PLAINTEXT_BYTES = 64 * 1024;
const MAX_FILE_BYTES = 256 * 1024;
const DPAPI_HELPER_TIMEOUT_MS = 10_000;
const SAFE_VAULT_FILE_CODES = new Set([
  'FIXTURE_VAULT_FILE_INVALID', 'FIXTURE_VAULT_FORMAT_INVALID', 'VAULT_NOT_FOUND',
]);
export const FIXTURE_VAULT_TTL_MS = 15 * 60 * 1000;

function fail(code) {
  const error = new Error(code);
  error.code = code;
  return error;
}

function pathKey(value) {
  return resolve(value).replace(/^\\\\\?\\/, '').replaceAll('/', '\\').toLowerCase();
}

function safeOrigin(value) {
  if (typeof value !== 'string') throw fail('FIXTURE_ORIGIN_NOT_ALLOWED');
  let url;
  try { url = new URL(value); } catch { throw fail('FIXTURE_ORIGIN_NOT_ALLOWED'); }
  if (url.protocol !== 'http:' || url.hostname !== '127.0.0.1' || !url.port ||
      url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== url.origin) {
    throw fail('FIXTURE_ORIGIN_NOT_ALLOWED');
  }
  return value;
}

function safeAccount(value) {
  return value === 'alpha' || value === 'beta';
}

function validNow(value) {
  return Number.isSafeInteger(value) && value >= 0;
}

function exactKeys(value, keys) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return false;
  const actual = Object.keys(value).sort();
  const expected = [...keys].sort();
  return actual.length === expected.length && actual.every((key, index) => key === expected[index]);
}

function validateCookies(snapshot, origin, nowMs) {
  let cookies;
  try {
    cookies = normalizeFixtureCookies({ cookies: snapshot }, origin, nowMs);
  } catch (error) {
    if (typeof error?.code === 'string' && /^[A-Z0-9_]+$/.test(error.code)) throw fail(error.code);
    throw fail('COOKIE_SNAPSHOT_INVALID');
  }
  if (cookies.length !== 1 || cookies[0].name !== 'fixture_session') {
    throw fail('FIXTURE_COOKIE_NOT_ALLOWED');
  }
  const cookie = cookies[0];
  const nowSeconds = nowMs / 1000;
  if (!/^[a-f0-9]{64}$/.test(cookie.value) || cookie.domain !== '127.0.0.1' ||
      cookie.path !== '/' || cookie.secure !== true ||
      cookie.httpOnly !== true || cookie.sameSite !== 'Lax' || cookie.partitionKey !== undefined ||
      cookie.expires <= nowSeconds || cookie.expires > nowSeconds + 3660) {
    throw fail('FIXTURE_COOKIE_NOT_ALLOWED');
  }
  return cookies;
}

function validateRunRootLexically(runRoot) {
  if (typeof runRoot !== 'string' || !runRoot || !/^[A-Za-z]:[\\/]/.test(runRoot)) {
    throw fail('FIXTURE_VAULT_ROOT_INVALID');
  }
  const candidate = resolve(runRoot);
  const name = basename(candidate);
  if (!/^run-[A-Za-z0-9][A-Za-z0-9_-]{0,63}$/.test(name)) {
    throw fail('FIXTURE_VAULT_ROOT_INVALID');
  }
  const expected = resolve(PROJECT_ROOT, ...VAULT_PARENT, name);
  if (pathKey(candidate) !== pathKey(expected)) throw fail('FIXTURE_VAULT_ROOT_INVALID');
  return expected;
}

async function validateRunRoot(runRoot) {
  const expected = validateRunRootLexically(runRoot);
  const root = parse(expected).root;
  let current = root;
  const parts = relative(root, expected).split(sep).filter(Boolean);
  for (const part of parts) {
    current = join(current, part);
    let info;
    try {
      info = await lstat(current);
    } catch {
      throw fail('FIXTURE_VAULT_ROOT_INVALID');
    }
    if (!info.isDirectory() || info.isSymbolicLink()) {
      throw fail('FIXTURE_VAULT_ROOT_INVALID');
    }
  }
  if (pathKey(current) !== pathKey(expected)) throw fail('FIXTURE_VAULT_ROOT_INVALID');
  return current;
}

async function safeVaultFile(root, { missingAllowed = false } = {}) {
  const file = join(root, VAULT_BASENAME);
  try {
    const info = await lstat(file);
    if (!info.isFile() || info.isSymbolicLink()) {
      throw fail('FIXTURE_VAULT_FILE_INVALID');
    }
    if (info.size < MAGIC.length + 1 || info.size > MAX_FILE_BYTES) {
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }
    return file;
  } catch (error) {
    if (error?.code === 'ENOENT') {
      if (missingAllowed) return null;
      throw fail('VAULT_NOT_FOUND');
    }
    if (SAFE_VAULT_FILE_CODES.has(error?.code)) throw error;
    throw fail('FIXTURE_VAULT_FILE_INVALID');
  }
}

function createWindowsDpapiProtector() {
  async function transform(operation, input) {
    if (process.platform !== 'win32') throw fail('DPAPI_UNAVAILABLE');
    if (!Buffer.isBuffer(input) || input.length === 0 || input.length > MAX_PLAINTEXT_BYTES) {
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }
    const request = JSON.stringify({ operation, data: input.toString('base64') });
    return new Promise((resolvePromise, rejectPromise) => {
      let settled = false;
      let timeoutId;
      const done = (error, value) => {
        if (settled) return;
        settled = true;
        if (timeoutId) clearTimeout(timeoutId);
        if (error) rejectPromise(error);
        else resolvePromise(value);
      };
      let child;
      try {
        child = spawn('powershell.exe', [
          '-NoLogo', '-NoProfile', '-NonInteractive', '-ExecutionPolicy', 'Bypass',
          '-File', join(MODULE_DIR, 'dpapi.ps1'),
        ], { windowsHide: true, stdio: ['pipe', 'pipe', 'pipe'] });
      } catch {
        done(fail('DPAPI_UNAVAILABLE'));
        return;
      }
      const stdout = [];
      let stdoutBytes = 0;
      child.stdout.on('data', chunk => {
        stdoutBytes += chunk.length;
        if (stdoutBytes > MAX_FILE_BYTES) {
          child.kill();
          done(fail(operation === 'protect' ? 'DPAPI_PROTECT_FAILED' : 'VAULT_UNPROTECT_FAILED'));
          return;
        }
        stdout.push(chunk);
      });
      child.stderr.on('data', () => {}); // Discard helper diagnostics; they may include platform details.
      child.on('error', () => done(fail('DPAPI_UNAVAILABLE')));
      child.on('close', code => {
        if (code !== 0) {
          done(fail(operation === 'protect' ? 'DPAPI_PROTECT_FAILED' : 'VAULT_UNPROTECT_FAILED'));
          return;
        }
        const encoded = Buffer.concat(stdout).toString('ascii').trim();
        if (!encoded || !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/.test(encoded)) {
          done(fail(operation === 'protect' ? 'DPAPI_PROTECT_FAILED' : 'VAULT_UNPROTECT_FAILED'));
          return;
        }
        const output = Buffer.from(encoded, 'base64');
        if (output.length === 0 || output.length > MAX_FILE_BYTES || output.toString('base64') !== encoded) {
          done(fail(operation === 'protect' ? 'DPAPI_PROTECT_FAILED' : 'VAULT_UNPROTECT_FAILED'));
          return;
        }
        done(null, output);
      });
      child.stdin.on('error', () => {});
      timeoutId = setTimeout(() => {
        try { child.kill(); } catch {}
        done(fail(operation === 'protect' ? 'DPAPI_PROTECT_FAILED' : 'VAULT_UNPROTECT_FAILED'));
      }, DPAPI_HELPER_TIMEOUT_MS);
      child.stdin.end(request, 'utf8');
    });
  }
  return Object.freeze({
    protect: input => transform('protect', input),
    unprotect: input => transform('unprotect', input),
  });
}

function validateProtector(value) {
  if (!value || typeof value.protect !== 'function' || typeof value.unprotect !== 'function') {
    throw fail('FIXTURE_VAULT_PROTECTOR_INVALID');
  }
  return value;
}

/**
 * DPAPI-backed persistence for one synthetic local auth fixture.
 * The only writable file is fixture-session.vault in an existing run-* root.
 */
export class FixtureVaultStore {
  #runRoot;
  #origin;
  #expectedAccountId;
  #protector;
  #now;
  #tail = Promise.resolve();

  constructor({ runRoot, origin, expectedAccountId, protector = createWindowsDpapiProtector(),
    now = () => Date.now() } = {}) {
    this.#runRoot = validateRunRootLexically(runRoot);
    this.#origin = safeOrigin(origin);
    if (!safeAccount(expectedAccountId)) throw fail('ACCOUNT_MISMATCH');
    if (typeof now !== 'function') throw fail('CLOCK_INVALID');
    this.#expectedAccountId = expectedAccountId;
    this.#protector = validateProtector(protector);
    this.#now = now;
  }

  save({ origin, accountId, cookies } = {}) {
    return this.#enqueue(() => this.#save({ origin, accountId, cookies }));
  }

  restore({ origin, expectedAccountId } = {}) {
    return this.#enqueue(() => this.#restore({ origin, expectedAccountId }));
  }

  #enqueue(work) {
    const task = this.#tail.then(work, work);
    this.#tail = task.catch(() => undefined);
    return task;
  }

  async #save(input) {
    if (!exactKeys(input, ['origin', 'accountId', 'cookies']) || !Array.isArray(input.cookies)) {
      throw fail('COOKIE_SNAPSHOT_INVALID');
    }
    if (safeOrigin(input.origin) !== this.#origin) throw fail('FIXTURE_ORIGIN_NOT_ALLOWED');
    if (!safeAccount(input.accountId) || input.accountId !== this.#expectedAccountId) {
      throw fail('VAULT_ACCOUNT_MISMATCH');
    }
    const nowMs = this.#now();
    if (!validNow(nowMs)) throw fail('CLOCK_INVALID');
    const cookies = validateCookies(input.cookies, this.#origin, nowMs);
    const root = await validateRunRoot(this.#runRoot);
    const target = await safeVaultFile(root, { missingAllowed: true });
    const file = target ?? join(root, VAULT_BASENAME);
    const expiresAt = nowMs + FIXTURE_VAULT_TTL_MS;
    const payload = Buffer.from(JSON.stringify({
      version: FORMAT_VERSION,
      origin: this.#origin,
      accountId: this.#expectedAccountId,
      savedAt: nowMs,
      expiresAt,
      cookies,
    }), 'utf8');
    if (payload.length > MAX_PLAINTEXT_BYTES) {
      payload.fill(0);
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }

    let encrypted;
    try {
      encrypted = await this.#protector.protect(payload);
    } catch {
      throw fail('DPAPI_PROTECT_FAILED');
    } finally {
      payload.fill(0);
    }
    if (!Buffer.isBuffer(encrypted) || encrypted.length === 0 || encrypted.length > MAX_FILE_BYTES - MAGIC.length) {
      throw fail('DPAPI_PROTECT_FAILED');
    }
    const blob = Buffer.concat([MAGIC, encrypted]);
    const temporary = join(root, `${TEMP_PREFIX}${randomUUID()}`);
    let handle;
    try {
      handle = await open(temporary, 'wx', 0o600);
      await handle.writeFile(blob);
      await handle.sync();
      await handle.close();
      handle = undefined;
      await safeVaultFile(root, { missingAllowed: true });
      await rename(temporary, file);
    } catch (error) {
      try { await handle?.close(); } catch {}
      try { await unlink(temporary); } catch {}
      if (error?.code && /^[A-Z0-9_]+$/.test(error.code)) throw fail(error.code);
      throw fail('FIXTURE_VAULT_WRITE_FAILED');
    }
    return Object.freeze({ ok: true, expiresAt });
  }

  async #restore(input) {
    if (!exactKeys(input, ['origin', 'expectedAccountId'])) throw fail('FIXTURE_VAULT_REQUEST_INVALID');
    if (safeOrigin(input.origin) !== this.#origin) throw fail('FIXTURE_ORIGIN_NOT_ALLOWED');
    if (!safeAccount(input.expectedAccountId) || input.expectedAccountId !== this.#expectedAccountId) {
      throw fail('VAULT_ACCOUNT_MISMATCH');
    }
    const nowMs = this.#now();
    if (!validNow(nowMs)) throw fail('CLOCK_INVALID');
    const root = await validateRunRoot(this.#runRoot);
    const file = await safeVaultFile(root);
    let blob;
    try { blob = await readFile(file); } catch { throw fail('FIXTURE_VAULT_READ_FAILED'); }
    if (blob.length < MAGIC.length + 1 || blob.length > MAX_FILE_BYTES ||
        !blob.subarray(0, MAGIC.length).equals(MAGIC)) {
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }
    let plaintext;
    try {
      plaintext = await this.#protector.unprotect(blob.subarray(MAGIC.length));
    } catch {
      throw fail('VAULT_UNPROTECT_FAILED');
    }
    if (!Buffer.isBuffer(plaintext) || plaintext.length === 0 || plaintext.length > MAX_PLAINTEXT_BYTES) {
      if (Buffer.isBuffer(plaintext)) plaintext.fill(0);
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }

    let payload;
    try {
      payload = JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(plaintext));
    } catch {
      plaintext.fill(0);
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }
    plaintext.fill(0);
    if (!exactKeys(payload, ['version', 'origin', 'accountId', 'savedAt', 'expiresAt', 'cookies']) ||
        payload.version !== FORMAT_VERSION || payload.origin !== this.#origin ||
        !safeAccount(payload.accountId) || !Array.isArray(payload.cookies) ||
        !Number.isSafeInteger(payload.savedAt) || !Number.isSafeInteger(payload.expiresAt) ||
        payload.expiresAt !== payload.savedAt + FIXTURE_VAULT_TTL_MS) {
      throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    }
    if (payload.accountId !== this.#expectedAccountId) throw fail('VAULT_ACCOUNT_MISMATCH');
    if (payload.savedAt > nowMs || payload.expiresAt <= nowMs) throw fail('VAULT_EXPIRED');

    const cookies = validateCookies(payload.cookies, this.#origin, nowMs);
    if (JSON.stringify(cookies) !== JSON.stringify(payload.cookies)) throw fail('FIXTURE_VAULT_FORMAT_INVALID');
    return Object.freeze({
      origin: this.#origin,
      accountId: payload.accountId,
      cookies,
    });
  }
}
