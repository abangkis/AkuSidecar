import assert from "node:assert/strict";
import test from "node:test";
import {
  BridgeActionConflict,
  createBridgeActions,
} from "../src/operations/bridge-actions.mjs";

const EXPECTED_BUILD = "aku-bridge-0.5.16-source-fidelity-v18";

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
  assert.equal(failed.errorCategory, "extension_unreachable");
});
