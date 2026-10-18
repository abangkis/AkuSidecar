import { performance } from 'node:perf_hooks';

const NANOS_PER_MILLISECOND = 1_000_000n;

function fail(code) {
  throw new TypeError(code);
}

function issuedAtNanoseconds(value) {
  if (typeof value !== 'string') fail('request_time_invalid');
  const match = /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z$/u.exec(value);
  if (!match) fail('request_time_invalid');
  const secondMs = Date.parse(`${match[1]}Z`);
  if (!Number.isSafeInteger(secondMs) || new Date(secondMs).toISOString().slice(0, 19) !== match[1]) {
    fail('request_time_invalid');
  }
  const fractionalNanoseconds = BigInt((match[2] ?? '').padEnd(9, '0'));
  return BigInt(secondMs) * NANOS_PER_MILLISECOND + fractionalNanoseconds;
}

function toEpochMilliseconds(nanoseconds) {
  let whole = nanoseconds / NANOS_PER_MILLISECOND;
  let remainder = nanoseconds % NANOS_PER_MILLISECOND;
  if (remainder < 0) { whole--; remainder += NANOS_PER_MILLISECOND; }
  return Number(whole) + Number(remainder) / Number(NANOS_PER_MILLISECOND);
}

function floorEpochMilliseconds(nanoseconds) {
  let whole = nanoseconds / NANOS_PER_MILLISECOND;
  if (nanoseconds % NANOS_PER_MILLISECOND < 0) whole--;
  return Number(whole);
}

/**
 * Wait briefly for a coarse local wall clock to catch up to a Go-issued UTC
 * timestamp. The ingress deadline rounds IssuedAt + ttlMs down to a whole
 * millisecond; floating-point conversion must never extend the exact Go TTL.
 */
export async function awaitIssuedAtCatchUp({
  issuedAt,
  clock = Date.now,
  elapsedClock = () => performance.now(),
  sleep = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds)),
  maxCatchUpMs = 50,
  ttlMs = 5_000,
} = {}) {
  if (typeof clock !== 'function' || typeof elapsedClock !== 'function' || typeof sleep !== 'function' ||
      !Number.isSafeInteger(maxCatchUpMs) || maxCatchUpMs <= 0 || maxCatchUpMs > 50 ||
      !Number.isSafeInteger(ttlMs) || ttlMs <= 0) fail('request_time_invalid');

  const issuedNs = issuedAtNanoseconds(issuedAt);
  const deadlineNs = issuedNs + BigInt(ttlMs) * NANOS_PER_MILLISECOND;
  let lastClock = null;
  const readClock = () => {
    let current;
    try { current = clock(); } catch { fail('request_clock_invalid'); }
    if (!Number.isSafeInteger(current)) fail('request_clock_invalid');
    if (lastClock !== null && current < lastClock) fail('request_clock_rollback');
    lastClock = current;
    return current;
  };

  let startElapsed;
  try { startElapsed = elapsedClock(); } catch { fail('request_clock_invalid'); }
  if (!Number.isFinite(startElapsed)) fail('request_clock_invalid');
  let lastElapsed = startElapsed;

  for (;;) {
    let elapsed;
    try { elapsed = elapsedClock(); } catch { fail('request_clock_invalid'); }
    if (!Number.isFinite(elapsed) || elapsed < lastElapsed) fail('request_clock_rollback');
    lastElapsed = elapsed;
    if (elapsed - startElapsed > maxCatchUpMs) fail('request_future');

    const nowMs = readClock();
    const nowNs = BigInt(nowMs) * NANOS_PER_MILLISECOND;
    if (nowNs >= deadlineNs) fail('request_expired');
    if (nowNs >= issuedNs) {
      return Object.freeze({
        nowMs,
        issuedAtMs: toEpochMilliseconds(issuedNs),
        deadlineMs: floorEpochMilliseconds(deadlineNs),
      });
    }

    const remaining = maxCatchUpMs - (elapsed - startElapsed);
    if (remaining <= 0) fail('request_future');
    try { await sleep(Math.min(1, remaining)); }
    catch { fail('request_clock_wait_failed'); }
  }
}
