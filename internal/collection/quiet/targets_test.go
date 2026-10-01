package quiet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type protocolCall struct {
	method  string
	params  any
	session string
}

type fakeProtocol struct {
	calls   []protocolCall
	handler func(protocolCall, int) (json.RawMessage, error)
}

var _ appshell.CaptureProtocol = (*fakeProtocol)(nil)

func (p *fakeProtocol) Call(_ context.Context, method string, params any, session string) (json.RawMessage, error) {
	call := protocolCall{method: method, params: params, session: session}
	p.calls = append(p.calls, call)
	if p.handler == nil {
		return json.RawMessage(`{}`), nil
	}
	return p.handler(call, len(p.calls))
}

func TestTargetsCreateOnePrivateTargetPerSource(t *testing.T) {
	var createIndex int
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		switch call.method {
		case "Target.attachToBrowserTarget":
			return raw(`{"sessionId":"browser-child"}`), nil
		case "Browser.getVersion":
			return raw(`{"product":"Chrome/152.0.0.0"}`), nil
		case "Target.createTarget":
			createIndex++
			return raw(fmt.Sprintf(`{"targetId":"target-%d"}`, createIndex)), nil
		case "Target.attachToTarget":
			return raw(fmt.Sprintf(`{"sessionId":"page-%s"}`, paramString(t, call.params, "targetId"))), nil
		case "Page.enable", "Runtime.enable", "Network.enable", "Emulation.setDeviceMetricsOverride":
			if call.session != "page-target-1" && call.session != "page-target-2" {
				t.Fatalf("page setup used unexpected session %q", call.session)
			}
			return raw(`{}`), nil
		case "Page.navigate":
			return raw(`{"frameId":"frame"}`), nil
		default:
			return nil, fmt.Errorf("unexpected method %s", call.method)
		}
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	version := targets.Version()
	if string(version) != `{"product":"Chrome/152.0.0.0"}` {
		t.Fatalf("Version() = %s", version)
	}
	for _, source := range []domain.Source{domain.SourceX, domain.SourceX, domain.SourceFacebook} {
		if _, err := targets.Call(context.Background(), source, "Page.navigate", map[string]any{"url": "about:blank"}); err != nil {
			t.Fatal(err)
		}
	}
	if createIndex != 2 {
		t.Fatalf("created %d targets, want one per source", createIndex)
	}

	createCalls := callsFor(protocol.calls, "Target.createTarget")
	attachCalls := callsFor(protocol.calls, "Target.attachToTarget")
	if len(createCalls) != 2 || len(attachCalls) != 2 {
		t.Fatalf("create/attach counts = %d/%d", len(createCalls), len(attachCalls))
	}
	for _, method := range []string{"Page.enable", "Runtime.enable", "Network.enable", "Emulation.setDeviceMetricsOverride"} {
		setupCalls := callsFor(protocol.calls, method)
		if len(setupCalls) != 2 {
			t.Fatalf("%s setup calls = %d, want one per source", method, len(setupCalls))
		}
		for _, call := range setupCalls {
			if call.session != "page-target-1" && call.session != "page-target-2" {
				t.Fatalf("%s was not sent to a page session: %q", method, call.session)
			}
		}
	}
	metrics := paramMap(t, callsFor(protocol.calls, "Emulation.setDeviceMetricsOverride")[0].params)
	if metrics["width"] != float64(1280) || metrics["height"] != float64(900) || metrics["deviceScaleFactor"] != float64(1) || metrics["mobile"] != false {
		t.Fatalf("unexpected hidden page metrics: %#v", metrics)
	}
	for i, call := range createCalls {
		if call.session != "browser-child" {
			t.Fatalf("create target %d used session %q", i, call.session)
		}
		params := paramMap(t, call.params)
		if params["url"] != "about:blank" || params["hidden"] != true || params["background"] != true || params["forTab"] != false {
			t.Fatalf("create target %d had unexpected params: %#v", i, params)
		}
		if _, ok := params["newWindow"]; ok {
			t.Fatal("create target requested a native window")
		}
		if _, ok := params["browserContextId"]; ok {
			t.Fatal("create target left the authenticated default context")
		}
		attachParams := paramMap(t, attachCalls[i].params)
		if attachCalls[i].session != "browser-child" || attachParams["flatten"] != true || attachParams["targetId"] != fmt.Sprintf("target-%d", i+1) {
			t.Fatalf("attach target %d was not scoped to its child browser session: %#v", i, attachCalls[i])
		}
	}
	navigations := callsFor(protocol.calls, "Page.navigate")
	if len(navigations) != 3 || navigations[0].session != "page-target-1" || navigations[1].session != "page-target-1" || navigations[2].session != "page-target-2" {
		t.Fatalf("source page sessions were not retained independently: %#v", navigations)
	}
}

func TestTargetsRejectUnknownSourcesAndUnsafeMethodsBeforeCDP(t *testing.T) {
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		if call.method == "Target.attachToBrowserTarget" {
			return raw(`{"sessionId":"browser-child"}`), nil
		}
		if call.method == "Browser.getVersion" {
			return raw(`{"product":"Chrome/152.0.0.0"}`), nil
		}
		return nil, fmt.Errorf("unexpected CDP call %s", call.method)
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	initialCalls := len(protocol.calls)
	for _, request := range []struct {
		source domain.Source
		method string
	}{
		{domain.SourceInstagram, "Page.navigate"},
		{domain.SourceX, "Browser.close"},
		{domain.SourceFacebook, "Target.createTarget"},
		{domain.SourceX, "Target.closeTarget"},
		{domain.SourceFacebook, "Target.getTargets"},
	} {
		if _, err := targets.Call(context.Background(), request.source, request.method, nil); err == nil {
			t.Fatalf("accepted source=%q method=%q", request.source, request.method)
		}
	}
	if len(protocol.calls) != initialCalls {
		t.Fatalf("rejected requests sent CDP commands: before=%d after=%d", initialCalls, len(protocol.calls))
	}
}

func TestTargetsPoisonOnUncertainCreateAndDetachWithoutAdoptingTarget(t *testing.T) {
	createCalls := 0
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		switch call.method {
		case "Target.attachToBrowserTarget":
			return raw(`{"sessionId":"browser-child"}`), nil
		case "Browser.getVersion":
			return raw(`{"product":"Chrome/152.0.0.0"}`), nil
		case "Target.createTarget":
			createCalls++
			return nil, errors.New("response lost after dispatch")
		case "Target.detachFromTarget":
			if call.session != "" || paramString(t, call.params, "sessionId") != "browser-child" {
				t.Fatal("detached the wrong session")
			}
			return raw(`{}`), nil
		default:
			t.Fatalf("unsafe or unexpected cleanup command: %s", call.method)
			return nil, errors.New("unexpected command")
		}
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targets.Call(context.Background(), domain.SourceX, "Page.navigate", nil); err == nil {
		t.Fatal("uncertain target creation was accepted")
	}
	if _, err := targets.Call(context.Background(), domain.SourceFacebook, "Page.navigate", nil); !errors.Is(err, errPoisoned) {
		t.Fatalf("poisoned broker allowed another source creation: %v", err)
	}
	if createCalls != 1 {
		t.Fatalf("uncertain creation was retried %d times", createCalls)
	}
	if err := targets.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := targets.Close(context.Background()); err != nil {
		t.Fatalf("successful cleanup was not idempotent: %v", err)
	}
	if len(callsFor(protocol.calls, "Target.closeTarget")) != 0 || len(callsFor(protocol.calls, "Target.getTargets")) != 0 || len(callsFor(protocol.calls, "Target.detachFromTarget")) != 1 {
		t.Fatal("uncertain creation adopted or targeted an arbitrary target during cleanup")
	}
}

func TestTargetsClosePollsUntilExactOwnedTargetDisappears(t *testing.T) {
	closeCalls, listingCalls := 0, 0
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		switch call.method {
		case "Target.attachToBrowserTarget":
			return raw(`{"sessionId":"browser-child"}`), nil
		case "Browser.getVersion":
			return raw(`{"product":"Chrome/152.0.0.0"}`), nil
		case "Target.createTarget":
			return raw(`{"targetId":"owned-x"}`), nil
		case "Target.attachToTarget":
			return raw(`{"sessionId":"page-x"}`), nil
		case "Page.enable", "Runtime.enable", "Network.enable", "Emulation.setDeviceMetricsOverride":
			return raw(`{}`), nil
		case "Page.navigate":
			return raw(`{"frameId":"frame"}`), nil
		case "Target.closeTarget":
			closeCalls++
			if call.session != "browser-child" || paramString(t, call.params, "targetId") != "owned-x" {
				t.Fatal("cleanup targeted a target outside the broker ownership map")
			}
			return raw(`{"success":true}`), nil
		case "Target.getTargets":
			listingCalls++
			if call.session != "browser-child" {
				t.Fatal("target absence was checked outside the child browser session")
			}
			if listingCalls < 4 {
				return raw(`{"targetInfos":[{"targetId":"owned-x"},{"targetId":"unowned-visible-target"}]}`), nil
			}
			return raw(`{"targetInfos":[{"targetId":"unowned-visible-target"}]}`), nil
		case "Target.detachFromTarget":
			if call.session != "" || paramString(t, call.params, "sessionId") != "browser-child" {
				t.Fatal("detached the wrong browser session")
			}
			return raw(`{}`), nil
		default:
			t.Fatalf("unexpected protocol command %s", call.method)
			return nil, errors.New("unexpected command")
		}
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targets.Call(context.Background(), domain.SourceX, "Page.navigate", nil); err != nil {
		t.Fatal(err)
	}
	if err := targets.Close(context.Background()); err != nil {
		t.Fatalf("bounded target absence polling failed: %v", err)
	}
	if closeCalls != 1 || listingCalls != 4 || len(callsFor(protocol.calls, "Target.detachFromTarget")) != 1 {
		t.Fatalf("cleanup retry counts close/list/detach=%d/%d/%d", closeCalls, listingCalls, len(callsFor(protocol.calls, "Target.detachFromTarget")))
	}
	for _, call := range callsFor(protocol.calls, "Target.closeTarget") {
		if paramString(t, call.params, "targetId") != "owned-x" {
			t.Fatal("cleanup closed an unowned target from the listing")
		}
	}
}

func TestTargetsCloseReturnsCallerCancellationAndCanRetry(t *testing.T) {
	listingCalls := 0
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		switch call.method {
		case "Target.attachToBrowserTarget":
			return raw(`{"sessionId":"browser-child"}`), nil
		case "Browser.getVersion":
			return raw(`{"product":"Chrome/152.0.0.0"}`), nil
		case "Target.createTarget":
			return raw(`{"targetId":"owned-cancel"}`), nil
		case "Target.attachToTarget":
			return raw(`{"sessionId":"page-cancel"}`), nil
		case "Page.enable", "Runtime.enable", "Network.enable", "Emulation.setDeviceMetricsOverride":
			return raw(`{}`), nil
		case "Page.navigate":
			return raw(`{"frameId":"frame"}`), nil
		case "Target.closeTarget":
			if paramString(t, call.params, "targetId") != "owned-cancel" {
				t.Fatal("cleanup targeted a target outside the broker ownership map")
			}
			return raw(`{"success":true}`), nil
		case "Target.getTargets":
			listingCalls++
			if listingCalls == 1 {
				return raw(`{"targetInfos":[{"targetId":"owned-cancel"}]}`), nil
			}
			return raw(`{"targetInfos":[]}`), nil
		case "Target.detachFromTarget":
			return raw(`{}`), nil
		default:
			t.Fatalf("unexpected command %s", call.method)
			return nil, errors.New("unexpected command")
		}
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targets.Call(context.Background(), domain.SourceX, "Page.navigate", nil); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := targets.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() = %v, want caller deadline", err)
	}
	if len(callsFor(protocol.calls, "Target.detachFromTarget")) != 0 {
		t.Fatal("child browser session detached before owned target absence was verified")
	}
	if err := targets.Close(context.Background()); err != nil {
		t.Fatalf("cleanup did not remain retryable after caller cancellation: %v", err)
	}
}

func TestTargetsCloseKnownTargetAfterAttachmentFailure(t *testing.T) {
	closedTarget := ""
	protocol := &fakeProtocol{handler: func(call protocolCall, _ int) (json.RawMessage, error) {
		switch call.method {
		case "Target.attachToBrowserTarget":
			return raw(`{"sessionId":"browser-child"}`), nil
		case "Browser.getVersion":
			return raw(`{"product":"Chrome/152.0.0.0"}`), nil
		case "Target.createTarget":
			return raw(`{"targetId":"owned-fb"}`), nil
		case "Target.attachToTarget":
			return raw(`{"sessionId":"page-fb"}`), nil
		case "Page.enable":
			return nil, errors.New("page setup response uncertain")
		case "Target.closeTarget":
			closedTarget = paramString(t, call.params, "targetId")
			return raw(`{"success":true}`), nil
		case "Target.getTargets":
			return raw(`{"targetInfos":[]}`), nil
		case "Target.detachFromTarget":
			return raw(`{}`), nil
		default:
			t.Fatalf("unexpected command %s", call.method)
			return nil, errors.New("unexpected command")
		}
	}}
	targets, err := NewTargets(context.Background(), protocol)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := targets.Call(context.Background(), domain.SourceFacebook, "Page.navigate", nil); err == nil {
		t.Fatal("uncertain page setup was accepted")
	}
	if err := targets.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if closedTarget != "owned-fb" {
		t.Fatalf("cleanup closed target %q, want exact owned ID", closedTarget)
	}
}

func raw(value string) json.RawMessage { return json.RawMessage(value) }

func callsFor(calls []protocolCall, method string) []protocolCall {
	result := make([]protocolCall, 0)
	for _, call := range calls {
		if call.method == method {
			result = append(result, call)
		}
	}
	return result
}

func paramMap(t *testing.T, params any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func paramString(t *testing.T, params any, key string) string {
	t.Helper()
	return fmt.Sprint(paramMap(t, params)[key])
}
