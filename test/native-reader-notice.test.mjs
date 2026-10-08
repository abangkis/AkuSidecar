import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { nativeReaderNotice, nativeReaderResumeMessage } from "../internal/httpapi/web/native-reader-notice.js";

test("reader banner distinguishes blocked, closing, failure and clear states", () => {
  assert.equal(nativeReaderNotice(null).visible, false);
  const blocked = nativeReaderNotice({ nativeReaderBlocked: true, nativeReaderBlockedSince: "2026-10-05T07:30:00Z" });
  assert.equal(blocked.disabled, false);
  assert.match(blocked.detail, /Tertunda sejak/);
  assert.match(blocked.button, /Tutup jendela post/);
  const busy = nativeReaderNotice({ nativeReaderBlocked: true }, { busy: true });
  assert.equal(busy.disabled, true);
  assert.match(busy.title, /Menutup/);
  const failure = nativeReaderNotice({ nativeReaderBlocked: true }, { feedback: "Chrome did not exit" });
  assert.equal(failure.disabled, false);
  assert.equal(failure.feedback, "Chrome did not exit");
  assert.doesNotMatch(nativeReaderNotice({ nativeReaderBlocked: true, nativeReaderBlockedSince: "invalid" }).detail, /Invalid|Tertunda sejak/);
});

test("manual closure is an explicit user action, including before a failed button click, and clears with runtime state", () => {
  const runtimeFailure = "native reader window contains an unverified page; close it manually";
  const status = { nativeReaderBlocked: true };
  const notice = nativeReaderNotice(status, { runtimeFailure });
  assert.match(notice.manualAction, /Anda perlu.*secara manual/);
  assert.match(notice.feedback, /langsung di browser/);
  assert.equal(nativeReaderNotice(status, { feedback: runtimeFailure }).manualAction, notice.manualAction);
  assert.equal(nativeReaderNotice(status, { busy: true, runtimeFailure }).manualAction, "");
  assert.equal(nativeReaderNotice({ nativeReaderBlocked: false }, { feedback: runtimeFailure }).visible, false);
});

function actionFixture(api) {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const state = { bootstrap: { autoUpdate: { nativeReaderBlocked: true } }, closingNativeReaders: false };
  const notices = [];
  let polls = 0;
  const context = { state, nativePostOpening: false, api, nativeReaderResumeMessage,
    syncRunButtons() {}, syncNativePostLinks() {}, renderAutoUpdateStatus() {},
    pollCollectionRuntime: async () => {}, renderSession() {}, startPolling() { polls++; },
    showNotice: message => notices.push(message), Error };
  vm.createContext(context);
  vm.runInContext(app.slice(app.indexOf("async function closeNativeReadersAndResume()"), app.indexOf("function syncRunButtons()")), context);
  return { state, context, notices, polls: () => polls };
}

test("explicit close action is single flight and only claims a batch if server started it", async () => {
  let finish, calls = 0;
  const fixture = actionFixture((path, options) => {
    assert.equal(path, "/api/collection/native-reader/close-and-resume");
    assert.equal(options.method, "POST");
    calls++;
    return new Promise(resolve => { finish = resolve; });
  });
  const first = fixture.context.closeNativeReadersAndResume();
  await fixture.context.closeNativeReadersAndResume();
  assert.equal(calls, 1);
  assert.equal(fixture.state.closingNativeReaders, true);
  finish({ closed: true, resumeReason: "Daily quota reached", autoUpdate: { nativeReaderBlocked: false, reason: "Waiting for cadence" }, collectionRuntime: { effective: "headless" } });
  await first;
  assert.equal(fixture.state.closingNativeReaders, false);
  assert.equal(fixture.state.bootstrap.collectionRuntime.effective, "headless");
  assert.match(fixture.notices[0], /Daily quota reached/);
  assert.equal(fixture.polls(), 0);
  await fixture.context.closeNativeReadersAndResume();
  assert.equal(calls, 1);
});

test("frequency-limited recovery explains the wait without promising an immediate batch", () => {
  const response = { resumeReason: "Bounded generation allowance reached", autoUpdate: { nextCheckAt: "2026-10-05T09:28:11Z" } };
  const message = nativeReaderResumeMessage(response);
  assert.match(message, /menunggu batas frekuensi/);
  assert.match(message, /Pemeriksaan berikutnya sekitar/);
  assert.doesNotMatch(message, /mulai menyiapkan batch|Bounded generation/);
  response.autoUpdate.nextCheckAt = "invalid";
  assert.match(nativeReaderResumeMessage(response), /memeriksa kembali secara otomatis/);
  assert.doesNotMatch(nativeReaderResumeMessage(response), /Invalid/);
  response.session = { id: "started" };
  assert.match(nativeReaderResumeMessage(response), /mulai menyiapkan batch/);
});

test("close failure preserves retry state; an unverified close never starts session polling", async () => {
  for (const result of [new Error("Reader window refused closure"), { closed: false, session: { id: "unsafe" } }]) {
    const fixture = actionFixture(async () => { if (result instanceof Error) throw result; return result; });
    await fixture.context.closeNativeReadersAndResume();
    assert.equal(fixture.state.closingNativeReaders, false);
    assert.ok(fixture.state.nativeReaderResumeFeedback);
    assert.equal(fixture.state.bootstrap.autoUpdate.nativeReaderBlocked, true);
    assert.equal(fixture.polls(), 0);
  }
});

test("verified close adopts prepared session and prevents closing during an in-flight open", async () => {
  let calls = 0;
  const fixture = actionFixture(async () => { calls++; return { closed: true, autoUpdate: { state: "running" }, session: { id: "prepared" } }; });
  fixture.context.nativePostOpening = true;
  await fixture.context.closeNativeReadersAndResume();
  assert.equal(calls, 0);
  fixture.context.nativePostOpening = false;
  await fixture.context.closeNativeReadersAndResume();
  assert.equal(fixture.state.session.id, "prepared");
  assert.equal(fixture.polls(), 1);
});

test("stale close notice refreshes authoritative no-reader state without claiming closure or starting a batch", async () => {
  const fixture = actionFixture(async () => ({ closed: false, resumeOutcome: "no_reader", autoUpdate: { nativeReaderBlocked: false }, collectionRuntime: { effective: "headless" } }));
  await fixture.context.closeNativeReadersAndResume();
  assert.equal(fixture.state.bootstrap.autoUpdate.nativeReaderBlocked, false);
  assert.equal(fixture.polls(), 0);
  assert.equal(fixture.state.nativeReaderResumeFeedback, "");
  assert.match(fixture.notices[0], /sudah tidak terbuka/);
  assert.doesNotMatch(fixture.notices[0], /mulai menyiapkan/);
});

test("verified closure with missing scheduler telemetry reports unknown recovery instead of erasing status", async () => {
  const fixture = actionFixture(async () => ({ closed: true, resumeOutcome: "headless_not_ready", collectionRuntime: { state: "failed" } }));
  await fixture.context.closeNativeReadersAndResume();
  assert.match(fixture.state.nativeReaderResumeFeedback, /status update belum tersedia/);
  assert.equal(fixture.state.bootstrap.collectionRuntime.state, "failed");
  assert.ok(fixture.state.bootstrap.autoUpdate);
  assert.equal(fixture.polls(), 0);
  assert.equal(fixture.notices.length, 0);
});
