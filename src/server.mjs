import { loadConfig } from "./config.mjs";
import { createAkuBrowserApp } from "./http/app.mjs";
import { createReasoningProvider } from "./reasoning/provider-factory.mjs";
import { SqliteStateStore } from "./store/sqlite-state-store.mjs";

const config = loadConfig();
const store = new SqliteStateStore(config.databasePath);
const reasoningProvider = await createReasoningProvider(config.reasoning);
const app = createAkuBrowserApp({ config, store, reasoningProvider });

try {
  const address = await app.start();
  console.log(`AkuBrowser running at http://${address.address}:${address.port}`);
  console.log(`Reasoning provider: ${reasoningProvider.name}`);
} catch (error) {
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
