//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// The child runs only an accountless fixed recipe on inherited stdio. It has
// neither a Chrome/profile command nor a native provenance path. Private reply
// bytes never reach test logs; output allocation is bounded independently.
type nativePilotNodeBuffer struct{ bytes.Buffer }

func TestNativePilotNodePreflightDiagnostics(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("offline Node runtime unavailable")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("offline fixture location unavailable")
	}
	module := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "auth-session-fixture", "x-manual-startup-diagnostic.mjs")
	const script = `import{pathToFileURL}from'node:url';const m=await import(pathToFileURL(process.argv[1]));const codes=[];for(const phase of ['navigation','page_ready','guard_bind'])for(const reason of ['unavailable','rejected','deadline'])for(const trace of [undefined,{firstCause:'unknown'}])codes.push(m.startupRPCCode(m.startupFailure('collector',phase,reason,false,undefined,trace)));console.log(JSON.stringify({phases:m.STARTUP_PHASES,codes}));`
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, "--input-type=module", "-e", script, module)
	var output nativePilotNodeBuffer
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	if cmd.Run() != nil {
		t.Fatal("offline diagnostic projection failed")
	}
	defer clear(output.Bytes())
	var result struct {
		Phases []string `json:"phases"`
		Codes  []string `json:"codes"`
	}
	if json.Unmarshal(output.Bytes(), &result) != nil || !slices.Equal(result.Phases, nativePilotInitialPhases) || len(result.Codes) != 18 {
		t.Fatal("Node and Go startup phase catalogs diverged")
	}
	i := 0
	for _, phase := range []string{"navigation", "page_ready", "guard_bind"} {
		for _, reason := range []string{"unavailable", "rejected", "deadline"} {
			for _, traced := range []bool{false, true} {
				code := result.Codes[i]
				i++
				detail, ok := nativePilotInitialCode(code)
				if !ok || len(code) > 80 || detail.role != "collector" || detail.phase != phase || detail.reason != reason || detail.peer || (detail.trace != nil) != traced {
					t.Fatal("preflight diagnostic changed across the private wire")
				}
				wrapped := nativePilotInitialOf(nativePilotInitialRPC(code), "collector", "identity")
				if wrapped.phase != phase || wrapped.reason != reason {
					t.Fatal("identity fallback replaced the preflight diagnostic")
				}
			}
		}
	}
	if _, ok := nativePilotInitialCode("manual_initial_collector_PRIVATE_unavailable_primary"); ok {
		t.Fatal("unknown phase accepted")
	}
}

func (b *nativePilotNodeBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > nativePilotStartupWireMax {
		return 0, errors.New("offline-node-output-limit")
	}
	return b.Buffer.Write(p)
}

func TestNativePilotNodeStartupControllerIntegration(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal("offline Node runtime unavailable")
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("offline fixture location unavailable")
	}
	script := filepath.Join(filepath.Dir(file), "..", "..", "scripts", "auth-session-fixture", "x-manual-startup-offline-fixture.mjs")
	for _, wrongDocument := range []bool{false, true} {
		t.Run(map[bool]string{false: "fixed-recipe-to-result", true: "actual-document-mismatch"}[wrongDocument], func(t *testing.T) {
			f := newTestNativePilotFixture()
			b := f.cfg.binding
			for role, owner := range map[string]*nativePilotOwner{"reader": &b.reader, "collector": &b.collector} {
				owner.owner, owner.context = "1", "7"
				if role == "collector" {
					owner.owner, owner.context = "2", "8"
				}
				owner.profile, owner.target = "offline-profile-"+role, "offline-target-"+role
				owner.frame, owner.document = "offline-frame-"+role, "offline-document-"+role
			}
			if wrongDocument {
				b.reader.document = "wrong-document"
			}
			f.cfg.binding, f.state.binding = b, b
			f.cfg.attemptStarted = time.Now()
			source, err := newNativePilotStartupSource(nativePilotStartupConfig{enabled: true, offlineOnly: true, binding: b,
				capture: func(ctx context.Context, challenge nativePilotCaptureChallenge) ([]byte, error) {
					request, err := challenge.requestJSON()
					if err != nil {
						return nil, errors.New("offline-node-request-invalid")
					}
					defer clear(request)
					cmd := exec.CommandContext(ctx, node, script, "--offline-manual-startup")
					cmd.Stdin, cmd.Stderr = bytes.NewReader(request), io.Discard
					var output nativePilotNodeBuffer
					cmd.Stdout = &output
					if cmd.Run() != nil {
						clear(output.Bytes())
						return nil, errors.New("offline-node-capture-rejected")
					}
					return output.Bytes(), nil
				}})
			if err != nil {
				t.Fatal("offline producer construction failed")
			}
			c, diagnostic := newNativePilotManualController(context.Background(), nativePilotManualConfig{enabled: true, offlineTest: true, pilot: f.cfg},
				nativePilotPorts{now: time.Now, source: source, owners: f.ports.owners})
			if c == nil {
				t.Fatalf("manual controller unavailable: %+v", diagnostic)
			}
			diagnostic = c.start(context.Background())
			if wrongDocument {
				if diagnostic.OfflineValidated || diagnostic.ManualEligible || diagnostic.CanAuthorize || diagnostic.StorePermit {
					t.Fatal("mismatched actual startup document was accepted")
				}
				return
			}
			assertNativePilotClosed(t, diagnostic, "manual-attempt-valid")
			if !diagnostic.OfflineValidated {
				t.Fatal("Node recipe did not reach manual controller")
			}
			ticket, diagnostic := c.dispatch(context.Background(), "offline-node-work")
			assertNativePilotClosed(t, diagnostic, "work-dispatched")
			claims, results := 0, 0
			assertNativePilotClosed(t, c.claim(context.Background(), ticket, func() bool { claims++; return true }), "work-claimed")
			assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { results++; return true }), "result-completed")
			assertNativePilotClosed(t, c.complete(context.Background(), ticket, func() bool { results++; return true }), "result-duplicate")
			if claims != 1 || results != 1 || ticket.collector != b.collector {
				t.Fatal("collector or exactly-once local result changed")
			}
			c.invalidate("opted-out")
			if _, d := c.dispatch(context.Background(), "after-stop"); d.OfflineValidated || d.ManualEligible {
				t.Fatal("manual attempt revived after opt-out")
			}
		})
	}
}
