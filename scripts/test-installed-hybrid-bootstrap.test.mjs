import test from 'node:test';
import assert from 'node:assert/strict';
import {createServer} from 'node:net';
import {parseArguments, runStopTestRestore, validateOriginalProjection, sameOriginalProjection, validateCandidateBridge, inspectWindowsListeners} from './test-installed-hybrid-bootstrap.mjs';

test('Windows listener inspection distinguishes an owned TCP listener from an available empty result',
  {skip: process.platform !== 'win32', timeout: 30_000}, async () => {
    const server = createServer();
    await new Promise((resolve, reject) => { server.once('error', reject); server.listen(0, '127.0.0.1', resolve); });
    const port = server.address().port;
    try {
      const occupied = await inspectWindowsListeners(port);
      assert.equal(occupied.available, true);
      assert.ok(occupied.listeners.some(value => value.pid === process.pid));
    } finally {
      await new Promise((resolve, reject) => server.close(error => error ? reject(error) : resolve()));
    }
    const released = await inspectWindowsListeners(port);
    assert.equal(released.available, true);
    assert.deepEqual(released.listeners, []);
  });

const originalOrigin = `chrome-extension://${'a'.repeat(32)}/`;
const originalSnapshot = {settingsSha256: 'abc', configuredBridgeOrigin: originalOrigin,
  heartbeatOrigin: null, identityEvidence: 'registered_config_only'};

test('heartbeat origin normalization accepts the server contract and rejects paths or other identities', () => {
  const actual = {extensionOrigin: originalOrigin.slice(0, -1),
    sourceAccess: {grantedSources: [], sources: []}};
  const projection = validateOriginalProjection({settings: {}}, {bridge: {compatible: true, actual}}, originalOrigin);
  assert.equal(projection.heartbeatOrigin, originalOrigin);
  assert.equal(projection.identityEvidence, 'heartbeat_and_registered_config_match');
  assert.equal(validateCandidateBridge({bridge: {compatible: true, actual}}, originalOrigin).extensionOrigin, originalOrigin);
  for (const origin of [undefined, `${originalOrigin}path`, `${originalOrigin}?x=1`, `chrome-extension://${'b'.repeat(32)}`]) {
    assert.throws(() => validateCandidateBridge({bridge: {compatible: true, actual: {...actual, extensionOrigin: origin}}}, originalOrigin),
      {code: 'candidate_production_bridge_identity_mismatch'});
  }
});

test('bootstrap remains a preflight plan unless both bounded-run approvals are present', () => {
  const plan = parseArguments([]);
  assert.equal(plan.allowRuntimeStop, false);
  assert.equal(plan.allowForeground, false);
  assert.throws(() => parseArguments(['--allow-runtime-stop']), {code: 'both_runtime_and_foreground_approval_required'});
  assert.throws(() => parseArguments(['--allow-foreground']), {code: 'both_runtime_and_foreground_approval_required'});
  const approved = parseArguments(['--allow-runtime-stop', '--allow-foreground']);
  assert.equal(approved.allowRuntimeStop, true);
  assert.equal(approved.allowForeground, true);
});

test('candidate release is proven before original runtime restoration', async () => {
  const sequence = [];
  const snapshot = originalSnapshot;
  const report = await runStopTestRestore({
    snapshotOriginal: async () => { sequence.push('snapshot-before'); return snapshot; },
    stopOriginal: async () => { sequence.push('stop'); },
    waitOriginalStopped: async () => { sequence.push('wait-stopped'); return true; },
    runCandidate: async () => { sequence.push('candidate-smoke-and-cooperative-stop'); return {grantedSourceCount: 0}; },
    verifyCandidateReleased: async () => { sequence.push('verify-candidate-released'); return true; },
    restoreOriginal: async () => { sequence.push('restore'); return {restored: true}; },
    verifyOriginalRestored: async () => { sequence.push('snapshot-after'); return snapshot; },
  });
  assert.deepEqual(sequence, ['snapshot-before', 'stop', 'wait-stopped', 'candidate-smoke-and-cooperative-stop',
    'verify-candidate-released', 'restore', 'snapshot-after']);
  assert.equal(report.runtimeStopIssued, true);
  assert.equal(report.restored, true);
  assert.equal(report.restoreVerified, true);
});

test('an owned candidate process or profile owner blocks original restore', async () => {
  const sequence = [];
  const report = await runStopTestRestore({
    snapshotOriginal: async () => originalSnapshot,
    stopOriginal: async () => { sequence.push('stop'); },
    waitOriginalStopped: async () => true,
    runCandidate: async () => { sequence.push('candidate'); throw Object.assign(new Error('bounded probe failed'), {code: 'candidate_bootstrap_timeout'}); },
    verifyCandidateReleased: async () => { sequence.push('drain-check'); return false; },
    restoreOriginal: async () => { sequence.push('restore'); return {restored: true}; },
    verifyOriginalRestored: async () => originalSnapshot,
  });
  assert.deepEqual(sequence, ['stop', 'candidate', 'drain-check']);
  assert.equal(report.failureCode, 'candidate_bootstrap_timeout');
  assert.equal(report.restoreBlocked, true);
  assert.equal(report.restoreReason, 'candidate_process_or_profile_owner_remains');
  assert.equal(report.restored, false);
});

test('a failed stop attempt still runs release verification and restoration', async () => {
  const sequence = [];
  const report = await runStopTestRestore({
    snapshotOriginal: async () => originalSnapshot,
    stopOriginal: async () => { sequence.push('stop'); throw Object.assign(new Error('stop was issued'), {code: 'supervisor_stop_failed'}); },
    waitOriginalStopped: async () => false,
    runCandidate: async () => { sequence.push('candidate'); },
    verifyCandidateReleased: async () => { sequence.push('drain-check'); return true; },
    restoreOriginal: async () => { sequence.push('restore'); return {restored: true}; },
    verifyOriginalRestored: async () => originalSnapshot,
  });
  assert.deepEqual(sequence, ['stop', 'drain-check', 'restore']);
  assert.equal(report.runtimeStopIssued, true);
  assert.equal(report.failureCode, 'supervisor_stop_failed');
  assert.equal(report.restoreVerified, true);
});

test('restore projection changes are reported as failed verification', async () => {
  const report = await runStopTestRestore({
    snapshotOriginal: async () => ({...originalSnapshot, settingsSha256: 'before'}),
    stopOriginal: async () => {}, waitOriginalStopped: async () => true,
    runCandidate: async () => ({grantedSourceCount: 0}), verifyCandidateReleased: async () => true,
    restoreOriginal: async () => ({restored: true}),
    verifyOriginalRestored: async () => ({...originalSnapshot, settingsSha256: 'after'}),
  });
  assert.equal(report.restored, true);
  assert.equal(report.restoreVerified, false);
  assert.equal(report.failureCode, 'original_projection_restore_mismatch');
});

test('legacy compatible Bridge without heartbeat origin is recorded as config-only identity evidence', () => {
  const projection = validateOriginalProjection({settings: {reasoningProvider: 'fixture'}},
    {bridge: {compatible: true, actual: {state: 'ready'}}}, originalOrigin);
  assert.equal(projection.configuredBridgeOrigin, originalOrigin);
  assert.equal(projection.heartbeatOrigin, null);
  assert.equal(projection.identityEvidence, 'registered_config_only');
});

test('original Bridge projection rejects mismatched heartbeat identity and invalid configuration origin', () => {
  assert.throws(() => validateOriginalProjection({settings: {}},
    {bridge: {compatible: true, actual: {extensionOrigin: `chrome-extension://${'b'.repeat(32)}/`}}}, originalOrigin),
  {code: 'original_projection_unverified'});
  assert.throws(() => validateOriginalProjection({settings: {}},
    {bridge: {compatible: true, actual: {}}}, 'chrome-extension://invalid/'),
  {code: 'original_projection_unverified'});
});

test('restore comparison preserves settings, configured Bridge origin, and heartbeat origin presence', () => {
  assert.equal(sameOriginalProjection(originalSnapshot, {...originalSnapshot}), true);
  assert.equal(sameOriginalProjection(originalSnapshot, {...originalSnapshot, heartbeatOrigin: originalOrigin}), false);
  assert.equal(sameOriginalProjection(originalSnapshot, {...originalSnapshot, configuredBridgeOrigin: `chrome-extension://${'b'.repeat(32)}/`}), false);
});
