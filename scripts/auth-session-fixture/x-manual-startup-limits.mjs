// Private manual-pilot timing policy. A longer startup never extends evidence
// freshness, a caller's shorter deadline, or the absolute 60-second attempt.
import {MANUAL_TIMING} from './x-manual-timing-policy.mjs';

export const MANUAL_STARTUP_MAX_MS = MANUAL_TIMING.startupTotalMS;
export const MANUAL_STARTUP_EVIDENCE_MAX_AGE_MS = MANUAL_TIMING.evidenceMaxAgeMS;
