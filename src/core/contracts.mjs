import { randomUUID } from "node:crypto";

export const RUN_MODES = new Set(["catch_up", "manual_live"]);
export const SOURCES = new Set(["x", "linkedin"]);
export const PRIORITIES = new Set(["P1", "P2", "P3", "P4"]);
export const FEEDBACK_KINDS = new Set([
  "correct_lane",
  "wrong_lane",
  "missed",
  "duplicate",
  "useful",
]);

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
  const scrolls = input.scrolls ?? 1;

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
        capturedAt: validDateString(snapshot.capturedAt) ?? new Date().toISOString(),
        scrollY: finiteNumber(snapshot.scrollY, 0),
        viewportHeight: finiteNumber(snapshot.viewportHeight, 0),
        blocks: blocks
          .map((block, blockIndex) => validateBlock(block, blockIndex, limits))
          .filter((block) => block.text.length > 0),
      };
    }),
    coverage: validateCoverage(input.coverage),
  };
}

function validateBlock(block, index, limits) {
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

  return {
    text: cleanString(block.text, limits.maxBlockCharacters),
    author: cleanString(block.author, 300),
    publishedAt: validDateString(block.publishedAt),
    permalink: safeHttpUrl(block.permalink),
    links,
  };
}

function validateCoverage(value) {
  if (!value || typeof value !== "object") {
    return { status: "partial", notes: ["The bridge did not provide coverage details."] };
  }
  const status = ["complete_within_scope", "partial", "unavailable"].includes(value.status)
    ? value.status
    : "partial";
  return {
    status,
    checkedThrough: validDateString(value.checkedThrough),
    candidateCount: Math.max(0, Math.trunc(finiteNumber(value.candidateCount, 0))),
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

function validateResultItem(item, index) {
  assertPlainObject(item, `result item ${index}`);
  const priority = PRIORITIES.has(item.priority) ? item.priority : "P3";
  const sourceUrl = safeHttpUrl(item.sourceUrl);
  if (!sourceUrl) {
    throw new ContractError(`result item ${index} requires a sourceUrl`);
  }
  return {
    id: cleanString(item.id, 100) || randomUUID(),
    priority,
    whatChanged: cleanString(item.whatChanged, 1_000),
    whyItMatters: cleanString(item.whyItMatters, 1_000),
    source: SOURCES.has(item.source) ? item.source : "x",
    sourceUrl,
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
  return {
    kind: input.kind,
    itemId: cleanString(input.itemId, 100),
    note: cleanString(input.note, 500),
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

function normalizeConfidence(value) {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0.5;
  return Math.max(0, Math.min(1, value));
}
