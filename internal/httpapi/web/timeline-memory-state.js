export function buildTimelineKeepPath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/timeline/${encodeURIComponent(normalized)}/keep-full-copy` : "";
}

export function timelineKeepConfirmation(item = {}) {
  const title = String(item?.item?.whatChanged ?? item?.item?.whyItMatters ?? "this Timeline item").trim() || "this Timeline item";
  return `Keep the source text for “${title}” as a full copy on this device? This stores the bounded text locally until you release or remove it.`;
}
