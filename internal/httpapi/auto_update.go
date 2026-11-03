package httpapi

import (
	"net/http"
	"path"
	"strings"
)

func (s *Server) revealPreparedBatch(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/reveal"))
	var body struct {
		Presentation string `json:"presentation"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	batch, err := s.engine.RevealPreparedBatch(ctx, id, body.Presentation)
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"batch": batch})
}

func (s *Server) resetAutoUpdateBudget(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	status, err := s.engine.ResetAutoUpdateDailyQuota(ctx)
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
}

func (s *Server) restoreAutoUpdateUsage(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	status, err := s.engine.ConfirmAutoUpdateUsageRestored(ctx)
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
}

func (s *Server) prepareAutoUpdate(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	session, err := s.engine.StartPreparedUpdateNow(ctx)
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"session": session})
}

func (s *Server) recordUIActivity(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	s.engine.RecordUIAccess(ctx)
	status, err := s.engine.AutoUpdateStatus(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"recorded": true, "autoUpdate": status})
}

func (s *Server) autoUpdateStatus(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	status, err := s.engine.AutoUpdateStatus(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"autoUpdate": status})
}
