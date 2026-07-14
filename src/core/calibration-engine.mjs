import { randomUUID } from "node:crypto";
import { ContractError } from "./contracts.mjs";

const LABELS = new Set(["more_like_this", "neutral", "less_like_this"]);
const ISSUE_CODES = new Set(["capture_incomplete", "wrong_source", "duplicate", "formatting"]);

export class CalibrationEngine {
  constructor({ store, maxItems = 10, maxItemsPerSource = 5 }) {
    this.store = store;
    this.maxItems = maxItems;
    this.maxItemsPerSource = maxItemsPerSource;
  }

  createFromUnifiedSession(unifiedSessionId, options = {}) {
    const existing = this.store.getCalibrationSessionByUnifiedSession(unifiedSessionId);
    if (existing) return existing;
    const triggerKind = options.triggerKind ?? "first_run";
    if (triggerKind === "first_run") {
      const existingFirstRun = this.store.getCalibrationSessionByTriggerKind("first_run");
      if (existingFirstRun) return existingFirstRun;
    }
    const unified = this.store.getUnifiedSession(unifiedSessionId);
    if (!unified || !["completed", "partial"].includes(unified.status)) {
      throw new ContractError("Calibration requires a completed or partial unified session.");
    }
    const samples = sampleCandidates(unified.children, {
      maxItems: Math.min(this.maxItems, options.maxItems ?? this.maxItems),
      maxItemsPerSource: this.maxItemsPerSource,
    });
    if (samples.length === 0) {
      throw new ContractError("Calibration requires at least one validated candidate.");
    }
    return this.store.createCalibrationSession({
      id: randomUUID(),
      unifiedSessionId,
      triggerKind,
      maxItems: Math.min(this.maxItems, options.maxItems ?? this.maxItems),
    }, samples);
  }

  get(id) {
    return this.store.getCalibrationSession(id);
  }

  getActive() {
    return this.store.getActiveCalibrationSession();
  }

  decide(id, ordinal, input) {
    const session = this.get(id);
    if (!session || session.status === "completed") {
      throw new ContractError("Calibration session is unavailable or already completed.");
    }
    const decision = normalizeDecision(input);
    const sample = session.samples.find((entry) => entry.ordinal === ordinal);
    let updated = this.store.recordCalibrationDecision(id, ordinal, decision);
    if (!updated) throw new ContractError("Calibration sample does not exist.");
    if (sample && ["more_like_this", "neutral", "less_like_this"].includes(decision.label)) {
      this.store.addPreferenceFeedback(sample.runId, {
        evidenceKey: sample.evidenceKey,
        kind: decision.label,
        reasonCode: null,
        note: "",
        origin: "calibration",
        contextId: id,
      });
    }
    if (updated.resolvedCount === updated.sampleCount) {
      updated = this.store.completeCalibrationSession(id, buildSnapshot(updated));
      if (updated.triggerKind === "first_run") {
        this.store.setSetting("calibration.first_run_status", "completed");
      }
    }
    return updated;
  }
}

export function sampleCandidates(children, { maxItems = 10, maxItemsPerSource = 5 } = {}) {
  const queues = children
    .filter((child) => child.run && child.run.status === "completed")
    .map((child) => ({
      source: child.source,
      index: 0,
      candidates: [...(child.run.candidateEvaluations ?? [])]
        .sort((a, b) => (a.feedPosition ?? 0) - (b.feedPosition ?? 0))
        .slice(0, maxItemsPerSource),
    }));
  const samples = [];
  while (samples.length < maxItems && queues.some((queue) => queue.index < queue.candidates.length)) {
    for (const queue of queues) {
      const candidate = queue.candidates[queue.index];
      if (!candidate) continue;
      queue.index += 1;
      samples.push({
        runId: candidate.runId ?? children.find((child) => child.source === queue.source)?.runId,
        evidenceKey: candidate.evidenceKey,
        source: queue.source,
        candidate,
      });
      if (samples.length >= maxItems) break;
    }
  }
  return samples;
}

function normalizeDecision(input) {
  if (LABELS.has(input?.label)) return { label: input.label, issueCode: null };
  if (ISSUE_CODES.has(input?.issueCode)) return { label: null, issueCode: input.issueCode };
  throw new ContractError("Calibration decision requires More, Neutral, Less, or a supported capture issue.");
}

function buildSnapshot(session) {
  const labeled = session.samples.filter((sample) => sample.label);
  return {
    version: 0,
    origin: "calibration",
    calibrationSessionId: session.id,
    createdAt: new Date().toISOString(),
    labels: {
      moreLikeThis: labeled.filter((sample) => sample.label === "more_like_this").length,
      neutral: labeled.filter((sample) => sample.label === "neutral").length,
      lessLikeThis: labeled.filter((sample) => sample.label === "less_like_this").length,
      captureIssues: session.samples.filter((sample) => sample.issueCode).length,
    },
    sources: [...new Set(labeled.map((sample) => sample.source))],
    liveInfluence: false,
    activationState: "feeds_local_fit",
  };
}
