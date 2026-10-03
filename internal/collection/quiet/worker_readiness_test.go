package quiet

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestCaptureAvailabilityClearsOnFailureAndRetirement(t *testing.T) {
	var zeroValue Worker
	if zeroValue.CaptureAvailable() {
		t.Fatal("zero-value Quiet worker must be unavailable")
	}
	failed := NewWorker(&Targets{closed: true}, headless.Options{})
	if !failed.CaptureAvailable() {
		t.Fatal("new Quiet worker should be available before its first capture")
	}
	cause := errors.New("worker failed")
	if err := failed.fail(cause); !errors.Is(err, cause) {
		t.Fatalf("failure result=%v", err)
	}
	if failed.CaptureAvailable() {
		t.Fatal("failed worker remained available")
	}

	retired := NewWorker(&Targets{unknownBrowserID: true}, headless.Options{})
	if !retired.CaptureAvailable() {
		t.Fatal("new Quiet worker should be available before retirement")
	}
	retired.op <- struct{}{}
	retireDone := make(chan error, 1)
	go func() { retireDone <- retired.Retire(context.Background()) }()
	deadline := time.Now().Add(time.Second)
	for retired.CaptureAvailable() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if retired.CaptureAvailable() {
		<-retired.op
		t.Fatal("readiness did not clear before retirement waited for the worker lock")
	}
	select {
	case err := <-retireDone:
		<-retired.op
		t.Fatalf("retirement bypassed an active operation: %v", err)
	default:
	}
	<-retired.op
	err := <-retireDone
	if err == nil || !strings.Contains(err.Error(), "cleanup is unverified") {
		t.Fatalf("failed cleanup result=%v", err)
	}
	if retired.CaptureAvailable() {
		t.Fatal("worker with failed retirement cleanup remained available")
	}
	if _, err := retired.Capture(context.Background(), domain.SourceX, nil); err == nil || !strings.Contains(err.Error(), "Quiet collector is unavailable") {
		t.Fatalf("retired worker did not fail pinned capture explicitly: %v", err)
	}
}
