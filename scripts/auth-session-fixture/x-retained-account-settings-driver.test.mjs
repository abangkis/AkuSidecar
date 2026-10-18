import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import test from 'node:test';

function control(wrapper, messages) {
  const result = spawnSync(process.execPath, [fileURLToPath(new URL(wrapper, import.meta.url)),
    process.execPath, process.cwd(), 'http://127.0.0.1:12345'], {
    input: [{ op: 'fixture_bootstrap', withholdReady: false }, ...messages].map(value => JSON.stringify(value)).join('\n') + '\n',
    encoding: 'utf8', timeout: 3000, maxBuffer: 65536, windowsHide: true,
  });
  assert.equal(result.error, undefined);
  assert.equal(result.status, 0);
  assert.equal(result.stderr, '');
  return result.stdout.trim().split('\n').map(line => JSON.parse(line));
}

test('retained driver rejects launch without an owner-issued scope before touching Chrome', () => {
  const replies = control('./x-retained-account-settings-driver.mjs', [
    { id: 1, op: 'launch', role: 'collector', nativeReaderStartup: false },
  ]);
  assert.deepEqual(replies[0], { ready: true });
  assert.equal(replies[1].ok, false);
  assert.equal(replies[1].code, 'retained_scope_required');
});

test('retained scope rejects malformed nonce and reuse flag before filesystem access', () => {
  for (const scope of [{ nonce: 'short', reuse: true }, { nonce: 'a'.repeat(64), reuse: 'true' }]) {
    const replies = control('./x-retained-account-settings-driver.mjs', [{ id: 1, op: 'qa_profile_scope', ...scope }]);
    assert.equal(replies[1].ok, false);
    assert.equal(replies[1].code, 'retained_scope_invalid');
  }
});

test('ordinary two-phase driver cannot acquire the retained-profile exception', () => {
  const replies = control('./x-two-phase-account-settings-driver.mjs', [
    { id: 1, op: 'qa_profile_scope', nonce: 'a'.repeat(64), reuse: true },
  ]);
  assert.equal(replies[1].ok, false);
  assert.equal(replies[1].code, 'retained_scope_invalid');
});

test('bootstrap entrypoint requires existing-profile scope and cannot start Network observation', () => {
  const replies = control('./x-retained-bootstrap-driver.mjs', [
    { id: 1, op: 'launch', role: 'collector', nativeReaderStartup: false },
    { id: 2, op: 'qa_profile_scope', nonce: 'a'.repeat(64), reuse: false },
    { id: 3, op: 'start_observation', owner: 1 },
    { id: 4, op: 'bootstrap_current_self', owner: 1 },
  ]);
  assert.deepEqual(replies.slice(1).map(value => value.code), [
    'retained_scope_required', 'retained_profile_required', 'network_observation_not_available', 'bootstrap_mode_required',
  ]);
  assert.ok(replies.slice(1).every(value => value.ok === false));
});

test('account-settings entrypoint cannot request a bootstrap probe or pass an expression', () => {
  const replies = control('./x-retained-account-settings-driver.mjs', [
    { id: 1, op: 'bootstrap_current_self', owner: 1 },
    { id: 2, op: 'bootstrap_current_self', owner: 1, expression: 'private' },
    { id: 3, op: 'bootstrap_early_current_self', owner: 1 },
  ]);
  assert.deepEqual(replies.slice(1).map(value => value.code), ['bootstrap_mode_required', 'rpc_fields_invalid', 'bootstrap_mode_required']);
});

test('early bootstrap rejects pre-owner calls and caller-provided timing or scripts', () => {
  const replies = control('./x-retained-bootstrap-driver.mjs', [
    { id: 1, op: 'bootstrap_early_current_self', owner: 1 },
    { id: 2, op: 'bootstrap_early_current_self', owner: 1, expression: 'private' },
    { id: 3, op: 'bootstrap_early_current_self', owner: 1, maxPauses: 9999 },
    { id: 4, op: 'bootstrap_early_diagnostics', owner: 1 },
  ]);
  assert.deepEqual(replies.slice(1).map(value => value.code), ['bootstrap_mode_required', 'rpc_fields_invalid', 'rpc_fields_invalid', 'early_diagnostics_unavailable']);
});
