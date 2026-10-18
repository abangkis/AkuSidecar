import test from 'node:test';
import assert from 'node:assert/strict';
import { canonicalizeFixtureXBody, readQualifiedFixtureXBody, FIXTURE_X_BODY_SCHEMA } from './x-evidence-body.mjs';

function response(text, base64Encoded = false) {
  return { body: base64Encoded ? Buffer.from(text, 'utf8').toString('base64') : text, base64Encoded };
}

function body(state = 'authenticated', data = { id_str: '1234567890', username: 'fixture', name: 'Synthetic User' }) {
  return JSON.stringify({ schema: FIXTURE_X_BODY_SCHEMA, state, ...(data === null ? {} : { data }) });
}

test('canonical body keeps only bounded synthetic identity fields and reports both byte counts', () => {
  for (const base64Encoded of [false, true]) {
    const raw = body();
    const result = canonicalizeFixtureXBody(response(raw, base64Encoded));
    assert.equal(result.body, JSON.stringify({ schema: FIXTURE_X_BODY_SCHEMA,
      state: 'authenticated', data: { id_str: '1234567890' } }));
    assert.equal(result.bodyBytes, Buffer.byteLength(result.body, 'utf8'));
    assert.equal(result.decodedBodyBytes, Buffer.byteLength(raw, 'utf8'));
    assert.equal(result.identityPresent, true);
    assert.equal(JSON.stringify(result).includes('Synthetic User'), false);
  }
});

test('authenticated response with no identity stays explicitly authenticated but identity-unavailable', () => {
  const result = canonicalizeFixtureXBody(response(body('authenticated', null)));
  assert.equal(result.state, 'authenticated');
  assert.equal(result.identityPresent, false);
  assert.equal(result.body, JSON.stringify({ schema: FIXTURE_X_BODY_SCHEMA, state: 'authenticated' }));
});

test('JSON whitespace emitted by the loopback encoder remains legal', () => {
  const raw = `${body('authenticated')}\n`;
  const result = canonicalizeFixtureXBody(response(raw));
  assert.equal(result.state, 'authenticated');
  assert.equal(result.identityPresent, true);
  assert.equal(result.decodedBodyBytes, Buffer.byteLength(raw, 'utf8'));
});

test('logout, challenge, and unknown states retain their explicit synthetic state', () => {
  for (const state of ['logged_out', 'challenge', 'unknown']) {
    const result = canonicalizeFixtureXBody(response(body(state, null)));
    assert.equal(result.state, state);
    assert.equal(result.identityPresent, false);
  }
});

test('duplicate, unknown, malformed, or conflicting body fields fail with fixed errors', () => {
  const cases = [
    [`{"schema":"${FIXTURE_X_BODY_SCHEMA}","state":"authenticated","state":"logged_out"}`, 'body_invalid'],
    [`{"schema":"${FIXTURE_X_BODY_SCHEMA}","state":"authenticated","data":{"id_str":"123","id_str":"456"}}`, 'body_invalid'],
    [JSON.stringify({ schema: FIXTURE_X_BODY_SCHEMA, state: 'authenticated', private: 'secret' }), 'body_schema_invalid'],
    [JSON.stringify({ schema: FIXTURE_X_BODY_SCHEMA, state: 'logged_out', data: {} }), 'body_state_conflict'],
    [body('authenticated', { id_str: '0123' }), 'body_identity_invalid'],
    [body('authenticated', { id_str: 123 }), 'body_identity_invalid'],
    [body('authenticated', { id_str: '1'.repeat(21) }), 'body_identity_invalid'],
    [body('authenticated', { id_str: '123', extra: 'private' }), 'body_unknown_key'],
    [body('authenticated', { id_str: '123', name: ['private'] }), 'body_property_invalid'],
    [body('authenticated', { username: 'no identity' }), 'body_identity_missing'],
    [body('pending', null), 'body_state_invalid'],
  ];
  for (const [raw, code] of cases) {
    assert.throws(() => canonicalizeFixtureXBody(response(raw)), error => error.message === code);
  }
});

test('encoded and decoded bodies are bounded before JSON parsing', () => {
  assert.throws(() => canonicalizeFixtureXBody(response('x'.repeat(4_097))),
    error => error.message === 'body_oversized');
  const oversizedBase64 = Buffer.alloc(4_097, 0x78).toString('base64');
  assert.throws(() => canonicalizeFixtureXBody({ body: oversizedBase64, base64Encoded: true }),
    error => error.message === 'body_oversized');
  assert.throws(() => canonicalizeFixtureXBody({ body: '!!!!', base64Encoded: true }),
    error => error.message === 'body_base64_invalid');
  assert.throws(() => canonicalizeFixtureXBody({ body: '/w==', base64Encoded: true }),
    error => error.message === 'body_encoding_invalid');
  assert.throws(() => canonicalizeFixtureXBody({ body: '{}', base64Encoded: false, headers: {} }),
    error => error.message === 'body_invalid');
});

test('qualified body reader passes only the ingress-bound request ID and projects before promise resolution', async () => {
  const ticket = Object.freeze({});
  const ingress = {
    withBodyRequest(received, callback) {
      assert.equal(received, ticket);
      return callback('opaque-network-request-id');
    },
  };
  let projected = false;
  const result = await readQualifiedFixtureXBody({
    ingress,
    ticket,
    getResponseBody(requestId, project) {
      assert.equal(requestId, 'opaque-network-request-id');
      const safe = project({ body: body('authenticated', null), base64Encoded: false });
      projected = true;
      return Promise.resolve(safe);
    },
  });
  assert.equal(projected, true);
  assert.equal(result.identityPresent, false);
  assert.equal(result.body, JSON.stringify({ schema: FIXTURE_X_BODY_SCHEMA, state: 'authenticated' }));
  assert.equal(JSON.stringify(result).includes('opaque-network-request-id'), false);
});
