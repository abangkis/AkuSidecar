import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { STRICT_JSON_PARSER_SOURCE } from './x-evidence-ingress.mjs';
import { BOOTSTRAP_CURRENT_SELF_PROJECTION_SOURCE } from './x-bootstrap-current-self.mjs';

import {
  QA_LIVE_SELF_EXPRESSION,
  projectQALiveSelf,
  validateQALiveSelfDiagnostic,
  validateQALiveSelfReport,
} from './x-qa-live-self.mjs';

const TARGET_URL = 'https://x.com/home';
const BODY_LIMIT = 512 * 1024;
const INITIALIZER_LIMIT = 256 * 1024;
const ID = '12345678901234567890';

function bytes(value) {
  return typeof value === 'string' ? new TextEncoder().encode(value) : value;
}

function responseFor(source, {
  status = 200,
  type = 'basic',
  redirected = false,
  url = TARGET_URL,
  contentType = 'text/html; charset=utf-8',
  chunks,
  getReaderError = null,
} = {}) {
  const sourceBytes = bytes(source);
  const sourceChunks = chunks ?? [sourceBytes];
  const state = {
    reads: 0,
    requested: [],
    delivered: 0,
    cancelled: false,
    bodyCancelled: false,
    readerCancelCalls: 0,
  };
  const response = {
    status,
    type,
    redirected,
    url,
    headers: { get(name) { return name.toLowerCase() === 'content-type' ? contentType : null; } },
    body: {
      cancel() { state.bodyCancelled = true; return Promise.resolve(); },
      getReader(options) {
        assert.equal(options?.mode, 'byob');
        assert.deepEqual(Object.keys(options).sort(), ['mode']);
        if (getReaderError !== null) throw getReaderError;
        let chunkIndex = 0;
        let chunkOffset = 0;
        return {
          async read(view) {
            state.reads += 1;
            assert.ok(view instanceof Uint8Array);
            state.requested.push(view.byteLength);
            while (chunkIndex < sourceChunks.length) {
              const current = bytes(sourceChunks[chunkIndex]);
              if (current.byteLength === 0) {
                chunkIndex += 1;
                chunkOffset = 0;
                structuredClone(view.buffer, { transfer: [view.buffer] });
                return { done: false, value: new Uint8Array(0) };
              }
              const count = Math.min(view.byteLength, current.byteLength - chunkOffset);
              const value = new Uint8Array(count);
              value.set(current.subarray(chunkOffset, chunkOffset + count));
              chunkOffset += count;
              state.delivered += count;
              if (chunkOffset === current.byteLength) {
                chunkIndex += 1;
                chunkOffset = 0;
              }
              structuredClone(view.buffer, { transfer: [view.buffer] });
              return { done: false, value };
            }
            structuredClone(view.buffer, { transfer: [view.buffer] });
            return { done: true, value: new Uint8Array(0) };
          },
          cancel() {
            state.cancelled = true;
            state.readerCancelCalls += 1;
            return Promise.resolve();
          },
        };
      },
    },
  };
  return { response, state };
}

function extractScripts(html, selector) {
  assert.equal(selector, 'script');
  const scripts = [];
  const expression = /<script\b[^>]*>([\s\S]*?)<\/script\s*>/giu;
  for (const match of html.matchAll(expression)) scripts.push({ textContent: match[1] });
  return scripts;
}

function fakeTemplateDocument({ innerHTMLFailure = null, contentFailure = null } = {}) {
  const state = { created: 0, assignments: 0, inserted: 0, htmlBytes: 0, queried: 0 };
  const document = {
    createElement(tagName) {
      assert.equal(tagName, 'template');
      state.created += 1;
      const template = { isConnected: false };
      Object.defineProperty(template, 'innerHTML', {
        enumerable: true,
        set(html) {
          state.assignments += 1;
          state.htmlBytes = Buffer.byteLength(html, 'utf8');
          if (innerHTMLFailure !== null) throw innerHTMLFailure;
          Object.defineProperty(template, 'content', {
            configurable: true,
            enumerable: true,
            get() {
              if (contentFailure !== null) throw contentFailure;
              return {
                querySelectorAll(selector) {
                  state.queried += 1;
                  return extractScripts(html, selector);
                },
              };
            },
          });
        },
      });
      return template;
    },
  };
  return { document, state };
}

function makeContext({
  fetch,
  timeoutMode = 'manual',
  locationHref = TARGET_URL,
  document,
  TextDecoderImpl = TextDecoder,
  clock,
} = {}) {
  let timeoutCallback = null;
  let timeoutId = 0;
  let tick = 0;
  const aborts = [];
  const templateHarness = document === undefined ? fakeTemplateDocument() : null;
  const activeDocument = document === undefined ? templateHarness.document : document;
  class TestAbortController {
    constructor() {
      this.signal = { aborted: false };
      aborts.push(this);
    }
    abort() { this.signal.aborted = true; }
  }
  const location = { href: locationHref };
  const context = {
    location,
    document: activeDocument,
    TextDecoder: TextDecoderImpl,
    TextEncoder,
    Uint8Array,
    AbortController: TestAbortController,
    fetch,
    performance: {
      now() {
        if (clock !== undefined) return clock();
        tick += 1;
        return tick;
      },
    },
    setTimeout(callback, milliseconds) {
      assert.equal(milliseconds, 2_000);
      timeoutCallback = callback;
      if (timeoutMode === 'real') return setTimeout(callback, milliseconds);
      timeoutId += 1;
      return timeoutId;
    },
    clearTimeout(id) {
      if (timeoutMode === 'real') clearTimeout(id);
      else assert.equal(id, timeoutId);
    },
  };
  return {
    context,
    location,
    aborts,
    templateState: templateHarness?.state ?? null,
    fireTimeout: () => timeoutCallback?.(),
  };
}

async function runPageProbe(options = {}) {
  const { context, ...state } = makeContext(options);
  const result = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  return { result, ...state };
}

function validSelfJSON({ session = {}, extra = {} } = {}) {
  return JSON.stringify({
    session: { user_id_str: ID, ...session },
    entities: { users: { entities: { [ID]: { id_str: ID } } } },
    ...extra,
  });
}

function htmlForJSON(sourceJSON) {
  return `<html><body><script>window.__INITIAL_STATE__ = ${sourceJSON};</script></body></html>`;
}

function fixedByteInitializer(byteLength, includeMultibyte = false) {
  const prefix = `{"session":{"user_id_str":"${ID}"},"entities":{"users":{"entities":{"${ID}":{"id_str":"${ID}"}}}},"filler":"`;
  const suffix = '"}';
  const remaining = byteLength - Buffer.byteLength(prefix + suffix, 'utf8');
  assert.ok(remaining >= 0);
  const filler = includeMultibyte && remaining >= 2
    ? `${'x'.repeat(remaining - 2)}é`
    : 'x'.repeat(remaining);
  const value = prefix + filler + suffix;
  assert.equal(Buffer.byteLength(value, 'utf8'), byteLength);
  return value;
}

function innerResultFields(result) {
  return Object.keys(result).sort();
}

test('executes one fixed GET and returns a bounded diagnostic from an inert template', async () => {
  const sourceJSON = validSelfJSON();
  const html = `<img src="https://images.invalid/pixel"><iframe src="https://frames.invalid/embed"></iframe>${htmlForJSON(sourceJSON)}`;
  const { response, state } = responseFor(html);
  let calls = 0;
  const harness = makeContext({ fetch: async (...args) => {
    calls += 1;
    assert.equal(args.length, 2);
    assert.equal(args[0], TARGET_URL);
    assert.deepEqual(Object.keys(args[1]).sort(), ['cache', 'credentials', 'method', 'redirect', 'signal']);
    assert.equal(args[1].method, 'GET');
    assert.equal(args[1].credentials, 'same-origin');
    assert.equal(args[1].cache, 'no-store');
    assert.equal(args[1].redirect, 'error');
    assert.equal(args[1].headers, undefined);
    return response;
  } });
  Object.defineProperty(harness.context, 'DOMParser', {
    enumerable: true,
    get() { throw new Error('DOMParser must not be used'); },
  });
  const result = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, harness.context, { timeout: 1_000 });

  assert.equal(calls, 1);
  assert.equal(state.cancelled, false);
  assert.ok(state.requested.every((length) => length > 0 && length <= 4_096));
  assert.ok(state.requested.reduce((sum, length) => sum + length, 0) <= BODY_LIMIT + 1 + 4_096);
  assert.equal(harness.templateState.created, 1);
  assert.equal(harness.templateState.assignments, 1);
  assert.equal(harness.templateState.inserted, 0);
  assert.equal(harness.templateState.queried, 1);
  assert.deepEqual(innerResultFields(result), [
    'bodyBytes', 'canAuthorize', 'freshness', 'identity', 'initializerBytes', 'nativeQualified',
    'outcome', 'parserRejection', 'projection', 'schema', 'sessionTransferred', 'status',
  ]);
  assert.equal(result.schema, 'aku.x.qa-live-self.v3');
  assert.equal(result.outcome, 'candidate');
  assert.equal(result.parserRejection, null);
  assert.equal(result.bodyBytes, Buffer.byteLength(html));
  assert.equal(result.initializerBytes, Buffer.byteLength(sourceJSON));
  assert.equal(JSON.stringify(result).includes(sourceJSON), false);
  assert.equal(JSON.stringify(result).includes(ID), false);
});

test('projects only a strong current-self entity match and keeps all authority flags false', async () => {
  const { response } = responseFor(htmlForJSON(validSelfJSON()));
  const pageResult = await runPageProbe({ fetch: async () => response });
  const diagnostic = projectQALiveSelf(pageResult.result);

  assert.equal(diagnostic.outcome, 'candidate');
  assert.equal(diagnostic.identity, 'candidate');
  assert.equal(diagnostic.freshness, 'server-response-candidate');
  assert.equal(diagnostic.projection.entityChecks.normalizedUsers, 'match');
  assert.equal(diagnostic.canAuthorize, false);
  assert.equal(diagnostic.nativeQualified, false);
  assert.equal(diagnostic.sessionTransferred, false);
  assert.equal(JSON.stringify(diagnostic).includes(ID), false);
  assert.deepEqual(validateQALiveSelfDiagnostic(diagnostic), diagnostic);
});

test('accepts semicolons inside JSON strings and rejects statements after the initializer', async () => {
  const semicolonString = validSelfJSON({ extra: { note: 'semi;colon' } });
  const valid = await runPageProbe({ fetch: async () => responseFor(htmlForJSON(semicolonString)).response });
  assert.equal(valid.result.outcome, 'candidate');

  const trailingCode = await runPageProbe({
    fetch: async () => responseFor(`<script>window.__INITIAL_STATE__ = ${validSelfJSON()}; alert(1);</script>`).response,
  });
  assert.equal(trailingCode.result.outcome, 'parser_rejected');
  assert.equal(trailingCode.result.projection, null);
});

test('distinguishes no exact initializer, duplicate markers, and HTML decode/template failures', async () => {
  const absent = responseFor('<html><script>window.other = {};</script></html>');
  const absentRun = await runPageProbe({ fetch: async () => absent.response });
  assert.equal(absentRun.result.outcome, 'source_absent');
  assert.equal(absentRun.result.bodyBytes, Buffer.byteLength('<html><script>window.other = {};</script></html>'));
  assert.equal(absentRun.result.initializerBytes, 0);
  assert.equal(absentRun.templateState.queried, 1);

  const nonExact = responseFor('<script>window.__INITIAL_STATE__ = {}; sideEffect();</script>');
  const nonExactRun = await runPageProbe({ fetch: async () => nonExact.response });
  assert.equal(nonExactRun.result.outcome, 'parser_rejected');

  const unterminated = responseFor('<script>window.__INITIAL_STATE__ = {"session":{}}</script>');
  const unterminatedRun = await runPageProbe({ fetch: async () => unterminated.response });
  assert.equal(unterminatedRun.result.outcome, 'source_absent');

  const duplicate = responseFor(
    `<script>window.__INITIAL_STATE__ = {"session":{}};</script><script>window.__INITIAL_STATE__ = {"session":{}};</script>`,
  );
  const ambiguous = await runPageProbe({ fetch: async () => duplicate.response });
  assert.equal(ambiguous.result.outcome, 'source_ambiguous');

  const invalidUTF8 = responseFor(new Uint8Array([0xc3, 0x28]));
  const decodeRejected = await runPageProbe({ fetch: async () => invalidUTF8.response });
  assert.equal(decodeRejected.result.outcome, 'html_rejected');
  assert.equal(decodeRejected.result.initializerBytes, 0);

  const decodeThrow = await runPageProbe({
    fetch: async () => responseFor('<html></html>').response,
    TextDecoderImpl: class { decode() { throw new Error('private decoder detail'); } },
  });
  assert.equal(decodeThrow.result.outcome, 'html_rejected');
  assert.equal(JSON.stringify(decodeThrow.result).includes('private decoder detail'), false);

  const throwingDocument = fakeTemplateDocument({ innerHTMLFailure: new Error('trusted types detail') });
  const templateThrow = await runPageProbe({
    fetch: async () => responseFor('<html></html>').response,
    document: throwingDocument.document,
  });
  assert.equal(templateThrow.result.outcome, 'html_rejected');
  assert.equal(templateThrow.result.initializerBytes, 0);
  assert.equal(JSON.stringify(templateThrow.result).includes('trusted types detail'), false);

  const contentThrow = fakeTemplateDocument({ contentFailure: new Error('content detail') });
  const contentRejected = await runPageProbe({
    fetch: async () => responseFor('<html></html>').response,
    document: contentThrow.document,
  });
  assert.equal(contentRejected.result.outcome, 'html_rejected');
});

test('strictly rejects malformed, duplicate-key, unsafe-number, invalid-Unicode, depth, and token-budget JSON', async () => {
  const rejected = [
    '',
    '{broken',
    '{"session":{},"session":{}}',
    '{"unsafe":9007199254740992}',
    '{"invalid":"\\uD800"}',
    `${'['.repeat(25)}0${']'.repeat(25)}`,
    `[${Array.from({ length: 16_384 }, () => '0').join(',')}]`,
  ];
  for (const source of rejected) {
    const { response } = responseFor(htmlForJSON(source));
    const result = await runPageProbe({ fetch: async () => response });
    assert.equal(result.result.outcome, 'parser_rejected');
    assert.equal(result.result.projection, null);
    assert.equal(result.result.initializerBytes, Buffer.byteLength(source));
  }

  const depth24 = `${'['.repeat(24)}0${']'.repeat(24)}`;
  const acceptedDepth = await runPageProbe({ fetch: async () => responseFor(htmlForJSON(depth24)).response });
  assert.equal(acceptedDepth.result.outcome, 'self_unqualified');

  const valueBoundary = `[${Array.from({ length: 16_383 }, () => '0').join(',')}]`;
  const acceptedValues = await runPageProbe({ fetch: async () => responseFor(htmlForJSON(valueBoundary)).response });
  assert.equal(acceptedValues.result.outcome, 'self_unqualified');
});

test('returns only closed rejection categories for synthetic parser failures', async () => {
  const cases = [
    ['', 'malformed_json'],
    ['{"PRIVATE_FIELD":', 'malformed_json'],
    ['{"PRIVATE_FIELD":1,"PRIVATE_FIELD":2}', 'duplicate_key'],
    ['{"PRIVATE_FIELD":"\\uD800"}', 'invalid_unicode'],
    ['{"PRIVATE_FIELD":9007199254740992}', 'unsafe_number'],
    ['{"PRIVATE_FIELD":1e400}', 'unsafe_number'],
    [`${'['.repeat(25)}0${']'.repeat(25)}`, 'depth_limit'],
    [`[${Array.from({ length: 16_384 }, () => '0').join(',')}]`, 'value_limit'],
  ];
  for (const [source, reason] of cases) {
    const page = await runPageProbe({ fetch: async () => responseFor(htmlForJSON(source)).response });
    const result = projectQALiveSelf(page.result);
    assert.equal(result.outcome, 'parser_rejected');
    assert.equal(result.parserRejection, reason);
    assert.equal(result.projection, null);
    const encoded = JSON.stringify(result);
    assert.ok(Buffer.byteLength(encoded) <= 4096);
    assert.equal(encoded.includes('PRIVATE_FIELD'), false);
    assert.equal(encoded.includes('9007199254740992'), false);
    assert.equal(result.canAuthorize, false);
  }
});

test('unexpected parser and projection faults never expose thrown messages', async () => {
  for (const [helper, reason] of [
    [STRICT_JSON_PARSER_SOURCE, 'parser_internal'],
    [BOOTSTRAP_CURRENT_SELF_PROJECTION_SOURCE, 'projection_failed'],
  ]) {
    const expression = QA_LIVE_SELF_EXPRESSION.replace(helper, '(() => { throw new Error("PRIVATE_THROWN_MESSAGE"); })');
    assert.notEqual(expression, QA_LIVE_SELF_EXPRESSION);
    const harness = makeContext({ fetch: async () => responseFor(htmlForJSON(validSelfJSON())).response });
    const result = projectQALiveSelf(await vm.runInNewContext(expression, harness.context, { timeout: 1_000 }));
    assert.equal(result.parserRejection, reason);
    assert.equal(result.outcome, 'parser_rejected');
    assert.equal(JSON.stringify(result).includes('PRIVATE_THROWN_MESSAGE'), false);
  }
});

test('rejects missing, arbitrary, accessor and unrelated rejection categories', async () => {
  const page = await runPageProbe({ fetch: async () => responseFor(htmlForJSON('{broken')).response });
  const valid = projectQALiveSelf(page.result);
  for (const reason of [null, '', 'PRIVATE_RAW_ERROR', {}, 7, ['malformed_json']]) {
    assert.throws(() => validateQALiveSelfDiagnostic({ ...valid, parserRejection: reason }), /qa_live_self_report_invalid/);
  }
  const missing = { ...valid };
  delete missing.parserRejection;
  assert.throws(() => validateQALiveSelfDiagnostic(missing), /qa_live_self_report_invalid/);
  const accessor = { ...valid };
  let reads = 0;
  Object.defineProperty(accessor, 'parserRejection', { enumerable: true, get() { reads++; throw new Error('PRIVATE'); } });
  assert.throws(() => validateQALiveSelfDiagnostic(accessor), /qa_live_self_report_invalid/);
  assert.equal(reads, 0);
  const other = await runPageProbe({ fetch: async () => responseFor('<html></html>').response });
  assert.throws(() => validateQALiveSelfDiagnostic({ ...other.result, parserRejection: 'malformed_json' }), /qa_live_self_report_invalid/);
  assert.throws(() => validateQALiveSelfDiagnostic({ ...valid, schema: 'aku.x.qa-live-self.v2' }), /qa_live_self_report_invalid/);
});

test('does not infer identity from numeric IDs, auth-false data, mismatches, or feed authors', async () => {
  const mismatched = JSON.parse(validSelfJSON());
  mismatched.entities.users.entities[ID].id_str = '999';
  const cases = [
    {
      name: 'numeric session ID',
      value: { session: { user_id: 123456789 }, entities: { users: { entities: {} } } },
    },
    { name: 'auth false', value: JSON.parse(validSelfJSON({ session: { loggedIn: false } })) },
    { name: 'normalized entity mismatch', value: mismatched },
    { name: 'feed author only', value: { session: {}, feed: { entries: [{ author_id: ID }] } } },
  ];

  for (const item of cases) {
    const { response } = responseFor(htmlForJSON(JSON.stringify(item.value)));
    const result = await runPageProbe({ fetch: async () => response });
    assert.equal(result.result.outcome, 'self_unqualified', item.name);
    assert.equal(result.result.identity, 'unavailable', item.name);
  }
});

test('keeps the complete DTO under 4 KiB and never exposes initializer, HTML, or account IDs', async () => {
  const source = validSelfJSON({ extra: { largeOpaqueState: 'PRIVATE_HTML_OR_ID '.repeat(8_000) } });
  const html = `<!-- PRIVATE_HTML_MARKER -->${htmlForJSON(source)}`;
  assert.ok(Buffer.byteLength(source) < INITIALIZER_LIMIT);
  const { response } = responseFor(html);
  const pageResult = await runPageProbe({ fetch: async () => response });
  const diagnostic = projectQALiveSelf(pageResult.result);
  const serialized = JSON.stringify(diagnostic);
  assert.ok(Buffer.byteLength(serialized, 'utf8') <= 4_096);
  for (const secret of [source, ID, 'PRIVATE_HTML_MARKER', 'PRIVATE_HTML_OR_ID']) {
    assert.equal(serialized.includes(secret), false);
  }
});

test('accepts a 512 KiB HTML body, probes only one overflow byte, and rejects 512 KiB plus one', async () => {
  const source = validSelfJSON();
  const base = htmlForJSON(source);
  const exactHTML = `${base}<!--${'x'.repeat(BODY_LIMIT - Buffer.byteLength(base) - 7)}-->`;
  assert.equal(Buffer.byteLength(exactHTML), BODY_LIMIT);
  const exact = responseFor(exactHTML);
  const exactResult = await runPageProbe({ fetch: async () => exact.response });
  assert.equal(exactResult.result.outcome, 'candidate');
  assert.equal(exactResult.result.bodyBytes, BODY_LIMIT);
  assert.equal(exact.state.delivered, BODY_LIMIT);
  assert.equal(exact.state.requested.at(-1), 1);
  assert.equal(exact.state.requested.length, 129);
  assert.ok(exact.state.requested.every((length) => length > 0 && length <= 4_096));

  const overHTML = `${exactHTML}x`;
  assert.equal(Buffer.byteLength(overHTML), BODY_LIMIT + 1);
  const over = responseFor(overHTML);
  const overRun = await runPageProbe({ fetch: async () => over.response });
  assert.equal(overRun.result.outcome, 'body_limit');
  assert.equal(overRun.result.bodyBytes, BODY_LIMIT + 1);
  assert.equal(overRun.result.initializerBytes, 0);
  assert.equal(over.state.delivered, BODY_LIMIT + 1);
  assert.equal(over.state.requested.at(-1), 1);
  assert.equal(over.state.cancelled, true);
  assert.equal(overRun.aborts[0].signal.aborted, true);

  const deadlineOver = responseFor(overHTML);
  const deadlineHarness = makeContext({
    fetch: async () => deadlineOver.response,
    clock: () => deadlineOver.state.delivered > BODY_LIMIT ? 2_000 : 0,
  });
  const deadlineResult = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, deadlineHarness.context, { timeout: 1_000 });
  assert.equal(deadlineResult.outcome, 'timeout');
  assert.equal(deadlineResult.bodyBytes, BODY_LIMIT + 1);
  assert.equal(deadlineResult.initializerBytes, 0);
  assert.equal(validateQALiveSelfDiagnostic(deadlineResult).bodyBytes, BODY_LIMIT + 1);
});

test('scans a near-cap whitespace-only initializer without regex backtracking', { timeout: 3_000 }, async () => {
  const shell = '<html><script>window.__INITIAL_STATE__ =</script></html>';
  const padding = ' '.repeat(BODY_LIMIT - Buffer.byteLength(shell));
  const html = `<html><script>window.__INITIAL_STATE__ =${padding}</script></html>`;
  assert.equal(Buffer.byteLength(html), BODY_LIMIT);
  const response = responseFor(html);
  const result = await runPageProbe({ fetch: async () => response.response });
  assert.equal(result.result.outcome, 'source_absent');
  assert.equal(result.result.bodyBytes, BODY_LIMIT);
  assert.equal(result.result.initializerBytes, 0);
});

test('enforces the UTF-8 initializer cap at 256 KiB plus one detection byte', async () => {
  const exactJSON = fixedByteInitializer(INITIALIZER_LIMIT, true);
  const exact = await runPageProbe({ fetch: async () => responseFor(htmlForJSON(exactJSON)).response });
  assert.equal(exact.result.outcome, 'candidate');
  assert.equal(exact.result.initializerBytes, INITIALIZER_LIMIT);

  const overJSON = fixedByteInitializer(INITIALIZER_LIMIT + 1);
  const overResponse = responseFor(htmlForJSON(overJSON));
  const over = await runPageProbe({ fetch: async () => overResponse.response });
  assert.equal(over.result.outcome, 'initializer_limit');
  assert.equal(over.result.bodyBytes, Buffer.byteLength(htmlForJSON(overJSON)));
  assert.equal(over.result.initializerBytes, INITIALIZER_LIMIT + 1);
  assert.equal(over.result.projection, null);
  assert.equal(overResponse.state.cancelled, true);
});

test('rejects wrong page and unsafe response metadata before reading, cancelling every rejected body', async () => {
  const cases = [
    { name: 'wrong page', locationHref: 'https://x.com/home?tab=1', response: responseFor('<script></script>') },
    { name: 'redirect', response: responseFor('<script></script>', { redirected: true }) },
    { name: 'redirected URL', response: responseFor('<script></script>', { url: 'https://x.com/login' }) },
    { name: 'opaque type', response: responseFor('<script></script>', { type: 'opaque' }) },
    { name: 'non-HTML', response: responseFor('<script></script>', { contentType: 'application/json' }) },
    { name: 'non-200', response: responseFor('<script></script>', { status: 503 }) },
  ];
  for (const item of cases) {
    const run = await runPageProbe({
      locationHref: item.locationHref,
      fetch: async () => item.response.response,
    });
    assert.equal(run.result.outcome, 'response_rejected', item.name);
    assert.equal(run.result.initializerBytes, 0, item.name);
    assert.equal(item.response.state.reads, 0, item.name);
    if (item.name !== 'wrong page') {
      assert.equal(item.response.state.bodyCancelled, true, item.name);
      assert.equal(run.aborts[0].signal.aborted, true, item.name);
    }
  }
});

test('fails closed without BYOB support and bounds nonempty and zero-read loops', async () => {
  const unsupported = responseFor('<html></html>', { getReaderError: new Error('private BYOB detail') });
  const unsupportedRun = await runPageProbe({ fetch: async () => unsupported.response });
  assert.equal(unsupportedRun.result.outcome, 'fetch_failed');
  assert.equal(unsupported.state.bodyCancelled, true);
  assert.equal(unsupportedRun.aborts[0].signal.aborted, true);
  assert.equal(JSON.stringify(unsupportedRun.result).includes('private BYOB detail'), false);

  for (const [name, chunks] of [
    ['nonempty', Array.from({ length: 257 }, () => new Uint8Array([1]))],
    ['zero-read', Array.from({ length: 257 }, () => new Uint8Array(0))],
  ]) {
    const response = responseFor('', { chunks });
    const run = await runPageProbe({ fetch: async () => response.response });
    assert.equal(run.result.outcome, 'fetch_failed', name);
    assert.equal(response.state.reads, 257, name);
    assert.equal(response.state.cancelled, true, name);
    assert.equal(run.aborts[0].signal.aborted, true, name);
  }
});

test('times out pending fetches and streams, cancels late responses, and rejects a regressing clock', async () => {
  let rejectFetch;
  const pendingFetch = makeContext({
    fetch: () => new Promise((_resolve, reject) => { rejectFetch = reject; }),
  });
  const pendingPromise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, pendingFetch.context, { timeout: 1_000 });
  await Promise.resolve();
  pendingFetch.fireTimeout();
  const timedOutFetch = await pendingPromise;
  rejectFetch?.(new Error('private timeout marker'));
  assert.equal(timedOutFetch.outcome, 'timeout');
  assert.equal(pendingFetch.aborts[0].signal.aborted, true);
  assert.equal(JSON.stringify(timedOutFetch).includes('private timeout marker'), false);

  let resolveFetch;
  const pendingResponse = makeContext({
    fetch: () => new Promise((resolve) => { resolveFetch = resolve; }),
  });
  const pendingResponsePromise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, pendingResponse.context, { timeout: 1_000 });
  await Promise.resolve();
  pendingResponse.fireTimeout();
  const lateResult = await pendingResponsePromise;
  const late = responseFor(htmlForJSON(validSelfJSON()));
  resolveFetch(late.response);
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(lateResult.outcome, 'timeout');
  assert.equal(late.state.bodyCancelled, true);
  assert.equal(late.state.reads, 0);

  let markReadStarted;
  const readStarted = new Promise((resolve) => { markReadStarted = resolve; });
  const stalledResponse = {
    status: 200,
    type: 'basic',
    redirected: false,
    url: TARGET_URL,
    headers: { get: () => 'text/html' },
    body: {
      getReader: () => ({
        read() { markReadStarted(); return new Promise(() => {}); },
        cancel() { return Promise.resolve(); },
      }),
    },
  };
  const stalled = makeContext({ fetch: async () => stalledResponse });
  const stalledPromise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, stalled.context, { timeout: 1_000 });
  await readStarted;
  stalled.fireTimeout();
  const stalledResult = await stalledPromise;
  assert.equal(stalledResult.outcome, 'timeout');
  assert.equal(stalled.aborts[0].signal.aborted, true);

  let calls = 0;
  const regressing = await runPageProbe({
    fetch: async () => responseFor('<html></html>').response,
    clock: () => [10, 11, 9][calls++] ?? 9,
  });
  assert.equal(regressing.result.outcome, 'timeout');
});

test('checks deadline immediately after a decode stage crosses two seconds', async () => {
  let now = 0;
  class CrossingDecoder extends TextDecoder {
    decode(...args) {
      const decoded = super.decode(...args);
      now = 2_000;
      return decoded;
    }
  }
  const crossed = await runPageProbe({
    fetch: async () => responseFor(htmlForJSON(validSelfJSON())).response,
    clock: () => now,
    TextDecoderImpl: CrossingDecoder,
  });
  assert.equal(crossed.result.outcome, 'timeout');
  assert.equal(crossed.result.projection, null);
});

test('keeps the closed timeout DTO valid if initializer sizing reaches its overflow sentinel at the deadline', async () => {
  let now = 0;
  class CrossingEncoder extends TextEncoder {
    encode(input = '') {
      const encoded = super.encode(input);
      if (typeof input === 'string' && input.length > INITIALIZER_LIMIT) now = 2_000;
      return encoded;
    }
  }
  const overJSON = fixedByteInitializer(INITIALIZER_LIMIT + 1);
  const harness = makeContext({
    fetch: async () => responseFor(htmlForJSON(overJSON)).response,
    clock: () => now,
  });
  harness.context.TextEncoder = CrossingEncoder;
  const result = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, harness.context, { timeout: 1_000 });
  assert.equal(result.outcome, 'timeout');
  assert.equal(result.initializerBytes, INITIALIZER_LIMIT + 1);
  assert.equal(result.status, 200);
  assert.equal(validateQALiveSelfDiagnostic(result).outcome, 'timeout');
});

test('rejects a location or document change before returning the fetched source', async () => {
  const source = htmlForJSON(validSelfJSON());
  const changedLocation = makeContext({ fetch: async () => {
    changedLocation.location.href = 'https://x.com/messages';
    return responseFor(source).response;
  } });
  const locationResult = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, changedLocation.context, { timeout: 1_000 });
  assert.equal(locationResult.outcome, 'response_rejected');

  const changedDocument = makeContext({ fetch: async () => {
    changedDocument.context.document = {};
    return responseFor(source).response;
  } });
  const documentResult = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, changedDocument.context, { timeout: 1_000 });
  assert.equal(documentResult.outcome, 'response_rejected');
});

test('does not reload, navigate, enable debugger access, or reread the existing bootstrap', async () => {
  let forbiddenReads = 0;
  let fetches = 0;
  const { response } = responseFor('<html><script>window.other = {};</script></html>');
  const { context, location } = makeContext({ fetch: async () => { fetches += 1; return response; } });
  Object.defineProperty(location, 'reload', {
    enumerable: true,
    get() { forbiddenReads += 1; throw new Error('reload must not be read'); },
  });
  for (const key of ['__INITIAL_STATE__', 'Debugger', 'history', 'window', 'DOMParser']) {
    Object.defineProperty(context, key, {
      configurable: true,
      enumerable: true,
      get() { forbiddenReads += 1; throw new Error('forbidden page capability read'); },
    });
  }

  const result = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  assert.equal(result.outcome, 'source_absent');
  assert.equal(fetches, 1);
  assert.equal(forbiddenReads, 0);
});

test('closes v3 reports against raw source fields, extra keys, getters, and unsafe nested projections', async () => {
  const page = await runPageProbe({ fetch: async () => responseFor('<html></html>').response });
  const valid = projectQALiveSelf(page.result);
  assert.equal(validateQALiveSelfReport(valid).schema, 'aku.x.qa-live-self.v3');
  assert.throws(() => validateQALiveSelfReport({ ...valid, sourceJSON: '{"private":true}' }), /qa_live_self_report_invalid/);
  assert.throws(() => validateQALiveSelfReport({ ...valid, privateID: 'must not pass' }), /qa_live_self_report_invalid/);

  const getterReport = { ...valid };
  Object.defineProperty(getterReport, 'projection', { enumerable: true, get() { throw new Error('private'); } });
  assert.throws(() => validateQALiveSelfDiagnostic(getterReport), /qa_live_self_report_invalid/);

  const unsafeNested = structuredClone(valid);
  unsafeNested.projection = {
    schema: 'aku.x.bootstrap-current-self.v1',
    privateID: 'must not pass',
  };
  assert.throws(() => validateQALiveSelfDiagnostic(unsafeNested), /bootstrap_projection_invalid|qa_live_self_report_invalid/);

  const statusMismatch = {
    ...valid,
    outcome: 'body_limit',
    identity: 'unavailable',
    bodyBytes: BODY_LIMIT,
    initializerBytes: 0,
    projection: null,
    freshness: 'unqualified',
  };
  assert.throws(() => validateQALiveSelfDiagnostic(statusMismatch), /qa_live_self_report_invalid/);

  const impossibleLengths = { ...valid, bodyBytes: 1, initializerBytes: 2 };
  assert.throws(() => validateQALiveSelfDiagnostic(impossibleLengths), /qa_live_self_report_invalid/);
});
