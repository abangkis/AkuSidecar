// Bounded diagnostic, deliberately independent of browser-observer and session policy.
import { createHash } from 'node:crypto';

export function fixtureCookieCheckpoint(cookies) {
  if (!Array.isArray(cookies) || cookies.some(cookie => cookie.domain !== '127.0.0.1')) {
    throw new Error('fixture_checkpoint_scope_invalid');
  }
  const session = cookies.filter(cookie => cookie.name === 'fixture_session');
  if (session.length !== 1 || typeof session[0].value !== 'string' || !session[0].value) {
    throw new Error('fixture_checkpoint_session_invalid');
  }
  const fields = ['name', 'value', 'domain', 'path', 'secure', 'httpOnly', 'sameSite', 'partitionKey'];
  return createHash('sha256').update(JSON.stringify(fields.map(field => session[0][field] ?? null))).digest('hex');
}

export async function createWorkerCapabilityProbe(browser, { targetType, origin, resumeOrder = 'after-network' }) {
  let parsed;
  try { parsed = new URL(origin); } catch { throw new Error('fixture_invalid_worker_probe'); }
  if (!['shared_worker', 'service_worker'].includes(targetType) ||
      !['after-network', 'before-network'].includes(resumeOrder) ||
      parsed.hostname !== '127.0.0.1' || parsed.protocol !== 'http:' || !parsed.port ||
      parsed.username || parsed.password || origin !== parsed.origin) {
    throw new Error('fixture_invalid_worker_probe');
  }
  const targets = new Map(), requests = new Map(), tasks = new Set(), earlyExtraInfo = new Map();
  const maxEarlyExtraInfo = 64;
  let unavailable = false, stopped = false;
  const evidence = {
    targetType, resumeOrder,
    observationWindow: resumeOrder === 'before-network' ? 'post_resume_only' : 'before_worker_execution',
    startupRequestCoverageQualified: false, attachedSessions: 0, authoritativeTargets: 0,
    duplicateSessions: 0, ignoredNonFixtureTargets: 0, pausedSessions: 0, networkEnabled: 0, resumed: 0,
    evidenceScope: 'exact_fixture_worker_sessions_only',
    detachedSessions: 0, targetLifecycle: [],
    setupErrors: [], rotateRequests: 0, rotateResponses: 0,
    responseHasExtraInfo: false, responseExtraInfo: false, responseSetCookie: false,
    requestExtraInfo: false, firstRotateStatus: null, earlyMetadataOverflow: false,
  };
  const safeError = error => /^fixture_cdp_[A-Za-z0-9_.]+$/.test(error?.message ?? '') ? error.message : 'fixture_worker_setup_failed';
  async function resume(session) {
    if (!session.paused || session.resumed) return;
    await browser.sendSession(session.id, 'Runtime.runIfWaitingForDebugger');
    session.resumed = true; evidence.resumed++;
  }
  async function setup(session) {
    try {
      if (resumeOrder === 'before-network') await resume(session);
      await browser.sendSession(session.id, 'Network.enable');
      evidence.networkEnabled++;
      await resume(session);
      session.ready = true;
    } catch (error) {
      evidence.setupErrors.push(safeError(error));
      // A failed diagnostic must not leave its own paused target wedged.
      await resume(session).catch(() => {});
    }
  }
  function queue(task) {
    tasks.add(task);
    task.finally(() => tasks.delete(task)).catch(() => {});
  }
  const hasSetCookie = headers => Object.keys(headers ?? {}).some(key => key.toLowerCase() === 'set-cookie');
  function applyExtraInfo(metadata) {
    evidence.requestExtraInfo ||= metadata.requestExtraInfo;
    evidence.responseExtraInfo ||= metadata.responseExtraInfo;
    evidence.responseSetCookie ||= metadata.responseSetCookie;
  }
  const unsubscribe = browser.subscribeProtocol(event => {
    const p = event.params ?? {};
    if (event.method === 'Target.detachedFromTarget') {
      const session = [...targets.values()].find(session => session.id === p.sessionId);
      if (session && !session.detached) {
        session.detached = true; evidence.detachedSessions++;
        evidence.targetLifecycle.push({ alias: session.alias, event: 'detached' });
      }
      return;
    }
    if (event.method === 'Target.attachedToTarget' && p.targetInfo?.type === targetType) {
      evidence.attachedSessions++;
      const session = { id: p.sessionId, paused: p.waitingForDebugger === true, resumed: false, ready: false, detached: false,
        alias: evidence.attachedSessions };
      evidence.targetLifecycle.push({ alias: session.alias, event: 'attached', paused: session.paused,
        fixtureScriptMatches: p.targetInfo.url === `${origin}/${targetType === 'shared_worker' ? 'shared-worker' : 'service-worker'}` });
      if (session.paused) evidence.pausedSessions++;
      if (p.targetInfo.url !== `${origin}/${targetType === 'shared_worker' ? 'shared-worker' : 'service-worker'}`) {
        // Fresh Chrome can own internal service workers too. Never claim their setup as fixture coverage.
        evidence.ignoredNonFixtureTargets++;
        queue(resume(session).catch(() => {}));
        return;
      }
      if (targets.has(p.targetInfo.targetId)) {
        evidence.duplicateSessions++;
        queue(resume(session).catch(() => {}));
      } else {
        targets.set(p.targetInfo.targetId, session);
        evidence.authoritativeTargets++;
        queue(setup(session));
      }
      return;
    }
    if (![...targets.values()].some(session => session.id === event.sessionId)) return;
    if (!['Network.requestWillBeSent', 'Network.responseReceived',
      'Network.requestWillBeSentExtraInfo', 'Network.responseReceivedExtraInfo'].includes(event.method)) return;
    if (typeof p.requestId !== 'string' || !p.requestId || p.requestId.length > 256) {
      unavailable = true; return;
    }
    const key = `${event.sessionId}:${p.requestId}`;
    if (event.method === 'Network.requestWillBeSent') {
      if (p.request?.url === `${origin}/reader-rotate`) {
        requests.set(key, true); evidence.rotateRequests++;
        if (earlyExtraInfo.has(key)) applyExtraInfo(earlyExtraInfo.get(key));
      }
      earlyExtraInfo.delete(key);
    } else if (event.method === 'Network.requestWillBeSentExtraInfo' || event.method === 'Network.responseReceivedExtraInfo') {
      // CDP permits ExtraInfo before the identifying request event. Retain only categories/booleans.
      const metadata = {
        requestExtraInfo: event.method === 'Network.requestWillBeSentExtraInfo',
        responseExtraInfo: event.method === 'Network.responseReceivedExtraInfo',
        responseSetCookie: event.method === 'Network.responseReceivedExtraInfo' && hasSetCookie(p.headers),
      };
      if (requests.has(key)) applyExtraInfo(metadata);
      else if (earlyExtraInfo.has(key)) {
        const pending = earlyExtraInfo.get(key);
        for (const field of Object.keys(metadata)) pending[field] ||= metadata[field];
      } else if (earlyExtraInfo.size < maxEarlyExtraInfo) earlyExtraInfo.set(key, metadata);
      else { evidence.earlyMetadataOverflow = true; unavailable = true; }
    } else if (requests.has(key) && event.method === 'Network.responseReceived') {
      evidence.rotateResponses++;
      evidence.firstRotateStatus ??= p.response?.status ?? null;
      evidence.responseHasExtraInfo ||= p.hasExtraInfo === true;
      evidence.responseSetCookie ||= hasSetCookie(p.response?.headers);
    }
  }, () => { unavailable = true; });
  try {
    // One root owner, one authority per worker. Pages are deliberately excluded.
    await browser.send('Target.setAutoAttach', {
      autoAttach: true, waitForDebuggerOnStart: true, flatten: true,
      filter: [{ type: targetType, exclude: false }, { exclude: true }],
    });
  } catch (error) { unsubscribe(); throw error; }
  return {
    async settle() { await Promise.allSettled([...tasks]); },
    status() {
      return { ...evidence, setupErrors: [...evidence.setupErrors], unavailable,
        pendingEarlyMetadata: earlyExtraInfo.size,
        targetLifecycle: evidence.targetLifecycle.map(event => ({ ...event })),
        readyTargets: [...targets.values()].filter(session => session.ready && !session.detached).length };
    },
    async close() {
      if (stopped) return;
      stopped = true;
      await browser.send('Target.setAutoAttach', { autoAttach: false, waitForDebuggerOnStart: false, flatten: true });
      await Promise.allSettled([...tasks]);
      unsubscribe();
      earlyExtraInfo.clear();
    },
  };
}
