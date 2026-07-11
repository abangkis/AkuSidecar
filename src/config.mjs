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

function parseReasoningEffort(value, fallback) {
  return REASONING_EFFORTS.has(value) ? value : fallback;
}

export function loadConfig(env = process.env) {
  const port = parseInteger(env.AKU_BROWSER_PORT, 47821);
  const provider = env.AKU_REASONING_PROVIDER ?? reasoningDefaults.provider ?? "deterministic";
  const sharedModel = env.AKU_CODEX_MODEL || null;

  return {
    host: "127.0.0.1",
    port,
    publicDirectory: path.join(projectRoot, "public"),
    databasePath: env.AKU_DATABASE_PATH
      ? path.resolve(env.AKU_DATABASE_PATH)
      : path.join(projectRoot, "runtime", "aku-browser.db"),
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
