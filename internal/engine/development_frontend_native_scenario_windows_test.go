//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"strings"
	"testing"
)

func TestDevelopmentFrontendNativeScenarioContract(t *testing.T) {
	for _, scenario := range []string{
		DevelopmentFrontendNativeScenarioNormal,
		DevelopmentFrontendNativeScenarioForegroundOnly,
		DevelopmentFrontendNativeScenarioReaderOpenParentDeath,
	} {
		if err := validateDevelopmentFrontendNativeScenario(scenario); err != nil {
			t.Fatalf("supported native scenario rejected %q: %v", scenario, err)
		}
	}
	for _, scenario := range []string{"", "headless", "reader_death", "reader_open_parent_death_extra"} {
		if err := validateDevelopmentFrontendNativeScenario(scenario); err == nil || !strings.Contains(err.Error(), "scenario rejected") {
			t.Fatalf("unknown native scenario accepted %q: %v", scenario, err)
		}
	}
	t.Setenv(developmentFrontendNativeScenarioEnv, "")
	selected, err := developmentFrontendNativeScenario()
	if err != nil || selected != DevelopmentFrontendNativeScenarioNormal {
		t.Fatalf("legacy native invocation default changed: scenario=%q err=%v", selected, err)
	}
	t.Setenv(developmentFrontendNativeScenarioEnv, DevelopmentFrontendNativeScenarioReaderOpenParentDeath)
	selected, err = developmentFrontendNativeScenario()
	if err != nil || selected != DevelopmentFrontendNativeScenarioReaderOpenParentDeath {
		t.Fatalf("explicit live-reader scenario not selected: scenario=%q err=%v", selected, err)
	}
	t.Setenv(developmentFrontendNativeScenarioEnv, "unapproved")
	if _, err = developmentFrontendNativeScenario(); err == nil {
		t.Fatal("unknown scenario environment accepted")
	}
}

func TestDevelopmentFrontendNativeStartRejectsScenarioBeforeRuntimeOpen(t *testing.T) {
	if err := validateDevelopmentFrontendNativeStart(processFixtureStart{}, "unexpected"); err == nil || !strings.Contains(err.Error(), "scenario rejected") {
		t.Fatalf("unknown scenario reached receipt/runtime validation: %v", err)
	}
	if err := validateDevelopmentFrontendNativeStart(processFixtureStart{}, DevelopmentFrontendNativeScenarioReaderOpenParentDeath); err == nil || !strings.Contains(err.Error(), "scoped startup frame rejected") {
		t.Fatalf("live-reader scenario bypassed scoped startup receipt/frame: %v", err)
	}
}
