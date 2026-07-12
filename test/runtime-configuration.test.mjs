import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { loadConfig } from "../src/config.mjs";
import { createAkuBrowserApp } from "../src/http/app.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

test("dashboard runtime configuration applies to the next run and survives restart", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-runtime-config-"));
  const databasePath = path.join(directory, "state.db");
  let app;
  let store;
  context.after(async () => {
    if (app) await app.stop();
    if (store) store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });

  ({ app, store } = await startRuntimeApp(databasePath));
  let address = app.server.address();
  let origin = `http://127.0.0.1:${address.port}`;
  let response = await jsonFetch(`${origin}/api/configuration/runtime`);
  assert.equal(response.configuration.missingSourceTabPolicy.effectiveValue, "open_missing_tab");
  assert.equal(response.configuration.missingSourceTabPolicy.source, "default");
  assert.equal(response.configuration.defaultPresentation.effectiveValue, "source");
  assert.equal(response.configuration.homePresentation.effectiveValue, "timeline");
  assert.equal(response.configuration.timelineCapacity.effectiveValue, 12);
  assert.equal(response.configuration.streamWidth.effectiveValue, "social");
  assert.equal(response.configuration.telemetryBehavior.effectiveValue, "flow");
  assert.deepEqual(response.configuration.activeSources.effectiveValue, ["x", "linkedin"]);
  assert.equal(response.configuration.maxItemsPerSource.effectiveValue, 5);
  assert.equal(response.configuration.maxScrolls.effectiveValue, 2);
  assert.equal(response.configuration.maxAcquisitionRounds.effectiveValue, 2);
  assert.equal(response.configuration.maxKnowledgeContextEvents.effectiveValue, 20);

  response = await jsonFetch(`${origin}/api/configuration/runtime`, {
    method: "PUT",
    body: JSON.stringify({
      missingSourceTabPolicy: "fail_fast",
      evaluationModel: "gpt-test-evaluation",
      evaluationEffort: "xhigh",
      planningPolicy: "always",
      defaultPresentation: "brief",
      homePresentation: "overview",
      timelineCapacity: 18,
      streamWidth: "comfortable",
      telemetryBehavior: "sticky",
      activeSources: ["x"],
      maxItemsPerSource: 7,
      maxScrolls: 3,
      maxAcquisitionRounds: 1,
      maxKnowledgeContextEvents: 30,
    }),
  });
  assert.equal(response.configuration.missingSourceTabPolicy.effectiveValue, "fail_fast");
  assert.equal(response.configuration.missingSourceTabPolicy.source, "dashboard");
  assert.equal(response.configuration.evaluationModel.effectiveValue, "gpt-5.6-terra");
  assert.equal(response.configuration.evaluationModel.persistedValue, "gpt-test-evaluation");
  assert.equal(response.configuration.evaluationModel.restartRequired, true);
  assert.equal(response.configuration.defaultPresentation.effectiveValue, "brief");
  assert.equal(response.configuration.homePresentation.effectiveValue, "overview");
  assert.equal(response.configuration.timelineCapacity.effectiveValue, 18);
  assert.equal(response.configuration.defaultPresentation.restartRequired, false);
  assert.equal(response.configuration.streamWidth.effectiveValue, "comfortable");
  assert.equal(response.configuration.streamWidth.restartRequired, false);
  assert.equal(response.configuration.telemetryBehavior.effectiveValue, "sticky");
  assert.equal(response.configuration.telemetryBehavior.restartRequired, false);
  assert.deepEqual(response.configuration.activeSources.effectiveValue, ["x"]);
  assert.equal(response.configuration.maxItemsPerSource.effectiveValue, 7);
  assert.equal(response.configuration.maxScrolls.effectiveValue, 3);
  assert.equal(response.configuration.maxAcquisitionRounds.effectiveValue, 1);
  assert.equal(response.configuration.maxKnowledgeContextEvents.effectiveValue, 30);

  const bootstrap = await jsonFetch(`${origin}/api/bootstrap`);
  assert.deepEqual(
    bootstrap.sourceRegistry.map((source) => [source.id, source.activationState]),
    [["x", "active"], ["linkedin", "inactive"]],
  );
  const xOnlySession = await jsonFetch(`${origin}/api/sessions`, {
    method: "POST",
    body: JSON.stringify({}),
  });
  assert.deepEqual(xOnlySession.session.children.map((child) => child.source), ["x"]);

  await app.stop();
  store.close();
  ({ app, store } = await startRuntimeApp(databasePath));
  address = app.server.address();
  origin = `http://127.0.0.1:${address.port}`;
  response = await jsonFetch(`${origin}/api/configuration/runtime`);
  assert.equal(response.configuration.missingSourceTabPolicy.effectiveValue, "fail_fast");
  assert.equal(response.configuration.missingSourceTabPolicy.source, "dashboard");
  assert.equal(response.configuration.evaluationModel.effectiveValue, "gpt-test-evaluation");
  assert.equal(response.configuration.evaluationEffort.effectiveValue, "xhigh");
  assert.equal(response.configuration.planningPolicy.effectiveValue, "always");
  assert.equal(response.configuration.defaultPresentation.effectiveValue, "brief");
  assert.equal(response.configuration.homePresentation.effectiveValue, "overview");
  assert.equal(response.configuration.timelineCapacity.effectiveValue, 18);
  assert.equal(response.configuration.streamWidth.effectiveValue, "comfortable");
  assert.equal(response.configuration.telemetryBehavior.effectiveValue, "sticky");
  assert.deepEqual(response.configuration.activeSources.effectiveValue, ["x"]);
  assert.equal(response.configuration.maxItemsPerSource.effectiveValue, 7);
  assert.equal(response.configuration.maxScrolls.effectiveValue, 3);
  assert.equal(response.configuration.maxAcquisitionRounds.effectiveValue, 1);
  assert.equal(response.configuration.maxKnowledgeContextEvents.effectiveValue, 30);
  assert.equal(response.configuration.evaluationModel.restartRequired, false);
});

test("environment override remains effective over a persisted dashboard value", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-runtime-override-"));
  const databasePath = path.join(directory, "state.db");
  const seed = new SqliteStateStore(databasePath);
  seed.setSetting("runtime.missing_source_tab_policy", "fail_fast");
  seed.close();

  const config = loadConfig({
    AKU_DATABASE_PATH: databasePath,
    AKU_MISSING_SOURCE_TAB_POLICY: "open_missing_tab",
  });
  config.port = 0;
  const store = new SqliteStateStore(databasePath);
  const app = createAkuBrowserApp({
    config,
    store,
    reasoningProvider: { name: "runtime-config-fixture" },
    logger: { error() {} },
  });
  context.after(async () => {
    await app.stop();
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });
  const address = await app.start();
  const response = await jsonFetch(
    `http://127.0.0.1:${address.port}/api/configuration/runtime`,
  );
  assert.equal(response.configuration.missingSourceTabPolicy.effectiveValue, "open_missing_tab");
  assert.equal(response.configuration.missingSourceTabPolicy.persistedValue, "fail_fast");
  assert.equal(response.configuration.missingSourceTabPolicy.source, "environment");
  const blocked = await fetch(
    `http://127.0.0.1:${address.port}/api/configuration/runtime`,
    {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ missingSourceTabPolicy: "fail_fast" }),
    },
  );
  assert.equal(blocked.status, 400);
  assert.match((await blocked.json()).message, /environment override/);
});

async function startRuntimeApp(databasePath) {
  const config = loadConfig({ AKU_DATABASE_PATH: databasePath });
  config.port = 0;
  const store = new SqliteStateStore(databasePath);
  const app = createAkuBrowserApp({
    config,
    store,
    reasoningProvider: { name: "runtime-config-fixture" },
    logger: { error() {} },
  });
  await app.start();
  return { app, store };
}

async function jsonFetch(url, options = {}) {
  const response = await fetch(url, {
    ...options,
    headers: { "Content-Type": "application/json", ...(options.headers ?? {}) },
  });
  const payload = await response.json();
  assert.equal(response.ok, true, JSON.stringify(payload));
  return payload;
}
