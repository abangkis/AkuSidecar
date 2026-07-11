import { randomUUID } from "node:crypto";
import { evidenceKeyForBlock, normalizeEventKey } from "./knowledge-continuity.mjs";

export const RUN_MODES = new Set(["catch_up", "manual_live"]);
export const SOURCES = new Set(["x", "linkedin"]);
export const PRIORITIES = new Set(["P1", "P2", "P3", "P4"]);
export const SOURCE_URL_KINDS = new Set([
  "native_post",
  "source_page",
  "external_reference",
]);
export const SCROLL_STOP_REASONS = new Set([
  "budget_exhausted",
  "no_movement",
  "deadline",
  "cancelled",
  "not_requested",
]);
export const PENDING_NEW_CONTENT_ACTIONS = new Set([
  "not_detected",
  "not_activated",
  "activated",
  "failed",
]);
export const PENDING_CONTENT_POLICIES = new Set(["detect_only", "reveal_if_present"]);
export const RESTORATION_SCOPES = new Set(["pre_run_position", "post_reveal_start"]);
export const PENDING_ACTIVATION_EVIDENCE = new Set([
  "feed_fingerprint_changed",
]);
export const SOURCE_READINESS_STATES = new Set([
  "feed_ready",
  "loading",
  "login_required",
  "selector_mismatch",
  "feed_not_visible",
  "page_shell",
  "wrong_page",
]);
export const ACQUISITION_DECISIONS = new Set(["finish", "request_follow_up"]);
export const KNOWLEDGE_DELTAS = new Set([
  "new_event",
  "material_update",
  "context",
  "contradiction",
]);
export const FEEDBACK_KINDS = new Set([
  "correct_empty",
  "correct_lane",
  "wrong_lane",
  "missed",
  "duplicate",
  "useful",
]);
export const PREFERENCE_FEEDBACK_KINDS = new Set([
  "more_like_this",
  "less_like_this",
]);
export const CANDIDATE_CONTENT_TYPES = new Set([
  "release",
  "tutorial",
  "opinion",
  "benchmark",
  "announcement",
  "hiring",
  "promotion",
  "news",
  "research",
  "other",
]);
export const PREFERENCE_REASON_CODES = new Set([
  "wrong_topic",
  "already_known",
  "duplicate",
  "stale_or_superseded",
  "low_signal",
  "wrong_priority",
  "other",
]);
export const UNIFIED_SESSION_SOURCES = Object.freeze(["x", "linkedin"]);

export class ContractError extends Error {
  constructor(message, details = undefined) {
    super(message);
    this.name = "ContractError";
    this.details = details;
  }
}

export function assertPlainObject(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new ContractError(`${label} must be an object`);
  }
}

export function validateRunRequest(input, limits) {
  assertPlainObject(input, "run request");

  const mode = input.mode ?? "catch_up";
  const source = input.source ?? "x";
  const maxItems = input.maxItems ?? limits.maxItems;
  const scrolls = input.scrolls ?? Math.min(limits.defaultScrolls ?? 0, limits.maxScrolls);

  if (!RUN_MODES.has(mode)) {
    throw new ContractError(`unsupported mode: ${mode}`);
  }
  if (!SOURCES.has(source)) {
    throw new ContractError(`unsupported source: ${source}`);
  }
  if (!Number.isInteger(maxItems) || maxItems < 1 || maxItems > limits.maxItems) {
    throw new ContractError(`maxItems must be between 1 and ${limits.maxItems}`);
  }
  if (!Number.isInteger(scrolls) || scrolls < 0 || scrolls > limits.maxScrolls) {
    throw new ContractError(`scrolls must be between 0 and ${limits.maxScrolls}`);
  }

  const defaultIntent =
    mode === "manual_live"
      ? "Show the material delta in the selected source right now."
      : "Show what materially changed since the previous checkpoint.";

  return {
    id: randomUUID(),
    mode,
    source,
    maxItems,
    scrolls,
    intent: cleanString(input.intent, 500) || defaultIntent,
  };
}

export function validateUnifiedSessionRequest(input, limits) {
  assertPlainObject(input, "unified session request");
  const mode = input.mode ?? "catch_up";
  if (!RUN_MODES.has(mode)) {
    throw new ContractError(`unsupported mode: ${mode}`);
  }
  const sources = input.sources ?? UNIFIED_SESSION_SOURCES;
  if (
    !Array.isArray(sources) ||
    sources.length !== UNIFIED_SESSION_SOURCES.length ||
    sources.some((source, index) => source !== UNIFIED_SESSION_SOURCES[index])
  ) {
    throw new ContractError("unified session sources must be x then linkedin");
  }
  const maxItemsPerSource = input.maxItemsPerSource ?? limits.maxItems;
  if (
    !Number.isInteger(maxItemsPerSource) ||
    maxItemsPerSource < 1 ||
    maxItemsPerSource > limits.maxItems
  ) {
    throw new ContractError(
      `maxItemsPerSource must be between 1 and ${limits.maxItems}`,
    );
  }
  const defaultIntent =
    mode === "manual_live"
      ? "Show the material delta across X and LinkedIn right now."
      : "Show what materially changed across X and LinkedIn since the previous checkpoints.";
  return {
    id: randomUUID(),
    mode,
    intent: cleanString(input.intent, 500) || defaultIntent,
    sources: [...UNIFIED_SESSION_SOURCES],
    maxItemsPerSource,
    maxItemsTotal: Math.min(10, maxItemsPerSource * UNIFIED_SESSION_SOURCES.length),
  };
}

export function validateAcquisitionPlan(input) {
  assertPlainObject(input, "acquisition plan");
  if (!ACQUISITION_DECISIONS.has(input.decision)) {
    throw new ContractError(`unsupported acquisition decision: ${input.decision}`);
  }
  const reason = cleanString(input.reason, 500);
  if (!reason) throw new ContractError("acquisition plan reason is required");
  return { decision: input.decision, reason };
}

export function validateBridgeObservation(input, limits) {
  assertPlainObject(input, "bridge observation");
  if (!SOURCES.has(input.source)) {
    throw new ContractError(`unsupported observation source: ${input.source}`);
  }

  const pageUrl = safeHttpUrl(input.pageUrl);
  if (!pageUrl) throw new ContractError("observation pageUrl must be an http(s) URL");

  const snapshots = Array.isArray(input.snapshots) ? input.snapshots : [];
  if (snapshots.length === 0 || snapshots.length > limits.maxScrolls + 1) {
    throw new ContractError("observation must contain a bounded snapshot list");
  }

  return {
    source: input.source,
    pageUrl,
    pageTitle: cleanString(input.pageTitle, 500),
    capturedAt: validDateString(input.capturedAt) ?? new Date().toISOString(),
    snapshots: snapshots.map((snapshot, snapshotIndex) => {
      assertPlainObject(snapshot, `snapshot ${snapshotIndex}`);
      const blocks = Array.isArray(snapshot.blocks)
        ? snapshot.blocks.slice(0, limits.maxBlocksPerSnapshot)
        : [];
      return {
        index: nonNegativeInteger(snapshot.index, snapshotIndex),
        adapterVersion: cleanString(snapshot.adapterVersion, 100),
        selectorCandidateCount: nonNegativeInteger(snapshot.selectorCandidateCount, 0),
        visibleContainerCount: nonNegativeInteger(snapshot.visibleContainerCount, 0),
        newCandidateCount: nonNegativeInteger(snapshot.newCandidateCount, 0),
        capturedAt: validDateString(snapshot.capturedAt) ?? new Date().toISOString(),
        scrollY: finiteNumber(snapshot.scrollY, 0),
        viewportHeight: finiteNumber(snapshot.viewportHeight, 0),
        blocks: blocks
          .map((block, blockIndex) => validateBlock(input.source, block, blockIndex, limits))
          .filter((block) => block.text.length > 0),
      };
    }),
    coverage: validateCoverage(input.coverage, limits),
  };
}

function validateBlock(source, block, index, limits) {
  assertPlainObject(block, `block ${index}`);
  const links = Array.isArray(block.links)
    ? block.links
        .slice(0, 10)
        .map((link) => ({
          text: cleanString(link?.text, 300),
          href: safeHttpUrl(link?.href),
        }))
        .filter((link) => link.href)
    : [];

  const validated = {
    text: cleanString(block.text, limits.maxBlockCharacters),
    author: cleanString(block.author, 300),
    publishedAt: validDateString(block.publishedAt),
    permalink: safeHttpUrl(block.permalink),
    platformId: cleanString(block.platformId, 200),
    feedPosition: nonNegativeInteger(block.feedPosition, 0),
    links,
  };
  return {
    ...validated,
    evidenceKey: evidenceKeyForBlock(source, validated),
  };
}

function validateCoverage(value, limits) {
  if (!value || typeof value !== "object") {
    return {
      status: "partial",
      notes: ["The bridge did not provide coverage details."],
      scrollDeltas: [],
    };
  }
  const status = ["complete_within_scope", "partial", "unavailable"].includes(value.status)
    ? value.status
    : "partial";
  const requestedScrolls = Math.min(
    limits.maxScrolls,
    nonNegativeInteger(value.requestedScrolls, 0),
  );
  const performedScrolls = Math.min(
    requestedScrolls,
    nonNegativeInteger(value.performedScrolls, 0),
  );
  return {
    status,
    checkedThrough: validDateString(value.checkedThrough),
    candidateCount: nonNegativeInteger(value.candidateCount, 0),
    observedBlockCount: nonNegativeInteger(value.observedBlockCount, 0),
    browserAdapter: cleanString(value.browserAdapter, 100),
    captureMethod: cleanString(value.captureMethod, 100),
    fallbackUsed: value.fallbackUsed === true,
    scrollContainer: cleanString(value.scrollContainer, 200),
    pendingNewContent: value.pendingNewContent === true,
    pendingNewContentLabel: cleanString(value.pendingNewContentLabel, 200),
    pendingNewContentAction: PENDING_NEW_CONTENT_ACTIONS.has(value.pendingNewContentAction)
      ? value.pendingNewContentAction
      : null,
    pendingContentActivationEvidence: PENDING_ACTIVATION_EVIDENCE.has(
      value.pendingContentActivationEvidence,
    )
      ? value.pendingContentActivationEvidence
      : null,
    pendingContentPolicy: PENDING_CONTENT_POLICIES.has(value.pendingContentPolicy)
      ? value.pendingContentPolicy
      : null,
    sourceReadinessState: SOURCE_READINESS_STATES.has(value.sourceReadinessState)
      ? value.sourceReadinessState
      : null,
    sourceReadinessWaitMs: nonNegativeInteger(value.sourceReadinessWaitMs, 0),
    sourceSelectorCandidateCount: nonNegativeInteger(
      value.sourceSelectorCandidateCount,
      0,
    ),
    sourceVisibleSelectorCandidateCount: nonNegativeInteger(
      value.sourceVisibleSelectorCandidateCount,
      0,
    ),
    sourceLoadingIndicator: value.sourceLoadingIndicator === true,
    sourceFeedRootPresent: value.sourceFeedRootPresent === true,
    sourceTabOpened: value.sourceTabOpened === true,
    sourceTabActivatedForReadiness: value.sourceTabActivatedForReadiness === true,
    sourceTabBackgroundAtDispatch: value.sourceTabBackgroundAtDispatch === true,
    sourceReadinessRetryCount: Math.min(
      1,
      nonNegativeInteger(value.sourceReadinessRetryCount, 0),
    ),
    feedMutation: value.feedMutation === true,
    sameTabMutation: value.sameTabMutation === true,
    restorationScope: RESTORATION_SCOPES.has(value.restorationScope)
      ? value.restorationScope
      : null,
    preActionScrollY: Math.trunc(finiteNumber(value.preActionScrollY, 0)),
    acquisitionRound: Math.min(
      limits.maxAcquisitionRounds ?? 1,
      Math.max(1, nonNegativeInteger(value.acquisitionRound, 1)),
    ),
    continuationRequested: value.continuationRequested === true,
    continuationAnchorMatched: value.continuationAnchorMatched === true,
    captureStartScrollY: Math.trunc(finiteNumber(value.captureStartScrollY, 0)),
    requestedScrolls,
    performedScrolls,
    snapshotCount: Math.min(
      limits.maxScrolls + 1,
      nonNegativeInteger(value.snapshotCount, 0),
    ),
    scrollDeltas: Array.isArray(value.scrollDeltas)
      ? value.scrollDeltas
          .slice(0, limits.maxScrolls)
          .map((delta) => Math.trunc(finiteNumber(delta, 0)))
      : [],
    scrollStopReason: SCROLL_STOP_REASONS.has(value.scrollStopReason)
      ? value.scrollStopReason
      : null,
    originalScrollY: Math.trunc(finiteNumber(value.originalScrollY, 0)),
    finalScrollY: Math.trunc(finiteNumber(value.finalScrollY, 0)),
    restoreAttempted: value.restoreAttempted === true,
    restored: value.restored === true,
    elapsedMs: nonNegativeInteger(value.elapsedMs, 0),
    notes: Array.isArray(value.notes)
      ? value.notes.slice(0, 10).map((note) => cleanString(note, 500)).filter(Boolean)
      : [],
  };
}

export function validateReasoningResult(input, maxItems) {
  assertPlainObject(input, "reasoning result");
  const items = Array.isArray(input.items) ? input.items.slice(0, maxItems) : [];
  return {
    summary: cleanString(input.summary, 1_000),
    items: items.map((item, index) => validateResultItem(item, index)),
    candidateAssessments: Array.isArray(input.candidateAssessments)
      ? input.candidateAssessments
          .slice(0, 20)
          .map((assessment, index) => validateCandidateAssessment(assessment, index))
      : [],
    repeatedClaimsCollapsed: Math.max(
      0,
      Math.trunc(finiteNumber(input.repeatedClaimsCollapsed, 0)),
    ),
    deferredByBudget: Math.max(0, Math.trunc(finiteNumber(input.deferredByBudget, 0))),
    limitations: Array.isArray(input.limitations)
      ? input.limitations.slice(0, 10).map((value) => cleanString(value, 500)).filter(Boolean)
      : [],
  };
}

function validateCandidateAssessment(value, index) {
  assertPlainObject(value, `candidate assessment ${index}`);
  const evidenceKey = cleanString(value.evidenceKey, 100);
  if (!/^(x|linkedin):[a-f0-9]{24}$/.test(evidenceKey)) {
    throw new ContractError(`candidate assessment ${index} requires a valid evidenceKey`);
  }
  return {
    evidenceKey,
    topicTags: Array.isArray(value.topicTags)
      ? [...new Set(value.topicTags.map((tag) => cleanString(tag, 80)).filter(Boolean))].slice(0, 5)
      : [],
    contentType: CANDIDATE_CONTENT_TYPES.has(value.contentType)
      ? value.contentType
      : "other",
    recommendedPriority: PRIORITIES.has(value.recommendedPriority)
      ? value.recommendedPriority
      : "P3",
    intentRelevance: normalizeConfidence(value.intentRelevance),
    novelty: normalizeConfidence(value.novelty),
    urgency: normalizeConfidence(value.urgency),
    actionability: normalizeConfidence(value.actionability),
    rationale: cleanString(value.rationale, 500),
  };
}

function validateResultItem(item, index) {
  assertPlainObject(item, `result item ${index}`);
  const priority = PRIORITIES.has(item.priority) ? item.priority : "P3";
  const sourceUrl = safeHttpUrl(item.sourceUrl);
  if (!sourceUrl) {
    throw new ContractError(`result item ${index} requires a sourceUrl`);
  }
  if (!SOURCE_URL_KINDS.has(item.sourceUrlKind)) {
    throw new ContractError(`result item ${index} requires a valid sourceUrlKind`);
  }
  const evidenceKey = cleanString(item.evidenceKey, 100);
  if (!/^(x|linkedin):[a-f0-9]{24}$/.test(evidenceKey)) {
    throw new ContractError(`result item ${index} requires a valid evidenceKey`);
  }
  const eventKey = normalizeEventKey(item.eventKey);
  if (eventKey.length < 3) {
    throw new ContractError(`result item ${index} requires a stable eventKey`);
  }
  if (!KNOWLEDGE_DELTAS.has(item.knowledgeDelta)) {
    throw new ContractError(`result item ${index} requires a valid knowledgeDelta`);
  }
  return {
    id: cleanString(item.id, 100) || randomUUID(),
    priority,
    whatChanged: cleanString(item.whatChanged, 1_000),
    whyItMatters: cleanString(item.whyItMatters, 1_000),
    source: SOURCES.has(item.source) ? item.source : "x",
    sourceUrl,
    sourceUrlKind: item.sourceUrlKind,
    evidenceKey,
    eventKey,
    knowledgeDelta: item.knowledgeDelta,
    author: cleanString(item.author, 300),
    publishedAt: validDateString(item.publishedAt),
    confidence: normalizeConfidence(item.confidence),
    evidenceState: ["primary", "secondary", "unverified"].includes(item.evidenceState)
      ? item.evidenceState
      : "unverified",
  };
}

export function validateFeedback(input) {
  assertPlainObject(input, "feedback");
  if (!FEEDBACK_KINDS.has(input.kind)) {
    throw new ContractError(`unsupported feedback kind: ${input.kind}`);
  }
  const feedback = {
    kind: input.kind,
    itemId: cleanString(input.itemId, 100),
    note: cleanString(input.note, 500),
  };
  if (feedback.kind === "missed" && !feedback.note) {
    throw new ContractError("missed feedback requires a note");
  }
  return feedback;
}

export function validatePreferenceFeedback(input) {
  assertPlainObject(input, "preference feedback");
  if (!PREFERENCE_FEEDBACK_KINDS.has(input.kind)) {
    throw new ContractError(`unsupported preference feedback kind: ${input.kind}`);
  }
  const evidenceKey = cleanString(input.evidenceKey, 80);
  if (!/^(x|linkedin):[a-f0-9]{24}$/.test(evidenceKey)) {
    throw new ContractError("preference feedback requires a valid evidenceKey");
  }
  const reasonCode = cleanString(input.reasonCode, 80) || null;
  if (reasonCode && !PREFERENCE_REASON_CODES.has(reasonCode)) {
    throw new ContractError(`unsupported preference reason code: ${reasonCode}`);
  }
  const note = cleanString(input.note, 500);
  if (reasonCode === "other" && !note) {
    throw new ContractError("other preference feedback requires a note");
  }
  return {
    kind: input.kind,
    evidenceKey,
    reasonCode,
    note,
  };
}

export function cleanString(value, maxLength) {
  if (typeof value !== "string") return "";
  return value.replace(/\s+/g, " ").trim().slice(0, maxLength);
}

export function safeHttpUrl(value) {
  if (typeof value !== "string" || value.length > 4_000) return null;
  try {
    const url = new URL(value);
    return url.protocol === "https:" || url.protocol === "http:" ? url.href : null;
  } catch {
    return null;
  }
}

function validDateString(value) {
  if (typeof value !== "string") return null;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? new Date(parsed).toISOString() : null;
}

function finiteNumber(value, fallback) {
  return typeof value === "number" && Number.isFinite(value) ? value : fallback;
}

function nonNegativeInteger(value, fallback) {
  return Math.max(0, Math.trunc(finiteNumber(value, fallback)));
}

function normalizeConfidence(value) {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0.5;
  return Math.max(0, Math.min(1, value));
}
