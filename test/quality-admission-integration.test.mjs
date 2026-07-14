import test from "node:test";
import assert from "node:assert/strict";
import { sourceFreshnessFixture } from "./source-freshness-fixture.mjs";
import { mediaRecoveryFixture, mediaRecoverySummaryFixture } from "./media-recovery-fixture.mjs";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { JobEngine } from "../src/core/job-engine.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";
import { DeterministicReasoningProvider } from "../src/reasoning/deterministic-provider.mjs";

test("JobEngine admits degraded evidence and excludes invalid parser output", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-quality-admission-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  context.after(() => store.close());
  context.after(() => fs.rmSync(directory, { recursive: true, force: true }));
  const engine = new JobEngine({
    store,
    reasoningProvider: new DeterministicReasoningProvider(),
    limits: {
      maxItems: 5,
      maxScrolls: 2,
      maxAcquisitionRounds: 1,
      defaultScrolls: 0,
      scrollFraction: 0.75,
      scrollSettleMs: 900,
      captureTimeoutMs: 45_000,
      pendingContentTimeoutMs: 5_000,
      pendingContentSettleMs: 700,
      maxBlocksPerSnapshot: 20,
      maxBlockCharacters: 4_000,
      qualityReportRequired: true,
      qualityRetryBudget: 1,
      qualityRetrySettleMs: 300,
    },
    logger: { error() {} },
  });
  const run = engine.startRun({ source: "x", maxItems: 5, scrolls: 0 });
  const command = engine.claimBridgeCommand(run.id, "quality-test-bridge");
  assert.equal(command.payload.qualityReportRequired, true);
  engine.acceptBridgeObservation(command.id, run.id, observationFixture());
  const completed = await engine.waitForRun(run.id);
  assert.equal(completed.status, "completed");
  assert.equal(completed.observations[0].payload.snapshots[0].blocks.length, 1);
  assert.equal(
    completed.observations[0].payload.coverage.qualityAdmission.verdict,
    "usable_degraded",
  );
  assert.equal(completed.result.items.length, 1);
  assert.match(completed.result.items[0].whatChanged, /usable captured post/i);
  assert.equal(completed.candidateEvaluations[0].mediaRecovery.outcome, "unavailable");
});

function observationFixture() {
  const completeIssue = {
    field: "media",
    code: "pending_hydration",
    observedState: "pending_hydration",
    severity: "high",
    recoverable: true,
    attempt: 1,
  };
  const invalidIssue = {
    field: "author",
    code: "detected_empty",
    observedState: "detected_empty",
    severity: "critical",
    recoverable: true,
    attempt: 1,
  };
  const degraded = qualityReport("usable_degraded", completeIssue);
  const invalid = qualityReport("invalid", invalidIssue);
  const mediaRecoveries = [
    mediaRecoveryFixture("x", "unavailable"),
    mediaRecoveryFixture("x", "not_applicable"),
  ];
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    pageTitle: "Home / X",
    capturedAt: "2026-07-14T00:00:00.000Z",
    snapshots: [{
      index: 0,
      adapterVersion: "x-dom-v13",
      selectorCandidateCount: 2,
      visibleContainerCount: 2,
      newCandidateCount: 2,
      capturedAt: "2026-07-14T00:00:00.000Z",
      scrollY: 0,
      viewportHeight: 900,
      qualityReports: [degraded, invalid],
      blocks: [
        {
          text: "A usable captured post remains available after bounded media recovery was exhausted.",
          author: "Usable Author",
          permalink: "https://x.com/example/status/1",
          platformId: "x:status:1",
          media: [],
          mediaRecovery: mediaRecoveries[0],
          links: [],
          captureQuality: degraded,
        },
        {
          text: "An invalid captured post must not be sent to the reasoning provider.",
          author: "",
          permalink: "https://x.com/example/status/2",
          platformId: "x:status:2",
          media: [],
          mediaRecovery: mediaRecoveries[1],
          links: [],
          captureQuality: invalid,
        },
      ],
    }],
    coverage: {
      sourceFreshness: sourceFreshnessFixture("x"),
      status: "partial",
      checkedThrough: "2026-07-14T00:00:00.000Z",
      candidateCount: 2,
      observedBlockCount: 2,
      browserAdapter: "aku-bridge",
      captureMethod: "native_dom",
      adapterVersion: "x-dom-v13",
      adapterCapabilities: [{
        source: "x",
        version: "x-dom-v13",
        qualityProfile: "social-post-v1",
        actions: ["collect_visible"],
      }],
      adapterHealth: {
        state: "degraded",
        strategies: ["tweet_testid"],
        selectorCounts: { tweet_testid: 2 },
        fieldCoverage: { author: { present: 1, total: 2 } },
        domSignature: "tweet_testid:2:2",
      },
      captureQuality: {
        profile: "social-post-v1",
        verdict: "invalid",
        candidateReportCount: 2,
        verdictCounts: { complete: 0, usable_degraded: 1, retryable: 0, invalid: 1 },
        issueCounts: { "media:pending_hydration": 1, "author:detected_empty": 1 },
        retryBudget: 1,
        retryAttempts: 2,
      },
      mediaRecovery: mediaRecoverySummaryFixture(mediaRecoveries),
      fallbackUsed: false,
      notes: [],
    },
  };
}

function qualityReport(verdict, issue) {
  return {
    profile: "social-post-v1",
    verdict,
    score: verdict === "invalid" ? 0.5 : 0.8,
    attempt: 1,
    issues: [issue],
  };
}
