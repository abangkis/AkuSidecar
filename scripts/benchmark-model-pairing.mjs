import { loadConfig } from "../src/config.mjs";
import { applyPersistedConfiguration } from "../src/configuration/runtime-configuration.mjs";
import {
  DEFAULT_LUNA_COST_RATIOS,
  PAIRED_MODEL_PROFILES,
  PAIRED_MODEL_REPORT_SETTING,
  runPairedModelReplay,
  selectPairedReplayCases,
} from "../src/core/paired-model-replay.mjs";
import { CodexSdkReasoningProvider } from "../src/reasoning/codex-sdk-provider.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const options = parseArguments(process.argv.slice(2));
const config = loadConfig();
const store = new SqliteStateStore(config.databasePath);

try {
  applyPersistedConfiguration(config, store);
  const hydratedRuns = store.listRunsWithFeedback(50).map((entry) => store.getRun(entry.id));
  const cases = selectPairedReplayCases(hydratedRuns, { limit: options.cases });
  if (cases.length < options.cases) {
    console.warn(`Only ${cases.length} eligible completed run(s) were available for ${options.cases} requested case(s).`);
  }
  const providers = new Map(PAIRED_MODEL_PROFILES.map((profile) => [
    profile.id,
    new CodexSdkReasoningProvider({
      ...config.reasoning,
      evaluationModel: profile.model,
      evaluationEffort: profile.effort,
    }),
  ]));
  const report = await runPairedModelReplay({
    cases,
    lunaCostRatios: options.lunaCostRatios,
    onProgress({ ordinal, totalInvocations, profile, runId, source }) {
      console.log(`[${ordinal}/${totalInvocations}] ${profile.id} on ${source} run ${runId}`);
    },
    async invoke({ profile, run, observation }) {
      return providers.get(profile.id).analyze({
        run,
        observation,
        knowledgeContext: { checkpoint: null, events: [] },
      });
    },
  });
  store.setSetting(PAIRED_MODEL_REPORT_SETTING, JSON.stringify(report));
  console.log(JSON.stringify(report, null, 2));
  if (report.failures.length > 0) process.exitCode = 2;
} finally {
  store.close();
}

function parseArguments(args) {
  let cases = 4;
  let positionalCasesSeen = false;
  const lunaCostRatios = [];
  for (let index = 0; index < args.length; index += 1) {
    const argument = args[index];
    if (argument === "--cases") {
      cases = boundedInteger(args[++index], 1, 6, "--cases");
      continue;
    }
    if (argument === "--luna-rate") {
      const value = Number(args[++index]);
      if (!Number.isFinite(value) || value <= 0 || value > 2) {
        throw new Error("--luna-rate must be greater than 0 and at most 2");
      }
      lunaCostRatios.push(value);
      continue;
    }
    if (/^\d+$/.test(argument) && !positionalCasesSeen) {
      cases = boundedInteger(argument, 1, 6, "case count");
      positionalCasesSeen = true;
      continue;
    }
    throw new Error(`unknown argument: ${argument}`);
  }
  return {
    cases,
    lunaCostRatios: lunaCostRatios.length > 0 ? lunaCostRatios : DEFAULT_LUNA_COST_RATIOS,
  };
}

function boundedInteger(value, minimum, maximum, label) {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isInteger(parsed) || parsed < minimum || parsed > maximum) {
    throw new Error(`${label} must be between ${minimum} and ${maximum}`);
  }
  return parsed;
}
