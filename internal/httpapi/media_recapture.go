package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func (s *Server) mediaRecaptureStatus(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimPrefix(p, "/api/media-recaptures/")
	if id == "" || strings.Contains(id, "/") {
		return notFound("media recapture")
	}
	job, err := s.engine.MediaRecapture(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("media recapture")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"recapture": map[string]any{"id": job.ID, "status": job.Status, "outcome": job.Outcome, "error": job.Error}})
}

func (s *Server) queueMediaRecapture(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/recapture"))
	var body struct {
		CaptureMode domain.MediaRecaptureMode   `json:"captureMode"`
		Reason      domain.MediaRecaptureReason `json:"reason"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	if body.CaptureMode != domain.MediaRecaptureBackground && body.CaptureMode != domain.MediaRecaptureForeground {
		return badRequest("captureMode must be background or foreground")
	}
	if body.Reason == "" {
		body.Reason = domain.MediaRecaptureMissingMedia
	}
	if !body.Reason.Valid() {
		return badRequest("reason must be missing_media, playback_error, or unresolved_video")
	}
	recapture, err := s.engine.QueueMediaRecaptureForReason(ctx, id, body.CaptureMode, body.Reason)
	if errors.Is(err, engine.ErrFacebookRecaptureUnavailable) {
		return apiError{Status: http.StatusConflict, Code: "facebook_browser_recapture_required", Message: "Facebook Recapture currently requires Browser mode. Select Browser in Settings, then retry."}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"recapture": recapture})
}

func (s *Server) acceptPassiveMediaEvidence(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	if err := s.requireBridge(r); err != nil {
		return err
	}
	id := path.Base(strings.TrimSuffix(p, "/media-evidence"))
	var body domain.PassiveXMediaEvidence
	if err := readJSON(r, &body); err != nil {
		return err
	}
	recapture, updated, err := s.engine.ApplyPassiveXMediaEvidence(ctx, id, r.Header.Get("X-Aku-Bridge-Id"), body)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	response := map[string]any{"updated": updated}
	if updated {
		response["recapture"] = recapture
	}
	return writeJSON(w, http.StatusOK, response)
}
