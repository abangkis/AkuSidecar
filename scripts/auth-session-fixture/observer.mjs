import { randomBytes } from 'node:crypto';

const COOKIE_NAME = 'fixture_session';
const NONCE_HEADER = 'x-aku-fixture-refresh-nonce';
const MAX_TRACKED_REQUESTS = 512;
const DEFAULT_FLUSH_TIMEOUT_MS = 1000;

function validateOrigin(value) {
  let url;
  try { url = new URL(value); } catch { throw new Error('fixture_observer_invalid_origin'); }
  if (typeof value !== 'string' || url.protocol !== 'http:' || url.hostname !== '127.0.0.1' ||
      !url.port || url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== url.origin && value !== `${url.origin}/`) {
    throw new Error('fixture_observer_invalid_origin');
  }
  return url.origin;
}

function validateId(value, label) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 128 || !/^[A-Za-z0-9._:-]+$/.test(value)) {
    throw new Error(`fixture_observer_invalid_${label}`);
  }
  return value;
}

function sameOrigin(value, origin) {
  try { return new URL(value).origin === origin; } catch { return false; }
}

function headerValues(headers, name) {
  if (!headers || typeof headers !== 'object' || Array.isArray(headers)) return [];
  const values = [];
  for (const [key, value] of Object.entries(headers)) {
    if (key.toLowerCase() !== name) continue;
    if (Array.isArray(value)) values.push(...value.filter(item => typeof item === 'string'));
    else if (typeof value === 'string') values.push(...value.split(/\r?\n/));
  }
  return values;
}

function hasCookieLine(values, cookieName) {
  const escaped = cookieName.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
  const pattern = new RegExp(`^\\s*${escaped}\\s*=`, 'i');
  return values.some(value => pattern.test(value));
}

function hasSessionCookie(headers, blockedCookies, cookieName) {
  if (hasCookieLine(headerValues(headers, 'set-cookie'), cookieName)) return { found: true, blocked: false };
  if (Array.isArray(blockedCookies)) {
    for (const item of blockedCookies) {
      if (item && typeof item.cookieLine === 'string' && hasCookieLine([item.cookieLine], cookieName)) {
        return { found: true, blocked: true };
      }
    }
  }
  return { found: false, blocked: false };
}

function headerMatches(headers, name, expected) {
  if (!headers || typeof headers !== 'object' || Array.isArray(headers)) return false;
  for (const [key, value] of Object.entries(headers)) {
    if (key.toLowerCase() !== name) continue;
    if (Array.isArray(value) ? value.some(item => item === expected) : value === expected) return true;
  }
  return false;
}

/**
 * Observe session-cookie mutation attempts on one fixture page. The callback must
 * be synchronous so the owning controller can fence work as soon as CDP reports
 * an unexpected response. Coverage is limited to this page's Network domain.
 */
export async function observeSessionMutations(page, {
  origin,
  sessionId,
  clientId,
  sessionCookieName = COOKIE_NAME,
  onUnexpected,
  flushTimeoutMs = DEFAULT_FLUSH_TIMEOUT_MS,
} = {}) {
  origin = validateOrigin(origin);
  sessionId = validateId(sessionId, 'session');
  clientId = validateId(clientId, 'client');
  if (!page || page.origin !== origin || typeof page.subscribeNetwork !== 'function' ||
      typeof page.networkBarrier !== 'function' || typeof page.request !== 'function') {
    throw new Error('fixture_observer_invalid_page');
  }
  if (sessionCookieName !== COOKIE_NAME) throw new Error('fixture_observer_invalid_cookie_name');
  if (typeof onUnexpected !== 'function') throw new Error('fixture_observer_missing_callback');
  if (!Number.isInteger(flushTimeoutMs) || flushTimeoutMs < 1 || flushTimeoutMs > 5000) {
    throw new Error('fixture_observer_invalid_timeout');
  }

  const counts = {
    networkEvents: 0,
    responseEvents: 0,
    sessionMutationResponses: 0,
    blockedSessionCookieResponses: 0,
    controlledRefreshRequests: 0,
    controlledRefreshMutations: 0,
    unexpectedMutations: 0,
    coverageFailures: 0,
    callbackFailures: 0,
  };
  const requests = new Map();
  const pendingExtraInfo = new Map();
  const earlyExtraInfo = new Map();
  const waiters = new Set();
  let activeTicket = null;
  let unsubscribe = null;
  let closed = false;
  let faulted = false;
  let lastEvidence = 'none';

  function pendingCount() {
    let count = 0;
    for (const value of pendingExtraInfo.values()) count += value;
    return count;
  }

  function earlyCount() {
    let count = 0;
    for (const value of earlyExtraInfo.values()) count += value;
    return count;
  }

  function settleWaiters() {
    if (pendingCount() !== 0 || earlyCount() !== 0) return;
    for (const resolve of [...waiters]) resolve();
  }

  function callback(signal) {
    try {
      const result = onUnexpected(Object.freeze({ ...signal, sessionId, clientId }));
      if (result && typeof result.then === 'function') {
        result.catch(() => {});
        throw new Error('asynchronous_observer_callback');
      }
    } catch {
      counts.callbackFailures++;
      counts.coverageFailures++;
      faulted = true;
      lastEvidence = 'callback_failed';
    }
  }

  function loseCoverage() {
    if (closed || faulted) return;
    counts.coverageFailures++;
    faulted = true;
    lastEvidence = 'coverage_lost';
    callback({ kind: 'observation_lost' });
  }

  function mutation(requestId, blocked = false) {
    let request = requests.get(requestId);
    if (!request) {
      request = { controlled: false, mutationReported: false, blockedReported: false, completed: false };
      requests.set(requestId, request);
    }
    if (blocked && !request.blockedReported) {
      request.blockedReported = true;
      counts.blockedSessionCookieResponses++;
    }
    if (blocked && request.controlled) {
      loseCoverage();
      if (!request.mutationReported) lastEvidence = 'blocked_session_cookie';
    }
    if (request.mutationReported) return;
    request.mutationReported = true;
    counts.sessionMutationResponses++;
    if (request.controlled && !request.redirected) {
      counts.controlledRefreshMutations++;
      lastEvidence = 'controlled_refresh_session_cookie';
      return;
    }
    counts.unexpectedMutations++;
    lastEvidence = 'unexpected_session_cookie';
    callback({ kind: 'unexpected_session_mutation' });
  }

  function expectExtraInfo(requestId) {
    const early = earlyExtraInfo.get(requestId) ?? 0;
    if (early > 0) {
      if (early === 1) earlyExtraInfo.delete(requestId);
      else earlyExtraInfo.set(requestId, early - 1);
      return;
    }
    pendingExtraInfo.set(requestId, (pendingExtraInfo.get(requestId) ?? 0) + 1);
  }

  function receivedExtraInfo(requestId) {
    const pending = pendingExtraInfo.get(requestId) ?? 0;
    if (pending > 0) {
      if (pending === 1) pendingExtraInfo.delete(requestId);
      else pendingExtraInfo.set(requestId, pending - 1);
    } else {
      if (earlyExtraInfo.size >= MAX_TRACKED_REQUESTS && !earlyExtraInfo.has(requestId)) loseCoverage();
      earlyExtraInfo.set(requestId, (earlyExtraInfo.get(requestId) ?? 0) + 1);
    }
    const request = requests.get(requestId);
    if (request?.completed && !pendingExtraInfo.has(requestId)) requests.delete(requestId);
    settleWaiters();
  }

  function markCompleted(requestId) {
    const request = requests.get(requestId);
    if (!request) return;
    request.completed = true;
    if (!pendingExtraInfo.has(requestId)) requests.delete(requestId);
  }

  function requestWillBeSent(params) {
    const requestId = params.requestId;
    if (typeof requestId !== 'string') { loseCoverage(); return; }
    const prior = requests.get(requestId);
    if (params.redirectResponse) {
      const evidence = hasSessionCookie(params.redirectResponse.headers, params.redirectResponse.blockedCookies, COOKIE_NAME);
      if (evidence.found) mutation(requestId, evidence.blocked);
      if (prior) {
        prior.controlled = false;
        prior.redirected = true;
        prior.mutationReported = false;
        prior.blockedReported = false;
        prior.completed = false;
      }
      if (params.redirectHasExtraInfo === true) expectExtraInfo(requestId);
      loseCoverage();
      requests.set(requestId, { controlled: false, redirected: true, mutationReported: false, blockedReported: false, completed: false });
      return;
    }
    if (prior) loseCoverage();
    const request = params.request ?? {};
    const allowed = sameOrigin(request.url, origin);
    const state = { controlled: false, redirected: false, mutationReported: false, blockedReported: false, completed: false };
    if (!allowed) loseCoverage();
    if (activeTicket && headerMatches(request.headers, NONCE_HEADER, activeTicket.nonce)) {
      if (!allowed || request.method !== activeTicket.method || activeTicket.matched) {
        loseCoverage();
      } else {
        state.controlled = true;
        activeTicket.matched = true;
        activeTicket.requestId = requestId;
        counts.controlledRefreshRequests++;
      }
    }
    if (requests.size >= MAX_TRACKED_REQUESTS) {
      loseCoverage();
      return;
    }
    requests.set(requestId, state);
  }

  function onNetworkEvent(event) {
    if (closed) return;
    try {
      const { method, params = {} } = event ?? {};
      if (typeof method !== 'string' || !method.startsWith('Network.')) return;
      counts.networkEvents++;
      if (method === 'Network.requestWillBeSent') {
        requestWillBeSent(params);
      } else if (method === 'Network.responseReceived') {
        counts.responseEvents++;
        if (typeof params.requestId !== 'string') { loseCoverage(); return; }
        const evidence = hasSessionCookie(params.response?.headers, undefined, COOKIE_NAME);
        if (evidence.found) mutation(params.requestId, false);
        if (params.hasExtraInfo === true) expectExtraInfo(params.requestId);
        settleWaiters();
      } else if (method === 'Network.responseReceivedExtraInfo') {
        if (typeof params.requestId !== 'string') { loseCoverage(); return; }
        const evidence = hasSessionCookie(params.headers, params.blockedCookies, COOKIE_NAME);
        if (evidence.found) mutation(params.requestId, evidence.blocked);
        receivedExtraInfo(params.requestId);
      } else if (method === 'Network.loadingFinished' || method === 'Network.loadingFailed') {
        if (typeof params.requestId === 'string') markCompleted(params.requestId);
      }
    } catch {
      loseCoverage();
    }
  }

  function status() {
    return Object.freeze({
      scope: 'page_network_only',
      state: faulted ? 'faulted' : closed ? 'closed' : 'active',
      ...counts,
      pendingExtraInfo: pendingCount(),
      lastEvidence,
    });
  }

  function ensureActive() {
    if (closed) throw new Error('fixture_observer_closed');
    if (faulted) throw new Error('fixture_observer_unavailable');
  }

  async function flush() {
    ensureActive();
    try { await page.networkBarrier(); } catch {
      loseCoverage();
      throw new Error('fixture_observer_barrier_failed');
    }
    if (faulted) throw new Error('fixture_observer_unavailable');
    if (pendingCount() > 0 || earlyCount() > 0) {
      await new Promise(resolve => {
        let settled = false;
        const finish = () => {
          if (settled) return;
          settled = true;
          clearTimeout(timer);
          waiters.delete(finish);
          resolve();
        };
        const timer = setTimeout(finish, flushTimeoutMs);
        waiters.add(finish);
        settleWaiters();
      });
    }
    if (pendingCount() > 0 || earlyCount() > 0) {
      loseCoverage();
      throw new Error('fixture_observer_extra_info_incomplete');
    }
    if (faulted) throw new Error('fixture_observer_unavailable');
    return status();
  }

  async function authorizeRefresh(run) {
    ensureActive();
    if (typeof run !== 'function' || activeTicket) throw new Error('fixture_observer_refresh_busy');
    const ticket = { nonce: randomBytes(32).toString('hex'), used: false, matched: false, method: null, requestId: null };
    activeTicket = ticket;
    let requestPromise;
    const request = (path, method = 'POST') => {
      if (ticket.used) throw new Error('fixture_observer_refresh_request_used');
      if (!['GET', 'POST'].includes(method)) throw new Error('fixture_observer_invalid_refresh_method');
      ticket.used = true;
      ticket.method = method;
      requestPromise = page.request(path, method, { refreshNonce: ticket.nonce });
      requestPromise.catch(() => {});
      return requestPromise;
    };
    try {
      const result = await run(request);
      if (!ticket.used) throw new Error('fixture_observer_refresh_request_missing');
      await requestPromise;
      const before = status();
      await flush();
      if (!ticket.matched || before.state === 'faulted') {
        loseCoverage();
        throw new Error('fixture_observer_refresh_identity_unverified');
      }
      return result;
    } finally {
      activeTicket = null;
      ticket.nonce = '';
    }
  }

  function close() {
    if (closed) return;
    closed = true;
    activeTicket = null;
    if (unsubscribe) unsubscribe();
    unsubscribe = null;
    for (const resolve of [...waiters]) resolve();
    waiters.clear();
  }

  try {
    unsubscribe = await page.subscribeNetwork(onNetworkEvent, loseCoverage);
  } catch {
    throw new Error('fixture_observer_subscribe_failed');
  }

  return Object.freeze({ authorizeRefresh, flush, status, close });
}
