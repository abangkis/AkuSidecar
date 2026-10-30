package appshell

import (
	"encoding/json"
	"strings"
	"testing"
)

const corroborationReply = `{"schema":"aku.instagram-bootstrap-corroboration.v1","reason":"corroborated","bodyBound":true,"documentBound":true,"serverFresh":true,"idMatched":true,"cookieMatched":true,"scanComplete":true,"qualified":false,"cleanup":"verified"}`

func TestInstagramBootstrapCorroborationProjection(t *testing.T) {
	if _, err := ValidateInstagramBootstrapCorroborationDiagnostic([]byte(corroborationReply)); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"bodyBound", "documentBound", "serverFresh", "idMatched", "cookieMatched", "scanComplete"} {
		value := strings.Replace(corroborationReply, `"`+key+`":true`, `"`+key+`":false`, 1)
		if _, err := ValidateInstagramBootstrapCorroborationDiagnostic([]byte(value)); err == nil {
			t.Fatalf("incomplete corroboration accepted: %s", key)
		}
	}
	for _, value := range []string{
		strings.Replace(corroborationReply, `"qualified":false`, `"qualified":true`, 1),
		strings.Replace(corroborationReply, `"cleanup":"verified"`, `"cleanup":"uncertain"`, 1),
		strings.Replace(corroborationReply, `"reason":"corroborated"`, `"reason":"PRIVATE"`, 1),
		strings.Replace(corroborationReply, `"bodyBound":true`, `"bodyBound":1`, 1),
		strings.Replace(corroborationReply, `"documentBound":true`, `"documentBound":null`, 1),
		strings.Replace(corroborationReply, `"scanComplete":true,`, ``, 1),
		strings.Replace(corroborationReply, `corroboration.v1`, `corroboration.v0`, 1),
		strings.TrimSuffix(corroborationReply, `}`) + `,"accountId":"PRIVATE"}`,
		strings.TrimSuffix(corroborationReply, `}`) + `,"cookie":"PRIVATE"}`,
		strings.TrimSuffix(corroborationReply, `}`) + `,"body":"PRIVATE"}`,
		strings.TrimSuffix(corroborationReply, `}`) + `,"reason":"corroborated"}`,
		corroborationReply + ` {}`, `[]`, `null`, strings.Repeat(" ", 1025), string([]byte{0xff}),
	} {
		if _, err := ValidateInstagramBootstrapCorroborationDiagnostic([]byte(value)); err == nil {
			t.Fatal("malformed/private diagnostic accepted")
		}
	}
}

func TestInstagramBootstrapCorroborationFailureAndCleanupSeparate(t *testing.T) {
	for _, reason := range []string{"invalid_request", "scope_unavailable", "setup_failed", "navigation_failed", "wire_unmatched", "http_rejected", "delivery_unproven", "body_bounds", "body_read", "body_encoding", "body_deadline", "body_invalid", "context_unavailable", "document_changed", "stale_delivery", "evaluation_failed", "projection_invalid", "page_scope", "login_visible", "challenge", "bootstrap_rejected", "id_mismatch", "cookie_unsupported", "cookie_missing", "cookie_ambiguous", "cookie_invalid", "cookie_mismatch", "cookie_failed", "cleanup_failed"} {
		var value map[string]any
		if err := json.Unmarshal([]byte(corroborationReply), &value); err != nil {
			t.Fatal(err)
		}
		value["reason"] = reason
		value["cleanup"] = "uncertain"
		value["cookieMatched"] = false
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		projected, err := ValidateInstagramBootstrapCorroborationDiagnostic(raw)
		if err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if !strings.Contains(string(projected), `"reason":"`+reason+`"`) || !strings.Contains(string(projected), `"cleanup":"uncertain"`) {
			t.Fatal("cleanup replaced the original cause")
		}
	}
}
