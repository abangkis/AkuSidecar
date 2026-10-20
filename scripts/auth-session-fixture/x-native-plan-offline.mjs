// Pure offline contract exercise. Trusted synthetic assertions are not native
// evidence. This module has no browser, network, filesystem, Store or transfer API.
import { inspectBootstrapCurrentSelf } from './x-bootstrap-current-self.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

export const OFFLINE_X_PLAN_SCHEMA = 'aku.x.native-plan.offline.v1';
export const OFFLINE_X_INPUT_SCHEMA = 'aku.x.native-evidence.offline.v1';
const PROVENANCE = 'synthetic-fixture';
const BINDING = ['ownerId', 'profileId', 'contextId', 'targetId', 'frameId', 'generation'];
const ATTRIBUTES = ['name', 'domain', 'path', 'hostOnly', 'secure', 'httpOnly', 'sameSite', 'session', 'expires', 'partitionKey'];
const SOURCES = new Set([
  'https://x.com/i/api/1.1/account/settings.json',
  'https://api.x.com/1.1/account/settings.json',
  'https://api.x.com/i/api/1.1/account/settings.json',
]);
const USED_ATTEMPTS = new Set();
const SETTINGS = { language: 'string', time_zone: 'nullable-string-object', protected: 'boolean',
  always_use_https: 'boolean', discoverable_by_email: 'boolean', discoverable_by_mobile_phone: 'boolean' };

const invalid = () => { throw new TypeError('offline_x_input_invalid'); };
const id = value => typeof value === 'string' && /^[1-9][0-9]{0,19}$/.test(value);
const key = value => typeof value === 'string' && /^[A-Za-z0-9._:-]{1,128}$/.test(value);
const time = value => Number.isFinite(value) && value >= 0 && value <= Number.MAX_SAFE_INTEGER;
const same = (a, b, fields) => fields.every(field => a[field] === b[field]);

// Reject accessors and non-data containers before reading any selected property.
function record(value, fields) {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      ![Object.prototype, null].includes(Object.getPrototypeOf(value))) invalid();
  const descriptors = Object.getOwnPropertyDescriptors(value);
  const keys = Reflect.ownKeys(descriptors);
  if (keys.length !== fields.length || keys.some(name => !fields.includes(name))) invalid();
  const copy = Object.create(null);
  for (const field of fields) {
    const descriptor = descriptors[field];
    if (!descriptor || !Object.hasOwn(descriptor, 'value') || !descriptor.enumerable) invalid();
    copy[field] = descriptor.value;
  }
  return copy;
}

function pair(value) {
  if (!Array.isArray(value) || value.length !== 2) invalid();
  const descriptors = Object.getOwnPropertyDescriptors(value);
  if (Reflect.ownKeys(descriptors).length !== 3) invalid();
  return [0, 1].map(index => {
    const descriptor = descriptors[index];
    if (!descriptor || !Object.hasOwn(descriptor, 'value')) invalid();
    return descriptor.value;
  });
}

function binding(value) {
  const result = record(value, BINDING);
  if (!BINDING.every(field => key(result[field]))) invalid();
  return Object.freeze(result);
}

function source(value) {
  const result = record(value, ['method', 'origin', 'path']);
  if (result.method !== 'GET' || !['https://x.com', 'https://api.x.com'].includes(result.origin) ||
      typeof result.path !== 'string' || !SOURCES.has(result.origin + result.path)) invalid();
  return Object.freeze(result);
}

function cookie(value, withValue = false) {
  const result = record(value, withValue ? [...ATTRIBUTES, 'value'] : ATTRIBUTES);
  if (!['auth_token', 'ct0'].includes(result.name) || result.domain !== '.x.com' || result.path !== '/' ||
      result.hostOnly !== false || result.secure !== true || typeof result.httpOnly !== 'boolean' ||
      !['Strict', 'Lax', 'None', 'Unspecified'].includes(result.sameSite) || typeof result.session !== 'boolean' ||
      result.partitionKey !== null || (result.session ? ![null, -1].includes(result.expires) :
        !time(result.expires) || result.expires === 0)) invalid();
  // A deliberate test marker prevents accidental use as a native cookie adapter.
  // It is not a security boundary against a caller inventing synthetic claims.
  if (withValue && (typeof result.value !== 'string' || result.value.length > 8192 ||
      !/^synthetic:[A-Za-z0-9_-]+$/.test(result.value) || Buffer.byteLength(JSON.stringify(result), 'utf8') > 8192)) invalid();
  return Object.freeze(result);
}

function normalizePlan(input) {
  const plan = record(input, ['schema', 'provenance', 'attemptId', 'expectedAccountId',
    'reader', 'collector', 'source', 'cookies', 'qualification']);
  if (plan.schema !== OFFLINE_X_PLAN_SCHEMA || plan.provenance !== PROVENANCE ||
      !key(plan.attemptId) || !id(plan.expectedAccountId)) invalid();
  plan.reader = binding(plan.reader);
  plan.collector = binding(plan.collector);
  if (plan.reader.ownerId === plan.collector.ownerId || plan.reader.profileId === plan.collector.profileId) invalid();
  plan.source = source(plan.source);
  plan.cookies = Object.freeze(pair(plan.cookies).map(value => cookie(value)));
  if (new Set(plan.cookies.map(value => value.name)).size !== 2) invalid();
  const q = record(plan.qualification, ['version', 'source', 'signedIn', 'signedOut']);
  q.source = source(q.source);
  if (!same(plan.source, q.source, ['method', 'origin', 'path']) ||
      !['unqualified', 'account-settings-protected.fixture.v2'].includes(q.version) ||
      !['unknown', 'settings_success'].includes(q.signedIn) ||
      !['unknown', 'authentication_required', 'no_request', 'network_error', 'rate_limited', 'csrf_error', 'challenge'].includes(q.signedOut)) invalid();
  plan.qualification = Object.freeze(q);
  return Object.freeze(plan);
}

function report(reason, fields = {}) {
  return Object.freeze({ schema: 'aku.x.native-plan.offline-report.v1', provenance: PROVENANCE,
    contract: 'invalid', transfer: 'not_validated', identity: 'unknown', authentication: 'unknown',
    continuity: 'unknown', refresh: 'refresh_not_observed', outcome: 'unqualified', reason, ...fields,
    canAuthorize: false, nativeQualified: false, realAuthTested: false });
}

export function validateXOfflineContract(input) {
  try {
    normalizePlan(input);
    return report('contract_valid', { contract: 'valid' });
  } catch { return report('contract_invalid'); }
}

function fresh(value, now) { return time(value) && value <= now && now - value <= 5000; }

function inspectRole(value, expected, plan, now) {
  const role = record(value, ['self', 'login', 'coverage']);
  const self = record(role.self, ['binding', 'observedAtMs', 'page']);
  const login = record(role.login, ['binding', 'requestId', 'startedAtMs', 'completedAtMs', 'source',
    'status', 'mimeType', 'fromDiskCache', 'fromServiceWorker', 'fromPrefetchCache', 'servedFromCache',
    'redirected', 'ambiguous', 'complete', 'body']);
  const coverage = record(role.coverage, ['binding', 'fromMs', 'throughMs', 'mutation', 'accountChange',
    'challenge', 'gap', 'detached', 'overflow']);
  if (![self, login, coverage].every(item => same(binding(item.binding), expected, BINDING)) ||
      !same(source(login.source), plan.source, ['method', 'origin', 'path']) || !key(login.requestId)) invalid();
  if (!fresh(self.observedAtMs, now) || !fresh(login.startedAtMs, now) ||
      !fresh(login.completedAtMs, now) || login.startedAtMs > login.completedAtMs) return { reason: 'evidence_stale' };
  if (!time(coverage.fromMs) || coverage.fromMs > Math.min(self.observedAtMs, login.startedAtMs) ||
      coverage.throughMs !== now || coverage.gap !== 'none' ||
      ['mutation', 'accountChange', 'challenge', 'detached', 'overflow'].some(field => coverage[field] !== false)) return { reason: 'coverage_unqualified' };

  const projection = inspectBootstrapCurrentSelf(self.page);
  const entityStates = Object.values(projection.entityChecks);
  if (projection.diagnostic !== 'projected' || projection.idFields.user_id !== 'string' ||
      !['absent', 'string'].includes(projection.idFields.user_id_str) || projection.sessionIdsAgree === false ||
      projection.entityChecks.normalizedUsers !== 'match' ||
      entityStates.some(state => !['match', 'absent'].includes(state))) return { reason: 'identity_unavailable' };
  // Extract only the validated own ID, never scan authors or include it in reports.
  const state = Object.getOwnPropertyDescriptor(self.page, '__INITIAL_STATE__').value;
  const session = Object.getOwnPropertyDescriptor(state, 'session').value;
  const accountId = Object.getOwnPropertyDescriptor(session, 'user_id').value;
  if (!id(accountId) || accountId !== plan.expectedAccountId) return { reason: 'identity_mismatch' };
  const authFields = ['loggedIn', 'isAuthenticated', 'authenticated'];
  if (authFields.some(field => !['absent', 'true'].includes(projection.authFields[field]))) return { reason: 'login_conflict', identity: 'match' };
  if (login.status !== 200 || !['application/json', 'application/json;charset=utf-8', 'application/json; charset=utf-8'].includes(login.mimeType) ||
      ['fromDiskCache', 'fromServiceWorker', 'fromPrefetchCache', 'servedFromCache', 'redirected', 'ambiguous'].some(field => login[field] !== false) ||
      login.complete !== true) return { reason: 'response_unqualified', identity: 'match' };
  if (typeof login.body !== 'string' || login.body.length > 16384 || Buffer.byteLength(login.body, 'utf8') > 16384 ||
      Buffer.from(login.body, 'utf8').toString('utf8') !== login.body) return { reason: 'body_invalid', identity: 'match' };
  let body;
  try { body = parseStrictJSON(login.body, { maxJSONDepth: 8, maxJSONTokens: 1024 }); }
  catch { return { reason: 'body_invalid', identity: 'match' }; }
  if (!body || typeof body !== 'object' || Array.isArray(body) ||
      ['error', 'errors', 'challenge'].some(field => Object.hasOwn(body, field)) ||
      Object.hasOwn(body, 'authenticated') && body.authenticated !== true ||
      Object.entries(SETTINGS).some(([field, type]) => !Object.hasOwn(body, field) ||
        (type === 'nullable-string-object' ? body[field] !== null && typeof body[field] !== 'string' && (typeof body[field] !== 'object' || Array.isArray(body[field])) : typeof body[field] !== type))) return { reason: 'body_invalid', identity: 'match' };
  for (const field of ['id_str', 'id', 'user_id', 'user_id_str']) {
    if (Object.hasOwn(body, field) && (!id(body[field]) || body[field] !== accountId)) return { reason: 'identity_mismatch' };
  }
  const q = plan.qualification;
  if (q.version !== 'account-settings-protected.fixture.v2' || q.signedIn !== 'settings_success' ||
      q.signedOut !== 'authentication_required') return { reason: 'login_source_unqualified', identity: 'match' };
  return { reason: 'synthetic_role_qualified', identity: 'match' };
}

export function createXOfflineEvidenceCombiner(input) {
  let plan;
  try { plan = normalizePlan(input); }
  catch { throw new TypeError('offline_x_contract_invalid'); }
  if (USED_ATTEMPTS.has(plan.attemptId) || USED_ATTEMPTS.size >= 1024) throw new TypeError('offline_x_attempt_unavailable');
  USED_ATTEMPTS.add(plan.attemptId);
  let consumed = false;
  return Object.freeze({
    evaluate(value) {
      if (consumed) return report('attempt_consumed');
      consumed = true;
      let transfer = 'not_validated';
      try {
        const evidence = record(value, ['schema', 'provenance', 'attemptId', 'nowMs', 'unixSeconds', 'cookies', 'reader', 'collector']);
        if (evidence.schema !== OFFLINE_X_INPUT_SCHEMA || evidence.provenance !== PROVENANCE ||
            evidence.attemptId !== plan.attemptId || !time(evidence.nowMs) || !time(evidence.unixSeconds)) invalid();
        const cookies = pair(evidence.cookies).map(value => cookie(value, true));
        if (new Set(cookies.map(value => value.name)).size !== 2 ||
            cookies.some(item => !same(item, plan.cookies.find(expected => expected.name === item.name), ATTRIBUTES) ||
              !item.session && item.expires <= evidence.unixSeconds)) invalid();
        if (cookies.reduce((bytes, item) => bytes + Buffer.byteLength(JSON.stringify(item), 'utf8'), 0) > 16384) invalid();
        transfer = 'synthetic_validated';
        const reader = inspectRole(evidence.reader, plan.reader, plan, evidence.nowMs);
        const collector = inspectRole(evidence.collector, plan.collector, plan, evidence.nowMs);
        const failed = [reader, collector].find(role => role.reason !== 'synthetic_role_qualified');
        if (failed) return report(failed.reason, { contract: 'valid', transfer,
          identity: failed.reason === 'identity_mismatch' ? 'mismatch' : reader.identity === 'match' && collector.identity === 'match' ? 'match' : 'unknown' });
        return report('synthetic_evidence_qualified', { contract: 'valid', transfer, identity: 'match',
          authentication: 'authenticated', continuity: 'synthetic_covered', outcome: 'synthetic_ready' });
      } catch { return report('evidence_invalid', { contract: 'valid', transfer }); }
      finally { plan = null; }
    },
    close() { consumed = true; plan = null; },
  });
}
