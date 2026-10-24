//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync/atomic"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

// No constructor is registered with main. The fresh loopback test factory binds
// this adapter to its exact process owner. Payload cannot choose a URL or script.
type nativePilotChromeCapture struct {
	bridge   *chromeFixtureBridge
	owner    nativePilotOwner
	inFlight atomic.Bool
	calls    atomic.Int32
}

func (p *nativePilotChromeCapture) nativePilotCollectorOwner() nativePilotOwner { return p.owner }
func (p *nativePilotChromeCapture) Capture(ctx context.Context, source domain.Source, _ map[string]any) (domain.Observation, error) {
	if source != domain.SourceX || p.bridge == nil || !validNativePilotOwner(p.owner, "collector") || !p.inFlight.CompareAndSwap(false, true) {
		return domain.Observation{}, errors.New("fixture capture unavailable")
	}
	defer p.inFlight.Store(false)
	p.calls.Add(1)
	// Explicit JSON names avoid accepting a process-local owner via public flags.
	var wire struct {
		Startup struct {
			Owner    string `json:"ownerId"`
			Profile  string `json:"profileId"`
			Context  string `json:"contextId"`
			Target   string `json:"targetId"`
			Frame    string `json:"frameId"`
			Document string `json:"documentId"`
		} `json:"startup"`
		CaptureTarget string             `json:"captureTarget"`
		Observation   domain.Observation `json:"observation"`
	}
	var raw json.RawMessage
	if err := p.bridge.rpcRoundTrip(ctx, "capture_fixture", map[string]any{"owner": 1}, &raw); err != nil {
		return domain.Observation{}, err
	}
	defer clear(raw)
	if len(raw) == 0 || len(raw) > 140*1024 {
		return domain.Observation{}, errors.New("fixture capture size rejected")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&wire) != nil || decoder.Decode(new(any)) != io.EOF {
		return domain.Observation{}, errors.New("fixture capture shape rejected")
	}
	o, w := p.owner, wire.Startup
	if w.Owner != o.owner || w.Profile != o.profile || w.Context != o.context || w.Target != o.target || w.Frame != o.frame || w.Document != o.document || !nativePilotRef(wire.CaptureTarget, 512) || wire.CaptureTarget == o.target || wire.Observation.Source != domain.SourceX {
		return domain.Observation{}, errors.New("fixture capture binding rejected")
	}
	if len(wire.Observation.Snapshots) != 1 || len(wire.Observation.Snapshots[0].Blocks) < 1 || len(wire.Observation.Snapshots[0].Blocks) > 20 {
		return domain.Observation{}, errors.New("fixture capture blocks rejected")
	}
	for _, block := range wire.Observation.Snapshots[0].Blocks {
		canonical, ok := domain.CanonicalSourceURL(domain.SourceX, block.Permalink)
		id := strings.TrimPrefix(block.PlatformID, "x:status:")
		if !ok || canonical != block.Permalink || id == block.PlatformID || !nativePilotAccount(id) || !strings.HasSuffix(canonical, "/status/"+id) || len(block.Text) > 8192 || len(block.Author) > 256 {
			return domain.Observation{}, errors.New("fixture post identity rejected")
		}
	}
	return wire.Observation, nil
}
