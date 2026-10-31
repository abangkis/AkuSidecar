//go:build windows

package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
)

// This fault-injecting Node peer tests real process pipes and the real Go
// reader. It cannot access a browser or an account.
func TestMainBrowserNoDispatchMalformedCrossProcess(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the offline Go-Node protocol regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	helper, err := filepath.Abs(filepath.Join("testdata", "source-retry-malformed-cross-process.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, node, helper)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	const nonce = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &mainBrowserClient{rpc: ownedrpc.New(stdin), ready: make(chan error, 1), nonce: nonce, diagnostic: newMainWorkerDiagnostic(nil)}
	readDone := make(chan struct{})
	go func() { defer close(readDone); client.read(stdout) }()
	processDone := make(chan error, 1)
	go func() { <-readDone; processDone <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = stdin.Close()
		client.rpc.Close()
		select {
		case err := <-processDone:
			if err != nil {
				t.Errorf("offline Node peer failed: %v; stderr=%s", err, stderr.String())
			}
		case <-time.After(3 * time.Second):
			cancel()
			<-processDone
			t.Error("offline Node peer did not exit after control EOF")
		}
	})
	if err := json.NewEncoder(stdin).Encode(map[string]any{"op": "main_transport_bootstrap", "schema": mainBrowserSchema, "nonce": nonce, "transport": "inherited-stdio"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-client.ready:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	tests := []struct {
		name, wantCode, diagnostic string
	}{
		{"exact", "main_ig_retry_not_dispatched", ""},
		{"exact with whitespace", "main_ig_retry_not_dispatched", ""},
		{"conflicting duplicate code", "main_transport_rejected", ""},
		{"identical duplicate code", "main_transport_rejected", ""},
		{"escaped duplicate key", "main_transport_rejected", ""},
		{"noncanonical key", "main_transport_rejected", ""},
		{"unknown field", "main_transport_rejected", ""},
		{"null diagnostic", "main_transport_rejected", ""},
		{"private diagnostic", "main_transport_rejected", ""},
		{"ordinary error retains diagnostic", "native_source_admission_retry", `{"stage":"fixture"}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := client.CallSource(ctx, "instagram", "source_retry", map[string]any{})
			var failure *MainBrowserError
			if !errors.As(err, &failure) || failure.Code != test.wantCode || string(failure.Diagnostic) != test.diagnostic {
				t.Fatalf("unexpected error classification: %v", err)
			}
		})
	}
	x, err := client.CallX(ctx, "daily_reader_check", map[string]any{})
	if err != nil || string(x) != `{"source":"x","usable":true}` {
		t.Fatalf("X lane lost after rejected retry frames: %s, %v", x, err)
	}
	version, err := client.Call(ctx, "Browser.getVersion", nil, "")
	if err != nil || string(version) != `{"product":"Chrome/offline-fault-fixture"}` {
		t.Fatalf("shared transport lost after rejected retry frames: %s, %v", version, err)
	}
}
