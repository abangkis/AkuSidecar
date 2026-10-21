//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

type xQALiveSelf struct {
	Schema      string          `json:"schema"`
	Source      string          `json:"source"`
	Outcome     string          `json:"outcome"`
	Observation json.RawMessage `json:"observation"`
	Transport   struct {
		Requests          int  `json:"requests"`
		Completed         bool `json:"completed"`
		DocumentPreserved bool `json:"documentPreserved"`
		ResponseFresh     bool `json:"responseFresh"`
	} `json:"transport"`
	ElapsedMS          float64 `json:"elapsedMS"`
	CanAuthorize       bool    `json:"canAuthorize"`
	NativeQualified    bool    `json:"nativeQualified"`
	SessionTransferred bool    `json:"sessionTransferred"`
}

func decodeXQALiveSelf(raw []byte) (xQALiveSelf, error) {
	var report xQALiveSelf
	invalid := errors.New("live current-self projection rejected")
	fields, ok := xQAShortDecodeExact(raw, 8192, []string{"schema", "source", "outcome", "observation", "transport", "elapsedMS", "canAuthorize", "nativeQualified", "sessionTransferred"}, &report)
	if !ok || report.Schema != "aku.x.qa-live-self-run.v1" || report.Source != "home_document_fetch.v1" ||
		!bytes.Equal(fields["canAuthorize"], []byte("false")) || !bytes.Equal(fields["nativeQualified"], []byte("false")) || !bytes.Equal(fields["sessionTransferred"], []byte("false")) {
		return report, invalid
	}
	if _, ok := xQAShortJSONFiniteNumber(fields["elapsedMS"]); !ok || report.ElapsedMS < 0 || report.ElapsedMS > 60000 {
		return report, invalid
	}
	var transport map[string]json.RawMessage
	if !xQABoundExactFields(fields["transport"], "requests", "completed", "documentPreserved", "responseFresh") || json.Unmarshal(fields["transport"], &transport) != nil {
		return report, invalid
	}
	if _, ok := xQAShortJSONInteger(transport["requests"]); !ok || report.Transport.Requests < 0 || report.Transport.Requests > 2 {
		return report, invalid
	}
	for _, key := range []string{"completed", "documentPreserved", "responseFresh"} {
		if !xQAShortJSONBool(transport[key]) {
			return report, invalid
		}
	}
	if report.Transport.ResponseFresh && (report.Transport.Requests != 1 || !report.Transport.Completed || !report.Transport.DocumentPreserved) {
		return report, invalid
	}
	var observation struct {
		Schema             string          `json:"schema"`
		Outcome            string          `json:"outcome"`
		Identity           string          `json:"identity"`
		BodyBytes          int             `json:"bodyBytes"`
		Status             *int            `json:"status"`
		Projection         json.RawMessage `json:"projection"`
		Freshness          string          `json:"freshness"`
		CanAuthorize       bool            `json:"canAuthorize"`
		NativeQualified    bool            `json:"nativeQualified"`
		SessionTransferred bool            `json:"sessionTransferred"`
	}
	obs, ok := xQAShortDecodeExact(report.Observation, 4096, []string{"schema", "outcome", "identity", "bodyBytes", "status", "projection", "freshness", "canAuthorize", "nativeQualified", "sessionTransferred"}, &observation)
	if !ok || observation.Schema != "aku.x.qa-live-self.v1" {
		return report, invalid
	}
	for _, key := range []string{"canAuthorize", "nativeQualified", "sessionTransferred"} {
		if !bytes.Equal(obs[key], []byte("false")) {
			return report, invalid
		}
	}
	if _, ok := xQAShortJSONInteger(obs["bodyBytes"]); !ok || observation.BodyBytes < 0 || observation.BodyBytes > 16385 {
		return report, invalid
	}
	if !bytes.Equal(obs["status"], []byte("null")) {
		value, ok := xQAShortJSONInteger(obs["status"])
		if !ok || value < 100 || value > 599 || observation.Status == nil {
			return report, invalid
		}
	}
	candidate := observation.Outcome == "candidate"
	var projection xBootstrapCurrentSelf
	if !bytes.Equal(observation.Projection, []byte("null")) {
		var err error
		projection, err = decodeXBootstrapCurrentSelf(observation.Projection)
		if err != nil {
			return report, invalid
		}
	}
	strong := projection.Diagnostic == "projected" && projection.BootstrapState == "object" && projection.SessionState == "object" &&
		projection.StableIDStringCandidates > 0 && (projection.SessionIDsAgree == nil || *projection.SessionIDsAgree) &&
		(projection.IDFields.UserID == "string" || projection.IDFields.UserID == "absent") && (projection.IDFields.UserIDString == "string" || projection.IDFields.UserIDString == "absent") &&
		projection.EntityChecks.NormalizedUsers == "match" && (projection.EntityChecks.RootUsers == "match" || projection.EntityChecks.RootUsers == "absent")
	for _, value := range []string{projection.AuthFields.LoggedIn, projection.AuthFields.IsAuthenticated, projection.AuthFields.Authenticated, projection.AuthFields.IsActive} {
		strong = strong && (value == "absent" || value == "true")
	}
	if candidate {
		if observation.Identity != "candidate" || observation.Freshness != "server-response-candidate" || observation.Status == nil || *observation.Status != 200 || observation.BodyBytes > 16384 || !strong {
			return report, invalid
		}
	} else {
		if observation.Identity != "unavailable" || observation.Freshness != "unqualified" {
			return report, invalid
		}
		switch observation.Outcome {
		case "self_unqualified":
			if bytes.Equal(observation.Projection, []byte("null")) || strong {
				return report, invalid
			}
		case "body_limit":
			if observation.BodyBytes != 16385 || observation.Status == nil || *observation.Status != 200 {
				return report, invalid
			}
			fallthrough
		case "timeout", "fetch_failed", "response_rejected", "source_absent", "source_ambiguous", "projection_invalid":
			if !bytes.Equal(observation.Projection, []byte("null")) {
				return report, invalid
			}
		default:
			return report, invalid
		}
		if observation.Outcome != "body_limit" && observation.BodyBytes > 16384 {
			return report, invalid
		}
		if observation.Outcome == "source_absent" || observation.Outcome == "source_ambiguous" || observation.Outcome == "projection_invalid" || observation.Outcome == "self_unqualified" {
			if observation.Status == nil || *observation.Status != 200 {
				return report, invalid
			}
		}
	}
	want := "identity_unavailable"
	if observation.Outcome == "body_limit" {
		want = "body_limit"
	} else if !report.Transport.DocumentPreserved {
		want = "source_unqualified"
	} else if report.ElapsedMS >= 2000 {
		want = "deadline"
	} else if candidate && report.Transport.ResponseFresh {
		want = "fresh_identity_observed"
	}
	if report.Outcome != want {
		return report, invalid
	}
	return report, nil
}

func TestXQALiveSelfOffline(t *testing.T) {
	const negative = `{"schema":"aku.x.qa-live-self-run.v1","source":"home_document_fetch.v1","outcome":"body_limit","observation":{"schema":"aku.x.qa-live-self.v1","outcome":"body_limit","identity":"unavailable","bodyBytes":16385,"status":200,"projection":null,"freshness":"unqualified","canAuthorize":false,"nativeQualified":false,"sessionTransferred":false},"transport":{"requests":1,"completed":false,"documentPreserved":true,"responseFresh":false},"elapsedMS":500,"canAuthorize":false,"nativeQualified":false,"sessionTransferred":false}`
	if _, err := decodeXQALiveSelf([]byte(negative)); err != nil {
		t.Fatal("bounded negative source diagnostic rejected")
	}
	for _, change := range []struct{ before, after string }{
		{`"canAuthorize":false`, `"canAuthorize":true`},
		{`"sessionTransferred":false`, `"sessionTransferred":true`},
		{`"nativeQualified":false`, `"nativeQualified":null`},
		{`"bodyBytes":16385`, `"bodyBytes":16386`},
		{`"projection":null`, `"projection":{"accountID":"private"}`},
		{`"responseFresh":false`, `"responseFresh":true`},
		{`"outcome":"body_limit"`, `"outcome":"fresh_identity_observed"`},
		{`"elapsedMS":500`, `"elapsedMS":null`},
		{`"source":"home_document_fetch.v1"`, `"source":"other"`},
		{`"status":200`, `"status":null`},
		{`"requests":1`, `"requests":1,"requests":1`},
	} {
		if _, err := decodeXQALiveSelf(bytes.Replace([]byte(negative), []byte(change.before), []byte(change.after), 1)); err == nil {
			t.Fatal("unsafe or inconsistent live-self report accepted")
		}
	}
	const projection = `{"schema":"aku.x.bootstrap-current-self.v1","diagnostic":"projected","bootstrapState":"object","sessionState":"object","idFields":{"user_id":"string","user_id_str":"absent"},"stableIdStringCandidates":1,"sessionIdsAgree":null,"authFields":{"loggedIn":"absent","isAuthenticated":"absent","authenticated":"absent","isActive":"true"},"entityChecks":{"normalizedUsers":"match","rootUsers":"absent"},"canAuthorize":false,"continuityProven":false}`
	var positive map[string]any
	_ = json.Unmarshal([]byte(negative), &positive)
	positive["outcome"] = "fresh_identity_observed"
	observation := positive["observation"].(map[string]any)
	observation["outcome"] = "candidate"
	observation["identity"] = "candidate"
	observation["bodyBytes"] = 1024
	observation["freshness"] = "server-response-candidate"
	observation["projection"] = json.RawMessage(projection)
	transport := positive["transport"].(map[string]any)
	transport["completed"] = true
	transport["responseFresh"] = true
	encoded, _ := json.Marshal(positive)
	if _, err := decodeXQALiveSelf(encoded); err != nil {
		t.Fatal("valid non-authorizing fresh candidate rejected")
	}
	for _, mutate := range []func(){
		func() { positive["elapsedMS"] = 2000 },
		func() { transport["documentPreserved"] = false },
		func() { transport["responseFresh"] = false },
		func() { observation["identity"] = "unavailable" },
		func() {
			observation["projection"] = json.RawMessage(bytes.Replace([]byte(projection), []byte(`"normalizedUsers":"match"`), []byte(`"normalizedUsers":"absent"`), 1))
		},
	} {
		_ = json.Unmarshal(encoded, &positive)
		observation = positive["observation"].(map[string]any)
		transport = positive["transport"].(map[string]any)
		mutate()
		bad, _ := json.Marshal(positive)
		if _, err := decodeXQALiveSelf(bad); err == nil {
			t.Fatal("unqualified candidate acquired fresh identity status")
		}
	}
}
