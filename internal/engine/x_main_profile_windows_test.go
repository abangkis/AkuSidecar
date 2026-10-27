//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
)

type mainXTestProtocol struct {
	calls   []string
	failure error
}

func (p *mainXTestProtocol) Call(context.Context, string, any, string) (json.RawMessage, error) {
	return nil, errors.New("public lane forbidden")
}
func (p *mainXTestProtocol) CallX(ctx context.Context, op string, _ map[string]any) (json.RawMessage, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	p.calls = append(p.calls, op)
	return json.RawMessage(`{"closed":true}`), p.failure
}
func (*mainXTestProtocol) TransportWorker() *appshell.OwnedCommand { return nil }

func TestMainXBorrowedRPCPreservesSafeDiagnosticsAndCancellation(t *testing.T) {
	p := &mainXTestProtocol{failure: &appshell.MainBrowserError{Code: "native_pilot_capture_failed", Diagnostic: json.RawMessage(`{"phase":"capture"}`)}}
	r := &xMainBorrowedRPC{protocol: p}
	reply, err := r.Call(context.Background(), "manual_capture", nil, nil)
	if err != nil || reply.Code == "" || string(reply.Diagnostic) != `{"phase":"capture"}` {
		t.Fatal("safe diagnostic lost")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = r.Call(ctx, "manual_capture", nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel ignored")
	}
	r.Close()
	if _, err = r.Call(context.Background(), "daily_batch_seed", nil, nil); err == nil {
		t.Fatal("closed capability reused")
	}
	if len(p.calls) != 1 {
		t.Fatal("late operation reached transport")
	}
}

func TestMainXBorrowedLeaseDoesNotReleaseMainOwner(t *testing.T) {
	e, _ := testEngine(t)
	manager := attachTestCapture(t, e)
	root := t.TempDir()
	profile := filepath.Join(root, "profile")
	if err := os.Mkdir(profile, 0700); err != nil {
		t.Fatal(err)
	}
	rootID, _ := ownedprocess.DirectoryID(root)
	id, _ := ownedprocess.DirectoryID(profile)
	lease := &xMainBorrowedLease{descriptor: ownedprofile.Descriptor{Root: root, Directory: profile, RootID: rootID, DirectoryID: id}, manager: manager, generation: manager.Snapshot().Generation}
	if lease.Validate() != nil || lease.Close() != nil || !manager.Accepts(lease.generation) {
		t.Fatal("borrowed lease closed its owner")
	}
	if lease.Matches(root, filepath.Join(root, "other")) {
		t.Fatal("wrong profile accepted")
	}
	manager.Terminate()
	if lease.Validate() == nil {
		t.Fatal("dead main owner accepted")
	}
}

func TestMainXBorrowerCloseUsesScopedOperationOnly(t *testing.T) {
	p := &mainXTestProtocol{}
	done := make(chan struct{})
	h := &xApplicationOwner{mainProtocol: p, pipe: &xMainBorrowedRPC{protocol: p}, closed: done, stopping: make(chan struct{}), readerDone: make(chan struct{})}
	if err := h.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.calls) != 1 || p.calls[0] != "daily_owner_close" {
		t.Fatal("borrower did not close only its X scope")
	}
	if err := h.Close(context.Background()); err != nil || len(p.calls) != 1 {
		t.Fatal("borrower close replayed")
	}
}

func TestMainXFirstFailureKeepsTypedCauseWhenCleanupAlsoFails(t *testing.T) {
	primary := &xApplicationRPCError{code: "native_pilot_capture_failed", diagnostic: json.RawMessage(`{"stage":"capture"}`)}
	combined := errors.Join(primary, errors.New("synthetic cleanup uncertain"))
	if xDailyClosedFailureCode(combined) != primary.code {
		t.Fatal("cleanup replaced the closed first cause")
	}
	var carrier interface{ diagnosticJSON() []byte }
	if !errors.As(combined, &carrier) || string(carrier.diagnosticJSON()) != string(primary.diagnostic) {
		t.Fatal("typed diagnostic lost")
	}
}
