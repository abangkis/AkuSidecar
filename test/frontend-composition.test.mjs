import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createSessionUi } from "../internal/httpapi/web/session-ui.js";
import { createProviderSetupUi } from "../internal/httpapi/web/provider-setup-ui.js";

function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

test("real frontend factories import and compose without invoking late host ports", async t => {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const imports = [...app.matchAll(/import \{ (create\w+) \} from "\.\/([\w-]+\.js)";/g)]
    .filter(([, , file]) => app.includes(`// Frontend feature: ${file}#`));
  assert.equal(imports.length, 20);
  for (const [, factory, file] of imports) {
    await t.test(file, async () => {
      const feature = await import(new URL(`../internal/httpapi/web/${file}`, import.meta.url));
      const state = {};
      const ports = new Proxy({}, {
        get: (_, key) => key === "state" ? state : () => assert.fail(`Eager host call: ${file}.${String(key)}`),
      });
      const instance = feature[factory](ports);
      assert.ok(Object.keys(instance).length > 0);
      assert.ok(Object.values(instance).every(value => typeof value === "function"));
      assert.deepEqual(state, {});
    });
  }
});

test("real session factory rejects a stale response using its shared state port", async () => {
  const pending = deferred();
  const state = { session: { id: "old" }, pollInFlight: false };
  const session = createSessionUi({
    state,
    api: () => pending.promise,
    showError: error => assert.fail(error.message),
  });
  const poll = session.pollSession();
  assert.equal(state.pollInFlight, true);
  state.session = { id: "replacement" };
  pending.resolve({ session: { id: "old", status: "completed" } });
  await poll;
  assert.equal(state.session.id, "replacement");
  assert.equal(state.pollInFlight, false);
});

test("real provider factory resolves the latest dirty tracker after its request", async () => {
  const pending = deferred();
  const elements = new Map();
  const $ = id => {
    if (!elements.has(id)) elements.set(id, { close() {} });
    return elements.get(id);
  };
  const state = { bootstrap: {
    settings: { reasoningProvider: "old", timelineCapacity: 36 },
    reasoningProviders: [{ name: "new", label: "New", configured: true }],
  } };
  let tracker;
  let baseline;
  let refreshes = 0;
  const provider = createProviderSetupUi({
    $, state,
    api: async (path, options) => {
      assert.equal(path, "/api/settings");
      assert.equal(options.method, "PUT");
      assert.equal(options.body.settings.timelineCapacity, 36);
      return pending.promise;
    },
    getSettingsDirty: () => {
      assert.ok(tracker, "dirty tracker is read only after initialization");
      return tracker;
    },
    renderActiveReasoningProvider() {},
    renderReasoningProcesses() {},
    renderReasoningProviderControl() {},
  });
  const activation = provider.activateSettingsReasoningProvider("new");
  tracker = {
    getBaselineKey: () => JSON.stringify({ reasoningProvider: "old", timelineCapacity: 24 }),
    setBaseline: value => { baseline = value; },
    refresh: () => { refreshes++; },
  };
  pending.resolve({ settings: { reasoningProvider: "new", timelineCapacity: 36 } });
  await activation;
  assert.deepEqual(baseline, { reasoningProvider: "new", timelineCapacity: 24 });
  assert.equal(refreshes, 1);
  assert.match($("#runtime-settings-status").textContent, /Provider changed/);
});
