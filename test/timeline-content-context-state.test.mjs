import test from "node:test";
import assert from "node:assert/strict";

import {
  backToTopBoundaryBottom,
  CONTENT_CONTEXT_DEFAULT_LIMIT,
  CONTENT_CONTEXT_MAX_LIMIT,
  contentContextTabPlacement,
  contentContextDrawerMode,
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
  assert.equal(contentContextDrawerMode({ viewportWidth: 1024, rightGutter: 160 }), "overlay");
  assert.equal(contentContextDrawerMode({ viewportWidth: 760, rightGutter: 500 }), "sheet");
});

test("Back to top boundary aligns its base with the marker line", () => {
  assert.equal(backToTopBoundaryBottom({ lineY: 640, viewportHeight: 800, restBottom: 20 }), 160);
  assert.equal(backToTopBoundaryBottom({ lineY: 790, viewportHeight: 800, restBottom: 20 }), 20);
});

test("Related context tab placement hides when the back-to-top gap is unsafe", () => {
  assert.deepEqual(
    contentContextTabPlacement({
      postRight: 900, boundaryLeft: 950, viewportWidth: 1200, postTop: 120, viewportHeight: 800,
      tabWidth: 42, tabHeight: 126, gap: 12, viewportPadding: 12,
    }),
    null,
  );
  const placement = contentContextTabPlacement({
    postRight: 760, boundaryLeft: 1160, viewportWidth: 1200, postTop: 120, viewportHeight: 800,
    tabWidth: 42, tabHeight: 126, gap: 12, viewportPadding: 12,
  });
  assert.equal(placement.availableWidth, 400);
  assert.equal(placement.right, 386);
  assert.equal(placement.top, 183);
});
