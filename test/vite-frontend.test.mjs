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
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const config = {
    host: "127.0.0.1",
    port: 0,
    publicDirectory: path.join(projectRoot, "public"),
    presentation: { defaultLayout: "source" },
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
  const bootstrap = await (await fetch(`${origin}/api/bootstrap`)).json();

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
  assert.match(appScript, /mountPilotRunBody/);
  assert.match(appScript, /unmountPilotRunBody/);
  assert.match(appScript, /buildItemPresentation/);
  assert.match(appScript, /buildSourceLayoutMedia/);
  assert.match(appScript, /referrerPolicy = "no-referrer"/);
  assert.match(appScript, /fitPreferenceExperiment/);
  assert.match(html, /Offline preference experiment/);
  assert.match(html, /default-presentation/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /ws:\/\/127\.0\.0\.1/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /https:\/\/pbs\.twimg\.com/);
  assert.match(htmlResponse.headers.get("content-security-policy"), /https:\/\/\*\.licdn\.com/);
  assert.equal(bootstrap.provider, "vite-test-provider");
  assert.equal(bootstrap.presentation.defaultLayout, "source");
  assert.equal(bootstrap.unifiedSession.maxItemsTotal, 10);
});
