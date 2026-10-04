// Operator-only dev activation. Uses the registered profile/config/database.
// No installation, database reset, registration edit or Settings write.
import {readFile, mkdir, copyFile, cp, rename, rm, stat, realpath, writeFile} from 'node:fs/promises';
import {createReadStream} from 'node:fs';
import {createHash, randomUUID} from 'node:crypto';
import {execFile as execFileCallback} from 'node:child_process';
import {promisify} from 'node:util';
import {dirname, join, resolve, relative, isAbsolute, sep} from 'node:path';
import {fileURLToPath} from 'node:url';
import {loadRegistration, preflight, supervisor, waitForStopped, waitForNoProfileOwners, restoreOriginal, verifyTuple} from './test-authenticated-headless-parity.mjs';
import {inspectWindowsListeners} from './test-installed-hybrid-bootstrap.mjs';

const execFile = promisify(execFileCallback);
const root = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const candidate = process.argv[process.argv.indexOf('--candidate') + 1];
const live = process.argv.includes('--allow-runtime-stop') && process.argv.includes('--allow-foreground');
if (process.argv.includes('--allow-runtime-stop') !== process.argv.includes('--allow-foreground')) throw new Error('paired_approval_required');
if (!candidate || !isAbsolute(candidate)) throw new Error('absolute_candidate_required');
const sleep = ms => new Promise(r => setTimeout(r, ms));
const digest = async path => {const hash = createHash('sha256'); for await (const chunk of createReadStream(path)) hash.update(chunk); return hash.digest('hex');};
const exists = async path => {try {await stat(path); return true;} catch (e) {if (e.code === 'ENOENT') return false; throw e;}};
function contained(parent, child) {const r = relative(parent, child); if (!r || r === '..' || r.startsWith(`..${sep}`) || isAbsolute(r)) throw new Error('path_outside_expected_root'); return child;}
async function api(path) {const response = await fetch(`http://127.0.0.1:11122${path}`, {signal: AbortSignal.timeout(5000)}); if (!response.ok) throw new Error('local_api_unavailable'); return response.json();}
async function released(registration) {
  const capture = await waitForNoProfileOwners(registration.profile, 5000);
  const ui = await waitForNoProfileOwners(`${registration.profile}-ui-split-cft`, 5000);
  const ports = await inspectWindowsListeners(11122);
  return capture.clear && ui.clear && ports.available && ports.listeners.length === 0;
}

async function main() {
  await verifyTuple(candidate);
  const services = JSON.parse(await readFile(join(process.env.LOCALAPPDATA, 'AkuSupervisor', 'services.json'), 'utf8'));
  const service = services.services.akusidecar;
  const registration = await loadRegistration();
  const before = await preflight(registration);
  const settings = (await api('/api/settings')).settings;
  if (settings.collectionMode !== 'browser') throw new Error('start_trial_in_browser_required');
  const runtimeRoot = contained(root, await realpath(dirname(service.command)));
  if (runtimeRoot !== join(root, 'runtime', 'dev')) throw new Error('unexpected_dev_runtime');
  const configPath = service.args[service.args.indexOf('--config') + 1];
  if (!isAbsolute(configPath)) throw new Error('registered_config_missing');
  const config = JSON.parse(await readFile(configPath, 'utf8'));
  const dbArg = service.args.indexOf('--database');
  const database = contained(root, await realpath(dbArg >= 0 ? service.args[dbArg + 1] : resolve(dirname(dirname(configPath)), config.database.path)));
  const version = JSON.parse(await readFile(join(candidate, 'runtime', 'current.json'), 'utf8')).version;
  const source = contained(await realpath(candidate), await realpath(join(candidate, 'runtime', 'versions', version)));
  const executable = join(source, 'AkuSidecar.exe');
  const inspection = JSON.parse((await execFile(executable, [...service.args, '--database-inspect'], {
    cwd: service.cwd, env: {...process.env, ...service.environment}, windowsHide: true, timeout: 30000,
  })).stdout);
  if (inspection.status !== 'current' || inspection.databaseSchemaVersion !== inspection.targetSchemaVersion) throw new Error('database_migration_not_authorized');
  const plan = {status: 'preflight_only', before, databaseSchema: inspection.databaseSchemaVersion,
    reuseAuthenticatedProfile: true, reuseDatabase: true, settings: {collectionMode: settings.collectionMode, captureVisibility: settings.captureVisibility},
    candidateSidecarSha256: await digest(executable), runtimeRoot};
  if (!live) {console.log(JSON.stringify(plan)); return;}
  const backupRoot = join(root, 'build', `current-profile-hybrid-trial-${randomUUID()}`);
  await mkdir(join(backupRoot, 'database'), {recursive: true});
  await mkdir(join(backupRoot, 'runtime'), {recursive: true});
  const report = {...plan, status: 'activating', backupRoot, databaseBackup: [], moved: [], activated: false, restored: false};
  let stopIssued = false;
  const components = ['aku-sidecar.exe', 'aku-reader-broker.exe', 'com.akubrowser.reader_activation.json', 'headless-worker', 'ui-reader-broker'];
  const installed = [];
  try {
    await preflight(registration);
    stopIssued = true;
    await supervisor(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'approved current-profile hybrid trial activation', '--request-id', randomUUID()]);
    if (!(await waitForStopped(registration.profile)).stopped || !(await released(registration))) throw new Error('original_release_unverified');
    for (const path of [database, `${database}-wal`, `${database}-shm`, `${database}-journal`, join(dirname(database), '.runtime-version')]) {
      if (!(await exists(path))) continue;
      const destination = join(backupRoot, 'database', path.split(sep).at(-1));
      await copyFile(path, destination);
      const sha256 = await digest(path);
      if (sha256 !== await digest(destination)) throw new Error('database_backup_hash_mismatch');
      report.databaseBackup.push({file: path.split(sep).at(-1), bytes: (await stat(path)).size, sha256});
    }
    if (!report.databaseBackup.some(x => x.file === database.split(sep).at(-1))) throw new Error('database_backup_missing');
    // Save originals by rename; new folders cannot inherit stale worker files.
    for (const name of components) {
      const target = join(runtimeRoot, name);
      if (await exists(target)) {await rename(target, join(backupRoot, 'runtime', name)); report.moved.push(name);}
      installed.push(name);
      const packagedName = name === 'aku-sidecar.exe' ? 'AkuSidecar.exe' : name;
      await cp(join(source, packagedName), target, {recursive: true, force: false, errorOnExist: true});
    }
    await supervisor(['start', 'akusidecar', '--actor', 'codex', '--reason', 'start approved same-profile hybrid trial candidate', '--request-id', randomUUID()]);
    const deadline = Date.now() + 120000;
    while (Date.now() < deadline) {
      try {
        const after = await preflight(registration);
        const current = await api('/api/settings');
        const runtime = current.collectionRuntime;
        if (runtime.headlessAvailable && runtime.supportedSources?.includes('instagram') && runtime.supportedSources?.includes('linkedin')
          && current.settings.collectionMode === 'browser' && before.bridge.authorizedSources.every(s => after.bridge.authorizedSources.includes(s))) {
          report.activated = true; report.status = 'trial_ready'; report.after = after;
          report.collectionRuntime = runtime; report.settingsUnchanged = JSON.stringify(settings) === JSON.stringify(current.settings);
          break;
        }
      } catch {}
      await sleep(1000);
    }
    if (!report.activated) throw new Error('candidate_readiness_timeout');
  } catch (error) {
    report.failureCode = /^[a-z_]+$/.test(error.message) ? error.message : 'trial_activation_failed';
    report.status = 'failed';
    if (stopIssued) {
      try {
        await supervisor(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'rollback unsuccessful trial activation', '--request-id', randomUUID()]);
        if (!(await waitForStopped(registration.profile)).stopped || !(await released(registration))) throw new Error('rollback_release_unverified');
        for (const name of installed) {
          const target = contained(runtimeRoot, join(runtimeRoot, name));
          if (await exists(target)) await rm(target, {recursive: true, force: false});
        }
        for (const name of report.moved) await rename(join(backupRoot, 'runtime', name), join(runtimeRoot, name));
        report.restoration = await restoreOriginal(registration, true, Date.now() + 45000);
        report.restored = report.restoration.restored === true;
      } catch {report.restoreBlocked = true;}
    }
  }
  await writeFile(join(backupRoot, 'activation-receipt.json'), JSON.stringify(report, null, 2));
  console.log(JSON.stringify(report));
  if (!report.activated) process.exitCode = 1;
}
main().catch(error => {console.error(JSON.stringify({status: 'failed', code: /^[a-z_]+$/.test(error.message) ? error.message : 'trial_preflight_failed'})); process.exitCode = 1;});
