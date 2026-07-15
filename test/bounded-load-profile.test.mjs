import assert from "node:assert/strict";
import test from "node:test";
import {
  applyBoundedLoadProfile,
  BOUNDED_LOAD_PROFILES,
  DEFAULT_BOUNDED_LOAD_PROFILE,
} from "../src/core/bounded-load-profile.mjs";

test("one bounded load profile coordinates every scaled runtime budget", () => {
  assert.equal(DEFAULT_BOUNDED_LOAD_PROFILE, "expanded_2x");
  const config = {
    limits: {},
    presentation: {},
  };
  assert.equal(applyBoundedLoadProfile(config, "stress_3x"), true);
  assert.deepEqual(config, {
    limits: {
      boundedLoadProfile: "stress_3x",
      boundedLoadScale: 3,
      maxItems: 15,
      maxItemsTotal: 30,
      maxScrolls: 6,
      defaultScrolls: 6,
      qualityRetrySettleMs: 1_000,
    },
    presentation: { timelineCapacity: 36 },
  });
  assert.equal(BOUNDED_LOAD_PROFILES.standard_1x.maxScrolls, 2);
  assert.equal(BOUNDED_LOAD_PROFILES.expanded_2x.maxScrolls, 4);
});

test("custom profile preserves explicit advanced budgets", () => {
  const config = {
    limits: { maxItems: 7, maxScrolls: 3 },
    presentation: { timelineCapacity: 19 },
  };
  assert.equal(applyBoundedLoadProfile(config, "custom"), true);
  assert.equal(config.limits.boundedLoadProfile, "custom");
  assert.equal(config.limits.boundedLoadScale, null);
  assert.equal(config.limits.maxItems, 7);
  assert.equal(config.limits.maxScrolls, 3);
  assert.equal(config.presentation.timelineCapacity, 19);
});
