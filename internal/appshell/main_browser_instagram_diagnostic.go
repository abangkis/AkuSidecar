package appshell

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

func validInstagramDiagnosticNonce(nonce string) bool {
	if len(nonce) != 48 {
		return false
	}
	for _, c := range nonce {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

// Decode each field exactly once, then re-encode only the fixed typed projection.
// No remote error text, headers, account identifiers, or response body can pass.
func validateInstagramDiagnosticProjection(raw json.RawMessage) (json.RawMessage, error) {
	rejected := func() (json.RawMessage, error) {
		return nil, &MainBrowserError{Code: "instagram_diagnostic_projection_invalid"}
	}
	if len(raw) > 1024 || !utf8.Valid(raw) {
		return rejected()
	}
	var p struct {
		Schema        string `json:"schema"`
		PrimaryCause  string `json:"primaryCause"`
		Status        int    `json:"status"`
		MIME          string `json:"mime"`
		BodyClass     string `json:"bodyClass"`
		ErrorCategory string `json:"errorCategory"`
		DocumentBound bool   `json:"documentBound"`
		WireMatched   bool   `json:"wireMatched"`
		Cleanup       string `json:"cleanup"`
	}
	fields := map[string]any{
		"schema": &p.Schema, "primaryCause": &p.PrimaryCause, "status": &p.Status,
		"mime": &p.MIME, "bodyClass": &p.BodyClass, "errorCategory": &p.ErrorCategory,
		"documentBound": &p.DocumentBound, "wireMatched": &p.WireMatched, "cleanup": &p.Cleanup,
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	if token, err := d.Token(); err != nil || token != json.Delim('{') {
		return rejected()
	}
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok {
			return rejected()
		}
		dest, exists := fields[key]
		if !exists {
			return rejected()
		}
		delete(fields, key)
		var value json.RawMessage
		if d.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) || json.Unmarshal(value, dest) != nil {
			return rejected()
		}
	}
	if token, err := d.Token(); err != nil || token != json.Delim('}') || len(fields) != 0 {
		return rejected()
	}
	if _, err := d.Token(); err != io.EOF {
		return rejected()
	}
	if p.Schema != "aku.instagram-reader-diagnostic.v1" || p.Status != 0 && (p.Status < 100 || p.Status > 599) {
		return rejected()
	}
	allowed := func(value string, choices ...string) bool {
		for _, choice := range choices {
			if value == choice {
				return true
			}
		}
		return false
	}
	if !allowed(p.PrimaryCause, "none", "invalid_request", "scope_unavailable", "setup_failed", "navigation_failed", "context_unavailable", "document_changed", "evaluation_failed", "projection_invalid", "fetch_timeout", "fetch_failed", "http_rejected", "unexpected_success", "response_scope", "body_oversize", "body_read_failed", "body_invalid", "wire_unmatched", "cleanup_failed", "page_scope", "login_visible") ||
		!allowed(p.MIME, "missing", "json", "html", "other") ||
		!allowed(p.BodyClass, "not_read", "json_object", "json_nonobject", "invalid_json", "invalid_utf8", "oversize", "read_failed") ||
		!allowed(p.ErrorCategory, "none", "login_required", "challenge_required", "checkpoint_required", "feedback_required", "sentry_block", "csrf_failed", "unknown") ||
		!allowed(p.Cleanup, "not_started", "verified", "uncertain") {
		return rejected()
	}
	result, err := json.Marshal(p)
	if err != nil {
		return rejected()
	}
	return result, nil
}
