//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xQAWebHook struct {
	Schema     string `json:"schema"`
	Candidates int    `json:"candidates"`
	Transport  string `json:"transport"`
	Prepared   *bool  `json:"prepared"`
	Attempted  *bool  `json:"attempted"`
	Outcome    string `json:"outcome"`
	Disposed   *bool  `json:"disposed"`
	Rejected   *bool  `json:"rejected"`
}
type xQAWebABA struct {
	Schema             string             `json:"schema"`
	A                  *xQADirectResponse `json:"a"`
	B                  *xQADirectResponse `json:"b"`
	AAgain             *xQADirectResponse `json:"aAgain"`
	Hook               *xQAWebHook        `json:"hook"`
	OwnerHomeBefore    *bool              `json:"ownerHomeBefore"`
	OwnerHomeAfter     *bool              `json:"ownerHomeAfter"`
	HookRemoved        *bool              `json:"hookRemoved"`
	Outcome            string             `json:"outcome"`
	Authentication     string             `json:"authentication"`
	CanAuthorize       *bool              `json:"canAuthorize"`
	SessionTransferred *bool              `json:"sessionTransferred"`
}

func xQAWebPositive(r *xQADirectResponse) bool {
	return r != nil && r.Status != nil && *r.Status == 200 && xQATrue(r.ResponseFresh) && xQATrue(r.Completed) &&
		r.Diagnostic == "http_200_unqualified" && r.Body != nil && r.Body.Category == "settings_match"
}
func decodeXQAWebABA(raw []byte) (xQAWebABA, error) {
	var r xQAWebABA
	bad := errors.New("QA web A/B/A report rejected")
	if len(raw) > 16384 || !xQAFields(raw, 12) {
		return r, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&r) != nil || d.Decode(new(any)) != io.EOF || r.Schema != "aku.x.qa-web-aba.v1" ||
		!xQATrue(r.OwnerHomeBefore) || r.OwnerHomeAfter == nil || !xQATrue(r.HookRemoved) ||
		r.Authentication != "unknown" || !xQAFalse(r.CanAuthorize) || !xQAFalse(r.SessionTransferred) {
		return r, bad
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	truth, falsity := true, false
	dummy := func(role string) *xQADirectResponse {
		return &xQADirectResponse{Schema: "aku.x.qa-direct-response.v1", Route: "api_v1", Role: role,
			Diagnostic: "not_observed", Completed: &falsity, CanAuthorize: &falsity, SessionTransferred: &falsity}
	}
	for _, phase := range []struct {
		key, role string
		value     *xQADirectResponse
	}{{"a", "a", r.A}, {"b", "b", r.B}, {"aAgain", "a", r.AAgain}} {
		if phase.key == "b" && phase.value == nil {
			continue
		}
		if phase.value == nil || phase.value.Role != phase.role || !xQAFields(fields[phase.key], 12) {
			return r, bad
		}
		// Reuse the already independent phase decoder, not the JavaScript claim.
		shim := xQADirectAB{Schema: "aku.x.qa-direct-ab.v1", Navigation: "direct_api_v1", A: dummy("a"), B: dummy("b"),
			OwnerHomeBefore: &truth, OwnerHomeAfter: &truth, TabsDisposed: &truth, GuestContextDisposed: &truth,
			Outcome: "positive_control_failed", Authentication: "unknown", CanAuthorize: &falsity, SessionTransferred: &falsity}
		if phase.role == "a" {
			shim.A = phase.value
			if xQAWebPositive(phase.value) {
				shim.Outcome = "comparison_inconclusive"
			}
		} else {
			shim.B = phase.value
		}
		encoded, _ := json.Marshal(shim)
		if _, err := decodeXQADirectAB(encoded); err != nil {
			return r, bad
		}
	}
	h := r.Hook
	if h == nil || !xQAFields(fields["hook"], 8) || h.Schema != "aku.x.qa-web-hook.v1" || h.Candidates < 0 || h.Candidates > 2 ||
		!xQAFalse(h.Prepared) || h.Attempted == nil || !xQATrue(h.Disposed) || h.Rejected == nil {
		return r, bad
	}
	if h.Transport != "none" && h.Transport != "fetch" && h.Transport != "xhr" {
		return r, bad
	}
	switch h.Outcome {
	case "not_run", "started", "completed", "network_error", "body_limit", "rejected":
	default:
		return r, bad
	}
	if r.B != nil && (!*h.Attempted || h.Candidates != 1 || h.Transport == "none" || *h.Rejected) || r.B == nil && *h.Attempted {
		return r, bad
	}
	expected := "positive_control_failed"
	if *r.OwnerHomeAfter && xQAWebPositive(r.A) && xQAWebPositive(r.AAgain) {
		expected = "comparison_not_run"
		if b := r.B; b != nil {
			expected = "comparison_inconclusive"
			if b.Status != nil && *b.Status == 401 && b.Diagnostic == "authentication_rejection_candidate" && b.Body != nil && b.Body.Category == "error_envelope" {
				expected = "cookie_dependency_candidate"
			} else if xQAWebPositive(b) {
				expected = "settings_without_cookie"
			}
		}
	}
	if r.Outcome != expected {
		return r, bad
	}
	return r, nil
}

func TestXQAWebABAOffline(t *testing.T) {
	f, v, status := false, true, 200
	positive := func() *xQADirectResponse {
		return &xQADirectResponse{Schema: "aku.x.qa-direct-response.v1", Route: "api_v1", Role: "a",
			Diagnostic: "http_200_unqualified", MatchingRequests: 1, Status: &status, ResponseFresh: &v, Completed: &v, BodyReadAttempts: 1,
			Body: &xQADirectBody{Schema: "aku.x.qa-direct-body.v1", Category: "settings_match", SettingsMatch: &v}, CanAuthorize: &f, SessionTransferred: &f}
	}
	r := xQAWebABA{Schema: "aku.x.qa-web-aba.v1", A: positive(), AAgain: positive(),
		Hook:            &xQAWebHook{Schema: "aku.x.qa-web-hook.v1", Candidates: 1, Transport: "fetch", Prepared: &f, Attempted: &f, Outcome: "not_run", Disposed: &v, Rejected: &f},
		OwnerHomeBefore: &v, OwnerHomeAfter: &v, HookRemoved: &v, Outcome: "comparison_not_run", Authentication: "unknown", CanAuthorize: &f, SessionTransferred: &f}
	raw, _ := json.Marshal(r)
	if _, err := decodeXQAWebABA(raw); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{
		{`"hookRemoved":true`, `"hookRemoved":false`}, {`"authentication":"unknown"`, `"authentication":"authenticated"`},
		{`"outcome":"comparison_not_run"`, `"outcome":"cookie_dependency_candidate"`},
		{`"candidates":1`, `"candidates":3`}, {`"transport":"fetch"`, `"transport":"PRIVATE"`},
		{`"prepared":false`, `"prepared":true`}, {`"disposed":true`, `"disposed":false`},
		{`"hook":{`, `"hook":{"headers":"PRIVATE",`}, {`"bodyReadAttempts":1`, `"bodyReadAttempts":2`},
	} {
		if _, err := decodeXQAWebABA(bytes.Replace(raw, []byte(change[0]), []byte(change[1]), 1)); err == nil {
			t.Fatal("unsafe web control accepted")
		}
	}
	negative := *positive()
	negative.Role = "b"
	negativeStatus := 401
	negative.Status = &negativeStatus
	negative.Diagnostic = "authentication_rejection_candidate"
	negative.Body = &xQADirectBody{Schema: "aku.x.qa-direct-body.v1", Category: "error_envelope", SettingsMatch: &f}
	r.B = &negative
	r.Hook.Attempted = &v
	r.Hook.Outcome = "completed"
	r.Outcome = "cookie_dependency_candidate"
	raw, _ = json.Marshal(r)
	if _, err := decodeXQAWebABA(raw); err != nil {
		t.Fatal(err)
	}
}
