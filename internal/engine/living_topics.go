package engine

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) LivingTopics(ctx context.Context) ([]domain.LivingTopic, error) {
	return e.store.ListLivingTopics(ctx)
}

func (e *Engine) LivingTopic(ctx context.Context, id string) (domain.LivingTopicDetail, error) {
	return e.store.LivingTopicDetail(ctx, id)
}

func (e *Engine) CreateLivingTopic(ctx context.Context, name string) (domain.LivingTopic, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.CreateLivingTopic(ctx, name)
}

func (e *Engine) RenameLivingTopic(ctx context.Context, id, name string) (domain.LivingTopic, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.RenameLivingTopic(ctx, id, name)
}

func (e *Engine) AddLivingTopicMember(ctx context.Context, topicID, memoryID string) (domain.LivingTopicDetail, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.AddLivingTopicMember(ctx, topicID, memoryID)
}

func (e *Engine) RemoveLivingTopicMember(ctx context.Context, topicID, memoryID string) (domain.LivingTopicDetail, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.RemoveLivingTopicMember(ctx, topicID, memoryID)
}

func (e *Engine) CreateLivingTopicSnapshot(ctx context.Context, topicID string) (domain.LivingTopicSnapshot, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if active, err := e.store.ActiveSession(ctx); err != nil {
		return domain.LivingTopicSnapshot{}, err
	} else if active != nil {
		return domain.LivingTopicSnapshot{}, errors.New("finish the active update before creating a Living Topic snapshot")
	}
	detail, err := e.store.LivingTopicDetail(ctx, topicID)
	if err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	digest, evidenceIDs, err := livingTopicInputDigest(detail.Members)
	if err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	previous, err := e.store.LatestLivingTopicSnapshot(ctx, topicID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.LivingTopicSnapshot{}, err
	}
	var previousPtr *domain.LivingTopicSnapshot
	if err == nil {
		previousPtr = &previous
	}
	base := domain.LivingTopicSnapshot{TopicID: topicID, EvidenceIDs: evidenceIDs, InputDigest: digest, Claims: []domain.LivingTopicClaim{}, Deltas: []domain.LivingTopicDelta{}}
	if previousPtr != nil {
		base.PreviousSnapshotID = previousPtr.ID
	}
	if len(detail.Members) == 0 {
		base.Status = "insufficient_evidence"
		base.Overview = "Add Library evidence before creating a topic snapshot."
		base.Provider, base.Model, base.Effort = "local-deterministic", "none", "none"
		return e.store.SaveLivingTopicSnapshot(ctx, base)
	}
	if previousPtr != nil && previousPtr.InputDigest == digest {
		base.Status = "no_change"
		base.Overview = "No evidence changed since the previous snapshot."
		base.Claims = append([]domain.LivingTopicClaim(nil), previousPtr.Claims...)
		base.Provider, base.Model, base.Effort = "local-deterministic", "none", "none"
		return e.store.SaveLivingTopicSnapshot(ctx, base)
	}
	if e.topics == nil {
		return domain.LivingTopicSnapshot{}, errors.New("the selected reasoning provider does not support Living Topic snapshots")
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.LivingTopicSnapshot{}, err
	}
	key := "living-topic:" + topicID
	invokeCtx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	if e.shuttingDown {
		e.mu.Unlock()
		cancel()
		return domain.LivingTopicSnapshot{}, errors.New("AkuSidecar is shutting down")
	}
	e.active[key] = cancel
	e.mu.Unlock()
	defer func() {
		cancel()
		e.mu.Lock()
		delete(e.active, key)
		e.mu.Unlock()
	}()
	result, usage, duration, err := e.topics.ResolveWithProfile(invokeCtx, detail.Topic, detail.Members, previousPtr, settings.ReasoningEvaluationProfile)
	if err != nil {
		return domain.LivingTopicSnapshot{}, fmt.Errorf("create Living Topic snapshot: %w", err)
	}
	model := e.topics.ModelForProfile(settings.ReasoningEvaluationProfile)
	base.Status, base.Overview, base.Claims, base.Deltas = result.Status, result.Overview, result.Claims, result.Deltas
	base.Provider, base.Model, base.Effort = e.topics.Name(), model.DisplayModel(), model.DisplayEffort()
	if usage.ProviderModel != "" {
		base.Model = usage.ProviderModel
	}
	if usage.NativeReasoning != "" {
		base.Effort = usage.NativeReasoning
	}
	base.DurationMS, base.Usage = duration.Milliseconds(), usage
	return e.store.SaveLivingTopicSnapshot(ctx, base)
}

func livingTopicInputDigest(items []domain.MemoryItem) (string, []string, error) {
	type digestItem struct {
		ID, UpdatedAt, Title, Summary, Author, FullContent string
		Source                                             domain.Source
		PublishedAt                                        *string
		Tags, Facets                                       []string
	}
	values := make([]digestItem, 0, len(items))
	ids := make([]string, 0, len(items))
	for _, item := range items {
		full := ""
		if item.FullContent != nil {
			full = *item.FullContent
		}
		values = append(values, digestItem{ID: item.ID, UpdatedAt: item.UpdatedAt, Title: item.Title, Summary: item.Summary, Author: item.Author, FullContent: full, Source: item.Source, PublishedAt: item.PublishedAt, Tags: item.Tags, Facets: item.Facets})
		ids = append(ids, item.ID)
	}
	sort.Slice(values, func(i, j int) bool { return values[i].ID < values[j].ID })
	sort.Strings(ids)
	raw, err := json.Marshal(values)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return strings.ToLower(hex.EncodeToString(sum[:])), ids, nil
}
