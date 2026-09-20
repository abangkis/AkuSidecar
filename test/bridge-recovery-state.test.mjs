import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { bridgeRecoveryState, bridgeReloadVerified, bridgeCaptureBusy } from "../internal/httpapi/web/bridge-recovery-state.js";

const identity = { runtimeRevision: "source-adapters-v110", buildId: "aku-bridge-0.9.1-source-adapters-v110", focusPolicyRevision: "quiet-containment-only-v2" };
const healthy = () => ({ state: "ready", compatible: true, expected: { ...identity }, actual: { ...identity, extensionVersion: "0.9.1" } });
const drifted = () => ({ ...healthy(), state: "degraded", actual: { ...identity, runtimeRevision: "source-adapters-v108", buildId: "aku-bridge-0.9.1-source-adapters-v108" } });
const source = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const render = source.slice(source.indexOf("function renderBridge("), source.indexOf("function configureBackgroundBridge("));
const reload = source.slice(source.indexOf("async function reloadIncompatibleBridge("), source.indexOf("async function refreshReasoningProviderReadiness("));

function fixture(health = healthy(), action = {}) {
  const elements = new Map();
  const element = (id) => {
    if (!elements.has(id)) {
      const classes = new Set();
      elements.set(id, { textContent: "", title: "", classList: { toggle: (key, value) => value ? classes.add(key) : classes.delete(key), remove: (key) => classes.delete(key), contains: (key) => classes.has(key) } });
    }
    return elements.get(id);
  };
  const requests = [];
  const uiTimers = [];
  const state = { bootstrap: { deployment: { mode: "development" }, settings: { activeSources: ["x"] } } };
  const context = { state, bridgeRecoveryState, bridgeReloadVerified, bridgeCaptureBusy, $: element,
    bridgeSourceReadiness: () => [{ source: "x", ready: true }],
    setPill: (id, text, tone) => { element(id).textContent = text; element(id).tone = tone; },
    configureBackgroundBridge() {}, renderBrowserConnection() {}, syncRunButtons() {}, renderSourceSettingsValues() {}, updateOnboardingSummary() {}, schedulePassiveMediaEnrichment() {},
    bridgeApi: async (url, options) => { requests.push({ url, options }); return { action: options ? { id: "test-action" } : action }; },
    api: async () => ({ bridge: health }), setTimeout: (callback) => callback(),
    window: { setTimeout(callback) { uiTimers.push(callback); } }, Date, Math, Error,
  };
  vm.createContext(context);
  vm.runInContext(render + reload, context);
  return { context, state, requests, element, flushUI: () => uiTimers.splice(0).forEach((callback) => callback()), render: (bridge) => context.renderBridge(bridge), reload: () => context.reloadIncompatibleBridge() };
}

test("compatible degraded identity drift is visible with contextual development reload", () => {
  const app = fixture(); app.render(drifted());
  assert.equal(app.element("#bridge-reload").classList.contains("hidden"), false);
  assert.match(app.element("#bridge-status").textContent, /revision mismatch.*v108.*v110/);
  assert.equal(app.element("#bridge-status").tone, "warning");
  assert.match(app.element("#bridge-status").title, /Loaded build:.*v108; expected build:.*v110/);
  assert.equal(app.requests.length, 0, "Rendering must never reload automatically");
});

test("focus mismatch permits recovery, exact parity hides it, missing identity stays unknown", () => {
  const app = fixture();
  app.render({ ...healthy(), state: "incompatible", compatible: false, reasons: ["bridge focus policy revision mismatch"], actual: { ...identity, focusPolicyRevision: "old" } });
  assert.equal(app.element("#bridge-reload").classList.contains("hidden"), false);
  app.render(healthy());
  assert.equal(app.element("#bridge-reload").classList.contains("hidden"), true);
  assert.match(app.element("#bridge-status").textContent, /ready/);
  app.render({ ...healthy(), actual: null });
  assert.equal(app.element("#bridge-reload").classList.contains("hidden"), true);
  assert.match(app.element("#bridge-status").textContent, /identity unverified/);
  assert.equal(bridgeRecoveryState(drifted(), false).showReload, false);
});

const completed = () => ({ status: "completed", heartbeatObservedAt: "2026-09-20T00:00:00Z", expectedBuildId: identity.buildId, observedBuildId: identity.buildId });
test("reload succeeds only after expected heartbeat and exact compatible identity readback", async () => {
  const app = fixture(healthy(), completed()); app.render(drifted()); await app.reload();
  assert.equal(app.element("#bridge-reload").textContent, "AkuBridge reloaded");
  assert.equal(app.element("#bridge-reload").classList.contains("hidden"), true);
  assert.equal(app.requests.filter((request) => request.options?.method === "POST").length, 1);
});

test("failed heartbeat or parity readback keeps recovery visible", async () => {
  for (const [health, action] of [[drifted(), completed()], [healthy(), { ...completed(), heartbeatObservedAt: null }], [healthy(), { status: "failed", message: "heartbeat timed out" }]]) {
    const app = fixture(health, action); app.render(drifted()); await app.reload();
    assert.equal(app.element("#bridge-reload").textContent, "Reload failed");
    assert.equal(app.element("#bridge-reload").classList.contains("hidden"), false);
    assert.match(app.element("#bridge-reload").title, /heartbeat|expected compatible/);
  }
});

test("active claimed capture blocks reload without blocking recovery of queued work", async () => {
  const app = fixture(); app.state.session = { runs: [{ status: "waiting_for_bridge", bridgeCommandStatus: "claimed" }] };
  app.render(drifted()); await app.reload();
  assert.equal(app.element("#bridge-reload").disabled, true);
  assert.equal(app.requests.length, 0);
  assert.match(app.element("#bridge-reload").title, /active capture/);
  app.state.session.runs[0].bridgeCommandStatus = "queued";
  app.flushUI();
  app.render(drifted());
  assert.equal(app.element("#bridge-reload").disabled, false);
});
