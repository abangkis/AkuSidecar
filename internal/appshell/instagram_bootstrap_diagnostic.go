package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"unicode/utf8"
)

type MainBrowserInstagramBootstrapDiagnosticProtocol interface {
	DiagnoseInstagramBootstrap(context.Context, string) (json.RawMessage, error)
}

func ValidateInstagramBootstrapDiagnostic(raw json.RawMessage) (json.RawMessage, error) {
	reject := func() (json.RawMessage, error) {
		return nil, &MainBrowserError{Code: "instagram_bootstrap_projection_invalid"}
	}
	if len(raw) > 1024 || !utf8.Valid(raw) {
		return reject()
	}
	fields := map[string]bool{"schema": true, "reason": true, "shapeReason": true, "modulePresent": true, "idString": true, "idSafeNumber": true, "usernameString": true, "documentBound": true, "serverFresh": true, "qualified": true, "cleanup": true}
	for _, key := range []string{"failureStage", "failureCondition", "cleanupStage", "cleanupCondition"} {
		fields[key] = true
	}
	fields["scanComplete"] = true
	fields["budgetExpanded"] = true
	values := map[string]any{}
	d := json.NewDecoder(bytes.NewReader(raw))
	if t, e := d.Token(); e != nil || t != json.Delim('{') {
		return reject()
	}
	for d.More() {
		t, e := d.Token()
		k, ok := t.(string)
		if e != nil || !ok || !fields[k] {
			return reject()
		}
		delete(fields, k)
		var v any
		if d.Decode(&v) != nil {
			return reject()
		}
		if k == "schema" || k == "reason" || k == "shapeReason" || k == "cleanup" || k == "failureStage" || k == "failureCondition" || k == "cleanupStage" || k == "cleanupCondition" {
			if _, ok := v.(string); !ok {
				return reject()
			}
		} else {
			if _, ok := v.(bool); !ok {
				return reject()
			}
		}
		values[k] = v
	}
	if t, e := d.Token(); e != nil || t != json.Delim('}') || len(fields) != 0 {
		return reject()
	}
	if _, e := d.Token(); e != io.EOF {
		return reject()
	}
	if values["schema"] != "aku.instagram-self-bootstrap-probe.v1" || values["qualified"] != false || values["serverFresh"] != false {
		return reject()
	}
	allowed := map[any]bool{"candidate": true, "missing": true, "duplicate": true, "conflict": true, "shape": true, "bounds": true, "invalid_json": true, "page_scope": true, "login_visible": true, "challenge": true, "document_changed": true, "unavailable": true}
	for _, reason := range []string{"bounds_script_count", "bounds_script_chars", "bounds_script_bytes", "bounds_total_bytes", "bounds_nodes", "bounds_depth", "bounds_fanout"} {
		allowed[reason] = true
	}
	for _, reason := range []string{"bounds_candidate_bytes", "bounds_parse_bytes", "bounds_parse_tokens", "bounds_entries", "scan_deadline", "scan_clock"} {
		allowed[reason] = true
	}
	complete := values["reason"] == "candidate" || values["reason"] == "missing" || values["reason"] == "duplicate" || values["reason"] == "conflict"
	if values["scanComplete"] != complete {
		return reject()
	}
	if !allowed[values["reason"]] || (values["cleanup"] != "verified" && values["cleanup"] != "uncertain") {
		return reject()
	}
	shapeReasons := map[string]bool{"none": true, "tuple_arity": true, "dependencies": true, "data_object": true, "missing_id": true, "missing_username": true, "id_type": true, "id_format": true, "username_type": true, "username_format": true}
	shapeReason, ok := values["shapeReason"].(string)
	shapeFailure := values["reason"] == "shape"
	if !ok || !shapeReasons[shapeReason] || shapeFailure != (shapeReason != "none") || shapeFailure && (values["modulePresent"] != true || values["idString"] != false || values["idSafeNumber"] != false || values["usernameString"] != false || values["scanComplete"] != false) {
		return reject()
	}
	enums := map[string][]string{"failureStage": {"none", "target_selection", "attachment", "setup", "frame_before", "evaluation", "frame_after"}, "failureCondition": {"none", "inventory", "missing_home", "ambiguous_home", "ownership", "request_failed", "context_missing", "document_changed", "invalid_operation", "invalid_projection", "exception"}, "cleanupStage": {"none", "detach", "attachment"}, "cleanupCondition": {"none", "request_failed", "attachment_pending"}}
	for field, choices := range enums {
		found := false
		for _, choice := range choices {
			found = found || values[field] == choice
		}
		if !found {
			return reject()
		}
	}
	if (values["failureStage"] == "none") != (values["failureCondition"] == "none") || (values["cleanupStage"] == "none") != (values["cleanupCondition"] == "none") || (values["cleanup"] == "verified" && values["cleanupStage"] != "none") || (values["cleanup"] == "uncertain" && values["cleanupStage"] == "none") {
		return reject()
	}
	if values["failureStage"] != "none" && values["scanComplete"] != false {
		return reject()
	}
	result, e := json.Marshal(values)
	if e != nil {
		return reject()
	}
	return result, nil
}
