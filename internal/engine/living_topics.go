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
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/livingtopics"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (e *Engine) LivingTopics(ctx context.Context) ([]domain.LivingTopic, error) {
	return e.store.ListLivingTopics(ctx)
}

func (e *Engine) LivingTopic(ctx context.Context, id string) (domain.LivingTopicDetail, error) {
	return e.store.LivingTopicDetail(ctx, id)
}

func (e *Engine) CreateLivingTopic(ctx context.Context, name string) (domain.LivingTopic, error) {
	return e.CreateLivingTopicWithCriteria(ctx, name, "")
}

func (e *Engine) CreateLivingTopicWithCriteria(ctx context.Context, name, description string) (domain.LivingTopic, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.CreateLivingTopicWithCriteria(ctx, name, description)
}

func (e *Engine) RenameLivingTopic(ctx context.Context, id, name string) (domain.LivingTopic, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.RenameLivingTopic(ctx, id, name)
}

func (e *Engine) UpdateLivingTopicCriteria(ctx context.Context, id, name, description string) (domain.LivingTopic, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.store.UpdateLivingTopicCriteria(ctx, id, name, description)
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

func (e *Engine) ResumeLivingTopicRouting(ctx context.Context) error {
	if err := e.store.ResetRunningLivingTopicRouting(ctx); err != nil {
		return err
	}
	e.launchLivingTopicRouting("")
	return nil
}

func (e *Engine) launchLivingTopicRouting(sessionID string) {
	if sessionID != "" {
		if _, err := e.store.QueueLivingTopicRouting(context.Background(), sessionID); err != nil {
			e.logger.Printf("Living Topics routing queue degraded safely for session %s: %v", sessionID, err)
			return
		}
	}
	const key = "living-topics-routing"
	e.mu.Lock()
	if _, active := e.active[key]; active {
		e.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.active[key] = cancel
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			delete(e.active, key)
			shuttingDown := e.shuttingDown
			e.mu.Unlock()
			if !shuttingDown {
				if pending, err := e.store.HasPendingLivingTopicRouting(context.Background()); err == nil && pending {
					e.launchLivingTopicRouting("")
				}
			}
		}()
		for {
			job, err := e.store.ClaimLivingTopicRouting(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.logger.Printf("Living Topics routing claim degraded safely: %v", err)
				}
				return
			}
			if job == nil {
				return
			}
			decisions, routeErr := e.routeLivingTopicItem(ctx, job.TimelineID)
			if finishErr := e.store.FinishLivingTopicRouting(context.Background(), job.ID, decisions, routeErr); finishErr != nil {
				e.logger.Printf("Living Topics routing job %s could not persist: %v", job.ID, finishErr)
			}
			if routeErr != nil {
				e.logger.Printf("Living Topics routing job %s degraded safely: %v", job.ID, routeErr)
			}
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
		}
	}()
}

func (e *Engine) routeLivingTopicItem(ctx context.Context, timelineID string) ([]domain.LivingTopicRoutingDecision, error) {
	item, err := e.store.LivingTopicRoutingItem(ctx, timelineID)
	if err != nil {
		return nil, err
	}
	topics, err := e.store.ListLivingTopics(ctx)
	if err != nil || len(topics) == 0 {
		return nil, err
	}
	storedExamples, err := e.store.LivingTopicFeedbackExamples(ctx, 3)
	if err != nil {
		return nil, err
	}
	examples := make([]domain.LivingTopicRoutingExample, 0, len(storedExamples))
	for _, value := range storedExamples {
		examples = append(examples, domain.LivingTopicRoutingExample{TopicID: value.TopicID, Verdict: value.Verdict, Item: value.Item})
	}
	decisions := make([]domain.LivingTopicRoutingDecision, 0, len(topics))
	remaining := make([]domain.LivingTopic, 0, len(topics))
	for _, topic := range topics {
		score, reason := deterministicLivingTopicScore(item, topic, examples)
		if score >= 0.70 {
			decisions = append(decisions, domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: true, Confidence: score, Mode: "deterministic", Reason: reason})
		} else {
			remaining = append(remaining, topic)
		}
	}
	if router, ok := e.topics.(livingtopics.Router); ok && len(remaining) > 0 {
		settings, settingsErr := e.store.GetSettings(ctx)
		if settingsErr != nil {
			return nil, settingsErr
		}
		llm, _, _, routeErr := router.RouteWithProfile(ctx, item, remaining, examples, settings.ReasoningEvaluationProfile)
		if routeErr != nil {
			e.logger.Printf("Living Topics semantic routing degraded to deterministic for Timeline %s: %v", timelineID, routeErr)
		} else {
			decisions = append(decisions, llm...)
		}
	}
	for _, decision := range decisions {
		if !decision.Match || decision.Confidence < 0.65 {
			continue
		}
		if err := e.store.AddAutomaticLivingTopicMember(ctx, decision.TopicID, item, decision); err != nil && !errors.Is(err, store.ErrLivingTopicMemberMax) {
			return decisions, err
		}
	}
	return decisions, nil
}

func deterministicLivingTopicScore(item domain.TimelineItem, topic domain.LivingTopic, examples []domain.LivingTopicRoutingExample) (float64, string) {
	postTokens := livingTopicTokens(strings.Join([]string{item.Item.WhatChanged, item.Item.WhyItMatters, strings.Join(item.Assessment.TopicTags, " "), strings.Join(item.Assessment.TopicFacets, " ")}, " "))
	criteriaTokens := livingTopicTokens(topic.Name + " " + topic.Description)
	base := tokenCoverage(postTokens, criteriaTokens)
	positive, negative := 0.0, 0.0
	for _, example := range examples {
		if example.TopicID != topic.ID {
			continue
		}
		tokens := livingTopicTokens(example.Item.Title + " " + example.Item.Summary + " " + strings.Join(example.Item.Tags, " ") + " " + strings.Join(example.Item.Facets, " "))
		overlap := tokenJaccard(postTokens, tokens)
		if example.Verdict == "include" && overlap > positive {
			positive = overlap
		}
		if example.Verdict == "exclude" && overlap > negative {
			negative = overlap
		}
	}
	score := base + 0.25*positive - 0.40*negative
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, fmt.Sprintf("Criteria token coverage %.2f; positive similarity %.2f; negative similarity %.2f", base, positive, negative)
}

func livingTopicTokens(value string) map[string]bool {
	value = strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, value))
	stop := map[string]bool{"the": true, "and": true, "for": true, "with": true, "from": true, "that": true, "this": true, "yang": true, "dan": true, "untuk": true, "dari": true, "dengan": true, "atau": true}
	result := map[string]bool{}
	for _, token := range strings.Fields(value) {
		if len([]rune(token)) >= 3 && !stop[token] {
			result[token] = true
		}
	}
	return result
}
func tokenCoverage(post, criteria map[string]bool) float64 {
	if len(criteria) == 0 {
		return 0
	}
	hits := 0
	for token := range criteria {
		if post[token] {
			hits++
		}
	}
	return float64(hits) / float64(len(criteria))
}
func tokenJaccard(left, right map[string]bool) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	intersection := 0
	union := map[string]bool{}
	for token := range left {
		union[token] = true
	}
	for token := range right {
		if left[token] {
			intersection++
		}
		union[token] = true
	}
	return float64(intersection) / float64(len(union))
}
