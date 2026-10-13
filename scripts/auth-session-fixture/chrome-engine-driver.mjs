// Narrow RPC bridge for the opt-in Go/Chrome integration. It accepts no
// profile path, CDP endpoint, URL, cookie value, or arbitrary CDP command.
import readline from 'node:readline';
import { launchFixtureChrome } from './chrome.mjs';

const [executable, root, origin] = process.argv.slice(2);
if (process.argv.length !== 5 || !executable || !root || !origin) {
  process.stderr.write('fixture_driver_arguments_invalid\n');
  process.exit(2);
}

let current = null;
let currentPage = null;
let currentPages = new Map();
let currentBootstrapTarget = '';
let currentOwner = 0;
let nextOwner = 0;
const owners = new Map();
const seenExit = new Set();

function emit(value) { process.stdout.write(JSON.stringify(value) + '\n'); }

async function waitChromeRootExit(browser) {
  // On Windows the private pipe can close before Node receives Chrome's exit
  // event. Give the explicitly requested final-window close a bounded window
  // to publish that exact root exit; never infer exit from the pipe closing.
  for (let attempt = 0; attempt < 400 && !browser.isExited(); attempt++) {
    await new Promise(resolve => setTimeout(resolve, 25));
  }
  return browser.isExited();
}

function cleanError(error) {
  const message = typeof error?.message === 'string' ? error.message : '';
  const cdp = /^fixture_cdp_([A-Za-z]+\.[A-Za-z]+)_(failed|timeout)$/.exec(message);
  const methods = {
    'Target.getTargets': 'target_get_targets',
    'Browser.getWindowForTarget': 'browser_get_window_for_target',
    'Target.closeTarget': 'target_close_target',
    'Target.createTarget': 'target_create_target',
    'Target.attachToTarget': 'target_attach_to_target',
    'Page.enable': 'page_enable',
    'Page.navigate': 'page_navigate',
    'Runtime.evaluate': 'runtime_evaluate',
    'Network.enable': 'network_enable',
    'Network.deleteCookies': 'network_delete_cookies',
    'Browser.close': 'browser_close',
  };
  const code = /^fixture_[a-z0-9_]+$/.test(message) ? message
    : cdp && methods[cdp[1]] ? `fixture_cdp_${methods[cdp[1]]}_${cdp[2]}` : 'fixture_bridge_failed';
  return code;
}

async function inventory(browser) {
  if (!browser || browser.isExited()) throw new Error('fixture_chrome_closed');
  for (let attempt = 0; attempt < 4; attempt++) {
    const result = await browser.send('Target.getTargets');
    if (!Array.isArray(result?.targetInfos)) throw new Error('fixture_inventory_unknown');
    const windows = new Map();
    let vanished = false;
    for (const target of result.targetInfos) {
      if (!target || target.type !== 'page') continue;
      if (typeof target.targetId !== 'string' || !target.targetId || typeof target.url !== 'string') {
        throw new Error('fixture_inventory_unknown');
      }
      const isBootstrap = target.targetId === currentBootstrapTarget;
      if (target.url !== 'about:blank' && target.url !== origin && !target.url.startsWith(origin + '/')) {
        throw new Error('fixture_inventory_unknown');
      }
      let located;
      try {
        located = await browser.send('Browser.getWindowForTarget', { targetId: target.targetId });
      } catch {
        const currentTargets = await browser.send('Target.getTargets');
        if (!Array.isArray(currentTargets?.targetInfos)) throw new Error('fixture_inventory_unknown');
        if (!currentTargets.targetInfos.some(item => item?.targetId === target.targetId)) {
          vanished = true;
          break;
        }
        throw new Error('fixture_inventory_unknown');
      }
      if (!Number.isInteger(located?.windowId)) throw new Error('fixture_inventory_unknown');
      let value = windows.get(located.windowId);
      if (!value) {
        value = { id: located.windowId, bootstrap: false, targetId: target.targetId };
        windows.set(located.windowId, value);
      }
      value.bootstrap ||= isBootstrap;
    }
    if (!vanished) return [...windows.values()].sort((a, b) => a.id - b.id);
  }
  throw new Error('fixture_inventory_unknown');
}

async function launch(role) {
  if (!['collector', 'owner'].includes(role)) throw new Error('fixture_role_invalid');
  if (current && !current.isExited()) throw new Error('fixture_profile_still_owned');
  const browser = await launchFixtureChrome(executable, root, role, 'shared');
  let page = null;
  let bootstrapPage = null;
  let bootstrapTarget = '';
  if (role === 'collector') {
    page = await browser.page(origin);
  } else {
    const initial = await browser.send('Target.getTargets');
    const candidates = Array.isArray(initial?.targetInfos)
      ? initial.targetInfos.filter(target => target?.type === 'page' && target.url === 'about:blank') : [];
    if (candidates.length !== 1 || typeof candidates[0].targetId !== 'string' || !candidates[0].targetId) {
      throw new Error('fixture_bootstrap_failed');
    }
    bootstrapTarget = candidates[0].targetId;
    bootstrapPage = await browser.pageForTarget(origin, bootstrapTarget);
    page = await browser.page(origin);
  }
  const owner = ++nextOwner;
  current = browser;
  currentPage = page;
  currentPages = new Map([[page.targetId, page]]);
  if (bootstrapPage) currentPages.set(bootstrapPage.targetId, bootstrapPage);
  currentBootstrapTarget = bootstrapTarget;
  currentOwner = owner;
  owners.set(owner, { browser, role });
  const windows = await inventory(browser);
  return { owner, pid: browser.pid, windowCount: windows.length,
    bootstrapCount: windows.filter(window => window.bootstrap).length };
}

async function execute(message) {
  if (!message || typeof message !== 'object' || !Number.isSafeInteger(message.id) || typeof message.op !== 'string') {
    throw new Error('fixture_rpc_invalid');
  }
  switch (message.op) {
    case 'launch':
      return await launch(message.role);
    case 'probe': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!currentPage) throw new Error('fixture_chrome_unavailable');
      const response = await currentPage.request('/probe');
      if (response?.status !== 200 || !response.body || !['alpha', 'beta'].includes(response.body.account) ||
          !Number.isSafeInteger(response.body.epoch)) return { authenticated: false };
      return { authenticated: true, account: response.body.account, revision: `fixture-${response.body.epoch}` };
    }
    case 'clear_fixture_cookie': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!current || current.isExited()) throw new Error('fixture_chrome_unavailable');
      if (!currentPage) throw new Error('fixture_chrome_unavailable');
      await currentPage.clearFixtureCookie();
      return { cleared: true };
    }
    case 'login':
    case 'account_change':
    case 'logout':
    case 'collect': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!currentPage) throw new Error('fixture_chrome_unavailable');
      const routes = { login: ['/login?account=alpha', 'POST'], account_change: ['/login?account=beta', 'POST'],
        logout: ['/logout', 'POST'], collect: ['/collect', 'POST'] };
      const [path, method] = routes[message.op];
      const response = await currentPage.request(path, method);
      return { status: Number.isInteger(response?.status) ? response.status : 0,
        account: ['alpha', 'beta'].includes(response?.body?.account) ? response.body.account : '',
        epoch: Number.isSafeInteger(response?.body?.epoch) ? response.body.epoch : 0,
        item: typeof response?.body?.postId === 'string' && /^fixture-post-[1-9][0-9]{0,5}$/.test(response.body.postId) };
    }
    case 'inventory': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!current || current.isExited()) return { exited: true, windowCount: 0, bootstrapCount: 0, bootstrap: [] };
      const windows = await inventory(current);
      return { exited: false, windowCount: windows.length, bootstrapCount: windows.filter(window => window.bootstrap).length,
        bootstrap: windows.map(window => window.bootstrap) };
    }
    case 'open_reader': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!current || current.role !== 'owner') throw new Error('fixture_reader_unavailable');
      currentPage = await current.page(origin);
      currentPages.set(currentPage.targetId, currentPage);
      const windows = await inventory(current);
      return { windowCount: windows.length };
    }
    case 'close_window': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!Number.isSafeInteger(message.index) || message.index < 0) throw new Error('fixture_window_index_invalid');
      const windows = await inventory(current);
      const selected = windows[message.index];
      if (!selected) throw new Error('fixture_window_index_invalid');
      const finalWindow = windows.length === 1;
      let result;
      try {
        result = await current.send('Target.closeTarget', { targetId: selected.targetId });
      } catch (error) {
        if (finalWindow && error?.message === 'fixture_chrome_closed' && await waitChromeRootExit(current)) {
          return { requested: true, exited: true };
        }
        throw error;
      }
      if (result?.success !== true) throw new Error('fixture_window_close_failed');
      for (let attempt = 0; attempt < 80; attempt++) {
        if (current.isExited()) {
          if (finalWindow) return { requested: true, exited: true };
          throw new Error('fixture_window_close_unverified');
        }
        let targets;
        try { targets = await current.send('Target.getTargets'); }
        catch (error) {
          if (finalWindow && error?.message === 'fixture_chrome_closed' && await waitChromeRootExit(current)) {
            return { requested: true, exited: true };
          }
          throw error;
        }
        if (!Array.isArray(targets?.targetInfos)) throw new Error('fixture_inventory_unknown');
        if (!targets.targetInfos.some(target => target?.targetId === selected.targetId)) {
          currentPages.delete(selected.targetId);
          if (currentPage?.targetId === selected.targetId) {
            const remaining = targets.targetInfos.find(target => target?.type === 'page' &&
              currentPages.has(target.targetId) && (target.url === origin || target.url.startsWith(origin + '/')));
            currentPage = remaining ? currentPages.get(remaining.targetId) : null;
          }
          return { requested: true, exited: false };
        }
        await new Promise(resolve => setTimeout(resolve, 25));
      }
      throw new Error('fixture_window_close_unverified');
    }
    case 'release': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!current || current.isExited()) return { exited: true };
      const windows = await inventory(current);
      if (current.role === 'owner' && windows.length !== 0) throw new Error('fixture_reader_windows_remain');
      await current.close();
      return { exited: current.isExited() };
    }
    case 'disconnect': {
      if (message.owner !== currentOwner) throw new Error('fixture_owner_stale');
      if (!current || current.isExited()) throw new Error('fixture_chrome_unavailable');
      current.disconnectForFixtureTest();
      return { disconnected: true };
    }
    default:
      throw new Error('fixture_rpc_operation_invalid');
  }
}

function pollExits() {
  for (const [owner, record] of owners) {
    if (record.browser.isExited() && !seenExit.has(owner)) {
      seenExit.add(owner);
      emit({ event: 'chrome_exit', owner });
    }
  }
}

emit({ ready: true });
setInterval(pollExits, 50).unref();
const input = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
let chain = Promise.resolve();
input.on('line', line => {
  chain = chain.then(async () => {
    let message;
    try { message = JSON.parse(line); } catch { emit({ id: 0, ok: false, code: 'fixture_rpc_invalid' }); return; }
    try { emit({ id: message?.id, ok: true, result: await execute(message) }); }
    catch (error) { emit({ id: message?.id, ok: false, code: cleanError(error) }); }
  });
});
