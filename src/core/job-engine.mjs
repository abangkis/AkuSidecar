import {
  ContractError,
  RUN_MODES,
  SOURCES,
  validateAcquisitionPlan,
  validateBridgeObservation,
  validateFeedback,
  validateReasoningResult,
  validateRunRequest,
} from "./contracts.mjs";
import {
  assertNativeCaptureOutcome,
  buildObservationContinuation,
  buildNativeCaptureCommand,
} from "../browser/browser-adapter-contract.mjs";
import {
  filterKnownEvidence,
  uniqueEvidenceKeys,
} from "./knowledge-continuity.mjs";

export class JobEngine {
  constructor({ store, reasoningProvider, limits, logger = console }) {
    this.store = store;
    this.reasoningProvider = reasoningProvider;
    this.limits = limits;
    this.logger = logger;
    this.activeReasoning = new Map();
  }

  startRun(input) {
    const request = validateRunRequest(input, this.limits);
    const run = this.store.createRun(request, this.reasoningProvider.name);
    this.store.enqueueBridgeCommand(
      run.id,
      "collect_visible",
      buildNativeCaptureCommand(run, this.limits),
    );
    return this.store.getRun(run.id);
  }

  getRun(id) {
    return this.store.getRun(id);
  }

  listRuns(limit) {
    return this.store.listRuns(limit);
  }

  getKnowledgeContext(source, mode, limit = this.limits.maxKnowledgeContextEvents ?? 20) {
    if (!SOURCES.has(source)) throw new ContractError(`unsupported source: ${source}`);
    if (!RUN_MODES.has(mode)) throw new ContractError(`unsupported mode: ${mode}`);
    return this.store.getKnowledgeContext(source, mode, limit);
  }

  getKnowledgeEventHistory(source, mode, eventKey, limit = 50) {
    if (!SOURCES.has(source)) throw new ContractError(`unsupported source: ${source}`);
    if (!RUN_MODES.has(mode)) throw new ContractError(`unsupported mode: ${mode}`);
    if (typeof eventKey !== "string" || eventKey.length < 3) {
      throw new ContractError("eventKey is required");
    }
    return this.store.getKnowledgeEventHistory(source, mode, eventKey, limit);
  }

  cancelRun(id) {
    return this.store.cancelRun(id);
  }

  claimBridgeCommand(runId, bridgeId) {
    const run = this.store.getRun(runId);
    if (!run) throw new ContractError("run not found");
    if (run.status !== "waiting_for_bridge") return null;
    return this.store.claimBridgeCommand(runId, bridgeId);
  }

  acceptBridgeObservation(commandId, runId, rawObservation) {
    const command = this.store.getBridgeCommand(commandId);
    if (!command || command.runId !== runId) {
      throw new ContractError("bridge command does not belong to this run");
    }
    if (command.status !== "claimed") {
      throw new ContractError(`bridge command is ${command.status}, expected claimed`);
    }

    const run = this.store.getRun(runId);
    if (!run) throw new ContractError("run not found");
    if (["cancelled", "completed", "failed"].includes(run.status)) {
      throw new ContractError(`run is already ${run.status}`);
    }

    const observation = validateBridgeObservation(rawObservation, this.limits);
    if (observation.source !== run.source) {
      throw new ContractError("observation source does not match run source");
    }
    assertNativeCaptureOutcome(command.payload, observation);

    this.store.saveObservation(runId, observation);
    this.store.completeBridgeCommand(commandId);
    this.store.setRunStatus(runId, "reasoning");

    const reasoningPromise = this.#processAcceptedObservation(runId, run, command, observation)
      .catch((error) => {
        this.logger.error?.("run processing failed", { runId, error: error.message });
      })
      .finally(() => this.activeReasoning.delete(runId));
    this.activeReasoning.set(runId, reasoningPromise);

    return this.store.getRun(runId);
  }

  failBridgeCommand(commandId, runId, rawError) {
    const command = this.store.getBridgeCommand(commandId);
    if (!command || command.runId !== runId) {
      throw new ContractError("bridge command does not belong to this run");
    }
    const error = new Error(
      typeof rawError?.message === "string" ? rawError.message : "AkuBridge failed",
    );
    this.store.failBridgeCommand(commandId, error);
    return this.store.failRun(runId, "browser_capture", error);
  }

  addFeedback(runId, input) {
    const run = this.store.getRun(runId);
    if (!run) throw new ContractError("run not found");
    const feedback = validateFeedback(input);
    if (
      feedback.kind === "correct_empty" &&
      (feedback.itemId || run.status !== "completed" || (run.result?.items?.length ?? 0) !== 0)
    ) {
      throw new ContractError("correct_empty feedback requires a completed empty run");
    }
    return this.store.addFeedback(runId, feedback);
  }

  async waitForRun(runId) {
    await this.activeReasoning.get(runId);
    return this.store.getRun(runId);
  }

  async #processAcceptedObservation(runId, run, command, observation) {
    const acquisitionRound = command.payload.acquisitionRound ?? 1;
    if (
      acquisitionRound === 1 &&
      run.scrolls > 0 &&
      this.limits.maxAcquisitionRounds > 1 &&
      typeof this.reasoningProvider.planAcquisition === "function"
    ) {
      const observedEvidenceKeys = uniqueEvidenceKeys(observation);
      const knownEvidenceKeys = this.store.getKnownEvidenceKeys(
        run.source,
        run.mode,
        observedEvidenceKeys,
      );
      const confirmedExcludedKeys = this.store.getConfirmedExcludedEvidenceKeys(
        run.source,
        run.mode,
        run.intent,
        observedEvidenceKeys,
      );
      const suppressedEvidenceKeys = new Set([
        ...knownEvidenceKeys,
        ...confirmedExcludedKeys,
      ]);
      if (
        observedEvidenceKeys.length > 0 &&
        suppressedEvidenceKeys.size === observedEvidenceKeys.length
      ) {
        await this.#reasonAboutRun(runId, run, {
          providerFollowUpRequested: false,
          providerFollowUpExecuted: false,
          providerFollowUpReason:
            "The initial bounded sample contained only previously evaluated evidence.",
        });
        return;
      }
      const knowledgeContext = this.store.getKnowledgeContext(
        run.source,
        run.mode,
        this.limits.maxKnowledgeContextEvents ?? 20,
      );
      let plan;
      try {
        plan = validateAcquisitionPlan(
          await this.reasoningProvider.planAcquisition({
            run,
            observation,
            knowledgeContext,
            budget: {
              currentRound: 1,
              maxRounds: this.limits.maxAcquisitionRounds,
              followUpScrolls: this.limits.followUpScrolls,
              sourceLocked: run.source,
              continuationRequiresAnchor: true,
              knownEvidenceInCurrentSample: suppressedEvidenceKeys.size,
            },
          }),
        );
      } catch (error) {
        this.store.failRun(runId, "acquisition_planning", error);
        throw error;
      }

      if (this.store.getRun(runId)?.status === "cancelled") return;

      if (plan.decision === "request_follow_up") {
        const continuation = buildObservationContinuation(observation, this.limits);
        if (continuation) {
          const current = this.store.getRun(runId);
          if (current?.status === "cancelled") return;
          this.store.enqueueBridgeCommand(
            runId,
            "collect_visible",
            buildNativeCaptureCommand(run, this.limits, {
              acquisitionRound: 2,
              scrolls: this.limits.followUpScrolls,
              continuation,
              followUpReason: plan.reason,
            }),
          );
          this.store.setRunStatus(runId, "waiting_for_bridge");
          return;
        }
      }

      await this.#reasonAboutRun(runId, run, {
        providerFollowUpRequested: plan.decision === "request_follow_up",
        providerFollowUpExecuted: false,
        providerFollowUpReason: plan.reason,
      });
      return;
    }

    await this.#reasonAboutRun(runId, run, {
      providerFollowUpRequested: acquisitionRound > 1,
      providerFollowUpExecuted: acquisitionRound > 1,
      providerFollowUpReason: command.payload.followUpReason ?? "",
    });
  }

  async #reasonAboutRun(runId, run, planning) {
    try {
      if (this.store.getRun(runId)?.status === "cancelled") return;
      const storedObservations = this.store
        .getRun(runId)
        .observations.map((entry) => entry.payload);
      const observation = mergeObservations(storedObservations);
      const allEvidenceKeys = uniqueEvidenceKeys(observation);
      const knownEvidenceKeys = this.store.getKnownEvidenceKeys(
        run.source,
        run.mode,
        allEvidenceKeys,
      );
      const confirmedExcludedKeys = this.store.getConfirmedExcludedEvidenceKeys(
        run.source,
        run.mode,
        run.intent,
        allEvidenceKeys,
      );
      const suppressedEvidenceKeys = new Set([
        ...knownEvidenceKeys,
        ...confirmedExcludedKeys,
      ]);
      const filtered = filterKnownEvidence(observation, suppressedEvidenceKeys);
      const knowledgeContext = this.store.getKnowledgeContext(
        run.source,
        run.mode,
        this.limits.maxKnowledgeContextEvents ?? 20,
      );
      const rawResult =
        filtered.unseenEvidenceCount === 0 && suppressedEvidenceKeys.size > 0
          ? noNewEvidenceResult(filtered.exactDuplicatesSuppressed)
          : await this.reasoningProvider.analyze({
              run,
              observation: filtered.observation,
              observations: storedObservations,
              knowledgeContext,
            });
      const result = validateReasoningResult(rawResult, run.maxItems);
      assertObservedSources(result, filtered.observation);
      assertUniqueResultEvidence(result, suppressedEvidenceKeys);
      if (this.store.getRun(runId)?.status === "cancelled") return;
      const coverage = {
        ...aggregateCoverage(storedObservations, planning),
        source: observation.source,
        checkedUrl: observation.pageUrl,
        resultCount: result.items.length,
        previousCheckpointRunId: knowledgeContext.checkpoint?.runId ?? null,
        previousCheckpointAt: knowledgeContext.checkpoint?.observedAt ?? null,
        exactDuplicatesSuppressed: filtered.exactDuplicatesSuppressed,
        deliveredEvidenceSuppressed: knownEvidenceKeys.size,
        confirmedExcludedSuppressed: confirmedExcludedKeys.size,
        unseenEvidenceCount: filtered.unseenEvidenceCount,
        knowledgeContextEvents: knowledgeContext.events.length,
        checkpointAdvanced: true,
        provider: this.reasoningProvider.name,
        scopeStatement:
          "Bounded native-browser sample only; this is not a claim of complete feed coverage.",
      };
      this.store.completeRunWithKnowledge(runId, result, coverage);
    } catch (error) {
      this.store.failRun(runId, "reasoning", error);
      throw error;
    }
  }
}

function assertObservedSources(result, observation) {
  const blocksByEvidenceKey = new Map();
  for (const block of observation.snapshots.flatMap((snapshot) => snapshot.blocks)) {
    blocksByEvidenceKey.set(block.evidenceKey, block);
  }
  const pageUrls = new Set(observation.pageUrls ?? [observation.pageUrl]);
  for (const item of result.items) {
    if (item.source !== observation.source) {
      throw new ContractError(
        `reasoning result source ${item.source} does not match observation source ${observation.source}`,
      );
    }
    const block = blocksByEvidenceKey.get(item.evidenceKey);
    if (!block) {
      throw new ContractError(
        `reasoning result referenced evidence that was not present in the current browser observation: ${item.evidenceKey}`,
      );
    }
    const sourceMatches =
      (item.sourceUrlKind === "native_post" && block.permalink === item.sourceUrl) ||
      (item.sourceUrlKind === "source_page" &&
        !block.permalink &&
        pageUrls.has(item.sourceUrl)) ||
      (item.sourceUrlKind === "external_reference" &&
        block.links.some((link) => link.href === item.sourceUrl));
    if (!sourceMatches) {
      throw new ContractError(
        `reasoning result referenced a ${item.sourceUrlKind} URL outside its bound evidence block: ${item.sourceUrl}`,
      );
    }
  }
}

function assertUniqueResultEvidence(result, knownEvidenceKeys) {
  const used = new Set();
  for (const item of result.items) {
    if (knownEvidenceKeys.has(item.evidenceKey)) {
      throw new ContractError(`reasoning result attempted to promote previously delivered evidence: ${item.evidenceKey}`);
    }
    if (used.has(item.evidenceKey)) {
      throw new ContractError(`reasoning result reused one evidence block more than once: ${item.evidenceKey}`);
    }
    used.add(item.evidenceKey);
  }
}

function noNewEvidenceResult(suppressedCount) {
  return {
    summary: "No new visible evidence advanced the checkpoint in this bounded sample.",
    items: [],
    repeatedClaimsCollapsed: suppressedCount,
    deferredByBudget: 0,
    limitations: ["Every visible evidence block had already been delivered in this source and mode."],
  };
}

function mergeObservations(observations) {
  if (observations.length === 0) throw new ContractError("run has no browser observations");
  const first = observations[0];
  for (const observation of observations) {
    if (observation.source !== first.source) {
      throw new ContractError("all acquisition rounds must use the same source");
    }
  }
  return {
    source: first.source,
    pageUrl: first.pageUrl,
    pageUrls: [...new Set(observations.map((observation) => observation.pageUrl))],
    pageTitle: observations.at(-1).pageTitle,
    capturedAt: observations.at(-1).capturedAt,
    snapshots: observations.flatMap((observation) => observation.snapshots),
    coverage: {
      acquisitionRounds: observations.length,
      rounds: observations.map((observation) => observation.coverage),
    },
  };
}

function aggregateCoverage(observations, planning) {
  const first = observations[0].coverage;
  const last = observations.at(-1).coverage;
  const continuationRounds = observations.filter(
    (observation) => observation.coverage.continuationRequested,
  );
  const uniqueCandidates = new Set();
  for (const block of observations.flatMap((observation) =>
    observation.snapshots.flatMap((snapshot) => snapshot.blocks),
  )) {
    const key = block.permalink || block.text.toLowerCase().replace(/\s+/g, " ").trim();
    if (key) uniqueCandidates.add(key);
  }
  return {
    ...first,
    checkedThrough: last.checkedThrough,
    candidateCount: uniqueCandidates.size,
    observedBlockCount: observations.reduce(
      (sum, observation) => sum + observation.coverage.observedBlockCount,
      0,
    ),
    fallbackUsed: observations.some((observation) => observation.coverage.fallbackUsed),
    requestedScrolls: observations.reduce(
      (sum, observation) => sum + observation.coverage.requestedScrolls,
      0,
    ),
    performedScrolls: observations.reduce(
      (sum, observation) => sum + observation.coverage.performedScrolls,
      0,
    ),
    snapshotCount: observations.reduce(
      (sum, observation) => sum + observation.snapshots.length,
      0,
    ),
    scrollDeltas: observations.flatMap((observation) => observation.coverage.scrollDeltas),
    restoreAttempted: observations.every(
      (observation) => observation.coverage.restoreAttempted,
    ),
    restored: observations.every((observation) => observation.coverage.restored),
    finalScrollY: last.finalScrollY,
    elapsedMs: observations.reduce(
      (sum, observation) => sum + observation.coverage.elapsedMs,
      0,
    ),
    acquisitionRounds: observations.length,
    continuationRequested: continuationRounds.length > 0,
    continuationAnchorMatched:
      continuationRounds.length > 0 &&
      continuationRounds.every(
        (observation) => observation.coverage.continuationAnchorMatched === true,
      ),
    providerFollowUpRequested: planning.providerFollowUpRequested === true,
    providerFollowUpExecuted: planning.providerFollowUpExecuted === true,
    providerFollowUpReason: planning.providerFollowUpReason || "",
    notes: observations.flatMap((observation, index) =>
      observation.coverage.notes.map((note) => `Round ${index + 1}: ${note}`),
    ),
  };
}
