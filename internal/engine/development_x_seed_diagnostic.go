//go:build auth_session_dev

package engine

import (
	"strconv"
	"strings"
)

// Seed validation and daily status share the same closed vocabulary. Never
// format peer code text or use diagnostic metadata as operational authority.
func nativePilotSeedOrigin(code string) (string, bool) {
	if len(code) > nativePilotSeedCodeMax {
		return "", false
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
			return "", false
		}
		var indices [3]int
		for i, part := range parts[:3] {
			n, err := strconv.Atoi(part)
			if err != nil || n < 0 || n >= len(catalogs[i]) || strconv.Itoa(n) != part {
				return "", false
			}
			indices[i] = n
		}
		initial, ok := nativePilotInitialCode("manual_initial_" + parts[3])
		if !ok || initial.trace == nil {
			return "", false
		}
		return origin(catalogs[0][indices[0]], catalogs[1][indices[1]], catalogs[2][indices[2]]) + " initial=(" + initial.String() + ")", true
	}
	for _, phase := range catalogs[0] {
		for _, reason := range catalogs[1] {
			for _, write := range catalogs[2] {
				if code == "manual_seed_"+phase+"_"+reason+"_"+write {
					return origin(phase, reason, write), true
				}
			}
		}
	}
	return "", false
}

func nativePilotSeedCode(code string) (string, bool) {
	if _, ok := nativePilotSeedOrigin(code); ok {
		return code, true
	}
	return "", false
}
