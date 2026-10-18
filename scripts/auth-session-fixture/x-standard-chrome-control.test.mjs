import test from 'node:test';
import assert from 'node:assert/strict';
import { resolve } from 'node:path';
import { decodeControl, standardChromeArgs } from './x-standard-chrome-control.mjs';

test('standard control launches only the new profile and normal Home window', () => {
  const profile = resolve('runtime/qa/auth-session-sidecar/control-profile');
  const args = standardChromeArgs(profile);
  assert.deepEqual(args, [`--user-data-dir=${profile}`, '--profile-directory=Default', '--no-first-run',
    '--no-default-browser-check', '--new-window', 'https://x.com/home']);
  assert.ok(args.every(value => !/remote-debugging|automation|disable-(?:web-security|blink)/u.test(value)));
  assert.throws(() => standardChromeArgs('relative-profile'), /invalid_profile/u);
});

test('launch-only control rejects oversized, malformed and duplicate-key input', () => {
  assert.deepEqual(JSON.parse(JSON.stringify(decodeControl(Buffer.from('{"op":"fixture_bootstrap","withholdReady":false}')))),
    { op: 'fixture_bootstrap', withholdReady: false });
  for (const value of [Buffer.alloc(4097), Buffer.from([255]), Buffer.from('{"op":1,"op":2}'), Buffer.from('[]')]) {
    assert.throws(() => decodeControl(value), /invalid_control/u);
  }
});
