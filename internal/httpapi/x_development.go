package httpapi

import "net/http"

func (s *Server) startXDaily(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !s.config.Dev {
		return notFound("development route")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	request, err := readXDailyStart(r)
	if err != nil {
		return err
	}
	if err = s.engine.StartXDaily(ctx, request); err != nil {
		return apiError{Status: 409, Code: "daily_start_unavailable", Message: "Daily X startup failed or consent was consumed; inspect runtime status."}
	}
	return writeJSON(w, 200, map[string]any{"xCapture": s.engine.XCaptureStatus()})
}

func (s *Server) stopXDaily(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !s.config.Dev {
		return notFound("development route")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	var request struct{}
	if err := readJSON(r, &request); err != nil {
		return err
	}
	if err := s.engine.RevokeXDailyPermission(ctx); err != nil {
		return apiError{Status: 409, Code: "daily_cleanup_uncertain", Message: "Daily X cleanup has not been verified."}
	}
	return writeJSON(w, 200, map[string]any{"xCapture": s.engine.XCaptureStatus()})
}

func (s *Server) startXManual(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !s.config.Dev {
		return notFound("development route")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	request, err := readXManualStart(r)
	if err != nil {
		return err
	}
	if err := s.engine.StartXManual(ctx, request); err != nil {
		return apiError{Status: 409, Code: "manual_start_unavailable", Message: "Manual X start was rejected or failed; inspect runtime status and cleanup."}
	}
	return writeJSON(w, http.StatusOK, map[string]any{"xManual": s.engine.XManualStatus()})
}

func (s *Server) stopXManual(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	if !s.config.Dev {
		return notFound("development route")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	var request struct{}
	if err := readJSON(r, &request); err != nil {
		return err
	}
	if err := s.engine.StopXManual(ctx); err != nil {
		return apiError{Status: 409, Code: "manual_cleanup_uncertain", Message: "Manual X cleanup has not been verified."}
	}
	return writeJSON(w, http.StatusOK, map[string]any{"xManual": s.engine.XManualStatus()})
}
