package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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
		return browserCollectionCleanupResult(result)
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
		return browserCollectionCleanupResult(&result)
	}
}

func browserCollectionCleanupResult(result *splitActionResult) error {
	if result == nil || !result.OK {
		return errors.New("Browser collection cleanup was rejected or unverified")
	}
	var outcome struct {
		Reason   string `json:"reason"`
		Released *bool  `json:"released"`
		Mode     string `json:"mode"`
	}
	if len(result.Result) == 0 || json.Unmarshal(result.Result, &outcome) != nil {
		return errors.New("Browser collection cleanup acknowledgement is invalid")
	}
	if outcome.Reason != "" && outcome.Reason != "no_owned_surface" {
		return errors.New("Browser collection cleanup did not release its owned surfaces")
	}
	if outcome.Released == nil || (outcome.Reason == "" && outcome.Mode == "") {
		return errors.New("Browser collection cleanup acknowledgement lacks an outcome")
	}
	if outcome.Reason == "" {
		switch outcome.Mode {
		case "owned_window_closed", "owned_windows_closed", "owned_transient_tabs_closed", "owned_tabs_closed_user_window_preserved":
		default:
			return errors.New("Browser collection cleanup outcome is unsupported")
		}
	}
	return nil
}
