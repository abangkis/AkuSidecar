package engine

import (
	"context"
	"errors"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) CorrectSelection(ctx context.Context, runID, candidateRef string) (domain.SelectionCorrection, domain.TimelineItem, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	} else if active != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, errors.New("finish the active update before correcting an earlier selection")
	}
	correction, item, err := e.store.CreateSelectionCorrection(ctx, runID, candidateRef)
	if err != nil {
		return domain.SelectionCorrection{}, domain.TimelineItem{}, err
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return correction, item, err
	}
	if e.events != nil && settings.SemanticEventMode != "show_all" {
		if _, eventErr := e.events.ProcessTimelineItem(ctx, item.ID, settings); eventErr != nil {
			e.logger.Printf("semantic event resolution for selection correction %s degraded safely: %v", correction.ID, eventErr)
		}
	}
	if err := e.store.SaveSelectionCorrectionKnowledge(ctx, correction.ID); err != nil {
		e.logger.Printf("knowledge continuity for selection correction %s degraded safely: %v", correction.ID, err)
	}
	item, err = e.store.TimelineItem(ctx, item.ID)
	if err != nil {
		return correction, domain.TimelineItem{}, err
	}
	if settings.AIDetectionEnabled {
		if err := e.store.SaveAIAssessments(ctx, e.aiFast.Detect([]domain.TimelineItem{item})); err != nil {
			e.logger.Printf("AI Fast Detection for selection correction %s degraded safely: %v", correction.ID, err)
		}
	}
	item, err = e.store.TimelineItem(ctx, item.ID)
	if err != nil {
		return correction, domain.TimelineItem{}, err
	}
	if settings.AIDetectionEnabled {
		e.launchDeepDetectionItems(item.SessionID, []domain.TimelineItem{item})
		e.launchMediaProvenanceItems([]domain.TimelineItem{item})
	}
	e.launchLivingTopicRouting(item.SessionID)
	return correction, item, nil
}

func (e *Engine) UndoSelectionCorrection(ctx context.Context, id string) (domain.SelectionCorrection, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.SelectionCorrection{}, err
	} else if active != nil {
		return domain.SelectionCorrection{}, errors.New("finish the active update before undoing a selection correction")
	}
	value, err := e.store.UndoSelectionCorrection(ctx, id)
	if err != nil {
		return domain.SelectionCorrection{}, err
	}
	e.mu.Lock()
	for key, cancel := range e.work.active {
		if strings.HasPrefix(key, "ai:"+value.SessionID+":") {
			cancel()
		}
	}
	e.mu.Unlock()
	_ = e.store.RefreshEventResolutionCounts(ctx, value.SessionID)
	return value, nil
}
