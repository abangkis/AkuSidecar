package httpapi

import (
	"errors"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/store"
)

func (s *Server) databaseHealth(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	health, err := s.store.DatabaseHealth(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"databaseHealth": health})
}

func (s *Server) timelineStorage(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	status, err := s.store.TimelineStorageStatus(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"timelineStorage": status})
}

func (s *Server) cleanDatabase(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body store.DatabaseCleanupRequest
	if err := readJSON(r, &body); err != nil {
		return err
	}
	result, err := s.engine.CleanDatabase(ctx, body)
	if err != nil {
		code := "database_cleanup_unavailable"
		if errors.Is(err, store.ErrDatabaseHealthChanged) {
			code = "database_health_changed"
		}
		return apiError{Status: http.StatusConflict, Code: code, Message: err.Error()}
	}
	return writeJSON(w, http.StatusOK, result)
}
