package engine

import (
	"context"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) AddAIFeedback(ctx context.Context, timelineID string, input domain.AIFeedbackInput) (domain.AIFeedbackEvent, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	feedback, err := e.store.AddAIFeedback(ctx, timelineID, input)
	if err != nil || input.Verdict != "unsure" {
		return feedback, err
	}
	settings, settingsErr := e.store.GetSettings(ctx)
	if settingsErr != nil {
		e.logger.Printf("AI Deep Detection could not inspect settings for feedback %s: %v", feedback.ID, settingsErr)
		return feedback, nil
	}
	if !settings.AIDetectionEnabled {
		return feedback, nil
	}
	item, itemErr := e.store.TimelineItem(ctx, timelineID)
	if itemErr != nil {
		e.logger.Printf("AI Deep Detection could not load feedback item %s: %v", feedback.ID, itemErr)
		return feedback, nil
	}
	e.launchDeepDetectionItems(item.SessionID, []domain.TimelineItem{item})
	return feedback, nil
}
func (e *Engine) UndoAIFeedback(ctx context.Context, id string) (domain.AIFeedbackEvent, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.UndoAIFeedback(ctx, id)
}
func (e *Engine) AIFeedbackHistory(ctx context.Context, timelineID string) ([]domain.AIFeedbackEvent, error) {
	return e.store.AIFeedbackHistory(ctx, timelineID)
}
