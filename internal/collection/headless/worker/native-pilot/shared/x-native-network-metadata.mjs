import {isObject, safeId, probeError} from './x-native-value-validation.mjs';

// Allowlisted, value-free network metadata projection and bounded aggregation.
const MAX_CANDIDATES = 64;

const MAX_ENCODED_BYTES = 100_000_000;

const OBSERVATION_MS = 20_000;

const CACHE_KEYS = Object.freeze(['servedFromCache', 'fromDiskCache', 'fromServiceWorker', 'fromPrefetchCache']);

const SAFE_GRAPHQL_OPERATIONS = new Set([
  'Account', 'CurrentUser', 'HomeLatestTimeline', 'HomeTimeline', 'UserByRestId',
  'UserByScreenName', 'UsersByRestIds', 'Viewer',
]);

const SAFE_REST_OPERATIONS = new Set([
  'rest:account-settings', 'rest:verify-credentials', 'rest:account-multi-list', 'rest:other',
]);

function classifyXOperation(rawURL) {
  if (typeof rawURL !== 'string' || rawURL.length > 8_192) return null;
  let parsed;
  try { parsed = new URL(rawURL); } catch { return null; }
  if (parsed.protocol !== 'https:' || parsed.username || parsed.password ||
      !['x.com', 'api.x.com'].includes(parsed.hostname)) return null;
  let apiPath = parsed.pathname;
  if (parsed.hostname === 'x.com') {
    if (!apiPath.startsWith('/i/api/')) return null;
    apiPath = apiPath.slice('/i/api'.length);
  } else if (apiPath.startsWith('/i/api/')) {
    apiPath = apiPath.slice('/i/api'.length);
  }
  if (apiPath.includes('/graphql/')) {
    const match = apiPath.match(/(?:^|\/)graphql\/[^/]+\/([^/]+)$/u);
    if (!match || !SAFE_GRAPHQL_OPERATIONS.has(match[1])) return 'graphql:other';
    return `graphql:${match[1]}`;
  }
  if (apiPath === '/1.1/account/settings.json') return 'rest:account-settings';
  if (apiPath === '/1.1/account/verify_credentials.json') return 'rest:verify-credentials';
  if (apiPath === '/1.1/account/multi/list.json') return 'rest:account-multi-list';
  return 'rest:other';
}

function safeMethod(value) {
  if (typeof value !== 'string') return 'OTHER';
  const method = value.toUpperCase();
  return ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD'].includes(method) ? method : 'OTHER';
}

function finiteNonnegative(value, max = Number.MAX_SAFE_INTEGER) {
  return typeof value === 'number' && Number.isFinite(value) && value >= 0 && value <= max ? value : null;
}

function optionalBoolean(value) {
  return typeof value === 'boolean' ? value : null;
}

function isSafeOperationCode(value) {
  if (SAFE_REST_OPERATIONS.has(value)) return true;
  if (value === 'graphql:other') return true;
  return typeof value === 'string' && value.startsWith('graphql:') &&
    SAFE_GRAPHQL_OPERATIONS.has(value.slice('graphql:'.length));
}

// This projection is the only place raw Network event fields enter probe logic.
// Its result deliberately excludes URLs, query/hash text, headers, bodies, and
// server-provided error strings. requestKey is retained only for local joining.
function projectNetworkEvent(method, params) {
  if (!isObject(params)) return null;
  if (method === 'Network.requestWillBeSent') {
    const request = params.request;
    if (!isObject(request) || !safeId(params.requestId, 512)) return null;
    const operation = classifyXOperation(request.url);
    if (operation === null) return null;
    return Object.freeze({
      type: 'request', requestKey: params.requestId, operation,
      method: safeMethod(request.method), timestamp: finiteNonnegative(params.timestamp),
      redirected: isObject(params.redirectResponse),
    });
  }
  if (!safeId(params.requestId, 512)) return null;
  if (method === 'Network.responseReceived') {
    const response = params.response;
    if (!isObject(response)) return null;
    return Object.freeze({
      type: 'response', requestKey: params.requestId,
      status: Number.isInteger(response.status) && response.status >= 100 && response.status <= 599 ? response.status : null,
      servedFromCache: null,
      hasExtraInfo: optionalBoolean(params.hasExtraInfo),
      fromDiskCache: optionalBoolean(response.fromDiskCache),
      fromServiceWorker: optionalBoolean(response.fromServiceWorker),
      fromPrefetchCache: optionalBoolean(response.fromPrefetchCache),
      timestamp: finiteNonnegative(params.timestamp),
    });
  }
  if (method === 'Network.requestServedFromCache') {
    return Object.freeze({ type: 'cache', requestKey: params.requestId, servedFromCache: true });
  }
  if (method === 'Network.loadingFinished') {
    const bytes = finiteNonnegative(params.encodedDataLength, MAX_ENCODED_BYTES);
    return Object.freeze({
      type: 'finished', requestKey: params.requestId,
      bytes: bytes === null ? null : Math.floor(bytes), timestamp: finiteNonnegative(params.timestamp),
    });
  }
  if (method === 'Network.loadingFailed') {
    return Object.freeze({ type: 'failed', requestKey: params.requestId, timestamp: finiteNonnegative(params.timestamp) });
  }
  return null;
}

function coarseTiming(start, end) {
  if (start === null || end === null) return null;
  const elapsed = Math.round((end - start) * 1_000);
  if (!Number.isSafeInteger(elapsed) || elapsed < 0 || elapsed > OBSERVATION_MS) return null;
  return Math.round(elapsed / 250) * 250;
}

class MetadataWindow {
  constructor(maxCandidates = MAX_CANDIDATES) {
    if (!Number.isSafeInteger(maxCandidates) || maxCandidates < 1 || maxCandidates > MAX_CANDIDATES) {
      throw probeError('candidate_limit_invalid');
    }
    this.maxCandidates = maxCandidates;
    this.requests = new Map();
    this.nextAlias = 0;
    this.dropped = 0;
  }

  observe(event) {
    if (!isObject(event) || !safeId(event.requestKey, 512)) return;
    const current = this.requests.get(event.requestKey);
    if (event.type === 'request') {
      if (!isSafeOperationCode(event.operation) ||
          !['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'OPTIONS', 'HEAD', 'OTHER'].includes(event.method)) return;
      if (current) {
        current.correlationAmbiguous = true;
        current.redirected ||= !!event.redirected;
        current.status = null;
        return;
      }
      if (this.requests.size >= this.maxCandidates) {
        this.dropped = Math.min(10_000, this.dropped + 1);
        return;
      }
      const cache = Object.create(null);
      for (const key of CACHE_KEYS) cache[key] = null;
      this.requests.set(event.requestKey, {
        alias: `r${++this.nextAlias}`, operation: event.operation, method: event.method,
        start: event.timestamp, response: null, finish: null, failed: false,
        redirected: !!event.redirected, correlationAmbiguous: false,
        hasExtraInfo: null, status: null, cache, bytes: null,
      });
      return;
    }
    if (!current) return;
    if (event.type === 'response') {
      current.hasExtraInfo = event.hasExtraInfo;
      current.status = current.redirected || current.correlationAmbiguous ? null : event.status;
      current.response = event.timestamp;
      for (const key of CACHE_KEYS) {
        if (event[key] !== null) current.cache[key] = event[key];
      }
    } else if (event.type === 'cache') {
      current.cache.servedFromCache = true;
    } else if (event.type === 'finished') {
      current.finish = event.timestamp;
      current.bytes = event.bytes;
    } else if (event.type === 'failed') {
      current.failed = true;
      current.finish = event.timestamp;
    }
  }

  finish() {
    const candidates = [];
    for (const item of this.requests.values()) {
      candidates.push({
        alias: item.alias,
        operation: item.operation,
        method: item.method,
        status: item.status,
        ambiguous: item.redirected || item.correlationAmbiguous,
        hasExtraInfo: item.hasExtraInfo,
        cache: { ...item.cache },
        completion: item.failed ? 'failed' : item.finish === null ? 'incomplete' : 'complete',
        bytes: item.bytes,
        timingMs: {
          response: coarseTiming(item.start, item.response),
          completion: coarseTiming(item.start, item.finish),
        },
      });
    }
    this.requests.clear();
    return { candidates, dropped: this.dropped };
  }
}

export {MAX_CANDIDATES, MAX_ENCODED_BYTES, OBSERVATION_MS, CACHE_KEYS, classifyXOperation, isSafeOperationCode, projectNetworkEvent, MetadataWindow};
