import { timelineHlsPlayback } from "./timeline-hls-policies.js";

function expectsMediaKind(recovery, ...kinds) {
  return [recovery?.expectedKinds, recovery?.expected].some(expected =>
    Array.isArray(expected) && expected.some(kind => kinds.includes(kind)));
}

export function primaryMediaMissing(evidence, source) {
  const recovery = evidence?.mediaRecovery;
  if (evidence?.media?.length) return false;
  if (source !== "linkedin") return recovery?.outcome === "unavailable";
  const unavailable = recovery?.outcome === "unavailable" ||
    (recovery?.outcome === "observed_dom_only" && recovery.status === "missing_expected_url");
  return unavailable && expectsMediaKind(recovery, "image", "video", "document");
}

// Posters and blob metadata establish video presence, not usable playback.
// Rendering this policy never starts capture; recovery remains an explicit click.
export function videoPlaybackMissing(evidence, source, canonicalPlaybackUrl) {
  const media = Array.isArray(evidence?.media) ? evidence.media : [];
  if (media.some((value) => value?.kind === "video" && value.playbackMode === "inline"
    && (canonicalPlaybackUrl(value.playbackUrl, source) || timelineHlsPlayback(value, source)))) return false;
  return evidence?.contentKind === "video"
    || media.some((value) => value?.kind === "video" || value?.kind === "video_poster")
    || expectsMediaKind(evidence?.mediaRecovery, "video");
}
