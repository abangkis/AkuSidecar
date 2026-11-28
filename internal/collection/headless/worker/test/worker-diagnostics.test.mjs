import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { PassThrough, Writable } from 'node:stream';
import test from 'node:test';
import { fileURLToPath } from 'node:url';
import { createWorkerDiagnostics } from '../worker-diagnostics.mjs';

test('worker process keeps errors on stderr and only RPC replies on stdout', () => {
  const script = fileURLToPath(new URL('../worker.mjs', import.meta.url));
  const input = ['not JSON: private', JSON.stringify({ id: 'private-id', type: 'capture' }),
    JSON.stringify({ id: 2, type: 'shutdown' })].join('\n') + '\n';
  const child = spawnSync(process.execPath, [script], { input, encoding: 'utf8', windowsHide: true, timeout: 10000 });
  assert.ifError(child.error);
  assert.equal(child.status, 0);
  const replies = child.stdout.trim().split('\n').map(line => JSON.parse(line));
  assert.deepEqual(replies.map(row => [row.id, row.error?.code, row.ok]), [
    [null, 'invalid_json', false], ['private-id', 'not_initialized', false], [2, undefined, true],
  ]);
  assert.equal(replies[2].result.stopped, true);
  const diagnostics = child.stderr.trim().split('\n').map(line => JSON.parse(line));
  assert.deepEqual(diagnostics, ['invalid_json', 'not_initialized'].map((reason, index) => ({
    operation: 'headless_worker', phase: 'request_failed', sequence: index + 1, level: 'error', reason,
  })));
});

test('uncatalogued errors are redacted and a failed diagnostic sink is contained', async () => {
  const output = new PassThrough();
  const report = createWorkerDiagnostics(output);
  report('idle_close', { code: 'private-code', message: 'private-message' });
  const row = JSON.parse(output.read().toString());
  assert.equal(row.reason, 'worker_error');
  assert.equal(row.phase, 'idle_close');
  assert.doesNotMatch(JSON.stringify(row), /private/);
  const failed = new Writable({ write(chunk, encoding, callback) { callback(Error('broken sink')); } });
  createWorkerDiagnostics(failed)('fatal', Error('private'));
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(failed.destroyed, true);
});
