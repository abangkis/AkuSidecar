// Explicit local-only fresh coordinator process. Credentials stay in memory and never cross stdout.
import { mkdir, realpath } from 'node:fs/promises';
import { join, isAbsolute } from 'node:path';
import { FixtureVaultStore } from './vault.mjs';
import { openScenario } from './scenario-context.mjs';

const args = process.argv.slice(2);
if (args.length !== 6 || args[0] !== '--run-local-fixture' || !isAbsolute(args[1]) || !isAbsolute(args[2]) ||
    !['recover', 'reject-stale'].includes(args[4]) || !/^child-[a-z0-9]+$/.test(args[5])) process.exit(2);
const [_, executable, root, origin, mode, directory] = args;
let context, launchFailed = false;
const receipt = { scope: 'fresh_fixture_coordinator', mode, checks: [], realAuthTested: false };
const check = (ok, name) => { if (!ok) throw new Error('fixture_assertion'); receipt.checks.push(name); };
try {
  const store = new FixtureVaultStore({ runRoot: root, origin, expectedAccountId: 'alpha' });
  const snapshot = await store.restore({ origin, expectedAccountId: 'alpha' });
  const childRoot = join(await realpath(root), directory);
  await mkdir(childRoot); // exclusive creation; never adopts an existing profile
  context = await openScenario({ executable, root: childRoot, origin });
  check((await context.pages.reader.request('/probe')).status === 401, 'fresh_reader_has_no_session');
  await context.clients.reader.replaceCookies({ cookies: snapshot.cookies });
  const started = await context.session.start('central-vault');
  if (mode === 'reject-stale') {
    check(!started.ok && !context.session.lease('collector'), 'decryption_does_not_override_server_rejection');
  } else {
    check(started.ok, 'fresh_coordinator_validates_restored_ticket_with_server');
    const results = await Promise.all(['reader', 'collector'].map(id => context.session.use(id,
      context.session.lease(id), () => context.pages[id].request(id === 'reader' ? '/reader' : '/collect'))));
    check(results.every(r => r.ok && r.value.status === 200 && r.value.body.account === 'alpha'), 'restored_reader_and_headless_read_concurrently');
    await context.observe();
    const background = await context.pages.reader.evaluate("fetch('/reader', {cache:'no-store'}).then(r => r.status)");
    const observed = await context.observers.reader.flush();
    check(background === 200 && observed.responseEvents > 0 && context.session.status().ready,
      'unmanaged_page_fetch_observed_without_false_conflict');
    await context.pages.reader.navigate('/reader-rotate');
    await context.observers.reader.flush();
    check(context.events.length > 0 && context.session.status().state === 'CONFLICT', 'ordinary_navigation_mutation_fences_restored_session');
    check(!context.session.lease('collector'), 'unmanaged_mutation_denies_headless_without_capture');
  }
  receipt.status = 'passed';
} catch (error) {
  launchFailed = error.fixtureBrowsersExited === false;
  receipt.status = 'failed'; process.exitCode = 1;
} finally {
  receipt.cleanup = context ? await context.close().catch(() => ({ ok: false, browsersExited: false })) : { ok: !launchFailed, browsersExited: !launchFailed };
  if (!receipt.cleanup.ok) { receipt.status = 'cleanup_failed'; process.exitCode = 1; }
  console.log(JSON.stringify(receipt));
}
