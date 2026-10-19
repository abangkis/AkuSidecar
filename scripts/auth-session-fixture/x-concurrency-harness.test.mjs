import test from 'node:test';
import assert from 'node:assert/strict';
import {
  runXSyntheticConcurrencyHarness,
  X_CONCURRENCY_EVIDENCE_SCHEMA,
  X_CONCURRENCY_OWNER_SCHEMA,
  X_CONCURRENCY_PROBE_SCHEMA,
  X_CONCURRENCY_SYNTHETIC_PROVENANCE,
} from './x-concurrency-harness.mjs';

const ORIGIN = 'http://127.0.0.1:43127';
const PARTITION = Object.freeze({ topLevelSite: 'http://127.0.0.1', hasCrossSiteAncestor: false });
let leaseNumber = 0;

function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((yes, no) => { resolve = yes; reject = no; });
  return { promise, resolve, reject };
}

function cookie(name = 'fixture_session', value = 'synthetic-cookie-secret') {
  return { name, value, domain: '127.0.0.1', path: '/', hostOnly: true, secure: true,
    httpOnly: true, sameSite: 'Lax', expires: null, partitionKey: { ...PARTITION } };
}

function manifestCookie(item) {
  const { value, ...tuple } = item;
  return tuple;
}

function makeFixture({ adapterOverrides = {}, readerPatch = {}, ownerPatch = {}, manifestEntries,
  sourceCookies, clockStart = 100, timeoutMs = 500, cleanupWaitMs = 25 } = {}) {
  let time = clockStart;
  let listener;
  const calls = { adapters: 0, seed: 0, close: 0, reads: [], seeded: null, unsubscribed: false };
  const clock = () => time++;
  const ownerBinding = {
    schema: X_CONCURRENCY_OWNER_SCHEMA, provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
    ownerId: 'synthetic-owner-A', generation: 'generation-7', leaseId: `lease-test-${++leaseNumber}`,
    observedAt: clockStart, ...ownerPatch,
  };
  const reader = {
    schema: X_CONCURRENCY_EVIDENCE_SCHEMA, provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
    origin: ORIGIN, ownerId: ownerBinding.ownerId, generation: ownerBinding.generation,
    profileId: 'synthetic-reader-profile', contextId: 'synthetic-reader-context',
    accountId: 'invented-account-42', probeSchema: X_CONCURRENCY_PROBE_SCHEMA,
    authState: 'authenticated', observedAt: clockStart, windowIds: ['window-A', 'window-B'], ...readerPatch,
  };
  const baseCookie = cookie();
  const approved = manifestEntries ?? [manifestCookie(baseCookie)];
  const cookies = sourceCookies ?? [baseCookie];
  const collector = {
    schema: 'aku.x.collector-context.v1', provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
    ownerId: ownerBinding.ownerId, generation: ownerBinding.generation,
    profileId: 'synthetic-collector-profile', contextId: 'synthetic-collector-context',
    resource: { ownedSyntheticResource: true },
  };
  function touch() { calls.adapters++; }
  function snapshot() {
    return { schema: 'aku.x.reader-window-inventory.v1', provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
      ownerId: reader.ownerId, generation: reader.generation, profileId: reader.profileId,
      contextId: reader.contextId, windowIds: [...reader.windowIds] };
  }
  function probe({ role, pairIndex }) {
    calls.reads.push({ role, pairIndex });
    return { schema: X_CONCURRENCY_PROBE_SCHEMA, provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
      role, pairIndex, ownerId: role === 'reader' ? reader.ownerId : collector.ownerId,
      generation: role === 'reader' ? reader.generation : collector.generation,
      profileId: role === 'reader' ? reader.profileId : collector.profileId,
      contextId: role === 'reader' ? reader.contextId : collector.contextId,
      accountId: reader.accountId, probeSchema: X_CONCURRENCY_PROBE_SCHEMA,
      httpStatus: 200, authState: 'authenticated', observedAt: clock() };
  }
  const pairWaiters = new Map();
  const adapters = {
    subscribeObservation(callback) { touch(); listener = callback; return () => { calls.unsubscribed = true; }; },
    inspectReader() { touch(); return snapshot(); },
    createCollector() { touch(); return { ...collector }; },
    readCookieTuples() { touch(); return cookies; },
    seedCollector({ cookies: copied }) { touch(); calls.seed++; calls.seeded = copied; return { seeded: true, tupleCount: copied.length }; },
    readProbe(args) {
      touch();
      calls.reads.push({ invoked: true, role: args.role, pairIndex: args.pairIndex });
      if (adapterOverrides.serialReads) return probe(args);
      return new Promise(resolve => {
        const waiters = pairWaiters.get(args.pairIndex) ?? [];
        waiters.push({ args, resolve });
        pairWaiters.set(args.pairIndex, waiters);
        if (waiters.length === 2) for (const waiter of waiters) waiter.resolve(probe(waiter.args));
      });
    },
    closeCollector() { touch(); calls.close++; return { closed: true }; },
    ...Object.fromEntries(Object.entries(adapterOverrides).filter(([key]) => key !== 'serialReads')),
  };
  const coverage = { schema: 'aku.x.synthetic-coverage.v1', provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
    readerStartup: true, collectorStartup: true, targetCoverage: true, workerCoverage: true,
    detachCoverage: true, mutationCoverage: true, identityCoverage: true };
  const input = { origin: ORIGIN, provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE, ownerBinding, reader, coverage,
    manifest: { schema: 'aku.x.cookie-manifest.synthetic.v1', provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
      origin: ORIGIN, tuples: approved }, adapters, clock, signal: null, timeoutMs, cleanupWaitMs };
  return { input, calls, reader, ownerBinding, collector, clock, setTime(value) { time = value; }, notify(kind) {
    listener?.({ schema: 'aku.x.synthetic-observation.v1', provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE, kind });
  } };
}

test('one exact seed and two overlapping same-account in-memory read pairs produce a non-authorizing receipt', async () => {
  const fixture = makeFixture();
  const result = await runXSyntheticConcurrencyHarness(fixture.input);
  const { receipt } = result;
  assert.equal(receipt.outcome, 'overlap_qualified');
  assert.equal(receipt.seedCalls, 1);
  assert.equal(receipt.cookieTuples, 1);
  assert.equal(receipt.readCalls, 4);
  assert.equal(receipt.authenticatedSameAccountReads, 4);
  assert.equal(receipt.overlappingPairs, 2);
  assert.ok(receipt.maxOverlapMs > 0);
  assert.equal(receipt.readerWindows, 2);
  assert.equal(receipt.readerWindowIdentity, 'retained');
  assert.equal(receipt.refresh, 'refresh_not_observed');
  assert.equal(receipt.canAuthorize, false);
  assert.equal(receipt.nativeQualified, false);
  assert.deepEqual(fixture.calls.seeded[0], cookie());
  assert.deepEqual(await result.waitForCleanup(), { pending: false, failed: false, complete: true });
  const output = JSON.stringify(receipt);
  for (const secret of ['invented-account-42', 'synthetic-cookie-secret', 'synthetic-owner-A']) {
    assert.equal(output.includes(secret), false);
  }
});

test('synchronous serial reads do not qualify overlap and reader window order may change safely', async () => {
  const fixture = makeFixture({ adapterOverrides: { serialReads: true,
    inspectReader: (() => { let count = 0; return () => {
      const value = { schema: 'aku.x.reader-window-inventory.v1', provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
        ownerId: fixture.reader.ownerId, generation: fixture.reader.generation, profileId: fixture.reader.profileId,
        contextId: fixture.reader.contextId, windowIds: count++ === 0 ? ['window-A', 'window-B'] : ['window-B', 'window-A'] };
      fixture.calls.adapters++; return value;
    }; })() } });
  const result = await runXSyntheticConcurrencyHarness(fixture.input);
  assert.equal(result.receipt.outcome, 'overlap_partial');
  assert.equal(result.receipt.overlappingPairs, 0);
  assert.equal(result.receipt.authenticatedSameAccountReads, 4);
  assert.equal(result.receipt.readerWindowIdentity, 'retained');
});

test('native, non-loopback, and non-synthetic scope is rejected before every adapter side effect', async () => {
  for (const change of [
    input => { input.origin = 'https://x.com'; },
    input => { input.origin = 'http://example.test:43127'; },
    input => { input.provenance = 'native-x'; },
  ]) {
    const fixture = makeFixture();
    change(fixture.input);
    await assert.rejects(runXSyntheticConcurrencyHarness(fixture.input));
    assert.equal(fixture.calls.adapters, 0);
  }
});

test('manifest names are primitive and fixed; path/domain wildcards and unknown fields fail before adapters', async () => {
  let coercions = 0;
  const invalidName = { toString() { coercions++; return 'session'; } };
  for (const change of [
    input => { input.manifest.tuples[0].name = invalidName; },
    input => { input.manifest.tuples[0].path = '/*'; },
    input => { input.manifest.tuples[0].domain = '*'; },
    input => { input.manifest.extra = 'unapproved'; },
  ]) {
    const fixture = makeFixture(); change(fixture.input);
    await assert.rejects(runXSyntheticConcurrencyHarness(fixture.input));
    assert.equal(fixture.calls.adapters, 0);
  }
  assert.equal(coercions, 0);
});

test('unknown auth, stale evidence, and reused owner leases fail closed before adapters', async () => {
  for (const change of [
    input => { input.reader.authState = 'challenge'; },
    input => { input.reader.authState = 'unknown'; },
    input => { input.reader.observedAt = 1; input.ownerBinding.observedAt = 1; input.clock = () => 6_000; },
  ]) {
    const fixture = makeFixture(); change(fixture.input);
    await assert.rejects(runXSyntheticConcurrencyHarness(fixture.input));
    assert.equal(fixture.calls.adapters, 0);
  }
  const first = makeFixture();
  await runXSyntheticConcurrencyHarness(first.input);
  const second = makeFixture();
  second.input.ownerBinding.leaseId = first.ownerBinding.leaseId;
  await assert.rejects(runXSyntheticConcurrencyHarness(second.input), /harness_replay/u);
  assert.equal(second.calls.adapters, 0);
});

test('owner freshness and trusted monotonic deadline are rechecked before seed dispatch', async () => {
  for (const age of [5_001, 501]) {
    const fixture = makeFixture({ timeoutMs: age === 501 ? 500 : 10_000, cleanupWaitMs: 25, adapterOverrides: {
      readCookieTuples() {
        fixture.calls.adapters++;
        fixture.setTime(age === 501 ? 100 + 500 : 100 + 5_001);
        return [cookie()];
      },
    } });
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    assert.equal(result.receipt.seedCalls, 0);
    assert.equal(fixture.calls.seed, 0);
    assert.equal(result.receipt.outcome, age === 501 ? 'timeout' : 'stale_evidence');
    assert.equal(fixture.calls.close, 1);
  }
});

test('collector must be a fresh context under the same owner generation', async () => {
  for (const invalid of [
    { profileId: 'synthetic-reader-profile' },
    { contextId: 'synthetic-reader-context' },
    { generation: 'generation-other' },
    { ownerId: 'owner-other' },
  ]) {
    const fixture = makeFixture({ adapterOverrides: { createCollector() {
      fixture.calls.adapters++;
      return { ...fixture.collector, ...invalid };
    } } });
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    assert.equal(result.receipt.outcome, 'collector_identity_invalid');
    assert.equal(result.receipt.seedCalls, 0);
    assert.equal(fixture.calls.close, 1);
  }
});

test('manifest rejects pollution, missing, extra, duplicate, unapproved, and changed-attribute tuples', async () => {
  const cases = [
    { manifestEntries: [manifestCookie(cookie()), manifestCookie(cookie('other'))], sourceCookies: [] },
    { manifestEntries: [manifestCookie(cookie()), manifestCookie(cookie('other'))], sourceCookies: [cookie(), cookie()] },
    { sourceCookies: [cookie(), cookie('unapproved')] },
    { sourceCookies: [cookie('unapproved')] },
    { sourceCookies: [{ ...cookie(), secure: false }] },
  ];
  for (const options of cases) {
    const fixture = makeFixture(options);
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    assert.equal(result.receipt.outcome, 'transfer_unsupported');
    assert.equal(fixture.calls.seed, 0);
  }
  const polluted = makeFixture({ sourceCookies: [{ ...cookie(), unexpected: 'credential' }] });
  assert.equal((await runXSyntheticConcurrencyHarness(polluted.input)).receipt.outcome, 'transfer_unsupported');
});

test('cookie getters are rejected without invocation and tuple/total byte caps are enforced', async () => {
  let getterReads = 0;
  const getterCookie = { ...cookie() };
  delete getterCookie.value;
  Object.defineProperty(getterCookie, 'value', { enumerable: true, get() { getterReads++; return 'getter-secret'; } });
  const getterFixture = makeFixture({ sourceCookies: [getterCookie] });
  const getterResult = await runXSyntheticConcurrencyHarness(getterFixture.input);
  assert.equal(getterResult.receipt.outcome, 'transfer_unsupported');
  assert.equal(getterReads, 0);
  assert.equal(JSON.stringify(getterResult.receipt).includes('getter-secret'), false);

  const tooLarge = makeFixture({ sourceCookies: [cookie('fixture_session', 'x'.repeat(8_200))] });
  assert.equal((await runXSyntheticConcurrencyHarness(tooLarge.input)).receipt.outcome, 'transfer_unsupported');

  const manyManifest = Array.from({ length: 9 }, (_, index) => manifestCookie(cookie(`session_${index}`, '')));
  const manySource = Array.from({ length: 9 }, (_, index) => cookie(`session_${index}`, 'x'.repeat(7_500)));
  const tooMuch = makeFixture({ manifestEntries: manyManifest, sourceCookies: manySource });
  assert.equal((await runXSyntheticConcurrencyHarness(tooMuch.input)).receipt.outcome, 'transfer_unsupported');
});

test('operation adapter errors and their message getters never reach receipts or diagnostics', async () => {
  for (const throws of [false, true]) {
    let messageReads = 0;
    const raw = {};
    Object.defineProperty(raw, 'message', { get() { messageReads++; return 'secret-cookie-error'; } });
    const fixture = makeFixture({ adapterOverrides: { readCookieTuples() {
      fixture.calls.adapters++;
      if (throws) throw raw;
      return Promise.reject(raw);
    } } });
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    assert.equal(result.receipt.outcome, 'adapter_failed');
    assert.equal(messageReads, 0);
    assert.equal(JSON.stringify(result.receipt).includes('secret-cookie-error'), false);
  }
});

test('mutation during an in-flight seed closes admission and cleans only after the seed settles', async () => {
  const gate = deferred();
  const fixture = makeFixture({ adapterOverrides: { seedCollector(args) {
    fixture.calls.adapters++; fixture.calls.seed++; fixture.calls.seeded = args.cookies;
    fixture.notify('mutation'); return gate.promise;
  } } });
  const result = await runXSyntheticConcurrencyHarness(fixture.input);
  assert.equal(result.receipt.outcome, 'unexpected_mutation');
  assert.equal(result.receipt.cleanupPending, true);
  assert.equal(fixture.calls.close, 0);
  gate.resolve({ seeded: true, tupleCount: 1 });
  assert.deepEqual(await result.waitForCleanup(), { pending: false, failed: false, complete: true });
  assert.equal(fixture.calls.close, 1);
});

test('coverage loss and stale response observations prevent any overlap success', async () => {
  for (const kind of ['coverage_loss', 'stale_response', 'identity_change', 'login_change', 'detach']) {
    const fixture = makeFixture({ adapterOverrides: { readProbe(args) {
      fixture.calls.adapters++; fixture.notify(kind); return probeForFixture(fixture, args);
    } } });
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    const outcome = ({ coverage_loss: 'coverage_unqualified', stale_response: 'stale_response',
      identity_change: 'identity_change', login_change: 'login_change', detach: 'coverage_unqualified' })[kind];
    assert.equal(result.receipt.outcome, outcome);
    assert.equal(result.receipt.overlappingPairs, 0);
    assert.equal(result.receipt.nativeQualified, false);
    if (kind === 'coverage_loss') assert.equal(result.receipt.coverage, 'coverage_unqualified');
  }
});

test('probe identity, auth state, and late response freshness are validated independently', async () => {
  const cases = [
    { expected: 'identity_change', change: (_fixture, response) => { response.accountId = 'different-account'; } },
    { expected: 'auth_unavailable', change: (_fixture, response) => { response.authState = 'challenge'; } },
    { expected: 'stale_response', change: (_fixture, response) => { response.observedAt = 0; } },
  ];
  for (const { expected, change } of cases) {
    const fixture = makeFixture({ adapterOverrides: { serialReads: true, readProbe(args) {
      fixture.calls.adapters++;
      const response = probeForFixture(fixture, args); change(fixture, response); return response;
    } } });
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    assert.equal(result.receipt.outcome, expected);
    assert.equal(result.receipt.canAuthorize, false);
  }
});

test('failed or malformed observation subscription is reported as unproven cleanup', async () => {
  for (const subscribeObservation of [() => { throw new Error('private subscription failure'); }, () => ({})]) {
    const fixture = makeFixture({ adapterOverrides: { subscribeObservation() {
      fixture.calls.adapters++; return subscribeObservation();
    } } });
    const result = await runXSyntheticConcurrencyHarness(fixture.input);
    assert.equal(result.receipt.cleanupFailed, true);
    assert.equal(result.receipt.cleanupPending, false);
    assert.equal(JSON.stringify(result.receipt).includes('private subscription failure'), false);
  }
});

function probeForFixture(fixture, { role, pairIndex }) {
  const { reader, collector, clock } = fixture;
  return { schema: X_CONCURRENCY_PROBE_SCHEMA, provenance: X_CONCURRENCY_SYNTHETIC_PROVENANCE,
    role, pairIndex, ownerId: role === 'reader' ? reader.ownerId : collector.ownerId,
    generation: role === 'reader' ? reader.generation : collector.generation,
    profileId: role === 'reader' ? reader.profileId : collector.profileId,
    contextId: role === 'reader' ? reader.contextId : collector.contextId,
    accountId: reader.accountId, probeSchema: X_CONCURRENCY_PROBE_SCHEMA,
    httpStatus: 200, authState: 'authenticated', observedAt: clock() };
}

test('cancellation or timeout during late context creation cannot reopen admission and settles cleanup safely', async () => {
  const gate = deferred();
  const fixture = makeFixture({ timeoutMs: 30, cleanupWaitMs: 10, adapterOverrides: {
    createCollector() { fixture.calls.adapters++; return gate.promise; },
  } });
  const timed = await runXSyntheticConcurrencyHarness(fixture.input);
  assert.equal(timed.receipt.outcome, 'timeout');
  assert.equal(timed.receipt.cleanupPending, true);
  gate.resolve({ ...fixture.collector });
  assert.deepEqual(await timed.waitForCleanup(), { pending: false, failed: false, complete: true });
  assert.equal(fixture.calls.close, 1);

  const lateSeed = deferred();
  const cancel = new AbortController();
  const cancelled = makeFixture({ adapterOverrides: { seedCollector(args) {
    cancelled.calls.adapters++; cancelled.calls.seed++; cancel.abort(); return lateSeed.promise;
  } } });
  cancelled.input.signal = cancel.signal;
  const result = await runXSyntheticConcurrencyHarness(cancelled.input);
  assert.equal(result.receipt.outcome, 'cancelled');
  lateSeed.resolve({ seeded: true, tupleCount: 1 });
  await result.waitForCleanup();
});

test('cleanup failure is separate, bounded, and never exposes raw adapter errors', async () => {
  const secret = 'RAW_ADAPTER_ERROR_COOKIE_VALUE';
  const fixture = makeFixture({ adapterOverrides: { closeCollector() {
    fixture.calls.adapters++; fixture.calls.close++; throw new Error(secret);
  } } });
  const result = await runXSyntheticConcurrencyHarness(fixture.input);
  assert.equal(result.receipt.outcome, 'cleanup_failed');
  assert.equal(result.receipt.cleanupFailed, true);
  assert.equal(result.receipt.cleanupPending, false);
  assert.equal(JSON.stringify(result).includes(secret), false);
  assert.equal(JSON.stringify(result.receipt).includes(secret), false);
});
