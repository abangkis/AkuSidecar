import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createSessionUi } from "../internal/httpapi/web/session-ui.js";
import { createProviderSetupUi } from "../internal/httpapi/web/provider-setup-ui.js";
import { createInboxUi } from "../internal/httpapi/web/inbox-ui.js";

function deferred() {
  let resolve;
  const promise = new Promise(done => { resolve = done; });
  return { promise, resolve };
}

test("inbox badges distinguish recorded providers, failed attempts and unavailable provenance", () => {
  const originalDocument = globalThis.document;
  globalThis.document = { createElement: () => ({ attributes: {}, setAttribute(key, value) { this.attributes[key] = value; } }) };
  try {
    const inbox = createInboxUi({
      state: new Proxy({}, { get() { assert.fail("historical provider must not read current Settings"); } }),
      formatReasoningProvider: value => value === "deterministic" ? "Local deterministic" : value,
    });
    const run = (providers, invocationCount = 1, failedInvocationCount = 0, unknownProviderCount = 0) => ({
      reasoning: { providers, invocationCount, failedInvocationCount, unknownProviderCount },
    });
    const gemini = inbox.buildInboxReasoningBadge([run(["gemini-flash-lite"], 1, 1)]);
    assert.equal(gemini.textContent, "Gemini");
    assert.match(gemini.className, /inbox-reasoning-gemini/);
    assert.match(gemini.title, /gemini-flash-lite/);
    assert.match(gemini.title, /1 failed.*do not prove completed model inference/);
    const codex = inbox.buildInboxReasoningBadge([run(["codex-app-server"]), run(["codex-app-server"])]);
    assert.equal(codex.textContent, "Codex");
    assert.match(codex.className, /inbox-reasoning-codex/);
    const mixed = inbox.buildInboxReasoningBadge([run(["gemini-flash-lite"]), run(["codex-app-server"])]);
    assert.equal(mixed.textContent, "Mixed");
    assert.equal(mixed.attributes["aria-label"], "Reasoning provider: Mixed.");
    assert.match(mixed.title, /codex-app-server, gemini-flash-lite/);
    assert.equal(inbox.buildInboxReasoningBadge([run(["gemini-flash-lite", "deterministic"], 2, 1)]).textContent, "Mixed");
    const missing = inbox.buildInboxReasoningBadge([{}, run([], 0)]);
    assert.equal(missing.textContent, "Unknown");
    assert.match(missing.title, /No recorded reasoning invocation; current Settings are not used/);
    const incomplete = inbox.buildInboxReasoningBadge([run(["codex-app-server"]), run([], 1, 0, 1), {}]);
    assert.equal(incomplete.textContent, "Codex + ?");
    assert.equal(inbox.buildInboxReasoningBadge([run(["codex-app-server", "gemini-flash-lite"], 3, 0, 1)]).textContent, "Mixed + ?");
    assert.match(incomplete.title, /1 recorded attempt\(s\) have no provider metadata/);
    assert.match(incomplete.title, /1 source run\(s\) have no recorded reasoning invocation/);
    assert.equal(inbox.buildInboxReasoningBadge([run(["codex-app-server"]), {}]).textContent, "Codex");
    assert.equal(inbox.buildInboxReasoningBadge([run(["deterministic"])]).textContent, "Local deterministic");
  } finally {
    globalThis.document = originalDocument;
  }
});

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
