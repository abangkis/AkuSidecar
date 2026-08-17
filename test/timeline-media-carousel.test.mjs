import test from "node:test";
import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import {
  MAX_TIMELINE_MEDIA,
  boundedTimelineMedia,
  moveTimelineCarouselIndex,
  normalizeTimelineCarouselIndex,
  shouldUseTimelineCarousel,
  timelineCarouselDotIndexes,
} from "../internal/httpapi/web/timeline-media-carousel.js";

test("timeline keeps one to four media in the existing layout and carousels five or more", () => {
  assert.equal(shouldUseTimelineCarousel(Array(4).fill({})), false);
  assert.equal(shouldUseTimelineCarousel(Array(5).fill({})), true);
});

test("timeline media remains bounded without discarding the fifth through twentieth slide", () => {
  const media = Array.from({ length: 25 }, (_, index) => ({ index }));
  const bounded = boundedTimelineMedia(media);

  assert.equal(MAX_TIMELINE_MEDIA, 20);
  assert.equal(bounded.length, 20);
  assert.equal(bounded[4].index, 4);
  assert.equal(bounded[19].index, 19);
});

test("carousel navigation stops at the first and last slide", () => {
  assert.equal(normalizeTimelineCarouselIndex(-5, 8), 0);
  assert.equal(moveTimelineCarouselIndex(0, -1, 8), 0);
  assert.equal(moveTimelineCarouselIndex(3, 1, 8), 4);
  assert.equal(moveTimelineCarouselIndex(7, 1, 8), 7);
});

test("carousel dot window follows the current slide in long galleries", () => {
  assert.deepEqual(timelineCarouselDotIndexes(5, 0), [0, 1, 2, 3, 4]);
  assert.deepEqual(timelineCarouselDotIndexes(12, 0), [0, 1, 2, 3, 4, 5, 6]);
  assert.deepEqual(timelineCarouselDotIndexes(12, 6), [3, 4, 5, 6, 7, 8, 9]);
  assert.deepEqual(timelineCarouselDotIndexes(12, 11), [5, 6, 7, 8, 9, 10, 11]);
});

test("carousel swipe capture stays on horizontal viewport movement and preserves taps", async () => {
  const appSource = await readFile(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");

  assert.match(appSource, /viewport\.addEventListener\("pointerdown"/);
  assert.match(appSource, /viewport\.addEventListener\("pointermove"/);
  assert.match(appSource, /viewport\.addEventListener\("pointerup"/);
  assert.match(appSource, /viewport\.addEventListener\("pointercancel"/);
  assert.doesNotMatch(appSource, /stage\.(?:addEventListener\("pointer(?:down|up|cancel)"|setPointerCapture)/);

  const pointerDownStart = appSource.indexOf('viewport.addEventListener("pointerdown"');
  const pointerMoveStart = appSource.indexOf('viewport.addEventListener("pointermove"');
  const pointerUpStart = appSource.indexOf('viewport.addEventListener("pointerup"');
  assert.ok(pointerDownStart >= 0 && pointerMoveStart > pointerDownStart && pointerUpStart > pointerMoveStart);
  assert.doesNotMatch(appSource.slice(pointerDownStart, pointerMoveStart), /setPointerCapture/);
  assert.match(appSource.slice(pointerMoveStart, pointerUpStart), /viewport\.setPointerCapture\?\.\(event\.pointerId\)/);
});

test("media viewer arrows use bounded timeline navigation at both ends", async () => {
  const appSource = await readFile(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");

  assert.match(appSource, /state\.mediaIndex = moveTimelineCarouselIndex\(state\.mediaIndex, delta, state\.media\.length\)/);
  assert.match(appSource, /\$\("#media-viewer-previous"\)\.disabled = state\.media\.length < 2 \|\| state\.mediaIndex === 0/);
  assert.match(appSource, /\$\("#media-viewer-next"\)\.disabled = state\.media\.length < 2 \|\| state\.mediaIndex === state\.media\.length - 1/);
});

test("media viewer keeps focus on the dialog and routes arrow keys to navigation", async () => {
  const appSource = await readFile(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
  const indexSource = await readFile(new URL("../internal/httpapi/web/index.html", import.meta.url), "utf8");

  assert.match(indexSource, /<dialog id="media-viewer"[^>]*tabindex="-1"/);
  assert.match(appSource, /\$\("#media-viewer"\)\.addEventListener\("keydown"/);
  assert.match(appSource, /moveMedia\(event\.key === "ArrowLeft" \? -1 : 1\)/);
  assert.match(appSource, /viewer\.focus\(\{ preventScroll: true \}\)/);
});
