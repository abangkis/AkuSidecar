// Optional, source-independent UHD admission. Network trust remains with the
// source policy; this module only narrows already-authorized playlists.
export const timelineHlsUhdLimits = Object.freeze({ resource: 32 * 1024 * 1024, playback: 128 * 1024 * 1024 });

function attributes(line) {
  const result = new Map();
  let rest = line.slice(line.indexOf(":") + 1);
  while (rest) {
    const match = /^([A-Z0-9-]+)=(?:"([^"\r\n]*)"|([^,\s"]+))(,|$)/.exec(rest);
    if (!match || result.has(match[1])) throw Error("Invalid HLS UHD attributes.");
    result.set(match[1], match[2] ?? match[3]);
    rest = rest.slice(match[0].length);
    if (match[4] && !rest) throw Error("Invalid HLS UHD attributes.");
  }
  return result;
}

async function abortable(promise, signal) {
  signal.throwIfAborted();
  let abort;
  try {
    return await Promise.race([promise, new Promise((_, reject) => {
      abort = () => reject(signal.reason);
      signal.addEventListener("abort", abort, { once: true });
    })]);
  } finally { signal.removeEventListener("abort", abort); }
}

function mediaPlaylist(text, url, audio = false) {
  const lines = text.split(/\r?\n/).map(line => line.trim()).filter(Boolean);
  const maps = lines.filter(line => line.startsWith("#EXT-X-MAP:"));
  const map = /^#EXT-X-MAP:URI="([^"]+)"$/.exec(maps[0] ?? "")?.[1];
  if (lines[0] !== "#EXTM3U" || !lines.includes("#EXT-X-ENDLIST") || maps.length !== 1 || !map ||
      lines.some(line => /^#EXT-X-(?:KEY|BYTERANGE|DISCONTINUITY|GAP|PART)(?::|$)/.test(line))) throw Error("UHD requires bounded fMP4 VOD.");
  const segments = [];
  let duration = 0;
  // AAC timestamps can round one or two audio frames beyond the video boundary.
  const tolerance = audio ? 0.05 : 0;
  for (let index = 0; index < lines.length; index++) {
    if (!lines[index].startsWith("#EXTINF:")) continue;
    const match = /^#EXTINF:(\d+(?:\.\d+)?),[^\r\n]*$/.exec(lines[index]);
    const seconds = Number(match?.[1]);
    const uri = lines[index + 1];
    if (!(seconds > 0 && seconds <= 6 + tolerance) || !uri || uri.startsWith("#")) throw Error("UHD segment exceeds its duration bound.");
    duration += seconds;
    segments.push(new URL(uri, url).href);
  }
  if (!segments.length || segments.length > 240 || duration > 20 + tolerance + 1e-9 ||
      lines.filter(line => !line.startsWith("#")).length !== segments.length) throw Error("UHD video exceeds its duration bound.");
  return { segments, init: new URL(map, url).href };
}

export function createTimelineHlsUhdAdmission({ signal, getLevels = () => [], decodingInfo = config => globalThis.navigator?.mediaCapabilities?.decodingInfo(config), preflightMilliseconds = 5000 }) {
  const admitted = new Map();
  const verifiedBodies = new Map();
  return {
    async transformPlaylist(text, context, readPlaylist, loadSignal = signal) {
      signal.throwIfAborted();
      const lines = text.split(/\r?\n/).map(line => line.trim()).filter(Boolean);
      const streams = lines.flatMap((line, index) => line.startsWith("#EXT-X-STREAM-INF:") ? [{ line, index, fields: attributes(line) }] : []);
      if (!streams.length) {
        // The engine must receive the same bounded playlist that qualified a
        // level, even if its endpoint changes after the preflight read.
        const key = `${context.type}|${context.url}`;
        if (verifiedBodies.has(key) && verifiedBodies.get(key) !== text) throw Error("Qualified UHD playlist changed.");
        return text;
      }
      if (streams.length > 32) throw Error("Too many HLS variants.");
      const hasLowerFallback = streams.some(entry => {
        const [width, height] = (entry.fields.get("RESOLUTION") ?? "").split("x").map(Number);
        return width > 0 && height > 0 && width < 3840 && height < 2160;
      });
      const renditions = lines.filter(line => line.startsWith("#EXT-X-MEDIA:")).map(attributes);
      const excluded = new Set();
      const preflightSignal = AbortSignal.any([signal, loadSignal, AbortSignal.timeout(preflightMilliseconds)]);
      const reads = new Map();
      const read = context => {
        preflightSignal.throwIfAborted();
        const key = `${context.type}|${context.url}`;
        if (!reads.has(key)) reads.set(key, readPlaylist(context, preflightSignal));
        return reads.get(key);
      };
      let attempts = 0;
      for (const entry of streams) {
        signal.throwIfAborted();
        const resolution = entry.fields.get("RESOLUTION") ?? "";
        const [width, height] = resolution.split("x").map(Number);
        if (!(width >= 3840 || height >= 2160)) continue;
        excluded.add(entry.index); excluded.add(entry.index + 1);
        try {
          if (!hasLowerFallback || attempts >= 4 || preflightSignal.aborted) continue;
          const frameRate = Number(entry.fields.get("FRAME-RATE"));
          const bitrate = Number(entry.fields.get("BANDWIDTH"));
          const codecs = (entry.fields.get("CODECS") ?? "").split(",").map(codec => codec.trim());
          const videoCodec = codecs.find(codec => /^avc[13]\.[a-f\d]{6}$/i.test(codec));
          if (width !== 3840 || height !== 2160 || !/^\d+(?:\.\d+)?$/.test(entry.fields.get("FRAME-RATE") ?? "") ||
              !/^\d+$/.test(entry.fields.get("BANDWIDTH") ?? "") || !(frameRate > 0 && frameRate <= 30) ||
              entry.fields.get("VIDEO-RANGE") !== "SDR" || !(bitrate > 0 && bitrate <= 35_000_000) ||
              codecs.length !== 2 || !videoCodec || !codecs.includes("mp4a.40.2")) continue;
          attempts++;
          const capability = await abortable(Promise.resolve(decodingInfo({ type: "media-source",
            video: { contentType: `video/mp4; codecs="${videoCodec}"`, width, height, bitrate, framerate: frameRate },
            audio: { contentType: 'audio/mp4; codecs="mp4a.40.2"', channels: "2", bitrate: 128000, samplerate: 48000 },
          })), preflightSignal);
          if (capability?.supported !== true || capability?.smooth !== true) continue;
          const videoUrl = new URL(lines[entry.index + 1], context.url).href;
          const videoText = await read({ url: videoUrl, type: "level" });
          preflightSignal.throwIfAborted();
          const video = mediaPlaylist(videoText, videoUrl);
          const audioBodies = [];
          if (entry.fields.has("AUDIO")) {
            const audio = renditions.filter(r => r.get("TYPE") === "AUDIO" && r.get("GROUP-ID") === entry.fields.get("AUDIO"));
            if (audio.length !== 1 || !audio[0].get("URI")) continue;
            const audioUrl = new URL(audio[0].get("URI"), context.url).href;
            const audioText = await read({ url: audioUrl, type: "audioTrack" });
            preflightSignal.throwIfAborted();
            mediaPlaylist(audioText, audioUrl, true);
            audioBodies.push([audioUrl, audioText]);
          }
          preflightSignal.throwIfAborted();
          admitted.set(videoUrl, new Set(video.segments));
          verifiedBodies.set(`level|${videoUrl}`, videoText);
          for (const [url, body] of audioBodies) verifiedBodies.set(`audioTrack|${url}`, body);
          excluded.delete(entry.index); excluded.delete(entry.index + 1);
        } catch (error) {
          if (signal.aborted || loadSignal.aborted || error.sessionBudget) throw error;
          // An unsuitable UHD candidate cannot remove a valid lower rendition.
        }
      }
      if (streams.every(entry => excluded.has(entry.index))) throw Error("No supported bounded HLS variant.");
      return lines.filter((_, index) => !excluded.has(index)).join("\n");
    },
    resourceProfile(context) {
      signal.throwIfAborted();
      if (context.type !== "media-fragment" || context.frag?.type !== "main" || context.frag.sn === "initSegment") return null;
      const level = getLevels()[context.frag.level];
      if (level?.width !== 3840 || level?.height !== 2160) return null;
      const urls = Array.isArray(level?.url) ? level.url : [level?.url];
      return urls.some(url => admitted.get(url)?.has(context.url)) ? timelineHlsUhdLimits : null;
    },
  };
}

// hls.js's built-in size/FPS controllers can eagerly flush alternate audio on
// resize. Adjust only the future Auto ceiling here, preserving buffered media.
export function observeTimelineHlsDisplay(video, getLevels, setCap, { observerClass = globalThis.ResizeObserver, eventTarget = globalThis.window, documentTarget = globalThis.document, pixelRatio = () => globalThis.devicePixelRatio || 1 } = {}) {
  let disposed = false;
  let mediaQuery;
  const update = () => {
    if (disposed) return;
    const levels = getLevels();
    const rect = video.getBoundingClientRect?.();
    if (!levels.length || !rect?.width || !rect?.height) return;
    const observedRatio = Number(pixelRatio());
    const ratio = observedRatio > 0 ? observedRatio : 1;
    const width = rect.width * ratio, height = rect.height * ratio;
    const eligible = levels.map((level, index) => ({ level, index })).filter(({ level }) =>
      !(level.width >= 3840 || level.height >= 2160) || width >= level.width && height >= level.height);
    let cap = eligible.at(-1)?.index ?? 0;
    for (const { level, index } of eligible) {
      if (level.width >= width && level.height >= height) { cap = index; break; }
    }
    setCap(cap);
  };
  const onDprChange = () => { if (!disposed) { watchDpr(); update(); } };
  function watchDpr() {
    mediaQuery?.removeEventListener?.("change", onDprChange);
    mediaQuery = eventTarget?.matchMedia?.(`(resolution: ${pixelRatio()}dppx)`);
    mediaQuery?.addEventListener?.("change", onDprChange, { once: true });
  }
  const observer = observerClass ? new observerClass(update) : null;
  observer?.observe(video);
  eventTarget?.addEventListener?.("resize", update);
  documentTarget?.addEventListener?.("fullscreenchange", update);
  watchDpr();
  return { update, dispose() {
    disposed = true;
    observer?.disconnect();
    eventTarget?.removeEventListener?.("resize", update);
    documentTarget?.removeEventListener?.("fullscreenchange", update);
    mediaQuery?.removeEventListener?.("change", onDprChange);
  } };
}
