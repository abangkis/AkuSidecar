import test from 'node:test';
import assert from 'node:assert/strict';
import { concurrentEngineCommand, concurrentFixtureCookie } from './concurrent-engine-commands.mjs';

test('private concurrent commands reject unknown operations and scope expansion', () => {
  for (const raw of [null, [], {}, { id: 1, op: 'Runtime.evaluate' }, { id: 1, op: 'seed_fixture', owner: 1, value: 'a'.repeat(64), domain: 'x.com' }, { id: 1, op: 'collect', owner: 1, url: 'https://x.com' }, { id: 1, op: 'launch', role: 'owner', nativeReaderStartup: false }, { op: 'fixture_bootstrap', withholdReady: true }]) assert.throws(() => concurrentEngineCommand(raw));
});
test('synthetic seed is bounded and attributes are forced to exact loopback scope', () => {
  for (const value of ['', 'a'.repeat(63), 'A'.repeat(64), 'a'.repeat(65), null]) {
    assert.throws(() => concurrentEngineCommand({ id: 1, op: 'seed_fixture', owner: 1, value }));
    assert.throws(() => concurrentFixtureCookie(value, 'http://127.0.0.1:1234'));
  }
  for (const origin of ['https://x.com', 'http://localhost:1234', 'http://127.0.0.1:1234/path', 'http://127.0.0.1:1234/?cookie=bad']) assert.throws(() => concurrentFixtureCookie('a'.repeat(64), origin));
  const cookie = concurrentFixtureCookie('a'.repeat(64), 'http://127.0.0.1:1234');
  assert.equal(cookie.name, 'fixture_session'); assert.equal(cookie.url, 'http://127.0.0.1:1234/');
  assert.equal(cookie.domain, undefined); assert.equal(cookie.secure, true); assert.equal(cookie.httpOnly, true); assert.equal(cookie.sameSite, 'Lax');
});
test('window and role commands remain bounded', () => {
  assert.equal(concurrentEngineCommand({ id: 1, op: 'launch', role: 'collector', nativeReaderStartup: false }).role, 'collector');
  for (const index of [-1, 16, 1.5, '1']) assert.throws(() => concurrentEngineCommand({ id: 1, op: 'close_window', owner: 1, index }));
  assert.equal(concurrentEngineCommand({ id: 1, op: 'close_window', owner: 1, index: 0 }).index, 0);
});
