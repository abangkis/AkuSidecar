import assert from 'node:assert/strict';
import { EventEmitter } from 'node:events';
import { test } from 'node:test';
import { randomBytes } from 'node:crypto';
import { mkdir, realpath, rm, stat, writeFile } from 'node:fs/promises';
import { join, dirname, resolve } from 'node:path';
import { PassThrough } from 'node:stream';
import { fileURLToPath } from 'node:url';
import { createQAFeasibilityCollector } from './x-qa-feasibility-collector.mjs';
import { CDPFrameDecoder } from './x-native-metadata-probe-driver.mjs';
import { QA_BOUND_BOOTSTRAP_EXPRESSION } from './x-qa-bound-observation.mjs';

const PROJECT_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..');
const QA_ROOT = join(PROJECT_ROOT, 'runtime', 'qa', 'auth-session-sidecar');

async function makeRoot(t) {
  const parent = await realpath(QA_ROOT);
  const root = join(parent, `run-x-qa-source-${randomBytes(16).toString('hex')}`);
  await mkdir(root);
  t.after(async () => rm(root, { recursive: true, force: true }));
  return root;
}

class FakeChrome extends EventEmitter {
  constructor(onRequest = () => ({})) {
    super();
    this.pid = 43210;
    this.requests = [];
    this.wireBuffers = [];
    this.onRequest = onRequest;
    const output = this;
    this.stdio = [null, null, null, {
      write(buffer, callback) {
        output.wireBuffers.push(buffer);
        const copy = Buffer.from(buffer);
        const request = JSON.parse(copy.subarray(0, copy.length - 1).toString('utf8'));
        output.requests.push(request);
        callback();
        output.onRequest(request, output);
        return true;
      },
    }, new PassThrough()];
    queueMicrotask(() => this.emit('spawn'));
  }

  respond(request, result = {}) {
    const response = { id: request.id };
    if (request.sessionId !== undefined) response.sessionId = request.sessionId;
    response.result = result;
    this.stdio[4].write(Buffer.from(`${JSON.stringify(response)}\0`, 'utf8'));
  }

  respondError(request, message = 'synthetic private error') {
    const response = { id: request.id };
    if (request.sessionId !== undefined) response.sessionId = request.sessionId;
    response.error = { code: -32000, message, data: 'synthetic private detail' };
    this.stdio[4].write(Buffer.from(`${JSON.stringify(response)}\0`, 'utf8'));
  }

  emitEvent(event) {
    this.stdio[4].write(Buffer.from(`${JSON.stringify(event)}\0`, 'utf8'));
  }
}

function cookie(name, value = `private-${name}-value`) {
  return { name, value, domain: '.x.com', path: '/', secure: true, httpOnly: true, sameSite: 'Lax' };
}

function cookieSerializedSize(name, valueLength) {
  return Buffer.byteLength(JSON.stringify(cookie(name, 'v'.repeat(valueLength))), 'utf8');
}

function cookieAtSize(name, target) {
  let low = 0;
  let high = target;
  while (low < high) {
    const middle = Math.ceil((low + high) / 2);
    if (cookieSerializedSize(name, middle) <= target) low = middle;
    else high = middle - 1;
  }
  return cookie(name, 'v'.repeat(low));
}

function createCollector(root, spawnChrome, extras = {}) {
  return createQAFeasibilityCollector({
    executable: process.execPath,
    root,
    spawnChrome,
    FrameDecoder: CDPFrameDecoder,
    allowedExpressions: ['read-only-expression'],
    ...extras,
  });
}

test('rejects malformed roots and missing executables before spawning', async (t) => {
  const parent = await realpath(QA_ROOT);
  const malformedRoot = join(parent, `not-a-control-root-${randomBytes(8).toString('hex')}`);
  await mkdir(malformedRoot);
  t.after(async () => rm(malformedRoot, { recursive: true, force: true }));
  let spawnCount = 0;
  const spawnChrome = () => { spawnCount += 1; throw new Error('must not spawn'); };

  const invalidRootCollector = createCollector(malformedRoot, spawnChrome);
  await assert.rejects(invalidRootCollector.launch(), /qa_root_rejected/u);
  assert.equal(spawnCount, 0);

  const validRoot = await makeRoot(t);
  const missingExecutableCollector = createQAFeasibilityCollector({
    executable: join(validRoot, 'missing-chrome.exe'),
    root: validRoot,
    spawnChrome,
    FrameDecoder: CDPFrameDecoder,
  });
  await assert.rejects(missingExecutableCollector.launch(), /qa_root_or_executable_unavailable/u);
  assert.equal(spawnCount, 0);
  await assert.rejects(stat(join(validRoot, 'derived-headless')));
});

test('refuses an existing profile and never reuses a previous derived headless directory', async (t) => {
  const root = await makeRoot(t);
  const existingProfile = join(root, 'derived-headless');
  await mkdir(existingProfile);
  let spawnCount = 0;
  const collector = createCollector(root, () => { spawnCount += 1; throw new Error('must not spawn'); });
  await assert.rejects(collector.launch(), /qa_profile_already_exists/u);
  assert.equal(spawnCount, 0);
  assert.equal((await stat(existingProfile)).isDirectory(), true);
});

test('uses an isolated headless profile and BrowserContext with a one-use, bounded cookie seed', async (t) => {
  const root = await makeRoot(t);
  let child;
  let spawnArguments;
  let spawnOptions;
  const buildArtifact = join(root, 'go-build-artifact.marker');
  await writeFile(buildArtifact, 'owned test artifact');
  const spawnChrome = (executable, args, options) => {
    spawnArguments = { executable, args: [...args] };
    spawnOptions = options;
    child = new FakeChrome((request, current) => {
      if (request.method === 'Page.getFrameTree') {
        current.respondError(request, 'private cookie-like response text');
      } else if (request.method === 'Runtime.evaluate') {
        current.respond(request, { result: { type: 'string', value: 'private raw page text' } });
      } else {
        current.respond(request, {});
        if (request.method === 'Browser.close') setImmediate(() => current.emit('exit', 0, null));
      }
    });
    return child;
  };

  const environmentKey = 'AKU_TEST_PRIVATE_COOKIE_SENTINEL';
  const oldEnvironmentValue = process.env[environmentKey];
  process.env[environmentKey] = 'do-not-inherit-this-value';
  const collector = createCollector(root, spawnChrome);
  try {
    const launched = await collector.launch();
    assert.deepEqual(Object.keys(launched), ['pid']);
    assert.equal(launched.pid, child.pid);
    const profile = join(root, 'derived-headless');
    assert.equal(spawnArguments.executable, process.execPath);
    assert.deepEqual(spawnArguments.args, [
      `--user-data-dir=${profile}`,
      '--headless=new',
      '--remote-debugging-pipe',
      '--no-first-run',
      '--no-default-browser-check',
      '--disable-extensions',
      '--disable-logging',
      '--disable-breakpad',
      '--disable-crash-reporter',
      '--disable-background-mode',
      '--disable-sync',
      'about:blank',
    ]);
    assert.equal(spawnOptions.windowsHide, true);
    assert.deepEqual(spawnOptions.stdio, ['ignore', 'ignore', 'ignore', 'pipe', 'pipe']);
    assert.equal(Object.hasOwn(spawnOptions.env, environmentKey), false);
    assert.equal(JSON.stringify(spawnArguments).includes('do-not-inherit-this-value'), false);

    const sentBeforeRejections = child.requests.length;
    await assert.rejects(collector.send('Storage.getCookies'), /qa_command_rejected/u);
    await assert.rejects(collector.send('Page.navigate', { url: 'https://unlisted.example/' }, 'session-private-id'), /qa_command_rejected/u);
    assert.equal(child.requests.length, sentBeforeRejections);

    const contexts = await collector.send('Target.getBrowserContexts');
    assert.equal(Object.keys(contexts).length, 0);
    const createdContext = await collector.send('Target.createBrowserContext', { disposeOnDetach: true });
    assert.equal(Object.keys(createdContext).length, 0);
    await collector.send('Target.getBrowserContexts');
    await collector.send('Target.createTarget', { url: 'about:blank', browserContextId: 'context-private-id' });
    await collector.send('Target.attachToTarget', { targetId: 'target-private-id', flatten: true });

    let receivedEvent;
    const unsubscribe = collector.subscribe((event) => { receivedEvent = event; });
    child.emitEvent({ method: 'Network.responseReceived', sessionId: 'session-private-id', params: { requestId: 'request-private-id' } });
    assert.equal(receivedEvent.method, 'Network.responseReceived');
    unsubscribe();

    const projected = await collector.send(
      'Runtime.evaluate',
      { expression: 'read-only-expression', returnByValue: true },
      'session-private-id',
      (result) => {
        assert.equal(result.result.type, 'string');
        assert.equal(result.result.value, 'private raw page text');
        return { resultKind: 'projected' };
      },
    );
    assert.deepEqual(projected, { resultKind: 'projected' });

    const error = await collector.send('Page.getFrameTree', {}, 'session-private-id').then(
      () => null,
      (caught) => caught,
    );
    assert.match(error.message, /^qa_cdp_command_failed$/u);
    assert.equal(error.message.includes('private'), false);

    const setCookieRequestsBefore = child.requests.filter((request) => request.method === 'Network.setCookies').length;
    const partitioned = [cookie('auth_token'), { ...cookie('ct0'), partitionKey: null }];
    await assert.rejects(collector.send('Network.setCookies', { cookies: partitioned }, 'session-private-id'), /qa_command_rejected/u);
    await assert.rejects(collector.send('Network.setCookies', {
      cookies: [cookie('auth_token', 'v'.repeat(9000)), cookie('ct0')],
    }, 'session-private-id'), /qa_command_rejected/u);
    const nearLimitCookies = [cookieAtSize('auth_token', 8191), cookieAtSize('ct0', 8191)];
    assert.ok(nearLimitCookies.every((row) => Buffer.byteLength(JSON.stringify(row), 'utf8') <= 8192));
    assert.ok(Buffer.byteLength(JSON.stringify(nearLimitCookies), 'utf8') > 16 * 1024);
    await assert.rejects(collector.send('Network.setCookies', { cookies: nearLimitCookies }, 'session-private-id'), /qa_command_rejected/u);
    assert.equal(child.requests.filter((request) => request.method === 'Network.setCookies').length, setCookieRequestsBefore);

    await assert.rejects(collector.send('Network.setCookies', {
      cookies: [cookie('auth_token'), cookie('ct0')],
    }), /qa_command_rejected/u);
    const authToken = 'private-auth-token-value';
    const csrfToken = 'private-csrf-token-value';
    const seeded = await collector.send('Network.setCookies', {
      cookies: [cookie('auth_token', authToken), cookie('ct0', csrfToken)],
    }, 'session-private-id');
    assert.equal(Object.keys(seeded).length, 0);
    const seedRequest = child.requests.filter((request) => request.method === 'Network.setCookies');
    assert.equal(seedRequest.length, 1);
    assert.equal(seedRequest[0].sessionId, 'session-private-id');
    assert.deepEqual(seedRequest[0].params.cookies.map(({ name }) => name), ['auth_token', 'ct0']);
    assert.equal(seedRequest[0].params.cookies[0].value, authToken);
    assert.equal(seedRequest[0].params.cookies[1].value, csrfToken);
    const seedWire = child.wireBuffers.at(-1);
    assert.ok(seedWire.every((byte) => byte === 0), 'outgoing credential buffer is zeroed after write');
    await assert.rejects(collector.send('Network.setCookies', {
      cookies: [cookie('auth_token'), cookie('ct0')],
    }, 'session-private-id'), /qa_command_rejected/u);
    assert.equal(child.requests.filter((request) => request.method === 'Network.setCookies').length, 1);

    await collector.send('Target.disposeBrowserContext', { browserContextId: 'context-private-id' });
    await collector.close();
    assert.equal(child.requests.at(-1).method, 'Browser.close');
    assert.equal((await stat(profile)).isDirectory(), true, 'collector leaves profile cleanup to the Job owner');
    assert.equal((await stat(buildArtifact)).isFile(), true, 'collector leaves other owned-root content untouched');
  } finally {
    if (oldEnvironmentValue === undefined) delete process.env[environmentKey];
    else process.env[environmentKey] = oldEnvironmentValue;
  }
});

test('fails closed on mismatched response sessions or reflected envelope fields', async (t) => {
  const root = await makeRoot(t);
  let child;
  const collector = createCollector(root, (_executable, _args, _options) => {
    child = new FakeChrome((request, current) => {
      current.stdio[4].write(Buffer.from(`${JSON.stringify({
        id: request.id,
        sessionId: 'wrong-private-session',
        result: {},
        reflected: 'private response content',
      })}\0`, 'utf8'));
    });
    return child;
  });

  await collector.launch();
  const caught = await collector.send('Page.getFrameTree', {}, 'session-private-id').then(
    () => null,
    (error) => error,
  );
  assert.match(caught.message, /^qa_transport_failed$/u);
  assert.equal(caught.message.includes('private'), false);
  child.emit('exit', 0, null);
});

test('requires the caller to inject the existing CDP frame decoder', async (t) => {
  const root = await makeRoot(t);
  assert.throws(() => createQAFeasibilityCollector({
    executable: process.execPath,
    root,
    spawnChrome: () => { throw new Error('must not spawn'); },
  }), /qa_configuration_rejected/u);
});

test('accepts the fixed bound bootstrap expression within a bounded whitelist limit', async (t) => {
  const root = await makeRoot(t);
  assert.equal(QA_BOUND_BOOTSTRAP_EXPRESSION.length, 8646);
  assert.ok(QA_BOUND_BOOTSTRAP_EXPRESSION.length > 8192);
  assert.ok(QA_BOUND_BOOTSTRAP_EXPRESSION.length <= 16 * 1024);
  assert.doesNotThrow(() => createQAFeasibilityCollector({
    executable: process.execPath,
    root,
    FrameDecoder: CDPFrameDecoder,
    allowedExpressions: [QA_BOUND_BOOTSTRAP_EXPRESSION],
  }));
  assert.throws(() => createQAFeasibilityCollector({
    executable: process.execPath,
    root,
    FrameDecoder: CDPFrameDecoder,
    allowedExpressions: ['x'.repeat(16 * 1024 + 1)],
  }), /qa_configuration_rejected/u);
});

