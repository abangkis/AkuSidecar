// Local-only central session authority experiment. No production/profile discovery.
import { mkdir, mkdtemp, realpath, rm } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';
import { launchFixtureChrome } from './chrome.mjs';
import { startFixture } from './fixture.mjs';
import { CentralSession } from './central.mjs';

const args = process.argv.slice(2);
if (args.length !== 3 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' || !isAbsolute(args[2])) {
  console.error('Usage: node scripts/auth-session-fixture/run-central.mjs --run-local-fixture --chrome <absolute chrome.exe>');
  process.exit(2);
}
const executable = await realpath(args[2]);
const sidecar = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const parent = join(sidecar, 'runtime', 'qa', 'auth-session-fixture');
await mkdir(parent, { recursive: true });
const root = await mkdtemp(join(parent, 'run-central-'));
const receipt = { scope: 'central_vault_local_fixture_only', startedAt: new Date().toISOString(), checks: [], realAuthTested: false };
let fixture, reader, readerPage, collector, collectorPage, collectorContext, session, stage = 'launch';
const children = [], events = [];
const check = (condition, name) => { stage = name; if (!condition) throw new Error('fixture_assertion'); receipt.checks.push(name); };
async function launch(role) { const browser = await launchFixtureChrome(executable, root, role); children.push(browser); return browser; }
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
      events.push('refresh');
      const response = await page().request('/rotate', 'POST');
      if (response.status !== 200) throw new Error('fixture_refresh_rejected');
    },
    clear: () => browser().isExited() ? Promise.resolve() : browser().send('Storage.clearCookies', scope()),
  };
}
const use = (id, path) => session.use(id, session.lease(id), () => (id === 'reader' ? readerPage : collectorPage).request(path));
const refresh = () => { const s = session.status(); return session.refresh({ expectedVersion: s.version, expectedEpoch: s.epoch }); };
try {
  fixture = await startFixture();
  reader = await launch('owner'); collector = await launch('collector');
  receipt.chrome = reader.product;
  check(reader.pid !== collector.pid && reader.profile !== collector.profile, 'distinct_reader_and_collector_profiles');
  readerPage = await reader.page(fixture.origin);
  ({ browserContextId: collectorContext } = await collector.send('Target.createBrowserContext', { disposeOnDetach: true }));
  collectorPage = await collector.page(fixture.origin, collectorContext);
  check((await readerPage.request('/login', 'POST')).status === 200, 'one_simulated_reader_login');
  const snapshot = await reader.send('Storage.getCookies');
  const clients = { reader: client('reader'), collector: client('collector') };
  session = new CentralSession({ origin: fixture.origin, expectedAccountId: 'alpha',
    authority: clients.collector, clients, bootstrapClientId: 'reader', refreshClientId: 'collector', timeoutMs: 5000 });
  stage = 'initialize';
  check((await session.initialize({ cookies: snapshot.cookies, accountId: 'alpha' })).ok, 'central_vault_initializes_both_clients');
  const concurrent = await Promise.all([use('reader', '/reader'), use('collector', '/collect')]);
  check(concurrent.every(r => r.ok && r.value.status === 200 && r.value.body.account === 'alpha') && fixture.stats().overlap >= 2 && fixture.stats().logins === 1, 'concurrent_reads_share_one_login');

  stage = 'refresh_drain';
  const oldReaderLease = session.lease('reader');
  let entered;
  const began = new Promise(resolve => { entered = resolve; });
  const inFlight = session.use('collector', session.lease('collector'), async () => {
    events.push('work_started'); entered();
    const result = await collectorPage.request('/collect');
    events.push('work_completed'); return result;
  });
  await began;
  const version = session.status();
  const request = { expectedVersion: version.version, expectedEpoch: version.epoch };
  const rotating = [session.refresh(request), session.refresh(request), session.refresh(request)];
  let unexpectedWork = false;
  const during = await session.use('reader', session.lease('reader'), async () => { unexpectedWork = true; });
  check(!during.ok && !unexpectedWork, 'refresh_gates_new_work');
  const [drained, ...rotations] = await Promise.all([inFlight, ...rotating]);
  check(rotations.every(r => r.ok) && fixture.stats().rotations === 1 && events.indexOf('refresh') > events.indexOf('work_completed'), 'single_refresh_after_managed_work_drains');
  receipt.inflightResultAdmittedBeforeRefresh = drained.ok;
  const old = await session.use('reader', oldReaderLease, async () => { unexpectedWork = true; });
  check(!old.ok && !unexpectedWork, 'old_version_lease_cannot_run');
  check((await use('reader', '/reader')).ok && (await use('collector', '/collect')).ok && fixture.stats().logins === 1, 'new_ticket_distributed_to_both_clients');

  stage = 'reader_close';
  check((await session.detach('reader')).ok, 'reader_detaches_without_deleting_vault');
  const { targetInfos } = await reader.send('Target.getTargets');
  for (const target of targetInfos.filter(t => t.type === 'page')) await reader.send('Target.closeTarget', { targetId: target.targetId }).catch(() => {});
  await delay(1500);
  receipt.readerExitedOnLastWindow = reader.isExited();
  if (!reader.isExited()) await reader.close();
  check((await refresh()).ok && fixture.stats().rotations === 2 && (await use('collector', '/collect')).ok, 'refresh_works_while_reader_process_is_absent');
  reader = await launch('owner'); readerPage = await reader.page(fixture.origin);
  check((await session.attach('reader')).ok, 'reopened_reader_receives_latest_vault_version');
  check((await use('reader', '/reader')).ok && fixture.stats().logins === 1, 'reopened_reader_does_not_need_login');

  stage = 'unexpected_rotation';
  const beforeConflict = session.lease('collector');
  const ordinary = await use('reader', '/reader-rotate');
  check(!ordinary.ok && session.status().state === 'CONFLICT', 'ordinary_response_rotation_is_detected_and_fenced');
  let ranAfterConflict = false;
  const rejected = await session.use('collector', beforeConflict, async () => { ranAfterConflict = true; });
  check(!rejected.ok && !ranAfterConflict, 'conflicting_generation_cannot_continue');
  check(!(await refresh()).ok && fixture.stats().rotations === 3, 'conflict_does_not_trigger_blind_refresh_or_merge');
  check((await session.revoke()).ok, 'revoke_clears_central_and_client_state');
  const cleared = await Promise.all([readerPage.request('/probe'), collectorPage.request('/probe')]);
  check(cleared.every(r => r.status === 401) && !session.lease('reader') && !session.lease('collector'), 'revoked_clients_are_unauthenticated');
  fixture.assertSanitized(JSON.stringify(session.status()));
  check(fixture.stats().logins === 1, 'all_central_scenarios_used_one_login');
  receipt.status = 'passed';
} catch (error) {
  receipt.status = 'failed'; receipt.failedStage = stage; process.exitCode = 1;
  if (/^fixture_[a-zA-Z0-9_.]+$/.test(error.message || '')) receipt.errorCode = error.message;
} finally {
  if (session) {
    const revoked = await session.revoke().catch(() => ({ ok: false }));
    if (!revoked.ok) { receipt.sessionCleanupFailed = true; process.exitCode = 1; }
  }
  const cleanup = await Promise.allSettled(children.map(c => c.close()));
  receipt.browserRootsExited = children.every(c => c.isExited());
  receipt.cleanupSucceeded = cleanup.every(r => r.status === 'fulfilled') && receipt.browserRootsExited;
  if (fixture) { receipt.fixtureStats = fixture.stats(); await fixture.close(); }
  const resolvedParent = await realpath(parent), resolvedRoot = await realpath(root);
  const rel = relative(resolvedParent, resolvedRoot);
  if (receipt.cleanupSucceeded && !isAbsolute(rel) && /^run-central-[^\\/]+$/.test(rel)) {
    try { await rm(resolvedRoot, { recursive: true, maxRetries: 3, retryDelay: 150 }); receipt.profilesRemoved = true; }
    catch { receipt.profilesRemoved = false; process.exitCode = 1; }
  } else { receipt.profilesRemoved = false; process.exitCode = 1; }
  if (process.exitCode && receipt.status === 'passed') receipt.status = 'cleanup_failed';
  receipt.finishedAt = new Date().toISOString();
  const output = JSON.stringify(receipt, null, 2); fixture?.assertSanitized(output); console.log(output);
}
