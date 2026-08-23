package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestBrowserProfilePathPrefersExplicitInstalledAppPath(t *testing.T) {
	cfg := config.Config{Root: `C:\Program Files\AkuBrowser\runtime\versions\1.2.3`}
	want := `C:\Users\tester\AppData\Local\AkuBrowser\browser-profile`
	if got := browserProfilePath(config.Options{BrowserProfilePath: "  " + want + "  "}, cfg); got != want {
		t.Fatalf("browserProfilePath=%q want=%q", got, want)
	}
}

func TestBrowserProfilePathKeepsLegacyFallback(t *testing.T) {
	cfg := config.Config{Root: t.TempDir()}
	want := filepath.Join(cfg.Root, "runtime", "app-profile")
	if got := browserProfilePath(config.Options{}, cfg); got != want {
		t.Fatalf("browserProfilePath=%q want=%q", got, want)
	}
}

func TestExistingInstanceDetectsHealthySidecar(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Fatalf("unexpected probe path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok","version":"0.8.0"}`))
	}))
	defer server.Close()
	version, running := existingInstance(server.URL+"/api/health", time.Second)
	if !running || version != "0.8.0" {
		t.Fatalf("existing instance not detected: running=%v version=%q", running, version)
	}
}

func TestExistingInstanceIgnoresUnhealthyResponses(t *testing.T) {
	for name, handler := range map[string]http.HandlerFunc{
		"not ok status": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`{"status":"degraded","version":"0.8.0"}`))
		},
		"error payload": func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "boom", http.StatusInternalServerError)
		},
		"malformed json": func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte(`not-json`))
		},
	} {
		server := httptest.NewServer(handler)
		if _, running := existingInstance(server.URL+"/api/health", time.Second); running {
			defer server.Close()
			t.Fatalf("%s must not count as an existing instance", name)
		}
		server.Close()
	}
}

func TestExistingInstanceTreatsUnreachableAsAbsent(t *testing.T) {
	if _, running := existingInstance("http://127.0.0.1:1/api/health", 250*time.Millisecond); running {
		t.Fatal("unreachable endpoint must not count as an existing instance")
	}
}

func TestRuntimeCandidateProbeMatchesPublishedUpdateMetadata(t *testing.T) {
	probe, err := runtimeCandidateProbe(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	metadata := domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion)
	if len(probe) != 6 {
		t.Fatalf("candidate probe must remain bounded for the strict native-host decoder: %+v", probe)
	}
	if probe["status"] != "ok" || probe["runtime"] != "go" || probe["configVersion"] != 1 {
		t.Fatalf("candidate probe identity=%+v", probe)
	}
	if probe["version"] != metadata.CurrentVersion || probe["databaseSchemaVersion"] != metadata.DatabaseSchemaVersion || probe["bridgeContractVersion"] != domain.BridgeContractVersion {
		t.Fatalf("candidate probe=%+v software update metadata=%+v", probe, metadata)
	}
	if metadata.BridgeProtocol.Name != "aku-browser.bridge" || metadata.BridgeProtocol.MinVersion != engine.BridgeProtocolMajor || metadata.BridgeProtocol.MaxVersion != engine.BridgeProtocolMajor {
		t.Fatalf("candidate probe Bridge contract=%v software update protocol=%+v", probe["bridgeContractVersion"], metadata.BridgeProtocol)
	}
	if _, exposed := probe["softwareUpdate"]; exposed {
		t.Fatal("candidate probe must not add fields rejected by the strict native-host decoder")
	}
}

func TestRuntimeCandidateProbeKeepsLegacyHostShapeExact(t *testing.T) {
	probe, err := runtimeCandidateProbe(1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(probe) != 5 {
		t.Fatalf("legacy candidate probe shape changed: %+v", probe)
	}
	if _, exposed := probe["databaseSchemaVersion"]; exposed {
		t.Fatal("legacy strict native host must not receive databaseSchemaVersion")
	}
	if _, err := runtimeCandidateProbe(1, 3); err == nil {
		t.Fatal("unsupported candidate probe schema was accepted")
	}
}
