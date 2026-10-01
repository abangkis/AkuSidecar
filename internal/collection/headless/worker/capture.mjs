import { applyQuoteRecovery, probeQuoteNavigation } from './quote-navigation.mjs';
import { canonicalSourceURL, captureError, toObservation } from './observation.mjs';
import { sourceProvenance } from './provenance.mjs';

const MAX_CAPTURE_MS = 90000;
const MAX_SNAPSHOT_BYTES = 4 * 1024 * 1024;
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));

export function validateCapture(source, payload = {}) {
  if (source !== 'x' && source !== 'facebook') throw captureError('unsupported_source', 'supported sources are x and facebook');
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) throw captureError('invalid_payload', 'payload must be a JSON object');
  const home = source === 'x' ? 'https://x.com/home' : 'https://www.facebook.com/';
  const rawPageUrl = payload.pageUrl;
  let pageUrl = home;
  if (rawPageUrl !== undefined && rawPageUrl !== null && rawPageUrl !== '') {
    if (typeof rawPageUrl !== 'string' || rawPageUrl.length > 2048) throw captureError('invalid_page_url', 'pageUrl must be a native source URL of at most 2048 characters');
    pageUrl = canonicalSourceURL(source, rawPageUrl);
    if (!pageUrl) throw captureError('invalid_page_url', 'pageUrl must be a canonical native X or Facebook post URL');
  }
  const options = { pageUrl, maxPosts: boundedInteger(payload.maxPosts, 20, 1, 20, 'maxPosts'),
    maxScrolls: boundedInteger(payload.maxScrolls, 4, 0, 6, 'maxScrolls'),
    waitMs: boundedInteger(payload.waitMs, 12000, 1000, 20000, 'waitMs') };
  return options;
}

function boundedInteger(value, fallback, min, max, name) {
  if (value === undefined || value === null) return fallback;
  if (!Number.isSafeInteger(value) || value < min || value > max) throw captureError('invalid_payload', `${name} must be an integer from ${min} to ${max}`);
  return value;
}

export async function capture(browser, assetsBySource, source, payload) {
  const options = validateCapture(source, payload);
  const deadline = Date.now() + MAX_CAPTURE_MS;
  const assets = assetsBySource?.[source];
  if (!Array.isArray(assets) || assets.length < 3) throw captureError('worker_not_ready', 'source assets are unavailable');
  const provenance = sourceProvenance(assets);
  const startedAt = Date.now();
  const capturedAt = new Date(startedAt).toISOString();
  const initialNavigation = await browser.navigate(options.pageUrl, timeLeft(deadline));
  if (initialNavigation.errorText) throw captureError('navigation_failed', 'Chrome could not navigate to the requested source page');

  let snapshot = null;
  const readinessDeadline = Math.min(deadline, Date.now() + options.waitMs);
  while (Date.now() < readinessDeadline) {
    ensureTime(deadline);
    await sleep(Math.min(400, Math.max(50, readinessDeadline - Date.now())));
    try {
      await inject(browser, assets, deadline);
      snapshot = await collect(browser, source, deadline);
    } catch (error) {
      if (isTransientContextError(error)) continue;
      throw error;
    }
    if (snapshot.posts.length || snapshot.loginRequired || snapshot.challengeDetected || snapshot.sourceUnavailable) break;
  }
  if (!snapshot) throw captureError('empty_unverified', 'source page produced no verifiable snapshot before the readiness deadline');

  const stateError = sourceStateError(snapshot);
  if (stateError) throw stateError;

  let quoteRecovery = null;
  let quoteIdentityProbe = null;
  if (source === 'x') {
    const resolution = await probeQuoteNavigation(browser, snapshot, { url: options.pageUrl, deadlineAt: deadline }, async () => {
      ensureTime(deadline);
      await inject(browser, assets, deadline);
      return collect(browser, source, deadline);
    });
    quoteRecovery = resolution?.recovery || null;
    quoteIdentityProbe = resolution?.probe || null;
  }
  snapshot = applyQuoteRecovery(snapshot, quoteRecovery);

  const snapshots = [];
  const seenIds = new Set();
  let unchangedRounds = 0;
  let stopReason = 'scroll_limit';
  for (let round = 0; round <= options.maxScrolls; round++) {
    ensureTime(deadline);
    if (round > 0) {
      await browser.evaluate('globalThis.XHeadlessPoC.scrollNext ? globalThis.XHeadlessPoC.scrollNext() : window.scrollBy(0, Math.round(innerHeight * 0.8))', timeLeft(deadline));
      await sleep(1200);
      snapshot = await collect(browser, source, deadline);
      snapshot = applyQuoteRecovery(snapshot, quoteRecovery);
      const nextError = sourceStateError(snapshot);
      if (nextError) throw nextError;
    }
    const previous = seenIds.size;
    const posts = snapshot.posts.slice(0, options.maxPosts);
    for (const post of posts) seenIds.add(post.id);
    snapshots.push({ ...snapshot, posts, capturedAt: new Date().toISOString(), ...(round === 0 && quoteIdentityProbe ? { quoteIdentityProbe } : {}) });
    unchangedRounds = seenIds.size === previous ? unchangedRounds + 1 : 0;
    if (seenIds.size >= options.maxPosts) { stopReason = 'post_limit'; break; }
    if (unchangedRounds >= 3) { stopReason = 'three_rounds_without_new_identity'; break; }
  }
  if (!seenIds.size) throw captureError('empty_unverified', 'no source post evidence was captured');
  return toObservation({ source, requestedUrl: options.pageUrl, snapshots, provenance, capturedAt, stopReason });
}

async function inject(browser, assets, deadline) {
  for (const asset of assets) if (asset.execute) await browser.evaluate(asset.content, timeLeft(deadline));
}

async function collect(browser, source, deadline) {
  ensureTime(deadline);
  const targets = await browser.evaluate('globalThis.XHeadlessPoC?.prepareEvidenceTargets?.() || []', timeLeft(deadline));
  if (Array.isArray(targets)) {
    for (const target of targets.slice(0, 4)) {
      ensureTime(deadline);
      if (!target || !Number.isFinite(target.x) || !Number.isFinite(target.y) || typeof target.token !== 'string') continue;
      await browser.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: Math.trunc(target.x), y: Math.trunc(target.y) }, timeLeft(deadline));
      await sleep(650);
      await browser.evaluate(`globalThis.XHeadlessPoC.recordHoverEvidence(${JSON.stringify(target.token)})`, timeLeft(deadline));
    }
    if (targets.length) await browser.send('Input.dispatchMouseEvent', { type: 'mouseMoved', x: 0, y: 0 }, timeLeft(deadline));
  }
  ensureTime(deadline);
  const expression = '(async () => JSON.stringify(await globalThis.XHeadlessPoC.collect()))()';
  const encoded = await browser.evaluate(expression, timeLeft(deadline));
  if (typeof encoded !== 'string' || Buffer.byteLength(encoded, 'utf8') > MAX_SNAPSHOT_BYTES) {
    throw captureError('evidence_too_large', 'page snapshot exceeds the 4 MiB limit');
  }
  let snapshot;
  try { snapshot = JSON.parse(encoded); } catch { throw captureError('invalid_observation', 'source extractor returned invalid JSON'); }
  if (!snapshot || !Array.isArray(snapshot.posts)) throw captureError('invalid_observation', 'source extractor returned no posts array');
  return snapshot;
}

function sourceStateError(snapshot) {
  if (snapshot.loginRequired === true) return captureError('login_required', 'source requires an authenticated login');
  if (snapshot.challengeDetected === true) return captureError('challenge_required', 'source presented an access challenge');
  if (snapshot.sourceUnavailable) return captureError('source_unavailable', 'source reports that content is unavailable');
  return null;
}

function ensureTime(deadline) {
  if (Date.now() >= deadline) throw captureError('capture_timeout', 'capture exceeded the 90 second worker limit');
}

function timeLeft(deadline) {
  ensureTime(deadline);
  return Math.max(1, deadline - Date.now());
}

function isTransientContextError(error) {
  return /context.*destroyed|cannot find context/i.test(String(error?.message || error));
}
