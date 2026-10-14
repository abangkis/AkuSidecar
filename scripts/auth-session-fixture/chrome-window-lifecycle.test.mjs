import test from 'node:test';
import assert from 'node:assert/strict';
import { inventoryFixtureWindows, closeFixtureWindow } from './chrome-window-lifecycle.mjs';

const origin = 'http://127.0.0.1:12345';
function browserFixture(entries, { autoExit = true, onClose = () => {} } = {}) {
  const targets = new Map(entries.map(([targetId, windowId]) => [targetId, { targetId, windowId, type: 'page', url: origin + '/' }]));
  const calls = [];
  let exited = false;
  return { targets, calls, isExited: () => exited, async send(method, args = {}) {
    calls.push({ method, ...args });
    if (exited) throw new Error('fixture_chrome_closed');
    if (method === 'Target.getTargets') return { targetInfos: [...targets.values()] };
    if (method === 'Browser.getWindowForTarget') {
      const target = targets.get(args.targetId);
      if (!target) throw new Error('target gone');
      return { windowId: target.windowId };
    }
    if (method === 'Target.closeTarget') {
      const success = targets.delete(args.targetId);
      onClose(args.targetId, targets);
      exited = autoExit && targets.size === 0;
      return { success };
    }
    throw new Error('unexpected method');
  } };
}
const inventory = browser => () => inventoryFixtureWindows(browser, origin, 'bootstrap');
const options = (browser, finalWindow) => ({ finalWindow, inventory: inventory(browser), waitRootExit: async () => browser.isExited(), pause: async () => {} });

test('closing an arbitrary tab leaves bootstrap window present: reproduce old false success', async () => {
  const browser = browserFixture([['other-tab', 1], ['bootstrap', 1], ['independent', 2]]);
  const before = await inventory(browser)();
  assert.equal(before.length, 2);
  assert.deepEqual(before[0], { id: 1, bootstrap: true, targetIds: ['other-tab', 'bootstrap'] });
  await browser.send('Target.closeTarget', { targetId: 'other-tab' });
  const after = await inventory(browser)();
  assert.equal(after.length, 2);
  assert.equal(after[0].bootstrap, true);
});
test('close every selected-window tab, preserve independent window, then prove final natural exit', async () => {
  const browser = browserFixture([['other-tab', 1], ['bootstrap', 1], ['independent', 2]]);
  const selected = (await inventory(browser)())[0];
  let removed = [];
  assert.deepEqual(await closeFixtureWindow(browser, selected, { ...options(browser, false), onClosed: ids => { removed = ids; } }), { requested: true, exited: false });
  assert.deepEqual(removed, ['other-tab', 'bootstrap']);
  const remaining = await inventory(browser)();
  assert.equal(remaining.length, 1);
  assert.equal(remaining[0].bootstrap, false);
  assert.equal(browser.targets.has('independent'), true);
  assert.deepEqual(await closeFixtureWindow(browser, remaining[0], options(browser, true)), { requested: true, exited: true });
  assert.equal(browser.isExited(), true);
  assert.deepEqual(browser.calls.filter(c => c.method === 'Target.closeTarget').map(c => c.targetId), ['other-tab', 'bootstrap', 'independent']);
  assert.equal(browser.calls.some(c => c.method === 'Browser.close'), false);
});
test('replacement target in the same window does not count as window closure', async () => {
  const browser = browserFixture([['bootstrap', 1], ['independent', 2]], { onClose(id, targets) {
    if (id === 'bootstrap') targets.set('replacement', { targetId: 'replacement', windowId: 1, type: 'page', url: origin + '/' });
  } });
  const selected = (await inventory(browser)())[0];
  let accepted = false;
  await assert.rejects(closeFixtureWindow(browser, selected, { ...options(browser, false), onClosed: () => { accepted = true; } }), /fixture_window_close_unverified/);
  assert.equal(accepted, false);
  assert.equal(browser.targets.has('replacement'), true);
  assert.equal(browser.targets.has('independent'), true);
});
test('a target moved to another window is never closed', async () => {
  const browser = browserFixture([['bootstrap', 1], ['independent', 2]]);
  const selected = (await inventory(browser)())[0];
  browser.targets.get('bootstrap').windowId = 2;
  await assert.rejects(closeFixtureWindow(browser, selected, options(browser, false)), /fixture_window_scope_changed/);
  assert.equal(browser.calls.some(c => c.method === 'Target.closeTarget'), false);
});
test('final target disappearing without root exit does not count as release', async () => {
  const browser = browserFixture([['bootstrap', 1]], { autoExit: false });
  const selected = (await inventory(browser)())[0];
  await assert.rejects(closeFixtureWindow(browser, selected, options(browser, true)), /fixture_window_close_unverified/);
});
test('foreign URL or unreadable inventory is unknown, never zero', async () => {
  const browser = browserFixture([['bootstrap', 1]]);
  browser.targets.get('bootstrap').url = 'https://example.com/';
  await assert.rejects(inventory(browser)(), /fixture_inventory_unknown/);
  await assert.rejects(inventoryFixtureWindows({ isExited: () => false, send: async () => ({}) }, origin, 'bootstrap'), /fixture_inventory_unknown/);
});
