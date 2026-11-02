package httpapi

import (
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return err
	}
	token, err := s.store.BridgeToken(ctx)
	if err != nil {
		return err
	}
	active, err := s.engine.ActiveSession(ctx)
	if err != nil {
		return err
	}
	timeline, err := s.engine.Timeline(ctx, settings.TimelineCapacity, 0)
	if err != nil {
		return err
	}
	latestCheck, err := s.engine.LatestTimelineCheck(ctx)
	if err != nil {
		return err
	}
	timelineBatches, err := s.engine.TimelineBatchSummaries(ctx)
	if err != nil {
		return err
	}
	onboarding, err := s.engine.Onboarding(ctx)
	if err != nil {
		return err
	}
	calibration, err := s.engine.CalibrationOverview(ctx)
	if err != nil {
		return err
	}
	autoUpdate, err := s.engine.AutoUpdateStatus(ctx)
	if err != nil {
		return err
	}
	health, err := s.store.DatabaseHealth(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"version": domain.ApplicationVersion, "runtime": "go", "deployment": s.config.Deployment.PublicStatus(), "provider": s.engine.ProviderName(), "reasoningProviders": s.engine.ReasoningProviders(), "reasoningRuntime": s.engine.ReasoningRuntime(), "reasoningProcesses": s.engine.ReasoningProcesses(settings), "mediaProvenanceRuntime": s.engine.MediaProvenanceRuntime(), "instanceEpoch": s.engine.Epoch(), "bridgeContractVersion": domain.BridgeContractVersion, "softwareUpdate": domain.SidecarSoftwareUpdateMetadata(store.SchemaVersion), "bridgeToken": token, "bridge": s.engine.BridgeStatus(), "database": map[string]any{"status": health.Status, "schemaVersion": store.SchemaVersion}, "databaseHealth": health, "sources": domain.Sources(), "settings": settings, "collectionRuntime": s.engine.CollectionRuntime(), "xCapture": s.engine.XCaptureStatus(), "instagramCapture": s.engine.SourceCaptureStatus(domain.SourceInstagram), "onboarding": onboarding, "calibration": calibration, "activeSession": sessionProgressProjection(active), "timeline": timeline, "timelineBatches": timelineBatches, "latestCheck": latestCheck, "autoUpdate": autoUpdate})
}
