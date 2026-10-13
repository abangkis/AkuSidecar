// Opt-in local-only capability experiment; no observer policy integration or archives.
import { mkdir, mkdtemp, realpath, rm, readdir } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { startFixture } from './fixture.mjs';
import { launchFixtureChrome } from './chrome.mjs';
import { createWorkerCapabilityProbe, fixtureCookieCheckpoint } from './worker-capability-probe.mjs';

const args = process.argv.slice(2);
if (args.length !== 3 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' || !isAbsolute(args[2])) {
  console.error('Usage: node run-worker-capabilities.mjs --run-local-fixture --chrome <absolute chrome.exe>');
  process.exit(2);
}
const executable = await realpath(args[2]);
const selectedWorker = process.env.AKU_FIXTURE_WORKER ?? 'all';
if (!['all', 'service_worker', 'shared_worker'].includes(selectedWorker)) throw new Error('fixture_invalid_case');
const parent = resolve(dirname(fileURLToPath(import.meta.url)), '../../runtime/qa/auth-session-fixture');
await mkdir(parent, { recursive: true });
const root = await mkdtemp(join(parent, 'run-worker-capabilities-'));
const receipt = { scope: 'fresh_owned_local_fixture_workers_only', startedAt: new Date().toISOString(), realAuthTested: false, scenarios: [] };
let allExited = true, allCleanupSucceeded = true;
const registration = `window.fixtureRegistrationResult = null;
  navigator.serviceWorker.register('/service-worker').then(() => navigator.serviceWorker.ready)
    .then(reg => { window.fixtureRegistration = reg; window.fixtureRegistrationResult = {registered:true, active:reg.active?.state === 'activated'}; })
    .catch(() => { window.fixtureRegistrationResult = {registered:false, active:false}; }); true`;
async function ready(page) {
  let result;
  for (let i = 0; i < 240; i++) {
    // ready may resolve during activation. Inspect live state rather than its old snapshot.
    result = await page.evaluate(`window.fixtureRegistrationResult && ({
      ...window.fixtureRegistrationResult, active:window.fixtureRegistration?.active?.state === 'activated'
    })`);
    if (result?.active || result?.registered === false) return result;
    await delay(50);
  }
  return { registered: result?.registered === true, active: false, timedOut: true };
}
async function scenario(name, targetType, resumeOrder) {
  const entry = { name, checks: [] }; receipt.scenarios.push(entry);
  const subroot = join(root, name); await mkdir(subroot);
  const fixture = await startFixture();
  let browser, observer, stage = 'launch';
  const check = (ok, label) => { stage = label; if (!ok) throw new Error('fixture_assertion'); entry.checks.push(label); };
  try {
    browser = await launchFixtureChrome(executable, subroot, 'collector');
    receipt.chrome = browser.product;
    const page = await browser.page(fixture.origin);
    check((await page.request('/login', 'POST')).status === 200, 'invented_owner_login');
    if (targetType) observer = await createWorkerCapabilityProbe(browser, { targetType, origin: fixture.origin, resumeOrder });
    if (!targetType || targetType === 'service_worker') {
      stage = 'registration'; await page.evaluate(registration);
      entry.registration = await ready(page);
      if (observer) await observer.settle();
      check(entry.registration.registered && entry.registration.active, 'registration_and_activation_complete');
      if (observer) check(observer.status().readyTargets === 1, 'one_authoritative_worker_network_ready');
    }
    if (!targetType) { entry.status = 'passed'; return entry; }
    const cookies = (await browser.send('Storage.getCookies')).cookies;
    const before = fixtureCookieCheckpoint(cookies);
    const { browserContextId } = await browser.send('Target.createBrowserContext', { disposeOnDetach: true });
    await browser.send('Storage.setCookies', { browserContextId, cookies });
    const derived = await browser.page(fixture.origin, browserContextId);
    check((await derived.request('/probe')).body.account === 'alpha', 'derived_initial_identity_matches');
    stage = 'worker_rotation';
    const expression = targetType === 'shared_worker'
      ? `new Promise(resolve => { window.fixtureShared = new SharedWorker('/shared-worker'); fixtureShared.port.onmessage=e=>resolve(e.data); fixtureShared.onerror=()=>resolve({status:0}); fixtureShared.port.start(); fixtureShared.port.postMessage('rotate'); })`
      : `new Promise(resolve => { const channel=new MessageChannel(); channel.port1.onmessage=e=>resolve(e.data); fixtureRegistration.active.postMessage('rotate', [channel.port2]); })`;
    const rotation = await page.evaluate(expression);
    await observer.settle();
    check(rotation.status === 200 && fixture.stats().rotations === 1, 'worker_first_rotation_request_succeeds');
    const afterCookies = (await browser.send('Storage.getCookies')).cookies;
    entry.cookieCheckpointChanged = before !== fixtureCookieCheckpoint(afterCookies);
    check(entry.cookieCheckpointChanged, 'storage_checkpoint_detects_cookie_change');
    check((await page.request('/probe')).body.account === 'alpha', 'owner_identity_preserved_after_rotation');
    check((await derived.request('/probe')).status === 401, 'unreseeded_derived_cookie_is_invalid');
    await browser.send('Storage.clearCookies', { browserContextId });
    await browser.send('Storage.setCookies', { browserContextId, cookies: afterCookies });
    check((await derived.request('/probe')).body.account === 'alpha', 'checkpoint_reseed_restores_same_account');
    check((await page.request('/login?account=beta', 'POST')).body.account === 'beta', 'explicit_synthetic_account_switch');
    const switchedCookies = (await browser.send('Storage.getCookies')).cookies;
    check(fixtureCookieCheckpoint(switchedCookies) !== fixtureCookieCheckpoint(afterCookies), 'account_switch_changes_checkpoint');
    await browser.send('Storage.clearCookies', { browserContextId });
    await browser.send('Storage.setCookies', { browserContextId, cookies: switchedCookies });
    check((await derived.request('/probe')).body.account === 'beta', 'reseed_account_switch_identity_verified');
    await browser.send('Target.disposeBrowserContext', { browserContextId });
    await page.networkBarrier(); await delay(100);
    entry.network = observer.status();
    entry.firstRotationNetworkObserved = entry.network.rotateRequests === 1 && entry.network.firstRotateStatus === 200;
    entry.rotationSetCookieEvidence = entry.network.responseSetCookie;
    entry.checkpointCapability = 'supported_at_explicit_checkpoint';
    entry.networkMutationEvidence = entry.rotationSetCookieEvidence ? 'observed_for_commanded_rotation' : 'not_observed_on_worker_session';
    check(entry.firstRotationNetworkObserved, 'first_rotation_network_request_and_response_observed');
    check(!entry.network.unavailable && entry.network.readyTargets === 1 && entry.network.duplicateSessions === 0, 'single_active_worker_connection_no_duplicates');
    entry.status = 'passed';
  } catch (error) {
    entry.status = 'failed'; entry.failedStage = stage;
    if (/^fixture_[A-Za-z0-9_.]+$/.test(error.message ?? '')) entry.errorCode = error.message;
  } finally {
    if (observer) {
      entry.network = observer.status();
      await observer.close().catch(() => { entry.observerCleanupFailed = true; });
    }
    if (browser) await browser.close().catch(() => { entry.browserCleanupFailed = true; });
    // A launch error without a returned owner handle leaves exit status unknown.
    // Preserve its disposable root rather than infer that no child was created.
    entry.browserExited = !!browser && browser.isExited(); allExited &&= entry.browserExited;
    allCleanupSucceeded &&= !entry.observerCleanupFailed && !entry.browserCleanupFailed && entry.browserExited;
    entry.fixtureStats = fixture.stats(); await fixture.close();
    fixture.assertSanitized(JSON.stringify(entry));
  }
  return entry;
}

try {
  const baseline = selectedWorker !== 'shared_worker' ? await scenario('service-worker-baseline', null) : null;
  if (baseline?.status === 'passed') {
    const paused = await scenario('service-worker-network-before-resume', 'service_worker', 'after-network');
    // One causal correction only: a single session still stalled while paused.
    if (paused.status !== 'passed' && paused.network?.setupErrors.includes('fixture_cdp_Network.enable_timeout')) {
      await scenario('service-worker-resume-before-network', 'service_worker', 'before-network');
    }
  }
  if (selectedWorker !== 'service_worker') await scenario('shared-worker-single-root', 'shared_worker', 'after-network');
} catch { receipt.runnerFailed = true; }
finally {
  const target = await realpath(root), base = await realpath(parent), rel = relative(base, target);
  if (allExited && !isAbsolute(rel) && /^run-worker-capabilities-[^\\/]+$/.test(rel)) {
    try { await rm(target, { recursive: true }); receipt.profilesRemoved = true; }
    catch { receipt.profilesRemoved = false; } // Leave locked files untouched; no forced process termination.
  } else receipt.profilesRemoved = false;
  receipt.browsersExited = allExited;
  receipt.cleanupSucceeded = allCleanupSucceeded;
  receipt.remainingOwnedEntries = receipt.profilesRemoved ? 0 : (await readdir(root)).length;
  const service = receipt.scenarios.filter(s => s.name.startsWith('service-worker') && s.network);
  receipt.serviceWorkerCommandedRotationVerified = selectedWorker === 'shared_worker' ? null : service.some(s => s.status === 'passed');
  receipt.sharedWorkerCommandedRotationVerified = selectedWorker === 'service_worker' ? null : receipt.scenarios.some(s => s.name === 'shared-worker-single-root' && s.status === 'passed');
  receipt.status = receipt.serviceWorkerCommandedRotationVerified !== false && receipt.sharedWorkerCommandedRotationVerified !== false &&
    receipt.profilesRemoved && allExited && allCleanupSucceeded && !receipt.runnerFailed ? 'passed_with_capability_boundaries' : 'failed';
  if (receipt.status === 'failed') process.exitCode = 1;
  receipt.finishedAt = new Date().toISOString();
  console.log(JSON.stringify(receipt, null, 2));
}
