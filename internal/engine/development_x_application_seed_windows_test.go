//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestXApplicationSeedPrivateReceiptAndReplay(t *testing.T) {
	cfg := applicationConfig(t, "seed-valid")
	b, err := startXApplicationOwner(context.Background(), context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer closeApplication(t, b)
	if _, _, err = b.initializeNative(context.Background()); err != nil {
		t.Fatal("synthetic role initialization failed")
	}
	seed, start, err := b.seedNative(context.Background())
	if err != nil || !nativePilotAccount(seed.AccountID) || start.IsZero() || time.Since(start) >= nativePilotLife {
		t.Fatal("synthetic seed receipt rejected")
	}
	if !b.seedReady {
		t.Fatal("validated seed not latched")
	}
	for _, format := range []string{"%s", "%v", "%#v"} {
		if out := fmt.Sprintf(format, seed); strings.Contains(out, seed.AccountID) || strings.Contains(out, cfg.reader.Directory) {
			t.Fatal("private receipt leaked")
		}
	}
	if _, err = json.Marshal(seed); err == nil {
		t.Fatal("private account receipt serialized")
	}
	if _, _, err = b.seedNative(context.Background()); err == nil {
		t.Fatal("seed replay accepted")
	}
	if err = b.rpcRoundTripObserved(context.Background(), "native_owner_seed", nil, nil, nil); err == nil {
		t.Fatal("generic seed bypass")
	}
}

func TestXApplicationSeedRejectsMalformedAndFailedOnce(t *testing.T) {
	for _, mode := range []string{"seed-extra", "seed-badaccount", "seed-swapped", "seed-failure"} {
		t.Run(mode, func(t *testing.T) {
			cfg := applicationConfig(t, mode)
			b, err := startXApplicationOwner(context.Background(), context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer closeApplication(t, b)
			if _, _, err = b.initializeNative(context.Background()); err != nil {
				t.Fatal("synthetic role initialization failed")
			}
			_, _, err = b.seedNative(context.Background())
			if err == nil || b.seedReady || !b.isClosed() {
				t.Fatal("failed seed did not retire owner")
			}
			if strings.Contains(err.Error(), cfg.reader.Directory) || strings.Contains(err.Error(), "1908123456789012345") {
				t.Fatal("seed diagnostic leaked")
			}
			if mode == "seed-failure" && err.Error() != "manual_seed_seed_write_unavailable_unknown" {
				t.Fatal("closed first cause lost")
			}
		})
	}
}

func TestXApplicationSeedConsentScopeAndDefaultOff(t *testing.T) {
	var disabled xApplicationOwnerConfig
	if newXApplicationManualConsent(disabled, true) != nil {
		t.Fatal("disabled consent issued")
	}
	cfg := applicationConfig(t, "normal")
	if newXApplicationManualConsent(cfg, false) != nil {
		t.Fatal("missing limitation acknowledgement accepted")
	}
	consent := newXApplicationManualConsent(cfg, true)
	other := cfg
	other.driver.digest = strings.Repeat("a", 64)
	e := &Engine{config: config.Config{Dev: true}}
	for _, test := range []struct {
		cfg     xApplicationOwnerConfig
		consent *xApplicationManualConsent
	}{{disabled, consent}, {other, consent}, {cfg, nil}} {
		factory := e.xApplicationNativeFactory(test.cfg, test.consent, func(context.Context, xManualPilotWindowsRole, readerbroker.Request, string) (readerbroker.NativePostPreparation, error) {
			return readerbroker.NativePostPreparation{}, nil
		})
		if _, err := factory(context.Background(), context.Background()); err == nil {
			t.Fatal("unselected factory started")
		}
	}
	if consent.used.Load() {
		t.Fatal("mismatched preflight consumed consent")
	}
}

func TestXApplicationSeedClosedDiagnosticVocabulary(t *testing.T) {
	for _, sample := range []struct {
		code  string
		valid bool
	}{
		{"manual_seed_seed_write_unavailable_unknown", true},
		{"manual_seed_v2_9_3_0_v6_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_4", true},
		{"manual_seed_private_secret", false},
		{"manual_seed_owner_after_PRIVATE_acknowledged", false},
		{"manual_seed_v2_09_3_0_v6_0_8_7_0_19_0_0_0_0_0_8_6_20_1_3_2_1_4", false},
	} {
		if _, ok := nativePilotSeedCode(sample.code); ok != sample.valid {
			t.Fatal("closed diagnostic vocabulary mismatch")
		}
	}
}
