//go:build windows

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func TestAuthenticatedSourceReloadUsesRealMaintenanceAction(t *testing.T) {
	s, token := splitTestServer(t)
	server := httptest.NewServer(s.api())
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	workerResult := make(chan error, 1)
	go func() {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/bridge/split-capture/next", nil)
		req.Header.Set("X-Aku-Bridge-Token", token)
		req.Header.Set("X-Aku-Bridge-Contract", domain.BridgeContractVersion)
		req.Header.Set("X-Aku-Capture-Instance", s.splitCapture.key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			workerResult <- err
			return
		}
		defer resp.Body.Close()
		var claim struct {
			Action splitCaptureAction `json:"action"`
		}
		if resp.StatusCode != 200 || json.NewDecoder(resp.Body).Decode(&claim) != nil || claim.Action.Type != "reload_self" {
			workerResult <- errors.New("real reload action was not relayed")
			return
		}
		// Accepting a synthetic/non-delivered ID would fail this actual route.
		w := splitRequest(s, token, s.splitCapture.key, "POST", "/api/operations/bridge/actions/"+claim.Action.ActionID+"/accept", "")
		if w.Code != http.StatusAccepted {
			workerResult <- errors.New("real maintenance acceptance rejected")
			return
		}
		w = splitRequest(s, token, s.splitCapture.key, "POST", "/api/bridge/split-capture/results/"+claim.Action.ID, `{"ok":true,"result":{"accepted":true}}`)
		if w.Code != http.StatusNoContent {
			workerResult <- errors.New("real split result rejected")
			return
		}
		s.engine.RecordHeartbeat(domain.BridgeHeartbeat{BuildID: engine.ExpectedBridgeBuildID})
		workerResult <- nil
	}()
	if err := reloadAuthJourneyControl(ctx, server.URL, token, s.engine.Epoch()); err != nil {
		cancel()
		t.Fatalf("helper=%v worker=%v", err, <-workerResult)
	}
	if err := <-workerResult; err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticatedSourceHostCapabilitiesRequireRetirement(t *testing.T) {
	s := &Server{splitCapture: &splitCaptureTransport{
		hostOnlyRequired: true, sourceTrackingSupported: true, hostCloseSupported: true,
		prepareSourceWindow: func(context.Context, string) error { return nil },
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := waitForAuthJourneyHostCapabilities(ctx, s); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("missing host-only negotiation must block the journey")
	}
	s.splitCapture.hostOnlySupported = true
	if err := waitForAuthJourneyHostCapabilities(context.Background(), s); err != nil {
		t.Fatal("fully negotiated transport must be accepted")
	}
}

type authJourneyGuardProtocol struct {
	targets             []authJourneyTarget
	windows             map[string]int
	closed              []string
	commitNextInventory bool
}

func (p *authJourneyGuardProtocol) Call(_ context.Context, method string, params any, _ string) (json.RawMessage, error) {
	switch method {
	case "Target.getTargets":
		raw, err := json.Marshal(map[string]any{"targetInfos": p.targets})
		if p.commitNextInventory {
			p.commitNextInventory = false
			for i := range p.targets {
				if p.targets[i].ID == "source" {
					p.targets[i].URL = "https://www.linkedin.com/feed/"
				}
			}
		}
		return raw, err
	case "Browser.getWindowForTarget":
		id := params.(map[string]any)["targetId"].(string)
		window, ok := p.windows[id]
		if !ok {
			return nil, errors.New("unknown window")
		}
		return json.Marshal(map[string]any{"windowId": window})
	case "Target.closeTarget":
		p.closed = append(p.closed, params.(map[string]any)["targetId"].(string))
		return json.Marshal(map[string]any{"success": true})
	default:
		return nil, errors.New("unexpected protocol operation")
	}
}
func newAuthJourneyGuardProtocol() *authJourneyGuardProtocol {
	return &authJourneyGuardProtocol{targets: []authJourneyTarget{
		{ID: "host", Type: "page", URL: "http://127.0.0.1:11122/split-capture-host#fixture"},
		{ID: "source", Type: "page", URL: "https://www.linkedin.com/feed/"},
	}, windows: map[string]int{"host": 1, "source": 2}}
}
func TestAuthenticatedSourceWindowCloseGuard(t *testing.T) {
	ctx := context.Background()
	host := "http://127.0.0.1:11122/split-capture-host#fixture"
	for _, tc := range []struct {
		name   string
		change func(*authJourneyGuardProtocol)
	}{
		{"another page shares window", func(p *authJourneyGuardProtocol) {
			p.targets = append(p.targets, authJourneyTarget{ID: "other", Type: "page", URL: "https://www.linkedin.com/feed/"})
			p.windows["other"] = 2
		}},
		{"source joins host window", func(p *authJourneyGuardProtocol) { p.windows["source"] = 1 }},
		{"window identity changes", func(p *authJourneyGuardProtocol) { p.windows["source"] = 3 }},
		{"source navigates to foreign host", func(p *authJourneyGuardProtocol) { p.targets[1].URL = "https://www.linkedin.com.evil.example/feed/" }},
		{"source target disappears", func(p *authJourneyGuardProtocol) { p.targets = p.targets[:1] }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := newAuthJourneyGuardProtocol()
			tc.change(p)
			if closeAuthJourneySourceTarget(ctx, p, "source", 2, host, domain.SourceLinkedIn) == nil {
				t.Fatal("unsafe close accepted")
			}
			if len(p.closed) != 0 {
				t.Fatal("close issued despite ownership failure")
			}
		})
	}
	p := newAuthJourneyGuardProtocol()
	if err := closeAuthJourneySourceTarget(ctx, p, "source", 2, host, domain.SourceLinkedIn); err != nil {
		t.Fatal(err)
	}
	if len(p.closed) != 1 || p.closed[0] != "source" {
		t.Fatal("exact new source target was not the sole close")
	}
}
func TestAuthenticatedSourceWindowDiscoveryGuard(t *testing.T) {
	ctx := context.Background()
	host := "http://127.0.0.1:11122/split-capture-host#fixture"
	p := newAuthJourneyGuardProtocol()
	if _, _, err := findNewAuthJourneySourceWindow(ctx, p, host, domain.SourceLinkedIn, "split_fixture", map[string]bool{"host": true, "source": true}); err == nil {
		t.Fatal("pre-existing source target adopted")
	}
	p.targets = append(p.targets, authJourneyTarget{ID: "second", Type: "page", URL: "https://www.linkedin.com/feed/"})
	p.windows["second"] = 3
	if _, _, err := findNewAuthJourneySourceWindow(ctx, p, host, domain.SourceLinkedIn, "split_fixture", map[string]bool{"host": true}); err == nil {
		t.Fatal("ambiguous new source target adopted")
	}
	if len(p.closed) != 0 {
		t.Fatal("discovery unexpectedly closed a target")
	}
}

func TestAuthenticatedSourceWindowWaitBindsPreparedTarget(t *testing.T) {
	host := "http://127.0.0.1:11122/split-capture-host#fixture"
	p := newAuthJourneyGuardProtocol()
	for i := range p.targets {
		if p.targets[i].ID == "source" {
			p.targets[i].URL = "http://127.0.0.1:11122/split-source-intent?id=split_fixture"
		}
	}
	p.commitNextInventory = true
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	id, window, err := waitForAuthJourneySourceWindow(ctx, p, host, domain.SourceLinkedIn, "split_fixture", "source", map[string]bool{"host": true})
	if err != nil || id != "source" || window != 2 {
		t.Fatalf("committed prepared target not found: %v", err)
	}
	if _, _, err := waitForAuthJourneySourceWindow(ctx, p, host, domain.SourceLinkedIn, "split_fixture", "foreign", map[string]bool{"host": true}); err == nil {
		t.Fatal("unrelated new source page adopted")
	}
	if len(p.closed) != 0 {
		t.Fatal("navigation waiting must never close a target")
	}
}
