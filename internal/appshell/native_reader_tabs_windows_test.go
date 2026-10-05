//go:build windows

package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Real Chrome/CDP window affinity proof with a fresh headless profile. The
// containment fixture does not establish Windows foreground or broker parity.
func TestNativeReaderTabsChrome(t *testing.T) {
	chrome := os.Getenv("AKU_NATIVE_READER_TABS_CHROME_PATH")
	if chrome == "" {
		t.Skip("requires an explicit Chrome executable for isolated headless proof")
	}
	if !filepath.IsAbs(chrome) {
		t.Fatal("absolute Chrome path required")
	}
	if _, err := os.Stat(chrome); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "build", fmt.Sprintf("reader-tabs-proof-%d-%d", os.Getpid(), time.Now().UnixNano())))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "<!doctype html><title>Local reader fixture</title><p>Local content only</p>")
	}))
	defer server.Close()
	idleURL := server.URL + "/idle"
	window, err := Launch(ctx, LaunchOptions{Executable: chrome, UserDataDir: filepath.Join(root, "empty-profile"), URL: idleURL,
		PrivateCDP: true, NormalWindow: true, ExtraArgs: []string{"--headless", "--disable-extensions", "--disable-background-networking"}})
	if err != nil {
		t.Fatal(err)
	}
	defer window.Terminate()
	r := &NativeReader{protocol: window.CaptureProtocol(), containment: &nativeContainmentFixture{}, idleURL: idleURL}
	for ctx.Err() == nil && r.idleTarget == "" {
		targets, err := r.targets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			if target.Type == "page" && target.URL == idleURL {
				r.idleTarget = target.ID
			}
		}
		if r.idleTarget == "" {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if r.idleTarget == "" {
		t.Fatal("initial page missing")
	}
	for i := 0; i < 3; i++ {
		_, verify, err := r.PrepareNativePost(ctx, fmt.Sprintf("split_local_%d", i), server.URL+fmt.Sprintf("/post/%d", i), server.URL+fmt.Sprintf("/marker/%d", i))
		if err != nil {
			t.Fatal(err)
		}
		if err := verify(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for {
		targets, err := r.targets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		pages, posts := 0, 0
		for _, target := range targets {
			if target.Type != "page" {
				continue
			}
			pages++
			id, err := r.targetWindow(ctx, target.ID)
			if err != nil || id != *r.readerWindowID {
				t.Fatalf("window affinity lost: %d %v", id, err)
			}
			for i := 0; i < 3; i++ {
				if target.URL == server.URL+fmt.Sprintf("/post/%d", i) {
					posts++
				}
			}
		}
		if pages == 3 && posts == 3 {
			break
		}
		if ctx.Err() != nil {
			t.Fatalf("expected three tabs, got %d pages %d posts", pages, posts)
		}
		time.Sleep(25 * time.Millisecond)
	}
	receipt, _ := json.Marshal(map[string]any{"headless": true, "pageCount": 3, "singleWindowVerified": true, "foregroundVerified": false})
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), receipt, 0600); err != nil {
		t.Fatal(err)
	}
}
