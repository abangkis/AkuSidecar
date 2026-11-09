import assert from "node:assert/strict";
import { once } from "node:events";
import { mkdtemp, readFile, realpath, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { basename, dirname, join } from "node:path";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { launchFixtureChrome } from "../scripts/auth-session-fixture/chrome.mjs";
import { videoPlaybackMissing } from "../internal/httpapi/web/video-recapture-policy.js";
import { xTimelineHlsUrl } from "../internal/httpapi/web/x-hls-playback.js";

const assetId = "2119481355943948288";
const asset = `https://video.twimg.com/amplify_video/${assetId}/`;
const evidence = {
  kind: "video",
  playbackMode: "inline",
  playbackUrl: asset + "pl/fixture.m3u8?tag=12",
  posterUrl: `https://pbs.twimg.com/amplify_video_thumb/${assetId}/img/fixture.jpg`,
};

test("Timeline admits HLS only when X video and poster describe the same CDN asset", () => {
  assert.equal(xTimelineHlsUrl(evidence, "x"), evidence.playbackUrl);
  assert.equal(videoPlaybackMissing({ media: [evidence] }, "x", () => null), false);
  assert.equal(videoPlaybackMissing({ media: [{ ...evidence, playbackUrl: "" }] }, "x", () => null), true);
  assert.equal(xTimelineHlsUrl(evidence, "instagram"), null);
  for (const playbackUrl of [
    evidence.playbackUrl.replace(assetId, "2119471570100113408"),
    evidence.playbackUrl.replace("video.twimg.com", "example.com"),
    asset + "pl/folder%2fmaster.m3u8",
    asset + "pl/avc1/640x360/variant.m3u8",
    "blob:https://x.com/fixture",
  ]) {
    assert.equal(xTimelineHlsUrl({ ...evidence, playbackUrl }, "x"), null);
  }
});

const fixtureRoot = process.env.AKUSIDECAR_HLS_FIXTURE_ROOT;
const chromePath = process.env.AKUSIDECAR_HLS_FIXTURE_CHROME;

test("offline Chrome decodes X HLS fixtures through Timeline and falls back for an unrelated segment", {
  skip: !fixtureRoot || !chromePath,
  timeout: 45_000,
}, async () => {
  const parent = await realpath(fixtureRoot);
  const root = await realpath(await mkdtemp(join(parent, "x-hls-")));
  assert.equal(dirname(root), parent);
  assert.match(basename(root), /^x-hls-/);
  const server = createServer(async (request, response) => {
    if (request.url === "/") {
      response.setHeader("Content-Type", "text/html");
      return response.end("<!doctype html><title>X HLS Timeline fixture</title>");
    }
    if (!/^\/[a-z-]+\.js$/.test(request.url)) {
      response.writeHead(404);
      return response.end();
    }
    try {
      const module = await readFile(new URL("../internal/httpapi/web" + request.url, import.meta.url));
      response.setHeader("Content-Type", "text/javascript");
      response.end(module);
    } catch {
      response.writeHead(404);
      response.end();
    }
  });
  let chrome;
  try {
    server.listen(0, "127.0.0.1");
    await once(server, "listening");
    chrome = await launchFixtureChrome(await realpath(chromePath), root, "collector");
    const { browserContextId } = await chrome.send("Target.createBrowserContext");
    const page = await chrome.page("http://127.0.0.1:" + server.address().port, browserContextId);
    await chrome.send("Target.activateTarget", { targetId: page.targetId });
    await page.evaluate(`void (${playFixture})(${JSON.stringify(asset)}, ${JSON.stringify(evidence)}).then(result => { window.fixtureResult = result; })`);
    let result;
    for (let step = 0; step < 120 && !result; step++) {
      await delay(100);
      result = await page.evaluate("window.fixtureResult");
    }
    assert.ok(result, await page.evaluate("window.fixtureStage + ' ' + JSON.stringify(window.fixturePlayback?.())"));
    assert.equal(result.error, undefined, result.error);
    assert.equal(result.beforeClick, 0);
    assert.ok(result.currentTime >= 0.15, JSON.stringify(result));
    assert.equal(result.videoWidth, 64);
    assert.ok(result.segments > 0);
    assert.equal(result.requests, result.segments + 3);
    assert.equal(result.crossAssetFallback, true);
    assert.equal(result.crossAssetRequests, 2);
    console.log(`Decoded ${result.videoWidth}px video at ${result.currentTime.toFixed(2)}s from ${result.segments} offline segment(s).`);
  } finally {
    try {
      await chrome?.close();
    } finally {
      server.close();
      // root was just allocated as a direct child of the caller's fixture directory.
      await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
    }
  }
});

async function playFixture(asset, evidence) {
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
  async function waitFor(check, label) {
    for (let step = 0; step < 100; step++) {
      if (check()) return;
      await delay(50);
    }
    throw new Error(label);
  }

  async function recordFixture() {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 64;
    const context = canvas.getContext("2d");
    const stream = canvas.captureStream(0);
    const [track] = stream.getVideoTracks();
    const recorder = new MediaRecorder(stream, { mimeType: "video/mp4;codecs=avc1.42001f" });
    const chunks = [];
    const stopped = new Promise((resolve, reject) => {
      recorder.onstop = resolve;
      recorder.onerror = reject;
    });
    recorder.ondataavailable = event => chunks.push(event.data);
    recorder.start(150);
    for (let frame = 0; frame < 5; frame++) {
      context.fillStyle = frame % 2 ? "#000" : "#fff";
      context.fillRect(0, 0, 64, 64);
      track.requestFrame();
      await delay(220);
    }
    recorder.stop();
    stream.getTracks().forEach(track => track.stop());
    await stopped;
    const bytes = new Uint8Array(await new Blob(chunks).arrayBuffer());
    const view = new DataView(bytes.buffer);
    const fragments = [];
    let avcConfig = -1;
    for (let index = 4; index < bytes.length - 7; index++) {
      if (view.getUint32(index) === 0x61766343) { // avcC
        avcConfig = index + 5;
        break;
      }
    }
    for (let index = 0; index + 8 <= bytes.length;) {
      const size = view.getUint32(index);
      if (size < 8 || index + size > bytes.length) throw new Error("Invalid fixture MP4 box");
      if (view.getUint32(index + 4) === 0x6d6f6f66) fragments.push(index); // moof
      index += size;
    }
    if (avcConfig < 0 || fragments.length === 0) throw new Error("Chrome did not record fragmented AVC MP4");
    const codecs = "avc1." + Array.from(bytes.slice(avcConfig, avcConfig + 3), byte => byte.toString(16).padStart(2, "0")).join("");
    return {
      codecs,
      init: bytes.slice(0, fragments[0]),
      segments: fragments.map((start, index) => bytes.slice(start, fragments[index + 1])),
    };
  }

  try {
    window.fixtureStage = "Loading Timeline";
    const { createTimelineMedia } = await import("/timeline-media.js");
    window.fixtureStage = "Recording fixture";
    const { codecs, init, segments } = await recordFixture();
    window.fixtureStage = "Constructing HLS fixture";
    const master = "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=100000,CODECS=\"" + codecs + "\"\nvariant.m3u8";
    const playlistHead = '#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:2\n#EXT-X-MAP:URI="../vid/init.mp4"';
    const playlist = [playlistHead, ...segments.flatMap((_, index) => ["#EXTINF:1.5,", "../vid/" + index + ".m4s"]), "#EXT-X-ENDLIST"].join("\n");
    const unrelated = [playlistHead, "#EXTINF:1,", "../../999/vid/foreign.m4s", "#EXT-X-ENDLIST"].join("\n");
    const resources = new Map([
      [evidence.playbackUrl, master],
      [asset + "pl/variant.m3u8", playlist],
      [asset + "vid/init.mp4", init],
      ...segments.map((bytes, index) => [asset + "vid/" + index + ".m4s", bytes]),
    ]);
    const requests = [];
    window.fetch = async (url, options) => {
      requests.push(url);
      if (options.credentials !== "omit" || options.redirect !== "error" || !resources.has(url)) {
        throw new Error("Unexpected fixture request");
      }
      return new Response(resources.get(url));
    };
    const nativePost = "https://x.com/fixture/status/456";
    const timeline = createTimelineMedia({
      configureNativePostLink: (anchor, url) => { anchor.href = url; },
      safeMediaUrl: () => "data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=",
      safePlaybackUrl: () => null,
      recordUIActivity: () => {},
      sourceLabel: () => "X",
    });
    function buildCard() {
      const card = timeline.buildMedia([evidence], "x", "video", nativePost);
      document.body.append(card);
      return card;
    }
    const card = buildCard();
    const beforeClick = requests.length;
    card.querySelector("button.source-layout-media-item").click();
    const video = card.querySelector("video");
    video.muted = true;
    window.fixtureStage = "Waiting for playback";
    window.fixturePlayback = () => ({
      currentTime: video.currentTime,
      readyState: video.readyState,
      paused: video.paused,
      connected: video.isConnected,
      error: video.error?.message,
      visibility: document.visibilityState,
      duration: video.duration,
      requests,
      segments: segments.length,
      codecs,
    });
    void video.play().catch(() => {});
    await waitFor(() => video.currentTime >= 0.15 || !video.isConnected, "Timeline video did not advance");
    if (!video.isConnected) throw new Error(`Fixture fell back after ${requests.length} requests. Media error: ${video.error?.message}`);
    const { currentTime, videoWidth } = video;
    const requestCount = requests.length;
    timeline.stopObservingInlineVideoVisibilityWithin(card);
    card.remove();

    resources.set(asset + "pl/variant.m3u8", unrelated);
    const fallbackCard = buildCard();
    window.fixtureStage = "Checking fallback";
    fallbackCard.querySelector("button.source-layout-media-item").click();
    await waitFor(() => fallbackCard.querySelector("a.source-layout-media-item"), "Native fallback was not shown");
    return {
      beforeClick,
      currentTime,
      videoWidth,
      segments: segments.length,
      requests: requestCount,
      crossAssetFallback: fallbackCard.querySelector("a.source-layout-media-item").href === nativePost,
      crossAssetRequests: requests.length - requestCount,
    };
  } catch (error) {
    return { error: error.stack || String(error) };
  }
}
