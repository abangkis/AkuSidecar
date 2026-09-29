import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const script = fs.readFileSync(new URL("../internal/httpapi/split_ui_bridge.js", import.meta.url), "utf8");
const diagnosticsScript = fs.readFileSync(new URL("../internal/httpapi/web/native-post-diagnostics.js", import.meta.url), "utf8");
const origin = "http://127.0.0.1:11122";
function fixture(reply = { ok: true, result: {} }) {
  const calls = [], messages = [], timers = [], traces = [];
  const stored = new Map();
  let listener;
  let now = Date.now();
  const window = { dispatchEvent() {}, addEventListener: (_event, fn) => { listener = fn; }, postMessage: (v, target) => messages.push({ ...v, target }) };
  const context = { Date: class extends Date { static now() { return now; } }, CustomEvent: class { constructor(type, options) { this.type = type; this.detail = options.detail; } }, window, sessionStorage: { getItem: (key) => stored.get(key) ?? null, setItem: (key, value) => stored.set(key, value) }, location: { origin }, performance: { now: () => Date.now() }, console: { info: (label, detail) => traces.push({ label, detail }) }, setTimeout: (fn) => timers.push(fn), fetch: async (url, options) => {
    calls.push({ url, options });
    return { ok: true, status: 200, json: async () => url === "/api/bootstrap"
      ? { bridgeToken: "trusted-token", bridgeContractVersion: "aku-browser.bridge.v2", instanceEpoch: "current-epoch" } : reply };
  } };
  vm.runInNewContext(diagnosticsScript, context);
  vm.runInNewContext(script, context);
  assert.equal(messages.shift().type, "AKU_BROWSER_READER_BROKER_PROBE");
  return { calls, messages, timers, traces, window, advance: (ms) => { now += ms; }, read: () => window.akuNativePostDiagnostics.read(), send: (data, eventOrigin = origin) => listener({ source: window, origin: eventOrigin, data }) };
}

test("native post fails visibly without broker readiness or trusted-click correlation", async () => {
  const f = fixture();
  const action = { type: "AKU_BROWSER_OPEN_NATIVE_POST", requestId: "broker_" + "a".repeat(32), source: "x", url: "https://x.com/a/status/1" };
  await f.send(action);
  assert.equal(f.calls.length, 0);
  assert.equal(f.messages.at(-1).type, "AKU_BROWSER_NATIVE_POST_OPEN_FAILED");
  assert.match(f.messages.at(-1).message, /UI reader broker is not ready/);
  await f.send({ type: "AKU_BROWSER_READER_BROKER_READY" }, "https://foreign.example");
  await f.send(action);
  assert.equal(f.calls.length, 0);
  await f.send({ type: "AKU_BROWSER_READER_BROKER_READY" });
  await f.send({ ...action, requestId: "native_post_programmatic" });
  assert.equal(f.calls.length, 0);
  await f.send(action);
  assert.equal(f.calls.length, 2);
  assert.equal(JSON.parse(f.calls[1].options.body).requestId, action.requestId);
  const nativeTraces = f.traces.filter((entry) => entry.label === "native_post_trace");
  assert.deepEqual(nativeTraces.slice(-4).map((entry) => entry.detail.phase), ["relay_received", "relay_bootstrap_done", "relay_request_start", "relay_request_end"]);
  assert.ok(nativeTraces.every((entry) => entry.detail.trace === action.requestId || entry.detail.trace === "invalid"));
  assert.ok(nativeTraces.every((entry) => !Object.hasOwn(entry.detail, "url")));
  assert.equal(f.read().at(-1).phase, "relay_request_end");
  const count = f.messages.length;
  f.timers[0]();
  assert.equal(f.messages.length, count, "readiness ends polling");
});

test("native relay keeps an early rejection category after the request fails", async () => {
  const f = fixture();
  const action = { type: "AKU_BROWSER_OPEN_NATIVE_POST", requestId: "broker_" + "c".repeat(32), source: "linkedin", url: "https://www.linkedin.com/posts/private" };
  await f.send(action);
  assert.equal(f.read().at(-1).phase, "relay_error");
  assert.equal(f.read().at(-1).errorKind, "broker_not_ready");
  assert.equal(JSON.stringify(f.read()).includes("linkedin.com"), false);
});

test("missing browser broker keeps the page fallback trace without authorizing a native action", async () => {
  const f = fixture();
  const requestId = "native_post_123_a1";
  await f.send({ type: "AKU_BROWSER_READER_BROKER_READY" });
  await f.send({ type: "AKU_BROWSER_OPEN_NATIVE_POST", requestId, source: "linkedin", url: "https://www.linkedin.com/posts/private" });
  assert.equal(f.calls.length, 0);
  assert.equal(f.read().at(-1).trace, requestId);
  assert.equal(f.read().at(-1).errorKind, "broker_click_missing");
});

test("split UI keeps request correlation and sends only typed actions using current bootstrap authority", async () => {
  const f = fixture({ ok: true, result: { source: "x", state: "permission_required", url: "chrome-extension://capture/source-permission.html" } });
  await f.send({ type: "AKU_BROWSER_OPEN_SOURCE", source: "x", requestId: "request-1", token: "untrusted", endpoint: "https://evil.example", script: "bad" });
  assert.equal(f.calls[1].url, "/api/split-capture/actions");
  assert.deepEqual(JSON.parse(f.calls[1].options.body), { type: "open_source", source: "x" });
  assert.equal(f.calls[1].options.headers["X-Aku-Bridge-Token"], "trusted-token");
  assert.equal(f.calls[1].options.headers["X-Aku-Split-Epoch"], "current-epoch");
  assert.equal(f.messages[0].type, "AKU_BROWSER_SOURCE_PERMISSION_REQUIRED");
  assert.equal(f.messages[0].requestId, "request-1");
});

test("split UI rejects foreign messages and unsupported operations without transport", async () => {
  const f = fixture();
  await f.send({ type: "AKU_BROWSER_DISPATCH", runId: "x" }, "https://foreign.example");
  await f.send({ type: "eval", script: "bad" });
  assert.equal(f.calls.length, 0);
});

test("split UI reports explicit correlated release failure", async () => {
  const f = fixture({ ok: false, message: "Capture unavailable" });
  await f.send({ type: "AKU_BROWSER_RELEASE_CAPTURE_SURFACE", leaseId: "lease-1", source: "x" });
  assert.equal(f.messages[0].type, "AKU_BROWSER_CAPTURE_SURFACE_RELEASE_FAILED");
  assert.equal(f.messages[0].leaseId, "lease-1");
  assert.equal(f.messages[0].message, "Capture unavailable");
});


test("broker handshake deadline exposes recovery and late readiness restores availability", async () => {
  const f = fixture();
  assert.equal(f.window.akuReaderBrokerStatus, "pending");
  f.advance(5001);
  f.timers.shift()();
  assert.equal(f.window.akuReaderBrokerStatus, "unavailable");
  assert.equal(f.read().at(-1).phase, "broker_unavailable");
  assert.equal(f.timers.length, 0);
  await f.send({ type: "AKU_BROWSER_READER_BROKER_READY" });
  assert.equal(f.window.akuReaderBrokerStatus, "ready");
  assert.equal(f.read().at(-1).phase, "broker_ready");
  assert.equal(f.calls.length, 0, "availability never authorizes a native action");
});
