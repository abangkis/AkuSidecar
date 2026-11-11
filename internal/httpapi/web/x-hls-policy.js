const masterPath = /^\/(amplify_video|ext_tw_video|tweet_video)\/(\d{1,30})\/(?:pu\/)?pl\/[\w.-]+\.m3u8$/;

function trustedUrl(value, host, base) {
  if (typeof value !== "string" || !value || value.startsWith("#")) return null;
  try {
    const url = new URL(value, base);
    if (url.origin !== `https://${host}` || url.username || url.password || /%2f|%5c/i.test(url.pathname)) return null;
    url.hash = "";
    return url;
  } catch { return null; }
}

export function xTimelineHlsUrl(media, source) {
  if (source !== "x" || media?.kind !== "video" || media.playbackMode !== "inline") return null;
  const poster = trustedUrl(media.posterUrl || media.url, "pbs.twimg.com");
  const asset = /^\/(amplify_video|ext_tw_video|tweet_video)_thumb\/(\d{1,30})\//.exec(poster?.pathname ?? "");
  const playlist = trustedUrl(media.playbackUrl, "video.twimg.com");
  const master = masterPath.exec(playlist?.pathname ?? "");
  return asset && master && asset[1] === master[1] && asset[2] === master[2] ? playlist.href : null;
}

function attributes(line) {
  const fields = new Map();
  let rest = line.slice(line.indexOf(":") + 1);
  while (rest) {
    const match = /^([A-Z0-9-]+)=(?:"([^"\r\n]*)"|([^,\s"]+))(,|$)/.exec(rest);
    if (!match || fields.has(match[1])) throw new Error("Unsupported X HLS attributes.");
    fields.set(match[1], match[2] ?? match[3]);
    rest = rest.slice(match[0].length);
    if (match[4] && !rest) throw new Error("Unsupported X HLS attributes.");
  }
  return fields;
}

export function createXHlsPolicy(playlist) {
  const master = trustedUrl(playlist, "video.twimg.com");
  const match = masterPath.exec(master?.pathname ?? "");
  if (!match) throw new Error("Unsupported X HLS master.");
  const prefix = `/${match[1]}/${match[2]}/`;
  function resource(value, base, extension) {
    const url = trustedUrl(value, "video.twimg.com", base);
    if (!url?.pathname.startsWith(prefix) || !extension.test(url.pathname)) throw new Error("Unsupported X HLS resource.");
    return url.href;
  }
  return Object.freeze({
    authorizeRequest(context) {
      const type = context.type;
      if (!["manifest", "level", "audioTrack", "media-fragment"].includes(type)
        || context.rangeStart || context.rangeEnd || context.part) throw new Error("Unsupported X HLS request.");
      return resource(context.url, master.href, type === "media-fragment" ? /\.(?:mp4|m4s)$/i : /\.m3u8$/i);
    },
    validatePlaylist(text, context) {
      const lines = text.split(/\r?\n/).map(line => line.trim()).filter(Boolean);
      if (lines[0] !== "#EXTM3U" || lines.some(line => /^#EXT-X-(?:KEY|SESSION-KEY|BYTERANGE|DISCONTINUITY|GAP|PART|PRELOAD-HINT|SKIP|DEFINE|I-FRAME|CONTENT-STEERING|DATERANGE|SESSION-DATA|RENDITION-REPORT)(?::|-|$)/.test(line))) {
        throw new Error("Unsupported X HLS playlist.");
      }
      const streams = lines.filter(line => line.startsWith("#EXT-X-STREAM-INF:"));
      if (streams.length) {
        if (streams.length > 32) throw new Error("Unsupported X HLS variant count.");
        const renditions = lines.filter(line => line.startsWith("#EXT-X-MEDIA:")).map(attributes);
        for (const rendition of renditions) {
          if (rendition.get("TYPE") !== "AUDIO" || !rendition.get("URI")) throw new Error("Unsupported X HLS rendition.");
          resource(rendition.get("URI"), context.url, /\.m3u8$/i);
        }
        for (const line of streams) {
          const fields = attributes(line);
          if (fields.has("AUDIO") && !renditions.some(r => r.get("GROUP-ID") === fields.get("AUDIO"))) throw new Error("X HLS requires its matching audio rendition.");
          resource(lines[lines.indexOf(line) + 1], context.url, /\.m3u8$/i);
        }
        if (lines.filter(line => !line.startsWith("#")).length !== streams.length) throw new Error("Unsupported X HLS master.");
      } else {
        const maps = lines.filter(line => line.startsWith("#EXT-X-MAP:"));
        const init = /^#EXT-X-MAP:URI="([^"]+)"$/.exec(maps[0] ?? "")?.[1];
        const segments = lines.flatMap((line, index) => line.startsWith("#EXTINF:") ? [lines[index + 1]] : []);
        if (maps.length !== 1 || !init || !lines.includes("#EXT-X-ENDLIST") || !segments.length || segments.length > 240
          || lines.filter(line => !line.startsWith("#")).length !== segments.length) throw new Error("X HLS requires a bounded fMP4 media playlist.");
        resource(init, context.url, /\.mp4$/i);
        for (const segment of segments) resource(segment, context.url, /\.(?:m4s|mp4)$/i);
      }
    },
  });
}
