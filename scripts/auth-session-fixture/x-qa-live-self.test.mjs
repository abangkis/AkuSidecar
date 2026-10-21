import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';

import {
  QA_LIVE_SELF_EXPRESSION,
  projectQALiveSelf,
  validateQALiveSelfDiagnostic,
  validateQALiveSelfReport,
} from './x-qa-live-self.mjs';

const TARGET_URL = 'https://x.com/home';

function asBytes(value) {
  return new TextEncoder().encode(value);
}

function responseFor(html, { status = 200, type = 'basic', redirected = false, url = TARGET_URL, contentType = 'text/html; charset=utf-8', chunks } = {}) {
  const sourceChunks = chunks ?? [asBytes(html)];
  const state = {
    index: 0, offset: 0, cancelled: false, bodyCancelled: false, reads: 0,
    viewLengths: [], detachedViews: [],
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
        return {
          async read(view) {
            state.reads += 1;
            assert.ok(view instanceof Uint8Array);
            state.viewLengths.push(view.byteLength);
            while (state.index < sourceChunks.length) {
              const source = sourceChunks[state.index];
              const remaining = source.byteLength - state.offset;
              if (remaining === 0) {
                state.index += 1;
                state.offset = 0;
                const empty = new Uint8Array(0);
                structuredClone(view.buffer, { transfer: [view.buffer] });
                state.detachedViews.push(view.byteLength === 0);
                return { done: false, value: empty };
              }
              const count = Math.min(view.byteLength, remaining);
              const output = new Uint8Array(count);
              output.set(source.subarray(state.offset, state.offset + count));
              state.offset += count;
              if (state.offset === source.byteLength) {
                state.index += 1;
                state.offset = 0;
              }
              structuredClone(view.buffer, { transfer: [view.buffer] });
              state.detachedViews.push(view.byteLength === 0);
              return { done: false, value: output };
            }
            const empty = new Uint8Array(0);
            structuredClone(view.buffer, { transfer: [view.buffer] });
            state.detachedViews.push(view.byteLength === 0);
            return { done: true, value: empty };
          },
          cancel() { state.cancelled = true; return Promise.resolve(); },
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

function fakeTemplateDocument() {
  const state = { created: 0, assigned: 0, inserted: 0 };
  const document = {
    createElement(tagName) {
      assert.equal(tagName, 'template');
      state.created += 1;
      const template = { content: null, isConnected: false };
      Object.defineProperty(template, 'innerHTML', {
        configurable: true,
        enumerable: true,
        set(html) {
          state.assigned += 1;
          template.content = { querySelectorAll: (selector) => extractScripts(html, selector) };
        },
      });
      return template;
    },
  };
  return { document, state };
}

function makeContext({ fetch, timeoutMode = 'real', locationHref = TARGET_URL, document } = {}) {
  let timeoutCallback = null;
  let timeoutId = 0;
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
    TextDecoder,
    Uint8Array,
    AbortController: TestAbortController,
    fetch,
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
  const promise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  const result = await promise;
  return { result, ...state };
}

function validSelfJSON({ session = {}, includeNormalized = true, extra = {} } = {}) {
  const id = '12345678901234567890';
  const state = {
    session: { user_id_str: id, ...session },
    ...extra,
  };
  if (includeNormalized) {
    state.entities = { users: { entities: { [id]: { id_str: id } } } };
  }
  return JSON.stringify(state);
}

function htmlForJSON(sourceJSON) {
  return `<html><body><script>window.__INITIAL_STATE__ = ${sourceJSON};</script></body></html>`;
}

test('uses one fixed same-origin GET, streams a bounded body, and returns only the initializer JSON', async () => {
  const sourceJSON = validSelfJSON();
  const html = `<img src="https://images.invalid/track"><iframe src="https://frames.invalid/embed"></iframe>${htmlForJSON(sourceJSON)}`;
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
  assert.ok(state.viewLengths.every((length) => length > 0 && length <= 4_096));
  assert.ok(state.detachedViews.every(Boolean));
  assert.equal(harness.templateState.created, 1);
  assert.equal(harness.templateState.assigned, 1);
  assert.equal(harness.templateState.inserted, 0);
  assert.deepEqual(JSON.parse(JSON.stringify(result)), {
    schema: 'aku.x.qa-live-self-page.v1',
    category: 'fetched',
    status: 200,
    bytesRead: Buffer.byteLength(html),
    sourceJSON,
  });
});

test('projects a strong current-self entity match as a candidate without granting authority', async () => {
  const sourceJSON = validSelfJSON();
  const { response } = responseFor(htmlForJSON(sourceJSON));
  const { result } = await runPageProbe({ fetch: async () => response });
  const diagnostic = projectQALiveSelf(result);

  assert.equal(diagnostic.outcome, 'candidate');
  assert.equal(diagnostic.identity, 'candidate');
  assert.equal(diagnostic.freshness, 'server-response-candidate');
  assert.equal(diagnostic.projection.entityChecks.normalizedUsers, 'match');
  assert.equal(diagnostic.canAuthorize, false);
  assert.equal(diagnostic.nativeQualified, false);
  assert.equal(diagnostic.sessionTransferred, false);
  assert.equal(JSON.stringify(diagnostic).includes('12345678901234567890'), false);
  assert.deepEqual(validateQALiveSelfDiagnostic(diagnostic), diagnostic);
});

test('returns source_absent for a missing initializer and source_ambiguous for duplicate markers', async () => {
  const noMarker = responseFor('<html><script>window.other = {};</script></html>');
  const absent = await runPageProbe({ fetch: async () => noMarker.response });
  assert.equal(absent.result.category, 'source_absent');
  assert.equal(absent.result.sourceJSON, null);
  assert.equal(projectQALiveSelf(absent.result).identity, 'unavailable');

  const duplicate = responseFor(
    `<script>window.__INITIAL_STATE__ = {"session":{}};</script><script>window.__INITIAL_STATE__ = {"session":{}};</script>`,
  );
  const ambiguous = await runPageProbe({ fetch: async () => duplicate.response });
  assert.equal(ambiguous.result.category, 'source_ambiguous');
  assert.equal(ambiguous.result.sourceJSON, null);
});

test('rejects malformed initializer JSON and strict-parser duplicate object keys without leaking source', async () => {
  const malformed = responseFor(htmlForJSON('{broken'));
  const malformedResult = await runPageProbe({ fetch: async () => malformed.response });
  assert.equal(malformedResult.result.category, 'source_absent');
  assert.equal(malformedResult.result.sourceJSON, null);

  const duplicateKey = responseFor(htmlForJSON('{"session":{},"session":{}}'));
  const duplicateResult = await runPageProbe({ fetch: async () => duplicateKey.response });
  assert.equal(duplicateResult.result.category, 'fetched');
  const diagnostic = projectQALiveSelf(duplicateResult.result);
  assert.equal(diagnostic.outcome, 'projection_invalid');
  assert.equal(diagnostic.projection, null);
  assert.equal(JSON.stringify(diagnostic).includes('"session":{}'), false);
});

test('rejects numeric IDs, auth-false evidence, conflicting entities, and feed-author-only identity', async () => {
  const cases = [
    {
      name: 'numeric ID',
      value: { session: { user_id: 123456789 }, entities: { users: { entities: {} } } },
    },
    {
      name: 'auth false',
      value: JSON.parse(validSelfJSON({ session: { loggedIn: false } })),
    },
    {
      name: 'entity mismatch',
      value: JSON.parse(validSelfJSON()),
      mutate(value) { value.entities.users.entities['12345678901234567890'].id_str = '999'; },
    },
    {
      name: 'feed author only',
      value: {
        session: {},
        feed: { entries: [{ author_id: '12345678901234567890' }] },
      },
    },
  ];

  for (const item of cases) {
    const value = structuredClone(item.value);
    item.mutate?.(value);
    const { response } = responseFor(htmlForJSON(JSON.stringify(value)));
    const pageResult = await runPageProbe({ fetch: async () => response });
    const diagnostic = projectQALiveSelf(pageResult.result);
    assert.equal(diagnostic.outcome, 'self_unqualified', item.name);
    assert.equal(diagnostic.identity, 'unavailable', item.name);
  }
});

test('rejects non-home, redirected, non-HTML, and non-200 responses without reading their body', async () => {
  const cases = [
    { name: 'wrong page', locationHref: 'https://x.com/home?tab=1', response: responseFor('<script></script>') },
    { name: 'redirect', response: responseFor('<script></script>', { redirected: true }) },
    { name: 'non-HTML', response: responseFor('<script></script>', { contentType: 'application/json' }) },
    { name: 'non-200', response: responseFor('<script></script>', { status: 503 }) },
  ];

  for (const item of cases) {
    const run = await runPageProbe({
      locationHref: item.locationHref,
      fetch: async () => item.response.response,
    });
    const { result } = run;
    assert.equal(result.category, 'response_rejected', item.name);
    assert.equal(result.sourceJSON, null, item.name);
    assert.equal(item.response.state.reads, 0, item.name);
    if (item.name !== 'wrong page') {
      assert.equal(item.response.state.bodyCancelled, true, item.name);
      assert.equal(run.aborts[0].signal.aborted, true, item.name);
    }
  }
});

test('aborts and cancels when the streamed response exceeds 16 KiB', async () => {
  const large = new Uint8Array(16_385);
  const { response, state } = responseFor('', { chunks: [large] });
  const { result, aborts } = await runPageProbe({ fetch: async () => response });

  assert.equal(result.category, 'body_limit');
  assert.equal(result.bytesRead, 16_385);
  assert.equal(result.sourceJSON, null);
  assert.equal(state.cancelled, true);
  assert.ok(state.viewLengths.every((length) => length > 0 && length <= 4_096));
  assert.ok(state.detachedViews.every(Boolean));
  assert.deepEqual(state.viewLengths, [4_096, 4_096, 4_096, 4_096, 1]);
  assert.equal(aborts[0].signal.aborted, true);
});

test('fails closed without BYOB support and bounds nonempty and zero-length read loops', async () => {
  let getReaderCalls = 0;
  let bodyCancelled = false;
  const unsupported = {
    status: 200,
    type: 'basic',
    redirected: false,
    url: TARGET_URL,
    headers: { get: () => 'text/html' },
    body: {
      cancel() { bodyCancelled = true; return Promise.resolve(); },
      getReader(options) {
        getReaderCalls += 1;
        assert.equal(options?.mode, 'byob');
        assert.deepEqual(Object.keys(options).sort(), ['mode']);
        throw new Error('BYOB unavailable');
      },
    },
  };
  const unsupportedResult = await runPageProbe({ fetch: async () => unsupported });
  assert.equal(unsupportedResult.result.category, 'fetch_failed');
  assert.equal(getReaderCalls, 1);
  assert.equal(bodyCancelled, true);
  assert.equal(unsupportedResult.aborts[0].signal.aborted, true);

  for (const [name, chunks] of [
    ['nonempty', Array.from({ length: 257 }, () => new Uint8Array([1]))],
    ['zero-length', Array.from({ length: 257 }, () => new Uint8Array(0))],
  ]) {
    const { response, state } = responseFor('', { chunks });
    const run = await runPageProbe({ fetch: async () => response });
    assert.equal(run.result.category, 'fetch_failed', name);
    assert.equal(run.result.sourceJSON, null, name);
    assert.equal(state.reads, 257, name);
    assert.equal(state.cancelled, true, name);
    assert.equal(run.aborts[0].signal.aborted, true, name);
  }
});

test('enforces the two-second timeout and aborts a pending request', async () => {
  let rejectFetch;
  const { context, aborts, fireTimeout } = makeContext({
    timeoutMode: 'manual',
    fetch: () => new Promise((_resolve, reject) => { rejectFetch = reject; }),
  });
  const promise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  await Promise.resolve();
  fireTimeout();
  const result = await promise;
  rejectFetch?.(new Error('private timeout marker'));

  assert.equal(result.category, 'timeout');
  assert.equal(result.sourceJSON, null);
  assert.equal(aborts[0].signal.aborted, true);
  assert.equal(JSON.stringify(result).includes('private timeout marker'), false);
});

test('cancels a response body that arrives after the timeout has already completed', async () => {
  let resolveFetch;
  const { context, aborts, fireTimeout } = makeContext({
    timeoutMode: 'manual',
    fetch: () => new Promise((resolve) => { resolveFetch = resolve; }),
  });
  const promise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  await Promise.resolve();
  fireTimeout();
  const result = await promise;
  const late = responseFor(htmlForJSON(validSelfJSON()));
  resolveFetch(late.response);
  await new Promise((resolve) => setImmediate(resolve));

  assert.equal(result.category, 'timeout');
  assert.equal(aborts[0].signal.aborted, true);
  assert.equal(late.state.bodyCancelled, true);
  assert.equal(late.state.reads, 0);
});

test('cancels and aborts a stream that stalls before the same two-second deadline', async () => {
  let markReadStarted;
  const readStarted = new Promise((resolve) => { markReadStarted = resolve; });
  let cancelled = false;
  const response = {
    status: 200,
    type: 'basic',
    redirected: false,
    url: TARGET_URL,
    headers: { get: () => 'text/html' },
    body: {
      getReader: () => ({
        read() { markReadStarted(); return new Promise(() => {}); },
        cancel() { cancelled = true; return Promise.resolve(); },
      }),
    },
  };
  const { context, aborts, fireTimeout } = makeContext({ timeoutMode: 'manual', fetch: async () => response });
  const promise = vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  await readStarted;
  fireTimeout();
  const result = await promise;

  assert.equal(result.category, 'timeout');
  assert.equal(result.bytesRead, 0);
  assert.equal(aborts[0].signal.aborted, true);
  assert.equal(cancelled, true);
});

test('rejects a location or document change before returning a source', async () => {
  const sourceJSON = validSelfJSON();
  const { response } = responseFor(htmlForJSON(sourceJSON));
  const changedLocation = makeContext({ fetch: async () => {
    changedLocation.location.href = 'https://x.com/messages';
    return response;
  } });
  const locationResult = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, changedLocation.context, { timeout: 1_000 });
  assert.equal(locationResult.category, 'response_rejected');
  assert.equal(locationResult.sourceJSON, null);

  const changedDocument = makeContext({ fetch: async () => {
    changedDocument.context.document = {};
    return response;
  } });
  const documentResult = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, changedDocument.context, { timeout: 1_000 });
  assert.equal(documentResult.category, 'response_rejected');
  assert.equal(documentResult.sourceJSON, null);
});

test('does not reload, navigate, enable debugger access, or reread the page bootstrap', async () => {
  let forbiddenReads = 0;
  let fetches = 0;
  const { response } = responseFor('<html><script>window.other = {};</script></html>');
  const { context, location } = makeContext({ fetch: async () => { fetches += 1; return response; } });
  Object.defineProperty(location, 'reload', {
    enumerable: true,
    get() { forbiddenReads += 1; throw new Error('reload must not be read'); },
  });
  for (const key of ['__INITIAL_STATE__', 'Debugger', 'history', 'window']) {
    Object.defineProperty(context, key, {
      configurable: true,
      enumerable: true,
      get() { forbiddenReads += 1; throw new Error('forbidden page capability read'); },
    });
  }

  const result = await vm.runInNewContext(QA_LIVE_SELF_EXPRESSION, context, { timeout: 1_000 });
  assert.equal(result.category, 'source_absent');
  assert.equal(fetches, 1);
  assert.equal(forbiddenReads, 0);
});

test('closes page reports and diagnostics against extra fields, accessors, and unsafe projections', () => {
  const validPageReport = {
    schema: 'aku.x.qa-live-self-page.v1',
    category: 'source_absent',
    status: 200,
    bytesRead: 12,
    sourceJSON: null,
  };
  assert.deepEqual(validateQALiveSelfReport(validPageReport), validPageReport);
  assert.throws(() => validateQALiveSelfReport({ ...validPageReport, privateID: 'must not pass' }), /qa_live_self_report_invalid/);

  const getterReport = { ...validPageReport };
  Object.defineProperty(getterReport, 'sourceJSON', { enumerable: true, get() { throw new Error('private'); } });
  assert.throws(() => validateQALiveSelfReport(getterReport), /qa_live_self_report_invalid/);

  const unsafeProjection = {
    schema: 'aku.x.qa-live-self.v1',
    outcome: 'source_absent',
    identity: 'unavailable',
    bodyBytes: 12,
    status: 200,
    projection: { schema: 'aku.x.bootstrap-current-self.v1', privateID: 'must not pass' },
    freshness: 'unqualified',
    canAuthorize: false,
    nativeQualified: false,
    sessionTransferred: false,
  };
  assert.throws(() => validateQALiveSelfDiagnostic(unsafeProjection), /bootstrap_projection_invalid|qa_live_self_report_invalid/);
});
