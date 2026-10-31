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
