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
	"time"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/livingtopics"
	"github.com/abangkis/AkuSidecar/internal/store"
)

const livingTopicUnderstandingContractVersion = "current-projection-v4"

func (e *Engine) LivingTopics(ctx context.Context) ([]domain.LivingTopic, error) {
	return e.store.ListLivingTopics(ctx)
}

func (e *Engine) LivingTopicNotifications(ctx context.Context) (domain.LivingTopicNotificationSummary, error) {
	return e.store.LivingTopicNotificationSummary(ctx)
}

func (e *Engine) LivingTopic(ctx context.Context, id string) (domain.LivingTopicDetail, error) {
	return e.store.LivingTopicDetail(ctx, id)
}

func (e *Engine) AcknowledgeLivingTopicEvidence(ctx context.Context, id, seenThrough string) (domain.LivingTopic, error) {
	return e.store.AcknowledgeLivingTopicEvidence(ctx, id, seenThrough)
}

func (e *Engine) CreateLivingTopic(ctx context.Context, name string) (domain.LivingTopic, error) {
	return e.CreateLivingTopicWithCriteria(ctx, name, "")
}

func (e *Engine) CreateLivingTopicWithCriteria(ctx context.Context, name, description string) (domain.LivingTopic, error) {
	return e.CreateLivingTopicWithRoutingCriteria(ctx, domain.LivingTopicCriteriaInput{Name: name, Description: description})
}

func (e *Engine) CreateLivingTopicWithRoutingCriteria(ctx context.Context, input domain.LivingTopicCriteriaInput) (domain.LivingTopic, error) {
	e.operation.Lock()
	topic, err := e.store.CreateLivingTopicWithRoutingCriteria(ctx, input)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopic{}, err
	}
	e.launchLivingTopicActivation(topic.ID, "topic_created")
	e.launchLivingTopicUnderstanding(topic.ID, "topic_created")
	return e.store.LivingTopic(ctx, topic.ID)
}

func (e *Engine) RenameLivingTopic(ctx context.Context, id, name string) (domain.LivingTopic, error) {
	current, err := e.store.LivingTopic(ctx, id)
	if err != nil {
		return domain.LivingTopic{}, err
	}
	return e.UpdateLivingTopicCriteria(ctx, id, name, current.Description)
}

func (e *Engine) UpdateLivingTopicCriteria(ctx context.Context, id, name, description string) (domain.LivingTopic, error) {
	current, err := e.store.LivingTopic(ctx, id)
	if err != nil {
		return domain.LivingTopic{}, err
	}
	return e.UpdateLivingTopicRoutingCriteria(ctx, id, domain.LivingTopicCriteriaInput{Name: name, Description: description, Aliases: current.Aliases, IncludeCriteria: current.IncludeCriteria, ExcludeCriteria: current.ExcludeCriteria})
}

func (e *Engine) UpdateLivingTopicRoutingCriteria(ctx context.Context, id string, input domain.LivingTopicCriteriaInput) (domain.LivingTopic, error) {
	e.operation.Lock()
	_, changed, err := e.store.UpdateLivingTopicRoutingCriteria(ctx, id, input)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopic{}, err
	}
	if changed {
		e.launchLivingTopicActivation(id, "criteria_changed")
		e.launchLivingTopicUnderstanding(id, "criteria_changed")
	}
	return e.store.LivingTopic(ctx, id)
}

func (e *Engine) ReviewLivingTopicCandidate(ctx context.Context, topicID, memoryID, action string) (domain.LivingTopicDetail, error) {
	e.operation.Lock()
	_, err := e.store.ReviewLivingTopicCandidate(ctx, topicID, memoryID, action)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	e.launchLivingTopicUnderstanding(topicID, "candidate_"+action)
	return e.store.LivingTopicDetail(ctx, topicID)
}

func (e *Engine) RequestLivingTopicActivation(ctx context.Context, topicID, trigger string) (domain.LivingTopicDetail, error) {
	if _, err := e.store.QueueLivingTopicActivation(ctx, topicID, trigger); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	e.launchLivingTopicActivation("", "")
	return e.store.LivingTopicDetail(ctx, topicID)
}

func (e *Engine) AddLivingTopicMember(ctx context.Context, topicID, memoryID string) (domain.LivingTopicDetail, error) {
	e.operation.Lock()
	_, err := e.store.AddLivingTopicMember(ctx, topicID, memoryID)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	e.launchLivingTopicUnderstanding(topicID, "evidence_added")
	return e.store.LivingTopicDetail(ctx, topicID)
}

func (e *Engine) RemoveLivingTopicMember(ctx context.Context, topicID, memoryID string) (domain.LivingTopicDetail, error) {
	e.operation.Lock()
	_, err := e.store.RemoveLivingTopicMember(ctx, topicID, memoryID)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopicDetail{}, err
	}
	e.launchLivingTopicUnderstanding(topicID, "evidence_removed")
	return e.store.LivingTopicDetail(ctx, topicID)
}

func (e *Engine) MoveLivingTopicMember(ctx context.Context, fromTopicID, toTopicID, memoryID string) (domain.LivingTopicMembershipMove, error) {
	e.operation.Lock()
	move, err := e.store.MoveLivingTopicMember(ctx, fromTopicID, toTopicID, memoryID)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	e.launchLivingTopicUnderstanding(move.FromTopicID, "evidence_moved_out")
	e.launchLivingTopicUnderstanding(move.ToTopicID, "evidence_moved_in")
	return move, nil
}

func (e *Engine) UndoLivingTopicMemberMove(ctx context.Context, moveID string) (domain.LivingTopicMembershipMove, error) {
	e.operation.Lock()
	move, err := e.store.UndoLivingTopicMemberMove(ctx, moveID)
	e.operation.Unlock()
	if err != nil {
		return domain.LivingTopicMembershipMove{}, err
	}
	e.launchLivingTopicUnderstanding(move.FromTopicID, "evidence_move_undone")
	e.launchLivingTopicUnderstanding(move.ToTopicID, "evidence_move_undone")
	return move, nil
}

// RequestLivingTopicUnderstanding schedules a secondary refresh. It never
// blocks the caller on provider inference; the durable worker coalesces pending
// changes and publishes a snapshot only for a material semantic delta.
func (e *Engine) RequestLivingTopicUnderstanding(ctx context.Context, topicID, trigger string) (domain.LivingTopicDetail, error) {
	if _, err := e.store.QueueLivingTopicUnderstanding(ctx, topicID, trigger); err != nil {
		return domain.LivingTopicDetail{}, err
	}
	e.launchLivingTopicUnderstanding("", "")
	return e.store.LivingTopicDetail(ctx, topicID)
}

func (e *Engine) evaluateLivingTopicUnderstanding(ctx context.Context, topicID, trigger string) (*domain.LivingTopicSnapshot, string, string, error) {
	detail, err := e.store.LivingTopicDetail(ctx, topicID)
	if err != nil {
		return nil, "", "", err
	}
	digest, evidenceIDs, err := livingTopicInputDigest(detail.Topic, detail.Members)
	if err != nil {
		return nil, "", "", err
	}
	forced := trigger == "refresh_now" || strings.HasPrefix(trigger, "migration_rebaseline")
	if !forced && detail.Topic.UnderstandingCheckedAt != "" && detail.Topic.UnderstandingInputDigest == digest {
		return nil, "no_change", digest, nil
	}
	if len(detail.Members) == 0 {
		return nil, "insufficient_evidence", digest, nil
	}
	var previousPtr *domain.LivingTopicSnapshot
	if previous, previousErr := e.store.LatestPublishedLivingTopicSnapshot(ctx, topicID); previousErr == nil {
		previousPtr = &previous
	} else if !errors.Is(previousErr, sql.ErrNoRows) {
		return nil, "", digest, previousErr
	}
	if e.topics == nil {
		return nil, "", digest, errors.New("the selected reasoning provider does not support Living Topic understanding")
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return nil, "", digest, err
	}
	// The previous projection is intentionally excluded from synthesis. It is
	// used only after validation to classify the semantic delta.
	sortLivingTopicEvidence(detail.Members)
	result, usage, duration, err := e.topics.ResolveWithProfile(ctx, detail.Topic, detail.Members, nil, settings.ReasoningEvaluationProfile)
	e.recordLivingTopicModelInvocation("living_topic_understanding", topicID, settings.ReasoningEvaluationProfile, usage, duration, err)
	if err != nil {
		return nil, "", digest, fmt.Errorf("refresh Living Topic understanding: %w", err)
	}
	if result.Status == "insufficient_evidence" {
		return nil, "insufficient_evidence", digest, nil
	}
	evidenceAsOf := annotateLivingTopicClaimTimes(result.Claims, detail.Members)
	if previousPtr != nil && previousPtr.ContractVersion == livingTopicUnderstandingContractVersion {
		result.Claims = reconcileLivingTopicClaimKeys(previousPtr.Claims, result.Claims)
	}
	material := previousPtr != nil && previousPtr.ContractVersion == livingTopicUnderstandingContractVersion && livingTopicClaimsMateriallyChanged(previousPtr.Claims, result.Claims)
	result.Deltas = livingTopicClaimDeltas(previousPtr, result.Claims, material)
	result.Overview = livingTopicCentralOverview(result.Claims)
	diversityState, platformCount, sourceCount, clusterCount := livingTopicSourceDiversity(detail.Members, result.EvidenceRoles)
	base := domain.LivingTopicSnapshot{
		TopicID: topicID, Status: result.Status, Overview: result.Overview, Claims: result.Claims, Deltas: result.Deltas,
		EvidenceIDs: evidenceIDs, EvidenceRoles: result.EvidenceRoles, InputDigest: digest,
		ContractVersion: livingTopicUnderstandingContractVersion, MaterialChange: material, CoverageState: result.CoverageState,
		EvidenceAsOf:         evidenceAsOf,
		SourceDiversityState: diversityState, SourcePlatformCount: platformCount, IndependentSourceCount: sourceCount, IndependentClusterCount: clusterCount,
	}
	if previousPtr != nil {
		base.PreviousSnapshotID = previousPtr.ID
	}
	model := e.topics.ModelForProfile(settings.ReasoningEvaluationProfile)
	base.Provider, base.Model, base.Effort = e.topics.Name(), model.DisplayModel(), model.DisplayEffort()
	if usage.ProviderModel != "" {
		base.Model = usage.ProviderModel
	}
	if usage.NativeReasoning != "" {
		base.Effort = usage.NativeReasoning
	}
	base.DurationMS, base.Usage = duration.Milliseconds(), usage
	saved, err := e.store.SaveLivingTopicSnapshot(ctx, base)
	if err != nil {
		return nil, "", digest, err
	}
	if material {
		return &saved, "updated", digest, nil
	}
	if previousPtr == nil {
		return &saved, "updated", digest, nil
	}
	return &saved, "refreshed_no_material_change", digest, nil
}

func livingTopicCentralOverview(claims []domain.LivingTopicClaim) string {
	parts := make([]string, 0, 3)
	for _, claim := range claims {
		if claim.Centrality == "central" && claim.Assessment == "supported" && claim.TemporalStatus == "current" {
			parts = append(parts, strings.TrimSpace(claim.Text))
			if len(parts) == 3 {
				break
			}
		}
	}
	if len(parts) == 0 {
		return "The available evidence does not establish a supported latest state. Review the uncertainties and historical context below."
	}
	overview := []rune(strings.Join(parts, " "))
	if len(overview) > 1200 {
		overview = overview[:1200]
	}
	return strings.TrimSpace(string(overview))
}

// Publication time describes the supplied source, not when an event happened
// or proof that no newer information exists. Missing dates remain unknown.
func livingTopicPublishedTime(item domain.MemoryItem) time.Time {
	if item.PublishedAt == nil {
		return time.Time{}
	}
	value, _ := time.Parse(time.RFC3339Nano, *item.PublishedAt)
	return value
}

func sortLivingTopicEvidence(items []domain.MemoryItem) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := livingTopicPublishedTime(items[i]), livingTopicPublishedTime(items[j])
		if !a.Equal(b) {
			return a.After(b)
		}
		return items[i].ID < items[j].ID
	})
}

func annotateLivingTopicClaimTimes(claims []domain.LivingTopicClaim, items []domain.MemoryItem) string {
	dates := make(map[string]time.Time, len(items))
	var newest time.Time
	for _, item := range items {
		date := livingTopicPublishedTime(item)
		dates[item.ID] = date
		if date.After(newest) {
			newest = date
		}
	}
	for i := range claims {
		claims[i].LatestEvidenceAt = ""
		var latest time.Time
		for _, id := range claims[i].EvidenceIDs {
			if dates[id].After(latest) {
				latest = dates[id]
			}
		}
		if !latest.IsZero() {
			claims[i].LatestEvidenceAt = latest.UTC().Format(time.RFC3339Nano)
		}
	}
	priority := func(status string) int {
		switch status {
		case "current":
			return 0
		case "historical":
			return 2
		default:
			return 1
		}
	}
	sort.SliceStable(claims, func(i, j int) bool {
		if a, b := priority(claims[i].TemporalStatus), priority(claims[j].TemporalStatus); a != b {
			return a < b
		}
		a, _ := time.Parse(time.RFC3339Nano, claims[i].LatestEvidenceAt)
		b, _ := time.Parse(time.RFC3339Nano, claims[j].LatestEvidenceAt)
		if !a.Equal(b) {
			return a.After(b)
		}
		terminal := func(status string) bool { return status == "completed" || status == "cancelled" }
		if terminal(claims[i].EventStatus) != terminal(claims[j].EventStatus) {
			return terminal(claims[i].EventStatus)
		}
		return claims[i].Key < claims[j].Key
	})
	if newest.IsZero() {
		return ""
	}
	return newest.UTC().Format(time.RFC3339Nano)
}

func (e *Engine) livingTopicWithRoutingContext(ctx context.Context, topic domain.LivingTopic) (domain.LivingTopic, error) {
	detail, err := e.store.LivingTopicDetail(ctx, topic.ID)
	if err != nil {
		return topic, err
	}
	sortLivingTopicEvidence(detail.Members)
	if len(detail.Members) > 5 {
		detail.Members = detail.Members[:5]
	}
	topic.RoutingContext = detail.Members
	return topic, nil
}

func livingTopicClaimsMateriallyChanged(previous, current []domain.LivingTopicClaim) bool {
	return strings.Join(livingTopicClaimKeys(previous), "\n") != strings.Join(livingTopicClaimKeys(current), "\n")
}

func livingTopicClaimKeys(claims []domain.LivingTopicClaim) []string {
	keys := make([]string, 0, len(claims))
	for _, claim := range claims {
		key := strings.ToLower(strings.TrimSpace(claim.Key))
		keys = append(keys, strings.Join([]string{key, strings.ToLower(strings.TrimSpace(claim.MaterialValue)), claim.Centrality, claim.Assessment, strings.ToLower(strings.TrimSpace(claim.Subtopic)), claim.TemporalStatus, claim.EventStatus}, "|"))
	}
	sort.Strings(keys)
	return keys
}

func livingTopicClaimDeltas(previous *domain.LivingTopicSnapshot, current []domain.LivingTopicClaim, material bool) []domain.LivingTopicDelta {
	if previous == nil || previous.ContractVersion != livingTopicUnderstandingContractVersion || !material {
		return []domain.LivingTopicDelta{}
	}
	prior := map[string]domain.LivingTopicClaim{}
	for _, claim := range previous.Claims {
		prior[strings.ToLower(strings.TrimSpace(claim.Key))] = claim
	}
	deltas := make([]domain.LivingTopicDelta, 0, 8)
	for _, claim := range current {
		key := strings.ToLower(strings.TrimSpace(claim.Key))
		old, exists := prior[key]
		if !exists {
			deltas = append(deltas, domain.LivingTopicDelta{Kind: "new", Text: claim.Text, EvidenceIDs: claim.EvidenceIDs})
		} else if old.Assessment != claim.Assessment || old.Centrality != claim.Centrality || old.Subtopic != claim.Subtopic || old.MaterialValue != claim.MaterialValue || old.TemporalStatus != claim.TemporalStatus || old.EventStatus != claim.EventStatus {
			deltas = append(deltas, domain.LivingTopicDelta{Kind: "updated", Text: claim.Text, EvidenceIDs: claim.EvidenceIDs})
		}
		delete(prior, key)
		if len(deltas) == 8 {
			return deltas
		}
	}
	priorKeys := make([]string, 0, len(prior))
	for key := range prior {
		priorKeys = append(priorKeys, key)
	}
	sort.Strings(priorKeys)
	for _, key := range priorKeys {
		claim := prior[key]
		deltas = append(deltas, domain.LivingTopicDelta{Kind: "removed", Text: claim.Text, EvidenceIDs: claim.EvidenceIDs})
		if len(deltas) == 8 {
			break
		}
	}
	return deltas
}

func reconcileLivingTopicClaimKeys(previous, current []domain.LivingTopicClaim) []domain.LivingTopicClaim {
	priorByKey := make(map[string]domain.LivingTopicClaim, len(previous))
	used := map[string]bool{}
	for _, claim := range previous {
		priorByKey[strings.ToLower(strings.TrimSpace(claim.Key))] = claim
	}
	for index := range current {
		key := strings.ToLower(strings.TrimSpace(current[index].Key))
		if _, exists := priorByKey[key]; exists {
			used[key] = true
			continue
		}
		bestKey, bestScore := "", 0.0
		for priorKey, prior := range priorByKey {
			if used[priorKey] || !strings.EqualFold(strings.TrimSpace(prior.Subtopic), strings.TrimSpace(current[index].Subtopic)) {
				continue
			}
			textScore := tokenJaccard(livingTopicTokens(prior.MaterialValue+" "+prior.Text), livingTopicTokens(current[index].MaterialValue+" "+current[index].Text))
			evidenceScore := stringSetJaccard(prior.EvidenceIDs, current[index].EvidenceIDs)
			score := 0.65*textScore + 0.35*evidenceScore
			if (textScore >= 0.70 || (textScore >= 0.45 && evidenceScore > 0)) && score > bestScore {
				bestKey, bestScore = priorKey, score
			}
		}
		if bestKey != "" {
			current[index].Key = bestKey
			used[bestKey] = true
		}
	}
	return current
}

func stringSetJaccard(left, right []string) float64 {
	a, b := map[string]bool{}, map[string]bool{}
	for _, value := range left {
		a[value] = true
	}
	for _, value := range right {
		b[value] = true
	}
	return tokenJaccard(a, b)
}

func livingTopicSourceDiversity(items []domain.MemoryItem, roles []domain.LivingTopicEvidenceRole) (string, int, int, int) {
	platforms, sources, clusters := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, item := range items {
		platform := strings.ToLower(strings.TrimSpace(string(item.Source)))
		if platform != "" {
			platforms[platform] = true
		}
		author := strings.ToLower(strings.TrimSpace(item.Author))
		if platform != "" || author != "" {
			sources[platform+"|"+author] = true
		}
	}
	for _, role := range roles {
		if cluster := strings.ToLower(strings.TrimSpace(role.SourceCluster)); cluster != "" {
			clusters[cluster] = true
		}
	}
	state := "limited"
	if len(platforms) <= 1 {
		state = "single_platform"
	} else if len(platforms) >= 3 && len(clusters) >= 3 {
		state = "diverse"
	}
	return state, len(platforms), len(sources), len(clusters)
}

func livingTopicInputDigest(topic domain.LivingTopic, items []domain.MemoryItem) (string, []string, error) {
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
	raw, err := json.Marshal(struct {
		ContractVersion  string       `json:"contractVersion"`
		CriteriaRevision int          `json:"criteriaRevision"`
		Name             string       `json:"name"`
		Description      string       `json:"description"`
		Aliases          []string     `json:"aliases"`
		IncludeCriteria  string       `json:"includeCriteria"`
		ExcludeCriteria  string       `json:"excludeCriteria"`
		Evidence         []digestItem `json:"evidence"`
	}{ContractVersion: livingTopicUnderstandingContractVersion, CriteriaRevision: topic.CriteriaRevision, Name: topic.Name, Description: topic.Description, Aliases: topic.Aliases, IncludeCriteria: topic.IncludeCriteria, ExcludeCriteria: topic.ExcludeCriteria, Evidence: values})
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)
	return strings.ToLower(hex.EncodeToString(sum[:])), ids, nil
}

func (e *Engine) ResumeLivingTopicUnderstanding(ctx context.Context) error {
	if err := e.store.ResetRunningLivingTopicUnderstanding(ctx); err != nil {
		return err
	}
	e.launchLivingTopicUnderstanding("", "")
	return nil
}

func (e *Engine) launchLivingTopicUnderstanding(topicID, trigger string) {
	if topicID != "" {
		if _, err := e.store.QueueLivingTopicUnderstanding(context.Background(), topicID, trigger); err != nil {
			e.logger.Printf("Living Topics understanding queue degraded safely for topic %s: %v", topicID, err)
			return
		}
	}
	const key = "living-topics-understanding"
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
				if pending, err := e.store.HasPendingLivingTopicUnderstanding(context.Background()); err == nil && pending {
					e.launchLivingTopicUnderstanding("", "")
				}
			}
		}()
		for {
			job, err := e.store.ClaimLivingTopicUnderstanding(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.logger.Printf("Living Topics understanding claim degraded safely: %v", err)
				}
				return
			}
			if job == nil {
				return
			}
			snapshot, outcome, digest, refreshErr := e.evaluateLivingTopicUnderstanding(ctx, job.TopicID, job.Trigger)
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			snapshotID := ""
			if snapshot != nil {
				snapshotID = snapshot.ID
			}
			if finishErr := e.store.FinishLivingTopicUnderstanding(context.Background(), *job, outcome, digest, snapshotID, refreshErr); finishErr != nil {
				e.logger.Printf("Living Topics understanding job %s could not persist: %v", job.ID, finishErr)
			}
			if refreshErr != nil {
				e.logger.Printf("Living Topics understanding for topic %s degraded safely: %v", job.TopicID, refreshErr)
			}
		}
	}()
}

func (e *Engine) ResumeLivingTopicRouting(ctx context.Context) error {
	if err := e.store.ResetRunningLivingTopicRouting(ctx); err != nil {
		return err
	}
	e.launchLivingTopicRouting("")
	return nil
}

func (e *Engine) ResumeLivingTopicActivation(ctx context.Context) error {
	if err := e.store.ResetRunningLivingTopicActivations(ctx); err != nil {
		return err
	}
	e.launchLivingTopicActivation("", "")
	return nil
}

func (e *Engine) launchLivingTopicActivation(topicID, trigger string) {
	if topicID != "" {
		if _, err := e.store.QueueLivingTopicActivation(context.Background(), topicID, trigger); err != nil {
			e.logger.Printf("Living Topics activation queue degraded safely for topic %s: %v", topicID, err)
			return
		}
	}
	const key = "living-topics-activation"
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
				if pending, err := e.store.HasPendingLivingTopicActivation(context.Background()); err == nil && pending {
					e.launchLivingTopicActivation("", "")
				}
			}
		}()
		for {
			job, err := e.store.ClaimLivingTopicActivation(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					e.logger.Printf("Living Topics activation claim degraded safely: %v", err)
				}
				return
			}
			if job == nil {
				return
			}
			result, activationErr := e.activateLivingTopic(ctx, *job)
			if errors.Is(ctx.Err(), context.Canceled) {
				return
			}
			if finishErr := e.store.FinishLivingTopicActivation(context.Background(), *job, result, activationErr); finishErr != nil {
				e.logger.Printf("Living Topics activation job %s could not persist: %v", job.ID, finishErr)
			}
			if activationErr != nil {
				e.logger.Printf("Living Topics activation for topic %s degraded safely: %v", job.TopicID, activationErr)
			}
		}
	}()
}

func (e *Engine) activateLivingTopic(ctx context.Context, job domain.LivingTopicActivationJob) (map[string]int, error) {
	result := map[string]int{"scanned": 0, "shortlisted": 0, "suggested": 0}
	topic, err := e.store.LivingTopic(ctx, job.TopicID)
	if err != nil {
		return result, err
	}
	if topic.CriteriaRevision != job.CriteriaRevision {
		result["stale"] = 1
		return result, nil
	}
	topic, err = e.livingTopicWithRoutingContext(ctx, topic)
	if err != nil {
		return result, err
	}
	items, err := e.store.LivingTopicActivationItems(ctx, topic.ID, store.LivingTopicActivationScanLimit)
	if err != nil {
		return result, err
	}
	result["scanned"] = len(items)
	storedExamples, err := e.store.LivingTopicFeedbackExamples(ctx, 3)
	if err != nil {
		return result, err
	}
	examples := make([]domain.LivingTopicRoutingExample, 0, len(storedExamples))
	for _, value := range storedExamples {
		examples = append(examples, domain.LivingTopicRoutingExample{TopicID: value.TopicID, Verdict: value.Verdict, Item: value.Item})
	}
	type scoredItem struct {
		item   domain.MemoryItem
		score  float64
		reason string
	}
	shortlist := make([]scoredItem, 0)
	for _, item := range items {
		timeline := livingTopicTimelineFromMemory(item)
		score, reason := deterministicLivingTopicScore(timeline, topic, examples)
		if score > 0 {
			shortlist = append(shortlist, scoredItem{item: item, score: score, reason: reason})
		}
	}
	sort.SliceStable(shortlist, func(i, j int) bool { return shortlist[i].score > shortlist[j].score })
	if len(shortlist) > store.LivingTopicActivationShortlist {
		shortlist = shortlist[:store.LivingTopicActivationShortlist]
	}
	result["shortlisted"] = len(shortlist)
	for _, candidate := range shortlist {
		decision := domain.LivingTopicRoutingDecision{TopicID: topic.ID, Match: candidate.score >= 0.70, Confidence: candidate.score, Mode: "deterministic", Reason: candidate.reason}
		if !decision.Match {
			if router, ok := e.topics.(livingtopics.Router); ok {
				settings, settingsErr := e.store.GetSettings(ctx)
				if settingsErr != nil {
					return result, settingsErr
				}
				values, usage, duration, routeErr := router.RouteWithProfile(ctx, livingTopicTimelineFromMemory(candidate.item), []domain.LivingTopic{topic}, examples, settings.ReasoningEvaluationProfile)
				e.recordLivingTopicModelInvocation("living_topic_routing", job.ID+":"+candidate.item.ID, settings.ReasoningEvaluationProfile, usage, duration, routeErr)
				if routeErr != nil {
					e.logger.Printf("Living Topics activation semantic routing degraded to local score for topic %s item %s: %v", topic.ID, candidate.item.ID, routeErr)
				} else if len(values) == 1 {
					decision = values[0]
				}
			}
		}
		if err := e.store.SaveLivingTopicCandidateDecision(ctx, topic, candidate.item, decision); err != nil {
			return result, err
		}
		if decision.Match && decision.Confidence >= 0.65 {
			result["suggested"]++
		}
	}
	return result, nil
}

func livingTopicTimelineFromMemory(item domain.MemoryItem) domain.TimelineItem {
	return domain.TimelineItem{
		Source: item.Source, EvidenceKey: item.CanonicalEvidenceKey,
		Item:       domain.ReasonedItem{WhatChanged: item.Title, WhyItMatters: item.Summary, Source: item.Source, SourceURL: item.CanonicalPermalink, EvidenceKey: item.CanonicalEvidenceKey, Author: item.Author, PublishedAt: item.PublishedAt},
		Assessment: domain.CandidateAssessment{EvidenceKey: item.CanonicalEvidenceKey, TopicTags: item.Tags, TopicFacets: item.Facets},
	}
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
		dirtyTopics := map[string]bool{}
		defer func() {
			for topicID := range dirtyTopics {
				if _, err := e.store.QueueLivingTopicUnderstanding(context.Background(), topicID, "routed_evidence_batch"); err != nil {
					e.logger.Printf("Living Topics understanding queue degraded safely for routed topic %s: %v", topicID, err)
				}
			}
			e.mu.Lock()
			delete(e.active, key)
			shuttingDown := e.shuttingDown
			e.mu.Unlock()
			if !shuttingDown {
				if len(dirtyTopics) > 0 {
					e.launchLivingTopicUnderstanding("", "")
				}
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
			decisions, changedTopics, routeErr := e.routeLivingTopicItem(ctx, job.TimelineID)
			for _, topicID := range changedTopics {
				dirtyTopics[topicID] = true
			}
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

func (e *Engine) routeLivingTopicItem(ctx context.Context, timelineID string) ([]domain.LivingTopicRoutingDecision, []string, error) {
	item, err := e.store.LivingTopicRoutingItem(ctx, timelineID)
	if err != nil {
		return nil, nil, err
	}
	topics, err := e.store.ListLivingTopics(ctx)
	if err != nil || len(topics) == 0 {
		return nil, nil, err
	}
	storedExamples, err := e.store.LivingTopicFeedbackExamples(ctx, 3)
	if err != nil {
		return nil, nil, err
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
			topic, err = e.livingTopicWithRoutingContext(ctx, topic)
			if err != nil {
				return nil, nil, err
			}
			remaining = append(remaining, topic)
		}
	}
	if router, ok := e.topics.(livingtopics.Router); ok && len(remaining) > 0 {
		settings, settingsErr := e.store.GetSettings(ctx)
		if settingsErr != nil {
			return nil, nil, settingsErr
		}
		llm, usage, duration, routeErr := router.RouteWithProfile(ctx, item, remaining, examples, settings.ReasoningEvaluationProfile)
		e.recordLivingTopicModelInvocation("living_topic_routing", timelineID, settings.ReasoningEvaluationProfile, usage, duration, routeErr)
		if routeErr != nil {
			e.logger.Printf("Living Topics semantic routing degraded to deterministic for Timeline %s: %v", timelineID, routeErr)
		} else {
			decisions = append(decisions, llm...)
		}
	}
	changedTopics := make([]string, 0)
	for _, decision := range decisions {
		if !decision.Match || decision.Confidence < 0.65 {
			continue
		}
		added, addErr := e.store.AddAutomaticLivingTopicMember(ctx, decision.TopicID, item, decision)
		if addErr != nil && !errors.Is(addErr, store.ErrLivingTopicMemberMax) {
			return decisions, changedTopics, addErr
		}
		if added {
			changedTopics = append(changedTopics, decision.TopicID)
		}
	}
	return decisions, changedTopics, nil
}

func (e *Engine) recordLivingTopicModelInvocation(category, ownerID, profile string, usage domain.ModelUsage, duration time.Duration, invocationErr error) {
	if e.topics == nil {
		return
	}
	model := e.topics.ModelForProfile(profile)
	status := "completed"
	if invocationErr != nil {
		status = "failed"
	}
	if err := e.store.RecordLivingTopicModelInvocation(context.Background(), category, ownerID, status, e.topics.Name(), model.DisplayModel(), model.DisplayEffort(), duration, usage); err != nil {
		e.logger.Printf("Living Topics model usage receipt degraded safely for %s %s: %v", category, ownerID, err)
	}
}

func deterministicLivingTopicScore(item domain.TimelineItem, topic domain.LivingTopic, examples []domain.LivingTopicRoutingExample) (float64, string) {
	postTokens := livingTopicTokens(strings.Join([]string{item.Item.WhatChanged, item.Item.WhyItMatters, strings.Join(item.Assessment.TopicTags, " "), strings.Join(item.Assessment.TopicFacets, " ")}, " "))
	criteriaTokens := livingTopicTokens(strings.Join([]string{topic.Name, strings.Join(topic.Aliases, " "), topic.Description, topic.IncludeCriteria}, " "))
	excludeTokens := livingTopicTokens(topic.ExcludeCriteria)
	base := tokenCoverage(postTokens, criteriaTokens)
	excluded := tokenCoverage(postTokens, excludeTokens)
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
	score := base + 0.25*positive - 0.40*negative - 0.75*excluded
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return score, fmt.Sprintf("Criteria token coverage %.2f; exclusion coverage %.2f; positive similarity %.2f; negative similarity %.2f", base, excluded, positive, negative)
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
