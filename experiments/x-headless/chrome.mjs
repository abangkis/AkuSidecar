import { spawn } from 'node:child_process';
import { mkdir, open, unlink, readFile } from 'node:fs/promises';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';

export const root = dirname(fileURLToPath(import.meta.url));
export const workspace = resolve(root, '../../..');
export const profile = resolve(root, 'profile');

// Chrome's debugging pipe stays inside this child process: no listening port.
export async function launchChrome({ headed = false, timeoutMs = 15000, existingCapture = null } = {}) {
  const binary = existingCapture?.executable ?? resolve(workspace, 'AkuSidecar/runtime/chromium/bin/chrome.exe');
  const hash = createHash('sha256').update(await readFile(binary)).digest('hex');
  if (!existingCapture) {
    const pin = JSON.parse(await readFile(resolve(dirname(binary), '../pin.json'), 'utf8'));
    if (hash !== pin.executableSha256) throw new Error('Pinned Chrome checksum mismatch.');
  } else {
    if (headed) throw new Error('Existing capture profile mode never opens a visible browser.');
    const { profileOwners } = await import('./capture-profile.mjs');
    if ((await profileOwners(existingCapture.profilePath)).length) throw new Error('Capture profile is still owned by a running Chrome process. Stop it first.');
  }
  await mkdir(profile, { recursive: true });
  const lockPath = resolve(profile, 'poc-owner.lock');
  let lock;
  try { lock = await open(lockPath, 'wx'); }
  catch { throw new Error('PoC profile is already in use (or a stale poc-owner.lock remains). Do not use another running browser profile.'); }
  await lock.writeFile(String(process.pid));
  const args = [
    `--user-data-dir=${existingCapture?.profilePath ?? profile}`,
    ...(existingCapture ? [`--profile-directory=${existingCapture.directory}`] : []),
    '--remote-debugging-pipe', '--no-first-run',
    '--no-default-browser-check', '--disable-component-update',
    '--disable-extensions', '--disable-background-mode',
    ...(headed ? [] : ['--headless']), 'about:blank',
  ];
  const child = spawn(binary, args, { windowsHide: !headed, stdio: ['ignore', 'ignore', 'pipe', 'pipe', 'pipe'] });
  const pending = new Map();
  const documents = [];
  const mediaResponses = new Map();
  let sequence = 0, buffer = '', closed = false;
  let exitResolve;
  const exited = new Promise(r => { exitResolve = r; });
  let stderrTail = '';
  child.stderr.on('data', chunk => { stderrTail = (stderrTail + chunk).slice(-4000); });
  const rejectPending = error => {
    for (const { reject, timer } of pending.values()) { clearTimeout(timer); reject(error); }
    pending.clear();
  };
  child.on('error', error => { closed = true; rejectPending(error); exitResolve(); });
  child.on('exit', () => { closed = true; rejectPending(new Error('PoC Chrome exited.')); exitResolve(); });
  child.stdio[3].on('error', error => rejectPending(error));
  child.stdio[4].setEncoding('utf8');
  child.stdio[4].on('data', chunk => {
    buffer += chunk;
    let index;
    while ((index = buffer.indexOf('\0')) !== -1) {
      const frame = buffer.slice(0, index); buffer = buffer.slice(index + 1);
      if (!frame) continue;
      let message;
      try { message = JSON.parse(frame); } catch { continue; }
      if (message.method === 'Network.responseReceived' && message.params?.type === 'Document') {
        const response = message.params.response;
        documents.push({ url: response.url, status: response.status, mimeType: response.mimeType });
      }
      if (message.method === 'Network.responseReceived') {
        const r = message.params.response;
        try {
          const host = new URL(r.url).hostname;
          if (['pbs.twimg.com', 'video.twimg.com'].includes(host)
            || ['fbcdn.net', 'fbsbx.com'].some(suffix => host === suffix || host.endsWith('.' + suffix))) {
            mediaResponses.set(message.params.requestId, { url: r.url, status: r.status, mimeType: r.mimeType,
              type: message.params.type, fromDiskCache: r.fromDiskCache === true, encodedBytes: null });
          }
        } catch {}
      }
      if (message.method === 'Network.loadingFinished' && mediaResponses.has(message.params.requestId)) {
        mediaResponses.get(message.params.requestId).encodedBytes = message.params.encodedDataLength;
      }
      const call = pending.get(message.id);
      if (!call) continue;
      pending.delete(message.id); clearTimeout(call.timer);
      if (message.error) call.reject(new Error(`CDP ${call.method}: ${message.error.message}`));
      else call.resolve(message.result);
    }
  });
  function send(method, params = {}, sessionId) {
    if (closed) return Promise.reject(new Error('PoC Chrome is closed.'));
    const id = ++sequence;
    return new Promise((resolveCall, reject) => {
      const timer = setTimeout(() => {
        pending.delete(id); reject(new Error(`CDP timeout: ${method}`));
      }, timeoutMs);
      pending.set(id, { resolve: resolveCall, reject, timer, method });
      child.stdio[3].write(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }) + '\0', error => {
        if (error && pending.has(id)) {
          clearTimeout(timer); pending.delete(id); reject(error);
        }
      });
    });
  }
  let closing;
  function close() {
    closing ??= closeOwnedBrowser();
    return closing;
  }
  async function closeOwnedBrowser() {
    try {
      if (!closed) await send('Browser.close').catch(() => {});
      if (!closed) await Promise.race([exited, new Promise(r => setTimeout(r, 2000))]);
      if (!closed) {
        // This PID belongs only to the child created above, never the user's Chrome.
        if (process.platform === 'win32') {
          await new Promise(r => {
            const killer = spawn('taskkill.exe', ['/PID', String(child.pid), '/T', '/F'], { windowsHide: true, stdio: 'ignore' });
            killer.on('error', r); killer.on('exit', r);
          });
        } else child.kill('SIGKILL');
        await Promise.race([exited, new Promise(r => setTimeout(r, 2000))]);
      }
    } finally {
      rejectPending(new Error('PoC browser closed.'));
      await lock.close();
      // Keep the lock if termination could not be confirmed.
      if (closed) await unlink(lockPath);
    }
  }
  try {
    const version = await send('Browser.getVersion');
    const { targetId } = await send('Target.createTarget', { url: 'about:blank' });
    const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
    await send('Page.enable', {}, sessionId);
    await send('Runtime.enable', {}, sessionId);
    await send('Network.enable', {}, sessionId);
    return {
      send: (method, params) => send(method, params, sessionId),
      async evaluate(expression) {
        const result = await send('Runtime.evaluate', { expression, returnByValue: true, awaitPromise: true }, sessionId);
        if (result.exceptionDetails) throw new Error(`Page evaluation failed: ${result.exceptionDetails.exception?.description ?? result.exceptionDetails.text}`);
        return result.result.value;
      },
      close, exited, version, binary, binarySha256: hash,
      documentResponses: () => documents.slice(-10),
      mediaResponses: () => [...mediaResponses.values()].slice(-1000),
      diagnostics: () => stderrTail,
    };
  } catch (error) {
    await close();
    throw new Error(`${error.message}${stderrTail ? '\nChrome startup diagnostics:\n' + stderrTail : ''}`);
  }
}
