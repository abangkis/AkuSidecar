export const MAX_TIMELINE_MEDIA = 20;
export const TIMELINE_CAROUSEL_THRESHOLD = 5;

export function boundedTimelineMedia(values, maximum = MAX_TIMELINE_MEDIA) {
  const limit = Number.isFinite(Number(maximum))
    ? Math.max(1, Math.min(MAX_TIMELINE_MEDIA, Math.trunc(Number(maximum))))
    : MAX_TIMELINE_MEDIA;
  return (Array.isArray(values) ? values : []).slice(0, limit);
}

export function shouldUseTimelineCarousel(values) {
  return Array.isArray(values) && values.length >= TIMELINE_CAROUSEL_THRESHOLD;
}

export function normalizeTimelineCarouselIndex(index, count) {
  const total = Math.max(0, Math.trunc(Number(count) || 0));
  if (total === 0) return 0;
  return Math.max(0, Math.min(total - 1, Math.trunc(Number(index) || 0)));
}

export function moveTimelineCarouselIndex(index, delta, count) {
  return normalizeTimelineCarouselIndex(
    normalizeTimelineCarouselIndex(index, count) + Math.trunc(Number(delta) || 0),
    count,
  );
}

export function timelineCarouselDotIndexes(count, index, maximumVisible = 7) {
  const total = Math.max(0, Math.trunc(Number(count) || 0));
  if (total === 0) return [];
  const visible = Math.max(1, Math.min(total, Math.trunc(Number(maximumVisible) || 7)));
  const current = normalizeTimelineCarouselIndex(index, total);
  const start = Math.max(0, Math.min(total - visible, current - Math.floor(visible / 2)));
  return Array.from({ length: visible }, (_, offset) => start + offset);
}

export function mediaViewerCanPan({ scrollWidth = 0, scrollHeight = 0, clientWidth = 0, clientHeight = 0 } = {}) {
  const values = [scrollWidth, scrollHeight, clientWidth, clientHeight].map(Number);
  if (values.some((value) => !Number.isFinite(value))) return false;
  const [width, height, viewportWidth, viewportHeight] = values;
  return width > viewportWidth + 1 || height > viewportHeight + 1;
}

export function mediaViewerPanPosition({ startLeft = 0, startTop = 0, startX = 0, startY = 0, x = 0, y = 0 } = {}) {
  const values = [startLeft, startTop, startX, startY, x, y].map(Number);
  if (values.some((value) => !Number.isFinite(value))) return null;
  const [left, top, originX, originY, currentX, currentY] = values;
  return {
    left: left - (currentX - originX),
    top: top - (currentY - originY),
  };
}
