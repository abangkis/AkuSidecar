import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { createXHlsPolicy, xTimelineHlsUrl } from "../internal/httpapi/web/x-hls-policy.js";
import { timelineHlsPlayback } from "../internal/httpapi/web/timeline-hls-policies.js";

const asset = "https://video.twimg.com/amplify_video/2120382215389351937/";
const url = asset + "pl/master.m3u8";
const policy = createXHlsPolicy(url);
const master = '#EXTM3U\n#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="audio",DEFAULT=YES,URI="audio.m3u8"\n#EXT-X-STREAM-INF:BANDWIDTH=100000,CODECS="mp4a.40.2,avc1.42001f",AUDIO="audio"\nvideo.m3u8';
const playlist = (count = 1) => ['#EXTM3U', '#EXT-X-MAP:URI="../vid/init.mp4"', ...Array.from({ length: count }, (_, i) => ['#EXTINF:1,', `../vid/${i}.m4s`]).flat(), '#EXT-X-ENDLIST'].join('\n');

test("X admission matches poster asset and dispatches a policy without exposing X in the player", () => {
  const media = { kind: "video", playbackMode: "inline", posterUrl: "https://pbs.twimg.com/amplify_video_thumb/2120382215389351937/img/image.jpg", playbackUrl: url };
  assert.equal(xTimelineHlsUrl(media, "x"), url);
  assert.equal(timelineHlsPlayback(media, "x").url, url);
  assert.equal(timelineHlsPlayback(media, "linkedin"), null);
  assert.equal(xTimelineHlsUrl({ ...media, playbackUrl: url.replace("2120382215389351937", "999") }, "x"), null);
});

test("X policy admits complete split audio master and bounded media playlists", () => {
  policy.validatePlaylist(master, { url, type: "manifest" });
  policy.validatePlaylist(playlist(), { url: asset + "pl/audio.m3u8", type: "audioTrack" });
  assert.equal(policy.authorizeRequest({ url: asset + "vid/1.m4s", type: "media-fragment", rangeStart: 0, rangeEnd: 0, part: null }), asset + "vid/1.m4s");
});

for (const [name, text, expected] of [
  ["missing matching audio", master.replace('AUDIO="audio"', 'AUDIO="missing"'), /matching audio/],
  ["foreign audio playlist", master.replace('URI="audio.m3u8"', 'URI="../../999/pl/audio.m3u8"'), /resource/],
  ["foreign audio segment", playlist().replace('../vid/0.m4s', '../../999/vid/audio.m4s'), /resource/],
  ["too many segments", playlist(241), /bounded/],
  ["live playlist", playlist().replace('#EXT-X-ENDLIST', ''), /bounded/],
  ["encrypted playlist", playlist() + '\n#EXT-X-KEY:METHOD=AES-128,URI="key"', /Unsupported/],
  ["steering", master + '\n#EXT-X-CONTENT-STEERING:SERVER-URI="https://evil.example/a"', /Unsupported/],
  ["byterange", playlist() + '\n#EXT-X-BYTERANGE:10@0', /Unsupported/],
]) {
  test(`X policy rejects ${name} before HLS sees the playlist`, () => assert.throws(() => policy.validatePlaylist(text, { url }), expected));
}

for (const context of [
  { url: asset + "vid/1.m4s", type: "key" },
  { url: asset + "vid/1.m4s", type: "unknown" },
  { url: asset + "vid/1.m4s", type: "media-fragment", rangeEnd: 5 },
  { url: asset + "vid/1.m4s", type: "media-fragment", part: {} },
  { url: "https://video.twimg.com/amplify_video/999/vid/1.m4s", type: "media-fragment" },
  { url: "https://evil.example/vid/1.m4s", type: "media-fragment" },
  { url: asset + "vid/%2f1.m4s", type: "media-fragment" },
  { url: asset.replace("https://", "https://user:pass@") + "vid/1.m4s", type: "media-fragment" },
]) {
  test(`X policy rejects unauthorized resource ${JSON.stringify(context)}`, () => assert.throws(() => policy.authorizeRequest(context), /Unsupported/));
}

for (const name of ["derya", "matt", "ananth"]) {
  test(`captured ${name} manifests keep immutable hashes and satisfy the source policy`, async () => {
    const metadata = JSON.parse(await readFile(new URL("./fixtures/x-hls/capture-metadata.json", import.meta.url), "utf8"));
    const capture = metadata.find(entry => entry.name === name);
    const capturedPolicy = createXHlsPolicy(capture.masterUrl);
    for (let i = 0; i < capture.resources.length; i++) {
      const record = capture.resources[i];
      const kind = i === 0 ? "master" : capture.selected[i - 1].name;
      const bytes = await readFile(new URL(`./fixtures/x-hls/${name}-${kind}.m3u8`, import.meta.url));
      assert.equal(bytes.byteLength, record.bytes);
      assert.equal(createHash("sha256").update(bytes).digest("hex"), record.sha256);
      capturedPolicy.validatePlaylist(bytes.toString(), { url: record.url });
    }
  });
}
