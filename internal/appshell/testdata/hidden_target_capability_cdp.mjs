// One opt-in diagnostic against an owned Chrome and static loopback fixture.
// Output is a closed schema of enums/booleans/counts; no URLs or page text.
const [endpoint, fixture] = process.argv.slice(2);
const socketURL = new URL(endpoint);
const fixtureURL = new URL(fixture);
if (socketURL.protocol !== 'ws:' || socketURL.hostname !== '127.0.0.1' ||
    fixtureURL.protocol !== 'http:' || fixtureURL.hostname !== '127.0.0.1' ||
    !fixtureURL.port || fixtureURL.port === '11122' || fixtureURL.pathname !== '/') {
  throw new Error('Only an isolated random-port loopback fixture is permitted.');
}
const hostURL = `${fixtureURL.origin}/host`;
const hiddenURL = `${fixtureURL.origin}/hidden`;
const forcedURL = `${hostURL}?force=1`;
const result = {
  probeState: 'starting', chromeVersion: 'unverified', anyExtensionWorker: false,
  initial: null, hiddenCapability: 'unverified', hidden: null,
  hiddenNativeWindow: 'unverified', forcedHost: null,
  defaultContextCookieShared: false, hiddenDisposed: false,
  childSessionDetached: false, failureClass: 'none',
};
const allowed = new Set(['Browser.getVersion', 'Target.getTargets',
  'Target.attachToTarget', 'Target.attachToBrowserTarget', 'Target.detachFromTarget',
  'Target.createTarget', 'Target.closeTarget', 'Browser.getWindowForTarget',
  'Page.enable', 'Runtime.enable', 'Network.enable', 'Page.getFrameTree',
  'Page.getNavigationHistory', 'Runtime.evaluate', 'Page.navigate']);
const socket = new WebSocket(endpoint);
const pending = new Map();
let nextID = 1;
let childSession = '';
let hiddenTarget = '';
let hostSession = '';
let hiddenSession = '';
const deadline = Date.now() + 24000;
const pause = ms => new Promise(resolve => setTimeout(resolve, ms));
const classify = raw => {
  if (typeof raw !== 'string' || raw === '') return 'empty';
  if (raw === 'about:blank' || raw.startsWith('about:blank#')) return 'about_blank';
  if (raw.startsWith('chrome-error:')) return 'chrome_error';
  if (/^(chrome|chrome-untrusted|devtools):/.test(raw)) return 'chrome_internal';
  try {
    const url = new URL(raw);
    if (url.origin === fixtureURL.origin && url.pathname === '/host') return 'expected_host';
    if (url.origin === fixtureURL.origin && url.pathname === '/hidden') return 'expected_hidden';
  } catch { return 'invalid'; }
  return 'other';
};
const errorClass = error => error?.kind ?? 'command_error';
const send = (method, params = {}, sessionId = '') => new Promise((resolve, reject) => {
  if (!allowed.has(method) || Date.now() >= deadline) return reject({kind: 'deadline'});
  const id = nextID++;
  const timer = setTimeout(() => {
    pending.delete(id);
    reject({kind: 'command_timeout'});
  }, 1000);
  pending.set(id, {resolve, reject, timer});
  socket.send(JSON.stringify({id, method, params, ...(sessionId ? {sessionId} : {})}));
});
socket.addEventListener('message', ({data}) => {
  let message;
  try { message = JSON.parse(data); } catch { return; }
  const item = pending.get(message.id);
  if (!item) return;
  clearTimeout(item.timer);
  pending.delete(message.id);
  if (message.error) {
    const text = String(message.error.message ?? '').toLowerCase();
    const reason = text.includes('browser window not found') ? 'browser_window_not_found'
      : text.includes('no web contents') ? 'no_web_contents'
      : message.error.code === -32601 ? 'method_unsupported' : 'other';
    item.reject({kind: 'protocol_error', code: message.error.code, reason});
  }
  else item.resolve(message.result);
});
const connect = new Promise((resolve, reject) => {
  const timer = setTimeout(() => reject({kind: 'connection_timeout'}), 1500);
  socket.addEventListener('open', () => { clearTimeout(timer); resolve(); }, {once: true});
  socket.addEventListener('error', () => { clearTimeout(timer); reject({kind: 'connection_error'}); }, {once: true});
});
async function documentSnapshot(session) {
  const expression = `(() => {
    const classify = ${classify.toString().replaceAll('fixtureURL.origin', JSON.stringify(fixtureURL.origin))};
    const text = document.documentElement?.innerText ?? '';
    const error = /ERR_BLOCKED_BY_CLIENT/i.test(text) ? 'blocked_by_client'
      : /ERR_BLOCKED_BY_ADMINISTRATOR/i.test(text) ? 'blocked_by_administrator'
      : /ERR_ACCESS_DENIED/i.test(text) ? 'access_denied'
      : /ERR_PROXY_CONNECTION_FAILED/i.test(text) ? 'proxy_connection_failed'
      : /ERR_CONNECTION_REFUSED/i.test(text) ? 'connection_refused'
      : /ERR_NAME_NOT_RESOLVED|DNS_PROBE/i.test(text) ? 'dns'
      : /ERR_TIMED_OUT/i.test(text) ? 'timeout'
      : /ERR_ADDRESS_UNREACHABLE/i.test(text) ? 'unreachable' : 'none';
    let fixtureCookie = false;
    let cookieRead = 'readable';
    try {
      fixtureCookie = document.cookie.split(';').some(item => item.trim() === 'aku_smoke_fixture=isolated');
    } catch (failure) {
      cookieRead = failure?.name === 'SecurityError' ? 'security_error' : 'other_error';
    }
    return {
      locationClass: classify(location.href), documentURLClass: classify(document.URL),
      baseURIClass: classify(document.baseURI), readyState: document.readyState,
      titleClass: document.title === 'AkuBrowser capture host' ? 'host'
        : document.title === 'Hidden collector fixture' ? 'hidden'
        : document.title === '' ? 'empty' : 'other',
      hostMarker: Boolean(document.querySelector('[data-aku-smoke="host"]')),
      hiddenMarker: Boolean(document.querySelector('[data-aku-smoke="hidden"]')),
      fixtureCookie, cookieRead,
      bodyPresent: Boolean(document.body), bodyTextPresent: text.length > 0, errorClass: error,
    };
  })()`;
  const evaluated = await send('Runtime.evaluate', {expression, returnByValue: true, timeout: 700}, session);
  if (evaluated.exceptionDetails || !evaluated.result?.value) {
    const exception = evaluated.exceptionDetails?.exception?.className;
    return {evaluation: 'failed', exceptionClass: ['SecurityError', 'TypeError', 'ReferenceError', 'Error', 'DOMException'].includes(exception) ? exception : 'other'};
  }
  return evaluated.result.value;
}
async function snapshot(session, targetClass) {
  const tree = await send('Page.getFrameTree', {}, session);
  const frame = tree.frameTree?.frame;
  const history = await send('Page.getNavigationHistory', {}, session);
  return {
    targetClass, frameClass: classify(frame?.url), unreachableClass: classify(frame?.unreachableUrl),
    historyCurrentIndex: history.currentIndex,
    historyClasses: (history.entries ?? []).slice(-16).map(entry => classify(entry.url)),
    document: await documentSnapshot(session),
  };
}
async function enable(session) {
  await send('Page.enable', {}, session);
  await send('Runtime.enable', {}, session);
  await send('Network.enable', {}, session);
}
async function waitDocument(session, marker, duration = 2500) {
  const until = Date.now() + duration;
  let document;
  do {
    document = await documentSnapshot(session);
    if (document[marker] && document.readyState === 'complete') break;
    await pause(100);
  } while (Date.now() < until && Date.now() < deadline - 3000);
  return document;
}
try {
  await connect;
  const version = await send('Browser.getVersion');
  result.chromeVersion = /^(Chrome|HeadlessChrome)\/\d+(\.\d+){3}$/.test(version.product ?? '') ? version.product : 'other';
  const targets = (await send('Target.getTargets')).targetInfos ?? [];
  // This is not packaged Bridge identity or authenticated bootstrap evidence.
  result.anyExtensionWorker = targets.some(target => target.type === 'service_worker' && target.url?.startsWith('chrome-extension://'));
  const host = targets.find(target => target.type === 'page' && classify(target.url) === 'expected_host');
  if (!host) throw {kind: 'host_target_missing'};
  hostSession = (await send('Target.attachToTarget', {targetId: host.targetId, flatten: true})).sessionId;
  await enable(hostSession);
  await waitDocument(hostSession, 'hostMarker', 1000);
  result.initial = await snapshot(hostSession, classify(host.url));

  childSession = (await send('Target.attachToBrowserTarget')).sessionId;
  try {
    // Omit newWindow and browserContextId: default authenticated context,
    // session-owned target, no tab strip or newly requested native window.
    hiddenTarget = (await send('Target.createTarget', {
      url: hiddenURL, hidden: true, background: true, forTab: false,
    }, childSession)).targetId;
    result.hiddenCapability = hiddenTarget ? 'supported' : 'unverified';
  } catch (error) {
    result.hiddenCapability = error.kind === 'protocol_error' && error.code === -32602
      ? 'unsupported_or_invalid_parameters' : 'refused_or_unverified';
    result.failureClass = errorClass(error);
  }
  if (hiddenTarget) {
    hiddenSession = (await send('Target.attachToTarget', {targetId: hiddenTarget, flatten: true}, childSession)).sessionId;
    await enable(hiddenSession);
    await waitDocument(hiddenSession, 'hiddenMarker');
    result.hidden = await snapshot(hiddenSession, 'expected_hidden');
    try {
      await send('Browser.getWindowForTarget', {targetId: hiddenTarget});
      result.hiddenNativeWindow = 'present';
    } catch (error) {
      result.hiddenNativeWindow = error.kind === 'protocol_error' ? 'no_window_protocol_error' : 'unverified';
      result.hiddenWindowError = error.kind === 'protocol_error' ? error.reason : 'unverified';
    }
  }

  // One causal second stage, after preserving the untouched startup snapshot.
  const navigation = await send('Page.navigate', {url: forcedURL}, hostSession);
  result.forcedNavigationError = navigation.errorText ? 'navigation_error' : 'none';
  await waitDocument(hostSession, 'hostMarker');
  result.forcedHost = await snapshot(hostSession, 'expected_host');
  if (hiddenSession) {
    const hiddenAfterHost = await documentSnapshot(hiddenSession);
    result.hiddenCookieAfterHost = hiddenAfterHost.fixtureCookie === true;
    result.defaultContextCookieShared = result.forcedHost.document?.hostMarker === true &&
      result.forcedHost.document.fixtureCookie === true && hiddenAfterHost.hiddenMarker === true &&
      hiddenAfterHost.fixtureCookie === true;
  }
  result.probeState = 'complete';
} catch (error) {
  result.probeState = 'partial';
  result.failureClass = errorClass(error);
} finally {
  if (hiddenTarget) {
    await send('Target.closeTarget', {targetId: hiddenTarget}, childSession).catch(() => {});
  }
  if (childSession) {
    result.childSessionDetached = await send('Target.detachFromTarget', {sessionId: childSession})
      .then(() => true, () => false);
  }
  if (hiddenTarget) {
    result.hiddenDisposed = await send('Target.getTargets')
      .then(value => !(value.targetInfos ?? []).some(target => target.targetId === hiddenTarget), () => false);
  }
  if (hostSession) await send('Target.detachFromTarget', {sessionId: hostSession}).catch(() => {});
  for (const item of pending.values()) clearTimeout(item.timer);
  pending.clear();
  // This diagnostic uses a WebSocket, never the headed root pipe.
  socket.close();
  console.log(JSON.stringify(result));
}
