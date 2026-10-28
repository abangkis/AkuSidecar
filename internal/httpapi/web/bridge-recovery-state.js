// Compatibility permits work; parity tells the user which code is loaded.
export function bridgeRecoveryState(bridge, development = false) {
  const keys = ["buildId", "runtimeRevision", "focusPolicyRevision"];
  const buildKnown = keys.every((key) => typeof bridge?.actual?.[key] === "string" && bridge.actual[key]
    && typeof bridge?.expected?.[key] === "string" && bridge.expected[key]);
  const expectedPolicy = bridge?.expected?.captureSurfacePolicy;
  const actualPolicy = bridge?.actual?.captureSurfacePolicy;
  const policyKnown = expectedPolicy?.schema === "aku.surface-policy.v1"
    && /^[a-f0-9]{64}$/.test(expectedPolicy.fingerprint ?? "")
    && actualPolicy?.schema === expectedPolicy.schema
    && /^[a-f0-9]{64}$/.test(actualPolicy?.fingerprint ?? "");
  const policyMismatch = Boolean(bridge?.actual && expectedPolicy &&
    (!policyKnown || actualPolicy.fingerprint !== expectedPolicy.fingerprint));
  const known = buildKnown && policyKnown;
  const mismatches = keys.filter((key) => bridge?.actual?.[key] && bridge?.expected?.[key]
    && bridge.actual[key] !== bridge.expected[key]);
  const focusMismatch = bridge?.state === "incompatible"
    && bridge.reasons?.includes("bridge focus policy revision mismatch");
  const drift = mismatches.length > 0 || focusMismatch || policyMismatch;
  const label = policyMismatch
    ? "AkuBridge capture policy mismatch · reload required"
    : drift
    ? `AkuBridge revision mismatch · loaded ${bridge?.actual?.runtimeRevision || "unknown"} · expected ${bridge?.expected?.runtimeRevision || "unknown"}`
    : !known && bridge?.compatible ? "AkuBridge runtime identity unverified" : "";
  const detail = policyMismatch
    ? `Loaded capture policy: ${actualPolicy?.fingerprint || "unverified"}; expected: ${expectedPolicy.fingerprint}`
    : drift
    ? `Loaded build: ${bridge?.actual?.buildId || "unknown"}; expected build: ${bridge?.expected?.buildId || "unknown"}`
    : label;
  return { parity: Boolean(known && !drift), drift, label, detail, showReload: Boolean(development && drift) };
}

export function bridgeReloadVerified(bridge, action) {
  return action?.status === "completed" && Boolean(action.heartbeatObservedAt)
    && Boolean(action.expectedBuildId) && action.observedBuildId === action.expectedBuildId
    && action.expectedBuildId === bridge?.expected?.buildId
    && Boolean(action.expectedSurfacePolicy)
    && action.observedSurfacePolicy === action.expectedSurfacePolicy
    && action.expectedSurfacePolicy === bridge?.expected?.captureSurfacePolicy?.fingerprint
    && bridge?.compatible === true && bridgeRecoveryState(bridge).parity;
}

export function bridgeCaptureBusy(session) {
  return Boolean(session?.runs?.some((run) => run.status === "waiting_for_bridge" && run.bridgeCommandStatus === "claimed"));
}
