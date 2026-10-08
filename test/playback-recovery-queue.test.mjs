import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";
import { createPlaybackRecoveryQueue } from "../internal/httpapi/web/playback-recovery-queue.js";
import { collectionModeState } from "../internal/httpapi/web/collection-mode.js";

const request = (id, playbackUrl = `https://s.cdninstagram.com/${id}.mp4`) => ({ id, source: "instagram", playbackUrl });

test("busy requests remain queued, drain serially, and deduplicate the failed URL", async () => {
  let ready = false; let release; const calls = [];
  const queue = createPlaybackRecoveryQueue({ isReady: () => ready, resolveEntry: (r) => r,
    recover: async (r) => { calls.push(r.id); await new Promise((resolve) => { release = resolve; }); } });
  assert.equal(queue.enqueue(request("first")), true);
  assert.equal(queue.enqueue(request("first")), false);
  queue.enqueue(request("second")); await queue.drain(); assert.deepEqual(calls, []);
  ready = true;
  const draining = queue.drain(); await queue.drain();
  assert.deepEqual(calls, ["first"]);
  assert.equal(queue.enqueue(request("first")), false);
  ready = false; release(); await draining;
  assert.deepEqual(calls, ["first"]);
  ready = true; const resumed = queue.drain();
  assert.deepEqual(calls, ["first", "second"]); release(); await resumed;
  assert.equal(queue.enqueue(request("first", "https://s.cdninstagram.com/new.mp4")), true);
});

test("changed or removed evidence cancels stale recovery; pending memory stays bounded", async () => {
  const calls = [];
  const queue = createPlaybackRecoveryQueue({ isReady: () => true, resolveEntry: (r) => r.id === "current" ? r : null,
    recover: async (r) => calls.push(r), maxEntries: 2 });
  assert.equal(queue.enqueue(request("stale")), true);
  assert.equal(queue.enqueue(request("current")), true);
  assert.equal(queue.enqueue(request("overflow")), false);
  assert.equal(queue.enqueue(request("current", "https://s.cdninstagram.com/refreshed.mp4")), true);
  await queue.drain();
  assert.equal(calls.length, 1); assert.equal(calls[0].playbackUrl, "https://s.cdninstagram.com/refreshed.mp4");
});

test("a failed attempt unlocks the queue without retrying the same failed URL forever", async () => {
  let calls = 0;
  const queue = createPlaybackRecoveryQueue({ isReady: () => true, resolveEntry: (r) => r,
    recover: async () => { calls++; throw new Error("unavailable"); } });
  queue.enqueue(request("first")); await assert.rejects(queue.drain(), /unavailable/);
  assert.equal(queue.enqueue(request("first")), false);
  queue.enqueue(request("second")); await assert.rejects(queue.drain(), /unavailable/);
  assert.equal(calls, 2);
});

function uiFixture() {
  const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const entry = { id: "ig", source: "instagram", item: { sourceUrl: "https://www.instagram.com/p/Own/" },
    evidence: { media: [{ kind: "video", playbackMode: "inline", playbackUrl: request("ig").playbackUrl }] } };
  const requests = []; const notices = []; const waits = []; const elements = new Map();
  const state = { session: { id: "batch" }, timelineItems: [entry], foregroundRecaptureOffers: new Map(),
    bootstrap: { onboarding: { status: "completed" }, collectionRuntime: { available: true, effective: "headless", state: "ready", activeLeases: 1 } } };
  const context = { state, requests, notices, waits, URL, console, createPlaybackRecoveryQueue, collectionModeState,
    sourceDescriptor: () => ({ playbackRecoveryCapability: "native_post_recapture" }),
    safeSourceUrl: (url) => url?.startsWith("https://www.instagram.com/p/") ? url : null,
    showNotice: (s) => notices.push(s), showError: (e) => assert.fail(e), clearNotice() {},
    renderNativeReaderNotice() {}, renderTimelineCalibrationProgress: () => false, runDisabledReason: () => "", preparedBatchDisabledReason: () => "",
    setSettingsText() {}, setNoticeText: (_, s) => notices.push(s),
    $: (key) => { if (!elements.has(key)) elements.set(key, { classList: { toggle() {} }, setAttribute() {} }); return elements.get(key); },
    document: { querySelectorAll: () => [] },
    api: async (url, options) => { requests.push({ url, options }); return { recapture: { id: "job", payload: { captureRuntime: { driver: "headless" } } } }; },
    mediaRecaptureTransport: () => "sidecar",
    waitForMediaRecapture: () => new Promise((resolve) => waits.push(resolve)),
    refreshTimeline: async () => { entry.evidence.media[0].playbackUrl = "https://s.cdninstagram.com/fresh.mp4"; },
  };
  vm.createContext(context);
  for (const [start, end] of [["function safePlaybackUrl(", "async function api("],
    ["function queueInlinePlaybackRecovery(", "function pauseOtherInlineVideos("],
    ["async function recaptureMedia(", "function dispatchMediaRecapture("],
    ["function syncRunButtons()", "function renderTimelineCalibrationProgress()"]]) {
    vm.runInContext(app.slice(app.indexOf(start), app.indexOf(end)), context);
  }
  return { context, state, entry, requests, notices, waits };
}
const settle = () => new Promise((resolve) => setImmediate(resolve));

test("actual UI queues playback_error during a batch and sends one recapture when idle", async () => {
  const f = uiFixture();
  f.context.queueInlinePlaybackRecovery(f.entry, "instagram", request("ig").playbackUrl);
  assert.equal(f.requests.length, 0); assert.match(f.notices[0], /queued/);
  f.state.session = null; f.context.syncRunButtons(); assert.equal(f.requests.length, 0, "Live capture lease still blocks recovery");
  f.state.bootstrap.collectionRuntime.activeLeases = 0; f.context.syncRunButtons(); await settle();
  assert.equal(f.requests.length, 1);
  assert.equal(f.requests[0].url, "/api/timeline/ig/recapture");
  assert.equal(f.requests[0].options.body.reason, "playback_error");
  assert.equal(f.requests[0].options.body.captureMode, "background");
  f.context.syncRunButtons(); assert.equal(f.requests.length, 1);
  f.waits[0]({ outcome: "recovered" }); await settle();
  f.context.syncRunButtons(); assert.equal(f.requests.length, 1);
  assert.ok(f.notices.some((s) => s.includes("Playback refreshed")));
});

test("actual UI drops obsolete playback evidence before idle and waits for other recapture", async () => {
  const f = uiFixture();
  f.state.session = null; f.state.bootstrap.collectionRuntime.activeLeases = 0; f.state.mediaRecaptureActive = true;
  f.context.queueInlinePlaybackRecovery(f.entry, "instagram", request("ig").playbackUrl);
  assert.equal(f.requests.length, 0);
  f.entry.evidence.media[0].playbackUrl = "https://s.cdninstagram.com/fresh.mp4";
  f.state.mediaRecaptureActive = false; f.context.syncRunButtons(); await settle();
  assert.equal(f.requests.length, 0);
});

test("actual UI X playback error uses the existing serial recovery queue", async () => {
  const f = uiFixture();
  f.entry.source = "x";
  f.entry.item.sourceUrl = "https://x.com/example/status/12345";
  f.entry.evidence.media[0].playbackUrl = "https://video.twimg.com/ext_tw_video/12345/pu/vid/clip.mp4";
  f.context.safeSourceUrl = (url) => url?.startsWith("https://x.com/") ? url : null;
  f.context.queueInlinePlaybackRecovery(f.entry, "x", f.entry.evidence.media[0].playbackUrl);
  assert.equal(f.requests.length, 0);
  f.state.session = null;
  f.state.bootstrap.collectionRuntime.activeLeases = 0;
  f.context.syncRunButtons(); await settle();
  assert.equal(f.requests.length, 1);
  assert.equal(f.requests[0].options.body.reason, "playback_error");
  f.waits[0]({ outcome: "recovered" }); await settle();
});
