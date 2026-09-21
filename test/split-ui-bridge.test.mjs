import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import vm from "node:vm";

const script = fs.readFileSync(new URL("../internal/httpapi/split_ui_bridge.js", import.meta.url), "utf8");
const origin = "http://127.0.0.1:11122";
function fixture(reply = { ok: true, result: {} }) {
  const calls = [], messages = [];
  let listener;
  const window = { addEventListener: (_event, fn) => { listener = fn; }, postMessage: (v, target) => messages.push({ ...v, target }) };
  vm.runInNewContext(script, { window, location: { origin }, fetch: async (url, options) => {
    calls.push({ url, options });
    return { ok: true, json: async () => url === "/api/bootstrap"
      ? { bridgeToken: "trusted-token", bridgeContractVersion: "aku-browser.bridge.v2", instanceEpoch: "current-epoch" } : reply };
  } });
  return { calls, messages, send: (data, eventOrigin = origin) => listener({ source: window, origin: eventOrigin, data }) };
}

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
