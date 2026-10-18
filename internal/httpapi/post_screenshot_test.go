package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestPostScreenshotHTTPUsesPersistedPostWithoutMutation(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	fixture := createTimelineMemoryHTTPFixture(t, state, "completed", "full captured source body", "9901")
	before, err := state.TimelineItem(context.Background(), fixture.TimelineID)
	if err != nil {
		t.Fatal(err)
	}
	server.postScreenshotRenderer = func(ctx context.Context, entry domain.TimelineItem, theme string) (postScreenshotResult, error) {
		if entry.ID != fixture.TimelineID || entry.Evidence.Text != fixture.Text || theme != "dark" {
			t.Fatal("not persisted evidence")
		}
		return postScreenshotResult{Parts: []postScreenshotPart{{Data: "fixture-png", Width: 800, Height: 1800}}, Warnings: []string{}, Filename: "fixture"}, nil
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/post-screenshots", strings.NewReader(`{"timelineId":"`+fixture.TimelineID+`","theme":"dark"}`))
	request.Header.Set("Content-Type", "application/json")
	server.api().ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "fixture-png") || !strings.Contains(response.Header().Get("Cache-Control"), "no-store") {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	after, err := state.TimelineItem(context.Background(), fixture.TimelineID)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if !bytes.Equal(a, b) {
		t.Fatal("export changed persisted post")
	}
	server.postScreenshotGate <- struct{}{}
	busy := httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/post-screenshots", strings.NewReader(`{"timelineId":"`+fixture.TimelineID+`","theme":"dark"}`))
	request.Header.Set("Content-Type", "application/json")
	server.api().ServeHTTP(busy, request)
	<-server.postScreenshotGate
	if busy.Code != 429 {
		t.Fatalf("concurrent export status %d", busy.Code)
	}
}

func TestPostScreenshotHTTPRejectsForeignOriginAndUntrustedPayload(t *testing.T) {
	server, _ := openLibraryHTTPFixture(t)
	for _, body := range []string{`{"timelineId":"missing","theme":"neon"}`, `{"timelineId":"missing","theme":"dark","html":"arbitrary"}`, `{"timelineId":"missing","theme":"dark"} {}`, strings.Repeat("x", 5000)} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/post-screenshots", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		server.api().ServeHTTP(response, request)
		if response.Code != 400 {
			t.Fatalf("status %d", response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/post-screenshots", strings.NewReader(`{}`))
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	server.security(server.api()).ServeHTTP(response, request)
	if response.Code != 403 {
		t.Fatal("foreign origin accepted")
	}
	request = httptest.NewRequest(http.MethodPost, "/api/post-screenshots", strings.NewReader(`{"timelineId":"missing","theme":"light"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != 404 {
		t.Fatalf("missing post status %d", response.Code)
	}
}

func TestPostScreenshotSnapshotExcludesPrivateContext(t *testing.T) {
	entry := domain.TimelineItem{ID: "private-id", SessionID: "private-session", Coverage: map[string]any{"private": "secret"}, Evidence: &domain.Block{Author: "public-author", Text: "public-body", DirectContext: []domain.DirectContext{{}}}}
	raw, _ := json.Marshal(screenshotSnapshot(entry))
	for _, private := range []string{"private-id", "private-session", "secret", "directContext", "coverage", "personalMemory", "aiDetection"} {
		if bytes.Contains(raw, []byte(private)) {
			t.Fatalf("private field %s retained", private)
		}
	}
	if !bytes.Contains(raw, []byte("public-body")) {
		t.Fatal("lost source content")
	}
}

func TestPostScreenshotMediaNetworkBoundary(t *testing.T) {
	for _, raw := range []string{"http://pbs.twimg.com/media/a.jpg", "https://pbs.twimg.com.attacker.test/a", "https://localhost/a", "https://pbs.twimg.com:443/a", "https://user@pbs.twimg.com/a", "file:///a", "data:image/png;base64,AA==", "https://pbs.twimg.com/a#fragment"} {
		if screenshotMediaURL(raw) {
			t.Fatalf("unsafe URL %s", raw)
		}
	}
	for _, raw := range []string{"https://pbs.twimg.com/media/a.jpg", "https://media.licdn.com/a", "https://scontent.fbcdn.net/a"} {
		if !screenshotMediaURL(raw) {
			t.Fatalf("valid URL rejected %s", raw)
		}
	}
	for _, raw := range []string{"127.0.0.1", "::1", "10.1.1.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "2001:db8::1"} {
		if publicScreenshotIP(net.ParseIP(raw)) {
			t.Fatalf("private address %s", raw)
		}
	}
	client := screenshotMediaClient()
	if client.Jar != nil || client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("ambient credentials/proxy configured")
	}
	redirect := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/media", nil)
	if client.CheckRedirect(redirect, nil) == nil {
		t.Fatal("unsafe redirect accepted")
	}
}

type screenshotRoundTrip func(*http.Request) (*http.Response, error)

func (f screenshotRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestPostScreenshotMediaRejectsHTMLSVGAndOversizeWithoutCredentials(t *testing.T) {
	for _, body := range []string{"<svg xmlns='http://www.w3.org/2000/svg'></svg>", "<html>login</html>", strings.Repeat("x", 100)} {
		client := &http.Client{Transport: screenshotRoundTrip(func(r *http.Request) (*http.Response, error) {
			if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" || r.Header.Get("Referer") != "" {
				t.Fatal("credentials forwarded")
			}
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})}
		if _, _, err := downloadScreenshotMedia(context.Background(), client, "https://pbs.twimg.com/media/a", 80); err == nil {
			t.Fatal("invalid image accepted")
		}
	}
	img := image.NewRGBA(image.Rect(0, 0, 4, 3))
	var pngBody bytes.Buffer
	_ = png.Encode(&pngBody, img)
	client := &http.Client{Transport: screenshotRoundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(pngBody.Bytes())), Header: http.Header{}}, nil
	})}
	data, mime, err := downloadScreenshotMedia(context.Background(), client, "https://pbs.twimg.com/media/a", 1024)
	if err != nil || mime != "image/png" || len(data) == 0 {
		t.Fatalf("valid raster: %s %v", mime, err)
	}
}

func TestPostScreenshotRendererFailureDoesNotExposeSourceOrPaths(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	fixture := createTimelineMemoryHTTPFixture(t, state, "completed", "fixture body", "9902")
	server.postScreenshotRenderer = func(context.Context, domain.TimelineItem, string) (postScreenshotResult, error) {
		return postScreenshotResult{}, errors.New("private-path credential-token")
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/post-screenshots", strings.NewReader(`{"timelineId":"`+fixture.TimelineID+`","theme":"dark"}`))
	request.Header.Set("Content-Type", "application/json")
	server.api().ServeHTTP(response, request)
	if response.Code != 503 || strings.Contains(response.Body.String(), "credential-token") {
		t.Fatal("unsafe error")
	}
}

// Opt-in only: fresh offline Chrome profiles, synthetic images/text, no login.
func TestPostScreenshotChromeOfflineSmoke(t *testing.T) {
	if os.Getenv("AKU_POST_SCREENSHOT_SMOKE") != "1" {
		t.Skip("set AKU_POST_SCREENSHOT_SMOKE=1 for isolated Chrome acceptance")
	}
	_, source, _, _ := runtime.Caller(0)
	project := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	node, err := screenshotNode(project, true)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Second)
	defer cancel()
	chrome, err := appshell.Discover(ctx, os.Getenv("AKU_POST_SCREENSHOT_CHROME"))
	if err != nil {
		t.Fatal(err)
	}
	imageFixture := image.NewRGBA(image.Rect(0, 0, 320, 160))
	for y := 0; y < 160; y++ {
		for x := 0; x < 320; x++ {
			imageFixture.Set(x, y, color.RGBA{240, 80, 30, 255})
		}
	}
	var pngFixture bytes.Buffer
	_ = png.Encode(&pngFixture, imageFixture)
	media := map[string]string{"https://pbs.twimg.com/media/synthetic.png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngFixture.Bytes())}
	artifacts := os.Getenv("AKU_POST_SCREENSHOT_ARTIFACTS")
	if artifacts == "" {
		artifacts = t.TempDir()
	}
	for _, tc := range []struct {
		name  string
		lines int
		parts int
	}{{"whole-post", 40, 1}, {"multipart", 850, 3}} {
		t.Run(tc.name, func(t *testing.T) {
			entry := map[string]any{"source": "x", "evidence": map[string]any{"author": "Synthetic author @fixture", "text": strings.Repeat("Captured full line: the screenshot retains this text.\n", tc.lines), "media": []any{map[string]any{"kind": "image", "url": "https://pbs.twimg.com/media/synthetic.png"}}, "quotedPost": map[string]any{"author": "Quoted author", "text": "Full quote remains visible"}}}
			root := t.TempDir()
			result, remove, err := runPostScreenshotProcess(ctx, root, node, chrome.Executable, entry, "dark", media, nil)
			if err != nil || !remove {
				t.Fatalf("renderer: %v cleanup=%v", err, remove)
			}
			if len(result.Parts) < tc.parts {
				t.Fatalf("parts %d want at least %d", len(result.Parts), tc.parts)
			}
			total := 0
			orangePixels := 0
			for i, part := range result.Parts {
				raw, err := base64.StdEncoding.DecodeString(part.Data)
				if err != nil {
					t.Fatal(err)
				}
				cfg, err := png.DecodeConfig(bytes.NewReader(raw))
				if err != nil || cfg.Width != 800 || cfg.Height != part.Height {
					t.Fatalf("PNG %v %v", cfg, err)
				}
				total += part.Height
				bitmap, decodeErr := png.Decode(bytes.NewReader(raw))
				if decodeErr != nil {
					t.Fatal(decodeErr)
				}
				for y := 0; y < bitmap.Bounds().Dy(); y += 8 {
					for x := 0; x < bitmap.Bounds().Dx(); x += 8 {
						red, green, blue, _ := bitmap.At(x, y).RGBA()
						if red>>8 == 240 && green>>8 == 80 && blue>>8 == 30 {
							orangePixels++
						}
					}
				}
				if artifacts != "" {
					if err := os.WriteFile(filepath.Join(artifacts, tc.name+"-"+string(rune('1'+i))+".png"), raw, 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			if total < 1000 {
				t.Fatal("post clipped to viewport")
			}
			if total < tc.lines*15 || orangePixels < 20 {
				t.Fatalf("full text or synthetic media missing: height=%d orange samples=%d", total, orangePixels)
			}
			if _, err := os.Stat(filepath.Join(root, "profile")); err != nil {
				t.Fatal("fresh profile missing")
			}
		})
	}
	t.Run("desktop-mobile-menu-and-preview", func(t *testing.T) {
		cmd := exec.Command(node, filepath.Join(project, "scripts", "post-screenshot-ui-fixture.mjs"), chrome.Executable, t.TempDir(), artifacts)
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		owner, err := appshell.StartOwnedCommand(cmd)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case err = <-owner.Done():
		case <-ctx.Done():
			cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
			_ = owner.CloseForRetry(cleanup)
			stop()
			err = ctx.Err()
		}
		if err != nil {
			t.Fatalf("UI fixture: %v\n%s", err, output.String())
		}
		t.Log(output.String())
	})
	t.Run("cancel-owned-renderer", func(t *testing.T) {
		cancelCtx, stop := context.WithCancel(context.Background())
		timer := time.AfterFunc(100*time.Millisecond, stop)
		defer timer.Stop()
		defer stop()
		_, remove, err := runPostScreenshotProcess(cancelCtx, t.TempDir(), node, chrome.Executable, map[string]any{"source": "x", "evidence": map[string]any{"text": "synthetic cancel fixture"}}, "dark", nil, nil)
		if !errors.Is(err, context.Canceled) || !remove {
			t.Fatalf("cancel must verify cleanup before releasing files: err=%v remove=%v", err, remove)
		}
	})
}
