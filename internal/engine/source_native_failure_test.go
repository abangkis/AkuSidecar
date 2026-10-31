//go:build auth_session_dev

package engine

import (
	"errors"
	"fmt"
	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"strings"
	"testing"
)

func TestSourceNativeBootstrapFailureWireCategories(t *testing.T) {
	for _, role := range []string{"reader", "collector", "unknown"} {
		for _, reason := range []string{"body_read", "body_bounds", "stale_delivery", "document_changed", "bootstrap_rejected", "cookie_mismatch", "cleanup_failed", "wire_unmatched", "delivery_unproven", "navigation_failed"} {
			code := "native_source_evidence_" + role + "_bootstrap_" + reason
			if got := sourceNativeWireFailure(&appshell.MainBrowserError{Code: code}).Error(); got != code {
				t.Fatalf("lost bounded bootstrap category: %s", got)
			}
		}
		for _, reason := range []string{"corroborated", "private_cookie_value", "http_400"} {
			code := "native_source_evidence_" + role + "_bootstrap_" + reason
			if got := sourceNativeWireFailure(&appshell.MainBrowserError{Code: code}).Error(); got != "native_source_wire_unavailable" {
				t.Fatal("unsupported bootstrap category admitted")
			}
		}
	}
}

func TestSourceNativeFailureRetainsSafeFirstCause(t *testing.T) {
	for _, test := range []struct {
		cause error
		want  string
	}{
		{sourceNativeFail(errors.New("private-cookie-and-profile"), "evidence", "pair"), "native_source_evidence_pair"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_session_required"}), "native_source_session_required"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_evidence_reader_identity_http_forbidden"}), "native_source_evidence_reader_identity_http_forbidden"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_evidence_collector_identity_body_json"}), "native_source_evidence_collector_identity_body_json"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_evidence_reader_identity_private-cookie-and-profile"}), "native_source_wire_unavailable"},
		{sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_secret_private-cookie-and-profile"}), "native_source_wire_unavailable"},
	} {
		e, _ := testEngine(t)
		e.config.Capture.InstagramMode = config.XCaptureConcurrent
		d := e.dailyController(domain.SourceInstagram)
		d.rememberFailure(test.cause, "capture")
		d.rememberFailure(sourceNativeFail(nil, "cleanup", "unverified"), "cleanup")
		if got := d.status().FailureCode; got != test.want {
			t.Fatalf("diagnostic=%s want %s", got, test.want)
		}
		if strings.Contains(fmt.Sprintf("%v %#v", test.cause, test.cause), "private-cookie") {
			t.Fatal("private cause escaped diagnostic")
		}
	}
	cause := errors.New("private-cookie-and-profile")
	err := sourceNativeFail(cause, "ownership", "changed")
	if !errors.Is(err, cause) {
		t.Fatal("private causal chain lost")
	}
}

func TestSourceNativeHTTPStatusContract(t *testing.T) {
	// Exhaust the same 100..599 contract as the browser projection, including
	// unassigned statuses. HTTP 200 is never a rejection; auth aliases remain.
	for _, role := range []string{"reader", "collector", "unknown"} {
		for status := 100; status <= 599; status++ {
			if status == 200 {
				continue
			}
			reason := fmt.Sprintf("http_status_%d", status)
			switch status {
			case 401:
				reason = "http_unauthorized"
			case 403:
				reason = "http_forbidden"
			case 429:
				reason = "http_rate_limited"
			}
			code := "native_source_evidence_" + role + "_identity_" + reason
			err := sourceNativeWireFailure(&appshell.MainBrowserError{Code: code})
			var failure *sourceNativeFailure
			if !errors.As(err, &failure) || failure.dailyFailureCode() != code || failure.dailyRetryClass() != "" {
				t.Fatalf("HTTP rejection changed or became retryable: %s: %v", code, err)
			}
		}
		for _, suffix := range []string{"099", "600", "200", "401", "403", "429", "0400", "400.0", "400_PRIVATE", "400\n", "-1"} {
			err := sourceNativeWireFailure(&appshell.MainBrowserError{Code: "native_source_evidence_" + role + "_identity_http_status_" + suffix})
			if err.Error() != "native_source_wire_unavailable" {
				t.Fatalf("unsupported status admitted: %q", suffix)
			}
		}
	}
}
