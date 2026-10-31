package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const workerLifecycleFixture = `{"event":"main_transport_lifecycle","result":{"schema":"aku.main-transport-lifecycle.v1","reason":"command_timeout","domain":"cdp","method":"IO.read","role":"ig_reader","stage":"command"}}`

func workerSnapshot(t *testing.T, r *mainWorkerDiagnostic) mainWorkerSnapshot {
	t.Helper()
	raw := r.json()
	if !ValidateMainBrowserWorkerDiagnostic(raw) {
		t.Fatalf("invalid safe snapshot: %s", raw)
	}
	var v mainWorkerSnapshot
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestMainWorkerLifecycleClosedProjection(t *testing.T) {
	c, found, valid := mainWorkerLifecycleFrame([]byte(workerLifecycleFixture))
	if !found || !valid || c.Method != "IO.read" {
		t.Fatal("valid lifecycle lost")
	}
	for _, raw := range []string{
		strings.Replace(workerLifecycleFixture, `"reason":"command_timeout"`, `"reason":"PRIVATE_TOKEN"`, 1),
		strings.Replace(workerLifecycleFixture, `"result":{`, `"result":{"token":"PRIVATE_TOKEN",`, 1),
		strings.Replace(workerLifecycleFixture, `"result":{`, `"result":{"reason":"unknown",`, 1),
		strings.Replace(workerLifecycleFixture, `"event":`, `"id":1,"event":`, 1),
		workerLifecycleFixture + `{}`,
	} {
		if _, _, valid := mainWorkerLifecycleFrame([]byte(raw)); valid {
			t.Fatal("unsafe lifecycle accepted")
		}
	}
	if _, found, _ := mainWorkerLifecycleFrame([]byte(`{"event":"main_x_progress"}`)); found {
		t.Fatal("other lane claimed")
	}
}

func TestMainWorkerFirstCauseAndCleanupIndependent(t *testing.T) {
	count := 0
	var r *mainWorkerDiagnostic
	r = newMainWorkerDiagnostic(func(raw json.RawMessage) {
		count++
		if !ValidateMainBrowserWorkerDiagnostic(raw) {
			t.Fatal("unsafe observer bytes")
		}
		// Observers may inspect the recorder; invocation must be outside locks.
		_ = r.json()
	})
	c, _, _ := mainWorkerLifecycleFrame([]byte(workerLifecycleFixture))
	r.report(c)
	r.intentional()
	r.readEnded("eof")
	c.Reason = "explicit_close"
	r.report(c)
	r.exited(0, errors.New("PRIVATE_CLEANUP"))
	r.intentional()
	v := workerSnapshot(t, r)
	if !v.Reported || v.Reason != "command_timeout" || v.Method != "IO.read" || v.Cleanup != "uncertain" || !v.Intentional || !v.Exited || *v.ExitCode != 0 || count < 4 {
		t.Fatalf("cause or cleanup overwritten: %+v", v)
	}
	if strings.Contains(string(r.json()), "PRIVATE") {
		t.Fatal("raw cleanup escaped")
	}
}

func TestMainWorkerLateOwnerShutdownCannotResetCleanup(t *testing.T) {
	r := newMainWorkerDiagnostic(nil)
	r.exited(0, nil)
	r.intentional()
	if v := workerSnapshot(t, r); v.Cleanup != "verified" || !v.Intentional {
		t.Fatal("cleanup settlement regressed")
	}
	raw := r.json()
	for _, mutation := range []string{
		strings.Replace(string(raw), `"reported":false`, `"reported":null`, 1),
		strings.Replace(string(raw), `"cleanup":"verified"`, `"cleanup":"PRIVATE"`, 1),
		strings.Replace(string(raw), `"schema":`, `"token":"PRIVATE","schema":`, 1),
		strings.Replace(string(raw), `"schema":`, `"reported":true,"schema":`, 1),
	} {
		if ValidateMainBrowserWorkerDiagnostic([]byte(mutation)) {
			t.Fatal("unsafe or malformed snapshot accepted")
		}
	}
}

func TestMainWorkerMissingReportNeverInventsExitCause(t *testing.T) {
	r := newMainWorkerDiagnostic(nil)
	r.readEnded("eof")
	r.exited(1, nil)
	v := workerSnapshot(t, r)
	if v.Reported || v.Complete || v.Reason != "unknown" || v.ControlRead != "eof" || v.Cleanup != "verified" {
		t.Fatalf("invented cause: %+v", v)
	}
	// A control EOF can race the final frame; a validated Node cause remains
	// useful, while the record remains incomplete after missing delivery.
	c, _, _ := mainWorkerLifecycleFrame([]byte(workerLifecycleFixture))
	r.report(c)
	if v = workerSnapshot(t, r); v.Reason != "command_timeout" || v.Complete {
		t.Fatal("late cause or incompleteness lost")
	}
}

func TestMainWorkerErrorCarrierPreservesOperationalCause(t *testing.T) {
	r := newMainWorkerDiagnostic(nil)
	cause := &MainBrowserError{Code: "native_source_wire_unavailable"}
	err := fmt.Errorf("outer: %w", &mainBrowserSourceError{cause: cause, diagnostic: r.json})
	var wire *MainBrowserError
	if !errors.As(err, &wire) || wire != cause || !ValidateMainBrowserWorkerDiagnostic(MainBrowserWorkerFailureDiagnostic(err)) {
		t.Fatal("operational error or sideband lost")
	}
	wrapped := &mainBrowserSourceError{cause: context.DeadlineExceeded, diagnostic: r.json}
	if !errors.Is(wrapped, context.DeadlineExceeded) {
		t.Fatal("deadline classification changed")
	}
	r.readEnded("eof")
	r.exited(7, nil)
	if !strings.Contains(string(MainBrowserWorkerFailureDiagnostic(err)), `"exitCode":7`) {
		t.Fatal("exit settlement frozen too early")
	}
	bad := &mainBrowserSourceError{cause: cause, diagnostic: func() json.RawMessage { return json.RawMessage(`{"token":"PRIVATE"}`) }}
	if len(MainBrowserWorkerFailureDiagnostic(bad)) != 0 {
		t.Fatal("unsafe diagnostic carrier escaped")
	}
}
