//go:build windows

package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const instagramDiagnosticFixture = `{"schema":"aku.instagram-reader-diagnostic.v1","primaryCause":"http_rejected","status":400,"mime":"json","bodyClass":"json_object","errorCategory":"unknown","documentBound":true,"wireMatched":true,"cleanup":"verified"}`

func TestInstagramDiagnosticNonceRejectsBeforeRPC(t *testing.T) {
	c, _, writes := syntheticMainBrowserClient(t)
	for _, nonce := range []string{"", strings.Repeat("a", 47), strings.Repeat("a", 49), strings.Repeat("A", 48), strings.Repeat("g", 48), strings.Repeat("0", 47) + "\n"} {
		raw, err := c.DiagnoseInstagramReader(context.Background(), nonce)
		if err == nil || raw != nil {
			t.Fatal("invalid nonce accepted")
		}
	}
	select {
	case <-writes:
		t.Fatal("invalid nonce wrote RPC")
	default:
	}
}

func TestInstagramDiagnosticClientClosedProjection(t *testing.T) {
	cases := map[string]string{"valid": instagramDiagnosticFixture}
	for _, category := range []string{"none", "login_required", "challenge_required", "checkpoint_required", "feedback_required", "sentry_block", "csrf_failed", "unknown"} {
		cases["category_"+category] = strings.Replace(instagramDiagnosticFixture, `"unknown"`, fmt.Sprintf("%q", category), 1)
	}
	for _, cause := range []string{"none", "invalid_request", "scope_unavailable", "setup_failed", "navigation_failed", "context_unavailable", "document_changed", "evaluation_failed", "projection_invalid", "fetch_timeout", "fetch_failed", "http_rejected", "unexpected_success", "response_scope", "body_oversize", "body_read_failed", "body_invalid", "wire_unmatched", "cleanup_failed", "page_scope", "login_visible"} {
		cases["cause_"+cause] = strings.Replace(instagramDiagnosticFixture, `"http_rejected"`, fmt.Sprintf("%q", cause), 1)
	}
	cases["reject_private"] = strings.TrimSuffix(instagramDiagnosticFixture, "}") + `,"headers":"PRIVATE_MARKER"}`
	cases["reject_duplicate"] = strings.TrimSuffix(instagramDiagnosticFixture, "}") + `,"status":400}`
	cases["reject_oversize"] = strings.TrimSuffix(instagramDiagnosticFixture, "}") + `,"body":"` + strings.Repeat("PRIVATE_MARKER", 100) + `"}`
	cases["reject_status"] = strings.Replace(instagramDiagnosticFixture, "400", "99", 1)
	cases["reject_fraction"] = strings.Replace(instagramDiagnosticFixture, "400", "400.0", 1)
	cases["reject_null"] = strings.Replace(instagramDiagnosticFixture, "true", "null", 1)
	cases["reject_type"] = strings.Replace(instagramDiagnosticFixture, "true", `"true"`, 1)
	cases["reject_missing"] = strings.Replace(instagramDiagnosticFixture, `"documentBound":true,`, "", 1)
	for _, key := range []string{"schema", "primaryCause", "mime", "bodyClass", "errorCategory", "cleanup"} {
		var fields map[string]any
		if err := json.Unmarshal([]byte(instagramDiagnosticFixture), &fields); err != nil {
			t.Fatal(err)
		}
		fields[key] = "PRIVATE_MARKER"
		encoded, _ := json.Marshal(fields)
		cases["reject_"+key] = string(encoded)
	}
	for name, reply := range cases {
		t.Run(name, func(t *testing.T) {
			c, writer, writes := syntheticMainBrowserClient(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			type outcome struct {
				raw json.RawMessage
				err error
			}
			done := make(chan outcome, 1)
			nonce := strings.Repeat("a1", 24)
			go func() { raw, err := c.DiagnoseInstagramReader(ctx, nonce); done <- outcome{raw, err} }()
			var request map[string]any
			select {
			case raw := <-writes:
				if err := json.Unmarshal(raw, &request); err != nil {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if len(request) != 3 || request["op"] != "main_instagram_reader_diagnostic" || request["nonce"] != nonce || request["id"] == nil {
				t.Fatal("unsealed request")
			}
			if _, err := fmt.Fprintf(writer, "{\"id\":%v,\"ok\":true,\"result\":%s}\n", request["id"], reply); err != nil {
				t.Fatal(err)
			}
			result := <-done
			if strings.HasPrefix(name, "reject_") {
				var failure *MainBrowserError
				if result.raw != nil || !errors.As(result.err, &failure) || len(failure.Diagnostic) != 0 || failure.Code != "instagram_diagnostic_projection_invalid" {
					t.Fatal("unsafe reply was not rejected privately")
				}
			} else if result.err != nil || string(result.raw) != reply {
				t.Fatalf("projection changed: %v", result.err)
			}
		})
	}
}

func TestInstagramDiagnosticMalformedProjection(t *testing.T) {
	for _, raw := range []string{instagramDiagnosticFixture + `{}`, "{", "[]", "null", strings.Replace(instagramDiagnosticFixture, "unknown", string([]byte{0xff}), 1)} {
		if result, err := validateInstagramDiagnosticProjection([]byte(raw)); result != nil || err == nil {
			t.Fatal("malformed projection accepted")
		}
	}
}

func TestInstagramDiagnosticProjectionBoundsAndEnums(t *testing.T) {
	for key, values := range map[string][]any{
		"mime":          {"missing", "json", "html", "other"},
		"bodyClass":     {"not_read", "json_object", "json_nonobject", "invalid_json", "invalid_utf8", "oversize", "read_failed"},
		"cleanup":       {"not_started", "verified", "uncertain"},
		"status":        {0, 100, 599},
		"documentBound": {false, true},
		"wireMatched":   {false, true},
	} {
		for _, value := range values {
			var fields map[string]any
			if err := json.Unmarshal([]byte(instagramDiagnosticFixture), &fields); err != nil {
				t.Fatal(err)
			}
			fields[key] = value
			raw, _ := json.Marshal(fields)
			if _, err := validateInstagramDiagnosticProjection(raw); err != nil {
				t.Fatalf("valid %s rejected: %v", key, err)
			}
		}
	}
	for _, size := range []int{1024, 1025} {
		raw := instagramDiagnosticFixture + strings.Repeat(" ", size-len(instagramDiagnosticFixture))
		result, err := validateInstagramDiagnosticProjection([]byte(raw))
		if size == 1024 && (err != nil || string(result) != instagramDiagnosticFixture) {
			t.Fatal("bounded projection rejected")
		}
		if size == 1025 && (err == nil || result != nil) {
			t.Fatal("over-limit projection accepted")
		}
	}
}

func TestInstagramDiagnosticTransportErrorIsPrivate(t *testing.T) {
	c, writer, writes := syntheticMainBrowserClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := c.DiagnoseInstagramReader(ctx, strings.Repeat("0", 48)); done <- err }()
	var request map[string]any
	select {
	case raw := <-writes:
		if err := json.Unmarshal(raw, &request); err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if _, err := fmt.Fprintf(writer, "{\"id\":%v,\"ok\":false,\"error\":{\"code\":\"PRIVATE_MARKER\",\"diagnostic\":{\"body\":\"PRIVATE_MARKER\"}}}\n", request["id"]); err != nil {
		t.Fatal(err)
	}
	var failure *MainBrowserError
	if err := <-done; !errors.As(err, &failure) || failure.Code != "instagram_diagnostic_rejected" || len(failure.Diagnostic) != 0 {
		t.Fatal("transport private data escaped")
	}
}
