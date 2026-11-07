//go:build auth_session_dev

package engine

import (
	"strconv"
	"strings"
)

// Seed validation and daily status share the same closed vocabulary. Never
// format peer code text or use diagnostic metadata as operational authority.
func nativePilotSeedOrigin(code string) (string, bool) {
	origin, _, ok := nativePilotSeedDetails(code)
	return origin, ok
}

func nativePilotSeedDetails(code string) (string, *XDailyEnrollmentCleanupDiagnostic, bool) {
	if len(code) > nativePilotSeedCodeMax {
		return "", nil, false
	}
	var cleanup *XDailyEnrollmentCleanupDiagnostic
	if tail, versioned := strings.CutPrefix(code, "manual_seed_v3_"); versioned {
		steps, seed, found := strings.Cut(tail, "_")
		if !found || len(steps) != 5 {
			return "", nil, false
		}
		var outcomes [5]string
		catalog := [...]string{"not_needed", "verified", "uncertain"}
		for i := range outcomes {
			if steps[i] < '0' || steps[i] > '2' {
				return "", nil, false
			}
			outcomes[i] = catalog[steps[i]-'0']
		}
		cleanup = &XDailyEnrollmentCleanupDiagnostic{
			ObserverAttachment: outcomes[0], RuntimeDisable: outcomes[1], ObserverDetach: outcomes[2],
			ContextContinuity: outcomes[3], SeedGuard: outcomes[4],
		}
		code = "manual_seed_" + seed
	}
	catalogs := [][]string{
		{"preconditions", "owner_before", "frame_before", "enrollment_evaluate", "enrollment_project", "enrollment_cleanup", "frame_after", "cookie_read", "cookie_select", "seed_write", "owner_after"},
		{"disabled", "replay", "unavailable", "rejected", "candidate_absent", "bootstrap_missing", "bootstrap_invalid", "wrong_page", "inspection_failed", "document_changed", "ownership_changed", "deadline", "clock_regressed"},
		{"not_attempted", "unknown", "acknowledged"},
	}
	origin := func(phase, reason, write string) string {
		return "seed_phase=" + phase + " seed_reason=" + reason + " seed_write=" + write
	}
	if strings.HasPrefix(code, "manual_seed_v2_") {
		parts := strings.SplitN(strings.TrimPrefix(code, "manual_seed_v2_"), "_", 4)
		if len(parts) != 4 {
			return "", nil, false
		}
		var indices [3]int
		for i, part := range parts[:3] {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= len(catalogs[i]) || strconv.Itoa(n) != part {
				return "", nil, false
			}
			indices[i] = n
		}
		initial, ok := nativePilotInitialCode("manual_initial_" + parts[3])
		if !ok || initial.trace == nil {
			return "", nil, false
		}
		return origin(catalogs[0][indices[0]], catalogs[1][indices[1]], catalogs[2][indices[2]]) + " initial=(" + initial.String() + ")", cleanup, true
	}
	for _, phase := range catalogs[0] {
		for _, reason := range catalogs[1] {
			for _, write := range catalogs[2] {
				if code == "manual_seed_"+phase+"_"+reason+"_"+write {
					return origin(phase, reason, write), cleanup, true
				}
			}
		}
	}
	return "", nil, false
}

func nativePilotSeedCode(code string) (string, bool) {
	if _, ok := nativePilotSeedOrigin(code); ok {
		return code, true
	}
	return "", false
}
