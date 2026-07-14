import {
  fitOfflinePreferenceExperiment,
  preferenceDatasetFingerprint,
  scorePreferenceCandidate,
} from "./offline-preference-experiment.mjs";

export const PREFERENCE_RUNTIME_POLICY = Object.freeze({
  version: "preference-runtime-v1",
  baseline: "source_platform_order",
  influence: "bounded_selected_rerank",
  maxRankDisplacement: 2,
  minimumScoreDelta: 0.03,
  automaticFitFeedbackDelta: 5,
  minimumSignals: 4,
  minimumPositiveSignals: 1,
  minimumNegativeSignals: 1,
});

const ACTIVE_SNAPSHOT_SETTING = "preference.runtime.active_snapshot_id";
const SUSPENDED_FINGERPRINT_SETTING = "preference.runtime.suspended_fingerprint";

export function preferenceRuntimeStatus(store, policy = {}, options = {}) {
  const effectivePolicy = { ...PREFERENCE_RUNTIME_POLICY, ...policy };
  const enabled = options.enabled !== false;
  const runs = store.listRunsWithFeedback(500);
  const fingerprint = preferenceDatasetFingerprint(runs);
  const activeSnapshotId = store.getSetting(ACTIVE_SNAPSHOT_SETTING);
  const suspendedFingerprint = store.getSetting(SUSPENDED_FINGERPRINT_SETTING);
  const activeSnapshot = activeSnapshotId
    ? store.getPreferenceModelSnapshotById(activeSnapshotId)
    : null;
  const current = activeSnapshot?.datasetFingerprint === fingerprint
    ? activeSnapshot
    : null;
  const suspended = suspendedFingerprint === fingerprint;
  const counts = signalCounts(runs);
  const eligible = counts.total >= effectivePolicy.minimumSignals &&
    counts.positive >= effectivePolicy.minimumPositiveSignals &&
    counts.negative >= effectivePolicy.minimumNegativeSignals;

  return {
    version: 1,
    mode: "local_preference_runtime",
    enabled,
    liveInfluence: enabled && !suspended && current?.liveInfluence === true,
    activationState: enabled && !suspended && current?.liveInfluence === true
      ? "active"
      : "baseline",
    baseline: effectivePolicy.baseline,
    influence: effectivePolicy.influence,
    policy: effectivePolicy,
    datasetFingerprint: fingerprint,
    signalCounts: counts,
    eligibleForPersonalFit: eligible,
    activeSnapshot,
    currentSnapshot: current,
    staleSnapshot: activeSnapshot && !current ? activeSnapshot : null,
    suspended,
    fallback: current ? null : "source_platform_order",
  };
}

export function ensureLocalPreferenceRuntime(store, policy = {}, options = {}) {
  const effectivePolicy = { ...PREFERENCE_RUNTIME_POLICY, ...policy };
  const before = preferenceRuntimeStatus(store, effectivePolicy, options);
  if (!before.enabled || !before.eligibleForPersonalFit) return before;
  if (before.suspended && !options.force) return before;
  if (before.currentSnapshot) return before;

  const previousSignals = before.activeSnapshot?.dataset?.assessedFeedback ?? 0;
  const newSignals = Math.max(0, before.signalCounts.total - previousSignals);
  if (
    !options.force &&
    before.activeSnapshot &&
    newSignals < effectivePolicy.automaticFitFeedbackDelta
  ) {
    return { ...before, pendingSignals: newSignals };
  }

  const fitted = fitOfflinePreferenceExperiment(store.listRunsWithFeedback(500), {
    runtime: true,
    trigger: options.trigger ?? "automatic",
  });
  if (!fitted.snapshot) return before;
  if (!safeSnapshot(fitted.snapshot)) {
    return { ...before, fitError: "snapshot_invariant_failed" };
  }
  const snapshot = store.savePreferenceModelSnapshot(fitted.snapshot);
  store.setSetting(ACTIVE_SNAPSHOT_SETTING, snapshot.id);
  store.setSetting(SUSPENDED_FINGERPRINT_SETTING, "");
  return {
    ...preferenceRuntimeStatus(store, effectivePolicy, options),
    fitTrigger: options.trigger ?? "automatic",
  };
}

export function resetLocalPreferenceRuntime(store, policy = {}, options = {}) {
  const status = preferenceRuntimeStatus(store, policy, options);
  store.setSetting(ACTIVE_SNAPSHOT_SETTING, "");
  store.setSetting(SUSPENDED_FINGERPRINT_SETTING, status.datasetFingerprint);
  return preferenceRuntimeStatus(store, policy, options);
}

export function composePreferenceOrder(entries, runtime) {
  const policy = runtime?.policy ?? PREFERENCE_RUNTIME_POLICY;
  const snapshot = runtime?.currentSnapshot;
  const enabled = runtime?.liveInfluence === true && snapshot?.model;
  const scored = entries.map((entry, baselineIndex) => {
    const candidate = entry.preferenceCandidate ?? null;
    const probability = candidate
      ? scorePreferenceCandidate(snapshot, candidate)
      : null;
    return {
      ...entry,
      baselineIndex,
      probability,
      currentIndex: baselineIndex,
    };
  });

  if (enabled) {
    for (let pass = 0; pass < policy.maxRankDisplacement; pass += 1) {
      for (let index = 0; index < scored.length - 1; index += 1) {
        const left = scored[index];
        const right = scored[index + 1];
        if (!Number.isFinite(left.probability) || !Number.isFinite(right.probability)) continue;
        if (right.probability - left.probability < policy.minimumScoreDelta) continue;
        if (Math.abs((index + 1) - left.baselineIndex) > policy.maxRankDisplacement) continue;
        if (Math.abs(index - right.baselineIndex) > policy.maxRankDisplacement) continue;
        scored[index] = right;
        scored[index + 1] = left;
      }
    }
  }

  return scored.map((entry, finalIndex) => {
    const {
      preferenceCandidate,
      baselineIndex,
      probability,
      currentIndex: _currentIndex,
      ...resultEntry
    } = entry;
    return {
      ...resultEntry,
      preference: {
        liveInfluence: enabled,
        baseline: policy.baseline,
        policyVersion: policy.version,
        snapshotId: enabled ? snapshot.id : null,
        probability: Number.isFinite(probability) ? probability : null,
        baselineIndex,
        finalIndex,
        displacement: finalIndex - baselineIndex,
      },
    };
  });
}

function signalCounts(runs) {
  const latest = new Map();
  for (const run of runs) {
    const candidates = new Map(
      (run.candidateEvaluations ?? []).map((candidate) => [candidate.evidenceKey, candidate]),
    );
    for (const feedback of run.preferenceFeedback ?? []) {
      if (!candidates.get(feedback.evidenceKey)?.assessment) continue;
      const key = `${run.source}:${feedback.evidenceKey}`;
      const value = {
        kind: feedback.kind,
        createdAt: String(feedback.createdAt ?? run.createdAt ?? ""),
        runId: String(run.id ?? ""),
      };
      const previous = latest.get(key);
      if (
        !previous ||
        value.createdAt > previous.createdAt ||
        (value.createdAt === previous.createdAt && value.runId > previous.runId)
      ) {
        latest.set(key, value);
      }
    }
  }
  const kinds = [...latest.values()].map((entry) => entry.kind);
  return {
    total: kinds.length,
    positive: kinds.filter((kind) => kind === "more_like_this").length,
    negative: kinds.filter((kind) => kind === "less_like_this").length,
  };
}

function safeSnapshot(snapshot) {
  if (!snapshot?.model || snapshot.liveInfluence !== true) return false;
  const values = [
    snapshot.model.intercept,
    ...Object.values(snapshot.model.continuous ?? {}).map((entry) => entry.weight),
    ...Object.values(snapshot.model.categorical ?? {}).flatMap((group) => Object.values(group)),
  ];
  return values.every((value) => Number.isFinite(value));
}
