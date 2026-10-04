package appshell

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type nativeProtocolFixture struct {
	pages       []nativeReaderTarget
	calls       []string
	navigations []string
	created     int
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
		if args["newWindow"] != true {
			return nil, fmt.Errorf("reader did not request a separate window")
		}
		p.created++
		return json.RawMessage(`{"targetId":"new-reader"}`), nil
	case "Page.navigate":
		p.navigations = append(p.navigations, params.(map[string]string)["url"])
	case "Target.closeTarget":
		return json.RawMessage(`{"success":true}`), nil
	}
	return json.RawMessage(`{}`), nil
}

type nativeContainmentFixture struct{ markers []string }

func (c *nativeContainmentFixture) PrepareReader(context.Context, string) (func(context.Context) error, error) {
	return nil, nil
}
func (c *nativeContainmentFixture) Stop() {}
func (c *nativeContainmentFixture) PrepareBrokerReader(_ context.Context, marker string) (readerbroker.Target, func(context.Context) error, error) {
	c.markers = append(c.markers, marker)
	return readerbroker.Target{HWND: 1, PID: 2, Expires: time.Now().Add(time.Second)}, func(context.Context) error { return nil }, nil
}

func TestNativeReaderReusesOnlyIdleThenOpensIndependentReader(t *testing.T) {
	p := &nativeProtocolFixture{pages: []nativeReaderTarget{{ID: "idle", Type: "page", URL: "http://local/native-reader-idle"}}}
	c := &nativeContainmentFixture{}
	r := &NativeReader{protocol: p, containment: c, idleTarget: "idle", idleURL: p.pages[0].URL}
	for _, id := range []string{"split_first", "split_second"} {
		_, verify, err := r.PrepareNativePost(context.Background(), id, "https://x.com/a/status/1", "http://local/split-reader-intent?id="+id)
		if err != nil || verify == nil {
			t.Fatal("prepare", err)
		}
	}
	if p.created != 1 || !reflect.DeepEqual(c.markers, []string{"AkuBrowser reader split_first", "AkuBrowser reader split_second"}) {
		t.Fatal("reader window/correlation mismatch", p.created, c.markers)
	}
	if err := r.closeIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range p.calls {
		if call == "Target.closeTarget" {
			t.Fatal("retirement closed a post page")
		}
	}
	if !reflect.DeepEqual(p.navigations, []string{"http://local/split-reader-intent?id=split_first", "https://x.com/a/status/1", "http://local/split-reader-intent?id=split_second", "https://x.com/a/status/1"}) {
		t.Fatal("navigation mismatch", p.navigations)
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
