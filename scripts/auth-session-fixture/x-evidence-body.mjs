import { TextDecoder } from 'node:util';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

const MAX_BODY_BYTES = 4_096;
const BODY_SCHEMA = 'aku.x.browser-self.fixture.v1';
const BODY_LIMITS = Object.freeze({ maxJSONDepth: 8, maxJSONTokens: 128 });

function fail(code) {
  throw new TypeError(code);
}

function object(value) {
  return !!value && typeof value === 'object' && !Array.isArray(value);
}

function hasOnlyKeys(value, allowed) {
  return Object.keys(value).every(key => allowed.has(key));
}

function validBoundedText(value, maxBytes) {
  if (typeof value !== 'string' || value.length === 0 || value.length > maxBytes ||
      /[\u0000-\u001f\u007f]/u.test(value)) return false;
  for (let index = 0; index < value.length; index++) {
    const unit = value.charCodeAt(index);
    if (unit >= 0xd800 && unit <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      index++;
    } else if (unit >= 0xdc00 && unit <= 0xdfff) return false;
  }
  return Buffer.byteLength(value, 'utf8') <= maxBytes;
}

function validUTF8SourceText(value, maxBytes) {
  if (typeof value !== 'string') return false;
  for (let index = 0; index < value.length; index++) {
    const unit = value.charCodeAt(index);
    if (unit >= 0xd800 && unit <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      index++;
    } else if (unit >= 0xdc00 && unit <= 0xdfff) return false;
  }
  return Buffer.byteLength(value, 'utf8') <= maxBytes;
}

function decodeBodyBytes(result) {
  if (!object(result) || !hasOnlyKeys(result, new Set(['body', 'base64Encoded'])) ||
      typeof result.body !== 'string' || typeof result.base64Encoded !== 'boolean') {
    fail('body_invalid');
  }
  if (result.base64Encoded) {
    const encoded = result.body;
    const maxBase64Bytes = Math.ceil(MAX_BODY_BYTES / 3) * 4;
    if (encoded.length > maxBase64Bytes || encoded.length % 4 !== 0 ||
        !/^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/u.test(encoded)) {
      fail(encoded.length > maxBase64Bytes ? 'body_oversized' : 'body_base64_invalid');
    }
    const bytes = Buffer.from(encoded, 'base64');
    if (bytes.length > MAX_BODY_BYTES) fail('body_oversized');
    if (bytes.toString('base64') !== encoded) fail('body_base64_invalid');
    return bytes;
  }
  if (!validUTF8SourceText(result.body, MAX_BODY_BYTES)) {
    fail(Buffer.byteLength(result.body, 'utf8') > MAX_BODY_BYTES ? 'body_oversized' : 'body_encoding_invalid');
  }
  const bytes = Buffer.from(result.body, 'utf8');
  if (bytes.length > MAX_BODY_BYTES) fail('body_oversized');
  return bytes;
}

function parseBody(bytes) {
  let text;
  try { text = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes); }
  catch { fail('body_encoding_invalid'); }
  try { return parseStrictJSON(text, BODY_LIMITS); }
  catch { fail('body_invalid'); }
}

/**
 * Parse and reduce a qualified synthetic CDP response body. The returned body
 * contains only the fixed schema, state, and an optional validated id_str.
 */
export function canonicalizeFixtureXBody(result) {
  const bytes = decodeBodyBytes(result);
  const parsed = parseBody(bytes);
  if (!object(parsed) || !hasOnlyKeys(parsed, new Set(['schema', 'state', 'data'])) ||
      parsed.schema !== BODY_SCHEMA || typeof parsed.state !== 'string') {
    fail('body_schema_invalid');
  }

  if (!['authenticated', 'logged_out', 'challenge', 'unknown'].includes(parsed.state)) {
    fail('body_state_invalid');
  }
  if (parsed.state !== 'authenticated' && parsed.data !== undefined) fail('body_state_conflict');

  let identity = null;
  if (parsed.data !== undefined) {
    if (!object(parsed.data) || !hasOnlyKeys(parsed.data, new Set(['id_str', 'username', 'name']))) {
      fail('body_unknown_key');
    }
    if (parsed.data.id_str === undefined) fail('body_identity_missing');
    if (typeof parsed.data.id_str !== 'string' || !/^[1-9][0-9]{0,19}$/u.test(parsed.data.id_str)) {
      fail('body_identity_invalid');
    }
    identity = parsed.data.id_str;
    for (const field of ['username', 'name']) {
      if (parsed.data[field] !== undefined && !validBoundedText(parsed.data[field], 128)) {
        fail('body_property_invalid');
      }
    }
  }

  const canonical = {
    schema: BODY_SCHEMA,
    state: parsed.state,
    ...(identity === null ? {} : { data: { id_str: identity } }),
  };
  const body = JSON.stringify(canonical);
  return Object.freeze({ body, state: parsed.state, identityPresent: identity !== null,
    bodyBytes: Buffer.byteLength(body, 'utf8'), decodedBodyBytes: bytes.length });
}

/**
 * Read a body only through an ingress ticket. The callback must project the CDP
 * response at dispatch time and resolve only the canonical bounded body result.
 */
export async function readQualifiedFixtureXBody({ ingress, ticket, getResponseBody } = {}) {
  if (!ingress || typeof ingress.withBodyRequest !== 'function' || !ticket ||
      typeof getResponseBody !== 'function') throw new TypeError('body_ticket_invalid');
  const result = await ingress.withBodyRequest(ticket, requestId =>
    getResponseBody(requestId, canonicalizeFixtureXBody));
  if (!result || typeof result !== 'object' || typeof result.body !== 'string' ||
      typeof result.state !== 'string' || typeof result.identityPresent !== 'boolean' ||
      !Number.isSafeInteger(result.bodyBytes) || result.bodyBytes < 0 || result.bodyBytes > MAX_BODY_BYTES ||
      !Number.isSafeInteger(result.decodedBodyBytes) || result.decodedBodyBytes < 0 ||
      result.decodedBodyBytes > MAX_BODY_BYTES || Buffer.byteLength(result.body, 'utf8') !== result.bodyBytes) {
    throw new TypeError('body_projection_invalid');
  }
  return Object.freeze({ body: result.body, state: result.state, identityPresent: result.identityPresent,
    bodyBytes: result.bodyBytes, decodedBodyBytes: result.decodedBodyBytes });
}

export const FIXTURE_X_BODY_SCHEMA = BODY_SCHEMA;
export const FIXTURE_X_BODY_MAX_BYTES = MAX_BODY_BYTES;
