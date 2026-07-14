export const BRIDGE_REQUIREMENTS = Object.freeze({
  minimumExtensionVersion: "0.5.37",
  runtimeRevision: "source-fidelity-v39",
  adapterVersions: Object.freeze({ x: "x-dom-v15", linkedin: "linkedin-dom-v13" }),
  requiredActions: Object.freeze([
    "reload_self",
    "report_capture_quality",
    "probe_freshness",
    "recover_source_freshness",
    "recover_missing_media",
  ]),
});

export function evaluateBridgeCompatibility(heartbeat) {
  const reasons = [];
  if (!heartbeat) reasons.push("AkuBridge heartbeat is unavailable.");
  else {
    if (compareVersions(heartbeat.extensionVersion, BRIDGE_REQUIREMENTS.minimumExtensionVersion) < 0) {
      reasons.push(
        `AkuBridge ${heartbeat.extensionVersion ?? "unknown"} is older than required ${BRIDGE_REQUIREMENTS.minimumExtensionVersion}.`,
      );
    }
    if (heartbeat.runtimeRevision !== BRIDGE_REQUIREMENTS.runtimeRevision) {
      reasons.push(
        `Runtime ${heartbeat.runtimeRevision ?? "unknown"} does not match required ${BRIDGE_REQUIREMENTS.runtimeRevision}.`,
      );
    }
    const expectedBuildId = `aku-bridge-${heartbeat.extensionVersion}-${heartbeat.runtimeRevision}`;
    if (heartbeat.buildId !== expectedBuildId) {
      reasons.push(`Build identity ${heartbeat.buildId ?? "unknown"} must be ${expectedBuildId}.`);
    }
    for (const [source, required] of Object.entries(BRIDGE_REQUIREMENTS.adapterVersions)) {
      const actual = heartbeat.adapterVersions?.[source];
      if (actual !== required) reasons.push(`${source} adapter ${actual ?? "unknown"} must be ${required}.`);
    }
    for (const action of BRIDGE_REQUIREMENTS.requiredActions) {
      if (!heartbeat.actions?.includes(action)) reasons.push(`AkuBridge must advertise ${action}.`);
    }
  }
  return {
    compatible: reasons.length === 0,
    reasons,
    required: BRIDGE_REQUIREMENTS,
    actual: heartbeat ? {
      extensionVersion: heartbeat.extensionVersion,
      runtimeRevision: heartbeat.runtimeRevision,
      buildId: heartbeat.buildId,
      adapterVersions: heartbeat.adapterVersions,
    } : null,
  };
}

function compareVersions(left, right) {
  const a = parseVersion(left);
  const b = parseVersion(right);
  if (!a) return -1;
  for (let index = 0; index < 3; index += 1) {
    if (a[index] !== b[index]) return a[index] > b[index] ? 1 : -1;
  }
  return 0;
}

function parseVersion(value) {
  const match = String(value ?? "").match(/^(\d+)\.(\d+)\.(\d+)$/);
  return match ? match.slice(1).map(Number) : null;
}
