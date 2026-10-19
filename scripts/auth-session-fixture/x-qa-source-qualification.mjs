// Pure projection/correlation for the retained QA diagnostic. No launch or transfer.
import { AccountSettingsCapture } from './x-account-settings-capture.mjs';
import { projectAccountSettingsBody } from './x-account-settings-body.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

const ROUTES = Object.freeze({ web_i_api: 'https://x.com/i/api/1.1/account/settings.json',
  api_v1: 'https://api.x.com/1.1/account/settings.json', api_i_api: 'https://api.x.com/i/api/1.1/account/settings.json' });
const names = ['auth_token', 'ct0'];
const fail = () => { throw new TypeError('qa_source_input_invalid'); };
function data(value, allowed, required = allowed) {
  if (!value || typeof value !== 'object' || Array.isArray(value) ||
      ![null, Object.prototype].includes(Object.getPrototypeOf(value))) fail();
  const ds = Object.getOwnPropertyDescriptors(value), keys = Reflect.ownKeys(ds);
  if (keys.some(key => !allowed.includes(key) || !Object.hasOwn(ds[key], 'value')) || required.some(key => !ds[key])) fail();
  return Object.fromEntries(keys.map(key => [key, ds[key].value]));
}
export function qaSourceRoute(value) {
  try {
    if (typeof value !== 'string' || value.length > 8192) return null;
    const url = new URL(value);
    if (url.username || url.password || url.hash) return null;
    return Object.keys(ROUTES).find(key => ROUTES[key] === url.origin + url.pathname) ?? null;
  } catch { return null; }
}
export function qaCookieRequest(route) {
  if (!Object.hasOwn(ROUTES, route)) fail();
  return { urls: ['https://x.com/home', ROUTES[route]] };
}

// Post-observation verification is a separate fixed read, not initial login
// polling (which must remain closed after beginObservation).
export async function finishQASourceQualification(source, { readHome, readCookies }) {
  try {
    const sample = data(source, ['schema', 'route', 'diagnostic', 'matchingRequests', 'bodyReadAttempts',
      'bodyContractMatches', 'authentication', 'negativeControl', 'canAuthorize']);
    if (sample.schema !== 'aku.x.qa-source-capture.v1' || sample.authentication !== 'unknown' ||
        sample.negativeControl !== 'not_run' || sample.canAuthorize !== false ||
        typeof readHome !== 'function' || typeof readCookies !== 'function') fail();
    const home = await readHome();
    if (typeof home !== 'boolean') fail();
    let cookies = null;
    if (home && sample.diagnostic === 'projected' && Object.hasOwn(ROUTES, sample.route)) {
      cookies = await readCookies(sample.route);
      if (cookies?.schema !== 'aku.x.qa-cookie-metadata.v1' || cookies.canAuthorize !== false ||
          cookies.sessionTransferred !== false || cookies.valuesPersisted !== false) fail();
    }
    return Object.freeze({ schema: 'aku.x.qa-source-qualification.v1', source, cookies,
      canAuthorize: false, sessionTransferred: false });
  } catch { throw new TypeError('qa_source_finalization_failed'); }
}

export function projectQASourceBody(value) {
  const shape = projectAccountSettingsBody(value);
  let contractMatches = null;
  if (shape.diagnostic === 'projected') {
    try {
      const input = data(value, ['body', 'base64Encoded']);
      const text = input.base64Encoded ? Buffer.from(input.body, 'base64').toString('utf8') : input.body;
      const body = parseStrictJSON(text, { maxJSONDepth: 8, maxJSONTokens: 1024 });
      contractMatches = ['language', 'time_zone', 'protected', 'always_use_https',
        'discoverable_by_email', 'discoverable_by_mobile_phone'].every(key => Object.hasOwn(body, key)) &&
        typeof body.language === 'string' && (body.time_zone === null || typeof body.time_zone === 'string') &&
        ['protected', 'always_use_https', 'discoverable_by_email', 'discoverable_by_mobile_phone'].every(key => typeof body[key] === 'boolean') &&
        !['error', 'errors', 'challenge'].some(key => Object.hasOwn(body, key)) &&
        (!Object.hasOwn(body, 'authenticated') || body.authenticated === true);
    } catch { contractMatches = false; }
  }
  return Object.freeze({ ...shape, contractMatches });
}

export class QASourceCapture {
  constructor({ readBody, frameId, now }) {
    if (typeof readBody !== 'function' || typeof frameId !== 'string' || !frameId || frameId.length > 256) fail();
    this.frameId = frameId;
    this.route = null;
    this.invalid = false;
    this.closed = false;
    this.base = new AccountSettingsCapture({ readBody, ...(now ? { now } : {}) });
  }
  consumeRead(id) { return !this.invalid && this.base.consumeRead(id); }
  invalidate() { this.invalid = true; this.base.invalidate(); }
  observe(method, params) {
    if (this.closed) return;
    if (method === 'Network.requestWillBeSent') {
      if (this.base.chosen?.id === params?.requestId) { this.base.observe(method, params); return; }
      const route = qaSourceRoute(params?.request?.url);
      if (!route || params.request.method !== 'GET') return;
      if (params.frameId !== this.frameId) { this.invalidate(); return; }
      if (this.route && this.route !== route) this.invalidate();
      this.route ??= route;
    }
    if (method === 'Network.responseReceived' && this.base.chosen?.id === params?.requestId &&
        qaSourceRoute(params.response?.url) !== this.route) this.invalidate();
    this.base.observe(method, params);
  }
  async finish() {
    this.closed = true;
    const capture = await this.base.finish();
    return Object.freeze({ schema: 'aku.x.qa-source-capture.v1', route: this.route,
      diagnostic: this.invalid ? 'candidate_rejected' : capture.diagnostic,
      matchingRequests: capture.matchingRequests, bodyReadAttempts: capture.bodyReadAttempts,
      bodyContractMatches: !this.invalid && capture.diagnostic === 'projected' ? capture.projection.contractMatches : null,
      authentication: 'unknown', negativeControl: 'not_run', canAuthorize: false });
  }
}

function metadata(raw, now) {
  const allowed = ['name', 'value', 'domain', 'path', 'expires', 'size', 'httpOnly', 'secure', 'session',
    'sameSite', 'priority', 'sameParty', 'sourceScheme', 'sourcePort', 'partitionKey', 'partitionKeyOpaque'];
  const c = data(raw, allowed, ['name', 'value', 'domain', 'path', 'expires', 'httpOnly', 'secure', 'session']);
  if (typeof c.value !== 'string' || !c.value || c.value.length > 8192) fail();
  if (c.domain !== '.x.com' || c.path !== '/') return { status: 'scope_mismatch', attributes: null };
  if (c.partitionKey !== undefined || c.partitionKeyOpaque === true) return { status: 'unsupported', attributes: null };
  if (c.secure !== true || typeof c.httpOnly !== 'boolean' || typeof c.session !== 'boolean' ||
      !['Strict', 'Lax', 'None', undefined].includes(c.sameSite) ||
      ![undefined, 'Low', 'Medium', 'High'].includes(c.priority) ||
      ![undefined, 'Unset', 'NonSecure', 'Secure'].includes(c.sourceScheme) ||
      c.sourcePort !== undefined && (!Number.isInteger(c.sourcePort) || c.sourcePort < -1 || c.sourcePort > 65535) ||
      c.sameParty !== undefined && typeof c.sameParty !== 'boolean' || c.sameParty === true ||
      !Number.isFinite(c.expires) || c.session && c.expires !== -1 ||
      !c.session && (c.expires <= 0 || c.expires > Number.MAX_SAFE_INTEGER)) fail();
  if (!c.session && c.expires <= now) return { status: 'expired', attributes: null };
  const attributes = Object.freeze({ domain: c.domain, path: c.path, hostOnly: false, secure: true,
    httpOnly: c.httpOnly, sameSite: c.sameSite ?? 'Unspecified', session: c.session, expires: c.expires,
    partitionKey: null, priority: c.priority ?? null, sameParty: c.sameParty ?? null,
    sourceScheme: c.sourceScheme ?? null, sourcePort: c.sourcePort ?? null });
  // Check the actual transfer-relevant serialized tuple privately; retain no value.
  if (Buffer.byteLength(JSON.stringify({ name: c.name, ...attributes, value: c.value }), 'utf8') > 8192) fail();
  return { status: 'candidate', attributes };
}

export function projectQACookieMetadata(result, now = Date.now() / 1000) {
  let tuples;
  try {
    const input = data(result, ['cookies']);
    if (!Number.isFinite(now) || now < 0 || !Array.isArray(input.cookies) || input.cookies.length > 64) fail();
    const descriptors = Object.getOwnPropertyDescriptors(input.cookies);
    if (Reflect.ownKeys(descriptors).length !== input.cookies.length + 1) fail();
    const selected = new Map(names.map(name => [name, []]));
    for (let index = 0; index < input.cookies.length; index++) {
      const descriptor = descriptors[index];
      if (!descriptor || !Object.hasOwn(descriptor, 'value')) fail();
      const raw = descriptor.value;
      if (!raw || typeof raw !== 'object') fail();
      const name = Object.getOwnPropertyDescriptor(raw, 'name');
      if (!name || !Object.hasOwn(name, 'value')) fail();
      if (selected.has(name.value)) selected.get(name.value).push(raw);
      // All nonallowlisted values are discarded without reading them here.
    }
    tuples = names.map(name => {
      const matches = selected.get(name);
      let item = { status: matches.length ? 'ambiguous' : 'missing', attributes: null };
      if (matches.length === 1) {
        try { item = metadata(matches[0], now); } catch { item = { status: 'unsupported', attributes: null }; }
      }
      return Object.freeze({ name, ...item });
    });
  } catch {
    tuples = names.map(name => Object.freeze({ name, status: 'unsupported', attributes: null }));
  }
  return Object.freeze({ schema: 'aku.x.qa-cookie-metadata.v1', tuples: Object.freeze(tuples),
    valuesPersisted: false, sessionTransferred: false, canAuthorize: false });
}
