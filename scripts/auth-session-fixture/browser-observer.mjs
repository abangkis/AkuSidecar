import { observeSessionMutations } from './observer.mjs';

const KNOWN_TYPES = Object.freeze([
  'page', 'iframe', 'worker', 'service_worker', 'shared_worker', 'background_page', 'browser_ui',
  'tab', 'browser', 'unsupported',
]);
const OBSERVED_TYPES = new Set([
  'page', 'iframe', 'worker', 'service_worker', 'shared_worker', 'background_page', 'browser_ui',
]);
const INFRA_TYPES = new Set(['tab', 'browser']);
const MAX_TARGETS = 512;
const MAX_CORRELATED_REQUESTS = 512;
const MAX_CORRELATED_EVENTS = 1024;
const MAX_EVENTS_PER_CORRELATION = 16;
const MAX_INITIALIZING_EVENTS_PER_SESSION = 128;
const MAX_UNSUPPORTED_DIAGNOSTIC_BUCKETS = 16;
const CLOSE_TIMEOUT_MS = 2500;
const AUTO_ATTACH = Object.freeze({ autoAttach: true, waitForDebuggerOnStart: true, flatten: true });
const INTERNAL_BROWSER_SCHEMES = Object.freeze(['chrome:', 'chrome-extension:', 'data:']);

function internalServiceWorker(info) {
  try { return ['chrome:', 'chrome-extension:'].includes(new URL(info?.url).protocol); }
  catch { return false; }
}

function fixtureOrigin(value) {
  let url;
  try { url = new URL(value); } catch { throw new Error('fixture_browser_observer_invalid_origin'); }
  if (typeof value !== 'string' || url.protocol !== 'http:' || url.hostname !== '127.0.0.1' ||
      !url.port || url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== url.origin && value !== `${url.origin}/`) {
    throw new Error('fixture_browser_observer_invalid_origin');
  }
  return url.origin;
}

function fixtureId(value, name) {
  if (typeof value !== 'string' || value.length < 1 || value.length > 128 || !/^[A-Za-z0-9._:-]+$/.test(value)) {
    throw new Error(`fixture_browser_observer_invalid_${name}`);
  }
  return value;
}

function typeKey(value) {
  return KNOWN_TYPES.includes(value) ? value : 'unsupported';
}

function diagnosticTargetType(value) {
  return typeof value === 'string' && value.length > 0 && value.length <= 48 && /^[A-Za-z0-9._-]+$/.test(value)
    ? value
    : '<invalid>';
}

function diagnosticUrlScheme(value) {
  if (typeof value !== 'string' || value.length > 4096) return 'unknown';
  const match = /^([A-Za-z][A-Za-z0-9+.-]{0,31}):/.exec(value);
  return match ? match[1].toLowerCase() : 'unknown';
}

function safeCall(callback, signal) {
  try {
    const result = callback(Object.freeze(signal));
    if (result && typeof result.then === 'function') {
      result.catch(() => {});
      return false;
    }
    return true;
  } catch { return false; }
}

/**
 * Observe fixture session-cookie mutations across every target in one owned Chrome
 * process. The private CDP pipe must expose sendSession and subscribeProtocol.
 */
export async function observeBrowserSessionMutations(browser, {
  origin,
  clientId,
  onUnexpected,
} = {}) {
  origin = fixtureOrigin(origin);
  clientId = fixtureId(clientId, 'client');
  if (!browser || typeof browser.send !== 'function' || typeof browser.sendSession !== 'function' ||
      typeof browser.subscribeProtocol !== 'function' || typeof onUnexpected !== 'function') {
    throw new Error('fixture_browser_observer_invalid_browser');
  }

  const targets = new Map();
  const sessions = new Map();
  const networkListeners = new Map();
  const networkCorrelations = new Map();
  const orphanExtraInfo = new Map();
  let initializingNetworkEvents = 0;
  const attachPromises = new Map();
  const background = new Set();
  const closingTasks = new Set();
  const evidenceByKind = Object.create(null);
  const unsupportedTargetDiagnostics = new Map();
  const diagnosedUnsupportedTargetIds = new Set();
  let unsubscribe = null;
  let closed = false;
  let closing = false;
  let unavailable = false;
  let lastEvidence = 'none';
  let startupStage = 'subscribe_protocol';
  let rootAutoAttachInstalled = false;
  const retirementTypeChanges = new Map();
  let lastFailure = null;
  let closePromise = null;
  let bufferedNetworkEvents = 0;

  function addEvidence(kind) {
    evidenceByKind[kind] = (evidenceByKind[kind] ?? 0) + 1;
    lastEvidence = kind;
  }

  function recordUnsupportedTarget(info) {
    if (typeof info?.targetId === 'string') {
      if (diagnosedUnsupportedTargetIds.has(info.targetId)) return;
      diagnosedUnsupportedTargetIds.add(info.targetId);
    }
    const type = diagnosticTargetType(info?.type);
    const scheme = diagnosticUrlScheme(info?.url);
    const key = `${type}\u0000${scheme}`;
    let bucket = unsupportedTargetDiagnostics.get(key);
    if (!bucket && unsupportedTargetDiagnostics.size >= MAX_UNSUPPORTED_DIAGNOSTIC_BUCKETS) {
      const overflowKey = '<other>\u0000<other>';
      bucket = unsupportedTargetDiagnostics.get(overflowKey);
      if (!bucket) {
        bucket = { type: '<other>', urlScheme: '<other>', count: 0 };
        unsupportedTargetDiagnostics.set(overflowKey, bucket);
      }
    } else if (!bucket) {
      bucket = { type, urlScheme: scheme, count: 0 };
      unsupportedTargetDiagnostics.set(key, bucket);
    }
    bucket.count++;
  }

  function getTarget(targetId, type = 'unsupported', info = undefined) {
    let target = targets.get(targetId);
    if (target) {
      // Preserve a destroyed target's tombstone; late discovery/info events
      // cannot revive it or change the coverage verdict for live targets.
      if (target.state === 'retired') return target;
      if (target.retirementVerification) return target;
      if (target.type !== type) {
        if (type === 'unsupported') {
          recordUnsupportedTarget(info);
          reportLoss(target, 'unsupported_target_type');
        } else {
          reportLoss(target, 'target_type_changed');
        }
      }
      if (info) target.info = info;
      if (target.type === 'service_worker' && !internalServiceWorker(target.info)) {
        reportLoss(target, 'service_worker_network_coverage_unavailable');
      }
      return target;
    }
    if (targets.size >= MAX_TARGETS) {
      reportLoss(null, 'target_limit_reached');
      return null;
    }
    target = {
      targetId,
      type,
      info,
      // The browser root is already managed by this transport; it has no
      // Network domain of its own and is only the top of the attach tree.
      state: type === 'browser' ? 'ready' : 'initializing',
      sessions: new Set(),
      primarySessionId: null,
      observer: null,
      requestPage: null,
      expectedClose: false,
      failureReported: false,
      destroyWaiters: new Set(),
      setupPromises: new Set(),
    };
    targets.set(targetId, target);
    if (type === 'service_worker' && !internalServiceWorker(info)) {
      reportLoss(target, 'service_worker_network_coverage_unavailable');
    }
    if (type === 'unsupported') {
      recordUnsupportedTarget(info);
      reportLoss(target, 'unsupported_target_type');
    }
    return target;
  }

  function targetSignalType(target) {
    return target?.type ?? 'browser';
  }

  function reportLoss(target, evidenceKind) {
    if (closed || closing) return;
    if (target?.failureReported) return;
    if (target) {
      target.failureReported = true;
      if (target.state !== 'retired') target.state = 'unavailable';
    }
    unavailable = true;
    addEvidence(evidenceKind);
    lastFailure ??= Object.freeze({
      stage: startupStage,
      targetType: targetSignalType(target),
      evidenceKind,
    });
    const okay = safeCall(onUnexpected, {
      kind: 'observation_lost',
      clientId,
      targetType: targetSignalType(target),
      evidenceKind,
    });
    if (!okay) {
      addEvidence('callback_failed');
      unavailable = true;
    }
  }

  function mutationSignal(target, signal) {
    if (closed || closing) return;
    addEvidence(signal.kind ?? 'observer_signal');
    if (signal.kind === 'observation_lost') {
      target.failureReported = true;
      target.state = 'unavailable';
      unavailable = true;
    }
    const okay = safeCall(onUnexpected, {
      ...signal,
      clientId,
      targetType: target.type,
    });
    if (!okay) reportLoss(target, 'callback_failed');
  }

  function subscribeSession(sessionId, onEvent, onUnavailable) {
    if (closed || closing) throw new Error('fixture_browser_observer_closed');
    const listeners = networkListeners.get(sessionId) ?? new Set();
    const listener = { onEvent, onUnavailable };
    listeners.add(listener);
    networkListeners.set(sessionId, listeners);
    return () => {
      listeners.delete(listener);
      if (!listeners.size) networkListeners.delete(sessionId);
    };
  }

  function targetAdapter(target, sessionId) {
    return {
      origin,
      targetId: target.targetId,
      subscribeNetwork: (onEvent, onUnavailable) => subscribeSession(sessionId, onEvent, onUnavailable),
      async networkBarrier() {
        const result = await browser.sendSession(sessionId, 'Runtime.evaluate', {
          expression: 'void 0', returnByValue: true,
        });
        if (result?.exceptionDetails) throw new Error('fixture_observer_barrier_failed');
      },
      request(path, method, options) {
        if (!target.requestPage || typeof target.requestPage.request !== 'function') {
          return Promise.reject(new Error('fixture_browser_observer_refresh_page_missing'));
        }
        return target.requestPage.request(path, method, options);
      },
    };
  }

  async function resumeIfPaused(session) {
    if (!session.waitingForDebugger || session.resumeAttempted || session.detached) return;
    session.resumeAttempted = true;
    try {
      await browser.sendSession(session.sessionId, 'Runtime.runIfWaitingForDebugger');
      session.resumed = true;
    } catch {
      if (session.target.state === 'retired') return;
      lastFailure ??= Object.freeze({ stage: 'resume_target', targetType: session.target.type, evidenceKind: 'resume_failed' });
      reportLoss(session.target, 'resume_failed');
      if (session.parentSessionId) {
        await browser.sendSession(session.parentSessionId, 'Target.detachFromTarget', {
          sessionId: session.sessionId,
        }).catch(() => {});
      } else {
        await browser.send('Target.detachFromTarget', { sessionId: session.sessionId }).catch(() => {});
      }
    }
  }

  function track(promise, target = null) {
    const task = Promise.resolve(promise).catch(() => {
      if (target && target.state !== 'retired') reportLoss(target, 'target_setup_failed');
      else if (!closed && !closing) reportLoss(null, 'target_setup_failed');
    }).finally(() => background.delete(task));
    background.add(task);
    return task;
  }

  async function initializeSession(session) {
    const target = session.target;
    if (closed || closing || target.state === 'retired' || session.detached) {
      await resumeIfPaused(session);
      return;
    }
    if (!session.authoritative) {
      // Duplicate/manual sessions do not replace the one observer for this target.
      const primary = sessions.get(target.primarySessionId);
      session.setupStage = 'waiting_for_primary';
      if (primary?.setupPromise) await primary.setupPromise.catch(() => {});
      session.setupStage = 'resume_duplicate';
      await resumeIfPaused(session);
      session.setupStage = 'complete';
      return;
    }
    let stage = 'target_auto_attach';
    try {
      // HTTP service-worker setup stalled at Network.enable on local Chrome.
      // Deny coverage before any setup await; finally still releases its pause.
      if (target.type === 'service_worker' && !internalServiceWorker(target.info)) {
        reportLoss(target, 'service_worker_network_coverage_unavailable');
        return;
      }
      session.setupStage = stage;
      session.autoAttachPending = true;
      session.autoAttachPromise = browser.sendSession(session.sessionId, 'Target.setAutoAttach', AUTO_ATTACH);
      await session.autoAttachPromise;
      session.autoAttachPending = false;
      session.autoAttachEnabled = true;
      if (closed || closing || target.state === 'retired') {
        await resumeIfPaused(session);
        return;
      }
      // Local Chrome evidence shows shared-worker cookie mutations without
      // Set-Cookie or ExtraInfo. Attachment alone cannot establish coverage,
      // including when an existing worker has no bootstrap traffic to observe.
      if (target.type === 'shared_worker') {
        reportLoss(target, 'shared_worker_network_coverage_unavailable');
        return;
      }
      if (OBSERVED_TYPES.has(target.type)) {
        stage = 'network_enable';
        session.setupStage = stage;
        await browser.sendSession(session.sessionId, 'Network.enable');
        session.networkEnabled = true;
        if (closed || closing || target.state === 'retired') {
          await resumeIfPaused(session);
          return;
        }
        target.primarySessionId = session.sessionId;
        stage = 'observer_subscribe';
        session.setupStage = stage;
        target.observer = await observeSessionMutations(targetAdapter(target, session.sessionId), {
          origin,
          sessionId: session.sessionId,
          clientId,
          internalRequestSchemes: INTERNAL_BROWSER_SCHEMES,
          onUnexpected: signal => mutationSignal(target, signal),
        });
      } else if (!INFRA_TYPES.has(target.type)) {
        reportLoss(target, 'unsupported_target_type');
      }
      session.coverageReady = OBSERVED_TYPES.has(target.type) || INFRA_TYPES.has(target.type);
      if (target.state !== 'unavailable') target.state = 'ready';
      if (target.state === 'ready') {
        const queued = session.initializingEvents.splice(0);
        initializingNetworkEvents -= queued.length;
        for (const event of queued) correlateNetworkEvent(event);
      }
    } catch {
      lastFailure ??= Object.freeze({ stage, targetType: target.type, evidenceKind: 'target_setup_failed' });
      reportLoss(target, 'target_setup_failed');
    } finally {
      initializingNetworkEvents -= session.initializingEvents.length;
      session.initializingEvents.length = 0;
      session.setupStage = 'resume_target';
      await resumeIfPaused(session);
      session.setupStage = target.state === 'ready' ? 'complete' : 'failed';
    }
  }

  function onAttached(event) {
    const params = event.params;
    const info = params?.targetInfo;
    const sessionId = params?.sessionId;
    if (typeof sessionId !== 'string' || !info || typeof info.targetId !== 'string' || typeof info.type !== 'string') {
      reportLoss(null, 'malformed_target_attachment');
      return;
    }
    const key = typeKey(info.type);
    const target = getTarget(info.targetId, key, info);
    if (!target) return;
    if (sessions.has(sessionId)) return;
    const parentSessionId = typeof event.sessionId === 'string' ? event.sessionId : null;
    const authoritative = !target.primarySessionId && OBSERVED_TYPES.has(target.type) ||
      !target.sessions.size && INFRA_TYPES.has(target.type);
    const session = {
      sessionId,
      parentSessionId,
      target,
      authoritative,
      waitingForDebugger: params.waitingForDebugger === true,
      resumeAttempted: false,
      resumed: false,
      detached: false,
      autoAttachEnabled: false,
      networkEnabled: false,
      coverageReady: false,
      initializingEvents: [],
      setupStage: 'attached',
    };
    sessions.set(sessionId, session);
    target.sessions.add(sessionId);
    if (authoritative && OBSERVED_TYPES.has(target.type)) target.primarySessionId = sessionId;
    const task = track(initializeSession(session), target);
    session.setupPromise = task;
    target.setupPromises.add(task);
    task.finally(() => target.setupPromises.delete(task));
  }

  function ensureAttached(info) {
    if (!info || typeof info.targetId !== 'string' || typeof info.type !== 'string' || info.type === 'browser') return Promise.resolve();
    const target = getTarget(info.targetId, typeKey(info.type), info);
    if (!target || target.state === 'retired' || target.sessions.size || attachPromises.has(info.targetId)) return Promise.resolve();
    const promise = (async () => {
      try {
        const result = await browser.send('Target.attachToTarget', { targetId: info.targetId, flatten: true });
        if (target.state === 'retired') return;
        if (typeof result?.sessionId !== 'string') throw new Error('fixture_attach_missing_session');
        if (!sessions.has(result.sessionId)) {
          onAttached({ method: 'Target.attachedToTarget', params: {
            sessionId: result.sessionId, targetInfo: info, waitingForDebugger: false,
          } });
        }
      } catch {
        if (target.state !== 'retired') {
          lastFailure ??= Object.freeze({ stage: 'target_attach', targetType: target.type, evidenceKind: 'target_attach_failed' });
          reportLoss(target, 'target_attach_failed');
        }
      }
    })().finally(() => attachPromises.delete(info.targetId));
    attachPromises.set(info.targetId, promise);
    return promise;
  }

  function retireRecord(target, evidenceIfIncomplete = true) {
    if (!target || target.state === 'retired') return;
    const brokerIncomplete = [...orphanExtraInfo.values()].some(items => items.some(item => item.target === target)) ||
      [...networkCorrelations.values()].some(item =>
        (item.requestTarget === target || item.responseTarget === target) &&
        (item.bufferedEvents.length > 0 || item.responseHasExtraInfo === true && item.extraInfoCount < 1));
    target.retirementComplete = !brokerIncomplete &&
      ![...target.sessions].some(id => sessions.get(id)?.initializingEvents.length) &&
      (target.observer?.status().pendingExtraInfo ?? 0) === 0;
    if (evidenceIfIncomplete && brokerIncomplete) reportLoss(target, 'retired_with_pending_network_evidence');
    if (evidenceIfIncomplete && [...target.sessions].some(id => sessions.get(id)?.initializingEvents.length)) {
      reportLoss(target, 'retired_with_initializing_network_events');
    }
    if (target.observer) {
      const state = target.observer.status();
      if (evidenceIfIncomplete && state.pendingExtraInfo > 0) {
        reportLoss(target, 'retired_with_pending_extra_info');
      }
      target.observer.close();
      target.observer = null;
    }
    target.state = 'retired';
    for (const sessionId of target.sessions) {
      const session = sessions.get(sessionId);
      if (session) session.detached = true; // Retain a tombstone for a later detach event.
      networkListeners.delete(sessionId);
    }
    for (const resolve of [...target.destroyWaiters]) resolve(true);
    target.destroyWaiters.clear();
  }

  function markUnavailable(target, evidenceKind) {
    if (!target) {
      reportLoss(null, evidenceKind);
      return;
    }
    reportLoss(target, evidenceKind);
  }

  function verifyRetiringInternalUi(target, info) {
    if (target.retirementVerification) return true;
    let internal = false;
    try { internal = ['chrome:', 'chrome-extension:'].includes(new URL(target.info?.url).protocol); } catch { /* strict */ }
    // Owned internal UI teardown events can follow the initiating page's
    // destruction. Confirm disappearance rather than relying on event timing.
    if (target.type !== 'browser_ui' || !internal) return false;
    const changedType = typeKey(info.type);
    retirementTypeChanges.set(changedType, (retirementTypeChanges.get(changedType) ?? 0) + 1);
    target.retirementVerification = true;
    track((async () => {
      let timer;
      try {
        const result = await Promise.race([
          browser.send('Target.getTargets'),
          new Promise((_, reject) => { timer = setTimeout(() => reject(new Error('fixture_retirement_probe_timeout')), CLOSE_TIMEOUT_MS); }),
        ]);
        if (target.state === 'retired') return; // Explicit destruction also confirms absence.
        const infos = result?.targetInfos;
        if (!Array.isArray(infos) || infos.some(item => !item || typeof item.targetId !== 'string') ||
            infos.some(item => item.targetId === target.targetId)) {
          reportLoss(target, 'retiring_ui_disappearance_unconfirmed');
          return;
        }
        checkCorrelationFlush();
        retireRecord(target);
      } catch {
        if (target.state !== 'retired') reportLoss(target, 'retiring_ui_disappearance_unconfirmed');
      } finally {
        clearTimeout(timer);
        target.retirementVerification = false;
      }
    })(), target);
    return true;
  }

  function networkSession(event) {
    const session = typeof event.sessionId === 'string' ? sessions.get(event.sessionId) : null;
    if (!session || session.detached || !session.networkEnabled || !session.coverageReady ||
        session.target.state !== 'ready' || !session.target.observer) {
      reportLoss(session?.target ?? null, 'network_source_unavailable');
      return null;
    }
    return session;
  }

  function targetAncestry(sessionId) {
    const result = [];
    const seen = new Set();
    let target = sessions.get(sessionId)?.target;
    while (target) {
      if (seen.has(target.targetId)) return null;
      seen.add(target.targetId);
      result.push(target.targetId);
      // Manual/root attachment can win the authoritative-session race. A
      // duplicate recursive attachment still supplies topology for this exact
      // target; absence of a parent on a manual session is not a competing edge.
      const parents = new Map();
      for (const attachedId of target.sessions) {
        const attached = sessions.get(attachedId);
        if (!attached?.parentSessionId) continue;
        const parent = sessions.get(attached.parentSessionId)?.target;
        if (!parent) return null;
        parents.set(parent.targetId, parent);
      }
      if (parents.size > 1) return null;
      if (!parents.size) return result;
      target = parents.values().next().value;
    }
    return null;
  }

  function ancestryRelated(leftSessionId, rightSessionId) {
    const left = sessions.get(leftSessionId);
    const right = sessions.get(rightSessionId);
    if (!left || !right) return false;
    if (left.target.targetId === right.target.targetId) return true;
    const leftPath = targetAncestry(leftSessionId);
    const rightPath = targetAncestry(rightSessionId);
    if (!leftPath || !rightPath) return false;
    return leftPath.includes(right.target.targetId) || rightPath.includes(left.target.targetId);
  }

  function requestHasRefreshNonce(event) {
    const headers = event?.params?.request?.headers;
    return !!headers && typeof headers === 'object' && !Array.isArray(headers) &&
      Object.keys(headers).some(name => name.toLowerCase() === 'x-aku-fixture-refresh-nonce');
  }

  function requestUrlIsCovered(value) {
    try {
      const url = new URL(value);
      return url.origin === origin || INTERNAL_BROWSER_SCHEMES.includes(url.protocol);
    } catch { return false; }
  }

  function correlationFailure(evidenceKind, ...candidates) {
    const unique = new Set(candidates.filter(target => target && target.state !== 'retired'));
    if (!unique.size) reportLoss(null, evidenceKind);
    else for (const target of unique) markUnavailable(target, evidenceKind);
  }

  function bufferCorrelationEvent(correlation, event, sourceSession) {
    if (bufferedNetworkEvents >= MAX_CORRELATED_EVENTS ||
        correlation.bufferedEvents.length >= MAX_EVENTS_PER_CORRELATION) {
      correlationFailure('network_correlation_queue_full', correlation.requestTarget, sourceSession.target);
      return false;
    }
    correlation.bufferedEvents.push({ event, sessionId: sourceSession.sessionId });
    bufferedNetworkEvents++;
    return true;
  }

  function bufferOrphanExtraInfo(requestId, event, sourceSession) {
    const events = orphanExtraInfo.get(requestId) ?? [];
    if (bufferedNetworkEvents >= MAX_CORRELATED_EVENTS || events.length >= MAX_EVENTS_PER_CORRELATION) {
      correlationFailure('network_correlation_queue_full', sourceSession.target);
      return;
    }
    events.push({ event, sessionId: sourceSession.sessionId, target: sourceSession.target });
    orphanExtraInfo.set(requestId, events);
    bufferedNetworkEvents++;
  }

  function releaseCorrelationBuffers(correlation) {
    const released = (correlation.requestEvent ? 1 : 0) + correlation.bufferedEvents.length;
    bufferedNetworkEvents = Math.max(0, bufferedNetworkEvents - released);
    correlation.requestEvent = null;
    correlation.bufferedEvents = [];
  }

  function deliverNetwork(target, event) {
    if (!target || target.state !== 'ready' || !target.observer || !target.primarySessionId) {
      reportLoss(target ?? null, 'network_replay_target_unavailable');
      return false;
    }
    const listeners = networkListeners.get(target.primarySessionId);
    if (!listeners?.size) {
      reportLoss(target, 'network_replay_listener_missing');
      return false;
    }
    const replay = { method: event.method, params: event.params ?? {}, sessionId: target.primarySessionId };
    for (const listener of [...listeners]) {
      try { listener.onEvent(replay); } catch {
        try { listener.onUnavailable(); } catch { /* private event boundary */ }
      }
    }
    return true;
  }

  function relatedExtraInfo(correlation, responseSessionId) {
    for (const item of correlation.bufferedEvents) {
      if (item.event.method !== 'Network.responseReceivedExtraInfo') continue;
      if (!ancestryRelated(correlation.requestSessionId, item.sessionId) ||
          !ancestryRelated(responseSessionId, item.sessionId)) return false;
    }
    return true;
  }

  function correlateRequest(event, source) {
    const params = event.params ?? {};
    const requestId = params.requestId;
    if (typeof requestId !== 'string' || !requestId) {
      reportLoss(source.target, 'network_request_id_missing');
      return;
    }
    const existing = networkCorrelations.get(requestId);
    if (params.redirectResponse) {
      correlationFailure('network_redirect_ambiguous', existing?.requestTarget, existing?.responseTarget, source.target);
      return;
    }
    if (existing) {
      correlationFailure('network_request_id_collision', existing.requestTarget, existing.responseTarget, source.target);
      return;
    }
    if (!requestUrlIsCovered(params.request?.url)) {
      reportLoss(source.target, 'request_outside_fixture_origin');
      return;
    }
    if (source.target.type === 'service_worker') {
      let internalRequest = false;
      try { internalRequest = INTERNAL_BROWSER_SCHEMES.includes(new URL(params.request?.url).protocol); } catch { /* strict */ }
      if (!internalRequest) {
        reportLoss(source.target, 'service_worker_http_request_unobservable');
        return;
      }
    }
    if (networkCorrelations.size >= MAX_CORRELATED_REQUESTS || bufferedNetworkEvents >= MAX_CORRELATED_EVENTS) {
      reportLoss(source.target, 'network_correlation_queue_full');
      return;
    }
    const correlation = {
      requestSessionId: source.sessionId,
      requestTarget: source.target,
      requestHasRefreshNonce: requestHasRefreshNonce(event),
      requestEvent: event,
      bufferedEvents: [],
      responseSessionId: null,
      responseTarget: null,
      responseHasExtraInfo: null,
      extraInfoCount: 0,
      completed: false,
    };
    networkCorrelations.set(requestId, correlation);
    bufferedNetworkEvents++;

    const early = orphanExtraInfo.get(requestId);
    if (early) {
      orphanExtraInfo.delete(requestId);
      bufferedNetworkEvents -= early.length;
      for (const item of early) {
        if (!ancestryRelated(source.sessionId, item.sessionId) ||
            !bufferCorrelationEvent(correlation, item.event, sessions.get(item.sessionId) ?? { target: item.target, sessionId: item.sessionId })) {
          correlationFailure('network_extra_info_owner_ambiguous', source.target, item.target);
          return;
        }
      }
    }
  }

  function correlateExtraInfo(event, source) {
    const requestId = event.params?.requestId;
    if (typeof requestId !== 'string' || !requestId) {
      reportLoss(source.target, 'network_extra_info_request_id_missing');
      return;
    }
    const correlation = networkCorrelations.get(requestId);
    if (!correlation) {
      bufferOrphanExtraInfo(requestId, event, source);
      return;
    }
    if (!ancestryRelated(correlation.requestSessionId, source.sessionId)) {
      correlationFailure('network_extra_info_owner_ambiguous', correlation.requestTarget, source.target);
      return;
    }
    if (correlation.requestHasRefreshNonce && source.sessionId !== correlation.requestSessionId) {
      correlationFailure('network_cross_session_refresh_or_extra_info', correlation.requestTarget, source.target);
      return;
    }
    if (!correlation.responseSessionId) {
      bufferCorrelationEvent(correlation, event, source);
      return;
    }
    if (!ancestryRelated(correlation.responseSessionId, source.sessionId) ||
        correlation.responseHasExtraInfo !== true || correlation.extraInfoCount >= 1) {
      correlationFailure('network_extra_info_owner_ambiguous', correlation.requestTarget, correlation.responseTarget, source.target);
      return;
    }
    if (deliverNetwork(correlation.responseTarget, event)) correlation.extraInfoCount++;
  }

  function correlateResponse(event, source) {
    const requestId = event.params?.requestId;
    if (typeof requestId !== 'string' || !requestId) {
      reportLoss(source.target, 'network_response_request_id_missing');
      return;
    }
    let correlation = networkCorrelations.get(requestId);
    if (!correlation) {
      // Chrome may finish its own internal request after observation starts.
      // Only an explicit internal response URL can establish this narrow case.
      let internal = false;
      try { internal = INTERNAL_BROWSER_SCHEMES.includes(new URL(event.params?.response?.url).protocol); } catch { /* unknown stays strict */ }
      if (internal) {
        correlateRequest({ method: 'Network.requestWillBeSent', sessionId: source.sessionId,
          params: { requestId, request: { url: event.params.response.url, method: 'GET', headers: {} } } }, source);
        correlation = networkCorrelations.get(requestId);
      }
    }
    if (!correlation) {
      correlationFailure('network_response_without_request', source.target);
      return;
    }
    if (correlation.responseSessionId || !ancestryRelated(correlation.requestSessionId, source.sessionId)) {
      correlationFailure('network_response_owner_ambiguous', correlation.requestTarget, correlation.responseTarget, source.target);
      return;
    }
    const nonceCrossedSession = correlation.requestHasRefreshNonce && source.sessionId !== correlation.requestSessionId;
    const nonceExtraCrossedSession = correlation.requestHasRefreshNonce &&
      correlation.bufferedEvents.some(item => item.sessionId !== correlation.requestSessionId);
    if (nonceCrossedSession || nonceExtraCrossedSession || !relatedExtraInfo(correlation, source.sessionId)) {
      correlationFailure('network_cross_session_refresh_or_extra_info', correlation.requestTarget, source.target,
        ...correlation.bufferedEvents.map(item => sessions.get(item.sessionId)?.target));
      return;
    }
    const hasExtraInfo = event.params?.hasExtraInfo;
    const bufferedExtras = correlation.bufferedEvents.filter(item => item.event.method === 'Network.responseReceivedExtraInfo');
    if (typeof hasExtraInfo !== 'boolean' || bufferedExtras.length > (hasExtraInfo ? 1 : 0)) {
      correlationFailure('network_response_extra_info_ambiguous', correlation.requestTarget, source.target);
      return;
    }
    correlation.responseSessionId = source.sessionId;
    correlation.responseTarget = source.target;
    correlation.responseHasExtraInfo = hasExtraInfo;
    if (!deliverNetwork(source.target, correlation.requestEvent)) return;
    for (const item of correlation.bufferedEvents) {
      if (!deliverNetwork(source.target, item.event)) return;
      if (item.event.method === 'Network.responseReceivedExtraInfo') correlation.extraInfoCount++;
    }
    if (!deliverNetwork(source.target, event)) return;
    releaseCorrelationBuffers(correlation);
  }

  function correlateTerminal(event, source) {
    const requestId = event.params?.requestId;
    if (typeof requestId !== 'string' || !requestId) {
      reportLoss(source.target, 'network_terminal_request_id_missing');
      return;
    }
    const correlation = networkCorrelations.get(requestId);
    if (!correlation) {
      // Internal browser UI targets can surface terminal-only lifecycle tails,
      // including after dynamic attachment. This event alone has no response
      // or cookie evidence; observed requests and orphan ExtraInfo stay strict.
      let internalBrowserUi = false;
      try {
        internalBrowserUi = source.target.type === 'browser_ui' &&
          INTERNAL_BROWSER_SCHEMES.includes(new URL(source.target.info?.url).protocol);
      } catch { /* unknown target URLs stay strict */ }
      if (internalBrowserUi && !orphanExtraInfo.has(requestId)) {
        addEvidence('internal_ui_terminal_without_request');
        return;
      }
      correlationFailure('network_terminal_without_request', source.target);
      return;
    }
    if (correlation.completed || !ancestryRelated(correlation.requestSessionId, source.sessionId)) {
      correlationFailure('network_terminal_owner_ambiguous', correlation.requestTarget, correlation.responseTarget, source.target);
      return;
    }
    if (!correlation.responseSessionId) {
      if (event.method !== 'Network.loadingFailed' || correlation.bufferedEvents.length ||
          correlation.requestHasRefreshNonce && source.sessionId !== correlation.requestSessionId) {
        correlationFailure('network_response_owner_missing', correlation.requestTarget, source.target);
        return;
      }
      correlation.responseSessionId = source.sessionId;
      correlation.responseTarget = source.target;
      correlation.responseHasExtraInfo = false;
      if (!deliverNetwork(source.target, correlation.requestEvent)) return;
      releaseCorrelationBuffers(correlation);
    } else if (!ancestryRelated(correlation.responseSessionId, source.sessionId)) {
      correlationFailure('network_terminal_owner_ambiguous', correlation.requestTarget, correlation.responseTarget, source.target);
      return;
    }
    if (deliverNetwork(correlation.responseTarget, event)) correlation.completed = true;
  }

  function correlateNetworkEvent(event) {
    const initializing = sessions.get(event.sessionId);
    if (initializing?.authoritative && !initializing.detached &&
        initializing.target.state === 'initializing' && !initializing.coverageReady) {
      if (initializing.initializingEvents.length >= MAX_INITIALIZING_EVENTS_PER_SESSION ||
          initializingNetworkEvents >= MAX_CORRELATED_EVENTS) {
        reportLoss(initializing.target, 'initializing_network_queue_full');
      } else {
        initializing.initializingEvents.push(event);
        initializingNetworkEvents++;
      }
      return;
    }
    const source = networkSession(event);
    if (!source) return;
    switch (event.method) {
      case 'Network.requestWillBeSent':
        correlateRequest(event, source);
        return;
      case 'Network.responseReceived':
        correlateResponse(event, source);
        return;
      case 'Network.responseReceivedExtraInfo':
        correlateExtraInfo(event, source);
        return;
      case 'Network.loadingFinished':
      case 'Network.loadingFailed':
        correlateTerminal(event, source);
        return;
      default:
        deliverNetwork(source.target, event);
    }
  }

  function checkCorrelationFlush() {
    if (orphanExtraInfo.size) {
      const target = [...orphanExtraInfo.values()][0]?.[0]?.target;
      correlationFailure('network_extra_info_owner_missing', target);
      return;
    }
    for (const correlation of networkCorrelations.values()) {
      if (correlation.bufferedEvents.length && !correlation.responseSessionId) {
        correlationFailure('network_response_owner_missing', correlation.requestTarget);
        return;
      }
    }
  }

  function onEvent(event) {
    if (closed || !event || typeof event.method !== 'string') return;
    if (closing) {
      if (event.method === 'Target.attachedToTarget' && event.params?.waitingForDebugger === true &&
          typeof event.params.sessionId === 'string') {
        const sessionId = event.params.sessionId;
        const parentSessionId = typeof event.sessionId === 'string' ? event.sessionId : null;
        const resume = browser.sendSession(sessionId, 'Runtime.runIfWaitingForDebugger').catch(async () => {
          const params = { sessionId };
          if (parentSessionId) await browser.sendSession(parentSessionId, 'Target.detachFromTarget', params).catch(() => {});
          else await browser.send('Target.detachFromTarget', params).catch(() => {});
        });
        closingTasks.add(resume);
        resume.finally(() => closingTasks.delete(resume));
      }
      return;
    }
    const params = event.params ?? {};
    if (event.method === 'Target.attachedToTarget') {
      onAttached(event);
    } else if (event.method === 'Target.targetCreated') {
      const info = params.targetInfo;
      if (!info || typeof info.targetId !== 'string' || typeof info.type !== 'string') {
        reportLoss(null, 'malformed_target_created');
      } else {
        const target = getTarget(info.targetId, typeKey(info.type), info);
        // Once recursive auto-attach is installed, let its paused session own
        // new targets. A manual attach can otherwise win authority and wait on
        // Network.enable while the paused duplicate waits for that same setup.
        if (!rootAutoAttachInstalled && target && target.type !== 'browser' && target.state !== 'retired') {
          track(ensureAttached(info), target);
        }
      }
    } else if (event.method === 'Target.targetDestroyed') {
      if (typeof params.targetId === 'string') retireRecord(targets.get(params.targetId));
    } else if (event.method === 'Target.detachedFromTarget') {
      const sessionId = params.sessionId;
      const session = typeof sessionId === 'string' ? sessions.get(sessionId) : null;
      if (session) {
        session.detached = true;
        networkListeners.delete(sessionId);
        const target = session.target;
        if (!target.expectedClose && !target.retirementVerification && target.state !== 'retired') markUnavailable(target, 'unexpected_detach');
      } else {
        markUnavailable(null, 'unknown_detach');
      }
    } else if (event.method === 'Target.targetCrashed') {
      const target = typeof params.targetId === 'string' ? targets.get(params.targetId) : null;
      markUnavailable(target, 'target_crashed');
    } else if (event.method === 'Target.targetInfoChanged') {
      const info = params.targetInfo;
      const target = typeof info?.targetId === 'string' ? targets.get(info.targetId) : null;
      if (target?.state === 'retired') return;
      if (target && typeof info.type === 'string' && typeKey(info.type) !== target.type && verifyRetiringInternalUi(target, info)) return;
      if (target && info) {
        target.info = info;
        if (target.type === 'service_worker' && !internalServiceWorker(info)) {
          reportLoss(target, 'service_worker_network_coverage_unavailable');
        }
      }
      if (target && typeof info.type === 'string' && typeKey(info.type) !== target.type) {
        if (typeKey(info.type) === 'unsupported') {
          recordUnsupportedTarget(info);
          markUnavailable(target, 'unsupported_target_type');
        } else {
          markUnavailable(target, 'target_type_changed');
        }
      }
    } else if (event.method.startsWith('Network.')) {
      correlateNetworkEvent(event);
    }
  }

  function onUnavailable() {
    if (closed || closing) return;
    unavailable = true;
    let found = false;
    for (const target of targets.values()) {
      if (target.state === 'retired' || target.state === 'unavailable') continue;
      found = true;
      reportLoss(target, 'transport_unavailable');
      target.state = 'unavailable';
      target.observer?.close();
      target.observer = null;
    }
    if (!found) reportLoss(null, 'transport_unavailable');
    for (const listeners of networkListeners.values()) {
      for (const listener of [...listeners]) {
        try { listener.onUnavailable(); } catch { /* event boundary */ }
      }
    }
    networkListeners.clear();
  }

  function active() {
    if (closed || closing) throw new Error('fixture_browser_observer_closed');
    if (unavailable) throw new Error('fixture_browser_observer_unavailable');
  }

  async function waitBackground() {
    while (background.size) await Promise.allSettled([...background]);
    const setup = [];
    for (const target of targets.values()) setup.push(...target.setupPromises);
    if (setup.length) await Promise.allSettled(setup);
  }

  async function flush() {
    active();
    await waitBackground();
    active();
    for (const target of targets.values()) {
      if (target.state === 'retired' || target.state === 'unavailable') continue;
      if (target.state !== 'ready') {
        reportLoss(target, 'target_not_ready');
        throw new Error('fixture_browser_observer_unavailable');
      }
    }
    const ready = [...targets.values()].filter(target => target.state === 'ready' && target.observer);
    const results = await Promise.allSettled(ready.map(target => target.observer.flush()));
    // A barrier can reject after its exact target has been verifiably retired.
    // Settle disappearance probes before deciding whether that rejection means
    // lost coverage; no incomplete evidence may be excused by retirement.
    await waitBackground();
    let failed = false;
    for (let index = 0; index < results.length; index++) {
      if (results[index].status !== 'rejected') continue;
      const target = ready[index];
      if (target.state === 'retired' && target.retirementComplete === true) continue;
      markUnavailable(target, 'target_flush_failed');
      failed = true;
    }
    if (failed) throw new Error('fixture_browser_observer_unavailable');
    checkCorrelationFlush();
    active();
    return status();
  }

  async function authorizeRefresh(page, run) {
    active();
    if (!page || typeof page.targetId !== 'string' || page.origin !== origin || typeof page.request !== 'function') {
      throw new Error('fixture_browser_observer_invalid_page');
    }
    const target = targets.get(page.targetId);
    if (!target || target.type !== 'page' || target.state !== 'ready' || !target.observer) {
      throw new Error('fixture_browser_observer_target_unavailable');
    }
    if (target.requestPage) throw new Error('fixture_browser_observer_refresh_busy');
    target.requestPage = page;
    try {
      const result = await target.observer.authorizeRefresh(run);
      active();
      return result;
    }
    finally { target.requestPage = null; }
  }

  async function waitForDestroy(target) {
    if (target.state === 'retired') return;
    await new Promise((resolve, reject) => {
      let settled = false;
      const finish = value => {
        if (settled) return;
        settled = true;
        clearTimeout(timer);
        target.destroyWaiters.delete(onDestroy);
        resolve(value);
      };
      const onDestroy = () => finish(true);
      const timer = setTimeout(() => {
        target.destroyWaiters.delete(onDestroy);
        reject(new Error('fixture_browser_observer_close_timeout'));
      }, CLOSE_TIMEOUT_MS);
      target.destroyWaiters.add(onDestroy);
      if (target.state === 'retired') finish(true);
    });
  }

  async function retireTarget(targetId) {
    active();
    const target = targets.get(targetId);
    if (!target) throw new Error('fixture_browser_observer_unknown_target');
    if (target.state === 'retired') return status();
    if (target.state !== 'ready') throw new Error('fixture_browser_observer_target_unavailable');
    // Flush first so outstanding ExtraInfo cannot be hidden by retirement.
    if (target.observer) {
      try { await target.observer.flush(); } catch {
        markUnavailable(target, 'retire_flush_failed');
        throw new Error('fixture_browser_observer_unavailable');
      }
    }
    checkCorrelationFlush();
    active();
    target.expectedClose = true;
    try {
      await browser.send('Target.closeTarget', { targetId });
    } catch {
      if (target.state !== 'retired') {
        markUnavailable(target, 'owned_close_failed');
        throw new Error('fixture_browser_observer_close_failed');
      }
    }
    try { await waitForDestroy(target); } catch {
      markUnavailable(target, 'owned_close_unconfirmed');
      throw new Error('fixture_browser_observer_close_unconfirmed');
    }
    if (target.state !== 'retired') retireRecord(target);
    await waitBackground();
    active();
    return status();
  }

  function status() {
    const setupBuckets = new Map();
    for (const target of targets.values()) {
      if (target.state !== 'initializing') continue;
      const attached = [...target.sessions].map(id => sessions.get(id)).filter(Boolean);
      const stages = attached.length ? attached.map(session => session.setupStage) : ['awaiting_attachment'];
      for (const stage of stages) {
        const key = `${target.type}:${stage}`;
        const bucket = setupBuckets.get(key) ?? { targetType: target.type, stage, count: 0 };
        bucket.count++;
        setupBuckets.set(key, bucket);
      }
    }
    const targetsByType = Object.fromEntries(KNOWN_TYPES.map(type => [type, {
      ready: 0, retired: 0, unavailable: 0, initializing: 0,
    }]));
    for (const target of targets.values()) {
      const counts = targetsByType[target.type] ?? targetsByType.unsupported;
      const state = target.state === 'ready' ? 'ready' : target.state === 'retired' ? 'retired' :
        target.state === 'unavailable' ? 'unavailable' : 'initializing';
      counts[state]++;
    }
    return Object.freeze({
      scope: 'browser_targets',
      state: closed ? 'closed' : unavailable ? 'unavailable' : 'active',
      targetsByType,
      initializingSetupStages: [...setupBuckets.values()],
      retirementTypeChanges: [...retirementTypeChanges].map(([targetType, count]) => ({ targetType, count })),
      unsupportedTargetDiagnostics: [...unsupportedTargetDiagnostics.values()].map(value => ({ ...value })),
      evidenceByKind: { ...evidenceByKind },
      lastEvidence,
      lastFailure,
    });
  }

  async function close() {
    if (closePromise) return closePromise;
    closing = true;
    closePromise = (async () => {
      const ownedSessions = [...sessions.values()].filter(session => !session.detached && session.target.state !== 'retired');
      // A recursive attach may still be awaiting its CDP reply. Let it settle,
      // then disable each child watcher before disabling the browser watcher.
      await Promise.allSettled(ownedSessions.map(session => session.autoAttachPromise).filter(Boolean));
      const disableNested = ownedSessions.filter(session => session.autoAttachEnabled).map(session =>
        browser.sendSession(session.sessionId, 'Target.setAutoAttach', {
          autoAttach: false, waitForDebuggerOnStart: false, flatten: true,
        }).catch(() => {}));
      const resumes = ownedSessions.filter(session => session.waitingForDebugger && !session.resumeAttempted).map(session =>
        resumeIfPaused(session).catch(() => {}));
      await Promise.allSettled(disableNested);
      await browser.send('Target.setAutoAttach', { autoAttach: false, waitForDebuggerOnStart: false, flatten: true }).catch(() => {});
      await Promise.allSettled(resumes);
      while (closingTasks.size) await Promise.allSettled([...closingTasks]);
      for (const target of targets.values()) {
        target.observer?.close();
        target.observer = null;
        if (target.state !== 'retired') target.state = 'unavailable';
        for (const resolve of [...target.destroyWaiters]) resolve(false);
        target.destroyWaiters.clear();
      }
      if (unsubscribe) unsubscribe();
      unsubscribe = null;
      networkListeners.clear();
      closed = true;
      closing = false;
      return status();
    })();
    return closePromise;
  }

  try {
    startupStage = 'subscribe_protocol';
    unsubscribe = browser.subscribeProtocol(onEvent, onUnavailable);
    startupStage = 'discover_targets';
    await browser.send('Target.setDiscoverTargets', { discover: true });
    startupStage = 'root_auto_attach';
    await browser.send('Target.setAutoAttach', AUTO_ATTACH);
    rootAutoAttachInstalled = true;
    startupStage = 'get_targets';
    const result = await browser.send('Target.getTargets');
    const infos = Array.isArray(result?.targetInfos) ? result.targetInfos : [];
    for (const info of infos) {
      if (!info || typeof info.targetId !== 'string' || typeof info.type !== 'string') {
        reportLoss(null, 'malformed_target_snapshot');
        continue;
      }
      const target = getTarget(info.targetId, typeKey(info.type), info);
      if (target && target.type !== 'browser') track(ensureAttached(info), target);
    }
    startupStage = 'reconcile_targets';
    await waitBackground();
    if (unavailable) throw new Error('fixture_browser_observer_unavailable');
    for (const target of targets.values()) {
      if (target.state !== 'ready' && target.state !== 'retired') {
        reportLoss(target, 'target_not_ready_at_start');
        throw new Error('fixture_browser_observer_unavailable');
      }
    }
  } catch {
    unavailable = true;
    addEvidence('startup_failed');
    lastFailure ??= Object.freeze({ stage: startupStage, targetType: 'browser', evidenceKind: 'startup_failed' });
    const observationStatus = status();
    await close();
    const error = new Error('fixture_browser_observer_start_failed');
    error.observationStatus = observationStatus;
    throw error;
  }

  return Object.freeze({ authorizeRefresh, flush, status, close, retireTarget });
}
