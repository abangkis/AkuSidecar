import {readFileSync} from 'node:fs';
import {fileURLToPath} from 'node:url';
import {parseStrictJSON} from './x-evidence-ingress.mjs';

const POLICY_PATH = fileURLToPath(new URL('../../internal/engine/x-manual-timing-policy.json', import.meta.url));
const POLICY_KEYS = Object.freeze([
  'schema', 'startupTotalMS', 'evidenceMaxAgeMS', 'cdpCommandMS', 'earlyCommandMS',
  'bootstrapObservationMS', 'bootstrapEvaluationMS', 'rendezvousMS', 'settingsFallbackMS',
  'collectorHomeReadyMS', 'collectorExitMS', 'ownerCleanupMS', 'captureCleanupMS',
  'captureCommandMS', 'attemptAuthorizationMaxMS',
]);
const TIMING_KEYS = POLICY_KEYS.slice(1);
// Collector home readiness is a separate preflight, completed before the paired
// startup budget begins, so it has its own 45-second bound.
const LOCAL_STARTUP_ENVELOPES = Object.freeze([
  'evidenceMaxAgeMS', 'cdpCommandMS', 'earlyCommandMS', 'bootstrapObservationMS', 'bootstrapEvaluationMS',
  'rendezvousMS', 'settingsFallbackMS', 'collectorExitMS', 'ownerCleanupMS',
  'captureCleanupMS', 'captureCommandMS',
]);
const invalidPolicy = () => new Error('manual_timing_policy_invalid');

export function parseManualTimingPolicy(source) {
  let input;
  try {
    if (typeof source !== 'string' || Buffer.byteLength(source, 'utf8') > 4096) throw invalidPolicy();
    input = parseStrictJSON(source, {maxJSONDepth: 2, maxJSONTokens: 64});
  } catch {
    throw invalidPolicy();
  }

  if (input === null || typeof input !== 'object' || Array.isArray(input)) throw invalidPolicy();
  const keys = Object.keys(input);
  if (keys.length !== POLICY_KEYS.length || keys.some(key => !POLICY_KEYS.includes(key)) ||
      input.schema !== 'aku.x.manual-timing.v1') throw invalidPolicy();
  for (const key of TIMING_KEYS) {
    if (!Number.isSafeInteger(input[key]) || input[key] < 1 || input[key] > 600000) throw invalidPolicy();
  }
  if (input.startupTotalMS < Math.max(...LOCAL_STARTUP_ENVELOPES.map(key => input[key])) ||
      input.earlyCommandMS <= input.cdpCommandMS ||
      input.bootstrapEvaluationMS > input.cdpCommandMS ||
      input.evidenceMaxAgeMS > input.startupTotalMS ||
      input.startupTotalMS > input.attemptAuthorizationMaxMS ||
      input.attemptAuthorizationMaxMS !== 60000) throw invalidPolicy();

  return Object.freeze(Object.fromEntries(POLICY_KEYS.map(key => [key, input[key]])));
}

export const MANUAL_TIMING = parseManualTimingPolicy(readFileSync(POLICY_PATH, 'utf8'));
