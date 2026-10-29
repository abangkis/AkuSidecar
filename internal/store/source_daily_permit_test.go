//go:build auth_session_dev

package store

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSourceDailyPermitRejectsCrossSourceClaimAndResult(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	settings, _ := s.GetSettings(ctx)
	settings.ActiveSources = []domain.Source{domain.SourceX, domain.SourceInstagram}
	settings.CollectionMode = "browser"
	session, err := s.CreateDailySourceUpdateSession(ctx, "synthetic source isolation", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser}, []domain.Source{domain.SourceX, domain.SourceInstagram})
	if err != nil {
		t.Fatal(err)
	}
	commands := make(map[domain.Source]domain.BridgeCommand)
	for _, run := range session.Runs {
		command, err := s.StartRun(ctx, run.ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
		if err != nil {
			t.Fatal(err)
		}
		commands[run.Source] = command
	}
	now := s.Now()
	binding := DevelopmentManualPilotBinding{Digest: strings.Repeat("b", 64), Attempt: "synthetic:instagram-attempt", OwnerGeneration: 1, StartedAt: now, Deadline: now.Add(time.Minute)}
	issuer := s.NewDevelopmentSourceDailyPilotIssuer(domain.SourceInstagram, func(context.Context) (DevelopmentManualPilotBinding, error) { return binding, nil })
	if _, err := issuer(ctx, session.ID, commands[domain.SourceX].RunID); err == nil {
		t.Fatal("IG issuer accepted X run")
	}
	p, err := issuer(ctx, session.ID, commands[domain.SourceInstagram].RunID)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately share the attempt text across sources: run identity must keep
	// one source's revocation from retiring the other source's process permit.
	xPermit, err := s.NewDevelopmentSourceDailyPilotIssuer(domain.SourceX, func(context.Context) (DevelopmentManualPilotBinding, error) { return binding, nil })(ctx, session.ID, commands[domain.SourceX].RunID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RevokeManualPilot(ctx, xPermit, "revoke-x-shared-attempt"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ClaimManualPilot(ctx, p, commands[domain.SourceX].ID, "cross-source-claim"); err == nil {
		t.Fatal("IG permit claimed X command")
	}
	if claim, err := s.ClaimCommandForCollector(ctx, commands[domain.SourceInstagram].RunID, "ordinary", "headless"); err == nil || claim != nil {
		t.Fatal("ordinary collector bypassed IG permit")
	}
	claim, err := s.ClaimManualPilot(ctx, p, commands[domain.SourceInstagram].ID, "ig-claim")
	if err != nil {
		t.Fatal(err)
	}
	if out, err := s.CommitManualPilotResult(ctx, claim.Handle, "cross-source-result", domain.Observation{Source: domain.SourceX}); err == nil || out != RejectedNoMutation {
		t.Fatal("IG permit committed X result")
	}
	if err = s.CancelSession(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if out, err := s.CommitManualPilotResult(ctx, claim.Handle, "canceled-result", domain.Observation{Source: domain.SourceInstagram}); err == nil || out != RejectedNoMutation {
		t.Fatal("session cancellation missed IG permit")
	}
	assertManualCounts(t, s, 0)
}

func TestSourceDailyCorruptSourceFieldCannotRemoveDurableFence(t *testing.T) {
	for _, source := range []domain.Source{domain.SourceX, domain.SourceInstagram} {
		t.Run(string(source), func(t *testing.T) {
			ctx := context.Background()
			s := openTestStore(t)
			settings, _ := s.GetSettings(ctx)
			settings.ActiveSources = []domain.Source{source}
			settings.CollectionMode = "headless"
			session, err := createVisibleUpdateSession(s, ctx, "durable source fence fixture", settings)
			if err != nil {
				t.Fatal(err)
			}
			command, err := s.StartRun(ctx, session.Runs[0].ID, map[string]any{"captureRuntime": map[string]any{"driver": "headless"}})
			if err != nil {
				t.Fatal(err)
			}
			now := s.Now()
			binding := DevelopmentManualPilotBinding{Digest: strings.Repeat("a", 64), Attempt: "synthetic:fence", OwnerGeneration: 1, StartedAt: now, Deadline: now.Add(time.Minute)}
			if _, err = s.NewDevelopmentSourceDailyPilotIssuer(source, func(context.Context) (DevelopmentManualPilotBinding, error) { return binding, nil })(ctx, session.ID, command.RunID); err != nil {
				t.Fatal(err)
			}
			if _, err = s.db.ExecContext(ctx, `UPDATE meta SET value=json_set(value,'$.source','foreign') WHERE key=?`, manualPilotSessionPrefix+manualScopeID(session.ID, source)); err != nil {
				t.Fatal(err)
			}
			if claim, err := s.ClaimCommandForCollector(ctx, command.RunID, "ordinary", "headless"); claim != nil || err == nil {
				t.Fatal("mutable source field removed the durable fence")
			}
		})
	}
}
