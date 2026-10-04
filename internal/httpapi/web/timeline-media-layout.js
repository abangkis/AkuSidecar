// Reserve only source-provided dimensions; unknown media keeps natural layout.
export function reserveMediaDimensions(element, media) {
  const width = Number(media?.width), height = Number(media?.height);
  if (!Number.isInteger(width) || !Number.isInteger(height) || width <= 0 || height <= 0 || width > 100000 || height > 100000) return false;
  element.width = width;
  element.height = height;
  return true;
}

// Take the snapshot after network waits. Rendering and restoration share the
// same task, so no wheel input or deferred callback can replay an older position.
export function renderWithCurrentScroll(render, viewport = window) {
  const top = viewport.scrollY;
  render();
  viewport.scrollTo({ top, behavior: "instant" });
}
