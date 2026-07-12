import assert from "node:assert/strict";
import test from "node:test";
import { getOnboardingProfile, saveOnboardingProfile } from "../src/core/onboarding-profile.mjs";

test("onboarding starts empty and persists only explicit v0 signals", () => {
  const values = new Map();
  const store = {
    getSetting: (key) => values.get(key) ?? null,
    setSetting: (key, value) => values.set(key, value),
  };
  assert.deepEqual(getOnboardingProfile(store), { status: "not_started", profile: null });
  const saved = saveOnboardingProfile(store, {
    selectedInterests: ["ai", "software_development"],
    activeSources: ["x", "linkedin"],
  }, new Date("2026-07-12T00:00:00.000Z"));
  assert.equal(saved.status, "completed");
  assert.deepEqual(saved.profile.selectedInterests, ["ai", "software_development"]);
  assert.equal("interestRefinement" in saved.profile, false);
  assert.equal("preferredContentTypes" in saved.profile, false);
  assert.equal(saved.profile.origin, "explicit_onboarding");
  assert.deepEqual(getOnboardingProfile(store), saved);
});

test("onboarding rejects incomplete and unsupported profiles", () => {
  const store = { getSetting() { return null; }, setSetting() {} };
  assert.throws(() => saveOnboardingProfile(store, {
    activeSources: ["x"],
  }), /at least one interest/i);
  assert.throws(() => saveOnboardingProfile(store, {
    selectedInterests: ["unsupported"], activeSources: ["x"],
  }), /unsupported value/i);
  assert.throws(() => saveOnboardingProfile(store, {
    selectedInterests: ["ai", "technology", "science", "gaming", "comedy", "finance"],
    activeSources: ["x"],
  }), /no more than five/i);
});
