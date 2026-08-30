export const CONTENT_CONTEXT_DEFAULT_LIMIT = 3;
export const CONTENT_CONTEXT_MIN_LIMIT = 1;
export const CONTENT_CONTEXT_MAX_LIMIT = 5;
export const CONTENT_CONTEXT_RAIL_MIN_GUTTER = 320;
export const CONTENT_CONTEXT_TAB_DEFAULT_WIDTH = 42;
export const CONTENT_CONTEXT_TAB_DEFAULT_HEIGHT = 126;

export function backToTopBoundaryBottom({ lineY = 0, viewportHeight = 0, restBottom = 0 } = {}) {
  const line = Number(lineY);
  const viewport = Number(viewportHeight);
  const rest = Number(restBottom) || 0;
  if (!Number.isFinite(line) || !Number.isFinite(viewport)) return rest;
  return Math.max(rest, viewport - line);
}

export function contentContextTabPlacement({
  postRight = 0,
  boundaryLeft = 0,
  viewportWidth = 0,
  postTop = 0,
  viewportHeight = 0,
  tabWidth = CONTENT_CONTEXT_TAB_DEFAULT_WIDTH,
  tabHeight = CONTENT_CONTEXT_TAB_DEFAULT_HEIGHT,
  gap = 12,
  viewportPadding = 12,
} = {}) {
  const values = [postRight, boundaryLeft, viewportWidth, postTop, viewportHeight, tabWidth, tabHeight, gap, viewportPadding]
    .map(Number);
  if (values.some((value) => !Number.isFinite(value))) return null;
  const [right, boundary, width, top, height, buttonWidth, buttonHeight, spacing, padding] = values;
  const safeBoundary = Math.min(boundary, width - padding);
  if (safeBoundary - right < buttonWidth + spacing) return null;
  const minimumTop = padding + buttonHeight / 2;
  const maximumTop = Math.max(minimumTop, viewportHeight - padding - buttonHeight / 2);
  // CSS centers the transformed tab on its `top`; offset by half its height
  // so the tab's visible top edge starts at the post's upper edge.
  const preferredTop = top + buttonHeight / 2;
  return {
    right: Math.max(padding, width - (right + spacing + buttonWidth)),
    top: Math.min(maximumTop, Math.max(minimumTop, preferredTop)),
    availableWidth: safeBoundary - right,
  };
}

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
