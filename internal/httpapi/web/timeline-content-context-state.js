export const CONTENT_CONTEXT_DEFAULT_LIMIT = 3;
export const CONTENT_CONTEXT_MIN_LIMIT = 1;
export const CONTENT_CONTEXT_MAX_LIMIT = 5;
export const CONTENT_CONTEXT_RAIL_MIN_GUTTER = 320;

export function contentContextDrawerMode({ viewportWidth = 0, rightGutter = 0 } = {}) {
  const width = Number(viewportWidth) || 0;
  const gutter = Number(rightGutter) || 0;
  if (width <= 760) return "sheet";
  return gutter >= CONTENT_CONTEXT_RAIL_MIN_GUTTER ? "rail" : "overlay";
}

export function buildTimelineContentContextPath(id, limit = CONTENT_CONTEXT_DEFAULT_LIMIT) {
  const normalized = String(id ?? "").trim();
  if (!normalized) return "";
  const bounded = Math.min(CONTENT_CONTEXT_MAX_LIMIT, Math.max(CONTENT_CONTEXT_MIN_LIMIT, Number.parseInt(limit, 10) || CONTENT_CONTEXT_DEFAULT_LIMIT));
  return `/api/timeline/${encodeURIComponent(normalized)}/content-context?limit=${bounded}`;
}
