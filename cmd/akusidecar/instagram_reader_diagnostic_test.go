package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
)

type diagnosticOperatorProcess struct {
	done   chan error
	once   sync.Once
	driver string
}

func (p *diagnosticOperatorProcess) Done() <-chan error { return p.done }
func (p *diagnosticOperatorProcess) Terminate()         { p.once.Do(func() { close(p.done) }) }
func (p *diagnosticOperatorProcess) CloseForRetry(context.Context) error {
	return errors.New("must not replace")
}
func (*diagnosticOperatorProcess) PID() int { return 1234 }
func (*diagnosticOperatorProcess) OpenExtensionsPage(context.Context) error {
	return errors.New("must not foreground")
}
func (*diagnosticOperatorProcess) ReplacementReadiness(context.Context) error { return nil }
func (p *diagnosticOperatorProcess) Driver() string                           { return p.driver }

type diagnosticOperatorProtocol struct {
	calls  int
	invoke func(context.Context, string) (json.RawMessage, error)
}

func (*diagnosticOperatorProtocol) Call(context.Context, string, any, string) (json.RawMessage, error) {
	return nil, errors.New("arbitrary CDP is forbidden")
}
func (p *diagnosticOperatorProtocol) DiagnoseInstagramReader(ctx context.Context, nonce string) (json.RawMessage, error) {
	p.calls++
	return p.invoke(ctx, nonce)
}

func (p *diagnosticOperatorProtocol) DiagnoseInstagramBootstrapCorroboration(ctx context.Context, nonce string) (json.RawMessage, error) {
	p.calls++
	return p.invoke(ctx, nonce)
}

func TestInstagramBootstrapCorroborationActionPinsExistingOwner(t *testing.T) {
	process := &diagnosticOperatorProcess{done: make(chan error), driver: "browser"}
	owner, err := captureruntime.New(process)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Terminate()
	if err = owner.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	protocol := &diagnosticOperatorProtocol{invoke: func(_ context.Context, nonce string) (json.RawMessage, error) {
		if nonce != strings.Repeat("a", 48) || owner.Snapshot().ActiveLeases != 1 {
			t.Fatal("diagnostic owner not pinned")
		}
		return json.RawMessage(`{"safe":true}`), nil
	}}
	action := instagramBootstrapCorroborationDiagnosticAction(protocol, owner, 1234)
	if action == nil {
		t.Fatal("existing persistent owner unavailable")
	}
	if _, err = action(context.Background(), strings.Repeat("a", 48)); err != nil || protocol.calls != 1 || owner.Snapshot().ActiveLeases != 0 {
		t.Fatal("diagnostic lease leaked")
	}
	if instagramBootstrapCorroborationDiagnosticAction(nil, owner, 1234) != nil || instagramBootstrapCorroborationDiagnosticAction(protocol, owner, 9999) != nil {
		t.Fatal("foreign protocol/owner bound")
	}
	lease, err := owner.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = action(context.Background(), strings.Repeat("a", 48)); err == nil || protocol.calls != 1 {
		t.Fatal("active capture owner admitted")
	}
	lease.Release()
}

func TestInstagramDiagnosticActionUsesOnlyRunningPersistentOwner(t *testing.T) {
	process := &diagnosticOperatorProcess{done: make(chan error), driver: "browser"}
	owner, err := captureruntime.New(process)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Terminate()
	if err = owner.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	protocol := &diagnosticOperatorProtocol{invoke: func(_ context.Context, nonce string) (json.RawMessage, error) {
		if nonce != strings.Repeat("a", 48) || owner.Snapshot().ActiveLeases != 1 {
			t.Fatal("owner was not pinned")
		}
		return json.RawMessage(`{"safe":true}`), nil
	}}
	action := instagramReaderDiagnosticAction(protocol, owner, 1234)
	if action == nil {
		t.Fatal("running owner was not bound")
	}
	if _, err = action(context.Background(), strings.Repeat("a", 48)); err != nil || protocol.calls != 1 || owner.Snapshot().ActiveLeases != 0 {
		t.Fatalf("owner result: %v", err)
	}
	lease, err := owner.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = action(context.Background(), strings.Repeat("a", 48)); err == nil || protocol.calls != 1 {
		t.Fatal("active owner admitted")
	}
	lease.Release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = action(ctx, strings.Repeat("a", 48)); err == nil || protocol.calls != 1 {
		t.Fatal("canceled action dispatched")
	}
	if instagramReaderDiagnosticAction(protocol, owner, 9999) != nil {
		t.Fatal("foreign owner bound")
	}
}

func TestInstagramDiagnosticActionRejectsMissingOrHeadlessOwnerAndReleasesFailure(t *testing.T) {
	for _, driver := range []string{"browser", "headless"} {
		p := &diagnosticOperatorProcess{done: make(chan error), driver: driver}
		owner, err := captureruntime.New(p)
		if err != nil {
			t.Fatal(err)
		}
		protocol := &diagnosticOperatorProtocol{invoke: func(context.Context, string) (json.RawMessage, error) { return nil, errors.New("PRIVATE") }}
		if instagramReaderDiagnosticAction(protocol, owner, 1234) != nil {
			t.Fatal("unretained owner bound")
		}
		if driver == "headless" {
			if owner.RetainForProcess() == nil || instagramReaderDiagnosticAction(protocol, owner, 1234) != nil || protocol.calls != 0 {
				t.Fatal("headless owner became an eligible diagnostic owner")
			}
			owner.Terminate()
			continue
		}
		if err = owner.RetainForProcess(); err != nil {
			t.Fatal(err)
		}
		action := instagramReaderDiagnosticAction(protocol, owner, 1234)
		if action == nil {
			t.Fatal("composition failed")
		}
		_, err = action(context.Background(), strings.Repeat("a", 48))
		if err == nil || owner.Snapshot().ActiveLeases != 0 {
			t.Fatal("failed action leaked lease")
		}
		if driver == "headless" && protocol.calls != 0 {
			t.Fatal("headless action dispatched")
		}
		owner.Terminate()
	}
	if instagramReaderDiagnosticAction(nil, nil, 0) != nil {
		t.Fatal("absent owner bound")
	}
}
