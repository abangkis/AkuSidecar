package httpapi

import (
	"net/http"
	"time"
)

func (s *Server) exportDiagnostics(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	since := r.URL.Query().Get("since")
	until := r.URL.Query().Get("until")
	sessionID := r.URL.Query().Get("sessionId")
	export, err := s.engine.DiagnosticsExport(ctx, since, until, sessionID)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Disposition", "attachment; filename=aku-diagnostics-"+time.Now().UTC().Format("20060102T150405")+".json")
	return writeJSON(w, http.StatusOK, export)
}
