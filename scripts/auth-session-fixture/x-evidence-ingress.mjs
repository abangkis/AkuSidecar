// Offline transport metadata only. No browser connection, body reader, account
// verifier or Store authorization is implemented here. Aliases are fixture-local
// references and do not establish anonymity or native source qualification.
const OPERATION_ID = 'x-fixture-operation-v1';
const OPERATION_PATH = '/x-fixture-operation';
const OPERATION_METHOD = 'POST';

const HARD_LIMITS = Object.freeze({
  maxFrameBytes: 1_048_576,
  maxInputBytes: 4_194_304,
  maxFramesPerPush: 1_024,
  maxPending: 1_024,
  maxOrphans: 512,
  maxAliases: 4_096,
  maxOutputRecords: 1_024,
  maxAgeMs: 60_000,
  maxWindowMs: 120_000,
  maxJSONDepth: 64,
  maxJSONTokens: 65_536,
});

const DEFAULT_LIMITS = Object.freeze({
  maxFrameBytes: 262_144,
  maxInputBytes: 1_048_576,
  maxFramesPerPush: 128,
  maxPending: 256,
  maxOrphans: 128,
  maxAliases: 1_024,
  maxOutputRecords: 256,
  maxAgeMs: 30_000,
  maxWindowMs: 30_000,
  maxJSONDepth: 32,
  maxJSONTokens: 16_384,
});

const NETWORK_METHODS = new Set([
  'Network.requestWillBeSent',
  'Network.responseReceived',
  'Network.responseReceivedExtraInfo',
  'Network.requestServedFromCache',
  'Network.loadingFinished',
  'Network.loadingFailed',
]);

const REASONS = new Set([
  'invalid_input', 'input_chunk_too_large', 'too_many_frames', 'frame_too_large',
  'invalid_utf8', 'malformed_json', 'invalid_clock', 'window_not_started',
  'window_expired', 'request_outside_window', 'event_outside_window',
  'unsupported_request', 'scope_mismatch', 'owner_changed', 'target_lost',
  'source_detached', 'source_lost', 'source_closed', 'unterminated_frame',
  'pending_limit', 'orphan_limit', 'alias_limit', 'output_limit',
  'request_id_collision', 'redirect_unsupported', 'extra_info_unexpected',
  'extra_info_duplicate', 'extra_info_missing', 'extra_info_ambiguous',
  'response_missing', 'response_conflict', 'response_status_ambiguous',
  'response_url_unsupported', 'event_order_ambiguous', 'terminal_conflict',
  'correlation_expired', 'incomplete_operation',
]);

function fixedError() {
  return new TypeError('invalid_fixture_x_ingress_configuration');
}

function boundedIdentifier(value) {
  return typeof value === 'string' && value.length > 0 && value.length <= 256 &&
    !/[\u0000-\u001f\u007f]/u.test(value);
}

function normalizeLimits(input) {
  if (input !== undefined && (!input || typeof input !== 'object' || Array.isArray(input))) throw fixedError();
  const result = {};
  for (const key of Object.keys(DEFAULT_LIMITS)) {
    const value = input?.[key] === undefined ? DEFAULT_LIMITS[key] : input[key];
    if (!Number.isSafeInteger(value) || value <= 0 || value > HARD_LIMITS[key]) throw fixedError();
    result[key] = value;
  }
  if (result.maxAliases < 5) throw fixedError();
  return Object.freeze(result);
}

function normalizeScope(scope) {
  if (!scope || typeof scope !== 'object' || Array.isArray(scope) ||
      !boundedIdentifier(scope.ownerId) || !boundedIdentifier(scope.contextId) ||
      !boundedIdentifier(scope.sessionId) || !boundedIdentifier(scope.targetId) ||
      typeof scope.origin !== 'string' || scope.origin.length > 128) throw fixedError();
  let origin;
  try { origin = new URL(scope.origin); } catch { throw fixedError(); }
  if (origin.protocol !== 'http:' || origin.hostname !== '127.0.0.1' || !origin.port ||
      origin.username || origin.password || origin.pathname !== '/' || origin.search || origin.hash ||
      origin.origin !== scope.origin) throw fixedError();
  return {
    ownerId: scope.ownerId,
    contextId: scope.contextId,
    sessionId: scope.sessionId,
    targetId: scope.targetId,
    origin: origin.origin,
  };
}

function normalizeWindow(window, now, limits) {
  if (!window || typeof window !== 'object' || Array.isArray(window) ||
      !boundedIdentifier(window.id) || !Number.isFinite(window.startTimestamp) ||
      !Number.isFinite(window.endTimestamp) || window.startTimestamp < 0 ||
      window.endTimestamp <= window.startTimestamp ||
      (window.endTimestamp - window.startTimestamp) * 1000 > limits.maxWindowMs ||
      !Number.isSafeInteger(window.openedAtMs) || !Number.isSafeInteger(window.expiresAtMs) ||
      window.expiresAtMs <= window.openedAtMs ||
      window.expiresAtMs - window.openedAtMs > limits.maxWindowMs || now < window.openedAtMs) throw fixedError();
  return {
    id: window.id,
    startTimestamp: window.startTimestamp,
    endTimestamp: window.endTimestamp,
    openedAtMs: window.openedAtMs,
    expiresAtMs: window.expiresAtMs,
  };
}

function isValidUnicode(value) {
  for (let i = 0; i < value.length; i++) {
    const code = value.charCodeAt(i);
    if (code >= 0xd800 && code <= 0xdbff) {
      const next = value.charCodeAt(i + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      i++;
    } else if (code >= 0xdc00 && code <= 0xdfff) return false;
  }
  return true;
}

function parseStrictJSON(text, limits) {
  let index = 0;
  let tokens = 0;

  const fail = () => { throw new SyntaxError('invalid_json'); };
  const whitespace = () => {
    while (index < text.length && (text[index] === ' ' || text[index] === '\t' ||
      text[index] === '\r' || text[index] === '\n')) index++;
  };
  const string = () => {
    if (text[index] !== '"') fail();
    const start = index++;
    while (index < text.length) {
      const code = text.charCodeAt(index);
      if (code === 0x22) {
        index++;
        let value;
        try { value = JSON.parse(text.slice(start, index)); } catch { fail(); }
        if (typeof value !== 'string' || !isValidUnicode(value)) fail();
        return value;
      }
      if (code < 0x20) fail();
      if (code === 0x5c) {
        index++;
        if (index >= text.length || !'"\\/bfnrtu'.includes(text[index])) fail();
        if (text[index] === 'u') {
          for (let offset = 1; offset <= 4; offset++) {
            const digit = text[index + offset];
            if (digit === undefined || !/[0-9a-f]/iu.test(digit)) fail();
          }
          index += 5;
        } else index++;
      } else index++;
    }
    fail();
  };
  const value = depth => {
    whitespace();
    if (depth > limits.maxJSONDepth || ++tokens > limits.maxJSONTokens) fail();
    const first = text[index];
    if (first === '"') return string();
    if (first === '{') {
      index++;
      whitespace();
      const output = Object.create(null);
      const keys = new Set();
      if (text[index] === '}') { index++; return output; }
      while (index < text.length) {
        whitespace();
        const key = string();
        if (keys.has(key)) fail();
        keys.add(key);
        whitespace();
        if (text[index++] !== ':') fail();
        output[key] = value(depth + 1);
        whitespace();
        if (text[index] === '}') { index++; return output; }
        if (text[index++] !== ',') fail();
      }
      fail();
    }
    if (first === '[') {
      index++;
      whitespace();
      const output = [];
      if (text[index] === ']') { index++; return output; }
      while (index < text.length) {
        output.push(value(depth + 1));
        whitespace();
        if (text[index] === ']') { index++; return output; }
        if (text[index++] !== ',') fail();
      }
      fail();
    }
    if (text.startsWith('true', index)) { index += 4; return true; }
    if (text.startsWith('false', index)) { index += 5; return false; }
    if (text.startsWith('null', index)) { index += 4; return null; }
    const start = index;
    if (text[index] === '-') index++;
    if (text[index] === '0') index++;
    else {
      if (!(text[index] >= '1' && text[index] <= '9')) fail();
      while (text[index] >= '0' && text[index] <= '9') index++;
    }
    if (text[index] === '.') {
      index++;
      if (!(text[index] >= '0' && text[index] <= '9')) fail();
      while (text[index] >= '0' && text[index] <= '9') index++;
    }
    if (text[index] === 'e' || text[index] === 'E') {
      index++;
      if (text[index] === '+' || text[index] === '-') index++;
      if (!(text[index] >= '0' && text[index] <= '9')) fail();
      while (text[index] >= '0' && text[index] <= '9') index++;
    }
    const number = Number(text.slice(start, index));
    if (!Number.isFinite(number) || (Number.isInteger(number) && !Number.isSafeInteger(number))) fail();
    return number;
  };

  const parsed = value(0);
  whitespace();
  if (index !== text.length) fail();
  return parsed;
}

function recordStatus(value) {
  return Number.isInteger(value) && value >= 0 && value <= 599 ? value : null;
}

function own(object, key) {
  return Object.prototype.hasOwnProperty.call(object, key);
}

function safeObject(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

export function createFixtureXEvidenceIngress({ scope: rawScope, window: rawWindow, clock, limits: rawLimits } = {}) {
  if (typeof clock !== 'function') throw fixedError();
  const limits = normalizeLimits(rawLimits);
  const scope = normalizeScope(rawScope);
  let now;
  try { now = clock(); } catch { throw fixedError(); }
  if (!Number.isSafeInteger(now)) throw fixedError();
  let lastClockMs = now;
  let window = normalizeWindow(rawWindow, now, limits);

  const frameBuffer = new Uint8Array(limits.maxFrameBytes);
  const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
  let frameLength = 0;
  let aliases = new Map();
  let aliasSequence = 0;
  let output = [];
  let correlations = new Map();
  let orphanExtraInfo = new Map();
  let unavailableReason = null;
  let finished = false;
  let closed = false;
  let recordSequence = 0;

  const aliasFor = (kind, raw) => {
    let byValue = aliases.get(kind);
    if (!byValue) { byValue = new Map(); aliases.set(kind, byValue); }
    if (byValue.has(raw)) return byValue.get(raw);
    let count = 0;
    for (const values of aliases.values()) count += values.size;
    if (count >= limits.maxAliases) { markUnavailable('alias_limit'); return null; }
    const alias = `a${++aliasSequence}`;
    byValue.set(raw, alias);
    return alias;
  };

  const scopeAliases = {
    owner: aliasFor('owner', scope.ownerId),
    context: aliasFor('context', scope.contextId),
    target: aliasFor('target', scope.targetId),
    session: aliasFor('session', scope.sessionId),
    window: aliasFor('window', window.id),
  };

  function clearTransient({ clearOutput = false, clearAliases = true } = {}) {
    frameBuffer.fill(0);
    frameLength = 0;
    correlations.clear();
    orphanExtraInfo.clear();
    if (clearAliases) aliases.clear();
    if (clearOutput) output = [];
    scope.ownerId = null;
    scope.contextId = null;
    scope.sessionId = null;
    scope.targetId = null;
    window = null;
  }

  function markUnavailable(reason) {
    if (!unavailableReason) unavailableReason = REASONS.has(reason) ? reason : 'source_lost';
    finished = true;
    clearTransient({ clearOutput: false });
  }

  function readClock() {
    let value;
    try { value = clock(); } catch { markUnavailable('invalid_clock'); return null; }
    if (!Number.isSafeInteger(value) || value < lastClockMs) { markUnavailable('invalid_clock'); return null; }
    lastClockMs = value;
    return value;
  }

  function projectedOffset(timestamp, reason = 'event_outside_window') {
    if (!window || typeof timestamp !== 'number' || !Number.isFinite(timestamp) ||
        timestamp < window.startTimestamp || timestamp > window.endTimestamp) {
      markUnavailable(reason);
      return null;
    }
    const offset = (timestamp - window.startTimestamp) * 1000;
    const maxOffset = (window.endTimestamp - window.startTimestamp) * 1000;
    if (!Number.isFinite(offset) || offset < 0 || offset > maxOffset) {
      markUnavailable(reason);
      return null;
    }
    return offset;
  }

  function tick(nowValue = undefined) {
    if (finished || unavailableReason) return false;
    const current = nowValue ?? readClock();
    if (current === null || unavailableReason) return false;
    if (!window) { markUnavailable('window_expired'); return false; }
    if (current < window.openedAtMs) { markUnavailable('window_not_started'); return false; }
    if (current >= window.expiresAtMs) { markUnavailable('window_expired'); return false; }
    for (const [requestId, correlation] of correlations) {
      if (!correlation.completed && current - correlation.createdAtMs > limits.maxAgeMs) {
        markUnavailable(correlation.extraExpected === true && correlation.extraInfoStatus === null
          ? 'extra_info_missing' : 'correlation_expired');
        return false;
      }
      void requestId;
    }
    for (const [requestId, extra] of orphanExtraInfo) {
      if (current - extra.createdAtMs > limits.maxAgeMs) {
        markUnavailable('extra_info_ambiguous');
        return false;
      }
      void requestId;
    }
    return true;
  }

  function emitRecord(correlation, terminalPhase, terminalOffset, reason = null) {
    if (output.length >= limits.maxOutputRecords) { markUnavailable('output_limit'); return; }
    const record = {
      operationId: OPERATION_ID,
      owner: scopeAliases.owner,
      context: scopeAliases.context,
      target: scopeAliases.target,
      session: scopeAliases.session,
      request: correlation.alias,
      window: scopeAliases.window,
      sequence: ++recordSequence,
      phase: terminalPhase,
      requestOffsetMs: correlation.requestOffsetMs,
      responseOffsetMs: correlation.responseOffsetMs,
      completionOffsetMs: terminalOffset,
      statusCode: correlation.responseStatus,
      extraInfoStatusCode: correlation.extraInfoStatus,
      extraInfoExpected: correlation.extraExpected,
      extraInfoReceived: correlation.extraInfoStatus !== null,
      cache: correlation.cacheKind,
      authorizing: false,
      ...(reason ? { reason } : {}),
    };
    output.push(Object.freeze(record));
  }

  function exactOperationRequest(urlValue, method) {
    if (typeof urlValue !== 'string' || urlValue.length > 2_048) return 'ignore';
    const prefix = `${scope.origin}${OPERATION_PATH}`;
    if (urlValue !== prefix && !urlValue.startsWith(`${prefix}?`) && !urlValue.startsWith(`${prefix}#`)) return 'ignore';
    let url;
    try { url = new URL(urlValue); } catch { return 'unsupported'; }
    if (url.origin !== scope.origin || url.pathname !== OPERATION_PATH || url.search || url.hash ||
        url.username || url.password || url.href !== prefix || method !== OPERATION_METHOD) return 'unsupported';
    return 'match';
  }

  function validRequestId(requestId) {
    return typeof requestId === 'string' && requestId.length > 0 && requestId.length <= 256 &&
      !/[\u0000-\u001f\u007f]/u.test(requestId);
  }

  function requestWillBeSent(params, arrivalMs) {
    if (!safeObject(params) || !safeObject(params.request)) { markUnavailable('malformed_json'); return; }
    const candidate = exactOperationRequest(params.request.url, params.request.method);
    const requestId = params.requestId;
    const existing = validRequestId(requestId) ? correlations.get(requestId) : null;
    if (existing) {
      markUnavailable(params.redirectResponse ? 'redirect_unsupported' : 'request_id_collision');
      return;
    }
    if (candidate === 'ignore') return;
    if (candidate !== 'match') { markUnavailable('unsupported_request'); return; }
    if (!validRequestId(requestId)) { markUnavailable('unsupported_request'); return; }
    if (params.redirectResponse !== undefined && params.redirectResponse !== null) {
      markUnavailable('redirect_unsupported');
      return;
    }
    const requestOffsetMs = projectedOffset(params.timestamp, 'request_outside_window');
    if (requestOffsetMs === null || unavailableReason) return;
    if (correlations.size >= limits.maxPending) { markUnavailable('pending_limit'); return; }
    const alias = aliasFor('request', requestId);
    if (!alias || unavailableReason) return;
    const correlation = {
      alias,
      requestOffsetMs,
      requestTimestamp: params.timestamp,
      responseOffsetMs: null,
      responseTimestamp: null,
      responseStatus: null,
      extraExpected: null,
      extraInfoStatus: null,
      cacheEvent: false,
      diskCacheFlag: null,
      diskCache: false,
      prefetchCacheFlag: null,
      prefetchCache: false,
      serviceWorkerFlag: null,
      serviceWorker: false,
      cacheKind: 'unreported',
      responseSeen: false,
      completed: false,
      terminalPhase: null,
      terminalOffsetMs: null,
      terminalTimestamp: null,
      createdAtMs: arrivalMs,
    };
    const early = orphanExtraInfo.get(requestId);
    if (early) {
      correlation.extraInfoStatus = early.statusCode;
      orphanExtraInfo.delete(requestId);
    }
    correlations.set(requestId, correlation);
  }

  function responseReceived(params, arrivalMs) {
    if (!safeObject(params)) { markUnavailable('malformed_json'); return; }
    if (!validRequestId(params.requestId)) { markUnavailable('malformed_json'); return; }
    const correlation = validRequestId(params.requestId) ? correlations.get(params.requestId) : null;
    if (!correlation) return;
    if (correlation.completed) { markUnavailable('response_conflict'); return; }
    const response = params.response;
    if (!safeObject(response)) { markUnavailable('response_conflict'); return; }
    const status = recordStatus(response.status);
    if (status === null || typeof params.hasExtraInfo !== 'boolean') { markUnavailable('response_conflict'); return; }
    if (exactOperationRequest(response.url, OPERATION_METHOD) !== 'match') {
      markUnavailable('response_url_unsupported');
      return;
    }
    const responseOffsetMs = projectedOffset(params.timestamp);
    if (responseOffsetMs === null || unavailableReason) return;
    if (params.timestamp < correlation.requestTimestamp) { markUnavailable('event_order_ambiguous'); return; }
    for (const key of ['fromDiskCache', 'fromServiceWorker']) {
      if (own(response, key) && typeof response[key] !== 'boolean') { markUnavailable('response_conflict'); return; }
    }
    if (own(response, 'fromPrefetchCache') && typeof response.fromPrefetchCache !== 'boolean') {
      markUnavailable('response_conflict');
      return;
    }
    const diskCache = response.fromDiskCache === true;
    const prefetchCache = response.fromPrefetchCache === true;
    const serviceWorker = response.fromServiceWorker === true;
    const diskCacheFlag = own(response, 'fromDiskCache') ? response.fromDiskCache : null;
    const prefetchCacheFlag = own(response, 'fromPrefetchCache') ? response.fromPrefetchCache : null;
    const serviceWorkerFlag = own(response, 'fromServiceWorker') ? response.fromServiceWorker : null;
    if (correlation.responseSeen) {
      if (correlation.responseStatus !== status || correlation.extraExpected !== params.hasExtraInfo ||
          correlation.responseTimestamp !== params.timestamp || correlation.diskCacheFlag !== diskCacheFlag ||
          correlation.prefetchCacheFlag !== prefetchCacheFlag || correlation.serviceWorkerFlag !== serviceWorkerFlag) {
        markUnavailable('response_conflict');
      }
      return;
    }
    if (!params.hasExtraInfo && correlation.extraInfoStatus !== null) {
      markUnavailable('extra_info_unexpected');
      return;
    }
    correlation.responseSeen = true;
    correlation.responseStatus = status;
    correlation.responseOffsetMs = responseOffsetMs;
    correlation.responseTimestamp = params.timestamp;
    correlation.extraExpected = params.hasExtraInfo;
    correlation.diskCacheFlag = diskCacheFlag;
    correlation.diskCache = diskCache;
    correlation.prefetchCacheFlag = prefetchCacheFlag;
    correlation.prefetchCache = prefetchCache;
    correlation.serviceWorkerFlag = serviceWorkerFlag;
    correlation.serviceWorker = serviceWorker;
    refreshCacheKind(correlation);
    void arrivalMs;
  }

  function extraInfoReceived(params, arrivalMs) {
    if (!safeObject(params)) { markUnavailable('malformed_json'); return; }
    const requestId = params.requestId;
    if (!validRequestId(requestId)) { markUnavailable('malformed_json'); return; }
    const statusCode = recordStatus(params.statusCode);
    if (statusCode === null) { markUnavailable('extra_info_ambiguous'); return; }
    const correlation = correlations.get(requestId);
    if (!correlation) {
      const existing = orphanExtraInfo.get(requestId);
      if (existing) {
        markUnavailable('extra_info_duplicate');
        return;
      }
      if (orphanExtraInfo.size >= limits.maxOrphans) { markUnavailable('orphan_limit'); return; }
      orphanExtraInfo.set(requestId, { statusCode, createdAtMs: arrivalMs });
      return;
    }
    if (correlation.extraInfoStatus !== null) { markUnavailable('extra_info_duplicate'); return; }
    if (correlation.responseSeen && correlation.extraExpected !== true) {
      markUnavailable('extra_info_unexpected');
      return;
    }
    correlation.extraInfoStatus = statusCode;
    if (correlation.terminalPhase) finalizeTerminal(correlation);
  }

  function refreshCacheKind(correlation) {
    correlation.cacheKind = correlation.cacheEvent || correlation.diskCache
      ? (correlation.serviceWorker ? 'cache_and_service_worker' : 'served_from_cache')
      : correlation.prefetchCache ? 'served_from_prefetch_cache'
        : correlation.serviceWorker ? 'served_from_service_worker' : 'unreported';
  }

  function finalizeTerminal(correlation) {
    if (!correlation.terminalPhase || correlation.completed) return;
    if (correlation.extraExpected === true && correlation.extraInfoStatus === null) return;
    if (correlation.extraExpected === false && correlation.extraInfoStatus !== null) {
      markUnavailable('extra_info_unexpected');
      return;
    }
    if (correlation.responseSeen && correlation.extraInfoStatus !== null &&
        correlation.responseStatus !== correlation.extraInfoStatus) {
      const revalidated = correlation.responseStatus === 200 && correlation.extraInfoStatus === 304;
      const explicitCache = correlation.cacheKind === 'served_from_cache' ||
        correlation.cacheKind === 'cache_and_service_worker' ||
        correlation.cacheKind === 'served_from_prefetch_cache';
      if (!revalidated) { markUnavailable('response_status_ambiguous'); return; }
      if (revalidated && !explicitCache) correlation.cacheKind = 'revalidated';
    }
    correlation.completed = true;
    emitRecord(correlation, correlation.terminalPhase, correlation.terminalOffsetMs,
      correlation.terminalPhase === 'failed' ? 'loading_failed' : null);
  }

  function cacheServed(params) {
    if (!safeObject(params) || !validRequestId(params.requestId)) { markUnavailable('malformed_json'); return; }
    const correlation = validRequestId(params.requestId) ? correlations.get(params.requestId) : null;
    if (!correlation) return;
    if (correlation.completed) {
      if (!correlation.cacheEvent && !correlation.diskCache) markUnavailable('response_conflict');
      return;
    }
    correlation.cacheEvent = true;
    refreshCacheKind(correlation);
  }

  function terminalEvent(method, params, arrivalMs) {
    if (!safeObject(params) || !validRequestId(params.requestId)) { markUnavailable('malformed_json'); return; }
    const correlation = correlations.get(params.requestId);
    if (!correlation) return;
    const phase = method === 'Network.loadingFinished' ? 'completed' : 'failed';
    const offset = projectedOffset(params.timestamp);
    if (offset === null || unavailableReason) return;
    const previousTimestamp = correlation.responseSeen ? correlation.responseTimestamp : correlation.requestTimestamp;
    if (params.timestamp < previousTimestamp) { markUnavailable('event_order_ambiguous'); return; }
    if (correlation.completed) {
      if (correlation.terminalPhase !== phase || correlation.terminalTimestamp !== params.timestamp) {
        markUnavailable('terminal_conflict');
      }
      return;
    }
    if (phase === 'completed' && !correlation.responseSeen) {
      markUnavailable('response_missing');
      return;
    }
    if (!correlation.responseSeen && correlation.extraInfoStatus !== null) {
      markUnavailable('extra_info_ambiguous');
      return;
    }
    if (correlation.responseSeen && correlation.extraExpected === null) {
      markUnavailable('response_conflict');
      return;
    }
    if (correlation.terminalPhase) {
      if (correlation.terminalPhase !== phase || correlation.terminalTimestamp !== params.timestamp) {
        markUnavailable('terminal_conflict');
      }
      return;
    }
    correlation.terminalPhase = phase;
    correlation.terminalOffsetMs = offset;
    correlation.terminalTimestamp = params.timestamp;
    finalizeTerminal(correlation);
    void arrivalMs;
  }

  function projectMessage(message, arrivalMs) {
    if (!safeObject(message)) { markUnavailable('malformed_json'); return; }
    if (!own(message, 'method')) {
      if (own(message, 'id') && (own(message, 'result') || own(message, 'error'))) return;
      markUnavailable('malformed_json');
      return;
    }
    if (typeof message.method !== 'string') { markUnavailable('malformed_json'); return; }
    if (message.method === 'Target.targetDestroyed') {
      if (!safeObject(message.params) || !boundedIdentifier(message.params.targetId)) {
        markUnavailable('malformed_json');
        return;
      }
      if (message.params.targetId === scope.targetId) markUnavailable('target_lost');
      return;
    }
    if (message.method === 'Target.detachedFromTarget') {
      if (!safeObject(message.params) || !boundedIdentifier(message.params.sessionId)) {
        markUnavailable('malformed_json');
        return;
      }
      if (message.params.sessionId === scope.sessionId) markUnavailable('source_detached');
      return;
    }
    if (!NETWORK_METHODS.has(message.method)) return;
    if (message.sessionId !== scope.sessionId) { markUnavailable('scope_mismatch'); return; }
    if (!safeObject(message.params)) { markUnavailable('malformed_json'); return; }
    switch (message.method) {
      case 'Network.requestWillBeSent':
        requestWillBeSent(message.params, arrivalMs);
        return;
      case 'Network.responseReceived':
        responseReceived(message.params, arrivalMs);
        return;
      case 'Network.responseReceivedExtraInfo':
        extraInfoReceived(message.params, arrivalMs);
        return;
      case 'Network.requestServedFromCache':
        cacheServed(message.params);
        return;
      case 'Network.loadingFinished':
      case 'Network.loadingFailed':
        terminalEvent(message.method, message.params, arrivalMs);
    }
  }

  function acceptFrame() {
    if (frameLength === 0) { markUnavailable('malformed_json'); return; }
    let text;
    try { text = decoder.decode(frameBuffer.subarray(0, frameLength)); }
    catch { markUnavailable('invalid_utf8'); return; }
    let message;
    try { message = parseStrictJSON(text, limits); }
    catch { markUnavailable('malformed_json'); return; }
    frameBuffer.fill(0, 0, frameLength);
    frameLength = 0;
    if (!safeObject(message)) { markUnavailable('malformed_json'); return; }
    const arrivalMs = readClock();
    if (arrivalMs === null || !tick(arrivalMs)) return;
    projectMessage(message, arrivalMs);
  }

  function push(bytes) {
    if (finished || closed || unavailableReason) return status();
    if (!(bytes instanceof Uint8Array)) { markUnavailable('invalid_input'); return status(); }
    if (bytes.byteLength > limits.maxInputBytes) { markUnavailable('input_chunk_too_large'); return status(); }
    const nowValue = readClock();
    if (nowValue === null || !tick(nowValue)) return status();
    let frames = 0;
    for (let i = 0; i < bytes.byteLength; i++) {
      const byte = bytes[i];
      if (byte === 0) {
        if (++frames > limits.maxFramesPerPush) { markUnavailable('too_many_frames'); return status(); }
        acceptFrame();
        if (finished || unavailableReason) return status();
      } else {
        if (frameLength >= limits.maxFrameBytes) { markUnavailable('frame_too_large'); return status(); }
        frameBuffer[frameLength++] = byte;
      }
    }
    return status();
  }

  function expire() {
    if (finished || closed || unavailableReason) return status();
    tick();
    return status();
  }

  function loss() {
    if (!closed && !unavailableReason) markUnavailable('source_lost');
    return status();
  }

  function changeOwner(nextOwnerId) {
    if (!closed && !finished && nextOwnerId !== scope.ownerId) markUnavailable('owner_changed');
    return status();
  }

  function finish() {
    if (closed || finished) return status();
    tick();
    if (frameLength > 0) markUnavailable('unterminated_frame');
    if (!unavailableReason) {
      if (orphanExtraInfo.size > 0) markUnavailable('extra_info_ambiguous');
      for (const correlation of correlations.values()) {
        if (unavailableReason) break;
        if (correlation.completed) continue;
        markUnavailable(correlation.extraExpected === true && correlation.extraInfoStatus === null
          ? 'extra_info_missing' : 'incomplete_operation');
        break;
      }
    }
    finished = true;
    clearTransient({ clearOutput: false });
    return status();
  }

  function close() {
    if (!closed) {
      if (!unavailableReason) unavailableReason = 'source_closed';
      finished = true;
      closed = true;
      clearTransient({ clearOutput: true });
    }
    return status();
  }

  function drain() {
    const drained = output.map(record => Object.freeze({ ...record }));
    output = [];
    return Object.freeze(drained);
  }

  function status() {
    return Object.freeze({
      state: closed ? 'closed' : unavailableReason ? 'unavailable' : finished ? 'finished' : 'open',
      unavailable: !!unavailableReason,
      reason: unavailableReason,
      authorizing: false,
      pending: correlations.size,
      orphans: orphanExtraInfo.size,
      queued: output.length,
      bufferedFrameBytes: frameLength,
      limits: Object.freeze({ ...limits }),
    });
  }

  return Object.freeze({ push, finish, expire, loss, close, drain, status, changeOwner });
}
