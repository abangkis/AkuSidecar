import fs from "node:fs";
import path from "node:path";
import { loadConfig } from "../src/config.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";
import {
  buildPilotDatasetExport,
  createSqliteBackup,
  inspectSqliteDatabase,
  previewRetention,
  resetSqliteForOnboarding,
} from "../src/store/sqlite-operations.mjs";

const [command = "health", ...args] = process.argv.slice(2);
const config = loadConfig();

if (command === "health") {
  print(inspectSqliteDatabase(config.databasePath));
} else if (command === "backup") {
  const target = requiredOption(args, "--output");
  print(createSqliteBackup(config.databasePath, target));
} else if (command === "export") {
  const target = path.resolve(requiredOption(args, "--output"));
  if (fs.existsSync(target)) throw new Error("export target already exists");
  const store = new SqliteStateStore(config.databasePath);
  try {
    const payload = buildPilotDatasetExport(store.listRunsWithFeedback(501));
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.writeFileSync(target, `${JSON.stringify(payload, null, 2)}\n`, "utf8");
    print({ target, runs: payload.runs.length, containsRawObservations: false });
  } finally {
    store.close();
  }
} else if (command === "retention-preview") {
  const days = Number.parseInt(option(args, "--days") ?? "90", 10);
  const store = new SqliteStateStore(config.databasePath);
  try {
    print(previewRetention(store.listRunsWithFeedback(501), { olderThanDays: days }));
  } finally {
    store.close();
  }
} else if (command === "reset-onboarding") {
  const target = option(args, "--backup-output") ?? args[0];
  const confirmation = option(args, "--confirm") ?? args[1];
  if (!target || !confirmation) {
    throw new Error("reset-onboarding requires <backup-output> RESET_ONBOARDING");
  }
  print(resetSqliteForOnboarding(config.databasePath, target, confirmation));
} else {
  throw new Error(`unknown sqlite operation: ${command}`);
}

function requiredOption(args, name) {
  const value = option(args, name);
  if (!value) throw new Error(`${name} is required`);
  return value;
}

function option(args, name) {
  const index = args.indexOf(name);
  return index >= 0 ? args[index + 1] : null;
}

function print(value) {
  console.log(JSON.stringify(value, null, 2));
}
