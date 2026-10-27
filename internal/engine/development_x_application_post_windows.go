//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
	"golang.org/x/sys/windows"
)

var xApplicationPostURL = regexp.MustCompile(`^https://x\.com/[A-Za-z0-9_]{1,15}/status/[0-9]{1,20}$`)
var xApplicationMarkerPath = regexp.MustCompile(`^/split-reader-intent\?id=split_[a-f0-9]{32}$`)
var xApplicationPostHandle = regexp.MustCompile(`^[a-f0-9]{48}$`)

func xApplicationPostPort(value string) bool {
	n, err := strconv.Atoi(value)
	return err == nil && n > 0 && n <= 65535 && n != 11122 && strconv.Itoa(n) == value
}

// Fixed local-origin policy belongs to trusted QA/application composition. A
// caller supplies only the existing split marker's relative path.
func newXApplicationPostPreparer(origin string) (xApplicationPostPreparer, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || !xApplicationPostPort(u.Port()) || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.String() != origin {
		return nil, errors.New("manual reader origin rejected")
	}
	return func(ctx context.Context, role xManualPilotWindowsRole, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
		if !xApplicationMarkerPath.MatchString(marker) {
			return readerbroker.NativePostPreparation{}, errors.New("manual reader marker rejected")
		}
		return xApplicationPreparePost(ctx, role, req, origin+marker)
	}, nil
}

type xApplicationPostPorts struct {
	allowMainOrigin bool // Trusted daily composition only; manual defaults remain unchanged.
	readerPID       uint32
	check           func(context.Context) error
	call            func(context.Context, string, map[string]any, *json.RawMessage) error
	bind            func(context.Context, string) (readerbroker.Target, func(context.Context) error, error)
	waitForeground  func(context.Context, string, func()) error
	release         func(readerbroker.Target)
	now             func() time.Time
}

// This entry borrows an already pinned reader and the production HWND marker
// binder. It launches no process and cannot navigate the original bootstrap.
func xApplicationPreparePost(ctx context.Context, role xManualPilotWindowsRole, req readerbroker.Request, marker string) (readerbroker.NativePostPreparation, error) {
	b, ok := role.bridge.(*xApplicationOwner)
	if !ok || role.role != "reader" || ctx == nil || ctx.Err() != nil || b.isClosed() {
		return readerbroker.NativePostPreparation{}, errors.New("manual reader owner unavailable")
	}
	b.initializationMu.Lock()
	seeded := b.seedReady && role.protocolOwner == b.protocolOwner
	b.initializationMu.Unlock()
	if !seeded {
		return readerbroker.NativePostPreparation{}, errors.New("manual reader seed unavailable")
	}
	b.postMu.Lock()
	if b.postRequests == nil {
		b.postRequests = map[string]bool{}
	}
	if b.postRequests[req.RequestID] || len(b.postRequests) >= 32 {
		b.postMu.Unlock()
		return readerbroker.NativePostPreparation{}, errors.New("manual reader request replayed")
	}
	b.postRequests[req.RequestID] = true
	if b.postBinding == nil {
		err := b.nativeManualJob(func(job windows.Handle) error {
			var err error
			b.postBinding, err = appshell.NewOwnedReaderBinding(job, role.chrome.PID)
			return err
		})
		if err != nil {
			b.postMu.Unlock()
			return readerbroker.NativePostPreparation{}, errors.New("manual reader binding unavailable")
		}
	}
	binding := b.postBinding
	b.postMu.Unlock()
	backend := &xManualPilotWindowsBackend{roles: map[string]xManualPilotWindowsRole{"reader": role}}
	ports := xApplicationPostPorts{readerPID: role.chrome.PID, now: time.Now, bind: binding.Prepare, waitForeground: binding.WaitForForeground, release: binding.Release,
		check: func(call context.Context) error { _, err := backend.snapshot(call, "reader"); return err },
		call: func(call context.Context, op string, fields map[string]any, out *json.RawMessage) error {
			fields["owner"] = role.protocolOwner
			reply, err := b.pipe.Call(call, op, fields, nil)
			if err != nil || reply.Code != "" {
				clear(reply.Result)
				return errors.New("manual reader operation unavailable")
			}
			*out = reply.Result
			return nil
		},
	}
	return prepareXApplicationPost(ctx, req, marker, ports)
}

func prepareXApplicationPost(ctx context.Context, req readerbroker.Request, marker string, p xApplicationPostPorts) (readerbroker.NativePostPreparation, error) {
	bad := func() (readerbroker.NativePostPreparation, error) {
		return readerbroker.NativePostPreparation{}, errors.New("manual reader preparation rejected")
	}
	if ctx == nil || ctx.Err() != nil || req.Validate() != nil || req.Source != "x" || !xApplicationPostURL.MatchString(req.URL) || p.readerPID == 0 || p.check == nil || p.call == nil || p.bind == nil || p.release == nil || p.now == nil {
		return bad()
	}
	u, err := url.Parse(marker)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || !(xApplicationPostPort(u.Port()) || p.allowMainOrigin && u.Port() == "11122") || u.User != nil || u.Fragment != "" || !xApplicationMarkerPath.MatchString(u.RequestURI()) || u.String() != marker {
		return bad()
	}
	call, cancel := context.WithTimeout(ctx, readerbroker.PreparationLifetime)
	defer cancel()
	if p.check(call) != nil {
		return bad()
	}
	var handle any
	var retireOnce sync.Once
	retire := func() {
		retireOnce.Do(func() {
			cleanup, done := context.WithTimeout(context.Background(), nativePilotCaptureCleanupTimeout)
			defer done()
			var out json.RawMessage
			_ = p.call(cleanup, "manual_cancel_post", map[string]any{"handle": handle, "requestId": req.RequestID}, &out)
			clear(out)
		})
	}
	success := false
	defer func() {
		if !success {
			retire()
		}
	}()
	var raw json.RawMessage
	if p.call(call, "manual_prepare_post", map[string]any{"request": req, "marker": marker}, &raw) != nil {
		return bad()
	}
	defer clear(raw)
	var ready struct {
		Handle    string `json:"handle"`
		TargetID  string `json:"targetId"`
		WindowID  int    `json:"windowId"`
		ExpiresMS int64  `json:"expiresMS"`
	}
	if xManualPilotDecode(raw, &ready) != nil || !xApplicationPostHandle.MatchString(ready.Handle) || !nativePilotRef(ready.TargetID, 512) || ready.WindowID < 1 {
		return bad()
	}
	handle = ready.Handle
	expires := time.UnixMilli(ready.ExpiresMS)
	if !p.now().Before(expires) || expires.Sub(p.now()) > readerbroker.PreparationLifetime {
		return bad()
	}
	if p.check(call) != nil || call.Err() != nil {
		return bad()
	}
	action := u.Query().Get("id")
	if p.waitForeground != nil {
		// Human reaction time belongs to preparation, not the five-second ticket.
		wait, stop := context.WithDeadline(call, expires)
		err := p.waitForeground(wait, "AkuBrowser reader "+action, func() { xQAMarkReaderReady(ctx) })
		stop()
		if err != nil || p.check(call) != nil || call.Err() != nil || expires.Sub(p.now()) < readerbroker.Lifetime {
			return bad()
		}
		if limit, ok := call.Deadline(); ok && time.Until(limit) < readerbroker.Lifetime {
			return bad()
		}
	}
	target, verify, err := p.bind(call, "AkuBrowser reader "+action)
	defer func() {
		if !success && target.HWND != 0 {
			p.release(target)
		}
	}()
	if err != nil || verify == nil || target.HWND == 0 || target.PID != p.readerPID || target.Value == 0 || target.Action != action || !p.now().Before(target.Expires) || target.Expires.Sub(p.now()) > readerbroker.Lifetime {
		return bad()
	}
	if target.Expires.After(expires) {
		return bad()
	}
	if p.check(call) != nil || call.Err() != nil {
		return bad()
	}
	var mu sync.Mutex
	verificationAttempted, foregroundConfirmed, navigated := false, false, false
	timer := time.AfterFunc(max(target.Expires.Sub(p.now()), 0), func() { p.release(target); retire() })
	check := func(active context.Context) error {
		if active == nil || active.Err() != nil || !p.now().Before(target.Expires) || !p.now().Before(expires) {
			return errors.New("manual reader capability expired")
		}
		if p.check(active) != nil {
			return errors.New("manual reader owner changed")
		}
		return nil
	}
	prepared := readerbroker.NativePostPreparation{Target: target,
		VerifyForeground: func(active context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			if verificationAttempted {
				return errors.New("manual reader foreground replayed")
			}
			verificationAttempted = true
			if err := check(active); err != nil {
				return err
			}
			if verify(active) != nil {
				return errors.New("manual reader foreground unverified")
			}
			foregroundConfirmed = true
			return nil
		},
		NavigatePost: func(active context.Context) error {
			mu.Lock()
			defer mu.Unlock()
			if navigated {
				return errors.New("manual reader navigation replayed")
			}
			navigated = true
			defer timer.Stop()
			defer p.release(target)
			accepted := false
			defer func() {
				if !accepted {
					retire()
				}
			}()
			if !foregroundConfirmed {
				return errors.New("manual reader foreground required")
			}
			if err := check(active); err != nil {
				retire()
				return err
			}
			var out json.RawMessage
			defer func() { clear(out) }()
			if p.call(active, "manual_complete_post", map[string]any{"handle": ready.Handle, "requestId": req.RequestID}, &out) != nil {
				return errors.New("manual reader navigation unavailable")
			}
			var completed struct {
				Navigated bool `json:"navigated"`
			}
			if xManualPilotDecode(out, &completed) != nil || !completed.Navigated || check(active) != nil {
				return errors.New("manual reader navigation rejected")
			}
			accepted = true
			return nil
		},
	}
	success = true
	return prepared, nil
}
