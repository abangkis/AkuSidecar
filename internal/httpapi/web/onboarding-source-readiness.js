export function sourcePermissionReadyForOnboarding({ access } = {}) {
  return Boolean(access?.permissionGranted && access?.scriptRegistered && access?.ready);
}
