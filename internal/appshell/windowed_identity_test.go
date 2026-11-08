package appshell

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

type identityProtocol func(string, any, string) (json.RawMessage, error)

func (p identityProtocol) Call(_ context.Context, method string, params any, session string) (json.RawMessage, error) {
	return p(method, params, session)
}

func TestReadWindowedIdentityBindsExactCaptureHost(t *testing.T) {
	const host = "http://127.0.0.1:11122/split-capture-host#fixture"
	var methods []string
	protocol := identityProtocol(func(method string, params any, session string) (json.RawMessage, error) {
		methods = append(methods, method)
		switch method {
		case "Target.getTargets":
			return json.RawMessage(`{"targetInfos":[{"targetId":"source","type":"page","url":"https://x.com/home"},{"targetId":"host","type":"page","url":"` + host + `"}]}`), nil
		case "Browser.getVersion":
			return json.RawMessage(`{"product":"Chrome/157.0.1.2","userAgent":"not-the-page-UA"}`), nil
		case "Target.attachToTarget":
			if params.(map[string]any)["targetId"] != "host" {
				t.Fatal("identity probe attached to the wrong tab")
			}
			return json.RawMessage(`{"sessionId":"scoped"}`), nil
		case "Runtime.evaluate":
			args := params.(map[string]any)
			if session != "scoped" || args["awaitPromise"] != true || strings.Count(args["expression"].(string), host) != 2 {
				t.Fatal("identity probe was not scoped to the same host before and after evaluation")
			}
			return json.RawMessage(`{"result":{"value":{"userAgent":"fixture Chrome/157","userAgentMetadata":{"mobile":false},"ignored":"secret"}}}`), nil
		case "Target.detachFromTarget":
			if params.(map[string]string)["sessionId"] != "scoped" {
				t.Fatal("identity probe did not release its attachment")
			}
			return json.RawMessage(`{}`), nil
		default:
			t.Fatalf("unexpected CDP call: %s", method)
			return nil, nil
		}
	})
	raw, err := ReadWindowedIdentity(context.Background(), protocol, host)
	if err != nil || string(raw) != `{"product":"Chrome/157.0.1.2","userAgent":"fixture Chrome/157","userAgentMetadata":{"mobile":false}}` {
		t.Fatalf("unexpected identity %s, err=%v", raw, err)
	}
	want := []string{"Target.getTargets", "Browser.getVersion", "Target.attachToTarget", "Runtime.evaluate", "Target.detachFromTarget"}
	if !reflect.DeepEqual(methods, want) {
		t.Fatalf("unexpected identity calls: %v", methods)
	}
}
