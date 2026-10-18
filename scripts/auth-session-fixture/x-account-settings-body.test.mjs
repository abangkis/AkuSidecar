import test from 'node:test';
import assert from 'node:assert/strict';
import {
  projectAccountSettingsBody,
  ACCOUNT_SETTINGS_BODY_MAX_BYTES,
} from './x-account-settings-body.mjs';

const FIELD_NAMES = [
  'id_str', 'id', 'user_id', 'user_id_str', 'screen_name', 'authenticated',
  'error', 'errors', 'challenge',
];

function response(text, base64Encoded = false) {
  return {
    body: base64Encoded ? Buffer.from(text, 'utf8').toString('base64') : text,
    base64Encoded,
  };
}

function assertFailure(result, diagnostic, expectedBytes = null) {
  assert.equal(result.diagnostic, diagnostic);
  assert.equal(result.decodedBodyBytes, expectedBytes);
  assert.deepEqual({ ...result.fields }, Object.fromEntries(FIELD_NAMES.map(key => [key, 'absent'])));
  assert.equal(result.stableIdStringCandidates, 0);
  assert.equal(result.stableIdStringsAgree, null);
  assert.equal(result.authenticatedFlag, 'absent');
  assert.equal(result.settingsFieldCount, 0);
  assert.equal(result.canAuthorize, false);
  assert.ok(Buffer.byteLength(JSON.stringify(result), 'utf8') < 512);
}

test('projects only fixed top-level shape and never returns private or unknown values', () => {
  const sourceValues = [
    '98765432101234567890', 'private_handle_7f19', 'private@example.invalid',
    'secret-access-token-91aa', 'server-error-private-2d50', 'challenge-private-3ba1',
    'language-private-0c91', 'zone-private-6fd0',
  ];
  const raw = JSON.stringify({
    id_str: sourceValues[0],
    id: sourceValues[0],
    user_id: sourceValues[0],
    user_id_str: sourceValues[0],
    screen_name: sourceValues[1],
    authenticated: true,
    error: sourceValues[4],
    errors: [{ message: sourceValues[4] }],
    challenge: { token: sourceValues[5] },
    language: sourceValues[6],
    time_zone: sourceValues[7],
    protected: false,
    always_use_https: true,
    discoverable_by_email: false,
    discoverable_by_mobile_phone: true,
    email: sourceValues[2],
    access_token: sourceValues[3],
    nested: { id_str: '111', author_id_str: '222' },
  });

  for (const encoded of [false, true]) {
    const result = projectAccountSettingsBody(response(raw, encoded));
    assert.equal(result.schema, 'aku.x.account-settings-shape.v1');
    assert.deepEqual(Object.keys(result), [
      'schema', 'diagnostic', 'decodedBodyBytes', 'fields', 'stableIdStringCandidates',
      'stableIdStringsAgree', 'authenticatedFlag', 'settingsFieldCount', 'canAuthorize',
    ]);
    assert.deepEqual(Object.keys(result.fields), FIELD_NAMES);
    assert.equal(result.diagnostic, 'projected');
    assert.equal(result.decodedBodyBytes, Buffer.byteLength(raw, 'utf8'));
    assert.deepEqual({ ...result.fields }, {
      id_str: 'string', id: 'string', user_id: 'string', user_id_str: 'string',
      screen_name: 'string', authenticated: 'boolean', error: 'string',
      errors: 'array', challenge: 'object',
    });
    assert.equal(result.stableIdStringCandidates, 4);
    assert.equal(result.stableIdStringsAgree, true);
    assert.equal(result.authenticatedFlag, 'true');
    assert.equal(result.settingsFieldCount, 6);
    assert.equal(result.canAuthorize, false);
    const serialized = JSON.stringify(result);
    for (const value of sourceValues) assert.equal(serialized.includes(value), false);
    assert.equal(serialized.includes('access_token'), false);
    assert.equal(serialized.includes('author_id_str'), false);
    assert.ok(Buffer.byteLength(serialized, 'utf8') < 512);
  }
});

test('counts only qualified top-level ID strings and compares candidates without returning them', () => {
  const equal = projectAccountSettingsBody(response(JSON.stringify({
    id_str: '123', user_id_str: '123', id: '0123', user_id: 123,
    profile: { id_str: '456', author_id_str: '789' },
  })));
  assert.equal(equal.stableIdStringCandidates, 2);
  assert.equal(equal.stableIdStringsAgree, true);
  assert.equal(equal.fields.id, 'string');
  assert.equal(equal.fields.user_id, 'number');
  assert.equal(JSON.stringify(equal).includes('123'), false);

  const conflicting = projectAccountSettingsBody(response(JSON.stringify({
    id_str: '123', user_id: '124', user_id_str: '123',
  })));
  assert.equal(conflicting.stableIdStringCandidates, 3);
  assert.equal(conflicting.stableIdStringsAgree, false);

  const onlyHandleAndNestedId = projectAccountSettingsBody(response(JSON.stringify({
    screen_name: 'handle-private', profile: { id_str: '123' }, author_id_str: '456',
  })));
  assert.equal(onlyHandleAndNestedId.stableIdStringCandidates, 0);
  assert.equal(onlyHandleAndNestedId.stableIdStringsAgree, null);
  assert.equal(onlyHandleAndNestedId.fields.id_str, 'absent');
  assert.equal(JSON.stringify(onlyHandleAndNestedId).includes('handle-private'), false);

  const absentId = projectAccountSettingsBody(response('{}'));
  const badId = projectAccountSettingsBody(response('{"id_str":"0123"}'));
  assert.equal(absentId.fields.id_str, 'absent');
  assert.equal(badId.fields.id_str, 'string');
  assert.equal(absentId.stableIdStringCandidates, 0);
  assert.equal(badId.stableIdStringCandidates, 0);
  assert.equal(badId.stableIdStringsAgree, null);

  const numericId = projectAccountSettingsBody(response('{"id":123}'));
  assert.equal(numericId.fields.id, 'number');
  assert.equal(numericId.stableIdStringCandidates, 0);
  assert.equal(numericId.stableIdStringsAgree, null);
  const unsafeNumericId = '{"id":9007199254740992}';
  assertFailure(projectAccountSettingsBody(response(unsafeNumericId)), 'body_json_invalid',
    Buffer.byteLength(unsafeNumericId, 'utf8'));
});

test('reports authentication and error/challenge shape only and never grants authority', () => {
  const cases = [
    [{ authenticated: true, error: 'err-a', errors: [], challenge: {} }, 'true', 'string', 'array', 'object'],
    [{ authenticated: false, error: null, errors: {}, challenge: [] }, 'false', 'null', 'object', 'array'],
    [{ authenticated: 'true', error: 7, errors: null, challenge: 'private-challenge' }, 'invalid', 'number', 'null', 'string'],
    [{ authenticated: null }, 'invalid', 'absent', 'absent', 'absent'],
    [{}, 'absent', 'absent', 'absent', 'absent'],
  ];
  for (const [value, flag, errorType, errorsType, challengeType] of cases) {
    const result = projectAccountSettingsBody(response(JSON.stringify(value)));
    assert.equal(result.diagnostic, 'projected');
    assert.equal(result.authenticatedFlag, flag);
    assert.equal(result.fields.error, errorType);
    assert.equal(result.fields.errors, errorsType);
    assert.equal(result.fields.challenge, challengeType);
    assert.equal(result.canAuthorize, false);
    assert.equal(result.settingsFieldCount, 0);
    for (const sourceValue of ['err-a', 'private-challenge']) {
      assert.equal(JSON.stringify(result).includes(sourceValue), false);
    }
  }

  const settings = projectAccountSettingsBody(response(JSON.stringify({
    preferences: { language: 'nested' }, unknown_preference: true,
  })));
  assert.equal(settings.settingsFieldCount, 0);
});

test('strictly rejects invalid CDP result shapes, malformed JSON, duplicates, and non-object roots', () => {
  for (const input of [
    null,
    [],
    { body: '{}' },
    { body: '{}', base64Encoded: 'false' },
    { body: '{}', base64Encoded: false, extra: 'secret' },
    Object.defineProperties({}, {
      body: { enumerable: true, get() { throw new Error('private getter text'); } },
      base64Encoded: { enumerable: true, value: false },
    }),
  ]) assertFailure(projectAccountSettingsBody(input), 'body_result_invalid');

  for (const raw of ['{', '{"id_str":"1","id_str":"2"}', '{"outer":{"x":1,"x":2}}']) {
    assertFailure(projectAccountSettingsBody(response(raw)), 'body_json_invalid', Buffer.byteLength(raw, 'utf8'));
  }
  for (const raw of ['[]', 'null', '7', '"text"']) {
    assertFailure(projectAccountSettingsBody(response(raw)), 'body_root_invalid', Buffer.byteLength(raw, 'utf8'));
  }
});

test('enforces parser depth and token limits and reports bounded output', () => {
  const tooDeep = '{"a":{"b":{"c":{"d":{"e":{"f":{"g":{"h":{"i":0}}}}}}}}}';
  assertFailure(projectAccountSettingsBody(response(tooDeep)), 'body_json_invalid', Buffer.byteLength(tooDeep, 'utf8'));
  const tooManyTokens = JSON.stringify(Array(1_025).fill(0));
  assertFailure(projectAccountSettingsBody(response(tooManyTokens)), 'body_json_invalid', Buffer.byteLength(tooManyTokens, 'utf8'));

  const largeValue = JSON.stringify({ screen_name: 'x'.repeat(ACCOUNT_SETTINGS_BODY_MAX_BYTES - 32) });
  const projected = projectAccountSettingsBody(response(largeValue));
  assert.equal(projected.diagnostic, 'projected');
  assert.equal(projected.fields.screen_name, 'string');
  assert.ok(Buffer.byteLength(JSON.stringify(projected), 'utf8') < 512);
});

test('checks overlong source strings before allocating a Buffer', () => {
  const originalFrom = Buffer.from;
  let allocationCalls = 0;
  Buffer.from = function countedFrom(...args) {
    allocationCalls++;
    return originalFrom.apply(this, args);
  };
  try {
    const oversizedPlain = projectAccountSettingsBody({
      body: 'x'.repeat(ACCOUNT_SETTINGS_BODY_MAX_BYTES + 1), base64Encoded: false,
    });
    assertFailure(oversizedPlain, 'body_oversized');
    const oversizedBase64 = projectAccountSettingsBody({
      body: 'A'.repeat(Math.ceil(ACCOUNT_SETTINGS_BODY_MAX_BYTES / 3) * 4 + 1),
      base64Encoded: true,
    });
    assertFailure(oversizedBase64, 'body_oversized');
    assert.equal(allocationCalls, 0);
  } finally {
    Buffer.from = originalFrom;
  }
});

test('enforces canonical base64, fatal UTF-8, and strict Unicode', () => {
  assertFailure(projectAccountSettingsBody({ body: '!!!!', base64Encoded: true }), 'body_encoding_invalid');
  assertFailure(projectAccountSettingsBody({ body: 'Zh==', base64Encoded: true }), 'body_encoding_invalid');
  assertFailure(projectAccountSettingsBody({ body: '/w==', base64Encoded: true }), 'body_encoding_invalid', 1);

  const loneSurrogate = `{"screen_name":"${String.fromCharCode(0xd800)}"}`;
  assertFailure(projectAccountSettingsBody({ body: loneSurrogate, base64Encoded: false }), 'body_encoding_invalid');
  const escapedLoneSurrogate = '{"screen_name":"\\ud800"}';
  assertFailure(projectAccountSettingsBody(response(escapedLoneSurrogate)), 'body_json_invalid',
    Buffer.byteLength(escapedLoneSurrogate, 'utf8'));

  const validBase64 = projectAccountSettingsBody(response('{"id_str":"123"}', true));
  assert.equal(validBase64.diagnostic, 'projected');
  assert.equal(validBase64.stableIdStringCandidates, 1);
  assert.equal(validBase64.stableIdStringsAgree, null);
});
