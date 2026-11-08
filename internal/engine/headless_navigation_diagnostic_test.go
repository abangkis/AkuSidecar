package engine

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
)

func TestHeadlessNavigationDiagnosticPersistsOnRun(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node is required for the offline worker fixture")
	}
	binary, err := os.ReadFile(node)
	if err != nil {
		t.Fatal(err)
	}
	worker, err := filepath.Abs(filepath.Join("..", "collection", "headless", "worker", "worker.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pin := filepath.Join(dir, "node.pin.json")
	if err := os.WriteFile(pin, []byte(fmt.Sprintf(`{"protocol":1,"nodeSha256":"%x"}`, sha256.Sum256(binary))), 0600); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(dir, "worker-fixture.mjs")
	if err := os.WriteFile(fixture, []byte(fmt.Sprintf(`
import { pathToFileURL } from 'node:url';
const { runWorker } = await import(pathToFileURL(%q).href);
const replies = [
  {kind:'terminal_redirect_response', errorText:'net::ERR_NAME_NOT_RESOLVED'},
  {kind:'unrelated_requests', errorText:'net::ERR_ABORTED https://private.example/?token=private'},
  {kind:'late_extra_info', errorText:'net::ERR_HTTP_RESPONSE_CODE_FAILURE'},
  {kind:'ambiguous_redirect_extra_info', errorText:'net::ERR_HTTP_RESPONSE_CODE_FAILURE'},
  {kind:'forbidden_response', errorText:'net::ERR_HTTP_RESPONSE_CODE_FAILURE'},
];
let listener;
const emit = (method, params) => listener({method, params});
await runWorker({
  sourceAssetsImpl: async () => [{}, {}, {}],
  launchChromeImpl: async () => ({
    backend: 'headless_worker',
    on(callback) {
      if (listener) throw Error('previous navigation listener was not removed');
      listener = callback;
      return () => { listener = undefined; };
    },
    async navigate(url) {
      if (url !== 'https://x.com/home' || replies.length === 0) throw Error('unexpected fixture navigation');
      const {kind, errorText} = replies.shift();
      const page = {frameId:'main', loaderId:'current'};
      const request = (requestId, type, frameId, loaderId, headers = {}) =>
        emit('Network.requestWillBeSent', {type, requestId, frameId, loaderId, request:{url,headers}});
      const requestExtra = (requestId, headers) => emit('Network.requestWillBeSentExtraInfo', {
        requestId, headers: {...headers, Cookie:'private-cookie', Authorization:'private-auth'},
      });
      const fail = (requestId, type, errorText, signals) =>
        emit('Network.loadingFailed', {type, requestId, errorText, ...signals});
      const extra = (requestId, statusCode) => emit('Network.responseReceivedExtraInfo', {
        requestId, statusCode, headers:{'set-cookie':'private'},
      });
      request('script', 'Script', 'main', 'current');
      fail('script', 'Script', 'net::ERR_BLOCKED_BY_CLIENT');
      request('child', 'Document', 'child-frame', 'current');
      fail('child', 'Document', 'net::ERR_ABORTED');
      request('old', 'Document', 'main', 'old-loader');
      requestExtra('old', {'User-Agent':'HeadlessChrome/999'});
      extra('old', 503);
      fail('old', 'Document', 'net::ERR_CONNECTION_RESET');
      const redirectedUrl = 'https://private.example/?token=private';
      if (kind !== 'unrelated_requests') {
        if (kind === 'forbidden_response') {
          requestExtra('main-request', {'uSeR-aGeNt':'Mozilla/5.0 HeadlessChrome/157.0.0.0',
            'SEC-CH-UA':'"Chromium";v="157", "Not HeadlessChrome";v="99"'});
        }
        request('main-request', 'Document', 'main', 'current',
          kind === 'forbidden_response' || kind === 'ambiguous_redirect_extra_info'
            ? {'User-Agent':'Mozilla/5.0 Chrome/157.0.0.0'} : {});
      }
      if (kind === 'terminal_redirect_response' || kind === 'ambiguous_redirect_extra_info') {
        emit('Network.requestWillBeSent', {...page, type:'Document', requestId:'main-request',
          request:{url:redirectedUrl}, redirectResponse:{status:302}});
        requestExtra('main-request', {'User-Agent':'HeadlessChrome/999'});
        extra('main-request', 302);
      }
      if (kind === 'terminal_redirect_response') {
        emit('Network.responseReceived', {...page, type:'Document', requestId:'main-request',
          response:{url:redirectedUrl,status:403,headers:{'set-cookie':'private'}}});
        setImmediate(() => fail('main-request', 'Document', 'net::ERR_BLOCKED_BY_RESPONSE', {
          canceled:false, blockedReason:'corp-not-same-origin', rawDetail:'private',
        }));
      } else if (kind === 'forbidden_response') {
        emit('Network.responseReceived', {...page, type:'Document', requestId:'main-request',
          response:{url,status:403,headers:{'set-cookie':'private'}}});
        fail('main-request', 'Document', errorText);
      } else if (kind !== 'unrelated_requests') {
        fail('main-request', 'Document', errorText);
        setImmediate(() => extra('main-request', 429));
        if (kind === 'late_extra_info') {
          setImmediate(() => requestExtra('main-request', {'sec-ch-ua':'"HeadlessChrome";v="157"'}));
        }
      }
      return {...page, errorText};
    },
    async close() {},
  }),
});
`, worker)), 0600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	process, err := headless.Launch(ctx, headless.Options{
		Node: node, Pin: pin, Worker: fixture, Chrome: fixture,
		Profile: dir, ProfileDirectory: "Default", BridgePath: dir,
	})
	if process != nil {
		t.Cleanup(process.Terminate)
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		name     string
		category string
		details  map[string]any
	}{
		{"terminal_redirect_response", "dns", map[string]any{
			"chromeErrorCode": "net::ERR_NAME_NOT_RESOLVED",
			"mainDocument": map[string]any{
				"state": "matched", "errorCode": "net::ERR_BLOCKED_BY_RESPONSE",
				"canceled": false, "blockedReason": "corp-not-same-origin",
				"httpStatus": float64(403), "httpStatusSource": "response_received",
			},
		}},
		{"unrelated_requests", "other", map[string]any{"mainDocument": map[string]any{"state": "not_observed"}}},
		{"late_extra_info", "other", map[string]any{
			"chromeErrorCode": "net::ERR_HTTP_RESPONSE_CODE_FAILURE",
			"mainDocument": map[string]any{"state": "matched", "errorCode": "net::ERR_HTTP_RESPONSE_CODE_FAILURE",
				"httpStatus": float64(429), "httpStatusSource": "extra_info",
				"clientHintsHeadless": true, "requestMetadataSource": "request_extra_info"},
		}},
		{"ambiguous_redirect_extra_info", "other", map[string]any{
			"chromeErrorCode": "net::ERR_HTTP_RESPONSE_CODE_FAILURE",
			"mainDocument": map[string]any{"state": "matched", "errorCode": "net::ERR_HTTP_RESPONSE_CODE_FAILURE",
				"httpStatusSource": "not_observed"},
		}},
		{"forbidden_response", "other", map[string]any{
			"chromeErrorCode": "net::ERR_HTTP_RESPONSE_CODE_FAILURE",
			"mainDocument": map[string]any{"state": "matched", "errorCode": "net::ERR_HTTP_RESPONSE_CODE_FAILURE",
				"httpStatus": float64(403), "httpStatusSource": "response_received",
				"userAgentHeadless": true, "clientHintsHeadless": false, "requestMetadataSource": "request_extra_info"},
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			e, state := singleSourceEngine(t, nil)
			manager := attachTestCapture(t, e)
			e.AttachCollectionCoordinator(collection.NewCoordinator(manager, nil, func() error { return nil }))
			settings, err := e.Settings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.CollectionMode = "headless"
			if _, err := e.SaveSettings(ctx, settings); err != nil {
				t.Fatal(err)
			}
			e.ResetCaptureHeartbeat()
			if err := manager.Replace(ctx, func(context.Context, uint64) (captureruntime.Process, error) {
				return &headlessTestProcess{&runtimeTestProcess{done: make(chan error, 1)}}, nil
			}); err != nil {
				t.Fatal(err)
			}
			session, err := e.StartVisibleUpdate(ctx, "offline navigation fixture")
			if err != nil {
				t.Fatal(err)
			}
			run := session.Runs[0]
			command, err := e.claimCommandForCollector(ctx, run.ID, "fixture", "headless", collection.BackendHeadless)
			if err != nil || command == nil {
				t.Fatalf("claim: %+v %v", command, err)
			}
			_, err = process.Capture(ctx, run.Source, command.Payload)
			if err == nil {
				t.Fatal("expected a navigation failure")
			}
			if _, err := e.FailCommand(ctx, command.ID, run.ID, internalCollectorFailure(err, collection.BackendHeadless)); err != nil {
				t.Fatal(err)
			}
			stored, err := state.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{"cdpCommand": "Page.navigate", "chromeErrorCategory": scenario.category}
			for key, value := range scenario.details {
				want[key] = value
			}
			message := "navigation_failed: Chrome could not navigate to the requested source page"
			retryable := scenario.name != "forbidden_response"
			if !retryable {
				message = "Headless navigation to the source was refused (HTTP 403)."
			}
			if stored.BridgeCommandStatus != "failed" || stored.Error == nil || stored.Error.Code != "headless_navigation_failed" ||
				stored.Error.Stage != "capture" || stored.Error.Retryable != retryable || stored.Error.Message != message ||
				!reflect.DeepEqual(stored.Error.Details, want) {
				t.Fatalf("unexpected persisted navigation failure: command=%s error=%+v", stored.BridgeCommandStatus, stored.Error)
			}
		})
	}
	if err := process.CloseForRetry(ctx); err != nil {
		t.Fatal(err)
	}
}
