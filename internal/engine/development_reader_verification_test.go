//go:build windows && auth_session_fixture && auth_session_dev

package engine

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDevelopmentReaderUsesBoundExpectedAuthorityBeforeMarker(t *testing.T) {
	for _, mode := range []string{"valid", "unauthenticated_authority", "equal_foreign_account", "empty_revision", "source_profile", "stale_owner"} {
		t.Run(mode, func(t *testing.T) {
			h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
			cap := developmentCapability(h)
			authority, browser := cap.Authority, cap.Browser
			var changed atomic.Bool
			cap.Authority = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentSessionEvidence, error) {
				v, err := authority(ctx, request)
				if changed.Load() {
					switch mode {
					case "unauthenticated_authority":
						v.AuthState = DevelopmentSessionAuthUnknown
					case "equal_foreign_account":
						v.AccountID = "beta"
					case "empty_revision":
						v.Revision = ""
					case "source_profile":
						v.ProfileID = "foreign"
					}
				}
				return v, err
			}
			cap.Browser = func(ctx context.Context, request DevelopmentSessionProbeRequest) (DevelopmentBrowserEvidence, error) {
				v, err := browser(ctx, request)
				if changed.Load() {
					switch mode {
					case "equal_foreign_account":
						v.AccountID = "beta"
					case "stale_owner":
						v.Generation++
					}
				}
				return v, err
			}
			bindDevelopmentHarness(t, h, cap)
			h.borrow(t)
			changed.Store(true)
			if mode == "valid" {
				if err := h.e.VerifyDevelopmentReader(h.ctx); err != nil {
					t.Fatal(err)
				}
				return
			}
			backend := &processFixtureBackend{}
			backend.current.Store(&chromeFixtureProcess{pid: uint32(h.m.PID()), nativeReaderStartup: true})
			runtime := &DevelopmentFixtureRuntime{Engine: h.e, Owner: h.m, backend: backend}
			// A nil bridge deliberately fails the test if denial reaches marker/RPC.
			if _, err := runtime.PrepareNativePost(h.ctx, "synthetic-action", "https://x.com/aku_fixture_only/status/1", "/split-reader-intent?id=synthetic-action"); err == nil {
				t.Fatal("unverified reader reached marker")
			}
			changed.Store(false)
			if err := h.e.VerifyDevelopmentReader(h.ctx); err == nil {
				t.Fatal("failed authority silently recovered")
			}
		})
	}
}

func TestDevelopmentPauseRequiresInitializedReadyHeadlessBoundary(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	if err := h.e.PauseDevelopmentSession(h.ctx); err == nil {
		t.Fatal("unbound dev pause enabled")
	}
	bindDevelopmentHarness(t, h, developmentCapability(h))
	if err := h.e.PauseDevelopmentSession(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.c.Retry()
	time.Sleep(40 * time.Millisecond)
	if h.e.fixtureAdmissionAllowed() {
		t.Fatal("coordinator polling bypassed lifecycle hold")
	}
	next := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	bindDevelopmentHarness(t, next, developmentCapability(next))
	next.borrow(t)
	if err := next.e.PauseDevelopmentSession(next.ctx); err == nil {
		t.Fatal("reader state admitted lifetime pause")
	}
}
