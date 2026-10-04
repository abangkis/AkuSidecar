import test from 'node:test';
import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { waitForExitOrTimeout } from '../chrome.mjs';

test('Chrome exit cancels the losing shutdown timer so Node can exit', async () => {
  const moduleURL = new URL('../chrome.mjs', import.meta.url).href;
  const script = `import {waitForExitOrTimeout} from ${JSON.stringify(moduleURL)}; await waitForExitOrTimeout(Promise.resolve(), 10000); console.log('closed');`;
  // The child must naturally exit; an uncancelled 10s timeout exceeds this bound.
  const { stdout } = await promisify(execFile)(process.execPath, ['--input-type=module', '-e', script], { timeout: 3000, windowsHide: true });
  assert.equal(stdout.trim(), 'closed');
});

test('shutdown remains bounded when Chrome does not exit', async () => {
  await waitForExitOrTimeout(new Promise(() => {}), 10);
});
