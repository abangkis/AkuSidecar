import test from "node:test";
import assert from "node:assert/strict";
import { settingsStorageView } from "../internal/httpapi/web/settings-storage-state.js";

const report = (overrides = {}) => ({ timelineStorage: {
  totalItems: 180,
  logicalBytes: 2_293_175,
  eligibleItems: 0,
  overItemLimit: false,
  overByteLimit: false,
  policy: { maxItems: 500, maxLogicalBytes: 10_485_760 },
  ...overrides,
} });

test("Timeline storage stays distinct from whole database size", () => {
  const view = settingsStorageView(report({ databaseEffectiveBytes: 32_575_488 }));
  assert.equal(view.status, "Within preview limits");
  assert.equal(view.itemPercent, 36);
  assert.equal(view.bytePercent < 100, true);
  assert.equal(view.databaseEffectiveBytes, 32_575_488);
});

test("pressure distinguishes eligible zero, candidates, and unknown", () => {
  assert.equal(settingsStorageView(report({ overItemLimit: true })).status, "Above preview limit · no eligible cards");
  assert.equal(settingsStorageView(report({ overByteLimit: true, eligibleItems: 3 })).status, "Above preview limit · candidates available");
  assert.equal(settingsStorageView(report({ overByteLimit: true, eligibleItems: null })).status, "Above preview limit · eligibility unknown");
});

test("incomplete metrics remain unavailable rather than zero", () => {
  assert.equal(settingsStorageView(report({ logicalBytes: null })), null);
  assert.equal(settingsStorageView(report({ policy: { maxItems: 500 } })), null);
  assert.equal(settingsStorageView(null), null);
});

test("over-limit meter caps display but preserves actual value", () => {
  const view = settingsStorageView(report({ totalItems: 600, overItemLimit: true, eligibleItems: 8 }));
  assert.equal(view.itemPercent, 100);
  assert.equal(view.totalItems, 600);
});
