// Private startup transport projection, not a public QA report or credential API.
// The injected runner is trusted owned-driver code. No runner is registered by
// importing this module. Native/OS provenance must be selected by the Go owner,
// never inferred from a caller's JSON, this schema or successful offline tests.
import { performance } from 'node:perf_hooks';
import { QA_BOUND_QUALIFICATION } from './x-qa-bound-observation.mjs';

export const MANUAL_STARTUP_SCHEMA = 'aku.x.manual-startup-receipt.v1';
const REQUEST = ['challenge', 'role', 'ownerId', 'profileId', 'contextId', 'targetId', 'frameId', 'documentId'];
const SAMPLE = ['accountId', 'ownerId', 'profileId', 'contextId', 'targetId', 'frameId', 'documentId',
  'settingsRequestId', 'identityAt', 'settingsAt', 'qualification'];
const fail = () => { throw new Error('manual_startup_unavailable'); };
const ref = v => typeof v === 'string' && v.length > 0 && Buffer.byteLength(v, 'utf8') <= 512 && !/[\u0000-\u001f\u007f]/u.test(v);

function record(input, fields) {
  if (!input || typeof input !== 'object' || ![Object.prototype, null].includes(Object.getPrototypeOf(input))) fail();
  const descriptors = Object.getOwnPropertyDescriptors(input), keys = Reflect.ownKeys(descriptors);
  if (keys.length !== fields.length || keys.some(k => !fields.includes(k))) fail();
  const result = Object.create(null);
  for (const name of fields) {
    const d = descriptors[name];
    if (!d || !Object.hasOwn(d, 'value') || !d.enumerable) fail();
    result[name] = d.value;
  }
  return result;
}

// run(request, {signal,onPrivateStartup}) must use the fixed QABoundObservation
// recipe on the actual already-pinned startup document. It may not echo request
// pins as observations or return an old sanitized report in place of the sink.
export function createManualStartupCapture({ run, now = () => performance.now(), wallNow = () => Date.now() }) {
  if (typeof run !== 'function' || typeof now !== 'function' || typeof wallNow !== 'function') fail();
  let used = false;
  return async (input, signal) => {
    if (used) fail();
    used = true;
    let sample = null, calls = 0, invalid = false;
    try {
      const request = Object.freeze(record(input, REQUEST));
      if (!REQUEST.every(k => ref(request[k])) || !['reader', 'collector'].includes(request.role) || signal?.aborted) fail();
      const start = now(), wall = wallNow();
      if (!Number.isFinite(start) || start < 0 || !Number.isFinite(wall) || wall < 0) fail();
      let last = start;
      const tick = () => {
        const value = now();
        if (!Number.isFinite(value) || value < last || value - start >= 2000 || signal?.aborted) fail();
        last = value;
        return value;
      };
      await run(request, { signal, onPrivateStartup: value => {
        if (++calls !== 1) { invalid = true; fail(); }
        sample = record(value, SAMPLE);
        tick();
      } });
      const end = tick();
      if (!sample || invalid || typeof sample.accountId !== 'string' || !/^[1-9][0-9]{0,19}$/.test(sample.accountId) ||
          !SAMPLE.filter(k => !['identityAt', 'settingsAt'].includes(k)).every(k => ref(sample[k])) ||
          REQUEST.filter(k => !['challenge', 'role'].includes(k)).some(k => sample[k] !== request[k]) ||
          sample.qualification !== QA_BOUND_QUALIFICATION || !Number.isFinite(sample.identityAt) || !Number.isFinite(sample.settingsAt) ||
          sample.identityAt < start || sample.settingsAt < sample.identityAt || sample.settingsAt > end) fail();
      const identityRequestId = sample.documentId + ':' + sample.contextId + ':bootstrap-current-self';
      if (!ref(identityRequestId) || identityRequestId === sample.settingsRequestId) fail();
      const wire = Buffer.from(JSON.stringify({ schema: MANUAL_STARTUP_SCHEMA,
        ...request, accountId: sample.accountId, identityRequestId,
        settingsRequestId: sample.settingsRequestId,
        identityObservedAtMS: wall + sample.identityAt - start,
        settingsObservedAtMS: wall + sample.settingsAt - start,
        elapsedMS: end - start, qualification: sample.qualification,
      }), 'utf8');
      if (wire.byteLength > 16384) { wire.fill(0); fail(); }
      return wire;
    } catch { fail(); }
    finally { sample = null; }
  };
}
