package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"
)

func TestEmbeddedAssetsExcludeTestSuites(t *testing.T) {
	err := fs.WalkDir(embeddedAssets, "web", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.Contains(path.Base(name), ".test.") || strings.Contains(path.Base(name), ".spec.")) {
			t.Errorf("test suite must not be embedded as a production asset: %s", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStaticAssetsExcludeTestSuites(t *testing.T) {
	server, _ := splitTestServer(t)
	for _, name := range []string{"collection-mode.test.mjs", "instagram-trial-controls.test.mjs"} {
		for _, method := range []string{http.MethodGet, http.MethodHead} {
			t.Run(method+"/"+name, func(t *testing.T) {
				request := httptest.NewRequest(method, "http://127.0.0.1:11122/"+name, nil)
				response := httptest.NewRecorder()
				server.http.Handler.ServeHTTP(response, request)
				if response.Code != http.StatusNotFound {
					t.Fatalf("test suite route returned %d, want 404", response.Code)
				}
				if response.Header().Get("X-Content-Type-Options") != "nosniff" || response.Header().Get("X-Frame-Options") != "DENY" {
					t.Fatal("missing static response security headers")
				}
			})
		}
	}
}

func TestStaticProductionAssetsPreserveBodiesAndGuards(t *testing.T) {
	server, _ := splitTestServer(t)
	for _, asset := range []struct {
		name string
		mime string
	}{
		{"app.js", "javascript"},
		{"styles.css", "text/css"},
		{"collection-mode.js", "javascript"},
		{"instagram-trial-controls.js", "javascript"},
		{"x-daily-controls.js", "javascript"},
		{"timeline-hls-player.js", "javascript"},
		{"timeline-hls-loader.js", "javascript"},
		{"vendor/hls.mjs", "javascript"},
	} {
		t.Run(asset.name, func(t *testing.T) {
			want, err := embeddedAssets.ReadFile("web/" + asset.name)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11122/"+asset.name, nil)
			request.Header.Set("Origin", "http://127.0.0.1:11122")
			response := httptest.NewRecorder()
			server.http.Handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK || !bytes.Equal(response.Body.Bytes(), want) {
				t.Fatalf("production asset changed or unavailable: status=%d", response.Code)
			}
			if !strings.Contains(response.Header().Get("Content-Type"), asset.mime) || response.Header().Get("Cache-Control") != "no-cache" {
				t.Fatalf("static asset headers changed: %v", response.Header())
			}
			if response.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(response.Header().Get("Content-Security-Policy"), "script-src 'self'") {
				t.Fatal("missing production asset security headers")
			}
		})
	}
	t.Run("index", func(t *testing.T) {
		response := httptest.NewRecorder()
		server.http.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:11122/", nil))
		if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), "text/html") || !strings.Contains(response.Body.String(), "/app.js?") {
			t.Fatalf("embedded UI entry unavailable: status=%d", response.Code)
		}
	})
	for _, guard := range []struct {
		name   string
		url    string
		origin string
	}{
		{"foreign-host", "http://foreign.invalid/app.js", ""},
		{"foreign-origin", "http://127.0.0.1:11122/app.js", "https://foreign.invalid"},
		{"extension-origin", "http://127.0.0.1:11122/app.js", "chrome-extension://mfeebfabkhmoaepbcdbbeefpobkedfmp"},
	} {
		t.Run(guard.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, guard.url, nil)
			request.Header.Set("Origin", guard.origin)
			response := httptest.NewRecorder()
			server.http.Handler.ServeHTTP(response, request)
			if response.Code != http.StatusForbidden {
				t.Fatalf("static route guard returned %d, want 403", response.Code)
			}
		})
	}
}
