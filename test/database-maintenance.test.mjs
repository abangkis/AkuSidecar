import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import vm from "node:vm";

const source = readFileSync(new URL("../internal/httpapi/web/app.js", import.meta.url), "utf8");
const code = source.slice(source.indexOf("const shownDatabaseIssues"), source.indexOf("function setView"));
function fixture(storage = new Map()) {
  const elements = new Map();
  const requests = [];
  const notices = [];
  const context = {
    state: { bootstrap: {} },
    document: { visibilityState: "visible" },
    $: (id) => { if (!elements.has(id)) elements.set(id, { open: false, disabled: false, shown: 0, showModal() { this.open = true; this.shown++; }, close() { this.open = false; } }); return elements.get(id); },
    localStorage: { getItem: (key) => storage.get(key), setItem: (key, value) => storage.set(key, value) },
    api: async (path, request) => { requests.push({ path, request }); return { removedRows: 5 }; },
    bootstrap: async () => {}, showNotice: (message) => notices.push(message),
  };
  vm.createContext(context);
  vm.runInContext(code, context);
  return { context, elements, requests, notices, storage };
}
const issue = { status: "attention", orphanRows: 3, affectedTables: [{table: "runs"}], repairable: true, fingerprint: "issue-a" };

test("maintenance action is deduplicated across polling and refresh, with explicit reopen", () => {
  const f = fixture();
  f.context.renderDatabaseHealth(issue);
  const dialog = f.elements.get("#database-maintenance-dialog");
  assert.equal(dialog.shown, 1);
  dialog.close();
  f.context.renderDatabaseHealth(issue);
  assert.equal(dialog.shown, 1);
  const reloaded = fixture(f.storage);
  reloaded.context.renderDatabaseHealth(issue);
  assert.equal(reloaded.elements.get("#database-maintenance-dialog").shown, 0);
  reloaded.context.renderDatabaseHealth(issue, true);
  assert.equal(reloaded.elements.get("#database-maintenance-dialog").shown, 1);
});

test("health polling has its own minute cadence and avoids overlapping or hidden scans", async () => {
  const autoPoll = source.slice(source.indexOf("async function pollAutoUpdate"), source.indexOf("const shownDatabaseIssues"));
  assert.doesNotMatch(autoPoll, /\/api\/database\/health/);
  assert.match(source, /databaseHealthPoller \?\?= setInterval\(pollDatabaseHealth, 60_000\)/);
  const f = fixture();
  let release;
  f.context.api = () => { f.requests.push("health"); return new Promise((resolve) => { release = resolve; }); };
  const pending = f.context.pollDatabaseHealth();
  await f.context.pollDatabaseHealth();
  assert.equal(f.requests.length, 1);
  release({databaseHealth: {status: "healthy"}});
  await pending;
  f.context.document.visibilityState = "hidden";
  await f.context.pollDatabaseHealth();
  assert.equal(f.requests.length, 1);
});

test("cleanup transmits explicit acknowledgement, displayed fingerprint, and backup choice", async () => {
  for (const backup of [true, false]) {
    const f = fixture(); f.context.renderDatabaseHealth(issue);
    await f.context.cleanDatabase(backup);
    assert.equal(f.requests.length, 1);
    assert.equal(f.requests[0].path, "/api/database/cleanup");
    assert.equal(f.requests[0].request.body.confirmed, true);
    assert.equal(f.requests[0].request.body.fingerprint, issue.fingerprint);
    assert.equal(f.requests[0].request.body.backup, backup);
    assert.equal(f.notices.length, 1);
  }
});

test("nonrepairable issues disable deletion and storage pressure stays distinct from orphan damage", async () => {
  const f = fixture(); f.context.renderDatabaseHealth({...issue, repairable: false});
  await f.context.cleanDatabase(false);
  assert.equal(f.requests.length, 0);
  assert.equal(f.elements.get("#database-maintenance-clean").disabled, true);
  f.context.renderDatabaseHealth({status: "healthy", storagePressure: true});
  assert.match(f.elements.get("#database-status").textContent, /Storage pressure/);
  assert.equal(f.elements.get("#database-maintenance-dialog").open, false);
});
