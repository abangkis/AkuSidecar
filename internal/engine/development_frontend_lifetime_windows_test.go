//go:build windows && auth_session_dev && auth_session_fixture

package engine

import "testing"

func TestDevelopmentFrontendLifetimeStartupContract(t *testing.T) {
	t.Setenv("AKU_DEV_FRONTEND_LIFETIME_ENABLED", "1")
	t.Setenv("AKU_DEV_FRONTEND_ENABLED", "1")
	t.Setenv("AKU_DEV_FRONTEND_HEADLESS", "0")
	t.Setenv("AKU_DEV_FRONTEND_NATIVE_ENABLED", "")
	t.Setenv("AKU_DEV_BROKER_ENABLED", "")
	if err := validateDevelopmentFrontendLifetimeStart(processFixtureStart{}); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ key, value string }{
		{"AKU_DEV_FRONTEND_LIFETIME_ENABLED", "unexpected"},
		{"AKU_DEV_FRONTEND_ENABLED", ""},
		{"AKU_DEV_FRONTEND_HEADLESS", "1"},
		{"AKU_DEV_FRONTEND_NATIVE_ENABLED", "1"},
		{"AKU_DEV_BROKER_ENABLED", "1"},
	} {
		t.Run(item.key, func(t *testing.T) {
			t.Setenv(item.key, item.value)
			if validateDevelopmentFrontendLifetimeStart(processFixtureStart{}) == nil {
				t.Fatal("invalid automatic startup accepted")
			}
		})
	}
	start := processFixtureStart{}
	start.Grant.Reopen = true
	if validateDevelopmentFrontendLifetimeStart(start) == nil {
		t.Fatal("automatic reopened startup accepted")
	}
}
