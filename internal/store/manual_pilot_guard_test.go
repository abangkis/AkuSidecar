package store

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// This test deliberately has no development build tag. A normal binary must
// honor a persisted pilot fence even though it cannot issue a pilot permit.
func TestManualPilotPersistedFenceAllBuilds(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	settings, err := s.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	settings.CollectionMode = "headless"
	session, err := createVisibleUpdateSession(s, ctx, "synthetic all-build fence", settings)
	if err != nil {
		t.Fatal(err)
	}
	command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	// A malformed persisted authority record must still close generic admission.
	if _, err = s.db.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?)`, manualPilotSessionPrefix+session.ID, "malformed synthetic sentinel"); err != nil {
		t.Fatal(err)
	}
	other, err := Open(s.path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if claimed, err := other.ClaimCommandForCollector(ctx, command.RunID, "synthetic", "headless"); err == nil || claimed != nil {
		t.Fatal("reopened normal claim bypassed persisted fence")
	}
	if _, err = s.db.ExecContext(ctx, `UPDATE bridge_commands SET status='claimed',claimed_at=? WHERE id=?`, time.Now().Add(-time.Hour).Format(time.RFC3339Nano), command.ID); err != nil {
		t.Fatal(err)
	}
	if err = other.SaveObservation(ctx, command.ID, command.RunID, domain.Observation{Source: domain.SourceX}); err == nil {
		t.Fatal("generic observation bypassed persisted fence")
	}
	if _, err = other.FailCommand(ctx, command.ID, command.RunID, domain.Failure{}); err == nil {
		t.Fatal("generic command failure bypassed fence")
	}
	if err = other.FailRun(ctx, command.RunID, domain.Failure{}); err == nil {
		t.Fatal("generic run failure bypassed fence")
	}
	if expired, err := other.ExpiredBridgeCommands(ctx, time.Now().Add(time.Hour)); err != nil || len(expired) != 0 {
		t.Fatal("generic recovery exposed protected work", expired, err)
	}
	var status string
	if err = other.db.QueryRowContext(ctx, `SELECT status FROM bridge_commands WHERE id=?`, command.ID).Scan(&status); err != nil || status != "claimed" {
		t.Fatal("guard changed command", status, err)
	}
	var observations int
	if err = other.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM observations`).Scan(&observations); err != nil || observations != 0 {
		t.Fatal("guard changed observations", observations, err)
	}
}
