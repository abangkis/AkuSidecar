import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { projectRoot } from "../src/config.mjs";
import { BRIDGE_CONTRACT_VERSION, createAkuBrowserApp } from "../src/http/app.mjs";
import { SqliteStateStore } from "../src/store/sqlite-state-store.mjs";

test("HTTP API drives one sequential unified session without changing the bridge run contract", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-http-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const config = testConfig();
  const app = createAkuBrowserApp({
    config,
    store,
    reasoningProvider: provider(),
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
  assert.deepEqual(bootstrap.unifiedSession.sources, ["x", "linkedin"]);
  assert.equal(bootstrap.unifiedSession.maxItemsTotal, 10);

  let { session } = await jsonFetch(`${origin}/api/sessions`, {
    method: "POST",
    body: JSON.stringify({ intent: "Material engineering changes." }),
  });
  assert.equal(session.activeSource, "x");
  const active = await jsonFetch(`${origin}/api/sessions/active`);
  assert.equal(active.session.id, session.id);
  await completeHttpChild(origin, bootstrap.bridgeToken, session.children[0].run);

  ({ session } = await jsonFetch(`${origin}/api/sessions/${session.id}`));
  assert.equal(session.activeSource, "linkedin");
  await completeHttpChild(origin, bootstrap.bridgeToken, session.children[1].run);

  ({ session } = await jsonFetch(`${origin}/api/sessions/${session.id}`));
  assert.equal(session.status, "completed");
  assert.equal(session.result.items.length, 2);
  assert.deepEqual(
    session.result.items.map((entry) => entry.item.source),
    ["x", "linkedin"],
  );
  const timeline = await jsonFetch(`${origin}/api/sessions?limit=1&offset=0`);
  assert.equal(timeline.sessions.length, 1);
  assert.equal(timeline.sessions[0].id, session.id);
  assert.equal(timeline.pagination.total, 1);
  assert.equal(timeline.pagination.hasNext, false);
  assert.equal("observations" in timeline.sessions[0].children[0].run, false);
  assert.equal(timeline.sessions[0].children[0].run.result.items.length, 1);
  const feed = await jsonFetch(`${origin}/api/timeline?limit=1&offset=0`);
  assert.equal(feed.timeline.capacity, 12);
  assert.equal(feed.timeline.entries.length, 1);
  assert.equal(feed.timeline.pagination.total, 2);
  assert.equal("observations" in feed.timeline.entries[0].run, false);
  const noActive = await jsonFetch(`${origin}/api/sessions/active`);
  assert.equal(noActive.session, null);

  let { calibration } = await jsonFetch(`${origin}/api/calibration/sessions`, {
    method: "POST",
    body: JSON.stringify({ unifiedSessionId: session.id, triggerKind: "first_run" }),
  });
  assert.equal(calibration.status, "reviewing");
  assert.equal(calibration.sampleCount, 2);
  ({ calibration } = await jsonFetch(
    `${origin}/api/calibration/sessions/${calibration.id}/samples/0`,
    { method: "PUT", body: JSON.stringify({ label: "more_like_this" }) },
  ));
  ({ calibration } = await jsonFetch(
    `${origin}/api/calibration/sessions/${calibration.id}/samples/1`,
    { method: "PUT", body: JSON.stringify({ label: "less_like_this" }) },
  ));
  assert.equal(calibration.status, "completed");
  assert.equal(calibration.snapshot.liveInfluence, false);
  assert.equal(calibration.snapshot.activationState, "shadow_only");
  const activeCalibration = await jsonFetch(`${origin}/api/calibration/active`);
  assert.equal(activeCalibration.calibration, null);

  const missing = await fetch(`${origin}/api/sessions/missing`);
  assert.equal(missing.status, 404);
});

test("HTTP API cancels the active child and prevents the queued source from starting", async (context) => {
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), "aku-unified-cancel-"));
  const store = new SqliteStateStore(path.join(directory, "state.db"));
  const app = createAkuBrowserApp({
    config: testConfig(),
    store,
    reasoningProvider: provider(),
    logger: { error() {} },
  });
  context.after(async () => {
    await app.stop();
    store.close();
    fs.rmSync(directory, { recursive: true, force: true });
  });
  const address = await app.start();
  const origin = `http://127.0.0.1:${address.port}`;
  const created = await jsonFetch(`${origin}/api/sessions`, {
    method: "POST",
    body: JSON.stringify({}),
  });
  const cancelled = await jsonFetch(`${origin}/api/sessions/${created.session.id}/cancel`, {
    method: "POST",
  });
  assert.equal(cancelled.session.status, "cancelled");
  assert.deepEqual(
    cancelled.session.children.map((child) => child.status),
    ["cancelled", "cancelled"],
  );
  assert.deepEqual(cancelled.session.coverage.cancelledSources, ["x", "linkedin"]);
});

async function completeHttpChild(origin, token, run) {
  const claimed = await jsonFetch(
    `${origin}/api/bridge/commands/next?runId=${run.id}`,
    { headers: bridgeHeaders(token) },
  );
  const accepted = await fetch(
    `${origin}/api/bridge/commands/${claimed.command.id}/observation`,
    {
      method: "POST",
      headers: { ...bridgeHeaders(token), "Content-Type": "application/json" },
      body: JSON.stringify({ runId: run.id, observation: observation(run.source) }),
    },
  );
  assert.equal(accepted.status, 202);
  for (let attempt = 0; attempt < 100; attempt += 1) {
    const current = await jsonFetch(`${origin}/api/runs/${run.id}`);
    if (["completed", "failed", "cancelled"].includes(current.run.status)) return;
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
  assert.fail(`run ${run.id} did not become terminal`);
}

function testConfig() {
  return {
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
      defaultScrolls: 0,
      scrollFraction: 0.75,
      scrollSettleMs: 900,
      captureTimeoutMs: 45_000,
      pendingContentTimeoutMs: 5_000,
      pendingContentSettleMs: 700,
      maxBlocksPerSnapshot: 20,
      maxBlockCharacters: 4_000,
    },
  };
}

function bridgeHeaders(token) {
  return {
    "X-Aku-Bridge-Token": token,
    "X-Aku-Bridge-Id": "unified-http-test",
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

function provider() {
  return {
    name: "unified-http-provider",
    async analyze({ run, observation: value }) {
      const block = value.snapshots[0].blocks[0];
      return {
        summary: `${run.source} result complete.`,
        items: [
          {
            id: `${run.source}-http-item`,
            priority: "P1",
            whatChanged: block.text,
            whyItMatters: run.intent,
            source: run.source,
            sourceUrl: block.permalink,
            sourceUrlKind: "native_post",
            evidenceKey: block.evidenceKey,
            eventKey: `${run.source}-unified-http-event`,
            knowledgeDelta: "new_event",
            author: block.author,
            publishedAt: null,
            confidence: 0.9,
            evidenceState: "primary",
          },
        ],
        repeatedClaimsCollapsed: 0,
        deferredByBudget: 0,
        limitations: [],
      };
    },
  };
}

function observation(source) {
  return {
    source,
    pageUrl: source === "x" ? "https://x.com/home" : "https://www.linkedin.com/feed/",
    pageTitle: source,
    capturedAt: "2026-07-11T09:00:00Z",
    snapshots: [
      {
        capturedAt: "2026-07-11T09:00:00Z",
        scrollY: 0,
        viewportHeight: 900,
        blocks: [
          {
            text: `${source} visible release evidence.`,
            author: "Fixture",
            permalink:
              source === "x"
                ? "https://x.com/fixture/status/unified-http"
                : "https://www.linkedin.com/posts/fixture-unified-http-activity-1234567890",
            publishedAt: null,
            feedPosition: 1,
            links: [],
          },
        ],
      },
    ],
    coverage: {
      status: "partial",
      checkedThrough: "2026-07-11T09:00:00Z",
      candidateCount: 1,
    },
  };
}
