import test from 'node:test';
import assert from 'node:assert/strict';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { concurrencyFixtureOrigin, concurrencyFixtureRoot, fixtureConcurrencyCookie,
  createXSyntheticChromeAdapter } from './x-concurrency-chrome-adapter.mjs';
import { createConcurrencyFixtureRPC } from './x-concurrency-chrome-driver.mjs';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../runtime/qa/auth-session-sidecar',
  'run-x-concurrency-' + 'a'.repeat(32));
const origin = 'http://127.0.0.1:43121';
const rawCookie = () => ({ name: 'fixture_session', value: 'a'.repeat(64), domain: '127.0.0.1',
  path: '/', expires: 2000000000, size: 80, httpOnly: true, secure: true, session: false,
  sameSite: 'Lax', priority: 'Medium', sourceScheme: 'Secure', sourcePort: 43121 });

test('only exact loopback fixture origin and scoped fresh profile-root names are accepted', () => {
  assert.equal(concurrencyFixtureOrigin(origin), origin);
  assert.equal(concurrencyFixtureRoot(root), root);
  for (const value of ['https://x.com', 'http://localhost:43121', origin + '/path', origin + '/?token=private',
    'http://user:password@127.0.0.1:43121', 'http://127.0.0.1:43121.evil.test', 'invalid']) {
    assert.throws(() => concurrencyFixtureOrigin(value));
  }
  for (const value of [resolve(root, '..', 'shared'), resolve(root, '../../outside'), 'relative-root']) {
    assert.throws(() => concurrencyFixtureRoot(value));
  }
});

test('native scope is refused before any injected launcher effect', () => {
  let launches = 0;
  assert.throws(() => createXSyntheticChromeAdapter({ root, origin: 'https://x.com', executable: 'chrome.exe',
    launch() { launches++; } }));
  assert.equal(launches, 0);
});

test('fixed invented cookie mapping retains all supported transfer attributes', () => {
  const mapped = fixtureConcurrencyCookie(rawCookie());
  assert.deepEqual(mapped, { name: 'fixture_session', value: 'a'.repeat(64), domain: '127.0.0.1',
    path: '/', hostOnly: true, secure: true, httpOnly: true, sameSite: 'Lax', expires: 2000000000, partitionKey: null });
  assert.equal(Object.isFrozen(mapped), true);
});

test('cookie mapping refuses unapproved tuples and unsupported attribute changes', () => {
  for (const patch of [{ domain: 'x.com' }, { name: 'auth_token' }, { path: '/other' }, { secure: false },
    { httpOnly: false }, { sameSite: 'None' }, { expires: Infinity }, { session: true },
    { partitionKey: { topLevelSite: origin } }, { partitionKeyOpaque: true }, { unknown: 'private' }]) {
    assert.throws(() => fixtureConcurrencyCookie({ ...rawCookie(), ...patch }));
  }
  const missing = rawCookie(); delete missing.value;
  assert.throws(() => fixtureConcurrencyCookie(missing));
});

test('CDP fractional expiry is preserved without rounding; nonfinite and negative expiry is rejected', () => {
  assert.equal(fixtureConcurrencyCookie({ ...rawCookie(), expires: 2000000000.125 }).expires, 2000000000.125);
  for (const expires of [NaN, -1, Number.MAX_SAFE_INTEGER + 1]) {
    assert.throws(() => fixtureConcurrencyCookie({ ...rawCookie(), expires }));
  }
});

test('cookie getters are rejected without invocation or secret-bearing error text', () => {
  let reads = 0;
  const cookie = rawCookie();
  Object.defineProperty(cookie, 'value', { get() { reads++; return 'private-cookie'; } });
  assert.throws(() => fixtureConcurrencyCookie(cookie), error => error.message === 'synthetic_concurrency_adapter_failed');
  assert.equal(reads, 0);
});

test('RPC handshake has no adapter effects and rejects unapproved arguments', async () => {
  let effects = 0;
  const rpc = createConcurrencyFixtureRPC({ prepare() { effects++; }, run() { effects++; }, close() { effects++; } });
  await assert.rejects(rpc({ id: 1, op: 'concurrency_prepare' }));
  await assert.rejects(rpc({ op: 'fixture_bootstrap', withholdReady: false, origin: 'https://x.com' }));
  assert.deepEqual(await rpc({ op: 'fixture_bootstrap', withholdReady: false }), { ready: true });
  await assert.rejects(rpc({ id: 1, op: 'concurrency_prepare', profile: 'existing-main' }));
  await assert.rejects(rpc({ id: 1, op: 'concurrency_run', scenario: 'native' }));
  assert.equal(effects, 0);
});

test('prepare and run are one-use and expose no arbitrary transfer or CDP operation', async () => {
  const calls = [];
  const rpc = createConcurrencyFixtureRPC({ prepare() { calls.push('prepare'); return { prepared: true }; },
    run(value) { calls.push(value); return { canAuthorize: false }; }, close() { calls.push('close'); return { closed: true }; } });
  await rpc({ op: 'fixture_bootstrap', withholdReady: false });
  await rpc({ id: 1, op: 'concurrency_prepare' });
  await assert.rejects(rpc({ id: 2, op: 'concurrency_prepare' }));
  await assert.rejects(rpc({ id: 2, op: 'Storage.getCookies' }));
  await rpc({ id: 3, op: 'concurrency_run', scenario: 'overlap' });
  await assert.rejects(rpc({ id: 4, op: 'concurrency_run', scenario: 'overlap' }));
  await rpc({ id: 5, op: 'concurrency_close' });
  await assert.rejects(rpc({ id: 6, op: 'concurrency_prepare' }));
  assert.deepEqual(calls, ['prepare', 'overlap', 'close']);
});

test('failed preparation fences later runs while preserving explicit cleanup', async () => {
  let runs = 0, closes = 0;
  const rpc = createConcurrencyFixtureRPC({ prepare() { throw new Error('private-cookie'); },
    run() { runs++; }, close() { closes++; return { closed: true }; } });
  await rpc({ op: 'fixture_bootstrap', withholdReady: false });
  await assert.rejects(rpc({ id: 1, op: 'concurrency_prepare' }), /synthetic_concurrency_operation_denied/);
  await assert.rejects(rpc({ id: 2, op: 'concurrency_run', scenario: 'overlap' }));
  await rpc({ id: 3, op: 'concurrency_close' });
  assert.equal(runs, 0); assert.equal(closes, 1);
});

test('run and cleanup failures expose only fixed RPC denial errors', async () => {
  const rpc = createConcurrencyFixtureRPC({ prepare() { return {}; },
    run() { throw new Error('private-cookie-value'); }, close() { throw new Error('private-cookie-value'); } });
  await rpc({ op: 'fixture_bootstrap', withholdReady: false });
  await rpc({ id: 1, op: 'concurrency_prepare' });
  for (const message of [{ id: 2, op: 'concurrency_run', scenario: 'overlap' }, { id: 3, op: 'concurrency_close' }]) {
    await assert.rejects(rpc(message), error => error.message === 'synthetic_concurrency_operation_denied');
  }
});
