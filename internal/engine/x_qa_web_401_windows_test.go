//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xQARejection struct {
	Schema         string `json:"schema"`
	Shape          string `json:"shape"`
	CodeClass      string `json:"codeClass"`
	MessageMatch   *bool  `json:"messageMatch"`
	Classification string `json:"classification"`
}
type xQAWeb401 struct {
	xQAWebABA
	Rejection *xQARejection `json:"rejection"`
}

func decodeXQAWeb401(raw []byte) (xQAWeb401, error) {
	var r xQAWeb401
	bad := errors.New("QA bounded rejection report rejected")
	if len(raw) > 16384 || !xQAFields(raw, 13) {
		return r, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Schema != "aku.x.qa-web-aba.v2" {
		return r, bad
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	delete(fields, "rejection")
	fields["schema"] = json.RawMessage(`"aku.x.qa-web-aba.v1"`)
	if r.Outcome == "cookie_auth_rejection_observed" {
		fields["outcome"] = json.RawMessage(`"cookie_dependency_candidate"`)
	}
	baseRaw, _ := json.Marshal(fields)
	base, err := decodeXQAWebABA(baseRaw)
	if err != nil {
		return r, bad
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	if r.Rejection == nil {
		if r.B != nil && r.B.Body != nil {
			return r, bad
		}
	} else {
		p := r.Rejection
		if !xQAFields(root["rejection"], 5) || p.Schema != "aku.x.qa-rejection.v1" || r.B == nil || r.B.Body == nil {
			return r, bad
		}
		switch p.Shape {
		case "single_error":
			if p.MessageMatch == nil || p.CodeClass != "code_32" && p.CodeClass != "other" || p.CodeClass == "other" && *p.MessageMatch {
				return r, bad
			}
		case "other_json", "unreadable":
			if p.CodeClass != "none" || p.MessageMatch != nil {
				return r, bad
			}
		default:
			return r, bad
		}
		expected := "unknown"
		if p.Shape == "single_error" && p.CodeClass == "code_32" && xQATrue(p.MessageMatch) {
			expected = "provider_authentication_error"
		}
		if p.Classification != expected || expected == "provider_authentication_error" && r.B.Body.Category != "error_envelope" {
			return r, bad
		}
	}
	expected := base.Outcome
	if base.Outcome == "cookie_dependency_candidate" && base.Hook.Outcome == "completed" &&
		r.Rejection != nil && r.Rejection.Classification == "provider_authentication_error" {
		expected = "cookie_auth_rejection_observed"
	}
	if r.Outcome != expected {
		return r, bad
	}
	return r, nil
}

func TestXQAWeb401Offline(t *testing.T) {
	f, v, success, unauthorized := false, true, 200, 401
	phase := func(role string, status *int, diagnostic, category string, match *bool) *xQADirectResponse {
		return &xQADirectResponse{Schema: "aku.x.qa-direct-response.v1", Route: "api_v1", Role: role, Diagnostic: diagnostic, MatchingRequests: 1,
			Status: status, ResponseFresh: &v, Completed: &v, BodyReadAttempts: 1, Body: &xQADirectBody{Schema: "aku.x.qa-direct-body.v1", Category: category, SettingsMatch: match}, CanAuthorize: &f, SessionTransferred: &f}
	}
	r := xQAWeb401{xQAWebABA: xQAWebABA{Schema: "aku.x.qa-web-aba.v2",
		A: phase("a", &success, "http_200_unqualified", "settings_match", &v), B: phase("b", &unauthorized, "authentication_rejection_candidate", "error_envelope", &f),
		AAgain:          phase("a", &success, "http_200_unqualified", "settings_match", &v),
		Hook:            &xQAWebHook{Schema: "aku.x.qa-web-hook.v1", Candidates: 1, Transport: "xhr", Prepared: &f, Attempted: &v, Outcome: "completed", Disposed: &v, Rejected: &f},
		OwnerHomeBefore: &v, OwnerHomeAfter: &v, HookRemoved: &v, Outcome: "cookie_auth_rejection_observed", Authentication: "unknown", CanAuthorize: &f, SessionTransferred: &f},
		Rejection: &xQARejection{Schema: "aku.x.qa-rejection.v1", Shape: "single_error", CodeClass: "code_32", MessageMatch: &v, Classification: "provider_authentication_error"}}
	raw, _ := json.Marshal(r)
	if _, err := decodeXQAWeb401(raw); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{
		{`"schema":"aku.x.qa-web-aba.v2"`, `"schema":"aku.x.qa-web-aba.v1"`},
		{`"rejection":{`, `"rejection":{"message":"PRIVATE",`}, {`"codeClass":"code_32"`, `"codeClass":"353"`},
		{`"codeClass":"code_32"`, `"codeClass":"other"`}, {`"messageMatch":true`, `"messageMatch":false`},
		{`"shape":"single_error"`, `"shape":"other_json"`}, {`"rejection":{`, `"rejection":{"headers":{},`},
		{`"hookRemoved":true`, `"hookRemoved":false`}, {`"authentication":"unknown"`, `"authentication":"authenticated"`},
	} {
		if _, err := decodeXQAWeb401(bytes.Replace(raw, []byte(change[0]), []byte(change[1]), 1)); err == nil {
			t.Fatal("unsafe rejection report accepted")
		}
	}
	r.Rejection.CodeClass = "other"
	r.Rejection.MessageMatch = &f
	r.Rejection.Classification = "unknown"
	r.Outcome = "cookie_dependency_candidate"
	raw, _ = json.Marshal(r)
	if _, err := decodeXQAWeb401(raw); err != nil {
		t.Fatal("unknown classification rejected")
	}
	// A failed bracket cannot be promoted even when the provider error matches.
	r.Rejection.CodeClass = "code_32"
	r.Rejection.MessageMatch = &v
	r.Rejection.Classification = "provider_authentication_error"
	r.AAgain.Status = &unauthorized
	r.AAgain.Diagnostic = "authentication_rejection_candidate"
	r.AAgain.Body = r.B.Body
	r.Outcome = "positive_control_failed"
	raw, _ = json.Marshal(r)
	if _, err := decodeXQAWeb401(raw); err != nil {
		t.Fatal(err)
	}
	bad := bytes.Replace(raw, []byte(`"outcome":"positive_control_failed"`), []byte(`"outcome":"cookie_auth_rejection_observed"`), 1)
	if _, err := decodeXQAWeb401(bad); err == nil {
		t.Fatal("failed bracket promoted by a provider error alone")
	}
}
