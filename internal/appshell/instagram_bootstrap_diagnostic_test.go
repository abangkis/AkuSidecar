package appshell

import (
	"encoding/json"
	"strings"
	"testing"
)

const bootstrapReply = `{"schema":"aku.instagram-self-bootstrap-probe.v2","reason":"missing","shapeReason":"none","usernameReason":"none","modulePresent":false,"idString":false,"idSafeNumber":false,"usernameString":false,"duplicateDetected":false,"conflictDetected":false,"documentBound":true,"serverFresh":false,"qualified":false,"scanComplete":true,"budgetExpanded":false,"failureStage":"none","failureCondition":"none","cleanup":"verified","cleanupStage":"none","cleanupCondition":"none"}`

func TestInstagramBootstrapProjection(t *testing.T) {
	if _, err := ValidateInstagramBootstrapDiagnostic([]byte(bootstrapReply)); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{strings.Replace(bootstrapReply, `"qualified":false`, `"qualified":true`, 1), strings.Replace(bootstrapReply, `"serverFresh":false`, `"serverFresh":true`, 1), strings.Replace(bootstrapReply, `"modulePresent":false`, `"modulePresent":null`, 1), strings.Replace(bootstrapReply, `"reason":"missing"`, `"reason":"PRIVATE"`, 1), strings.TrimSuffix(bootstrapReply, "}") + `,"reason":"missing"}`, strings.TrimSuffix(bootstrapReply, "}") + `,"id":"PRIVATE"}`, bootstrapReply + ` {}`} {
		if _, err := ValidateInstagramBootstrapDiagnostic([]byte(value)); err == nil {
			t.Fatal("invalid projection accepted")
		}
	}
}

func TestInstagramBootstrapIndependentIdentityV2(t *testing.T) {
	cases := []struct {
		name   string
		fields map[string]any
		valid  bool
	}{
		{"id_without_username", map[string]any{"reason": "candidate", "modulePresent": true, "idString": true, "usernameReason": "missing_username"}, true},
		{"invalid_username", map[string]any{"reason": "candidate", "modulePresent": true, "idSafeNumber": true, "usernameReason": "username_type"}, true},
		{"mixed_username_observations", map[string]any{"reason": "duplicate", "modulePresent": true, "idString": true, "usernameString": true, "usernameReason": "missing_username", "duplicateDetected": true}, true},
		{"shape_and_later_conflict", map[string]any{"reason": "shape", "shapeReason": "dependencies", "modulePresent": true, "idString": true, "usernameString": true, "conflictDetected": true}, true},
		{"both_ambiguities", map[string]any{"reason": "conflict", "modulePresent": true, "idString": true, "idSafeNumber": true, "usernameString": true, "duplicateDetected": true, "conflictDetected": true}, true},
		{"incomplete_with_retained_evidence", map[string]any{"reason": "invalid_json", "scanComplete": false, "shapeReason": "id_type", "modulePresent": true, "idString": true, "usernameString": true, "conflictDetected": true}, true},
		{"legacy_schema", map[string]any{"schema": "aku.instagram-self-bootstrap-probe.v1"}, false},
		{"username_not_shape", map[string]any{"reason": "shape", "shapeReason": "missing_username", "modulePresent": true}, false},
		{"incomplete_shape", map[string]any{"reason": "shape", "shapeReason": "dependencies", "modulePresent": true, "scanComplete": false}, false},
		{"unreported_username", map[string]any{"reason": "candidate", "modulePresent": true, "idString": true}, false},
		{"unreported_shape", map[string]any{"reason": "unavailable", "scanComplete": false, "modulePresent": true}, false},
		{"ambiguity_without_id", map[string]any{"duplicateDetected": true}, false},
		{"conflict_missing_flag", map[string]any{"reason": "conflict", "modulePresent": true, "idString": true, "usernameString": true}, false},
		{"candidate_with_duplicate", map[string]any{"reason": "candidate", "modulePresent": true, "idString": true, "usernameString": true, "duplicateDetected": true}, false},
		{"unknown_username_reason", map[string]any{"usernameReason": "PRIVATE"}, false},
		{"private_identity", map[string]any{"accountId": "PRIVATE"}, false},
		{"fabricated_authority", map[string]any{"qualified": true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var value map[string]any
			if err := json.Unmarshal([]byte(bootstrapReply), &value); err != nil {
				t.Fatal(err)
			}
			for key, v := range tc.fields {
				value[key] = v
			}
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			projected, err := ValidateInstagramBootstrapDiagnostic(raw)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
			if tc.valid && !strings.Contains(string(projected), `"qualified":false`) {
				t.Fatal("discovery acquired authority")
			}
		})
	}
}

func TestInstagramBootstrapFailureAndCleanupProjection(t *testing.T) {
	raw := strings.Replace(bootstrapReply, `"failureStage":"none"`, `"failureStage":"evaluation"`, 1)
	raw = strings.Replace(raw, `"reason":"missing"`, `"reason":"unavailable"`, 1)
	raw = strings.Replace(raw, `"scanComplete":true`, `"scanComplete":false`, 1)
	raw = strings.Replace(raw, `"failureCondition":"none"`, `"failureCondition":"request_failed"`, 1)
	raw = strings.Replace(raw, `"cleanup":"verified"`, `"cleanup":"uncertain"`, 1)
	raw = strings.Replace(raw, `"cleanupStage":"none"`, `"cleanupStage":"detach"`, 1)
	raw = strings.Replace(raw, `"cleanupCondition":"none"`, `"cleanupCondition":"request_failed"`, 1)
	projected, err := ValidateInstagramBootstrapDiagnostic([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(projected), `"failureStage":"evaluation"`) || !strings.Contains(string(projected), `"cleanupStage":"detach"`) {
		t.Fatal("primary and cleanup details lost")
	}
	for _, invalid := range []string{strings.Replace(raw, `"failureCondition":"request_failed"`, `"failureCondition":"PRIVATE"`, 1), strings.Replace(raw, `"failureStage":"evaluation"`, `"failureStage":"none"`, 1), strings.Replace(raw, `"cleanup":"uncertain"`, `"cleanup":"verified"`, 1)} {
		if _, err := ValidateInstagramBootstrapDiagnostic([]byte(invalid)); err == nil {
			t.Fatal("forged diagnostic accepted")
		}
	}
}

func TestInstagramBootstrapBoundCategories(t *testing.T) {
	for _, reason := range []string{"bounds", "bounds_script_count", "bounds_script_chars", "bounds_script_bytes", "bounds_total_bytes", "bounds_nodes", "bounds_depth", "bounds_fanout", "bounds_candidate_bytes", "bounds_parse_bytes", "bounds_parse_tokens", "bounds_entries", "scan_deadline", "scan_clock"} {
		raw := strings.Replace(bootstrapReply, `"reason":"missing"`, `"reason":"`+reason+`"`, 1)
		raw = strings.Replace(raw, `"scanComplete":true`, `"scanComplete":false`, 1)
		projected, err := ValidateInstagramBootstrapDiagnostic([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", reason, err)
		}
		if !strings.Contains(string(projected), `"reason":"`+reason+`"`) {
			t.Fatal("bound category lost")
		}
	}
	for _, reason := range []string{"bounds_PRIVATE", "bounds_123", "bounds_script_count PRIVATE"} {
		raw := strings.Replace(bootstrapReply, `"reason":"missing"`, `"reason":"`+reason+`"`, 1)
		if _, err := ValidateInstagramBootstrapDiagnostic([]byte(raw)); err == nil {
			t.Fatal("unbounded diagnostic admitted")
		}
	}
}

func TestInstagramBootstrapScanCompleteContract(t *testing.T) {
	for _, change := range []struct{ old, new string }{
		{`"scanComplete":true`, `"scanComplete":false`},
		{`"reason":"missing"`, `"reason":"scan_deadline"`},
		{`"budgetExpanded":false`, `"budgetExpanded":null`},
		{`"failureStage":"none","failureCondition":"none"`, `"failureStage":"evaluation","failureCondition":"request_failed"`},
	} {
		raw := strings.Replace(bootstrapReply, change.old, change.new, 1)
		if _, err := ValidateInstagramBootstrapDiagnostic([]byte(raw)); err == nil {
			t.Fatal("inconsistent scan accepted")
		}
	}
}

func TestInstagramBootstrapShapeReasonProjection(t *testing.T) {
	shapeReasons := []string{"tuple_arity", "dependencies", "data_object", "missing_id", "id_type", "id_format"}
	for _, shapeReason := range shapeReasons {
		raw := strings.Replace(bootstrapReply, `"reason":"missing"`, `"reason":"shape"`, 1)
		raw = strings.Replace(raw, `"shapeReason":"none"`, `"shapeReason":"`+shapeReason+`"`, 1)
		raw = strings.Replace(raw, `"modulePresent":false`, `"modulePresent":true`, 1)
		projected, err := ValidateInstagramBootstrapDiagnostic([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", shapeReason, err)
		}
		if !strings.Contains(string(projected), `"shapeReason":"`+shapeReason+`"`) {
			t.Fatalf("shape reason lost: %s", shapeReason)
		}
	}
	validShape := strings.Replace(bootstrapReply, `"reason":"missing"`, `"reason":"shape"`, 1)
	validShape = strings.Replace(validShape, `"shapeReason":"none"`, `"shapeReason":"tuple_arity"`, 1)
	validShape = strings.Replace(validShape, `"modulePresent":false`, `"modulePresent":true`, 1)
	for _, invalid := range []string{
		strings.Replace(validShape, `"shapeReason":"tuple_arity"`, `"shapeReason":"PRIVATE"`, 1),
		strings.Replace(validShape, `"shapeReason":"tuple_arity"`, `"shapeReason":"none"`, 1),
		strings.Replace(bootstrapReply, `"shapeReason":"none"`, `"shapeReason":"tuple_arity"`, 1),
		strings.Replace(validShape, `"modulePresent":true`, `"modulePresent":false`, 1),
		strings.Replace(validShape, `"scanComplete":true`, `"scanComplete":false`, 1),
		strings.Replace(bootstrapReply, `"shapeReason":"none"`, `"shapeReason":null`, 1),
		strings.Replace(bootstrapReply, `"shapeReason":"none",`, ``, 1),
		strings.TrimSuffix(bootstrapReply, `}`) + `,"shapeReason":"none"}`,
	} {
		if _, err := ValidateInstagramBootstrapDiagnostic([]byte(invalid)); err == nil {
			t.Fatal("inconsistent or private shape projection accepted")
		}
	}
}
