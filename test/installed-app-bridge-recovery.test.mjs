import test from "node:test";
import assert from "node:assert/strict";
import {
  INSTALLED_APP_BRIDGE_RECOVERY_KEY,
  clearInstalledAppBridgeRecoveryAttempt,
  hasInstalledAppBridgeRecoveryAttempt,
  isEligibleForInstalledAppBridgeRecovery,
  recordInstalledAppBridgeRecoveryAttempt,
  shouldScheduleInstalledAppBridgeRecovery,
} from "../internal/httpapi/web/installed-app-bridge-recovery.js";

const reconnectingInstalledApp = {
  deployment: { mode: "production-installed-app" },
  bridge: { state: "reconnecting", actual: null },
};

function storage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  };
}

test("installed-app recovery only targets reconnecting Bridge without actual evidence", () => {
  assert.equal(isEligibleForInstalledAppBridgeRecovery(reconnectingInstalledApp), true);
  assert.equal(isEligibleForInstalledAppBridgeRecovery({
    ...reconnectingInstalledApp,
    deployment: { mode: "development" },
  }), false);
  assert.equal(isEligibleForInstalledAppBridgeRecovery({
    ...reconnectingInstalledApp,
    bridge: { state: "healthy", actual: { extensionVersion: "0.8.0" } },
  }), false);
  assert.equal(isEligibleForInstalledAppBridgeRecovery({
    ...reconnectingInstalledApp,
    bridge: { state: "incompatible", actual: { bridgeId: "observed" } },
  }), false);
});

test("installed-app recovery records at most one attempt and clears on Bridge ready", () => {
  const store = storage();
  assert.equal(shouldScheduleInstalledAppBridgeRecovery(reconnectingInstalledApp, false), true);
  assert.equal(shouldScheduleInstalledAppBridgeRecovery(reconnectingInstalledApp, true), false);
  assert.equal(hasInstalledAppBridgeRecoveryAttempt(store), false);

  recordInstalledAppBridgeRecoveryAttempt(store);
  assert.equal(store.getItem(INSTALLED_APP_BRIDGE_RECOVERY_KEY), "1");
  assert.equal(hasInstalledAppBridgeRecoveryAttempt(store), true);
  assert.equal(shouldScheduleInstalledAppBridgeRecovery(reconnectingInstalledApp, true), false);

  clearInstalledAppBridgeRecoveryAttempt(store);
  assert.equal(hasInstalledAppBridgeRecoveryAttempt(store), false);
});
