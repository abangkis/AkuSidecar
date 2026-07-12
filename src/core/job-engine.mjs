import {
  ContractError,
  RUN_MODES,
  SOURCES,
  validateAcquisitionPlan,
  validateBridgeObservation,
  validateFeedback,
  validatePreferenceFeedback,
  validateReasoningResult,
  validateRunRequest,
  validateUnifiedSessionRequest,
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
import { buildPreferenceReplay } from "./preference-replay.mjs";
import {
  buildShadowComparison,
  fitOfflinePreferenceExperiment,
  preferenceExperimentStatus,
} from "./offline-preference-experiment.mjs";
import { buildPilotReview } from "./pilot-review.mjs";

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

  startUnifiedSession(input) {
    const request = validateUnifiedSessionRequest(input, this.limits);
    this.store.createUnifiedSession(request);
    return this.#reconcileUnifiedSession(request.id);
  }

  getUnifiedSession(id) {
    if (!this.store.getUnifiedSession(id)) return null;
    return this.#reconcileUnifiedSession(id);
  }

  getActiveUnifiedSession() {
    const session = this.store.listOpenUnifiedSessions().at(-1);
    return session ? this.#reconcileUnifiedSession(session.id) : null;
  }

  cancelUnifiedSession(id) {
    let session = this.store.getUnifiedSession(id);
    if (!session) throw new ContractError("unified session not found");
    if (isUnifiedSessionTerminal(session.status)) return session;
    const activeChild = session.children.find(
      (child) => child.run && !isRunTerminal(child.run.status),
    );
    if (activeChild) this.store.cancelRun(activeChild.run.id);
    session = this.#syncUnifiedSessionChildren(id);
    session = {
      ...session,
      children: session.children.map((child) =>
        child.status === "queued" ? { ...child, status: "cancelled" } : child,
      ),
    };
    const completedCount = session.children.filter(
      (child) => child.run?.status === "completed",
    ).length;
    const status = completedCount > 0 ? "partial" : "cancelled";
    const outcome = buildUnifiedSessionOutcome(session, status);
    return this.store.cancelUnifiedSession(id, status, outcome.result, outcome.coverage);
  }

  getPilotReview(options = {}) {
    const source = options.source ?? "all";
    if (!["all", "x", "linkedin"].includes(source)) {
      throw new ContractError("unsupported pilot review source");
    }
    const allowedVerdicts = new Set([
      "all",
      "unreviewed",
      "correct",
      "correct_empty",
      "correct_lane",
      "missed",
      "useful",
      "wrong_lane",
      "duplicate",
      "failed",
    ]);
    if (!allowedVerdicts.has(options.verdict ?? "all")) {
      throw new ContractError("unsupported pilot review verdict");
    }
    const loaded = this.store.listRunsWithFeedback(501);
    const truncated = loaded.length > 500;
    const pilotStartedAt = this.store.getPilotReviewStartedAt();
    const cohort = loaded
      .slice(0, 500)
      .filter((run) => !pilotStartedAt || run.createdAt >= pilotStartedAt);
    return buildPilotReview(cohort, {
      ...options,
      source,
      window: {
        loaded: cohort.length,
        truncated,
        pilotStartedAt,
      },
    });
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
    if (command.payload.pendingContentRecovery) {
      observation.coverage.pendingContentRecovery = command.payload.pendingContentRecovery;
      observation.coverage.notes.push(
        "Pending-content reveal timed out; capture continued once with native detect-only policy.",
      );
    }
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
    if (canRetryPendingContentDetectOnly(command, error)) {
      const run = this.store.getRun(runId);
      this.store.enqueueBridgeCommand(
        runId,
        "collect_visible",
        buildNativeCaptureCommand(run, this.limits, {
          acquisitionRound: command.payload.acquisitionRound ?? 1,
          scrolls: command.payload.scrolls,
          revealPendingContent: false,
          pendingContentRecovery: "detect_only_after_reveal_timeout",
        }),
      );
      return this.store.setRunStatus(runId, "waiting_for_bridge");
    }
    const failed = this.store.failRun(runId, "browser_capture", error);
    this.#advanceParentSessionForRun(runId);
    return failed;
  }

  addFeedback(runId, input) {
    const run = this.store.getRun(runId);
    if (!run) throw new ContractError("run not found");
    const feedback = validateFeedback(input);
    const runLevel = ["correct_empty", "missed"].includes(feedback.kind);
    if (runLevel) {
      if (
        feedback.itemId ||
        run.status !== "completed" ||
        (run.result?.items?.length ?? 0) !== 0 ||
        run.coverage?.status === "unavailable" ||
        !runHasObservedEvidence(run)
      ) {
        throw new ContractError(
          `${feedback.kind} feedback requires a completed, evidence-bearing empty run without itemId`,
        );
      }
      const existingVerdict = run.feedback.find(
        (entry) => !entry.itemId && ["correct_empty", "missed"].includes(entry.kind),
      );
      if (existingVerdict?.kind === feedback.kind) return run;
      if (existingVerdict) {
        throw new ContractError("empty result already has a different verdict");
      }
    } else {
      if (run.status !== "completed" || !feedback.itemId) {
        throw new ContractError("item feedback requires a completed run and itemId");
      }
      if (!(run.result?.items ?? []).some((item) => item.id === feedback.itemId)) {
        throw new ContractError("feedback itemId is not present in the run result");
      }
      if (
        run.feedback.some(
          (entry) => entry.itemId === feedback.itemId && entry.kind === feedback.kind,
        )
      ) {
        return run;
      }
    }
    return this.store.addFeedback(runId, feedback);
  }

  addPreferenceFeedback(runId, input) {
    const run = this.store.getRun(runId);
    if (!run || run.status !== "completed") {
      throw new ContractError("preference feedback requires a completed run");
    }
    const feedback = validatePreferenceFeedback(input);
    const candidate = run.candidateEvaluations.find(
      (entry) => entry.evidenceKey === feedback.evidenceKey,
    );
    if (!candidate) throw new ContractError("preference feedback target was not evaluated");
    return this.store.addPreferenceFeedback(runId, feedback);
  }

  getPreferenceProfile() {
    return this.store.getPreferenceProfile();
  }

  getPreferenceReplay(limit = 500) {
    return buildPreferenceReplay(this.store.listRunsWithFeedback(limit));
  }

  getPreferenceExperiment(limit = 500) {
    const runs = this.store.listRunsWithFeedback(limit);
    return preferenceExperimentStatus(
      runs,
      this.store.getLatestPreferenceModelSnapshot(),
    );
  }

  fitPreferenceExperiment(limit = 500) {
    const runs = this.store.listRunsWithFeedback(limit);
    const experiment = fitOfflinePreferenceExperiment(runs);
    if (experiment.status !== "fitted") return experiment;
    const snapshot = this.store.savePreferenceModelSnapshot(experiment.snapshot);
    return preferenceExperimentStatus(runs, snapshot);
  }

  getPreferenceShadowComparison({ limit = 50, offset = 0, runLimit = 500 } = {}) {
    const runs = this.store.listRunsWithFeedback(runLimit);
    const status = preferenceExperimentStatus(
      runs,
      this.store.getLatestPreferenceModelSnapshot(),
    );
    return buildShadowComparison(status.currentSnapshot, runs, { limit, offset });
  }

  async waitForRun(runId) {
    await this.activeReasoning.get(runId);
    return this.store.getRun(runId);
  }

  async waitForUnifiedSession(sessionId) {
    let session = this.getUnifiedSession(sessionId);
    const activeRun = session?.children.find(
      (child) => child.run && !isRunTerminal(child.run.status),
    )?.run;
    if (activeRun) await this.waitForRun(activeRun.id);
    return this.getUnifiedSession(sessionId);
  }

  #syncUnifiedSessionChildren(sessionId) {
    let session = this.store.getUnifiedSession(sessionId);
    if (!session) throw new ContractError("unified session not found");
    for (const child of session.children) {
      if (child.run && child.status !== child.run.status) {
        this.store.setUnifiedSessionChildStatus(sessionId, child.source, child.run.status);
      }
    }
    return this.store.getUnifiedSession(sessionId);
  }

  #reconcileUnifiedSession(sessionId) {
    let session = this.#syncUnifiedSessionChildren(sessionId);
    if (isUnifiedSessionTerminal(session.status)) return session;

    const activeChild = session.children.find(
      (child) => child.run && !isRunTerminal(child.run.status),
    );
    if (activeChild) {
      this.#recoverUnifiedSessionRun(activeChild.run);
      return this.#syncUnifiedSessionChildren(sessionId);
    }

    const queuedChild = session.children.find((child) => child.status === "queued");
    if (queuedChild) {
      const run = this.startRun({
        mode: session.mode,
        source: queuedChild.source,
        intent: session.intent,
        maxItems: session.maxItemsPerSource,
        scrolls: Math.min(
          this.limits.defaultScrolls ?? 0,
          this.limits.maxScrolls,
        ),
      });
      return this.store.attachUnifiedSessionChild(sessionId, queuedChild.source, run.id);
    }

    session = this.store.getUnifiedSession(sessionId);
    const completedCount = session.children.filter(
      (child) => child.run?.status === "completed",
    ).length;
    const status =
      completedCount === session.children.length
        ? "completed"
        : completedCount > 0
          ? "partial"
          : "failed";
    const outcome = buildUnifiedSessionOutcome(session, status);
    return this.store.completeUnifiedSession(sessionId, status, outcome.result, outcome.coverage);
  }

  #recoverUnifiedSessionRun(run) {
    if (run.status !== "reasoning" || this.activeReasoning.has(run.id)) return;
    const pendingCommand = this.store.getPendingBridgeCommandForRun(run.id);
    if (pendingCommand) {
      this.store.setRunStatus(run.id, "waiting_for_bridge");
      return;
    }
    const command = this.store.getLatestBridgeCommandForRun(run.id);
    const observation = run.observations?.at(-1)?.payload;
    if (!command || !observation) {
      this.store.failRun(
        run.id,
        "recovery",
        new Error("Persisted reasoning run is missing its command or observation"),
      );
      return;
    }
    const reasoningPromise = this.#processAcceptedObservation(
      run.id,
      run,
      command,
      observation,
    )
      .catch((error) => {
        this.logger.error?.("run recovery failed", { runId: run.id, error: error.message });
      })
      .finally(() => this.activeReasoning.delete(run.id));
    this.activeReasoning.set(run.id, reasoningPromise);
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
      const planningGate = decideAcquisitionPlanning({
        policy: this.limits.acquisitionPlanningPolicy,
        observation,
        unseenEvidenceCount: Math.max(
          0,
          observedEvidenceKeys.length - suppressedEvidenceKeys.size,
        ),
      });
      if (!planningGate.invokeProvider) {
        await this.#reasonAboutRun(runId, run, {
          providerFollowUpRequested: false,
          providerFollowUpExecuted: false,
          providerFollowUpReason: planningGate.reason,
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
        const providerResponse = await this.reasoningProvider.planAcquisition({
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
          });
        const invocation = unwrapProviderInvocation(providerResponse);
        if (invocation.telemetry) this.store.saveReasoningInvocation(invocation.telemetry);
        plan = validateAcquisitionPlan(invocation.output);
      } catch (error) {
        if (error.reasoningTelemetry) this.store.saveReasoningInvocation(error.reasoningTelemetry);
        this.store.failRun(runId, "acquisition_planning", error);
        this.#advanceParentSessionForRun(runId);
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
      if (allEvidenceKeys.length === 0) {
        const error = new Error(
          `${sourceLabel(run.source)} did not become evidence-ready within the bounded capture`,
        );
        error.name = "SourceReadinessError";
        throw error;
      }
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
      const providerResponse =
        filtered.unseenEvidenceCount === 0 && suppressedEvidenceKeys.size > 0
          ? noNewEvidenceResult(filtered.exactDuplicatesSuppressed)
          : await this.reasoningProvider.analyze({
              run,
              observation: filtered.observation,
              observations: storedObservations,
              knowledgeContext,
            });
      const invocation = unwrapProviderInvocation(providerResponse);
      if (invocation.telemetry) this.store.saveReasoningInvocation(invocation.telemetry);
      const rawResult = invocation.output;
      const result = validateReasoningResult(rawResult, run.maxItems);
      assertObservedSources(result, filtered.observation);
      assertCandidateAssessments(
        result,
        filtered.observation,
        invocation.evaluatedEvidenceKeys,
      );
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
      this.store.completeRunWithKnowledge(
        runId,
        result,
        coverage,
        buildCandidateEvaluations(
          run,
          filtered.observation,
          result,
          invocation.evaluatedEvidenceKeys,
        ),
      );
    } catch (error) {
      if (error.reasoningTelemetry) this.store.saveReasoningInvocation(error.reasoningTelemetry);
      const stage = error.name === "SourceReadinessError" ? "source_readiness" : "reasoning";
      this.store.failRun(runId, stage, error);
      this.#advanceParentSessionForRun(runId);
      throw error;
    }
    this.#advanceParentSessionForRun(runId);
  }

  #advanceParentSessionForRun(runId) {
    const session = this.store.getUnifiedSessionByRunId(runId);
    if (!session || isUnifiedSessionTerminal(session.status)) return;
    this.#reconcileUnifiedSession(session.id);
  }
}

function unwrapProviderInvocation(response) {
  if (
    response &&
    typeof response === "object" &&
    Object.prototype.hasOwnProperty.call(response, "output") &&
    Object.prototype.hasOwnProperty.call(response, "telemetry")
  ) {
    return response;
  }
  return { output: response, telemetry: null, evaluatedEvidenceKeys: null };
}

export function decideAcquisitionPlanning({
  policy = "always",
  observation,
  unseenEvidenceCount,
}) {
  if (policy !== "deterministic_sparse_gap") {
    return { invokeProvider: true, reason: "Provider planning policy is always." };
  }
  if (unseenEvidenceCount <= 0) {
    return {
      invokeProvider: false,
      reason: "Deterministic planning gate: no unseen evidence requires another viewport.",
    };
  }
  if (unseenEvidenceCount >= 3) {
    return {
      invokeProvider: false,
      reason: `Deterministic planning gate: ${unseenEvidenceCount} unseen candidates are sufficient for bounded evaluation.`,
    };
  }
  const coverage = observation.coverage ?? {};
  if (
    coverage.scrollStopReason !== "budget_exhausted" ||
    !Number.isFinite(coverage.requestedScrolls) ||
    !Number.isFinite(coverage.performedScrolls) ||
    coverage.performedScrolls < coverage.requestedScrolls
  ) {
    return {
      invokeProvider: false,
      reason: "Deterministic planning gate: browser movement cannot justify another bounded viewport.",
    };
  }
  return {
    invokeProvider: true,
    reason: `Deterministic planning gate: sparse sample (${unseenEvidenceCount}) may justify one anchored follow-up.`,
  };
}

function buildCandidateEvaluations(run, observation, result, evaluatedEvidenceKeys = null) {
  const selectedByEvidence = new Map(result.items.map((item) => [item.evidenceKey, item]));
  const assessmentByEvidence = new Map(
    (result.candidateAssessments ?? []).map((assessment) => [assessment.evidenceKey, assessment]),
  );
  const evaluated = Array.isArray(evaluatedEvidenceKeys)
    ? new Set(evaluatedEvidenceKeys)
    : null;
  const unique = new Map();
  for (const block of observation.snapshots.flatMap((snapshot) => snapshot.blocks)) {
    if (
      !block.evidenceKey ||
      unique.has(block.evidenceKey) ||
      (evaluated && !evaluated.has(block.evidenceKey))
    ) continue;
    unique.set(block.evidenceKey, block);
  }
  return [...unique.values()].map((block) => {
    const item = selectedByEvidence.get(block.evidenceKey) ?? null;
    const assessment = assessmentByEvidence.get(block.evidenceKey) ?? null;
    return {
      evidenceKey: block.evidenceKey,
      source: run.source,
      decision: item ? "selected" : "excluded",
      reasonCode: item ? "selected_by_provider" : "not_promoted_by_provider",
      itemId: item?.id ?? null,
      author: block.author ?? "",
      text: block.text ?? "",
      sourceUrl: block.permalink || observation.pageUrl,
      media: block.media ?? [],
      publishedAt: block.publishedAt ?? null,
      feedPosition: Number.isInteger(block.feedPosition) ? block.feedPosition : null,
      policyVersion: "learning-loop-v0",
      preferenceProfileVersion: 0,
      assessment: assessment
        ? {
            topicTags: assessment.topicTags,
            contentType: assessment.contentType,
            recommendedPriority: assessment.recommendedPriority,
            intentRelevance: assessment.intentRelevance,
            novelty: assessment.novelty,
            urgency: assessment.urgency,
            actionability: assessment.actionability,
            rationale: assessment.rationale,
          }
        : null,
    };
  });
}

function assertCandidateAssessments(result, observation, evaluatedEvidenceKeys) {
  const observed = new Set(uniqueEvidenceKeys(observation));
  const assessed = new Set();
  for (const assessment of result.candidateAssessments ?? []) {
    if (!observed.has(assessment.evidenceKey)) {
      throw new ContractError(
        `candidate assessment referenced evidence outside the current observation: ${assessment.evidenceKey}`,
      );
    }
    if (assessed.has(assessment.evidenceKey)) {
      throw new ContractError(`candidate assessment duplicated evidence: ${assessment.evidenceKey}`);
    }
    assessed.add(assessment.evidenceKey);
  }
  if (Array.isArray(evaluatedEvidenceKeys)) {
    const expected = new Set(evaluatedEvidenceKeys);
    if (
      expected.size !== assessed.size ||
      [...expected].some((evidenceKey) => !assessed.has(evidenceKey))
    ) {
      throw new ContractError(
        "candidate assessments must cover every evidence block supplied to the reasoning provider",
      );
    }
  }
}

export function buildUnifiedSessionOutcome(session, status = session.status) {
  const completedChildren = session.children.filter(
    (child) => child.run?.status === "completed",
  );
  const mergedItems = mergeUnifiedItems(session.id, completedChildren, session.maxItemsTotal);
  const completedSources = completedChildren.map((child) => child.source);
  const failedSources = session.children
    .filter((child) => child.status === "failed")
    .map((child) => child.source);
  const cancelledSources = session.children
    .filter((child) => child.status === "cancelled")
    .map((child) => child.source);
  const limitations = completedChildren.flatMap((child) =>
    (child.run.result?.limitations ?? []).map(
      (limitation) => `${sourceLabel(child.source)}: ${limitation}`,
    ),
  );
  if (status !== "completed") {
    limitations.push("The unified brief is partial because not every requested source completed.");
  }
  const result = {
    summary: `${mergedItems.length} material item${mergedItems.length === 1 ? "" : "s"} from ${completedSources.length} completed source${completedSources.length === 1 ? "" : "s"}.`,
    items: mergedItems,
    repeatedClaimsCollapsed: completedChildren.reduce(
      (sum, child) => sum + (child.run.result?.repeatedClaimsCollapsed ?? 0),
      0,
    ),
    deferredByBudget: completedChildren.reduce(
      (sum, child) => sum + (child.run.result?.deferredByBudget ?? 0),
      0,
    ),
    limitations,
  };
  const coverage = {
    status,
    requestedSources: [...session.sources],
    completedSources,
    failedSources,
    cancelledSources,
    resultCount: mergedItems.length,
    resultCountBySource: Object.fromEntries(
      session.children.map((child) => [child.source, child.run?.result?.items?.length ?? 0]),
    ),
    children: session.children.map((child) => ({
      source: child.source,
      runId: child.runId,
      status: child.status,
      coverage: child.run?.coverage ?? null,
    })),
    scopeStatement:
      "Unified brief over bounded X and LinkedIn samples; this is not a claim of complete feed coverage.",
  };
  return { result, coverage };
}

export function mergeUnifiedItems(sessionId, completedChildren, maxItemsTotal = 10) {
  const priorities = ["P1", "P2", "P3", "P4"];
  const merged = [];
  for (const priority of priorities) {
    const queues = completedChildren.map((child) => ({
      child,
      items: (child.run.result?.items ?? []).filter((item) => item.priority === priority),
      index: 0,
    }));
    while (queues.some((queue) => queue.index < queue.items.length)) {
      for (const queue of queues) {
        const item = queue.items[queue.index];
        if (!item) continue;
        merged.push({ sessionId, runId: queue.child.run.id, item });
        queue.index += 1;
        if (merged.length >= maxItemsTotal) return merged;
      }
    }
  }
  return merged;
}

function canRetryPendingContentDetectOnly(command, error) {
  return (
    command.payload.pendingContentPolicy === "reveal_if_present" &&
    command.payload.acquisitionRound === 1 &&
    !command.payload.pendingContentRecovery &&
    /pending-content control did not reveal a changed, visible feed within the bounded deadline/i.test(
      error.message,
    )
  );
}

function runHasObservedEvidence(run) {
  return (run.observations ?? []).some(
    (observation) => uniqueEvidenceKeys(observation.payload).length > 0,
  );
}

function isRunTerminal(status) {
  return ["completed", "failed", "cancelled"].includes(status);
}

function isUnifiedSessionTerminal(status) {
  return ["completed", "partial", "failed", "cancelled"].includes(status);
}

function sourceLabel(source) {
  return source === "x" ? "X" : "LinkedIn";
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
    candidateAssessments: [],
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
