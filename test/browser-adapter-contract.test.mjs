import test from "node:test";
import assert from "node:assert/strict";
import {
  assertNativeCaptureOutcome,
  buildNativeCaptureCommand,
} from "../src/browser/browser-adapter-contract.mjs";

const limits = {
  maxScrolls: 2,
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
    openIfMissing: false,
    restoreScroll: true,
    browserAdapter: "aku-bridge",
  });
});

test("Gate 0B accepts an auditable native scroll-and-restore outcome", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
  );
  const observation = gate0bObservation();

  assert.doesNotThrow(() => assertNativeCaptureOutcome(command, observation));
});

test("Gate 0B rejects missing native coverage instead of silently falling back", () => {
  const command = buildNativeCaptureCommand(
    { mode: "catch_up", source: "linkedin", scrolls: 2 },
    limits,
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
  );
  const observation = gate0bObservation();
  observation.coverage.pendingContentActivationEvidence = null;

  assert.throws(
    () => assertNativeCaptureOutcome(command, observation),
    /same-tab feed mutation semantics/i,
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
