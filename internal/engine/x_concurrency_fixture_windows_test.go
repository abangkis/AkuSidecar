//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

type xConcurrencyFixtureReceipt struct {
	Schema                        string  `json:"schema"`
	Provenance                    string  `json:"provenance"`
	OriginClass                   string  `json:"originClass"`
	Outcome                       string  `json:"outcome"`
	SeedCalls                     int     `json:"seedCalls"`
	CookieTuples                  int     `json:"cookieTuples"`
	CookieBytes                   int     `json:"cookieBytes"`
	ReadCalls                     int     `json:"readCalls"`
	AuthenticatedSameAccountReads int     `json:"authenticatedSameAccountReads"`
	OverlappingPairs              int     `json:"overlappingPairs"`
	MaxOverlapMS                  float64 `json:"maxOverlapMs"`
	ReaderWindows                 int     `json:"readerWindows"`
	ReaderWindowIdentity          string  `json:"readerWindowIdentity"`
	Coverage                      string  `json:"coverage"`
	Refresh                       string  `json:"refresh"`
	CleanupPending                *bool   `json:"cleanupPending"`
	CleanupFailed                 *bool   `json:"cleanupFailed"`
	DurationMS                    float64 `json:"durationMs"`
	CanAuthorize                  *bool   `json:"canAuthorize"`
	NativeQualified               *bool   `json:"nativeQualified"`
}

type xConcurrencyFixtureReport struct {
	Schema          string                      `json:"schema"`
	Scenario        string                      `json:"scenario"`
	Receipt         *xConcurrencyFixtureReceipt `json:"receipt"`
	CanAuthorize    *bool                       `json:"canAuthorize"`
	NativeQualified *bool                       `json:"nativeQualified"`
	RealAuthTested  *bool                       `json:"realAuthTested"`
}

func decodeXConcurrencyFixtureReport(raw []byte, scenario string) (*xConcurrencyFixtureReport, error) {
	invalid := errors.New("synthetic concurrency report invalid")
	expected := map[string]string{"overlap": "overlap_qualified", "mutation": "unexpected_mutation", "coverage_loss": "coverage_unqualified"}
	if len(raw) == 0 || len(raw) > 4096 || expected[scenario] == "" {
		return nil, invalid
	}
	var report xConcurrencyFixtureReport
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || decoder.Decode(new(any)) != io.EOF || report.Receipt == nil {
		return nil, invalid
	}
	var envelope map[string]json.RawMessage
	var receiptFields map[string]json.RawMessage
	if json.Unmarshal(raw, &envelope) != nil || len(envelope) != 6 ||
		json.Unmarshal(envelope["receipt"], &receiptFields) != nil || len(receiptFields) != 20 {
		return nil, invalid
	}
	flags := []*bool{report.CanAuthorize, report.NativeQualified, report.RealAuthTested,
		report.Receipt.CanAuthorize, report.Receipt.NativeQualified, report.Receipt.CleanupPending, report.Receipt.CleanupFailed}
	for _, flag := range flags {
		if flag == nil || *flag {
			return nil, invalid
		}
	}
	r := report.Receipt
	if report.Schema != "aku.x.concurrency-chrome.synthetic.v1" || report.Scenario != scenario ||
		r.Schema != "aku.x.concurrency-harness.v1" || r.Provenance != "synthetic-fixture" ||
		r.OriginClass != "loopback_synthetic_fixture" || r.Outcome != expected[scenario] ||
		r.Refresh != "refresh_not_observed" || r.SeedCalls != 1 || r.CookieTuples != 1 ||
		r.CookieBytes < 1 || r.CookieBytes > 65536 || r.ReadCalls < 0 || r.ReadCalls > 4 ||
		r.AuthenticatedSameAccountReads < 0 || r.AuthenticatedSameAccountReads > r.ReadCalls ||
		r.OverlappingPairs < 0 || r.OverlappingPairs > 2 || r.MaxOverlapMS < 0 || r.MaxOverlapMS > 60000 ||
		r.DurationMS < 0 || r.DurationMS > 60000 || r.ReaderWindows < 1 || r.ReaderWindows > 16 {
		return nil, invalid
	}
	if scenario == "overlap" {
		if r.ReadCalls != 4 || r.AuthenticatedSameAccountReads != 4 || r.OverlappingPairs < 1 || r.MaxOverlapMS <= 0 ||
			r.ReaderWindowIdentity != "retained" || r.Coverage != "synthetic_fixture_only" {
			return nil, invalid
		}
	} else if r.OverlappingPairs != 0 || r.ReaderWindowIdentity != "unverified" ||
		(scenario == "coverage_loss" && r.Coverage != "coverage_unqualified") {
		return nil, invalid
	}
	return &report, nil
}

func TestXConcurrencyFixtureReportOffline(t *testing.T) {
	base := map[string]any{
		"schema": "aku.x.concurrency-chrome.synthetic.v1", "scenario": "overlap",
		"canAuthorize": false, "nativeQualified": false, "realAuthTested": false,
		"receipt": map[string]any{
			"schema": "aku.x.concurrency-harness.v1", "provenance": "synthetic-fixture", "originClass": "loopback_synthetic_fixture",
			"outcome": "overlap_qualified", "seedCalls": 1, "cookieTuples": 1, "cookieBytes": 256, "readCalls": 4,
			"authenticatedSameAccountReads": 4, "overlappingPairs": 2, "maxOverlapMs": 100, "readerWindows": 1,
			"readerWindowIdentity": "retained", "coverage": "synthetic_fixture_only", "refresh": "refresh_not_observed",
			"cleanupPending": false, "cleanupFailed": false, "durationMs": 500, "canAuthorize": false, "nativeQualified": false,
		},
	}
	raw, _ := json.Marshal(base)
	if _, err := decodeXConcurrencyFixtureReport(raw, "overlap"); err != nil {
		t.Fatal("bounded synthetic report rejected")
	}
	for _, change := range []func(map[string]any){
		func(v map[string]any) { v["cookie"] = "private" },
		func(v map[string]any) { delete(v, "nativeQualified") },
		func(v map[string]any) { v["canAuthorize"] = true },
		func(v map[string]any) { v["receipt"].(map[string]any)["accountId"] = "private" },
		func(v map[string]any) { v["receipt"].(map[string]any)["cleanupPending"] = true },
		func(v map[string]any) { delete(v["receipt"].(map[string]any), "readCalls") },
		func(v map[string]any) { v["receipt"].(map[string]any)["overlappingPairs"] = 0 },
	} {
		var altered map[string]any
		_ = json.Unmarshal(raw, &altered)
		change(altered)
		bad, _ := json.Marshal(altered)
		if _, err := decodeXConcurrencyFixtureReport(bad, "overlap"); err == nil {
			t.Fatal("unqualified or private report accepted")
		}
	}
}

type xConcurrencyLoopback struct {
	mu           sync.Mutex
	server       *httptest.Server
	token        string
	logins       int
	active       int
	overlap      int
	rotates      int
	barrier      chan struct{}
	barrierReady bool
}

func startXConcurrencyLoopback(t *testing.T) *xConcurrencyLoopback {
	f := &xConcurrencyLoopback{barrier: make(chan struct{})}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *xConcurrencyLoopback) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
	w.Header().Set("Content-Type", "application/json")
	if r.URL.RawQuery != "" {
		w.WriteHeader(400)
		return
	}
	if r.URL.Path == "/" && r.Method == http.MethodGet {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<!doctype html><title>AkuBrowser Synthetic Concurrency Fixture</title><h1>Synthetic concurrency fixture</h1>")
		return
	}
	if r.URL.Path == "/login" && r.Method == http.MethodPost {
		var token [32]byte
		if _, err := rand.Read(token[:]); err != nil {
			w.WriteHeader(500)
			return
		}
		f.mu.Lock()
		f.token = hex.EncodeToString(token[:])
		f.logins++
		value := f.token
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: value, Path: "/", Secure: true,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(time.Hour).Truncate(time.Second)})
		_ = json.NewEncoder(w).Encode(map[string]any{"account": "alpha", "epoch": 1})
		return
	}
	f.mu.Lock()
	cookie, err := r.Cookie("fixture_session")
	valid := err == nil && f.token != "" && cookie.Value == f.token
	if !valid {
		f.mu.Unlock()
		w.WriteHeader(401)
		_ = json.NewEncoder(w).Encode(map[string]string{"state": "needs_login"})
		return
	}
	if r.URL.Path == "/read-barrier" && r.Method == http.MethodGet {
		f.mu.Unlock()
		select {
		case <-f.barrier:
			_ = json.NewEncoder(w).Encode(map[string]bool{"ready": true})
		case <-time.After(2 * time.Second):
			w.WriteHeader(408)
			_ = json.NewEncoder(w).Encode(map[string]bool{"ready": false})
		case <-r.Context().Done():
		}
		return
	}
	if r.URL.Path == "/rotate" && r.Method == http.MethodPost {
		var next [32]byte
		if _, err := rand.Read(next[:]); err != nil {
			f.mu.Unlock()
			w.WriteHeader(500)
			return
		}
		f.token = hex.EncodeToString(next[:])
		f.rotates++
		value := f.token
		f.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "fixture_session", Value: value, Path: "/", Secure: true,
			HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Now().Add(time.Hour).Truncate(time.Second)})
		_ = json.NewEncoder(w).Encode(map[string]bool{"rotated": true})
		return
	}
	if r.Method == http.MethodGet && (r.URL.Path == "/reader" || r.URL.Path == "/collect") {
		f.active++
		if f.active > f.overlap {
			f.overlap = f.active
		}
		if f.active >= 2 && !f.barrierReady {
			f.barrierReady = true
			close(f.barrier)
		}
		f.mu.Unlock()
		select {
		case <-time.After(200 * time.Millisecond):
		case <-r.Context().Done():
		}
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	} else {
		f.mu.Unlock()
		if r.Method != http.MethodGet || r.URL.Path != "/probe" {
			w.WriteHeader(404)
			return
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"account": "alpha", "epoch": 1, "postId": "fixture-post-1"})
}

func TestXConcurrencyChromeFixture(t *testing.T) {
	if os.Getenv("AKU_X_CONCURRENCY_FIXTURE_ENABLED") != "1" {
		t.Skip("synthetic concurrency requires its explicit project-local helper")
	}
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("fixture source unavailable")
	}
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	for _, scenario := range []string{"overlap", "mutation", "coverage_loss"} {
		t.Run(scenario, func(t *testing.T) {
			f := startXConcurrencyLoopback(t)
			var nonce [16]byte
			if _, err := rand.Read(nonce[:]); err != nil {
				t.Fatal("fixture scope unavailable")
			}
			root := filepath.Join(os.Getenv("AKU_CHROME_FIXTURE_ROOT"), "run-x-concurrency-"+hex.EncodeToString(nonce[:]))
			if err := os.Mkdir(root, 0700); err != nil {
				t.Fatal("fresh fixture root unavailable")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
			defer cancel()
			cfg := chromeFixtureConfig{NodePath: os.Getenv("AKU_TEST_NODE_PATH"), ChromePath: os.Getenv("AKU_TEST_CHROME_PATH"),
				Root: root, Origin: f.server.URL, DriverPath: filepath.Join(project, "scripts", "auth-session-fixture", "x-concurrency-chrome-driver.mjs")}
			bridge, err := startChromeFixtureBridge(ctx, cfg)
			if err != nil {
				t.Fatal("synthetic bridge start failed:", err)
			}
			registerChromeFixtureBridge(bridge)
			t.Cleanup(func() {
				bridge.terminateOwnedTree()
				if bridge.cleanupError() != nil || !bridge.cleanupVerified() {
					t.Error("fixture owned Job cleanup unverified")
				}
			})
			if bridge.assertBridgeOnly(ctx) != nil {
				t.Fatal("Chrome existed before Job assignment")
			}
			var prepared struct {
				ReaderPID        uint32 `json:"readerPID"`
				CollectorPID     uint32 `json:"collectorPID"`
				DistinctProfiles bool   `json:"distinctProfiles"`
				ReaderWindows    int    `json:"readerWindows"`
			}
			if err := bridge.rpc(ctx, "concurrency_prepare", nil, &prepared); err != nil {
				t.Fatal("synthetic concurrency preparation failed:", err)
			}
			if prepared.ReaderPID == 0 || prepared.CollectorPID == 0 || prepared.ReaderPID == prepared.CollectorPID ||
				!prepared.DistinctProfiles || prepared.ReaderWindows < 1 {
				t.Fatal("distinct browser/profile ownership unavailable")
			}
			for _, pid := range []uint32{bridge.pid, prepared.ReaderPID, prepared.CollectorPID} {
				image := cfg.ChromePath
				if pid == bridge.pid {
					image = cfg.NodePath
				}
				verifyXChromeFixtureImage(t, pid, image)
				handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
				if err != nil {
					t.Fatal("owned process handle unavailable")
				}
				member, membershipErr := chromeFixtureJobMembership(handle, bridge.job)
				_ = windows.CloseHandle(handle)
				if membershipErr != nil || !member {
					t.Fatal("fixture process escaped parent Job")
				}
			}
			var raw json.RawMessage
			if err := bridge.rpc(ctx, "concurrency_run", map[string]any{"scenario": scenario}, &raw); err != nil {
				t.Fatal("synthetic concurrency run failed:", err)
			}
			report, err := decodeXConcurrencyFixtureReport(raw, scenario)
			if err != nil {
				t.Fatal("synthetic concurrency evidence rejected")
			}
			f.mu.Lock()
			logins, overlap, rotations := f.logins, f.overlap, f.rotates
			f.mu.Unlock()
			if logins != 1 || overlap < 2 ||
				(scenario == "mutation" && rotations != 1) || (scenario != "mutation" && rotations != 0) {
				t.Fatal("independent server evidence disagrees with concurrency result")
			}
			var closed struct {
				Closed bool `json:"closed"`
			}
			if bridge.rpc(ctx, "concurrency_close", nil, &closed) != nil || !closed.Closed {
				t.Fatal("owned browsers did not close")
			}
			t.Logf("synthetic scenario=%s outcome=%s logins=%d serverOverlap=%d reads=%d nativeQualified=false", scenario, report.Receipt.Outcome, logins, overlap, report.Receipt.ReadCalls)
		})
	}
}
