//go:build auth_session_dev

package store

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"strings"
	"testing"
	"time"
)

func TestDailyPilotRunScopePreservesOtherSources(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	settings, _ := s.GetSettings(ctx)
	settings.CollectionMode = "browser"
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceLinkedIn}
	session, err := s.CreateDailyXUpdateSession(ctx, "synthetic daily mixed", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		t.Fatal(err)
	}
	drivers := session.Coverage["collectionSourceDrivers"].(map[string]any)
	if drivers["x"] != "headless" || drivers["linkedin"] != "browser" {
		t.Fatalf("routing changed %+v", drivers)
	}
	x, other := session.Runs[0], session.Runs[1]
	command, err := s.StartRun(ctx, x.ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
	if err != nil {
		t.Fatal(err)
	}
	start := s.Now()
	binding := DevelopmentManualPilotBinding{Digest: strings.Repeat("a", 64), Attempt: "synthetic-daily", OwnerGeneration: 1, StartedAt: start, Deadline: start.Add(time.Minute)}
	p, err := s.NewDevelopmentDailyPilotIssuer(func(context.Context) (DevelopmentManualPilotBinding, error) { return binding, nil })(ctx, session.ID, x.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ClaimCommand(ctx, x.ID, "ordinary"); err == nil {
		t.Fatal("ordinary X bypassed daily permit")
	}
	claim, err := s.ClaimManualPilot(ctx, p, command.ID, "daily-claim")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.CommitManualPilotResult(ctx, claim.Handle, "daily-result", manualObservation()); err != nil {
		t.Fatal(err)
	}
	if got, err := s.IsDailyXCapture(ctx, x.ID); err != nil || !got {
		t.Fatal("trusted acquisition receipt absent", err)
	}
	if _, err = s.RevokeManualPilot(ctx, p, "daily-revoke"); err != nil {
		t.Fatal(err)
	}
	if len(s.manualPilots) != 0 {
		t.Fatal("completed daily permit retained in process memory")
	}
	if err = s.RejectUnavailableXCapture(ctx, x.ID); err != nil {
		t.Fatal("accepted reasoning blocked by persistent daily marker", err)
	}
	if _, err = s.StartRun(ctx, other.ID, map[string]any{}); err != nil {
		t.Fatal("non-X blocked by daily scope", err)
	}
	if _, err = s.ClaimCommand(ctx, other.ID, "ordinary"); err != nil {
		t.Fatal("non-X ordinary claim blocked", err)
	}
}

func TestDailyPilotIssuerRejectsExtendedDeadline(t *testing.T) {
	s := openTestStore(t)
	now := s.Now()
	issuer := s.NewDevelopmentDailyPilotIssuer(func(context.Context) (DevelopmentManualPilotBinding, error) {
		return DevelopmentManualPilotBinding{Digest: strings.Repeat("a", 64), Attempt: "synthetic", OwnerGeneration: 1, StartedAt: now, Deadline: now.Add(61 * time.Second)}, nil
	})
	if p, err := issuer(context.Background(), "synthetic-session", "synthetic-run"); err == nil || p != nil {
		t.Fatal("extended deadline issued")
	}
}

func TestDailyPilotPolicyCannotComeFromObservationCoverage(t *testing.T) {
	s := openTestStore(t)
	settings, _ := s.GetSettings(context.Background())
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := createVisibleUpdateSession(s, context.Background(), "ordinary", settings)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SetRunCoverageField(context.Background(), session.Runs[0].ID, "dailyXAcquisitionPolicy", "round_one_v1"); err != nil {
		t.Fatal(err)
	}
	if trusted, err := s.IsDailyXCapture(context.Background(), session.Runs[0].ID); err != nil || trusted {
		t.Fatal("caller coverage selected trusted policy", err)
	}
}
