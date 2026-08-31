export const CONTENT_CONTEXT_DEFAULT_LIMIT = 3;
export const CONTENT_CONTEXT_MIN_LIMIT = 1;
export const CONTENT_CONTEXT_MAX_LIMIT = 5;
export const CONTENT_CONTEXT_RAIL_MIN_GUTTER = 280;
export const CONTENT_CONTEXT_RAIL_MAX_WIDTH = 420;
export const CONTENT_CONTEXT_TAB_DEFAULT_WIDTH = 42;
export const CONTENT_CONTEXT_READING_EXIT_RATIO = 0.2;
export const CONTENT_CONTEXT_TRIGGER_ENTRY_RATIO = 0.8;
export const CONTENT_CONTEXT_TRIGGER_HYSTERESIS = 16;
export const CONTENT_CONTEXT_TRIGGER_MIN_DRAWER_HEIGHT = 320;
export const CONTENT_CONTEXT_UP_SCROLL_MODE_CLOSE_OFFSCREEN = "close_offscreen";
export const CONTENT_CONTEXT_UP_SCROLL_MODE_PRESERVE = "preserve";
export const CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT = CONTENT_CONTEXT_UP_SCROLL_MODE_CLOSE_OFFSCREEN;

export function normalizeContentContextUpScrollMode(value) {
  return value === CONTENT_CONTEXT_UP_SCROLL_MODE_PRESERVE
    ? CONTENT_CONTEXT_UP_SCROLL_MODE_PRESERVE
    : CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT;
}

export function contentContextReadingExitLine({ viewportHeight = 0 } = {}) {
  const height = Number(viewportHeight);
  if (!Number.isFinite(height) || height <= 0) return Number.NaN;
  return height * CONTENT_CONTEXT_READING_EXIT_RATIO;
}

export function contentContextPostPassedReadingExitLine({ postBottom = 0, viewportHeight = 0 } = {}) {
  const bottom = Number(postBottom);
  const line = contentContextReadingExitLine({ viewportHeight });
  if (!Number.isFinite(bottom) || !Number.isFinite(line)) return false;
  return bottom <= line;
}

export function contentContextPostPassedViewportBottom({ postTop = 0, viewportHeight = 0 } = {}) {
  const top = Number(postTop);
  const viewport = Number(viewportHeight);
  if (!Number.isFinite(top) || !Number.isFinite(viewport) || viewport <= 0) return false;
  return top >= viewport;
}

// Only one rendered post owns the viewport-scoped trigger at a time. Prefer
// the post crossing the reading line, then the next substantially visible
// post. A small hysteresis band keeps the trigger from flickering at a card
// boundary while the user makes fine scroll adjustments.
export function selectContentContextViewportID({
  candidates = [],
  viewportHeight = 0,
  previousID = "",
  hysteresis = CONTENT_CONTEXT_TRIGGER_HYSTERESIS,
} = {}) {
  const viewport = Number(viewportHeight);
  const tolerance = Math.max(0, Number(hysteresis) || 0);
  if (!Number.isFinite(viewport) || viewport <= 0 || !Array.isArray(candidates)) return "";
  const readingLine = contentContextReadingExitLine({ viewportHeight: viewport });
  const entryLine = Math.min(
    viewport * CONTENT_CONTEXT_TRIGGER_ENTRY_RATIO,
    viewport - CONTENT_CONTEXT_TRIGGER_MIN_DRAWER_HEIGHT,
  );
  const eligible = candidates
    .map((candidate, index) => ({
      id: String(candidate?.id || ""),
      top: Number(candidate?.top),
      bottom: Number(candidate?.bottom),
      eligible: candidate?.eligible !== false,
      index,
    }))
    .filter((candidate) => candidate.id
      && candidate.eligible
      && Number.isFinite(candidate.top)
      && Number.isFinite(candidate.bottom)
      && candidate.bottom > 0
      && candidate.top >= 0
      && candidate.top < viewport);
  const previous = eligible.find((candidate) => candidate.id === String(previousID || ""));
  if (previous
    && previous.top <= readingLine + tolerance
    && previous.bottom > readingLine) {
    return previous.id;
  }
  const crossing = eligible.find((candidate) => candidate.top <= readingLine && candidate.bottom > readingLine);
  if (crossing) return crossing.id;
  const upcoming = eligible
    .filter((candidate) => candidate.top > readingLine && candidate.top < entryLine)
    .sort((left, right) => left.top - right.top || left.index - right.index)[0];
  return upcoming?.id || "";
}

// Downward movement keeps the existing 20% reading exit rule. Upward movement
// has a separate offscreen rule and can be configured to preserve the active
// drawer state while its post is below the viewport.
export function contentContextShouldCloseOnScroll({
  previousScrollY = 0,
  scrollY = 0,
  postTop = 0,
  postBottom = 0,
  viewportHeight = 0,
  upScrollMode = CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT,
} = {}) {
  const previous = Number(previousScrollY);
  const current = Number(scrollY);
  if (!Number.isFinite(previous) || !Number.isFinite(current)) return false;
  if (current > previous) {
    return contentContextPostPassedReadingExitLine({ postBottom, viewportHeight });
  }
  if (current < previous && normalizeContentContextUpScrollMode(upScrollMode) === CONTENT_CONTEXT_UP_SCROLL_MODE_CLOSE_OFFSCREEN) {
    return contentContextPostPassedViewportBottom({ postTop, viewportHeight });
  }
  return false;
}

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

export function buildTimelineContentContextFeedbackPath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/timeline/${encodeURIComponent(normalized)}/content-context-feedback` : "";
}

export function buildContentContextFeedbackUndoPath(id) {
  const normalized = String(id ?? "").trim();
  return normalized ? `/api/content-context-feedback/${encodeURIComponent(normalized)}/undo` : "";
}
