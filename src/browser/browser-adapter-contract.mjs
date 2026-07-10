import { ContractError } from "../core/contracts.mjs";

export const NATIVE_BROWSER_ADAPTER = "aku-bridge";
export const NATIVE_CAPTURE_METHOD = "native_dom";

const STOP_REASONS = new Set([
  "budget_exhausted",
  "no_movement",
  "deadline",
  "cancelled",
  "not_requested",
]);
const PENDING_NEW_CONTENT_ACTIONS = new Set(["not_detected", "not_activated", "activated"]);
const PENDING_ACTIVATION_EVIDENCE = new Set(["feed_fingerprint_changed"]);

export function buildNativeCaptureCommand(run, limits) {
  return {
    mode: run.mode,
    source: run.source,
    scrolls: run.scrolls,
    scrollFraction: limits.scrollFraction,
    scrollSettleMs: limits.scrollSettleMs,
    captureTimeoutMs: limits.captureTimeoutMs,
    pendingContentPolicy: "reveal_if_present",
    sameTabMutationAllowed: true,
    pendingContentTimeoutMs: limits.pendingContentTimeoutMs,
    pendingContentSettleMs: limits.pendingContentSettleMs,
    maxBlocksPerSnapshot: limits.maxBlocksPerSnapshot,
    maxBlockCharacters: limits.maxBlockCharacters,
    openIfMissing: false,
    restoreScroll: true,
    browserAdapter: NATIVE_BROWSER_ADAPTER,
  };
}

export function assertNativeCaptureOutcome(commandPayload, observation) {
  if (commandPayload.scrolls === 0) return;

  const coverage = observation.coverage;
  if (coverage.browserAdapter !== NATIVE_BROWSER_ADAPTER) {
    throw new ContractError("Gate 0B observation did not identify AkuBridge as its browser adapter");
  }
  if (coverage.captureMethod !== NATIVE_CAPTURE_METHOD || coverage.fallbackUsed !== false) {
    throw new ContractError("Gate 0B observation did not use the required native DOM capture path");
  }
  if (coverage.requestedScrolls !== commandPayload.scrolls) {
    throw new ContractError("Gate 0B observation scroll budget does not match its command");
  }
  if (
    !Number.isInteger(coverage.performedScrolls) ||
    coverage.performedScrolls < 0 ||
    coverage.performedScrolls > commandPayload.scrolls
  ) {
    throw new ContractError("Gate 0B observation reported an invalid performed-scroll count");
  }
  if (
    coverage.snapshotCount !== observation.snapshots.length ||
    observation.snapshots.length !== coverage.performedScrolls + 1
  ) {
    throw new ContractError("Gate 0B observation snapshots do not match the performed-scroll count");
  }
  if (!STOP_REASONS.has(coverage.scrollStopReason)) {
    throw new ContractError("Gate 0B observation requires a valid scroll stop reason");
  }
  if (coverage.restoreAttempted !== true || typeof coverage.restored !== "boolean") {
    throw new ContractError("Gate 0B observation must report its scroll-restoration outcome");
  }
  if (coverage.scrollDeltas.length !== coverage.performedScrolls) {
    throw new ContractError("Gate 0B observation scroll deltas do not match performed scrolls");
  }
  if (!coverage.scrollContainer) {
    throw new ContractError("Gate 0B observation must identify its source scroll container");
  }
  if (!PENDING_NEW_CONTENT_ACTIONS.has(coverage.pendingNewContentAction)) {
    throw new ContractError("Gate 0B.2 observation requires a valid pending-content action state");
  }
  if (coverage.pendingContentPolicy !== commandPayload.pendingContentPolicy) {
    throw new ContractError("Gate 0B.2 observation pending-content policy does not match its command");
  }
  const shouldReveal =
    commandPayload.pendingContentPolicy === "reveal_if_present" &&
    commandPayload.sameTabMutationAllowed === true;
  const expectedPendingAction = coverage.pendingNewContent
    ? shouldReveal
      ? "activated"
      : "not_activated"
    : "not_detected";
  if (coverage.pendingNewContentAction !== expectedPendingAction) {
    throw new ContractError("Gate 0B.2 observation reported an inconsistent pending-content state");
  }
  if (coverage.pendingNewContentAction === "activated") {
    if (
      coverage.feedMutation !== true ||
      coverage.sameTabMutation !== true ||
      coverage.restorationScope !== "post_reveal_start" ||
      !PENDING_ACTIVATION_EVIDENCE.has(coverage.pendingContentActivationEvidence)
    ) {
      throw new ContractError("Gate 0B.2 reveal must report its same-tab feed mutation semantics");
    }
  } else if (
    coverage.feedMutation !== false ||
    coverage.sameTabMutation !== false ||
    coverage.restorationScope !== "pre_run_position" ||
    Math.abs(coverage.preActionScrollY - coverage.originalScrollY) >= 2
  ) {
    throw new ContractError("Gate 0B.2 no-reveal outcome must preserve the pre-run feed baseline");
  }
  if (
    coverage.restored &&
    Math.abs(coverage.finalScrollY - coverage.originalScrollY) >= 2
  ) {
    throw new ContractError("Gate 0B observation claimed restoration with a mismatched final position");
  }
}
