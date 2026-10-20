//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xQABoundObservation struct {
	Schema                     string             `json:"schema"`
	SourceQualification        string             `json:"sourceQualification"`
	Diagnostic                 string             `json:"diagnostic"`
	Identity                   string             `json:"identity"`
	Login                      string             `json:"login"`
	Binding                    string             `json:"binding"`
	Freshness                  string             `json:"freshness"`
	Bootstrap                  json.RawMessage    `json:"bootstrap"`
	Source                     *xQADirectResponse `json:"source"`
	CleanupVerified            *bool              `json:"cleanupVerified"`
	StartupCookieWriteObserved *bool              `json:"startupCookieWriteObserved"`
	Enrollment                 string             `json:"enrollment"`
	Continuity                 string             `json:"continuity"`
	CanAuthorize               *bool              `json:"canAuthorize"`
	SessionTransferred         *bool              `json:"sessionTransferred"`
}

func xQABoundScanValue(decoder *json.Decoder, token json.Token, depth int) bool {
	if depth > 32 {
		return false
	}
	delimiter, compound := token.(json.Delim)
	if !compound {
		return true
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return false
			}
			key, ok := keyToken.(string)
			if !ok {
				return false
			}
			if _, exists := seen[key]; exists {
				return false
			}
			seen[key] = struct{}{}
			value, err := decoder.Token()
			if err != nil || !xQABoundScanValue(decoder, value, depth+1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil || !xQABoundScanValue(decoder, value, depth+1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

// xQABoundStrictJSON checks duplicate keys at every depth before typed decoding.
// encoding/json otherwise accepts duplicate object keys and keeps the last one.
func xQABoundStrictJSON(raw []byte) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	first, err := decoder.Token()
	if err != nil || first != json.Delim('{') || !xQABoundScanValue(decoder, first, 0) {
		return false
	}
	_, err = decoder.Token()
	return err == io.EOF
}

func xQABoundExactFields(raw []byte, keys ...string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != len(keys) {
		return false
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			return false
		}
	}
	return true
}

func xQABoundIdentityObserved(value *xBootstrapCurrentSelf) bool {
	if value == nil || value.Diagnostic != "projected" || value.BootstrapState != "object" || value.SessionState != "object" ||
		value.IDFields.UserID != "string" || (value.IDFields.UserIDString != "absent" && value.IDFields.UserIDString != "string") ||
		value.SessionIDsAgree != nil && !*value.SessionIDsAgree {
		return false
	}
	entities := []string{value.EntityChecks.NormalizedUsers, value.EntityChecks.RootUsers}
	hasMatch := false
	for _, check := range entities {
		if check == "match" {
			hasMatch = true
		} else if check != "absent" {
			return false
		}
	}
	if !hasMatch {
		return false
	}
	for _, auth := range []string{value.AuthFields.LoggedIn, value.AuthFields.IsAuthenticated, value.AuthFields.Authenticated, value.AuthFields.IsActive} {
		if auth != "absent" && auth != "true" {
			return false
		}
	}
	return true
}

func xQABoundLoginObserved(source *xQADirectResponse) bool {
	return source != nil && source.Diagnostic == "http_200_unqualified" && source.MatchingRequests == 1 &&
		source.Status != nil && *source.Status == 200 && xQATrue(source.ResponseFresh) && xQATrue(source.Completed) &&
		source.BodyReadAttempts == 1 && source.Body != nil && source.Body.Category == "settings_match" && xQATrue(source.Body.SettingsMatch)
}

func xQABoundValidateSource(raw json.RawMessage, source *xQADirectResponse) bool {
	if source == nil || source.Role != "a" || !xQABoundExactFields(raw,
		"schema", "route", "role", "diagnostic", "matchingRequests", "status", "responseFresh", "completed", "bodyReadAttempts", "body", "canAuthorize", "sessionTransferred") {
		return false
	}
	var sourceFields map[string]json.RawMessage
	if json.Unmarshal(raw, &sourceFields) != nil {
		return false
	}
	if !bytes.Equal(bytes.TrimSpace(sourceFields["body"]), []byte("null")) && !xQABoundExactFields(sourceFields["body"], "schema", "category", "settingsMatch") {
		return false
	}
	f, tr := false, true
	dummy := func(role string) *xQADirectResponse {
		return &xQADirectResponse{Schema: "aku.x.qa-direct-response.v1", Route: "api_v1", Role: role,
			Diagnostic: "not_observed", Completed: &f, CanAuthorize: &f, SessionTransferred: &f}
	}
	b := dummy("b")
	outcome := "positive_control_failed"
	if xQABoundLoginObserved(source) {
		outcome = "comparison_inconclusive"
	}
	shim := xQADirectAB{Schema: "aku.x.qa-direct-ab.v1", Navigation: "direct_api_v1", A: source, B: b,
		OwnerHomeBefore: &tr, OwnerHomeAfter: &tr, TabsDisposed: &tr, GuestContextDisposed: &tr,
		Outcome: outcome, Authentication: "unknown", CanAuthorize: &f, SessionTransferred: &f}
	encoded, err := json.Marshal(shim)
	if err != nil {
		return false
	}
	if _, err = decodeXQADirectAB(encoded); err != nil {
		return false
	}
	return true
}

func decodeXQABoundObservation(raw []byte) (xQABoundObservation, error) {
	var report xQABoundObservation
	bad := errors.New("QA bound observation report rejected")
	if len(raw) == 0 || len(raw) > 16384 || !xQABoundStrictJSON(raw) || !xQABoundExactFields(raw,
		"schema", "sourceQualification", "diagnostic", "identity", "login", "binding", "freshness", "bootstrap", "source", "cleanupVerified", "startupCookieWriteObserved", "enrollment", "continuity", "canAuthorize", "sessionTransferred") {
		return report, bad
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF ||
		report.Schema != "aku.x.qa-bound-observation.v1" || report.SourceQualification != "api_v1.cookie-auth-rejection.2026-10-20" ||
		!xQAFalse(report.CanAuthorize) || !xQAFalse(report.SessionTransferred) || report.CleanupVerified == nil || report.StartupCookieWriteObserved == nil ||
		report.Enrollment != "not_verified" || report.Continuity != "unknown" {
		return report, bad
	}
	if report.Diagnostic != "bound" && report.Diagnostic != "scope_lost" && report.Diagnostic != "observed_cookie_write" &&
		report.Diagnostic != "stale_observation" && report.Diagnostic != "cleanup_unverified" &&
		report.Diagnostic != "identity_unavailable" && report.Diagnostic != "source_unavailable" {
		return report, bad
	}
	bootstrapObserved := false
	if !bytes.Equal(bytes.TrimSpace(report.Bootstrap), []byte("null")) {
		bootstrap, err := decodeXBootstrapCurrentSelf(report.Bootstrap)
		if err != nil {
			return report, bad
		}
		bootstrapObserved = xQABoundIdentityObserved(&bootstrap)
	}
	if (report.Identity == "current_self_observed") != bootstrapObserved || report.Identity != "current_self_observed" && report.Identity != "unknown" {
		return report, bad
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil || !xQABoundValidateSource(top["source"], report.Source) {
		return report, bad
	}
	loginObserved := xQABoundLoginObserved(report.Source)
	if (report.Login == "protected_response_observed") != loginObserved || report.Login != "protected_response_observed" && report.Login != "unknown" {
		return report, bad
	}
	if report.Freshness != "within_5s" && report.Freshness != "unknown" {
		return report, bad
	}
	if report.Freshness == "within_5s" && (!bootstrapObserved || report.Source.MatchingRequests < 1 || !xQATrue(report.Source.Completed)) {
		return report, bad
	}
	expectedBinding := "unknown"
	if report.Diagnostic == "bound" {
		if !*report.CleanupVerified || !bootstrapObserved || !loginObserved || report.Freshness != "within_5s" {
			return report, bad
		}
		expectedBinding = "same_document_observed"
	} else {
		switch report.Diagnostic {
		case "cleanup_unverified":
			if *report.CleanupVerified {
				return report, bad
			}
		case "identity_unavailable":
			if !*report.CleanupVerified || bootstrapObserved || report.Freshness != "unknown" {
				return report, bad
			}
		case "source_unavailable":
			if !*report.CleanupVerified || !bootstrapObserved || loginObserved {
				return report, bad
			}
		}
	}
	if report.Binding != expectedBinding {
		return report, bad
	}
	return report, nil
}

func xQABoundBootstrapJSON() json.RawMessage {
	return json.RawMessage(`{"schema":"aku.x.bootstrap-current-self.v1","diagnostic":"projected","bootstrapState":"object","sessionState":"object","idFields":{"user_id":"string","user_id_str":"absent"},"stableIdStringCandidates":1,"sessionIdsAgree":null,"authFields":{"loggedIn":"absent","isAuthenticated":"absent","authenticated":"absent","isActive":"true"},"entityChecks":{"normalizedUsers":"match","rootUsers":"absent"},"canAuthorize":false,"continuityProven":false}`)
}

func xQABoundSourceJSON(status int, diagnostic, category string, completed, fresh bool, attempts int) *xQADirectResponse {
	f := false
	response := &xQADirectResponse{Schema: "aku.x.qa-direct-response.v1", Route: "api_v1", Role: "a", Diagnostic: diagnostic,
		Completed: &completed, CanAuthorize: &f, SessionTransferred: &f}
	if diagnostic == "not_observed" {
		return response
	}
	response.MatchingRequests = 1
	response.Status = &status
	response.ResponseFresh = &fresh
	response.BodyReadAttempts = attempts
	if category != "" {
		settingsMatch := category == "settings_match"
		response.Body = &xQADirectBody{Schema: "aku.x.qa-direct-body.v1", Category: category, SettingsMatch: &settingsMatch}
	}
	return response
}

func xQABoundReportFixture(source *xQADirectResponse, bootstrap json.RawMessage, diagnostic, identity, login, binding, freshness string, cleanup bool) xQABoundObservation {
	f, cleanupValue, startupCookieWriteObserved := false, cleanup, false
	return xQABoundObservation{Schema: "aku.x.qa-bound-observation.v1", SourceQualification: "api_v1.cookie-auth-rejection.2026-10-20",
		Diagnostic: diagnostic, Identity: identity, Login: login, Binding: binding, Freshness: freshness, Bootstrap: bootstrap, Source: source,
		CleanupVerified: &cleanupValue, StartupCookieWriteObserved: &startupCookieWriteObserved,
		Enrollment: "not_verified", Continuity: "unknown", CanAuthorize: &f, SessionTransferred: &f}
}

func TestXQABoundObservationOffline(t *testing.T) {
	positive := xQABoundReportFixture(xQABoundSourceJSON(200, "http_200_unqualified", "settings_match", true, true, 1),
		xQABoundBootstrapJSON(), "bound", "current_self_observed", "protected_response_observed", "same_document_observed", "within_5s", true)
	raw, err := json.Marshal(positive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeXQABoundObservation(raw); err != nil {
		t.Fatalf("valid bound observation rejected: %v", err)
	}
	earlyWrite := positive
	earlyWrite.StartupCookieWriteObserved = func() *bool { value := true; return &value }()
	earlyWriteRaw, _ := json.Marshal(earlyWrite)
	earlyWriteDecoded, err := decodeXQABoundObservation(earlyWriteRaw)
	if err != nil || earlyWriteDecoded.Binding != "same_document_observed" || earlyWriteDecoded.CanAuthorize == nil || *earlyWriteDecoded.CanAuthorize {
		t.Fatal("an observed startup cookie write changed the bound evidence claims")
	}
	var withoutStartupField map[string]json.RawMessage
	_ = json.Unmarshal(raw, &withoutStartupField)
	delete(withoutStartupField, "startupCookieWriteObserved")
	missingStartupField, _ := json.Marshal(withoutStartupField)
	if _, err := decodeXQABoundObservation(missingStartupField); err == nil {
		t.Fatal("missing startup cookie observation field accepted")
	}
	wrongStartupField := bytes.Replace(raw, []byte(`"startupCookieWriteObserved":false`), []byte(`"startupCookieWriteObserved":"yes"`), 1)
	if _, err := decodeXQABoundObservation(wrongStartupField); err == nil {
		t.Fatal("nonboolean startup cookie observation field accepted")
	}

	partial := xQABoundReportFixture(xQABoundSourceJSON(0, "not_observed", "", false, false, 0), nil,
		"identity_unavailable", "unknown", "unknown", "unknown", "unknown", true)
	partial.Bootstrap = json.RawMessage("null")
	partialRaw, _ := json.Marshal(partial)
	if _, err := decodeXQABoundObservation(partialRaw); err != nil {
		t.Fatalf("valid partial observation rejected: %v", err)
	}

	failedSource := xQABoundReportFixture(xQABoundSourceJSON(401, "authentication_rejection_candidate", "error_envelope", true, true, 1),
		xQABoundBootstrapJSON(), "source_unavailable", "current_self_observed", "unknown", "unknown", "within_5s", true)
	failedSourceRaw, _ := json.Marshal(failedSource)
	if _, err := decodeXQABoundObservation(failedSourceRaw); err != nil {
		t.Fatalf("valid failed-source observation rejected: %v", err)
	}
	ambiguous := xQABoundReportFixture(&xQADirectResponse{Schema: "aku.x.qa-direct-response.v1", Route: "api_v1", Role: "a",
		Diagnostic: "candidate_rejected", MatchingRequests: 2, Completed: func() *bool { value := true; return &value }(),
		CanAuthorize: func() *bool { value := false; return &value }(), SessionTransferred: func() *bool { value := false; return &value }()},
		xQABoundBootstrapJSON(), "source_unavailable", "current_self_observed", "unknown", "unknown", "within_5s", true)
	ambiguousRaw, _ := json.Marshal(ambiguous)
	if _, err := decodeXQABoundObservation(ambiguousRaw); err != nil {
		t.Fatalf("valid ambiguous partial observation rejected: %v", err)
	}

	for _, mutation := range []struct {
		name string
		from []byte
		to   []byte
	}{
		{"bound diagnostic contradicted by source", []byte(`"diagnostic":"bound"`), []byte(`"diagnostic":"source_unavailable"`)},
		{"identity claim without bootstrap", []byte(`"bootstrap":`), []byte(`"bootstrap":`)},
		{"binding claim after scope loss", []byte(`"diagnostic":"bound"`), []byte(`"diagnostic":"scope_lost"`)},
		{"authorization claim", []byte(`"canAuthorize":false`), []byte(`"canAuthorize":true`)},
		{"session transfer claim", []byte(`"sessionTransferred":false`), []byte(`"sessionTransferred":true`)},
		{"unknown reflected root field", []byte(`"continuity":"unknown"`), []byte(`"continuity":"unknown","accountId":"123456789"`)},
		{"duplicate root key", []byte(`"identity":"current_self_observed"`), []byte(`"identity":"current_self_observed","identity":"unknown"`)},
	} {
		if mutation.name == "identity claim without bootstrap" {
			bad := bytes.Replace(partialRaw, []byte(`"identity":"unknown"`), []byte(`"identity":"current_self_observed"`), 1)
			if _, err := decodeXQABoundObservation(bad); err == nil {
				t.Fatal("identity without bootstrap evidence accepted")
			}
			continue
		}
		bad := bytes.Replace(raw, mutation.from, mutation.to, 1)
		if _, err := decodeXQABoundObservation(bad); err == nil {
			t.Fatalf("%s accepted", mutation.name)
		}
	}

	for _, bad := range [][]byte{
		bytes.Replace(raw, []byte(`"normalizedUsers":"match"`), []byte(`"normalizedUsers":"match","accountId":"123456789"`), 1),
		bytes.Replace(raw, []byte(`"user_id":"string"`), []byte(`"user_id":"string","user_id":"string"`), 1),
		bytes.Replace(raw, []byte(`"user_id":"string"`), []byte(`"user_id":"123456789"`), 1),
		bytes.Replace(raw, []byte(`"role":"a"`), []byte(`"role":"a","cookie":"PRIVATE"`), 1),
		bytes.Replace(raw, []byte(`"category":"settings_match"`), []byte(`"category":"settings_match","value":"PRIVATE"`), 1),
		bytes.Replace(raw, []byte(`"category":"settings_match"`), []byte(`"category":"settings_match","category":"error_envelope"`), 1),
		append(append([]byte(nil), raw...), []byte(` {}`)...),
	} {
		if _, err := decodeXQABoundObservation(bad); err == nil {
			t.Fatal("malformed, duplicate, unknown or reflected nested evidence accepted")
		}
	}
}
