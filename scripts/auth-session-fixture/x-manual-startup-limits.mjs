// Private manual-pilot timing policy. A longer startup never extends evidence
// freshness, a caller's shorter deadline, or the absolute 60-second attempt.
export const MANUAL_STARTUP_MAX_MS = 6000;
export const MANUAL_STARTUP_EVIDENCE_MAX_AGE_MS = 2000;
