// This module is a bounded in-memory experiment harness. Its callbacks are
// trusted synthetic-fixture adapters; it has no browser, network, or Store API.
export const X_CONCURRENCY_HARNESS_SCHEMA = 'aku.x.concurrency-harness.v1';
export const X_CONCURRENCY_SYNTHETIC_PROVENANCE = 'synthetic-fixture';
export const X_CONCURRENCY_PROBE_SCHEMA = 'aku.x.concurrency-probe.v1';
export const X_CONCURRENCY_OWNER_SCHEMA = 'aku.x.synthetic-owner-binding.v1';
export const X_CONCURRENCY_EVIDENCE_SCHEMA = 'aku.x.synthetic-reader-evidence.v1';
export const X_CONCURRENCY_COVERAGE_SCHEMA = 'aku.x.synthetic-coverage.v1';

const MAX_TOTAL_MS = 60_000;
const MAX_FRESHNESS_MS = 5_000;
const MAX_MANIFEST_TUPLES = 16;
const MAX_TUPLE_BYTES = 8_192;
const MAX_TOTAL_COOKIE_BYTES = 65_536;
const MAX_WINDOWS = 16;
const MAX_USED_LEASES = 1_024;
const LOOPBACK_HOST = '127.0.0.1';
const COOKIE_ATTRIBUTES = Object.freeze([
  'name', 'domain', 'path', 'hostOnly', 'secure', 'httpOnly', 'sameSite', 'expires', 'partitionKey',
]);
const COOKIE_FIELDS = Object.freeze(['name', 'value', ...COOKIE_ATTRIBUTES.slice(1)]);
const ADAPTER_FIELDS = Object.freeze([
  'subscribeObservation', 'inspectReader', 'createCollector', 'readCookieTuples',
  'seedCollector', 'readProbe', 'closeCollector',
]);
const OBSERVATION_KINDS = new Set([
  'mutation', 'identity_change', 'login_change', 'detach', 'coverage_loss', 'stale_response', 'challenge',
]);
const USED_LEASES = new Set();
const SAFE_FAILURES = new WeakMap();

function fail(code) {
  const error = new TypeError(code);
  SAFE_FAILURES.set(error, code);
  return error;
}

function plainData(value, allowed, code) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw fail(code);
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) throw fail(code);
  const descriptors = Object.getOwnPropertyDescriptors(value);
  const keys = Reflect.ownKeys(descriptors);
  if (keys.some(key => typeof key !== 'string' || !allowed.includes(key))) throw fail(code);
  for (const key of allowed) {
    const descriptor = descriptors[key];
    if (!descriptor || !Object.hasOwn(descriptor, 'value') || descriptor.get || descriptor.set) throw fail(code);
  }
  return Object.fromEntries(allowed.map(key => [key, descriptors[key].value]));
}

function boundedText(value, max = 128) {
  return typeof value === 'string' && value.length > 0 && value.length <= max &&
    Buffer.byteLength(value, 'utf8') <= max && !/[\u0000-\u001f\u007f]/u.test(value);
}

function strictArray(value, max, code, minimum = 0) {
  if (!Array.isArray(value) || value.length < minimum || value.length > max) throw fail(code);
  const descriptors = Object.getOwnPropertyDescriptors(value);
  const ownKeys = Reflect.ownKeys(descriptors);
  if (ownKeys.some(key => key !== 'length' && !/^(?:0|[1-9][0-9]*)$/u.test(String(key)))) throw fail(code);
  const items = [];
  for (let index = 0; index < value.length; index++) {
    const descriptor = descriptors[String(index)];
    if (!descriptor || !Object.hasOwn(descriptor, 'value') || descriptor.get || descriptor.set) throw fail(code);
    items.push(descriptor.value);
  }
  return items;
}

function safeOrigin(value) {
  if (typeof value !== 'string') throw fail('harness_scope_invalid');
  let parsed;
  try { parsed = new URL(value); } catch { throw fail('harness_scope_invalid'); }
  if (parsed.protocol !== 'http:' || parsed.hostname !== LOOPBACK_HOST || !parsed.port ||
      parsed.username || parsed.password || parsed.pathname !== '/' || parsed.search || parsed.hash ||
      parsed.origin !== value) throw fail('harness_scope_invalid');
  return value;
}

function validIdentifier(value) {
  return boundedText(value, 128) && /^[A-Za-z0-9._:-]+$/u.test(value);
}

function validTimestamp(value, now) {
  return Number.isFinite(value) && value >= 0 && value <= now && now - value <= MAX_FRESHNESS_MS;
}

function frozenPartition(value, origin, code) {
  if (value === null) return null;
  const part = plainData(value, ['topLevelSite', 'hasCrossSiteAncestor'], code);
  if (part.topLevelSite !== 'http://127.0.0.1' || typeof part.hasCrossSiteAncestor !== 'boolean') throw fail(code);
  return Object.freeze({ topLevelSite: part.topLevelSite, hasCrossSiteAncestor: part.hasCrossSiteAncestor });
}

function cookieAttrs(source, origin, code, withValue) {
  const fields = withValue ? COOKIE_FIELDS : COOKIE_ATTRIBUTES;
  const item = plainData(source, fields, code);
  if (typeof item.name !== 'string' || item.name.length === 0 || item.name.length > 256 ||
      Buffer.byteLength(item.name, 'utf8') > 256 || !/^[!#$%&'()+\-.0-9A-Z^_a-z|~]+$/u.test(item.name) ||
      item.name.includes('*') || item.name.includes('?') ||
      item.domain !== LOOPBACK_HOST || typeof item.path !== 'string' || !item.path.startsWith('/') ||
      Buffer.byteLength(item.path, 'utf8') > 256 || /[\u0000-\u001f\u007f*?]/u.test(item.path) ||
      typeof item.hostOnly !== 'boolean' || typeof item.secure !== 'boolean' || typeof item.httpOnly !== 'boolean' ||
      !['Strict', 'Lax', 'None', 'Unspecified'].includes(item.sameSite) ||
      !(item.expires === null || item.expires === -1 || (Number.isSafeInteger(item.expires) && item.expires >= 0))) throw fail(code);
  const partitionKey = frozenPartition(item.partitionKey, origin, code);
  if (withValue && typeof item.value !== 'string') throw fail(code);
  return Object.freeze({
    name: item.name, ...(withValue ? { value: item.value } : {}), domain: item.domain, path: item.path,
    hostOnly: item.hostOnly, secure: item.secure, httpOnly: item.httpOnly, sameSite: item.sameSite,
    expires: item.expires, partitionKey,
  });
}

function cookieKey(cookie) {
  return JSON.stringify([cookie.name, cookie.domain, cookie.path, cookie.partitionKey]);
}

function canonicalCookieBytes(cookie) {
  return Buffer.byteLength(JSON.stringify({
    name: cookie.name, value: cookie.value, domain: cookie.domain, path: cookie.path,
    hostOnly: cookie.hostOnly, secure: cookie.secure, httpOnly: cookie.httpOnly,
    sameSite: cookie.sameSite, expires: cookie.expires, partitionKey: cookie.partitionKey,
  }), 'utf8');
}

function validateManifest(input, origin) {
  const manifest = plainData(input, ['schema', 'provenance', 'origin', 'tuples'], 'harness_manifest_invalid');
  if (manifest.schema !== 'aku.x.cookie-manifest.synthetic.v1' ||
      manifest.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE || manifest.origin !== origin) {
    throw fail('harness_manifest_invalid');
  }
  const rawTuples = strictArray(manifest.tuples, MAX_MANIFEST_TUPLES, 'harness_manifest_invalid', 1);
  const tuples = rawTuples.map(item => cookieAttrs(item, origin, 'harness_manifest_invalid', false));
  const keys = tuples.map(cookieKey);
  if (new Set(keys).size !== keys.length) throw fail('harness_manifest_invalid');
  return Object.freeze({ schema: manifest.schema, provenance: manifest.provenance, origin,
    tuples: Object.freeze(tuples) });
}

function validateCoverage(input) {
  const keys = ['schema', 'provenance', 'readerStartup', 'collectorStartup', 'targetCoverage',
    'workerCoverage', 'detachCoverage', 'mutationCoverage', 'identityCoverage'];
  const coverage = plainData(input, keys, 'harness_coverage_unqualified');
  if (coverage.schema !== 'aku.x.synthetic-coverage.v1' ||
      coverage.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE ||
      keys.slice(2).some(key => coverage[key] !== true)) throw fail('harness_coverage_unqualified');
  return Object.freeze({ schema: coverage.schema, provenance: coverage.provenance,
    readerStartup: true, collectorStartup: true, targetCoverage: true, workerCoverage: true,
    detachCoverage: true, mutationCoverage: true, identityCoverage: true });
}

function validateInputs(input) {
  const fields = ['origin', 'provenance', 'ownerBinding', 'reader', 'coverage', 'manifest', 'adapters',
    'clock', 'signal', 'timeoutMs', 'cleanupWaitMs'];
  const options = plainData(input, fields, 'harness_input_invalid');
  const origin = safeOrigin(options.origin);
  if (options.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE || typeof options.clock !== 'function' ||
      !(options.signal === null || options.signal instanceof AbortSignal) ||
      !Number.isSafeInteger(options.timeoutMs) || options.timeoutMs < 1 || options.timeoutMs > MAX_TOTAL_MS ||
      !Number.isSafeInteger(options.cleanupWaitMs) || options.cleanupWaitMs < 0 ||
      options.cleanupWaitMs > options.timeoutMs) throw fail('harness_input_invalid');

  let now;
  try { now = options.clock(); } catch { throw fail('harness_clock_invalid'); }
  if (!Number.isFinite(now) || now < 0) throw fail('harness_clock_invalid');
  const owner = plainData(options.ownerBinding,
    ['schema', 'provenance', 'ownerId', 'generation', 'leaseId', 'observedAt'], 'harness_owner_binding_invalid');
  if (owner.schema !== X_CONCURRENCY_OWNER_SCHEMA || owner.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE ||
      !validIdentifier(owner.ownerId) || !validIdentifier(owner.generation) ||
      !validIdentifier(owner.leaseId) || !validTimestamp(owner.observedAt, now)) throw fail('harness_owner_binding_invalid');
  const reader = plainData(options.reader, ['schema', 'provenance', 'origin', 'ownerId', 'generation',
    'profileId', 'contextId', 'accountId', 'probeSchema', 'authState', 'observedAt', 'windowIds'],
  'harness_evidence_invalid');
  if (reader.schema !== X_CONCURRENCY_EVIDENCE_SCHEMA ||
      reader.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE || reader.origin !== origin ||
      reader.ownerId !== owner.ownerId || reader.generation !== owner.generation ||
      !validIdentifier(reader.profileId) || !validIdentifier(reader.contextId) || !validIdentifier(reader.accountId) ||
      reader.probeSchema !== X_CONCURRENCY_PROBE_SCHEMA || reader.authState !== 'authenticated' ||
      !validTimestamp(reader.observedAt, now)) throw fail('harness_evidence_invalid');
  const windowIds = strictArray(reader.windowIds, MAX_WINDOWS, 'harness_evidence_invalid', 1);
  if (windowIds.some(id => !validIdentifier(id)) || new Set(windowIds).size !== windowIds.length) {
    throw fail('harness_evidence_invalid');
  }
  const safeReader = Object.freeze({ ...reader, windowIds: Object.freeze([...windowIds]) });
  const manifest = validateManifest(options.manifest, origin);
  const coverage = validateCoverage(options.coverage);
  const adapterInput = plainData(options.adapters, ADAPTER_FIELDS, 'harness_adapter_invalid');
  if (ADAPTER_FIELDS.some(key => typeof adapterInput[key] !== 'function')) throw fail('harness_adapter_invalid');
  return { options, origin, now, owner: Object.freeze(owner), reader: safeReader, manifest, coverage,
    adapters: adapterInput };
}

function verifyReaderSnapshot(value, reader) {
  const snapshot = plainData(value, ['schema', 'provenance', 'ownerId', 'generation', 'profileId', 'contextId', 'windowIds'],
    'reader_window_changed');
  const windows = strictArray(snapshot.windowIds, MAX_WINDOWS, 'reader_window_changed', 1);
  if (snapshot.schema !== 'aku.x.reader-window-inventory.v1' ||
      snapshot.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE ||
      snapshot.ownerId !== reader.ownerId || snapshot.generation !== reader.generation ||
      snapshot.profileId !== reader.profileId || snapshot.contextId !== reader.contextId ||
      windows.length !== reader.windowIds.length || windows.some(id => !reader.windowIds.includes(id))) {
    throw fail('reader_window_changed');
  }
}

function validateCollector(value, owner, reader) {
  const collector = plainData(value, ['schema', 'provenance', 'ownerId', 'generation', 'profileId', 'contextId', 'resource'],
    'collector_identity_invalid');
  if (collector.schema !== 'aku.x.collector-context.v1' ||
      collector.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE ||
      collector.ownerId !== owner.ownerId || collector.generation !== owner.generation ||
      !validIdentifier(collector.profileId) || !validIdentifier(collector.contextId) ||
      collector.profileId === reader.profileId || collector.contextId === reader.contextId ||
      !collector.resource || (typeof collector.resource !== 'object' && typeof collector.resource !== 'function')) {
    throw fail('collector_identity_invalid');
  }
  return Object.freeze({ ...collector });
}

function validateCookieTransfer(value, manifest) {
  const input = strictArray(value, MAX_MANIFEST_TUPLES, 'transfer_unsupported', 1);
  if (input.length !== manifest.tuples.length) throw fail('transfer_unsupported');
  const cookies = input.map(item => cookieAttrs(item, manifest.origin, 'transfer_unsupported', true));
  const byKey = new Map();
  for (const cookie of cookies) {
    const key = cookieKey(cookie);
    if (byKey.has(key)) throw fail('transfer_unsupported');
    byKey.set(key, cookie);
  }
  let totalBytes = 0;
  const ordered = [];
  for (const approved of manifest.tuples) {
    const cookie = byKey.get(cookieKey(approved));
    if (!cookie || COOKIE_ATTRIBUTES.some(key => JSON.stringify(cookie[key]) !== JSON.stringify(approved[key]))) {
      throw fail('transfer_unsupported');
    }
    const bytes = canonicalCookieBytes(cookie);
    if (bytes > MAX_TUPLE_BYTES) throw fail('transfer_unsupported');
    totalBytes += bytes;
    if (totalBytes > MAX_TOTAL_COOKIE_BYTES) throw fail('transfer_unsupported');
    ordered.push(Object.freeze({ ...cookie }));
  }
  return { cookies: Object.freeze(ordered), totalBytes };
}

function validateProbe(value, expected, start, end, clock) {
  const response = plainData(value, ['schema', 'provenance', 'role', 'pairIndex', 'ownerId', 'generation',
    'profileId', 'contextId', 'accountId', 'probeSchema', 'httpStatus', 'authState', 'observedAt'],
  'probe_response_invalid');
  if (response.schema !== X_CONCURRENCY_PROBE_SCHEMA ||
      response.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE ||
      response.role !== expected.role || response.pairIndex !== expected.pairIndex ||
      response.ownerId !== expected.ownerId || response.generation !== expected.generation ||
      response.profileId !== expected.profileId || response.contextId !== expected.contextId ||
      response.probeSchema !== X_CONCURRENCY_PROBE_SCHEMA || response.httpStatus !== 200 ||
      response.authState !== 'authenticated') throw fail('auth_unavailable');
  if (response.accountId !== expected.accountId) throw fail('identity_change');
  const now = clock();
  if (!Number.isFinite(now) || now < end || !validTimestamp(response.observedAt, now) ||
      response.observedAt < start || response.observedAt > end) throw fail('stale_response');
  return true;
}

function validateClosed(value) {
  const result = plainData(value, ['closed'], 'cleanup_failed');
  if (result.closed !== true) throw fail('cleanup_failed');
}

function overlapDuration(first, second) {
  return Math.max(0, Math.min(first.end, second.end) - Math.max(first.start, second.start));
}

function publicCleanupStatus(state) {
  return Object.freeze({ pending: state.pending, failed: state.failed, complete: !state.pending && !state.failed });
}

/**
 * Run one synthetic reader-to-collector seed and two pairs of concurrent
 * in-memory reads. Adapters are trusted in-process fixture code; the receipt
 * is scheduling evidence only and can never authorize collection or claim a
 * native X session.
 */
export async function runXSyntheticConcurrencyHarness(input) {
  let validated;
  try { validated = validateInputs(input); }
  catch (error) { if (SAFE_FAILURES.has(error)) throw error; throw fail('harness_input_invalid'); }
  const { options, origin, owner, reader, manifest, coverage, adapters } = validated;
  if (options.signal?.aborted) throw fail('harness_cancelled');
  if (USED_LEASES.has(owner.leaseId)) throw fail('harness_replay');
  if (USED_LEASES.size >= MAX_USED_LEASES) throw fail('harness_replay_capacity');
  USED_LEASES.add(owner.leaseId);

  let lastClock = validated.now;
  const clock = () => {
    let value;
    try { value = options.clock(); } catch { throw fail('harness_clock_invalid'); }
    if (!Number.isFinite(value) || value < lastClock) throw fail('harness_clock_invalid');
    lastClock = value;
    return value;
  };
  const operationBudget = Math.max(1, options.timeoutMs - options.cleanupWaitMs);
  const startTime = validated.now;
  const abort = new AbortController();
  let admissionOpen = true;
  let finalized = false;
  let reason = null;
  let coverageLost = false;
  let readerWindowIdentity = 'unverified';
  let subscription = null;
  let activeCalls = 0;
  let collectorHandle = null;
  let collectorCreateStarted = false;
  let collectorCreateSettled = false;
  let collectorCreateFailed = false;
  let cleanupStarted = false;
  let cleanupFinished = false;
  let cleanupFailed = false;
  let cleanupResolve;
  const cleanupDone = new Promise(resolve => { cleanupResolve = resolve; });
  const cleanupState = { get pending() { return !cleanupFinished; }, get failed() { return cleanupFailed; } };
  let transferCookies = null;
  let calls = { seedCalls: 0, readCalls: 0, authenticatedSameAccountReads: 0, overlappingPairs: 0,
    cookieTuples: 0, cookieBytes: 0, readerWindows: reader.windowIds.length, maxOverlapMs: 0 };

  function closeAdmission(nextReason, shouldAbort = true) {
    if (finalized) return;
    if (admissionOpen) admissionOpen = false;
    if (nextReason && reason === null) reason = nextReason;
    if (shouldAbort && !abort.signal.aborted) abort.abort();
    transferCookies = null;
    beginCleanupIfReady();
  }

  function settleCleanup(failed = false) {
    if (cleanupFinished) return;
    cleanupFailed ||= failed;
    cleanupFinished = true;
    cleanupResolve();
  }

  function beginCleanupIfReady() {
    if (admissionOpen || activeCalls !== 0 || cleanupStarted) return;
    if (collectorCreateStarted && !collectorCreateSettled) return;
    if (collectorCreateFailed && !collectorHandle) {
      settleCleanup(true);
      return;
    }
    if (!collectorHandle) {
      settleCleanup(false);
      return;
    }
    cleanupStarted = true;
    let result;
    try { result = adapters.closeCollector(collectorHandle); }
    catch { settleCleanup(true); return; }
    const finish = value => {
      try { validateClosed(value); settleCleanup(false); }
      catch { settleCleanup(true); }
    };
    if (result instanceof Promise) result.then(finish, () => settleCleanup(true));
    else finish(result);
  }

  function stopFromObservation(event) {
    let parsed;
    try {
      parsed = plainData(event, ['schema', 'provenance', 'kind'], 'coverage_unqualified');
      if (parsed.schema !== 'aku.x.synthetic-observation.v1' ||
          parsed.provenance !== X_CONCURRENCY_SYNTHETIC_PROVENANCE || !OBSERVATION_KINDS.has(parsed.kind)) {
        throw fail('coverage_unqualified');
      }
    } catch { coverageLost = true; readerWindowIdentity = 'unverified'; closeAdmission('coverage_unqualified'); return; }
    if (parsed.kind === 'detach' || parsed.kind === 'coverage_loss') coverageLost = true;
    readerWindowIdentity = 'unverified';
    const mapped = {
      mutation: 'unexpected_mutation', identity_change: 'identity_change', login_change: 'login_change',
      detach: 'coverage_unqualified', coverage_loss: 'coverage_unqualified', stale_response: 'stale_response',
      challenge: 'auth_unavailable',
    }[parsed.kind];
    closeAdmission(mapped);
  }

  function invoke(name, fn, args) {
    if (!admissionOpen) return Promise.resolve({ ok: false });
    let start;
    try {
      start = clock();
      if (start - startTime >= operationBudget || start - startTime > MAX_TOTAL_MS) {
        closeAdmission('timeout'); return Promise.resolve({ ok: false });
      }
      if (!validTimestamp(owner.observedAt, start) || !validTimestamp(reader.observedAt, start)) {
        closeAdmission('stale_evidence'); return Promise.resolve({ ok: false });
      }
    } catch { closeAdmission('clock_invalid'); return Promise.resolve({ ok: false }); }
    activeCalls++;
    if (name === 'createCollector') collectorCreateStarted = true;
    let raw;
    try { raw = fn(args); }
    catch {
      activeCalls--;
      if (name === 'createCollector') { collectorCreateSettled = true; collectorCreateFailed = true; }
      beginCleanupIfReady();
      return Promise.resolve({ ok: false });
    }

    const finish = (value, failed) => {
      let end;
      try { end = clock(); } catch { failed = true; closeAdmission('clock_invalid'); }
      if (name === 'createCollector') {
        collectorCreateSettled = true;
        if (failed) collectorCreateFailed = true;
        else {
          try {
            const descriptor = value && typeof value === 'object' ? Object.getOwnPropertyDescriptor(value, 'resource') : null;
            if (descriptor && Object.hasOwn(descriptor, 'value') && descriptor.value &&
                (typeof descriptor.value === 'object' || typeof descriptor.value === 'function')) {
              collectorHandle = descriptor.value;
            } else collectorCreateFailed = true;
          } catch { collectorCreateFailed = true; }
        }
      }
      activeCalls--;
      beginCleanupIfReady();
      return { ok: !failed, value, start, end };
    };
    let tracked;
    if (raw instanceof Promise) tracked = raw.then(value => finish(value, false), () => finish(undefined, true));
    else tracked = Promise.resolve(finish(raw, false));
    return new Promise(resolve => {
      let done = false;
      const finishRace = result => {
        if (done) return;
        done = true;
        abort.signal.removeEventListener('abort', onAbort);
        resolve(result);
      };
      const onAbort = () => finishRace({ ok: false });
      if (abort.signal.aborted) onAbort();
      else abort.signal.addEventListener('abort', onAbort, { once: true });
      tracked.then(finishRace, () => finishRace({ ok: false }));
    });
  }

  function unsubscribe() {
    if (typeof subscription !== 'function') return;
    const remove = subscription;
    subscription = null;
    try { remove(); } catch { cleanupFailed = true; }
  }

  let timer;
  let externalAbort;
  const timeout = new Promise(resolve => {
    timer = setTimeout(() => { closeAdmission('timeout'); resolve(); }, operationBudget);
  });
  if (options.signal) {
    externalAbort = () => { closeAdmission('cancelled'); };
    options.signal.addEventListener('abort', externalAbort, { once: true });
  }

  async function mustCall(name, fn, args) {
    ensureEvidenceFresh();
    const result = await invoke(name, fn, args);
    if (!result.ok || !admissionOpen) throw fail(reason ?? 'adapter_failed');
    return result;
  }

  function ensureEvidenceFresh() {
    if (!admissionOpen) throw fail(reason ?? 'admission_closed');
    const now = clock();
    if (now - startTime >= operationBudget || now - startTime > MAX_TOTAL_MS) throw fail('timeout');
    if (!validTimestamp(owner.observedAt, now) || !validTimestamp(reader.observedAt, now)) throw fail('stale_evidence');
  }

  async function run() {
    try {
      try { subscription = adapters.subscribeObservation(stopFromObservation); }
      catch { cleanupFailed = true; closeAdmission('adapter_failed'); throw fail('adapter_failed'); }
      if (typeof subscription !== 'function') {
        cleanupFailed = true;
        closeAdmission('coverage_unqualified');
        throw fail('coverage_unqualified');
      }
      if (options.signal?.aborted) { closeAdmission('cancelled'); throw fail('cancelled'); }
      const initial = await mustCall('inspectReader', adapters.inspectReader, { reader, origin, signal: abort.signal });
      try { verifyReaderSnapshot(initial.value, reader); }
      catch (error) {
        if (SAFE_FAILURES.get(error) === 'reader_window_changed') readerWindowIdentity = 'changed';
        throw error;
      }
      const created = await mustCall('createCollector', adapters.createCollector,
        { ownerBinding: owner, reader, origin, probeSchema: X_CONCURRENCY_PROBE_SCHEMA, signal: abort.signal });
      const collector = validateCollector(created.value, owner, reader);

      const source = await mustCall('readCookieTuples', adapters.readCookieTuples,
        { reader, manifest, origin, signal: abort.signal });
      const transferred = validateCookieTransfer(source.value, manifest);
      transferCookies = transferred.cookies;
      calls.cookieTuples = transferred.cookies.length;
      calls.cookieBytes = transferred.totalBytes;
      if (!admissionOpen) throw fail(reason ?? 'admission_closed');
      ensureEvidenceFresh();
      const seeded = await mustCall('seedCollector', args => {
        calls.seedCalls++;
        if (calls.seedCalls !== 1) throw fail('harness_replay');
        return adapters.seedCollector(args);
      },
        { collector: collector.resource, cookies: transferCookies, manifest, origin, signal: abort.signal });
      const seedResult = plainData(seeded.value, ['seeded', 'tupleCount'], 'transfer_unsupported');
      if (seedResult.seeded !== true || seedResult.tupleCount !== calls.cookieTuples) throw fail('transfer_unsupported');
      transferCookies = null;

      const intervals = [];
      for (let pairIndex = 0; pairIndex < 2; pairIndex++) {
        ensureEvidenceFresh();
        const readerCall = invoke('readProbe', args => { calls.readCalls++; return adapters.readProbe(args); }, {
          role: 'reader', pairIndex, reader, origin, probeSchema: X_CONCURRENCY_PROBE_SCHEMA, signal: abort.signal,
        });
        if (!admissionOpen) throw fail(reason ?? 'admission_closed');
        ensureEvidenceFresh();
        const collectorCall = invoke('readProbe', args => { calls.readCalls++; return adapters.readProbe(args); }, {
          role: 'collector', pairIndex, reader, collector: collector.resource,
          origin, probeSchema: X_CONCURRENCY_PROBE_SCHEMA, signal: abort.signal,
        });
        const [left, right] = await Promise.all([readerCall, collectorCall]);
        if (!left.ok || !right.ok || !admissionOpen) throw fail(reason ?? 'adapter_failed');
        const expectedReader = { role: 'reader', pairIndex, ownerId: reader.ownerId, generation: reader.generation,
          profileId: reader.profileId, contextId: reader.contextId, accountId: reader.accountId };
        const expectedCollector = { role: 'collector', pairIndex, ownerId: owner.ownerId, generation: owner.generation,
          profileId: collector.profileId, contextId: collector.contextId, accountId: reader.accountId };
        validateProbe(left.value, expectedReader, left.start, left.end, clock);
        validateProbe(right.value, expectedCollector, right.start, right.end, clock);
        if (!admissionOpen) throw fail(reason ?? 'admission_closed');
        calls.authenticatedSameAccountReads += 2;
        const overlapMs = overlapDuration(left, right);
        if (overlapMs > 0) {
          calls.overlappingPairs++;
          calls.maxOverlapMs = Math.max(calls.maxOverlapMs, overlapMs);
        }
        intervals.push({ overlapMs });
      }
      const finalSnapshot = await mustCall('inspectReader', adapters.inspectReader, { reader, origin, signal: abort.signal });
      try { verifyReaderSnapshot(finalSnapshot.value, reader); readerWindowIdentity = 'retained'; }
      catch (error) {
        if (SAFE_FAILURES.get(error) === 'reader_window_changed') readerWindowIdentity = 'changed';
        throw error;
      }
      if (clock() - startTime > MAX_TOTAL_MS) throw fail('timeout');
      closeAdmission(null, false);
    } catch (error) {
      if (admissionOpen) closeAdmission(SAFE_FAILURES.get(error) ?? 'adapter_failed');
    }
  }

  const work = run();
  await Promise.race([work, timeout]);
  if (admissionOpen) closeAdmission('timeout');
  clearTimeout(timer);
  if (options.signal && externalAbort) options.signal.removeEventListener('abort', externalAbort);
  beginCleanupIfReady();

  const cleanupWait = Math.min(options.cleanupWaitMs, options.timeoutMs - operationBudget);
  if (!cleanupFinished && cleanupWait > 0) {
    await Promise.race([cleanupDone, new Promise(resolve => setTimeout(resolve, cleanupWait))]);
  }
  unsubscribe();

  let endTime = lastClock;
  try { endTime = clock(); } catch { reason ??= 'clock_invalid'; }
  const pending = !cleanupFinished;
  const failed = cleanupFailed;
  let outcome = reason ?? (calls.overlappingPairs > 0 ? 'overlap_qualified' : 'overlap_partial');
  if (!reason && (pending || failed)) outcome = failed ? 'cleanup_failed' : 'cleanup_pending';
  if (!reason && calls.overlappingPairs === 0 && outcome === 'overlap_partial') outcome = 'overlap_partial';
  finalized = true;
  const receipt = Object.freeze({
    schema: X_CONCURRENCY_HARNESS_SCHEMA,
    provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
    originClass: 'loopback_synthetic_fixture',
    outcome,
    seedCalls: calls.seedCalls,
    cookieTuples: calls.cookieTuples,
    cookieBytes: calls.cookieBytes,
    readCalls: calls.readCalls,
    authenticatedSameAccountReads: calls.authenticatedSameAccountReads,
    overlappingPairs: calls.overlappingPairs,
    maxOverlapMs: Math.round(calls.maxOverlapMs * 1000) / 1000,
    readerWindows: calls.readerWindows,
    readerWindowIdentity,
    coverage: coverageLost ? 'coverage_unqualified' : 'synthetic_fixture_only',
    refresh: 'refresh_not_observed',
    cleanupPending: pending,
    cleanupFailed: failed,
    durationMs: Math.max(0, Math.round((endTime - startTime) * 1000) / 1000),
    canAuthorize: false,
    nativeQualified: false,
  });
  return Object.freeze({
    receipt,
    cleanupStatus: () => publicCleanupStatus(cleanupState),
    waitForCleanup: async () => { await cleanupDone; return publicCleanupStatus(cleanupState); },
  });
}
