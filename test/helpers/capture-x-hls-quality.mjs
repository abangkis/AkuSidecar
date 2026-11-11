// Extend the existing public Matt capture with one bounded quality/audio pair.
// Only QA evidence is written; no application API or signed-in profile is used.
import { createHash } from "node:crypto";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";

const input = resolve(process.argv[2] ?? "runtime/qa/x-hls-split-20261111");
const output = resolve(process.argv[3] ?? "runtime/qa/timeline-hls-quality-20261111");
const capture = JSON.parse(await readFile(join(input, "matt.json"), "utf8"));
const master = Buffer.from(capture.resources[0].body, "base64").toString();
const lines = master.split(/\r?\n/);
const variants = lines.flatMap((line, index) => line.startsWith("#EXT-X-STREAM-INF:") ? [{ line, uri: lines[index + 1] }] : []);
const selected = variants.slice(0, 2);
const audio = [...new Set(selected.map(({ line }) => /AUDIO="([^"]+)"/.exec(line)[1]))].map(group => lines.find(line => line.startsWith("#EXT-X-MEDIA:") && line.includes(`GROUP-ID="${group}"`)));
const playlists = [...selected.map(entry => entry.uri), ...audio.map(line => /URI="([^"]+)"/.exec(line)[1])];
const prefix = /^\/amplify_video\/\d+\//.exec(new URL(capture.masterUrl).pathname)[0];
let remaining = 64 * 1024 * 1024 - capture.resources.reduce((sum, entry) => sum + entry.bytes, 0);
async function read(uri, base, isPlaylist) {
  const url = new URL(uri, base);
  if (url.origin !== "https://video.twimg.com" || url.username || url.password || !url.pathname.startsWith(prefix) || /%2f|%5c/i.test(url.pathname)) throw Error("Untrusted quality fixture URL");
  const cached = capture.resources.find(entry => entry.url === url.href);
  if (cached) return Buffer.from(cached.body, "base64").toString();
  const response = await fetch(url, { credentials: "omit", referrerPolicy: "no-referrer", redirect: "error", signal: AbortSignal.timeout(15000) });
  if (!response.ok || !response.body) throw Error(`HTTP ${response.status}`);
  const chunks = [];
  let length = 0;
  for await (const chunk of response.body) {
    length += chunk.byteLength;
    remaining -= chunk.byteLength;
    if (length > (isPlaylist ? 65536 : 8388608) || remaining < 0) throw Error("Quality fixture byte limit");
    chunks.push(chunk);
  }
  const bytes = Buffer.concat(chunks);
  capture.resources.push({ url: url.href, status: response.status, bytes: bytes.byteLength, sha256: createHash("sha256").update(bytes).digest("hex"), body: bytes.toString("base64") });
  return bytes.toString();
}
for (const uri of playlists) {
  const url = new URL(uri, capture.masterUrl).href;
  const playlist = await read(url, undefined, true);
  const map = /#EXT-X-MAP:URI="([^"]+)"/.exec(playlist)?.[1];
  const segments = playlist.split(/\r?\n/).map(line => line.trim()).filter(line => line && !line.startsWith("#"));
  if (!map || !playlist.includes("#EXT-X-ENDLIST") || !segments.length || segments.length > 240) throw Error("Unbounded quality playlist");
  for (const resource of [map, ...segments]) await read(resource, url, false);
}
capture.qualityFixture = {
  capturedAt: new Date().toISOString(),
  transformation: "Master restricted to the first two original variants and their exact audio groups; original master retained in resources. All child playlists and media bytes unchanged.",
  master: ["#EXTM3U", "#EXT-X-VERSION:6", "#EXT-X-INDEPENDENT-SEGMENTS", ...audio, ...selected.flatMap(({ line, uri }) => [line, uri])].join("\n") + "\n",
};
await mkdir(output, { recursive: true });
await writeFile(join(output, "matt-two-level.json"), JSON.stringify(capture));
console.log(`Two-level Matt capture: ${capture.resources.length} resources, ${64 * 1024 * 1024 - remaining} aggregate bytes.`);
