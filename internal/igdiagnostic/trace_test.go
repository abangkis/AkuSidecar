package igdiagnostic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestTraceContract(t *testing.T) {
	ctx, a := Ensure(context.Background(), "corroboration")
	Note(ctx, "admission")
	Fail(ctx, "owner", "lease_present")
	Fail(ctx, "rpc", "deadline")
	SetCleanup(ctx, "uncertain", "cleanup_failed")
	SetDelivery(ctx, "unavailable", "response_lost")
	raw := Projection(ctx)
	if len(raw) > 2048 {
		t.Fatal("unbounded")
	}
	if _, ok := ValidateJSON(raw); !ok {
		t.Fatalf("invalid own projection: %s", raw)
	}
	var e Envelope
	_ = json.Unmarshal(raw, &e)
	if e.Cause.Condition != "lease_present" || e.Cleanup.State != "uncertain" || e.Evidence["idMatched"] != "not_observed" {
		t.Fatal("observations conflated")
	}
	sentinel := errors.New("PRIVATE_MARKER")
	wrapped := Wrap(ctx, sentinel)
	if !errors.Is(wrapped, sentinel) || strings.Contains(fmt.Sprintf("%v %#v", wrapped, wrapped), "PRIVATE_MARKER") {
		t.Fatal("cause or privacy lost")
	}
	for _, bad := range []string{strings.Replace(string(raw), `"complete":true`, `"complete":null`, 1), strings.Replace(string(raw), `"schema":`, `"Schema":`, 1), strings.Replace(string(raw), `"cause":`, `"cause":null,"cause":`, 1), strings.Replace(string(raw), `"revision":"ig1",`, "", 1), strings.Replace(string(raw), `"none"`, `"https://PRIVATE_MARKER"`, 1)} {
		if _, ok := ValidateJSON([]byte(bad)); ok {
			t.Fatal("malformed accepted")
		}
	}
	a.Evidence("idMatched", "passed")
	a.Evidence("documentBound", "invalidated")
	if _, ok := ValidateJSON(a.SnapshotJSON()); !ok {
		t.Fatal("evidence state rejected")
	}
}
func TestTracePeerAndKnownness(t *testing.T) {
	_, a := Ensure(nil, "reader")
	if _, ok := ValidateJSON(a.SnapshotJSON()); !ok {
		t.Fatal("initial snapshot invalid")
	}
	w := Wire{Schema: Schema, Stage: "body_read", Completed: "root_delivery", Disposition: "started", Sequence: 1, Cleanup: Settlement{"pending", "none"}, Complete: true}
	raw, _ := json.Marshal(w)
	if !a.Observe(raw) {
		t.Fatal("valid progress rejected")
	}
	if a.Observe(raw) {
		t.Fatal("duplicate accepted")
	}
	w.Sequence++
	ms := int64(0)
	w.ElapsedMS = &ms
	w.Cause = &Cause{"body_read", "body_read"}
	raw, _ = json.Marshal(w)
	if !a.Observe(raw) {
		t.Fatal("known zero rejected")
	}
	w.Sequence++
	w.Disposition = "completed"
	w.Cleanup = Settlement{"verified", "none"}
	raw, _ = json.Marshal(w)
	if !a.Observe(raw) {
		t.Fatal("late settlement rejected")
	}
	a.Observe([]byte(`{"PRIVATE_MARKER":true}`))
	var e Envelope
	_ = json.Unmarshal(a.SnapshotJSON(), &e)
	if e.Complete || e.NodeElapsedMS == nil || *e.NodeElapsedMS != 0 || e.Cause.Condition != "body_read" || e.Cleanup.State != "verified" {
		t.Fatal("reconciliation destroyed cause")
	}
	if _, other := Ensure(WithAttempt(context.Background(), a), "bootstrap"); other != nil {
		t.Fatal("operation rebinding")
	}
}

func TestNodeElapsedTracksLatestAcceptedObservation(t *testing.T) {
	_, a := Ensure(nil, "reader")
	ms := int64(100)
	w := Wire{Schema: Schema, Stage: "body_read", Completed: "root_delivery", Disposition: "started", Sequence: 1, Cleanup: Settlement{"pending", "none"}, Complete: true, ElapsedMS: &ms}
	observe := func() bool {
		raw, err := json.Marshal(w)
		if err != nil {
			t.Fatal(err)
		}
		return a.Observe(raw)
	}
	if !observe() {
		t.Fatal("known elapsed rejected")
	}
	w.Sequence++
	w.ElapsedMS = nil
	w.Complete = false
	if !observe() {
		t.Fatal("unknown elapsed rejected")
	}
	var e Envelope
	if err := json.Unmarshal(a.SnapshotJSON(), &e); err != nil || e.NodeSequence != 2 || e.NodeElapsedMS != nil || e.Complete {
		t.Fatalf("unknown elapsed was not preserved: %+v, %v", e, err)
	}
	// An old or duplicate frame must not restore an earlier elapsed value.
	w.Sequence = 1
	w.ElapsedMS = &ms
	if observe() {
		t.Fatal("stale elapsed accepted")
	}
	if err := json.Unmarshal(a.SnapshotJSON(), &e); err != nil || e.NodeElapsedMS != nil {
		t.Fatal("stale elapsed changed the latest observation")
	}
	w.Sequence = 3
	if !observe() {
		t.Fatal("later known elapsed rejected")
	}
	if err := json.Unmarshal(a.SnapshotJSON(), &e); err != nil || e.NodeElapsedMS == nil || *e.NodeElapsedMS != ms || e.Complete {
		t.Fatal("later known elapsed was not preserved or incompleteness was lost")
	}
}

func TestCatalogParity(t *testing.T) {
	raw, err := os.ReadFile("../collection/headless/worker/native-pilot/instagram-diagnostic-catalog.mjs")
	if err != nil {
		t.Fatal(err)
	}
	start := strings.Index(string(raw), "Object.freeze(") + len("Object.freeze(")
	end := strings.LastIndex(string(raw), ");")
	var peer map[string][]string
	if json.Unmarshal(raw[start:end], &peer) != nil {
		t.Fatal("invalid Node catalog")
	}
	a, _ := json.Marshal(catalog)
	b, _ := json.Marshal(peer)
	if string(a) != string(b) {
		t.Fatal("Go/Node catalog drift")
	}
}

func TestReadFailureConditionsPreserveFirstCauseAcrossProgressAndSnapshot(t *testing.T) {
	for _, condition := range []string{"read_command_failed", "read_command_timeout", "read_transport_closed", "read_cdp_rejected", "read_reply_invalid", "read_empty_nonfinal"} {
		t.Run(condition, func(t *testing.T) {
			ctx, a := Ensure(context.Background(), "corroboration")
			w := Wire{Schema: Schema, Stage: "body_read", Completed: "root_delivery", Disposition: "started", Sequence: 1, Cause: &Cause{Stage: "body_read", Condition: condition}, Cleanup: Settlement{State: "pending", Condition: "none"}, Complete: true}
			raw, err := json.Marshal(w)
			if err != nil || len(raw) > 512 || !a.Observe(raw) {
				t.Fatal("read condition rejected at wire boundary")
			}
			Fail(ctx, "cleanup", "cleanup_failed")
			SetCleanup(ctx, "uncertain", "stream_close_failed")
			projection, ok := ValidateJSON(a.SnapshotJSON())
			if !ok || len(projection) > 2048 {
				t.Fatal("read condition rejected by snapshot validation")
			}
			var e Envelope
			if json.Unmarshal(projection, &e) != nil || e.Cause.Condition != condition || e.Cleanup.Condition != "stream_close_failed" {
				t.Fatal("first read failure or cleanup lost")
			}
		})
	}
}

func TestDocumentFailureConditionsSurviveWireAndCleanup(t *testing.T) {
	for _, condition := range []string{
		"document_frame_invalid", "document_frame_mismatch", "document_loader_mismatch", "document_url_mismatch", "document_context_mismatch",
		"document_context_cleared", "document_context_destroyed", "document_context_replaced", "document_frame_navigated", "document_frame_detached", "document_within_navigation",
		"document_frame_request_failed", "document_evaluation_request_failed", "document_evaluation_exception", "document_not_ready",
	} {
		t.Run(condition, func(t *testing.T) {
			ctx, a := Ensure(context.Background(), "verifier")
			w := Wire{Schema: Schema, Stage: "post_check", Completed: "cookie_comparison", Disposition: "started", Sequence: 1,
				Cause: &Cause{Stage: "post_check", Condition: condition}, Cleanup: Settlement{State: "verified", Condition: "none"}, Complete: true}
			raw, err := json.Marshal(w)
			if err != nil || len(raw) > 512 || !a.Observe(raw) {
				t.Fatal("document condition rejected at wire boundary")
			}
			Fail(ctx, "cleanup", "request_failed")
			SetCleanup(ctx, "uncertain", "dispose_failed")
			projection, ok := ValidateJSON(a.SnapshotJSON())
			if !ok || len(projection) > 2048 {
				t.Fatal("document condition rejected by snapshot validation")
			}
			var e Envelope
			if json.Unmarshal(projection, &e) != nil || e.Cause.Stage != "post_check" || e.Cause.Condition != condition || e.Cleanup.Condition != "dispose_failed" {
				t.Fatal("document cause or independent cleanup lost")
			}
		})
	}
}

func TestDocumentInvalidationObservationStagesSurviveWire(t *testing.T) {
	for _, stage := range []string{"parsing", "cleanup", "handoff", "cookie", "post_check"} {
		t.Run(stage, func(t *testing.T) {
			ctx, a := Ensure(context.Background(), "verifier")
			w := Wire{Schema: Schema, Stage: stage, Completed: "cookie_comparison", Disposition: "started", Sequence: 1,
				Cause: &Cause{Stage: stage, Condition: "document_within_navigation"}, Cleanup: Settlement{State: "verified", Condition: "none"}, Complete: true}
			raw, err := json.Marshal(w)
			if err != nil || len(raw) > 512 || !a.Observe(raw) {
				t.Fatal("document observation stage rejected at wire boundary")
			}
			Fail(ctx, "post_check", "request_failed")
			projection, ok := ValidateJSON(a.SnapshotJSON())
			var e Envelope
			if !ok || json.Unmarshal(projection, &e) != nil || e.Stage != stage || e.Cause.Stage != stage || e.Cause.Condition != "document_within_navigation" || e.Cleanup.State != "verified" {
				t.Fatal("observation stage or first cause lost in snapshot")
			}
		})
	}
}

func TestSaturatedTraceTerminalRetainsFirstCauseAndCleanup(t *testing.T) {
	_, a := Ensure(context.Background(), "verifier")
	w := Wire{Schema: Schema, Stage: "cleanup", Completed: "cookie_comparison", Disposition: "started", Sequence: 9998,
		Cause: &Cause{Stage: "root_delivery", Condition: "root_request_duplicate"}, Cleanup: Settlement{State: "pending", Condition: "none"}, Complete: true}
	raw, err := json.Marshal(w)
	if err != nil || !a.Observe(raw) {
		t.Fatal("last progress snapshot rejected")
	}
	w.Sequence = 9999
	w.Complete = false
	raw, err = json.Marshal(w)
	if err != nil || !a.Observe(raw) {
		t.Fatal("saturation progress marker rejected")
	}
	w.Sequence = 10000
	w.Cleanup = Settlement{State: "verified", Condition: "none"}
	raw, err = json.Marshal(w)
	if err != nil || !a.Observe(raw) || a.Observe(raw) {
		t.Fatal("bounded terminal sequence was rejected or replayed")
	}
	projection, ok := ValidateJSON(a.SnapshotJSON())
	var e Envelope
	if !ok || json.Unmarshal(projection, &e) != nil || e.Complete || e.NodeSequence != 10000 ||
		e.Cause == nil || e.Cause.Condition != "root_request_duplicate" || e.Cleanup.State != "verified" {
		t.Fatal("saturated terminal lost incompleteness, first cause, or cleanup")
	}
}

func TestLocalSnapshotSequenceSaturationRemainsValid(t *testing.T) {
	ctx, a := Ensure(context.Background(), "verifier")
	Fail(ctx, "root_delivery", "root_request_duplicate")
	for i := 0; i < 10005; i++ {
		Note(ctx, "cleanup")
	}
	SetCleanup(ctx, "verified", "none")
	projection, ok := ValidateJSON(a.SnapshotJSON())
	var e Envelope
	if !ok || json.Unmarshal(projection, &e) != nil || e.Sequence != 10000 || e.Complete ||
		e.Cause == nil || e.Cause.Condition != "root_request_duplicate" || e.Cleanup.State != "verified" {
		t.Fatal("local saturated snapshot lost boundedness, cause, or cleanup")
	}
}
