package httpapi

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/appshell"
)

func TestAppShellStartupEndpoint(t *testing.T) {
	const origin = "http://127.0.0.1:11122"
	server := &Server{}
	handler := server.security(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := server.route(w, r); err != nil {
			server.writeError(w, err)
		}
	}))
	request := func(token, requestOrigin string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, origin+"/api/app-shell/startup-ready", nil)
		r.Header.Set("Origin", requestOrigin)
		r.Header.Set("Sec-Fetch-Site", "same-origin")
		r.Header.Set("X-Aku-Startup-Token", token)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	if w := request("", origin); w.Code != http.StatusForbidden {
		t.Fatalf("no launch: %d", w.Code)
	}
	first, _ := appshell.NewStartup(origin + "/")
	u, _ := url.Parse(first.LaunchURL(origin + "/"))
	token := strings.TrimPrefix(u.Fragment, "aku-startup=")
	server.SetAppShellStartup(first)
	if w := request(token, "https://example.com"); w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: %d", w.Code)
	}
	if w := request(token, origin); w.Code != http.StatusNoContent || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("valid acknowledgement: %d", w.Code)
	}
	if w := request(token, origin); w.Code != http.StatusForbidden {
		t.Fatalf("replay: %d", w.Code)
	}
	second, _ := appshell.NewStartup(origin + "/")
	server.SetAppShellStartup(second)
	if w := request(token, origin); w.Code != http.StatusForbidden {
		t.Fatalf("old launch: %d", w.Code)
	}
	server.SetAppShellStartup(nil)
	u, _ = url.Parse(second.LaunchURL(origin + "/"))
	if w := request(strings.TrimPrefix(u.Fragment, "aku-startup="), origin); w.Code != http.StatusForbidden {
		t.Fatalf("detached launch: %d", w.Code)
	}
}
