package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/igdiagnostic"
)

func TestInstagramDiagnosticOwnerTraceRejectsOnlyItsFailedPredicate(t *testing.T) {
	p := &diagnosticOperatorProcess{done: make(chan error), driver: "browser"}
	m, err := captureruntime.New(p)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Terminate()
	if err = m.RetainForProcess(); err != nil {
		t.Fatal(err)
	}
	action := bindInstagramDiagnosticAction(func(context.Context, string) (json.RawMessage, error) {
		t.Fatal("busy owner dispatched")
		return nil, nil
	}, m, 1234)
	lease, err := m.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	ctx, a := igdiagnostic.Ensure(context.Background(), "reader")
	_, err = action(ctx, strings.Repeat("a", 48))
	if err == nil {
		t.Fatal("active lease accepted")
	}
	var s igdiagnostic.Envelope
	if json.Unmarshal(a.SnapshotJSON(), &s) != nil || s.Cause == nil || s.Cause.Stage != "owner" || s.Cause.Condition != "lease_present" || s.Disposition != "not_started" {
		t.Fatal("owner predicate lost")
	}
	if m.Snapshot().ActiveLeases != 1 {
		t.Fatal("diagnostic released someone else's lease")
	}
}
