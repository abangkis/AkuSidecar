import test from "node:test";
import assert from "node:assert/strict";
import { admitObservationQuality } from "../src/browser/observation-quality-policy.mjs";
import { mediaRecoveryFixture, mediaRecoverySummaryFixture } from "./media-recovery-fixture.mjs";

test("complete capture quality is admitted without degradation", () => {
  const observation = fixture([report("complete")]);
  const admitted = admitObservationQuality(observation, { required: true });
  assert.equal(admitted.coverage.qualityAdmission.verdict, "complete");
  assert.equal(admitted.coverage.qualityAdmission.admittedBlockCount, 1);
});

test("degraded candidates remain admitted with explicit limitations", () => {
  const observation = fixture([report("usable_degraded", {
    field: "media",
    code: "pending_hydration",
    observedState: "pending_hydration",
    severity: "high",
    recoverable: true,
    attempt: 1,
  })]);
  const admitted = admitObservationQuality(observation, { required: true });
  assert.equal(admitted.coverage.qualityAdmission.verdict, "usable_degraded");
  assert.equal(admitted.coverage.qualityAdmission.degradedBlockCount, 1);
});

test("recovered media requires matching block, aggregate, and fallback evidence", () => {
  const observation = fixture([report("complete")]);
  observation.snapshots[0].blocks[0].media = [{
    kind: "image",
    url: "https://pbs.twimg.com/media/recovered.jpg",
  }];
  observation.snapshots[0].blocks[0].mediaRecovery = mediaRecoveryFixture("x", "recovered");
  observation.coverage.mediaRecovery = mediaRecoverySummaryFixture([
    observation.snapshots[0].blocks[0].mediaRecovery,
  ]);
  observation.coverage.fallbackUsed = true;
  assert.doesNotThrow(() => admitObservationQuality(observation, { required: true }));

  observation.coverage.fallbackUsed = false;
  assert.throws(
    () => admitObservationQuality(observation, { required: true }),
    /fallbackUsed must match/i,
  );
});

test("media recovery rejects contradictory aggregate accounting", () => {
  const observation = fixture([report("complete")]);
  observation.coverage.mediaRecovery.attempts = 1;
  assert.throws(
    () => admitObservationQuality(observation, { required: true }),
    /aggregate accounting is inconsistent/i,
  );
});

test("invalid candidates are removed while usable candidates continue", () => {
  const invalid = report("invalid", {
    field: "author",
    code: "detected_empty",
    observedState: "detected_empty",
    severity: "critical",
    recoverable: true,
    attempt: 1,
  });
  const complete = report("complete");
  const observation = fixture([invalid, complete]);
  observation.snapshots[0].blocks[0].author = "";
  const admitted = admitObservationQuality(observation, { required: true });
  assert.equal(admitted.snapshots[0].blocks.length, 1);
  assert.equal(admitted.snapshots[0].blocks[0].author, "Author 2");
  assert.equal(admitted.coverage.qualityAdmission.rejectedCandidateCount, 1);
});

test("an observation with no usable candidate fails closed", () => {
  const invalid = report("invalid", {
    field: "author",
    code: "required_missing",
    observedState: "missing",
    severity: "critical",
    recoverable: false,
    attempt: 0,
  });
  const observation = fixture([invalid]);
  observation.snapshots[0].blocks[0].author = "";
  assert.throws(
    () => admitObservationQuality(observation, { required: true }),
    /rejected every observed candidate/,
  );
});

test("retryable reports cannot cross the final bridge boundary", () => {
  const retryable = report("retryable", {
    field: "media",
    code: "pending_hydration",
    observedState: "pending_hydration",
    severity: "high",
    recoverable: true,
    attempt: 0,
  });
  const observation = fixture([retryable]);
  assert.throws(
    () => admitObservationQuality(observation, { required: true }),
    /retryable candidate as a final observation/,
  );
});

test("quality summaries must match their candidate reports", () => {
  const observation = fixture([report("complete")]);
  observation.coverage.captureQuality.candidateReportCount = 2;
  assert.throws(
    () => admitObservationQuality(observation, { required: true }),
    /summary count/,
  );
});

test("quality issue totals and aggregate verdicts are recomputed", () => {
  const degraded = report("usable_degraded", {
    field: "media",
    code: "pending_hydration",
    observedState: "pending_hydration",
    severity: "high",
    recoverable: true,
    attempt: 1,
  });
  const issueMismatch = fixture([degraded]);
  issueMismatch.coverage.captureQuality.issueCounts = {};
  assert.throws(
    () => admitObservationQuality(issueMismatch, { required: true }),
    /issue counts are inconsistent/,
  );

  const verdictMismatch = fixture([degraded]);
  verdictMismatch.coverage.captureQuality.verdict = "complete";
  assert.throws(
    () => admitObservationQuality(verdictMismatch, { required: true }),
    /aggregate verdict is inconsistent/,
  );
});

test("a block report must belong to its snapshot report set", () => {
  const observation = fixture([report("complete")]);
  observation.snapshots[0].blocks[0].captureQuality = report("complete");
  observation.snapshots[0].blocks[0].captureQuality.score = 0.9;
  assert.throws(
    () => admitObservationQuality(observation, { required: true }),
    /absent from its snapshot reports/,
  );
});

function fixture(reports) {
  const blocks = reports.map((entry, index) => ({
    text: `A complete captured source post ${index + 1} with enough stable evidence text.`,
    author: `Author ${index + 1}`,
    evidenceKey: `x:${String(index + 1).padStart(24, "0")}`,
    media: [],
    mediaRecovery: mediaRecoveryFixture(
      "x",
      entry.issues.some((issue) => issue.field === "media") ? "unavailable" : "not_applicable",
    ),
    captureQuality: entry,
  }));
  const verdictCounts = Object.fromEntries(
    ["complete", "usable_degraded", "retryable", "invalid"].map((verdict) => [
      verdict,
      reports.filter((entry) => entry.verdict === verdict).length,
    ]),
  );
  const retryAttempts = reports.reduce((sum, entry) => sum + entry.attempt, 0);
  const issueCounts = {};
  for (const entry of reports) {
    for (const issue of entry.issues) {
      const key = `${issue.field}:${issue.code}`;
      issueCounts[key] = (issueCounts[key] ?? 0) + 1;
    }
  }
  const verdict = verdictCounts.invalid
    ? "invalid"
    : verdictCounts.retryable
      ? "retryable"
      : verdictCounts.usable_degraded
        ? "usable_degraded"
        : "complete";
  const mediaRecoveries = blocks.map((block) => block.mediaRecovery);
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    snapshots: [{ blocks, qualityReports: reports }],
    coverage: {
      notes: [],
      fallbackUsed: false,
      mediaRecovery: mediaRecoverySummaryFixture(mediaRecoveries),
      captureQuality: {
        profile: "social-post-v1",
        verdict,
        candidateReportCount: reports.length,
        verdictCounts,
        issueCounts,
        retryBudget: 1,
        retryAttempts,
      },
    },
  };
}

function report(verdict, issue = null) {
  return {
    profile: "social-post-v1",
    verdict,
    score: issue ? 0.8 : 1,
    attempt: issue?.attempt ?? 0,
    issues: issue ? [issue] : [],
  };
}
