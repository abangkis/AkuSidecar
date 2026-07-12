import assert from "node:assert/strict";
import test from "node:test";
import { getOnboardingProfile, saveOnboardingProfile } from "../src/core/onboarding-profile.mjs";

test("onboarding starts empty and persists only explicit source selection", () => {
  const values = new Map();
  const store = {
    getSetting: (key) => values.get(key) ?? null,
    setSetting: (key, value) => values.set(key, value),
  };
  assert.deepEqual(getOnboardingProfile(store), { status: "not_started", profile: null });
  const saved = saveOnboardingProfile(store, {
    activeSources: ["x", "linkedin"],
  }, new Date("2026-07-12T00:00:00.000Z"));
  assert.equal(saved.status, "completed");
  assert.equal("selectedInterests" in saved.profile, false);
  assert.equal("interestRefinement" in saved.profile, false);
  assert.equal("preferredContentTypes" in saved.profile, false);
  assert.equal(saved.profile.origin, "explicit_onboarding");
  assert.deepEqual(getOnboardingProfile(store), saved);
});

test("onboarding rejects incomplete and unsupported profiles", () => {
  const store = { getSetting() { return null; }, setSetting() {} };
  assert.throws(() => saveOnboardingProfile(store, {
    activeSources: [],
  }), /at least one active source/i);
  assert.throws(() => saveOnboardingProfile(store, {
    activeSources: ["unsupported"],
  }), /unsupported value/i);
});
