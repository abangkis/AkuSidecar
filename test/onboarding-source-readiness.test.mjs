import test from "node:test";
import assert from "node:assert/strict";

import { sourcePermissionReadyForOnboarding } from "../internal/httpapi/web/onboarding-source-readiness.js";

const readyAccess = {
  permissionGranted: true,
  scriptRegistered: true,
  ready: true,
};

test("closed source tabs do not block onboarding after access is ready", () => {
  assert.equal(sourcePermissionReadyForOnboarding({
    access: readyAccess,
    session: { state: "not_observed", tabCount: 0 },
  }), true);
});

test("login observation remains advisory during onboarding", () => {
  assert.equal(sourcePermissionReadyForOnboarding({
    access: readyAccess,
    session: { state: "login_required", tabCount: 1 },
  }), true);
});

test("missing permission or capture registration still blocks onboarding", () => {
  assert.equal(sourcePermissionReadyForOnboarding({ access: { ...readyAccess, permissionGranted: false } }), false);
  assert.equal(sourcePermissionReadyForOnboarding({ access: { ...readyAccess, scriptRegistered: false } }), false);
  assert.equal(sourcePermissionReadyForOnboarding({ access: { ...readyAccess, ready: false } }), false);
});
