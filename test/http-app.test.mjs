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
  assert.ok(bootstrap.bridgeToken);

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
