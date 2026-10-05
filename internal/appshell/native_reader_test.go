package appshell

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type nativeProtocolFixture struct {
	pages           []nativeReaderTarget
	calls           []string
	navigations     []string
	created         int
	createdWindowID int
}

func (p *nativeProtocolFixture) Call(_ context.Context, method string, params any, _ string) (json.RawMessage, error) {
	p.calls = append(p.calls, method)
	switch method {
	case "Target.getTargets":
		return json.Marshal(map[string]any{"targetInfos": p.pages})
	case "Target.attachToTarget":
		return json.RawMessage(`{"sessionId":"owned-session"}`), nil
	case "Target.createTarget":
		args := params.(map[string]any)
		if args["newWindow"] != false {
			return nil, fmt.Errorf("reader did not request a tab in the existing window")
		}
		p.created++
		return json.Marshal(map[string]string{"targetId": fmt.Sprintf("new-reader-%d", p.created)})
	case "Browser.getWindowForTarget":
		windowID := 10
		if params.(map[string]string)["targetId"] != "idle" && p.createdWindowID != 0 {
			windowID = p.createdWindowID
		}
		return json.Marshal(map[string]int{"windowId": windowID})
	case "Page.navigate":
		p.navigations = append(p.navigations, params.(map[string]string)["url"])
	case "Target.closeTarget":
		return json.RawMessage(`{"success":true}`), nil
	}
	return json.RawMessage(`{}`), nil
}

type nativeContainmentFixture struct {
	markers           []string
	verificationError error
	closeErr          error
	closeCalls        int
	onClose           func()
}

func (c *nativeContainmentFixture) PrepareReader(context.Context, string) (func(context.Context) error, error) {
	return nil, nil
}
func (c *nativeContainmentFixture) Stop() {}
func (c *nativeContainmentFixture) CloseReaderWindow(_ context.Context, hwnd uintptr) error {
	if hwnd != 1 {
		return errors.New("wrong HWND")
	}
	c.closeCalls++
	if c.closeErr != nil {
		return c.closeErr
	}
	if c.onClose != nil {
		c.onClose()
	}
	return nil
}
func (c *nativeContainmentFixture) PrepareBrokerReader(_ context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
	c.markers = append(c.markers, marker)
	return readerbroker.Target{HWND: 1, PID: 2, Expires: time.Now().Add(time.Second)}, func(context.Context) error { return c.verificationError }, nil
}

func TestNativeReaderRejectedActivationNeverNavigatesSocialURL(t *testing.T) {
	p := &nativeProtocolFixture{pages: []nativeReaderTarget{{ID: "idle", Type: "page", URL: "http://local/native-reader-idle"}}}
	c := &nativeContainmentFixture{verificationError: errors.New("not foreground")}
	r := &NativeReader{protocol: p, containment: c, idleTarget: "idle", idleURL: p.pages[0].URL}
	_, verify, err := r.PrepareNativePost(context.Background(), "split_one", "https://x.com/a/status/1", "http://local/marker")
	if err != nil {
		t.Fatal(err)
	}
	if err := verify(context.Background()); err == nil {
		t.Fatal("failed foreground accepted")
	}
	if !reflect.DeepEqual(p.navigations, []string{"http://local/marker"}) {
		t.Fatal("unverified activation navigated post", p.navigations)
	}
}

func TestNativeReaderReusesIdleThenCreatesTabsInSameWindow(t *testing.T) {
	p := &nativeProtocolFixture{pages: []nativeReaderTarget{{ID: "idle", Type: "page", URL: "http://local/native-reader-idle"}}}
	c := &nativeContainmentFixture{}
	r := &NativeReader{protocol: p, containment: c, idleTarget: "idle", idleURL: p.pages[0].URL}
	var timing bytes.Buffer
	r.logger = log.New(&timing, "", 0)
	for _, id := range []string{"split_first", "split_second", "split_third"} {
		_, verify, err := r.PrepareNativePost(context.Background(), id, "https://x.com/a/status/1", "http://local/split-reader-intent?id="+id)
		if err != nil || verify == nil {
			t.Fatal("prepare", err)
		}
		if len(p.navigations) == 0 || strings.HasPrefix(p.navigations[len(p.navigations)-1], "https://") {
			t.Fatal("social navigation occurred before activation")
		}
		if err := verify(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := len(p.navigations)
		if err := verify(context.Background()); err != nil || len(p.navigations) != before {
			t.Fatal("verification replayed navigation", err)
		}
	}
	if p.created != 2 || !reflect.DeepEqual(c.markers, []string{"AkuBrowser reader split_first", "AkuBrowser reader split_second", "AkuBrowser reader split_third"}) {
		t.Fatal("reader window/correlation mismatch", p.created, c.markers)
	}
	for _, stage := range []string{"target_attach", "marker_navigation", "window_binding", "post_navigation_dispatch"} {
		if !strings.Contains(timing.String(), "stage="+stage) {
			t.Fatalf("missing timing %s", stage)
		}
	}
	if strings.Contains(timing.String(), "https://") || strings.Contains(timing.String(), "http://") {
		t.Fatal("timing leaked a URL")
	}
	if err := r.closeIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range p.calls {
		if call == "Target.closeTarget" {
			t.Fatal("retirement closed a post page")
		}
	}
	if !reflect.DeepEqual(p.navigations, []string{"http://local/split-reader-intent?id=split_first", "https://x.com/a/status/1", "http://local/split-reader-intent?id=split_second", "https://x.com/a/status/1", "http://local/split-reader-intent?id=split_third", "https://x.com/a/status/1"}) {
		t.Fatal("navigation mismatch", p.navigations)
	}
}

func TestNativeReaderRejectsTabInDifferentWindow(t *testing.T) {
	p := &nativeProtocolFixture{createdWindowID: 99}
	windowID := 10
	r := &NativeReader{protocol: p, containment: &nativeContainmentFixture{}, readerWindowID: &windowID}
	_, verify, err := r.PrepareNativePost(context.Background(), "split_wrong", "https://x.com/a/status/1", "http://local/marker")
	if err == nil || verify != nil || len(p.navigations) != 0 {
		t.Fatal("unverified window was admitted or navigated", p.navigations, err)
	}
}

func TestNativeReaderChangedPlaceholderIsNeverNavigatedOrClosed(t *testing.T) {
	for _, operation := range []string{"prepare", "retire"} {
		p := &nativeProtocolFixture{pages: []nativeReaderTarget{{ID: "idle", Type: "page", URL: "https://x.com/user/page"}}}
		r := &NativeReader{protocol: p, idleTarget: "idle", idleURL: "http://local/native-reader-idle"}
		var err error
		if operation == "prepare" {
			_, _, err = r.PrepareNativePost(context.Background(), "split_test", "https://x.com/a/status/1", "http://local/marker")
		} else {
			err = r.closeIdle(context.Background())
		}
		if err == nil || len(p.calls) != 1 || p.calls[0] != "Target.getTargets" {
			t.Fatal("changed page was modified", operation, p.calls, err)
		}
	}
}

func TestNativeReaderCloseRequiresExactOwnedPagesAndNaturalExit(t *testing.T) {
	window := &Window{closed: make(chan struct{})}
	c := &nativeContainmentFixture{}
	p := &nativeProtocolFixture{createdWindowID: 10, pages: []nativeReaderTarget{
		{ID: "post", Type: "page", URL: "https://x.com/a/status/1"},
		{ID: "personal", Type: "page", URL: "https://accounts.example/login"},
	}}
	id := 10
	r := &NativeReader{Window: window, protocol: p, containment: c, readerWindowID: &id, readerWindowHWND: 1, readerTargets: map[string]string{"post": "https://x.com/a/status/1"}}
	if err := r.CloseOwnedWindow(context.Background()); err == nil || c.closeCalls != 0 {
		t.Fatal("close reached an HWND while its window contained an unknown page", err, c.closeCalls)
	}
	p.pages = p.pages[:1]
	c.closeErr = errors.New("WM_CLOSE rejected")
	if err := r.CloseOwnedWindow(context.Background()); err == nil || c.closeCalls != 1 {
		t.Fatal("close failure was hidden", err, c.closeCalls)
	}
	c.closeErr = nil
	c.onClose = func() { close(window.closed) }
	if err := r.CloseOwnedWindow(context.Background()); err != nil || c.closeCalls != 2 {
		t.Fatal("owned reader did not wait for its natural process exit", err, c.closeCalls)
	}
}
