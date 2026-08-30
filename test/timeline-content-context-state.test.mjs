import test from "node:test";
import assert from "node:assert/strict";

import {
  CONTENT_CONTEXT_DEFAULT_LIMIT,
  CONTENT_CONTEXT_MAX_LIMIT,
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
