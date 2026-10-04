package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/abangkis/AkuSidecar/internal/captureruntime"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

// DispatchBrowserMediaRecapture uses the existing Bridge adapter after the
// engine has admitted a hybrid job to its exact Browser generation. Pending
// actions survive a caller timeout; retries inspect the same action's receipt.
func (s *Server) DispatchBrowserMediaRecapture(ctx context.Context, id string, generation uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !splitID(id) {
		return errors.New("invalid Browser recapture job")
	}
	t := s.splitCapture
	if t == nil {
		return errors.New("Browser recapture transport unavailable")
	}
	t.mu.Lock()
	if t.closed || t.runtime == nil {
		t.mu.Unlock()
		return errors.New("Browser recapture transport stopped")
	}
	snapshot := t.runtime.Snapshot()
	if snapshot.State != captureruntime.Ready || snapshot.Driver != "browser" || snapshot.Generation != generation {
		t.mu.Unlock()
		return errors.New("Browser recapture belongs to another owner")
	}
	var entry *pendingSplitAction
	for _, candidate := range t.actions {
		if candidate.browserRecaptureGeneration == generation && candidate.action.RecaptureID == id {
			entry = candidate
			break
		}
	}
	if entry == nil {
		if len(t.actions) >= splitActionLimit {
			t.mu.Unlock()
			return errors.New("Browser recapture queue is full")
		}
		lease, err := t.runtime.Acquire()
		if err != nil {
			t.mu.Unlock()
			return err
		}
		if lease.Driver() != "browser" || lease.Generation() != generation {
			lease.Release()
			t.mu.Unlock()
			return errors.New("Browser recapture owner changed")
		}
		entry = &pendingSplitAction{
			action:   splitCaptureAction{ID: domain.NewID("recapture_dispatch"), Type: "media_recapture", RecaptureID: id},
			queuedAt: time.Now(), result: make(chan splitActionResult, 1), runtimeLease: lease,
			browserRecaptureGeneration: generation,
		}
		t.actions = append(t.actions, entry)
	}
	if entry.completed {
		result := entry.completionResult
		t.mu.Unlock()
		return browserRecaptureDispatchResult(id, result)
	}
	t.mu.Unlock()
	t.notifyCapture()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return errors.New("Browser recapture transport stopped")
	case result := <-entry.result:
		return browserRecaptureDispatchResult(id, &result)
	}
}

func browserRecaptureDispatchResult(id string, result *splitActionResult) error {
	if result == nil || !result.OK {
		return errors.New("Browser recapture dispatch was rejected or unverified")
	}
	var outcome struct {
		Recapture struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"recapture"`
	}
	if json.Unmarshal(result.Result, &outcome) != nil || outcome.Recapture.ID != id || outcome.Recapture.Status != "completed" {
		return errors.New("Browser recapture dispatch acknowledgement lacks a matching completed job")
	}
	return nil
}

func sidecarOwnsBrowserRecapture(job domain.MediaRecapture) bool {
	_, exists, err := store.MediaRecaptureAdmission(job.Payload)
	return job.Source == domain.SourceFacebook && exists && err == nil
}
