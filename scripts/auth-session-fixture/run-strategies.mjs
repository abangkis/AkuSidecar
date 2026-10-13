// Local fixture strategy selector. Production profiles/endpoints cannot be supplied.
import { mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { launchFixtureChrome } from './chrome.mjs';
import { startFixture } from './fixture.mjs';
import { SessionStrategyController } from './strategies.mjs';

const args = process.argv.slice(2);
const allowed = ['reader-owned', 'central-vault'];
if (args.length !== 5 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' ||
    !isAbsolute(args[2]) || args[3] !== '--strategy' || !allowed.includes(args[4])) {
  console.error('Usage: node scripts/auth-session-fixture/run-strategies.mjs --run-local-fixture --chrome <absolute chrome.exe> --strategy reader-owned|central-vault');
  process.exit(2);
}
const initial = args[4], alternate = allowed.find(id => id !== initial);
const executable = await realpath(args[2]);
const sidecar = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const parent = join(sidecar, 'runtime', 'qa', 'auth-session-fixture');
await mkdir(parent, { recursive: true });
const root = await mkdtemp(join(parent, 'run-strategies-'));
const receipt = { scope: 'switchable_strategies_local_fixture_only', initialStrategy: initial,
  startedAt: new Date().toISOString(), checks: [], realAuthTested: false };
let fixture, reader, readerPage, collector, collectorPage, collectorContext, controller, stage = 'launch';
const children = [], refreshOwners = [];
const check = (condition, name) => { stage = name; if (!condition) throw new Error('fixture_assertion'); receipt.checks.push(name); };
async function launch(role) { const c = await launchFixtureChrome(executable, root, role); children.push(c); return c; }
function client(id) {
  const browser = () => id === 'reader' ? reader : collector;
  const page = () => id === 'reader' ? readerPage : collectorPage;
  const scope = () => id === 'reader' ? {} : { browserContextId: collectorContext };
  return {
    async replaceCookies({ cookies }) {
      await browser().send('Storage.clearCookies', scope());
      await browser().send('Storage.setCookies', { ...scope(), cookies });
    },
    exportCookies: () => browser().send('Storage.getCookies', scope()),
    async probe() {
      const response = await page().request('/probe');
      return { authenticated: response.status === 200, identity: { accountId: response.body.account } };
    },
    async refresh() {
      refreshOwners.push(id);
      if ((await page().request('/rotate', 'POST')).status !== 200) throw new Error('fixture_refresh_rejected');
    },
    clear: () => browser().isExited() ? Promise.resolve() : browser().send('Storage.clearCookies', scope()),
  };
}
const use = id => controller.use(id, controller.lease(id), () => (id === 'reader' ? readerPage : collectorPage).request(id === 'reader' ? '/reader' : '/collect'));
async function verifyPair(label) {
  const results = await Promise.all([use('reader'), use('collector')]);
  check(results.every(r => r.ok && r.value.status === 200 && r.value.body.account === 'alpha'), label);
}
try {
  fixture = await startFixture(); reader = await launch('owner'); collector = await launch('collector');
  receipt.chrome = reader.product;
  readerPage = await reader.page(fixture.origin);
  ({ browserContextId: collectorContext } = await collector.send('Target.createBrowserContext', { disposeOnDetach: true }));
  collectorPage = await collector.page(fixture.origin, collectorContext);
  check((await readerPage.request('/login', 'POST')).status === 200, 'single_fixture_login');
  controller = new SessionStrategyController({ origin: fixture.origin, expectedAccountId: 'alpha',
    clients: { reader: client('reader'), collector: client('collector') }, timeoutMs: 5000 });
  stage = 'start_strategy'; check((await controller.start(initial)).ok, 'selected_initial_strategy_started');
  await verifyPair('initial_strategy_concurrent_reads');
  check((await controller.refresh()).ok && refreshOwners.at(-1) === (initial === 'reader-owned' ? 'reader' : 'collector'), 'initial_refresh_uses_correct_authority');
  await verifyPair('initial_refresh_distributed');
  const beforeInvalid = controller.lease('reader');
  check(!(await controller.switchTo('unregistered-strategy')).ok, 'unsupported_strategy_rejected');
  check((await controller.use('reader', beforeInvalid, () => readerPage.request('/reader'))).ok, 'invalid_switch_preserves_active_strategy');
  check((await controller.switchTo(initial)).ok, 'same_strategy_switch_is_noop');

  stage = 'switch_during_work';
  const stale = controller.lease('reader');
  let entered, completed = false, ranBlocked = false;
  const began = new Promise(resolve => { entered = resolve; });
  const work = controller.use('collector', controller.lease('collector'), async () => {
    entered(); const result = await collectorPage.request('/collect'); completed = true; return result;
  });
  await began;
  const transition = controller.switchTo(alternate);
  const blocked = await controller.use('reader', controller.lease('reader'), async () => { ranBlocked = true; });
  check(!blocked.ok && !ranBlocked, 'switch_fences_new_work_immediately');
  const [oldResult, switched] = await Promise.all([work, transition]);
  check(switched.ok && completed, 'switch_waits_for_inflight_work');
  check(!oldResult.ok, 'old_generation_result_rejected');
  let ranStale = false;
  const oldLease = await controller.use('reader', stale, async () => { ranStale = true; });
  check(!oldLease.ok && !ranStale, 'old_strategy_lease_cannot_run');
  await verifyPair('alternate_strategy_concurrent_reads');
  check((await controller.refresh()).ok && refreshOwners.at(-1) === (alternate === 'reader-owned' ? 'reader' : 'collector'), 'alternate_refresh_uses_correct_authority');
  await verifyPair('alternate_refresh_distributed');
  stage = 'switch_back'; check((await controller.switchTo(initial)).ok, 'switch_back_to_initial_strategy');
  await verifyPair('switched_back_reads');
  check(fixture.stats().logins === 1 && fixture.stats().overlap >= 2, 'bidirectional_switch_without_relogin');
  fixture.assertSanitized(JSON.stringify(controller.status()));
  check((await controller.revoke()).ok, 'strategy_cleanup_acknowledged');
  check(!controller.lease('reader') && !controller.lease('collector'), 'revocation_denies_new_work');
  receipt.refreshOwners = refreshOwners;
  receipt.status = 'passed';
} catch (error) {
  receipt.status = 'failed'; receipt.failedStage = stage; process.exitCode = 1;
  if (/^fixture_[a-zA-Z0-9_.]+$/.test(error.message || '')) receipt.errorCode = error.message;
} finally {
  if (controller) {
    const result = await controller.revoke().catch(() => ({ ok: false }));
    if (!result.ok) { receipt.strategyCleanupFailed = true; process.exitCode = 1; }
  }
  const cleanup = await Promise.allSettled(children.map(c => c.close()));
  receipt.browserRootsExited = children.every(c => c.isExited());
  receipt.cleanupSucceeded = cleanup.every(r => r.status === 'fulfilled') && receipt.browserRootsExited;
  if (fixture) { receipt.fixtureStats = fixture.stats(); await fixture.close(); }
  const resolvedParent = await realpath(parent), resolvedRoot = await realpath(root);
  const rel = relative(resolvedParent, resolvedRoot);
  if (receipt.cleanupSucceeded && !isAbsolute(rel) && /^run-strategies-[^\\/]+$/.test(rel)) {
    try { await rm(resolvedRoot, { recursive: true, maxRetries: 3, retryDelay: 150 }); receipt.profilesRemoved = true; }
    catch { receipt.profilesRemoved = false; process.exitCode = 1; }
  } else { receipt.profilesRemoved = false; process.exitCode = 1; }
  if (process.exitCode && receipt.status === 'passed') receipt.status = 'cleanup_failed';
  receipt.finishedAt = new Date().toISOString();
  const output = JSON.stringify(receipt, null, 2); fixture?.assertSanitized(output); console.log(output);
}
