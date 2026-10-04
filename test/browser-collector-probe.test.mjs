import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";
import { browserCollectorProbeAllowed } from "../internal/httpapi/web/collection-mode.js";

test("only a ready Browser collector accepts passive probes", () => {
  assert.equal(browserCollectorProbeAllowed(undefined), true, "legacy Browser remains supported");
  assert.equal(browserCollectorProbeAllowed({ available: true, effective: "browser", state: "ready", pending: true }), true, "interactive source windows still need probes");
  for (const runtime of [
    { available: true, effective: "browser", state: "ready", nativeReaderOnly: true },
    { available: true, effective: "headless", state: "ready" },
    { available: true, effective: "", state: "replacing" },
    { available: true, effective: "", state: "failed" },
    { available: true, effective: "", state: "blocked" },
  ]) assert.equal(browserCollectorProbeAllowed(runtime), false, JSON.stringify(runtime));
});

test("actual UI ping and source probe do not queue requests or timeout notices for a reader", () => {
  const app = fs.readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const ping = app.slice(app.indexOf("function pingBridge() {"), app.indexOf("let lastBridgeReturnRefreshAt"));
  const start = app.indexOf("function requestSourceSessionReadiness() {");
  const probe = app.slice(start, app.indexOf("function openSourceFromSettings", start));
  for (const runtime of [
    { available: true, effective: "browser", state: "ready", nativeReaderOnly: true },
    { available: true, effective: "", state: "replacing" },
    { available: true, effective: "headless", state: "ready" },
    { available: true, effective: "browser", state: "ready" },
  ]) {
    const messages = [], timers = [];
    const state = { bootstrap: { collectionRuntime: runtime, bridge: { compatible: true } }, sourceSessionProbeInFlight: false };
    const context = vm.createContext({ state, browserCollectorProbeAllowed, endpoint: "http://localhost", window: { postMessage: value => messages.push(value), setTimeout: fn => timers.push(fn) }, renderSourceSessionReadiness() {} });
    vm.runInContext(ping + probe + "pingBridge(); requestSourceSessionReadiness();", context);
    assert.equal(messages.length, browserCollectorProbeAllowed(runtime) ? 2 : 0);
    assert.equal(timers.length, browserCollectorProbeAllowed(runtime) ? 1 : 0);
  }
});
