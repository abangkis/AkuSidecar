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

test("LinkedIn initial capture detects pending content without activating it", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
  );
  assert.equal(command.pendingContentPolicy, "detect_only");
  assert.equal(command.sameTabMutationAllowed, false);
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
