//go:build windows

package appshell

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

// Explicit foreground approval required: local, empty-profile HWND binding
// smoke. It does not exercise the UI broker or social login session.
func TestNativeReaderTabsWindowsBindings(t *testing.T) {
	if os.Getenv("AKU_READER_TAB_BINDING_ALLOW_FOREGROUND") != "1" {
		t.Skip("requires explicit foreground approval")
	}
	chrome := os.Getenv("AKU_READER_TAB_BINDING_CHROME_PATH")
	if !filepath.IsAbs(chrome) {
		t.Fatal("absolute Chrome path required")
	}
	if _, err := os.Stat(chrome); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "build", fmt.Sprintf("reader-tab-bindings-%d-%d", os.Getpid(), time.Now().UnixNano())))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if id == "" {
			id = "idle"
		}
		_, _ = fmt.Fprintf(w, "<!doctype html><title>AkuBrowser reader %s</title><p>Local reader only</p>", id)
	}))
	defer server.Close()
	idleURL := server.URL + "/idle"
	window, err := Launch(ctx, LaunchOptions{Executable: chrome, UserDataDir: filepath.Join(root, "empty-profile"), URL: idleURL,
		PrivateCDP: true, NormalWindow: true, ExtraArgs: []string{"--disable-extensions", "--disable-background-networking"}})
	if err != nil {
		t.Fatal(err)
	}
	defer window.Terminate() // Only this newly created empty-profile test process.
	r, err := NewNativeReader(ctx, window, idleURL, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	var previous readerbroker.Target
	var oldVerify func(context.Context) error
	for i := 0; i < 3; i++ {
		id := fmt.Sprintf("split_local_%d", i)
		target, verify, err := r.PrepareNativePost(ctx, id, server.URL+"/post", server.URL+"/marker?id="+id)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 && (target.HWND != previous.HWND || target.Value == previous.Value || !target.Expires.After(previous.Expires)) {
			t.Fatal("reader HWND not shared or capability not renewed")
		}
		if oldVerify != nil && oldVerify(ctx) == nil {
			t.Fatal("previous capability was reusable")
		}
		previous, oldVerify = target, verify
	}
	if err := window.ReplacementReadiness(ctx); err == nil {
		t.Fatal("open reader released profile ownership")
	}
	targets, err := r.targets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	pages := 0
	for _, target := range targets {
		if target.Type != "page" {
			continue
		}
		pages++
		if pages == 3 {
			continue
		}
		if _, err := r.protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": target.ID}, ""); err != nil {
			t.Fatal(err)
		}
	}
	if pages != 3 {
		t.Fatalf("expected three reader tabs, got %d", pages)
	}
	if err := window.ReplacementReadiness(ctx); err == nil {
		t.Fatal("remaining tab released profile")
	}
	targets, err = r.targets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range targets {
		if target.Type == "page" {
			_, _ = r.protocol.Call(ctx, "Target.closeTarget", map[string]string{"targetId": target.ID}, "")
		}
	}
	select {
	case <-window.Done():
	case <-ctx.Done():
		t.Fatal("reader did not naturally exit")
	}
	if err := window.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), []byte(`{"threeTabsOneHWND":true,"freshCapabilities":true,"oldCapabilityRejected":true,"profileHeldUntilLastTab":true,"naturalExit":true}`), 0600); err != nil {
		t.Fatal(err)
	}
}
