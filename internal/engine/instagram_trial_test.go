//go:build auth_session_dev

package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestInstagramTrialRequiresSeparateExplicitConsentAndSavesProfileChoice(t *testing.T) {
	e, _ := testEngine(t)
	f := attachInstagramOffline(t, e)
	e.config.Capture.InstagramTrial = true
	ctx := context.Background()
	if err := e.StartInstagramTrial(ctx, XDailyStartRequest{true, true}, false); err == nil {
		t.Fatal("unqualified recipe acknowledgement missing")
	}
	if e.SourceCaptureStatus(domain.SourceInstagram).DerivationAuthorized {
		t.Fatal("rejected trial authorized")
	}
	if err := e.StartInstagramTrial(ctx, XDailyStartRequest{true, true}, true); err != nil {
		t.Fatal(err)
	}
	d := e.dailyController(domain.SourceInstagram)
	if granted, err := e.store.DailySourcePermission(ctx, domain.SourceInstagram, d.state.profileScope); err != nil || !granted {
		t.Fatalf("trial permission was not saved: granted=%t err=%v", granted, err)
	}
	status := e.SourceCaptureStatus(domain.SourceInstagram)
	if status.AuthorizationPolicy != "profile" || !status.RememberedPermission || !status.DerivationAuthorized || f.derivations != 0 {
		t.Fatalf("trial status %+v", status)
	}
	if granted, err := e.store.DailySourcePermission(ctx, domain.SourceX, d.state.profileScope); err != nil || granted {
		t.Fatal("IG consent leaked into X")
	}
}

func TestInstagramTrialPermissionSurvivesDatabaseReopenWithFreshVerification(t *testing.T) {
	ctx := context.Background()
	e, state := testEngine(t)
	attachInstagramOffline(t, e)
	e.config.Capture.InstagramTrial = true
	d := e.dailyController(domain.SourceInstagram)
	if err := e.StartInstagramTrial(ctx, XDailyStartRequest{true, true}, true); err != nil {
		t.Fatal(err)
	}
	firstOwner := d.state.owner
	dbPath, cfg, provider, logger := state.Path(), e.config, e.provider, e.logger
	if err := e.closeInstagramDaily(ctx); err != nil {
		t.Fatal(err)
	}
	e.Shutdown()
	if err := state.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(dbPath, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	restarted := New(reopened, provider, cfg, logger)
	f := attachInstagramOffline(t, restarted)
	checks := 0
	f.mutate = func(v *instagramSyntheticEvidence, collector bool) {
		if collector {
			t.Fatal("restoring consent created a collector")
		}
		checks++
	}
	if err := restarted.ResumeXDailyScheduling(ctx); err != nil {
		t.Fatal(err)
	}
	restored := restarted.dailyController(domain.SourceInstagram)
	status := restored.status()
	if !restored.ready() || !status.RememberedPermission || restored.state.owner == firstOwner || checks != 1 || f.derivations != 0 {
		t.Fatalf("restart did not verify a fresh owner without deriving: status=%+v checks=%d", status, checks)
	}
	if err := restored.resume(ctx); err != nil || checks != 1 {
		t.Fatalf("restore repeated: checks=%d err=%v", checks, err)
	}
	if err := restarted.RevokeInstagramDailyPermission(ctx); err != nil {
		t.Fatal(err)
	}
	if granted, err := reopened.DailySourcePermission(ctx, domain.SourceInstagram, restored.state.profileScope); err != nil || granted || restored.status().RememberedPermission || restored.ready() {
		t.Fatal("Stop IG did not revoke saved and process permission")
	}
}

func TestInstagramTrialRestoreRejectsOtherProfileAndFreshProofFailure(t *testing.T) {
	for _, mode := range []string{"other-profile", "fresh-proof-failed"} {
		t.Run(mode, func(t *testing.T) {
			e, state := testEngine(t)
			f := attachInstagramOffline(t, e)
			e.config.Capture.InstagramTrial = true
			d := e.dailyController(domain.SourceInstagram)
			scope := d.state.profileScope
			if mode == "other-profile" {
				scope = strings.Repeat("a", 64)
			} else {
				f.mutate = func(v *instagramSyntheticEvidence, _ bool) { v.Authenticated = false }
			}
			ctx := context.Background()
			if err := state.SaveDailySourcePermission(ctx, domain.SourceInstagram, scope, true); err != nil {
				t.Fatal(err)
			}
			if err := d.resume(ctx); err != nil {
				t.Fatal(err)
			}
			if d.ready() || f.derivations != 0 {
				t.Fatal("saved choice bypassed profile or fresh verification")
			}
			if d.status().RememberedPermission != (mode == "fresh-proof-failed") {
				t.Fatal("failure lost the saved choice or imported another profile's choice")
			}
		})
	}
}
