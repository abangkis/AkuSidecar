// Posters and blob metadata establish video presence, not usable playback.
// Rendering this policy never starts capture; recovery remains an explicit click.
export function videoPlaybackMissing(evidence, source, canonicalPlaybackUrl) {
  const media = Array.isArray(evidence?.media) ? evidence.media : [];
  if (media.some((value) => value?.kind === "video" && value.playbackMode === "inline"
    && canonicalPlaybackUrl(value.playbackUrl, source))) return false;
  return evidence?.contentKind === "video"
    || media.some((value) => value?.kind === "video" || value?.kind === "video_poster")
    || (Array.isArray(evidence?.mediaRecovery?.expected) && evidence.mediaRecovery.expected.includes("video"));
}
