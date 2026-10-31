//go:build windows

package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/ownedrpc"
)

// This exercises the real Go client and Node server/registry in different
// processes. Only owner behavior and the browser-facing CDP streams are fake.
func TestMainBrowserSourceRetryCrossProcess(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the offline Go-Node protocol regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	helper, err := filepath.Abs(filepath.Join("testdata", "source-retry-cross-process.mjs"))
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
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	var wireMu sync.Mutex
	var writes [][]byte
	writer := mainBrowserTestWriter(func(raw []byte) (int, error) {
		wireMu.Lock()
		writes = append(writes, append([]byte(nil), raw...))
		wireMu.Unlock()
		return stdin.Write(raw)
	})
	const nonce = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	client := &mainBrowserClient{rpc: ownedrpc.New(writer), ready: make(chan error, 1), nonce: nonce, diagnostic: newMainWorkerDiagnostic(nil)}
	readDone := make(chan struct{})
	go func() { defer close(readDone); client.read(stdout) }()
	// Wait starts only after the stdout reader exits, as required by StdoutPipe.
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
	if err = json.NewEncoder(stdin).Encode(map[string]any{"op": "main_transport_bootstrap", "schema": mainBrowserSchema, "nonce": nonce, "transport": "inherited-stdio"}); err != nil {
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
	call := func(operation string, params map[string]any) json.RawMessage {
		t.Helper()
		raw, err := client.CallSource(ctx, "instagram", operation, params)
		if err != nil {
			t.Fatalf("%s: %v", operation, err)
		}
		return raw
	}
	reject := func(operation string, params map[string]any, want string) {
		t.Helper()
		_, err := client.CallSource(ctx, "instagram", operation, params)
		var failure *MainBrowserError
		if !errors.As(err, &failure) || failure.Code != want {
			t.Fatalf("%s rejection: got %v, want %s", operation, err, want)
		}
	}
	empty := map[string]any{}
	call("source_bootstrap", empty)
	call("source_initialize", empty)
	reject("source_seed", empty, "native_source_evidence_document_changed")
	reject("source_retry", empty, "native_source_admission_retry")
	call("source_end", empty)
	// Null and invented authority fields must be rejected by the actual Node
	// registry before the synthetic owner sees them or consumes the retry.
	reject("source_retry", nil, "native_source_unavailable")
	reject("source_retry", map[string]any{"diagnostic": map[string]any{"reason": "document_changed", "cleanup": "verified"}}, "native_source_operation")

	var receipt struct {
		Rearmed    bool `json:"rearmed"`
		RetryCount int  `json:"retryCount"`
		RetryLimit int  `json:"retryLimit"`
		Descriptor struct {
			Source          string `json:"source"`
			Recipe          string `json:"recipe"`
			ReaderOwnerID   string `json:"readerOwnerID"`
			ReaderProfileID string `json:"readerProfileID"`
		} `json:"descriptor"`
	}
	raw := call("source_retry", empty)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&receipt); err != nil || !receipt.Rearmed || receipt.RetryCount != 1 || receipt.RetryLimit != 2 ||
		receipt.Descriptor.Source != "instagram" || receipt.Descriptor.Recipe != "instagram-native-v1" || receipt.Descriptor.ReaderOwnerID != "123" || receipt.Descriptor.ReaderProfileID != "synthetic-profile" {
		t.Fatalf("rearm receipt decode: %+v, %v", receipt, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("trailing rearm receipt data: %v", err)
	}
	wireMu.Lock()
	request := append([]byte(nil), writes[len(writes)-1]...)
	beforeWrongSource := len(writes)
	wireMu.Unlock()
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(request, &envelope); err != nil || len(envelope) != 5 || string(envelope["op"]) != `"main_source"` || string(envelope["source"]) != `"instagram"` || string(envelope["operation"]) != `"source_retry"` || string(envelope["params"]) != `{}` {
		t.Fatalf("source_retry wire changed: %s (%v)", request, err)
	}
	reject("source_retry", empty, "native_source_admission_retry")
	if _, err := client.CallSource(ctx, "x", "source_retry", empty); err == nil {
		t.Fatal("Go source client accepted X source_retry")
	}
	wireMu.Lock()
	if len(writes) != beforeWrongSource+1 {
		t.Errorf("Go wrong-source rejection wrote control bytes")
	}
	wireMu.Unlock()
	// Also bypass CallSource's source guard to prove Node independently rejects
	// the operation for a registered non-IG source over the same real pipe.
	if _, err := client.request(ctx, "main_source", map[string]any{"source": "x", "operation": "source_bootstrap", "params": empty}); err != nil {
		t.Fatal(err)
	}
	_, err = client.request(ctx, "main_source", map[string]any{"source": "x", "operation": "source_retry", "params": empty})
	var failure *MainBrowserError
	if !errors.As(err, &failure) || failure.Code != "native_source_operation" {
		t.Fatalf("Node wrong-source rejection: %v", err)
	}
	if _, err := client.CallX(ctx, "daily_owner_bootstrap", empty); err != nil {
		t.Fatal(err)
	}
	x, err := client.CallX(ctx, "daily_reader_check", empty)
	if err != nil || string(x) != `{"source":"x","usable":true}` {
		t.Fatalf("X lane lost after source retry: %s, %v", x, err)
	}
	if got := string(call("source_check", empty)); got != `{"source":"instagram","usable":true}` {
		t.Fatalf("IG owner was closed or replaced: %s", got)
	}
	version, err := client.Call(ctx, "Browser.getVersion", nil, "")
	if err != nil || !strings.Contains(string(version), `"product":"Chrome/offline-source-retry-fixture"`) {
		t.Fatalf("shared transport lost after source retry: %s, %v", version, err)
	}
}
