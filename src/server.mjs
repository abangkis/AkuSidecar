import { loadConfig } from "./config.mjs";
import { applyPersistedConfiguration } from "./configuration/runtime-configuration.mjs";
import { createAkuBrowserApp } from "./http/app.mjs";
import { attachViteFrontend } from "./http/vite-frontend.mjs";
import { createReasoningProvider } from "./reasoning/provider-factory.mjs";
import { SqliteStateStore } from "./store/sqlite-state-store.mjs";

const config = loadConfig();
const viteDevelopment = process.argv.includes("--vite");
const store = new SqliteStateStore(config.databasePath);
applyPersistedConfiguration(config, store);
const reasoningProvider = await createReasoningProvider(config.reasoning);
const app = createAkuBrowserApp({
  config,
  store,
  reasoningProvider,
  enforceBridgeCompatibility: true,
});

try {
  if (viteDevelopment) await attachViteFrontend(app, config);
  const address = await app.start();
  console.log(`AkuBrowser running at http://${address.address}:${address.port}`);
  console.log(`Reasoning provider: ${reasoningProvider.name}`);
  console.log(`Frontend: ${viteDevelopment ? "Vite HMR" : "static production assets"}`);
} catch (error) {
  await app.stop();
  store.close();
  throw error;
}

let shuttingDown = false;
async function shutdown() {
  if (shuttingDown) return;
  shuttingDown = true;
  await app.stop();
  store.close();
}

process.on("SIGINT", async () => {
  await shutdown();
  process.exit(0);
});
process.on("SIGTERM", async () => {
  await shutdown();
  process.exit(0);
});
