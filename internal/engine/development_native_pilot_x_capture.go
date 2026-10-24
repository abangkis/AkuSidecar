//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type xNativeCapturePins xManualPilotPins
type xNativePilotCapture struct {
	ports    *xManualPilotOwnerPorts
	prepared nativePilotPreparedRole
	deadline time.Time
	inFlight atomic.Bool
	revoke   func()
}

func (p *xNativePilotCapture) nativePilotCollectorOwner() nativePilotOwner { return p.prepared.owner }
func (p *xNativePilotCapture) Capture(ctx context.Context, source domain.Source, payload map[string]any) (result domain.Observation, captureErr error) {
	started := false
	defer func() {
		if started && captureErr != nil && p.revoke != nil {
			p.revoke()
		}
	}()
	bad := errors.New("native X capture rejected")
	if ctx == nil || source != domain.SourceX || p.ports == nil || !p.inFlight.CompareAndSwap(false, true) {
		return domain.Observation{}, bad
	}
	defer p.inFlight.Store(false)
	if !p.ports.backend.now().Before(p.deadline) {
		return domain.Observation{}, bad
	}
	owner, err := p.ports.inspect(ctx, p.prepared)
	if err != nil || owner != p.prepared.owner {
		started = true
		return domain.Observation{}, bad
	}
	projected := map[string]any{}
	for _, key := range []string{"acquisitionRound", "round"} {
		if value, ok := payload[key]; ok {
			switch n := value.(type) {
			case int:
				if n != 1 {
					return domain.Observation{}, bad
				}
			case float64:
				if n != 1 {
					return domain.Observation{}, bad
				}
			default:
				return domain.Observation{}, bad
			}
		}
	}
	if value, ok := payload["continuation"]; ok && value != nil {
		return domain.Observation{}, bad
	}
	for _, key := range []string{"pageUrl", "scrolls", "maxBlocksPerSnapshot", "sourceHydrationTimeoutMs"} {
		if value, ok := payload[key]; ok {
			projected[key] = value
		}
	}
	for alias, key := range map[string]string{"maxScrolls": "scrolls", "maxPosts": "maxBlocksPerSnapshot"} {
		if value, ok := payload[alias]; ok {
			if _, exists := projected[key]; exists {
				return domain.Observation{}, bad
			}
			projected[key] = value
		}
	}
	for _, key := range []string{"script", "expression", "url"} {
		if _, ok := payload[key]; ok {
			return domain.Observation{}, bad
		}
	}
	pins := xNativeCapturePins{OwnerID: owner.owner, ProfileID: owner.profile, ContextID: owner.context, TargetID: owner.target, FrameID: owner.frame, DocumentID: owner.document}
	correlation := domain.NewID("capture")
	request := map[string]any{"correlation": correlation, "pins": pins, "deadlineMS": p.deadline.UnixMilli(), "payload": projected}
	call, cancel := context.WithDeadline(ctx, p.deadline)
	defer cancel()
	stop := context.AfterFunc(call, func() {
		cleanup, done := context.WithTimeout(context.Background(), time.Second)
		defer done()
		_ = p.ports.backend.rpc(cleanup, "collector", "manual_capture_cancel", map[string]any{"handle": p.prepared.handle}, nil)
	})
	defer stop()
	var raw json.RawMessage
	started = true
	if err := p.ports.backend.rpc(call, "collector", "manual_capture", map[string]any{"handle": p.prepared.handle, "request": request}, &raw); err != nil {
		return domain.Observation{}, bad
	}
	defer clear(raw)
	var wire struct {
		Schema          string             `json:"schema"`
		Correlation     string             `json:"correlation"`
		Pins            xNativeCapturePins `json:"pins"`
		CaptureTarget   string             `json:"captureTarget"`
		ObservationJSON string             `json:"observationJSON"`
		Digest          string             `json:"digest"`
	}
	if len(raw) > 768*1024 || !xNativeCaptureJSON(raw) {
		return domain.Observation{}, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF || wire.Schema != "aku.x.manual-capture.v1" || wire.Correlation != correlation || wire.Pins != pins || !nativePilotRef(wire.CaptureTarget, 512) || wire.CaptureTarget == owner.target || len(wire.ObservationJSON) > 512*1024 {
		return domain.Observation{}, bad
	}
	hash := sha256.Sum256([]byte(wire.ObservationJSON))
	if hex.EncodeToString(hash[:]) != wire.Digest || !xNativeCaptureJSON([]byte(wire.ObservationJSON)) {
		return domain.Observation{}, bad
	}
	var observation domain.Observation
	d = json.NewDecoder(bytes.NewBufferString(wire.ObservationJSON))
	d.DisallowUnknownFields()
	if d.Decode(&observation) != nil || d.Decode(new(any)) != io.EOF || observation.Source != domain.SourceX || validateObservation(observation) != nil {
		return domain.Observation{}, bad
	}
	owner, err = p.ports.inspect(call, p.prepared)
	if err != nil || owner != p.prepared.owner || call.Err() != nil || !p.ports.backend.now().Before(p.deadline) {
		return domain.Observation{}, bad
	}
	return observation, nil
}

// Private QA composition only. No main, CLI or HTTP caller is registered.
func (e *Engine) bindXNativePilotRuntime(ctx context.Context, b *nativePilotBroker, p *xManualPilotOwnerPorts, reader, collector *captureruntime.Manager, open func(context.Context, readerbroker.Request) error, sessionID, runID string) (*nativePilotRuntime, error) {
	if b == nil || p == nil || p.backend.provenance() != nativePilotTrustedStartup {
		return nil, errors.New("native X owner unavailable")
	}
	startup, err := b.storeStartup(ctx)
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	prepared, ok := p.prepared["collector"]
	p.mu.Unlock()
	if p.cfg.pilot.binding != b.cfg.pilot.binding || p.cfg.pilot.attemptStarted != b.cfg.pilot.attemptStarted {
		return nil, errors.New("native X attempt mismatch")
	}
	if !ok {
		return nil, errors.New("native X collector unavailable")
	}
	b.mu.Lock()
	c := b.controller
	var expected nativePilotOwner
	if c != nil {
		c.mu.Lock()
		if c.vector != nil {
			expected = c.vector.binding.collector
		}
		c.mu.Unlock()
	}
	b.mu.Unlock()
	if expected != prepared.owner {
		return nil, errors.New("native X collector binding mismatch")
	}
	backend := &xNativePilotCapture{ports: p, prepared: prepared, deadline: startup.Deadline}
	r, err := newNativePilotRuntime(b, reader, collector, backend, open)
	if err != nil {
		return nil, err
	}
	if err = e.bindNativePilotStore(ctx, r, sessionID, runID); err != nil {
		return nil, err
	}
	r.mu.Lock()
	backend.revoke = r.storeLink.close
	r.mu.Unlock()
	return r, nil
}

// Capture observations may contain arrays and nulls, unlike startup receipts;
// duplicate fields, invalid Unicode and unbounded nesting remain rejected.
func xNativeCaptureJSON(raw []byte) bool {
	if len(raw) == 0 || !nativePilotStartupUnicode(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	nodes := 0
	var walk func(int) bool
	walk = func(depth int) bool {
		nodes++
		if depth > 32 || nodes > 50000 {
			return false
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		delim, compound := token.(json.Delim)
		if !compound {
			return true
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				key, err := d.Token()
				name, ok := key.(string)
				if err != nil || !ok || seen[name] {
					return false
				}
				seen[name] = true
				if !walk(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim('}')
		case '[':
			for d.More() {
				if !walk(depth + 1) {
					return false
				}
			}
			end, err := d.Token()
			return err == nil && end == json.Delim(']')
		default:
			return false
		}
	}
	if !walk(0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}
