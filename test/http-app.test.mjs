import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { projectRoot } from "../src/config.mjs";
import { BRIDGE_CONTRACT_VERSION, createAkuBrowserApp } from "../src/http/app.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

test("HTTP API enforces the bridge token and completes a finite run", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-browser-http-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const config = {
    host: "127.0.0.1",
    port: 0,
    publicDirectory: path.join(projectRoot, "public"),
    limits: {
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
  const provider = {
    name: "http-test-provider",
    async analyze({ run, observation }) {
      const block = observation.snapshots[0].blocks[0];
      return {
        summary: "HTTP vertical slice complete.",
        items: [
          {
            id: "http-result",
            priority: "P1",
            whatChanged: block.text,
            whyItMatters: run.intent,
            source: run.source,
            sourceUrl: block.permalink,
            sourceUrlKind: "native_post",
            evidenceKey: block.evidenceKey,
            eventKey: "http-fixture-update",
            knowledgeDelta: "new_event",
            author: block.author,
            publishedAt: null,
            confidence: 0.8,
            evidenceState: "primary",
          },
        ],
        repeatedClaimsCollapsed: 0,
        deferredByBudget: 0,
        limitations: [],
      };
    },
  };
  const app = createAkuBrowserApp({
    config,
    store,
    reasoningProvider: provider,
    logger: { error() {} },
  });
  context.after(async () => {
    await app.stop();
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });
  const address = await app.start();
  const origin = `http://127.0.0.1:${address.port}`;

  const bootstrap = await jsonFetch(`${origin}/api/bootstrap`);
  assert.equal(bootstrap.provider, "http-test-provider");
  assert.equal(bootstrap.bridgeContractVersion, BRIDGE_CONTRACT_VERSION);
  assert.equal(bootstrap.limits.defaultScrolls, 2);
  assert.ok(bootstrap.bridgeToken);

  const unauthorizedReload = await fetch(
    `${origin}/api/operations/bridge/actions/reload-self`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ requestId: "http-reload-1", actor: "codex", reason: "test reload" }),
    },
  );
  assert.equal(unauthorizedReload.status, 400);
  const requestedReload = await jsonFetch(
    `${origin}/api/operations/bridge/actions/reload-self`,
    {
      method: "POST",
      headers: bridgeHeaders(bootstrap.bridgeToken),
      body: JSON.stringify({ requestId: "http-reload-1", actor: "codex", reason: "test reload" }),
    },
  );
  assert.equal(requestedReload.action.type, "reload_self");
  const deliveredReload = await jsonFetch(`${origin}/api/operations/bridge/actions/next`);
  assert.equal(deliveredReload.action.status, "delivered");
  await jsonFetch(
    `${origin}/api/operations/bridge/actions/${requestedReload.action.id}/accept`,
    { method: "POST", headers: bridgeHeaders(bootstrap.bridgeToken) },
  );
  await jsonFetch(`${origin}/api/operations/bridge/heartbeat`, {
    method: "POST",
    body: JSON.stringify({
      capabilities: {
        extensionVersion: "0.5.17",
        runtimeRevision: "source-fidelity-v19",
        buildId: "aku-bridge-0.5.17-source-fidelity-v19",
        adapterVersions: { x: "x-dom-v12", linkedin: "linkedin-dom-v6" },
        actions: ["reload_self"],
      },
    }),
  });
  const completedReload = await jsonFetch(
    `${origin}/api/operations/bridge/actions/${requestedReload.action.id}`,
    { headers: bridgeHeaders(bootstrap.bridgeToken) },
  );
  assert.equal(completedReload.action.status, "completed");

  const replay = await jsonFetch(`${origin}/api/preferences/replay`);
  assert.equal(replay.replay.mode, "offline_replay");
  assert.equal(replay.replay.liveInfluence, false);
  const experiment = await jsonFetch(`${origin}/api/preferences/experiment`);
  assert.equal(experiment.experiment.status, "blocked");
  assert.equal(experiment.experiment.liveInfluence, false);
  const blockedFit = await jsonFetch(`${origin}/api/preferences/experiment/fit`, {
    method: "POST",
  });
  assert.equal(blockedFit.experiment.status, "blocked");
  assert.equal(store.getLatestPreferenceModelSnapshot(), null);

  const created = await jsonFetch(`${origin}/api/runs`, {
    method: "POST",
    body: JSON.stringify({ source: "x", maxItems: 1, scrolls: 0 }),
  });

  const unauthorized = await fetch(
    `${origin}/api/bridge/commands/next?runId=${created.run.id}`,
  );
  assert.equal(unauthorized.status, 400);

  const claimed = await jsonFetch(
    `${origin}/api/bridge/commands/next?runId=${created.run.id}`,
    { headers: bridgeHeaders(bootstrap.bridgeToken) },
  );
  assert.equal(claimed.command.status, "claimed");

  const accepted = await fetch(
    `${origin}/api/bridge/commands/${claimed.command.id}/observation`,
    {
      method: "POST",
      headers: { ...bridgeHeaders(bootstrap.bridgeToken), "Content-Type": "application/json" },
      body: JSON.stringify({ runId: created.run.id, observation: sampleObservation() }),
    },
  );
  assert.equal(accepted.status, 202);

  const completed = await app.engine.waitForRun(created.run.id);
  assert.equal(completed.status, "completed");
  assert.equal(completed.result.items.length, 1);

  const feedback = await jsonFetch(`${origin}/api/runs/${completed.id}/feedback`, {
    method: "POST",
    body: JSON.stringify({ kind: "useful", itemId: completed.result.items[0].id }),
  });
  assert.equal(feedback.run.feedback.at(-1).kind, "useful");
  assert.equal(feedback.run.feedback.at(-1).itemId, completed.result.items[0].id);

  const review = await jsonFetch(`${origin}/api/pilot/review?source=x&verdict=useful`);
  assert.equal(review.review.summary.completedRuns, 1);
  assert.equal(review.review.totalMatching, 1);
  assert.equal(review.review.runs[0].id, completed.id);
  assert.equal(review.review.window.pilotStartedAt, completed.createdAt);

  const invalidReview = await fetch(`${origin}/api/pilot/review?verdict=unknown`);
  assert.equal(invalidReview.status, 400);

  const knowledge = await jsonFetch(`${origin}/api/knowledge?source=x&mode=catch_up`);
  assert.equal(knowledge.knowledge.checkpoint.runId, completed.id);
  assert.equal(knowledge.knowledge.events[0].eventKey, "http-fixture-update");
  const history = await jsonFetch(
    `${origin}/api/knowledge/events/http-fixture-update?source=x&mode=catch_up`,
  );
  assert.equal(history.versions.length, 1);
  assert.equal(history.versions[0].evidenceKey, completed.result.items[0].evidenceKey);
});

function bridgeHeaders(token) {
  return {
    "X-Aku-Bridge-Token": token,
    "X-Aku-Bridge-Id": "http-test",
    "X-Aku-Bridge-Contract": BRIDGE_CONTRACT_VERSION,
  };
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

function sampleObservation() {
  return {
    source: "x",
    pageUrl: "https://x.com/home",
    capturedAt: "2026-07-10T10:00:00Z",
    snapshots: [
      {
        capturedAt: "2026-07-10T10:00:00Z",
        blocks: [
          {
            text: "A visible engineering release with specific availability was announced.",
            author: "Example",
            permalink: "https://x.com/example/status/1",
            links: [],
          },
        ],
      },
    ],
    coverage: { status: "partial", candidateCount: 1 },
  };
}
