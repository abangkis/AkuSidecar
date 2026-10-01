import test from 'node:test';
import assert from 'node:assert/strict';
import {PassThrough} from 'node:stream';
import {fileURLToPath} from 'node:url';
import {createBorrowedChrome, createBorrowedRPC} from '../borrowed.mjs';
import {runWorker, validateBorrowedInit} from '../worker.mjs';

test('borrowed pages expose source-scoped operations without Chrome ownership', async () => {
  const calls = [];
  const browser = createBorrowedChrome(async (...args) => {
    calls.push(args);
    return args[1] === 'Runtime.evaluate' ? {result: {value: 42}} : {};
  }, {product: 'fixture'});
  const page = await browser.forSource('x');
  assert.equal(await page.evaluate('21*2'), 42);
  await page.navigate('https://x.com/home');
  await assert.rejects(page.send('Browser.close'));
  await assert.rejects(browser.forSource('linkedin'));
  assert.deepEqual(calls.map(call => call.slice(0, 2)), [['x', 'Runtime.evaluate'], ['x', 'Page.navigate']]);
  assert.equal('pid' in browser, false);
  await browser.close();
  await assert.rejects(page.evaluate('1'));
  assert.equal(calls.length, 2);
});

test('late borrowed RPC reply cannot resolve a later operation', async () => {
  const sent = [];
  const rpc = createBorrowedRPC(async message => { sent.push(message); });
  await assert.rejects(rpc.send('x', 'Runtime.evaluate', {}, 5), {code: 'capture_timeout'});
  const next = rpc.send('facebook', 'Runtime.evaluate', {}, 1000);
  rpc.receive({type: 'cdp_result', rpcId: sent[0].rpcId, ok: true, result: {stale: true}});
  rpc.receive({type: 'cdp_result', rpcId: sent[1].rpcId, ok: true, result: {fresh: true}});
  assert.deepEqual(await next, {fresh: true});
  rpc.close();
  await assert.rejects(rpc.send('x', 'Runtime.evaluate', {}));
});

test('Quiet worker initializes and shuts down with host metadata and no Chrome launch paths', async () => {
  const input = new PassThrough();
  const output = new PassThrough();
  const lines = [];
  let buffer = '';
  output.setEncoding('utf8');
  output.on('data', chunk => {
    buffer += chunk;
    let end;
    while ((end = buffer.indexOf('\n')) >= 0) {
      lines.push(JSON.parse(buffer.slice(0, end)));
      buffer = buffer.slice(end + 1);
    }
  });
  const done = runWorker({input, output, errorOutput: null});
  const bridgePath = fileURLToPath(new URL('../../../../../../AkuBridge/', import.meta.url));
  input.write(JSON.stringify({id: 1, type: 'init', backend: 'browser_quiet_hidden', bridgePath, chromeVersion: {product: 'fixture'}}) + '\n');
  const until = Date.now() + 2000;
  while (lines.length === 0 && Date.now() < until) await new Promise(resolve => setTimeout(resolve, 5));
  assert.equal(lines[0]?.ok, true);
  assert.equal(lines[0].result.workerDriver.name, 'aku-quiet-worker');
  assert.equal('pid' in lines[0].result, false);
  input.write(JSON.stringify({id: 2, type: 'shutdown'}) + '\n');
  await done;
  assert.deepEqual(lines[1], {id: 2, ok: true, result: {stopped: true}});
  assert.throws(() => validateBorrowedInit({backend: 'browser_quiet_hidden', bridgePath, chrome: 'C:/chrome.exe', chromeVersion: {}}));
});
