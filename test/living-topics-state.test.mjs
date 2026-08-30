import test from "node:test";
import assert from "node:assert/strict";

import {
  LIVING_TOPIC_MAX_MEMBERS,
  LIVING_TOPIC_MAX_NAME,
  LIVING_TOPIC_TAB_EVIDENCE,
  LIVING_TOPIC_TAB_SNAPSHOT,
  buildLivingTopicActivationPath,
  buildLivingTopicCandidateActionPath,
  buildLivingTopicNotificationsPath,
  buildLivingTopicMemberPath,
  buildLivingTopicMembersPath,
  buildLivingTopicPath,
  buildLivingTopicSnapshotsPath,
  buildLivingTopicSeenPath,
  buildLivingTopicsPath,
  livingTopicStatusLabel,
  livingTopicUnderstandingLabel,
  normalizeLivingTopicName,
  normalizeLivingTopicNotifications,
  normalizeLivingTopicTab,
} from "../internal/httpapi/web/living-topics-state.js";

test("Living Topics paths are explicit and encoded", () => {
  assert.equal(buildLivingTopicsPath(), "/api/living-topics");
  assert.equal(buildLivingTopicPath("topic/one"), "/api/living-topics/topic%2Fone");
  assert.equal(buildLivingTopicMembersPath("topic/one"), "/api/living-topics/topic%2Fone/members");
  assert.equal(buildLivingTopicMemberPath("topic/one", "memory/two"), "/api/living-topics/topic%2Fone/members/memory%2Ftwo");
  assert.equal(buildLivingTopicSnapshotsPath("topic/one"), "/api/living-topics/topic%2Fone/snapshots");
  assert.equal(buildLivingTopicActivationPath("topic/one"), "/api/living-topics/topic%2Fone/activation");
  assert.equal(buildLivingTopicNotificationsPath(), "/api/living-topics/notifications");
  assert.equal(buildLivingTopicSeenPath("topic/one"), "/api/living-topics/topic%2Fone/seen");
  assert.equal(buildLivingTopicCandidateActionPath("topic/one", "memory/two", "accept"), "/api/living-topics/topic%2Fone/candidates/memory%2Ftwo/accept");
  assert.equal(buildLivingTopicCandidateActionPath("topic/one", "memory/two", "unknown"), "");
  assert.equal(buildLivingTopicPath(""), "");
});

test("Living Topics notification state is bounded and truthful", () => {
  assert.deepEqual(normalizeLivingTopicNotifications({ newEvidenceCount: 3.8, topicsWithNewEvidence: 2, latestEvidenceAt: "2026-08-30T00:00:00Z" }), {
    newEvidenceCount: 3,
    topicsWithNewEvidence: 2,
    latestEvidenceAt: "2026-08-30T00:00:00Z",
  });
  assert.deepEqual(normalizeLivingTopicNotifications({ newEvidenceCount: -4, topicsWithNewEvidence: "invalid" }), {
    newEvidenceCount: 0,
    topicsWithNewEvidence: 0,
    latestEvidenceAt: "",
  });
});

test("Living Topics defaults to Understanding and bounds detail tabs", () => {
  assert.equal(normalizeLivingTopicTab(), LIVING_TOPIC_TAB_SNAPSHOT);
  assert.equal(normalizeLivingTopicTab("unknown"), LIVING_TOPIC_TAB_SNAPSHOT);
  assert.equal(normalizeLivingTopicTab(LIVING_TOPIC_TAB_EVIDENCE), LIVING_TOPIC_TAB_EVIDENCE);
});

test("Living Topics UI preserves bounded Full Stage 1 semantics", () => {
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
