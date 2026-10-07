import { captureError } from './observation.mjs';

const MAX_PHASE_MS = 5000;
const CAPTURE_HEADROOM_MS = 1000;
const ROUTE_CHECK_RESERVE_MS = 250;
const POLL_INTERVAL_MS = 250;
const SOURCE_ERROR_CODES = new Set([
  'login_required', 'challenge_required', 'source_unavailable', 'source_route_changed',
]);
const SHARED_ERROR_CODES = [
  'freshness_deadline', 'freshness_route_changed', 'freshness_control_unavailable', 'freshness_reveal_unsupported',
];
const KNOWN_ERROR_CODES = [...SOURCE_ERROR_CODES, ...SHARED_ERROR_CODES];

const delay = ms => new Promise(resolve => setTimeout(resolve, ms));

/**
 * Qualify source freshness on the retained headless source page. This
 * controller deliberately does not own extraction, navigation, or the browser
 * window. Source route and primary identity rules come from the adapter.
 */
export async function recoverHeadlessFreshness({
  page, snapshot, options, deadlineAt, collectSnapshot, backend, source, contract,
}) {
  const startedAt = Date.now();
  const sourceName = boundedString(source, 128);
  const adapterFreshnessVersion = boundedString(contract?.version, 128);
  const requestedPolicy = enumValue(options?.sourceFreshnessPolicy,
    ['wake_and_reveal', 'preserve_frontier', 'preserve_target']);
  const base = (status, extra = {}) => ({
    schema: 'aku.headless-source-freshness.v1',
    source: sourceName,
    adapterFreshnessVersion,
    requestedPolicy,
    status,
    workerStatus: status,
    pendingContentScope: 'visible_page_controls',
    comparisonScope: 'same_capture_primary_posts',
    probeStatus: 'not_attempted',
    pendingContentDetected: null,
    pendingContentLabel: null,
    strategy: null,
    activationCount: 0,
    activationAttempts: 0,
    addedPrimaryCount: 0,
    changedPrimaryCount: 0,
    verifiedPrimaryCount: 0,
    elapsedMs: boundedCount(Date.now() - startedAt),
    ...extra,
  });
  const unchanged = freshness => ({ snapshot, freshness: { ...freshness, elapsedMs: boundedCount(Date.now() - startedAt) } });

  const contractShape = validateContractShape({ source: sourceName, options, contract });
  if (!contractShape.valid) {
    return unchanged(base('not_verified', { limitation: 'source_contract_unsupported' }));
  }

  if (backend !== 'headless_worker') return unchanged(base('not_applicable', { limitation: 'backend_unsupported' }));
  if (options?.acquisitionRound !== 1) return unchanged(base('preserved', { limitation: 'round_not_initial' }));
  if (options?.explicitPageUrl === true) return unchanged(base('not_applicable', { limitation: 'explicit_target' }));

  const contractState = validateContractContent({ options, snapshot, contract });
  if (!contractState.valid) {
    return unchanged(base('not_verified', { limitation: 'source_contract_unsupported' }));
  }
  if (!snapshot || !Array.isArray(snapshot.posts) || typeof page?.evaluate !== 'function') {
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: 'source_probe_unavailable' }));
  }
  if (typeof deadlineAt !== 'number' || !Number.isFinite(deadlineAt)) {
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: 'deadline_unavailable' }));
  }

  const phaseDeadline = Math.min(deadlineAt - CAPTURE_HEADROOM_MS, Date.now() + MAX_PHASE_MS);
  if (phaseDeadline <= Date.now()) {
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: 'capture_headroom_reserved' }));
  }

  let currentUrl;
  try {
    currentUrl = await evaluateBounded(page, 'location.href', phaseDeadline);
    assertApprovedRoute(currentUrl, options, contract);
    assertUsable(snapshot);
  } catch (error) {
    const sourceError = normalizedSourceError(error);
    if (sourceError) throw sourceError;
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: boundedFailure(error, phaseDeadline) }));
  }

  const probeRouteReserveMs = Math.min(ROUTE_CHECK_RESERVE_MS, Math.floor((phaseDeadline - Date.now()) / 4));
  const probeDeadline = phaseDeadline - probeRouteReserveMs;
  if (probeRouteReserveMs < 1 || probeDeadline <= Date.now()) {
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: 'capture_headroom_reserved' }));
  }

  let probe;
  const sourceArgument = JSON.stringify(sourceName);
  try {
    probe = await evaluateBounded(page,
      `(async () => { const runtime = globalThis.AkuSourceFreshnessRuntime; return typeof runtime?.probe === "function" ? await runtime.probe(${sourceArgument}) : null; })()`,
      probeDeadline);
  } catch (error) {
    const sourceError = normalizedSourceError(error);
    if (sourceError) throw sourceError;
    await requireRouteAfterProbe(page, options, contract, phaseDeadline);
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: boundedFailure(error, phaseDeadline) }));
  }
  const probeStateError = sourceStateError(probe);
  if (probeStateError) throw probeStateError;
  await requireRouteAfterProbe(page, options, contract, phaseDeadline);
  if (!isRecord(probe) || typeof probe.pendingContentDetected !== 'boolean') {
    return unchanged(base('unavailable', { probeStatus: 'unavailable', limitation: 'source_probe_unavailable' }));
  }

  const probeFields = {
    probeStatus: 'observed',
    pendingContentDetected: probe.pendingContentDetected,
    pendingContentLabel: safeLabel(probe.pendingContentLabel),
    strategy: safeStrategy(probe.strategy?.version ?? probe.strategy),
  };
  if (!probe.pendingContentDetected) {
    return unchanged(base('checked_no_pending', { ...probeFields, limitation: null }));
  }

  const mayReveal = options?.pendingContentPolicy === 'reveal_if_present'
    && options?.sourceFreshnessPolicy === 'wake_and_reveal'
    && options?.sameTabMutationAllowed === true;
  if (!mayReveal) {
    const status = options?.pendingContentPolicy === 'detect_only' ? 'pending_not_revealed' : 'preserved';
    return unchanged(base(status, { ...probeFields, limitation: policyLimitation(options) }));
  }
  if (typeof collectSnapshot !== 'function') {
    return unchanged(base('reveal_failed', { ...probeFields, limitation: 'snapshot_collection_unavailable' }));
  }

  // Re-check directly before the one allowed source mutation. The observed URL
  // is passed through exactly, including query and fragment, for helper-side
  // ownership checks.
  try {
    currentUrl = await evaluateBounded(page, 'location.href', phaseDeadline);
    assertApprovedRoute(currentUrl, options, contract);
  } catch (error) {
    const sourceError = normalizedSourceError(error);
    if (sourceError) throw sourceError;
    throw captureError('source_route_changed', 'source route could not be verified before freshness activation');
  }

  const routeReserveMs = Math.min(ROUTE_CHECK_RESERVE_MS, Math.floor((phaseDeadline - Date.now()) / 4));
  const activationDeadline = phaseDeadline - routeReserveMs;
  if (routeReserveMs < 1 || activationDeadline <= Date.now()) {
    return unchanged(base('unavailable', { ...probeFields, limitation: 'capture_headroom_reserved' }));
  }
  const activationArgs = JSON.stringify({ expectedPageUrl: currentUrl, deadlineAt: activationDeadline });
  let activation;
  try {
    activation = await evaluateBounded(page,
      `(async () => { const runtime = globalThis.AkuSourceFreshnessRuntime; if (typeof runtime?.activatePending !== "function") return { available: false }; return { available: true, result: await runtime.activatePending(${sourceArgument}, ${activationArgs}) }; })()`,
      activationDeadline);
  } catch (error) {
    const sourceError = normalizedSourceError(error);
    if (sourceError) throw sourceError;
    const sharedCode = sharedErrorCode(error);
    if (sharedCode === 'freshness_route_changed') {
      throw captureError('source_route_changed', 'source route changed during freshness activation');
    }
    if (sharedCode === 'freshness_control_unavailable') {
      await assertCurrentRoute(page, options, contract, phaseDeadline, true);
      return unchanged(base('reveal_unverified', {
        ...probeFields, activationAttempts: 1, limitation: 'pending_control_unavailable',
      }));
    }
    if (isRouteCode(error)) throw captureError('source_route_changed', 'source route changed during freshness activation');
    await assertCurrentRoute(page, options, contract, phaseDeadline, true);
    return unchanged(base('reveal_failed', {
      ...probeFields, activationAttempts: 1,
      activationCount: sharedCode ? 0 : null,
      limitation: sharedCode === 'freshness_reveal_unsupported' ? 'activation_unsupported' : boundedFailure(error, phaseDeadline),
    }));
  }
  if (!isRecord(activation) || activation.available !== true) {
    return unchanged(base('reveal_failed', { ...probeFields, activationAttempts: 1, limitation: 'activation_unavailable' }));
  }
  const activationResult = activation.result;
  const activationStateError = sourceStateError(activationResult);
  if (activationStateError) throw activationStateError;
  const activationFields = {
    ...probeFields,
    activationAttempts: 1,
    activationCount: activationResult?.activated === true ? 1 : 0,
    activationLabel: safeLabel(activationResult?.label),
  };
  if (!isRecord(activationResult) || activationResult.activated !== true) {
    await assertCurrentRoute(page, options, contract, phaseDeadline, true);
    return unchanged(base('reveal_unverified', { ...activationFields, limitation: 'pending_control_unavailable' }));
  }

  const pollingDeadline = phaseDeadline - routeReserveMs;
  const originalPrimary = contractState.primaryPosts;
  while (Date.now() < pollingDeadline) {
    // A navigation to login/challenge or another page makes the original feed
    // unsafe to admit as the post-activation result.
    await assertCurrentRoute(page, options, contract, phaseDeadline, true);
    let fresh;
    try {
      fresh = await within(Promise.resolve().then(() => collectSnapshot(pollingDeadline)), pollingDeadline);
    } catch (error) {
      const sourceError = normalizedSourceError(error);
      if (sourceError) throw sourceError;
      if (isRouteCode(error)) throw captureError('source_route_changed', 'source route changed during freshness polling');
      await assertCurrentRoute(page, options, contract, phaseDeadline, true);
      return unchanged(base('reveal_failed', { ...activationFields, limitation: boundedFailure(error, phaseDeadline) }));
    }
    const stateError = sourceStateError(fresh);
    if (stateError) throw stateError;
    if (typeof fresh?.url === 'string' && !isApprovedRoute(fresh.url, options, contract)) {
      throw captureError('source_route_changed', 'source route changed during freshness polling');
    }
    await assertCurrentRoute(page, options, contract, phaseDeadline, true);
    let freshPrimary;
    try {
      freshPrimary = primaryPosts(fresh, contract);
    } catch {
      return unchanged(base('reveal_failed', { ...activationFields, limitation: 'source_contract_unsupported' }));
    }
    const comparison = comparePrimaryPosts(originalPrimary, freshPrimary);
    if (comparison.addedPrimaryCount + comparison.changedPrimaryCount > 0) {
      return {
        snapshot: fresh,
        freshness: {
          ...base('verified', {
            ...activationFields,
            limitation: null,
            addedPrimaryCount: comparison.addedPrimaryCount,
            changedPrimaryCount: comparison.changedPrimaryCount,
            verifiedPrimaryCount: comparison.addedPrimaryCount + comparison.changedPrimaryCount,
          }),
          elapsedMs: boundedCount(Date.now() - startedAt),
        },
      };
    }
    await delay(Math.min(POLL_INTERVAL_MS, Math.max(1, pollingDeadline - Date.now())));
  }

  await assertCurrentRoute(page, options, contract, phaseDeadline, true);
  return unchanged(base('reveal_unverified', { ...activationFields, limitation: 'no_primary_content_change' }));
}

function primaryPosts(snapshot, contract) {
  const posts = Array.isArray(snapshot?.posts) ? snapshot.posts : [];
  const result = new Map();
  const ambiguous = new Set();
  for (const post of posts.slice(0, 100)) {
    const identity = contractIdentity(contract, post);
    if (identity === null) continue;
    if (identity === INVALID_IDENTITY) throw new TypeError('source adapter identity contract is malformed');
    const { id, permalink } = identity;
    if (result.has(id)) {
      const prior = result.get(id);
      if (prior.permalink !== permalink || normalizedContent(prior.text) !== normalizedContent(post?.text)) ambiguous.add(id);
      continue;
    }
    result.set(id, { permalink, text: typeof post?.text === 'string' ? post.text : null });
  }
  for (const identity of ambiguous) result.delete(identity);
  return result;
}

function validateContractShape({ source, options, contract }) {
  if (!source || !isRecord(contract) || contract.enabled !== true
      || boundedString(contract.source, 128) !== source
      || (typeof options?.source === 'string' && options.source !== source)
      || !boundedString(contract.version, 128)
      || typeof contract.matchesFeedURL !== 'function'
      || typeof contract.primaryIdentity !== 'function') return { valid: false };
  return { valid: true };
}

function validateContractContent({ options, snapshot, contract }) {
  const requestedRoute = boundedString(options?.pageUrl, 2048);
  if (!requestedRoute) return { valid: false, primaryPosts: new Map() };
  try {
    if (contract.matchesFeedURL(requestedRoute) !== true) return { valid: false, primaryPosts: new Map() };
    // Validate the adapter identity contract before evaluating the page. An
    // empty record is a harmless canary for adapters that accept source posts.
    if (contractIdentity(contract, {}) === INVALID_IDENTITY) return { valid: false, primaryPosts: new Map() };
    const primary = primaryPosts(snapshot, contract);
    return { valid: true, primaryPosts: primary };
  } catch {
    return { valid: false, primaryPosts: new Map() };
  }
}

const INVALID_IDENTITY = Symbol('invalid_source_identity');

function contractIdentity(contract, post) {
  let value;
  try { value = contract.primaryIdentity(post); } catch { return INVALID_IDENTITY; }
  if (value === null) return null;
  if (!isRecord(value) || !boundedString(value.id, 256) || !boundedString(value.permalink, 2048)) {
    return INVALID_IDENTITY;
  }
  return { id: value.id, permalink: value.permalink };
}

function comparePrimaryPosts(before, after) {
  let addedPrimaryCount = 0;
  let changedPrimaryCount = 0;
  for (const [identity, post] of after) {
    const prior = before.get(identity);
    if (!prior) {
      addedPrimaryCount++;
      continue;
    }
    if (typeof prior.text === 'string' && typeof post.text === 'string'
        && normalizedContent(prior.text) !== normalizedContent(post.text)) changedPrimaryCount++;
  }
  return { addedPrimaryCount, changedPrimaryCount };
}

function normalizedContent(value) {
  return typeof value === 'string' ? value.normalize('NFKC').replace(/\s+/gu, ' ').trim() : null;
}

async function assertCurrentRoute(page, options, contract, deadlineAt, afterMutation) {
  try {
    const actual = await evaluateBounded(page, 'location.href', deadlineAt);
    assertApprovedRoute(actual, options, contract);
  } catch (error) {
    const sourceError = normalizedSourceError(error);
    if (sourceError) throw sourceError;
    if (afterMutation) {
      throw captureError(Date.now() >= deadlineAt ? 'capture_timeout' : 'source_route_changed',
        'source route could not be verified during freshness polling');
    }
    throw error;
  }
}

async function requireRouteAfterProbe(page, options, contract, deadlineAt) {
  try {
    await assertCurrentRoute(page, options, contract, deadlineAt, false);
  } catch (error) {
    const sourceError = normalizedSourceError(error);
    if (sourceError) throw sourceError;
    throw captureError(error?.code === 'capture_timeout' ? 'capture_timeout' : 'source_route_changed',
      'source route could not be verified after freshness probing');
  }
}

function assertApprovedRoute(actual, options, contract) {
  if (!isApprovedRoute(actual, options, contract)) {
    throw captureError('source_route_changed', 'source route changed during freshness recovery');
  }
}

function isApprovedRoute(value, options, contract) {
  const requested = boundedString(options?.pageUrl, 2048);
  if (!boundedString(value, 2048) || !requested) return false;
  try {
    return contract.matchesFeedURL(requested) === true && contract.matchesFeedURL(value) === true;
  } catch { return false; }
}

function sourceStateError(value) {
  if (value?.loginRequired === true) return captureError('login_required', 'source requires an authenticated login');
  if (value?.challengeDetected === true) return captureError('challenge_required', 'source presented an access challenge');
  if (value?.sourceUnavailable === true) return captureError('source_unavailable', 'source reports that content is unavailable');
  return null;
}

function assertUsable(value) {
  const error = sourceStateError(value);
  if (error) throw error;
}

async function evaluateBounded(page, expression, deadlineAt) {
  const timeoutMs = remaining(deadlineAt);
  return within(Promise.resolve().then(() => page.evaluate(expression, timeoutMs)), deadlineAt);
}

function within(promise, deadlineAt) {
  const timeoutMs = remaining(deadlineAt);
  let timer;
  return Promise.race([
    Promise.resolve(promise),
    new Promise((_, reject) => {
      timer = setTimeout(() => reject(captureError('capture_timeout', 'source freshness phase timed out')), timeoutMs);
    }),
  ]).finally(() => clearTimeout(timer));
}

function remaining(deadlineAt) {
  const value = Math.trunc(deadlineAt - Date.now());
  if (value < 1) throw captureError('capture_timeout', 'source freshness phase timed out');
  return value;
}

function boundedFailure(error, deadlineAt) {
  const code = sharedErrorCode(error);
  if (Date.now() >= deadlineAt || error?.code === 'capture_timeout' || code === 'freshness_deadline') return 'phase_timeout';
  if (error?.code === 'source_probe_unavailable') return 'source_probe_unavailable';
  return 'runtime_unavailable';
}

function normalizedSourceError(error) {
  const code = errorCode(error);
  if (!SOURCE_ERROR_CODES.has(code)) return null;
  if (error?.code === code) return error;
  const messages = {
    login_required: 'source requires an authenticated login',
    challenge_required: 'source presented an access challenge',
    source_unavailable: 'source reports that content is unavailable',
    source_route_changed: 'source route changed during freshness recovery',
  };
  return captureError(code, messages[code]);
}

function isRouteCode(error) {
  return ['source_route_changed', 'navigation_changed', 'login_required', 'challenge_required', 'freshness_route_changed']
    .includes(errorCode(error));
}

function errorCode(error) {
  if (typeof error?.code === 'string') return error.code;
  const detail = [error?.message, error?.description, error?.data?.description]
    .filter(value => typeof value === 'string').join(' ').slice(0, 2000);
  return KNOWN_ERROR_CODES.find(code => new RegExp(`\\b${code}\\b`).test(detail)) || null;
}

function sharedErrorCode(error) {
  const code = errorCode(error);
  return SHARED_ERROR_CODES.includes(code) ? code : null;
}

function policyLimitation(options) {
  if (options?.pendingContentPolicy === 'detect_only') return 'detect_only_policy';
  if (options?.sameTabMutationAllowed !== true) return 'same_tab_mutation_not_allowed';
  if (options?.sourceFreshnessPolicy === 'preserve_frontier') return 'preserve_frontier_policy';
  if (options?.sourceFreshnessPolicy === 'preserve_target') return 'preserve_target_policy';
  return 'reveal_policy_not_enabled';
}

function safeLabel(value) {
  if (typeof value !== 'string') return null;
  const label = value.replace(/[\u0000-\u001f\u007f]/g, ' ').replace(/\s+/g, ' ').trim();
  return label ? label.slice(0, 120) : null;
}

function safeStrategy(value) {
  return typeof value === 'string' && /^[a-z0-9_-]{1,64}$/i.test(value) ? value : null;
}

function enumValue(value, allowed) {
  return allowed.includes(value) ? value : null;
}

function boundedCount(value) {
  return Number.isFinite(value) ? Math.max(0, Math.min(1_000_000, Math.trunc(value))) : 0;
}

function boundedString(value, maxLength) {
  return typeof value === 'string' && value.length > 0 && value.length <= maxLength && value.trim().length > 0
    ? value : null;
}

function isRecord(value) {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}
