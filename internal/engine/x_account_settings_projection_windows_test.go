//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xAccountSettingsShape struct {
	Schema           string `json:"schema"`
	Diagnostic       string `json:"diagnostic"`
	DecodedBodyBytes *int   `json:"decodedBodyBytes"`
	Fields           struct {
		IDString      string `json:"id_str"`
		ID            string `json:"id"`
		UserID        string `json:"user_id"`
		UserIDString  string `json:"user_id_str"`
		ScreenName    string `json:"screen_name"`
		Authenticated string `json:"authenticated"`
		Error         string `json:"error"`
		Errors        string `json:"errors"`
		Challenge     string `json:"challenge"`
	} `json:"fields"`
	StableIDStringCandidates int    `json:"stableIdStringCandidates"`
	StableIDStringsAgree     *bool  `json:"stableIdStringsAgree"`
	AuthenticatedFlag        string `json:"authenticatedFlag"`
	SettingsFieldCount       int    `json:"settingsFieldCount"`
	CanAuthorize             bool   `json:"canAuthorize"`
}

type xAccountSettingsCapture struct {
	Schema           string                 `json:"schema"`
	Diagnostic       string                 `json:"diagnostic"`
	MatchingRequests int                    `json:"matchingRequests"`
	BodyReadAttempts int                    `json:"bodyReadAttempts"`
	Projection       *xAccountSettingsShape `json:"projection"`
	CanAuthorize     bool                   `json:"canAuthorize"`
}

func decodeXAccountSettingsEnvelope(raw []byte) (xNativeMetadataReport, xAccountSettingsCapture, error) {
	invalid := errors.New("account-settings diagnostic projection rejected")
	var envelope struct {
		Metadata        json.RawMessage `json:"metadata"`
		AccountSettings json.RawMessage `json:"accountSettings"`
	}
	var emptyMetadata xNativeMetadataReport
	var emptyCapture xAccountSettingsCapture
	if len(raw) == 0 || len(raw) > 65536 {
		return emptyMetadata, emptyCapture, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&envelope) != nil || decoder.Decode(new(any)) != io.EOF {
		return emptyMetadata, emptyCapture, invalid
	}
	metadata, err := decodeXNativeMetadata(envelope.Metadata)
	if err != nil {
		return emptyMetadata, emptyCapture, invalid
	}
	var capture xAccountSettingsCapture
	decoder = json.NewDecoder(bytes.NewReader(envelope.AccountSettings))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&capture) != nil || decoder.Decode(new(any)) != io.EOF || capture.Schema != "aku.x.account-settings-capture.v1" || capture.CanAuthorize || capture.MatchingRequests < 0 || capture.MatchingRequests > 2 || capture.BodyReadAttempts < 0 || capture.BodyReadAttempts > 1 {
		return emptyMetadata, emptyCapture, invalid
	}
	switch capture.Diagnostic {
	case "not_observed", "candidate_rejected", "ambiguous", "read_failed", "body_projection_rejected", "projected":
	default:
		return emptyMetadata, emptyCapture, invalid
	}
	if capture.MatchingRequests == 0 && (capture.BodyReadAttempts != 0 || capture.Diagnostic != "not_observed") || capture.Diagnostic == "projected" && (capture.MatchingRequests != 1 || capture.BodyReadAttempts != 1 || capture.Projection == nil) {
		return emptyMetadata, emptyCapture, invalid
	}
	shape := capture.Projection
	if shape == nil {
		if capture.Diagnostic == "projected" || capture.Diagnostic == "body_projection_rejected" {
			return emptyMetadata, emptyCapture, invalid
		}
		return metadata, capture, nil
	}
	if capture.BodyReadAttempts != 1 || capture.Diagnostic != "projected" && capture.Diagnostic != "body_projection_rejected" || shape.Schema != "aku.x.account-settings-shape.v1" || shape.CanAuthorize || shape.StableIDStringCandidates < 0 || shape.StableIDStringCandidates > 4 || shape.SettingsFieldCount < 0 || shape.SettingsFieldCount > 6 || shape.DecodedBodyBytes != nil && (*shape.DecodedBodyBytes < 0 || *shape.DecodedBodyBytes > 16384) || shape.StableIDStringCandidates < 2 && shape.StableIDStringsAgree != nil || shape.StableIDStringCandidates >= 2 && shape.StableIDStringsAgree == nil {
		return emptyMetadata, emptyCapture, invalid
	}
	switch shape.Diagnostic {
	case "projected", "body_result_invalid", "body_oversized", "body_encoding_invalid", "body_json_invalid", "body_root_invalid":
	default:
		return emptyMetadata, emptyCapture, invalid
	}
	if (capture.Diagnostic == "projected") != (shape.Diagnostic == "projected") {
		return emptyMetadata, emptyCapture, invalid
	}
	for _, kind := range []string{shape.Fields.IDString, shape.Fields.ID, shape.Fields.UserID, shape.Fields.UserIDString, shape.Fields.ScreenName, shape.Fields.Authenticated, shape.Fields.Error, shape.Fields.Errors, shape.Fields.Challenge} {
		switch kind {
		case "absent", "null", "string", "number", "boolean", "object", "array":
		default:
			return emptyMetadata, emptyCapture, invalid
		}
	}
	stringIDs := 0
	for _, kind := range []string{shape.Fields.IDString, shape.Fields.ID, shape.Fields.UserID, shape.Fields.UserIDString} {
		if kind == "string" {
			stringIDs++
		}
	}
	if shape.StableIDStringCandidates > stringIDs {
		return emptyMetadata, emptyCapture, invalid
	}
	switch shape.AuthenticatedFlag {
	case "absent":
		if shape.Fields.Authenticated != "absent" {
			return emptyMetadata, emptyCapture, invalid
		}
	case "true", "false":
		if shape.Fields.Authenticated != "boolean" {
			return emptyMetadata, emptyCapture, invalid
		}
	case "invalid":
		if shape.Fields.Authenticated == "absent" || shape.Fields.Authenticated == "boolean" {
			return emptyMetadata, emptyCapture, invalid
		}
	default:
		return emptyMetadata, emptyCapture, invalid
	}
	return metadata, capture, nil
}

func TestXAccountSettingsProjectionOffline(t *testing.T) {
	metadata := json.RawMessage(`{"schema":"aku.x.native-metadata-observation.v1","coverage":"partial_single_page_home_reload","extraInfoCoverage":"not_observed","observationWindowMs":20000,"candidateCount":0,"droppedCandidates":0,"candidates":[],"canAuthorize":false}`)
	shape := &xAccountSettingsShape{Schema: "aku.x.account-settings-shape.v1", Diagnostic: "projected", AuthenticatedFlag: "absent"}
	shape.Fields.IDString = "absent"
	shape.Fields.ID = "absent"
	shape.Fields.UserID = "absent"
	shape.Fields.UserIDString = "absent"
	shape.Fields.ScreenName = "string"
	shape.Fields.Authenticated = "absent"
	shape.Fields.Error = "absent"
	shape.Fields.Errors = "absent"
	shape.Fields.Challenge = "absent"
	capture := xAccountSettingsCapture{Schema: "aku.x.account-settings-capture.v1", Diagnostic: "projected", MatchingRequests: 1, BodyReadAttempts: 1, Projection: shape}
	encode := func() []byte {
		raw, _ := json.Marshal(map[string]any{"metadata": metadata, "accountSettings": capture})
		return raw
	}
	if _, got, err := decodeXAccountSettingsEnvelope(encode()); err != nil || got.Projection.StableIDStringCandidates != 0 {
		t.Fatal("non-authorizing handle-only shape rejected")
	}
	for _, change := range []func(){func() { shape.Fields.ScreenName = "private-value" }, func() { shape.Fields.ScreenName = "string"; shape.CanAuthorize = true }, func() { shape.CanAuthorize = false; capture.BodyReadAttempts = 2 }, func() { capture.BodyReadAttempts = 1; shape.StableIDStringCandidates = 1 }} {
		change()
		if _, _, err := decodeXAccountSettingsEnvelope(encode()); err == nil {
			t.Fatal("unsafe shape or invalid counts accepted")
		}
	}
	shape.StableIDStringCandidates = 0
	var payload map[string]any
	_ = json.Unmarshal(encode(), &payload)
	payload["rawBody"] = "private"
	bad, _ := json.Marshal(payload)
	if _, _, err := decodeXAccountSettingsEnvelope(bad); err == nil {
		t.Fatal("raw body crossed envelope boundary")
	}
}
