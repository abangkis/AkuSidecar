import assert from "node:assert/strict";
import test from "node:test";
import {
  buildPilotReview,
  classifyRunFailure,
  summarizePilotRuns,
  summarizeSourceHealth,
} from "../src/core/pilot-review.mjs";

test("pilot review summarizes trust, item feedback, latency, and suppression", () => {
  const runs = [
    runFixture({
      id: "empty-correct",
      feedback: [{ kind: "correct_empty", itemId: null }],
      durationMs: 3_000,
      suppressed: 4,
    }),
    runFixture({
      id: "empty-missed",
      source: "linkedin",
      feedback: [{ kind: "missed", itemId: null, note: "A release was omitted." }],
      durationMs: 9_000,
    }),
    runFixture({
      id: "promoted",
      items: [{ id: "item-1" }],
      feedback: [{ kind: "useful", itemId: "item-1" }],
      durationMs: 6_000,
      rounds: 2,
    }),
    runFixture({ id: "failed", status: "failed", completedAt: null }),
  ];

  const summary = summarizePilotRuns(runs);
  assert.equal(summary.totalRuns, 4);
  assert.equal(summary.completedRuns, 3);
  assert.equal(summary.failedRuns, 1);
  assert.equal(summary.reviewedRuns, 3);
  assert.equal(summary.emptyTrustRate, 0.5);
  assert.equal(summary.positiveItemRate, 1);
  assert.equal(summary.medianDurationMs, 6_000);
  assert.equal(summary.averageAcquisitionRounds, 4 / 3);
  assert.equal(summary.evidenceSuppressed, 4);
});

test("pilot review filters source and verdict without changing the source summary", () => {
  const runs = [
    runFixture({ id: "x-reviewed", feedback: [{ kind: "correct_empty", itemId: null }] }),
    runFixture({ id: "x-unreviewed" }),
    runFixture({ id: "linkedin-unreviewed", source: "linkedin" }),
  ];
  const review = buildPilotReview(runs, {
    source: "x",
    verdict: "unreviewed",
    limit: 10,
  });
  assert.equal(review.summary.totalRuns, 2);
  assert.equal(review.totalMatching, 1);
  assert.equal(review.runs[0].id, "x-unreviewed");
});

test("pilot review rejects unknown verdicts", () => {
  assert.throws(
    () => buildPilotReview([], { verdict: "silently-accept-everything" }),
    /unsupported pilot review verdict/,
  );
});

test("pilot review pages ten runs within a fifty-run browsing boundary", () => {
  const runs = Array.from({ length: 60 }, (_, index) => runFixture({ id: `run-${index}` }));
  const review = buildPilotReview(runs, { limit: 10, offset: 40, maxRuns: 50 });
  assert.equal(review.totalMatching, 60);
  assert.deepEqual(review.runs.map((run) => run.id), runs.slice(40, 50).map((run) => run.id));
  assert.deepEqual(review.pagination, {
    limit: 10,
    offset: 40,
    available: 50,
    hasPrevious: true,
    hasNext: false,
  });
});

test("pilot review ignores malformed legacy feedback outside its run context", () => {
  const runs = [
    runFixture({
      id: "non-empty-run-level",
      items: [{ id: "real-item" }],
      feedback: [{ kind: "correct_empty", itemId: null }],
    }),
    runFixture({
      id: "empty-item-level",
      feedback: [{ kind: "useful", itemId: "missing-item" }],
    }),
    runFixture({
      id: "missed-without-note",
      feedback: [{ kind: "missed", itemId: null, note: "" }],
    }),
  ];

  const review = buildPilotReview(runs, { verdict: "unreviewed" });
  assert.equal(review.summary.reviewedRuns, 0);
  assert.equal(review.summary.correctlyEmptyRuns, 0);
  assert.equal(review.summary.missedRuns, 0);
  assert.equal(review.summary.reviewedItems, 0);
  assert.equal(review.totalMatching, 3);
  assert.deepEqual(review.runs.flatMap((run) => run.feedback), []);
});

test("zero-evidence completed history is not treated as a reviewable empty result", () => {
  const run = runFixture({
    id: "unavailable-empty",
    feedback: [{ kind: "correct_empty", itemId: null }],
    coverageStatus: "unavailable",
    observedBlockCount: 0,
  });
  const review = buildPilotReview([run], { verdict: "unreviewed" });
  assert.equal(review.summary.completedRuns, 1);
  assert.equal(review.summary.reviewableRuns, 0);
  assert.equal(review.summary.emptyRuns, 0);
  assert.equal(review.summary.reviewedRuns, 0);
  assert.equal(review.totalMatching, 0);
});

test("pilot review preserves unified-session grouping metadata", () => {
  const review = buildPilotReview([
    {
      ...runFixture({ id: "grouped-x" }),
      unifiedSessionId: "unified-session-1",
      unifiedSessionCreatedAt: "2026-07-11T09:00:00.000Z",
    },
  ]);
  assert.equal(review.runs[0].unifiedSessionId, "unified-session-1");
  assert.equal(review.runs[0].unifiedSessionCreatedAt, "2026-07-11T09:00:00.000Z");
});

test("rolling source health separates current reliability from historical totals", () => {
  const recent = Array.from({ length: 20 }, (_, index) => runFixture({
    id: `recent-${index}`,
    source: index % 2 === 0 ? "x" : "linkedin",
    status: index === 0 ? "failed" : "completed",
    createdAt: new Date(Date.UTC(2026, 6, 12, 0, 0, 20 - index)).toISOString(),
    error: index === 0 ? { stage: "browser_capture", message: "No tab with id: 42" } : null,
  }));
  const historical = Array.from({ length: 25 }, (_, index) => runFixture({
    id: `old-${index}`,
    status: "failed",
    createdAt: new Date(Date.UTC(2026, 6, 10, 0, 0, index)).toISOString(),
  }));
  const health = summarizeSourceHealth([...recent, ...historical]);

  assert.equal(health.totalRuns, 20);
  assert.equal(health.completedRuns, 19);
  assert.equal(health.status, "healthy");
  assert.deepEqual(health.failureCategories, [{ category: "stale_tab", count: 1 }]);
  assert.equal(health.sources.x.totalRuns, 10);
  assert.equal(health.sources.linkedin.totalRuns, 10);
});

test("failure taxonomy is deterministic and stage-aware", () => {
  assert.equal(classifyRunFailure({ error: { message: "No open, rendered x tab was found." } }), "missing_tab");
  assert.equal(classifyRunFailure({ error: { message: "LinkedIn source readiness failed: selector_mismatch" } }), "source_readiness");
  assert.equal(classifyRunFailure({ error: { stage: "reasoning", message: "candidate assessment duplicated evidence" } }), "reasoning_contract");
  assert.equal(classifyRunFailure({ error: { message: "The x follow-up frontier no longer matched" } }), "frontier_mismatch");
});

function runFixture({
  id,
  source = "x",
  status = "completed",
  items = [],
  feedback = [],
  durationMs = 5_000,
  suppressed = 0,
  rounds = 1,
  coverageStatus = "partial",
  observedBlockCount = 1,
  completedAt = "2026-07-11T01:00:05.000Z",
  createdAt = "2026-07-11T01:00:00.000Z",
  error = null,
}) {
  return {
    id,
    mode: "catch_up",
    source,
    intent: "Technical engineering changes.",
    status,
    provider: "test-provider",
    createdAt,
    startedAt: "2026-07-11T01:00:00.000Z",
    completedAt:
      completedAt && durationMs !== 5_000
        ? new Date(new Date("2026-07-11T01:00:00.000Z").valueOf() + durationMs).toISOString()
        : completedAt,
    coverage: {
      status: coverageStatus,
      observedBlockCount,
      acquisitionRounds: rounds,
      exactDuplicatesSuppressed: suppressed,
    },
    result: status === "completed" ? { summary: "Fixture", items } : null,
    error: status === "failed" ? (error ?? { message: "Fixture failure" }) : null,
    feedback,
  };
}
