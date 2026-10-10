import test from 'node:test';
import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { mkdir, mkdtemp, rm, realpath } from 'node:fs/promises';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { PassThrough } from 'node:stream';
import { setTimeout as delay } from 'node:timers/promises';
import { launchChrome } from '../chrome.mjs';
import { runWorker } from '../worker.mjs';

const projectRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../../../../../');
const cookie = 'aku_idle_fixture=persisted';

async function until(check, message, timeout = 10000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    if (await check()) return;
    await delay(25);
  }
  throw new Error(message);
}

test('owned Chrome idles, resumes the same profile and retains authentication; a whole batch pins its frontier', {
  skip: !process.env.AKU_TEST_CHROME && 'set AKU_TEST_CHROME to opt into disposable local Chrome verification',
  timeout: 60000,
}, async () => {
  const build = resolve(projectRoot, 'build');
  await mkdir(build, { recursive: true });
  const profile = await mkdtemp(resolve(await realpath(build), 'headless-idle-smoke-'));
  const observed = [];
  const server = createServer((request, response) => {
    if (request.url === '/seed') response.setHeader('Set-Cookie', `${cookie}; Path=/; HttpOnly; SameSite=Lax; Max-Age=3600`);
    if (request.url.startsWith('/observe')) observed.push(request.headers.cookie || '');
    response.writeHead(200, { 'Content-Type': 'text/html' });
    response.end('<!doctype html><title>Idle fixture</title>');
  });
  const input = new PassThrough(), output = new PassThrough();
  const browsers = [], waiters = new Map();
  let buffer = '', sequence = 0, workerRun, cleanupVerified = false, uncertainLaunch = false;
  output.on('data', chunk => {
    buffer += chunk;
    let end;
    while ((end = buffer.indexOf('\n')) >= 0) {
      const reply = JSON.parse(buffer.slice(0, end));
      buffer = buffer.slice(end + 1);
      waiters.get(reply.id)?.(reply);
      waiters.delete(reply.id);
    }
  });
  async function rpc(request) {
    const id = ++sequence;
    const replyPromise = new Promise(resolveReply => waiters.set(id, resolveReply));
    input.write(JSON.stringify({ id, ...request }) + '\n');
    const reply = await replyPromise;
    assert.equal(reply.ok, true, JSON.stringify(reply.error));
    return reply.result;
  }
  async function navigate(page, url) {
    const result = await page.navigate(url);
    assert.equal(result.errorText, undefined);
    await until(async () => {
      try { return await page.evaluate(`location.href === ${JSON.stringify(url)} && document.readyState === 'complete'`); }
      catch { return false; }
    }, 'local fixture did not load');
  }
  try {
    await new Promise((resolveListen, reject) => {
      server.once('error', reject);
      server.listen(0, '127.0.0.1', resolveListen);
    });
    const base = `http://127.0.0.1:${server.address().port}`;
    workerRun = runWorker({
      input, output, errorOutput: new PassThrough(), idleTimeoutMs: 200,
      sourceAssetsImpl: async () => [],
      launchChromeImpl: async options => {
        let browser;
        try { browser = await launchChrome(options); }
        catch (error) { uncertainLaunch ||= error.code === 'owned_chrome_cleanup_failed'; throw error; }
        const record = { browser, closed: false };
        browsers.push(record);
        const close = browser.close;
        browser.close = async () => { await close(); record.closed = true; };
        return browser;
      },
      captureImpl: async (browser, _assets, source, payload) => {
        const page = await browser.forSource(source);
        if (payload.seed) await navigate(page, `${base}/seed`);
        if (payload.continue) assert.equal(page.fixtureFrontier, 'retained', 'held batch lost its source frontier');
        await navigate(page, `${base}/observe?source=${source}&round=${payload.round}`);
        page.fixtureFrontier = 'retained';
        return { pid: browser.pid };
      },
    });
    const initialized = await rpc({ type: 'init', chrome: resolve(process.env.AKU_TEST_CHROME), profile, profileDirectory: 'Default', bridgePath: projectRoot });
    assert.equal(initialized.idleReleaseVersion, 1);
    await rpc({ type: 'setIdleHold', held: true });
    const first = await rpc({ type: 'capture', source: 'x', payload: { seed: true, round: 1 } });
    await rpc({ type: 'capture', source: 'facebook', payload: { round: 1 } });
    await delay(600);
    assert.equal(browsers.length, 1);
    assert.equal(browsers[0].closed, false, 'Chrome closed during an admitted batch');
    await rpc({ type: 'capture', source: 'x', payload: { continue: true, round: 2 } });
    await rpc({ type: 'setIdleHold', held: false });
    await until(() => browsers[0].closed, 'Chrome did not exit after idle');
    assert.throws(() => process.kill(first.pid, 0), { code: 'ESRCH' }, 'the old Chrome PID must be gone');
    await rpc({ type: 'setIdleHold', held: true });
    assert.equal(browsers.length, 1, 'admitting a batch must not launch Chrome by itself');
    const resumed = await rpc({ type: 'capture', source: 'x', payload: { round: 3 } });
    assert.notEqual(resumed.pid, first.pid);
    assert.equal(browsers.length, 2);
    for (const entry of browsers) assert.equal(entry.browser.profilePath, profile);
    assert.equal(observed.length, 4);
    for (const received of observed) assert.ok(received.split(';').map(part => part.trim()).includes(cookie), 'shared authentication cookie was lost');
    await rpc({ type: 'shutdown' });
    await workerRun;
    assert.ok(browsers.every(record => record.closed));
    cleanupVerified = true;
  } finally {
    input.destroy();
    if (workerRun) await workerRun;
    // Only fixture processes are touched. Keep the disposable profile if exit
    // cannot be verified; never force another browser or remove locked files.
    const cleanup = await Promise.allSettled(browsers.map(record => record.browser.close()));
    cleanupVerified ||= cleanup.every(result => result.status === 'fulfilled');
    if (server.listening) await new Promise(resolveClose => server.close(resolveClose));
    if (cleanupVerified && !uncertainLaunch) await rm(profile, { recursive: true, force: true });
  }
});
