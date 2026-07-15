import test from "node:test";
import assert from "node:assert/strict";
import {
  assertNativeCaptureOutcome,
  buildObservationContinuation,
  buildNativeCaptureCommand,
} from "../src/browser/browser-adapter-contract.mjs";

const limits = {
  maxScrolls: 2,
  maxAcquisitionRounds: 2,
  followUpScrolls: 1,
  maxContinuationAnchors: 3,
  scrollFraction: 0.75,
  scrollSettleMs: 900,
  captureTimeoutMs: 45_000,
  pendingContentTimeoutMs: 5_000,
  pendingContentSettleMs: 700,
  maxBlocksPerSnapshot: 20,
  maxBlockCharacters: 4_000,
};

test("Gate 0B capture commands are provider-neutral and deterministically bounded", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "x", scrolls: 2 },
    limits,
  );

  assert.deepEqual(command, {
    mode: "catch_up",
    source: "x",
    scrolls: 2,
    scrollFraction: 0.75,
    scrollSettleMs: 900,
    captureTimeoutMs: 45_000,
    pendingContentPolicy: "reveal_if_present",
    sameTabMutationAllowed: true,
    pendingContentTimeoutMs: 5_000,
    pendingContentSettleMs: 700,
    sourceFreshnessPolicy: "wake_and_reveal",
    captureVisibilityPolicy: "quiet",
    captureLeaseId: null,
    maxBlocksPerSnapshot: 20,
    maxBlockCharacters: 4_000,
    qualityReportRequired: false,
    qualityRetryBudget: 0,
    qualityRetrySettleMs: 300,
    openIfMissing: true,
    tabLifecycle: { ownership: "shared", openedTabDisposition: "preserve" },
    restoreScroll: true,
    browserAdapter: "aku-bridge",
    acquisitionRound: 1,
    maxAcquisitionRounds: 2,
    continuation: null,
    followUpReason: "",
  });
});

test("missing source tab policy is configurable and follow-up never opens a replacement tab", () => {
  const failFast = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    { ...limits, missingSourceTabPolicy: "fail_fast" },
  );
  assert.equal(failFast.openIfMissing, false);

  const followUp = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    { ...limits, missingSourceTabPolicy: "open_missing_tab" },
    {
      acquisitionRound: 2,
      scrolls: 1,
      revealPendingContent: false,
      continuation: { startScrollY: 100, anchorKeys: ["text:anchor"], settleMs: 900 },
    },
  );
  assert.equal(followUp.openIfMissing, false);
});

test("capture visibility is a command authority boundary", () => {
  const quiet = buildNativeCaptureCommand(
    { mode: "catch_up", source: "x", scrolls: 0 },
    limits,
  );
  const adaptive = buildNativeCaptureCommand(
    { mode: "catch_up", source: "x", scrolls: 0 },
    { ...limits, captureVisibilityPolicy: "adaptive_fidelity" },
  );
  assert.equal(quiet.captureVisibilityPolicy, "quiet");
  assert.equal(adaptive.captureVisibilityPolicy, "adaptive_fidelity");
});

test("capture surfaces carry one explicit bounded lifecycle lease", () => {
  const standalone = buildNativeCaptureCommand(
    { id: "run-1", mode: "catch_up", source: "x", scrolls: 0 },
    limits,
  );
  const unified = buildNativeCaptureCommand(
    { id: "run-2", mode: "catch_up", source: "linkedin", scrolls: 0 },
    limits,
    { captureLeaseId: "session-1" },
  );
  assert.equal(standalone.captureLeaseId, "run-1");
  assert.equal(unified.captureLeaseId, "session-1");
});

test("LinkedIn initial capture authorizes adapter-driven freshness reveal", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
  );
  assert.equal(command.pendingContentPolicy, "reveal_if_present");
  assert.equal(command.sameTabMutationAllowed, true);
});

test("zero-scroll background capture still requires bounded freshness wake", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 0 },
    limits,
  );
  const observation = gate0bObservation();
  observation.coverage.sourceTabBackgroundAtDispatch = true;
  observation.coverage.sourceFreshness = freshnessFixture("linkedin", "active_feed_ready");

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /bounded source wake recovery/i,
  );
});

test("capture commands pre-authorize only one local quality retry", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "x", scrolls: 0 },
    {
      ...limits,
      qualityReportRequired: true,
      qualityRetryBudget: 5,
      qualityRetrySettleMs: 5_000,
    },
  );
  assert.equal(command.qualityReportRequired, true);
  assert.equal(command.qualityRetryBudget, 1);
  assert.equal(command.qualityRetrySettleMs, 1_000);
});

test("Gate 0B accepts an auditable native scroll-and-restore outcome", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
    { revealPendingContent: true },
  );
  const observation = gate0bObservation();

  assert.doesNotThrow(() => assertNativeCaptureOutcome(command, observation));
});

test("Gate 0B rejects missing native coverage instead of silently falling back", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
    { revealPendingContent: true },
  );
  const observation = gate0bObservation();
  observation.coverage.captureMethod = "computer_use";
  observation.coverage.fallbackUsed = true;

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /required native DOM capture path/i,
  );
});

test("Gate 0B rejects inconsistent movement accounting", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "x", scrolls: 2 },
    limits,
  );
  const observation = gate0bObservation();
  observation.coverage.performedScrolls = 1;

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /snapshots do not match/i,
  );
});

test("Gate 0B.2 rejects inconsistent pending-content activation", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
    { revealPendingContent: true },
  );
  const observation = gate0bObservation();
  observation.coverage.pendingNewContent = false;

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /inconsistent pending-content state/i,
  );
});

test("Gate 0B.2 requires activated content to disclose same-tab mutation semantics", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
    { revealPendingContent: true },
  );
  const observation = gate0bObservation();
  observation.coverage.sameTabMutation = false;

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /same-tab feed mutation semantics/i,
  );
});

test("Gate 0B.2 rejects activated content without bounded activation evidence", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
    { revealPendingContent: true },
  );
  const observation = gate0bObservation();
  observation.coverage.pendingContentActivationEvidence = null;

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /same-tab feed mutation semantics/i,
  );
});

test("Gate 0B.3 follow-up is anchored to the prior observation frontier", () => {
  const continuation = buildObservationContinuation(
    {
      coverage: {
        frontier: {
          scrollY: 1_360,
          anchorKeys: ["urn:li:activity:coverage-frontier"],
          hasMoreCandidateSignal: true,
        },
      },
      snapshots: [
        {
          scrollY: 1_350,
          blocks: [
            {
              text: "Frontier post",
              permalink: "https://www.linkedin.com/feed/update/urn:li:activity:frontier",
            },
          ],
        },
      ],
    },
    limits,
  );
  assert.equal(continuation.startScrollY, 1_360);
  assert.equal(continuation.anchorKeys[0], "urn:li:activity:coverage-frontier");
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
    {
      acquisitionRound: 2,
      scrolls: 1,
      continuation,
      followUpReason: "One adjacent viewport may resolve the evidence gap.",
    },
  );
  const observation = gate0bObservation();
  observation.snapshots = [{}, {}];
  Object.assign(observation.coverage, {
    sourceFreshness: freshnessFixture("linkedin", "follow_up_preserved"),
    pendingNewContent: false,
    pendingNewContentLabel: "",
    pendingNewContentAction: "not_detected",
    pendingContentActivationEvidence: null,
    pendingContentPolicy: "detect_only",
    feedMutation: false,
    sameTabMutation: false,
    restorationScope: "pre_run_position",
    preActionScrollY: 0,
    acquisitionRound: 2,
    continuationRequested: true,
    continuationAnchorMatched: true,
    captureStartScrollY: 1_360,
    requestedScrolls: 1,
    performedScrolls: 1,
    snapshotCount: 2,
    scrollDeltas: [675],
    originalScrollY: 0,
    finalScrollY: 0,
  });

  assert.equal(command.pendingContentPolicy, "detect_only");
  assert.equal(command.sameTabMutationAllowed, false);
  assert.equal(command.acquisitionRound, 2);
  assert.doesNotThrow(() => assertNativeCaptureOutcome(command, observation));

  observation.coverage.continuationAnchorMatched = false;
  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /prior-observation frontier anchor/,
  );
});

test("Gate 0B.3 command builder rejects a second round without an anchor", () => {
  assert.throws(
    () =>
      buildNativeCaptureCommand(
        { mode: "catch_up", source: "linkedin", scrolls: 2 },
        limits,
        { acquisitionRound: 2, scrolls: 1 },
      ),
    /continuation is required/,
  );
});

function gate0bObservation() {
  return {
    snapshots: [{}, {}, {}],
    coverage: {
      browserAdapter: "aku-bridge",
      captureMethod: "native_dom",
      fallbackUsed: false,
      scrollContainer: "#workspace",
      pendingNewContent: true,
      pendingNewContentLabel: "New posts",
      pendingNewContentAction: "activated",
      pendingContentActivationEvidence: "feed_fingerprint_changed",
      pendingContentPolicy: "reveal_if_present",
      sourceFreshness: freshnessFixture("linkedin", "pending_content_revealed"),
      captureVisibilityPolicy: "quiet",
      captureVisibilityMode: "managed_window",
      workingTabPreserved: true,
      feedMutation: true,
      sameTabMutation: true,
      restorationScope: "post_reveal_start",
      preActionScrollY: 1_024,
      acquisitionRound: 1,
      continuationRequested: false,
      continuationAnchorMatched: false,
      captureStartScrollY: 0,
      requestedScrolls: 2,
      performedScrolls: 2,
      snapshotCount: 3,
      scrollDeltas: [675, 675],
      scrollStopReason: "budget_exhausted",
      originalScrollY: 0,
      finalScrollY: 0,
      restoreAttempted: true,
      restored: true,
    },
  };
}

function freshnessFixture(source, outcome) {
  const revealed = outcome === "pending_content_revealed";
  const followUp = outcome === "follow_up_preserved";
  return {
    policyVersion: "source-freshness-recovery-v1",
    adapterFreshnessVersion: `${source}-freshness-v1`,
    source,
    status: "ready",
    outcome,
    verification: revealed ? "feed_change" : followUp ? "frontier_contract" : "active_dispatch",
    evidence: revealed
      ? "feed_fingerprint_changed"
      : followUp
        ? "follow_up_no_freshness_mutation"
        : "active_at_dispatch",
    backgroundAtDispatch: false,
    opened: false,
    wakeAttempted: false,
    activated: false,
    probeCount: 1,
    pendingContentDetected: revealed,
    pendingContentLabel: revealed ? "New posts" : "",
    pendingContentAction: revealed ? "activated" : "not_detected",
    feedChanged: revealed,
    feedMutation: revealed,
    waitMs: 10,
    preActionScrollY: revealed ? 1_024 : 0,
  };
}
