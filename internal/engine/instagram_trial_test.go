//go:build auth_session_dev

package engine

import (
	"context"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"testing"
)

func TestInstagramTrialRequiresSeparateProcessConsent(t *testing.T) {
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
	if granted, err := e.store.DailySourcePermission(ctx, domain.SourceInstagram, d.state.profileScope); err != nil || granted {
		t.Fatal("trial permission persisted")
	}
	status := e.SourceCaptureStatus(domain.SourceInstagram)
	if status.AuthorizationPolicy != "process" || status.RememberedPermission || !status.DerivationAuthorized || f.derivations != 0 {
		t.Fatalf("trial status %+v", status)
	}
}

func TestInstagramTrialDoesNotRestoreAnEarlierSavedGrant(t *testing.T) {
	e, _ := testEngine(t)
	f := attachInstagramOffline(t, e)
	e.config.Capture.InstagramTrial = true
	d := e.dailyController(domain.SourceInstagram)
	if err := e.store.SaveDailySourcePermission(context.Background(), domain.SourceInstagram, d.state.profileScope, true); err != nil {
		t.Fatal(err)
	}
	if err := d.resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if d.ready() || f.derivations != 0 {
		t.Fatal("trial restored permission across process boundary")
	}
}
