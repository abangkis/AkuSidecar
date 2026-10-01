// Explicit operator-only stop/test/restore wrapper. Default is read-only preflight.
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { mkdir, readFile, realpath, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { randomUUID } from 'node:crypto';

const exec = promisify(execFile);
const sidecar = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const workspace = dirname(sidecar);
const args = process.argv.slice(2);
const allowStop = args.includes('--allow-runtime-stop');
const artifactIndex = args.indexOf('--artifact');
const artifact = artifactIndex >= 0 ? args[artifactIndex + 1] : null;
if (process.platform !== 'win32' || !artifact || !isAbsolute(artifact) ||
    args.some((arg, index) => !['--allow-runtime-stop', '--artifact'].includes(arg) && index !== artifactIndex + 1)) {
  throw new Error('Usage: node scripts/test-bridge-headless-handoff.mjs --artifact <absolute candidate directory> [--allow-runtime-stop]');
}
const artifactRoot = await realpath(artifact);
const artifactChild = relative(join(workspace, 'AkuBrowser', 'build'), artifactRoot);
if (!artifactChild || artifactChild.startsWith('..') || isAbsolute(artifactChild)) throw new Error('Only a project-local candidate under AkuBrowser/build is allowed.');
const pointer = JSON.parse(await readFile(join(artifactRoot, 'runtime', 'current.json'), 'utf8'));
if (!/^[A-Za-z0-9._-]+$/.test(pointer.version)) throw new Error('Invalid candidate version.');
const versionRoot = join(artifactRoot, 'runtime', 'versions', pointer.version);
const configPath = join(process.env.LOCALAPPDATA, 'AkuSupervisor', 'services.json');
const registration = JSON.parse(await readFile(configPath, 'utf8')).services?.akusidecar;
if (!registration || !Array.isArray(registration.args)) throw new Error('Registered akusidecar service missing.');
const flagValue = flag => {
  const index = registration.args.indexOf(flag);
  if (index < 0 || !registration.args[index + 1]) throw new Error(`Registration missing ${flag}.`);
  return registration.args[index + 1];
};
const profile = await realpath(flagValue('--browser-profile'));
const captureExe = await realpath(flagValue('--chromium-path'));
const profileChild = relative(await realpath(join(sidecar, 'runtime')), profile);
if (!profileChild || profileChild.startsWith('..') || isAbsolute(profileChild) || profile.endsWith('-ui-split-cft')) throw new Error('Registered capture profile is outside the allowed runtime root.');
const supervisor = join(workspace, 'AkuSupervisor', 'target', 'dev', 'aku-supervisor.exe');
async function cli(command) {
  const { stdout } = await exec(supervisor, [...command, '--json', '--config', configPath], {windowsHide: true, timeout: 45000});
  return JSON.parse(stdout);
}
async function status() {
  const value = (await cli(['status'])).response?.services?.find(service => service.id === 'akusidecar');
  if (!value) throw new Error('Supervisor status has no akusidecar service.');
  return value;
}
async function owners() {
  const script = '$values=@(Get-CimInstance Win32_Process -Filter "name = \'chrome.exe\'" | Where-Object { $_.CommandLine -notlike \'*--type=*\' } | ForEach-Object { $m=[regex]::Match($_.CommandLine, \'--user-data-dir=(?:"([^\"]+)"|(\\S+))\'); $p=$m.Groups[1].Value+$m.Groups[2].Value; if ($p -and [IO.Path]::GetFullPath($p).TrimEnd(\'\\\') -eq $env:AKU_HANDOFF_PROFILE.TrimEnd(\'\\\')) { [pscustomobject]@{pid=$_.ProcessId; executable=$_.ExecutablePath} } }); ConvertTo-Json -InputObject $values -Compress';
  const { stdout } = await exec('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {windowsHide: true, timeout: 10000, env: {...process.env, AKU_HANDOFF_PROFILE: profile}});
  return JSON.parse(stdout.trim() || '[]');
}
async function bridge() {
  const response = await fetch('http://127.0.0.1:11122/api/bridge/health', {signal: AbortSignal.timeout(5000)});
  if (!response.ok) throw new Error('Bridge health unavailable.');
  const value = (await response.json()).bridge;
  return {state: value?.state, compatible: value?.compatible};
}
async function preflight() {
  const service = await status();
  const profileOwners = await owners();
  const response = await fetch('http://127.0.0.1:11122/api/inbox?limit=100', {signal: AbortSignal.timeout(5000)});
  if (!response.ok) throw new Error('Inbox state unavailable; nothing stopped.');
  const inbox = await response.json();
  if (!Array.isArray(inbox.sessions) || inbox.sessions.length >= 100 || inbox.sessions.some(session => !['completed', 'partial', 'failed', 'cancelled', 'canceled'].includes(session.status) || !session.completedAt)) throw new Error('Inbox is active or unverifiable; nothing stopped.');
  if (service.lifecycle !== 'running' || service.health?.status !== 'healthy' || service.operatorHold !== 'none' || profileOwners.length !== 1 || profileOwners[0].executable?.toLowerCase() !== captureExe.toLowerCase()) throw new Error('Original service or profile ownership is not healthy; nothing stopped.');
  const bridgeHealth = await bridge();
  if (bridgeHealth.compatible !== true) throw new Error('Original Bridge is incompatible; nothing stopped.');
  return {service: {lifecycle: service.lifecycle, rootPid: service.rootPid, health: service.health.status}, profileOwners, bridge: bridgeHealth, activeSessions: 0};
}
const before = await preflight();
if (!allowStop) {
  console.log(JSON.stringify({status: 'preflight_only', candidate: artifactRoot, before, requiresExplicitRuntimeStop: true}, null, 2));
} else {
  // Check the tuple before any interruption. This verifier does not launch UI.
  await exec('pwsh.exe', ['-NoProfile', '-File', join(workspace, 'AkuBrowser', 'scripts', 'test-windows-installed-app-builder.ps1'), '-ArtifactDirectory', artifactRoot], {windowsHide: true, timeout: 60000});
  await preflight();
  const receiptRoot = join(sidecar, 'build', `bridge-handoff-${randomUUID()}`);
  await mkdir(receiptRoot, {recursive: true});
  const receipt = {startedAt: new Date().toISOString(), candidate: artifactRoot, before, stopIssued: false, restored: false};
  const pause = () => new Promise(resolveDelay => setTimeout(resolveDelay, 250));
  try {
    receipt.stopIssued = true; // Restoration is required even if stop ACK fails.
    await cli(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'authorized isolated Bridge headless handoff test', '--request-id', randomUUID()]);
    const deadline = Date.now() + 15000;
    let stopped = false;
    while (Date.now() < deadline) {
      const current = await status();
      if (current.lifecycle === 'stopped' && (current.ownedPids ?? []).length === 0 && (await owners()).length === 0) { stopped = true; break; }
      await pause();
    }
    if (!stopped) throw new Error('Original owner was not released; candidate not launched.');
    const testEnv = {...process.env, GOCACHE: join(sidecar, '.go-build'), GOTMPDIR: join(sidecar, 'build'), TEMP: receiptRoot, TMP: receiptRoot,
      AKU_BRIDGE_HANDOFF_SMOKE_RUNTIME: join(versionRoot, 'headless-worker'), AKU_BRIDGE_HANDOFF_SMOKE_CHROME: join(versionRoot, 'chromium', 'bin', 'chrome.exe'), AKU_BRIDGE_HANDOFF_SMOKE_BRIDGE: join(versionRoot, 'AkuBridge')};
    try {
      const result = await exec('go.exe', ['test', './internal/httpapi', '-run', '^TestBridgeHeadlessHandoffWindowsSmoke$', '-count=1', '-timeout=75s', '-v'], {cwd: sidecar, env: testEnv, windowsHide: true, timeout: 90000, maxBuffer: 1024 * 1024});
      if (!result.stdout.includes('--- PASS: TestBridgeHeadlessHandoffWindowsSmoke ')) throw new Error('Expected smoke did not run or did not pass.');
      receipt.testPassed = true;
      await writeFile(join(receiptRoot, 'test-output.txt'), result.stdout + result.stderr);
    } catch (error) {
      receipt.testPassed = false;
      await writeFile(join(receiptRoot, 'test-output.txt'), String(error.stdout || '') + String(error.stderr || ''));
      throw new Error('Isolated Bridge smoke failed; inspect private local receipt after restoration.');
    }
  } finally {
    if (receipt.stopIssued) {
      try {
        await cli(['start', 'akusidecar', '--actor', 'codex', '--reason', 'restore original runtime after isolated Bridge test', '--request-id', randomUUID()]);
        const deadline = Date.now() + 25000;
        while (Date.now() < deadline) {
          const service = await status();
          if (service.lifecycle === 'running' && service.health?.status === 'healthy') {
            try {
              const health = await bridge();
              const values = await owners();
              if (health.compatible === true && values.length === 1 && values[0].executable?.toLowerCase() === captureExe.toLowerCase()) { receipt.restored = true; break; }
            } catch {}
          }
          await pause();
        }
      } catch { receipt.restored = false; }
    }
    receipt.finishedAt = new Date().toISOString();
    await writeFile(join(receiptRoot, 'receipt.json'), JSON.stringify(receipt, null, 2));
    console.log(JSON.stringify({testPassed: receipt.testPassed ?? false, restored: receipt.restored, receipt: join(receiptRoot, 'receipt.json')}, null, 2));
    if (!receipt.restored) throw new Error('Original runtime restoration is unverified; operator attention required.');
  }
}
