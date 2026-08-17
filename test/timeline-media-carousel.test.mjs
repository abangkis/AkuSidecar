import test from "node:test";
import assert from "node:assert/strict";
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
