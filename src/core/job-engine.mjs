import {
  ContractError,
  validateBridgeObservation,
  validateFeedback,
  validateReasoningResult,
  validateRunRequest,
} from "./contracts.mjs";

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
    this.store.enqueueBridgeCommand(run.id, "collect_visible", {
      mode: run.mode,
      source: run.source,
      scrolls: run.scrolls,
      maxBlocksPerSnapshot: this.limits.maxBlocksPerSnapshot,
      maxBlockCharacters: this.limits.maxBlockCharacters,
      openIfMissing: false,
      restoreScroll: true,
    });
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

    this.store.saveObservation(runId, observation);
    this.store.completeBridgeCommand(commandId);
    this.store.setRunStatus(runId, "reasoning");

    const reasoningPromise = this.#reasonAboutRun(runId, run, observation)
      .catch((error) => {
        this.logger.error?.("reasoning failed", { runId, error: error.message });
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

  async #reasonAboutRun(runId, run, observation) {
    try {
      const rawResult = await this.reasoningProvider.analyze({ run, observation });
      const result = validateReasoningResult(rawResult, run.maxItems);
      assertObservedSources(result, observation);
      const coverage = {
        ...observation.coverage,
        source: observation.source,
        checkedUrl: observation.pageUrl,
        snapshotCount: observation.snapshots.length,
        resultCount: result.items.length,
        provider: this.reasoningProvider.name,
        scopeStatement:
          "Bounded visible-browser sample only; this is not a claim of complete feed coverage.",
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
    source_page: new Set([observation.pageUrl]),
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
