package httpapi

import (
	"net/http"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (s *Server) health(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "version": domain.ApplicationVersion, "runtime": "go", "deployment": s.config.Deployment.PublicStatus(), "provider": s.engine.ProviderName(), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime(), "bridgeContractVersion": domain.BridgeContractVersion, "softwareUpdate": domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion), "instanceEpoch": s.engine.Epoch(), "uptimeMs": time.Since(s.started).Milliseconds(), "database": map[string]any{"status": "healthy"}, "loadProfile": settings.LoadProfile})
}
