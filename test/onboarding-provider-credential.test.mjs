import test from "node:test";
import assert from "node:assert/strict";

import {
  providerCanActivate,
  providerRequiresSecureCredential,
} from "../internal/httpapi/web/onboarding-provider-credential.js";

test("development fallback must be migrated before remote provider activation", () => {
  const provider = {
    configured: true,
    credentialName: "gemini.primary",
    configurationStatus: "development_fallback",
  };
  assert.equal(providerRequiresSecureCredential(provider), true);
  assert.equal(providerCanActivate(provider), false);
});

test("securely stored credential permits provider activation", () => {
  const provider = {
    configured: true,
    credentialName: "gemini.primary",
    configurationStatus: "ready",
  };
  assert.equal(providerRequiresSecureCredential(provider), false);
  assert.equal(providerCanActivate(provider), true);
});

test("local provider waits for a successful availability probe", () => {
  assert.equal(providerCanActivate({
    configured: true,
    configurationStatus: "ready",
    availabilityRequired: true,
    availabilityChecked: false,
    available: false,
  }), false);
  assert.equal(providerCanActivate({
    configured: true,
    configurationStatus: "ready",
    availabilityRequired: true,
    availabilityChecked: true,
    available: true,
  }), true);
});

test("failed local readiness blocks provider activation", () => {
  assert.equal(providerCanActivate({
    configured: true,
    configurationStatus: "ready",
    availabilityRequired: true,
    availabilityChecked: true,
    available: false,
  }), false);
});
