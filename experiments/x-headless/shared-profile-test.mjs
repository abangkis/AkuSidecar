// Explicit operator command: stops only registered akusidecar, tests, restores.
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { randomUUID } from 'node:crypto';
import { configuredCapture, profileOwners } from './capture-profile.mjs';
import { root, workspace } from './chrome.mjs';
import { capture, parseOptions } from './capture.mjs';

const exec = promisify(execFile);
const delay = ms => new Promise(r => setTimeout(r, ms));
const comparison = process.argv.includes('--facebook-compare');
const args=process.argv.slice(2).filter(arg=>arg!=='--facebook-compare');
const baselineIndex=args.indexOf('--comparison-baseline');
let baselinePath;
if(baselineIndex>=0){
  baselinePath=args[baselineIndex+1];
  if(!comparison||!baselinePath||baselinePath.startsWith('--'))throw new Error('--comparison-baseline requires --facebook-compare and a report path.');
  args.splice(baselineIndex,2);
}
const options = parseOptions(args);
if (comparison && options.source !== 'facebook') throw new Error('Facebook comparison requires --source facebook.');
if (options.login || options.help) throw new Error('Use capture.mjs for login/help. This command performs a controlled shared-profile test.');
options.useCaptureProfile = true;
const registration = await configuredCapture();
const supervisor = resolve(workspace, 'AkuSupervisor/target/dev/aku-supervisor.exe');
async function cli(args) {
  const { stdout } = await exec(supervisor, [...args, '--json', '--config', registration.config], { windowsHide: true, timeout: 45000 });
  return JSON.parse(stdout);
}
async function service() {
  const result = await cli(['status']);
  const s = result.response?.services?.find(s => s.id === 'akusidecar');
  if (!s) throw new Error('Supervisor status does not contain akusidecar.');
  return s;
}
function summary(s) {
  return { lifecycle: s.lifecycle, desiredState: s.desiredState, rootPid: s.rootPid,
    health: s.health?.status, ownedPids: s.ownedPids, operatorHold: s.operatorHold };
}
async function bridge() {
  const response = await fetch('http://127.0.0.1:11122/api/bridge/health', { signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error('Bridge health request failed.');
  const { bridge: b } = await response.json();
  return { state: b.state, compatible: b.compatible };
}
const before = await service();
if (before.lifecycle !== 'running' || before.health?.status !== 'healthy') throw new Error('Expected healthy running akusidecar before test. Nothing stopped.');
const owners = await profileOwners(registration.profilePath);
if (owners.length !== 1 || owners[0].executable?.toLowerCase() !== registration.executable.toLowerCase()) {
  throw new Error('Expected exactly one profile owner with the configured executable. Nothing stopped.');
}
const inbox = await fetch('http://127.0.0.1:11122/api/inbox?limit=100', { signal: AbortSignal.timeout(5000) }).then(r => r.json());
if (!Array.isArray(inbox.sessions) || inbox.sessions.some(s =>
  !['completed', 'partial', 'failed', 'cancelled', 'canceled'].includes(s.status) || !s.completedAt)) {
  throw new Error('An Inbox session may still be active. Nothing stopped; wait for completion.');
}
const receipt = { startedAt: new Date().toISOString(), profilePath: registration.profilePath,
  profileDirectory: registration.directory, browserExecutable: registration.executable,
  before: summary(before), bridgeBefore: await bridge(), initialProfileOwners: owners,
  stopIssued: false, restored: false };
// Select authenticated Timeline targets while Sidecar is still available.
const comparisonModule=comparison?await import('./facebook-comparison.mjs'):null;
const comparisonTargets = comparison ? baselinePath?await comparisonModule.loadComparisonTargets(baselinePath):await comparisonModule.selectTargets() : null;
if (comparison && !comparisonTargets.some(target => target.url)) throw new Error('No comparable Facebook targets. Nothing stopped.');
await mkdir(resolve(root, 'runs'), { recursive: true });
const receiptPath = resolve(root, 'runs', 'shared-profile-' + new Date().toISOString().replace(/[:.]/g, '-') + '.json');
let interrupted = false;
const interrupt = () => { interrupted = true; };
process.once('SIGINT', interrupt);
process.once('SIGTERM', interrupt);
try {
  if (comparison) {
    const currentInbox = await fetch('http://127.0.0.1:11122/api/inbox?limit=100', { signal: AbortSignal.timeout(5000) }).then(r => r.json());
    if (!Array.isArray(currentInbox.sessions) || currentInbox.sessions.some(s =>
      !['completed', 'partial', 'failed', 'cancelled', 'canceled'].includes(s.status) || !s.completedAt)) {
      throw new Error('Inbox became active during comparison target selection. Nothing stopped.');
    }
  }
  console.log('Stopping registered akusidecar briefly; UI and capture will close. Restore runs in finally.');
  receipt.stopIssued = true;
  await cli(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'authorized same-profile headless X PoC', '--request-id', randomUUID()]);
  const stopDeadline = Date.now() + 15000;
  while (Date.now() < stopDeadline) {
    const current = await service();
    if (current.lifecycle === 'stopped' && !(await profileOwners(registration.profilePath)).length) break;
    await delay(500);
  }
  receipt.afterStop = summary(await service());
  receipt.profileOwnersAfterStop = await profileOwners(registration.profilePath);
  if (receipt.afterStop.lifecycle !== 'stopped' || receipt.profileOwnersAfterStop.length) throw new Error('Capture profile was not released; refusing headless launch.');
  if (interrupted) throw new Error('Test interrupted before capture.');
  const report = comparison
    ? await (await import('./facebook-comparison.mjs')).captureComparison(options, registration, comparisonTargets)
    : await capture(options, registration);
  receipt.capture = { status: report.status, posts: report.posts.length, durationMs: report.durationMs,
    authenticatedUiObserved: report.samples.some(s => s.authenticatedUiObserved),
    browser: report.browser?.product, documentResponses: report.documentResponses, error: report.error };
} catch (error) {
  receipt.error = error.message;
} finally {
  if (receipt.stopIssued) {
    try {
      receipt.profileOwnersAfterCapture = await profileOwners(registration.profilePath);
      if (receipt.profileOwnersAfterCapture.length) throw new Error('PoC profile still has a browser owner; refusing concurrent restore.');
      await cli(['start', 'akusidecar', '--actor', 'codex', '--reason', 'restore original runtime after headless X PoC', '--request-id', randomUUID()]);
      const deadline = Date.now() + 60000;
      while (Date.now() < deadline) {
        const current = await service();
        receipt.afterRestore = summary(current);
        try { receipt.bridgeAfterRestore = await bridge(); } catch {}
        if (current.lifecycle === 'running' && current.health?.status === 'healthy' && receipt.bridgeAfterRestore?.compatible === true) {
          receipt.restored = true; break;
        }
        await delay(1000);
      }
      receipt.profileOwnersAfterRestore = await profileOwners(registration.profilePath);
      if (!receipt.restored) throw new Error('Restore was started but health/Bridge readiness did not verify within 60 seconds.');
    } catch (error) { receipt.restoreError = error.message; }
  }
  process.removeListener('SIGINT', interrupt);
  process.removeListener('SIGTERM', interrupt);
  receipt.completedAt = new Date().toISOString();
  await writeFile(receiptPath, JSON.stringify(receipt, null, 2) + '\n');
  console.log(JSON.stringify({ captured: receipt.capture?.posts ?? null, captureStatus: receipt.capture?.status ?? null,
    authenticatedUiObserved: receipt.capture?.authenticatedUiObserved ?? null, restored: receipt.restored,
    receipt: receiptPath, error: receipt.error, restoreError: receipt.restoreError }));
}
if (!receipt.restored || receipt.error || receipt.capture?.status !== 'captured_partial') process.exitCode = 2;
