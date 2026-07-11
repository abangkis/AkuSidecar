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

export function buildNativeCaptureCommand(run, limits, options = {}) {
  const acquisitionRound = options.acquisitionRound ?? 1;
  const continuation = options.continuation ?? null;
  const maxAcquisitionRounds = limits.maxAcquisitionRounds ?? 1;
  if (
    !Number.isInteger(acquisitionRound) ||
    acquisitionRound < 1 ||
    acquisitionRound > Math.min(2, maxAcquisitionRounds)
  ) {
    throw new ContractError("acquisition round exceeds the deterministic Gate 0B.3 budget");
  }
  if ((acquisitionRound === 1 && continuation) || (acquisitionRound > 1 && !continuation)) {
    throw new ContractError("Gate 0B.3 continuation is required only for a follow-up round");
  }
  const revealPendingContent = options.revealPendingContent
    ?? (acquisitionRound === 1 && run.source !== "linkedin");
  return {
    mode: run.mode,
    source: run.source,
    scrolls: options.scrolls ?? run.scrolls,
    scrollFraction: limits.scrollFraction,
    scrollSettleMs: limits.scrollSettleMs,
    captureTimeoutMs: limits.captureTimeoutMs,
    pendingContentPolicy: revealPendingContent ? "reveal_if_present" : "detect_only",
    sameTabMutationAllowed: revealPendingContent,
    pendingContentTimeoutMs: limits.pendingContentTimeoutMs,
    pendingContentSettleMs: limits.pendingContentSettleMs,
    maxBlocksPerSnapshot: limits.maxBlocksPerSnapshot,
    maxBlockCharacters: limits.maxBlockCharacters,
    openIfMissing:
      acquisitionRound === 1 && limits.missingSourceTabPolicy !== "fail_fast",
    tabLifecycle: {
      ownership: "shared",
      openedTabDisposition: "preserve",
    },
    restoreScroll: true,
    browserAdapter: NATIVE_BROWSER_ADAPTER,
    acquisitionRound,
    maxAcquisitionRounds,
    continuation,
    followUpReason: options.followUpReason ?? "",
    ...(options.pendingContentRecovery
      ? { pendingContentRecovery: options.pendingContentRecovery }
      : {}),
  };
}

export function buildObservationContinuation(observation, limits) {
  const frontier = observation.snapshots.at(-1);
  if (!frontier) return null;
  const anchorKeys = [];
  for (const key of observation.coverage?.frontier?.anchorKeys ?? []) {
    if (!key || anchorKeys.includes(key)) continue;
    anchorKeys.push(key);
    if (anchorKeys.length >= (limits.maxContinuationAnchors ?? 3)) break;
  }
  for (const block of frontier.blocks) {
    const key = blockIdentity(block);
    if (!key || anchorKeys.includes(key)) continue;
    anchorKeys.push(key);
    if (anchorKeys.length >= (limits.maxContinuationAnchors ?? 3)) break;
  }
  if (anchorKeys.length === 0) return null;
  return {
    startScrollY: Math.max(
      0,
      Math.trunc(observation.coverage?.frontier?.scrollY ?? frontier.scrollY),
    ),
    anchorKeys,
    settleMs: limits.scrollSettleMs,
  };
}

export function assertNativeCaptureOutcome(commandPayload, observation) {
  if (commandPayload.scrolls === 0) return;

  const coverage = observation.coverage;
  if (coverage.browserAdapter !== NATIVE_BROWSER_ADAPTER) {
    throw new ContractError("Gate 0B observation did not identify AkuBridge as its browser adapter");
  }
  if (coverage.acquisitionRound !== (commandPayload.acquisitionRound ?? 1)) {
    throw new ContractError("Gate 0B.3 observation acquisition round does not match its command");
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
  if (commandPayload.continuation) {
    if (
      coverage.continuationRequested !== true ||
      coverage.continuationAnchorMatched !== true ||
      Math.abs(
        coverage.captureStartScrollY - commandPayload.continuation.startScrollY,
      ) >= 2
    ) {
      throw new ContractError(
        "Gate 0B.3 continuation must prove its prior-observation frontier anchor",
      );
    }
  } else if (coverage.continuationRequested === true) {
    throw new ContractError("Gate 0B.3 observation reported an unrequested continuation");
  }
  if (
    coverage.restored &&
    Math.abs(coverage.finalScrollY - coverage.originalScrollY) >= 2
  ) {
    throw new ContractError("Gate 0B observation claimed restoration with a mismatched final position");
  }
}

function blockIdentity(block) {
  if (block?.platformId) return block.platformId;
  if (block?.permalink) return block.permalink;
  return typeof block?.text === "string"
    ? block.text.replace(/\s+/g, " ").trim().toLowerCase().slice(0, 300)
    : "";
}
