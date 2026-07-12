import path from "node:path";
import fs from "node:fs";
import { fileURLToPath } from "node:url";

const currentDirectory = path.dirname(fileURLToPath(import.meta.url));
export const projectRoot = path.resolve(currentDirectory, "..");
const reasoningDefaults = loadReasoningDefaults();

function parseInteger(value, fallback) {
  if (value === undefined || value === "") return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

const REASONING_EFFORTS = new Set(["minimal", "low", "medium", "high", "xhigh"]);
const MISSING_SOURCE_TAB_POLICIES = new Set(["open_missing_tab", "fail_fast"]);
const REASONING_PROVIDERS = new Set(["deterministic", "codex-sdk"]);
const PLANNING_POLICIES = new Set(["always", "deterministic_sparse_gap"]);

function parseReasoningEffort(value, fallback) {
  return REASONING_EFFORTS.has(value) ? value : fallback;
}

function parseMissingSourceTabPolicy(value, fallback = "open_missing_tab") {
  return MISSING_SOURCE_TAB_POLICIES.has(value) ? value : fallback;
}

export function loadConfig(env = process.env) {
  const port = parseInteger(env.AKU_BROWSER_PORT, 47821);
  const sharedModel = env.AKU_CODEX_MODEL || null;
  const missingSourceTabOverride = MISSING_SOURCE_TAB_POLICIES.has(
    env.AKU_MISSING_SOURCE_TAB_POLICY,
  )
    ? env.AKU_MISSING_SOURCE_TAB_POLICY
    : null;
  const providerOverride = REASONING_PROVIDERS.has(env.AKU_REASONING_PROVIDER)
    ? env.AKU_REASONING_PROVIDER
    : null;
  const provider = providerOverride ?? reasoningDefaults.provider ?? "deterministic";
  const planningModelOverride = env.AKU_CODEX_PLANNING_MODEL || sharedModel || null;
  const evaluationModelOverride = env.AKU_CODEX_EVALUATION_MODEL || sharedModel || null;
  const planningEffortOverride = REASONING_EFFORTS.has(env.AKU_CODEX_PLANNING_EFFORT)
    ? env.AKU_CODEX_PLANNING_EFFORT
    : null;
  const evaluationEffortOverride = REASONING_EFFORTS.has(env.AKU_CODEX_EVALUATION_EFFORT)
    ? env.AKU_CODEX_EVALUATION_EFFORT
    : null;
  const planningPolicyOverride = PLANNING_POLICIES.has(env.AKU_CODEX_PLANNING_POLICY)
    ? env.AKU_CODEX_PLANNING_POLICY
    : null;
  const parsedTimeoutOverride = env.AKU_CODEX_TIMEOUT_MS
    ? parseInteger(env.AKU_CODEX_TIMEOUT_MS, null)
    : null;
  const timeoutOverride = Number.isInteger(parsedTimeoutOverride) && parsedTimeoutOverride >= 1_000
    ? parsedTimeoutOverride
    : null;

  return {
    host: "127.0.0.1",
    port,
    publicDirectory: path.join(projectRoot, "public"),
    databasePath: env.AKU_DATABASE_PATH
      ? path.resolve(env.AKU_DATABASE_PATH)
      : path.join(projectRoot, "runtime", "aku-browser.db"),
    runtimeConfiguration: {
      missingSourceTabPolicy: {
        defaultValue: "open_missing_tab",
        environmentOverride: missingSourceTabOverride,
      },
      defaultPresentation: {
        defaultValue: "source",
        environmentOverride: null,
      },
      homePresentation: {
        defaultValue: "timeline",
        environmentOverride: null,
      },
      timelineCapacity: {
        defaultValue: 12,
        environmentOverride: null,
      },
      streamWidth: {
        defaultValue: "social",
        environmentOverride: null,
      },
      telemetryBehavior: {
        defaultValue: "flow",
        environmentOverride: null,
      },
      activeSources: {
        defaultValue: ["x", "linkedin"],
        environmentOverride: null,
      },
      calibrationEnabled: {
        defaultValue: true,
        environmentOverride: null,
      },
      calibrationBatchSize: {
        defaultValue: 10,
        environmentOverride: null,
      },
      maxItemsPerSource: {
        defaultValue: 5,
        environmentOverride: null,
      },
      maxScrolls: {
        defaultValue: 2,
        environmentOverride: null,
      },
      maxAcquisitionRounds: {
        defaultValue: 2,
        environmentOverride: null,
      },
      maxKnowledgeContextEvents: {
        defaultValue: 20,
        environmentOverride: null,
      },
      reasoningProvider: {
        defaultValue: reasoningDefaults.provider ?? "deterministic",
        environmentOverride: providerOverride,
      },
      planningModel: {
        defaultValue: reasoningDefaults.acquisitionPlanning?.model ?? null,
        environmentOverride: planningModelOverride,
      },
      evaluationModel: {
        defaultValue: reasoningDefaults.candidateEvaluation?.model ?? null,
        environmentOverride: evaluationModelOverride,
      },
      planningEffort: {
        defaultValue: reasoningDefaults.acquisitionPlanning?.effort ?? "low",
        environmentOverride: planningEffortOverride,
      },
      evaluationEffort: {
        defaultValue: reasoningDefaults.candidateEvaluation?.effort ?? "low",
        environmentOverride: evaluationEffortOverride,
      },
      planningPolicy: {
        defaultValue: reasoningDefaults.acquisitionPlanning?.policy ?? "always",
        environmentOverride: planningPolicyOverride,
      },
      timeoutMs: {
        defaultValue: 120_000,
        environmentOverride: timeoutOverride,
      },
    },
    presentation: {
      defaultLayout: "source",
      homePresentation: "timeline",
      timelineCapacity: 12,
      streamWidth: "social",
      telemetryBehavior: "flow",
    },
    sources: {
      active: ["x", "linkedin"],
    },
    calibration: {
      enabled: true,
      triggerPolicy: "first_run",
      batchSize: 10,
      maxItemsPerSource: 5,
      liveInfluence: false,
    },
    reasoning: {
      provider,
      model: sharedModel || reasoningDefaults.candidateEvaluation?.model || null,
      planningModel:
        env.AKU_CODEX_PLANNING_MODEL ||
        sharedModel ||
        reasoningDefaults.acquisitionPlanning?.model ||
        null,
      evaluationModel:
        env.AKU_CODEX_EVALUATION_MODEL ||
        sharedModel ||
        reasoningDefaults.candidateEvaluation?.model ||
        null,
      planningEffort: parseReasoningEffort(
        env.AKU_CODEX_PLANNING_EFFORT,
        reasoningDefaults.acquisitionPlanning?.effort || "low",
      ),
      evaluationEffort: parseReasoningEffort(
        env.AKU_CODEX_EVALUATION_EFFORT,
        reasoningDefaults.candidateEvaluation?.effort || "low",
      ),
      planningPolicy:
        env.AKU_CODEX_PLANNING_POLICY ||
        reasoningDefaults.acquisitionPlanning?.policy ||
        "always",
      codexPathOverride: env.AKU_CODEX_PATH || undefined,
      timeoutMs: parseInteger(env.AKU_CODEX_TIMEOUT_MS, 120_000),
      schemaPath: path.join(projectRoot, "schemas", "reasoning-result.schema.json"),
      acquisitionPlanSchemaPath: path.join(
        projectRoot,
        "schemas",
        "acquisition-plan.schema.json",
      ),
      workingDirectory: projectRoot,
    },
    limits: {
      missingSourceTabPolicy: parseMissingSourceTabPolicy(
        missingSourceTabOverride,
      ),
      acquisitionPlanningPolicy:
        env.AKU_CODEX_PLANNING_POLICY ||
        reasoningDefaults.acquisitionPlanning?.policy ||
        "always",
      maxBodyBytes: 1_000_000,
      maxItems: 5,
      maxScrolls: 2,
      maxAcquisitionRounds: 2,
      followUpScrolls: 1,
      maxContinuationAnchors: 3,
      maxKnowledgeContextEvents: 20,
      defaultScrolls: 2,
      scrollFraction: 0.75,
      scrollSettleMs: 900,
      captureTimeoutMs: 45_000,
      pendingContentTimeoutMs: 5_000,
      pendingContentSettleMs: 700,
      maxBlocksPerSnapshot: 20,
      maxBlockCharacters: 4_000,
      maxMediaPerBlock: 4,
    },
  };
}

function loadReasoningDefaults() {
  const file = path.join(projectRoot, "config", "reasoning.json");
  try {
    const value = JSON.parse(fs.readFileSync(file, "utf8"));
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}
