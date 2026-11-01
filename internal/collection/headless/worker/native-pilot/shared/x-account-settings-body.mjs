import { TextDecoder } from 'node:util';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

export const ACCOUNT_SETTINGS_BODY_MAX_BYTES = 16_384;

const SCHEMA = 'aku.x.account-settings-shape.v1';
const JSON_LIMITS = Object.freeze({ maxJSONDepth: 8, maxJSONTokens: 1_024 });
const MAX_BASE64_CHARS = Math.ceil(ACCOUNT_SETTINGS_BODY_MAX_BYTES / 3) * 4;
const BASE64_PATTERN = /^(?:[A-Za-z0-9+/]{4})*(?:[A-Za-z0-9+/]{2}==|[A-Za-z0-9+/]{3}=)?$/u;
const ID_KEYS = Object.freeze(['id_str', 'id', 'user_id', 'user_id_str']);
const FIELD_KEYS = Object.freeze([
  'id_str', 'id', 'user_id', 'user_id_str', 'screen_name', 'authenticated',
  'error', 'errors', 'challenge',
]);
const SETTINGS_KEYS = Object.freeze([
  'language', 'time_zone', 'protected', 'always_use_https',
  'discoverable_by_email', 'discoverable_by_mobile_phone',
]);

function output(diagnostic, decodedBodyBytes = null, options = {}) {
  const fields = Object.create(null);
  for (const key of FIELD_KEYS) fields[key] = options.fields?.[key] ?? 'absent';
  Object.freeze(fields);
  return Object.freeze({
    schema: SCHEMA,
    diagnostic,
    decodedBodyBytes,
    fields,
    stableIdStringCandidates: options.stableIdStringCandidates ?? 0,
    stableIdStringsAgree: options.stableIdStringsAgree ?? null,
    authenticatedFlag: options.authenticatedFlag ?? 'absent',
    settingsFieldCount: options.settingsFieldCount ?? 0,
    canAuthorize: false,
  });
}

function validUnicode(value) {
  for (let index = 0; index < value.length; index++) {
    const unit = value.charCodeAt(index);
    if (unit >= 0xd800 && unit <= 0xdbff) {
      const next = value.charCodeAt(index + 1);
      if (!(next >= 0xdc00 && next <= 0xdfff)) return false;
      index++;
    } else if (unit >= 0xdc00 && unit <= 0xdfff) return false;
  }
  return true;
}

function exactBodyResult(value) {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const prototype = Object.getPrototypeOf(value);
  if (prototype !== Object.prototype && prototype !== null) return null;
  const keys = Reflect.ownKeys(value);
  if (keys.length !== 2 || !keys.includes('body') || !keys.includes('base64Encoded')) return null;
  const bodyDescriptor = Object.getOwnPropertyDescriptor(value, 'body');
  const base64Descriptor = Object.getOwnPropertyDescriptor(value, 'base64Encoded');
  if (!bodyDescriptor || !base64Descriptor ||
      !Object.hasOwn(bodyDescriptor, 'value') || !Object.hasOwn(base64Descriptor, 'value') ||
      !bodyDescriptor.enumerable || !base64Descriptor.enumerable ||
      typeof bodyDescriptor.value !== 'string' || typeof base64Descriptor.value !== 'boolean') return null;
  return { body: bodyDescriptor.value, base64Encoded: base64Descriptor.value };
}

function fieldType(object, key) {
  if (!Object.prototype.hasOwnProperty.call(object, key)) return 'absent';
  const value = object[key];
  if (value === null) return 'null';
  if (Array.isArray(value)) return 'array';
  if (typeof value === 'object') return 'object';
  return typeof value;
}

function projectFields(root) {
  const fields = Object.create(null);
  for (const key of FIELD_KEYS) fields[key] = fieldType(root, key);

  let stableIdStringCandidates = 0;
  let firstCandidate = null;
  let stableIdStringsAgree = null;
  for (const key of ID_KEYS) {
    if (!Object.prototype.hasOwnProperty.call(root, key)) continue;
    const value = root[key];
    if (typeof value !== 'string' || !/^[1-9][0-9]{0,19}$/u.test(value)) continue;
    stableIdStringCandidates++;
    if (firstCandidate === null) firstCandidate = value;
    else stableIdStringsAgree = stableIdStringsAgree === null
      ? value === firstCandidate
      : stableIdStringsAgree && value === firstCandidate;
  }

  let authenticatedFlag = 'absent';
  if (Object.prototype.hasOwnProperty.call(root, 'authenticated')) {
    authenticatedFlag = root.authenticated === true ? 'true'
      : root.authenticated === false ? 'false' : 'invalid';
  }

  let settingsFieldCount = 0;
  for (const key of SETTINGS_KEYS) {
    if (Object.prototype.hasOwnProperty.call(root, key)) settingsFieldCount++;
  }

  return {
    fields,
    stableIdStringCandidates,
    stableIdStringsAgree: stableIdStringCandidates > 1 ? stableIdStringsAgree : null,
    authenticatedFlag,
    settingsFieldCount,
  };
}

/**
 * Reduce one exact CDP response-body result to bounded account-settings shape
 * diagnostics. No source values are included in the returned DTO.
 */
export function projectAccountSettingsBody(result) {
  let bytes = null;
  let bodyText = null;
  let parsed = null;
  let decodedBodyBytes = null;
  try {
    const input = exactBodyResult(result);
    if (!input) return output('body_result_invalid');

    const maxChars = input.base64Encoded ? MAX_BASE64_CHARS : ACCOUNT_SETTINGS_BODY_MAX_BYTES;
    if (input.body.length > maxChars) return output('body_oversized');
    if (!validUnicode(input.body)) return output('body_encoding_invalid');

    if (input.base64Encoded) {
      if (!BASE64_PATTERN.test(input.body)) return output('body_encoding_invalid');
      const padding = input.body.endsWith('==') ? 2 : input.body.endsWith('=') ? 1 : 0;
      const expectedBytes = input.body.length / 4 * 3 - padding;
      if (expectedBytes > ACCOUNT_SETTINGS_BODY_MAX_BYTES) return output('body_oversized');
      bytes = Buffer.from(input.body, 'base64');
      if (bytes.length !== expectedBytes || bytes.toString('base64') !== input.body) {
        return output('body_encoding_invalid');
      }
    } else {
      if (Buffer.byteLength(input.body, 'utf8') > ACCOUNT_SETTINGS_BODY_MAX_BYTES) {
        return output('body_oversized');
      }
      bytes = Buffer.from(input.body, 'utf8');
    }

    if (bytes.length > ACCOUNT_SETTINGS_BODY_MAX_BYTES) return output('body_oversized');
    decodedBodyBytes = bytes.length;
    try {
      bodyText = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true }).decode(bytes);
    } catch {
      return output('body_encoding_invalid', decodedBodyBytes);
    }
    try {
      parsed = parseStrictJSON(bodyText, JSON_LIMITS);
    } catch {
      return output('body_json_invalid', decodedBodyBytes);
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return output('body_root_invalid', decodedBodyBytes);
    }

    const projection = projectFields(parsed);
    return output('projected', decodedBodyBytes, projection);
  } catch {
    return output('body_result_invalid');
  } finally {
    if (bytes) bytes.fill(0);
    bytes = null;
    bodyText = null;
    parsed = null;
  }
}
