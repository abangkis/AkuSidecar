import test from "node:test";
import assert from "node:assert/strict";

import {
  CONTENT_CONTEXT_DEFAULT_LIMIT,
  CONTENT_CONTEXT_MAX_LIMIT,
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
