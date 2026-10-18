//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xEarlyBootstrapCapture struct {
	Schema           string          `json:"schema"`
	StopReason       string          `json:"stopReason"`
	PauseCount       int             `json:"pauseCount"`
	ReadAttempts     int             `json:"readAttempts"`
	BootstrapSeen    bool            `json:"bootstrapSeen"`
	Snapshot         json.RawMessage `json:"snapshot"`
	CleanupVerified  bool            `json:"cleanupVerified"`
	CanAuthorize     bool            `json:"canAuthorize"`
	ContinuityProven bool            `json:"continuityProven"`
}

type xEarlyBootstrapDiagnostics struct {
	Schema          string            `json:"schema"`
	Commands        map[string]string `json:"commands"`
	FailureCategory string            `json:"failureCategory"`
	CanAuthorize    bool              `json:"canAuthorize"`
}

func decodeXEarlyBootstrapDiagnostics(raw []byte) (xEarlyBootstrapDiagnostics, error) {
	var value xEarlyBootstrapDiagnostics
	invalid := errors.New("early transport diagnostics rejected")
	var fields map[string]json.RawMessage
	if len(raw) == 0 || len(raw) > 2048 || json.Unmarshal(raw, &fields) != nil || len(fields) != 4 ||
		string(fields["canAuthorize"]) != "false" {
		return value, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.CanAuthorize ||
		value.Schema != "aku.x.early-bootstrap-transport.v1" || len(value.Commands) != 9 {
		return value, invalid
	}
	for _, key := range []string{"frameTree", "runtimeEnable", "debuggerEnable", "breakpointSet", "reload", "resume", "breakpointRemove", "debuggerDisable", "runtimeDisable"} {
		switch value.Commands[key] {
		case "not_attempted", "sent", "acknowledged", "failed":
		default:
			return value, invalid
		}
	}
	switch value.FailureCategory {
	case "none", "frame_count_limit", "frame_invalid", "command_timeout", "other":
	default:
		return value, invalid
	}
	return value, nil
}

func decodeXEarlyBootstrap(raw []byte) (xEarlyBootstrapCapture, error) {
	var value xEarlyBootstrapCapture
	invalid := errors.New("early bootstrap capture rejected")
	if len(raw) == 0 || len(raw) > 4096 {
		return value, invalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 9 {
		return value, invalid
	}
	for _, key := range []string{"schema", "stopReason", "pauseCount", "readAttempts", "bootstrapSeen", "snapshot", "cleanupVerified", "canAuthorize", "continuityProven"} {
		if fields[key] == nil {
			return value, invalid
		}
	}
	if string(fields["canAuthorize"]) != "false" || string(fields["continuityProven"]) != "false" {
		return value, invalid
	}
	for _, key := range []string{"bootstrapSeen", "cleanupVerified"} {
		if string(fields[key]) != "true" && string(fields[key]) != "false" {
			return value, invalid
		}
	}
	for _, key := range []string{"pauseCount", "readAttempts"} {
		if bytes.Equal(bytes.TrimSpace(fields[key]), []byte("null")) {
			return value, invalid
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Schema != "aku.x.early-bootstrap-capture.v1" ||
		value.CanAuthorize || value.ContinuityProven || value.PauseCount < 0 || value.PauseCount > 64 || value.ReadAttempts < 0 || value.ReadAttempts > value.PauseCount {
		return value, invalid
	}
	switch value.StopReason {
	case "captured", "deadline", "script_limit", "wrong_page", "unexpected_pause", "evaluation_failed", "setup_failed":
	default:
		return value, invalid
	}
	if bytes.Equal(value.Snapshot, []byte("null")) {
		if value.BootstrapSeen || value.StopReason == "captured" {
			return value, invalid
		}
		return value, nil
	}
	projection, err := decodeXBootstrapCurrentSelf(value.Snapshot)
	if err != nil || value.ReadAttempts == 0 {
		return value, invalid
	}
	if value.BootstrapSeen != (projection.BootstrapState == "object") {
		return value, invalid
	}
	if value.StopReason == "captured" && (!value.BootstrapSeen || projection.StableIDStringCandidates == 0) {
		return value, invalid
	}
	// Persist the reconstructed, enum-only DTO, never the supplied JSON bytes.
	value.Snapshot, err = json.Marshal(projection)
	if err != nil {
		return value, invalid
	}
	return value, nil
}

func TestXEarlyBootstrapOffline(t *testing.T) {
	diagnostics := xEarlyBootstrapDiagnostics{Schema: "aku.x.early-bootstrap-transport.v1", FailureCategory: "none", Commands: map[string]string{}}
	for _, key := range []string{"frameTree", "runtimeEnable", "debuggerEnable", "breakpointSet", "reload", "resume", "breakpointRemove", "debuggerDisable", "runtimeDisable"} {
		diagnostics.Commands[key] = "acknowledged"
	}
	diagnosticRaw, _ := json.Marshal(diagnostics)
	if _, err := decodeXEarlyBootstrapDiagnostics(diagnosticRaw); err != nil {
		t.Fatal("fixed diagnostic labels rejected")
	}
	diagnostics.Commands["frameTree"] = "private-source-value"
	diagnosticRaw, _ = json.Marshal(diagnostics)
	if _, err := decodeXEarlyBootstrapDiagnostics(diagnosticRaw); err == nil {
		t.Fatal("private command status accepted")
	}
	projection := xBootstrapCurrentSelf{Schema: "aku.x.bootstrap-current-self.v1", Diagnostic: "projected", BootstrapState: "object", SessionState: "object", StableIDStringCandidates: 1}
	projection.IDFields.UserID, projection.IDFields.UserIDString = "string", "absent"
	projection.AuthFields.LoggedIn, projection.AuthFields.IsAuthenticated = "absent", "absent"
	projection.AuthFields.Authenticated, projection.AuthFields.IsActive = "absent", "true"
	projection.EntityChecks.NormalizedUsers, projection.EntityChecks.RootUsers = "match", "absent"
	snapshot, _ := json.Marshal(projection)
	positive := xEarlyBootstrapCapture{Schema: "aku.x.early-bootstrap-capture.v1", StopReason: "captured", PauseCount: 2, ReadAttempts: 1, BootstrapSeen: true, Snapshot: snapshot, CleanupVerified: true}
	positiveRaw, _ := json.Marshal(positive)
	if _, err := decodeXEarlyBootstrap(positiveRaw); err != nil {
		t.Fatal("valid positive capture rejected")
	}
	// Even duplicate JSON fields cannot survive in the stored raw-message slot.
	positive.Snapshot = bytes.Replace(snapshot, []byte(`"schema":`), []byte(`"schema":"private-original-value","schema":`), 1)
	positiveRaw, _ = json.Marshal(positive)
	sanitized, err := decodeXEarlyBootstrap(positiveRaw)
	if err != nil || bytes.Contains(sanitized.Snapshot, []byte("private-original-value")) {
		t.Fatal("supplied JSON was not reconstructed before persistence")
	}
	const valid = `{"schema":"aku.x.early-bootstrap-capture.v1","stopReason":"deadline","pauseCount":3,"readAttempts":0,"bootstrapSeen":false,"snapshot":null,"cleanupVerified":true,"canAuthorize":false,"continuityProven":false}`
	if _, err := decodeXEarlyBootstrap([]byte(valid)); err != nil {
		t.Fatal("bounded negative capture rejected")
	}
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["contextId"] = 123 },
		func(v map[string]any) { v["canAuthorize"] = true },
		func(v map[string]any) { v["continuityProven"] = true },
		func(v map[string]any) { v["stopReason"] = "private-source-error" },
		func(v map[string]any) { v["pauseCount"] = 65 },
		func(v map[string]any) { v["pauseCount"] = nil },
		func(v map[string]any) { v["readAttempts"] = 4 },
		func(v map[string]any) { v["snapshot"] = map[string]any{"privateId": "secret"} },
		func(v map[string]any) { v["bootstrapSeen"] = true },
		func(v map[string]any) { v["stopReason"] = "captured" },
		func(v map[string]any) { v["cleanupVerified"] = nil },
		func(v map[string]any) { delete(v, "cleanupVerified") },
	} {
		var candidate map[string]any
		_ = json.Unmarshal([]byte(valid), &candidate)
		mutate(candidate)
		raw, _ := json.Marshal(candidate)
		if _, err := decodeXEarlyBootstrap(raw); err == nil {
			t.Fatal("invalid or private capture accepted")
		}
	}
}
