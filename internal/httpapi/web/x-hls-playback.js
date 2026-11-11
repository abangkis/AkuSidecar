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

async function fetchBytes(url, limit, signal, budget) {
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
      if (value.byteLength > budget.remaining) throw new Error("X HLS playback exceeds its byte limit.");
      budget.remaining -= value.byteLength;
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

async function readPlaylist(url, signal, budget) {
  const bytes = await fetchBytes(url, playlistBytes, signal, budget);
  const lines = new TextDecoder().decode(bytes).split(/\r?\n/).map(line => line.trim()).filter(Boolean);
  if (lines[0] !== "#EXTM3U" || lines.some(line => /^#EXT-X-(?:KEY|SESSION-KEY|BYTERANGE|DISCONTINUITY|GAP|PART|SKIP|DEFINE|I-FRAMES-ONLY)(?::|$)/.test(line))) {
    throw new Error("Unsupported X HLS playlist.");
  }
  return lines;
}

function attributes(line) {
  const result = new Map();
  let rest = line.slice(line.indexOf(":") + 1);
  while (rest) {
    const match = /^([A-Z0-9-]+)=(?:"([^"\r\n]*)"|([^,\s"]+))(,|$)/.exec(rest);
    if (!match || result.has(match[1])) throw new Error("Unsupported X HLS attributes.");
    result.set(match[1], match[2] ?? match[3]);
    rest = rest.slice(match[0].length);
    if (match[4] && !rest) throw new Error("Unsupported X HLS attributes.");
  }
  return result;
}

function selectVariant(lines, playlist, assetPath) {
  for (let index = 0; index < lines.length; index++) {
    const line = lines[index];
    if (!line.startsWith("#EXT-X-STREAM-INF:")) continue;
    const fields = attributes(line);
    const codecs = fields.get("CODECS");
    if (!codecs || !/^[\w., -]{1,100}$/.test(codecs)) continue;
    if (fields.has("AUDIO")) {
      const codecList = codecs.split(",").map(codec => codec.trim());
      const videoCodecs = codecList.filter(codec => /^avc[13]\.[a-f\d]{6}$/i.test(codec));
      const audioCodecs = codecList.filter(codec => /^mp4a\.40\.2$/.test(codec));
      if (codecList.length !== 2 || videoCodecs.length !== 1 || audioCodecs.length !== 1) continue;
      const videoMime = `video/mp4; codecs="${videoCodecs[0]}"`;
      const audioMime = `audio/mp4; codecs="${audioCodecs[0]}"`;
      if (!MediaSource.isTypeSupported(videoMime) || !MediaSource.isTypeSupported(audioMime)) continue;
      const renditions = lines.filter(entry => entry.startsWith("#EXT-X-MEDIA:"))
        .map(attributes).filter(entry => entry.get("TYPE") === "AUDIO" && entry.get("GROUP-ID") === fields.get("AUDIO"));
      const audio = renditions.find(entry => entry.get("DEFAULT") === "YES") ??
        renditions.find(entry => entry.get("AUTOSELECT") === "YES") ?? renditions[0];
      if (!audio?.get("URI")) throw new Error("X HLS requires its matching audio rendition.");
      return [
        { mime: videoMime, url: assetResource(lines[index + 1], playlist, assetPath, /\.m3u8$/i) },
        { mime: audioMime, url: assetResource(audio.get("URI"), playlist, assetPath, /\.m3u8$/i) },
      ];
    }
    const mime = `video/mp4; codecs="${codecs}"`;
    if (MediaSource.isTypeSupported(mime)) {
      const url = assetResource(lines[index + 1], playlist, assetPath, /\.m3u8$/i);
      return [{ mime, url }];
    }
  }
  throw new Error("X HLS requires compatible fMP4 tracks.");
}

function mediaResources(lines, playlist, assetPath) {
  const maps = lines.filter(line => line.startsWith("#EXT-X-MAP:"));
  const init = /^#EXT-X-MAP:URI="([^"]+)"$/.exec(maps[0] ?? "")?.[1];
  const segments = lines.flatMap((line, index) => line.startsWith("#EXTINF:") ? [lines[index + 1]] : []);
  if (maps.length !== 1 || !init || !lines.includes("#EXT-X-ENDLIST") ||
      segments.length === 0 || segments.length > maxSegments ||
      lines.filter(line => !line.startsWith("#")).length !== segments.length) {
    throw new Error("X HLS requires a bounded fMP4 media playlist.");
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
    const budget = { remaining: playbackBytes };
    const master = await readPlaylist(playlist, signal, budget);
    const tracks = selectVariant(master, playlist, assetPath);
    // Validate both playlists before appending either track. MP4 presentation
    // timestamps share this MediaSource clock; do not reset either track's offset.
    for (const track of tracks) {
      track.urls = mediaResources(await readPlaylist(track.url, signal, budget), track.url, assetPath);
      track.buffer = media.addSourceBuffer(track.mime);
    }
    for (let index = 0; tracks.some(track => index < track.urls.length); index++) {
      for (const track of tracks) {
        if (index >= track.urls.length) continue;
        const bytes = await fetchBytes(track.urls[index], segmentBytes, signal, budget);
        await waitForEvent(track.buffer, "updateend", signal, () => track.buffer.appendBuffer(bytes));
      }
    }
    media.endOfStream();
  }).catch(error => {
    // A later audio failure must terminate an already-started video track too.
    controller.abort(error);
    if (media.readyState === "open") {
      try { media.endOfStream("decode"); } catch { /* disposal may have detached it */ }
    }
    throw error;
  });
  return {
    finished,
    dispose() {
      controller.abort();
      URL.revokeObjectURL(objectUrl);
    },
  };
}
