//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/ownedprocess"
	"github.com/abangkis/AkuSidecar/internal/ownedprofile"
)

type mainXTestProtocol struct {
	calls          []string
	failure        error
	progress       []byte
	progressFrames [][]byte
	lastProgress   func([]byte)
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
func (p *mainXTestProtocol) CallXWithProgress(ctx context.Context, op string, fields map[string]any, progress func([]byte)) (json.RawMessage, error) {
	p.lastProgress = progress
	raw, err := p.CallX(ctx, op, fields)
	if err != nil || ctx.Err() != nil {
		return raw, err
	}
	if len(p.progress) > 0 {
		progress(p.progress)
	}
	for _, frame := range p.progressFrames {
		progress(frame)
	}
	return raw, nil
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

func TestMainXBorrowedRPCForwardsOnlyCorrelatedBoundedStartupProgress(t *testing.T) {
	valid := []byte(`{"operation":"settings_wait","elapsedMS":12,"cause":"manual_initial_reader_settings_source_unavailable_primary","settingsWait":"response_pending"}`)
	p := &mainXTestProtocol{progress: valid}
	r := &xMainBorrowedRPC{protocol: p}
	var received [][]byte
	if _, err := r.Call(context.Background(), "manual_acquire", nil, func(raw []byte) {
		received = append(received, append([]byte(nil), raw...))
	}); err != nil {
		t.Fatal(err)
	}
	if len(received) != 1 || string(received[0]) != string(valid) {
		t.Fatal("valid startup progress was not forwarded")
	}
	p.lastProgress(valid)
	if len(received) != 1 {
		t.Fatal("progress after the terminal reply was forwarded")
	}
	failedProtocol := &mainXTestProtocol{failure: &appshell.MainBrowserError{Code: "native_pilot_capture_failed"}}
	failedRPC := &xMainBorrowedRPC{protocol: failedProtocol}
	lateCount := 0
	reply, err := failedRPC.Call(context.Background(), "manual_acquire", nil, func([]byte) { lateCount++ })
	if err != nil || reply.Code != "native_pilot_capture_failed" {
		t.Fatal("terminal failure was not retained")
	}
	failedProtocol.lastProgress(valid)
	if lateCount != 0 || reply.Code != "native_pilot_capture_failed" {
		t.Fatal("late progress changed the retained terminal failure")
	}

	invalid := [][]byte{
		[]byte(`"synthetic-secret"`),
		[]byte(`{"operation":"settings_wait","elapsedMS":12,"cause":"","extra":"synthetic-secret"}`),
		[]byte(`{"operation":"settings_wait","operation":"cleanup","elapsedMS":12,"cause":""}`),
		[]byte(`{"operation":"settings_wait","elapsedMS":12,"cause":"synthetic-secret"}`),
		[]byte(`{"operation":"synthetic-secret","elapsedMS":12,"cause":""}`),
		[]byte(`{"operation":"settings_wait","elapsedMS":60001,"cause":""}`),
		[]byte(`{"operation":"settings_wait","elapsedMS":12}`),
		[]byte(`{"operation":"settings_wait","cause":""}`),
		[]byte(`{"operation":"settings_wait","elapsedMS":12,"cause":null}`),
		[]byte(`{"operation":"settings_wait","elapsedMS":12,"cause":"","settingsWait":null}`),
		make([]byte, 513),
	}
	for _, raw := range invalid {
		p := &mainXTestProtocol{progress: raw}
		r := &xMainBorrowedRPC{protocol: p}
		count := 0
		if _, err := r.Call(context.Background(), "manual_acquire", nil, func([]byte) { count++ }); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("malformed startup progress reached its callback")
		}
	}
}

func TestMainXBorrowedRPCAdmitsMeasuredCallsAndClosedProgressCatalog(t *testing.T) {
	cases := map[string][]string{
		"manual_acquire":   {"acquire_rpc", "identity", "rendezvous", "settings_wait", "cleanup", "root_restore", "complete"},
		"manual_inspect":   {"acquire_rpc", "inspect_rpc", "complete"},
		"manual_inventory": {"acquire_rpc", "inventory_rpc", "complete"},
	}
	for operation, stages := range cases {
		p := &mainXTestProtocol{}
		for i, stage := range stages {
			p.progressFrames = append(p.progressFrames, []byte(fmt.Sprintf(`{"operation":%q,"elapsedMS":%d,"cause":""}`, stage, i)))
		}
		r := &xMainBorrowedRPC{protocol: p}
		count := 0
		if _, err := r.Call(context.Background(), operation, nil, func([]byte) { count++ }); err != nil {
			t.Fatal(err)
		}
		if count != len(stages) {
			t.Fatalf("%s admitted %d of %d valid progress stages", operation, count, len(stages))
		}
	}
	p := &mainXTestProtocol{progress: []byte(`{"operation":"settings_wait","elapsedMS":12,"cause":""}`)}
	r := &xMainBorrowedRPC{protocol: p}
	count := 0
	if _, err := r.Call(context.Background(), "manual_capture", nil, func([]byte) { count++ }); err != nil {
		t.Fatal(err)
	}
	if count != 0 || p.lastProgress != nil {
		t.Fatal("unmeasured owner RPC admitted progress")
	}
}

func TestMainXBorrowedRPCCapsProgressPerPendingCall(t *testing.T) {
	valid := []byte(`{"operation":"settings_wait","elapsedMS":12,"cause":""}`)
	p := &mainXTestProtocol{}
	for i := 0; i < 40; i++ {
		p.progressFrames = append(p.progressFrames, valid)
	}
	r := &xMainBorrowedRPC{protocol: p}
	count := 0
	if _, err := r.Call(context.Background(), "manual_acquire", nil, func([]byte) { count++ }); err != nil {
		t.Fatal(err)
	}
	if count != 32 {
		t.Fatalf("progress cap mismatch: got %d events", count)
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

func TestXDailyMainPermissionBindsSelectedSubprofile(t *testing.T) {
	root := t.TempDir()
	userData := filepath.Join(root, "user-data")
	selected := filepath.Join(userData, "Default")
	for _, path := range []string{selected, filepath.Join(userData, "Profile 1")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	rootID, err := ownedprocess.DirectoryID(root)
	if err != nil {
		t.Fatal(err)
	}
	userDataID, err := ownedprocess.DirectoryID(userData)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := ownedprofile.Descriptor{Root: root, Directory: userData, RootID: rootID, DirectoryID: userDataID}
	original, err := xMainDailyProfileScope(descriptor, "Default")
	if err != nil {
		t.Fatal(err)
	}
	same, err := xMainDailyProfileScope(descriptor, "Default")
	if err != nil || same != original {
		t.Fatalf("unchanged selected subprofile lost its scope: %v", err)
	}
	other, err := xMainDailyProfileScope(descriptor, "Profile 1")
	if err != nil {
		t.Fatal(err)
	}
	// Retain the original directory so Windows cannot reuse its identity.
	if err := os.Rename(selected, filepath.Join(userData, "retired-default")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(selected, 0700); err != nil {
		t.Fatal(err)
	}
	replaced, err := xMainDailyProfileScope(descriptor, "Default")
	if err != nil {
		t.Fatal(err)
	}
	changedRoot := descriptor
	changedRoot.RootID += "-replacement"
	rootReplaced, err := xMainDailyProfileScope(changedRoot, "Default")
	if err != nil {
		t.Fatal(err)
	}
	changedUserData := descriptor
	changedUserData.DirectoryID += "-replacement"
	userDataReplaced, err := xMainDailyProfileScope(changedUserData, "Default")
	if err != nil {
		t.Fatal(err)
	}
	if rootReplaced == replaced || userDataReplaced == replaced {
		t.Fatal("selected subprofile scope lost an enclosing directory identity guard")
	}
	for _, tc := range []struct {
		name    string
		scope   string
		restore bool
	}{
		{"unchanged selected profile", same, true},
		{"different selected profile", other, false},
		{"replaced selected directory", replaced, false},
		{"replaced enclosing root", rootReplaced, false},
		{"replaced user-data directory", userDataReplaced, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, state := testEngine(t)
			ctx := context.Background()
			if err := state.SaveXDailyPermission(ctx, original, true); err != nil {
				t.Fatal(err)
			}
			owner := &syntheticDailyOwner{}
			attachDailyPermissionRuntime(t, e, state, tc.scope, owner)
			if err := e.ResumeXDailyScheduling(ctx); err != nil {
				t.Fatal(err)
			}
			owner.mu.Lock()
			checks := owner.readerChecks
			owner.mu.Unlock()
			status := e.XCaptureStatus()
			if status.RememberedPermission != tc.restore || e.xDailyReady() != tc.restore || (checks > 0) != tc.restore {
				t.Fatalf("wrong profile restoration: status=%+v readerChecks=%d", status, checks)
			}
		})
	}
	for _, name := range []string{"../Default", "Missing"} {
		if _, err := xMainDailyProfileScope(descriptor, name); err == nil {
			t.Fatalf("invalid or missing subprofile %q received a permission scope", name)
		}
	}
}
