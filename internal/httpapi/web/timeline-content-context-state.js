export const CONTENT_CONTEXT_DEFAULT_LIMIT = 3;
export const CONTENT_CONTEXT_MIN_LIMIT = 1;
export const CONTENT_CONTEXT_MAX_LIMIT = 5;
export const CONTENT_CONTEXT_RAIL_MIN_GUTTER = 280;
export const CONTENT_CONTEXT_RAIL_MAX_WIDTH = 420;
export const CONTENT_CONTEXT_TAB_DEFAULT_WIDTH = 42;

export function backToTopBoundaryBottom({ lineY = 0, viewportHeight = 0, restBottom = 0 } = {}) {
  const line = Number(lineY);
  const viewport = Number(viewportHeight);
  const rest = Number(restBottom) || 0;
  if (!Number.isFinite(line) || !Number.isFinite(viewport)) return rest;
  return Math.max(rest, viewport - line);
}

export function contentContextTabFits({
  postRight = 0,
  boundaryLeft = 0,
  viewportWidth = 0,
  tabWidth = CONTENT_CONTEXT_TAB_DEFAULT_WIDTH,
  gap = 12,
  viewportPadding = 12,
} = {}) {
  const values = [postRight, boundaryLeft, viewportWidth, tabWidth, gap, viewportPadding]
    .map(Number);
  if (values.some((value) => !Number.isFinite(value))) return false;
  const [right, boundary, width, buttonWidth, spacing, padding] = values;
  const safeBoundary = Math.min(boundary, width - padding);
  return safeBoundary - right >= buttonWidth + spacing;
}

export function contentContextDrawerMode({ viewportWidth = 0, rightGutter = 0 } = {}) {
  const width = Number(viewportWidth) || 0;
  const gutter = Number(rightGutter) || 0;
  if (width <= 760) return "sheet";
  return gutter >= CONTENT_CONTEXT_RAIL_MIN_GUTTER ? "rail" : "overlay";
}

export function contentContextRailPlacement({
  postRight = 0,
  safeBoundary = 0,
  viewportWidth = 0,
  maxWidth = CONTENT_CONTEXT_RAIL_MAX_WIDTH,
} = {}) {
  const values = [postRight, safeBoundary, viewportWidth, maxWidth].map(Number);
  if (values.some((value) => !Number.isFinite(value))) return null;
  const [rightEdge, boundary, viewport, maximumWidth] = values;
  const availableWidth = Math.max(0, boundary - rightEdge);
  if (availableWidth < CONTENT_CONTEXT_RAIL_MIN_GUTTER) return null;
  const width = Math.min(Math.max(0, maximumWidth), availableWidth);
  return {
    width,
    right: Math.max(0, viewport - (rightEdge + width)),
  };
}

export function buildTimelineContentContextPath(id, limit = CONTENT_CONTEXT_DEFAULT_LIMIT) {
  const normalized = String(id ?? "").trim();
  if (!normalized) return "";
  const bounded = Math.min(CONTENT_CONTEXT_MAX_LIMIT, Math.max(CONTENT_CONTEXT_MIN_LIMIT, Number.parseInt(limit, 10) || CONTENT_CONTEXT_DEFAULT_LIMIT));
  return `/api/timeline/${encodeURIComponent(normalized)}/content-context?limit=${bounded}`;
}
