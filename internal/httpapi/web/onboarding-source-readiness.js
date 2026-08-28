export function sourcePermissionReadyForOnboarding({ access } = {}) {
  return Boolean(access?.permissionGranted && access?.scriptRegistered && access?.ready);
}

// Keep the Bridge's typed access outcomes visible to the Sidecar UI. Permission
// and content-script registration are separate recovery actions; collapsing
// them into one "not ready" state makes a missing registration opaque.
export function sourceAccessReadinessState(access) {
  if (!access) return "checking";
  if (!access.permissionGranted) return "permission_not_granted";
  if (!access.scriptRegistered) return "registration_missing";
  if (!access.ready) return "capture_not_ready";
  return "ready";
}
