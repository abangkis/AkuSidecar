import { mkdir, mkdtemp, realpath, rm, readFile } from 'node:fs/promises';
import { dirname, join, resolve, relative, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { spawn } from 'node:child_process';
import { startFixture } from './fixture.mjs';
import { openScenario } from './scenario-context.mjs';
import { FixtureCaptureQueue } from './capture-queue.mjs';
import { FixtureVaultStore } from './vault.mjs';

const args = process.argv.slice(2);
if (args.length !== 3 || args[0] !== '--run-local-fixture' || args[1] !== '--chrome' || !isAbsolute(args[2])) {
  console.error('Usage: node run-resilience.mjs --run-local-fixture --chrome <absolute chrome.exe>'); process.exit(2);
}
const executable = await realpath(args[2]);
const here = dirname(fileURLToPath(import.meta.url));
const parent = resolve(here, '../../runtime/qa/auth-session-fixture');
await mkdir(parent, { recursive: true });
const root = await mkdtemp(join(parent, 'run-resilience-'));
const receipt = { scope: 'local_auth_resilience_only', startedAt: new Date().toISOString(), realAuthTested: false, checks: [] };
let fixture, context, stage = 'launch', allExited = true;
function check(ok, name) { stage = name; if (!ok) throw new Error('fixture_assertion'); receipt.checks.push(name); }
async function recover(mode, directory) {
  const child = spawn(process.execPath, [join(here, 'recovery-child.mjs'), '--run-local-fixture', executable, root, fixture.origin, mode, directory],
    { windowsHide: true, stdio: ['ignore', 'pipe', 'ignore'] });
  let output = '', overflow = false;
  child.stdout.setEncoding('utf8');
  child.stdout.on('data', chunk => { if (output.length + chunk.length > 16000) overflow = true; else output += chunk; });
  const code = await new Promise((resolve, reject) => { child.once('error', reject); child.once('exit', resolve); });
  if (overflow) throw new Error('fixture_child_output');
  let result; try { result = JSON.parse(output); } catch { allExited = false; throw new Error('fixture_child_failed'); }
  allExited &&= result.cleanup?.browsersExited !== false && result.cleanup?.ok === true;
  check(code === 0 && result.status === 'passed', `child_${mode}_passed`);
  receipt.children ??= []; receipt.children.push(result);
}
try {
  fixture = await startFixture();
  context = await openScenario({ executable, root, origin: fixture.origin });
  receipt.chrome = context.chrome;
  check((await context.pages.reader.request('/login', 'POST')).status === 200, 'single_synthetic_login');
  check((await context.session.start('central-vault')).ok, 'central_session_ready');
  await context.observe();
  check((await context.session.refresh()).ok, 'nonce_bound_refresh_allowed');
  await context.observers.collector.flush();
  check(context.events.length === 0, 'controlled_refresh_is_not_external_rotation');

  const queue = new FixtureCaptureQueue({ session: context.session });
  async function retryDuringControl(id, control) {
    let entered;
    const began = new Promise(r => { entered = r; });
    queue.enqueueRead({ id, read: async () => { entered(); return context.pages.collector.request('/collect'); } });
    const capture = queue.run(id); await began;
    const change = control();
    check((await capture).state === 'pending', `${id}_retained_after_stale_result`);
    check((await change).ok, `${id}_control_completed`);
    await queue.resume();
    check(queue.status(id).state === 'completed' && queue.status(id).attempts === 2, `${id}_retried_once`);
    await queue.resume();
    check(queue.status(id).attempts === 2 && queue.result(id).postId === 'fixture-post-1', `${id}_single_admitted_result`);
  }
  await retryDuringControl('capture-switch', () => context.session.switchTo('reader-owned'));
  await retryDuringControl('capture-refresh', () => context.session.refresh());
  const store = new FixtureVaultStore({ runRoot: root, origin: fixture.origin, expectedAccountId: 'alpha' });
  const snapshot = await context.clients.reader.exportCookies({ origin: fixture.origin });
  stage = 'save_dpapi_vault';
  check((await store.save({ origin: fixture.origin, accountId: 'alpha', cookies: snapshot.cookies })).ok, 'synthetic_ticket_saved_with_dpapi');
  const encrypted = await readFile(join(root, 'fixture-session.vault'));
  check(snapshot.cookies.every(c => !encrypted.includes(Buffer.from(c.value))), 'vault_contains_no_plaintext_ticket');
  const closed = await context.close(); allExited &&= closed.browsersExited; context = null;
  check(closed.ok, 'original_context_closed_before_process_recovery');
  await recover('recover', 'child-recover');
  await recover('reject-stale', 'child-stale');
  check(fixture.stats().logins === 1, 'fresh_process_and_profiles_restore_without_relogin');
  receipt.status = 'passed';
} catch (error) {
  if (error.fixtureBrowsersExited === false) allExited = false;
  receipt.status = 'failed'; receipt.failedStage = stage;
  receipt.code = /^[A-Z_]{1,60}$/.test(error.code ?? '') ? error.code : 'FIXTURE_SCENARIO_FAILED';
  process.exitCode = 1;
} finally {
  if (context) { const closed = await context.close().catch(() => ({ ok: false, browsersExited: false })); allExited &&= closed.ok && closed.browsersExited; }
  if (fixture) { receipt.fixtureStats = fixture.stats(); await fixture.close(); }
  const resolvedParent = await realpath(parent), resolvedRoot = await realpath(root);
  const rel = relative(resolvedParent, resolvedRoot);
  receipt.browsersExited = allExited;
  if (allExited && !isAbsolute(rel) && /^run-resilience-[^\\/]+$/.test(rel)) {
    try { await rm(resolvedRoot, { recursive: true, maxRetries: 3, retryDelay: 150 }); receipt.profilesAndVaultRemoved = true; }
    catch { receipt.profilesAndVaultRemoved = false; process.exitCode = 1; }
  } else { receipt.profilesAndVaultRemoved = false; process.exitCode = 1; }
  if (process.exitCode && receipt.status === 'passed') receipt.status = 'cleanup_failed';
  receipt.finishedAt = new Date().toISOString();
  const output = JSON.stringify(receipt, null, 2); fixture?.assertSanitized(output); console.log(output);
}
