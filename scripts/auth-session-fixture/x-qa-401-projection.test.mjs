import test from 'node:test';
import assert from 'node:assert/strict';
import { projectQA401Body, validateQARejection, finishQAWeb401 } from './x-qa-401-projection.mjs';

const AUTH_MESSAGE = 'Could not authenticate you.';
const project = (body, encoded = false) => projectQA401Body({ body: encoded ? Buffer.from(JSON.stringify(body)).toString('base64') : JSON.stringify(body), base64Encoded: encoded });
test('one code-32 envelope with privately matching canonical message yields only a fixed authentication-error category', () => {
  for (const message of [AUTH_MESSAGE, AUTH_MESSAGE.slice(0, -1)]) {
    const result = project({ errors: [{ code: 32, message }] });
    assert.equal(result.body.category, 'error_envelope');
    assert.deepEqual(result.rejection, { schema: 'aku.x.qa-rejection.v1', shape: 'single_error', codeClass: 'code_32',
      messageMatch: true, classification: 'provider_authentication_error' });
    assert.equal(JSON.stringify(result).includes(message), false);
    assert.deepEqual(project({ errors: [{ code: 32, message }] }, true), result);
  }
});
test('CSRF, rate-limit, anti-abuse and unknown codes never become code-32 authentication evidence', () => {
  for (const code of [353, 88, 226, 326, 9999]) {
    const r = project({ errors: [{ code, message: 'PRIVATE_NATIVE_MESSAGE' }] });
    assert.equal(r.rejection.codeClass, 'other'); assert.equal(r.rejection.messageMatch, false);
    assert.equal(r.rejection.classification, 'unknown'); assert.equal(JSON.stringify(r).includes('PRIVATE_NATIVE_MESSAGE'), false);
    assert.equal(JSON.stringify(r.rejection).includes(String(code)), false);
  }
});
test('message drift, conflicts, extra fields, multiple errors and nonnumeric codes fail closed without reflecting values', () => {
  const envelope = { errors: [{ code: 32, message: AUTH_MESSAGE }] };
  for (const body of [
    { errors: [{ code: 32, message: 'PRIVATE_DRIFT' }] },
    { ...envelope, challenge: 'PRIVATE' }, { ...envelope, csrf: 'PRIVATE' },
    { errors: [{ code: 32, message: AUTH_MESSAGE, detail: 'PRIVATE' }] },
    { errors: [{ code: '32', message: AUTH_MESSAGE }] }, { errors: [] },
    { errors: [envelope.errors[0], { code: 353, message: 'PRIVATE' }] },
    { errors: [{ code: 32, message: 'PRIVATE'.repeat(100) }] },
    { error: 'PRIVATE' }, { id: 'PRIVATE', errors: envelope.errors },
  ]) {
    const r = project(body); assert.equal(r.rejection.classification, 'unknown');
    assert.equal(JSON.stringify(r).includes('PRIVATE'), false); assert.equal(JSON.stringify(r).includes(AUTH_MESSAGE), false);
  }
});
test('malformed duplicate-key oversized and accessor bodies cannot enter the rejection projector', () => {
  for (const body of ['{', ' '.repeat(16385), '{"errors":[],"errors":[]}']) {
    assert.equal(projectQA401Body({ body, base64Encoded: false }).rejection.shape, 'unreadable');
  }
  let reads = 0;
  const input = { base64Encoded: false }; Object.defineProperty(input, 'body', { get() { reads++; throw new Error('PRIVATE'); }, enumerable: true });
  assert.equal(projectQA401Body(input).rejection.classification, 'unknown'); assert.equal(reads, 0);
});
test('closed DTO rejects native message reflection, inconsistent classifications and malformed pairs', () => {
  const good = project({ errors: [{ code: 32, message: AUTH_MESSAGE }] }).rejection;
  for (const patch of [{ message: 'PRIVATE' }, { code: 32 }, { messageMatch: false }, { codeClass: 'other' },
    { shape: 'other_json' }, { classification: 'authenticated' }]) assert.throws(() => validateQARejection({ ...good, ...patch }));
});
test('source qualification requires successful A/B/A, completed hook and independent narrow rejection; authority stays unknown', () => {
  const base = { schema: 'aku.x.qa-web-aba.v1', outcome: 'cookie_dependency_candidate',
    hook: { outcome: 'completed', attempted: true, rejected: false }, hookRemoved: true,
    authentication: 'unknown', canAuthorize: false, sessionTransferred: false };
  const projection = project({ errors: [{ code: 32, message: AUTH_MESSAGE }] }).rejection;
  const result = finishQAWeb401(base, projection);
  assert.equal(result.schema, 'aku.x.qa-web-aba.v2'); assert.equal(result.outcome, 'cookie_auth_rejection_observed');
  assert.equal(result.authentication, 'unknown'); assert.equal(result.canAuthorize, false); assert.equal(result.sessionTransferred, false);
  for (const outcome of ['positive_control_failed', 'comparison_inconclusive', 'comparison_not_run']) {
    assert.equal(finishQAWeb401({ ...base, outcome }, projection).outcome, outcome);
  }
  assert.equal(finishQAWeb401(base, null).outcome, 'cookie_dependency_candidate');
  assert.equal(finishQAWeb401({ ...base, hook: { ...base.hook, outcome: 'network_error' } }, projection).outcome, 'cookie_dependency_candidate');
});
