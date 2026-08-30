import test from "node:test";
import assert from "node:assert/strict";

import {
  backToTopBoundaryBottom,
  CONTENT_CONTEXT_DEFAULT_LIMIT,
  CONTENT_CONTEXT_MAX_LIMIT,
  CONTENT_CONTEXT_READING_EXIT_RATIO,
  contentContextRailPlacement,
  contentContextTabFits,
  contentContextDrawerMode,
  contentContextPostPassedReadingExitLine,
  contentContextReadingExitLine,
  buildTimelineContentContextPath,
} from "../internal/httpapi/web/timeline-content-context-state.js";

test("Content Context stays explicit, encoded, and bounded", () => {
  assert.equal(CONTENT_CONTEXT_DEFAULT_LIMIT, 3);
  assert.equal(CONTENT_CONTEXT_MAX_LIMIT, 5);
  assert.equal(
    buildTimelineContentContextPath("timeline/one"),
    "/api/timeline/timeline%2Fone/content-context?limit=3",
  );
  assert.equal(
    buildTimelineContentContextPath("timeline/one", 99),
    "/api/timeline/timeline%2Fone/content-context?limit=5",
  );
  assert.equal(buildTimelineContentContextPath(""), "");
});

test("Content Context uses an item anchor to choose the responsive drawer mode", () => {
  assert.equal(contentContextDrawerMode({ viewportWidth: 1440, rightGutter: 360 }), "rail");
  assert.equal(contentContextDrawerMode({ viewportWidth: 1280, rightGutter: 311.5 }), "rail");
  assert.equal(contentContextDrawerMode({ viewportWidth: 1024, rightGutter: 160 }), "overlay");
  assert.equal(contentContextDrawerMode({ viewportWidth: 760, rightGutter: 500 }), "sheet");
});

test("Related context rail mirrors AI Signals and attaches to the post edge", () => {
  assert.deepEqual(contentContextRailPlacement({
    postRight: 760,
    safeBoundary: 1160,
    viewportWidth: 1200,
  }), { width: 400, right: 40 });
  assert.deepEqual(contentContextRailPlacement({
    postRight: 620,
    safeBoundary: 1160,
    viewportWidth: 1200,
  }), { width: 420, right: 160 });
  assert.deepEqual(contentContextRailPlacement({
    postRight: 952.5,
    safeBoundary: 1249,
    viewportWidth: 1265,
  }), { width: 296.5, right: 16 });
  assert.deepEqual(contentContextRailPlacement({
    postRight: 760,
    safeBoundary: 1184,
    viewportWidth: 1200,
  }), { width: 420, right: 20 });
  assert.equal(contentContextRailPlacement({
    postRight: 900,
    safeBoundary: 1100,
    viewportWidth: 1200,
  }), null);
});

test("Back to top boundary aligns its base with the marker line", () => {
  assert.equal(backToTopBoundaryBottom({ lineY: 640, viewportHeight: 800, restBottom: 20 }), 160);
  assert.equal(backToTopBoundaryBottom({ lineY: 790, viewportHeight: 800, restBottom: 20 }), 20);
});

test("Related context closes at the active post's 20% reading exit line", () => {
  assert.equal(CONTENT_CONTEXT_READING_EXIT_RATIO, 0.2);
  assert.equal(contentContextReadingExitLine({ viewportHeight: 800 }), 160);
  assert.equal(contentContextPostPassedReadingExitLine({ postBottom: 160, viewportHeight: 800 }), true);
  assert.equal(contentContextPostPassedReadingExitLine({ postBottom: 161, viewportHeight: 800 }), false);
  assert.equal(contentContextPostPassedReadingExitLine({ postBottom: 480, viewportHeight: 800 }), false);
  assert.equal(contentContextPostPassedReadingExitLine({ postBottom: 80, viewportHeight: 0 }), false);
});

test("Per-post Related context tabs hide when the back-to-top gap is unsafe", () => {
  assert.equal(contentContextTabFits({
    postRight: 900, boundaryLeft: 950, viewportWidth: 1200, tabWidth: 42, gap: 12, viewportPadding: 12,
  }), false);
  assert.equal(contentContextTabFits({
    postRight: 760, boundaryLeft: 1160, viewportWidth: 1200, tabWidth: 42, gap: 12, viewportPadding: 12,
  }), true);
});
