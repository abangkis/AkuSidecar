import { spawn, execFile } from 'node:child_process';
import { access, mkdir, realpath } from 'node:fs/promises';
import { constants } from 'node:fs';
import { resolve, isAbsolute } from 'node:path';
import { promisify } from 'node:util';
import {createChromeCommandSender,settleChromeCommandReply} from './chrome-command-queue.mjs';

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

export async function createTemporaryPage({ send, subscribe, browserSessionId, closeTargetAndWait, timeoutMs = CDP_TIMEOUT_MS, retainLateResources = false }) {
  let targetId;
  let allocationsPending=0,targetRetired=false;
  const streams=new Set();
  let closePromise = null;
  const subscriptions=new Set();
  const boundedTimeout = Math.max(1, Math.min(CDP_TIMEOUT_MS, Number.isFinite(timeoutMs) ? timeoutMs : CDP_TIMEOUT_MS));
  const setupDeadline = Date.now() + boundedTimeout;
  const setupTimeLeft = () => {
    const left = Math.trunc(setupDeadline - Date.now());
    if (left < 1) throw Object.assign(new Error('Temporary Chrome page setup timed out.'), { code: 'capture_timeout' });
    return left;
  };
  const retireTarget=async id=>{await closeTargetAndWait(id);if(id===targetId)targetRetired=true;};
  const lateTarget=async value=>{if(typeof value?.targetId!=='string'||!value.targetId)return;await retireTarget(value.targetId);allocationsPending--;};
  const lateSession=async value=>{if(typeof value?.sessionId!=='string'||!value.sessionId)return;if(!targetRetired)await retireTarget(targetId);allocationsPending--;};
  const streamHandle=value=>{if(typeof value?.stream!=='string'||!value.stream||value.stream.length>512||streams.has(value.stream))throw new Error('Chrome stream ownership is unverified.');streams.add(value.stream);return value;};
  try {
    if(retainLateResources)allocationsPending++;
    ({ targetId } = await send('Target.createTarget', {
      url: 'about:blank', background: true, forTab: false,
    }, browserSessionId, setupTimeLeft(),retainLateResources?lateTarget:undefined));
    if (typeof targetId !== 'string' || !targetId) throw new Error('Chrome returned no temporary target identity.');
    if(retainLateResources){allocationsPending--;allocationsPending++;}
    const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true }, browserSessionId, setupTimeLeft(),retainLateResources?lateSession:undefined);
    if(retainLateResources){if(typeof sessionId!=='string'||!sessionId)throw new Error('Chrome returned no temporary session identity.');allocationsPending--;}
    async function sendOwned(method,params={},operationTimeout=boundedTimeout,late){
      if(!retainLateResources)return send(method,params,sessionId,operationTimeout);
      if(closePromise&&!(method==='IO.close'&&streams.has(params.handle)))throw new Error('Owned page is retired.');
      if(method==='Fetch.takeResponseBodyAsStream'){
        allocationsPending++;
        const retireLate=async value=>{
          streamHandle(value);
          if(typeof late==='function')await late(value);else await sendOwned('IO.close',{handle:value.stream},1000);
          if(!streams.has(value.stream))allocationsPending--;
        };
        const value=streamHandle(await send(method,params,sessionId,operationTimeout,retireLate));allocationsPending--;return value;
      }
      if(['IO.read','IO.close'].includes(method)&&!streams.has(params.handle))throw new Error('Chrome stream ownership is unverified.');
      const value=await send(method,params,sessionId,operationTimeout,late);
      if(method==='IO.close')streams.delete(params.handle);return value;
    }
    await send('Page.enable', {}, sessionId, setupTimeLeft());
    await send('Runtime.enable', {}, sessionId, setupTimeLeft());
    await send('Network.enable', {}, sessionId, setupTimeLeft());
    await send('Page.addScriptToEvaluateOnNewDocument', { source: CAPTURE_PLAYBACK_GUARD_SOURCE }, sessionId, setupTimeLeft());
    await send('Emulation.setFocusEmulationEnabled', { enabled: true }, sessionId, setupTimeLeft());
    await send('Emulation.setDeviceMetricsOverride', { width: 1280, height: 900, deviceScaleFactor: 1, mobile: false }, sessionId, setupTimeLeft());
    return {
      on(listener) {
        if(typeof subscribe!=='function'||typeof listener!=='function'||closePromise)throw new Error('Owned page subscription unavailable.');
        let active=true;
        const unsubscribe=subscribe(event=>{if(active&&!closePromise&&event.sessionId===sessionId)listener(event);});
        const off=()=>{if(!active)return;active=false;subscriptions.delete(off);unsubscribe();};
        subscriptions.add(off);return off;
      },
      send:sendOwned,
      async evaluate(expression, operationTimeout = boundedTimeout) {
        const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }, sessionId, operationTimeout);
        if (result.exceptionDetails) throw new Error(`Page evaluation failed: ${result.exceptionDetails.exception?.description || result.exceptionDetails.text || 'exception'}`);
        return result.result?.value;
      },
      navigate(url, operationTimeout = boundedTimeout) { return send('Page.navigate', { url }, sessionId, operationTimeout); },
      close() {
        closePromise ??= Promise.resolve().then(async () => {
          let subscriptionFailure=false;
          for(const off of [...subscriptions])try{off();}catch{subscriptionFailure=true;}
          await retireTarget(targetId);
          streams.clear();
          if(subscriptionFailure||allocationsPending!==0)throw new Error('Owned page subscription cleanup failed.');
        }).catch(() => {
          throw Object.assign(new Error('Chrome could not confirm cleanup of the temporary target.'), { code: 'temporary_target_cleanup_failed' });
        });
        return closePromise;
      },
    };
  } catch (error) {
    if (targetId) {
      try { await retireTarget(targetId); }
      catch {
        throw Object.assign(new Error('Chrome could not confirm cleanup of a failed temporary target.'), { code: 'temporary_target_cleanup_failed' });
      }
    }
    if(retainLateResources&&allocationsPending!==0)throw Object.assign(new Error('Chrome temporary resource cleanup is unverified.'),{code:'temporary_target_cleanup_failed'});
    throw error;
  }
}

export async function launchChrome({ chromePath, profilePath, profileDirectory = 'Default', retainLateResources = false }) {
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
  return connectOwnedChrome(child, executable, userData,retainLateResources);
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

function connectOwnedChrome(child, executable, profilePath,retainLateResources=false) {
  const pending = new Map();
  const listeners = new Set();
  const subscribe = listener => { if(closed||typeof listener!=='function'||listeners.size>=32)throw new Error('Owned browser subscription unavailable.');listeners.add(listener);return()=>listeners.delete(listener); };
  let buffer = '';
  let closed = false;
  let closePromise = null;
  let stderrTail = '';
  let exitResolve;
  let processExitResolve;
  let processSpawnObserved = false;
  let processSpawnFailed = false;
  let processExitObserved = false;
  const exited = new Promise(resolveExit => { exitResolve = resolveExit; });
  const processExited = new Promise(resolveExit => { processExitResolve = resolveExit; });
  child.stderr.setEncoding('utf8');
  child.stderr.on('data', chunk => { stderrTail = (stderrTail + chunk).slice(-3000); });
  child.once('spawn', () => { processSpawnObserved = true; });
  child.on('error', error => {
    if (!processSpawnObserved && child.pid === undefined) processSpawnFailed = true;
    closePending(error);
    markClosed();
  });
  child.on('exit', code => {
    processExitObserved = true;
    processExitResolve();
    closePending(new Error(`Chrome exited${code === null ? '' : ` (${code})`}.`));
    markClosed();
  });
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
      if(!Object.hasOwn(message,'id')&&typeof message.method==='string'){
        for(const listener of listeners)try{listener(message);}catch{}
        continue;
      }
      settleChromeCommandReply(pending,message);
    }
  });

  function markClosed() {
    if (closed) return;
    closed = true;
    listeners.clear();
    exitResolve();
  }
  function closePending(error) {
    for (const [id, call] of pending) {
      clearTimeout(call.timer);
      call.reject(error);
      pending.delete(id);
    }
  }
  const send=createChromeCommandSender({pending,write:(wire,callback)=>child.stdio[3].write(wire,callback),isClosed:()=>closed,retainLateResources,timeoutMS:CDP_TIMEOUT_MS});
  async function close() {
    closePromise ??= closeOwned();
    return closePromise;
  }
  async function closeOwned() {
    const processGone = () => processExitObserved || processSpawnFailed;
    try {
      if (!closed) await send('Browser.close').catch(() => {});
      if (!processGone()) await waitForExitOrTimeout(processExited, 2500);
      if (!processGone()) {
        await terminateOwnedTree(child.pid);
        if (!processGone()) await waitForExitOrTimeout(processExited, 2500);
      }
      if (!processGone()) {
        throw Object.assign(new Error('Chrome process exit could not be verified after owned-process cleanup.'), { code: 'owned_chrome_cleanup_failed' });
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
        const owned = await createTemporaryPage({ send, subscribe, browserSessionId, closeTargetAndWait });
        const { close, ...context } = owned;
        void close; // Source targets are released together with the owned Chrome process.
        return context;
      }
      return {
        backend: 'headless_worker',
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
        createTemporaryPage(timeoutMs) {
          return createTemporaryPage({ send, subscribe, browserSessionId, closeTargetAndWait, timeoutMs, retainLateResources });
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
