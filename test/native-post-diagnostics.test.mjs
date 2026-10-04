import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const script = readFileSync(new URL("../internal/httpapi/web/native-post-diagnostics.js", import.meta.url), "utf8");
const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");

function load(storage, messages = []) {
  const window = { dispatchEvent() {} };
  vm.runInNewContext(script, {
    window,
    CustomEvent: class { constructor(type) { this.type = type; } },
    sessionStorage: storage,
    console: { info: (label, detail) => messages.push({ label, detail }) },
  });
  return window.akuNativePostDiagnostics;
}

test("native post trace survives a reload and stores only allowlisted fields", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  const requestId = "broker_" + "a".repeat(32);
  load(storage).record(requestId, "relay_error", {
    errorKind: "epoch_mismatch", status: 409, elapsedMs: 12,
    url: "https://www.linkedin.com/posts/private", bridgeToken: "secret-token",
    message: "private error text",
  });
  const messages = [];
  const recovered = load(storage, messages).read();
  assert.equal(recovered.length, 1);
  assert.equal(recovered[0].trace, requestId);
  assert.equal(recovered[0].errorKind, "epoch_mismatch");
  assert.equal(recovered[0].status, 409);
  assert.equal(messages[0].label, "native_post_trace_recovered");
  assert.equal(JSON.stringify([...values.values()]).includes("linkedin.com"), false);
  assert.equal(JSON.stringify([...values.values()]).includes("secret-token"), false);
  assert.equal(JSON.stringify([...values.values()]).includes("private error text"), false);
});

test("a pressed native-post link remains traceable if no click follows", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  const diagnostics = load(storage);
  let pointerdown;
  const handler = app.slice(app.indexOf("const nativePointerTraces = new WeakMap();"), app.indexOf("function configureNativePostLink"));
  vm.runInNewContext(handler, {
    state: {}, nativePostWaitReason: () => "", syncNativePostAvailability() {},
    document: { addEventListener: (_name, callback) => { pointerdown = callback; } },
    crypto: { randomUUID: () => "11111111-2222-4333-8444-555555555555" },
    logNativePostTrace: diagnostics.record,
  });
  const link = { dataset: {} };
  pointerdown({ isTrusted: true, button: 0, target: { closest: () => link } });
  assert.equal(load(storage).read().at(-1).trace, "pointer_11111111222243338444555555555555");
  assert.equal(load(storage).read().at(-1).phase, "pointerdown");
});

test("native post trace remains bounded and does not block clicks when storage is denied", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  const diagnostics = load(storage);
  for (let i = 0; i < 70; i++) diagnostics.record(`native_post_${i}_a`, "click");
  assert.equal(load(storage).read().length, 64);
  const denied = {
    getItem: () => { throw new Error("Storage denied"); },
    setItem: () => { throw new Error("Storage denied"); },
  };
  assert.doesNotThrow(() => load(denied).record("broker_" + "b".repeat(32), "click"));
});

test("native post error classification never retains raw error text", () => {
  const diagnostics = load({ getItem: () => null, setItem: () => {} });
  assert.equal(diagnostics.errorKind("AkuBrowser UI must be active."), "ui_not_foreground");
  assert.equal(diagnostics.errorKind("Reader intent expired or UI foreground changed"), "ui_not_foreground");
  assert.equal(diagnostics.errorKind("AkuBrowser restarted; refresh the page."), "epoch_mismatch");
  assert.equal(diagnostics.errorKind("Extension context invalidated."), "extension_unavailable");
  assert.equal(diagnostics.errorKind("https://www.linkedin.com/posts/private"), "other");
});
