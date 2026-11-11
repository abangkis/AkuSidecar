import assert from "node:assert/strict";
import { once } from "node:events";
import { mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { launchFixtureChrome } from "../scripts/auth-session-fixture/chrome.mjs";

const fixtureRoot = process.env.AKUSIDECAR_HLS_FIXTURE_ROOT;
const chromePath = process.env.AKUSIDECAR_HLS_FIXTURE_CHROME;
const captureRoot = process.env.AKUSIDECAR_HLS_QUALITY_CAPTURE_ROOT;

test("isolated Chromium preserves playback across manual quality, Auto adaptation, seeking and disposal", {
  skip: !fixtureRoot || !chromePath || !captureRoot,
  timeout: 90_000,
}, async () => {
  const parent = await realpath(fixtureRoot);
  const root = await realpath(await mkdtemp(join(parent, "timeline-hls-quality-")));
  assert.equal(dirname(root), parent);
  const capture = await readFile(join(captureRoot, "matt-two-level.json"));
  const server = createServer(async (request, response) => {
    if (request.url === "/") return response.end('<!doctype html><title>Isolated generic Timeline HLS quality fixture</title><link rel="stylesheet" href="/styles.css"><main class="shell"><article id="fixture-host" class="source-layout-card" style="max-width:640px;margin:auto"></article></main>');
    if (request.url === "/capture.json") { response.setHeader("Content-Type", "application/json"); return response.end(capture); }
    if (!/^\/(?:[a-z-]+\.js|styles\.css|vendor\/hls\.mjs)$/.test(request.url)) { response.writeHead(404); return response.end(); }
    try {
      response.setHeader("Content-Type", request.url.endsWith(".css") ? "text/css" : "text/javascript");
      response.end(await readFile(new URL("../internal/httpapi/web" + request.url, import.meta.url)));
    } catch { response.writeHead(404); response.end(); }
  });
  let chrome;
  try {
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    chrome = await launchFixtureChrome(await realpath(chromePath), root, "collector");
    const { browserContextId } = await chrome.send("Target.createBrowserContext");
    const page = await chrome.page("http://127.0.0.1:" + server.address().port, browserContextId);
    await chrome.send("Target.activateTarget", { targetId: page.targetId });
    await page.evaluate(`void (${exerciseQuality})().then(result => { window.fixtureResult = result; })`);
    let visualReady = false;
    for (let step = 0; step < 150 && !visualReady; step++) { await delay(100); visualReady = await page.evaluate("window.qualityVisualReady || !!window.fixtureResult"); }
    assert.equal(await page.evaluate("window.qualityVisualReady"), true, JSON.stringify(await page.evaluate("window.fixtureResult")));
    const { sessionId } = await chrome.send("Target.attachToTarget", { targetId: page.targetId, flatten: true });
    const layouts = [];
    for (const [name, width, height] of [["desktop", 900, 680], ["narrow", 360, 640]]) {
      await chrome.sendSession(sessionId, "Emulation.setDeviceMetricsOverride", { width, height, deviceScaleFactor: 1, mobile: false });
      const layout = await page.evaluate(`(() => {
        const select = document.querySelector('select[aria-label="Video quality"]');
        const quality = document.querySelector('.source-layout-video-quality');
        const rect = select.getBoundingClientRect();
        return { width: innerWidth, pageWidth: document.documentElement.scrollWidth, options: Array.from(select.options, option => option.textContent), disabled: select.disabled,
          select: { left: rect.left, right: rect.right, top: rect.top, bottom: rect.bottom, height: rect.height },
          toolbarVisible: getComputedStyle(quality).display !== 'none' && quality.getBoundingClientRect().height > 0 };
      })()`);
      assert.equal(layout.toolbarVisible, true);
      assert.equal(layout.disabled, false);
      assert.equal(layout.options.length, 3);
      assert.ok(layout.pageWidth <= width && layout.select.left >= 0 && layout.select.right <= width && layout.select.height >= 24, JSON.stringify(layout));
      layouts.push({ name, ...layout });
      const screenshot = await chrome.sendSession(sessionId, "Page.captureScreenshot", { format: "png" });
      await writeFile(join(captureRoot, `quality-${name}.png`), Buffer.from(screenshot.data, "base64"));
    }
    await chrome.sendSession(sessionId, "Emulation.setDeviceMetricsOverride", { width: 900, height: 680, deviceScaleFactor: 1, mobile: false });
    await page.evaluate("window.qualityVisualDone = true");
    let result;
    for (let step = 0; step < 700 && !result; step++) { await delay(100); result = await page.evaluate("window.fixtureResult"); }
    assert.ok(result, await page.evaluate("window.fixtureStage"));
    await writeFile(join(captureRoot, "quality-playback-evidence.json"), JSON.stringify({ chrome: chrome.product, muted: true, layouts, ...result }, null, 2) + "\n");
    assert.equal(result.error, undefined, JSON.stringify(result));
    assert.deepEqual(result.manual.selected, ["0", "1", "auto"]);
    assert.equal(result.manual.sameVideo, true);
    assert.ok(result.manual.afterHigh >= result.manual.beforeHigh);
    assert.ok(result.manual.afterAuto >= result.manual.afterHigh);
    assert.ok(result.manual.naturalAdvance >= 6.3);
    assert.ok(result.manual.maxUnexpectedJump < 0.35, JSON.stringify(result.manual));
    assert.ok(result.manual.naturalAudioDecodedBytes > result.manual.switchAudioDecodedBytes);
    assert.ok(result.manual.naturalBuffered.every(track => track.ranges.some(([start, end]) => start <= Math.max(0, result.manual.afterHigh - 0.25) && end >= result.manual.naturalAdvance)), JSON.stringify(result.manual.naturalBuffered));
    assert.ok(result.manual.seekTime >= 17.2 && result.manual.seekTime < 18);
    assert.ok(result.manual.audioDecodedBytes > 0 && result.manual.decodedFrames > 0);
    assert.equal(result.manual.buffered.length, 2);
    assert.ok(result.manual.buffered.every(track => track.ranges.some(([start, end]) => start <= result.manual.seekTime && end >= result.manual.seekTime)));
    assert.equal(result.manual.disposedNoRequests, true);
    assert.equal(result.generic.source, "https://media.fixture.invalid");
    assert.equal(result.generic.upshift, true);
    assert.equal(result.generic.downshift, true);
    assert.ok(result.generic.maxUnexpectedJump < 0.35, JSON.stringify(result.generic));
    assert.ok(result.generic.finalAudioDecodedBytes > result.generic.upshiftAudioDecodedBytes);
    assert.ok(result.generic.decodedFrames > 0 && result.generic.videoHeight === 270);
    assert.ok(result.generic.buffered.every(track => track.ranges.some(([start, end]) => start <= Math.max(0, result.generic.upshiftTime - 0.25) && end >= result.generic.currentTime)), JSON.stringify(result.generic.buffered));
    assert.equal(result.generic.disposedNoRequests, true);
    assert.ok(result.generic.throttledRequests > 0);
    console.log(`Manual 270p → 360p → Auto retained the same video; seek=${result.manual.seekTime.toFixed(2)}s, audio=${result.manual.audioDecodedBytes} decoded bytes. Generic non-X Auto upshift/downshift observed under 400 kbps throttling.`);
  } finally {
    try { await chrome?.close(); }
    finally { server.close(); await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
  }
});

async function exerciseQuality() {
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
  const requests = [];
  const states = [];
  let stage = "setup";
  try {
    const nativeFetch = window.fetch;
    const capture = await (await nativeFetch("/capture.json")).json();
    const resources = new Map(capture.resources.map(entry => [entry.url, Uint8Array.from(atob(entry.body), character => character.charCodeAt(0))]));
    resources.set(capture.masterUrl, new TextEncoder().encode(capture.qualityFixture.master));
    const { createTimelineMedia } = await import("/timeline-media.js");
    const { attachTimelineHls } = await import("/timeline-hls-player.js");
    const buffers = [];
    const addSourceBuffer = MediaSource.prototype.addSourceBuffer;
    MediaSource.prototype.addSourceBuffer = function(mime) {
      const buffer = addSourceBuffer.call(this, mime);
      buffers.push({ mime, buffer });
      return buffer;
    };
    let throttled = false;
    let generic = false;
    let throttleAfterFirstHigh = false;
    const pending = new Set();
    const continuity = [];
    let continuityVideo;
    window.fetch = async (url, options) => {
      const lookup = generic ? url.replace("https://media.fixture.invalid", "https://video.twimg.com") : url;
      const key = generic && url.endsWith("/master.m3u8") ? capture.masterUrl : lookup;
      if (!resources.has(key) || options.credentials !== "omit" || options.referrerPolicy !== "no-referrer" || options.redirect !== "error") throw Error(`Unexpected quality fixture request: ${url}`);
      const bytes = resources.get(key);
      const media = /\.(?:mp4|m4s)(?:\?|$)/.test(key);
      const slow = throttled && media;
      const triggerThrottle = generic && !throttleAfterFirstHigh && /\/vid\/avc1\/\d+\/\d+\/694x360\/.+\.m4s$/.test(key);
      if (triggerThrottle) throttleAfterFirstHigh = true;
      const request = { url, throttled: slow, bytes: bytes.byteLength, aborted: false };
      requests.push(request);
      if (!media) return new Response(bytes);
      let offset = 0;
      const token = {};
      pending.add(token);
      return new Response(new ReadableStream({
        async pull(controller) {
          if (slow) await delay(80);
          if (options.signal.aborted) { request.aborted = true; pending.delete(token); controller.error(new DOMException("Aborted", "AbortError")); return; }
          const end = Math.min(bytes.length, offset + (slow ? 4096 : bytes.length));
          controller.enqueue(bytes.slice(offset, end));
          offset = end;
          if (offset >= bytes.length) {
            pending.delete(token);
            controller.close();
            if (triggerThrottle) throttled = true;
          }
        },
        cancel() { request.aborted = true; pending.delete(token); },
      }));
    };
    async function wait(check, description, steps = 300) {
      stage = window.fixtureStage = description;
      for (let i = 0; i < steps; i++) {
        if (continuityVideo) continuity.push({ time: continuityVideo.currentTime, at: performance.now() });
        if (check()) return;
        await delay(50);
      }
      throw Error(`Timed out: ${description}`);
    }
    const timeline = createTimelineMedia({
      configureNativePostLink: (anchor, url) => { anchor.href = url; },
      safeMediaUrl: () => "data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=",
      safePlaybackUrl: () => null, recordUIActivity: () => {}, sourceLabel: () => "X",
    });
    const assetId = /\/amplify_video\/(\d+)\//.exec(capture.masterUrl)[1];
    const media = { kind: "video", playbackMode: "inline", playbackUrl: capture.masterUrl, posterUrl: `https://pbs.twimg.com/amplify_video_thumb/${assetId}/img/fixture.jpg` };
    const card = timeline.buildMedia([media], "x", "video", capture.postUrl);
    document.querySelector("#fixture-host").append(card);
    if (requests.length) throw Error("Playback fetched before the explicit click");
    card.querySelector("button.source-layout-media-item").click();
    const video = card.querySelector("video");
    video.muted = true;
    void video.play().catch(() => {});
    const select = card.querySelector('select[aria-label="Video quality"]');
    const status = card.querySelector(".source-layout-video-quality-status");
    await wait(() => select.options.length === 3 && video.currentTime >= 0.25, "Timeline two-level playback");
    video.pause();
    window.qualityVisualReady = true;
    await wait(() => window.qualityVisualDone, "Desktop and narrow quality layout verification", 400);
    void video.play().catch(() => {});
    continuityVideo = video;
    const selected = [];
    const choose = value => { select.value = value; select.dispatchEvent(new Event("change")); selected.push(value); };
    choose("0");
    await wait(() => select.value === "0" && /270p/.test(status.textContent) && !/Playing|Switching/.test(status.textContent), "Manual low quality");
    const beforeHigh = video.currentTime;
    choose("1");
    await wait(() => video.videoHeight === 360 && !/Playing|Switching/.test(status.textContent), "Manual high quality");
    const afterHigh = video.currentTime;
    choose("auto");
    await wait(() => select.value === "auto" && /^Auto/.test(status.textContent) && video.currentTime > afterHigh + 0.2, "Return to Auto without reset");
    const afterAuto = video.currentTime;
    const switchAudioDecodedBytes = video.webkitAudioDecodedByteCount;
    await wait(() => video.currentTime >= Math.max(6.3, afterAuto + 3.1), "Natural audio/video playback across the quality-switch boundary");
    const naturalAdvance = video.currentTime;
    const naturalAudioDecodedBytes = video.webkitAudioDecodedByteCount;
    const naturalBuffered = buffers.map(({ mime, buffer }) => ({ mime, ranges: Array.from({ length: buffer.buffered.length }, (_, index) => [buffer.buffered.start(index), buffer.buffered.end(index)]) }));
    const maxUnexpectedJump = Math.max(0, ...continuity.slice(1).map((sample, index) => sample.time - continuity[index].time - (sample.at - continuity[index].at) / 1000));
    continuityVideo = null;
    video.currentTime = 17;
    await wait(() => video.currentTime >= 17.2 && video.webkitAudioDecodedByteCount > 0, "Seek with decoded audio and video");
    const manual = { selected, beforeHigh, afterHigh, afterAuto, naturalAdvance, switchAudioDecodedBytes, naturalAudioDecodedBytes, naturalBuffered, maxUnexpectedJump, sameVideo: card.querySelector("video") === video, seekTarget: 17, seekTime: video.currentTime, audioDecodedBytes: video.webkitAudioDecodedByteCount, decodedFrames: video.getVideoPlaybackQuality().totalVideoFrames,
      buffered: buffers.map(({ mime, buffer }) => ({ mime, ranges: Array.from({ length: buffer.buffered.length }, (_, index) => [buffer.buffered.start(index), buffer.buffered.end(index)]) })),
    };
    video.pause();
    timeline.stopObservingInlineVideoVisibilityWithin(card);
    card.remove();
    const afterDispose = requests.length;
    await delay(300);
    manual.disposedNoRequests = requests.length === afterDispose;

    generic = true;
    const genericVideo = document.createElement("video");
    genericVideo.muted = true;
    document.body.append(genericVideo);
    let fatal;
    const genericBufferStart = buffers.length;
    const policy = {
      authorizeRequest(context) {
        const url = new URL(context.url);
        if (url.origin !== "https://media.fixture.invalid" || url.username || url.password || /%2f|%5c/i.test(url.pathname)) throw Error("Foreign generic fixture URL");
        return url.href;
      },
      validatePlaylist(text) { if (!text.startsWith("#EXTM3U")) throw Error("Invalid generic fixture manifest"); },
    };
    const master = capture.qualityFixture.master.replaceAll("https://video.twimg.com", "https://media.fixture.invalid");
    resources.set(capture.masterUrl, new TextEncoder().encode(master));
    const session = attachTimelineHls(genericVideo, { url: "https://media.fixture.invalid/master.m3u8", policy, onState: state => states.push({ ...state, time: genericVideo.currentTime, throttled }), onError: error => { fatal = error.message; } });
    await session.ready;
    continuity.length = 0;
    continuityVideo = genericVideo;
    void genericVideo.play().catch(() => {});
    await wait(() => fatal || states.some(state => state.selected === "auto" && state.active === "1"), "Generic Auto upshift");
    if (fatal) throw Error(fatal);
    const upshiftIndex = states.findIndex(state => state.selected === "auto" && state.active === "1");
    const upshiftTime = genericVideo.currentTime;
    const upshiftAudioDecodedBytes = genericVideo.webkitAudioDecodedByteCount;
    const throttledAt = genericVideo.currentTime;
    // The fixture drops throughput immediately after the first high-quality
    // video segment, before HLS can prefetch the entire short clip.
    await wait(() => fatal || states.slice(upshiftIndex + 1).some(state => state.throttled && state.selected === "auto" && state.active === "0"), "Generic Auto downshift under 400 kbps", 600);
    if (fatal) throw Error(fatal);
    const downshiftTime = genericVideo.currentTime;
    await wait(() => fatal || genericVideo.currentTime >= downshiftTime + 0.8 && genericVideo.videoHeight === 270 && genericVideo.webkitAudioDecodedByteCount > upshiftAudioDecodedBytes, "Natural decoded audio/video after Auto downshift", 300);
    if (fatal) throw Error(fatal);
    const genericResult = { source: "https://media.fixture.invalid", upshift: true, downshift: true, throttledAt, upshiftTime, downshiftTime, currentTime: genericVideo.currentTime,
      upshiftAudioDecodedBytes, finalAudioDecodedBytes: genericVideo.webkitAudioDecodedByteCount,
      decodedFrames: genericVideo.getVideoPlaybackQuality().totalVideoFrames, videoHeight: genericVideo.videoHeight,
      maxUnexpectedJump: Math.max(0, ...continuity.slice(1).map((sample, index) => sample.time - continuity[index].time - (sample.at - continuity[index].at) / 1000)),
      buffered: buffers.slice(genericBufferStart).map(({ mime, buffer }) => ({ mime, ranges: Array.from({ length: buffer.buffered.length }, (_, index) => [buffer.buffered.start(index), buffer.buffered.end(index)]) })),
      throttledRequests: requests.filter(request => request.throttled).length, states };
    continuityVideo = null;
    genericVideo.pause();
    session.dispose();
    genericVideo.remove();
    const disposedAt = requests.length;
    await delay(400);
    genericResult.disposedNoRequests = requests.length === disposedAt && pending.size === 0;
    return { manual, generic: genericResult, requests };
  } catch (error) { return { error: error.stack || String(error), stage, states, requests }; }
}
