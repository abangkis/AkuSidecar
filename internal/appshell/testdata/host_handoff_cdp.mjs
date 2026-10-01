// Opt-in smoke helper: one command against an isolated loopback Chrome only.
const [endpoint, method, encoded] = process.argv.slice(2);
const url = new URL(endpoint);
if (url.protocol !== 'ws:' || url.hostname !== '127.0.0.1' ||
    !new Set(['Target.getTargets', 'Target.createTarget', 'Target.closeTarget',
      'Browser.getWindowForTarget', 'Browser.getVersion']).has(method)) {
  if (url.protocol !== 'ws:' || url.hostname !== '127.0.0.1' || method !== 'CaptureHostStartup') {
    throw new Error('Only isolated loopback smoke commands are permitted.');
  }
  const params = JSON.parse(encoded);
  if (!/^chrome-extension:\/\/[a-p]{32}$/.test(params.expectedExtensionOrigin ?? '')) {
    throw new Error('Invalid diagnostic extension origin.');
  }
  await captureHostStartup(endpoint, params.expectedExtensionOrigin);
  process.exit(0);
}
const params = JSON.parse(encoded);
if (method === 'Target.createTarget' &&
    (params.background !== true || params.windowState !== 'minimized')) {
  throw new Error('Smoke windows must start minimized in the background.');
}
const ws = new WebSocket(endpoint);
const timer = setTimeout(() => {
  console.error('Isolated CDP smoke command timed out.');
  process.exitCode = 1;
  ws.close();
}, 10000);
ws.addEventListener('open', () => ws.send(JSON.stringify({id: 1, method, params})));
ws.addEventListener('message', ({data}) => {
  const message = JSON.parse(data);
  if (message.id !== 1) return;
  clearTimeout(timer);
  if (message.error) {
    console.error(JSON.stringify(message.error));
    process.exitCode = 1;
  } else {
    console.log(JSON.stringify(message.result));
  }
  ws.close();
});
ws.addEventListener('error', (event) => {
  clearTimeout(timer);
  console.error('Isolated CDP smoke connection failed:', event.error?.message ?? event.message ?? 'unknown');
  process.exitCode = 1;
});

async function captureHostStartup(endpoint, expectedExtensionOrigin) {
  const socket = new WebSocket(endpoint);
  const pending = new Map();
  const contexts = [];
  const network = {documentResponses: 0, documentStatus: 0, failures: 0, failureClass: 'none'};
  let nextId = 1;
  let targetSession = '';
  let emitted = false;
  const targetHostPrefix = 'http://127.0.0.1:11122/split-capture-host#';
  let summary = {
    probeState: 'starting',
    targetFound: false, fragmentValid: false, extensionWorker: false,
    attached: false, frameMatches: false, unreachable: false,
    documentState: 'unknown', titleMatches: false, hostMarker: false,
    statusState: 'unknown', errorClass: 'none', extensionWorld: false,
    bridgeContentScript: false,
    documentResponses: 0, documentStatus: 0, networkFailures: 0, failureClass: 'none',
  };
  const syncNetwork = () => {
    summary.documentResponses = network.documentResponses;
    summary.documentStatus = network.documentStatus;
    summary.networkFailures = network.failures;
    summary.failureClass = network.failureClass;
  };
  const emit = () => {
    if (emitted) return;
    syncNetwork();
    emitted = true;
    console.log(JSON.stringify(summary));
  };
  const globalTimer = setTimeout(() => {
    summary.probeState = 'deadline_partial';
    emit();
    socket.close();
  }, 6200);

  const send = (method, params = {}, sessionId = '') => new Promise((resolve, reject) => {
    const id = nextId++;
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error('cdp_timeout'));
    }, 700);
    pending.set(id, {resolve, reject, timer});
    const message = {id, method, params};
    if (sessionId) message.sessionId = sessionId;
    socket.send(JSON.stringify(message));
  });

  const connection = new Promise((resolve, reject) => {
    const timeout = setTimeout(() => reject(new Error('cdp_open_timeout')), 700);
    socket.addEventListener('open', () => { clearTimeout(timeout); resolve(); }, {once: true});
    socket.addEventListener('error', () => { clearTimeout(timeout); reject(new Error('cdp_open_error')); }, {once: true});
  });

  socket.addEventListener('message', ({data}) => {
    let message;
    try { message = JSON.parse(data); } catch { return; }
    if (message.method === 'Runtime.executionContextCreated' && message.sessionId === targetSession) {
      const context = message.params?.context;
      if (context && contexts.length < 32) contexts.push(context);
    }
    if (message.method === 'Network.responseReceived' && message.sessionId === targetSession) {
      const response = message.params?.response;
      if (message.params?.type === 'Document' && response) {
        network.documentResponses++;
        if (Number.isInteger(response.status)) network.documentStatus = response.status;
        syncNetwork();
      }
    }
    if (message.method === 'Network.loadingFailed' && message.sessionId === targetSession) {
      network.failures++;
      const error = String(message.params?.errorText ?? '').toLowerCase();
      network.failureClass = error.includes('connection_refused') ? 'connection_refused'
        : error.includes('blocked_by_client') ? 'blocked_by_client'
        : error.includes('blocked_by_administrator') ? 'blocked_by_administrator'
        : error.includes('access_denied') ? 'access_denied'
        : error.includes('proxy_connection_failed') ? 'proxy_connection_failed'
        : error.includes('name_not_resolved') ? 'dns'
        : error.includes('timed_out') ? 'timeout'
        : error.includes('address_unreachable') ? 'unreachable'
        : 'other';
      syncNetwork();
    }
    const item = pending.get(message.id);
    if (!item) return;
    clearTimeout(item.timer);
    pending.delete(message.id);
    if (message.error) item.reject(new Error('cdp_command_error'));
    else item.resolve(message.result);
  });

  try {
    await connection;
    const targetResult = await send('Target.getTargets');
    const targets = Array.isArray(targetResult.targetInfos) ? targetResult.targetInfos : [];
    const target = targets.find((item) => item.type === 'page' &&
      typeof item.url === 'string' && item.url.startsWith(targetHostPrefix) &&
      /^[a-f0-9]{64}$/.test(item.url.slice(targetHostPrefix.length)));
    summary.targetFound = Boolean(target);
    summary.fragmentValid = Boolean(target);
    summary.probeState = target ? 'target_found' : 'target_missing';
    summary.extensionWorker = targets.some((item) => item.type === 'service_worker' &&
      typeof item.url === 'string' && item.url.startsWith(`${expectedExtensionOrigin}/`));
    if (target) {
      const attached = await send('Target.attachToTarget', {targetId: target.targetId, flatten: true});
      targetSession = attached.sessionId;
      summary.attached = Boolean(targetSession);
      if (targetSession) {
        summary.probeState = 'attached';
        const enabled = await Promise.allSettled([
          send('Page.enable', {}, targetSession),
          send('Network.enable', {}, targetSession),
          send('Runtime.enable', {}, targetSession),
        ]);
        if (enabled.some((result) => result.status === 'rejected')) summary.probeState = 'enable_partial';
        const tree = await send('Page.getFrameTree', {}, targetSession).catch(() => null);
        const frame = tree?.frameTree?.frame;
        if (typeof frame?.url === 'string') {
          try {
            const frameURL = new URL(frame.url);
            summary.frameMatches = frameURL.origin === 'http://127.0.0.1:11122' &&
              frameURL.pathname === '/split-capture-host';
          } catch { /* Keep the fixed-path match false. */ }
        }
        summary.unreachable = Boolean(frame?.unreachableUrl);
        const mainFrameId = frame?.id;
        const expression = `(() => {
          const status = document.getElementById("split-capture-status")?.dataset?.state ?? "";
          const text = document.documentElement?.innerText ?? "";
          const errorClass = /ERR_BLOCKED_BY_CLIENT/i.test(text) ? "blocked_by_client"
            : /ERR_BLOCKED_BY_ADMINISTRATOR/i.test(text) ? "blocked_by_administrator"
            : /ERR_ACCESS_DENIED/i.test(text) ? "access_denied"
            : /ERR_PROXY_CONNECTION_FAILED/i.test(text) ? "proxy_connection_failed"
            : /ERR_CONNECTION_REFUSED/i.test(text) ? "connection_refused"
            : /ERR_NAME_NOT_RESOLVED|DNS_PROBE/i.test(text) ? "dns"
            : /ERR_TIMED_OUT/i.test(text) ? "timeout"
            : /ERR_ADDRESS_UNREACHABLE/i.test(text) ? "unreachable"
            : location.protocol === "chrome-error:" ? "chrome_error"
            : document.readyState === "complete" && !text.includes("Experimental capture host") ? "unexpected_document"
            : "none";
          return {
            documentState: ["loading", "interactive", "complete"].includes(document.readyState) ? document.readyState : "other",
            titleMatches: document.title === "AkuBrowser capture host",
            hostMarker: text.includes("Experimental capture host"),
            statusState: ["connected", "retrying", "failed"].includes(status) ? status : status ? "other" : "unset",
            errorClass,
            originMatches: location.origin === "http://127.0.0.1:11122",
            pathMatches: location.pathname === "/split-capture-host",
            fragmentValid: /^[a-f0-9]{64}$/.test(location.hash.slice(1))
          };
        })()`;
        const evaluated = await send('Runtime.evaluate', {
          expression, returnByValue: true, awaitPromise: false, timeout: 500,
        }, targetSession).catch(() => null);
        const document = evaluated?.result?.value;
        if (document && !evaluated.exceptionDetails) {
          summary.probeState = 'document_read';
          summary.documentState = document.documentState;
          summary.titleMatches = document.titleMatches === true;
          summary.hostMarker = document.hostMarker === true;
          summary.statusState = document.statusState;
          summary.errorClass = document.errorClass;
          summary.frameMatches = summary.frameMatches && document.originMatches === true &&
            document.pathMatches === true && document.fragmentValid === true;
        }
        await new Promise((resolve) => setTimeout(resolve, 150));
        const context = contexts.find((item) => item.origin === expectedExtensionOrigin &&
          item.auxData?.frameId === mainFrameId);
        summary.extensionWorld = Boolean(context);
        if (context) {
          const bridgeState = await send('Runtime.evaluate', {
            expression: 'globalThis.__akuBrowserTabBridgeInstalled === true',
            contextId: context.id, returnByValue: true, awaitPromise: false, timeout: 500,
          }, targetSession).catch(() => null);
          summary.bridgeContentScript = bridgeState?.result?.value === true &&
            !bridgeState.exceptionDetails;
        }
        syncNetwork();
        if (summary.probeState !== 'enable_partial') summary.probeState = 'complete';
      }
    }
    emit();
  } catch (error) {
    summary.probeState = error?.message === 'cdp_timeout' || error?.message === 'cdp_open_timeout'
      ? 'command_timeout' : 'command_error';
    emit();
  } finally {
    clearTimeout(globalTimer);
    for (const item of pending.values()) clearTimeout(item.timer);
    pending.clear();
    socket.close();
  }
}
