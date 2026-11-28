package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/operational"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
)

func (e *Engine) SetMediaProvenanceInspector(value mediaprovenance.Inspector) {
	e.mediaOrigin = value
}

func (e *Engine) MediaProvenanceRuntime() domain.MediaProvenanceRuntime {
	if e.mediaOrigin == nil {
		return domain.MediaProvenanceRuntime{Provider: mediaprovenance.ProviderName, Version: mediaprovenance.VerifierVersion}
	}
	return domain.MediaProvenanceRuntime{
		Provider: e.mediaOrigin.Name(), Version: e.mediaOrigin.Version(), Available: e.mediaOrigin.Available(),
	}
}

func (e *Engine) ResumeMediaProvenance(ctx context.Context) error {
	if e.mediaOrigin == nil || !e.mediaOrigin.Available() {
		return nil
	}
	if err := e.store.CancelRunningMediaProvenance(ctx); err != nil {
		return err
	}
	e.launchMediaProvenanceItems(nil)
	return nil
}

func (e *Engine) launchMediaProvenance(sessionID string) {
	inspector := e.mediaOrigin
	if inspector == nil || !inspector.Available() {
		return
	}
	items, err := e.store.ListSessionItems(context.Background(), sessionID)
	if err != nil {
		operational.Problem(e.logger, "media_provenance", "load_session", "failed", err, sessionID)
		return
	}
	e.launchMediaProvenanceItems(items)
}

func (e *Engine) launchMediaProvenanceItems(items []domain.TimelineItem) {
	inspector := e.mediaOrigin
	if inspector == nil || !inspector.Available() {
		return
	}
	if len(items) > 0 {
		if _, err := e.store.QueueMediaProvenance(context.Background(), items, inspector.Name(), inspector.Version()); err != nil {
			operational.Problem(e.logger, "media_provenance", "enqueue", "degraded", err, "")
			return
		}
	}
	const key = "media-provenance"
	e.mu.Lock()
	if _, active := e.work.active[key]; active {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.work.registerActive(key, cancel)
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			e.work.retireActive(key)
			e.mu.Unlock()
		}()
		for {
			assessment, err := e.store.ClaimMediaProvenance(ctx, inspector.Version())
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					operational.Problem(e.logger, "media_provenance", "claim", "degraded", err, "")
				}
				return
			}
			if assessment == nil {
				return
			}
			descriptor, ok := domain.SourceByID(assessment.Source)
			if !ok {
				_ = e.store.FinishMediaProvenance(context.Background(), assessment.ID, mediaprovenance.Result{
					ManifestState: "unsupported", TrustState: "not_applicable", AIOrigin: "unknown",
				}, fmt.Errorf("source %q has no media provenance contract", assessment.Source))
				continue
			}
			result, inspectErr := inspector.Inspect(ctx, *assessment, descriptor.TrustedMediaHostSuffixes)
			if finishErr := e.store.FinishMediaProvenance(context.Background(), assessment.ID, result, inspectErr); finishErr != nil {
				operational.Problem(e.logger, "media_provenance", "persist", "failed", finishErr, assessment.ID)
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
		}
	}()
}
