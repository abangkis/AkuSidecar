import test from "node:test";
import assert from "node:assert/strict";
import vm from "node:vm";
import { readFileSync } from "node:fs";
import { createNativePostRouter } from "../internal/httpapi/web/native-post-routing.js";

test("native post routes by live owner including temporary Browser under headless preference", async () => {
  let runtime = { requested: "browser", effective: "headless", state: "ready" };
  const calls = [];
  const route = createNativePostRouter({ readRuntime: async () => runtime,
    openHeadless: (request) => calls.push(["headless", request]),
    openForeground: (request) => calls.push(["foreground", request]) });
  const request = { source: "x", url: "https://x.com/a/status/1" };
  await route(request);
  runtime = { requested: "headless", effective: "browser", state: "ready", pending: true };
  await route(request);
  runtime = { requested: "headless", effective: "browser", state: "ready", pending: true, nativeReaderOnly: true };
  await route(request);
  assert.deepEqual(calls, [["headless", request], ["foreground", request], ["headless", request]]);
});

test("unknown or starting owners and failed status reads never open a reader", async () => {
  for (const runtime of [null, { effective: "unavailable" }, { effective: "headless", state: "starting" }]) {
    const route = createNativePostRouter({ readRuntime: async () => runtime,
      openHeadless: () => assert.fail("opened"), openForeground: () => assert.fail("opened") });
    await assert.rejects(route({}));
  }
  const route = createNativePostRouter({ readRuntime: async () => { throw new Error("offline"); },
    openHeadless: () => assert.fail("opened"), openForeground: () => assert.fail("opened") });
  await assert.rejects(route({}), /offline/);
});

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
function fixture(runtime, delayed = false) {
  const handlers = new Map(), messages = [], calls = [];
  let releaseRuntime;
  const status = delayed ? new Promise((resolve) => { releaseRuntime = resolve; }) : Promise.resolve({ collectionRuntime: runtime });
  const window = { setTimeout, clearTimeout,
    addEventListener: (name, fn) => handlers.set(name, fn),
    removeEventListener: (name) => handlers.delete(name),
    postMessage: (message) => messages.push(message) };
  const context = { createNativePostRouter, state: { bootstrap: { bridge: { compatible: false } } },
    api: (path) => { calls.push(path); return status; }, window,
    endpoint: "http://127.0.0.1:11122", performance, AbortController,
    NATIVE_POST_OPEN_TIMEOUT_MS: 35000, logNativePostTrace() {} };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("const routeNativePost ="), app.indexOf("function setSourceSessionStatus")), context);
  return { context, calls, messages, releaseRuntime,
    open: () => context.openNativePostInReaderWindow("https://x.com/a/status/1", "x", "broker_" + "a".repeat(32)),
    reply: (type) => handlers.get("message")({ source: window, origin: context.endpoint,
      data: { type, requestId: "broker_" + "a".repeat(32), message: "broker rejected" } }) };
}

for (const effective of ["headless", "browser"]) {
  test(`${effective}: one click dispatches the URL and completes without preparation-only click`, async () => {
    const f = fixture({ effective, state: "ready", available: true });
    const opened = f.open();
    await new Promise(setImmediate);
    assert.deepEqual(f.calls, ["/api/collection/runtime"]);
    assert.equal(f.messages.length, 1);
    assert.equal(f.messages[0].url, "https://x.com/a/status/1");
    assert.equal(f.messages[0].requestId, "broker_" + "a".repeat(32));
    f.reply("AKU_BROWSER_NATIVE_POST_OPENED");
    await opened;
  });
}

test("an early trusted broker failure cancels routing before status arrives", async () => {
  const f = fixture(null, true);
  const opened = f.open();
  f.reply("AKU_BROWSER_NATIVE_POST_OPEN_FAILED");
  await assert.rejects(opened, /broker rejected/);
  f.releaseRuntime({ collectionRuntime: { effective: "headless", state: "ready" } });
  await new Promise(setImmediate);
  assert.equal(f.messages.length, 0, "late status must not replay the failed click");
});
