//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type nativeMainOfflineProtocol struct {
	raw []byte
	err error
}

func TestInstagramNativeRetryReceiptValidation(t *testing.T) {
	const valid = `{"rearmed":true,"retryCount":1,"retryLimit":2,"descriptor":{"source":"instagram","recipe":"instagram-native-v1","readerOwnerID":"123","readerProfileID":"fixture-profile"}}`
	tests := map[string]string{
		"valid":              valid,
		"missing descriptor": `{"rearmed":true,"retryCount":1,"retryLimit":2}`,
		"wrong source":       strings.Replace(valid, `"source":"instagram"`, `"source":"x"`, 1),
		"wrong owner":        strings.Replace(valid, `"readerOwnerID":"123"`, `"readerOwnerID":"124"`, 1),
		"wrong profile":      strings.Replace(valid, `"fixture-profile"`, `"other-profile"`, 1),
		"wrong recipe":       strings.Replace(valid, `instagram-native-v1`, `other`, 1),
		"wrong count":        strings.Replace(valid, `"retryCount":1`, `"retryCount":3`, 1),
		"wrong limit":        strings.Replace(valid, `"retryLimit":2`, `"retryLimit":3`, 1),
		"duplicate":          strings.Replace(valid, `"rearmed":true`, `"rearmed":true,"rearmed":true`, 1),
		"extra field":        strings.Replace(valid, `"rearmed":true`, `"rearmed":true,"extra":1`, 1),
		"null":               `null`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			transport := &instagramMainNativeTransport{protocol: nativeMainOfflineProtocol{raw: []byte(raw)}}
			transport.reader.chrome.PID = 123
			transport.reader.profile = "fixture-profile"
			var reply instagramRetryReceipt
			err := transport.call(context.Background(), "source_retry", nil, &reply)
			if err == nil {
				err = transport.validateRetryReceipt(reply, 1)
			}
			if (err == nil) != (name == "valid") {
				t.Fatalf("receipt validation: %v", err)
			}
		})
	}
}

func TestInstagramNativeRetryNoDispatchClassification(t *testing.T) {
	ctx := context.Background()
	for _, test := range []struct {
		name string
		err  error
		want bool
	}{
		{"typed", &appshell.MainBrowserError{Code: "main_ig_retry_not_dispatched"}, true},
		{"generic", &appshell.MainBrowserError{Code: "main_transport_rejected"}, false},
		{"text", errors.New("main_ig_retry_not_dispatched"), false},
		{"disconnect", errors.New("disconnected"), false},
	} {
		t.Run(test.name, func(t *testing.T) {
			transport := &instagramMainNativeTransport{protocol: nativeMainOfflineProtocol{err: test.err}}
			err := transport.call(ctx, "source_retry", nil, nil)
			if instagramRetryDefinitelyNotDispatched(ctx, err) != test.want {
				t.Fatalf("classification: %v", err)
			}
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if instagramRetryDefinitelyNotDispatched(canceled, err) {
				t.Fatal("canceled context restored retry authority")
			}
		})
	}
}

func (p nativeMainOfflineProtocol) CallSource(context.Context, string, string, map[string]any) (json.RawMessage, error) {
	return append([]byte(nil), p.raw...), p.err
}

func TestInstagramNativeObservationWireUsesCaptureBounds(t *testing.T) {
	observation := domain.Observation{Source: domain.SourceInstagram, CapturedAt: domain.Now(), Coverage: map[string]any{}, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "instagram:offline-wire", Text: strings.Repeat("fixture text ", 2000)}}}}}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) <= 16384 {
		t.Fatal("fixture must exceed small control limit")
	}
	transport := &instagramMainNativeTransport{protocol: nativeMainOfflineProtocol{raw: raw}}
	var got domain.Observation
	if err = transport.call(context.Background(), "source_capture", nil, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Snapshots) != 1 || got.Snapshots[0].Blocks[0].Text != observation.Snapshots[0].Blocks[0].Text {
		t.Fatal("capture body lost")
	}
	for name, invalid := range map[string][]byte{"oversize": []byte(strings.Repeat(" ", 512*1024+1)), "duplicate": []byte(`{"source":"instagram","source":"x","snapshots":[]}`), "wrong-source": []byte(strings.Replace(string(raw), `"source":"instagram"`, `"source":"x"`, 1)), "unknown": []byte(`{"source":"instagram","private":"secret","snapshots":[]}`)} {
		t.Run(name, func(t *testing.T) {
			if err := decodeSourceNativeObservation(invalid, domain.SourceInstagram, &domain.Observation{}); err == nil {
				t.Fatal("invalid capture accepted")
			}
		})
	}
}

func TestInstagramNativeQualificationDoesNotReadOrAttachProfiles(t *testing.T) {
	if instagramNativeQualified {
		t.Fatal("offline fixture cannot qualify native Instagram")
	}
	e, _ := testEngine(t)
	e.config.Dev = true
	e.config.Capture.InstagramMode = config.XCaptureConcurrent
	// Nil owners and deliberately unusable paths must remain untouched while
	// the reviewed recipe is not qualified.
	if err := ConfigureInstagramMainDailyRuntime(context.Background(), e, nil, nil, "unread-profile", "invalid/subprofile", "unread-chrome"); err != nil {
		t.Fatal(err)
	}
	if e.instagramDaily.attached || e.instagramDaily.factory != nil || e.instagramDaily.owner != nil {
		t.Fatal("unqualified recipe installed native capabilities")
	}
	status := e.SourceCaptureStatus(domain.SourceInstagram)
	if status.Reason != "native-adapter-qualification-required" || status.DerivationAuthorized {
		t.Fatalf("qualification status=%+v", status)
	}
	if err := e.StartInstagramDaily(context.Background(), XDailyStartRequest{true, true}); err == nil {
		t.Fatal("process consent bypassed native qualification")
	}
}
