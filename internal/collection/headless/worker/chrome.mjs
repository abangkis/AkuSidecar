import { spawn, execFile } from 'node:child_process';
import { access, mkdir, realpath } from 'node:fs/promises';
import { constants } from 'node:fs';
import { resolve, isAbsolute } from 'node:path';
import { promisify } from 'node:util';

const CDP_TIMEOUT_MS = 15000;
const TARGET_CLEANUP_TIMEOUT_MS = 1000;
const TARGET_CLEANUP_VERIFY_TIMEOUT_MS = 2000;
const execFileAsync = promisify(execFile);

// Muted autoplay is allowed by Chrome's ordinary autoplay policy. Collection
// needs media metadata/URLs, not a running player. Stop each playback attempt
// without replacing play(), removing sources or blocking metadata acquisition.
export const CAPTURE_PLAYBACK_GUARD_SOURCE = `(() => {
  const stopPlayback = event => {
    const media = event.target;
    if (media instanceof HTMLMediaElement && !media.paused) media.pause();
  };
  document.addEventListener('play', stopPlayback, true);
  document.addEventListener('playing', stopPlayback, true);
})();`;

export async function launchChrome({ chromePath, profilePath, profileDirectory = 'Default' }) {
  if (!isAbsolute(chromePath) || !isAbsolute(profilePath)) throw new Error('Chrome and profile paths must be absolute.');
  const executable = await realpath(chromePath);
  await access(executable, constants.X_OK).catch(() => access(executable));
  const userData = resolve(profilePath);
  await mkdir(userData, { recursive: true });
  await assertProfileUnowned(userData);
  const args = [
    `--user-data-dir=${userData}`,
    `--profile-directory=${profileDirectory}`,
    '--remote-debugging-pipe',
    '--headless=new',
    '--disable-gpu',
    '--no-first-run',
    '--no-default-browser-check',
    '--disable-component-update',
    '--disable-background-mode',
    '--disable-extensions',
    '--disable-session-crashed-bubble',
    '--hide-crash-restore-bubble',
    '--no-startup-window',
  ];
  const child = spawn(executable, args, {
    windowsHide: true,
    stdio: ['ignore', 'ignore', 'pipe', 'pipe', 'pipe'],
  });
  return connectOwnedChrome(child, executable, userData);
}

async function assertProfileUnowned(profilePath) {
  if (process.platform !== 'win32') return;
  const systemRoot = process.env.SystemRoot || 'C:\\Windows';
  const powershell = resolve(systemRoot, 'System32', 'WindowsPowerShell', 'v1.0', 'powershell.exe');
  const script = String.raw`$ErrorActionPreference='Stop'; $wanted=[IO.Path]::GetFullPath($env:AKU_HEADLESS_PROFILE).TrimEnd('\'); $owners=@(Get-CimInstance Win32_Process -Filter "name = 'chrome.exe'" | Where-Object { $_.CommandLine -notlike '*--type=*' } | Where-Object { $m=[regex]::Match($_.CommandLine, '--user-data-dir=(?:"([^"]+)"|(\S+))'); $p=$m.Groups[1].Value+$m.Groups[2].Value; $p -and [IO.Path]::GetFullPath($p).TrimEnd('\') -ieq $wanted }); ConvertTo-Json -InputObject @{owned=($owners.Count -gt 0)} -Compress`;
  let result;
  try {
    result = await execFileAsync(powershell, ['-NoProfile', '-NonInteractive', '-Command', script], {
      windowsHide: true,
      timeout: 10000,
      maxBuffer: 4096,
      env: { ...process.env, AKU_HEADLESS_PROFILE: profilePath },
    });
  } catch {
    throw Object.assign(new Error('Could not verify that the requested Chrome profile is not in use.'), { code: 'profile_owner_check_failed' });
  }
  try {
    if (JSON.parse(result.stdout.trim()).owned === true) {
      throw Object.assign(new Error('Requested Chrome profile is already owned by a running Chrome process.'), { code: 'profile_in_use' });
    }
  } catch (error) {
    if (error?.code === 'profile_in_use') throw error;
    throw Object.assign(new Error('Chrome profile ownership check returned invalid data.'), { code: 'profile_owner_check_failed' });
  }
}

function connectOwnedChrome(child, executable, profilePath) {
  const pending = new Map();
  let sequence = 0;
  let buffer = '';
  let closed = false;
  let closePromise = null;
  let stderrTail = '';
  let exitResolve;
  const exited = new Promise(resolveExit => { exitResolve = resolveExit; });
  child.stderr.setEncoding('utf8');
  child.stderr.on('data', chunk => { stderrTail = (stderrTail + chunk).slice(-3000); });
  child.on('error', error => { closePending(error); markClosed(); });
  child.on('exit', code => { closePending(new Error(`Chrome exited${code === null ? '' : ` (${code})`}.`)); markClosed(); });
  child.stdio[3].on('error', error => closePending(error));
  child.stdio[4].setEncoding('utf8');
  child.stdio[4].on('data', chunk => {
    buffer += chunk;
    if (buffer.length > 16 * 1024 * 1024 && !buffer.includes('\0')) {
      closePending(new Error('Chrome debugging pipe frame exceeded the 16 MiB limit.'));
      return;
    }
    let end;
    while ((end = buffer.indexOf('\0')) >= 0) {
      const frame = buffer.slice(0, end);
      buffer = buffer.slice(end + 1);
      if (!frame) continue;
      let message;
      try { message = JSON.parse(frame); } catch { continue; }
      const call = pending.get(message.id);
      if (!call) continue;
      pending.delete(message.id);
      clearTimeout(call.timer);
      if (message.error) call.reject(new Error(`CDP ${call.method}: ${message.error.message || 'command failed'}`));
      else call.resolve(message.result || {});
    }
  });

  function markClosed() {
    if (closed) return;
    closed = true;
    exitResolve();
  }
  function closePending(error) {
    for (const [id, call] of pending) {
      clearTimeout(call.timer);
      call.reject(error);
      pending.delete(id);
    }
  }
  function send(method, params = {}, sessionId, timeoutMs = CDP_TIMEOUT_MS) {
    if (closed) return Promise.reject(new Error('Chrome is closed.'));
    const id = ++sequence;
    return new Promise((resolveCall, reject) => {
      const timer = setTimeout(() => {
        pending.delete(id);
        reject(Object.assign(new Error(`CDP timeout: ${method}`), { code: 'capture_timeout' }));
      }, Math.max(1, Math.min(CDP_TIMEOUT_MS, Number.isFinite(timeoutMs) ? timeoutMs : CDP_TIMEOUT_MS)));
      pending.set(id, { resolve: resolveCall, reject, timer, method });
      child.stdio[3].write(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }) + '\0', error => {
        if (error && pending.has(id)) {
          pending.delete(id);
          clearTimeout(timer);
          reject(error);
        }
      });
    });
  }
  async function close() {
    closePromise ??= closeOwned();
    return closePromise;
  }
  async function closeOwned() {
    try {
      if (!closed) await send('Browser.close').catch(() => {});
      if (!closed) await waitForExitOrTimeout(exited, 2500);
      if (!closed) {
        await terminateOwnedTree(child.pid);
        await waitForExitOrTimeout(exited, 2500);
      }
    } finally {
      closePending(new Error('Chrome closed.'));
    }
  }

  return (async () => {
    try {
      const version = await send('Browser.getVersion');
      const { sessionId: browserSessionId } = await send('Target.attachToBrowserTarget');
      const sourceContexts = new Map();
      async function closeTargetAndWait(targetId) {
        await send('Target.closeTarget', { targetId }, browserSessionId, TARGET_CLEANUP_TIMEOUT_MS);
        const deadline = Date.now() + TARGET_CLEANUP_VERIFY_TIMEOUT_MS;
        while (Date.now() < deadline) {
          const timeoutMs = Math.min(TARGET_CLEANUP_TIMEOUT_MS, deadline - Date.now());
          const { targetInfos = [] } = await send('Target.getTargets', {}, browserSessionId, timeoutMs);
          if (!targetInfos.some(target => target.targetId === targetId)) return;
          await delay(Math.min(50, deadline - Date.now()));
        }
        throw new Error('Chrome did not release its temporary target.');
      }
      async function createPageContext() {
        let targetId;
        try {
          // This process is already headless. A CDP hidden target additionally
          // suppresses animation frames, even when visibilityState is visible.
          // Keep normal rendering without activating a desktop window.
          ({ targetId } = await send('Target.createTarget', {
            url: 'about:blank', background: true, forTab: false,
          }, browserSessionId));
          const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true }, browserSessionId);
          await send('Page.enable', {}, sessionId);
          await send('Runtime.enable', {}, sessionId);
          await send('Network.enable', {}, sessionId);
          // Install before navigation, including subsequent documents/frames.
          // Interactive/borrowed Chrome contexts do not use this launch path.
          await send('Page.addScriptToEvaluateOnNewDocument', { source: CAPTURE_PLAYBACK_GUARD_SOURCE }, sessionId);
          // The second ordinary background tab otherwise remains occluded and
          // stops animation frames. CDP emulation changes page lifecycle only;
          // no Target.activateTarget or OS foreground operation is needed.
          await send('Emulation.setFocusEmulationEnabled', { enabled: true }, sessionId);
          await send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sessionId);
          return {
            send: (method, params = {}, timeoutMs) => send(method, params, sessionId, timeoutMs),
            async evaluate(expression, timeoutMs) {
              const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }, sessionId, timeoutMs);
              if (result.exceptionDetails) throw new Error(`Page evaluation failed: ${result.exceptionDetails.exception?.description || result.exceptionDetails.text || 'exception'}`);
              return result.result?.value;
            },
            navigate(url, timeoutMs) { return send('Page.navigate', { url }, sessionId, timeoutMs); },
          };
        } catch (error) {
          if (targetId) await closeTargetAndWait(targetId).catch(() => {});
          throw error;
        }
      }
      return {
        pid: child.pid,
        executable,
        profilePath,
        version: { product: version.product || '', revision: version.revision || '', protocolVersion: version.protocolVersion || '' },
        async forSource(source) {
          if (!['x', 'facebook', 'instagram', 'linkedin'].includes(source)) throw new Error('Unsupported Chrome source context.');
          if (sourceContexts.has(source)) return sourceContexts.get(source);
          const context = await createPageContext();
          sourceContexts.set(source, context);
          return context;
        },
        close,
        exited,
        diagnostics: () => stderrTail,
      };
    } catch (error) {
      await close();
      const wrapped = Object.assign(new Error(error.message), { code: error.code || 'chrome_start_failed' });
      if (stderrTail) wrapped.message += ` Chrome diagnostic: ${stderrTail.slice(-1000)}`;
      throw wrapped;
    }
  })();
}

async function terminateOwnedTree(pid) {
  if (!pid) return;
  if (process.platform === 'win32') {
    const systemRoot = process.env.SystemRoot || 'C:\\Windows';
    const taskkill = resolve(systemRoot, 'System32', 'taskkill.exe');
    await new Promise(resolveKill => {
      const killer = spawn(taskkill, ['/PID', String(pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' });
      killer.on('error', resolveKill);
      killer.on('exit', resolveKill);
    });
  } else {
    try { process.kill(pid, 'SIGKILL'); } catch {}
  }
}

// A losing timeout must not keep the Node worker alive after Chrome exits.
export async function waitForExitOrTimeout(exited, milliseconds) {
  let timer;
  try {
    await Promise.race([exited, new Promise(resolveTimeout => {
      timer = setTimeout(resolveTimeout, milliseconds);
    })]);
  } finally {
    clearTimeout(timer);
  }
}
const delay = ms => new Promise(resolveDelay => setTimeout(resolveDelay, ms));
