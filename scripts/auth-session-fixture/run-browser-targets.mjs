// Owned-browser target coverage experiment; all credentials and profiles are disposable.
import { mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { startFixture } from './fixture.mjs';
import { openScenario } from './scenario-context.mjs';
import { observeBrowserSessionMutations } from './browser-observer.mjs';

const args = process.argv.slice(2);
if (args.length !== 3 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' || !isAbsolute(args[2])) {
  console.error('Usage: node run-browser-targets.mjs --run-local-fixture --chrome <absolute chrome.exe>'); process.exit(2);
}
const executable = await realpath(args[2]);
const selectedCase = process.env.AKU_FIXTURE_CASE ?? '';
if (selectedCase && !['new-tab', 'dedicated-worker', 'shared-worker-coverage-boundary', 'service-worker-refresh-race', 'target-lifecycle'].includes(selectedCase)) throw new Error('fixture_invalid_case');
const parent = resolve(dirname(fileURLToPath(import.meta.url)), '../../runtime/qa/auth-session-fixture');
await mkdir(parent, { recursive: true });
const root = await mkdtemp(join(parent, 'run-targets-'));
const receipt = { scope: 'owned_fixture_browser_targets', startedAt: new Date().toISOString(), realAuthTested: false, scenarios: [] };
let allExited = true;
async function scenario(name, body) {
  if (selectedCase && selectedCase !== name) return;
  const entry = { name, checks: [] }; receipt.scenarios.push(entry);
  const subroot = join(root, name); await mkdir(subroot);
  const fixture = await startFixture();
  let context, stage = 'launch';
  const watchers = {}, events = [];
  const networkTrace = [], traceSessions = new Map(), traceIds = new Map(), traceSessionAliases = new Map();
  let stopTrace;
  const check = (ok, label) => { stage = label; if (!ok) throw new Error('fixture_assertion'); entry.checks.push(label); };
  try {
    context = await openScenario({ executable, root: subroot, origin: fixture.origin });
    receipt.chrome = context.chrome;
    check((await context.pages.reader.request('/login', 'POST')).status === 200, 'one_invented_login');
    check((await context.session.start('central-vault')).ok, 'session_initialized');
    stage = 'start_browser_observation';
    const traceStops = Object.entries(context.browsers).map(([browserRole, browser]) => browser.subscribeProtocol(event => {
      const p = event.params ?? {};
      if (event.method === 'Target.attachedToTarget') traceSessions.set(p.sessionId, p.targetInfo?.type ?? 'unknown');
      if (!['Network.requestWillBeSent', 'Network.responseReceived', 'Network.responseReceivedExtraInfo', 'Network.loadingFinished'].includes(event.method)) return;
      if (typeof p.requestId !== 'string' || networkTrace.length >= 160) return;
      const url = p.request?.url ?? p.response?.url;
      let urlScheme; try { if (url) urlScheme = new URL(url).protocol; } catch { urlScheme = 'invalid'; }
      if (!traceIds.has(p.requestId)) traceIds.set(p.requestId, traceIds.size + 1);
      if (!traceSessionAliases.has(event.sessionId)) traceSessionAliases.set(event.sessionId, traceSessionAliases.size + 1);
      networkTrace.push({ browserRole, requestIdAlias: traceIds.get(p.requestId), sessionAlias: traceSessionAliases.get(event.sessionId), targetType: traceSessions.get(event.sessionId) ?? 'existing_session', method: event.method, urlScheme, hasExtraInfo: p.hasExtraInfo, cookieHeader: Object.keys(p.headers ?? p.response?.headers ?? {}).some(k => k.toLowerCase() === 'set-cookie') });
    }));
    stopTrace = () => traceStops.forEach(stop => stop());
    for (const id of ['reader', 'collector']) {
      watchers[id] = await observeBrowserSessionMutations(context.browsers[id], {
        origin: fixture.origin, clientId: id,
        onUnexpected(signal) {
          events.push(signal);
          context.session.notifyExternalSessionChange(id, signal.kind === 'unexpected_session_mutation' ? 'mutation' : 'coverage_lost');
        },
      });
      context.clients[id].refresh = async () => {
        const result = await watchers[id].authorizeRefresh(context.pages[id], request => request('/rotate', 'POST'));
        if (result.status !== 200) throw new Error('fixture_refresh_rejected');
      };
    }
    await Promise.all(Object.values(watchers).map(w => w.flush()));
    check(context.session.status().ready && events.length === 0, 'existing_targets_covered_without_conflict');
    stage = 'scenario_body';
    await body({ context, fixture, watchers, events, check });
    check(fixture.stats().logins === 1, 'no_second_login');
    entry.status = 'passed';
  } catch (error) {
    entry.status = 'failed'; entry.failedStage = stage;
    if (/^fixture_[a-zA-Z0-9_.]+$/.test(error.message ?? '')) entry.code = error.message;
    entry.signals = events.map(({ kind, targetType, evidenceKind }) => ({ kind, targetType, evidenceKind }));
    if (error.observationStatus) entry.startupObservation = error.observationStatus;
    entry.networkTrace = networkTrace;
    if (error.fixtureBrowsersExited === false) allExited = false;
    process.exitCode = 1;
  } finally {
    stopTrace?.();
    entry.observation = Object.fromEntries(Object.entries(watchers).map(([id, w]) => [id, w.status()]));
    const stops = await Promise.allSettled(Object.values(watchers).map(w => w.close()));
    entry.watchersStopped = stops.every(r => r.status === 'fulfilled');
    if (context) {
      const cleanup = await context.close().catch(() => ({ ok: false, browsersExited: false }));
      allExited &&= cleanup.browsersExited;
      entry.cleanup = cleanup;
      if (!cleanup.ok) process.exitCode = 1;
    }
    if (!entry.watchersStopped) process.exitCode = 1;
    entry.fixtureStats = fixture.stats();
    await fixture.close();
    fixture.assertSanitized(JSON.stringify(entry));
  }
}

try {
  await scenario('new-tab', async ({ context, fixture, watchers, events, check }) => {
    check((await context.session.refresh()).ok, 'refresh_nonce_matches_only_own_target');
    const tab = await context.browsers.reader.page(fixture.origin);
    await watchers.reader.flush();
    check(context.session.status().ready, 'new_tab_adopted_automatically');
    check((await tab.request('/reader-rotate')).status === 200, 'new_tab_mutates_synthetic_session');
    await watchers.reader.flush();
    check(events.some(e => e.kind === 'unexpected_session_mutation' && e.targetType === 'page'), 'new_tab_network_mutation_observed');
    check(context.session.status().state === 'CONFLICT' && !context.session.lease('collector'), 'new_tab_fences_headless');
  });
  if (!process.exitCode) await scenario('dedicated-worker', async ({ context, watchers, events, check }) => {
    const result = await context.pages.reader.evaluate(`new Promise(resolve => {
      window.fixtureWorker = new Worker('/worker');
      fixtureWorker.onmessage = e => resolve(e.data);
      fixtureWorker.onerror = () => resolve({status:0});
    })`);
    check(result.status === 200, 'worker_first_request_rotates_session');
    await watchers.reader.flush();
    check(events.some(e => e.kind === 'unexpected_session_mutation' && e.targetType === 'worker'), 'worker_network_mutation_observed');
    check(context.session.status().state === 'CONFLICT' && !context.session.lease('collector'), 'worker_fences_headless');
  });
  if (!process.exitCode) await scenario('shared-worker-coverage-boundary', async ({ context, watchers, events, check }) => {
    const result = await context.pages.reader.evaluate(`new Promise(resolve => {
      window.fixtureShared = new SharedWorker('/shared-worker');
      fixtureShared.port.onmessage = e => resolve(e.data);
      fixtureShared.onerror = () => resolve({status:0});
      fixtureShared.port.start(); fixtureShared.port.postMessage('rotate');
    })`);
    check(result.status === 200, 'shared_worker_request_rotates_session');
    let rejected = false;
    try { await watchers.reader.flush(); } catch { rejected = true; }
    check(rejected && events.some(e => e.kind === 'observation_lost' && e.targetType === 'shared_worker'), 'unverified_shared_worker_coverage_is_explicit');
    check(context.session.status().code === 'SESSION_OBSERVATION_LOST' && !context.session.lease('collector'), 'unsupported_shared_worker_denies_session_use');
  });
  if (!process.exitCode) await scenario('service-worker-refresh-race', async ({ context, watchers, events, check }) => {
    let entered, release;
    const began = new Promise(r => { entered = r; }), gate = new Promise(r => { release = r; });
    const original = context.clients.collector.refresh;
    context.clients.collector.refresh = async () => { entered(); await gate; return original(); };
    const refresh = context.session.refresh();
    try {
      await Promise.race([began, refresh.then(() => { throw new Error('fixture_refresh_never_entered'); })]);
      check(await context.pages.reader.evaluate(`window.fixtureRegistration = navigator.serviceWorker.register('/service-worker').catch(()=>null); true`), 'service_worker_registration_requested_during_refresh');
      for (let i = 0; i < 100 && !events.some(e => e.targetType === 'service_worker'); i++) await delay(20);
      let rejected = false;
      try { await watchers.reader.flush(); } catch { rejected = true; }
      check(rejected && events.some(e => e.kind === 'observation_lost' && e.targetType === 'service_worker'), 'service_worker_coverage_boundary_reported');
      check(context.session.status().code === 'SESSION_OBSERVATION_LOST', 'service_worker_denies_inflight_refresh');
    } catch (error) {
      release(); await refresh;
      throw error;
    } finally { release(); }
    check(!(await refresh).ok && !context.session.lease('reader'), 'refresh_cannot_publish_after_coverage_loss');
  });
  if (!process.exitCode) await scenario('target-lifecycle', async ({ context, fixture, watchers, check }) => {
    const tab = await context.browsers.reader.page(fixture.origin);
    await watchers.reader.flush();
    await watchers.reader.retireTarget(tab.targetId);
    await watchers.reader.flush();
    check(watchers.reader.status().targetsByType.page.retired > 0, 'owned_tab_retirement_recorded');
    check(context.session.status().ready, 'confirmed_owned_target_close_preserves_remaining_coverage');
    await context.browsers.collector.close();
    check(!context.session.lease('reader') && context.session.status().code === 'SESSION_OBSERVATION_LOST', 'browser_disconnect_is_explicit_coverage_loss');
  });
} catch { process.exitCode = 1; }
finally {
  const target = await realpath(root), base = await realpath(parent), rel = relative(base, target);
  if (allExited && !isAbsolute(rel) && /^run-targets-[^\\/]+$/.test(rel)) {
    try { await rm(target, { recursive: true, maxRetries: 3, retryDelay: 150 }); receipt.profilesRemoved = true; }
    catch { receipt.profilesRemoved = false; process.exitCode = 1; }
  } else { receipt.profilesRemoved = false; process.exitCode = 1; }
  receipt.status = process.exitCode ? 'failed' : 'passed';
  receipt.browsersExited = allExited;
  receipt.finishedAt = new Date().toISOString();
  console.log(JSON.stringify(receipt, null, 2));
}
