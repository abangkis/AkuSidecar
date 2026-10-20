//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"testing"
)

type xQACookieWriteScope struct {
	Schema             string                    `json:"schema"`
	Observation        json.RawMessage           `json:"observation"`
	CookieWrites       *xQACookieWriteProjection `json:"cookieWrites"`
	CanAuthorize       *bool                     `json:"canAuthorize"`
	SessionTransferred *bool                     `json:"sessionTransferred"`
}

type xQACookieWriteProjection struct {
	Schema        string           `json:"schema"`
	Diagnostic    string           `json:"diagnostic"`
	HeaderEvents  int              `json:"headerEvents"`
	Unresolved    int              `json:"unresolved"`
	Writes        []xQACookieWrite `json:"writes"`
	Qualification string           `json:"qualification"`
	Acceptance    string           `json:"acceptance"`
}

type xQACookieWrite struct {
	Name        string `json:"name"`
	Domain      string `json:"domain"`
	HostOnly    *bool  `json:"hostOnly"`
	Path        string `json:"path"`
	Secure      bool   `json:"secure"`
	HTTPOnly    bool   `json:"httpOnly"`
	SameSite    string `json:"sameSite"`
	Partitioned bool   `json:"partitioned"`
	Phase       string `json:"phase"`
	Setter      string `json:"setter"`
	RequestRole string `json:"requestRole"`
	Source      string `json:"source"`
}

func xQACookieWriteJSONBool(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	return bytes.Equal(trimmed, []byte("true")) || bytes.Equal(trimmed, []byte("false"))
}

func xQACookieWriteJSONInteger(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	value, err := strconv.Atoi(string(trimmed))
	return err == nil && value >= 0 && strconv.Itoa(value) == string(trimmed)
}

func xQACookieWriteComplete(write xQACookieWrite) bool {
	return write.Name != "other" && (write.Domain == "x.com" || write.Domain == "api.x.com") &&
		write.Path != "other" && write.Path != "unknown" && write.Setter != "unknown" && write.RequestRole != "unbound" &&
		(write.Setter == write.Domain || write.Setter == "api.x.com" && write.Domain == "x.com")
}

func xQACookieWriteValidRow(write xQACookieWrite) bool {
	switch write.Name {
	case "auth_token", "ct0", "twid", "kdt", "auth_multi", "att", "lang", "guest_id", "guest_id_ads",
		"guest_id_marketing", "personalization_id", "external_referer", "__cf_bm", "cf_clearance", "other":
	default:
		return false
	}
	switch write.Domain {
	case "x.com", "api.x.com", "other", "unknown":
	default:
		return false
	}
	switch write.Path {
	case "/", "/i", "/i/", "/1.1/account", "/1.1/account/", "/i/api/1.1/account", "/i/api/1.1/account/", "other", "unknown":
	default:
		return false
	}
	switch write.SameSite {
	case "absent", "strict", "lax", "none":
	default:
		return false
	}
	switch write.Phase {
	case "before_identity", "after_identity":
	default:
		return false
	}
	switch write.Setter {
	case "x.com", "api.x.com", "unknown":
	default:
		return false
	}
	switch write.RequestRole {
	case "settings", "other_x", "unbound":
	default:
		return false
	}
	switch write.Source {
	case "response", "extra_info":
	default:
		return false
	}
	if write.Setter == "unknown" {
		if write.RequestRole != "unbound" {
			return false
		}
	} else if write.RequestRole == "unbound" || write.Domain == "unknown" || write.Path == "unknown" {
		return false
	}
	switch {
	case write.HostOnly == nil:
		if write.Setter != "unknown" || write.Domain != "unknown" {
			return false
		}
	case *write.HostOnly:
		if write.Setter == "unknown" || write.Domain != write.Setter {
			return false
		}
	default:
		if write.Domain == "unknown" {
			return false
		}
	}
	return true
}

func xQACookieWriteValidateProjection(raw json.RawMessage, projection *xQACookieWriteProjection) bool {
	if projection == nil || !xQABoundExactFields(raw,
		"schema", "diagnostic", "headerEvents", "unresolved", "writes", "qualification", "acceptance") ||
		projection.Schema != "aku.x.qa-cookie-write-projection.v1" || projection.HeaderEvents < 0 || projection.HeaderEvents > 64 ||
		projection.Unresolved < 0 || projection.Unresolved > 64 || len(projection.Writes) > 32 || projection.Acceptance != "header_only" {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(bytes.TrimSpace(fields["writes"])) == 0 || bytes.TrimSpace(fields["writes"])[0] != '[' {
		return false
	}
	if !xQACookieWriteJSONInteger(fields["headerEvents"]) || !xQACookieWriteJSONInteger(fields["unresolved"]) {
		return false
	}
	var encodedWrites []json.RawMessage
	if json.Unmarshal(fields["writes"], &encodedWrites) != nil || len(encodedWrites) != len(projection.Writes) {
		return false
	}
	candidate := false
	partial := projection.Unresolved > 0
	for index, write := range projection.Writes {
		if !xQABoundExactFields(encodedWrites[index], "name", "domain", "hostOnly", "path", "secure", "httpOnly", "sameSite", "partitioned", "phase", "setter", "requestRole", "source") ||
			!xQACookieWriteValidRow(write) {
			return false
		}
		var rowFields map[string]json.RawMessage
		if json.Unmarshal(encodedWrites[index], &rowFields) != nil ||
			!xQACookieWriteJSONBool(rowFields["secure"]) || !xQACookieWriteJSONBool(rowFields["httpOnly"]) ||
			!xQACookieWriteJSONBool(rowFields["partitioned"]) {
			return false
		}
		if !xQACookieWriteComplete(write) {
			partial = true
		}
		candidate = candidate || write.Name == "auth_token" || write.Name == "ct0"
	}
	if len(projection.Writes) > 0 && projection.HeaderEvents == 0 {
		return false
	}
	if projection.HeaderEvents == 64 && projection.Unresolved != 64 ||
		projection.HeaderEvents > 0 && len(projection.Writes) == 0 && projection.Unresolved == 0 {
		return false
	}
	expectedDiagnostic := "projected"
	if partial {
		expectedDiagnostic = "partial"
	} else if projection.HeaderEvents == 0 {
		expectedDiagnostic = "not_observed"
	}
	expectedQualification := "not_observed"
	if partial {
		expectedQualification = "mixed_or_unresolved"
	} else if candidate {
		expectedQualification = "candidate_names_present"
	} else if len(projection.Writes) > 0 {
		expectedQualification = "outside_candidate_names_only"
	}
	return projection.Diagnostic == expectedDiagnostic && projection.Qualification == expectedQualification
}

func decodeXQACookieWriteScope(raw []byte) (xQACookieWriteScope, error) {
	var result xQACookieWriteScope
	bad := errors.New("QA cookie write scope rejected")
	if len(raw) == 0 || len(raw) > 16384 || !xQABoundStrictJSON(raw) || !xQABoundExactFields(raw,
		"schema", "observation", "cookieWrites", "canAuthorize", "sessionTransferred") {
		return result, bad
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF || result.Schema != "aku.x.qa-cookie-write-scope.v1" ||
		!xQAFalse(result.CanAuthorize) || !xQAFalse(result.SessionTransferred) || result.CookieWrites == nil {
		return result, bad
	}
	observation, err := decodeXQABoundObservation(result.Observation)
	if err != nil {
		return result, bad
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || !xQACookieWriteValidateProjection(fields["cookieWrites"], result.CookieWrites) {
		return result, bad
	}
	if observation.Diagnostic == "bound" {
		for _, write := range result.CookieWrites.Writes {
			if write.Phase == "after_identity" {
				return result, bad
			}
		}
	}
	return result, nil
}

func xQACookieWriteObservationJSON() json.RawMessage {
	observation := xQABoundReportFixture(xQABoundSourceJSON(200, "http_200_unqualified", "settings_match", true, true, 1),
		xQABoundBootstrapJSON(), "bound", "current_self_observed", "protected_response_observed", "same_document_observed", "within_5s", true)
	encoded, _ := json.Marshal(observation)
	return encoded
}

func xQACookieWriteScopeFixture(projection xQACookieWriteProjection) []byte {
	f := false
	result := xQACookieWriteScope{Schema: "aku.x.qa-cookie-write-scope.v1", Observation: xQACookieWriteObservationJSON(),
		CookieWrites: &projection, CanAuthorize: &f, SessionTransferred: &f}
	encoded, _ := json.Marshal(result)
	return encoded
}

func TestXQACookieWriteScopeOffline(t *testing.T) {
	hostOnly, notHostOnly := true, false
	positive := xQACookieWriteProjection{Schema: "aku.x.qa-cookie-write-projection.v1", Diagnostic: "projected", HeaderEvents: 1,
		Writes: []xQACookieWrite{{Name: "auth_token", Domain: "api.x.com", HostOnly: &hostOnly, Path: "/", Secure: true,
			HTTPOnly: true, SameSite: "lax", Partitioned: false, Phase: "before_identity", Setter: "api.x.com", RequestRole: "settings", Source: "response"}},
		Qualification: "candidate_names_present", Acceptance: "header_only"}
	raw := xQACookieWriteScopeFixture(positive)
	if _, err := decodeXQACookieWriteScope(raw); err != nil {
		t.Fatalf("valid candidate-name scope rejected: %v", err)
	}
	afterIdentityWrite := positive
	afterIdentityWrite.Writes = append([]xQACookieWrite(nil), positive.Writes...)
	afterIdentityWrite.Writes[0].Phase = "after_identity"
	if _, err := decodeXQACookieWriteScope(xQACookieWriteScopeFixture(afterIdentityWrite)); err == nil {
		t.Fatal("after-identity cookie write did not preserve the observation mutation fence")
	}

	partial := xQACookieWriteProjection{Schema: "aku.x.qa-cookie-write-projection.v1", Diagnostic: "partial", HeaderEvents: 1,
		Unresolved: 1, Writes: []xQACookieWrite{}, Qualification: "mixed_or_unresolved", Acceptance: "header_only"}
	if _, err := decodeXQACookieWriteScope(xQACookieWriteScopeFixture(partial)); err != nil {
		t.Fatalf("valid partial projection rejected: %v", err)
	}
	notObserved := xQACookieWriteProjection{Schema: "aku.x.qa-cookie-write-projection.v1", Diagnostic: "not_observed",
		Writes: []xQACookieWrite{}, Qualification: "not_observed", Acceptance: "header_only"}
	if _, err := decodeXQACookieWriteScope(xQACookieWriteScopeFixture(notObserved)); err != nil {
		t.Fatalf("valid no-write projection rejected: %v", err)
	}

	outside := xQACookieWriteProjection{Schema: "aku.x.qa-cookie-write-projection.v1", Diagnostic: "projected", HeaderEvents: 1,
		Writes: []xQACookieWrite{{Name: "lang", Domain: "x.com", HostOnly: &notHostOnly, Path: "/i", Secure: false,
			HTTPOnly: false, SameSite: "absent", Partitioned: false, Phase: "before_identity", Setter: "api.x.com", RequestRole: "other_x", Source: "extra_info"}},
		Qualification: "outside_candidate_names_only", Acceptance: "header_only"}
	if _, err := decodeXQACookieWriteScope(xQACookieWriteScopeFixture(outside)); err != nil {
		t.Fatalf("valid outside-candidate name projection rejected: %v", err)
	}

	for _, bad := range [][]byte{
		bytes.Replace(raw, []byte(`"canAuthorize":false`), []byte(`"canAuthorize":true`), 1),
		bytes.Replace(raw, []byte(`"sessionTransferred":false`), []byte(`"sessionTransferred":true`), 1),
		bytes.Replace(raw, []byte(`"acceptance":"header_only"},"canAuthorize":false`), []byte(`"acceptance":"header_only"},"canAuthorize":true`), 1),
		bytes.Replace(raw, []byte(`"acceptance":"header_only"},"canAuthorize":false,"sessionTransferred":false}`), []byte(`"acceptance":"header_only"},"canAuthorize":false,"sessionTransferred":true}`), 1),
		bytes.Replace(raw, []byte(`"continuity":"unknown","canAuthorize":false`), []byte(`"continuity":"unknown","canAuthorize":true`), 1),
		bytes.Replace(raw, []byte(`"schema":"aku.x.qa-cookie-write-scope.v1"`), []byte(`"schema":"aku.x.qa-cookie-write-scope.v1","accountId":"123456789"`), 1),
		bytes.Replace(raw, []byte(`"qualification":"candidate_names_present"`), []byte(`"qualification":"outside_candidate_names_only"`), 1),
		bytes.Replace(raw, []byte(`"diagnostic":"projected"`), []byte(`"diagnostic":"not_observed"`), 1),
		bytes.Replace(raw, []byte(`"headerEvents":1`), []byte(`"headerEvents":65`), 1),
		bytes.Replace(raw, []byte(`"headerEvents":1`), []byte(`"headerEvents":null`), 1),
		bytes.Replace(raw, []byte(`"unresolved":0`), []byte(`"unresolved":null`), 1),
		bytes.Replace(raw, []byte(`"secure":true`), []byte(`"secure":null`), 1),
		bytes.Replace(raw, []byte(`"httpOnly":true`), []byte(`"httpOnly":null`), 1),
		bytes.Replace(raw, []byte(`"partitioned":false`), []byte(`"partitioned":null`), 1),
		bytes.Replace(raw, []byte(`"name":"auth_token"`), []byte(`"name":"private_cookie"`), 1),
		bytes.Replace(raw, []byte(`"domain":"api.x.com"`), []byte(`"domain":"private.example"`), 1),
		bytes.Replace(raw, []byte(`"path":"/"`), []byte(`"path":"/private"`), 1),
		bytes.Replace(raw, []byte(`"name":"auth_token"`), []byte(`"name":"auth_token=PRIVATE"`), 1),
		bytes.Replace(raw, []byte(`"name":"auth_token"`), []byte(`"name":"auth_token","value":"PRIVATE"`), 1),
		bytes.Replace(raw, []byte(`"source":"response"`), []byte(`"source":"https://api.x.com/private"`), 1),
		bytes.Replace(raw, []byte(`"name":"auth_token"`), []byte(`"name":"auth_token","name":"ct0"`), 1),
		bytes.Replace(raw, []byte(`"headerEvents":1`), []byte(`"headerEvents":1,"headerEvents":1`), 1),
		bytes.Replace(raw, []byte(`"identity":"current_self_observed"`), []byte(`"identity":"current_self_observed","identity":"unknown"`), 1),
	} {
		if _, err := decodeXQACookieWriteScope(bad); err == nil {
			t.Fatal("inconsistent, promoted, duplicated or reflected cookie-write report accepted")
		}
	}

	partialRow := positive
	partialRow.Writes = append([]xQACookieWrite(nil), positive.Writes...)
	partialRow.Writes[0].RequestRole = "unbound"
	partialRow.Writes[0].Setter = "unknown"
	partialRow.Writes[0].HostOnly = &notHostOnly
	partialRow.Diagnostic = "partial"
	partialRow.Qualification = "mixed_or_unresolved"
	if _, err := decodeXQACookieWriteScope(xQACookieWriteScopeFixture(partialRow)); err != nil {
		t.Fatalf("valid incomplete unbound write rejected: %v", err)
	}
	partialRow.Diagnostic = "projected"
	partialRow.Qualification = "candidate_names_present"
	if _, err := decodeXQACookieWriteScope(xQACookieWriteScopeFixture(partialRow)); err == nil {
		t.Fatal("partial unbound scope promoted to a complete write")
	}
}
