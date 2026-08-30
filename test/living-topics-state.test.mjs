import test from "node:test";
import assert from "node:assert/strict";

import {
  LIVING_TOPIC_MAX_MEMBERS,
  LIVING_TOPIC_MAX_NAME,
  LIVING_TOPIC_TAB_EVIDENCE,
  LIVING_TOPIC_TAB_SNAPSHOT,
  buildLivingTopicMemberPath,
  buildLivingTopicMembersPath,
  buildLivingTopicPath,
  buildLivingTopicSnapshotsPath,
  buildLivingTopicsPath,
  livingTopicStatusLabel,
  livingTopicUnderstandingLabel,
  normalizeLivingTopicName,
  normalizeLivingTopicTab,
} from "../internal/httpapi/web/living-topics-state.js";

test("Living Topics paths are explicit and encoded", () => {
  assert.equal(buildLivingTopicsPath(), "/api/living-topics");
  assert.equal(buildLivingTopicPath("topic/one"), "/api/living-topics/topic%2Fone");
  assert.equal(buildLivingTopicMembersPath("topic/one"), "/api/living-topics/topic%2Fone/members");
  assert.equal(buildLivingTopicMemberPath("topic/one", "memory/two"), "/api/living-topics/topic%2Fone/members/memory%2Ftwo");
  assert.equal(buildLivingTopicSnapshotsPath("topic/one"), "/api/living-topics/topic%2Fone/snapshots");
  assert.equal(buildLivingTopicPath(""), "");
});

test("Living Topics defaults to Understanding and bounds detail tabs", () => {
  assert.equal(normalizeLivingTopicTab(), LIVING_TOPIC_TAB_SNAPSHOT);
  assert.equal(normalizeLivingTopicTab("unknown"), LIVING_TOPIC_TAB_SNAPSHOT);
  assert.equal(normalizeLivingTopicTab(LIVING_TOPIC_TAB_EVIDENCE), LIVING_TOPIC_TAB_EVIDENCE);
});

test("Living Topics UI preserves bounded thin-slice semantics", () => {
  assert.equal(LIVING_TOPIC_MAX_NAME, 120);
  assert.equal(LIVING_TOPIC_MAX_MEMBERS, 20);
  assert.equal(Array.from(normalizeLivingTopicName(" x".repeat(200))).length, 120);
  assert.equal(livingTopicStatusLabel("ready"), "Current understanding");
  assert.equal(livingTopicStatusLabel("no_change"), "No evidence changed");
  assert.equal(livingTopicStatusLabel("insufficient_evidence"), "More evidence needed");
  assert.equal(livingTopicUnderstandingLabel("pending"), "Refresh queued");
  assert.equal(livingTopicUnderstandingLabel("running"), "Updating understanding");
  assert.equal(livingTopicUnderstandingLabel("current"), "Understanding current");
});
