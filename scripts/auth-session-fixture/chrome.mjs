// Fixture-only private pipe transport. Ordinary launches reject existing profiles;
// process recovery requires an owned QA lineage through gated private IPC.
import { spawn } from 'node:child_process';
import { lstat, mkdir, realpath, readFile, writeFile } from 'node:fs/promises';
import { join, relative, isAbsolute, dirname, basename } from 'node:path';
import { fileURLToPath } from 'node:url';
import { setTimeout as delay } from 'node:timers/promises';

const ownedProfiles = new Map();
const processScopes = new Map();
const processScopeParent = join(dirname(fileURLToPath(import.meta.url)), '..', '..', 'runtime', 'qa', 'auth-session-sidecar');
async function checkedProcessRecord(root) {
  const path = join(root, 'process-profile-lineage.json');
  const stat = await lstat(path);
  if (!stat.isFile() || stat.isSymbolicLink() || stat.size > 4096 || await realpath(path) !== path) {
    throw new Error('fixture_process_record_invalid');
  }
  return path;
}

// Only the tagged Go process fixture calls this through its gated private RPC.
// No ordinary launcher option permits an existing profile.
export async function scopeProcessFixtureProfile(root, nonce, reopen) {
  if (!/^[a-f0-9]{64}$/.test(nonce) || typeof reopen !== 'boolean' || processScopes.has(root)) {
    throw new Error('fixture_process_scope_invalid');
  }
  if (await realpath(root) !== root || (await lstat(root)).isSymbolicLink()) throw new Error('fixture_process_scope_invalid');
  const parent = await realpath(processScopeParent);
  const rootRelative = relative(parent, root);
  if (!rootRelative || isAbsolute(rootRelative) || rootRelative.startsWith('..') || !basename(root).startsWith('run-')) {
    throw new Error('fixture_process_scope_outside_qa');
  }
  const profile = join(root, 'shared');
  if (reopen) {
    const record = JSON.parse(await readFile(await checkedProcessRecord(root), 'utf8'));
    const stat = await lstat(profile, { bigint: true });
    if (record.nonce !== nonce || record.root !== root || !Number.isSafeInteger(record.pid) ||
        !stat.isDirectory() || stat.isSymbolicLink() || record.ino !== String(stat.ino) ||
        record.dev !== String(stat.dev) || record.birth !== String(stat.birthtimeNs) ||
        await realpath(profile) !== profile) throw new Error('fixture_process_profile_identity_invalid');
    let alive = false;
    try { process.kill(record.pid, 0); alive = true; } catch (error) { if (error.code !== 'ESRCH') throw error; }
    if (alive) throw new Error('fixture_process_profile_still_owned');
    ownedProfiles.set(profile, () => true);
  } else {
    try { await lstat(profile); throw new Error('fixture_process_profile_not_new'); }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
  }
  processScopes.set(root, nonce);
}

function fixtureOrigin(value) {
  let url;
  try { url = new URL(value); } catch { throw new Error('invalid_fixture_origin'); }
  if (typeof value !== 'string' || url.protocol !== 'http:' || url.hostname !== '127.0.0.1' ||
      !url.port || url.username || url.password || url.pathname !== '/' || url.search || url.hash ||
      value !== url.origin && value !== `${url.origin}/`) {
    throw new Error('invalid_fixture_origin');
  }
  return url.origin;
}

function fixturePath(path) {
  if (typeof path !== 'string' || !/^\/(?!\/)[a-z0-9/?=&_-]*$/i.test(path)) {
    throw new Error('fixture_invalid_request');
  }
  return path;
}

export async function launchFixtureChrome(executable, root, role, profileName = role, startup = 'retained') {
  if (!['owner', 'collector'].includes(role)) throw new Error('invalid_fixture_role');
  if (!['retained', 'native_reader'].includes(startup) || startup === 'native_reader' && role !== 'owner') {
    throw new Error('invalid_fixture_startup');
  }
  if (!['owner', 'collector', 'shared'].includes(profileName) ||
      (profileName !== role && profileName !== 'shared')) throw new Error('invalid_fixture_profile');
  const requestedRoot = root;
  const rootStat = await lstat(requestedRoot);
  if (!rootStat.isDirectory() || rootStat.isSymbolicLink()) throw new Error('invalid_fixture_root');
  root = await realpath(requestedRoot);
  if (root !== requestedRoot) throw new Error('invalid_fixture_root');
  const profile = join(root, profileName);
  // Reopen only a profile created by this process after its exact child has exited.
  if (ownedProfiles.has(profile)) {
    if (!ownedProfiles.get(profile)()) throw new Error('fixture_profile_still_owned');
  } else await mkdir(profile);
  const profileStat = await lstat(profile);
  if (!profileStat.isDirectory() || profileStat.isSymbolicLink()) throw new Error('invalid_fixture_profile');
  const resolved = await realpath(profile);
  const rel = relative(root, resolved);
  if (isAbsolute(rel) || rel.startsWith('..')) throw new Error('invalid_fixture_profile');
  if (processScopes.has(root)) {
    const stat = await lstat(profile, { bigint: true });
    const recordPath = join(root, 'process-profile-lineage.json');
    let flag = 'wx';
    try { await checkedProcessRecord(root); flag = 'w'; }
    catch (error) { if (error.code !== 'ENOENT') throw error; }
    await writeFile(recordPath, JSON.stringify({
      root, nonce: processScopes.get(root), pid: process.pid,
      ino: String(stat.ino), dev: String(stat.dev), birth: String(stat.birthtimeNs),
    }), { flag, mode: 0o600 });
  }
  const child = spawn(executable, [
    `--user-data-dir=${resolved}`, '--remote-debugging-pipe', '--no-first-run',
    '--no-default-browser-check', '--disable-extensions', '--disable-background-networking',
    '--disable-component-update', '--disable-sync', '--disable-background-mode',
    '--disable-default-apps', '--no-proxy-server', '--metrics-recording-only',
    '--host-resolver-rules=MAP * ~NOTFOUND, EXCLUDE localhost, EXCLUDE 127.0.0.1',
    ...(role === 'collector' ? ['--headless=new', '--no-startup-window']
      : startup === 'native_reader' ? ['--new-window', 'about:blank'] : ['--start-minimized', 'about:blank']),
  ], { windowsHide: startup !== 'native_reader', stdio: ['ignore', 'ignore', 'ignore', 'pipe', 'pipe'] });
  let exited = false, transportClosed = false, sequence = 0, buffer = '';
  ownedProfiles.set(profile, () => exited);
  const pending = new Map();
  const sessionListeners = new Map();
  const protocolListeners = new Set();
  function notifyUnavailable(listeners) {
    for (const listener of [...listeners]) {
      try { listener.onUnavailable(); } catch { /* event callbacks must not break the private pipe */ }
    }
  }
  function markTransportClosed() {
    if (transportClosed) return;
    transportClosed = true;
    notifyUnavailable(protocolListeners);
    for (const listeners of sessionListeners.values()) notifyUnavailable(listeners);
    rejectPending();
  }
  const rejectPending = () => {
    for (const item of pending.values()) { clearTimeout(item.timer); item.reject(new Error('fixture_chrome_closed')); }
    pending.clear();
  };
  child.on('error', () => { exited = true; markTransportClosed(); });
  child.on('exit', () => { exited = true; markTransportClosed(); });
  child.stdio[3].on('error', markTransportClosed);
  child.stdio[4].on('error', markTransportClosed);
  child.stdio[3].on('close', markTransportClosed);
  child.stdio[4].on('close', markTransportClosed);
  child.stdio[4].setEncoding('utf8');
  child.stdio[4].on('data', data => {
    buffer += data;
    for (let end; (end = buffer.indexOf('\0')) >= 0;) {
      const raw = buffer.slice(0, end); buffer = buffer.slice(end + 1);
      let message;
      try { message = JSON.parse(raw); } catch {
        notifyUnavailable(protocolListeners);
        for (const listeners of sessionListeners.values()) notifyUnavailable(listeners);
        continue;
      }
      if (!message || typeof message !== 'object') {
        notifyUnavailable(protocolListeners);
        for (const listeners of sessionListeners.values()) notifyUnavailable(listeners);
        continue;
      }
      const item = pending.get(message.id);
      if (item) {
        pending.delete(message.id); clearTimeout(item.timer);
        // CDP errors can contain input secrets. Expose only the method, never protocol payloads.
        if (message.error) item.reject(new Error(`fixture_cdp_${item.method}_failed`));
        else item.resolve(message.result);
        continue;
      }
      if (typeof message.method !== 'string') continue;
      for (const listener of [...protocolListeners]) {
        try { listener.onEvent(message); } catch { try { listener.onUnavailable(); } catch { /* private event boundary */ } }
      }
      if (message.sessionId) {
        const listeners = sessionListeners.get(message.sessionId);
        if (listeners) {
          for (const listener of [...listeners]) {
            if (!message.method.startsWith('Network.')) continue;
            try { listener.onEvent(message); } catch { try { listener.onUnavailable(); } catch { /* private event boundary */ } }
          }
        }
      }
      if (message.method === 'Target.detachedFromTarget' && typeof message.params?.sessionId === 'string') {
        notifyUnavailable(sessionListeners.get(message.params.sessionId) ?? []);
      } else if (message.method === 'Target.targetCrashed' && typeof message.params?.targetId === 'string') {
        for (const listeners of sessionListeners.values()) {
          if ([...listeners].some(listener => listener.targetId === message.params.targetId)) notifyUnavailable(listeners);
        }
      }
    }
  });
  function send(method, params = {}, sessionId) {
    if (exited || transportClosed) return Promise.reject(new Error('fixture_chrome_closed'));
    return new Promise((resolve, reject) => {
      const id = ++sequence;
      const timer = setTimeout(() => { pending.delete(id); reject(new Error(`fixture_cdp_${method}_timeout`)); }, 8000);
      pending.set(id, { resolve, reject, timer, method });
      child.stdio[3].write(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }) + '\0');
    });
  }
  function sendSession(sessionId, method, params = {}) {
    if (typeof sessionId !== 'string' || sessionId.length < 1 || sessionId.length > 256 ||
        typeof method !== 'string' || !/^[A-Za-z][A-Za-z0-9]*\.[A-Za-z][A-Za-z0-9]*$/.test(method) ||
        !params || typeof params !== 'object' || Array.isArray(params)) {
      return Promise.reject(new Error('fixture_invalid_cdp_session_command'));
    }
    return send(method, params, sessionId);
  }
  function subscribeProtocol(onEvent, onUnavailable = () => {}) {
    if (typeof onEvent !== 'function' || typeof onUnavailable !== 'function') throw new Error('fixture_invalid_protocol_listener');
    if (transportClosed || exited) throw new Error('fixture_chrome_closed');
    const listener = { onEvent, onUnavailable };
    protocolListeners.add(listener);
    return () => protocolListeners.delete(listener);
  }
  async function close() {
    if (!exited) await send('Browser.close').catch(() => {});
    for (let i = 0; i < 80 && !exited; i++) await delay(100);
    if (!exited) throw new Error('fixture_chrome_exit_unverified');
    rejectPending();
  }
  async function page(origin, contextId) {
    origin = fixtureOrigin(origin);
    const { targetId } = await send('Target.createTarget', {
      url: 'about:blank', background: true,
      ...(contextId ? { browserContextId: contextId } : { newWindow: true, windowState: 'minimized' }),
    });
    return attachPage(origin, targetId);
  }
  async function pageForTarget(origin, targetId) {
    origin = fixtureOrigin(origin);
    if (typeof targetId !== 'string' || targetId.length < 1 || targetId.length > 256) throw new Error('fixture_invalid_target');
    return attachPage(origin, targetId);
  }
  async function attachPage(origin, targetId) {
    const { sessionId } = await send('Target.attachToTarget', { targetId, flatten: true });
    await send('Page.enable', {}, sessionId);
    async function evaluate(expression) {
      const result = await send('Runtime.evaluate', { expression, awaitPromise: true, returnByValue: true }, sessionId);
      if (result.exceptionDetails) throw new Error('fixture_evaluation_failed');
      return result.result?.value;
    }
    const requestPath = fixturePath('/');
    const nav = await send('Page.navigate', { url: origin + requestPath }, sessionId);
    if (nav.errorText) throw new Error(/^net::ERR_[A-Z_]+$/.test(nav.errorText) ? 'fixture_navigation_' + nav.errorText.slice(5) : 'fixture_navigation_failed');
    let ready = false;
    for (let i = 0; i < 80; i++) {
      ready = await evaluate(`location.origin === ${JSON.stringify(origin)} && document.readyState === 'complete'`).catch(() => false);
      if (ready) break;
      await delay(50);
    }
    if (!ready) throw new Error('fixture_page_not_ready');
    let networkEnabled = false;
    let enablingNetwork;
    const listeners = new Set();
    async function subscribeNetwork(onEvent, onUnavailable = () => {}) {
      if (typeof onEvent !== 'function' || typeof onUnavailable !== 'function') throw new Error('fixture_invalid_network_listener');
      if (transportClosed || exited) throw new Error('fixture_chrome_closed');
      const listener = { onEvent, onUnavailable, targetId };
      if (!sessionListeners.has(sessionId)) sessionListeners.set(sessionId, new Set());
      sessionListeners.get(sessionId).add(listener);
      listeners.add(listener);
      if (!networkEnabled) {
        enablingNetwork ??= send('Network.enable', {}, sessionId).then(() => { networkEnabled = true; });
        try { await enablingNetwork; } catch (error) {
          sessionListeners.get(sessionId)?.delete(listener); listeners.delete(listener);
          throw error;
        }
      }
      return () => {
        sessionListeners.get(sessionId)?.delete(listener);
        listeners.delete(listener);
        if (sessionListeners.get(sessionId)?.size === 0) sessionListeners.delete(sessionId);
      };
    }
    async function navigate(path) {
      const safePath = fixturePath(path);
      const result = await send('Page.navigate', { url: origin + safePath }, sessionId);
      if (result.errorText) throw new Error(/^net::ERR_[A-Z_]+$/.test(result.errorText) ? 'fixture_navigation_' + result.errorText.slice(5) : 'fixture_navigation_failed');
      let loaded = false;
      for (let i = 0; i < 80; i++) {
        loaded = await evaluate(`location.origin === ${JSON.stringify(origin)} && document.readyState === 'complete'`).catch(() => false);
        if (loaded) break;
        await delay(50);
      }
      if (!loaded) throw new Error('fixture_page_not_ready');
      return true;
    }
    return {
      targetId, origin, evaluate, navigate, subscribeNetwork,
      async networkBarrier() {
        const result = await send('Runtime.evaluate', { expression: 'void 0', returnByValue: true }, sessionId);
        if (result.exceptionDetails) throw new Error('fixture_network_barrier_failed');
      },
      request(path, method = 'GET', options = {}) {
        const safePath = fixturePath(path);
        if (!['GET', 'POST'].includes(method) || !options || typeof options !== 'object' || Array.isArray(options) ||
            Object.keys(options).some(key => key !== 'refreshNonce') ||
            (options.refreshNonce !== undefined && !/^[a-f0-9]{64}$/i.test(options.refreshNonce))) {
          throw new Error('fixture_invalid_request');
        }
        const headers = options.refreshNonce ? { 'x-aku-fixture-refresh-nonce': options.refreshNonce } : undefined;
        return evaluate(`fetch(${JSON.stringify(safePath)}, {method:${JSON.stringify(method)}, cache:'no-store'${headers ? `, headers:${JSON.stringify(headers)}` : ''}}).then(async r => ({status:r.status, body:await r.json()}))`);
      },
      clearFixtureCookie: () => send('Network.deleteCookies', { name: 'fixture_session', url: origin + '/' }, sessionId),
      close: () => send('Target.closeTarget', { targetId }),
    };
  }
  try {
    const version = await send('Browser.getVersion');
    return { send, sendSession, subscribeProtocol, page, pageForTarget, close, pid: child.pid, profile: resolved, role, startup, product: version.product,
      isExited: () => exited,
      disconnectForFixtureTest() {
        markTransportClosed();
        child.stdio[3].destroy(); child.stdio[4].destroy();
      } };
  } catch (error) { await close(); throw error; }
}
