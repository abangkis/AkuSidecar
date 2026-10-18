import assert from 'node:assert/strict';
import test from 'node:test';
import {
  TwoPhaseWorkflowGrant,
  twoPhaseChromeArgs,
} from './x-native-metadata-probe-driver.mjs';

const GOOD_NONCE = 'a'.repeat(64);

function grant(now = () => 1_000) {
  return new TwoPhaseWorkflowGrant({ owner: 7, pid: 1234, now });
}

function authorization(overrides = {}) {
  return {
    owner: 7,
    oldPID: 1234,
    nonce: GOOD_NONCE,
    exitCode: 0,
    identityUnchanged: true,
    ...overrides,
  };
}

test('two-phase Chrome arguments keep phase one standard and add only the CDP pipe in phase two', () => {
  const profile = 'C:\\qa\\run-x-two-phase-0123456789abcdef\\shared';
  const standard = [
    `--user-data-dir=${profile}`,
    '--profile-directory=Default',
    '--no-first-run',
    '--no-default-browser-check',
    '--new-window',
    'https://x.com/home',
  ];
  assert.deepEqual(twoPhaseChromeArgs(profile, false), standard);
  assert.deepEqual(twoPhaseChromeArgs(profile, true), [
    ...standard.slice(0, -1), '--remote-debugging-pipe', standard.at(-1),
  ]);
  assert.throws(() => twoPhaseChromeArgs(profile, 'true'), /xnp_chrome_args_invalid/u);
});

test('authorization requires the phase-one owner, PID, clean exit, nonce shape, and stable directories', () => {
  assert.throws(() => grant().authorize(authorization({ exitCode: null })), /xnp_phase1_exit_required/u);
  assert.throws(() => grant().authorize(authorization({ exitCode: 1 })), /xnp_phase1_exit_required/u);
  assert.throws(() => grant().authorize(authorization({ owner: 8 })), /xnp_workflow_ticket_invalid/u);
  assert.throws(() => grant().authorize(authorization({ oldPID: 4321 })), /xnp_workflow_ticket_invalid/u);
  assert.throws(() => grant().authorize(authorization({ nonce: 'not-a-64-hex-nonce' })), /xnp_workflow_ticket_invalid/u);
  assert.throws(() => grant().authorize(authorization({ identityUnchanged: false })), /xnp_owned_profile_changed/u);
});

test('authorization yields a ten-second one-use phase-two grant', () => {
  let now = 5_000;
  const current = grant(() => now);
  assert.deepEqual(current.authorize(authorization()), { authorized: true });
  assert.equal(current.consume({ identityUnchanged: true }), true);
  assert.throws(() => current.consume({ identityUnchanged: true }), /xnp_workflow_grant_replay/u);
  assert.throws(() => current.authorize(authorization({ nonce: 'b'.repeat(64) })), /xnp_workflow_grant_replay/u);

  const expired = grant(() => now);
  expired.authorize(authorization());
  now += 10_000;
  assert.throws(() => expired.consume({ identityUnchanged: true }), /xnp_workflow_grant_expired/u);
  assert.throws(() => expired.consume({ identityUnchanged: true }), /xnp_workflow_grant_replay/u);
});

test('invalid or backward clock readings consume and disable the grant', () => {
  const invalid = grant(() => Number.NaN);
  assert.throws(() => invalid.authorize(authorization()), /xnp_workflow_clock_invalid/u);
  assert.throws(() => invalid.authorize(authorization()), /xnp_workflow_grant_replay/u);

  let now = 5_000;
  const regressed = grant(() => now);
  regressed.authorize(authorization());
  now = 4_999;
  assert.throws(() => regressed.consume({ identityUnchanged: true }), /xnp_workflow_clock_regressed/u);
  assert.throws(() => regressed.consume({ identityUnchanged: true }), /xnp_workflow_grant_replay/u);

  let invalidAfterGrant = 8_000;
  const consumedOnNaN = grant(() => invalidAfterGrant);
  consumedOnNaN.authorize(authorization());
  invalidAfterGrant = Number.NaN;
  assert.throws(() => consumedOnNaN.consume({ identityUnchanged: true }), /xnp_workflow_clock_invalid/u);
  assert.throws(() => consumedOnNaN.consume({ identityUnchanged: true }), /xnp_workflow_grant_replay/u);
});

test('replaced profile/root identity blocks authorization or phase-two consumption', () => {
  const beforeGrant = grant();
  assert.throws(() => beforeGrant.authorize(authorization({ identityUnchanged: false })), /xnp_owned_profile_changed/u);

  const afterGrant = grant();
  afterGrant.authorize(authorization());
  assert.throws(() => afterGrant.consume({ identityUnchanged: false }), /xnp_owned_profile_changed/u);
  assert.throws(() => afterGrant.consume({ identityUnchanged: true }), /xnp_workflow_grant_replay/u);
});
