import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { attachXHls } from "../internal/httpapi/web/x-hls-playback.js";

const asset = "https://video.twimg.com/amplify_video/2120382215389351937/";
const masterUrl = asset + "pl/master.m3u8";
const master = '#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="wrong",URI="wrong.m3u8"\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",DEFAULT=YES,URI="audio.m3u8"\n#EXT-X-STREAM-INF:BANDWIDTH=100000,CODECS="mp4a.40.2,avc1.42001f",AUDIO="audio"\nvideo.m3u8';
const playlist = (track, count = 1) => ['#EXTM3U', `#EXT-X-MAP:URI="../vid/${track}-init.mp4"`, ...Array.from({ length: count }, (_, i) => [`#EXTINF:1,`, `../vid/${track}-${i}.m4s`]).flat(), '#EXT-X-ENDLIST'].join('\n');

async function playback(t, options = {}) {
  const requests = [], appends = [], ended = [], buffers = [];
  let media;
  class SourceBuffer extends EventTarget {
    constructor(mime) { super(); this.mime = mime; }
    appendBuffer(bytes) {
      appends.push({ mime: this.mime, bytes: bytes.byteLength });
      queueMicrotask(() => this.dispatchEvent(new Event("updateend")));
    }
  }
  class MediaSource extends EventTarget {
    static isTypeSupported(mime) { return !options.unsupported?.test(mime); }
    constructor() { super(); media = this; this.readyState = "open"; }
    addSourceBuffer(mime) { const buffer = new SourceBuffer(mime); buffers.push(buffer); return buffer; }
    endOfStream(error) { ended.push(error ?? "success"); this.readyState = "ended"; }
  }
  t.mock.method(URL, "createObjectURL", () => {
    queueMicrotask(() => media.dispatchEvent(new Event("sourceopen")));
    return "blob:fixture";
  });
  t.mock.method(URL, "revokeObjectURL", () => {});
  const previous = globalThis.MediaSource;
  globalThis.MediaSource = MediaSource;
  t.after(() => { globalThis.MediaSource = previous; });
  t.mock.method(globalThis, "fetch", async (url, config) => {
    requests.push(url);
    assert.equal(config.credentials, "omit");
    assert.equal(config.referrerPolicy, "no-referrer");
    assert.equal(config.redirect, "error");
    if (options.resources?.has(url)) return new Response(options.resources.get(url));
    if (options.resources) {
      assert.match(url, /\/(?:vid|aud)\/[^?#]+\.(?:mp4|m4s)$/);
      return new Response(new Uint8Array(1));
    }
    if (url === masterUrl) return new Response(options.master ?? master);
    if (url === asset + "pl/video.m3u8") return new Response(options.video ?? playlist("video", options.count));
    if (url === asset + "pl/audio.m3u8") return new Response(options.audio ?? playlist("audio", options.count));
    if (options.failAudio && url.endsWith("audio-0.m4s")) throw Error("Audio unavailable");
    if (options.blockAudio && url.endsWith("audio-init.mp4")) {
      return new Promise((_, reject) => config.signal.addEventListener("abort", () => reject(config.signal.reason), { once: true }));
    }
    assert.match(url, /\/vid\/(?:audio|video)-(?:init\.mp4|\d+\.m4s)$/);
    return new Response(new Uint8Array(options.resourceBytes ?? 1));
  });
  const session = attachXHls({}, options.masterUrl ?? masterUrl);
  return { session, requests, appends, ended, buffers };
}

test("separate audio uses the exact group, independent supported buffers and interleaved timestamps", async t => {
  const result = await playback(t);
  await result.session.finished;
  assert.deepEqual(result.buffers.map(buffer => buffer.mime), ['video/mp4; codecs="avc1.42001f"', 'audio/mp4; codecs="mp4a.40.2"']);
  assert.deepEqual(result.requests.slice(0, 3), [masterUrl, asset + "pl/video.m3u8", asset + "pl/audio.m3u8"]);
  assert.deepEqual(result.requests.slice(3).map(url => url.split("/").pop()), ["video-init.mp4", "audio-init.mp4", "video-0.m4s", "audio-0.m4s"]);
  assert.deepEqual(result.ended, ["success"]);
});

for (const [name, options, message] of [
  ["missing exact audio group", { master: master.replace('AUDIO="audio"', 'AUDIO="missing"') }, /matching audio/],
  ["foreign audio playlist", { master: master.replace('URI="audio.m3u8"', 'URI="../../999/pl/audio.m3u8"') }, /resource/],
  ["foreign audio segment", { audio: playlist("audio").replace('../vid/audio-0.m4s', '../../999/vid/audio.m4s') }, /resource/],
  ["unsupported audio", { unsupported: /^audio\// }, /compatible/],
  ["audio fails after video append", { failAudio: true }, /Audio unavailable/],
  ["audio segment cap", { audio: playlist("audio", 241) }, /bounded/],
  ["audio playlist size", { audio: "#EXTM3U\n" + "#".repeat(65536) }, /byte limit/],
  ["per-resource size", { resourceBytes: 8388609 }, /byte limit/],
  ["shared audio and video byte budget", { resourceBytes: 8 * 1024 * 1024, count: 4 }, /byte limit/],
]) {
  test(`rejects ${name} without completing a silent video-only stream`, async t => {
    const result = await playback(t, options);
    await assert.rejects(result.session.finished, message);
    assert.deepEqual(result.ended, ["decode"]);
    if (options.failAudio) assert.equal(result.appends.length, 3);
    if (name.includes("foreign") || name.includes("group")) assert.equal(result.appends.length, 0);
    assert.ok(result.requests.every(url => !url.includes("/999/")));
  });
}

test("dispose aborts a blocked audio fetch after video starts", async t => {
  const result = await playback(t, { blockAudio: true });
  while (result.requests.length < 5) await new Promise(resolve => setImmediate(resolve));
  result.session.dispose();
  await assert.rejects(result.session.finished, { name: "AbortError" });
  assert.equal(result.appends.length, 1);
  assert.ok(!result.ended.includes("success"));
});

test("all audio segments finish before endOfStream even when video finishes first", async t => {
  const result = await playback(t, { audio: playlist("audio", 3) });
  await result.session.finished;
  assert.equal(result.requests.at(-1), asset + "vid/audio-2.m4s");
  assert.equal(result.appends.length, 6);
  assert.deepEqual(result.ended, ["success"]);
});

for (const name of ["derya", "matt", "ananth"]) {
  test(`captured ${name} manifests retain immutable hashes and select their supported tracks`, async t => {
    const metadata = JSON.parse(await readFile(new URL("./fixtures/x-hls/capture-metadata.json", import.meta.url), "utf8"));
    const capture = metadata.find(entry => entry.name === name);
    const resources = new Map();
    for (let i = 0; i < capture.resources.length; i++) {
      const record = capture.resources[i];
      const kind = i === 0 ? "master" : capture.selected[i - 1].name;
      const bytes = await readFile(new URL(`./fixtures/x-hls/${name}-${kind}.m3u8`, import.meta.url));
      assert.equal(bytes.byteLength, record.bytes);
      assert.equal(createHash("sha256").update(bytes).digest("hex"), record.sha256);
      resources.set(record.url, bytes);
    }
    const text = resources.get(capture.masterUrl).toString();
    const streams = text.split(/\r?\n/).filter(line => line.startsWith("#EXT-X-STREAM-INF:"));
    if (name !== "ananth") {
      assert.match(text, /#EXT-X-MEDIA:[^\n]*TYPE=AUDIO/);
      assert.equal(streams.length, name === "derya" ? 4 : 5);
      assert.ok(streams.every(line => /AUDIO="[^"]+"/.test(line)));
    } else assert.ok(streams.every(line => !/AUDIO=/.test(line)));
    const result = await playback(t, { masterUrl: capture.masterUrl, resources });
    await result.session.finished;
    assert.equal(result.buffers.length, name === "ananth" ? 1 : 2);
    assert.deepEqual(result.requests.slice(0, capture.resources.length), capture.resources.map(entry => entry.url));
    assert.deepEqual(result.ended, ["success"]);
  });
}
