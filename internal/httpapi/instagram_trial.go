package httpapi

import (
	"context"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
)

func (s *Server) instagramTrialAction(w http.ResponseWriter, r *http.Request, ctx context.Context, start bool) error {
	if !s.config.Dev || !s.config.Capture.InstagramTrial {
		return notFound("development trial route")
	}
	if r.Header.Get("X-Aku-Split-Epoch") != s.engine.Epoch() {
		return apiError{Status: 409, Code: "capture_epoch_mismatch", Message: "AkuBrowser restarted; refresh the page."}
	}
	if start {
		v, err := readConsentFields(r, []string{"acknowledgeLimitations", "allowProcessSessionDerivation", "acknowledgeUnqualifiedRecipe"})
		if err != nil {
			return err
		}
		if err = s.engine.StartInstagramTrial(ctx, engine.XDailyStartRequest{AcknowledgeLimitations: v["acknowledgeLimitations"], AllowProcessSessionDerivation: v["allowProcessSessionDerivation"]}, v["acknowledgeUnqualifiedRecipe"]); err != nil {
			return apiError{Status: 409, Code: "instagram_trial_start_unavailable", Message: "Instagram trial startup was rejected or failed; inspect its status."}
		}
	} else {
		var request struct{}
		if err := readJSON(r, &request); err != nil {
			return err
		}
		if err := s.engine.RevokeInstagramDailyPermission(ctx); err != nil {
			return apiError{Status: 409, Code: "instagram_trial_cleanup_uncertain", Message: "Instagram trial cleanup has not been verified."}
		}
	}
	return writeJSON(w, 200, map[string]any{"instagramCapture": s.engine.SourceCaptureStatus(domain.SourceInstagram)})
}
