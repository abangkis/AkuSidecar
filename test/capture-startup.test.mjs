import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import vm from "node:vm";

const script = readFileSync(new URL("../internal/httpapi/capture_startup.js", import.meta.url), "utf8");
test("initial host can report without extension code and observe a later receipt", () => {
  const requests = [], events = new Map(), dataset = {};
  let later;
  vm.runInNewContext(script, {
    location: { origin: "http://localhost:11122", pathname: "/split-capture-host", hash: "#" + "b".repeat(64) },
    document: { currentScript: { dataset: { document: "2" } }, getElementById: () => ({ dataset }) },
    performance: { getEntriesByType: () => [{ type: "navigate" }] },
    window: { addEventListener: (name, fn) => events.set(name, fn) },
    setTimeout: fn => { later = fn; return 1; }, clearTimeout() {},
    fetch: async (url, options) => { requests.push([url, JSON.parse(options.body)]); },
  });
  events.get("load")();
  dataset.akuScriptStarted = "1";
  dataset.akuWorkerReceived = "1";
  later();
  assert.deepEqual(requests, [
    ["/api/bridge/split-capture/startup-sample", { doc: 2, phase: "early", reload: false, scriptStarted: false, workerReceived: false }],
    ["/api/bridge/split-capture/startup-sample", { doc: 2, phase: "late", reload: false, scriptStarted: true, workerReceived: true }],
  ]);
});
