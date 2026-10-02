//go:build windows

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestHostBindingRejectsRestoredAndAmbiguousTargets(t *testing.T) {
	url := "http://127.0.0.1:1234/quiet-qa-unique"
	for _, tc := range []struct {
		name, raw, id string
		fails         bool
	}{
		{"restored", `{"targetInfos":[{"targetId":"unknown","type":"page","url":"about:blank"}]}`, "", false},
		{"exact", `{"targetInfos":[{"targetId":"unknown","type":"page","url":"about:blank"},{"targetId":"owned","type":"page","url":"` + url + `"}]}`, "owned", false},
		{"worker", `{"targetInfos":[{"targetId":"extension","type":"service_worker","url":"` + url + `"}]}`, "", false},
		{"ambiguous", `{"targetInfos":[{"targetId":"one","type":"page","url":"` + url + `"},{"targetId":"two","type":"page","url":"` + url + `"}]}`, "", true},
		{"missing id", `{"targetInfos":[{"type":"page","url":"` + url + `"}]}`, "", true},
		{"invalid", `{`, "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id, err := hostFromListing(json.RawMessage(tc.raw), url)
			if id != tc.id || (err != nil) != tc.fails {
				t.Fatal("host identity did not fail closed")
			}
		})
	}
}

// Explicit native test uses a disposable project-local profile only. It never
// stops Supervisor or touches the registered authenticated profile.
func TestDisposableQuietProbeNaturalExit(t *testing.T) {
	chrome, root, bridge := os.Getenv("AKU_QUIET_PROBE_CHROME"), os.Getenv("AKU_QUIET_PROBE_RUNTIME"), os.Getenv("AKU_QUIET_PROBE_BRIDGE")
	if chrome == "" || root == "" || bridge == "" {
		t.Skip("explicit candidate paths required")
	}
	t.Setenv("AKU_PARITY_WORKER_ROOT", root)
	profile := filepath.Join(t.TempDir(), "capture-profile")
	p := &probe{}
	defer func() {
		if p.window != nil {
			p.window.Terminate()
		}
		if p.host != nil {
			p.host.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	metadata, err := p.init(ctx, request{Chrome: chrome, Profile: profile, ProfileDirectory: "Default", BridgePath: bridge})
	if err != nil {
		t.Fatal(err)
	}
	if metadata.(map[string]any)["extensionRegistrationChanged"] != false {
		t.Fatal("extension registration scope changed")
	}
	// Exercise hidden target creation through production broker before natural
	// host shutdown; no source/account page is navigated in this fixture.
	_, err = p.targets.Call(ctx, domain.SourceX, "Runtime.evaluate", map[string]any{"expression": "({ready:document.readyState,href:location.href})", "returnByValue": true})
	if err != nil {
		t.Fatal("hidden source setup failed", err)
	}
	protocol := p.window.CaptureProtocol()
	raw, err := protocol.Call(ctx, "Target.createTarget", map[string]any{"url": p.host.URL + "/interactive-fixture", "newWindow": true, "background": true}, "")
	var created struct {
		TargetID string `json:"targetId"`
	}
	if err != nil || json.Unmarshal(raw, &created) != nil || created.TargetID == "" {
		t.Fatal("interactive fixture creation failed")
	}
	raw, err = protocol.Call(ctx, "Browser.getWindowForTarget", map[string]string{"targetId": created.TargetID}, "")
	var native struct {
		WindowID int `json:"windowId"`
	}
	if err != nil || json.Unmarshal(raw, &native) != nil || native.WindowID == 0 {
		t.Fatal("interactive fixture window missing")
	}
	if _, err = protocol.Call(ctx, "Browser.setWindowBounds", map[string]any{"windowId": native.WindowID, "bounds": map[string]string{"windowState": "minimized"}}, ""); err != nil {
		t.Fatal("fixture minimize failed")
	}
	short, cancelShort := context.WithTimeout(ctx, 250*time.Millisecond)
	err = p.shutdown(short)
	cancelShort()
	if err == nil || err.Error() != "quiet_profile_owner_retained" {
		t.Fatal("remaining window should retain profile owner")
	}
	if _, err = protocol.Call(ctx, "Browser.getVersion", nil, ""); err != nil {
		t.Fatal("private root pipe lost while another window remained")
	}
	// Only this test's explicitly created interactive fixture is closed.
	if _, err = protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": created.TargetID}, ""); err != nil {
		t.Fatal("interactive fixture close failed")
	}
	if err = p.shutdown(ctx); err != nil {
		t.Fatal("natural profile drain failed", err)
	}
	t.Log("Hidden targets/host retired; remaining fixture retained root pipe; exact fixture close enabled natural drain; no extension loaded")
}
