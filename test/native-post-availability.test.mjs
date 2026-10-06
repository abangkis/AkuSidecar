import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { nativePostWaitReason, syncNativePostAvailability } from "../internal/httpapi/web/native-post-availability.js";

test("hybrid collection waits until session cleanup and profile leases finish", () => {
  const runtime = { requested: "headless", effective: "headless", activeLeases: 0 };
  assert.equal(nativePostWaitReason(runtime, { status: "collecting" }), "Menunggu koleksi selesai");
  assert.equal(nativePostWaitReason({ ...runtime, activeLeases: 1 }, { status: "completed" }), "Menunggu koleksi selesai");
  assert.equal(nativePostWaitReason({ ...runtime, effective: "browser", collectionBorrowSource: "facebook" }, null), "Menunggu koleksi selesai");
  assert.equal(nativePostWaitReason(runtime, { status: "completed" }), "");
  assert.equal(nativePostWaitReason(runtime, null), "");
  assert.equal(nativePostWaitReason({ ...runtime, nativeReaderOnly: true, effective: "browser", activeLeases: 1 }, null), "");
  assert.equal(nativePostWaitReason({ requested: "browser", effective: "browser", activeLeases: 1 }, { status: "collecting" }), "");
  assert.equal(nativePostWaitReason(runtime, null, true), "Membuka native post…");
});

test("actual native link blocks collection clicks and concurrent opens, then restores after completion", async () => {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  let click, finish, calls = 0;
  const link = { dataset: {}, setAttribute() {}, removeAttribute() {},
    addEventListener(name, fn) { if (name === "click") click = fn; } };
  const state = { bootstrap: { collectionRuntime: { requested: "headless", effective: "headless" } }, session: { status: "collecting" } };
  const context = { state, nativePostWaitReason, syncNativePostAvailability,
    document: { addEventListener() {}, querySelectorAll: () => [link] },
    logNativePostTrace() {}, showError: (error) => assert.fail(error),
    openNativePostInReaderWindow() { calls++; return new Promise(resolve => { finish = resolve; }); } };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("const nativePointerTraces ="), app.indexOf("const routeNativePost =")), context);
  context.configureNativePostLink(link, "https://x.com/a/status/1", "x");
  const event = () => ({ isTrusted: true, defaultPrevented: false, button: 0, preventDefault() { this.defaultPrevented = true; } });
  click(event()); click(event());
  assert.equal(calls, 0);
  state.session = null;
  context.syncNativePostLinks();
  click(event()); click(event());
  assert.equal(calls, 1);
  assert.equal(link.dataset.akuNativeWait, "Membuka native post…");
  finish(); await new Promise(setImmediate);
  assert.equal(link.dataset.akuNativeWait, undefined);
  click(event());
  assert.equal(calls, 2);
  finish(); await new Promise(setImmediate);
});

test("manual foreground result reaches the user as a visible success notice", async () => {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const endpoint = "http://127.0.0.1:11122";
  const notices = [];
  let messageListener;
  const window = {
    addEventListener(type, listener) { if (type === "message") messageListener = listener; },
    removeEventListener() {},
    setTimeout,
    clearTimeout,
  };
  const context = {
    window, endpoint, NATIVE_POST_OPEN_TIMEOUT_MS: 5_000, AbortController,
    performance: { now: () => 1 },
    routeNativePost: async () => {},
    logNativePostTrace() {},
    showNotice: message => notices.push(message),
    setTimeout, clearTimeout, Error,
  };
  vm.createContext(context);
  const start = app.indexOf("function openNativePostInReaderWindow(");
  const end = app.indexOf("function setSourceSessionStatus(", start);
  vm.runInContext(app.slice(start, end), context);
  const requestId = "broker_" + "a".repeat(32);
  const message = "The post opened in your Native Reader tab. Bring that window forward manually.";
  const opened = context.openNativePostInReaderWindow("https://x.com/a/status/1", "x", requestId);
  messageListener({ source: window, origin: endpoint, data: {
    requestId, type: "AKU_BROWSER_NATIVE_POST_OPENED", foreground: "manual_required", message,
  } });
  const result = await opened;
  assert.equal(result.foreground, "manual_required");
  assert.deepEqual(notices, [message]);
});

test("wait state restores link without replacing poster or label and avoids repeated writes", () => {
  const attributes = new Map(); let writes = 0;
  const link = { dataset: {}, children: ["poster", "label"],
    setAttribute(k, v) { writes++; attributes.set(k, v); },
    removeAttribute(k) { writes++; attributes.delete(k); } };
  syncNativePostAvailability(link, "Menunggu koleksi selesai");
  assert.equal(attributes.get("aria-disabled"), "true");
  const before = writes;
  syncNativePostAvailability(link, "Menunggu koleksi selesai");
  assert.equal(writes, before);
  syncNativePostAvailability(link, "");
  assert.equal(attributes.has("aria-disabled"), false);
  assert.equal(link.dataset.akuNativeWait, undefined);
  assert.deepEqual(link.children, ["poster", "label"]);
});
