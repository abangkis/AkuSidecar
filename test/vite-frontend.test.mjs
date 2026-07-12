import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { projectRoot } from "../src/config.mjs";
import { createAkuBrowserApp } from "../src/http/app.mjs";
import { attachViteFrontend } from "../src/http/vite-frontend.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

test("Vite middleware and the Sidecar API share one HTTP port", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-sidecar-vite-"));
  const databasePath = path.join(directory, "state.db");
  const store = new SqliteStateStore(databasePath);
  const config = {
    host: "127.0.0.1",
    port: 0,
    publicDirectory: path.join(projectRoot, "public"),
    databasePath,
    presentation: { defaultLayout: "source", streamWidth: "social" },
    limits: {
      maxBodyBytes: 1_000_000,
      maxItems: 5,
      maxScrolls: 2,
      maxAcquisitionRounds: 2,
      followUpScrolls: 1,
      maxContinuationAnchors: 3,
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
  const app = createAkuBrowserApp({
    config,
    store,
    reasoningProvider: { name: "vite-test-provider" },
    logger: { error() {} },
  });
  context.after(async () => {
    await app.stop();
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });

  await attachViteFrontend(app, config);
  const address = await app.start();
  const origin = `http://127.0.0.1:${address.port}`;

  const htmlResponse = await fetch(`${origin}/`);
  const html = await htmlResponse.text();
  const appScript = await (await fetch(`${origin}/app.js`)).text();
  const styles = await (await fetch(`${origin}/styles.css`)).text();
  const bootstrap = await (await fetch(`${origin}/api/bootstrap`)).json();
  const shadowComparison = await (
    await fetch(`${origin}/api/preferences/shadow-comparison`)
  ).json();
  const databaseHealthResponse = await fetch(`${origin}/api/operations/database/health`);
  const databaseHealth = await databaseHealthResponse.json();

  assert.equal(htmlResponse.status, 200);
  assert.match(html, /\/\@vite\/client/);
  assert.match(html, /Unified X \+ LinkedIn/);
  assert.match(html, /Advanced single source/);
  assert.match(appScript, /startExternalSessionDiscovery/);
  assert.match(appScript, /\/api\/sessions\/active/);
  assert.match(appScript, /REVIEW_PAGE_SIZE = 10/);
  assert.match(appScript, /REVIEW_MAX_RUNS = 50/);
  assert.match(appScript, /IntersectionObserver/);
  assert.match(appScript, /appendPilotRunGroups/);
  assert.match(appScript, /sourceReviewOrder/);
  assert.match(appScript, /sortReviewGroupCards/);
  assert.match(html, /review-scroll-sentinel/);
  assert.match(html, /class="review-layout"/);
  assert.match(html, /class="review-stream"/);
  assert.match(html, /class="review-telemetry"/);
  assert.match(html, /class="review-telemetry" aria-labelledby="telemetry-heading"/);
  assert.match(styles, /grid-template-columns: minmax\(0, var\(--stream-width\)\) minmax\(340px, 380px\)/);
  assert.match(styles, /\.review-telemetry \{[^}]*overflow-y: auto/s);
  assert.match(html, /form="runtime-settings-form">Save settings/);
  assert.match(styles, /@media \(max-width: 1050px\)/);
  assert.match(appScript, /mountPilotRunBody/);
  assert.match(appScript, /unmountPilotRunBody/);
  assert.match(appScript, /buildItemPresentation/);
  assert.match(appScript, /buildSourceLayoutMedia/);
  assert.match(appScript, /referrerPolicy = "no-referrer"/);
  assert.match(appScript, /fitPreferenceExperiment/);
  assert.match(html, /Offline preference experiment/);
  assert.match(html, /Shadow comparison/);
  assert.match(html, /default-presentation/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /ws:\/\/127\.0\.0\.1/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /https:\/\/pbs\.twimg\.com/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /https:\/\/\*\.licdn\.com/);
  assert.equal(bootstrap.provider, "vite-test-provider");
  assert.equal(bootstrap.presentation.defaultLayout, "source");
  assert.equal(bootstrap.presentation.streamWidth, "social");
  assert.equal(bootstrap.unifiedSession.maxItemsTotal, 10);
  assert.equal(shadowComparison.comparison.available, false);
  assert.equal(shadowComparison.comparison.liveInfluence, false);
  assert.equal(databaseHealth.database.status, "healthy");
  assert.equal(path.basename(databaseHealth.database.databasePath), "state.db");
  assert.equal(JSON.stringify(databaseHealth).includes(directory), false);
  assert.equal(JSON.stringify(databaseHealth).includes("bridge_token"), false);
  assert.equal(databaseHealthResponse.headers.has("access-control-allow-origin"), false);
});
