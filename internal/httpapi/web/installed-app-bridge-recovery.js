export const INSTALLED_APP_BRIDGE_RECOVERY_KEY = "akuBrowserInstalledAppBridgeRecoveryV1";
export const INSTALLED_APP_BRIDGE_RECOVERY_GRACE_MS = 1500;

export function isEligibleForInstalledAppBridgeRecovery(bootstrap) {
  return bootstrap?.deployment?.mode === "production-installed-app"
    && bootstrap?.bridge?.state === "reconnecting"
    && bootstrap?.bridge?.actual == null;
}

export function shouldScheduleInstalledAppBridgeRecovery(bootstrap, attemptRecorded) {
  return !attemptRecorded && isEligibleForInstalledAppBridgeRecovery(bootstrap);
}

export function hasInstalledAppBridgeRecoveryAttempt(storage) {
  return storage?.getItem(INSTALLED_APP_BRIDGE_RECOVERY_KEY) === "1";
}

export function recordInstalledAppBridgeRecoveryAttempt(storage) {
  storage?.setItem(INSTALLED_APP_BRIDGE_RECOVERY_KEY, "1");
}

export function clearInstalledAppBridgeRecoveryAttempt(storage) {
  storage?.removeItem(INSTALLED_APP_BRIDGE_RECOVERY_KEY);
}
