//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	xverifier "github.com/abangkis/AkuSidecar/internal/sessionverifier/x"
	"golang.org/x/sys/windows"
)

const xChromeFixtureAccount = "1234567890123456789"
const xChromeFixtureOtherAccount = "2234567890123456789"
const xChromeFixtureMaxWireBody = 65536

func writeXChromeFixtureBody(w http.ResponseWriter, status int, body []byte) {
	if len(body) > xChromeFixtureMaxWireBody {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func writeXChromeFixtureJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	writeXChromeFixtureBody(w, status, body)
}

type xChromeEvidenceHTTP struct {
	mu           sync.Mutex
	server       *httptest.Server
	token        string
	mode         string
	logins       int
	selfRequests int
	generation   uint64
}

func startXChromeEvidenceHTTP(t *testing.T) *xChromeEvidenceHTTP {
	t.Helper()
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		t.Fatal("synthetic secret unavailable")
	}
	f := &xChromeEvidenceHTTP{token: hex.EncodeToString(secret[:]), generation: 1}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *xChromeEvidenceHTTP) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	if r.URL.RawQuery != "" {
		w.WriteHeader(400)
		return
	}
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		writeXChromeFixtureBody(w, http.StatusOK, []byte("<!doctype html><title>Synthetic X evidence fixture</title><h1>Fixture</h1>"))
		return
	}
	if r.URL.Path == "/login" && r.Method == http.MethodPost {
		f.logins++
		http.SetCookie(w, &http.Cookie{Name: "fixture_x_session", Value: f.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
		writeXChromeFixtureBody(w, http.StatusOK, []byte(`{"synthetic":true}`))
		return
	}
	if r.URL.Path != xverifier.LoopbackFixturePath || r.Method != http.MethodPost {
		w.WriteHeader(404)
		return
	}
	f.selfRequests++
	cookie, err := r.Cookie("fixture_x_session")
	if err != nil || cookie.Value != f.token || f.mode == "logout" {
		writeXChromeFixtureJSON(w, http.StatusUnauthorized, map[string]any{"schema": xverifier.FixtureSchema, "state": "logged_out"})
		return
	}
	if f.mode == "malformed" {
		writeXChromeFixtureBody(w, http.StatusOK, []byte(`{"schema":"aku.x.browser-self.fixture.v1","state":"authenticated","state":"logged_out"}`))
		return
	}
	if f.mode == "oversized" {
		// Keep the oversized transport response valid JSON so this case
		// isolates the private reader's byte limit from invalid MIME content.
		writeXChromeFixtureJSON(w, http.StatusOK, map[string]any{
			"schema": xverifier.FixtureSchema, "state": "authenticated",
			"data": map[string]any{"id_str": xChromeFixtureAccount, "name": strings.Repeat("fixture-body-", 1000)},
		})
		return
	}
	if f.mode == "missing_identity" {
		writeXChromeFixtureJSON(w, http.StatusOK, map[string]any{"schema": xverifier.FixtureSchema, "state": "authenticated"})
		return
	}
	id := xChromeFixtureAccount
	if f.mode == "account_switch" {
		id = xChromeFixtureOtherAccount
	}
	writeXChromeFixtureJSON(w, http.StatusOK, map[string]any{
		"schema": xverifier.FixtureSchema, "state": "authenticated",
		"data": map[string]any{"id_str": id, "username": "synthetic-private-handle", "name": "synthetic-private-name"},
	})
}

func TestXEvidenceChromeFixtureOversizedResponseIsCappedJSON(t *testing.T) {
	fixture := startXChromeEvidenceHTTP(t)
	client := fixture.server.Client()
	login, err := client.Post(fixture.server.URL+"/login", "application/json", nil)
	if err != nil {
		t.Fatal("synthetic login transport failed")
	}
	_ = login.Body.Close()
	cookies := login.Cookies()
	if len(cookies) != 1 {
		t.Fatal("synthetic cookie unavailable")
	}
	fixture.setMode("oversized")
	request, err := http.NewRequest(http.MethodPost, fixture.server.URL+xverifier.LoopbackFixturePath, nil)
	if err != nil {
		t.Fatal("synthetic request invalid")
	}
	request.AddCookie(cookies[0])
	response, err := client.Do(request)
	if err != nil {
		t.Fatal("synthetic response transport failed")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, xChromeFixtureMaxWireBody+1))
	if err != nil || response.StatusCode != http.StatusOK || len(body) <= xverifier.MaxBodyBytes ||
		len(body) > xChromeFixtureMaxWireBody || response.ContentLength != int64(len(body)) || !json.Valid(body) {
		t.Fatal("oversized fixture did not preserve its bounded valid-JSON wire contract")
	}
}

func (f *xChromeEvidenceHTTP) setMode(mode string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.mode = mode
	if mode == "logout" || mode == "account_switch" {
		f.generation++
	}
}

type xChromeSelfReply struct {
	Schema              string            `json:"schema"`
	Request             xverifier.Request `json:"request"`
	StatusCode          int               `json:"statusCode"`
	ObservedAt          time.Time         `json:"observedAt"`
	Body                string            `json:"body"`
	EvidenceID          string            `json:"evidenceId"`
	ProductionQualified bool              `json:"productionQualified"`
	NativeQualified     bool              `json:"nativeQualified"`
	CaptureAuthorized   bool              `json:"captureAuthorized"`
	Transport           struct {
		Completed             bool   `json:"completed"`
		Cache                 string `json:"cache"`
		CacheBypassConfigured bool   `json:"cacheBypassConfigured"`
		RequestAfterWindow    bool   `json:"requestAfterWindow"`
		OwnerBound            bool   `json:"ownerBound"`
		BodyBound             bool   `json:"bodyBound"`
		BodyBytes             int    `json:"bodyBytes"`
		DecodedBodyBytes      int    `json:"decodedBodyBytes"`
		Authorizing           bool   `json:"authorizing"`
	} `json:"transport"`
}

func verifyXChromeFixtureImage(t *testing.T, pid uint32, executable string) {
	t.Helper()
	before, err := processFixtureProcessIdentity(pid)
	if err != nil {
		t.Fatal("fixture process identity unavailable")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		t.Fatal("fixture image handle unavailable")
	}
	defer windows.CloseHandle(handle)
	buffer := make([]uint16, 32768)
	size := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(handle, 0, &buffer[0], &size); err != nil {
		t.Fatal("fixture image path unavailable")
	}
	actual, err := filepath.EvalSymlinks(windows.UTF16ToString(buffer[:size]))
	if err != nil {
		t.Fatal("fixture image could not be resolved")
	}
	expected, err := filepath.EvalSymlinks(executable)
	if err != nil || !samePath(actual, expected) {
		t.Fatal("fixture image differs from configured executable")
	}
	after, err := processFixtureProcessIdentity(pid)
	if err != nil || before != after {
		t.Fatal("fixture process creation identity changed")
	}
}

func xChromeSelfEvidence(t *testing.T, bridge *chromeFixtureBridge, ctx context.Context, owner int,
	request xverifier.Request, f *xChromeEvidenceHTTP) (xverifier.ThreeProofEvidence, error) {
	t.Helper()
	var reply xChromeSelfReply
	if err := bridge.rpc(ctx, "observe_self", map[string]any{"owner": owner, "request": request}, &reply); err != nil {
		return xverifier.ThreeProofEvidence{}, err
	}
	if reply.Schema != "aku.x.browser-self.observation.v1" || !reflect.DeepEqual(reply.Request, request) || reply.ProductionQualified || reply.NativeQualified || reply.CaptureAuthorized ||
		!reply.Transport.Completed || !reply.Transport.CacheBypassConfigured || !reply.Transport.RequestAfterWindow ||
		!reply.Transport.OwnerBound || !reply.Transport.BodyBound || reply.Transport.Authorizing ||
		reply.Transport.Cache != "unreported" || reply.Transport.BodyBytes != len([]byte(reply.Body)) ||
		reply.Transport.BodyBytes < 1 || reply.Transport.BodyBytes > xverifier.MaxBodyBytes ||
		reply.Transport.DecodedBodyBytes < reply.Transport.BodyBytes || reply.Transport.DecodedBodyBytes > xverifier.MaxBodyBytes ||
		reply.ObservedAt.Before(request.IssuedAt) || reply.ObservedAt.After(time.Now().UTC()) {
		t.Fatal("bounded Chrome observation did not preserve its trusted fixture binding")
	}
	encoded, err := json.Marshal(reply)
	if err != nil {
		t.Fatal("fixture observation encoding failed")
	}
	for _, secret := range []string{f.token, "synthetic-private-handle", "synthetic-private-name", "fixture_x_session"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatal("private fixture data escaped the ingress boundary")
		}
	}
	envelope := xverifier.Envelope{
		Source: request.Source, Operation: request.Operation, Phase: request.Phase,
		Challenge: request.Challenge, RequestID: request.RequestID,
		RequestOrigin: request.Origin, RequestPath: request.Path, FinalURL: request.Origin + request.Path,
		ProfileID: request.ProfileID, OwnerID: request.OwnerID, OwnerEpoch: request.OwnerEpoch,
		OwnerGeneration: request.OwnerGeneration, CoverageComplete: true, StatusCode: reply.StatusCode, ObservedAt: reply.ObservedAt,
	}
	return xverifier.ThreeProofEvidence{
		Attempt: xverifier.Attempt{Request: request, Envelope: envelope, Body: []byte(reply.Body)},
		Metadata: xverifier.EvidenceMetadata{
			Provenance: xverifier.FixtureEvidenceProvenance, Schema: xverifier.ThreeProofEvidenceSchema,
			Kind: xverifier.EvidenceKindSessionSelf, EvidenceID: reply.EvidenceID, Authority: "fixture-owned-chrome-self",
			Request: request, ObservedAt: reply.ObservedAt, Freshness: xverifier.EvidenceFresh, Cache: xverifier.EvidenceCacheBypass,
		},
	}, nil
}

func TestXEvidenceChromeFixtureToVerifier(t *testing.T) {
	if os.Getenv("AKU_X_EVIDENCE_CHROME_ENABLED") != "1" {
		t.Skip("requires the dedicated X evidence Chrome fixture helper")
	}
	for _, mode := range []string{"happy", "missing_identity", "logout", "account_switch", "malformed", "oversized"} {
		t.Run(mode, func(t *testing.T) {
			fixture := startXChromeEvidenceHTTP(t)
			config := chromeFixtureConfigFromEnv(t, fixture.server.URL)
			config.DriverPath = filepath.Join(filepath.Dir(config.DriverPath), "x-evidence-chrome-driver.mjs")
			profile := filepath.Join(config.Root, "shared")
			if _, err := os.Lstat(profile); !os.IsNotExist(err) {
				t.Fatal("fixture profile was not new")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			bridge, err := startChromeFixtureBridge(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			registerChromeFixtureBridge(bridge)
			var rootHandle windows.Handle
			t.Cleanup(func() {
				bridge.terminateOwnedTree()
				if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
					t.Error("fixture Job cleanup was not verified")
				}
				if rootHandle != 0 {
					// Job accounting can become empty just before the root
					// process handle is signaled. Wait on the pinned handle.
					state, waitErr := windows.WaitForSingleObject(rootHandle, 5000)
					if waitErr != nil || state != windows.WAIT_OBJECT_0 {
						t.Error("owned Chrome root did not signal death")
					}
					_ = windows.CloseHandle(rootHandle)
				}
			})
			if err := bridge.assertBridgeOnly(ctx); err != nil {
				t.Fatal("Chrome started before the fixture Job was assigned")
			}
			owner, err := bridge.launch(ctx, "collector")
			if err != nil {
				t.Fatal(err)
			}
			rootHandle, err = windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(owner.PID()))
			if err != nil {
				t.Fatal("owned Chrome root handle unavailable")
			}
			for _, identityPath := range []struct {
				pid  uint32
				path string
			}{{bridge.pid, config.NodePath}, {uint32(owner.PID()), config.ChromePath}} {
				verifyXChromeFixtureImage(t, identityPath.pid, identityPath.path)
			}
			if info, err := os.Lstat(profile); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatal("new fixture profile identity invalid")
			}
			if resolved, err := filepath.EvalSymlinks(profile); err != nil || !samePath(resolved, profile) {
				t.Fatal("fixture profile escaped its owned root")
			}
			var login struct {
				LoggedIn bool `json:"loggedIn"`
			}
			if err := bridge.rpc(ctx, "fixture_login", map[string]any{"owner": owner.owner}, &login); err != nil || !login.LoggedIn {
				t.Fatalf("synthetic browser login failed: %v", err)
			}

			logicalNow := time.Now().UTC()
			pin := xverifier.Pin{ProfileID: profile, OwnerID: fmt.Sprintf("fixture-owner-%d", owner.PID()), OwnerEpoch: "fixture-epoch", OwnerGeneration: 1,
				AccountID: xChromeFixtureAccount, LineageGeneration: 1}
			verifier, err := xverifier.NewLoopbackFixtureVerifier(pin, fixture.server.URL, func() time.Time { return logicalNow })
			if err != nil {
				t.Fatal(err)
			}
			firstRequest, err := verifier.Issue(xverifier.PhaseInitial)
			if err != nil {
				t.Fatal(err)
			}
			first, err := xChromeSelfEvidence(t, bridge, ctx, owner.owner, firstRequest, fixture)
			if err != nil {
				t.Fatalf("initial Chrome evidence failed: %v", err)
			}
			fixture.setMode(mode)
			logicalNow = time.Now().UTC()
			secondRequest, err := verifier.Issue(xverifier.PhaseRecheck)
			if err != nil {
				t.Fatal(err)
			}
			second, observationErr := xChromeSelfEvidence(t, bridge, ctx, owner.owner, secondRequest, fixture)
			if mode == "malformed" || mode == "oversized" {
				if observationErr == nil {
					t.Fatal("invalid body crossed the bounded private reader")
				}
				wantReason := "body_invalid"
				if mode == "oversized" {
					wantReason = "body_oversized"
				}
				if observationErr.Error() != wantReason {
					t.Fatalf("body was rejected at the wrong boundary: %v", observationErr)
				}
				logicalNow = time.Now().UTC()
				// Spend the failed request too; a body rejection cannot leave a
				// challenge available for a later replacement observation.
				result := verifier.VerifyThreeProof(xverifier.ThreeProofInput{Initial: first,
					Recheck: xverifier.ThreeProofEvidence{Attempt: xverifier.Attempt{Request: secondRequest}},
					Policy:  xverifier.EnrollmentPolicy{ExpectedAccountID: pin.AccountID}})
				if result.Identity.State == xverifier.IdentityMatch || result.CanAuthorize() || result.CaptureAuthorized {
					t.Fatal("failed body produced verification authorization")
				}
			} else {
				if observationErr != nil {
					t.Fatalf("recheck Chrome evidence failed: %v", observationErr)
				}
				// A controlled server generation is an out-of-band synthetic
				// witness. It is not a native X or all-browser coverage claim.
				logicalNow = time.Now().UTC()
				fixture.mu.Lock()
				generation := fixture.generation
				fixture.mu.Unlock()
				input := xverifier.ThreeProofInput{Initial: first, Recheck: second, Policy: xverifier.EnrollmentPolicy{ExpectedAccountID: pin.AccountID},
					Lifecycle: &xverifier.LifecycleEvidence{
						Metadata: xverifier.EvidenceMetadata{Provenance: xverifier.FixtureEvidenceProvenance, Schema: xverifier.ThreeProofLifecycleSchema,
							Kind: xverifier.EvidenceKindLifecycle, EvidenceID: "fixture-lifecycle-coverage", Authority: "controlled-loopback-server",
							Request: secondRequest, ObservedAt: logicalNow, Freshness: xverifier.EvidenceFresh, Cache: xverifier.EvidenceCacheNotApplicable},
						BaselineEvidenceID: first.Metadata.EvidenceID, BaselineAcceptedAt: first.Attempt.Envelope.ObservedAt,
						CoveredFrom: first.Attempt.Envelope.ObservedAt, CoveredThrough: logicalNow, BaselineGeneration: 1, CurrentGeneration: generation, CoverageComplete: true,
					}}
				result := verifier.VerifyThreeProof(input)
				if result.CanAuthorize() || result.CaptureAuthorized || result.NativeQualified || result.ProductionQualified {
					t.Fatal("fixture result became a permit")
				}
				switch mode {
				case "happy":
					if result.Identity.State != xverifier.IdentityMatch || result.Authentication.State != xverifier.Authenticated || result.Continuity.State != xverifier.ContinuitySame {
						t.Fatalf("synthetic Chrome proof did not reach the verifier: %+v", result)
					}
				case "missing_identity":
					if result.Identity.State != xverifier.IdentityUnavailable || result.Authentication.State != xverifier.Authenticated || result.Continuity.State != xverifier.ContinuityUnavailable {
						t.Fatal("missing identity erased or promoted independent auth proof")
					}
				case "logout":
					if result.Authentication.Recheck != xverifier.LoggedOut || result.Continuity.State != xverifier.ContinuityChanged {
						t.Fatal("server logout was not observed through Chrome")
					}
				case "account_switch":
					if result.Identity.State != xverifier.IdentityMismatch || result.Authentication.State != xverifier.Authenticated || result.Continuity.State != xverifier.ContinuityChanged {
						t.Fatal("changed synthetic self account was not fenced")
					}
				}
				replay := verifier.VerifyThreeProof(input)
				if replay.Identity.State == xverifier.IdentityMatch || replay.Authentication.RecheckProven || replay.CanAuthorize() {
					t.Fatal("browser evidence replay was accepted")
				}
			}
			fixture.mu.Lock()
			logins, requests := fixture.logins, fixture.selfRequests
			fixture.mu.Unlock()
			if logins != 1 || requests != 2 {
				t.Fatal("fixture did not perform one login and two browser self requests")
			}
			t.Log("fixture_only=true physical_chrome=true private_pipe=true new_profile=true login_count=1 self_requests=2 bounded_body=true capture_authorized=false")
		})
	}
}
