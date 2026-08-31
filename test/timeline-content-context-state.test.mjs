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
  contentContextPostPassedViewportBottom,
  contentContextShouldCloseOnScroll,
  selectContentContextViewportID,
  contentContextViewportTriggerTop,
  CONTENT_CONTEXT_UP_SCROLL_MODE_CLOSE_OFFSCREEN,
  CONTENT_CONTEXT_UP_SCROLL_MODE_PRESERVE,
  CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT,
  contentContextReadingExitLine,
  buildContentContextFeedbackUndoPath,
  buildTimelineContentContextPath,
  buildTimelineContentContextFeedbackPath,
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
  assert.equal(
    buildTimelineContentContextFeedbackPath("timeline/one"),
    "/api/timeline/timeline%2Fone/content-context-feedback",
  );
  assert.equal(
    buildContentContextFeedbackUndoPath("feedback/one"),
    "/api/content-context-feedback/feedback%2Fone/undo",
  );
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

test("Related context closes when scrolling up past the viewport by default", () => {
  assert.equal(CONTENT_CONTEXT_UP_SCROLL_MODE_DEFAULT, CONTENT_CONTEXT_UP_SCROLL_MODE_CLOSE_OFFSCREEN);
  assert.equal(contentContextPostPassedViewportBottom({ postTop: 800, viewportHeight: 800 }), true);
  assert.equal(contentContextPostPassedViewportBottom({ postTop: 799, viewportHeight: 800 }), false);
  assert.equal(contentContextShouldCloseOnScroll({
    previousScrollY: 400, scrollY: 300, postTop: 800, postBottom: 1200, viewportHeight: 800,
  }), true);
  assert.equal(contentContextShouldCloseOnScroll({
    previousScrollY: 400, scrollY: 300, postTop: 799, postBottom: 1199, viewportHeight: 800,
  }), false);
});

test("Preserve mode keeps an offscreen drawer open while downward exit remains unchanged", () => {
  assert.equal(contentContextShouldCloseOnScroll({
    previousScrollY: 400, scrollY: 300, postTop: 900, postBottom: 1200, viewportHeight: 800,
    upScrollMode: CONTENT_CONTEXT_UP_SCROLL_MODE_PRESERVE,
  }), false);
  assert.equal(contentContextShouldCloseOnScroll({
    previousScrollY: 300, scrollY: 400, postTop: -200, postBottom: 160, viewportHeight: 800,
    upScrollMode: CONTENT_CONTEXT_UP_SCROLL_MODE_PRESERVE,
  }), true);
});

test("Per-post Related context tabs hide when the back-to-top gap is unsafe", () => {
  assert.equal(contentContextTabFits({
    postRight: 900, boundaryLeft: 950, viewportWidth: 1200, tabWidth: 42, gap: 12, viewportPadding: 12,
  }), false);
  assert.equal(contentContextTabFits({
    postRight: 760, boundaryLeft: 1160, viewportWidth: 1200, tabWidth: 42, gap: 12, viewportPadding: 12,
  }), true);
});

test("Related context exposes only the post at the viewport reading line", () => {
  const candidates = [
    { id: "first", top: -180, bottom: 140 },
    { id: "second", top: 156, bottom: 620 },
    { id: "third", top: 640, bottom: 1040 },
  ];
  assert.equal(selectContentContextViewportID({ candidates, viewportHeight: 800 }), "second");
  assert.equal(selectContentContextViewportID({
    candidates: [
      { id: "first", top: -180, bottom: 161 },
      { id: "second", top: 167, bottom: 620 },
    ],
    viewportHeight: 800,
    previousID: "first",
  }), "first");
  assert.equal(selectContentContextViewportID({
    candidates: [
      { id: "first", top: -180, bottom: 140 },
      { id: "second", top: 167, bottom: 620 },
    ],
    viewportHeight: 800,
    previousID: "first",
  }), "second");
});

test("Related context waits until an upcoming post substantially enters the viewport", () => {
  assert.equal(selectContentContextViewportID({
    candidates: [{ id: "late", top: 700, bottom: 1100 }],
    viewportHeight: 800,
  }), "");
  assert.equal(selectContentContextViewportID({
    candidates: [{ id: "ready", top: 460, bottom: 1020 }],
    viewportHeight: 800,
  }), "ready");
  assert.equal(selectContentContextViewportID({
    candidates: [{ id: "too-little-drawer-room", top: 500, bottom: 1020 }],
    viewportHeight: 720,
  }), "");
  assert.equal(selectContentContextViewportID({
    candidates: [{ id: "hidden-duplicate", top: 200, bottom: 500, eligible: false }],
    viewportHeight: 800,
  }), "");
});

test("Viewport-scoped trigger stays within its owning post", () => {
  assert.equal(contentContextViewportTriggerTop({ postTop: 220, postBottom: 800, tabHeight: 90 }), 220);
  assert.equal(contentContextViewportTriggerTop({ postTop: -700, postBottom: 260, tabHeight: 90 }), 16);
  assert.equal(contentContextViewportTriggerTop({ postTop: -700, postBottom: 80, tabHeight: 90 }), -10);
});
