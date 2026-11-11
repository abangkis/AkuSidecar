// Capture bounded public CDN evidence, never credentials or a signed-in profile.
import { createHash } from "node:crypto";
import { mkdir, writeFile } from "node:fs/promises";
import { resolve, join } from "node:path";

const output = resolve(process.argv[2] ?? "runtime/qa/x-hls-capture");
const fixtures = resolve("test/fixtures/x-hls");
const sources = [
  ["derya", "https://x.com/DeryaTR_/status/2120382619082727604", "https://video.twimg.com/amplify_video/2120382215389351937/pl/YXyZk-zX1ENaHqfd.m3u8?tag=29"],
  ["matt", "https://x.com/mattshumer_/status/2120308139153326225", "https://video.twimg.com/amplify_video/2120307833774170112/pl/abhB60FGDNFrfXZG.m3u8?tag=29&v=cfc"],
  ["ananth", null, "https://video.twimg.com/amplify_video/2119481355943948288/pl/JbQ6Hrz22-kYlRom.m3u8?tag=29"],
];
await mkdir(output, { recursive: true });
await mkdir(fixtures, { recursive: true });
const metadata = [];
for (const [name, postUrl, masterUrl] of sources) {
  let remaining = 64 * 1024 * 1024;
  const resources = [];
  const asset = /^\/(amplify_video|ext_tw_video|tweet_video)\/\d+\//.exec(new URL(masterUrl).pathname)[0];
  async function read(value, base, playlist) {
    const url = new URL(value, base);
    if (url.origin !== "https://video.twimg.com" || !url.pathname.startsWith(asset) || url.username || url.password || /%2f|%5c/i.test(url.pathname)) throw Error("Untrusted resource");
    const response = await fetch(url, { credentials: "omit", referrerPolicy: "no-referrer", redirect: "error", signal: AbortSignal.timeout(15000) });
    if (!response.ok || !response.body) throw Error(`HTTP ${response.status}`);
    const chunks = [];
    let length = 0;
    for await (const chunk of response.body) {
      length += chunk.byteLength;
      remaining -= chunk.byteLength;
      if (length > (playlist ? 65536 : 8388608) || remaining < 0) throw Error("Capture byte limit");
      chunks.push(chunk);
    }
    const bytes = Buffer.concat(chunks);
    const record = { url: url.href, status: response.status, bytes: length, sha256: createHash("sha256").update(bytes).digest("hex"), body: bytes.toString("base64") };
    resources.push(record);
    return bytes.toString("utf8");
  }
  const master = await read(masterUrl, undefined, true);
  await writeFile(join(fixtures, `${name}-master.m3u8`), master);
  const lines = master.split(/\r?\n/);
  const index = lines.findIndex(line => line.startsWith("#EXT-X-STREAM-INF:"));
  const selected = [{ name: "video", uri: lines[index + 1] }];
  const group = /(?:[:,])AUDIO="([^"]+)"/.exec(lines[index])?.[1];
  if (group) {
    const audio = lines.find(line => line.startsWith("#EXT-X-MEDIA:") && line.includes(`GROUP-ID="${group}"`));
    selected.push({ name: "audio", uri: /URI="([^"]+)"/.exec(audio)[1] });
  }
  for (const track of selected) {
    const trackUrl = new URL(track.uri, masterUrl).href;
    const playlist = await read(trackUrl, undefined, true);
    await writeFile(join(fixtures, `${name}-${track.name}.m3u8`), playlist);
    if (name === "ananth") continue;
    const map = /#EXT-X-MAP:URI="([^"]+)"/.exec(playlist)?.[1];
    const segments = playlist.split(/\r?\n/).map(line => line.trim()).filter(line => line && !line.startsWith("#"));
    if (!map || !playlist.includes("#EXT-X-ENDLIST") || !segments.length || segments.length > 240) throw Error("Unbounded playlist");
    for (const uri of [map, ...segments]) await read(uri, trackUrl, false);
  }
  const capturedAt = new Date().toISOString();
  await writeFile(join(output, `${name}.json`), JSON.stringify({ name, postUrl, masterUrl, capturedAt, resources }));
  metadata.push({ name, postUrl, masterUrl, capturedAt, selected, resources: resources.filter(entry => entry.url.includes(".m3u8")).map(({ body, ...entry }) => entry) });
  console.log(`${name}: ${resources.length} resources, ${64 * 1024 * 1024 - remaining} bytes`);
}
await writeFile(join(fixtures, "capture-metadata.json"), JSON.stringify(metadata, null, 2) + "\n");
