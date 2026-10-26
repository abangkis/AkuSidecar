//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type applicationPostFixture struct {
	now       time.Time
	ops       []string
	changed   bool
	pid       uint32
	malformed bool
	verify    atomic.Int32
	release   atomic.Int32
}

func (f *applicationPostFixture) ports() xApplicationPostPorts {
	return xApplicationPostPorts{readerPID: 123, now: func() time.Time { return f.now },
		check: func(context.Context) error {
			if f.changed {
				return errors.New("PRIVATE_OWNER_ERROR")
			}
			return nil
		},
		bind: func(_ context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
			return readerbroker.Target{HWND: 10, PID: f.pid, Value: 7, Property: "owned", Action: strings.TrimPrefix(marker, "AkuBrowser reader "), Expires: f.now.Add(readerbroker.Lifetime)}, func(context.Context) error { f.verify.Add(1); return nil }, nil
		},
		release: func(readerbroker.Target) { f.release.Add(1) },
		call: func(_ context.Context, op string, fields map[string]any, out *json.RawMessage) error {
			f.ops = append(f.ops, op)
			switch op {
			case "manual_prepare_post":
				if f.malformed {
					*out = json.RawMessage(`{"handle":"wrong","targetId":"post","windowId":2,"expiresMS":1,"canAuthorize":true}`)
					return nil
				}
				*out, _ = json.Marshal(map[string]any{"handle": strings.Repeat("a", 48), "targetId": "post", "windowId": 2, "expiresMS": f.now.Add(readerbroker.PreparationLifetime).UnixMilli()})
			case "manual_complete_post":
				*out = json.RawMessage(`{"navigated":true}`)
			case "manual_cancel_post":
				*out = json.RawMessage(`{"retired":true}`)
			default:
				return errors.New("UNEXPECTED_RAW_OPERATION")
			}
			return nil
		},
	}
}
func applicationPostRequest() readerbroker.Request {
	return readerbroker.Request{RequestID: "broker_" + strings.Repeat("a", 32), Source: "x", URL: "https://x.com/example/status/123"}
}
func applicationPostMarker() string {
	return "http://127.0.0.1:12345/split-reader-intent?id=split_" + strings.Repeat("b", 32)
}

func TestXApplicationPostFixedOriginAndCanonicalRequest(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:11122", "http://localhost:12345", "https://127.0.0.1:12345", "http://127.0.0.1:12345/", "http://127.0.0.1:12345?", "http://127.0.0.1:12345?secret=PRIVATE"} {
		if _, err := newXApplicationPostPreparer(origin); err == nil {
			t.Fatal("unbounded origin accepted")
		}
	}
	if p, err := newXApplicationPostPreparer("http://127.0.0.1:12345"); err != nil || p == nil {
		t.Fatal("fixed origin rejected")
	}
	for _, badURL := range []string{"https://x.com/home", "https://www.x.com/example/status/123", "https://x.com/example/status/123?private=value", "https://x.com/example/status/123/photo/1"} {
		f := &applicationPostFixture{now: time.Now(), pid: 123}
		req := applicationPostRequest()
		req.URL = badURL
		if _, err := prepareXApplicationPost(context.Background(), req, applicationPostMarker(), f.ports()); err == nil || len(f.ops) != 0 {
			t.Fatal("invalid request reached source")
		}
	}
}
func TestXApplicationPostOwnedBindingAndOneUseCompletion(t *testing.T) {
	f := &applicationPostFixture{now: time.Now(), pid: 123}
	prepared, err := prepareXApplicationPost(context.Background(), applicationPostRequest(), applicationPostMarker(), f.ports())
	if err != nil || prepared.Target.PID != 123 {
		t.Fatal("synthetic owned marker rejected")
	}
	if prepared.VerifyForeground(context.Background()) != nil || prepared.VerifyForeground(context.Background()) == nil || f.verify.Load() != 1 {
		t.Fatal("foreground verification replayed")
	}
	if prepared.NavigatePost(context.Background()) != nil || prepared.NavigatePost(context.Background()) == nil {
		t.Fatal("post completion replayed")
	}
	if strings.Join(f.ops, ",") != "manual_prepare_post,manual_complete_post" || f.release.Load() != 1 {
		t.Fatal("unexpected post operation or property leak")
	}
}
func TestXApplicationPostMismatchExpiryAndPartialCleanup(t *testing.T) {
	for _, fault := range []string{"pid", "malformed", "changed", "expired", "canceled"} {
		t.Run(fault, func(t *testing.T) {
			f := &applicationPostFixture{now: time.Now(), pid: 123}
			if fault == "pid" {
				f.pid = 124
			}
			f.malformed = fault == "malformed"
			prepared, err := prepareXApplicationPost(context.Background(), applicationPostRequest(), applicationPostMarker(), f.ports())
			if fault == "pid" || fault == "malformed" {
				if err == nil || f.ops[len(f.ops)-1] != "manual_cancel_post" {
					t.Fatal("invalid partial source not retired")
				}
				return
			}
			if err != nil {
				t.Fatal("synthetic preparation failed")
			}
			if prepared.VerifyForeground(context.Background()) != nil {
				t.Fatal("synthetic foreground failed before fault")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if fault == "changed" {
				f.changed = true
			}
			if fault == "expired" {
				f.now = f.now.Add(readerbroker.Lifetime)
			}
			if fault == "canceled" {
				cancel()
			}
			if err = prepared.NavigatePost(ctx); err == nil || strings.Contains(err.Error(), "PRIVATE") {
				t.Fatal("unsafe completion admitted or raw diagnostic leaked")
			}
			if strings.Join(f.ops, ",") != "manual_prepare_post,manual_cancel_post" {
				t.Fatal("late operation navigated or closed a user window")
			}
		})
	}
}

func TestXApplicationPostRequiresSuccessfulForegroundAndRetiresFailure(t *testing.T) {
	for _, fault := range []string{"unverified", "foreground-failed", "rpc-failed", "reply-failed"} {
		t.Run(fault, func(t *testing.T) {
			f := &applicationPostFixture{now: time.Now(), pid: 123}
			p := f.ports()
			bind := p.bind
			p.bind = func(ctx context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
				target, verify, err := bind(ctx, marker)
				if fault == "foreground-failed" {
					verify = func(context.Context) error { return errors.New("private foreground failure") }
				}
				return target, verify, err
			}
			call := p.call
			p.call = func(ctx context.Context, op string, fields map[string]any, out *json.RawMessage) error {
				if err := call(ctx, op, fields, out); err != nil {
					return err
				}
				if op == "manual_complete_post" {
					if fault == "rpc-failed" {
						return errors.New("private transport failure")
					}
					if fault == "reply-failed" {
						*out = json.RawMessage(`{"navigated":false}`)
					}
				}
				return nil
			}
			prepared, err := prepareXApplicationPost(context.Background(), applicationPostRequest(), applicationPostMarker(), p)
			if err != nil {
				t.Fatal(err)
			}
			if fault != "unverified" {
				err = prepared.VerifyForeground(context.Background())
				if (err != nil) != (fault == "foreground-failed") {
					t.Fatal("incorrect foreground result")
				}
			}
			if err = prepared.NavigatePost(context.Background()); err == nil || strings.Contains(err.Error(), "private") {
				t.Fatal("unsafe completion accepted or leaked private error")
			}
			want := "manual_prepare_post,manual_cancel_post"
			if fault == "rpc-failed" || fault == "reply-failed" {
				want = "manual_prepare_post,manual_complete_post,manual_cancel_post"
			}
			if strings.Join(f.ops, ",") != want || f.release.Load() != 1 {
				t.Fatal("navigation bypass or pending capability leak")
			}
		})
	}
}
