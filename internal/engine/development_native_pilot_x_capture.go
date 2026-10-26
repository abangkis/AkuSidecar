//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/collection/headless"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/readerbroker"
)

type xNativeCapturePins xManualPilotPins

// Traffic completeness is observational metadata, never an admission input.
type nativePilotCaptureTraffic struct {
	Schema     string   `json:"schema"`
	Coverage   string   `json:"coverage"`
	Conditions []string `json:"conditions"`
	Records    int      `json:"records"`
	Dropped    int      `json:"dropped"`
}

var nativePilotCaptureTrafficConditions = []string{"orphan_response", "orphan_extra", "extra_flag_missing", "extra_flag_invalid", "request_reused", "redirect", "response_pending", "completion_pending", "extra_pending", "unexpected_extra", "network_failed", "record_capacity", "event_capacity", "metadata_shape"}

func decodeNativePilotCaptureTraffic(raw []byte) nativePilotCaptureTraffic {
	var report nativePilotCaptureTraffic
	if len(raw) == 0 || len(raw) > 1024 || !xNativeCaptureJSON(raw) {
		return report
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 5 {
		return report
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&report) != nil || d.Decode(new(any)) != io.EOF || report.Schema != "aku.capture-traffic.v1" || report.Coverage != "complete" && report.Coverage != "partial" || report.Records < 0 || report.Records > 256 || report.Dropped < 0 || report.Dropped > 65535 || report.Conditions == nil || len(report.Conditions) > len(nativePilotCaptureTrafficConditions) {
		return nativePilotCaptureTraffic{}
	}
	seen := map[string]bool{}
	for _, condition := range report.Conditions {
		if seen[condition] || nativePilotCaptureMember(condition, nativePilotCaptureTrafficConditions) == "unknown" {
			return nativePilotCaptureTraffic{}
		}
		seen[condition] = true
	}
	if report.Coverage == "complete" && (len(seen) != 0 || report.Dropped != 0) {
		return nativePilotCaptureTraffic{}
	}
	return report
}
func (v nativePilotCaptureTraffic) String() string {
	if v.Schema != "aku.capture-traffic.v1" || v.Coverage != "complete" && v.Coverage != "partial" || v.Records < 0 || v.Records > 256 || v.Dropped < 0 || v.Dropped > 65535 || len(v.Conditions) > len(nativePilotCaptureTrafficConditions) {
		return "coverage=unavailable"
	}
	for _, condition := range v.Conditions {
		if nativePilotCaptureMember(condition, nativePilotCaptureTrafficConditions) == "unknown" {
			return "coverage=unavailable"
		}
	}
	if v.Coverage == "complete" && (len(v.Conditions) != 0 || v.Dropped != 0) {
		return "coverage=unavailable"
	}
	return fmt.Sprintf("coverage=%s conditions=%s records=%d dropped=%d", v.Coverage, strings.Join(v.Conditions, ","), v.Records, v.Dropped)
}
func (v nativePilotCaptureTraffic) GoString() string { return v.String() }

type xNativePilotCapture struct {
	ports     *xManualPilotOwnerPorts
	prepared  nativePilotPreparedRole
	deadline  time.Time
	inFlight  atomic.Bool
	revoke    func()
	trafficMu sync.Mutex
	traffic   nativePilotCaptureTraffic
}

func (p *xNativePilotCapture) trafficDiagnostic() string {
	p.trafficMu.Lock()
	defer p.trafficMu.Unlock()
	return p.traffic.String()
}

func (p *xNativePilotCapture) nativePilotCollectorOwner() nativePilotOwner { return p.prepared.owner }
func (p *xNativePilotCapture) Capture(ctx context.Context, source domain.Source, payload map[string]any) (result domain.Observation, captureErr error) {
	started := false
	defer func() {
		var challenge *headless.CaptureError
		if started && captureErr != nil && p.revoke != nil && !errors.As(captureErr, &challenge) {
			p.revoke()
		}
	}()
	bad := func(stage, reason string) error {
		var err error
		if ctx != nil {
			err = ctx.Err()
		}
		return nativePilotCaptureFail(err, stage, reason)
	}
	if ctx == nil || source != domain.SourceX || p.ports == nil || !p.inFlight.CompareAndSwap(false, true) {
		return domain.Observation{}, bad("admission", "invalid_request")
	}
	defer p.inFlight.Store(false)
	p.trafficMu.Lock()
	p.traffic = nativePilotCaptureTraffic{}
	p.trafficMu.Unlock()
	if !p.ports.backend.now().Before(p.deadline) {
		return domain.Observation{}, bad("admission", "deadline")
	}
	owner, err := p.ports.inspect(ctx, p.prepared)
	if failure := nativePilotCaptureInspectError(ctx, "owner_before", err, owner != p.prepared.owner); failure != nil {
		started = true
		return domain.Observation{}, failure
	}
	projected := map[string]any{}
	for _, key := range []string{"acquisitionRound", "round"} {
		if value, ok := payload[key]; ok {
			switch n := value.(type) {
			case int:
				if n != 1 {
					return domain.Observation{}, bad("payload", "invalid_request")
				}
			case float64:
				if n != 1 {
					return domain.Observation{}, bad("payload", "invalid_request")
				}
			default:
				return domain.Observation{}, bad("payload", "invalid_request")
			}
		}
	}
	if value, ok := payload["continuation"]; ok && value != nil {
		return domain.Observation{}, bad("payload", "invalid_request")
	}
	for _, key := range []string{"pageUrl", "scrolls", "maxBlocksPerSnapshot", "sourceHydrationTimeoutMs", "scrollFraction", "scrollSettleMs"} {
		if value, ok := payload[key]; ok {
			projected[key] = value
		}
	}
	for alias, key := range map[string]string{"maxScrolls": "scrolls", "maxPosts": "maxBlocksPerSnapshot"} {
		if value, ok := payload[alias]; ok {
			if _, exists := projected[key]; exists {
				return domain.Observation{}, bad("payload", "invalid_request")
			}
			projected[key] = value
		}
	}
	for _, key := range []string{"script", "expression", "url"} {
		if _, ok := payload[key]; ok {
			return domain.Observation{}, bad("payload", "invalid_request")
		}
	}
	pins := xNativeCapturePins{OwnerID: owner.owner, ProfileID: owner.profile, ContextID: owner.context, TargetID: owner.target, FrameID: owner.frame, DocumentID: owner.document}
	correlation := domain.NewID("capture")
	request := map[string]any{"correlation": correlation, "pins": pins, "deadlineMS": p.deadline.UnixMilli(), "payload": projected}
	call, cancel := context.WithDeadline(ctx, p.deadline)
	defer cancel()
	stop := context.AfterFunc(call, func() {
		cleanup, done := context.WithTimeout(context.Background(), nativePilotCaptureCleanupTimeout)
		defer done()
		_ = p.ports.backend.rpc(cleanup, "collector", "manual_capture_cancel", map[string]any{"handle": p.prepared.handle}, nil)
	})
	defer stop()
	var raw json.RawMessage
	defer func() { clear(raw) }()
	started = true
	if err := p.ports.backend.rpc(call, "collector", "manual_capture", map[string]any{"handle": p.prepared.handle, "request": request}, &raw); err != nil {
		return domain.Observation{}, nativePilotCaptureTransportError(call, err)
	}
	var wire struct {
		Schema          string             `json:"schema"`
		Correlation     string             `json:"correlation"`
		Pins            xNativeCapturePins `json:"pins"`
		CaptureTarget   string             `json:"captureTarget"`
		ObservationJSON string             `json:"observationJSON"`
		Digest          string             `json:"digest"`
		Traffic         json.RawMessage    `json:"traffic"`
	}
	if len(raw) > 768*1024 || !xNativeCaptureJSON(raw) {
		return domain.Observation{}, bad("wire", "envelope_invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&wire) != nil || d.Decode(new(any)) != io.EOF || wire.Schema != "aku.x.manual-capture.v1" || wire.Correlation != correlation || wire.Pins != pins || !nativePilotRef(wire.CaptureTarget, 512) || wire.CaptureTarget == owner.target || len(wire.ObservationJSON) > 512*1024 {
		return domain.Observation{}, bad("wire", "envelope_invalid")
	}
	hash := sha256.Sum256([]byte(wire.ObservationJSON))
	if hex.EncodeToString(hash[:]) != wire.Digest {
		return domain.Observation{}, bad("wire", "digest_mismatch")
	}
	if !xNativeCaptureJSON([]byte(wire.ObservationJSON)) {
		return domain.Observation{}, bad("observation", "observation_invalid")
	}
	var observation domain.Observation
	d = json.NewDecoder(bytes.NewBufferString(wire.ObservationJSON))
	d.DisallowUnknownFields()
	if d.Decode(&observation) != nil || d.Decode(new(any)) != io.EOF || observation.Source != domain.SourceX || validateObservation(observation) != nil {
		return domain.Observation{}, bad("observation", "observation_invalid")
	}
	owner, err = p.ports.inspect(call, p.prepared)
	if failure := nativePilotCaptureInspectError(call, "owner_after", err, owner != p.prepared.owner); failure != nil {
		return domain.Observation{}, failure
	}
	if call.Err() != nil || !p.ports.backend.now().Before(p.deadline) {
		return domain.Observation{}, nativePilotCaptureFail(call.Err(), "owner_after", "deadline")
	}
	// Invalid/absent diagnostic metadata cannot reject or authorize a result.
	p.trafficMu.Lock()
	p.traffic = decodeNativePilotCaptureTraffic(wire.Traffic)
	p.trafficMu.Unlock()
	return observation, nil
}

// Runtime construction follows startup verification. Store issuance is a
// separate step so application source routing stays closed throughout startup.
func newXNativePilotRuntime(ctx context.Context, b *nativePilotBroker, p *xManualPilotOwnerPorts, reader, collector *captureruntime.Manager, open func(context.Context, readerbroker.Request) error) (*nativePilotRuntime, error) {
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
	return r, nil
}

// Existing QA composition retains its single constructor/Store binding entry.
func (e *Engine) bindXNativePilotRuntime(ctx context.Context, b *nativePilotBroker, p *xManualPilotOwnerPorts, reader, collector *captureruntime.Manager, open func(context.Context, readerbroker.Request) error, sessionID, runID string) (*nativePilotRuntime, error) {
	r, err := newXNativePilotRuntime(ctx, b, p, reader, collector, open)
	if err != nil {
		return nil, err
	}
	if err = e.bindNativePilotStore(ctx, r, sessionID, runID); err != nil {
		return nil, err
	}
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
