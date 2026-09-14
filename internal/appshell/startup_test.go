package appshell

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func startupRequest(token string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:11122/api/app-shell/startup-ready", nil)
	r.Header.Set("Origin", "http://127.0.0.1:11122")
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	r.Header.Set("X-Aku-Startup-Token", token)
	return r
}

func TestStartupAcknowledgementBoundaries(t *testing.T) {
	for name, change := range map[string]func(*http.Request){
		"missing token":    func(r *http.Request) { r.Header.Del("X-Aku-Startup-Token") },
		"wrong token":      func(r *http.Request) { r.Header.Set("X-Aku-Startup-Token", strings.Repeat("x", 64)) },
		"missing origin":   func(r *http.Request) { r.Header.Del("Origin") },
		"foreign origin":   func(r *http.Request) { r.Header.Set("Origin", "https://example.com") },
		"extension":        func(r *http.Request) { r.Header.Set("Origin", "chrome-extension://example") },
		"different port":   func(r *http.Request) { r.Host = "127.0.0.1:11123" },
		"cross site":       func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") },
		"missing metadata": func(r *http.Request) { r.Header.Del("Sec-Fetch-Site") },
		"get":              func(r *http.Request) { r.Method = http.MethodGet },
	} {
		t.Run(name, func(t *testing.T) {
			s, err := NewStartup("http://127.0.0.1:11122/")
			if err != nil {
				t.Fatal(err)
			}
			token := s.token
			r := startupRequest(token)
			change(r)
			if s.Acknowledge(r) {
				t.Fatal("unrelated request accepted")
			}
			if !s.Acknowledge(startupRequest(token)) {
				t.Fatal("invalid attempt consumed capability")
			}
			if s.Acknowledge(startupRequest(token)) {
				t.Fatal("replay accepted")
			}
		})
	}
}

func TestStartupLifecycle(t *testing.T) {
	s, err := NewStartup("http://127.0.0.1:11122/")
	if err != nil {
		t.Fatal(err)
	}
	token := s.token
	base := s.started
	for _, test := range []struct {
		elapsed       time.Duration
		show, overdue bool
	}{
		{0, false, false}, {time.Second - time.Nanosecond, false, false}, {time.Second, true, false}, {60 * time.Second, true, true},
	} {
		show, overdue, finished, _ := s.status(base.Add(test.elapsed))
		if show != test.show || overdue != test.overdue || finished {
			t.Fatalf("unexpected state at %v: %v %v %v", test.elapsed, show, overdue, finished)
		}
	}
	s.keepWaiting(base.Add(65 * time.Second))
	if _, overdue, _, _ := s.status(base.Add(66 * time.Second)); overdue {
		t.Fatal("keep waiting did not reset deadline")
	}
	if _, overdue, _, _ := s.status(base.Add(125 * time.Second)); !overdue {
		t.Fatal("next timeout missing")
	}
	if !s.Acknowledge(startupRequest(token)) {
		t.Fatal("late ready was not accepted")
	}
	if show, overdue, finished, ready := s.status(base.Add(200 * time.Second)); !show || overdue || finished || !ready {
		t.Fatal("ready must preserve an accessible recovery window without timeout")
	}
	s.keepWaiting(base.Add(201 * time.Second))
	if _, overdue, _, ready := s.status(base.Add(302 * time.Second)); overdue || !ready {
		t.Fatal("waiting cleared the acknowledgement")
	}
	s.Stop()
	s.Stop()
	if show, _, finished, _ := s.status(base.Add(400 * time.Second)); show || !finished {
		t.Fatal("dismiss did not finish fallback")
	}
	var absent *Startup
	absent.Stop()
	if absent.Acknowledge(startupRequest(token)) {
		t.Fatal("missing launch accepted acknowledgement")
	}
}

func TestStartupStoppedAndStaleLaunchDenied(t *testing.T) {
	first, _ := NewStartup("http://127.0.0.1:11122/")
	second, _ := NewStartup("http://127.0.0.1:11122/")
	token := first.token
	if second.Acknowledge(startupRequest(token)) {
		t.Fatal("stale token accepted")
	}
	first.Stop()
	if first.Acknowledge(startupRequest(token)) {
		t.Fatal("dismissed launch accepted")
	}
	if first.token != "" {
		t.Fatal("stopped capability retained")
	}
}

func TestStartupOnlyOneConcurrentAcknowledgement(t *testing.T) {
	s, _ := NewStartup("http://127.0.0.1:11122/")
	token := s.token
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range 20 {
		group.Add(1)
		go func() {
			defer group.Done()
			if s.Acknowledge(startupRequest(token)) {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	if accepted.Load() != 1 {
		t.Fatalf("accepted %d times", accepted.Load())
	}
}

func TestStartupCapabilityStaysOutOfHTTPURLAndDiagnostics(t *testing.T) {
	s, _ := NewStartup("http://127.0.0.1:11122/")
	u, err := url.Parse(s.LaunchURL("http://127.0.0.1:11122/"))
	if err != nil {
		t.Fatal(err)
	}
	if u.RequestURI() != "/" || u.Fragment != "aku-startup="+s.token {
		t.Fatal("capability leaked into request URL")
	}
	if strings.Contains(s.diagnostics(time.Now()), s.token) {
		t.Fatal("capability leaked into diagnostics")
	}
	for _, target := range []string{"", "https://127.0.0.1/", "http://user@127.0.0.1/", "http://127.0.0.1/#existing"} {
		if _, err := NewStartup(target); err == nil {
			t.Fatalf("invalid target accepted: %s", target)
		}
	}
}
