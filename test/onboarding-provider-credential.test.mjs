import test from "node:test";
import assert from "node:assert/strict";

import {
  providerCanActivate,
  providerHasSecureCredential,
  providerReadinessFeedback,
  providerRequiresSecureCredential,
} from "../internal/httpapi/web/onboarding-provider-credential.js";

test("development fallback must be migrated before remote provider activation", () => {
  const provider = {
    configured: true,
    credentialName: "gemini.primary",
    configurationStatus: "development_fallback",
  };
  assert.equal(providerRequiresSecureCredential(provider), true);
  assert.equal(providerHasSecureCredential(provider), false);
  assert.equal(providerCanActivate(provider), false);
  assert.deepEqual(providerReadinessFeedback(provider), { label: "Key required", className: "is-unavailable" });
});

test("securely stored credential permits provider activation", () => {
  const provider = {
    configured: true,
    credentialName: "gemini.primary",
    configurationStatus: "ready",
  };
  assert.equal(providerRequiresSecureCredential(provider), false);
  assert.equal(providerHasSecureCredential(provider), true);
  assert.equal(providerCanActivate(provider), true);
  assert.deepEqual(providerReadinessFeedback(provider), { label: "Key saved", className: "is-ready" });
});

test("missing or unconfirmed credentials never display as securely saved", () => {
  assert.equal(providerHasSecureCredential({ credentialName: "gemini.primary", configured: false, configurationStatus: "missing_credential" }), false);
  assert.equal(providerHasSecureCredential({ credentialName: "gemini.primary", configurationStatus: "ready" }), false);
  assert.equal(providerCanActivate({ credentialName: "gemini.primary", configurationStatus: "ready" }), false);
  assert.equal(providerHasSecureCredential({ credentialName: "gemini.primary", configured: true, configurationStatus: "development_fallback", availabilityChecked: true, available: true }), false);
  assert.deepEqual(providerReadinessFeedback({ credentialName: "gemini.primary", configured: true, configurationStatus: "development_fallback", availabilityChecked: true, available: true }), { label: "Key required", className: "is-unavailable" });
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
  assert.deepEqual(providerReadinessFeedback({ availabilityRequired: true, availabilityChecked: true, available: true }), { label: "Ready", className: "is-ready" });
});

test("failed local readiness blocks provider activation", () => {
  assert.equal(providerCanActivate({
    configured: true,
    configurationStatus: "ready",
    availabilityRequired: true,
    availabilityChecked: true,
    available: false,
  }), false);
  assert.deepEqual(providerReadinessFeedback({ availabilityRequired: true, availabilityChecked: true, available: false }), { label: "Unavailable", className: "is-unavailable" });
});
