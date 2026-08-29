import test from "node:test";
import assert from "node:assert/strict";

import {
  buildTimelineKeepPath,
  timelineKeepConfirmation,
} from "../internal/httpapi/web/timeline-memory-state.js";

test("Timeline Keep sends only an encoded Timeline id and explains local retention", () => {
  assert.equal(buildTimelineKeepPath("timeline/one"), "/api/timeline/timeline%2Fone/keep-full-copy");
  assert.equal(buildTimelineKeepPath(""), "");
  const confirmation = timelineKeepConfirmation({ item: { whatChanged: "A bounded update" } });
  assert.match(confirmation, /A bounded update/);
  assert.match(confirmation, /full copy/);
  assert.match(confirmation, /bounded text locally/);
});
