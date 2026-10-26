//go:build auth_session_dev

package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestXQAEvidenceActualExecutionAndReaderIntervals(t *testing.T) {
	for _, mode := range []string{"queued", "overlap", "completed_before_post", "no_reader"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now()
			trace := newXQAAttemptEvidence(func() time.Time { return now })
			trace.mark("startup_accepted")
			finish := func() {}
			if mode != "queued" {
				finish = xQACaptureExecuting(withXQAEvidence(context.Background(), trace))
				now = now.Add(time.Millisecond)
			}
			if mode == "completed_before_post" {
				finish()
			}
			if mode != "no_reader" {
				trace.mark("reader_marker_ready")
				trace.mark("reader_foreground_verified")
			}
			finish()
			finish()
			want := "unknown"
			if mode == "overlap" {
				want = "observed"
			}
			if mode == "completed_before_post" {
				want = "not_observed"
			}
			got := trace.snapshot()
			if got.Overlap != want {
				t.Fatalf("overlap=%s want %s", got.Overlap, want)
			}
			entered, returned := 0, 0
			for _, event := range got.Stages {
				if event.Stage == "capture_entered" {
					entered++
				}
				if event.Stage == "capture_returned" {
					returned++
				}
			}
			if mode != "queued" && (entered != 1 || returned != 1) {
				t.Fatal("capture lifecycle was duplicated")
			}
			if mode == "queued" && (entered != 0 || returned != 0) {
				t.Fatal("scheduled work claimed capture execution")
			}
		})
	}
}
func TestXQAEvidenceFirstFailureAndIndependentCleanup(t *testing.T) {
	trace := newXQAAttemptEvidence(time.Now)
	first := xQAFailure("reader_verify", "rejected", errors.New("PRIVATE_cookie=secret"))
	trace.fail(first)
	trace.cleaned(false)
	trace.fail(xQAFailure("cleanup", "uncertain", nil))
	got := trace.snapshot()
	if got.FirstFailure == nil || *got.FirstFailure != first || got.Cleanup != "uncertain" {
		t.Fatal("cleanup overwrote first cause")
	}
	select {
	case <-trace.failed:
	default:
		t.Fatal("failure notification missing")
	}
	got.FirstFailure.Stage = "mutated"
	if trace.snapshot().FirstFailure.Stage != "reader_verify" {
		t.Fatal("snapshot exposed mutable first failure")
	}
	if value := XApplicationQAFailureOf(errors.New("PRIVATE_cookie=secret")); value.Detail != "" || strings.Contains(value.Reason, "PRIVATE") {
		t.Fatal("unbranded error data escaped")
	}
}
func TestXQAEvidenceCapacityDoesNotInventOverlap(t *testing.T) {
	trace := newXQAAttemptEvidence(time.Now)
	for i := 0; i < 200; i++ {
		trace.mark("reader_requested")
	}
	trace.mark("arbitrary PRIVATE input")
	got := trace.snapshot()
	if len(got.Stages) != 96 || !got.Dropped || got.Overlap != "unknown" {
		t.Fatal("bounded diagnostic capacity violated")
	}
}
