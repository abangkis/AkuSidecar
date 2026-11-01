//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	// Initialization has causally completed before authorization is requested.
	// Its elapsed time must not be silently included in the returned seed clock.
	preflightCompleted := time.Now()
	seed, start, err := b.seedNative(context.Background())
	if err != nil || !nativePilotAccount(seed.AccountID) || start.Before(preflightCompleted) || time.Since(start) >= nativePilotLife {
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
	if _, replayStart, replayErr := b.seedNative(context.Background()); replayErr == nil || !replayStart.IsZero() {
		t.Fatal("seed replay accepted or minted another authorization clock")
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

// Exercise the production validator with real Node output, including all closed
// first causes and the longest V10 trace. No browser or profile is accessed.
func TestXApplicationSeedNodeDiagnosticLength(t *testing.T) {
	node, project := os.Getenv("AKU_CHROME_FIXTURE_NODE"), os.Getenv("AKU_NATIVE_MANUAL_PROJECT_ROOT")
	if node == "" {
		var err error
		node, err = exec.LookPath("node")
		if err != nil {
			t.Fatal("offline Node unavailable")
		}
	}
	if project == "" {
		_, file, _, ok := runtime.Caller(0)
		if !ok {
			t.Fatal("offline source path unavailable")
		}
		project = filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	}
	const source = `import {STARTUP_TRACE,STARTUP_AGES,startupFailure} from './internal/collection/headless/worker/native-pilot/shared/x-manual-startup-diagnostic.mjs';
import {manualSeedFailure,manualSeedDiagnosticCode,MANUAL_SEED_CODE_MAX_BYTES} from './internal/collection/headless/worker/native-pilot/shared/x-manual-retained-setup.mjs';
const traces=STARTUP_TRACE.firstCause.map(firstCause=>({firstCause}));
traces.push({...Object.fromEntries(Object.entries(STARTUP_TRACE).map(([key,values])=>[key,values.at(-1)])),...Object.fromEntries(STARTUP_AGES.map(key=>[key,60000]))});
const codes=traces.map(trace=>manualSeedDiagnosticCode(manualSeedFailure('owner_after','clock_regressed','acknowledged',startupFailure('unknown','guard_bind','time_invalid',true,undefined,trace))));
console.log(JSON.stringify({maxBytes:MANUAL_SEED_CODE_MAX_BYTES,codes}));`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--input-type=module", "--eval", source)
	cmd.Dir = project
	raw, err := cmd.Output()
	if err != nil {
		t.Fatal("offline Node seed diagnostic emission failed")
	}
	defer clear(raw)
	var payload struct {
		MaxBytes int
		Codes    []string
	}
	if len(raw) > 32768 || json.Unmarshal(raw, &payload) != nil || payload.MaxBytes != nativePilotSeedCodeMax || len(payload.Codes) != len(nativePilotTraceCatalogs[0])+1 {
		t.Fatal("Node-Go seed length contract disagrees")
	}
	for _, code := range payload.Codes {
		if len(code) <= 80 || len(code) > nativePilotSeedCodeMax {
			t.Fatal("V10 seed did not exercise the expanded byte bound")
		}
		if got, ok := nativePilotSeedCode(code); !ok || got != code {
			t.Fatal("valid Node seed diagnostic rejected")
		}
		failure := xDailyNativeStartupFail(&xApplicationRPCError{code: code}, "seed_rpc", "rpc", nil, nativePilotDiagnostic{})
		projected := xDailyStartupDetail(failure)
		if projected == nil || !strings.HasPrefix(projected.Origin, "seed_phase=owner_after seed_reason=clock_regressed seed_write=acknowledged initial=(role=unknown phase=guard_bind reason=time_invalid kind=peer ") ||
			projected.Stage != "seed_rpc" || projected.Condition != "rpc" || projected.Cause != "unavailable" {
			t.Fatal("Node seed diagnostic lost its closed metadata in daily status")
		}
	}
	code := payload.Codes[len(payload.Codes)-1]
	for _, invalid := range []string{
		strings.Replace(code, "_10_12_2_", "_010_12_2_", 1),
		strings.Replace(code, "_10_12_2_", "_11_12_2_", 1),
		strings.Replace(code, "_60001", "_60002", 1),
		code + "_0", code + strings.Repeat("0", nativePilotSeedCodeMax),
	} {
		if _, ok := nativePilotSeedCode(invalid); ok {
			t.Fatal("expanded seed byte bound admitted an invalid diagnostic")
		}
	}
}
