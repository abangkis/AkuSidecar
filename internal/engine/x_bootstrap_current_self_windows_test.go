//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

type xBootstrapCurrentSelf struct {
	Schema         string `json:"schema"`
	Diagnostic     string `json:"diagnostic"`
	BootstrapState string `json:"bootstrapState"`
	SessionState   string `json:"sessionState"`
	IDFields       struct {
		UserID       string `json:"user_id"`
		UserIDString string `json:"user_id_str"`
	} `json:"idFields"`
	StableIDStringCandidates int   `json:"stableIdStringCandidates"`
	SessionIDsAgree          *bool `json:"sessionIdsAgree"`
	AuthFields               struct {
		LoggedIn        string `json:"loggedIn"`
		IsAuthenticated string `json:"isAuthenticated"`
		Authenticated   string `json:"authenticated"`
		IsActive        string `json:"isActive"`
	} `json:"authFields"`
	EntityChecks struct {
		NormalizedUsers string `json:"normalizedUsers"`
		RootUsers       string `json:"rootUsers"`
	} `json:"entityChecks"`
	CanAuthorize     bool `json:"canAuthorize"`
	ContinuityProven bool `json:"continuityProven"`
}

func decodeXBootstrapCurrentSelf(raw []byte) (xBootstrapCurrentSelf, error) {
	var value xBootstrapCurrentSelf
	invalid := errors.New("bootstrap current-self projection rejected")
	if len(raw) == 0 || len(raw) > 2048 {
		return value, invalid
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 11 {
		return value, invalid
	}
	for _, key := range []string{"schema", "diagnostic", "bootstrapState", "sessionState", "idFields", "stableIdStringCandidates", "sessionIdsAgree", "authFields", "entityChecks", "canAuthorize", "continuityProven"} {
		if fields[key] == nil {
			return value, invalid
		}
	}
	if string(fields["canAuthorize"]) != "false" || string(fields["continuityProven"]) != "false" {
		return value, invalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Schema != "aku.x.bootstrap-current-self.v1" || value.CanAuthorize || value.ContinuityProven {
		return value, invalid
	}
	oneOf := func(candidate string, allowed ...string) bool {
		for _, item := range allowed {
			if candidate == item {
				return true
			}
		}
		return false
	}
	if !oneOf(value.Diagnostic, "projected", "wrong_page", "bootstrap_missing", "bootstrap_invalid", "inspection_failed") ||
		!oneOf(value.BootstrapState, "absent", "object", "invalid", "accessor") || !oneOf(value.SessionState, "absent", "object", "invalid", "accessor") {
		return value, invalid
	}
	stringFields := 0
	for _, kind := range []string{value.IDFields.UserID, value.IDFields.UserIDString} {
		if !oneOf(kind, "absent", "accessor", "null", "string", "number", "boolean", "object", "array", "invalid") {
			return value, invalid
		}
		if kind == "string" {
			stringFields++
		}
	}
	if value.StableIDStringCandidates != stringFields ||
		value.StableIDStringCandidates < 2 && value.SessionIDsAgree != nil || value.StableIDStringCandidates == 2 && value.SessionIDsAgree == nil {
		return value, invalid
	}
	authAbsent := true
	for _, kind := range []string{value.AuthFields.LoggedIn, value.AuthFields.IsAuthenticated, value.AuthFields.Authenticated, value.AuthFields.IsActive} {
		if !oneOf(kind, "absent", "accessor", "true", "false", "invalid") {
			return value, invalid
		}
		authAbsent = authAbsent && kind == "absent"
	}
	eligible := value.StableIDStringCandidates > 0 && oneOf(value.IDFields.UserID, "absent", "string") &&
		oneOf(value.IDFields.UserIDString, "absent", "string") && (value.SessionIDsAgree == nil || *value.SessionIDsAgree)
	for _, check := range []string{value.EntityChecks.NormalizedUsers, value.EntityChecks.RootUsers} {
		if !oneOf(check, "unknown", "absent", "invalid", "accessor", "match", "mismatch") {
			return value, invalid
		}
		if eligible == (check == "unknown") {
			return value, invalid
		}
	}
	fieldsAbsent := value.IDFields.UserID == "absent" && value.IDFields.UserIDString == "absent" && authAbsent &&
		value.EntityChecks.NormalizedUsers == "unknown" && value.EntityChecks.RootUsers == "unknown"
	if value.SessionState != "object" && !fieldsAbsent {
		return value, invalid
	}
	switch value.Diagnostic {
	case "wrong_page", "bootstrap_missing", "inspection_failed":
		if value.BootstrapState != "absent" || value.SessionState != "absent" || !fieldsAbsent {
			return value, invalid
		}
	case "projected":
		if value.BootstrapState != "object" || !oneOf(value.SessionState, "absent", "object") {
			return value, invalid
		}
	case "bootstrap_invalid":
		if !oneOf(value.BootstrapState, "invalid", "accessor") && !oneOf(value.SessionState, "invalid", "accessor") {
			return value, invalid
		}
	}
	return value, nil
}

func TestXBootstrapCurrentSelfOffline(t *testing.T) {
	const valid = `{"schema":"aku.x.bootstrap-current-self.v1","diagnostic":"projected","bootstrapState":"object","sessionState":"object","idFields":{"user_id":"string","user_id_str":"absent"},"stableIdStringCandidates":1,"sessionIdsAgree":null,"authFields":{"loggedIn":"absent","isAuthenticated":"absent","authenticated":"absent","isActive":"true"},"entityChecks":{"normalizedUsers":"match","rootUsers":"absent"},"canAuthorize":false,"continuityProven":false}`
	if _, err := decodeXBootstrapCurrentSelf([]byte(valid)); err != nil {
		t.Fatal("valid unqualified bootstrap shape rejected")
	}
	const missing = `{"schema":"aku.x.bootstrap-current-self.v1","diagnostic":"bootstrap_missing","bootstrapState":"absent","sessionState":"absent","idFields":{"user_id":"absent","user_id_str":"absent"},"stableIdStringCandidates":0,"sessionIdsAgree":null,"authFields":{"loggedIn":"absent","isAuthenticated":"absent","authenticated":"absent","isActive":"absent"},"entityChecks":{"normalizedUsers":"unknown","rootUsers":"unknown"},"canAuthorize":false,"continuityProven":false}`
	if _, err := decodeXBootstrapCurrentSelf([]byte(missing)); err != nil {
		t.Fatal("native-shaped missing-bootstrap diagnostic rejected")
	}
	for _, mutation := range []func(map[string]any){
		func(v map[string]any) { v["canAuthorize"] = true },
		func(v map[string]any) { v["continuityProven"] = true },
		func(v map[string]any) { v["rawState"] = "private" },
		func(v map[string]any) { delete(v, "canAuthorize") },
		func(v map[string]any) { v["canAuthorize"] = nil },
		func(v map[string]any) { v["idFields"].(map[string]any)["user_id"] = "private-account-id" },
		func(v map[string]any) { v["authFields"].(map[string]any)["token"] = "private" },
		func(v map[string]any) { v["stableIdStringCandidates"] = 0 },
		func(v map[string]any) { v["sessionIdsAgree"] = true },
		func(v map[string]any) { v["diagnostic"] = "bootstrap_missing" },
		func(v map[string]any) { v["sessionState"] = "absent" },
		func(v map[string]any) { v["entityChecks"].(map[string]any)["rootUsers"] = "unknown" },
		func(v map[string]any) { v["idFields"].(map[string]any)["user_id_str"] = "accessor" },
	} {
		var value map[string]any
		_ = json.Unmarshal([]byte(valid), &value)
		mutation(value)
		raw, _ := json.Marshal(value)
		if _, err := decodeXBootstrapCurrentSelf(raw); err == nil {
			t.Fatal("private, missing or inconsistent bootstrap DTO accepted")
		}
	}
}
