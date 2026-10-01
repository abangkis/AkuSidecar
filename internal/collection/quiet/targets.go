// Package quiet provides a bounded broker for hidden source targets owned by
// the private capture pipe's browser session.
package quiet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/appshell"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

var (
	errUnavailable = errors.New("hidden target broker is unavailable")
	errPoisoned    = errors.New("hidden target broker is poisoned after uncertain setup")
	errClosed      = errors.New("hidden target broker is closed")
)

var sourceOrder = [...]domain.Source{domain.SourceX, domain.SourceFacebook}

const (
	ownedTargetClosePollInterval = 25 * time.Millisecond
	ownedTargetClosePollLimit    = 3 * time.Second
)

var allowedPageMethods = map[string]struct{}{
	"Page.navigate":                      {},
	"Runtime.evaluate":                   {},
	"Input.dispatchMouseEvent":           {},
	"Page.enable":                        {},
	"Runtime.enable":                     {},
	"Network.enable":                     {},
	"Emulation.setDeviceMetricsOverride": {},
}

type ownedTarget struct {
	targetID string
	pageID   string
	absent   bool
}

// Targets serializes target lifecycle operations so target IDs and CDP session
// IDs stay private to this broker and can never be confused across sources.
type Targets struct {
	mu sync.Mutex

	protocol  appshell.CaptureProtocol
	browserID string
	version   json.RawMessage
	owned     map[domain.Source]*ownedTarget

	poisoned         bool
	unknownCreation  bool
	unknownBrowserID bool
	closed           bool
}

// NewTargets creates a dedicated child browser session on the private capture
// protocol. An error after dispatch has an uncertain outcome; in that case a
// partial broker is returned so its caller can retain the owning window and
// attempt bounded cleanup. Close reports that cleanup cannot be verified if
// the browser session ID itself was lost.
func NewTargets(ctx context.Context, protocol appshell.CaptureProtocol) (*Targets, error) {
	if ctx == nil || protocol == nil {
		return nil, errUnavailable
	}
	t := &Targets{protocol: protocol, owned: make(map[domain.Source]*ownedTarget)}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := protocol.Call(ctx, "Target.attachToBrowserTarget", nil, "")
	if err != nil {
		t.unknownBrowserID = true
		t.poisoned = true
		return t, errors.New("could not establish the private child browser session")
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(raw, &attached) != nil || !validID(attached.SessionID) {
		t.unknownBrowserID = true
		t.poisoned = true
		return t, errors.New("private child browser session response was invalid")
	}
	t.browserID = attached.SessionID
	version, err := protocol.Call(ctx, "Browser.getVersion", nil, "")
	if err != nil || !json.Valid(version) || len(version) == 0 {
		t.poisoned = true
		return t, errors.New("browser version query failed during broker setup")
	}
	t.version = append(json.RawMessage(nil), version...)
	return t, nil
}

// Version returns the browser version metadata without exposing any target or
// session identifiers. It is captured during construction so worker startup
// does not make an unbounded protocol call without a caller context.
func (t *Targets) Version() json.RawMessage {
	if t == nil {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return append(json.RawMessage(nil), t.version...)
}

// Call forwards only the page methods needed by the capture worker. Source
// validation and method validation happen before any CDP command is sent.
func (t *Targets) Call(ctx context.Context, source domain.Source, method string, params any) (json.RawMessage, error) {
	if !supportedSource(source) {
		return nil, errors.New("hidden target broker supports X and Facebook only")
	}
	if _, ok := allowedPageMethods[method]; !ok {
		return nil, errors.New("CDP method is not allowed for hidden source targets")
	}
	if t == nil || ctx == nil {
		return nil, errUnavailable
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t.closed {
		return nil, errClosed
	}
	if t.poisoned {
		return nil, errPoisoned
	}
	if t.browserID == "" || t.unknownBrowserID {
		return nil, errUnavailable
	}
	target, ok := t.owned[source]
	if !ok {
		var err error
		target, err = t.createTarget(ctx, source)
		if err != nil {
			return nil, err
		}
	}
	if target.pageID == "" {
		return nil, errPoisoned
	}
	raw, err := t.protocol.Call(ctx, method, params, target.pageID)
	if err != nil {
		if isSetupMethod(method) {
			t.poisoned = true
		}
		return nil, fmt.Errorf("hidden target %s call failed", method)
	}
	return raw, nil
}

func (t *Targets) createTarget(ctx context.Context, source domain.Source) (*ownedTarget, error) {
	// The default authenticated context is deliberate. Do not add newWindow or
	// browserContextId: Chrome 152 proved this hidden, non-tab target shape.
	raw, err := t.protocol.Call(ctx, "Target.createTarget", map[string]any{
		"url": "about:blank", "hidden": true, "background": true, "forTab": false,
	}, t.browserID)
	if err != nil {
		t.unknownCreation = true
		t.poisoned = true
		return nil, errors.New("hidden source target creation failed; broker is poisoned")
	}
	var created struct {
		TargetID string `json:"targetId"`
	}
	if json.Unmarshal(raw, &created) != nil || !validID(created.TargetID) {
		t.unknownCreation = true
		t.poisoned = true
		return nil, errors.New("hidden source target creation response was invalid; broker is poisoned")
	}
	owned := &ownedTarget{targetID: created.TargetID}
	t.owned[source] = owned

	attachedRaw, err := t.protocol.Call(ctx, "Target.attachToTarget", map[string]any{
		"targetId": created.TargetID, "flatten": true,
	}, t.browserID)
	if err != nil {
		t.poisoned = true
		return nil, errors.New("hidden source target attachment failed; broker is poisoned")
	}
	var attached struct {
		SessionID string `json:"sessionId"`
	}
	if json.Unmarshal(attachedRaw, &attached) != nil || !validID(attached.SessionID) {
		t.poisoned = true
		return nil, errors.New("hidden source target attachment response was invalid; broker is poisoned")
	}
	owned.pageID = attached.SessionID
	for _, setup := range []struct {
		method string
		params any
	}{
		{method: "Page.enable"},
		{method: "Runtime.enable"},
		{method: "Network.enable"},
		{method: "Emulation.setDeviceMetricsOverride", params: map[string]any{
			"width": 1280, "height": 900, "deviceScaleFactor": 1, "mobile": false,
		}},
	} {
		if err := ctx.Err(); err != nil {
			t.poisoned = true
			return nil, errors.New("hidden source target setup was interrupted; broker is poisoned")
		}
		if _, err := t.protocol.Call(ctx, setup.method, setup.params, owned.pageID); err != nil {
			t.poisoned = true
			return nil, errors.New("hidden source target setup failed; broker is poisoned")
		}
	}
	return owned, nil
}

// Close closes only targets whose IDs this broker received from its own
// Target.createTarget calls. It verifies those IDs are absent in the child
// browser session before detaching that session. If target creation was
// uncertain, detaching the session is the protocol ownership boundary for its
// hidden target; no target is adopted from a listing.
func (t *Targets) Close(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("cleanup context is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.unknownBrowserID || t.browserID == "" {
		return errors.New("child browser session cleanup is unverified")
	}

	knownTargets := make([]*ownedTarget, 0, len(t.owned))
	for _, source := range sourceOrder {
		if target := t.owned[source]; target != nil && !target.absent {
			knownTargets = append(knownTargets, target)
		}
	}
	if len(knownTargets) > 0 {
		pollCtx, cancel := context.WithTimeout(ctx, ownedTargetClosePollLimit)
		defer cancel()
		for _, target := range knownTargets {
			// A failed close response is ambiguous. The following owned-session
			// listing decides whether another bounded Close attempt is needed.
			_, _ = t.protocol.Call(pollCtx, "Target.closeTarget", map[string]any{"targetId": target.targetID}, t.browserID)
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := pollCtx.Err(); err != nil {
				return errors.New("owned hidden target close did not complete; cleanup can be retried")
			}
		}
		ticker := time.NewTicker(ownedTargetClosePollInterval)
		defer ticker.Stop()
		for {
			raw, err := t.protocol.Call(pollCtx, "Target.getTargets", nil, t.browserID)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if pollCtx.Err() != nil {
					return errors.New("owned hidden target absence timed out; cleanup can be retried")
				}
				return errors.New("owned hidden target absence could not be verified")
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			var listing struct {
				TargetInfos []struct {
					TargetID string `json:"targetId"`
				} `json:"targetInfos"`
			}
			if json.Unmarshal(raw, &listing) != nil {
				return errors.New("owned hidden target listing was invalid")
			}
			present := make(map[string]bool, len(listing.TargetInfos))
			for _, info := range listing.TargetInfos {
				if info.TargetID != "" {
					present[info.TargetID] = true
				}
			}
			allAbsent := true
			for _, target := range knownTargets {
				target.absent = !present[target.targetID]
				allAbsent = allAbsent && target.absent
			}
			if allAbsent {
				break
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-pollCtx.Done():
				return errors.New("owned hidden target is still present; cleanup can be retried")
			case <-ticker.C:
			}
		}
	}

	// When creation was uncertain, detaching this browser session is the only
	// safe cleanup operation: there is no target ID to close or adopt.
	_, err := t.protocol.Call(ctx, "Target.detachFromTarget", map[string]any{"sessionId": t.browserID}, "")
	if err != nil {
		return errors.New("private child browser session detach failed; cleanup can be retried")
	}
	t.closed = true
	return nil
}

func supportedSource(source domain.Source) bool {
	return source == domain.SourceX || source == domain.SourceFacebook
}

func isSetupMethod(method string) bool {
	switch method {
	case "Page.enable", "Runtime.enable", "Network.enable", "Emulation.setDeviceMetricsOverride":
		return true
	default:
		return false
	}
}

func validID(value string) bool {
	if value == "" || len(value) > 512 {
		return false
	}
	for _, r := range value {
		if r == 0 {
			return false
		}
	}
	return true
}
