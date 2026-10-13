// Fixture-only private pipe transport. Never accepts an existing profile or CDP endpoint.
import { spawn } from 'node:child_process';
import { mkdir, realpath } from 'node:fs/promises';
import { join, relative, isAbsolute } from 'node:path';
import { setTimeout as delay } from 'node:timers/promises';

const ownedProfiles = new Map();

export async function launchFixtureChrome(executable, root, role) {
  if (!['owner', 'collector'].includes(role)) throw new Error('invalid_fixture_role');
  root = await realpath(root);
  const profile = join(root, role);
  // Reopen only a profile created by this process after its exact child has exited.
  if (ownedProfiles.has(profile)) {
    if (!ownedProfiles.get(profile)()) throw new Error('fixture_profile_still_owned');
  } else await mkdir(profile);
  const resolved = await realpath(profile);
  const rel = relative(root, resolved);
  if (isAbsolute(rel) || rel.startsWith('..')) throw new Error('invalid_fixture_profile');
  const child = spawn(executable, [
    `--user-data-dir=${resolved}`, '--remote-debugging-pipe', '--no-first-run',
    '--no-default-browser-check', '--disable-extensions', '--disable-background-networking',
    '--disable-component-update', '--disable-sync', '--disable-background-mode',
    '--disable-default-apps', '--no-proxy-server', '--metrics-recording-only',
    '--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE localhost, EXCLUDE 127.0.0.1',
    ...(role === 'collector' ? ['--headless=new', '--no-startup-window'] : ['--start-minimized', 'about:blank']),
  ], { windowsHide: true, stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  let exited = false, sequence = 0, buffer = '';
  ownedProfiles.set(profile, () => exited);
  const pending = new Map();
  const rejectPending = () => {
    for (const item of pending.values()) { clearTimeout(item.timer); item.reject(new Error('fixture_chrome_closed')); }
    pending.clear();
  };
  child.on('error', () => { exited = true; rejectPending(); });
  child.on('exit', () => { exited = true; rejectPending(); });
  child.stdio[3].on('error', rejectPending);
  child.stdio[4].on('error', rejectPending);
  child.stdio[4].setEncoding('utf8');
  child.stdio[4].on('data', data => {
    buffer += data;
    for (let end; (end = buffer.indexOf('\0')) >= 0;) {
      const raw = buffer.slice(0, end); buffer = buffer.slice(end + 1);
      let message; try { message = JSON.parse(raw); } catch { continue; }
      const item = pending.get(message.id);
      if (!item) continue;
      pending.delete(message.id); clearTimeout(item.timer);
      // CDP errors can contain input secrets. Expose only the method, never protocol payloads.
      if (message.error) item.reject(new Error(`fixture_cdp_${item.method}_failed`));
      else item.resolve(message.result);
    }
  });
  function send(method, params = {}, sessionId) {
    if (exited) return Promise.reject(new Error('fixture_chrome_closed'));
    return new Promise((resolve, reject) => {
      const id = ++sequence;
      const timer = setTimeout(() => { pending.delete(id); reject(new Error(`fixture_cdp_${method}_timeout`)); }, 8000);
      pending.set(id, { resolve, reject, timer, method });
      child.stdio[3].write(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }) + '\0');
    });
  }
  async function close() {
    if (!exited) await send('Browser.close').catch(() => {});
    for (let i = 0; i < 80 && !exited; i++) await delay(100);
    if (!exited) throw new Error('fixture_chrome_exit_unverified');
    rejectPending();
  }
  async function page(origin, contextId) {
    const { targetId } = await send('Target.createTarget', {
      url: 'about:blank', background: true,
      ...(contextId ? { browserContextId: contextId } : { newWindow: true, windowState: 'minimized' }),
    });
    const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
    await send('Page.enable', {}, sessionId);
    async function evaluate(expression) {
      const result = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true }, sessionId);
      if (result.exceptionDetails) throw new Error('fixture_evaluation_failed');
      return result.result?.value;
    }
    const nav = await send('Page.navigate', { url: origin + '/' }, sessionId);
    if (nav.errorText) throw new Error(/^net::ERR_[A-Z_]+$/.test(nav.errorText) ? 'fixture_navigation_' + nav.errorText.slice(5) : 'fixture_navigation_failed');
    let ready = false;
    for (let i = 0; i < 80; i++) {
      ready = await evaluate(`location.origin === ${JSON.stringify(origin)} && document.readyState === 'complete'`).catch(() => false);
      if (ready) break;
      await delay(50);
    }
    if (!ready) throw new Error('fixture_page_not_ready');
    return {
      targetId, evaluate,
      request(path, method = 'GET') {
        if (!/^\/[a-z0-9/?=&_-]*$/i.test(path) || !['GET', 'POST'].includes(method)) throw new Error('fixture_invalid_request');
        return evaluate(`fetch(${JSON.stringify(path)}, {method:${JSON.stringify(method)}, cache:'no-store'}).then(async r => ({status:r.status, body:await r.json()}))`);
      },
      close: () => send('Target.closeTarget', { targetId }),
    };
  }
  try {
    const version = await send('Browser.getVersion');
    return { send, page, close, pid: child.pid, profile: resolved, role, product: version.product, isExited: () => exited };
  } catch (error) { await close(); throw error; }
}
