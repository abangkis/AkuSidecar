import test from "node:test";
import assert from "node:assert/strict";
import {
  aggregateCaptureQuality,
  aggregateQualityAdmission,
} from "../src/core/job-engine.mjs";

test("multi-round quality coverage aggregates every report and admission", () => {
  const observations = [
    observation({
      captureVerdict: "complete",
      verdictCounts: { complete: 2, usable_degraded: 0, retryable: 0, invalid: 0 },
      issueCounts: {},
      reportCount: 2,
      attempts: 0,
      admissionVerdict: "complete",
      admitted: 2,
      degraded: 0,
      rejected: 0,
    }),
    observation({
      captureVerdict: "invalid",
      verdictCounts: { complete: 0, usable_degraded: 1, retryable: 0, invalid: 1 },
      issueCounts: { "media:pending_hydration": 1, "author:detected_empty": 1 },
      reportCount: 2,
      attempts: 2,
      admissionVerdict: "usable_degraded",
      admitted: 1,
      degraded: 1,
      rejected: 1,
    }),
  ];

  assert.deepEqual(aggregateCaptureQuality(observations), {
    profile: "social-post-v1",
    verdict: "invalid",
    candidateReportCount: 4,
    verdictCounts: { complete: 2, usable_degraded: 1, retryable: 0, invalid: 1 },
    issueCounts: { "media:pending_hydration": 1, "author:detected_empty": 1 },
    retryBudget: 1,
    retryAttempts: 2,
  });
  assert.deepEqual(aggregateQualityAdmission(observations), {
    verdict: "usable_degraded",
    profile: "social-post-v1",
    admittedBlockCount: 3,
    degradedBlockCount: 1,
    presentationWarningCount: 0,
    rejectedCandidateCount: 1,
    retryAttempts: 2,
    issueCounts: { "media:pending_hydration": 1, "author:detected_empty": 1 },
  });
});

function observation({
  captureVerdict,
  verdictCounts,
  issueCounts,
  reportCount,
  attempts,
  admissionVerdict,
  admitted,
  degraded,
  rejected,
}) {
  return {
    coverage: {
      captureQuality: {
        profile: "social-post-v1",
        verdict: captureVerdict,
        candidateReportCount: reportCount,
        verdictCounts,
        issueCounts,
        retryBudget: 1,
        retryAttempts: attempts,
      },
      qualityAdmission: {
        verdict: admissionVerdict,
        profile: "social-post-v1",
        admittedBlockCount: admitted,
        degradedBlockCount: degraded,
        rejectedCandidateCount: rejected,
        retryAttempts: attempts,
        issueCounts,
      },
    },
  };
}
