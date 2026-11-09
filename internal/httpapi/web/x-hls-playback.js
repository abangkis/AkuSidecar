const playlistBytes = 64 * 1024;
const segmentBytes = 8 * 1024 * 1024;
const playbackBytes = 64 * 1024 * 1024;
const maxSegments = 240;
const xMasterPath = /^\/(amplify_video|ext_tw_video|tweet_video)\/(\d{1,30})\/(?:pu\/)?pl\/[\w.-]+\.m3u8$/;

function trustedUrl(value, host, base) {
  if (typeof value !== "string" || !value || value.startsWith("#")) return null;
  try {
    const url = new URL(value, base);
    if (url.origin !== `https://${host}` || url.username || url.password || /%2f|%5c/i.test(url.pathname)) return null;
    url.hash = "";
    return url;
  } catch {
    return null;
  }
}

export function xTimelineHlsUrl(media, source) {
  if (source !== "x" || media?.kind !== "video" || media.playbackMode !== "inline") return null;
  const poster = trustedUrl(media.posterUrl || media.url, "pbs.twimg.com");
  const asset = /^\/(amplify_video|ext_tw_video|tweet_video)_thumb\/(\d{1,30})\//.exec(poster?.pathname ?? "");
  const playlist = trustedUrl(media.playbackUrl, "video.twimg.com");
  const master = xMasterPath.exec(playlist?.pathname ?? "");
  return asset && master && asset[1] === master[1] && asset[2] === master[2] ? playlist.href : null;
}

function assetResource(value, playlist, assetPath, extension) {
  const url = trustedUrl(value, "video.twimg.com", playlist);
  if (!assetPath || !url?.pathname.startsWith(assetPath) || !extension.test(url.pathname)) {
    throw new Error("Unsupported X HLS resource.");
  }
  return url.href;
}

async function fetchBytes(url, limit, signal) {
  const response = await fetch(url, {
    signal: AbortSignal.any([signal, AbortSignal.timeout(15_000)]),
    credentials: "omit",
    referrerPolicy: "no-referrer",
    redirect: "error",
  });
  if (!response.ok || !response.body) throw new Error("X HLS resource unavailable.");
  const reader = response.body.getReader();
  const chunks = [];
  let size = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      size += value.byteLength;
      if (size > limit) throw new Error("X HLS resource exceeds its byte limit.");
      chunks.push(value);
    }
  } finally {
    await reader.cancel().catch(() => {});
    reader.releaseLock();
  }
  const bytes = new Uint8Array(size);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}

async function readPlaylist(url, signal) {
  const bytes = await fetchBytes(url, playlistBytes, signal);
  const lines = new TextDecoder().decode(bytes).split(/\r?\n/).map(line => line.trim()).filter(Boolean);
  if (lines[0] !== "#EXTM3U" || lines.some(line => /^#EXT-X-(?:KEY|SESSION-KEY|BYTERANGE|DISCONTINUITY|GAP|PART|SKIP|DEFINE|I-FRAMES-ONLY)(?::|$)/.test(line))) {
    throw new Error("Unsupported X HLS playlist.");
  }
  return lines;
}

function selectVariant(lines, playlist, assetPath) {
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    if (!line.startsWith("#EXT-X-STREAM-INF:") || /(?:[:,])AUDIO=/.test(line)) continue;
    const codecs = /(?:[:,])CODECS="([\w., -]{1,100})"/.exec(line)?.[1];
    const mime = `video/mp4; codecs="${codecs}"`;
    if (codecs && MediaSource.isTypeSupported(mime)) {
      const url = assetResource(lines[index + 1], playlist, assetPath, /\.m3u8$/i);
      return { mime, url };
    }
  }
  throw new Error("X HLS requires a compatible fMP4 variant without a separate audio playlist.");
}

function mediaResources(lines, playlist, assetPath) {
  const maps = lines.filter(line => line.startsWith("#EXT-X-MAP:"));
  const init = /^#EXT-X-MAP:URI="([^"]+)"$/.exec(maps[0] ?? "")?.[1];
  const segments = lines.flatMap((line, index) => line.startsWith("#EXTINF:") ? [lines[index + 1]] : []);
  if (maps.length !== 1 || !init || !lines.includes("#EXT-X-ENDLIST") ||
      segments.length === 0 || segments.length > maxSegments ||
      lines.filter(line => !line.startsWith("#")).length !== segments.length) {
    throw new Error("X HLS requires a bounded fMP4 video playlist.");
  }
  return [
    assetResource(init, playlist, assetPath, /\.mp4$/i),
    ...segments.map(uri => assetResource(uri, playlist, assetPath, /\.(?:m4s|mp4)$/i)),
  ];
}

function waitForEvent(target, eventName, signal, start = () => {}) {
  return new Promise((resolve, reject) => {
    const finish = error => {
      target.removeEventListener(eventName, success);
      target.removeEventListener("error", failure);
      signal.removeEventListener("abort", abort);
      if (error) reject(error);
      else resolve();
    };
    const success = () => finish();
    const failure = () => finish(new Error("X HLS media could not be decoded."));
    const abort = () => finish(signal.reason);
    if (signal.aborted) return abort();
    target.addEventListener(eventName, success, { once: true });
    target.addEventListener("error", failure, { once: true });
    signal.addEventListener("abort", abort, { once: true });
    try { start(); } catch (error) { finish(error); }
  });
}

export function attachXHls(video, playlist) {
  const assetPath = /^\/(?:amplify_video|ext_tw_video|tweet_video)\/\d+\//.exec(new URL(playlist).pathname)?.[0];
  assetResource(playlist, undefined, assetPath, /\.m3u8$/i);
  const controller = new AbortController();
  const { signal } = controller;
  const media = new MediaSource();
  const objectUrl = URL.createObjectURL(media);
  const ready = waitForEvent(media, "sourceopen", signal, () => { video.src = objectUrl; });
  const finished = ready.then(async () => {
    const master = await readPlaylist(playlist, signal);
    const variant = selectVariant(master, playlist, assetPath);
    const urls = mediaResources(await readPlaylist(variant.url, signal), variant.url, assetPath);
    const buffer = media.addSourceBuffer(variant.mime);
    let remaining = playbackBytes;
    for (const url of urls) {
      const bytes = await fetchBytes(url, Math.min(segmentBytes, remaining), signal);
      remaining -= bytes.byteLength;
      await waitForEvent(buffer, "updateend", signal, () => buffer.appendBuffer(bytes));
    }
    media.endOfStream();
  });
  return {
    finished,
    dispose() {
      controller.abort();
      URL.revokeObjectURL(objectUrl);
    },
  };
}
