package appshell

import (
	"strings"
	"testing"
)

const bootstrapReply = `{"schema":"aku.instagram-self-bootstrap-probe.v1","reason":"missing","shapeReason":"none","modulePresent":false,"idString":false,"idSafeNumber":false,"usernameString":false,"documentBound":true,"serverFresh":false,"qualified":false,"scanComplete":true,"budgetExpanded":false,"failureStage":"none","failureCondition":"none","cleanup":"verified","cleanupStage":"none","cleanupCondition":"none"}`

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
	shapeReasons := []string{"tuple_arity", "dependencies", "data_object", "missing_id", "missing_username", "id_type", "id_format", "username_type", "username_format"}
	for _, shapeReason := range shapeReasons {
		raw := strings.Replace(bootstrapReply, `"reason":"missing"`, `"reason":"shape"`, 1)
		raw = strings.Replace(raw, `"shapeReason":"none"`, `"shapeReason":"`+shapeReason+`"`, 1)
		raw = strings.Replace(raw, `"modulePresent":false`, `"modulePresent":true`, 1)
		raw = strings.Replace(raw, `"scanComplete":true`, `"scanComplete":false`, 1)
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
	validShape = strings.Replace(validShape, `"scanComplete":true`, `"scanComplete":false`, 1)
	for _, invalid := range []string{
		strings.Replace(validShape, `"shapeReason":"tuple_arity"`, `"shapeReason":"PRIVATE"`, 1),
		strings.Replace(validShape, `"shapeReason":"tuple_arity"`, `"shapeReason":"none"`, 1),
		strings.Replace(bootstrapReply, `"shapeReason":"none"`, `"shapeReason":"tuple_arity"`, 1),
		strings.Replace(validShape, `"modulePresent":true`, `"modulePresent":false`, 1),
		strings.Replace(validShape, `"scanComplete":false`, `"scanComplete":true`, 1),
		strings.Replace(validShape, `"idString":false`, `"idString":true`, 1),
		strings.Replace(bootstrapReply, `"shapeReason":"none"`, `"shapeReason":null`, 1),
		strings.Replace(bootstrapReply, `"shapeReason":"none",`, ``, 1),
		strings.TrimSuffix(bootstrapReply, `}`) + `,"shapeReason":"none"}`,
	} {
		if _, err := ValidateInstagramBootstrapDiagnostic([]byte(invalid)); err == nil {
			t.Fatal("inconsistent or private shape projection accepted")
		}
	}
}
