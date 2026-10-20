//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xQADirectBody struct {
	Schema        string `json:"schema"`
	Category      string `json:"category"`
	SettingsMatch *bool  `json:"settingsMatch"`
}
type xQADirectResponse struct {
	Schema             string         `json:"schema"`
	Route              string         `json:"route"`
	Role               string         `json:"role"`
	Diagnostic         string         `json:"diagnostic"`
	MatchingRequests   int            `json:"matchingRequests"`
	Status             *int           `json:"status"`
	ResponseFresh      *bool          `json:"responseFresh"`
	Completed          *bool          `json:"completed"`
	BodyReadAttempts   int            `json:"bodyReadAttempts"`
	Body               *xQADirectBody `json:"body"`
	CanAuthorize       *bool          `json:"canAuthorize"`
	SessionTransferred *bool          `json:"sessionTransferred"`
}
type xQADirectAB struct {
	Schema               string             `json:"schema"`
	Navigation           string             `json:"navigation"`
	A                    *xQADirectResponse `json:"a"`
	B                    *xQADirectResponse `json:"b"`
	OwnerHomeBefore      *bool              `json:"ownerHomeBefore"`
	OwnerHomeAfter       *bool              `json:"ownerHomeAfter"`
	TabsDisposed         *bool              `json:"tabsDisposed"`
	GuestContextDisposed *bool              `json:"guestContextDisposed"`
	Outcome              string             `json:"outcome"`
	Authentication       string             `json:"authentication"`
	CanAuthorize         *bool              `json:"canAuthorize"`
	SessionTransferred   *bool              `json:"sessionTransferred"`
}

func decodeXQADirectAB(raw []byte) (xQADirectAB, error) {
	var r xQADirectAB
	bad := errors.New("QA direct A/B report rejected")
	if len(raw) > 16384 || !xQAFields(raw, 12) {
		return r, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Schema != "aku.x.qa-direct-ab.v1" || r.Navigation != "direct_api_v1" ||
		!xQATrue(r.OwnerHomeBefore) || r.OwnerHomeAfter == nil || !xQATrue(r.TabsDisposed) || !xQATrue(r.GuestContextDisposed) ||
		r.Authentication != "unknown" || !xQAFalse(r.CanAuthorize) || !xQAFalse(r.SessionTransferred) {
		return r, bad
	}
	var root map[string]json.RawMessage
	_ = json.Unmarshal(raw, &root)
	for role, entry := range map[string]*xQADirectResponse{"a": r.A, "b": r.B} {
		if entry == nil || !xQAFields(root[role], 12) || entry.Schema != "aku.x.qa-direct-response.v1" || entry.Role != role || entry.Route != "api_v1" ||
			entry.MatchingRequests < 0 || entry.MatchingRequests > 2 || entry.Completed == nil || entry.BodyReadAttempts < 0 || entry.BodyReadAttempts > 1 ||
			!xQAFalse(entry.CanAuthorize) || !xQAFalse(entry.SessionTransferred) || entry.Status != nil && (*entry.Status < 100 || *entry.Status > 599) {
			return r, bad
		}
		switch entry.Diagnostic {
		case "candidate_rejected":
			if entry.Body != nil {
				return r, bad
			}
		case "not_observed":
			if entry.MatchingRequests != 0 || entry.Status != nil || entry.ResponseFresh != nil || *entry.Completed || entry.BodyReadAttempts != 0 {
				return r, bad
			}
		case "incomplete":
			if entry.MatchingRequests != 1 || *entry.Completed || entry.BodyReadAttempts != 0 {
				return r, bad
			}
		case "http_200_unqualified", "authentication_rejection_candidate", "other_response":
			if entry.MatchingRequests != 1 || !*entry.Completed || !xQATrue(entry.ResponseFresh) || entry.Status == nil {
				return r, bad
			}
			expected := "other_response"
			if *entry.Status == 200 {
				expected = "http_200_unqualified"
			}
			if *entry.Status == 401 {
				expected = "authentication_rejection_candidate"
			}
			if entry.Diagnostic != expected {
				return r, bad
			}
		default:
			return r, bad
		}
		if body := entry.Body; body != nil {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(root[role], &fields)
			if !xQAFields(fields["body"], 3) || body.Schema != "aku.x.qa-direct-body.v1" || body.SettingsMatch == nil ||
				entry.BodyReadAttempts != 1 || !*entry.Completed || !xQATrue(entry.ResponseFresh) || entry.MatchingRequests != 1 || entry.Status == nil {
				return r, bad
			}
			if *entry.Status != 200 && *entry.Status != 401 && *entry.Status != 403 && *entry.Status != 429 {
				return r, bad
			}
			switch body.Category {
			case "settings_match", "error_envelope", "challenge", "other_json", "unreadable":
			default:
				return r, bad
			}
			if *body.SettingsMatch != (body.Category == "settings_match") {
				return r, bad
			}
		}
	}
	a, b := r.A, r.B
	positive := *r.OwnerHomeAfter && a.Status != nil && *a.Status == 200 && a.Diagnostic == "http_200_unqualified" && a.Body != nil && a.Body.Category == "settings_match"
	negative := b.Status != nil && *b.Status == 401 && b.Diagnostic == "authentication_rejection_candidate" && b.Body != nil && b.Body.Category == "error_envelope"
	expected := "positive_control_failed"
	if positive {
		expected = "comparison_inconclusive"
		if negative {
			expected = "negative_response_candidate"
		} else if b.Status != nil && *b.Status == 200 && b.Body != nil && b.Body.Category == "settings_match" {
			expected = "guest_settings_match"
		}
	}
	if r.Outcome != expected {
		return r, bad
	}
	return r, nil
}

func TestXQADirectABOffline(t *testing.T) {
	response := func(role string) map[string]any {
		return map[string]any{"schema": "aku.x.qa-direct-response.v1", "route": "api_v1", "role": role,
			"diagnostic": "authentication_rejection_candidate", "matchingRequests": 1, "status": 401, "responseFresh": true, "completed": true,
			"bodyReadAttempts": 1, "body": map[string]any{"schema": "aku.x.qa-direct-body.v1", "category": "error_envelope", "settingsMatch": false},
			"canAuthorize": false, "sessionTransferred": false}
	}
	r := map[string]any{"schema": "aku.x.qa-direct-ab.v1", "navigation": "direct_api_v1", "a": response("a"), "b": response("b"),
		"ownerHomeBefore": true, "ownerHomeAfter": true, "tabsDisposed": true, "guestContextDisposed": true, "outcome": "positive_control_failed",
		"authentication": "unknown", "canAuthorize": false, "sessionTransferred": false}
	raw, _ := json.Marshal(r)
	if _, err := decodeXQADirectAB(raw); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{
		{`"outcome":"positive_control_failed"`, `"outcome":"negative_response_candidate"`},
		{`"authentication":"unknown"`, `"authentication":"authenticated"`},
		{`"guestContextDisposed":true`, `"guestContextDisposed":false`}, {`"tabsDisposed":true`, `"tabsDisposed":false`},
		{`"responseFresh":true`, `"responseFresh":false`}, {`"bodyReadAttempts":1`, `"bodyReadAttempts":2`},
		{`"settingsMatch":false`, `"settingsMatch":true`}, {`"category":"error_envelope"`, `"category":"PRIVATE"`},
		{`"role":"a"`, `"role":"b"`}, {`"route":"api_v1"`, `"route":"api_i_api"`},
		{`"body":{`, `"body":{"value":"PRIVATE",`},
	} {
		if _, err := decodeXQADirectAB(bytes.Replace(raw, []byte(change[0]), []byte(change[1]), 1)); err == nil {
			t.Fatal("unsafe direct comparison accepted")
		}
	}
	a := r["a"].(map[string]any)
	a["status"] = 200
	a["diagnostic"] = "http_200_unqualified"
	a["body"] = map[string]any{"schema": "aku.x.qa-direct-body.v1", "category": "settings_match", "settingsMatch": true}
	r["outcome"] = "negative_response_candidate"
	raw, _ = json.Marshal(r)
	if _, err := decodeXQADirectAB(raw); err != nil {
		t.Fatal(err)
	}
}
