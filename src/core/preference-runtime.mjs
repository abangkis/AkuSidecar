import {
  fitOfflinePreferenceExperiment,
  preferenceDatasetFingerprint,
  runtimePreferenceDatasetFingerprint,
  scorePreferenceCandidate,
} from "./offline-preference-experiment.mjs";
import { preferenceFeedbackWeight } from "./preference-features.mjs";

export const PREFERENCE_RUNTIME_POLICY = Object.freeze({
  version: "preference-runtime-v2",
  baseline: "source_platform_order",
  influence: "bounded_selected_rerank",
  maxRankDisplacement: 2,
  minimumScoreDelta: 0.03,
  automaticFitFeedbackDelta: 5,
  minimumSignals: 4,
  minimumPositiveSignals: 1,
  minimumNegativeSignals: 1,
  minimumPromotionGain: 0.02,
});

const ACTIVE_SNAPSHOT_SETTING = "preference.runtime.active_snapshot_id";
const CHALLENGER_SNAPSHOT_SETTING = "preference.runtime.challenger_snapshot_id";
const SUSPENDED_SETTING = "preference.runtime.suspended";
const SUSPENDED_FINGERPRINT_SETTING = "preference.runtime.suspended_fingerprint";

export function preferenceRuntimeStatus(store, policy = {}, options = {}) {
  const effectivePolicy = { ...PREFERENCE_RUNTIME_POLICY, ...policy };
  const enabled = options.enabled !== false;
  const runs = store.listRunsWithFeedback(500);
  const legacyFingerprint = preferenceDatasetFingerprint(runs);
  const fingerprint = runtimePreferenceDatasetFingerprint(legacyFingerprint);
  const activeSnapshotId = store.getSetting(ACTIVE_SNAPSHOT_SETTING);
  const challengerSnapshotId = store.getSetting(CHALLENGER_SNAPSHOT_SETTING);
  const suspendedFingerprint = store.getSetting(SUSPENDED_FINGERPRINT_SETTING);
  const activeSnapshot = activeSnapshotId
    ? store.getPreferenceModelSnapshotById(activeSnapshotId)
    : null;
  const challengerSnapshot = challengerSnapshotId
    ? store.getPreferenceModelSnapshotById(challengerSnapshotId)
    : null;
  const current = activeSnapshot?.datasetFingerprint === fingerprint ? activeSnapshot : null;
  const suspended = store.getSetting(SUSPENDED_SETTING) === "true" ||
    suspendedFingerprint === fingerprint || suspendedFingerprint === legacyFingerprint;
  const counts = signalCounts(runs);
  const eligible = counts.total >= effectivePolicy.minimumSignals &&
    counts.positive >= effectivePolicy.minimumPositiveSignals &&
    counts.negative >= effectivePolicy.minimumNegativeSignals;

  return {
    version: 2,
    mode: "local_preference_runtime",
    enabled,
    liveInfluence: enabled && !suspended && activeSnapshot?.liveInfluence === true,
    activationState: enabled && !suspended && activeSnapshot?.liveInfluence === true
      ? "active"
      : "baseline",
    baseline: effectivePolicy.baseline,
    influence: effectivePolicy.influence,
    policy: effectivePolicy,
    datasetFingerprint: fingerprint,
    signalCounts: counts,
    eligibleForPersonalFit: eligible,
    activeSnapshot,
    challengerSnapshot,
    currentSnapshot: current,
    staleSnapshot: activeSnapshot && !current ? activeSnapshot : null,
    suspended,
    fallback: activeSnapshot ? null : "source_platform_order",
  };
}

export function ensureLocalPreferenceRuntime(store, policy = {}, options = {}) {
  const effectivePolicy = { ...PREFERENCE_RUNTIME_POLICY, ...policy };
  const before = preferenceRuntimeStatus(store, effectivePolicy, options);
  if (!before.enabled || !before.eligibleForPersonalFit) return before;
  const forceFit = options.forceFit === true || options.force === true;
  if (before.suspended && options.ignoreSuspension !== true) return before;
  if (before.currentSnapshot) return before;

  const previousSignals = before.activeSnapshot?.dataset?.assessedFeedback ?? 0;
  const newSignals = Math.max(0, before.signalCounts.total - previousSignals);
  if (
    !forceFit &&
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
  const promote = !before.activeSnapshot || shouldPromoteSnapshot(
    before.activeSnapshot,
    snapshot,
    effectivePolicy,
  );
  if (promote) {
    store.setSetting(ACTIVE_SNAPSHOT_SETTING, snapshot.id);
    store.setSetting(CHALLENGER_SNAPSHOT_SETTING, "");
  } else {
    store.setSetting(CHALLENGER_SNAPSHOT_SETTING, snapshot.id);
  }
  store.setSetting(SUSPENDED_SETTING, "false");
  store.setSetting(SUSPENDED_FINGERPRINT_SETTING, "");
  return {
    ...preferenceRuntimeStatus(store, effectivePolicy, options),
    fitTrigger: options.trigger ?? "automatic",
    promotionDecision: promote ? "promoted" : "challenger_retained",
  };
}

export function resetLocalPreferenceRuntime(store, policy = {}, options = {}) {
  const status = preferenceRuntimeStatus(store, policy, options);
  store.setSetting(ACTIVE_SNAPSHOT_SETTING, "");
  store.setSetting(CHALLENGER_SNAPSHOT_SETTING, "");
  store.setSetting(SUSPENDED_SETTING, "true");
  store.setSetting(SUSPENDED_FINGERPRINT_SETTING, status.datasetFingerprint);
  return preferenceRuntimeStatus(store, policy, options);
}

export function composePreferenceOrder(entries, runtime) {
  const policy = runtime?.policy ?? PREFERENCE_RUNTIME_POLICY;
  const snapshot = runtime?.activeSnapshot ?? runtime?.currentSnapshot;
  const enabled = runtime?.liveInfluence === true && snapshot?.model;
  const authority = enabled ? preferenceAuthority(snapshot, policy) : 0;
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

  if (enabled && authority > 0) {
    for (let pass = 0; pass < authority; pass += 1) {
      for (let index = 0; index < scored.length - 1; index += 1) {
        const left = scored[index];
        const right = scored[index + 1];
        if (!Number.isFinite(left.probability) || !Number.isFinite(right.probability)) continue;
        if (right.probability - left.probability < policy.minimumScoreDelta) continue;
        if (Math.abs((index + 1) - left.baselineIndex) > authority) continue;
        if (Math.abs(index - right.baselineIndex) > authority) continue;
        scored[index] = right;
        scored[index + 1] = left;
      }
    }
  }

  enforceSourceDiversity(scored, authority);
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
        authority,
      },
    };
  });
}

function preferenceAuthority(snapshot, policy) {
  const score = snapshot?.evaluation?.balancedAccuracy;
  if (!Number.isFinite(score)) return Math.min(1, policy.maxRankDisplacement);
  if (score >= 0.65) return Math.min(2, policy.maxRankDisplacement);
  if (score >= 0.5) return Math.min(1, policy.maxRankDisplacement);
  return 0;
}

function enforceSourceDiversity(entries, authority) {
  if (authority <= 0) return;
  for (let index = 2; index < entries.length; index += 1) {
    const source = entrySource(entries[index]);
    if (!source || entrySource(entries[index - 1]) !== source || entrySource(entries[index - 2]) !== source) continue;
    const replacement = entries.findIndex((entry, candidateIndex) =>
      candidateIndex > index && entrySource(entry) !== source &&
      Math.abs(index - entry.baselineIndex) <= authority &&
      Math.abs(candidateIndex - entries[index].baselineIndex) <= authority
    );
    if (replacement > index) [entries[index], entries[replacement]] = [entries[replacement], entries[index]];
  }
}

function entrySource(entry) {
  return entry.preferenceCandidate?.source ?? entry.item?.source ?? null;
}

function signalCounts(runs) {
  const latest = new Map();
  for (const run of runs) {
    const candidates = new Map(
      (run.candidateEvaluations ?? []).map((candidate) => [candidate.evidenceKey, candidate]),
    );
    for (const feedback of run.preferenceFeedback ?? []) {
      if (!candidates.get(feedback.evidenceKey)?.assessment) continue;
      if (preferenceFeedbackWeight(feedback) <= 0) continue;
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

function shouldPromoteSnapshot(champion, challenger, policy) {
  if ((champion.version ?? 0) < (challenger.version ?? 0)) return true;
  const championScore = champion.evaluation?.balancedAccuracy;
  const challengerScore = challenger.evaluation?.balancedAccuracy;
  if (!Number.isFinite(championScore)) return true;
  if (!Number.isFinite(challengerScore)) return false;
  const championNegative = champion.evaluation?.negativeRecall;
  const challengerNegative = challenger.evaluation?.negativeRecall;
  if (
    Number.isFinite(championNegative) && Number.isFinite(challengerNegative) &&
    challengerNegative + 0.05 < championNegative
  ) return false;
  return challengerScore >= championScore + policy.minimumPromotionGain;
}
