import assert from "node:assert/strict";
import test from "node:test";
import { createTimelineHlsLoader, timelineHlsLimits } from "../internal/httpapi/web/timeline-hls-loader.js";

const policy = {
  authorizeRequest(context) {
    const url = new URL(context.url);
    if (url.origin !== "http://localhost:1234" || !url.pathname.startsWith("/fixture/")) throw Error("Denied fixture resource");
    return url.href;
  },
  validatePlaylist(text) { if (!text.startsWith("#EXTM3U")) throw Error("Invalid fixture playlist"); },
};
const fragment = { url: "http://localhost:1234/fixture/segment.mp4", type: "media-fragment", responseType: "arraybuffer" };
function run(Loader, context = fragment) {
  const loader = new Loader();
  const originalStats = loader.stats;
  const events = [];
  const finished = new Promise(resolve => loader.load(context, {}, {
    onSuccess: (response, stats) => { events.push("success"); resolve({ response, stats }); },
    onError: error => { events.push("error"); resolve({ error }); },
    onTimeout: () => { events.push("timeout"); resolve({ timeout: true }); },
    onAbort: () => { events.push("abort"); resolve({ aborted: true }); },
  }));
  return { loader, originalStats, events, finished };
}
const turn = () => new Promise(resolve => setImmediate(resolve));

function streamedBytes(size, withLength = false) {
  let sent = 0;
  return new Response(new ReadableStream({
    pull(controller) {
      if (sent === size) { controller.close(); return; }
      const length = Math.min(64 * 1024, size - sent);
      const bytes = new Uint8Array(length);
      bytes.fill(0x5a);
      sent += length;
      controller.enqueue(bytes);
    },
  }), { headers: withLength ? { "Content-Length": String(size) } : {} });
}

test("default loader accepts the measured Matt fragment and the exact 12 MiB boundary", async t => {
  for (const size of [9_606_473, 12 * 1024 * 1024]) {
    for (const withLength of [false, true]) {
      await t.test(`${size} streamed bytes ${withLength ? "with" : "without"} Content-Length`, async t => {
        t.mock.method(globalThis, "fetch", async () => streamedBytes(size, withLength));
        const result = run(createTimelineHlsLoader({ policy }));
        const { response, stats } = await result.finished;
        assert.ok(response, "The complete supported fragment must reach HLS.");
        assert.equal(response.data.byteLength, size);
        assert.equal(stats.loaded, size);
        assert.equal(stats.total, size);
        assert.ok(stats.chunkCount > 1);
        assert.equal(new Uint8Array(response.data)[size - 1], 0x5a);
        assert.deepEqual(result.events, ["success"]);
        result.loader.destroy();
      });
    }
  }
});

test("a 12 MiB plus one byte stream is rejected even without Content-Length", async t => {
  const size = 12 * 1024 * 1024 + 1;
  t.mock.method(globalThis, "fetch", async () => {
    const response = streamedBytes(size);
    assert.equal(response.headers.get("content-length"), null);
    return response;
  });
  const result = run(createTimelineHlsLoader({ policy }));
  assert.match((await result.finished).error.text, /byte limit/);
  assert.equal(result.loader.stats.loaded, size);
  assert.deepEqual(result.events, ["error"]);
  result.loader.destroy();
});

test("an oversized Content-Length is rejected before any response body read", async t => {
  let reads = 0;
  t.mock.method(globalThis, "fetch", async () => ({
    ok: true,
    headers: new Headers({ "Content-Length": String(12 * 1024 * 1024 + 1) }),
    body: { getReader() { reads++; throw Error("An oversized body must not be read"); } },
  }));
  const result = run(createTimelineHlsLoader({ policy }));
  assert.match((await result.finished).error.text, /byte limit/);
  assert.equal(reads, 0);
  assert.equal(result.loader.stats.loaded, 0);
  assert.deepEqual(result.events, ["error"]);
  result.loader.destroy();
});

test("generic loader supports a non-X source and preserves the stats reference used by ABR", async t => {
  t.mock.method(globalThis, "fetch", async (url, init) => {
    assert.equal(url, fragment.url);
    assert.equal(init.credentials, "omit"); assert.equal(init.redirect, "error"); assert.equal(init.referrerPolicy, "no-referrer");
    return new Response(new Uint8Array(23));
  });
  const result = run(createTimelineHlsLoader({ policy }));
  const { response, stats } = await result.finished;
  assert.equal(response.data.byteLength, 23);
  assert.equal(stats, result.originalStats);
  assert.equal(stats.loaded, 23);
  assert.ok(stats.loading.first >= stats.loading.start && stats.loading.end >= stats.loading.first);
  result.loader.destroy();
  assert.equal(stats.aborted, false);
  assert.deepEqual(result.events, ["success"]);
});

test("denied resource never reaches fetch and denial poisons the session", async t => {
  const fetch = t.mock.method(globalThis, "fetch", async () => new Response("unused"));
  const errors = [];
  const Loader = createTimelineHlsLoader({ policy, onTerminalError: error => errors.push(error) });
  assert.match((await run(Loader, { ...fragment, url: "https://evil.example/a" }).finished).error.text, /Denied/);
  assert.match((await run(Loader).finished).error.text, /Denied/);
  assert.equal(fetch.mock.callCount(), 0); assert.equal(errors.length, 1);
});

test("stream limits apply without Content-Length and shared budget includes concurrent tracks", async t => {
  t.mock.method(globalThis, "fetch", async () => new Response(new Uint8Array(6)));
  const Loader = createTimelineHlsLoader({ policy, limits: { ...timelineHlsLimits, resource: 8, playback: 10 } });
  const results = await Promise.all([run(Loader).finished, run(Loader).finished]);
  assert.equal(results.filter(r => r.response).length, 1);
  assert.equal(results.filter(r => /byte limit/.test(r.error?.text)).length, 1);
  assert.match((await run(Loader).finished).error.text, /byte limit/);
});

test("oversized resource and playlist are rejected before callbacks can hand bytes to HLS", async t => {
  t.mock.method(globalThis, "fetch", async () => new Response("#EXTM3U\n12345678"));
  for (const context of [fragment, { ...fragment, type: "manifest" }]) {
    const result = await run(createTimelineHlsLoader({ policy, limits: { ...timelineHlsLimits, resource: 8, playlist: 8 } }), context).finished;
    assert.match(result.error.text, /byte limit/);
  }
});

test("playlist validation runs before success", async t => {
  t.mock.method(globalThis, "fetch", async () => new Response("invalid"));
  assert.match((await run(createTimelineHlsLoader({ policy }), { ...fragment, type: "manifest" }).finished).error.text, /Invalid fixture/);
});

test("destroy aborts blocked fetch once and discards a late response", async t => {
  let release, capturedSignal;
  t.mock.method(globalThis, "fetch", (_url, init) => { capturedSignal = init.signal; return new Promise(resolve => { release = resolve; }); });
  const result = run(createTimelineHlsLoader({ policy }));
  result.loader.destroy(); result.loader.destroy();
  assert.equal((await result.finished).aborted, true);
  assert.equal(capturedSignal.aborted, true);
  release(new Response(new Uint8Array(3))); await turn();
  assert.deepEqual(result.events, ["abort"]);
});

test("parent cancellation aborts both active tracks and suppresses late callbacks", async t => {
  t.mock.method(globalThis, "fetch", (_url, init) => new Promise((_resolve, reject) => init.signal.addEventListener("abort", () => reject(init.signal.reason))));
  const controller = new AbortController();
  const Loader = createTimelineHlsLoader({ policy, signal: controller.signal });
  const audio = run(Loader), video = run(Loader);
  controller.abort();
  assert.equal((await audio.finished).aborted, true); assert.equal((await video.finished).aborted, true);
  await turn(); assert.deepEqual(audio.events, ["abort"]); assert.deepEqual(video.events, ["abort"]);
});

test("timeout aborts network and emits timeout once", async t => {
  t.mock.method(globalThis, "fetch", (_url, init) => new Promise((_resolve, reject) => init.signal.addEventListener("abort", () => reject(init.signal.reason))));
  const result = run(createTimelineHlsLoader({ policy, limits: { ...timelineHlsLimits, timeout: 5 } }));
  assert.equal((await result.finished).timeout, true);
  result.loader.destroy(); assert.deepEqual(result.events, ["timeout"]);
});

test("failed required audio aborts its concurrent video request through the session boundary", async t => {
  let videoSignal;
  t.mock.method(globalThis, "fetch", (url, init) => {
    if (url.endsWith("audio.mp4")) return Promise.resolve(new Response("missing", { status: 404 }));
    videoSignal = init.signal;
    return new Promise((_resolve, reject) => init.signal.addEventListener("abort", () => reject(init.signal.reason)));
  });
  const controller = new AbortController();
  const errors = [];
  const Loader = createTimelineHlsLoader({ policy, signal: controller.signal, onTerminalError: error => { errors.push(error); controller.abort(); } });
  const video = run(Loader);
  const audio = run(Loader, { ...fragment, url: "http://localhost:1234/fixture/audio.mp4" });
  await Promise.all([video.finished, audio.finished]);
  assert.equal(errors.length, 1); assert.equal(videoSignal.aborted, true);
  assert.deepEqual(video.events, ["abort"]); assert.deepEqual(audio.events, ["abort"]);
});
