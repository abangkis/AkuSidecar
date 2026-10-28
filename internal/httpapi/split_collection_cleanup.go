package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

// ReleaseBrowserCollectionSurfaces waits for the existing Bridge's lease-bound
// cleanup before the engine gives up the Browser collection owner. It does not
// close user-adopted tabs or authorize process termination.
func (s *Server) ReleaseBrowserCollectionSurfaces(ctx context.Context, sessionID string, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !splitID(sessionID) {
		return errors.New("invalid Browser collection session")
	}
	t := s.splitCapture
	if t == nil {
		return errors.New("Browser collection cleanup transport unavailable")
	}
	t.mu.Lock()
	if t.closed || t.runtime == nil {
		t.mu.Unlock()
		return errors.New("Browser collection cleanup transport stopped")
	}
	snapshot := t.runtime.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "browser" || snapshot.Generation != generation {
		t.mu.Unlock()
		return errors.New("Browser collection cleanup belongs to another owner")
	}
	var entry *pendingSplitAction
	for _, candidate := range t.actions {
		if candidate.collectionCleanupGeneration == generation && candidate.action.LeaseID == sessionID {
			entry = candidate
			break
		}
	}
	if entry == nil {
		if len(t.actions) >= splitActionLimit {
			t.mu.Unlock()
			return errors.New("Browser collection cleanup queue is full")
		}
		lease, err := t.runtime.Acquire()
		if err != nil {
			t.mu.Unlock()
			return err
		}
		if lease.Driver() != "browser" || lease.Generation() != generation {
			lease.Release()
			t.mu.Unlock()
			return errors.New("Browser collection cleanup owner changed")
		}
		entry = &pendingSplitAction{
			action:   splitCaptureAction{ID: domain.NewID("cleanup"), Type: "release", LeaseID: sessionID},
			queuedAt: time.Now(), result: make(chan splitActionResult, 1), runtimeLease: lease,
			collectionCleanupGeneration: generation,
		}
		t.actions = append(t.actions, entry)
	}
	if entry.completed {
		result := entry.completionResult
		t.mu.Unlock()
		return browserCollectionCleanupResult(result, sessionID, nil)
	}
	t.mu.Unlock()
	t.notifyCapture()
	select {
	case <-ctx.Done():
		// Keep the action and its lease: a claimed cleanup is never replayed.
		return ctx.Err()
	case <-t.done:
		return errors.New("Browser collection cleanup transport stopped")
	case result := <-entry.result:
		return browserCollectionCleanupResult(&result, sessionID, nil)
	}
}

func browserCollectionCleanupResult(result *splitActionResult, requestedLeaseID string, requestedSource *string) error {
	if result == nil || !result.OK {
		return errors.New("Browser collection cleanup was rejected or unverified")
	}
	// The existing Bridge release handler returns { outcome: ... }; the split
	// client preserves that envelope as result rather than flattening it.
	var receipt struct {
		Outcome *struct {
			Reason  json.RawMessage `json:"reason"`
			Cleanup *struct {
				Schema      string          `json:"schema"`
				LeaseID     string          `json:"leaseId"`
				Source      json.RawMessage `json:"source"`
				Expected    *float64        `json:"expected"`
				Closed      *float64        `json:"closed"`
				Transferred *float64        `json:"transferred"`
				Retained    *float64        `json:"retained"`
				Pending     *float64        `json:"pending"`
				Complete    *bool           `json:"complete"`
			} `json:"cleanup"`
		} `json:"outcome"`
	}
	if len(result.Result) == 0 || json.Unmarshal(result.Result, &receipt) != nil {
		return errors.New("Browser collection cleanup acknowledgement is invalid")
	}
	if receipt.Outcome == nil {
		return errors.New("Browser collection cleanup acknowledgement lacks an outcome")
	}
	outcome := receipt.Outcome
	cleanup := outcome.Cleanup
	if cleanup == nil || cleanup.Schema != "aku.surface-cleanup.v1" || cleanup.LeaseID != requestedLeaseID {
		return errors.New("Browser collection cleanup acknowledgement lacks a matching strict receipt")
	}
	if len(cleanup.Source) == 0 {
		return errors.New("Browser collection cleanup receipt lacks the requested source")
	}
	var echoedSource *string
	if err := json.Unmarshal(cleanup.Source, &echoedSource); err != nil || !sameOptionalString(echoedSource, requestedSource) {
		return errors.New("Browser collection cleanup receipt belongs to another source")
	}
	counts := []*float64{cleanup.Expected, cleanup.Closed, cleanup.Transferred, cleanup.Retained, cleanup.Pending}
	for _, count := range counts {
		if count == nil || math.IsNaN(*count) || math.IsInf(*count, 0) || *count < 0 || math.Trunc(*count) != *count || *count > 9007199254740991 {
			return errors.New("Browser collection cleanup receipt has invalid counts")
		}
		if *count > 4096 {
			return errors.New("Browser collection cleanup receipt exceeds the record bound")
		}
	}
	if *cleanup.Expected != *cleanup.Closed+*cleanup.Transferred+*cleanup.Retained+*cleanup.Pending {
		return errors.New("Browser collection cleanup receipt counts do not conserve the ledger")
	}
	if cleanup.Complete == nil || !*cleanup.Complete || *cleanup.Pending != 0 {
		return errors.New("Browser collection cleanup is pending or incomplete")
	}
	if requestedSource == nil && *cleanup.Retained != 0 {
		return errors.New("Browser collection cleanup retained unaccounted owned surfaces")
	}
	if len(outcome.Reason) > 0 {
		var reason *string
		if err := json.Unmarshal(outcome.Reason, &reason); err != nil || reason == nil {
			return errors.New("Browser collection cleanup receipt has an invalid reason")
		}
		switch *reason {
		case "":
		case "no_owned_surface":
			if *cleanup.Retained != 0 {
				return errors.New("Browser collection cleanup did not prove an empty owned-surface ledger")
			}
		case "no_owned_source_surface":
			if requestedSource == nil {
				return errors.New("Browser collection cleanup reason does not match the requested surface")
			}
		case "surface_already_closed":
		default:
			return errors.New("Browser collection cleanup receipt has an unsupported reason")
		}
	}
	return nil
}

func sameOptionalString(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// noOwnedSurfaceCleanupOutcome is used only by split relay paths that cannot
// own Browser capture surfaces (headless collection or direct-reader mode).
// The validated action identifiers are echoed into the strict receipt.
func noOwnedSurfaceCleanupOutcome(leaseID, source string) json.RawMessage {
	var sourceEcho *string
	if source != "" {
		value := source
		sourceEcho = &value
	}
	type cleanupReceipt struct {
		Schema      string  `json:"schema"`
		LeaseID     string  `json:"leaseId"`
		Source      *string `json:"source"`
		Expected    int     `json:"expected"`
		Closed      int     `json:"closed"`
		Transferred int     `json:"transferred"`
		Retained    int     `json:"retained"`
		Pending     int     `json:"pending"`
		Complete    bool    `json:"complete"`
	}
	result := struct {
		Outcome struct {
			Reason  string         `json:"reason"`
			Cleanup cleanupReceipt `json:"cleanup"`
		} `json:"outcome"`
	}{}
	result.Outcome.Reason = "no_owned_surface"
	result.Outcome.Cleanup = cleanupReceipt{
		Schema: "aku.surface-cleanup.v1", LeaseID: leaseID, Source: sourceEcho, Complete: true,
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return raw
}
