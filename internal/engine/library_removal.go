package engine

import (
	"context"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) RemoveLibraryItem(ctx context.Context, id string) error {
	topicIDs, err := e.store.LivingTopicIDsForMemory(ctx, id)
	if err != nil {
		return err
	}
	if err := e.store.RemoveMemory(ctx, id); err != nil {
		return err
	}
	for _, topicID := range topicIDs {
		e.launchLivingTopicUnderstanding(topicID, "evidence_removed")
	}
	return nil
}
func (e *Engine) ForgetLibraryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	topicIDs, err := e.store.LivingTopicIDsForMemory(ctx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	item, err := e.store.ForgetMemory(ctx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	for _, topicID := range topicIDs {
		e.launchLivingTopicUnderstanding(topicID, "evidence_forgotten")
	}
	return item, nil
}
