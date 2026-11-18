import test from 'node:test';
import assert from 'node:assert/strict';
import { renderSourceCapturePause, confirmSourceCaptureRecovery } from '../internal/httpapi/web/source-capture-pause.js';
import { createAutoUpdateUi } from '../internal/httpapi/web/auto-update-ui.js';

test('both challenges show their existing copy and unrecognized categories do not add a prefix', () => {
  let hidden = true;
  const nodes = { panel: { classList: { toggle(name, value) { assert.equal(name, 'hidden'); hidden = value; } } }, detail: { textContent: '' } };
  for (const [category, prefix] of [['cloudflare', 'Cloudflare'], ['x_human_verification', 'X requests']]) {
    renderSourceCapturePause(nodes, { category, message: 'Manual verification required' });
    assert.equal(hidden, false);
    assert.equal(nodes.panel.hidden, false);
    assert.ok(nodes.detail.textContent.startsWith(prefix));
  }
  renderSourceCapturePause(nodes, { category: '<unsafe>', message: 'Fixed message' });
  assert.equal(nodes.detail.textContent, 'Fixed message');
  renderSourceCapturePause(nodes, null);
  assert.equal(hidden, true);
  assert.equal(nodes.panel.hidden, true);
  assert.equal(nodes.detail.textContent, '');
});

test('recovery makes no request without explicit confirmation and never changes Auto Update', async () => {
  const requests = [];
  const api = async (...args) => { requests.push(args); return { autoUpdate: { enabled: false } }; };
  assert.equal(await confirmSourceCaptureRecovery(api, { id: 'pause-1' }, false), null);
  assert.equal(await confirmSourceCaptureRecovery(api, null, true), null);
  assert.equal(requests.length, 0);
  const reply = await confirmSourceCaptureRecovery(api, { id: 'pause-1' }, true);
  assert.deepEqual(requests, [['/api/capture/x/confirm-recovery', { method: 'POST', body: { pauseId: 'pause-1', confirmed: true } }]]);
  assert.equal(reply.autoUpdate.enabled, false);
});

test('restored X pause renders with Auto Update off and remains locked across polling until confirmation completes', async t => {
  const originalWindow = globalThis.window;
  t.after(() => {
    if (originalWindow === undefined) delete globalThis.window;
    else globalThis.window = originalWindow;
  });
  let confirmed = false;
  globalThis.window = { confirm: () => confirmed };

  const paused = { enabled: false, state: 'disabled', sourceCapturePause: {
    id: 'persisted-pause', category: 'x_human_verification', message: 'Manual verification required',
  } };
  const calls = [];
  let finish;
  const pending = new Promise(resolve => { finish = resolve; });
  const nodes = {
    '#x-capture-pause': { hidden: true, classList: { toggle() {} } },
    '#x-capture-pause-detail': { textContent: '' },
    '#confirm-x-capture-recovery': { disabled: false },
  };
  const state = { bootstrap: { autoUpdate: paused } };
  const ui = createAutoUpdateUi({
    $: selector => nodes[selector] ?? null,
    state, renderNativeReaderNotice() {}, showError(error) { throw error; },
    api: (...args) => { calls.push(args); return pending; },
  });
  ui.renderAutoUpdateStatus(state.bootstrap.autoUpdate);
  assert.equal(nodes['#x-capture-pause'].hidden, false);
  assert.match(nodes['#x-capture-pause-detail'].textContent, /phone app/);
  await ui.confirmXCaptureRecovery();
  assert.equal(calls.length, 0);

  confirmed = true;
  const recovery = ui.confirmXCaptureRecovery();
  try {
    ui.renderAutoUpdateStatus(paused);
    assert.equal(nodes['#confirm-x-capture-recovery'].disabled, true);
    assert.deepEqual(calls, [['/api/capture/x/confirm-recovery', {
      method: 'POST', body: { pauseId: 'persisted-pause', confirmed: true },
    }]]);
  } finally {
    finish({ autoUpdate: { enabled: false, state: 'disabled' } });
    await recovery;
  }
  assert.equal(nodes['#x-capture-pause'].hidden, true);
  assert.equal(nodes['#x-capture-pause-detail'].textContent, '');
  assert.equal(state.bootstrap.autoUpdate.enabled, false);
});
