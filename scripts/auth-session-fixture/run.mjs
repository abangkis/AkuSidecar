// Explicit fixture-only experiment. No production config, profiles, tokens or APIs are read.
import { mkdtemp, mkdir, realpath, rm } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { launchFixtureChrome } from './chrome.mjs';
import { startFixture } from './fixture.mjs';
import { AuthSessionBroker } from './broker.mjs';

const sidecar = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const args = process.argv.slice(2);
if (args.length !== 3 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' || !isAbsolute(args[2])) {
  console.error('Usage: node scripts/auth-session-fixture/run.mjs --run-local-fixture --chrome <absolute chrome.exe>');
  process.exit(2);
}
const chromePath = await realpath(args[2]);
const parent = join(sidecar, 'runtime', 'qa', 'auth-session-fixture');
await mkdir(parent, { recursive: true });
const root = await mkdtemp(join(parent, 'run-'));
const receipt = { scope: 'local_fixture_only', startedAt: new Date().toISOString(), checks: [], realAuthTested: false };
let stage = 'start', fixture, owner, ownerPage, collector, broker;
const children = [];
const check = (condition, name) => { stage = name; if (!condition) throw new Error('fixture_assertion'); receipt.checks.push(name); };
async function start(role) { const c = await launchFixtureChrome(chromePath, root, role); children.push(c); return c; }
function makeBroker() {
  return new AuthSessionBroker({
    fixtureOrigins: [fixture.origin],
    owner: {
      async probe() { const r = await ownerPage.request('/probe'); return { authenticated: r.status === 200, identity: { accountId: r.body.account } }; },
      exportCookies: () => owner.send('Storage.getCookies'),
    },
    derived: {
      async create() {
        const browser = collector;
        const { browserContextId } = await browser.send('Target.createBrowserContext', { disposeOnDetach: true });
        try { return { browser, browserContextId, page: await browser.page(fixture.origin, browserContextId) }; }
        catch (error) { await browser.send('Target.disposeBrowserContext', { browserContextId }); throw error; }
      },
      seed: ({ context, cookies }) => context.browser.send('Storage.setCookies', { cookies, browserContextId: context.browserContextId }),
      async probe({ context }) { const r = await context.page.request('/probe'); return { authenticated: r.status === 200, identity: { accountId: r.body.account } }; },
      dispose: ({ context }) => context.browser.isExited() ? Promise.resolve() : context.browser.send('Target.disposeBrowserContext', { browserContextId: context.browserContextId }),
    },
  });
}
const current = () => broker.getContext(broker.lease());
try {
  fixture = await startFixture();
  stage = 'launch_two_browsers'; owner = await start('owner'); collector = await start('collector');
  receipt.chrome = owner.product;
  check(owner.pid !== collector.pid && owner.profile !== collector.profile, 'distinct_processes_and_profiles');
  stage = 'create_owner_page'; ownerPage = await owner.page(fixture.origin);
  const blankContext = await collector.send('Target.createBrowserContext');
  const blankPage = await collector.page(fixture.origin, blankContext.browserContextId);
  check((await blankPage.request('/probe')).status === 401, 'isolated_context_has_no_implicit_login');
  await collector.send('Target.disposeBrowserContext', blankContext);
  stage = 'fixture_login'; check((await ownerPage.request('/login', 'POST')).status === 200, 'owner_login');
  await owner.send('Storage.setCookies', { cookies: [
    { name: 'fixture_strict', value: 'synthetic-strict', domain: '127.0.0.1', path: '/', secure: true, httpOnly: true, sameSite: 'Strict', expires: Math.floor(Date.now()/1000)+3600 },
    { name: 'fixture_scoped', value: 'synthetic-path', domain: '127.0.0.1', path: '/scoped', secure: true, sameSite: 'Lax' },
    { name: 'fixture_partition', value: 'synthetic-partition', domain: '127.0.0.1', path: '/', secure: true, httpOnly: true, sameSite: 'None', partitionKey: { topLevelSite: 'http://127.0.0.1', hasCrossSiteAncestor: false } },
    { name: 'fixture_other_partition', value: 'synthetic-other', domain: '127.0.0.1', path: '/', secure: true, sameSite: 'None', partitionKey: { topLevelSite: 'http://localhost', hasCrossSiteAncestor: false } },
    { name: 'fixture_expired', value: 'synthetic-expired', domain: '127.0.0.1', path: '/', secure: true, expires: Math.floor(Date.now()/1000)-60 },
  ] });
  broker = makeBroker(); stage = 'bootstrap';
  check((await broker.bootstrap({ origin: fixture.origin, expectedAccountId: 'alpha' })).ok, 'bootstrap_same_account');
  const firstLease = broker.lease();
  const [reader, collected] = await Promise.all([ownerPage.request('/reader'), current().page.request('/collect')]);
  check(reader.status === 200 && collected.status === 200 && reader.body.account === collected.body.account && fixture.stats().overlap >= 2 && fixture.stats().logins === 1 && broker.admitResult(firstLease), 'concurrent_reader_collector_one_login');
  const sourceCookies = (await owner.send('Storage.getCookies')).cookies;
  const derivedCookies = (await collector.send('Storage.getCookies', { browserContextId: current().browserContextId })).cookies;
  check(sourceCookies.length === derivedCookies.length && sourceCookies.length === 5, 'cookie_count_and_expired_exclusion');
  const fields = ['name', 'value', 'domain', 'path', 'secure', 'httpOnly', 'sameSite', 'expires', 'session', 'partitionKey'];
  const canonical = values => values.map(c => JSON.stringify(Object.fromEntries(fields.map(k => [k, c[k]])))).sort().join('\n');
  check(canonical(sourceCookies) === canonical(derivedCookies), 'cookie_attributes_and_partition_keys_roundtrip');
  const names = (await current().page.request('/cookie-evidence')).body.names;
  const scoped = (await current().page.request('/scoped/cookie-evidence')).body.names;
  check(names.includes('fixture_partition') && !names.includes('fixture_other_partition') && !names.includes('fixture_scoped') && scoped.includes('fixture_scoped'), 'partition_and_path_network_isolation');
  const visible = await current().page.evaluate('document.cookie');
  check(!visible.includes('fixture_session=') && !visible.includes('fixture_strict=') && !visible.includes('fixture_partition='), 'httponly_not_visible_to_page_script');

  stage = 'collector_restart'; await broker.revoke(); await collector.close(); collector = await start('collector'); broker = makeBroker();
  check((await broker.bootstrap({ origin: fixture.origin, expectedAccountId: 'alpha' })).ok && fixture.stats().logins === 1, 'collector_restart_reseed_without_login');
  const preBrokerLease = broker.lease(); await broker.revoke();
  check(!broker.admitResult(preBrokerLease), 'old_broker_lease_rejected');
  broker = makeBroker();
  check((await broker.bootstrap({ origin: fixture.origin })).ok && fixture.stats().logins === 1 && !broker.admitResult(preBrokerLease), 'fresh_broker_reseed_without_saved_snapshot');
  stage = 'owner_restart'; await owner.close();
  check((await current().page.request('/collect')).status === 200, 'collector_survives_owner_process_exit');
  owner = await start('owner'); ownerPage = await owner.page(fixture.origin);
  check((await ownerPage.request('/probe')).status === 200 && fixture.stats().logins === 1, 'owner_profile_restart_preserves_fixture_login');

  stage = 'owner_rotation'; const oldLease = broker.lease();
  await ownerPage.request('/rotate', 'POST');
  check((await current().page.request('/probe')).status === 401, 'owner_rotation_invalidates_derived_session');
  const episode = broker.markStale(oldLease);
  check(!broker.admitResult(oldLease), 'stale_result_rejected');
  check((await broker.recoverStale({ episode })).ok && fixture.stats().logins === 1, 'one_reseed_recovers_owner_rotation');
  check(!(await broker.recoverStale({ episode })).ok, 'same_reseed_episode_cannot_repeat');

  stage = 'derived_rotation_conflict';
  await current().page.request('/rotate', 'POST');
  check((await ownerPage.request('/probe')).status === 401, 'derived_refresh_can_invalidate_reader');
  const conflictEpisode = broker.markStale(broker.lease());
  check(!(await broker.recoverStale({ episode: conflictEpisode })).ok && broker.status().state === 'NEEDS_USER', 'refresh_conflict_stops_without_reverse_sync');

  stage = 'logout_and_switch';
  await ownerPage.request('/login', 'POST');
  check((await broker.bootstrap({ origin: fixture.origin, expectedAccountId: 'alpha' })).ok, 'explicit_fixture_relogin');
  const beforeLogout = broker.lease();
  const delayed = current().page.request('/collect').catch(() => null);
  await ownerPage.request('/logout', 'POST');
  const revocation = broker.revoke();
  check(!broker.admitResult(beforeLogout), 'logout_immediately_fences_old_results');
  check((await revocation).ok, 'revocation_cleanup_acknowledged'); await delayed;
  check((await collector.send('Target.getBrowserContexts')).browserContextIds.length === 0, 'revocation_disposes_derived_context');
  await ownerPage.request('/login?account=beta', 'POST');
  check((await broker.bootstrap({ origin: fixture.origin, expectedAccountId: 'beta' })).ok && (await current().page.request('/probe')).body.account === 'beta' && !broker.admitResult(beforeLogout), 'account_switch_does_not_accept_old_account_results');

  stage = 'last_owner_window';
  const { targetInfos } = await owner.send('Target.getTargets');
  for (const target of targetInfos.filter(t => t.type === 'page')) await owner.send('Target.closeTarget', { targetId: target.targetId }).catch(() => {});
  await delay(1500);
  receipt.ownerAliveAfterLastWindowClosed = !owner.isExited();
  check((await current().page.request('/collect')).status === 200, 'reader_window_close_does_not_stop_collector');
  receipt.status = 'passed';
} catch (error) {
  receipt.status = 'failed'; receipt.failedStage = stage; process.exitCode = 1;
  if (/^fixture_[a-zA-Z0-9_.]+$/.test(error.message || '')) receipt.errorCode = error.message;
} finally {
  if (broker) {
    const revoked = await broker.revoke().catch(() => ({ ok: false }));
    if (!revoked.ok) { receipt.brokerCleanupFailed = true; process.exitCode = 1; }
  }
  const cleanup = await Promise.allSettled(children.map(child => child.close()));
  receipt.allBrowserRootsExited = children.every(child => child.isExited());
  receipt.cleanupSucceeded = cleanup.every(item => item.status === 'fulfilled') && receipt.allBrowserRootsExited;
  if (fixture) { receipt.fixtureStats = fixture.stats(); await fixture.close(); }
  // No evidence archive: remove only this freshly generated disposable fixture tree.
  // Resolved path validation is mandatory before recursive deletion on Windows.
  const resolvedParent = await realpath(parent), resolvedRoot = await realpath(root);
  const rel = relative(resolvedParent, resolvedRoot);
  if (receipt.cleanupSucceeded && !isAbsolute(rel) && /^run-[^\\/]+$/.test(rel)) {
    try { await rm(resolvedRoot, { recursive: true, maxRetries: 3, retryDelay: 150 }); receipt.profilesRemoved = true; }
    catch { receipt.profilesRemoved = false; process.exitCode = 1; }
  } else { receipt.profilesRemoved = false; process.exitCode = 1; }
  if (process.exitCode && receipt.status === 'passed') receipt.status = 'cleanup_failed';
  receipt.finishedAt = new Date().toISOString();
  const output = JSON.stringify(receipt, null, 2);
  fixture?.assertSanitized(output);
  console.log(output);
}
