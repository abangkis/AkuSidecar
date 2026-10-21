//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"testing"
	"time"
)

// Use a valid old-authority configuration and change only the policy identity.
// Pilot observations must not acquire authority by reusing fixture grants.
func TestConcurrentPilotCannotEnterStrictSyntheticAuthority(t *testing.T) {
	clock := newFixtureClock()
	baseline, err := newConcurrentAuthorization(fixtureConfigForTest(true), fixtureGrantForTest(), clock.Now)
	if err != nil {
		t.Fatal("strict fixture baseline unavailable")
	}
	if view := baseline.view(); view.nativeQualified || view.canAuthorize {
		t.Fatal("fixture baseline acquired native authority")
	}
	for _, change := range []string{"contract", "provider", "schema", "all"} {
		t.Run(change, func(t *testing.T) {
			config := fixtureConfigForTest(true)
			if change == "contract" || change == "all" {
				config.enrollment.contractID = "aku.x.dev-pilot.snapshot.v1"
			}
			if change == "provider" || change == "all" {
				config.enrollment.providerID = "x-dev-pilot-snapshot"
			}
			if change == "schema" || change == "all" {
				config.enrollment.evidenceSchema = "aku.x.dev-pilot.snapshot.v1"
			}
			clockCalls := 0
			authority, err := newConcurrentAuthorization(config, fixtureGrantForTest(), func() time.Time {
				clockCalls++
				return clock.Now()
			})
			if err == nil || authority != nil || clockCalls != 0 {
				t.Fatal("pilot policy entered strict fixture authority")
			}
		})
	}
}
