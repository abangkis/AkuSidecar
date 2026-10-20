//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"strconv"
	"testing"
)

const (
	xQAShortCheckpointSchema    = "aku.x.qa-short-checkpoint.v1"
	xQAShortConcurrencySchema   = "aku.x.qa-short-concurrency.v1"
	xQAShortPreparationSchema   = "aku.x.qa-short-preparation.v1"
	xQAShortPointMaxBytes       = 16 * 1024
	xQAShortPreparationMaxBytes = 20 * 1024
	xQAShortConcurrencyMaxBytes = 64 * 1024
	xQAShortMaximumDurationMS   = 70_000
)

type xQAShortCheckpointReport struct {
	Schema       string          `json:"schema"`
	Passed       *bool           `json:"passed"`
	Observation  json.RawMessage `json:"observation"`
	MutationGate string          `json:"mutationGate"`
	Continuity   string          `json:"continuity"`
	CanAuthorize *bool           `json:"canAuthorize"`
}

type xQAShortConcurrencyCheckpoint struct {
	Role       string          `json:"role"`
	Checkpoint json.RawMessage `json:"checkpoint"`
}

type xQAShortConcurrencyReport struct {
	Schema               string                          `json:"schema"`
	Outcome              string                          `json:"outcome"`
	CollectorCreated     bool                            `json:"collectorCreated"`
	SeedAttempted        bool                            `json:"seedAttempted"`
	SessionTransferred   *bool                           `json:"sessionTransferred"`
	Checkpoints          []xQAShortConcurrencyCheckpoint `json:"checkpoints"`
	CompletedCheckpoints int                             `json:"completedCheckpoints"`
	OverlapMS            float64                         `json:"overlapMS"`
	DurationMS           float64                         `json:"durationMS"`
	CleanupVerified      bool                            `json:"cleanupVerified"`
	AccountMatch         string                          `json:"accountMatch"`
	Continuity           string                          `json:"continuity"`
	MutationGate         string                          `json:"mutationGate"`
	Refresh              string                          `json:"refresh"`
	CanAuthorize         *bool                           `json:"canAuthorize"`
	NativeQualified      bool                            `json:"nativeQualified"`
}

type xQAShortPreparationReport struct {
	Schema             string          `json:"schema"`
	CollectorCreated   *bool           `json:"collectorCreated"`
	Headless           *bool           `json:"headless"`
	IsolatedContext    *bool           `json:"isolatedContext"`
	DistinctProfile    *bool           `json:"distinctProfile"`
	Checkpoint         json.RawMessage `json:"checkpoint"`
	CollectorClosed    *bool           `json:"collectorClosed"`
	SessionTransferred *bool           `json:"sessionTransferred"`
	CanAuthorize       *bool           `json:"canAuthorize"`
}

func xQAShortJSONBool(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false"))
}

func xQAShortJSONInteger(raw json.RawMessage) (int, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0, false
	}
	value, err := strconv.Atoi(string(trimmed))
	if err != nil || strconv.Itoa(value) != string(trimmed) {
		return 0, false
	}
	return value, true
}

func xQAShortJSONFiniteNumber(raw json.RawMessage) (float64, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '-' && (trimmed[0] < '0' || trimmed[0] > '9') {
		return 0, false
	}
	var value float64
	if json.Unmarshal(trimmed, &value) != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false
	}
	return value, true
}

func xQAShortDecodeExact(raw []byte, maxBytes int, keys []string, target any) (map[string]json.RawMessage, bool) {
	if len(raw) == 0 || len(raw) > maxBytes || !xQABoundStrictJSON(raw) || !xQABoundExactFields(raw, keys...) {
		return nil, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(target) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, false
	}
	return fields, true
}

func xQAShortObservationPasses(observation xQABoundObservation) bool {
	return (observation.Diagnostic == "bound" || observation.Diagnostic == "observed_cookie_write") &&
		observation.Identity == "current_self_observed" && observation.Login == "protected_response_observed" &&
		observation.Freshness == "within_5s" && observation.CleanupVerified != nil && *observation.CleanupVerified &&
		xQABoundLoginObserved(observation.Source)
}

// decodeXQAShortCheckpoint independently recomputes the JS checkpoint's passed claim.
func decodeXQAShortCheckpoint(raw []byte) (xQAShortCheckpointReport, error) {
	var report xQAShortCheckpointReport
	bad := errors.New("QA short checkpoint rejected")
	fields, ok := xQAShortDecodeExact(raw, xQAShortPointMaxBytes,
		[]string{"schema", "passed", "observation", "mutationGate", "continuity", "canAuthorize"}, &report)
	if !ok || report.Schema != xQAShortCheckpointSchema || report.Passed == nil || !xQAShortJSONBool(fields["passed"]) ||
		report.MutationGate != "deferred_for_short_qa" || report.Continuity != "unknown" || !xQAFalse(report.CanAuthorize) {
		return report, bad
	}
	observationJSON := bytes.TrimSpace(report.Observation)
	if len(observationJSON) == 0 {
		return report, bad
	}
	if bytes.Equal(observationJSON, []byte("null")) {
		if *report.Passed {
			return report, bad
		}
		return report, nil
	}
	observation, err := decodeXQABoundObservation(observationJSON)
	if err != nil || *report.Passed != xQAShortObservationPasses(observation) {
		return report, bad
	}
	return report, nil
}

func xQAShortValidOutcome(outcome string) bool {
	switch outcome {
	case "checkpoint_rejected", "seed_rejected", "deadline", "reader_checkpoint_rejected", "collector_checkpoint_rejected",
		"account_mismatch", "overlap_not_observed", "reader_window_changed", "short_overlap_observed", "cleanup_unverified":
		return true
	default:
		return false
	}
}

func xQAShortValidCheckpointOrder(checkpoints []xQAShortConcurrencyCheckpoint, decoded []xQAShortCheckpointReport) bool {
	if len(checkpoints) != len(decoded) || len(checkpoints) > 4 {
		return false
	}
	if len(checkpoints) == 0 {
		return true
	}
	if checkpoints[0].Role != "reader" {
		return false
	}
	if len(checkpoints) >= 2 && checkpoints[1].Role != "collector" {
		return false
	}
	if len(checkpoints) >= 3 {
		if checkpoints[2].Role != "reader" && checkpoints[2].Role != "collector" {
			return false
		}
		if len(checkpoints) == 4 && (checkpoints[3].Role == checkpoints[2].Role ||
			(checkpoints[3].Role != "reader" && checkpoints[3].Role != "collector")) {
			return false
		}
	}
	return true
}

// decodeXQAShortConcurrency validates the exact report emitted by runQAShortFeasibility.
func decodeXQAShortConcurrency(raw []byte) (xQAShortConcurrencyReport, error) {
	var report xQAShortConcurrencyReport
	bad := errors.New("QA short concurrency report rejected")
	keys := []string{"schema", "outcome", "collectorCreated", "seedAttempted", "sessionTransferred", "checkpoints",
		"completedCheckpoints", "overlapMS", "durationMS", "cleanupVerified", "accountMatch", "continuity",
		"mutationGate", "refresh", "canAuthorize", "nativeQualified"}
	fields, ok := xQAShortDecodeExact(raw, xQAShortConcurrencyMaxBytes, keys, &report)
	if !ok || report.Schema != xQAShortConcurrencySchema || !xQAShortValidOutcome(report.Outcome) ||
		!xQAShortJSONBool(fields["collectorCreated"]) || !xQAShortJSONBool(fields["seedAttempted"]) ||
		!xQAShortJSONBool(fields["cleanupVerified"]) || !xQAShortJSONBool(fields["nativeQualified"]) ||
		!xQAFalse(report.CanAuthorize) || report.NativeQualified || report.Continuity != "unknown" || report.MutationGate != "deferred_for_short_qa" ||
		report.Refresh != "not_qualified" || (report.AccountMatch != "unknown" && report.AccountMatch != "match") {
		return report, bad
	}
	if _, ok := fields["sessionTransferred"]; !ok || report.SessionTransferred == nil && !bytes.Equal(bytes.TrimSpace(fields["sessionTransferred"]), []byte("null")) {
		return report, bad
	}
	if report.CompletedCheckpoints < 0 || report.CompletedCheckpoints > 4 {
		return report, bad
	}
	completedRaw, integerOK := xQAShortJSONInteger(fields["completedCheckpoints"])
	overlapMS, overlapOK := xQAShortJSONFiniteNumber(fields["overlapMS"])
	durationMS, durationOK := xQAShortJSONFiniteNumber(fields["durationMS"])
	if !integerOK || completedRaw != report.CompletedCheckpoints || !overlapOK || overlapMS < 0 || overlapMS > 5000 ||
		!durationOK || durationMS < 0 || durationMS > xQAShortMaximumDurationMS || report.OverlapMS != overlapMS ||
		report.DurationMS != durationMS {
		return report, bad
	}
	trimmedCheckpoints := bytes.TrimSpace(fields["checkpoints"])
	if len(trimmedCheckpoints) == 0 || trimmedCheckpoints[0] != '[' {
		return report, bad
	}
	var checkpointFields []json.RawMessage
	if json.Unmarshal(fields["checkpoints"], &checkpointFields) != nil || len(checkpointFields) != len(report.Checkpoints) || len(checkpointFields) > 4 {
		return report, bad
	}
	decodedCheckpoints := make([]xQAShortCheckpointReport, 0, len(checkpointFields))
	passedPoints := 0
	for index, checkpointRaw := range checkpointFields {
		if !xQABoundExactFields(checkpointRaw, "role", "checkpoint") {
			return report, bad
		}
		var checkpoint xQAShortConcurrencyCheckpoint
		decoder := json.NewDecoder(bytes.NewReader(checkpointRaw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&checkpoint) != nil || decoder.Decode(new(any)) != io.EOF ||
			checkpoint.Role != report.Checkpoints[index].Role || !bytes.Equal(bytes.TrimSpace(checkpoint.Checkpoint), bytes.TrimSpace(report.Checkpoints[index].Checkpoint)) {
			return report, bad
		}
		point, err := decodeXQAShortCheckpoint(checkpoint.Checkpoint)
		if err != nil {
			return report, bad
		}
		if *point.Passed {
			passedPoints++
		}
		decodedCheckpoints = append(decodedCheckpoints, point)
	}
	if !xQAShortValidCheckpointOrder(report.Checkpoints, decodedCheckpoints) || report.CompletedCheckpoints > passedPoints {
		return report, bad
	}
	transferred := report.SessionTransferred
	if !report.SeedAttempted {
		if transferred == nil || *transferred || report.CompletedCheckpoints != 0 || overlapMS != 0 {
			return report, bad
		}
	} else if !report.CollectorCreated || transferred != nil && !*transferred || len(decodedCheckpoints) == 0 ||
		decodedCheckpoints[0].Passed == nil || !*decodedCheckpoints[0].Passed || report.CompletedCheckpoints == 0 {
		return report, bad
	}
	if !report.CollectorCreated && (report.SeedAttempted || transferred != nil && *transferred || len(decodedCheckpoints) != 0 ||
		report.CompletedCheckpoints != 0 || overlapMS != 0) {
		return report, bad
	}
	if len(decodedCheckpoints) >= 2 {
		if transferred == nil || !*transferred {
			return report, bad
		}
	}
	if len(decodedCheckpoints) >= 3 && (!*decodedCheckpoints[0].Passed || !*decodedCheckpoints[1].Passed || report.CompletedCheckpoints < 2) {
		return report, bad
	}
	if overlapMS > 0 && (len(decodedCheckpoints) != 4 || report.CompletedCheckpoints != 4 || passedPoints != 4 ||
		transferred == nil || !*transferred) {
		return report, bad
	}
	if report.Outcome == "checkpoint_rejected" && report.CollectorCreated ||
		report.Outcome == "seed_rejected" && (!report.CollectorCreated || !report.SeedAttempted || transferred != nil ||
			report.CompletedCheckpoints != 1 || len(decodedCheckpoints) != 1 || decodedCheckpoints[0].Passed == nil || !*decodedCheckpoints[0].Passed) ||
		report.Outcome == "collector_checkpoint_rejected" && (transferred == nil || !*transferred) ||
		report.Outcome == "account_mismatch" && (!report.SeedAttempted || transferred == nil || !*transferred) ||
		report.Outcome == "overlap_not_observed" && (overlapMS != 0 || report.CompletedCheckpoints != 4) ||
		report.Outcome == "reader_window_changed" && report.CompletedCheckpoints != 4 {
		return report, bad
	}
	if report.Outcome == "cleanup_unverified" {
		if report.CleanupVerified {
			return report, bad
		}
	} else if report.Outcome != "deadline" && !report.CleanupVerified {
		return report, bad
	}
	if report.DurationMS > 60000 && report.Outcome != "deadline" {
		return report, bad
	}
	wantAccountMatch := "unknown"
	if report.CompletedCheckpoints == 4 && report.Outcome == "short_overlap_observed" {
		wantAccountMatch = "match"
	}
	if report.AccountMatch != wantAccountMatch {
		return report, bad
	}
	if report.Outcome == "short_overlap_observed" &&
		(!report.CollectorCreated || !report.SeedAttempted || transferred == nil || !*transferred || !report.CleanupVerified ||
			report.CompletedCheckpoints != 4 || len(decodedCheckpoints) != 4 || passedPoints != 4 || overlapMS <= 0 ||
			report.AccountMatch != "match") {
		return report, bad
	}
	return report, nil
}

// decodeXQAShortPreparation accepts only the value-free owned-collector summary.
func decodeXQAShortPreparation(raw []byte) (xQAShortPreparationReport, error) {
	var report xQAShortPreparationReport
	bad := errors.New("QA short preparation report rejected")
	fields, ok := xQAShortDecodeExact(raw, xQAShortPreparationMaxBytes,
		[]string{"schema", "collectorCreated", "headless", "isolatedContext", "distinctProfile", "checkpoint", "collectorClosed", "sessionTransferred", "canAuthorize"}, &report)
	if !ok || report.Schema != xQAShortPreparationSchema || !xQATrue(report.CollectorCreated) || !xQATrue(report.Headless) ||
		!xQATrue(report.IsolatedContext) || !xQATrue(report.DistinctProfile) || !xQATrue(report.CollectorClosed) ||
		!xQAFalse(report.SessionTransferred) || !xQAFalse(report.CanAuthorize) {
		return report, bad
	}
	for _, key := range []string{"collectorCreated", "headless", "isolatedContext", "distinctProfile", "collectorClosed", "sessionTransferred", "canAuthorize"} {
		if !xQAShortJSONBool(fields[key]) {
			return report, bad
		}
	}
	if len(bytes.TrimSpace(report.Checkpoint)) == 0 {
		return report, bad
	}
	if _, err := decodeXQAShortCheckpoint(report.Checkpoint); err != nil {
		return report, bad
	}
	return report, nil
}

func xQAShortCheckpointJSON(observation json.RawMessage, passed bool) json.RawMessage {
	canAuthorize := false
	encoded, _ := json.Marshal(xQAShortCheckpointReport{Schema: xQAShortCheckpointSchema, Passed: &passed,
		Observation: observation, MutationGate: "deferred_for_short_qa", Continuity: "unknown", CanAuthorize: &canAuthorize})
	return encoded
}

func xQAShortValidObservationJSON(diagnostic, binding string) json.RawMessage {
	observation := xQABoundReportFixture(xQABoundSourceJSON(200, "http_200_unqualified", "settings_match", true, true, 1),
		xQABoundBootstrapJSON(), diagnostic, "current_self_observed", "protected_response_observed", binding, "within_5s", true)
	encoded, _ := json.Marshal(observation)
	return encoded
}

func xQAShortConcurrencyFixture() xQAShortConcurrencyReport {
	passed := true
	observation := xQAShortValidObservationJSON("observed_cookie_write", "unknown")
	checkpoint := xQAShortCheckpointJSON(observation, true)
	return xQAShortConcurrencyReport{Schema: xQAShortConcurrencySchema, Outcome: "short_overlap_observed", CollectorCreated: true,
		SeedAttempted: true, SessionTransferred: &passed,
		Checkpoints: []xQAShortConcurrencyCheckpoint{{Role: "reader", Checkpoint: checkpoint}, {Role: "collector", Checkpoint: checkpoint},
			{Role: "collector", Checkpoint: checkpoint}, {Role: "reader", Checkpoint: checkpoint}},
		CompletedCheckpoints: 4, OverlapMS: 24.5, DurationMS: 1200, CleanupVerified: true, AccountMatch: "match",
		Continuity: "unknown", MutationGate: "deferred_for_short_qa", Refresh: "not_qualified", CanAuthorize: func() *bool { value := false; return &value }(),
		NativeQualified: false}
}

func xQAShortConcurrencyJSON(report xQAShortConcurrencyReport) []byte {
	encoded, _ := json.Marshal(report)
	return encoded
}

func TestXQAShortFeasibilityOffline(t *testing.T) {
	boundObservation := xQAShortValidObservationJSON("bound", "same_document_observed")
	checkpoint, err := decodeXQAShortCheckpoint(xQAShortCheckpointJSON(boundObservation, true))
	if err != nil || checkpoint.Passed == nil || !*checkpoint.Passed {
		t.Fatalf("valid bound checkpoint rejected: %v", err)
	}
	mutationObserved := xQAShortValidObservationJSON("observed_cookie_write", "unknown")
	mutationCheckpoint, err := decodeXQAShortCheckpoint(xQAShortCheckpointJSON(mutationObserved, true))
	if err != nil || mutationCheckpoint.Passed == nil || !*mutationCheckpoint.Passed || mutationCheckpoint.MutationGate != "deferred_for_short_qa" {
		t.Fatalf("observed startup write checkpoint lost its deferred mutation gate: %v", err)
	}
	partialObservation := xQABoundReportFixture(xQABoundSourceJSON(0, "not_observed", "", false, false, 0), json.RawMessage("null"),
		"identity_unavailable", "unknown", "unknown", "unknown", "unknown", true)
	partialObservationRaw, _ := json.Marshal(partialObservation)
	partialCheckpoint, err := decodeXQAShortCheckpoint(xQAShortCheckpointJSON(partialObservationRaw, false))
	if err != nil || partialCheckpoint.Passed == nil || *partialCheckpoint.Passed {
		t.Fatalf("valid partial checkpoint rejected: %v", err)
	}
	nullCheckpoint, err := decodeXQAShortCheckpoint(xQAShortCheckpointJSON(json.RawMessage("null"), false))
	if err != nil || nullCheckpoint.Passed == nil || *nullCheckpoint.Passed {
		t.Fatalf("null failed checkpoint rejected: %v", err)
	}

	for _, invalid := range [][]byte{
		xQAShortCheckpointJSON(boundObservation, false),
		xQAShortCheckpointJSON(json.RawMessage("null"), true),
		bytes.Replace(xQAShortCheckpointJSON(boundObservation, true), []byte(`"canAuthorize":false`), []byte(`"canAuthorize":null`), 1),
		bytes.Replace(xQAShortCheckpointJSON(boundObservation, true), []byte(`"passed":true`), []byte(`"passed":true,"passed":false`), 1),
		bytes.Replace(xQAShortCheckpointJSON(boundObservation, true), []byte(`"mutationGate":"deferred_for_short_qa"`), []byte(`"mutationGate":"deferred_for_short_qa","cookieValue":"PRIVATE"`), 1),
	} {
		if _, err := decodeXQAShortCheckpoint(invalid); err == nil {
			t.Fatal("forged, duplicated, null-authority, or reflected short checkpoint accepted")
		}
	}

	preparationCheckpoint := xQAShortCheckpointJSON(boundObservation, true)
	prepared := xQAShortPreparationReport{Schema: xQAShortPreparationSchema, CollectorCreated: func() *bool { value := true; return &value }(),
		Headless: func() *bool { value := true; return &value }(), IsolatedContext: func() *bool { value := true; return &value }(),
		DistinctProfile: func() *bool { value := true; return &value }(), Checkpoint: preparationCheckpoint,
		CollectorClosed: func() *bool { value := true; return &value }(), SessionTransferred: func() *bool { value := false; return &value }(),
		CanAuthorize: func() *bool { value := false; return &value }()}
	preparedRaw, _ := json.Marshal(prepared)
	if decoded, err := decodeXQAShortPreparation(preparedRaw); err != nil || decoded.Checkpoint == nil {
		t.Fatalf("valid short preparation rejected: %v", err)
	}
	for _, invalid := range [][]byte{
		bytes.Replace(preparedRaw, []byte(`"distinctProfile":true`), []byte(`"distinctProfile":false`), 1),
		bytes.Replace(preparedRaw, []byte(`"collectorClosed":true`), []byte(`"collectorClosed":null`), 1),
		bytes.Replace(preparedRaw, []byte(`"sessionTransferred":false`), []byte(`"sessionTransferred":true`), 1),
		bytes.Replace(preparedRaw, []byte(`"collectorClosed":true`), []byte(`"collectorClosed":true,"collectorPID":42`), 1),
	} {
		if _, err := decodeXQAShortPreparation(invalid); err == nil {
			t.Fatal("forged or reflected short preparation accepted")
		}
	}

	positive := xQAShortConcurrencyFixture()
	positiveRaw := xQAShortConcurrencyJSON(positive)
	if decoded, err := decodeXQAShortConcurrency(positiveRaw); err != nil || decoded.AccountMatch != "match" {
		t.Fatalf("valid four-point short overlap report rejected: %v", err)
	}
	partial := xQAShortConcurrencyReport{Schema: xQAShortConcurrencySchema, Outcome: "reader_checkpoint_rejected", CollectorCreated: true,
		SeedAttempted: false, SessionTransferred: func() *bool { value := false; return &value }(),
		Checkpoints:          []xQAShortConcurrencyCheckpoint{{Role: "reader", Checkpoint: xQAShortCheckpointJSON(boundObservation, true)}},
		CompletedCheckpoints: 0, OverlapMS: 0, DurationMS: 250, CleanupVerified: true, AccountMatch: "unknown",
		Continuity: "unknown", MutationGate: "deferred_for_short_qa", Refresh: "not_qualified", CanAuthorize: func() *bool { value := false; return &value }(),
		NativeQualified: false}
	if _, err := decodeXQAShortConcurrency(xQAShortConcurrencyJSON(partial)); err != nil {
		t.Fatalf("valid unresolved-seed partial report rejected: %v", err)
	}
	failed := xQAShortConcurrencyReport{Schema: xQAShortConcurrencySchema, Outcome: "checkpoint_rejected", CollectorCreated: false,
		SeedAttempted: false, SessionTransferred: func() *bool { value := false; return &value }(), Checkpoints: []xQAShortConcurrencyCheckpoint{},
		CompletedCheckpoints: 0, OverlapMS: 0, DurationMS: 0, CleanupVerified: true, AccountMatch: "unknown",
		Continuity: "unknown", MutationGate: "deferred_for_short_qa", Refresh: "not_qualified", CanAuthorize: func() *bool { value := false; return &value }(),
		NativeQualified: false}
	if _, err := decodeXQAShortConcurrency(xQAShortConcurrencyJSON(failed)); err != nil {
		t.Fatalf("valid pre-collector partial report rejected: %v", err)
	}
	seedRejected := xQAShortConcurrencyReport{Schema: xQAShortConcurrencySchema, Outcome: "seed_rejected", CollectorCreated: true,
		SeedAttempted: true, SessionTransferred: nil,
		Checkpoints:          []xQAShortConcurrencyCheckpoint{{Role: "reader", Checkpoint: xQAShortCheckpointJSON(boundObservation, true)}},
		CompletedCheckpoints: 1, OverlapMS: 0, DurationMS: 400, CleanupVerified: true, AccountMatch: "unknown",
		Continuity: "unknown", MutationGate: "deferred_for_short_qa", Refresh: "not_qualified", CanAuthorize: func() *bool { value := false; return &value }(),
		NativeQualified: false}
	if _, err := decodeXQAShortConcurrency(xQAShortConcurrencyJSON(seedRejected)); err != nil {
		t.Fatalf("valid lost-acknowledgement seed rejection report rejected: %v", err)
	}
	noSeedNullTransfer := failed
	noSeedNullTransfer.SessionTransferred = nil
	if _, err := decodeXQAShortConcurrency(xQAShortConcurrencyJSON(noSeedNullTransfer)); err == nil {
		t.Fatal("null transfer state accepted when no seed was attempted")
	}

	for name, mutate := range map[string]func(*xQAShortConcurrencyReport){
		"forged completion count":     func(report *xQAShortConcurrencyReport) { report.CompletedCheckpoints = 3 },
		"missing seed ownership":      func(report *xQAShortConcurrencyReport) { report.SeedAttempted = false },
		"missing cleanup":             func(report *xQAShortConcurrencyReport) { report.CleanupVerified = false },
		"missing collector":           func(report *xQAShortConcurrencyReport) { report.CollectorCreated = false },
		"zero overlap":                func(report *xQAShortConcurrencyReport) { report.OverlapMS = 0 },
		"claim mismatch":              func(report *xQAShortConcurrencyReport) { report.AccountMatch = "unknown" },
		"forged native qualification": func(report *xQAShortConcurrencyReport) { report.NativeQualified = true },
	} {
		forged := xQAShortConcurrencyFixture()
		mutate(&forged)
		if _, err := decodeXQAShortConcurrency(xQAShortConcurrencyJSON(forged)); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	for _, invalid := range [][]byte{
		bytes.Replace(positiveRaw, []byte(`"sessionTransferred":true`), []byte(`"sessionTransferred":false`), 1),
		bytes.Replace(positiveRaw, []byte(`"canAuthorize":false`), []byte(`"canAuthorize":true`), 1),
		bytes.Replace(positiveRaw, []byte(`"nativeQualified":false`), []byte(`"nativeQualified":null`), 1),
		bytes.Replace(positiveRaw, []byte(`"nativeQualified":false`), []byte(`"nativeQualified":false,"cookieValue":"PRIVATE"`), 1),
		bytes.Replace(positiveRaw, []byte(`"schema":"aku.x.qa-short-concurrency.v1"`), []byte(`"schema":"aku.x.qa-short-concurrency.v1","schema":"forged"`), 1),
		bytes.Replace(positiveRaw, []byte(`"identity":"current_self_observed"`), []byte(`"identity":"current_self_observed","identity":"unknown"`), 1),
	} {
		if _, err := decodeXQAShortConcurrency(invalid); err == nil {
			t.Fatal("malformed, duplicate, promoted, or reflected concurrency report accepted")
		}
	}
}
