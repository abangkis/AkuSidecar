// Separate driver for the dual-tag, fresh-profile synthetic integration. The
// old single-profile fixture driver and retained/native drivers are unchanged.
import readline from 'node:readline';
import { launchFixtureChrome } from './chrome.mjs';
import { inventoryFixtureWindows, closeFixtureWindow } from './chrome-window-lifecycle.mjs';
import { concurrentEngineCommand, concurrentFixtureCookie } from './concurrent-engine-commands.mjs';

const [executable, root, origin] = process.argv.slice(2);
if (process.argv.length !== 5) process.exit(2);
// Validate the fixed origin before starting Chrome, without accepting a seed.
concurrentFixtureCookie('0'.repeat(64), origin);
let browser, page, bootstrapTarget = '', owner = 0, seeded = false, loggedIn = false, ready = false, exitSent = false;
const pages = new Map();
const emit = message => process.stdout.write(JSON.stringify(message) + '\n');
const inventory = () => inventoryFixtureWindows(browser, origin, bootstrapTarget);
const waitRootExit = async value => {
  for (let i = 0; i < 400 && !value.isExited(); i++) await new Promise(resolve => setTimeout(resolve, 25));
  return value.isExited();
};
async function execute(message) {
  if (message.op === 'launch') {
    if (owner !== 0) throw new Error('fixture_concurrent_launch_reuse');
    browser = await launchFixtureChrome(executable, root, message.role, 'shared', message.nativeReaderStartup ? 'native_reader' : 'retained');
    if (message.role === 'owner') {
      const targets = await browser.send('Target.getTargets');
      const blanks = targets.targetInfos?.filter(value => value.type === 'page' && value.url === 'about:blank');
      if (blanks?.length !== 1) throw new Error('fixture_concurrent_bootstrap_invalid');
      bootstrapTarget = blanks[0].targetId;
      page = await browser.pageForTarget(origin, bootstrapTarget);
    } else page = await browser.page(origin);
    pages.set(page.targetId, page); owner = 1;
    const windows = await inventory();
    return { owner, pid: browser.pid, nativeReaderStartup: message.nativeReaderStartup, windowCount: windows.length, bootstrapCount: windows.filter(value => value.bootstrap).length };
  }
  if (!browser || message.owner !== owner) throw new Error('fixture_concurrent_owner_stale');
  switch (message.op) {
    case 'seed_fixture':
      if (browser.role !== 'collector' || seeded) throw new Error('fixture_concurrent_seed_reuse');
      await browser.send('Storage.setCookies', { cookies: [concurrentFixtureCookie(message.value, origin)] });
      seeded = true; return { seeded: true };
    case 'login': {
      if (browser.role !== 'owner' || loggedIn) throw new Error('fixture_concurrent_login_reuse');
      const response = await page.request('/login', 'POST'); loggedIn = true;
      return { status: response?.status === 200 ? 200 : 0 };
    }
    case 'probe': {
      const response = await page.request('/probe');
      if (response?.status !== 200 || !['alpha', 'beta'].includes(response.body?.account) || response.body?.epoch !== 1) return { authenticated: false };
      return { authenticated: true, account: response.body.account, revision: 'fixture-1' };
    }
    case 'collect': {
      if (browser.role !== 'collector' || !seeded) throw new Error('fixture_concurrent_collection_invalid');
      const response = await page.request('/collect', 'POST');
      return { status: response?.status === 200 ? 200 : 0, item: response?.body?.postId === 'fixture-post-1' };
    }
    case 'reader_request': {
      if (browser.role !== 'owner') throw new Error('fixture_concurrent_reader_invalid');
      const response = await page.request('/reader');
      return { authenticated: response?.status === 200 && response.body?.account === 'alpha' && response.body?.epoch === 1 };
    }
    case 'inventory': {
      if (browser.isExited()) return { exited: true, windowCount: 0, bootstrapCount: 0, bootstrap: [] };
      const windows = await inventory();
      return { exited: false, windowCount: windows.length, bootstrapCount: windows.filter(value => value.bootstrap).length, bootstrap: windows.map(value => value.bootstrap), windowKeys: windows.map(value => value.id) };
    }
    case 'open_independent_window': {
      if (browser.role !== 'owner' || pages.size >= 16) throw new Error('fixture_concurrent_window_invalid');
      const next = await browser.page(origin); pages.set(next.targetId, next); page = next;
      const windows = await inventory();
      return { windowCount: windows.length, bootstrapCount: windows.filter(value => value.bootstrap).length };
    }
    case 'close_window': {
      const windows = await inventory(), selected = windows[message.index];
      if (!selected) throw new Error('fixture_concurrent_window_invalid');
      return closeFixtureWindow(browser, selected, { finalWindow: windows.length === 1, inventory, waitRootExit,
        onClosed: ids => { for (const id of ids) pages.delete(id); if (ids.includes(page?.targetId)) page = pages.values().next().value; } });
    }
    case 'release':
      if (browser.role === 'owner' && !browser.isExited() && (await inventory()).length !== 0) throw new Error('fixture_concurrent_windows_remain');
      if (!browser.isExited()) await browser.close();
      return { exited: browser.isExited() };
    default: throw new Error('fixture_concurrent_operation_invalid');
  }
}
setInterval(() => { if (browser?.isExited() && owner && !exitSent) { exitSent = true; emit({ event: 'chrome_exit', owner }); } }, 25).unref();
const input = readline.createInterface({ input: process.stdin, crlfDelay: Infinity });
let chain = Promise.resolve();
input.on('line', line => {
  chain = chain.then(async () => {
    let message;
    try {
      if (line.length > 8192) throw new Error('fixture_concurrent_frame_invalid');
      message = concurrentEngineCommand(JSON.parse(line));
      if (!ready) {
        if (message.op !== 'fixture_bootstrap') throw new Error('fixture_concurrent_bootstrap_invalid');
        ready = true; emit({ ready: true }); return;
      }
      if (message.op === 'fixture_bootstrap') throw new Error('fixture_concurrent_bootstrap_reuse');
      emit({ id: message.id, ok: true, result: await execute(message) });
    } catch (error) {
      const fixed = typeof error?.message === 'string' && /^(fixture|invalid_fixture)_[a-z0-9_]+$/.test(error.message) ? error.message : '';
      const cdp = /^fixture_cdp_([A-Za-z]+)\.([A-Za-z]+)_(failed|timeout)$/.exec(error?.message ?? '');
      const code = fixed || (cdp ? `fixture_concurrent_cdp_${cdp[1].toLowerCase()}_${cdp[2].toLowerCase()}_${cdp[3]}` : 'fixture_concurrent_command_rejected');
      emit({ id: Number.isSafeInteger(message?.id) ? message.id : 0, ok: false, code });
    }
  });
});
