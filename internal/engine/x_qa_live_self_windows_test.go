//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

const (
	xQALiveHTMLMax        = 512 * 1024
	xQALiveInitializerMax = 256 * 1024
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
	fields, ok := xQAShortDecodeExact(raw, 4096, []string{"schema", "source", "outcome", "observation", "transport", "elapsedMS", "canAuthorize", "nativeQualified", "sessionTransferred"}, &report)
	if !ok || report.Schema != "aku.x.qa-live-self-run.v3" || report.Source != "home_document_fetch.v3" ||
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
		ParserRejection    *string         `json:"parserRejection"`
		Identity           string          `json:"identity"`
		BodyBytes          int             `json:"bodyBytes"`
		InitializerBytes   int             `json:"initializerBytes"`
		Status             *int            `json:"status"`
		Projection         json.RawMessage `json:"projection"`
		Freshness          string          `json:"freshness"`
		CanAuthorize       bool            `json:"canAuthorize"`
		NativeQualified    bool            `json:"nativeQualified"`
		SessionTransferred bool            `json:"sessionTransferred"`
	}
	obs, ok := xQAShortDecodeExact(report.Observation, 4096, []string{"schema", "outcome", "parserRejection", "identity", "bodyBytes", "initializerBytes", "status", "projection", "freshness", "canAuthorize", "nativeQualified", "sessionTransferred"}, &observation)
	if !ok || observation.Schema != "aku.x.qa-live-self.v3" {
		return report, invalid
	}
	if observation.Outcome == "parser_rejected" {
		if observation.ParserRejection == nil {
			return report, invalid
		}
		switch *observation.ParserRejection {
		case "malformed_json", "duplicate_key", "invalid_unicode", "unsafe_number", "depth_limit", "value_limit", "parser_internal", "projection_failed":
		default:
			return report, invalid
		}
	} else if !bytes.Equal(obs["parserRejection"], []byte("null")) {
		return report, invalid
	}
	for _, key := range []string{"canAuthorize", "nativeQualified", "sessionTransferred"} {
		if !bytes.Equal(obs[key], []byte("false")) {
			return report, invalid
		}
	}
	if _, ok := xQAShortJSONInteger(obs["bodyBytes"]); !ok || observation.BodyBytes < 0 || observation.BodyBytes > xQALiveHTMLMax+1 {
		return report, invalid
	}
	if _, ok := xQAShortJSONInteger(obs["initializerBytes"]); !ok || observation.InitializerBytes < 0 || observation.InitializerBytes > xQALiveInitializerMax+1 || observation.InitializerBytes > observation.BodyBytes {
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
		if observation.Identity != "candidate" || observation.Freshness != "server-response-candidate" || observation.Status == nil || *observation.Status != 200 || observation.BodyBytes > xQALiveHTMLMax || observation.InitializerBytes == 0 || observation.InitializerBytes > xQALiveInitializerMax || !strong {
			return report, invalid
		}
	} else {
		if observation.Identity != "unavailable" || observation.Freshness != "unqualified" {
			return report, invalid
		}
		switch observation.Outcome {
		case "self_unqualified":
			if bytes.Equal(observation.Projection, []byte("null")) || strong || observation.InitializerBytes == 0 || observation.InitializerBytes > xQALiveInitializerMax {
				return report, invalid
			}
		case "body_limit":
			if observation.BodyBytes != xQALiveHTMLMax+1 || observation.InitializerBytes != 0 || observation.Status == nil || *observation.Status != 200 {
				return report, invalid
			}
			fallthrough
		case "timeout", "fetch_failed", "response_rejected", "source_absent", "source_ambiguous", "initializer_limit", "parser_rejected", "html_rejected":
			if !bytes.Equal(observation.Projection, []byte("null")) {
				return report, invalid
			}
		default:
			return report, invalid
		}
		if observation.Outcome != "body_limit" && observation.Outcome != "timeout" && observation.BodyBytes > xQALiveHTMLMax {
			return report, invalid
		}
		if observation.Outcome == "source_absent" || observation.Outcome == "source_ambiguous" || observation.Outcome == "parser_rejected" || observation.Outcome == "self_unqualified" || observation.Outcome == "initializer_limit" || observation.Outcome == "html_rejected" {
			if observation.Status == nil || *observation.Status != 200 {
				return report, invalid
			}
		}
		if observation.Outcome == "initializer_limit" && observation.InitializerBytes != xQALiveInitializerMax+1 {
			return report, invalid
		}
		if observation.Outcome != "initializer_limit" && observation.Outcome != "timeout" && observation.InitializerBytes > xQALiveInitializerMax {
			return report, invalid
		}
		if observation.Outcome == "html_rejected" && observation.InitializerBytes != 0 {
			return report, invalid
		}
		if observation.Outcome == "timeout" && (observation.BodyBytes > xQALiveHTMLMax || observation.InitializerBytes > xQALiveInitializerMax) {
			if observation.Status == nil || *observation.Status != 200 || (observation.BodyBytes > xQALiveHTMLMax && observation.InitializerBytes != 0) {
				return report, invalid
			}
		}
	}
	want := "identity_unavailable"
	if observation.Outcome == "body_limit" || observation.Outcome == "initializer_limit" || observation.Outcome == "parser_rejected" {
		want = observation.Outcome
	} else if !report.Transport.DocumentPreserved {
		want = "source_unqualified"
	} else if report.ElapsedMS >= 2000 || observation.Outcome == "timeout" {
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
	const negative = `{"schema":"aku.x.qa-live-self-run.v3","source":"home_document_fetch.v3","outcome":"body_limit","observation":{"schema":"aku.x.qa-live-self.v3","outcome":"body_limit","parserRejection":null,"identity":"unavailable","bodyBytes":524289,"initializerBytes":0,"status":200,"projection":null,"freshness":"unqualified","canAuthorize":false,"nativeQualified":false,"sessionTransferred":false},"transport":{"requests":1,"completed":false,"documentPreserved":true,"responseFresh":false},"elapsedMS":500,"canAuthorize":false,"nativeQualified":false,"sessionTransferred":false}`
	if _, err := decodeXQALiveSelf([]byte(negative)); err != nil {
		t.Fatal("bounded negative source diagnostic rejected")
	}
	for _, reason := range []string{"malformed_json", "duplicate_key", "invalid_unicode", "unsafe_number", "depth_limit", "value_limit", "parser_internal", "projection_failed"} {
		t.Run(reason, func(t *testing.T) {
			var diagnostic map[string]any
			_ = json.Unmarshal([]byte(negative), &diagnostic)
			diagnostic["outcome"] = "parser_rejected"
			observation := diagnostic["observation"].(map[string]any)
			observation["outcome"] = "parser_rejected"
			observation["bodyBytes"] = 1000
			observation["initializerBytes"] = 500
			observation["parserRejection"] = reason
			raw, _ := json.Marshal(diagnostic)
			if _, err := decodeXQALiveSelf(raw); err != nil {
				t.Fatal("closed parser category rejected")
			}
			for _, unsafe := range []any{nil, "", "PRIVATE_RAW_ERROR", 7, map[string]any{"message": "PRIVATE"}, []string{reason}} {
				observation["parserRejection"] = unsafe
				raw, _ = json.Marshal(diagnostic)
				if _, err := decodeXQALiveSelf(raw); err == nil {
					t.Fatal("arbitrary parser category accepted")
				}
			}
			delete(observation, "parserRejection")
			raw, _ = json.Marshal(diagnostic)
			if _, err := decodeXQALiveSelf(raw); err == nil {
				t.Fatal("missing parser category accepted")
			}
		})
	}
	for _, change := range []struct{ before, after string }{
		{`"canAuthorize":false`, `"canAuthorize":true`},
		{`"sessionTransferred":false`, `"sessionTransferred":true`},
		{`"nativeQualified":false`, `"nativeQualified":null`},
		{`"bodyBytes":524289`, `"bodyBytes":524290`},
		{`"projection":null`, `"projection":{"accountID":"private"}`},
		{`"responseFresh":false`, `"responseFresh":true`},
		{`"outcome":"body_limit"`, `"outcome":"fresh_identity_observed"`},
		{`"elapsedMS":500`, `"elapsedMS":null`},
		{`"source":"home_document_fetch.v3"`, `"source":"other"`},
		{`"status":200`, `"status":null`},
		{`"requests":1`, `"requests":1,"requests":1`},
		{`"initializerBytes":0`, `"initializerBytes":262146`},
		{`"schema":"aku.x.qa-live-self-run.v3"`, `"schema":"aku.x.qa-live-self-run.v1"`},
		{`"schema":"aku.x.qa-live-self-run.v3"`, `"schema":"aku.x.qa-live-self-run.v2"`},
		{`"parserRejection":null`, `"parserRejection":"malformed_json"`},
		{`"parserRejection":null`, `"parserRejection":null,"parserRejection":null`},
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
	observation["initializerBytes"] = 500
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
		func() { observation["initializerBytes"] = 0 },
		func() { observation["initializerBytes"] = xQALiveInitializerMax + 1 },
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
	if _, err := decodeXQALiveSelf(append(encoded, bytes.Repeat([]byte(" "), 4097)...)); err == nil {
		t.Fatal("oversized aggregate projection accepted")
	}
	for _, outcome := range []string{"initializer_limit", "parser_rejected"} {
		var diagnostic map[string]any
		_ = json.Unmarshal([]byte(negative), &diagnostic)
		diagnostic["outcome"] = outcome
		observation := diagnostic["observation"].(map[string]any)
		observation["outcome"] = outcome
		if outcome == "parser_rejected" {
			observation["parserRejection"] = "malformed_json"
		}
		observation["bodyBytes"] = xQALiveHTMLMax
		if outcome == "initializer_limit" {
			observation["initializerBytes"] = xQALiveInitializerMax + 1
		} else {
			observation["initializerBytes"] = xQALiveInitializerMax
		}
		raw, _ := json.Marshal(diagnostic)
		if _, err := decodeXQALiveSelf(raw); err != nil {
			t.Fatal("separate initializer/parser diagnostic rejected")
		}
		observation["initializerBytes"] = 0
		raw, _ = json.Marshal(diagnostic)
		if _, err := decodeXQALiveSelf(raw); (outcome == "initializer_limit") != (err != nil) {
			t.Fatal("empty initializer parse rejection or overflow counter classified incorrectly")
		}
	}
	var timedOut map[string]any
	_ = json.Unmarshal([]byte(negative), &timedOut)
	timedOut["outcome"] = "deadline"
	timeoutObservation := timedOut["observation"].(map[string]any)
	timeoutObservation["outcome"] = "timeout"
	timeoutObservation["bodyBytes"] = xQALiveHTMLMax
	timeoutObservation["initializerBytes"] = xQALiveInitializerMax + 1
	timeoutJSON, _ := json.Marshal(timedOut)
	if _, err := decodeXQALiveSelf(timeoutJSON); err != nil {
		t.Fatal("deadline crossing initializer sizing rejected as malformed metadata")
	}
	timeoutObservation["bodyBytes"] = xQALiveHTMLMax + 1
	timeoutObservation["initializerBytes"] = 0
	timeoutJSON, _ = json.Marshal(timedOut)
	if _, err := decodeXQALiveSelf(timeoutJSON); err != nil {
		t.Fatal("deadline crossing body overflow rejected as malformed metadata")
	}
}
