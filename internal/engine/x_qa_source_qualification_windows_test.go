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
	Schema              string  `json:"schema"`
	Route               *string `json:"route"`
	Diagnostic          string  `json:"diagnostic"`
	MatchingRequests    int     `json:"matchingRequests"`
	BodyReadAttempts    int     `json:"bodyReadAttempts"`
	BodyContractMatches *bool   `json:"bodyContractMatches"`
	Authentication      string  `json:"authentication"`
	NegativeControl     string  `json:"negativeControl"`
	CanAuthorize        *bool   `json:"canAuthorize"`
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

func xQAFalse(value *bool) bool { return value != nil && !*value }
func xQATrue(value *bool) bool  { return value != nil && *value }
func xQAFields(raw json.RawMessage, count int) bool {
	var fields map[string]json.RawMessage
	return json.Unmarshal(raw, &fields) == nil && len(fields) == count
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
		result.Schema != "aku.x.qa-source-qualification.v1" || !xQAFalse(result.CanAuthorize) || !xQAFalse(result.SessionTransferred) || result.Source == nil {
		return result, bad
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	s := result.Source
	if !xQAFields(fields["source"], 9) || s.Schema != "aku.x.qa-source-capture.v1" || !xQAFalse(s.CanAuthorize) ||
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
	raw := []byte(`{"schema":"aku.x.qa-source-qualification.v1","source":{"schema":"aku.x.qa-source-capture.v1","route":null,"diagnostic":"not_observed","matchingRequests":0,"bodyReadAttempts":0,"bodyContractMatches":null,"authentication":"unknown","negativeControl":"not_run","canAuthorize":false},"cookies":null,"canAuthorize":false,"sessionTransferred":false}`)
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
	if report, err := decodeXQASourceQualification(positive); err != nil || report.Cookies == nil || *report.Cookies.Tuples[0].Attributes.Expires != 4102444800.125 {
		t.Fatal("valid attribute-only diagnostic rejected")
	}
	for _, change := range [][2]string{{`"secure":true`, `"secure":null`}, {`"secure":true,`, ``},
		{`"expires":4102444800.125`, `"expires":0`}, {`"sourcePort":443`, `"sourcePort":65536`},
		{`"domain":".x.com"`, `"domain":".twitter.com"`}, {`"partitionKey":null`, `"partitionKey":{}`},
		{`"valuesPersisted":false`, `"valuesPersisted":true`}, {`"name":"ct0"`, `"name":"auth_token"`},
		{`"sameSite":"Unspecified"`, `"sameSite":"invalid"`}, {`"priority":"Medium"`, `"priority":"Medium","value":"PRIVATE"`}} {
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
	if os.Getenv("AKU_X_QA_SOURCE_QUALIFICATION_ENABLED") != "1" || os.Getenv("AKU_X_QA_COOKIE_METADATA_ALLOWED") != "1" {
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
	if bridge.rpc(ctx, "qualify_source", map[string]any{"owner": owner.owner}, &raw) != nil {
		t.Fatal("bounded QA qualification failed")
	}
	report, err := decodeXQASourceQualification(raw)
	if err != nil {
		t.Fatal("QA diagnostic report rejected")
	}
	var exited struct {
		Exited bool `json:"exited"`
	}
	if bridge.rpc(ctx, "release", map[string]any{"owner": owner.owner}, &exited) != nil || !exited.Exited || bridge.assertBridgeOnly(ctx) != nil {
		t.Fatal("QA Chrome release unverified")
	}
	bridge.terminateOwnedTree()
	clean = bridge.cleanupError() == nil && bridge.cleanupVerified()
	if !clean {
		t.Fatal("QA Job release unverified")
	}
	out, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal("QA report unavailable")
	}
	err = json.NewEncoder(out).Encode(report)
	_ = out.Close()
	if err != nil {
		t.Fatal("QA report write failed")
	}
	t.Log("QA source metadata recorded; login semantics and native admission remain unqualified; no session transferred")
}
