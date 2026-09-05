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
  buildLivingTopicMemberMovePath,
  buildLivingTopicMembersPath,
  buildLivingTopicMoveUndoPath,
  buildLivingTopicPath,
  buildLivingTopicSnapshotsPath,
  buildLivingTopicSeenPath,
  buildLivingTopicsPath,
  livingTopicStatusLabel,
  livingTopicClaimGroups,
  livingTopicStatementLabel,
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
  assert.equal(buildLivingTopicMemberMovePath("topic/one", "memory/two"), "/api/living-topics/topic%2Fone/members/memory%2Ftwo/move");
  assert.equal(buildLivingTopicMoveUndoPath("move/three"), "/api/living-topic-moves/move%2Fthree/undo");
  assert.equal(buildLivingTopicSnapshotsPath("topic/one"), "/api/living-topics/topic%2Fone/snapshots");
  assert.equal(buildLivingTopicActivationPath("topic/one"), "/api/living-topics/topic%2Fone/activation");
  assert.equal(buildLivingTopicNotificationsPath(), "/api/living-topics/notifications");
  assert.equal(buildLivingTopicSeenPath("topic/one"), "/api/living-topics/topic%2Fone/seen");
  assert.equal(buildLivingTopicCandidateActionPath("topic/one", "memory/two", "accept"), "/api/living-topics/topic%2Fone/candidates/memory%2Ftwo/accept");
  assert.equal(buildLivingTopicCandidateActionPath("topic/one", "memory/two", "unknown"), "");
  assert.equal(buildLivingTopicPath(""), "");
  assert.equal(buildLivingTopicMoveUndoPath(""), "");
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
  assert.equal(LIVING_TOPIC_MAX_MEMBERS, 30);
  assert.equal(Array.from(normalizeLivingTopicName(" x".repeat(200))).length, 120);
  assert.equal(livingTopicStatusLabel("ready"), "Based on local evidence");
  assert.equal(livingTopicStatusLabel("no_change"), "No evidence changed");
  assert.equal(livingTopicStatusLabel("insufficient_evidence"), "More evidence needed");
  assert.equal(livingTopicUnderstandingLabel("pending"), "Refresh queued");
  assert.equal(livingTopicUnderstandingLabel("running"), "Updating understanding");
  assert.equal(livingTopicUnderstandingLabel("current"), "Evidence evaluated");
});

test("completed rollout leads latest state while old resets and uncertain banked credits stay distinct", () => {
  const old = { temporalStatus: "historical", eventStatus: "completed", assessment: "supported", centrality: "central", text: "Milestone reset" };
  const rollout = { temporalStatus: "current", eventStatus: "completed", assessment: "supported", centrality: "central", text: "Rollout complete" };
  const credits = { temporalStatus: "current", eventStatus: "unknown", assessment: "uncertain", centrality: "central", text: "Credit validity unconfirmed" };
  const legacy = { assessment: "supported", centrality: "central", text: "Old snapshot without time metadata" };
  assert.deepEqual(livingTopicClaimGroups([old, credits, rollout, legacy]), { current: [rollout], secondary: [], uncertain: [credits, legacy], historical: [old] });
  assert.equal(livingTopicStatementLabel(rollout, "assessment"), "Latest known · supported · completed");
  assert.equal(livingTopicStatementLabel({ kind: "resolved" }, "kind"), "No longer in projection");
  assert.equal(livingTopicStatementLabel({ kind: "removed" }, "kind"), "No longer in projection");
});
