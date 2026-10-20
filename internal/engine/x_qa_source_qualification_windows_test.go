//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"
	"time"
)

type xQASourceCapture struct {
	Schema              string             `json:"schema"`
	Route               *string            `json:"route"`
	Diagnostic          string             `json:"diagnostic"`
	MatchingRequests    int                `json:"matchingRequests"`
	BodyReadAttempts    int                `json:"bodyReadAttempts"`
	BodyContractMatches *bool              `json:"bodyContractMatches"`
	SettingsFields      *xQASettingsFields `json:"settingsFields"`
	Authentication      string             `json:"authentication"`
	NegativeControl     string             `json:"negativeControl"`
	CanAuthorize        *bool              `json:"canAuthorize"`
}
type xQAFieldType struct {
	Kind    string `json:"kind"`
	Matches *bool  `json:"matches"`
}
type xQASettingsFields struct {
	Language      *xQAFieldType `json:"language"`
	TimeZone      *xQAFieldType `json:"time_zone"`
	Protected     *xQAFieldType `json:"protected"`
	AlwaysHTTPS   *xQAFieldType `json:"always_use_https"`
	DiscoverEmail *xQAFieldType `json:"discoverable_by_email"`
	DiscoverPhone *xQAFieldType `json:"discoverable_by_mobile_phone"`
}
type xQACookieAttributes struct {
	Domain       string          `json:"domain"`
	Path         string          `json:"path"`
	HostOnly     *bool           `json:"hostOnly"`
	Secure       *bool           `json:"secure"`
	HTTPOnly     *bool           `json:"httpOnly"`
	SameSite     string          `json:"sameSite"`
	Session      *bool           `json:"session"`
	Expires      *float64        `json:"expires"`
	PartitionKey json.RawMessage `json:"partitionKey"`
	Priority     *string         `json:"priority"`
	SameParty    *bool           `json:"sameParty"`
	SourceScheme *string         `json:"sourceScheme"`
	SourcePort   *int            `json:"sourcePort"`
}
type xQACookieTuple struct {
	Name       string               `json:"name"`
	Status     string               `json:"status"`
	Attributes *xQACookieAttributes `json:"attributes"`
}
type xQACookieMetadata struct {
	Schema             string           `json:"schema"`
	Tuples             []xQACookieTuple `json:"tuples"`
	ValuesPersisted    *bool            `json:"valuesPersisted"`
	SessionTransferred *bool            `json:"sessionTransferred"`
	CanAuthorize       *bool            `json:"canAuthorize"`
}
type xQASourceQualification struct {
	Schema             string             `json:"schema"`
	Source             *xQASourceCapture  `json:"source"`
	Cookies            *xQACookieMetadata `json:"cookies"`
	CanAuthorize       *bool              `json:"canAuthorize"`
	SessionTransferred *bool              `json:"sessionTransferred"`
}

type xQAGuestSource struct {
	Schema             string `json:"schema"`
	Route              string `json:"route"`
	Diagnostic         string `json:"diagnostic"`
	MatchingRequests   int    `json:"matchingRequests"`
	Status             *int   `json:"status"`
	ResponseFresh      *bool  `json:"responseFresh"`
	Completed          *bool  `json:"completed"`
	BodyReadAttempts   int    `json:"bodyReadAttempts"`
	CanAuthorize       *bool  `json:"canAuthorize"`
	SessionTransferred *bool  `json:"sessionTransferred"`
}
type xQAAuthControl struct {
	Schema             string          `json:"schema"`
	SignedIn           json.RawMessage `json:"signedIn"`
	Guest              *xQAGuestSource `json:"guest"`
	Isolation          string          `json:"isolation"`
	Navigation         string          `json:"navigation"`
	OwnerHomeBefore    *bool           `json:"ownerHomeBefore"`
	OwnerHomeAfter     *bool           `json:"ownerHomeAfter"`
	ContextDisposed    *bool           `json:"contextDisposed"`
	Authentication     string          `json:"authentication"`
	CanAuthorize       *bool           `json:"canAuthorize"`
	SessionTransferred *bool           `json:"sessionTransferred"`
}

func decodeXQAAuthControl(raw []byte) (xQAAuthControl, error) {
	var result xQAAuthControl
	bad := errors.New("QA authentication control rejected")
	if len(raw) > 16384 || !xQAFields(raw, 11) {
		return result, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&result) != nil || d.Decode(new(any)) != io.EOF || result.Schema != "aku.x.qa-auth-control.v1" ||
		result.Isolation != "fresh_browser_context" || result.Navigation != "home" || result.Authentication != "unknown" ||
		!xQATrue(result.OwnerHomeBefore) || result.OwnerHomeAfter == nil || !xQATrue(result.ContextDisposed) ||
		!xQAFalse(result.CanAuthorize) || !xQAFalse(result.SessionTransferred) {
		return result, bad
	}
	positive, err := decodeXQASourceQualification(result.SignedIn)
	if err != nil || positive.Cookies != nil || positive.Source.Route == nil || *positive.Source.Route != "api_v1" ||
		positive.Source.Diagnostic != "projected" || !xQATrue(positive.Source.BodyContractMatches) {
		return result, bad
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	g := result.Guest
	if g == nil || !xQAFields(fields["guest"], 10) || g.Schema != "aku.x.qa-guest-source.v1" || g.Route != "api_v1" ||
		g.MatchingRequests < 0 || g.MatchingRequests > 2 || g.BodyReadAttempts != 0 || g.Completed == nil ||
		!xQAFalse(g.CanAuthorize) || !xQAFalse(g.SessionTransferred) || g.Status != nil && (*g.Status < 100 || *g.Status > 599) {
		return result, bad
	}
	if g.Diagnostic == "candidate_rejected" {
		return result, nil
	}
	if g.MatchingRequests == 0 {
		if g.Diagnostic != "not_observed" || g.Status != nil || g.ResponseFresh != nil || *g.Completed {
			return result, bad
		}
		return result, nil
	}
	if g.MatchingRequests != 1 || g.ResponseFresh != nil && !*g.ResponseFresh {
		return result, bad
	}
	if !*g.Completed {
		if g.Diagnostic != "incomplete" {
			return result, bad
		}
		return result, nil
	}
	if g.Status == nil || !xQATrue(g.ResponseFresh) {
		return result, bad
	}
	expected := "other_response"
	if *g.Status == 200 {
		expected = "http_200_unqualified"
	}
	if *g.Status == 401 {
		expected = "authentication_rejection_candidate"
	}
	if g.Diagnostic != expected {
		return result, bad
	}
	return result, nil
}

func xQAFalse(value *bool) bool { return value != nil && !*value }
func xQATrue(value *bool) bool  { return value != nil && *value }
func xQAFields(raw json.RawMessage, count int) bool {
	var fields map[string]json.RawMessage
	return json.Unmarshal(raw, &fields) == nil && len(fields) == count
}

// A successful Browser.close reply is not proof that every Chrome descendant
// left the Job. End this diagnostic's entire owned Job before saving evidence;
// do not wait on a bridge-only predicate until the outer request expires.
func finishXQASourceRun(closeOwned func() bool, stopOwned func(), verified func() bool, save func() error) (bool, error) {
	graceful := closeOwned()
	stopOwned()
	if !verified() {
		return graceful, errors.New("QA Job release unverified")
	}
	if err := save(); err != nil {
		return graceful, errors.New("QA report write failed")
	}
	return graceful, nil
}
func decodeXQASourceQualification(raw []byte) (xQASourceQualification, error) {
	var result xQASourceQualification
	bad := errors.New("QA source qualification report rejected")
	if len(raw) < 1 || len(raw) > 16384 || !xQAFields(raw, 5) {
		return result, bad
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil || decoder.Decode(new(any)) != io.EOF ||
		result.Schema != "aku.x.qa-source-qualification.v3" || !xQAFalse(result.CanAuthorize) || !xQAFalse(result.SessionTransferred) || result.Source == nil {
		return result, bad
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	s := result.Source
	if !xQAFields(fields["source"], 10) || s.Schema != "aku.x.qa-source-capture.v3" || !xQAFalse(s.CanAuthorize) ||
		s.Authentication != "unknown" || s.NegativeControl != "not_run" || s.MatchingRequests < 0 || s.MatchingRequests > 2 || s.BodyReadAttempts < 0 || s.BodyReadAttempts > 1 {
		return result, bad
	}
	if s.Route != nil && *s.Route != "web_i_api" && *s.Route != "api_v1" && *s.Route != "api_i_api" {
		return result, bad
	}
	switch s.Diagnostic {
	case "not_observed", "candidate_rejected", "ambiguous", "read_failed", "body_projection_rejected", "projected":
	default:
		return result, bad
	}
	if s.Diagnostic == "projected" && (s.Route == nil || s.MatchingRequests != 1 || s.BodyReadAttempts != 1 || s.BodyContractMatches == nil) ||
		s.Diagnostic != "projected" && s.BodyContractMatches != nil {
		return result, bad
	}
	if s.Diagnostic != "projected" && s.SettingsFields != nil || s.Diagnostic == "projected" && s.SettingsFields == nil {
		return result, bad
	}
	if f := s.SettingsFields; f != nil {
		var sourceFields map[string]json.RawMessage
		_ = json.Unmarshal(fields["source"], &sourceFields)
		if !xQAFields(sourceFields["settingsFields"], 6) {
			return result, bad
		}
		var fieldEntries map[string]json.RawMessage
		_ = json.Unmarshal(sourceFields["settingsFields"], &fieldEntries)
		entries := []*xQAFieldType{f.Language, f.TimeZone, f.Protected, f.AlwaysHTTPS, f.DiscoverEmail, f.DiscoverPhone}
		keys := []string{"language", "time_zone", "protected", "always_use_https", "discoverable_by_email", "discoverable_by_mobile_phone"}
		allMatch := true
		for index, entry := range entries {
			if entry == nil || entry.Matches == nil || !xQAFields(fieldEntries[keys[index]], 2) {
				return result, bad
			}
			switch entry.Kind {
			case "absent", "null", "string", "boolean", "number", "object", "array":
			default:
				return result, bad
			}
			matches := entry.Kind == "boolean"
			if index == 0 {
				matches = entry.Kind == "string"
			}
			if index == 1 {
				matches = entry.Kind == "string" || entry.Kind == "null" || entry.Kind == "object"
			}
			if *entry.Matches != matches {
				return result, bad
			}
			allMatch = allMatch && matches
		}
		if *s.BodyContractMatches && !allMatch {
			return result, bad
		}
	}
	if result.Cookies == nil {
		return result, nil
	}
	c := result.Cookies
	if s.Diagnostic != "projected" || !xQAFields(fields["cookies"], 5) || c.Schema != "aku.x.qa-cookie-metadata.v1" ||
		!xQAFalse(c.ValuesPersisted) || !xQAFalse(c.SessionTransferred) || !xQAFalse(c.CanAuthorize) || len(c.Tuples) != 2 {
		return result, bad
	}
	var cookieFields map[string]json.RawMessage
	_ = json.Unmarshal(fields["cookies"], &cookieFields)
	var tuples []map[string]json.RawMessage
	_ = json.Unmarshal(cookieFields["tuples"], &tuples)
	for index, tuple := range c.Tuples {
		if len(tuples[index]) != 3 || tuple.Name != []string{"auth_token", "ct0"}[index] {
			return result, bad
		}
		switch tuple.Status {
		case "missing", "ambiguous", "scope_mismatch", "unsupported", "expired", "candidate":
		default:
			return result, bad
		}
		a := tuple.Attributes
		if tuple.Status != "candidate" {
			if a != nil {
				return result, bad
			}
			continue
		}
		if a == nil || !xQAFields(tuples[index]["attributes"], 13) || a.Domain != ".x.com" || a.Path != "/" ||
			!xQAFalse(a.HostOnly) || !xQATrue(a.Secure) || a.HTTPOnly == nil || a.Session == nil || a.Expires == nil ||
			!bytes.Equal(a.PartitionKey, []byte("null")) || a.SameParty != nil && *a.SameParty {
			return result, bad
		}
		switch a.SameSite {
		case "Strict", "Lax", "None", "Unspecified":
		default:
			return result, bad
		}
		if *a.Session && *a.Expires != -1 || !*a.Session && (*a.Expires <= float64(time.Now().Unix()) || *a.Expires > 9007199254740991) {
			return result, bad
		}
		if a.Priority != nil && *a.Priority != "Low" && *a.Priority != "Medium" && *a.Priority != "High" ||
			a.SourceScheme != nil && *a.SourceScheme != "Unset" && *a.SourceScheme != "NonSecure" && *a.SourceScheme != "Secure" ||
			a.SourcePort != nil && (*a.SourcePort < -1 || *a.SourcePort > 65535) {
			return result, bad
		}
	}
	return result, nil
}

func TestXQASourceQualificationOffline(t *testing.T) {
	for _, verified := range []bool{false, true} {
		steps := []string{}
		graceful, err := finishXQASourceRun(func() bool { steps = append(steps, "root_close"); return true },
			func() { steps = append(steps, "owned_job_stop") }, func() bool { return verified },
			func() error { steps = append(steps, "save"); return nil })
		if !graceful || (err == nil) != verified || len(steps) != 2+map[bool]int{false: 0, true: 1}[verified] {
			t.Fatal("report was saved before verified owned-Job cleanup")
		}
		if steps[0] != "root_close" || steps[1] != "owned_job_stop" {
			t.Fatal("QA cleanup order changed")
		}
	}
	raw := []byte(`{"schema":"aku.x.qa-source-qualification.v3","source":{"schema":"aku.x.qa-source-capture.v3","route":null,"diagnostic":"not_observed","matchingRequests":0,"bodyReadAttempts":0,"bodyContractMatches":null,"settingsFields":null,"authentication":"unknown","negativeControl":"not_run","canAuthorize":false},"cookies":null,"canAuthorize":false,"sessionTransferred":false}`)
	if _, err := decodeXQASourceQualification(raw); err != nil {
		t.Fatal(err)
	}
	attributes := `{"domain":".x.com","path":"/","hostOnly":false,"secure":true,"httpOnly":true,"sameSite":"Unspecified","session":false,"expires":4102444800.125,"partitionKey":null,"priority":"Medium","sameParty":null,"sourceScheme":"Secure","sourcePort":443}`
	cookies := `{"schema":"aku.x.qa-cookie-metadata.v1","tuples":[{"name":"auth_token","status":"candidate","attributes":` + attributes + `},{"name":"ct0","status":"candidate","attributes":` + attributes + `}],"valuesPersisted":false,"sessionTransferred":false,"canAuthorize":false}`
	positive := bytes.Replace(raw, []byte(`"cookies":null`), []byte(`"cookies":`+cookies), 1)
	for _, change := range [][2]string{{`"route":null`, `"route":"api_v1"`}, {`"diagnostic":"not_observed"`, `"diagnostic":"projected"`},
		{`"matchingRequests":0`, `"matchingRequests":1`}, {`"bodyReadAttempts":0`, `"bodyReadAttempts":1`}, {`"bodyContractMatches":null`, `"bodyContractMatches":true`}} {
		positive = bytes.Replace(positive, []byte(change[0]), []byte(change[1]), 1)
	}
	fieldTypes := `{"language":{"kind":"string","matches":true},"time_zone":{"kind":"null","matches":true},"protected":{"kind":"boolean","matches":true},"always_use_https":{"kind":"boolean","matches":true},"discoverable_by_email":{"kind":"boolean","matches":true},"discoverable_by_mobile_phone":{"kind":"boolean","matches":true}}`
	positive = bytes.Replace(positive, []byte(`"settingsFields":null`), []byte(`"settingsFields":`+fieldTypes), 1)
	objectPositive := bytes.Replace(positive, []byte(`"time_zone":{"kind":"null","matches":true}`), []byte(`"time_zone":{"kind":"object","matches":true}`), 1)
	if _, err := decodeXQASourceQualification(objectPositive); err != nil {
		t.Fatal("object timezone rejected")
	}
	var positiveFields map[string]json.RawMessage
	_ = json.Unmarshal(objectPositive, &positiveFields)
	positiveFields["cookies"] = json.RawMessage(`null`)
	positiveNoCookies, _ := json.Marshal(positiveFields)
	control := map[string]any{"schema": "aku.x.qa-auth-control.v1", "signedIn": json.RawMessage(positiveNoCookies),
		"guest": map[string]any{"schema": "aku.x.qa-guest-source.v1", "route": "api_v1", "diagnostic": "not_observed", "matchingRequests": 0,
			"status": nil, "responseFresh": nil, "completed": false, "bodyReadAttempts": 0, "canAuthorize": false, "sessionTransferred": false},
		"isolation": "fresh_browser_context", "navigation": "home", "ownerHomeBefore": true, "ownerHomeAfter": true, "contextDisposed": true,
		"authentication": "unknown", "canAuthorize": false, "sessionTransferred": false}
	controlRaw, _ := json.Marshal(control)
	if _, err := decodeXQAAuthControl(controlRaw); err != nil {
		t.Fatal(err)
	}
	guestFields := control["guest"].(map[string]any)
	for _, sample := range []struct {
		status     int
		diagnostic string
	}{
		{200, "http_200_unqualified"}, {401, "authentication_rejection_candidate"}, {429, "other_response"},
	} {
		guestFields["matchingRequests"] = 1
		guestFields["status"] = sample.status
		guestFields["responseFresh"] = true
		guestFields["completed"] = true
		guestFields["diagnostic"] = sample.diagnostic
		encoded, _ := json.Marshal(control)
		if _, err := decodeXQAAuthControl(encoded); err != nil {
			t.Fatal("valid non-authorizing status comparison rejected")
		}
	}
	for _, change := range [][2]string{
		{`"authentication":"unknown"`, `"authentication":"authenticated"`},
		{`"contextDisposed":true`, `"contextDisposed":false`},
		{`"ownerHomeBefore":true`, `"ownerHomeBefore":false`},
		{`"bodyReadAttempts":0`, `"bodyReadAttempts":1`},
		{`"diagnostic":"not_observed"`, `"diagnostic":"authentication_rejection_candidate"`},
		{`"status":null`, `"status":401`},
		{`"guest":{`, `"guest":{"value":"PRIVATE",`},
	} {
		bad := bytes.Replace(controlRaw, []byte(change[0]), []byte(change[1]), 1)
		if _, err := decodeXQAAuthControl(bad); err == nil {
			t.Fatal("unsafe authentication control accepted")
		}
	}
	if report, err := decodeXQASourceQualification(positive); err != nil || report.Cookies == nil || *report.Cookies.Tuples[0].Attributes.Expires != 4102444800.125 {
		t.Fatal("valid attribute-only diagnostic rejected")
	}
	for _, change := range [][2]string{{`"secure":true`, `"secure":null`}, {`"secure":true,`, ``},
		{`"expires":4102444800.125`, `"expires":0`}, {`"sourcePort":443`, `"sourcePort":65536`},
		{`"domain":".x.com"`, `"domain":".twitter.com"`}, {`"partitionKey":null`, `"partitionKey":{}`},
		{`"valuesPersisted":false`, `"valuesPersisted":true`}, {`"name":"ct0"`, `"name":"auth_token"`},
		{`"sameSite":"Unspecified"`, `"sameSite":"invalid"`}, {`"priority":"Medium"`, `"priority":"Medium","value":"PRIVATE"`},
		{`"kind":"string"`, `"kind":"PRIVATE"`}, {`"kind":"string","matches":true`, `"kind":"string","matches":false`},
		{`"kind":"string","matches":true`, `"kind":"string","matches":true,"value":"PRIVATE"`},
		{`"time_zone":{"kind":"null","matches":true}`, `"time_zone":{"kind":"object","matches":false}`}} {
		bad := bytes.Replace(positive, []byte(change[0]), []byte(change[1]), 1)
		if _, err := decodeXQASourceQualification(bad); err == nil {
			t.Fatal("unsafe cookie metadata accepted")
		}
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"authentication":"unknown"`), []byte(`"authentication":"authenticated"`), 1),
		bytes.ReplaceAll(raw, []byte(`"canAuthorize":false`), []byte(`"canAuthorize":true`)),
		bytes.Replace(raw, []byte(`"sessionTransferred":false`), []byte(`"sessionTransferred":true`), 1),
		bytes.Replace(raw, []byte(`"cookies":null`), []byte(`"cookies":null,"value":"private"`), 1),
		bytes.Replace(raw, []byte(`"negativeControl":"not_run",`), nil, 1)} {
		if _, err := decodeXQASourceQualification(bad); err == nil {
			t.Fatal("unsafe or incomplete report accepted")
		}
	}
}

func TestXQASourceQualification(t *testing.T) {
	typesOnly := os.Getenv("AKU_X_QA_SETTINGS_TYPES_ONLY") == "1"
	authControl := os.Getenv("AKU_X_QA_AUTH_CONTROL") == "1"
	directAB := os.Getenv("AKU_X_QA_DIRECT_AB") == "1"
	webABA := os.Getenv("AKU_X_QA_WEB_ABA") == "1"
	web401 := os.Getenv("AKU_X_QA_WEB_401") == "1"
	typesOnly = typesOnly || authControl || directAB || webABA
	if os.Getenv("AKU_X_QA_SOURCE_QUALIFICATION_ENABLED") != "1" || !typesOnly && os.Getenv("AKU_X_QA_COOKIE_METADATA_ALLOWED") != "1" {
		t.Skip("requires explicit retained QA cookie-read authorization")
	}
	_, file, _, _ := runtime.Caller(0)
	project := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	parent := filepath.Join(project, "runtime", "qa", "auth-session-sidecar")
	if _, err := os.Lstat(filepath.Join(parent, xRetainedQAProfileManifestName)); err != nil {
		t.Fatal("existing retained QA profile required; no new login")
	}
	lease, err := acquireXRetainedQAProfile(project)
	if err != nil {
		t.Fatal("retained QA lease unavailable")
	}
	clean := true
	t.Cleanup(func() {
		if lease.Release(clean) != nil {
			t.Error("retained QA lease release unverified")
		}
	})
	if !lease.Existing {
		t.Fatal("existing retained QA profile required")
	}
	output := os.Getenv("AKU_X_QA_SOURCE_REPORT_PATH")
	if !samePath(filepath.Dir(output), parent) || !regexp.MustCompile(`^x-qa-source-[a-f0-9]{32}\.report\.json$`).MatchString(filepath.Base(output)) {
		t.Fatal("QA report scope rejected")
	}
	if _, err := os.Lstat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("QA report must be new")
	}
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	config := chromeFixtureConfig{NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"),
		Root: lease.Root, Origin: server.URL, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-retained-source-qualification-driver.mjs"), qaProfileLease: lease}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	clean = false
	bridge, err := startChromeFixtureBridge(ctx, config)
	if err != nil {
		t.Fatal("QA bridge startup failed")
	}
	registerChromeFixtureBridge(bridge)
	t.Cleanup(func() {
		bridge.terminateOwnedTree()
		clean = bridge.cleanupError() == nil && bridge.cleanupVerified()
		if !clean {
			t.Error("QA Job cleanup unverified")
		}
	})
	if bridge.assertBridgeOnly(ctx) != nil {
		t.Fatal("QA Chrome launched before Job admission")
	}
	verifyXChromeFixtureImage(t, bridge.pid, config.NodePath)
	owner, err := bridge.launch(ctx, "collector")
	if err != nil {
		t.Fatal("retained QA Chrome unavailable")
	}
	verifyXChromeFixtureImage(t, uint32(owner.PID()), config.ChromePath)
	if bridge.containsPID(uint32(owner.PID())) != nil {
		t.Fatal("QA Chrome outside owned Job")
	}
	ready := false
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var state struct {
			Ready bool `json:"ready"`
		}
		if bridge.rpc(ctx, "login_state", map[string]any{"owner": owner.owner}, &state) != nil {
			t.Fatal("QA Home check failed")
		}
		if state.Ready {
			ready = true
			break
		}
		time.Sleep(time.Second)
	}
	if !ready {
		t.Fatal("retained QA login unavailable; no login attempted")
	}
	var raw json.RawMessage
	op := "qualify_source"
	if typesOnly {
		op = "qualify_settings_types"
	}
	if authControl {
		op = "qualify_auth_control"
	}
	if directAB {
		op = "qualify_direct_ab"
	}
	if webABA {
		op = "qualify_web_aba"
	}
	if web401 {
		op = "qualify_web_401"
	}
	if bridge.rpc(ctx, op, map[string]any{"owner": owner.owner}, &raw) != nil {
		t.Fatal("bounded QA qualification failed")
	}
	if web401 {
		if _, err := decodeXQAWeb401(raw); err != nil {
			t.Fatal("QA bounded rejection comparison rejected")
		}
	} else if webABA {
		if _, err := decodeXQAWebABA(raw); err != nil {
			t.Fatal("QA web A/B/A comparison rejected")
		}
	} else if directAB {
		if _, err := decodeXQADirectAB(raw); err != nil {
			t.Fatal("QA direct A/B comparison rejected")
		}
	} else if authControl {
		if _, err := decodeXQAAuthControl(raw); err != nil {
			t.Fatal("QA authentication comparison rejected")
		}
	} else {
		report, err := decodeXQASourceQualification(raw)
		if err != nil {
			t.Fatal("QA diagnostic report rejected")
		}
		if typesOnly && report.Cookies != nil {
			t.Fatal("settings-only run returned unexpected cookie metadata")
		}
	}
	graceful, err := finishXQASourceRun(func() bool {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 7*time.Second)
		defer closeCancel()
		var exited struct {
			Exited bool `json:"exited"`
		}
		return bridge.rpc(closeCtx, "release", map[string]any{"owner": owner.owner}, &exited) == nil && exited.Exited
	}, bridge.terminateOwnedTree, func() bool {
		clean = bridge.cleanupError() == nil && bridge.cleanupVerified()
		return clean
	}, func() error {
		out, openErr := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if openErr != nil {
			return openErr
		}
		writeErr := json.NewEncoder(out).Encode(raw)
		closeErr := out.Close()
		if writeErr != nil {
			return writeErr
		}
		return closeErr
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("QA root graceful close acknowledged=%t; complete owned-Job cleanup verified", graceful)
	t.Log("QA source metadata recorded; login semantics and native admission remain unqualified; no session transferred")
}
