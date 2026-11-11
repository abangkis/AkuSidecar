import { createXHlsPolicy, xTimelineHlsUrl } from "./x-hls-policy.js";

// Source admission belongs here; the player and loader know no source hosts.
export function timelineHlsPlayback(media, source) {
  const url = xTimelineHlsUrl(media, source);
  return url ? { url, policy: createXHlsPolicy(url) } : null;
}
