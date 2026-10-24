//go:build auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

func TestManualRuntimeFixtureScenarios(t *testing.T) {
	for _, scenario := range manualRuntimeScenarios {
		t.Run(scenario, func(t *testing.T) {
			var output bytes.Buffer
			if err := RunDevelopmentXManualFixture(strings.NewReader(`{"grant":"synthetic-x-manual-owned","scenario":"`+scenario+`"}`), &output); err != nil {
				t.Fatal(err)
			}
			var report map[string]any
			if json.Unmarshal(output.Bytes(), &report) != nil || len(report) != 7 || report["provider"] != "synthetic-x-manual-fixture" || report["scenario"] != scenario || report["passed"] != true || report["offlineValidated"] != true || report["canAuthorize"] != false || report["nativeQualified"] != false || report["storePermit"] != false {
				t.Fatalf("closed report mismatch: %s", output.Bytes())
			}
		})
	}
}

func TestManualRuntimeFixtureRejectsInput(t *testing.T) {
	for _, raw := range []string{
		``, `null`, `[]`, `{}`, `{"grant":null,"scenario":"overlap"}`, `{"grant":"synthetic-x-manual-owned","scenario":null}`,
		`{"Grant":"synthetic-x-manual-owned","scenario":"overlap"}`,
		`{"grant":"synthetic-x-manual-owned","scenario":"overlap","scenario":"expiry"}`,
		`{"grant":"synthetic-x-manual-owned","grant":"synthetic-x-manual-owned","scenario":"overlap"}`,
		`{"grant":"synthetic-x-manual-owned","scenario":"overlap","nativeQualified":true}`,
		`{"grant":"trusted","scenario":"overlap"}`,
		`{"grant":"synthetic-x-manual-owned","scenario":"native"}`,
		`{"grant":"synthetic-x-manual-owned","scenario":"overlap"}{}`,
		`{"grant":"synthetic-x-manual-owned","scenario":"overlap"} trailing`, strings.Repeat(" ", 1025),
		`{"grant":"synthetic-x-manual-owned","scenario":"\ud800"}`,
	} {
		var output bytes.Buffer
		if RunDevelopmentXManualFixture(strings.NewReader(raw), &output) == nil || output.Len() != 0 {
			t.Fatalf("input accepted: %q", raw)
		}
	}
}

func TestManualRuntimeOwnerPins(t *testing.T) {
	for _, kind := range []string{"reader-missing-owner", "reader-wrong-pid", "reader-stale-generation", "collector-missing-owner", "collector-wrong-pid", "collector-stale-generation"} {
		t.Run(kind, func(t *testing.T) {
			f, err := newManualRuntimeFixture(false)
			if err != nil {
				t.Fatal(err)
			}
			defer f.close()
			role := "collector"
			if strings.HasPrefix(kind, "reader-") {
				role = "reader"
			}
			f.mu.Lock()
			owner := f.actual[role]
			switch {
			case strings.HasSuffix(kind, "missing-owner"):
				owner.owner = ""
			case strings.HasSuffix(kind, "wrong-pid"):
				owner.pid += 10
			case strings.HasSuffix(kind, "stale-generation"):
				owner.generation++
			}
			f.actual[role] = owner
			f.mu.Unlock()
			d := f.broker.start(context.Background())
			if strings.HasSuffix(kind, "missing-owner") {
				if nativePilotRuntimeAllowed(d) {
					t.Fatal("missing owner admitted")
				}
				return
			}
			// Validly shaped source pins still must match the actual Manager.
			if !nativePilotRuntimeAllowed(d) {
				t.Fatalf("startup failed before manager comparison: %+v", d)
			}
			if role == "reader" {
				err = f.route(context.Background(), manualRuntimeRequest())
			} else {
				_, err = f.runtime.capture(context.Background(), "owner-pins", domain.SourceX)
			}
			if err == nil || len(f.runtime.results) != 0 || f.opens != 0 {
				t.Fatal("manager mismatch admitted an external effect")
			}
		})
	}
}

func TestManualRuntimeSourceAndTicketFences(t *testing.T) {
	f, err := newManualRuntimeFixture(false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	// Fixed trusted backend metadata cannot be replaced through fixture JSON.
	f.mu.Lock()
	f.state.account = "29"
	f.mu.Unlock()
	if nativePilotRuntimeAllowed(f.broker.start(context.Background())) || len(f.acquired) != 0 {
		t.Fatal("mismatched offline account acquired")
	}
	good, err := newManualRuntimeFixture(false)
	if err != nil {
		t.Fatal(err)
	}
	defer good.close()
	if !nativePilotRuntimeAllowed(good.broker.start(context.Background())) {
		t.Fatal("valid startup unavailable")
	}
	if good.runtime.complete(context.Background(), nil, domain.Observation{Source: domain.SourceX}) == nil {
		t.Fatal("missing ticket accepted")
	}
	ticket, d := good.broker.dispatch(context.Background(), "synthetic-ticket-fences")
	if ticket == nil || !nativePilotRuntimeAllowed(d) {
		t.Fatal("valid dispatch unavailable")
	}
	if !nativePilotRuntimeAllowed(good.broker.claim(context.Background(), ticket, func() bool { return true })) {
		t.Fatal("valid claim unavailable")
	}
	stale := *ticket
	stale.version++
	if good.runtime.complete(context.Background(), &stale, domain.Observation{Source: domain.SourceX}) == nil {
		t.Fatal("stale ticket accepted before completion")
	}
	if good.runtime.complete(context.Background(), ticket, domain.Observation{Source: domain.SourceFacebook}) == nil {
		t.Fatal("wrong source completed")
	}
	if len(good.runtime.results) != 0 {
		t.Fatal("invalid completion mutated sink")
	}
}

func TestManualRuntimeBackendBinding(t *testing.T) {
	for _, kind := range []string{"pid", "generation", "missing", "post-startup-pin", "result-source"} {
		t.Run(kind, func(t *testing.T) {
			f, err := newManualRuntimeFixture(false)
			if err != nil {
				t.Fatal(err)
			}
			defer f.close()
			if kind == "post-startup-pin" || kind == "result-source" {
				if !nativePilotRuntimeAllowed(f.broker.start(context.Background())) {
					t.Fatal("startup unavailable")
				}
				if kind == "post-startup-pin" {
					f.backend.owner.document = "synthetic-unrelated-document"
				} else {
					f.backend.resultSource = domain.SourceFacebook
					close(f.backend.release)
				}
				if _, err := f.runtime.capture(context.Background(), "synthetic-backend-binding", domain.SourceX); err == nil || len(f.runtime.results) != 0 {
					t.Fatal("unbound backend result accepted")
				}
				return
			}
			other := manualRuntimeFixtureProcess{owner: f.backend.owner}
			switch kind {
			case "pid":
				other.owner.pid++
			case "generation":
				other.owner.generation++
			case "missing":
				other.owner.owner = ""
			}
			if _, err := newNativePilotRuntime(f.broker, f.reader, f.collector, &other, f.runtime.open); err == nil {
				t.Fatal("unrelated collector backend bound")
			}
		})
	}
}

func TestManualRuntimeExternalOperationsOutsideLocks(t *testing.T) {
	f, err := newManualRuntimeFixture(false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.close()
	if !nativePilotRuntimeAllowed(f.broker.start(context.Background())) {
		t.Fatal("startup unavailable")
	}
	probe := func() error {
		if !f.broker.mu.TryLock() {
			return errors.New("external operation holds broker lock")
		}
		f.broker.mu.Unlock()
		if !f.broker.controller.mu.TryLock() {
			return errors.New("external operation holds controller lock")
		}
		f.broker.controller.mu.Unlock()
		return nil
	}
	open := f.runtime.open
	f.runtime.open = func(ctx context.Context, request readerbroker.Request) error {
		if err := probe(); err != nil {
			return err
		}
		return open(ctx, request)
	}
	if err := f.route(context.Background(), manualRuntimeRequest()); err != nil {
		t.Fatal(err)
	}
	f.backend.probe = probe
	close(f.backend.release)
	if _, err := f.runtime.capture(context.Background(), "synthetic-unlocked-external", domain.SourceX); err != nil {
		t.Fatal(err)
	}
}
