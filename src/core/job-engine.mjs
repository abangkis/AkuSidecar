import {
  ContractError,
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
    if (!this.store.getRun(runId)) throw new ContractError("run not found");
    return this.store.addFeedback(runId, validateFeedback(input));
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
      let plan;
      try {
        plan = validateAcquisitionPlan(
          await this.reasoningProvider.planAcquisition({
            run,
            observation,
            budget: {
              currentRound: 1,
              maxRounds: this.limits.maxAcquisitionRounds,
              followUpScrolls: this.limits.followUpScrolls,
              sourceLocked: run.source,
              continuationRequiresAnchor: true,
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
      const rawResult = await this.reasoningProvider.analyze({
        run,
        observation,
        observations: storedObservations,
      });
      const result = validateReasoningResult(rawResult, run.maxItems);
      assertObservedSources(result, observation);
      if (this.store.getRun(runId)?.status === "cancelled") return;
      const coverage = {
        ...aggregateCoverage(storedObservations, planning),
        source: observation.source,
        checkedUrl: observation.pageUrl,
        resultCount: result.items.length,
        provider: this.reasoningProvider.name,
        scopeStatement:
          "Bounded native-browser sample only; this is not a claim of complete feed coverage.",
      };
      this.store.completeRun(runId, result, coverage);
    } catch (error) {
      this.store.failRun(runId, "reasoning", error);
      throw error;
    }
  }
}

function assertObservedSources(result, observation) {
  const observedUrls = {
    native_post: new Set(),
    source_page: new Set(observation.pageUrls ?? [observation.pageUrl]),
    external_reference: new Set(),
  };
  for (const block of observation.snapshots.flatMap((snapshot) => snapshot.blocks)) {
    if (block.permalink) observedUrls.native_post.add(block.permalink);
    for (const link of block.links) observedUrls.external_reference.add(link.href);
  }
  for (const item of result.items) {
    if (item.source !== observation.source) {
      throw new ContractError(
        `reasoning result source ${item.source} does not match observation source ${observation.source}`,
      );
    }
    if (!observedUrls[item.sourceUrlKind]?.has(item.sourceUrl)) {
      throw new ContractError(
        `reasoning result referenced a ${item.sourceUrlKind} URL that was not present in the matching browser-observation provenance lane: ${item.sourceUrl}`,
      );
    }
  }
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
    providerFollowUpRequested: planning.providerFollowUpRequested === true,
    providerFollowUpExecuted: planning.providerFollowUpExecuted === true,
    providerFollowUpReason: planning.providerFollowUpReason || "",
    notes: observations.flatMap((observation, index) =>
      observation.coverage.notes.map((note) => `Round ${index + 1}: ${note}`),
    ),
  };
}
