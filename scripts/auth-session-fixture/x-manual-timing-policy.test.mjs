import test from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {MANUAL_TIMING,parseManualTimingPolicy} from './x-manual-timing-policy.mjs';

const policyURL = new URL('../../internal/engine/x-manual-timing-policy.json', import.meta.url);
const source = readFileSync(fileURLToPath(policyURL), 'utf8');
const clone = () => JSON.parse(source);
const invalid = value => {
  const raw = typeof value === 'string' ? value : JSON.stringify(value);
  assert.throws(() => parseManualTimingPolicy(raw), error =>
    error.message === 'manual_timing_policy_invalid' && !error.message.includes('PRIVATE'));
};

test('canonical policy is closed, safe-integer validated, and immutable', () => {
  const parsed = parseManualTimingPolicy(source);
  assert.deepEqual(parsed, MANUAL_TIMING);
  assert.equal(MANUAL_TIMING.schema, 'aku.x.manual-timing.v1');
  assert.equal(MANUAL_TIMING.startupTotalMS, 18000);
  assert.equal(MANUAL_TIMING.evidenceMaxAgeMS, 6000);
  assert.equal(MANUAL_TIMING.attemptAuthorizationMaxMS, 60000);
  assert.equal(MANUAL_TIMING.collectorHomeReadyMS, 45000);
  assert.ok(Object.isFrozen(MANUAL_TIMING));
  assert.ok(Object.isFrozen(parsed));
});

test('policy parser rejects malformed, open, missing, and unsafe configuration without echoing it', () => {
  invalid('{"schema":"aku.x.manual-timing.v1","PRIVATE":"secret"');
  for (const mutate of [
    p => { p.privateToken = 'PRIVATE'; },
    p => { delete p.captureCommandMS; },
    p => { p.startupTotalMS = 1.5; },
    p => { p.startupTotalMS = Number.MAX_SAFE_INTEGER + 1; },
    p => { p.settingsFallbackMS = 600001; },
    p => { p.rendezvousMS = 0; },
    p => { p.schema = 'other'; },
  ]) {
    const value = clone(); mutate(value); invalid(value);
  }
  invalid(source.replace('"startupTotalMS": 18000', '"startupTotalMS": 18000, "startupTotalMS": 18000'));
  invalid(source + ' '.repeat(4097));
  assert.throws(() => parseManualTimingPolicy({}), /manual_timing_policy_invalid/);
});

test('policy parser enforces local startup envelopes and preserves the external ceiling', () => {
  for (const mutate of [
    p => { p.startupTotalMS = p.bootstrapObservationMS - 1; },
    p => { p.earlyCommandMS = p.cdpCommandMS; },
    p => { p.evidenceMaxAgeMS = p.startupTotalMS + 1; },
    p => { p.attemptAuthorizationMaxMS = p.startupTotalMS - 1; },
    p => { p.attemptAuthorizationMaxMS = 180000; },
    p => { p.bootstrapEvaluationMS = p.cdpCommandMS + 1; },
  ]) {
    const value = clone(); mutate(value); invalid(value);
  }
  const valid = clone();
  valid.collectorHomeReadyMS = 45000;
  assert.equal(parseManualTimingPolicy(JSON.stringify(valid)).collectorHomeReadyMS, 45000);
});
