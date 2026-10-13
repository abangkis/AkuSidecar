// Local-only execution fallback. All browser handles/profiles belong to this run.
import { mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { startFixture } from './fixture.mjs';
import { launchFixtureChrome } from './chrome.mjs';
import { fixtureCookieCheckpoint } from './worker-capability-probe.mjs';
import { FixtureCaptureQueue } from './capture-queue.mjs';
import { ExclusiveFallbackController } from './exclusive-fallback.mjs';

const args = process.argv.slice(2);
if (args.length !== 3 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' || !isAbsolute(args[2])) {
  console.error('Usage: node run-exclusive-fallback.mjs --run-local-fixture --chrome <absolute chrome.exe>');
  process.exit(2);
}
const executable = await realpath(args[2]);
const project = await realpath(resolve(dirname(fileURLToPath(import.meta.url)), '../..'));
const parent = join(project, 'runtime/qa/auth-session-fixture');
await mkdir(parent, { recursive: true });
const base = await realpath(parent);
if (relative(project, base).replaceAll('\\', '/') !== 'runtime/qa/auth-session-fixture') throw new Error('fixture_output_scope_invalid');
const root = await mkdtemp(join(base, 'run-exclusive-fallback-'));
const VERIFIED = { cookieObservation: 'verified', startupCoverage: 'verified', ownership: 'verified' };
const CHECKPOINT = { ...VERIFIED, cookieObservation: 'checkpoint_only' };
const receipt = { scope: 'owned_local_exclusive_execution_fixture', startedAt: new Date().toISOString(),
  realAuthTested: false, capabilitiesAreFixtureInputs: true, scenarios: [] };
let allExited = true;

function deferred() { let resolve; const promise = new Promise(r => { resolve = r; }); return { promise, resolve }; }

async function scenario(name, capabilities, body) {
  const entry = { name, checks: [] }; receipt.scenarios.push(entry);
  const subroot = join(root, name); await mkdir(subroot);
  const fixture = await startFixture();
  const browsers = [], readerTargets = [], snapshots = new Map(), launching = new Set();
  let reader, ownerPage, collector, collectorPage, controller, stage = 'launch';
  let launches = 0, pauses = 0, releases = [], unknownLaunchExit = false;
  const ownedLaunch = async role => {
    const attempt = launchFixtureChrome(executable, subroot, role);
    launching.add(attempt);
    try { const browser = await attempt; browsers.push(browser); return browser; }
    catch { unknownLaunchExit = true; throw new Error('fixture_launch_failed'); }
    finally { launching.delete(attempt); }
  };
  const check = (ok, label) => { stage = label; if (!ok) throw new Error('fixture_assertion'); entry.checks.push(label); };
  const inventory = async () => {
    const { targetInfos } = await reader.send('Target.getTargets');
    if (!Array.isArray(targetInfos)) throw new Error('fixture_inventory_invalid');
    const present = new Set(targetInfos.map(t => t.targetId));
    return readerTargets.filter(t => present.has(t.targetId));
  };
  const addReader = async () => {
    const page = await reader.page(fixture.origin);
    const { windowId } = await reader.send('Browser.getWindowForTarget', { targetId: page.targetId });
    if (!Number.isInteger(windowId)) throw new Error('fixture_window_invalid');
    const record = { targetId: page.targetId, windowId };
    readerTargets.push(record); return record;
  };
  const verifySession = async () => {
    const probe = await ownerPage.request('/probe');
    if (probe.status !== 200) return { authenticated: false, accountId: null, revision: null };
    const { cookies } = await reader.send('Storage.getCookies');
    const revision = fixtureCookieCheckpoint(cookies);
    snapshots.set(revision, { cookies, accountId: probe.body.account });
    return { authenticated: true, accountId: probe.body.account, revision };
  };
  const pauseCollector = async () => {
    await Promise.allSettled([...launching]);
    if (collector && !collector.isExited()) { await collector.close(); pauses++; }
    return { stopped: !collector || collector.isExited() };
  };
  try {
    reader = await ownedLaunch('owner');
    receipt.chrome = reader.product;
    ownerPage = await reader.page(fixture.origin);
    check((await ownerPage.request('/login', 'POST')).status === 200, 'invented_initial_login');
    controller = new ExclusiveFallbackController({ expectedAccountId: 'alpha', capabilities, timeoutMs: 2000,
      adapters: {
        readerCount: async () => (await inventory()).length,
        openReader: async () => { if (!(await inventory()).length) await addReader(); },
        verifySession, pauseCollector,
        resumeCollector: async verification => {
          const current = await verifySession();
          if (!current.authenticated || current.accountId !== verification.accountId || current.revision !== verification.revision) throw new Error('fixture_session_changed');
          const snapshot = snapshots.get(verification.revision);
          if (!snapshot || snapshot.accountId !== 'alpha') throw new Error('fixture_session_unverified');
          if (!collector || collector.isExited()) {
            collector = await ownedLaunch('collector'); launches++;
            const { browserContextId } = await collector.send('Target.createBrowserContext', { disposeOnDetach: true });
            await collector.send('Storage.setCookies', { browserContextId, cookies: snapshot.cookies });
            collectorPage = await collector.page(fixture.origin, browserContextId);
          }
          const probe = await collectorPage.request('/probe');
          return { ready: probe.status === 200, accountId: probe.body.account, revision: current.revision };
        },
      },
    });
    check((await controller.start()).state === 'READY', 'collector_initialized_after_session_verification');
    const queue = new FixtureCaptureQueue({ session: controller });
    const actorClose = async record => {
      // Explicit fixture stand-in for the user's close action, outside controller.
      if (!readerTargets.includes(record)) throw new Error('fixture_close_scope_invalid');
      await reader.send('Target.closeTarget', { targetId: record.targetId });
      const deadline = Date.now() + 2000;
      while ((await inventory()).some(t => t.targetId === record.targetId)) {
        if (Date.now() >= deadline) throw new Error('fixture_close_not_confirmed');
        await new Promise(resolve => setTimeout(resolve, 25));
      }
    };
    stage = 'scenario_body';
    await body({ entry, controller, queue, check, fixture, ownerPage, reader, inventory, addReader, actorClose,
      collectorRead: () => collectorPage.request('/collect'), collectorExited: () => collector.isExited(),
      launches: () => launches, pauses: () => pauses,
      heldRead: () => {
        const began = deferred(), gate = deferred(); releases.push(gate.resolve);
        let calls = 0;
        return { began: began.promise, release: gate.resolve, calls: () => calls,
          read: async () => { const page = collectorPage; if (++calls === 1) { began.resolve(); await gate.promise; } return page.request('/collect'); } };
      },
    });
    entry.status = 'passed';
  } catch (error) {
    entry.status = 'failed'; entry.failedStage = stage;
    if (/^fixture_[A-Za-z0-9_.]+$/.test(error.message ?? '')) entry.code = error.message;
    process.exitCode = 1;
  } finally {
    for (const release of releases) release();
    entry.controller = controller?.status();
    if (controller) {
      try { const stopped = await controller.stop(); if (stopped.cleanupPending || stopped.code) { entry.controllerStopFailed = true; process.exitCode = 1; } }
      catch { entry.controllerStopFailed = true; process.exitCode = 1; }
    }
    await Promise.allSettled([...launching]);
    const cleanup = await Promise.allSettled(browsers.map(browser => browser.close()));
    entry.browsersExited = !unknownLaunchExit && browsers.length > 0 && browsers.every(browser => browser.isExited());
    entry.cleanupSucceeded = cleanup.every(r => r.status === 'fulfilled') && entry.browsersExited;
    allExited &&= entry.browsersExited;
    if (!entry.cleanupSucceeded) process.exitCode = 1;
    entry.collectorLaunches = launches; entry.collectorPauses = pauses;
    entry.fixtureStats = fixture.stats(); await fixture.close();
    fixture.assertSanitized(JSON.stringify(entry));
  }
}

try {
  await scenario('fallback-during-batch', VERIFIED, async h => {
    const held = h.heldRead();
    h.queue.enqueueRead({ id: 'first', read: held.read });
    h.queue.enqueueRead({ id: 'second', read: h.collectorRead });
    const active = h.queue.run('first'); await held.began;
    await h.controller.requestReader(); await h.addReader();
    const before = await h.inventory(), pid = h.reader.pid;
    h.check(before.length === 2, 'two_fixture_reader_windows_open');
    const fallback = h.controller.setCapabilities(CHECKPOINT);
    h.check(!h.controller.lease('collector'), 'fallback_fences_new_batch_admission_synchronously');
    h.check((await h.queue.run('second')).state === 'pending', 'unstarted_batch_work_retained');
    held.release(); await active; await fallback;
    h.check(h.queue.status('first').state === 'pending' && h.queue.result('first') === null, 'old_generation_result_rejected_and_retained');
    h.check(h.collectorExited(), 'owned_headless_collector_stopped');
    const after = await h.inventory();
    const sameWindows = after.length === before.length && await Promise.all(before.map(async t =>
      (await h.reader.send('Browser.getWindowForTarget', { targetId: t.targetId })).windowId === t.windowId)).then(v => v.every(Boolean));
    h.check(sameWindows && h.reader.pid === pid && !h.reader.isExited(), 'reader_process_targets_and_windows_preserved');
    await h.controller.setCapabilities(VERIFIED);
    h.check(h.controller.status().mode === 'exclusive', 'fallback_does_not_flap_on_capability_upgrade');
    await h.actorClose(before[0]); await h.controller.readerClosed();
    h.check(!h.controller.lease('collector') && h.launches() === 1, 'remaining_reader_window_keeps_batch_paused');
    await h.actorClose(before[1]); await h.controller.readerClosed();
    h.check(h.controller.status().state === 'READY' && h.launches() === 2, 'last_user_close_revalidates_session_and_restarts_collector');
    await h.queue.resume();
    h.check(h.queue.status('first').state === 'completed' && h.queue.status('second').state === 'completed', 'retained_batch_completes_after_resume');
    const calls = held.calls(); await h.queue.resume();
    h.check(calls === 2 && held.calls() === calls && h.queue.status('first').attempts === 2, 'one_fresh_retry_and_no_duplicate_publication');
    h.check(h.fixture.stats().logins === 1, 'fallback_requires_no_second_login');
  });
  if (!process.exitCode) await scenario('reader-priority-exclusive', CHECKPOINT, async h => {
    const held = h.heldRead(); h.queue.enqueueRead({ id: 'first', read: held.read });
    const active = h.queue.run('first'); await held.began;
    const requested = h.controller.requestReader();
    h.check(!h.controller.lease('collector'), 'reader_request_fences_batch_before_waiting');
    held.release(); await active; await requested;
    h.check((await h.inventory()).length === 1 && h.collectorExited(), 'reader_opens_after_owned_collector_pause');
    h.check(h.queue.status('first').state === 'pending' && h.queue.result('first') === null, 'interrupted_batch_remains_pending');
    const [window] = await h.inventory(); await h.actorClose(window); await h.controller.readerClosed(); await h.queue.resume();
    h.check(h.queue.status('first').state === 'completed' && held.calls() === 2, 'batch_resumes_after_user_close');
    h.check(h.fixture.stats().logins === 1, 'reader_priority_preserves_initial_login');
  });
  if (!process.exitCode) await scenario('changed-account-blocks-resume', CHECKPOINT, async h => {
    await h.controller.requestReader();
    h.queue.enqueueRead({ id: 'blocked', read: h.collectorRead });
    h.check((await h.queue.run('blocked')).state === 'pending', 'paused_job_retained_before_identity_change');
    await h.ownerPage.request('/login?account=beta', 'POST');
    const [window] = await h.inventory(); await h.actorClose(window); await h.controller.readerClosed();
    h.check(h.controller.status().state === 'NEEDS_USER' && !h.controller.lease('collector'), 'different_account_denies_resume');
    h.check(h.collectorExited() && h.launches() === 1 && h.queue.result('blocked') === null, 'no_headless_restart_or_result_on_account_mismatch');
  });
  if (!process.exitCode) await scenario('lost-login-blocks-resume', CHECKPOINT, async h => {
    await h.controller.requestReader(); await h.ownerPage.request('/logout', 'POST');
    const [window] = await h.inventory(); await h.actorClose(window); await h.controller.readerClosed();
    h.check(h.controller.status().state === 'NEEDS_USER' && !h.controller.lease('collector'), 'lost_login_denies_resume');
    h.check(h.collectorExited() && h.launches() === 1, 'lost_login_never_silently_restarts_collector');
  });
} catch { process.exitCode = 1; }
finally {
  const target = await realpath(root), rel = relative(base, target), projectRel = relative(project, target);
  if (allExited && !isAbsolute(projectRel) && !projectRel.startsWith('..') && /^run-exclusive-fallback-[^\\/]+$/.test(rel)) {
    try { await rm(target, { recursive: true, maxRetries: 2, retryDelay: 100 }); receipt.profilesRemoved = true; }
    catch { receipt.profilesRemoved = false; process.exitCode = 1; }
  } else { receipt.profilesRemoved = false; process.exitCode = 1; }
  receipt.browsersExited = allExited; receipt.status = process.exitCode ? 'failed' : 'passed';
  receipt.finishedAt = new Date().toISOString(); console.log(JSON.stringify(receipt, null, 2));
}
