import { validateReasoningResult } from "./contracts.mjs";
import { feedbackLaneForReason } from "./preference-features.mjs";
import { selectReasoningCandidates } from "./selection-engine.mjs";
import {
  assertCandidateAssessments,
  assertObservedSources,
  assertPlatformOrderItems,
} from "./job-engine.mjs";

export const PAIRED_MODEL_REPORT_SETTING = "diagnostic.paired_model_replay.latest";

export const PAIRED_MODEL_PROFILES = Object.freeze([
  Object.freeze({ id: "terra_high", model: "gpt-5.6-terra", effort: "high" }),
  Object.freeze({ id: "luna_high", model: "gpt-5.6-luna", effort: "high" }),
  Object.freeze({ id: "luna_xhigh", model: "gpt-5.6-luna", effort: "xhigh" }),
]);

export const DEFAULT_LUNA_COST_RATIOS = Object.freeze([0.25, 0.5, 0.75]);

export function selectPairedReplayCases(runs, { limit = 4 } = {}) {
  const boundedLimit = Math.max(1, Math.min(6, Number.parseInt(limit, 10) || 4));
  const eligible = runs
    .filter((run) =>
      run?.status === "completed" &&
      Array.isArray(run.observations) &&
      run.observations.some((entry) => entry?.payload?.snapshots?.length > 0)
    )
    .map((run) => ({
      run,
      observation: [...run.observations].reverse().find((entry) =>
        entry?.payload?.snapshots?.length > 0
      )?.payload,
      routineFeedback: latestDirectionalFeedback(run.preferenceFeedback, {
        routineOnly: true,
      }),
    }))
    .sort((left, right) =>
      right.routineFeedback.length - left.routineFeedback.length ||
      String(right.run.createdAt).localeCompare(String(left.run.createdAt))
    );

  const selected = [];
  const used = new Set();
  const sources = ["x", "linkedin"];
  while (selected.length < boundedLimit) {
    let progressed = false;
    for (const source of sources) {
      const next = eligible.find((entry) =>
        entry.run.source === source && !used.has(entry.run.id)
      );
      if (!next) continue;
      selected.push(next);
      used.add(next.run.id);
      progressed = true;
      if (selected.length >= boundedLimit) break;
    }
    if (!progressed) break;
  }
  return selected;
}

export async function runPairedModelReplay({
  cases,
  profiles = PAIRED_MODEL_PROFILES,
  invoke,
  lunaCostRatios = DEFAULT_LUNA_COST_RATIOS,
  generatedAt = new Date().toISOString(),
  onProgress = () => {},
}) {
  if (!Array.isArray(cases) || cases.length === 0) {
    throw new Error("paired model replay requires at least one completed observation case");
  }
  if (typeof invoke !== "function") throw new TypeError("paired model replay requires invoke");
  const normalizedProfiles = normalizeProfiles(profiles);
  const results = [];
  let ordinal = 0;
  const totalInvocations = cases.length * normalizedProfiles.length;

  for (const replayCase of cases) {
    for (const profile of normalizedProfiles) {
      ordinal += 1;
      onProgress({
        ordinal,
        totalInvocations,
        runId: replayCase.run.id,
        source: replayCase.run.source,
        profile,
      });
      results.push(await evaluateCase({ replayCase, profile, invoke }));
    }
  }

  const summaries = normalizedProfiles.map((profile) =>
    summarizeProfile(profile, results.filter((entry) => entry.profileId === profile.id))
  );
  const pairwise = pairwiseAgreement(normalizedProfiles, results);
  const cost = buildCostAnalysis(summaries, lunaCostRatios);
  const routing = buildRoutingRecommendations(summaries, cost);
  return {
    version: 1,
    mode: "explicit_paired_model_replay",
    generatedAt,
    mutatesTimeline: false,
    mutatesPreferenceRuntime: false,
    pricing: {
      currency: null,
      basis: "observed_input_plus_output_tokens_times_relative_profile_rate",
      note:
        "Relative scenarios are sensitivity analysis, not a monetary price claim. Supply the actual Luna-to-Terra rate ratio to evaluate a known rate card.",
    },
    cases: cases.map((entry) => ({
      runId: entry.run.id,
      source: entry.run.source,
      createdAt: entry.run.createdAt,
      candidateBlocks: uniqueEvidenceKeys(entry.observation).length,
      routineFeedback: entry.routineFeedback.length,
    })),
    caseResults: results.map(summarizeCaseResult),
    profiles: summaries,
    pairwise,
    cost,
    routing,
    failures: results.filter((entry) => entry.status === "failed").map((entry) => ({
      runId: entry.runId,
      profileId: entry.profileId,
      message: entry.error,
    })),
  };
}

function summarizeCaseResult(result) {
  if (result.status === "failed") {
    return {
      runId: result.runId,
      source: result.source,
      profileId: result.profileId,
      status: result.status,
      error: result.error,
      telemetry: result.telemetry,
    };
  }
  return {
    runId: result.runId,
    source: result.source,
    profileId: result.profileId,
    status: result.status,
    candidateCount: result.candidateCount,
    selectedCount: result.selectedCount,
    selectedKeys: result.selectedKeys,
    assessments: result.assessments,
    feedbackOutcomes: result.feedbackOutcomes,
    telemetry: result.telemetry,
  };
}

export function parsePairedModelReport(value) {
  if (!value) return null;
  try {
    const parsed = typeof value === "string" ? JSON.parse(value) : value;
    return parsed?.version === 1 && parsed?.mode === "explicit_paired_model_replay"
      ? parsed
      : null;
  } catch {
    return null;
  }
}

async function evaluateCase({ replayCase, profile, invoke }) {
  const { run, observation } = replayCase;
  try {
    const invocation = await invoke({ profile, run, observation });
    const validated = validateReasoningResult(invocation.output, run.maxItems);
    assertObservedSources(validated, observation);
    assertCandidateAssessments(
      validated,
      observation,
      invocation.evaluatedEvidenceKeys,
    );
    assertPlatformOrderItems(validated, invocation.evaluatedEvidenceKeys);
    const selected = selectReasoningCandidates(validated, run);
    const decisions = selected.selection?.decisions ?? {};
    const feedback = latestDirectionalFeedback(run.preferenceFeedback, {
      routineOnly: true,
    });
    const feedbackOutcomes = feedback.flatMap((entry) => {
      const decision = decisions[entry.evidenceKey]?.decision;
      if (!decision) return [];
      const expectedDecision = entry.kind === "more_like_this" ? "selected" : "excluded";
      return [{
        evidenceKey: entry.evidenceKey,
        kind: entry.kind,
        reasonCode: entry.reasonCode ?? null,
        lane: feedbackLaneForReason(entry.kind, entry.reasonCode),
        expectedDecision,
        actualDecision: decision,
        agreed: decision === expectedDecision,
      }];
    });
    const assessments = new Map(validated.candidateAssessments.map((entry) => [
      entry.evidenceKey,
      entry,
    ]));
    const selectedKeys = Object.entries(decisions)
      .filter(([, decision]) => decision.decision === "selected")
      .map(([evidenceKey]) => evidenceKey)
      .sort();
    return {
      runId: run.id,
      source: run.source,
      profileId: profile.id,
      status: "completed",
      candidateCount: Object.keys(decisions).length,
      selectedCount: selectedKeys.length,
      selectedKeys,
      selectedAssessment: meanAssessment(
        selectedKeys.map((key) => assessments.get(key)).filter(Boolean),
      ),
      assessments: Object.fromEntries([...assessments].map(([key, value]) => [key, {
        novelty: value.novelty,
        urgency: value.urgency,
        actionability: value.actionability,
        materiality: value.materiality,
        evidenceStrength: value.evidenceStrength,
      }])),
      feedbackOutcomes,
      telemetry: normalizeTelemetry(invocation.telemetry, profile),
    };
  } catch (error) {
    return {
      runId: run.id,
      source: run.source,
      profileId: profile.id,
      status: "failed",
      error: String(error?.message ?? error).slice(0, 500),
      telemetry: normalizeTelemetry(error?.reasoningTelemetry, profile),
    };
  }
}

function summarizeProfile(profile, results) {
  const completed = results.filter((entry) => entry.status === "completed");
  const telemetry = results.map((entry) => entry.telemetry);
  const feedback = completed.flatMap((entry) => entry.feedbackOutcomes);
  const preferenceFeedback = feedback.filter((entry) => entry.lane === "preference");
  const selectedCount = sum(completed, "selectedCount");
  const candidateCount = sum(completed, "candidateCount");
  const tokens = {
    input: sum(telemetry, "inputTokens"),
    cachedInput: sum(telemetry, "cachedInputTokens"),
    output: sum(telemetry, "outputTokens"),
    reasoningOutput: sum(telemetry, "reasoningOutputTokens"),
  };
  const tokenUnits = tokens.input + tokens.output;
  return {
    id: profile.id,
    model: profile.model,
    effort: profile.effort,
    completedCases: completed.length,
    failedCases: results.length - completed.length,
    candidateCount,
    selectedCount,
    selectionRate: ratio(selectedCount, candidateCount),
    selectedAssessment: meanAssessment(
      completed.map((entry) => entry.selectedAssessment).filter(Boolean),
    ),
    feedback: summarizeFeedback(feedback),
    preferenceFeedback: summarizeFeedback(preferenceFeedback),
    latency: {
      totalMs: sum(telemetry, "durationMs"),
      averageMs: ratio(sum(telemetry, "durationMs"), results.length),
    },
    tokens: {
      ...tokens,
      observedUnits: tokenUnits,
      averageObservedUnits: ratio(tokenUnits, completed.length),
      unitsPerCandidate: ratio(tokenUnits, candidateCount),
    },
  };
}

function buildCostAnalysis(profiles, lunaCostRatios) {
  const reference = profiles.find((profile) => profile.id === "terra_high") ?? profiles[0];
  const ratios = [...new Set(lunaCostRatios.map(Number))]
    .filter((value) => Number.isFinite(value) && value > 0 && value <= 2)
    .sort((left, right) => left - right);
  const breakEven = Object.fromEntries(profiles.map((profile) => [
    profile.id,
    {
      observedUnits: profile.tokens.observedUnits,
      breakEvenRelativeRate: profile.tokens.observedUnits > 0
        ? reference.tokens.observedUnits / profile.tokens.observedUnits
        : null,
    },
  ]));
  const scenarios = ratios.map((lunaRate) => {
    const normalized = Object.fromEntries(profiles.map((profile) => {
      const rate = profile.model.includes("luna") ? lunaRate : 1;
      const cost = profile.tokens.observedUnits * rate;
      return [profile.id, {
        relativeRate: rate,
        normalizedCost: cost,
        savingsVsTerra: reference.tokens.observedUnits > 0
          ? 1 - cost / reference.tokens.observedUnits
          : null,
      }];
    }));
    return { id: `luna_at_${String(lunaRate).replace(".", "_")}x_terra`, lunaRate, profiles: normalized };
  });
  return { referenceProfileId: reference.id, breakEven, scenarios };
}

function buildRoutingRecommendations(profiles, cost) {
  const feedbackCount = Math.max(...profiles.map((profile) => profile.feedback.total), 0);
  const completedCases = Math.min(...profiles.map((profile) => profile.completedCases));
  const confidence = feedbackCount >= 8 && completedCases >= 4
    ? "supported"
    : feedbackCount >= 4 && completedCases >= 2
      ? "provisional"
      : "insufficient";
  const scenarios = cost.scenarios.map((scenario) => {
    if (confidence === "insufficient") {
      return { scenarioId: scenario.id, profileId: null, reason: "insufficient paired feedback" };
    }
    const bestAgreement = Math.max(...profiles.map((profile) =>
      profile.feedback.agreement ?? -1
    ));
    const qualityBand = profiles.filter((profile) =>
      profile.failedCases === 0 &&
      profile.feedback.agreement !== null &&
      bestAgreement - profile.feedback.agreement <= 0.05
    );
    const selected = [...qualityBand].sort((left, right) =>
      scenario.profiles[left.id].normalizedCost - scenario.profiles[right.id].normalizedCost ||
      (right.feedback.agreement ?? 0) - (left.feedback.agreement ?? 0)
    )[0] ?? null;
    return {
      scenarioId: scenario.id,
      profileId: selected?.id ?? null,
      reason: selected
        ? "lowest normalized cost within five percentage points of the best paired feedback agreement"
        : "no complete profile inside the quality band",
    };
  });
  return {
    confidence,
    feedbackComparisons: feedbackCount,
    completedCases,
    productionMutation: false,
    phaseBoundary: {
      candidateEvaluation: "paired replay recommendation only",
      acquisitionPlanning: "not evaluated by this experiment",
    },
    scenarios,
  };
}

function pairwiseAgreement(profiles, results) {
  const entries = [];
  for (let leftIndex = 0; leftIndex < profiles.length; leftIndex += 1) {
    for (let rightIndex = leftIndex + 1; rightIndex < profiles.length; rightIndex += 1) {
      const left = profiles[leftIndex];
      const right = profiles[rightIndex];
      const leftByRun = new Map(results.filter((entry) =>
        entry.profileId === left.id && entry.status === "completed"
      ).map((entry) => [entry.runId, entry]));
      const rightByRun = new Map(results.filter((entry) =>
        entry.profileId === right.id && entry.status === "completed"
      ).map((entry) => [entry.runId, entry]));
      let matching = 0;
      let compared = 0;
      for (const [runId, leftResult] of leftByRun) {
        const rightResult = rightByRun.get(runId);
        if (!rightResult) continue;
        const keys = new Set([
          ...Object.keys(leftResult.assessments),
          ...Object.keys(rightResult.assessments),
        ]);
        const leftSelected = new Set(leftResult.selectedKeys);
        const rightSelected = new Set(rightResult.selectedKeys);
        for (const key of keys) {
          compared += 1;
          if (leftSelected.has(key) === rightSelected.has(key)) matching += 1;
        }
      }
      entries.push({
        leftProfileId: left.id,
        rightProfileId: right.id,
        comparedCandidates: compared,
        selectionAgreement: ratio(matching, compared),
      });
    }
  }
  return entries;
}

function normalizeProfiles(profiles) {
  if (!Array.isArray(profiles) || profiles.length < 2 || profiles.length > 3) {
    throw new Error("paired model replay requires two or three profiles");
  }
  const ids = new Set();
  return profiles.map((profile) => {
    const normalized = {
      id: String(profile.id ?? "").trim(),
      model: String(profile.model ?? "").trim(),
      effort: String(profile.effort ?? "").trim(),
    };
    if (!/^[a-z0-9][a-z0-9_-]{2,50}$/.test(normalized.id) || ids.has(normalized.id)) {
      throw new Error("paired model profile IDs must be unique and bounded");
    }
    if (!/^gpt-[a-z0-9._-]{3,80}$/.test(normalized.model)) {
      throw new Error(`unsupported paired model: ${normalized.model}`);
    }
    if (!["high", "xhigh"].includes(normalized.effort)) {
      throw new Error(`unsupported paired effort: ${normalized.effort}`);
    }
    ids.add(normalized.id);
    return normalized;
  });
}

function latestDirectionalFeedback(entries, { routineOnly = false } = {}) {
  const latest = new Map();
  for (const entry of entries ?? []) {
    if (routineOnly && entry.origin !== "routine") continue;
    if (!["more_like_this", "less_like_this"].includes(entry.kind)) continue;
    latest.set(entry.evidenceKey, entry);
  }
  return [...latest.values()];
}

function summarizeFeedback(entries) {
  const agreed = entries.filter((entry) => entry.agreed).length;
  return {
    total: entries.length,
    agreed,
    disagreement: entries.length - agreed,
    agreement: ratio(agreed, entries.length),
    positive: entries.filter((entry) => entry.kind === "more_like_this").length,
    negative: entries.filter((entry) => entry.kind === "less_like_this").length,
  };
}

function normalizeTelemetry(value, profile) {
  return {
    model: value?.model ?? profile.model,
    reasoningEffort: value?.reasoningEffort ?? profile.effort,
    durationMs: finite(value?.durationMs),
    inputTokens: finite(value?.inputTokens),
    cachedInputTokens: finite(value?.cachedInputTokens),
    outputTokens: finite(value?.outputTokens),
    reasoningOutputTokens: finite(value?.reasoningOutputTokens),
  };
}

function meanAssessment(entries) {
  if (entries.length === 0) return null;
  return Object.fromEntries([
    "novelty",
    "urgency",
    "actionability",
    "materiality",
    "evidenceStrength",
  ].map((field) => [field, entries.reduce((sum, entry) => sum + finite(entry[field]), 0) / entries.length]));
}

function uniqueEvidenceKeys(observation) {
  return [...new Set((observation?.snapshots ?? []).flatMap((snapshot) =>
    (snapshot.blocks ?? []).map((block) => block.evidenceKey).filter(Boolean)
  ))];
}

function finite(value) {
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function sum(entries, field) {
  return entries.reduce((total, entry) => total + finite(entry?.[field]), 0);
}

function ratio(numerator, denominator) {
  return denominator > 0 ? numerator / denominator : null;
}
