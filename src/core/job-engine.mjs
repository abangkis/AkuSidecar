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
import { admitObservationQuality } from "../browser/observation-quality-policy.mjs";
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
import {
  composePreferenceOrder,
  ensureLocalPreferenceRuntime,
  preferenceRuntimeStatus,
  resetLocalPreferenceRuntime,
} from "./preference-runtime.mjs";
import { buildPilotReview } from "./pilot-review.mjs";

export class JobEngine {
  constructor({ store, reasoningProvider, limits, preferencePolicy = {}, logger = console }) {
    this.store = store;
    this.reasoningProvider = reasoningProvider;
    this.limits = limits;
    this.preferencePolicy = preferencePolicy;
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
    this.#ensurePreferenceRuntime({ force: true, trigger: "before_session" });
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

  getTimelineSessions({ limit = 10, offset = 0 } = {}) {
    const sessions = this.store
      .listPresentableUnifiedSessions(limit, offset)
      .map(projectUnifiedSessionForPresentation);
    const total = this.store.countPresentableUnifiedSessions();
    return {
      sessions,
      pagination: {
        total,
        offset,
        limit,
        returned: sessions.length,
        hasNext: offset + sessions.length < total,
      },
    };
  }

  getTimelineFeed({ capacity = 12, limit = 12, offset = 0 } = {}) {
    const sessions = this.store
      .listPresentableUnifiedSessions(50, 0)
      .map(projectUnifiedSessionForPresentation);
    const latestSession = sessions[0] ?? null;
    const olderEvidence = new Set(
      sessions.slice(1).flatMap((session) =>
        (session.result?.items ?? []).map((resultEntry) =>
          timelineEvidenceIdentity(session, resultEntry),
        ),
      ),
    );
    const latestEvidence = new Set();
    let latestAdditions = 0;
    for (const resultEntry of latestSession?.result?.items ?? []) {
      const identity = timelineEvidenceIdentity(latestSession, resultEntry);
      if (latestEvidence.has(identity)) continue;
      latestEvidence.add(identity);
      if (!olderEvidence.has(identity)) latestAdditions += 1;
    }
    const entries = [];
    const seenEvidence = new Set();
    for (const session of sessions) {
      for (const resultEntry of session.result?.items ?? []) {
        const item = resultEntry.item;
        const evidenceIdentity = timelineEvidenceIdentity(session, resultEntry);
        if (seenEvidence.has(evidenceIdentity)) continue;
        const child = session.children.find((candidate) => candidate.runId === resultEntry.runId);
        if (!child?.run) continue;
        seenEvidence.add(evidenceIdentity);
        entries.push({
          sessionId: session.id,
          sessionMode: session.mode,
          sessionCompletedAt: session.completedAt,
          runId: child.run.id,
          source: child.source,
          item,
          run: child.run,
          isLatestAddition:
            session.id === latestSession?.id && !olderEvidence.has(evidenceIdentity),
        });
        if (entries.length >= capacity) break;
      }
      if (entries.length >= capacity) break;
    }
    const page = entries.slice(offset, offset + limit);
    return {
      version: 1,
      capacity,
      entries: page,
      summary: {
        retained: entries.length,
        sessionsScanned: sessions.length,
        newestSessionAt: latestSession?.completedAt ?? null,
        latestSessionId: latestSession?.id ?? null,
        latestSessionStatus: latestSession?.status ?? null,
        latestAdditions,
        sources: Object.fromEntries(["x", "linkedin"].map((source) => [
          source,
          entries.filter((entry) => entry.source === source).length,
        ])),
      },
      pagination: {
        total: entries.length,
        offset,
        limit,
        returned: page.length,
        hasNext: offset + page.length < entries.length,
      },
    };
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
    const outcome = buildUnifiedSessionOutcome(session, status, {
      preferenceRuntime: this.getPreferenceRuntime(),
    });
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

    let observation = validateBridgeObservation(rawObservation, this.limits);
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
    observation = admitObservationQuality(observation, {
      required: command.payload.qualityReportRequired === true,
    });

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
    const updated = this.store.addPreferenceFeedback(runId, feedback);
    this.#ensurePreferenceRuntime({ trigger: "feedback_threshold" });
    return updated;
  }

  getPreferenceRuntime() {
    return preferenceRuntimeStatus(this.store, this.preferencePolicy, {
      enabled: this.preferencePolicy.enabled !== false,
    });
  }

  refitPreferenceRuntime() {
    return this.#ensurePreferenceRuntime({ force: true, trigger: "manual_diagnostic" });
  }

  resetPreferenceRuntime() {
    return resetLocalPreferenceRuntime(this.store, this.preferencePolicy, {
      enabled: this.preferencePolicy.enabled !== false,
    });
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

  #ensurePreferenceRuntime(options = {}) {
    return ensureLocalPreferenceRuntime(this.store, this.preferencePolicy, {
      ...options,
      enabled: this.preferencePolicy.enabled !== false,
    });
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
    const outcome = buildUnifiedSessionOutcome(session, status, {
      preferenceRuntime: this.getPreferenceRuntime(),
    });
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
      const boundedObservation = filtered.observation;
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
              observation: boundedObservation,
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
        boundedObservation,
        invocation.evaluatedEvidenceKeys,
      );
      assertPlatformOrderItems(result, invocation.evaluatedEvidenceKeys);
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
          boundedObservation,
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

function timelineEvidenceIdentity(session, resultEntry) {
  const item = resultEntry?.item ?? {};
  const child = session?.children?.find((candidate) => candidate.runId === resultEntry?.runId);
  const source = item.source || child?.source || "unknown";
  const candidate = child?.run?.candidateEvaluations?.find(
    (entry) => entry.evidenceKey === item.evidenceKey,
  );
  if (source === "linkedin" && candidate) {
    const signature = capturedContentSignature(source, candidate);
    if (signature) return `content:${signature}`;
  }
  return `${source}:${item.evidenceKey ?? ""}`;
}

function projectUnifiedSessionForPresentation(session) {
  return {
    ...session,
    children: session.children.map((child) => ({
      ...child,
      run: child.run ? projectRunForPresentation(child.run) : null,
    })),
  };
}

function projectRunForPresentation(run) {
  return {
    id: run.id,
    mode: run.mode,
    source: run.source,
    intent: run.intent,
    status: run.status,
    createdAt: run.createdAt,
    completedAt: run.completedAt,
    coverage: run.coverage,
    result: run.result,
    feedback: run.feedback ?? [],
    candidateEvaluations: run.candidateEvaluations ?? [],
    preferenceFeedback: run.preferenceFeedback ?? [],
    reasoningInvocations: run.reasoningInvocations ?? [],
  };
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
      (evaluated && !evaluated.has(block.evidenceKey))
    ) continue;
    unique.set(block.evidenceKey, mergeCapturedBlock(unique.get(block.evidenceKey), block));
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
      avatarUrl: block.avatarUrl ?? null,
      text: block.text ?? "",
      sourceUrl: block.permalink || observation.pageUrl,
      relationshipType: block.relationshipType ?? "original",
      parentPermalink: block.parentPermalink ?? null,
      quotedPost: block.quotedPost ?? null,
      media: block.media ?? [],
      links: block.links ?? [],
      engagement: block.engagement ?? {},
      presentation: block.presentation ?? {},
      publishedAt: block.publishedAt ?? null,
      feedPosition: Number.isInteger(block.feedPosition) ? block.feedPosition : null,
      policyVersion: "learning-loop-v0",
      preferenceProfileVersion: 0,
      assessment: assessment
        ? {
            topicTags: assessment.topicTags,
            contentType: assessment.contentType,
            novelty: assessment.novelty,
            urgency: assessment.urgency,
            actionability: assessment.actionability,
            rationale: assessment.rationale,
          }
        : null,
    };
  });
}

export function mergeCapturedBlock(previous, current) {
  if (!previous) return current;
  const previousMedia = previous.media ?? [];
  const currentMedia = current.media ?? [];
  const media = currentMedia.length > previousMedia.length ? currentMedia : previousMedia;
  const quotedPost = mergeQuotedPost(previous.quotedPost, current.quotedPost);
  return {
    ...previous,
    ...current,
    author: current.author || previous.author,
    avatarUrl: current.avatarUrl || previous.avatarUrl,
    text: current.text.length >= previous.text.length ? current.text : previous.text,
    permalink: current.permalink || previous.permalink,
    platformId: current.platformId || previous.platformId,
    publishedAt: current.publishedAt || previous.publishedAt,
    feedPosition: Math.min(
      ...[previous.feedPosition, current.feedPosition].filter(Number.isInteger),
    ),
    engagement: { ...(previous.engagement ?? {}), ...(current.engagement ?? {}) },
    presentation: { ...(previous.presentation ?? {}), ...(current.presentation ?? {}) },
    quotedPost,
    media,
    links: [...new Map(
      [...(previous.links ?? []), ...(current.links ?? [])].map((link) => [link.href, link]),
    ).values()].slice(0, 10),
    evidenceKey: current.evidenceKey,
  };
}

function mergeQuotedPost(previous, current) {
  if (!previous) return current ?? null;
  if (!current) return previous;
  const previousMedia = previous.media ?? [];
  const currentMedia = current.media ?? [];
  return {
    ...previous,
    ...current,
    author: current.author || previous.author || "",
    avatarUrl: current.avatarUrl || previous.avatarUrl || null,
    text: current.text.length >= previous.text.length ? current.text : previous.text,
    permalink: current.permalink || previous.permalink || null,
    publishedAt: current.publishedAt || previous.publishedAt || null,
    links: (current.links?.length ?? 0) >= (previous.links?.length ?? 0)
      ? current.links ?? []
      : previous.links ?? [],
    media: currentMedia.length >= previousMedia.length ? currentMedia : previousMedia,
  };
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

export function buildUnifiedSessionOutcome(session, status = session.status, options = {}) {
  const completedChildren = session.children.filter(
    (child) => child.run?.status === "completed",
  );
  const mergedItems = mergeUnifiedItems(
    session.id,
    completedChildren,
    session.maxItemsTotal,
    options.preferenceRuntime,
  );
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
    preferenceRuntime: summarizePreferenceComposition(mergedItems, options.preferenceRuntime),
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

export function mergeUnifiedItems(
  sessionId,
  completedChildren,
  maxItemsTotal = 10,
  preferenceRuntime = null,
) {
  const merged = [];
  const queues = completedChildren.map((child) => ({
    child,
    items: child.run.result?.items ?? [],
    index: 0,
  }));
  while (queues.some((queue) => queue.index < queue.items.length)) {
    for (const queue of queues) {
      const item = queue.items[queue.index];
      if (!item) continue;
      const preferenceCandidate = queue.child.run.candidateEvaluations?.find(
        (candidate) => candidate.evidenceKey === item.evidenceKey,
      );
      merged.push({
        sessionId,
        runId: queue.child.run.id,
        item,
        preferenceCandidate: preferenceCandidate?.assessment
          ? {
              source: queue.child.source,
              decision: preferenceCandidate.decision,
              assessment: preferenceCandidate.assessment,
            }
          : null,
      });
      queue.index += 1;
      if (merged.length >= maxItemsTotal) {
        return composePreferenceOrder(merged, preferenceRuntime);
      }
    }
  }
  return composePreferenceOrder(merged, preferenceRuntime);
}

function summarizePreferenceComposition(items, runtime) {
  const influenced = items.filter((entry) => entry.preference?.liveInfluence);
  return {
    policyVersion: runtime?.policy?.version ?? "preference-runtime-v1",
    activationState: runtime?.activationState ?? "baseline",
    liveInfluence: influenced.length > 0,
    snapshotId: runtime?.currentSnapshot?.id ?? null,
    scoredItems: items.filter((entry) => Number.isFinite(entry.preference?.probability)).length,
    movedItems: influenced.filter((entry) => entry.preference.displacement !== 0).length,
    maxObservedDisplacement: influenced.reduce(
      (maximum, entry) => Math.max(maximum, Math.abs(entry.preference.displacement)),
      0,
    ),
    eligibilityChanged: false,
  };
}

function assertPlatformOrderItems(result, evaluatedEvidenceKeys) {
  if (!Array.isArray(evaluatedEvidenceKeys)) return;
  const actual = result.items.map((item) => item.evidenceKey);
  let previousIndex = -1;
  const outOfOrder = actual.some((evidenceKey) => {
    const index = evaluatedEvidenceKeys.indexOf(evidenceKey);
    if (index < 0 || index <= previousIndex) return true;
    previousIndex = index;
    return false;
  });
  if (outOfOrder) {
    throw new ContractError(
      "result items must preserve supplied platform order",
    );
  }
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
  const snapshots = reconcileCapturedSnapshots(
    first.source,
    observations.flatMap((observation) => observation.snapshots),
  );
  return {
    source: first.source,
    pageUrl: first.pageUrl,
    pageUrls: [...new Set(observations.map((observation) => observation.pageUrl))],
    pageTitle: observations.at(-1).pageTitle,
    capturedAt: observations.at(-1).capturedAt,
    snapshots,
    coverage: {
      acquisitionRounds: observations.length,
      rounds: observations.map((observation) => observation.coverage),
    },
  };
}

export function reconcileCapturedSnapshots(source, snapshots) {
  const bestBySignature = new Map();
  for (const block of snapshots.flatMap((snapshot) => snapshot.blocks ?? [])) {
    const signature = capturedContentSignature(source, block);
    if (!signature) continue;
    const previous = bestBySignature.get(signature);
    const merged = mergeCapturedBlock(previous, block);
    bestBySignature.set(signature, previous?.permalink && !block.permalink
      ? {
          ...merged,
          permalink: previous.permalink,
          platformId: previous.platformId,
          evidenceKey: previous.evidenceKey,
          presentation: {
            ...(merged.presentation ?? {}),
            ...(previous.presentation ?? {}),
          },
        }
      : merged);
  }
  return snapshots.map((snapshot) => ({
    ...snapshot,
    blocks: (snapshot.blocks ?? []).map((block) => {
      const best = bestBySignature.get(capturedContentSignature(source, block));
      if (!best?.permalink || !best?.evidenceKey) return block;
      return {
        ...mergeCapturedBlock(block, best),
        feedPosition: block.feedPosition,
        permalink: best.permalink,
        platformId: best.platformId || block.platformId,
        evidenceKey: best.evidenceKey,
        presentation: {
          ...(block.presentation ?? {}),
          ...(best.presentation ?? {}),
        },
      };
    }),
  }));
}

function capturedContentSignature(source, block) {
  const author = String(block?.author ?? "").toLowerCase().replace(/\s+/g, " ").trim();
  const text = String(block?.text ?? "")
    .toLowerCase()
    .replace(/\s+/g, " ")
    .replace(/\s*([.,!?;:])\s*/g, "$1")
    .trim();
  if (!author || text.length < 80) return null;
  return `${source}\u0000${author}\u0000${text.slice(0, 500)}`;
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
    captureQuality: aggregateCaptureQuality(observations),
    qualityAdmission: aggregateQualityAdmission(observations),
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

export function aggregateCaptureQuality(observations) {
  const summaries = observations
    .map((observation) => observation.coverage.captureQuality)
    .filter(Boolean);
  if (summaries.length === 0) return undefined;
  const verdictCounts = sumCountMaps(summaries.map((summary) => summary.verdictCounts));
  return {
    profile: summaries[0].profile,
    verdict: worstQualityVerdict(summaries.map((summary) => summary.verdict)),
    candidateReportCount: summaries.reduce(
      (sum, summary) => sum + summary.candidateReportCount,
      0,
    ),
    verdictCounts,
    issueCounts: sumCountMaps(summaries.map((summary) => summary.issueCounts)),
    retryBudget: Math.max(...summaries.map((summary) => summary.retryBudget)),
    retryAttempts: summaries.reduce((sum, summary) => sum + summary.retryAttempts, 0),
  };
}

export function aggregateQualityAdmission(observations) {
  const admissions = observations
    .map((observation) => observation.coverage.qualityAdmission)
    .filter(Boolean);
  if (admissions.length === 0) return undefined;
  return {
    verdict: worstQualityVerdict(admissions.map((admission) => admission.verdict)),
    profile: admissions[0].profile,
    admittedBlockCount: admissions.reduce(
      (sum, admission) => sum + admission.admittedBlockCount,
      0,
    ),
    degradedBlockCount: admissions.reduce(
      (sum, admission) => sum + admission.degradedBlockCount,
      0,
    ),
    rejectedCandidateCount: admissions.reduce(
      (sum, admission) => sum + admission.rejectedCandidateCount,
      0,
    ),
    retryAttempts: admissions.reduce((sum, admission) => sum + admission.retryAttempts, 0),
    issueCounts: sumCountMaps(admissions.map((admission) => admission.issueCounts)),
  };
}

function sumCountMaps(maps) {
  const result = {};
  for (const counts of maps) {
    for (const [key, count] of Object.entries(counts ?? {})) {
      result[key] = (result[key] ?? 0) + count;
    }
  }
  return result;
}

function worstQualityVerdict(verdicts) {
  const rank = new Map([
    ["complete", 0],
    ["usable_degraded", 1],
    ["retryable", 2],
    ["invalid", 3],
  ]);
  return verdicts.reduce((worst, verdict) =>
    (rank.get(verdict) ?? 3) > (rank.get(worst) ?? 3) ? verdict : worst,
  "complete");
}
