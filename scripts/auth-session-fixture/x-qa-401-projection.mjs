// One bounded rejection projection at private CDP dispatch. Never export
// messages, arbitrary numeric codes, body keys or account/session values.
import { projectQADirectBody } from './x-qa-direct-ab.mjs';
import { parseStrictJSON } from './x-evidence-ingress.mjs';

function result(shape, codeClass = 'none', messageMatch = null) {
  return Object.freeze({ schema: 'aku.x.qa-rejection.v1', shape, codeClass, messageMatch,
    classification: shape === 'single_error' && codeClass === 'code_32' && messageMatch === true ? 'provider_authentication_error' : 'unknown' });
}

export function projectQA401Body(input) {
  // Reuse strict CDP-result, UTF-8/base64, 16 KiB, depth and token validation.
  const body = projectQADirectBody(input);
  if (body.category === 'unreadable') return Object.freeze({ body, rejection: result('unreadable') });
  let rejection = result('other_json');
  try {
    const text = input.base64Encoded ? Buffer.from(input.body, 'base64').toString('utf8') : input.body;
    const parsed = parseStrictJSON(text, { maxJSONDepth: 8, maxJSONTokens: 1024 });
    if (parsed && !Array.isArray(parsed) && Object.keys(parsed).length === 1 && Array.isArray(parsed.errors) && parsed.errors.length === 1) {
      const error = parsed.errors[0];
      if (error && !Array.isArray(error) && Object.keys(error).length === 2 && Object.hasOwn(error, 'code') && Object.hasOwn(error, 'message') &&
          Number.isSafeInteger(error.code) && error.code >= 0 && error.code <= 65535 && typeof error.message === 'string' &&
          error.message.length <= 256 && Buffer.byteLength(error.message, 'utf8') <= 512) {
        const codeClass = error.code === 32 ? 'code_32' : 'other';
        const match = codeClass === 'code_32' && ['Could not authenticate you.', 'Could not authenticate you'].includes(error.message);
        rejection = result('single_error', codeClass, match);
      }
    }
  } catch { rejection = result('unreadable'); }
  return Object.freeze({ body, rejection });
}

export function validateQARejection(value) {
  const keys = ['schema', 'shape', 'codeClass', 'messageMatch', 'classification'];
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length !== 5 ||
      Object.keys(value).some(key => !keys.includes(key)) || value.schema !== 'aku.x.qa-rejection.v1' ||
      !['single_error', 'other_json', 'unreadable'].includes(value.shape) ||
      !['none', 'code_32', 'other'].includes(value.codeClass)) throw new TypeError('qa_rejection_invalid');
  if (value.shape === 'single_error' ? value.codeClass === 'none' || typeof value.messageMatch !== 'boolean' ||
      value.codeClass !== 'code_32' && value.messageMatch : value.codeClass !== 'none' || value.messageMatch !== null) throw new TypeError('qa_rejection_invalid');
  const expected = result(value.shape, value.codeClass, value.messageMatch);
  if (expected.classification !== value.classification) throw new TypeError('qa_rejection_invalid');
  return expected;
}

export function finishQAWeb401(base, rejection) {
  if (base?.schema !== 'aku.x.qa-web-aba.v1' || base.authentication !== 'unknown' || base.canAuthorize !== false || base.sessionTransferred !== false) throw new TypeError('qa_rejection_base_invalid');
  const projected = rejection === null ? null : validateQARejection(rejection);
  const qualifies = base.outcome === 'cookie_dependency_candidate' && base.hook.outcome === 'completed' &&
    base.hook.attempted === true && base.hook.rejected === false && base.hookRemoved === true &&
    projected?.classification === 'provider_authentication_error';
  return Object.freeze({ ...base, schema: 'aku.x.qa-web-aba.v2', rejection: projected,
    outcome: qualifies ? 'cookie_auth_rejection_observed' : base.outcome });
}
