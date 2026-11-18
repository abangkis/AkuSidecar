import test from "node:test";
import assert from "node:assert/strict";
import { createCaptureJournalUI, requestCaptureJournal } from "../internal/httpapi/web/capture-journal-ui.js";
import { createAutoUpdateUi } from "../internal/httpapi/web/auto-update-ui.js";

const full = { capacityBlocked: true, recordsUsed: 4095, capacity: 4096, protectedRecords: 3 };
const preview = { ...full, eligibleRecords: 120, previewToken: "a".repeat(64), backupPolicy: "latest_verified" };
const ready = { ...full, recordsUsed: 3975, capacityBlocked: false, archivedRecords: 120, outcome: "archived",
  backupRecords: 4095, preserved: { active: 1, pending: 1, retained: 1 } };
const tick = () => new Promise(resolve => setImmediate(resolve));

test("journal banner restores from the capture owner and clears after confirmed archival while quota stays visible", async () => {
  const $ = elements();
  const calls = [];
  const ui = createCaptureJournalUI({ $, getRuntime: () => ({ available: true, effective: "browser", state: "ready" }),
    request: async (...args) => { calls.push(args); return { status: full, preview, archive: ready }[args[0]]; } });
  const autoUpdate = createAutoUpdateUi({ $, state: { bootstrap: {} }, renderNativeReaderNotice() {}, renderTimelineActions() {},
    humanize: String, formatTokenCount: String, formatDate: String });
  autoUpdate.renderAutoUpdateStatus({ enabled: true, state: "budget_paused", dailyTokensRemaining: 10, estimatedNextRunTokens: 50 });
  assert.equal($("#auto-update-timeline-settings").textContent, "Review Auto Update");
  assert.match($("#auto-update-timeline-title").textContent, /today’s budget/);
  await ui.refresh();
  assert.equal($("#capture-journal-banner").classList.contains("hidden"), false);

  $("#capture-journal-review").click();
  await tick();
  assert.equal($("#capture-journal-dialog").open, true);
  assert.equal($("#capture-journal-used").textContent, "4,095 / 4,096");
  assert.equal($("#capture-journal-submit").textContent, "Archive 120 closed records");
  assert.deepEqual(calls.map(call => call[0]), ["status", "preview"]);
  $("#capture-journal-submit").click();
  await tick();
  assert.deepEqual(calls.at(-1), ["archive", preview.previewToken]);
  assert.equal($("#capture-journal-title").textContent, "Capture journal ready");
  assert.equal($("#capture-journal-summary").textContent, "Backup and archive verified");
  assert.match($("#capture-journal-feedback").textContent, /4,095 records. Preserved: 1 active, 1 pending, 1 retained/);
  assert.equal($("#capture-journal-banner").classList.contains("hidden"), true);
  assert.equal($("#auto-update-timeline-status").classList.contains("hidden"), false);
  $("#capture-journal-cancel").click();
  assert.equal($("#capture-journal-dialog").open, false);
});

test("a changed preview requires confirmation again and a fully protected journal disables archival", async () => {
  const $ = elements();
  const calls = [];
  const updated = { ...preview, outcome: "review_required", eligibleRecords: 0, previewToken: "b".repeat(64) };
  const ui = createCaptureJournalUI({ $, getRuntime: () => null, request: async (operation) => {
    calls.push(operation);
    return { status: full, preview, archive: updated }[operation];
  } });
  await ui.refresh();
  $("#capture-journal-review").click();
  await tick();
  $("#capture-journal-submit").click();
  await tick();
  assert.equal($("#capture-journal-banner").classList.contains("hidden"), false);
  assert.match($("#capture-journal-feedback").textContent, /preview changed/);
  assert.equal($("#capture-journal-submit").disabled, true);
  $("#capture-journal-submit").click();
  assert.deepEqual(calls, ["status", "preview", "archive"]);
});

test("journal relay correlates replies to the requesting page, origin and request", async () => {
  const origin = "http://127.0.0.1:11122";
  let receive;
  const window = {
    addEventListener: (_type, listener) => { receive = listener; },
    removeEventListener: () => { receive = null; },
    postMessage(message) {
      const data = { type: "AKU_BROWSER_CAPTURE_JOURNAL_RESULT", journal: full, requestId: message.requestId };
      receive({ source: this, origin: "https://foreign.example", data });
      receive({ source: this, origin, data: { ...data, requestId: "another" } });
      assert.notEqual(receive, null);
      receive({ source: this, origin, data });
    },
  };
  assert.deepEqual(await requestCaptureJournal(window, origin, "status"), full);
  assert.equal(receive, null);
});

function elements() {
  const nodes = new Map();
  return selector => {
    if (!nodes.has(selector)) {
      const classes = new Set(["hidden"]);
      const listeners = new Map();
      nodes.set(selector, {
        textContent: "", open: false, hidden: false, disabled: false,
        classList: { toggle(name, enable) { if (enable) classes.add(name); else classes.delete(name); }, contains: name => classes.has(name) },
        addEventListener(type, listener) { listeners.set(type, listener); },
        setAttribute() {},
        showModal() { this.open = true; }, close() { this.open = false; },
        click() { if (!this.disabled) listeners.get("click")(); },
      });
    }
    return nodes.get(selector);
  };
}
