import test from "node:test";
import assert from "node:assert/strict";

import {
  buildTimelineReadLaterPath,
  timelineReadLaterConfirmation,
  buildTimelineKeepPath,
  timelineKeepConfirmation,
} from "../internal/httpapi/web/timeline-memory-state.js";

test("Timeline Read later sends only an encoded Timeline id and stays source-dependent when needed", () => {
  assert.equal(buildTimelineReadLaterPath("timeline/one"), "/api/timeline/timeline%2Fone/read-later");
  assert.equal(buildTimelineReadLaterPath(""), "");
  const confirmation = timelineReadLaterConfirmation({ item: { whatChanged: "A bounded update" } });
  assert.match(confirmation, /A bounded update/);
  assert.match(confirmation, /best locally available text or source reference/);
  assert.doesNotMatch(confirmation, /provider/);
});

test("Timeline Keep sends only an encoded Timeline id and explains local retention", () => {
  assert.equal(buildTimelineKeepPath("timeline/one"), "/api/timeline/timeline%2Fone/keep-full-copy");
  assert.equal(buildTimelineKeepPath(""), "");
  const confirmation = timelineKeepConfirmation({ item: { whatChanged: "A bounded update" } });
  assert.match(confirmation, /A bounded update/);
  assert.match(confirmation, /full copy/);
  assert.match(confirmation, /bounded text locally/);
});
