import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const app = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const functionBetween = (start, end) => app.slice(app.indexOf(start), app.indexOf(end, app.indexOf(start)));

test("daily action transport sends the explicitly pinned process epoch", async () => {
  const calls = [];
  const context = vm.createContext({
    fetchFromSidecar: async (path, init) => {
      calls.push({ path, init });
      return { ok: true, status: 200, json: async () => ({ xCapture: { state: "ready" } }) };
    },
    enforceCurrentSidecarEpoch: () => {},
  });
  vm.runInContext(functionBetween("async function api(", "async function bridgeApi("), context);
  await context.api("/api/development/x-daily/start", {
    method: "POST", instanceEpoch: "synthetic-epoch",
    body: { acknowledgeLimitations: true, allowProcessSessionDerivation: true },
  });
  assert.equal(calls.length, 1);
  assert.equal(calls[0].init.headers["X-Aku-Split-Epoch"], "synthetic-epoch");
  assert.deepEqual(JSON.parse(calls[0].init.body), {
    acknowledgeLimitations: true, allowProcessSessionDerivation: true,
  });
  await context.api("/api/settings");
  assert.equal(calls[1].init.headers["X-Aku-Split-Epoch"], undefined);
});

test("ordinary runtime polling hands X status to its pre-request ticket without mutating settings", async () => {
  let release;
  const pending = new Promise(resolve => { release = resolve; });
  const events = [];
  const settings = { autoUpdateEnabled: true, collectionMode: "browser" };
  const xCapture = { state: "ready", derivationAuthorized: true };
  const ticket = { epoch: "synthetic-epoch", revision: 7 };
  const state = { bootstrap: { settings }, collectionPollInFlight: false };
  const context = vm.createContext({
    state,
    api: async path => { events.push(path); return pending; },
    xDailyControls: {
      beginRead: () => { events.push("ticket"); return ticket; },
      applyRead: (actualTicket, value) => {
        assert.equal(actualTicket, ticket);
        assert.equal(value, xCapture);
        events.push("apply");
      },
    },
    renderCollectionRuntime: () => {}, renderSourceSessionReadiness: () => {}, syncRunButtons: () => {},
  });
  vm.runInContext(functionBetween("async function pollCollectionRuntime(", "function renderSettings("), context);
  const first = context.pollCollectionRuntime();
  await context.pollCollectionRuntime();
  assert.deepEqual(events, ["ticket", "/api/collection/runtime"]);
  release({ collectionRuntime: { effective: "browser" }, xCapture });
  await first;
  assert.deepEqual(events, ["ticket", "/api/collection/runtime", "apply"]);
  assert.equal(state.bootstrap.settings, settings);
  assert.equal(settings.autoUpdateEnabled, true);
  assert.equal(state.collectionPollInFlight, false);
});

test("failed runtime polling rejects its own status ticket without automatic mutations", async () => {
  const ticket = { epoch: "synthetic-epoch", revision: 1 };
  const rejected = [];
  let requests = 0;
  const state = { bootstrap: {}, collectionPollInFlight: false };
  const context = vm.createContext({
    state,
    api: async () => { requests++; throw new Error("synthetic disconnect"); },
    xDailyControls: { beginRead: () => ticket, rejectRead: value => rejected.push(value) },
  });
  vm.runInContext(functionBetween("async function pollCollectionRuntime(", "function renderSettings("), context);
  await context.pollCollectionRuntime();
  assert.deepEqual(rejected, [ticket]);
  assert.equal(requests, 1);
  assert.equal(state.collectionPollInFlight, false);
});
