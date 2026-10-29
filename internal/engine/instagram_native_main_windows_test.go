//go:build windows && auth_session_dev

package engine

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type nativeMainOfflineProtocol struct {
	raw []byte
	err error
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
