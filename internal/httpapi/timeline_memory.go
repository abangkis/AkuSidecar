package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/store"
)

func (s *Server) keepTimelineFullCopy(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	// Legacy API compatibility. The embedded UI exposes Read later, while
	// older clients may finish an already-issued Keep request.
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/timeline/"), "/keep-full-copy")
	if id == "" || strings.Contains(id, "/") {
		return notFound("timeline item")
	}
	item, alreadyKept, err := s.engine.KeepTimelineFullCopy(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if errors.Is(err, store.ErrMemoryTombstoned) {
		return apiError{Status: http.StatusConflict, Code: "memory_tombstoned", Message: "This Timeline item was permanently forgotten, so a full copy cannot be kept."}
	}
	if errors.Is(err, store.ErrTimelineMemoryTextUnavailable) {
		return apiError{Status: http.StatusConflict, Code: "timeline_memory_text_unavailable", Message: "The source text is unavailable for this Timeline item, so no full copy was kept."}
	}
	if errors.Is(err, store.ErrTimelineMemoryNotEligible) {
		return apiError{Status: http.StatusConflict, Code: "timeline_memory_not_eligible", Message: "Only a final Timeline item from a completed update can be kept as a full copy."}
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"kept": true, "alreadyKept": alreadyKept, "retentionTier": item.RetentionTier})
}

func (s *Server) readLaterTimeline(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/timeline/"), "/read-later")
	if id == "" || strings.Contains(id, "/") {
		return notFound("timeline item")
	}
	item, alreadySaved, err := s.engine.ReadLaterTimeline(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if errors.Is(err, store.ErrMemoryTombstoned) {
		return apiError{Status: http.StatusConflict, Code: "memory_tombstoned", Message: "This Timeline item was permanently forgotten, so it cannot be Saved."}
	}
	if errors.Is(err, store.ErrTimelineMemoryNotEligible) {
		return apiError{Status: http.StatusConflict, Code: "timeline_memory_not_eligible", Message: "Only a final Timeline item from a completed update can be Saved for later."}
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"saved": true, "alreadySaved": alreadySaved, "retentionTier": item.RetentionTier, "permanentKeep": item.PermanentKeep})
}
