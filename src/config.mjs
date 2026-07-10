import path from "node:path";
import { fileURLToPath } from "node:url";

const currentDirectory = path.dirname(fileURLToPath(import.meta.url));
export const projectRoot = path.resolve(currentDirectory, "..");

function parseInteger(value, fallback) {
  if (value === undefined || value === "") return fallback;
  const parsed = Number.parseInt(value, 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}

export function loadConfig(env = process.env) {
  const port = parseInteger(env.AKU_BROWSER_PORT, 47821);
  const provider = env.AKU_REASONING_PROVIDER ?? "deterministic";

  return {
    host: "127.0.0.1",
    port,
    publicDirectory: path.join(projectRoot, "public"),
    databasePath: env.AKU_DATABASE_PATH
      ? path.resolve(env.AKU_DATABASE_PATH)
      : path.join(projectRoot, "runtime", "aku-browser.db"),
    reasoning: {
      provider,
      codexPathOverride: env.AKU_CODEX_PATH || undefined,
      timeoutMs: parseInteger(env.AKU_CODEX_TIMEOUT_MS, 120_000),
      schemaPath: path.join(projectRoot, "schemas", "reasoning-result.schema.json"),
      workingDirectory: projectRoot,
    },
    limits: {
      maxBodyBytes: 1_000_000,
      maxItems: 5,
      maxScrolls: 2,
      defaultScrolls: 2,
      scrollFraction: 0.75,
      scrollSettleMs: 900,
      captureTimeoutMs: 45_000,
      maxBlocksPerSnapshot: 20,
      maxBlockCharacters: 4_000,
    },
  };
}
