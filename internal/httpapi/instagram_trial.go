package httpapi

import (
	"context"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func (s *Server) instagramTrialAction(w http.ResponseWriter, r *http.Request, ctx context.Context, action string) error {
	if !s.config.Dev || !s.config.Capture.InstagramTrial {
		return notFound("development trial route")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	switch action {
	case "start":
		request, acknowledgeUnqualifiedRecipe, err := readInstagramTrialConsent(r)
		if err != nil {
			return err
		}
		if err = s.engine.StartInstagramTrial(ctx, request, acknowledgeUnqualifiedRecipe); err != nil {
			return apiError{Status: 409, Code: "instagram_trial_start_unavailable", Message: "Instagram trial startup was rejected or failed; inspect its status."}
		}
	case "retry":
		request, acknowledgeUnqualifiedRecipe, err := readInstagramTrialConsent(r)
		if err != nil {
			return err
		}
		if err = s.engine.RetryInstagramTrial(ctx, request, acknowledgeUnqualifiedRecipe); err != nil {
			return apiError{Status: 409, Code: "instagram_trial_retry_unavailable", Message: "Instagram manual retry was rejected or failed; inspect its status."}
		}
	case "stop":
		var request struct{}
		if err := readJSON(r, &request); err != nil {
			return err
		}
		if err := s.engine.RevokeInstagramDailyPermission(ctx); err != nil {
			return apiError{Status: 409, Code: "instagram_trial_cleanup_uncertain", Message: "Instagram trial cleanup has not been verified."}
		}
	default:
		return notFound("development trial action")
	}
	return writeJSON(w, 200, map[string]any{"instagramCapture": s.engine.SourceCaptureStatus(domain.SourceInstagram)})
}

func readInstagramTrialConsent(r *http.Request) (engine.XDailyStartRequest, bool, error) {
	v, err := readConsentFields(r, []string{"acknowledgeLimitations", "allowProcessSessionDerivation", "acknowledgeUnqualifiedRecipe"})
	return engine.XDailyStartRequest{
		AcknowledgeLimitations:        v["acknowledgeLimitations"],
		AllowProcessSessionDerivation: v["allowProcessSessionDerivation"],
	}, v["acknowledgeUnqualifiedRecipe"], err
}
