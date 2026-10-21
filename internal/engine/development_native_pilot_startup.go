//go:build auth_session_dev

package engine

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"sync"
	"time"
	"unicode/utf8"
)

const nativePilotStartupWireSchema = "aku.x.manual-startup-receipt.v1"
const nativePilotStartupWireMax = 16 * 1024

// These ports are trusted private transports. They must run the fixed reviewed
// startup bootstrap/settings recipe, honor cancellation and retire observation
// resources. They cannot accept a historical public QA report as a receipt.
type nativePilotCapture func(context.Context, nativePilotCaptureChallenge) ([]byte, error)
type nativePilotCaptureChallenge struct {
	challenge   nativePilotChallenge
	correlation string
}

func (nativePilotCaptureChallenge) String() string     { return "private-native-capture-challenge" }
func (v nativePilotCaptureChallenge) GoString() string { return v.String() }

// requestJSON is an explicit private transport operation, never a diagnostic or
// general-purpose serialization of the private challenge/account/binding.
func (c nativePilotCaptureChallenge) requestJSON() ([]byte, error) {
	o := c.challenge.owner
	if !nativePilotRef(c.correlation, 512) || !validNativePilotOwner(o, c.challenge.role) {
		return nil, errors.New("startup-challenge-invalid")
	}
	return json.Marshal(map[string]string{"challenge": c.correlation, "role": c.challenge.role, "ownerId": o.owner, "profileId": o.profile, "contextId": o.context, "targetId": o.target, "frameId": o.frame, "documentId": o.document})
}

type nativePilotStartupConfig struct {
	enabled, offlineOnly bool
	binding              nativePilotBinding
	capture              nativePilotCapture
}
type nativePilotStartupSource struct {
	mu                                      sync.Mutex
	cfg                                     nativePilotStartupConfig
	begun, ready, readerRead, collectorRead bool
	reader, collector                       nativePilotChallenge
	readerReceipt, collectorReceipt         nativePilotReceipt
}

func (*nativePilotStartupSource) String() string     { return "private-native-startup-source" }
func (v *nativePilotStartupSource) GoString() string { return v.String() }
func (nativePilotStartupConfig) String() string      { return "private-native-startup-config" }
func (v nativePilotStartupConfig) GoString() string  { return v.String() }

func newNativePilotStartupSource(cfg nativePilotStartupConfig) (*nativePilotStartupSource, error) {
	if !cfg.enabled || cfg.capture == nil || !validNativePilotBinding(cfg.binding) {
		return nil, errors.New("startup-source-unavailable")
	}
	return &nativePilotStartupSource{cfg: cfg}, nil
}
func (s *nativePilotStartupSource) provenance() nativePilotProvenance {
	if s.cfg.offlineOnly {
		return nativePilotOfflineOnly
	}
	return nativePilotTrustedStartup
}

// The adapter asks for roles serially. This source instead acquires both roles
// together on the first read and buffers the exact second challenge once.
// No native document/context pin is guessed or replaced after capture. A recipe
// that navigates to another document must first supply an approved ownership
// choreography; this boundary correctly rejects that mismatch today.
func (s *nativePilotStartupSource) observe(ctx context.Context, c nativePilotChallenge) (nativePilotReceipt, error) {
	s.mu.Lock()
	if ctx == nil || ctx.Err() != nil {
		s.mu.Unlock()
		return nativePilotReceipt{}, errors.New("startup-canceled")
	}
	if s.begun {
		if s.ready && !s.collectorRead && s.readerRead && c == s.collector {
			s.collectorRead = true
			r := s.collectorReceipt
			s.collectorReceipt = nativePilotReceipt{}
			s.mu.Unlock()
			return r, nil
		}
		s.mu.Unlock()
		return nativePilotReceipt{}, errors.New("startup-replayed-or-mismatched")
	}
	if c.seal == nil || c.vector != 1 || c.binding != s.cfg.binding || c.role != "reader" || c.owner != c.binding.reader || c.started.IsZero() {
		s.mu.Unlock()
		return nativePilotReceipt{}, errors.New("startup-challenge-invalid")
	}
	s.begun = true
	s.reader, s.collector = c, c
	s.collector.role, s.collector.owner = "collector", c.binding.collector
	collector := s.collector
	s.mu.Unlock()
	context.AfterFunc(ctx, s.discard)
	var entropy [24]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return nativePilotReceipt{}, errors.New("startup-source-unavailable")
	}
	correlation := hex.EncodeToString(entropy[:])
	group, cancel := context.WithCancel(ctx)
	defer cancel()
	type output struct {
		role    string
		receipt nativePilotReceipt
		err     error
	}
	outputs := make(chan output, 2)
	for _, challenge := range []nativePilotChallenge{c, collector} {
		go func(ch nativePilotChallenge) {
			raw, err := s.cfg.capture(group, nativePilotCaptureChallenge{challenge: ch, correlation: correlation})
			var receipt nativePilotReceipt
			if err == nil && group.Err() == nil {
				receipt, err = decodeNativePilotStartup(raw, ch, correlation)
			} else if err == nil {
				err = group.Err()
			}
			clear(raw)
			outputs <- output{role: ch.role, receipt: receipt, err: err}
		}(challenge)
	}
	var readerReceipt, collectorReceipt nativePilotReceipt
	for count := 0; count < 2; count++ {
		select {
		case <-ctx.Done():
			return nativePilotReceipt{}, errors.New("startup-canceled")
		case out := <-outputs:
			if out.err != nil {
				return nativePilotReceipt{}, errors.New("startup-projection-rejected")
			}
			if out.role == "reader" {
				readerReceipt = out.receipt
			} else {
				collectorReceipt = out.receipt
			}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ctx.Err() != nil {
		return nativePilotReceipt{}, errors.New("startup-canceled")
	}
	s.ready, s.readerRead, s.readerReceipt, s.collectorReceipt = true, true, nativePilotReceipt{}, collectorReceipt
	return readerReceipt, nil
}

func (s *nativePilotStartupSource) discard() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = false
	s.readerReceipt, s.collectorReceipt = nativePilotReceipt{}, nativePilotReceipt{}
}

// A private DTO is never returned, persisted or formatted. All fields, including
// source markers, are mandatory; duplicate/unknown/null fields are rejected.
type nativePilotStartupWire struct {
	schema, challenge, role, accountId, ownerId, profileId string
	contextId, targetId, frameId, documentId               string
	identityRequestId, settingsRequestId, qualification    string
	identityObservedAtMS, settingsObservedAtMS, elapsedMS  float64
}

func (nativePilotStartupWire) String() string     { return "private-native-startup-wire" }
func (v nativePilotStartupWire) GoString() string { return v.String() }

func decodeNativePilotStartup(raw []byte, ch nativePilotChallenge, correlation string) (nativePilotReceipt, error) {
	bad := errors.New("startup-projection-rejected")
	var empty nativePilotReceipt
	if len(raw) == 0 || len(raw) > nativePilotStartupWireMax || !nativePilotStartupUnicode(raw) {
		return empty, bad
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		return empty, bad
	}
	fields := make(map[string]json.RawMessage, 16)
	for d.More() {
		token, err := d.Token()
		key, ok := token.(string)
		if err != nil || !ok || fields[key] != nil {
			return empty, bad
		}
		var value json.RawMessage
		if d.Decode(&value) != nil {
			return empty, bad
		}
		fields[key] = value
	}
	last, err := d.Token()
	if err != nil || last != json.Delim('}') || len(fields) != 16 {
		return empty, bad
	}
	if _, err := d.Token(); err != io.EOF {
		return empty, bad
	}
	var wire nativePilotStartupWire
	texts := map[string]*string{"schema": &wire.schema, "challenge": &wire.challenge, "role": &wire.role, "accountId": &wire.accountId, "ownerId": &wire.ownerId, "profileId": &wire.profileId, "contextId": &wire.contextId, "targetId": &wire.targetId, "frameId": &wire.frameId, "documentId": &wire.documentId, "identityRequestId": &wire.identityRequestId, "settingsRequestId": &wire.settingsRequestId, "qualification": &wire.qualification}
	for key, dest := range texts {
		value := fields[key]
		if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, dest) != nil || !nativePilotRef(*dest, 512) {
			return empty, bad
		}
	}
	for key, dest := range map[string]*float64{"identityObservedAtMS": &wire.identityObservedAtMS, "settingsObservedAtMS": &wire.settingsObservedAtMS, "elapsedMS": &wire.elapsedMS} {
		value := fields[key]
		if len(value) == 0 || bytes.Equal(value, []byte("null")) || json.Unmarshal(value, dest) != nil || math.IsNaN(*dest) || math.IsInf(*dest, 0) || *dest < 0 {
			return empty, bad
		}
	}
	o := ch.owner
	if wire.schema != nativePilotStartupWireSchema || wire.challenge != correlation || wire.role != ch.role || !nativePilotAccount(wire.accountId) || wire.ownerId != o.owner || wire.profileId != o.profile || wire.contextId != o.context || wire.targetId != o.target || wire.frameId != o.frame || wire.documentId != o.document || wire.qualification != nativePilotSettingsQualification || wire.identityRequestId == wire.settingsRequestId || wire.elapsedMS >= float64(nativePilotJoint/time.Millisecond) {
		return empty, bad
	}
	// Millisecond epoch values are bounded before conversion to avoid overflow.
	toTime := func(ms float64) (time.Time, bool) {
		if ms > 8640000000000000 {
			return time.Time{}, false
		}
		sec, fraction := math.Modf(ms / 1000)
		return time.Unix(int64(sec), int64(fraction*1e9)).UTC(), true
	}
	identityAt, ok := toTime(wire.identityObservedAtMS)
	settingsAt, ok2 := toTime(wire.settingsObservedAtMS)
	if !ok || !ok2 || identityAt.Before(ch.started) || settingsAt.Before(identityAt) {
		return empty, bad
	}
	base := nativePilotResponse{challenge: ch, elapsed: time.Duration(wire.elapsedMS * float64(time.Millisecond)), semanticSuccess: true, fresh: true, direct: true, wellFormed: true, clockValid: true}
	r := nativePilotReceipt{identity: base, settings: base}
	r.identity.request, r.identity.source, r.identity.account, r.identity.currentSelf = wire.identityRequestId, nativePilotStartupIdentity, wire.accountId, true
	r.identity.observed, r.identity.validUntil = identityAt, identityAt.Add(nativePilotAge)
	r.settings.request, r.settings.method, r.settings.url, r.settings.qualification = wire.settingsRequestId, "GET", nativePilotSettingsURL, nativePilotSettingsQualification
	r.settings.observed, r.settings.validUntil = settingsAt, settingsAt.Add(nativePilotAge)
	return r, nil
}

// encoding/json replaces invalid UTF-8/surrogates. Private reference comparison
// requires rejection instead, so malformed strings cannot normalize into pins.
func nativePilotStartupUnicode(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	quoted := false
	word := func(at int) (uint16, bool) {
		if at+4 > len(raw) {
			return 0, false
		}
		var n uint16
		for _, b := range raw[at : at+4] {
			n <<= 4
			switch {
			case b >= '0' && b <= '9':
				n += uint16(b - '0')
			case b >= 'a' && b <= 'f':
				n += uint16(b - 'a' + 10)
			case b >= 'A' && b <= 'F':
				n += uint16(b - 'A' + 10)
			default:
				return 0, false
			}
		}
		return n, true
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		n, ok := word(i + 1)
		if !ok || n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		i += 4
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, ok := word(i + 3)
			if !ok || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
