import assert from "node:assert/strict";
import { once } from "node:events";
import { createHash } from "node:crypto";
import { mkdtemp, readFile, realpath, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { dirname, join } from "node:path";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { launchFixtureChrome } from "../scripts/auth-session-fixture/chrome.mjs";

const rootPath = process.env.AKUSIDECAR_HLS_FIXTURE_ROOT;
const chromePath = process.env.AKUSIDECAR_HLS_FIXTURE_CHROME;
const audioPath = process.env.AKUSIDECAR_HLS_CDN_CAPTURE_ROOT;

test("isolated Chromium records and decodes genuine UHD AVC through the bounded Timeline pilot", {
  skip: !rootPath || !chromePath || !audioPath, timeout: 150000,
}, async () => {
  const parent = await realpath(rootPath);
  const root = await realpath(await mkdtemp(join(parent, "uhd-profile-")));
  assert.equal(dirname(root), parent);
  const audio = JSON.parse(await readFile(join(audioPath, "matt.json"), "utf8"));
  audio.resources = audio.resources.filter(entry => /\/aud\//.test(entry.url) || /\/pl\/mp4a\/32000\//.test(entry.url));
  const server = createServer(async (request, response) => {
    if (request.url === "/") return response.end('<!doctype html><link rel="stylesheet" href="/styles.css"><title>Bounded UHD fixture</title><main style="max-width:640px;margin:24px auto" id="fixture"></main>');
    if (request.url === "/audio.json") { response.setHeader("Content-Type", "application/json"); return response.end(JSON.stringify(audio)); }
    if (!/^\/(?:[a-z-]+\.js|styles\.css|vendor\/hls\.mjs)$/.test(request.url)) { response.writeHead(404); return response.end(); }
    try { response.setHeader("Content-Type", request.url.endsWith(".css") ? "text/css" : "text/javascript"); response.end(await readFile(new URL("../internal/httpapi/web" + request.url, import.meta.url))); }
    catch { response.writeHead(404); response.end(); }
  });
  let chrome;
  try {
    server.listen(0, "127.0.0.1"); await once(server, "listening");
    chrome = await launchFixtureChrome(await realpath(chromePath), root, "collector");
    const { browserContextId } = await chrome.send("Target.createBrowserContext");
    const page = await chrome.page("http://127.0.0.1:" + server.address().port, browserContextId);
    await chrome.send("Target.activateTarget", { targetId: page.targetId });
    await page.evaluate(`void (${exerciseUhd})().then(result => { window.uhdResult = result; })`);
    let result, screenshotSaved = false;
    for (let step = 0; step < 1300 && !result; step++) {
      await delay(100);
      if (!screenshotSaved && await page.evaluate("window.uhdScreenshotReady")) {
        const { sessionId } = await chrome.send("Target.attachToTarget", { targetId: page.targetId, flatten: true });
        const screenshot = await chrome.sendSession(sessionId, "Page.captureScreenshot", { format: "png" });
        await writeFile(join(parent, "uhd-pilot.png"), Buffer.from(screenshot.data, "base64"));
        screenshotSaved = true;
        await page.evaluate("window.uhdScreenshotDone = true");
      }
      result = await page.evaluate("window.uhdResult");
    }
    assert.ok(result, await page.evaluate("window.uhdStage"));
    const artifact = await page.evaluate("window.uhdArtifact");
    if (artifact) {
      const bytes = Buffer.from(artifact, "base64");
      await writeFile(join(parent, "uhd-recorded-avc.mp4"), bytes);
      result.recordedSha256 = createHash("sha256").update(bytes).digest("hex");
    }
    await writeFile(join(parent, "uhd-pilot-evidence.json"), JSON.stringify({ chrome: chrome.product, muted: true, ...result }, null, 2) + "\n");
    assert.equal(result.error, undefined, JSON.stringify(result));
    assert.equal(result.uhdWidth, 3840); assert.equal(result.uhdHeight, 2160);
    assert.ok(result.frames > 0 && result.audioEnd > result.audioStart);
    assert.ok(result.naturalTime >= result.uhdTime + 6);
    assert.ok(result.maxUnexpectedJump < 0.35, JSON.stringify(result));
    assert.ok(result.largeVideoResources > 0);
    assert.ok(result.totalBytes <= 128 * 1024 * 1024);
    assert.equal(result.sameMediaSource, true); assert.equal(result.afterDisposeStable, true);
    console.log(`Genuine ${result.uhdWidth}×${result.uhdHeight} AVC, ${result.frames} decoded frames; ${result.largeVideoResources} video resources above 12 MiB, ${(result.totalBytes / 1048576).toFixed(1)} MiB cumulative reads; audio and resize/Auto continuity passed.`);
  } finally {
    try { await chrome?.close(); }
    finally { server.close(); await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 }); }
  }
});

async function exerciseUhd() {
  const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
  const requests = [];
  let stage = "setup";
  try {
    const nativeFetch = window.fetch;
    const audio = await (await nativeFetch("/audio.json")).json();
    const { createTimelineMedia } = await import("/timeline-media.js");
    stage = window.uhdStage = "Recording genuine 3840×2160 AVC with 30 fps capture";
    const recordings = [record(640, 360), record(3840, 2160)];
    const [hd, uhd] = await Promise.all(recordings);
    window.uhdArtifact = btoa(Array.from(uhd.bytes, byte => String.fromCharCode(byte)).join(''));
    const assetId = "999999999999999999";
    const asset = `https://video.twimg.com/amplify_video/${assetId}/`;
    const oldAsset = /https:\/\/video\.twimg\.com\/amplify_video\/\d+\//.exec(audio.masterUrl)[0];
    const resources = new Map(audio.resources.map(entry => [entry.url.replace(oldAsset, asset), Uint8Array.from(atob(entry.body), value => value.charCodeAt(0))]));
    const audioEntry = audio.resources.find(entry => entry.url.includes('/pl/mp4a/32000/'));
    const audioLines = atob(audioEntry.body).replaceAll(oldAsset, asset).split(/\r?\n/);
    const map = audioLines.find(line => line.startsWith('#EXT-X-MAP:')).replaceAll('/amplify_video/2120307833774170112/', `/amplify_video/${assetId}/`);
    const audioSegments = audioLines.flatMap((line, index) => line.startsWith('#EXTINF:') ? [line, audioLines[index + 1].replaceAll('/amplify_video/2120307833774170112/', `/amplify_video/${assetId}/`)] : []).slice(0, 12);
    resources.set(asset + 'pl/audio.m3u8', '#EXTM3U\n#EXT-X-TARGETDURATION:3\n' + map + '\n' + audioSegments.join('\n') + '\n#EXT-X-ENDLIST');
    function addVideo(name, media) {
      resources.set(asset + `vid/${name}-init.mp4`, media.init);
      for (let index = 0; index < media.segments.length; index++) {
        let bytes = media.segments[index];
        if (name === 'uhd' && media.durations[index] >= 3.8 && bytes.byteLength < 16 * 1024 * 1024) {
          // A valid ignored BMFF free box exercises large-resource accounting.
          // Encoded pictures and timestamps remain the genuine 4K recording.
          const padded = new Uint8Array(16 * 1024 * 1024); padded.set(bytes);
          const view = new DataView(padded.buffer); view.setUint32(bytes.length, padded.length - bytes.length); view.setUint32(bytes.length + 4, 0x66726565);
          bytes = padded;
        }
        resources.set(asset + `vid/${name}-${index}.m4s`, bytes);
      }
      resources.set(asset + `pl/${name}.m3u8`, ['#EXTM3U', '#EXT-X-VERSION:7', '#EXT-X-TARGETDURATION:6', `#EXT-X-MAP:URI="../vid/${name}-init.mp4"`, ...media.durations.flatMap((duration, index) => [`#EXTINF:${duration.toFixed(6)},`, `../vid/${name}-${index}.m4s`]), '#EXT-X-ENDLIST'].join('\n'));
    }
    addVideo('hd', hd); addVideo('uhd', uhd);
    const masterUrl = asset + 'pl/master.m3u8';
    resources.set(masterUrl, ['#EXTM3U', '#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="shared",AUTOSELECT=YES,URI="audio.m3u8"', `#EXT-X-STREAM-INF:BANDWIDTH=1000000,RESOLUTION=640x360,FRAME-RATE=30,VIDEO-RANGE=SDR,CODECS="${hd.codec},mp4a.40.2",AUDIO="shared"`, 'hd.m3u8', `#EXT-X-STREAM-INF:BANDWIDTH=35000000,RESOLUTION=3840x2160,FRAME-RATE=30,VIDEO-RANGE=SDR,CODECS="${uhd.codec},mp4a.40.2",AUDIO="shared"`, 'uhd.m3u8'].join('\n'));
    window.fetch = async (url, options) => {
      if (!resources.has(url) || options.credentials !== 'omit' || options.redirect !== 'error' || options.referrerPolicy !== 'no-referrer') throw Error('Unexpected UHD fixture resource');
      const body = resources.get(url);
      requests.push({ url, bytes: typeof body === 'string' ? new TextEncoder().encode(body).length : body.byteLength });
      return new Response(body);
    };
    const timeline = createTimelineMedia({ configureNativePostLink: (anchor, url) => { anchor.href = url; }, safeMediaUrl: () => 'data:image/gif;base64,R0lGODlhAQABAAD/ACwAAAAAAQABAAACADs=', safePlaybackUrl: () => null, recordUIActivity() {}, sourceLabel: () => 'X' });
    const card = timeline.buildMedia([{ kind: 'video', playbackMode: 'inline', playbackUrl: masterUrl, posterUrl: `https://pbs.twimg.com/amplify_video_thumb/${assetId}/img/fixture.jpg`, width: 640, height: 360 }], 'x', 'video', 'https://x.com/fixture/status/999');
    document.querySelector('#fixture').append(card);
    if (requests.length) throw Error('UHD fetched before Play');
    card.querySelector('button.is-video-poster').click();
    const video = card.querySelector('video'); video.muted = true; void video.play().catch(() => {});
    const select = card.querySelector('select[aria-label="Video quality"]');
    async function wait(check, name, count = 500) {
      stage = window.uhdStage = name;
      for (let i = 0; i < count; i++) { if (!video.isConnected) throw Error('UHD fixture fell back'); if (check()) return; await delay(50); }
      throw Error(`Timeout ${name}; options=${select.innerHTML}; media=${video.error?.message}; requests=${requests.length}`);
    }
    await wait(() => select.options.length === 3 && video.currentTime > 0.15, 'HD Auto before manual UHD');
    if (requests.some(request => /\/vid\/uhd-\d+\.m4s$/.test(request.url))) throw Error('Small viewport Auto loaded UHD');
    const source = video.src;
    select.value = '1'; select.dispatchEvent(new Event('change'));
    await wait(() => video.videoWidth === 3840 && video.videoHeight === 2160, 'Genuine UHD decoded');
    const uhdTime = video.currentTime, audioStart = video.webkitAudioDecodedByteCount;
    video.pause(); window.uhdScreenshotReady = true;
    await wait(() => window.uhdScreenshotDone, 'UHD screenshot');
    void video.play().catch(() => {});
    select.value = 'auto'; select.dispatchEvent(new Event('change'));
    // Actual size and DPR notifications exercise conservative capping while
    // separate AAC and video continue playing; no native eager switch is called.
    video.style.width = '620px'; window.dispatchEvent(new Event('resize'));
    document.dispatchEvent(new Event('fullscreenchange'));
    const samples = [];
    await wait(() => { samples.push({ t: video.currentTime, at: performance.now() }); return video.currentTime >= uhdTime + 7; }, 'Natural UHD audio/Auto/resize continuity');
    const naturalTime = video.currentTime;
    const maxUnexpectedJump = Math.max(0, ...samples.slice(1).map((sample, i) => sample.t - samples[i].t - (sample.at - samples[i].at) / 1000));
    // Seek within the same session; the unit suite separately verifies that
    // its cumulative upgrade never resets consumption or follows shared AAC.
    select.value = '1'; select.dispatchEvent(new Event('change')); video.currentTime = 0;
    await wait(() => video.currentTime > 0.25 && video.videoHeight === 2160, 'Seek back to genuine UHD');
    const result = { codec: uhd.codec, recordingFrames: uhd.frames, recordedBytes: uhd.bytes.byteLength, segmentDurations: uhd.durations,
      captureTargetFramesPerSecond: 30, declaredPeakBandwidthMbps: 35,
      estimatedRawEncodedMbps: uhd.bytes.byteLength * 8 / uhd.durations.reduce((sum, duration) => sum + duration, 0) / 1000000,
      uhdWidth: 3840, uhdHeight: 2160, uhdTime, naturalTime, maxUnexpectedJump, audioStart, audioEnd: video.webkitAudioDecodedByteCount,
      frames: video.getVideoPlaybackQuality().totalVideoFrames, sameMediaSource: video.src === source,
      largeVideoResources: requests.filter(entry => /\/vid\/uhd-\d+\.m4s$/.test(entry.url) && entry.bytes > 12 * 1024 * 1024).length,
      totalBytes: requests.reduce((sum, entry) => sum + entry.bytes, 0), requests,
      fixtureNote: 'Genuine 3840x2160 AVC canvas recording with 30 fps capture; cached AAC audio; valid BMFF free boxes pad qualifying UHD fragments to 16 MiB for resource accounting. The 35 Mbps peak is declared fixture metadata, not a measured complex-scene encoded bitrate. Muted decoder evidence does not establish human audibility.' };
    video.pause(); timeline.stopObservingInlineVideoVisibilityWithin(card); card.remove();
    const before = requests.length; await delay(300); result.afterDisposeStable = requests.length === before;
    return result;
  } catch (error) { return { error: error.stack || String(error), stage, requests }; }

  async function record(width, height) {
    const canvas = document.createElement('canvas'); canvas.width = width; canvas.height = height;
    const context = canvas.getContext('2d'); const stream = canvas.captureStream(30);
    const recorder = new MediaRecorder(stream, { mimeType: 'video/mp4;codecs=avc1.640033', videoBitsPerSecond: 8000000, videoKeyFrameIntervalDuration: 4000 });
    const chunks = []; let frames = 0;
    const stopped = new Promise((resolve, reject) => { recorder.onstop = resolve; recorder.onerror = event => reject(Error(event.error?.message ?? '4K recorder error')); });
    recorder.ondataavailable = event => chunks.push(event.data);
    recorder.start(4000);
    const start = performance.now();
    while (performance.now() - start < 16500) {
      context.fillStyle = '#12251c'; context.fillRect(0, 0, width, height);
      context.fillStyle = frames % 2 ? '#b7f26c' : '#8fc9ff'; context.fillRect((frames * 17) % (width - 300), height / 3, 300, 300);
      context.fillStyle = '#fff'; context.font = '64px sans-serif'; context.fillText(`${width}×${height} frame${frames++}`, 64, 100);
      await delay(33);
    }
    recorder.stop(); stream.getTracks().forEach(track => track.stop()); await stopped;
    const bytes = new Uint8Array(await new Blob(chunks).arrayBuffer()); const view = new DataView(bytes.buffer);
    const fragments = []; let avc = -1, timescale = 0;
    for (let index = 4; index < bytes.length - 28; index++) {
      if (view.getUint32(index) === 0x61766343 && avc < 0) avc = index + 5;
      if (view.getUint32(index) === 0x6d646864 && !timescale) timescale = view.getUint32(index + (bytes[index + 4] === 1 ? 24 : 16));
    }
    for (let index = 0; index + 8 <= bytes.length;) {
      const size = view.getUint32(index); if (size < 8 || index + size > bytes.length) throw Error('Invalid recordedMP4 box');
      if (view.getUint32(index + 4) === 0x6d6f6f66) fragments.push(index);
      index += size;
    }
    if (avc < 0 || fragments.length < 3 || !timescale) throw Error('Chromium did not produce bounded fragmentedAVC');
    const segments = fragments.map((start, index) => bytes.slice(start, fragments[index + 1]));
    const starts = segments.map(segment => {
      const data = new DataView(segment.buffer);
      for (let i = 4; i < segment.length - 16; i++) if (data.getUint32(i) === 0x74666474) return segment[i + 4] === 1 ? Number(data.getBigUint64(i + 8)) / timescale : data.getUint32(i + 8) / timescale;
      throw Error('Recorded fragment has no decode timestamp');
    });
    function sampleDuration(segment) {
      const data = new DataView(segment.buffer);
      let defaultDuration = 0, trun = -1;
      for (let i = 4; i + 20 < segment.length; i++) {
        const type = data.getUint32(i);
        if (type === 0x6d646174) break; // mdat: only inspect structural boxes
        if (type === 0x74666864) { // tfhd
          const flags = data.getUint32(i + 4) & 0xffffff;
          let offset = i + 12;
          if (flags & 1) offset += 8;
          if (flags & 2) offset += 4;
          if (flags & 8) defaultDuration = data.getUint32(offset);
        }
        if (type === 0x7472756e) trun = i;
      }
      if (trun < 0) throw Error('Recorded fragment has no sample table');
      const flags = data.getUint32(trun + 4) & 0xffffff;
      const count = data.getUint32(trun + 8);
      let offset = trun + 12, duration = 0;
      if (flags & 1) offset += 4;
      if (flags & 4) offset += 4;
      for (let sample = 0; sample < count; sample++) {
        duration += flags & 0x100 ? data.getUint32(offset) : defaultDuration;
        if (flags & 0x100) offset += 4;
        if (flags & 0x200) offset += 4;
        if (flags & 0x400) offset += 4;
        if (flags & 0x800) offset += 4;
      }
      return duration / timescale;
    }
    const durations = starts.map((value, index) => index + 1 < starts.length ? starts[index + 1] - value : sampleDuration(segments[index]));
    if (durations.some(duration => duration <= 0 || duration > 6) || durations.reduce((a, b) => a + b, 0) > 20) throw Error('Recorded fragment duration outside pilot: ' + JSON.stringify({ width, timescale, starts, durations }));
    return { bytes, frames, codec: 'avc1.' + Array.from(bytes.slice(avc, avc + 3), value => value.toString(16).padStart(2, '0')).join(''), init: bytes.slice(0, fragments[0]), segments, durations };
  }
}
