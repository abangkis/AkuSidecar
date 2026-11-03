package httpapi

import (
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Server) collectionRuntimeStatus(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, http.StatusOK, map[string]any{"collectionRuntime": s.engine.CollectionRuntime(), "xManual": s.engine.XManualStatus(), "xCapture": s.engine.XCaptureStatus(), "instagramCapture": s.engine.SourceCaptureStatus(domain.SourceInstagram)})
}

func (s *Server) prepareInteractiveCapture(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if err := s.engine.PrepareInteractiveCapture(ctx); err != nil {
		return apiError{Status: 409, Code: "interactive_handoff_unavailable", Message: err.Error()}
	}
	return writeJSON(w, http.StatusOK, map[string]any{"collectionRuntime": s.engine.CollectionRuntime()})
}
