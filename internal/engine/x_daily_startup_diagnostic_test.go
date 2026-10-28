package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestXDailyStartupDiagnosticPreservesCauseWithoutExposingIt(t *testing.T) {
	secret := errors.New("auth_token=private-value C:\\private\\profile account=123456789")
	first := xDailyStartupFail(secret, "seed_rpc", "rpc")
	wrapped := xDailyStartupFail(fmt.Errorf("cleanup: %w", first), "runtime", "runtime")
	if wrapped != first || !errors.Is(wrapped, secret) {
		t.Fatal("original operational cause was replaced")
	}
	for _, text := range []string{fmt.Sprint(wrapped), fmt.Sprintf("%+v", wrapped), fmt.Sprintf("%#v", wrapped)} {
		if text != "daily_startup_seed_rpc_rpc_unavailable" {
			t.Fatalf("unsafe formatted diagnostic: %q", text)
		}
	}
	raw, err := json.Marshal(xDailyStartupDetail(wrapped))
	if err != nil || strings.Contains(string(raw), "private") || strings.Contains(string(raw), "123456789") {
		t.Fatal("private operational data escaped public projection")
	}
	copy := xDailyStartupDetail(wrapped)
	copy.Stage = "changed"
	if xDailyStartupDetail(wrapped).Stage != "seed_rpc" {
		t.Fatal("public projection mutated retained first cause")
	}
}

func TestXDailyStartupDiagnosticClosesUnknownFieldsAndContext(t *testing.T) {
	for _, tc := range []struct {
		cause error
		want  string
	}{
		{nil, "rejected"}, {context.Canceled, "canceled"},
		{fmt.Errorf("wrapped: %w", context.DeadlineExceeded), "deadline"},
	} {
		err := xDailyStartupFail(tc.cause, "private-path", "raw-account")
		d := xDailyStartupDetail(err)
		if d.Stage != "admission" || d.Condition != "unavailable" || d.Cause != tc.want || !errors.Is(err, tc.cause) && tc.cause != nil {
			t.Fatal("closed diagnostic lost category or private cause")
		}
	}
	d := (XDailyStartupDiagnostic{Stage: "seed_rpc", Condition: "rpc", Cause: "deadline", Origin: "secret", BrokerReason: "secret"}).closed()
	if d.Origin != "" || d.BrokerReason != "" || xDailyStartupDetail(errors.New("raw")) != nil {
		t.Fatal("untrusted diagnostic metadata accepted")
	}
}
