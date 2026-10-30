package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"unicode/utf8"
)

// Optional sealed diagnostic; it cannot issue account proof or session consent.
type MainBrowserInstagramBootstrapCorroborationDiagnosticProtocol interface {
	DiagnoseInstagramBootstrapCorroboration(context.Context, string) (json.RawMessage, error)
}

func ValidateInstagramBootstrapCorroborationDiagnostic(raw json.RawMessage) (json.RawMessage, error) {
	reject := func() (json.RawMessage, error) {
		return nil, &MainBrowserError{Code: "instagram_bootstrap_corroboration_projection_invalid"}
	}
	if len(raw) > 1024 || !utf8.Valid(raw) {
		return reject()
	}
	fields := map[string]bool{"schema": true, "reason": true, "bodyBound": true, "documentBound": true,
		"serverFresh": true, "idMatched": true, "cookieMatched": true, "scanComplete": true, "qualified": true, "cleanup": true}
	values := map[string]any{}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return reject()
	}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || !fields[key] {
			return reject()
		}
		delete(fields, key)
		var value any
		if d.Decode(&value) != nil {
			return reject()
		}
		if key == "schema" || key == "reason" || key == "cleanup" {
			if _, ok := value.(string); !ok {
				return reject()
			}
		} else if _, ok := value.(bool); !ok {
			return reject()
		}
		values[key] = value
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || len(fields) != 0 {
		return reject()
	}
	if _, err := d.Token(); err != io.EOF {
		return reject()
	}
	if values["schema"] != "aku.instagram-bootstrap-corroboration.v1" || values["qualified"] != false {
		return reject()
	}
	reasons := map[any]bool{}
	for _, reason := range []string{"corroborated", "invalid_request", "scope_unavailable", "setup_failed", "navigation_failed", "wire_unmatched", "http_rejected", "delivery_unproven", "body_bounds", "body_read", "body_encoding", "body_deadline", "body_invalid", "context_unavailable", "document_changed", "stale_delivery", "evaluation_failed", "projection_invalid", "page_scope", "login_visible", "challenge", "bootstrap_rejected", "id_mismatch", "cookie_unsupported", "cookie_missing", "cookie_ambiguous", "cookie_invalid", "cookie_mismatch", "cookie_failed", "cleanup_failed"} {
		reasons[reason] = true
	}
	if !reasons[values["reason"]] || values["cleanup"] != "not_started" && values["cleanup"] != "verified" && values["cleanup"] != "uncertain" {
		return reject()
	}
	if values["reason"] == "corroborated" {
		for _, key := range []string{"bodyBound", "documentBound", "serverFresh", "idMatched", "cookieMatched", "scanComplete"} {
			if values[key] != true {
				return reject()
			}
		}
		if values["cleanup"] != "verified" {
			return reject()
		}
	}
	result, err := json.Marshal(values)
	if err != nil {
		return reject()
	}
	return result, nil
}
