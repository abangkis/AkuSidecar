package store

import (
	"context"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestCreateUpdateSessionPersistsHybridSourcePlanWithoutChangingSettings(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "headless"
	settings.ActiveSources = []domain.Source{domain.SourceFacebook, domain.SourceLinkedIn, domain.SourceX, domain.SourceInstagram}
	originalSources := append([]domain.Source(nil), settings.ActiveSources...)
	session, err := state.CreateUpdateSession(ctx, "frozen hybrid source plan", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantSources := []domain.Source{domain.SourceLinkedIn, domain.SourceX, domain.SourceInstagram, domain.SourceFacebook}
	if !reflect.DeepEqual(settings.ActiveSources, originalSources) {
		t.Fatalf("creation reordered the user's settings: got %v want %v", settings.ActiveSources, originalSources)
	}
	if len(session.Runs) != len(wantSources) {
		t.Fatalf("runs=%+v", session.Runs)
	}
	for index, run := range session.Runs {
		if run.Source != wantSources[index] || run.Ordinal != index {
			t.Fatalf("run[%d]=%+v want source %s", index, run, wantSources[index])
		}
	}
	if session.Coverage["collectionPolicy"] != domain.SessionCollectionPolicyHybridHeadlessV1 {
		t.Fatalf("hybrid policy was not persisted: %+v", session.Coverage)
	}
	if configured, ok := session.Coverage["collectionSources"].([]any); !ok || !reflect.DeepEqual(configured, []any{"facebook", "linkedin", "x", "instagram"}) {
		t.Fatalf("configured source order was not preserved in the session plan: %+v", session.Coverage["collectionSources"])
	}
	drivers, ok := session.Coverage["collectionSourceDrivers"].(map[string]any)
	if !ok || drivers["x"] != "headless" || drivers["instagram"] != "headless" || drivers["linkedin"] != "headless" || drivers["facebook"] != "browser" {
		t.Fatalf("frozen source drivers=%+v", session.Coverage["collectionSourceDrivers"])
	}
}

func TestBrowserSessionKeepsLegacyCollectionCoverage(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.CollectionMode = "browser"
	session, err := state.CreateUpdateSession(ctx, "legacy browser source plan", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := session.Coverage["collectionPolicy"]; exists {
		t.Fatalf("browser session was upgraded to a new policy: %+v", session.Coverage)
	}
}

func TestSessionCaptureCommandsDrainedCanFencePriorOrdinals(t *testing.T) {
	ctx := context.Background()
	state := openTestStore(t)
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceFacebook}
	session, err := state.CreateUpdateSession(ctx, "command drain fence", settings, domain.UpdatePolicy{
		Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
	})
	if err != nil {
		t.Fatal(err)
	}
	first := session.Runs[0]
	if _, err := state.StartRun(ctx, first.ID, map[string]any{}); err != nil {
		t.Fatal(err)
	}
	secondOrdinal := 1
	drained, err := state.SessionCaptureCommandsDrained(ctx, session.ID, &secondOrdinal)
	if err != nil || drained {
		t.Fatalf("queued prior command reported drained=%v err=%v", drained, err)
	}
	command, err := state.ClaimCommandForCollector(ctx, first.ID, "test", "bridge")
	if err != nil || command == nil {
		t.Fatalf("claim=%+v err=%v", command, err)
	}
	if _, err := state.FailCommand(ctx, command.ID, first.ID, domain.Failure{Code: "fixture", Stage: "capture", Message: "done"}); err != nil {
		t.Fatal(err)
	}
	drained, err = state.SessionCaptureCommandsDrained(ctx, session.ID, &secondOrdinal)
	if err != nil || !drained {
		t.Fatalf("terminal prior command reported drained=%v err=%v", drained, err)
	}
}
