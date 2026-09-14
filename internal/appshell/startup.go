package appshell

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const startupDelay = time.Second
const startupTimeout = 60 * time.Second

// Startup only authorizes acknowledgement of this launch's initialization. Its token
// is independent of runtime control and grants no access to application data.
type Startup struct {
	mu           sync.Mutex
	token        string
	origin       string
	started      time.Time
	waitingSince time.Time
	finished     bool
	ready        bool
	retry        chan struct{}
	retrying     bool
	retryFailed  bool
	stopped      chan struct{}
	onReady      func()
}

func NewStartup(target string) (*Startup, error) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "http" || u.Host == "" || u.User != nil || u.Fragment != "" {
		return nil, fmt.Errorf("invalid app-shell startup target")
	}
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return nil, fmt.Errorf("create startup acknowledgement: %w", err)
	}
	now := time.Now()
	return &Startup{token: hex.EncodeToString(secret), origin: u.Scheme + "://" + u.Host, started: now, waitingSince: now, retry: make(chan struct{}, 1), stopped: make(chan struct{})}, nil
}

// LaunchURL uses a fragment so the capability is not sent in HTTP URLs or
// referrers. The independent startup watchdog removes it before app bootstrap.
func (s *Startup) LaunchURL(target string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return target + "#aku-startup=" + s.token
}

func (s *Startup) Acknowledge(r *http.Request) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	if s.finished || s.ready || r.Method != http.MethodPost || r.Header.Get("Origin") != s.origin ||
		r.Host != s.origin[len("http://"):] || r.Header.Get("Sec-Fetch-Site") != "same-origin" ||
		len(r.Header.Get("X-Aku-Startup-Token")) != 64 ||
		subtle.ConstantTimeCompare([]byte(s.token), []byte(r.Header.Get("X-Aku-Startup-Token"))) != 1 {
		s.mu.Unlock()
		return false
	}
	s.ready = true
	s.token = ""
	onReady := s.onReady
	s.mu.Unlock()
	if onReady != nil {
		onReady()
	}
	return true
}

func (s *Startup) requestRetry() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.finished || s.retrying {
		return false
	}
	s.retrying = true
	s.retryFailed = false
	s.token = "" // The old page cannot acknowledge a replacement launch.
	s.retry <- struct{}{}
	return true
}

func (s *Startup) resetAttempt(target string) error {
	fresh, err := NewStartup(target)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.token, s.origin = fresh.token, fresh.origin
	s.started, s.waitingSince = fresh.started, fresh.waitingSince
	s.ready, s.retryFailed = false, false
	return nil
}

func (s *Startup) retryComplete(failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retrying, s.retryFailed = false, failed
	if failed {
		s.token = ""
	}
}

func (s *Startup) retryStatus() (busy, failed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retrying, s.retryFailed
}

func (s *Startup) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finished && s.stopped != nil {
		close(s.stopped)
	}
	s.finished = true
	s.token = ""
}

func (s *Startup) keepWaiting(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.finished {
		s.waitingSince = now
	}
}

func (s *Startup) status(now time.Time) (show, overdue, finished, ready bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return !s.finished && now.Sub(s.started) >= startupDelay, !s.finished && !s.ready && now.Sub(s.waitingSince) >= startupTimeout, s.finished, s.ready
}

func (s *Startup) diagnostics(now time.Time) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	acknowledgement := "not received"
	if s.ready {
		acknowledgement = "received; visible pixels are not verified"
	}
	return fmt.Sprintf("AkuBrowser startup\r\nLocal HTTP service: started\r\nInterface acknowledgement: %s\r\nManual window retry: pending=%t failed=%t\r\nElapsed: %d seconds\r\nThis does not identify the cause of a blank interface. No automatic restart was attempted.", acknowledgement, s.retrying, s.retryFailed, int(now.Sub(s.started).Seconds()))
}
