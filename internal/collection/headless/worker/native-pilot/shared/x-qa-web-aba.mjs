// The serialized hook executes only inside the owned QA page. Application
// headers stay in its closure; the private bridge receives only closed DTOs.
export function installQAWebHook() {
  const key = '__akuQaWebABA_v1';
  if (location.origin !== 'https://x.com' || location.pathname !== '/home' || Object.hasOwn(globalThis, key)) return;
  const originalFetch = globalThis.fetch, NativeRequest = globalThis.Request, NativeHeaders = globalThis.Headers;
  const proto = globalThis.XMLHttpRequest?.prototype;
  const originalOpen = proto?.open, originalSet = proto?.setRequestHeader, originalSend = proto?.send;
  const records = new WeakMap();
  let request = null, candidates = 0, transport = 'none', outcome = 'not_run', attempted = false, disposed = false, rejected = false;
  const selected = (url, method) => {
    try { const u = new URL(url, location.href); return String(method).toUpperCase() === 'GET' && !u.username && !u.password && !u.hash &&
      u.origin === 'https://api.x.com' && u.pathname === '/1.1/account/settings.json'; } catch { return false; }
  };
  const keep = (value, type) => {
    candidates = Math.min(2, candidates + 1);
    if (candidates !== 1 || disposed || attempted) { rejected = true; request = null; return; }
    transport = type;
    try { request = value(); } catch { rejected = true; request = null; }
  };
  const wrappedFetch = function(input, init) {
    const url = input instanceof NativeRequest ? input.url : typeof input === 'string' ? input : null;
    const method = init?.method ?? (input instanceof NativeRequest ? input.method : 'GET');
    if (url !== null && selected(url, method)) keep(() => new NativeRequest(input, init), 'fetch');
    return Reflect.apply(originalFetch, this, arguments);
  };
  const wrappedOpen = function(method, url) {
    records.delete(this);
    if (typeof url === 'string' && selected(url, method)) records.set(this, { url, headers: [], bytes: 0, rejected: false });
    return Reflect.apply(originalOpen, this, arguments);
  };
  const wrappedSet = function(name, value) {
    const record = records.get(this);
    if (record) {
      if (typeof name !== 'string' || typeof value !== 'string' || record.headers.length >= 32 ||
          (record.bytes += name.length + value.length) > 8192 || ['cookie', 'set-cookie'].includes(name.toLowerCase())) record.rejected = true;
      else record.headers.push([name, value]);
    }
    return Reflect.apply(originalSet, this, arguments);
  };
  const wrappedSend = function() {
    const record = records.get(this);
    if (record) {
      keep(() => {
        if (record.rejected || arguments[0] != null) throw new Error();
        return new NativeRequest(record.url, { method: 'GET', headers: new NativeHeaders(record.headers), mode: 'cors' });
      }, 'xhr');
      // Discard the temporary per-XHR header list immediately after snapshot.
      records.delete(this);
    }
    return Reflect.apply(originalSend, this, arguments);
  };
  globalThis.fetch = wrappedFetch;
  if (proto) { proto.open = wrappedOpen; proto.setRequestHeader = wrappedSet; proto.send = wrappedSend; }
  const snapshot = () => ({ schema: 'aku.x.qa-web-hook.v1', candidates, transport, prepared: request !== null,
    attempted, outcome, disposed, rejected });
  const start = () => {
    if (disposed || attempted || rejected || candidates !== 1 || !request) return false;
    attempted = true; outcome = 'started';
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 4000);
    let copy;
    try { copy = new NativeRequest(request, { credentials: 'omit', cache: 'no-store', redirect: 'error', signal: controller.signal }); }
    catch { clearTimeout(timer); request = null; outcome = 'rejected'; return false; }
    request = null;
    // Never return B's response/promise to the application. B's response cookies
    // are ignored by credentials:omit; no application logout callback runs.
    Promise.resolve(Reflect.apply(originalFetch, globalThis, [copy])).then(async response => {
      const reader = response.body?.getReader();
      let bytes = 0;
      if (reader) {
        for (;;) {
          const part = await reader.read();
          if (part.done) break;
          bytes += part.value.byteLength;
          if (bytes > 16384) { controller.abort(); await reader.cancel(); outcome = 'body_limit'; return; }
        }
      }
      outcome = 'completed';
    }, () => { outcome = 'network_error'; }).catch(() => { outcome = 'network_error'; }).finally(() => clearTimeout(timer));
    return true;
  };
  const dispose = () => {
    if (globalThis.fetch === wrappedFetch) globalThis.fetch = originalFetch;
    if (proto) {
      if (proto.open === wrappedOpen) proto.open = originalOpen;
      if (proto.setRequestHeader === wrappedSet) proto.setRequestHeader = originalSet;
      if (proto.send === wrappedSend) proto.send = originalSend;
    }
    request = null; disposed = true; return true;
  };
  Object.defineProperty(globalThis, key, { value: Object.freeze({ snapshot, start, dispose }), configurable: true });
}

export const QA_WEB_INSTALL = `(${installQAWebHook.toString()})()`;
export const QA_WEB_SNAPSHOT = "globalThis.__akuQaWebABA_v1?.snapshot() ?? null";
export const QA_WEB_START = "globalThis.__akuQaWebABA_v1?.start() === true";
export const QA_WEB_DISPOSE = "globalThis.__akuQaWebABA_v1?.dispose() === true";

export function qaWebCommand(method, params, state) {
  if (!state?.running) return false;
  const exact = expected => JSON.stringify(params) === JSON.stringify(expected);
  switch (method) {
    case 'Page.addScriptToEvaluateOnNewDocument': return !state.scriptId && exact({ source: QA_WEB_INSTALL });
    case 'Page.removeScriptToEvaluateOnNewDocument': return !!state.scriptId && exact({ identifier: state.scriptId });
    case 'Page.getFrameTree': case 'Network.disable': return exact({});
    case 'Network.setCacheDisabled': return exact({ cacheDisabled: true });
    case 'Network.enable': return exact({ maxTotalBufferSize: 1000000, maxResourceBufferSize: 100000, maxPostDataSize: 0 });
    case 'Network.getResponseBody': return Object.keys(params).join(',') === 'requestId' && state.capture?.consumeRead(params.requestId) === true;
    case 'Page.reload': return ['a', 'a_again'].includes(state.stage) && !state.reloaded && exact({ ignoreCache: false });
    case 'Runtime.evaluate': return [QA_WEB_SNAPSHOT, QA_WEB_DISPOSE, ...(state.stage === 'b' && !state.bStarted ? [QA_WEB_START] : [])].includes(params.expression) &&
      exact({ expression: params.expression, returnByValue: true, awaitPromise: false, userGesture: false });
    default: return false;
  }
}

export function validateQAWebHook(value) {
  const keys = ['schema', 'candidates', 'transport', 'prepared', 'attempted', 'outcome', 'disposed', 'rejected'];
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).length !== keys.length ||
      Object.keys(value).some(k => !keys.includes(k)) || value.schema !== 'aku.x.qa-web-hook.v1' ||
      !Number.isInteger(value.candidates) || value.candidates < 0 || value.candidates > 2 || !['none', 'fetch', 'xhr'].includes(value.transport) ||
      !['not_run', 'started', 'completed', 'network_error', 'body_limit', 'rejected'].includes(value.outcome) ||
      ['prepared', 'attempted', 'disposed', 'rejected'].some(k => typeof value[k] !== 'boolean')) throw new TypeError('qa_web_hook_invalid');
  return Object.freeze(Object.fromEntries(keys.map(k => [k, value[k]])));
}

export function finishQAWebABA(a, b, aAgain, hook, before, after, removed) {
  const positive = r => r?.status === 200 && r.responseFresh === true && r.completed === true &&
    r.diagnostic === 'http_200_unqualified' && r.body?.category === 'settings_match';
  const ready = before && after && positive(a) && positive(aAgain);
  const negative = b?.status === 401 && b.responseFresh === true && b.completed === true &&
    b.diagnostic === 'authentication_rejection_candidate' && b.body?.category === 'error_envelope';
  const outcome = !ready ? 'positive_control_failed' : !b ? 'comparison_not_run' : negative ? 'cookie_dependency_candidate' :
    positive(b) ? 'settings_without_cookie' : 'comparison_inconclusive';
  return Object.freeze({ schema: 'aku.x.qa-web-aba.v1', a, b, aAgain, hook: validateQAWebHook(hook),
    ownerHomeBefore: before, ownerHomeAfter: after, hookRemoved: removed, outcome,
    authentication: 'unknown', canAuthorize: false, sessionTransferred: false });
}
