import assert from "node:assert/strict";
import { once } from "node:events";
import { mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { basename, dirname, join } from "node:path";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { launchFixtureChrome } from "../scripts/auth-session-fixture/chrome.mjs";

const fixtureRoot = process.env.AKUSIDECAR_HLS_FIXTURE_ROOT;
const chromePath = process.env.AKUSIDECAR_HLS_FIXTURE_CHROME;
const captureRoot = process.env.AKUSIDECAR_HLS_CDN_CAPTURE_ROOT;

test("isolated Chrome decodes captured Derya and Matt audio plus video through explicit Timeline playback", {
  skip: !fixtureRoot || !chromePath || !captureRoot,
  timeout: 90_000,
}, async () => {
  const parent = await realpath(fixtureRoot);
  const root = await realpath(await mkdtemp(join(parent, "x-hls-split-")));
  assert.equal(dirname(root), parent);
  assert.match(basename(root), /^x-hls-split-/);
  const captures = new Map();
  for (const name of ["derya", "matt"]) captures.set(`/capture-${name}.json`, await readFile(join(captureRoot, `${name}.json`)));
  const server = createServer(async (request, response) => {
    if (request.url === "/") {
      response.setHeader("Content-Type", "text/html");
      return response.end("<!doctype html><title>Isolated X split audio fixture</title>");
    }
    if (captures.has(request.url)) {
      response.setHeader("Content-Type", "application/json");
      return response.end(captures.get(request.url));
    }
    if (!/^\/[a-z-]+\.js$/.test(request.url)) { response.writeHead(404); return response.end(); }
    try {
      response.setHeader("Content-Type", "text/javascript");
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
    await page.evaluate(`void (${playCapturedFixtures})().then(result => { window.fixtureResult = result; })`);
    let result;
    for (let step = 0; step < 600 && !result; step++) {
      await delay(100);
      result = await page.evaluate("window.fixtureResult");
    }
    assert.ok(result, await page.evaluate("window.fixtureStage"));
    assert.equal(result.error, undefined, result.error);
    for (const clip of result.clips) {
      assert.equal(clip.beforeClick, 0);
      assert.ok(clip.currentTime >= 1, JSON.stringify(clip));
      assert.ok(clip.videoWidth > 0 && clip.decodedFrames > 0, JSON.stringify(clip));
      assert.ok(clip.audioDecodedBytes > 0, JSON.stringify(clip));
      assert.ok(clip.audioDecodedBytes > clip.initialAudioDecodedBytes, JSON.stringify(clip));
      assert.equal(clip.sourceBuffers.length, 2);
      assert.equal(clip.mediaSourceState, "ended");
      assert.equal(clip.sourceBuffers.every(buffer => buffer.ranges.length > 0), true);
      const [videoRange, audioRange] = clip.sourceBuffers.map(buffer => buffer.ranges[0]);
      assert.ok(Math.abs(videoRange[0] - audioRange[0]) < 0.1);
      assert.ok(Math.abs(videoRange[1] - audioRange[1]) < 0.1);
      assert.ok(clip.sourceBuffers.every(buffer => buffer.ranges.some(([start, end]) => start <= clip.currentTime && end >= clip.currentTime)));
      assert.equal(clip.successfulRequests, clip.capturedResources);
      assert.equal(clip.audioFailureFallback, true);
      console.log(`${clip.name}: ${clip.videoWidth}x${clip.videoHeight}, ${clip.currentTime.toFixed(2)}s, ${clip.decodedFrames} decoded frames, ${clip.audioDecodedBytes} decoded audio bytes, ${clip.successfulRequests} bounded CDN resources.`);
    }
    const evidence = { chrome: chrome.product, isolatedProfile: chrome.profile, network: "Offline replay of bounded public CDN captures; no signed-in profile", muted: true, ...result };
    await writeFile(join(captureRoot, "playback-evidence.json"), JSON.stringify(evidence, null, 2) + "\n");
  } finally {
    try { await chrome?.close(); }
    finally {
      server.close();
      // This exact direct child was allocated by this test, never a user profile.
      await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 });
    }
  }
});

async function playCapturedFixtures() {
  try {
    const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
    const nativeFetch = window.fetch;
    const { createTimelineMedia } = await import("/timeline-media.js");
    const addBuffer = MediaSource.prototype.addSourceBuffer;
    const buffers = [];
    let currentMediaSource;
    MediaSource.prototype.addSourceBuffer = function(mime) {
      currentMediaSource = this;
      const buffer = addBuffer.call(this, mime);
      buffers.push({ mime, buffer });
      return buffer;
    };
    const timeline = createTimelineMedia({
      configureNativePostLink: (anchor, url) => { anchor.href = url; },
      safeMediaUrl: () => "data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=",
      safePlaybackUrl: () => null,
      recordUIActivity: () => {},
      sourceLabel: () => "X",
    });
    const clips = [];
    for (const name of ["derya", "matt"]) {
      window.fixtureStage = `Loading ${name} captured resources`;
      const capture = await (await nativeFetch(`/capture-${name}.json`)).json();
      const resources = new Map(capture.resources.map(entry => [entry.url, Uint8Array.from(atob(entry.body), character => character.charCodeAt(0))]));
      const assetId = /\/amplify_video\/(\d+)\//.exec(capture.masterUrl)[1];
      const evidence = { kind: "video", playbackMode: "inline", playbackUrl: capture.masterUrl, posterUrl: `https://pbs.twimg.com/amplify_video_thumb/${assetId}/img/fixture.jpg` };
      let requests = 0, failAudio = false;
      window.fetch = async (url, options) => {
        requests++;
        if (options.credentials !== "omit" || options.referrerPolicy !== "no-referrer" || options.redirect !== "error" || !resources.has(url)) throw Error("Unexpected CDN fixture request");
        if (failAudio && /\/aud\//.test(url) && /\.m4s(?:\?|$)/.test(url)) throw Error("Injected audio resource failure");
        return new Response(resources.get(url));
      };
      const build = () => {
        const card = timeline.buildMedia([evidence], "x", "video", capture.postUrl);
        document.body.append(card);
        return card;
      };
      buffers.length = 0;
      const card = build();
      const beforeClick = requests;
      card.querySelector("button.source-layout-media-item").click();
      const video = card.querySelector("video");
      video.muted = true;
      void video.play().catch(() => {});
      window.fixtureStage = `Decoding ${name} audio and video`;
      let initialAudioDecodedBytes;
      for (let step = 0; step < 400; step++) {
        if (!video.isConnected) throw Error(`${name} unexpectedly fell back`);
        if (video.currentTime >= 0.25 && video.webkitAudioDecodedByteCount > 0) initialAudioDecodedBytes ??= video.webkitAudioDecodedByteCount;
        if (video.currentTime >= 1 && video.webkitAudioDecodedByteCount > initialAudioDecodedBytes && currentMediaSource?.readyState === "ended") break;
        await delay(50);
      }
      const clip = {
        name, beforeClick, currentTime: video.currentTime, videoWidth: video.videoWidth, videoHeight: video.videoHeight,
        decodedFrames: video.getVideoPlaybackQuality().totalVideoFrames,
        initialAudioDecodedBytes,
        audioDecodedBytes: video.webkitAudioDecodedByteCount,
        mediaSourceState: currentMediaSource?.readyState,
        sourceBuffers: buffers.map(({ mime, buffer }) => ({ mime, ranges: Array.from({ length: buffer.buffered.length }, (_, i) => [buffer.buffered.start(i), buffer.buffered.end(i)]) })),
        successfulRequests: requests, capturedResources: resources.size,
      };
      timeline.stopObservingInlineVideoVisibilityWithin(card);
      card.remove();
      failAudio = true;
      const fallback = build();
      fallback.querySelector("button.source-layout-media-item").click();
      const failedVideo = fallback.querySelector("video");
      failedVideo.muted = true;
      for (let step = 0; step < 100 && !fallback.querySelector("a.source-layout-media-item"); step++) await delay(50);
      clip.audioFailureFallback = fallback.querySelector("a.source-layout-media-item")?.href === capture.postUrl;
      timeline.stopObservingInlineVideoVisibilityWithin(fallback);
      fallback.remove();
      clips.push(clip);
    }
    return { clips };
  } catch (error) { return { error: error.stack || String(error), stage: window.fixtureStage }; }
}
