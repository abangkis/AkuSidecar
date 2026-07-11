import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { loadConfig } from "../src/config.mjs";
import { BRIDGE_CONTRACT_VERSION, createAkuBrowserApp } from "../src/http/app.mjs";
import { createReasoningProvider } from "../src/reasoning/provider-factory.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-sidecar-smoke-"));
const config = loadConfig({
  AKU_BROWSER_PORT: "0",
  AKU_DATABASE_PATH: path.join(directory, "smoke.db"),
  AKU_REASONING_PROVIDER: "deterministic",
});
const store = new SqliteStateStore(config.databasePath);
const reasoningProvider = await createReasoningProvider(config.reasoning);
const app = createAkuBrowserApp({ config, store, reasoningProvider });

try {
  const address = await app.start();
  const origin = `http://127.0.0.1:${address.port}`;
  const bootstrap = await (await fetch(`${origin}/api/bootstrap`)).json();
  const html = await (await fetch(`${origin}/`)).text();
  assert.equal(bootstrap.bridgeContractVersion, BRIDGE_CONTRACT_VERSION);
  assert.equal(bootstrap.limits.defaultScrolls, 2);
  assert.equal(bootstrap.limits.maxAcquisitionRounds, 2);
  assert.equal(bootstrap.limits.followUpScrolls, 1);
  assert.equal(bootstrap.limits.pendingContentTimeoutMs, 5_000);
  assert.deepEqual(bootstrap.unifiedSession.sources, ["x", "linkedin"]);
  assert.equal(bootstrap.unifiedSession.maxItemsPerSource, 5);
  assert.equal(bootstrap.unifiedSession.maxItemsTotal, 10);
  assert.match(html, /<title>AkuBrowser<\/title>/);
  assert.match(html, /UNIFIED KNOWLEDGE CONTINUITY/);
  assert.match(html, /Run unified brief/);
  assert.match(html, /Pilot Review/);
  const review = await (await fetch(`${origin}/api/pilot/review`)).json();
  assert.equal(review.review.summary.totalRuns, 0);
  console.log(JSON.stringify({ status: "ok", provider: bootstrap.provider, bridgeContractVersion: bootstrap.bridgeContractVersion }));
} finally {
  await app.stop();
  store.close();
  fs.rmSync(directory, { recursive: true, force: true });
}
