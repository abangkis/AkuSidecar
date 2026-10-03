// Bounded operator fixture. The default path reads local registration files
// only; runtime inspection and interruption require an explicit opt-in.
import { execFile } from 'node:child_process';
import { promisify } from 'node:util';
import { createHash, randomUUID } from 'node:crypto';
import { mkdir, readFile, realpath, writeFile } from 'node:fs/promises';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { nativeBridgeRecaptureBaseline } from './native-bridge-recapture-baseline.mjs';

const exec = promisify(execFile);
const sidecar = resolve(dirname(fileURLToPath(import.meta.url)), '..');
const workspace = dirname(sidecar);
const targetUrl = 'https://www.facebook.com/reel/1639349817600268/';
const testName = 'TestFacebookReelBridgeForegroundMediaRecaptureBaselineWindows';
const args = process.argv.slice(2);
const allowStop = args.includes('--allow-runtime-stop');
const approveForeground = args.includes('--approve-foreground-baseline');
const allowedFlags = new Set(['--allow-runtime-stop', '--approve-foreground-baseline']);
if (process.platform !== 'win32' ||
    (args.length > 0 && (args.length !== 2 || new Set(args).size !== 2 || args.some(arg => !allowedFlags.has(arg)) || !allowStop || !approveForeground))) {
  throw new Error('Usage: node scripts/test-facebook-reel-bridge-baseline.mjs [--allow-runtime-stop --approve-foreground-baseline]');
}

const configPath = join(process.env.LOCALAPPDATA ?? '', 'AkuSupervisor', 'services.json');
const config = JSON.parse(await readFile(configPath, 'utf8'));
const registration = config.services?.akusidecar;
if (!registration || !Array.isArray(registration.args)) throw new Error('Registered akusidecar service is missing.');
function registeredValue(flag) {
  const indices = registration.args.flatMap((value, index) => value === flag ? [index] : []);
  if (indices.length !== 1 || !registration.args[indices[0] + 1]) throw new Error(`Registration must contain one ${flag} value.`);
  return registration.args[indices[0] + 1];
}
const bridgePath = await realpath(join(workspace, 'AkuBridge'));
const registeredBridgePath = await realpath(registeredValue('--bridge-extension-path'));
const profile = await realpath(registeredValue('--browser-profile'));
const chrome = await realpath(registeredValue('--chromium-path'));
const profileRoot = await realpath(join(sidecar, 'runtime'));
const profileRelative = relative(profileRoot, profile);
if (!profileRelative || profileRelative.startsWith('..') || isAbsolute(profileRelative) || profile.endsWith('-ui-split-cft')) {
  throw new Error('Registered profile is outside the normal AkuSidecar runtime profile tree.');
}
if (!samePath(registeredBridgePath, bridgePath)) throw new Error('Registered service does not use the original source-tree AkuBridge.');
if (basenameLower(chrome) !== 'chrome.exe') throw new Error('Registered capture browser must be Chrome.');
const manifest = JSON.parse(await readFile(join(bridgePath, 'manifest.json'), 'utf8'));
if (!manifest.key) throw new Error('Source-tree Bridge manifest identity is unavailable.');
const extensionId = [...createHash('sha256').update(Buffer.from(manifest.key, 'base64')).digest().subarray(0, 16)]
  .flatMap(byte => ['abcdefghijklmnop'[byte >> 4], 'abcdefghijklmnop'[byte & 0x0f]]).join('');
const bridgeOrigin = `chrome-extension://${extensionId}`;
if (!sameOrigin(registeredValue('--bridge-extension-origin'), bridgeOrigin)) {
  throw new Error('Registered service Bridge origin does not match the source-tree manifest identity.');
}
const supervisor = join(workspace, 'AkuSupervisor', 'target', 'dev', 'aku-supervisor.exe');
const publicPlan = {
  status: allowStop ? 'foreground_baseline_approval_received' : 'filesystem_dry_run_ready',
  scope: 'facebook native-target media-recapture foreground baseline; not normal feed Update parity',
  targetUrl,
  source: 'facebook',
  mode: 'recapture_media',
  captureVisibilityPolicy: 'adaptive_fidelity',
  foregroundAuthorized: allowStop && approveForeground,
  browser: 'registered Chrome profile; Chrome/154.x verified only by the opt-in test',
  browserLifecycle: 'ordinary owned foreground test UI plus one managed Recapture target; owned-process drain required',
  privateCDP: false,
  versionEvidence: 'registered executable Windows version resource; no browser launch needed',
  bridge: 'original source-tree AkuBridge; manifest origin matched to registration',
  bounds: { scrolls: 0, maxBlocksPerSnapshot: 1, maxBlockCharacters: 4000, retries: 0, acquisitionRounds: 1 },
  isolatedDatabase: 'project-local temporary SQLite database',
  runtimeStopIssued: false,
  browserLaunchIssued: false,
  liveApiUsed: false,
};
if (!allowStop) {
  console.log(JSON.stringify({ ...publicPlan, profileRegistered: true, chromePathRegistered: true, sourceBridgeRegistered: true, supervisorAvailable: await exists(supervisor), nextStepRequiresBothRuntimeStopAndForegroundApproval: true }, null, 2));
  process.exit(0);
}

const origin = 'http://127.0.0.1:11122';
async function cli(command) {
  const { stdout } = await exec(supervisor, [...command, '--json', '--config', configPath], { windowsHide: true, timeout: 45000 });
  return JSON.parse(stdout);
}
async function status() {
  const service = (await cli(['status'])).response?.services?.find(item => item.id === 'akusidecar');
  if (!service) throw new Error('Supervisor status has no akusidecar service.');
  return service;
}
async function profileOwners() {
  const script = '$values=@(Get-CimInstance Win32_Process -Filter "name = \'chrome.exe\'" | Where-Object { $_.CommandLine -notlike \'*--type=*\' } | ForEach-Object { $m=[regex]::Match($_.CommandLine, \'--user-data-dir=(?:"([^"]+)"|(\\S+))\'); $p=$m.Groups[1].Value+$m.Groups[2].Value; if ($p -and [IO.Path]::GetFullPath($p).TrimEnd(\'\\\') -eq $env:AKU_FACEBOOK_PROFILE.TrimEnd(\'\\\')) { [pscustomobject]@{pid=$_.ProcessId; executable=$_.ExecutablePath} } }); ConvertTo-Json -InputObject $values -Compress';
  const { stdout } = await exec('powershell.exe', ['-NoProfile', '-NonInteractive', '-Command', script], {
    windowsHide: true, timeout: 10000, env: { ...process.env, AKU_FACEBOOK_PROFILE: profile },
  });
  return JSON.parse(stdout.trim() || '[]');
}
async function bridgeReadiness() {
  const response = await fetch(`${origin}/api/bridge/health`, { signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error('Bridge health is unavailable.');
  const bridge = (await response.json()).bridge;
  const facebook = bridge?.actual?.sourceAccess?.sources?.find(source => source.source === 'facebook');
  return {
    compatible: bridge?.compatible === true,
    originMatches: sameOrigin(bridge?.actual?.extensionOrigin, bridgeOrigin),
    facebookReady: facebook?.permissionGranted === true && facebook?.scriptRegistered === true && facebook?.ready === true,
  };
}
async function preflight() {
  const service = await status();
  const owners = await profileOwners();
  const response = await fetch(`${origin}/api/inbox?limit=100`, { signal: AbortSignal.timeout(5000) });
  if (!response.ok) throw new Error('Inbox state is unavailable; nothing has been stopped.');
  const inbox = await response.json();
  const activeOrUnknown = !Array.isArray(inbox.sessions) || inbox.sessions.length >= 100 || inbox.sessions.some(session =>
    !['completed', 'partial', 'failed', 'cancelled', 'canceled'].includes(session.status) || !session.completedAt);
  const bridge = await bridgeReadiness();
  if (service.lifecycle !== 'running' || service.health?.status !== 'healthy' || service.operatorHold !== 'none' ||
      owners.length !== 1 || !samePath(owners[0].executable, chrome) || activeOrUnknown ||
      !bridge.compatible || !bridge.originMatches || !bridge.facebookReady) {
    throw new Error('Registered runtime, profile ownership, idle state, and real Facebook Bridge readiness must all verify; nothing has been stopped.');
  }
  return { lifecycle: service.lifecycle, healthy: true, profileOwnerCount: owners.length, activeSessions: 0, bridge };
}
const before = await preflight();
const artifactRoot = join(sidecar, 'build', 'facebook-reel-bridge-baseline');
const receiptRoot = join(artifactRoot, `run-${randomUUID()}`);
await mkdir(receiptRoot, { recursive: true });
await writeFile(join(receiptRoot, 'operator-plan.json'), JSON.stringify({
  ...publicPlan,
  createdAt: new Date().toISOString(),
  bounds: { scrolls: 0, maxBlocksPerSnapshot: 1, maxBlockCharacters: 4000, retries: 0, acquisitionRounds: 1 },
}, null, 2), { encoding: 'utf8', flag: 'wx', mode: 0o600 });
const receipt = { startedAt: new Date().toISOString(), targetUrl, scope: publicPlan.scope, foregroundApproval: true, before, runtimeStopIssued: false, testPassed: false, restored: false };
const pause = () => new Promise(resolveDelay => setTimeout(resolveDelay, 250));
try {
  // Restoration is required even when Supervisor does not acknowledge stop.
  receipt.runtimeStopIssued = true;
  await cli(['stop', 'akusidecar', '--actor', 'codex', '--reason', 'bounded Facebook native-target baseline', '--request-id', randomUUID()]);
  const deadline = Date.now() + 15000;
  let stopped = false;
  while (Date.now() < deadline) {
    const service = await status();
    if (service.lifecycle === 'stopped' && (service.ownedPids ?? []).length === 0 && (await profileOwners()).length === 0) { stopped = true; break; }
    await pause();
  }
  if (!stopped) throw new Error('Original runtime/profile ownership did not drain; baseline browser was not launched.');
  const env = {
    ...process.env,
    GOCACHE: join(sidecar, '.go-build'),
    GOTMPDIR: join(sidecar, 'build'),
    TEMP: receiptRoot,
    TMP: receiptRoot,
    AKU_FACEBOOK_REEL_BRIDGE_BASELINE_SMOKE: '1',
    AKU_FACEBOOK_REEL_BRIDGE_FOREGROUND_APPROVED: '1',
    AKU_FACEBOOK_REEL_BRIDGE_PROFILE: profile,
    AKU_FACEBOOK_REEL_BRIDGE_CHROME: chrome,
    AKU_FACEBOOK_REEL_BRIDGE_SOURCE: bridgePath,
    AKU_FACEBOOK_REEL_BRIDGE_ORIGIN: bridgeOrigin,
    AKU_FACEBOOK_REEL_BRIDGE_RECEIPT_DIR: receiptRoot,
  };
  const result = await exec('go.exe', ['test', './internal/httpapi', '-run', `^${testName}$`, '-count=1', '-timeout=210s', '-v'], {
    cwd: sidecar, env, windowsHide: true, timeout: 240000, maxBuffer: 2 * 1024 * 1024,
  });
  const output = result.stdout + result.stderr;
  await writeFile(join(receiptRoot, 'test-output.txt'), output, 'utf8');
  if (!output.includes(`--- PASS: ${testName} `)) throw new Error('Opt-in baseline test did not report PASS.');
  const observation = JSON.parse(await readFile(join(receiptRoot, 'raw-observation.json'), 'utf8'));
  const baseline = nativeBridgeRecaptureBaseline(observation, targetUrl);
  await writeFile(join(receiptRoot, 'native-target-baseline.json'), JSON.stringify(baseline, null, 2), {encoding:'utf8',flag:'wx',mode:0o600});
  receipt.testPassed = true;
  receipt.exactTargetObserved = baseline.captureStatus === 'exact_target_observed';
  receipt.observedTargetCopies = baseline.targets.length;
} catch (error) {
  receipt.error = String(error.message ?? error);
  if (error.stdout || error.stderr) await writeFile(join(receiptRoot, 'test-output.txt'), String(error.stdout ?? '') + String(error.stderr ?? ''), 'utf8');
  throw error;
} finally {
  if (receipt.runtimeStopIssued) {
    try {
      await cli(['start', 'akusidecar', '--actor', 'codex', '--reason', 'restore runtime after Facebook native-target baseline', '--request-id', randomUUID()]);
      const deadline = Date.now() + 30000;
      while (Date.now() < deadline) {
        try {
          const service = await status();
          const owners = await profileOwners();
          const bridge = await bridgeReadiness();
          if (service.lifecycle === 'running' && service.health?.status === 'healthy' && owners.length === 1 &&
              samePath(owners[0].executable, chrome) && bridge.compatible && bridge.originMatches && bridge.facebookReady) {
            receipt.restored = true;
            break;
          }
        } catch {}
        await pause();
      }
    } catch { receipt.restored = false; }
  }
  receipt.finishedAt = new Date().toISOString();
  await writeFile(join(receiptRoot, 'receipt.json'), JSON.stringify(receipt, null, 2), 'utf8');
  console.log(JSON.stringify({ testPassed: receipt.testPassed, restored: receipt.restored, receipt: receiptRoot }, null, 2));
  if (!receipt.restored) throw new Error('Original runtime restoration is unverified; operator attention is required.');
}

function samePath(a, b) { return resolve(a).toLowerCase() === resolve(b).toLowerCase(); }
function sameOrigin(a, b) { return String(a ?? '').replace(/\/$/, '').toLowerCase() === String(b ?? '').replace(/\/$/, '').toLowerCase(); }
function basenameLower(value) { return value.split(/[\\/]/).at(-1)?.toLowerCase(); }
async function exists(path) { try { await realpath(path); return true; } catch { return false; } }
