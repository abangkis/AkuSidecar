package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/contentcontext"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	contentContextDefaultLimit = domain.ContentContextDefaultLimit
	contentContextMinLimit     = domain.ContentContextMinLimit
	contentContextMaxLimit     = domain.ContentContextMaxLimit
)

var ErrContentContextNotEligible = errors.New("Timeline item is not a final visible item")

var ErrContentContextFeedbackNotCurrent = errors.New("only the current content context feedback decision can be cleared")

var contentContextEngine = contentcontext.NewEngine()

// ContentContext reads one final Timeline item and searches only the local
// Personal Memory FTS index. It intentionally has no write path, provider
// invocation, Bridge call, or Timeline/feedback side effect.
func (s *Store) ContentContext(ctx context.Context, timelineID string, limit int) (domain.ContentContextResult, error) {
	if limit == 0 {
		limit = contentContextDefaultLimit
	}
	if limit < contentContextMinLimit || limit > contentContextMaxLimit {
		return domain.ContentContextResult{}, fmt.Errorf("content context limit must be between %d and %d", contentContextMinLimit, contentContextMaxLimit)
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("begin content context read snapshot: %w", err)
	}
	defer tx.Rollback()

	timeline, err := timelineItemForRetentionTx(ctx, tx, strings.TrimSpace(timelineID), false)
	if err != nil {
		if errors.Is(err, ErrTimelineMemoryNotEligible) {
			return domain.ContentContextResult{}, ErrContentContextNotEligible
		}
		return domain.ContentContextResult{}, err
	}
	var visible int
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM timeline_items t
			JOIN sessions s ON s.id=t.session_id
			LEFT JOIN auto_update_batches b ON b.session_id=s.id
			WHERE t.id=?
			  AND s.status IN ('completed','partial')
			  AND s.completed_at IS NOT NULL
			  AND (b.state IS NULL OR b.state='visible')
		)`, strings.TrimSpace(timelineID)).Scan(&visible); err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("check content context Timeline visibility: %w", err)
	}
	if visible == 0 {
		return domain.ContentContextResult{}, ErrContentContextNotEligible
	}

	query := contentContextEngine.Extract(timeline)
	if err := tx.Commit(); err != nil {
		return domain.ContentContextResult{}, fmt.Errorf("commit content context read snapshot: %w", err)
	}
	if len(query.Terms) == 0 {
		return domain.ContentContextResult{Matches: []domain.ContentContextMatch{}, TopicInsights: []domain.ContentContextTopicInsight{}}, nil
	}

	items, err := s.searchMemoryContextCandidates(ctx, query.Terms, contentContextEngine.CandidatePool)
	if err != nil {
		return domain.ContentContextResult{}, err
	}
	candidates := make([]contentcontext.Candidate, 0, len(items))
	for _, candidate := range items {
		if sameTimelineMemoryIdentity(timeline, candidate.Item) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	feedback, err := s.latestContentContextFeedbackStates(ctx, contentContextKey(timeline), candidates)
	if err != nil {
		return domain.ContentContextResult{}, err
	}
	for index := range candidates {
		if state, ok := feedback[candidates[index].Item.ID]; ok {
			candidates[index].Feedback = state.Verdict
			candidates[index].FeedbackID = state.ID
		}
	}
	topicInsights, err := s.livingTopicContentContextInsights(ctx, query, limit)
	if err != nil {
		return domain.ContentContextResult{}, err
	}
	return domain.ContentContextResult{Matches: contentContextEngine.Match(query, candidates, limit), TopicInsights: topicInsights}, nil
}

func (s *Store) livingTopicContentContextInsights(ctx context.Context, query contentcontext.Query, limit int) ([]domain.ContentContextTopicInsight, error) {
	topics, err := s.ListLivingTopics(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]domain.ContentContextTopicInsight)
	candidates := make([]contentcontext.Candidate, 0, len(topics))
	for _, topic := range topics {
		snapshot := topic.LatestSnapshot
		if snapshot == nil || !snapshot.IsCurrent || snapshot.ActiveEvidenceCount < 1 {
			continue
		}
		if !contentcontext.TopicIdentityMatches(query, topic.Name, topic.Aliases) {
			continue
		}
		claims := make([]domain.LivingTopicClaim, 0, 3)
		claimText := make([]string, 0, 3)
		for _, claim := range snapshot.Claims {
			if claim.Assessment != "supported" {
				continue
			}
			claims = append(claims, claim)
			claimText = append(claimText, claim.Text)
			if len(claims) == 3 {
				break
			}
		}
		if len(claims) == 0 {
			continue
		}
		byID[topic.ID] = domain.ContentContextTopicInsight{
			TopicID: topic.ID, TopicName: topic.Name, Overview: snapshot.Overview, Claims: claims,
			SnapshotVersion: snapshot.Version, UpdatedAt: snapshot.CreatedAt,
			EvidenceCount: snapshot.ActiveEvidenceCount,
		}
		candidates = append(candidates, contentcontext.Candidate{Item: domain.MemoryItem{
			ID: topic.ID, Title: topic.Name, Summary: strings.Join(append([]string{snapshot.Overview, topic.Description}, claimText...), " "),
			Tags: topic.Aliases, Facets: []string{topic.Name}, LifecycleState: domain.MemoryStateActive, UpdatedAt: snapshot.CreatedAt,
		}})
	}
	if len(candidates) == 0 {
		return []domain.ContentContextTopicInsight{}, nil
	}
	topicLimit := limit
	if topicLimit > 2 {
		topicLimit = 2
	}
	matches := contentContextEngine.Match(query, candidates, topicLimit)
	result := make([]domain.ContentContextTopicInsight, 0, len(matches))
	for _, match := range matches {
		insight := byID[match.Item.ID]
		insight.MatchReason = match.MatchReason
		result = append(result, insight)
	}
	return result, nil
}

// AddContentContextFeedback records an explicit relationship decision only for
// a match the current engine can reproduce. The server derives rank, reason,
// context identity, and engine version; clients cannot author learning data.
func (s *Store) AddContentContextFeedback(ctx context.Context, timelineID string, input domain.ContentContextFeedbackInput) (domain.ContentContextFeedbackEvent, error) {
	if err := input.Validate(); err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	timelineID = strings.TrimSpace(timelineID)
	input.MemoryItemID = strings.TrimSpace(input.MemoryItemID)
	result, err := s.ContentContext(ctx, timelineID, contentContextMaxLimit)
	if err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	resultRank := 0
	matchReason := ""
	for index, match := range result.Matches {
		if match.Item.ID == input.MemoryItemID {
			resultRank = index + 1
			matchReason = match.MatchReason
			break
		}
	}
	if resultRank == 0 {
		return domain.ContentContextFeedbackEvent{}, errors.New("feedback requires a currently surfaced related context match")
	}
	timeline, err := s.TimelineItem(ctx, timelineID)
	if err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	value := domain.ContentContextFeedbackEvent{
		ID:            domain.NewID("content_context_feedback"),
		TimelineID:    timeline.ID,
		ContextKey:    contentContextKey(timeline),
		MemoryItemID:  input.MemoryItemID,
		Verdict:       input.Verdict,
		EngineVersion: domain.ContentContextEngineVersion,
		ResultRank:    resultRank,
		MatchReason:   matchReason,
		CreatedAt:     domain.Now(),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	defer tx.Rollback()
	var latest domain.ContentContextFeedbackEvent
	err = tx.QueryRowContext(ctx, `
		SELECT id,timeline_id,context_key,memory_item_id,verdict,engine_version,result_rank,match_reason,COALESCE(supersedes_id,''),created_at
		FROM content_context_feedback_events
		WHERE context_key=? AND memory_item_id=?
		ORDER BY rowid DESC LIMIT 1`, value.ContextKey, value.MemoryItemID).
		Scan(&latest.ID, &latest.TimelineID, &latest.ContextKey, &latest.MemoryItemID, &latest.Verdict,
			&latest.EngineVersion, &latest.ResultRank, &latest.MatchReason, &latest.SupersedesID, &latest.CreatedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.ContentContextFeedbackEvent{}, err
	}
	if err == nil && latest.Verdict == value.Verdict {
		if err := tx.Commit(); err != nil {
			return domain.ContentContextFeedbackEvent{}, err
		}
		return latest, nil
	}
	if err == nil {
		value.SupersedesID = latest.ID
	}
	var supersedes any
	if value.SupersedesID != "" {
		supersedes = value.SupersedesID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO content_context_feedback_events(
		  id,timeline_id,context_key,memory_item_id,verdict,engine_version,result_rank,match_reason,supersedes_id,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TimelineID, value.ContextKey, value.MemoryItemID,
		value.Verdict, value.EngineVersion, value.ResultRank, value.MatchReason, supersedes, value.CreatedAt); err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	return value, nil
}

func (s *Store) UndoContentContextFeedback(ctx context.Context, id string) (domain.ContentContextFeedbackEvent, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	defer tx.Rollback()
	var prior domain.ContentContextFeedbackEvent
	err = tx.QueryRowContext(ctx, `
		SELECT id,timeline_id,context_key,memory_item_id,verdict,engine_version,result_rank,match_reason,COALESCE(supersedes_id,''),created_at
		FROM content_context_feedback_events WHERE id=? AND verdict<>'clear'`, strings.TrimSpace(id)).
		Scan(&prior.ID, &prior.TimelineID, &prior.ContextKey, &prior.MemoryItemID, &prior.Verdict,
			&prior.EngineVersion, &prior.ResultRank, &prior.MatchReason, &prior.SupersedesID, &prior.CreatedAt)
	if err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	var latestID string
	if err := tx.QueryRowContext(ctx, `
		SELECT id FROM content_context_feedback_events
		WHERE context_key=? AND memory_item_id=?
		ORDER BY rowid DESC LIMIT 1`, prior.ContextKey, prior.MemoryItemID).Scan(&latestID); err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	if latestID != prior.ID {
		return domain.ContentContextFeedbackEvent{}, ErrContentContextFeedbackNotCurrent
	}
	value := prior
	value.ID = domain.NewID("content_context_feedback")
	value.Verdict = domain.ContentContextFeedbackClear
	value.SupersedesID = prior.ID
	value.CreatedAt = domain.Now()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO content_context_feedback_events(
		  id,timeline_id,context_key,memory_item_id,verdict,engine_version,result_rank,match_reason,supersedes_id,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?)`, value.ID, value.TimelineID, value.ContextKey, value.MemoryItemID,
		value.Verdict, value.EngineVersion, value.ResultRank, value.MatchReason, value.SupersedesID, value.CreatedAt); err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ContentContextFeedbackEvent{}, err
	}
	return value, nil
}

func (s *Store) latestContentContextFeedbackStates(ctx context.Context, contextKey string, candidates []contentcontext.Candidate) (map[string]domain.ContentContextFeedbackState, error) {
	result := make(map[string]domain.ContentContextFeedbackState)
	if contextKey == "" || len(candidates) == 0 {
		return result, nil
	}
	placeholders := make([]string, 0, len(candidates))
	args := make([]any, 0, len(candidates)+1)
	args = append(args, contextKey)
	for _, candidate := range candidates {
		placeholders = append(placeholders, "?")
		args = append(args, candidate.Item.ID)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT event.memory_item_id,event.id,event.verdict
		FROM content_context_feedback_events event
		JOIN (
		  SELECT memory_item_id,MAX(rowid) AS latest_rowid
		  FROM content_context_feedback_events
		  WHERE context_key=? AND memory_item_id IN (`+strings.Join(placeholders, ",")+`)
		  GROUP BY memory_item_id
		) latest ON latest.latest_rowid=event.rowid`, args...)
	if err != nil {
		return nil, fmt.Errorf("read content context feedback state: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var memoryID string
		var state domain.ContentContextFeedbackState
		if err := rows.Scan(&memoryID, &state.ID, &state.Verdict); err != nil {
			return nil, err
		}
		if state.Verdict.ValidDecision() {
			result[memoryID] = state
		}
	}
	return result, rows.Err()
}

func contentContextKey(timeline domain.TimelineItem) string {
	source := strings.TrimSpace(string(timeline.Source))
	evidenceKey := strings.TrimSpace(timeline.EvidenceKey)
	if evidenceKey == "" {
		evidenceKey = strings.TrimSpace(timeline.ID)
	}
	return source + "|" + evidenceKey
}

// searchMemoryContextCandidates uses the existing FTS5 index with a bounded
// OR query. It intentionally overfetches so the relevance engine can reject
// weak generic-token matches without filling the public result quota from the
// first BM25 rows.
func (s *Store) searchMemoryContextCandidates(ctx context.Context, terms []string, limit int) ([]contentcontext.Candidate, error) {
	quoted := make([]string, 0, len(terms))
	for _, term := range terms {
		normalized, err := memorySearchTerms(term)
		if err != nil || len(normalized) != 1 {
			continue
		}
		quoted = append(quoted, normalized[0])
	}
	if len(quoted) == 0 {
		return []contentcontext.Candidate{}, nil
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin content context FTS snapshot: %w", err)
	}
	defer tx.Rollback()
	rank := libraryBM25Expression()
	rows, err := tx.QueryContext(ctx, `
		SELECT mi.id,`+rank+` AS score
		FROM memory_search_fts
		JOIN memory_items mi ON mi.id=memory_search_fts.memory_item_id
		WHERE mi.lifecycle_state='active' AND memory_search_fts MATCH ?
		ORDER BY `+rank+` ASC,mi.updated_at DESC,mi.id DESC
		LIMIT ?`, strings.Join(quoted, " OR "), limit)
	if err != nil {
		return nil, fmt.Errorf("search local content context: %w", err)
	}
	defer rows.Close()
	items := make([]contentcontext.Candidate, 0, limit)
	for rows.Next() {
		var id string
		var bm25 float64
		if err := rows.Scan(&id, &bm25); err != nil {
			return nil, err
		}
		item, err := memoryItemByQueryer(ctx, tx, id)
		if err != nil {
			return nil, err
		}
		if item.LifecycleState == domain.MemoryStateActive {
			items = append(items, contentcontext.Candidate{Item: item, BM25: bm25})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit content context FTS snapshot: %w", err)
	}
	return items, nil
}

func sameTimelineMemoryIdentity(timeline domain.TimelineItem, memory domain.MemoryItem) bool {
	if timeline.Source != memory.Source {
		return false
	}
	if evidenceKey := strings.TrimSpace(timeline.EvidenceKey); evidenceKey != "" && evidenceKey == strings.TrimSpace(memory.CanonicalEvidenceKey) {
		return true
	}
	if sourceURL := strings.TrimSpace(timeline.Item.SourceURL); sourceURL != "" && sourceURL == strings.TrimSpace(memory.CanonicalPermalink) {
		return true
	}
	if timeline.Evidence == nil {
		return false
	}
	if permalink := strings.TrimSpace(timeline.Evidence.Permalink); permalink != "" && permalink == strings.TrimSpace(memory.CanonicalPermalink) {
		return true
	}
	if platformID := strings.TrimSpace(timeline.Evidence.PlatformID); platformID != "" && platformID == strings.TrimSpace(memory.CanonicalPlatformID) {
		return true
	}
	return false
}
