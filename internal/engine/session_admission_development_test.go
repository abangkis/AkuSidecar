//go:build auth_session_dev

package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestEngineSessionAdmissionCharacterizationDailyPlan(t *testing.T) {
	for _, selectedX := range []bool{false, true} {
		name := "ready-daily-unselected"
		if selectedX {
			name = "append-ready-daily"
		}
		t.Run(name, func(t *testing.T) {
			configured := []domain.Source{domain.SourceLinkedIn}
			if selectedX {
				configured = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
			}
			e, _, manager := dailyBrowserFixture(t, configured)
			h := admissionHeartbeatWithoutX()
			e.RecordHeartbeat(h)
			session, err := e.StartVisibleUpdate(context.Background(), "offline daily admission plan")
			if err != nil {
				t.Fatal(err)
			}
			if session.Coverage["collectionPolicy"] != domain.SessionCollectionPolicyHybridHeadlessV1 {
				t.Fatal("ready daily route did not freeze hybrid plan")
			}
			if session.Runs[0].Source != domain.SourceLinkedIn {
				t.Fatalf("ordinary ready source order changed: %+v", session.Runs)
			}
			if selectedX {
				if len(session.Runs) != 2 || session.Runs[1].Source != domain.SourceX {
					t.Fatalf("daily append order=%+v", session.Runs)
				}
			} else if len(session.Runs) != 1 {
				t.Fatal("unselected daily source was admitted")
			}
			drivers, ok := session.Coverage["collectionSourceDrivers"].(map[string]any)
			if !ok || drivers["linkedin"] != "browser" {
				t.Fatalf("frozen drivers=%v", drivers)
			}
			if selectedX && drivers["x"] != "headless" {
				t.Fatal("daily owner did not retain its source driver")
			}
			saved, err := e.Settings(context.Background())
			if err != nil || !reflect.DeepEqual(saved.ActiveSources, configured) {
				t.Fatalf("settings changed: %v %v", saved.ActiveSources, err)
			}
			if err := e.CancelSession(context.Background(), session.ID); err != nil {
				t.Fatal(err)
			}
			waitCaptureLeases(t, manager, 0)
		})
	}
}
