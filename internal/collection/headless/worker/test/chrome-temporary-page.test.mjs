import test from 'node:test';
import assert from 'node:assert/strict';
import { createTemporaryPage } from '../chrome.mjs';

test('temporary page exposes scoped operations and closes only its own target once', async () => {
  const calls = [];
  const closed = [];
  const page = await createTemporaryPage({
    browserSessionId: 'browser-session',
    closeTargetAndWait: async targetId => { closed.push(targetId); },
    async send(method, params, sessionId, timeoutMs) {
      calls.push({ method, params, sessionId, timeoutMs });
      if (method === 'Target.createTarget') return { targetId: 'temporary-target' };
      if (method === 'Target.attachToTarget') return { sessionId: 'temporary-session' };
      if (method === 'Runtime.evaluate') return { result: { value: 42 } };
      if (method === 'Page.navigate') return { frameId: 'temporary-frame' };
      return {};
    },
  });

  assert.equal(await page.evaluate('21 * 2'), 42);
  await page.navigate('https://x.com/theo/status/2106847019319062819');
  await Promise.all([page.close(), page.close()]);
  assert.equal(calls[0].method, 'Target.createTarget');
  assert.equal(calls[0].sessionId, 'browser-session');
  assert.ok(calls.some(call => call.method === 'Runtime.evaluate' && call.sessionId === 'temporary-session'));
  assert.deepEqual(closed, ['temporary-target']);
});

test('temporary page setup shares one deadline across CDP steps and confirms cleanup on failure', async () => {
  const methods = [];
  const timeouts = [];
  const closed = [];
  const started = Date.now();
  await assert.rejects(createTemporaryPage({
    browserSessionId: 'browser-session', timeoutMs: 55,
    closeTargetAndWait: async targetId => { closed.push(targetId); },
    async send(method, _params, _sessionId, timeoutMs) {
      methods.push(method);
      timeouts.push(timeoutMs);
      await new Promise(resolve => setTimeout(resolve, 20));
      if (method === 'Target.createTarget') return { targetId: 'temporary-target' };
      if (method === 'Target.attachToTarget') return { sessionId: 'temporary-session' };
      return {};
    },
  }), { code: 'capture_timeout' });
  assert.ok(Date.now() - started < 120, 'setup should stop near its one 55 ms budget');
  assert.ok(methods.length < 7, 'later setup steps should not each receive a fresh 55 ms timeout');
  assert.ok(timeouts.every(timeout => timeout <= 55));
  assert.deepEqual(closed, ['temporary-target']);
});

test('temporary page setup reports a target cleanup failure', async () => {
  await assert.rejects(createTemporaryPage({
    browserSessionId: 'browser-session', timeoutMs: 100,
    closeTargetAndWait: async () => { throw new Error('target still exists'); },
    async send(method) {
      if (method === 'Target.createTarget') return { targetId: 'temporary-target' };
      throw new Error('attach failed');
    },
  }), { code: 'temporary_target_cleanup_failed' });
});
