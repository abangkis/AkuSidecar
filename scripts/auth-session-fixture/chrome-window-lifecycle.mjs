import { setTimeout as delay } from 'node:timers/promises';

// The private fixture browser and loopback origin are supplied by its owned
// driver. Preserve every page target in each window; a tab is not a window.
export async function inventoryFixtureWindows(browser, origin, bootstrapTarget) {
  if (!browser || browser.isExited()) throw new Error('fixture_chrome_closed');
  for (let attempt = 0; attempt < 4; attempt++) {
    const result = await browser.send('Target.getTargets');
    if (!Array.isArray(result?.targetInfos)) throw new Error('fixture_inventory_unknown');
    const windows = new Map();
    let vanished = false;
    for (const target of result.targetInfos) {
      if (!target || target.type !== 'page') continue;
      if (typeof target.targetId !== 'string' || !target.targetId || typeof target.url !== 'string') throw new Error('fixture_inventory_unknown');
      if (target.url !== 'about:blank' && target.url !== origin && !target.url.startsWith(origin + '/')) throw new Error('fixture_inventory_unknown');
      let located;
      try { located = await browser.send('Browser.getWindowForTarget', { targetId: target.targetId }); }
      catch {
        const fresh = await browser.send('Target.getTargets');
        if (!Array.isArray(fresh?.targetInfos)) throw new Error('fixture_inventory_unknown');
        if (!fresh.targetInfos.some(item => item?.targetId === target.targetId)) { vanished = true; break; }
        throw new Error('fixture_inventory_unknown');
      }
      if (!Number.isInteger(located?.windowId)) throw new Error('fixture_inventory_unknown');
      let value = windows.get(located.windowId);
      if (!value) { value = { id: located.windowId, bootstrap: false, targetIds: [] }; windows.set(located.windowId, value); }
      value.targetIds.push(target.targetId);
      value.bootstrap ||= target.targetId === bootstrapTarget;
    }
    if (!vanished) return [...windows.values()].sort((a, b) => a.id - b.id);
  }
  throw new Error('fixture_inventory_unknown');
}

export async function closeFixtureWindow(browser, selected, { finalWindow, inventory, waitRootExit, onClosed = () => {}, pause = delay }) {
  if (!Number.isInteger(selected?.id) || !Array.isArray(selected.targetIds) || selected.targetIds.length === 0 ||
      selected.targetIds.some(id => typeof id !== 'string' || !id) || new Set(selected.targetIds).size !== selected.targetIds.length) throw new Error('fixture_window_index_invalid');
  const closed = () => { onClosed(selected.targetIds); return { requested: true, exited: finalWindow }; };
  try {
    for (const targetId of selected.targetIds) {
      const targets = await browser.send('Target.getTargets');
      if (!Array.isArray(targets?.targetInfos)) throw new Error('fixture_inventory_unknown');
      // Closing one tab can destroy its related targets. Do not close a target
      // that vanished or moved into a different ownership window.
      if (!targets.targetInfos.some(target => target?.targetId === targetId)) continue;
      const location = await browser.send('Browser.getWindowForTarget', { targetId });
      if (location?.windowId !== selected.id) throw new Error('fixture_window_scope_changed');
      const result = await browser.send('Target.closeTarget', { targetId });
      if (result?.success !== true) throw new Error('fixture_window_close_failed');
    }
    for (let attempt = 0; attempt < 80; attempt++) {
      if (browser.isExited()) {
        if (finalWindow && await waitRootExit(browser)) return closed();
        throw new Error('fixture_window_close_unverified');
      }
      const remaining = await inventory();
      if (!remaining.some(window => window.id === selected.id)) {
        if (!finalWindow || await waitRootExit(browser)) return closed();
        throw new Error('fixture_window_close_unverified');
      }
      await pause(25);
    }
  } catch (error) {
    if (finalWindow && error?.message === 'fixture_chrome_closed' && await waitRootExit(browser)) return closed();
    throw error;
  }
  throw new Error('fixture_window_close_unverified');
}
