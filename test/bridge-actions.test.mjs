import assert from "node:assert/strict";
import test from "node:test";
import {
  BridgeActionConflict,
  createBridgeActions,
} from "../src/operations/bridge-actions.mjs";

const EXPECTED_BUILD = "aku-bridge-0.5.17-source-fidelity-v19";

test("reload_self is bounded, idempotent, and completes only on the expected heartbeat", () => {
  const clock = { value: Date.parse("2026-07-14T01:00:00.000Z") };
  const actions = createBridgeActions({
    now: () => clock.value,
    timeoutMs: 5_000,
    expectedBuildId: EXPECTED_BUILD,
  });
  const request = { requestId: "reload-1", actor: "codex", reason: "load build v16" };
  const created = actions.requestReload(request, { buildId: "aku-bridge-0.5.13-source-fidelity-v15" });
  assert.equal(created.status, "pending");
  assert.equal(actions.requestReload(request).id, created.id);
  assert.throws(
    () => actions.requestReload({ ...request, reason: "different" }),
    BridgeActionConflict,
  );
  assert.throws(
    () => actions.requestReload({ ...request, requestId: "reload-2" }),
    BridgeActionConflict,
  );

  assert.equal(actions.next().status, "delivered");
  assert.equal(actions.next(), null);
  assert.equal(actions.accept(created.id).status, "accepted");
  assert.equal(actions.observeHeartbeat({ buildId: "old-build" }).status, "accepted");
  const completed = actions.observeHeartbeat({ buildId: EXPECTED_BUILD });
  assert.equal(completed.status, "completed");
  assert.equal(completed.observedBuildId, EXPECTED_BUILD);
});

test("an unreachable extension fails closed after the deadline", () => {
  const clock = { value: Date.parse("2026-07-14T01:00:00.000Z") };
  const actions = createBridgeActions({
    now: () => clock.value,
    timeoutMs: 1_000,
    expectedBuildId: EXPECTED_BUILD,
  });
  const created = actions.requestReload({ requestId: "reload-timeout", actor: "user", reason: "test" });
  clock.value += 1_001;
  const failed = actions.get(created.id);
  assert.equal(failed.status, "failed");
  assert.equal(failed.errorCategory, "relay_page_stale");
});

test("a pending long poll is woken immediately by a new reload action", async () => {
  const actions = createBridgeActions({ expectedBuildId: EXPECTED_BUILD });
  const waiting = actions.waitForNext(1_000);
  const created = actions.requestReload({
    requestId: "reload-long-poll",
    actor: "codex",
    reason: "background relay test",
  });
  const delivered = await waiting;
  assert.equal(delivered.id, created.id);
  assert.equal(delivered.status, "delivered");
});

test("expiry taxonomy preserves the last proven cooperative stage", () => {
  const cases = [
    { expected: "extension_not_accepted", advance(action) { action.next(); } },
    { expected: "reload_heartbeat_timeout", advance(action, id) { action.next(); action.accept(id); } },
    {
      expected: "build_mismatch",
      advance(action, id) {
        action.next();
        action.accept(id);
        action.observeHeartbeat({ buildId: "unexpected-build" });
      },
    },
  ];
  for (const [index, scenario] of cases.entries()) {
    const clock = { value: Date.parse("2026-07-14T01:00:00.000Z") };
    const actions = createBridgeActions({
      now: () => clock.value,
      timeoutMs: 1_000,
      expectedBuildId: EXPECTED_BUILD,
    });
    const created = actions.requestReload({
      requestId: `reload-stage-${index}`,
      actor: "codex",
      reason: "stage taxonomy",
    });
    scenario.advance(actions, created.id);
    clock.value += 1_001;
    assert.equal(actions.get(created.id).errorCategory, scenario.expected);
  }
});
