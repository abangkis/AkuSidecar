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
    interestRefinement: "Practical Codex workflows and local AI tools.",
    preferredContentTypes: ["tutorial", "research"],
    activeSources: ["x", "linkedin"],
  }, new Date("2026-07-12T00:00:00.000Z"));
  assert.equal(saved.status, "completed");
  assert.deepEqual(saved.profile.selectedInterests, ["ai", "software_development"]);
  assert.equal(saved.profile.interestRefinement, "Practical Codex workflows and local AI tools.");
  assert.equal(saved.profile.origin, "explicit_onboarding");
  assert.deepEqual(getOnboardingProfile(store), saved);
});

test("onboarding rejects incomplete and unsupported profiles", () => {
  const store = { getSetting() { return null; }, setSetting() {} };
  assert.throws(() => saveOnboardingProfile(store, {
    preferredContentTypes: ["tutorial"], activeSources: ["x"],
  }), /at least one interest/i);
  assert.throws(() => saveOnboardingProfile(store, {
    selectedInterests: ["ai"], preferredContentTypes: ["video"], activeSources: ["x"],
  }), /unsupported value/i);
  assert.throws(() => saveOnboardingProfile(store, {
    selectedInterests: ["unsupported"], preferredContentTypes: ["tutorial"], activeSources: ["x"],
  }), /unsupported value/i);
});
