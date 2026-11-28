import assert from "node:assert/strict";
import test from "node:test";
import vm from "node:vm";
import { createPassiveMediaUi } from "../internal/httpapi/web/passive-media-ui.js";
import { webDiagnosticsScripts } from "./fixtures/web-diagnostics.mjs";

function diagnostics(window, sink) {
  return vm.runInNewContext(webDiagnosticsScripts + "\nAkuWebDiagnostics", {
    window, CustomEvent: class extends Event {}, console: { info: sink, warn: sink, debug: sink, error: sink },
  });
}

test("web records retain valid correlation fields while rejecting private error details", () => {
  const rows = [];
  const api = diagnostics({ dispatchEvent() {} }, (...args) => rows.push(args));
  const trace = "12345678-1234-4567-8023-123456789abc";
  const run = "run_" + "a".repeat(32);
  api.issue("dispatch_stopped", { code: "invalid_bridge_token", message: "private" }, run);
  api.issue("show_error", { code: "private", message: "private" }, "private");
  api.feedback(trace, "request_end", { elapsedMs: 0, outcome: "ok", message: "private" });
  api.feedback("private", "click", { direction: "less" });
  assert.equal(rows[0][1].runId, run);
  assert.equal(rows[0][1].reason, "invalid_bridge_token");
  assert.equal(rows[1][1].sequence, 2);
  assert.equal(rows[1][1].reason, "unavailable");
  assert.deepEqual(JSON.parse(JSON.stringify(rows[2])), ["timeline_feedback_trace", {
    trace, outcome: "ok", elapsedMs: 0, operation: "timeline_feedback", phase: "request_end", sequence: 1, level: "info",
  }]);
  assert.equal(rows[3][1].trace, "invalid");
  assert.doesNotMatch(JSON.stringify(rows), /private/);
});

test("deferred passive media logs once, releases the UI and keeps its existing cooldown", async t => {
  const events = [];
  const window = new EventTarget();
  const endpoint = "https://fixture.invalid";
  Object.assign(window, { setTimeout, clearTimeout, postMessage(request) {
    queueMicrotask(() => window.dispatchEvent(Object.assign(new Event("message"), {
      source: window, origin: endpoint, data: { type: "AKU_BROWSER_X_MEDIA_EVIDENCE_FAILED",
        requestId: request.requestId, message: "timed out at private location" },
    })));
  } });
  const originals = new Map(["window", "AkuWebDiagnostics"].map(key => [key, Object.getOwnPropertyDescriptor(globalThis, key)]));
  t.after(() => { for (const [key, descriptor] of originals) {
    if (descriptor) Object.defineProperty(globalThis, key, descriptor);
    else delete globalThis[key];
  } });
  globalThis.window = window;
  globalThis.AkuWebDiagnostics = diagnostics(window, (...args) => events.push(args));
  const state = { passiveMediaEvidenceAttempts: new Map() };
  const ui = createPassiveMediaUi({ state, endpoint, PASSIVE_MEDIA_LOOKUP_TIMEOUT_MS: 1000,
    PASSIVE_MEDIA_LOOKUP_COOLDOWN_MS: 10000, sourceDescriptor: () => ({ passiveMediaCapability: "x_response" }) });
  const item = { id: "private-item", source: "x", evidence: { platformId: "123456", mediaRecovery: { outcome: "unavailable" } } };
  await ui.enrichPassiveXMedia([item]);
  await ui.enrichPassiveXMedia([item]);
  assert.equal(state.passiveMediaEnrichmentActive, false);
  assert.equal(events.length, 1);
  assert.equal(events[0][0], "ui_activity");
  assert.equal(events[0][1].phase, "media_evidence");
  assert.equal(events[0][1].level, "debug");
  assert.equal(events[0][1].reason, "timeout");
  assert.doesNotMatch(JSON.stringify(events), /private|123456/);
});
