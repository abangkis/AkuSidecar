import test from 'node:test';
import assert from 'node:assert/strict';
import { renderSourceCapturePause, confirmSourceCaptureRecovery } from '../internal/httpapi/web/source-capture-pause.js';

test('both challenges are visible even when Auto Update is disabled and unknown categories are not rendered', () => {
  let hidden = true;
  const nodes = { panel: { classList: { toggle(name, value) { assert.equal(name, 'hidden'); hidden = value; } } }, detail: { textContent: '' } };
  for (const [category, prefix] of [['cloudflare', 'Cloudflare'], ['x_human_verification', 'X requests']]) {
    renderSourceCapturePause(nodes, { category, message: 'Manual verification required' });
    assert.equal(hidden, false);
    assert.ok(nodes.detail.textContent.startsWith(prefix));
  }
  renderSourceCapturePause(nodes, { category: '<unsafe>', message: 'Fixed message' });
  assert.equal(nodes.detail.textContent, 'Fixed message');
  renderSourceCapturePause(nodes, null);
  assert.equal(hidden, true);
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
