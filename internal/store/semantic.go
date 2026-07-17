package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) SemanticCandidates(ctx context.Context, sessionID string) ([]domain.SemanticCandidate, error) {
	items, err := s.ListSessionItems(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.SemanticCandidate, 0, len(items))
	for index, item := range items {
		text := item.Item.WhatChanged
		if item.Evidence != nil && strings.TrimSpace(item.Evidence.Text) != "" {
			text = item.Evidence.Text
		}
		result = append(result, domain.SemanticCandidate{
			Alias:       fmt.Sprintf("candidate_%03d", index+1),
			TimelineID:  item.ID,
			SessionID:   item.SessionID,
			RunID:       item.RunID,
			EvidenceKey: item.EvidenceKey,
			Source:      item.Source,
			Author:      item.Item.Author,
			PublishedAt: item.Item.PublishedAt,
			Text:        text,
			WhatChanged: item.Item.WhatChanged,
			EventKey:    item.Item.EventKey,
			TopicTags:   append([]string(nil), item.Assessment.TopicTags...),
			Item:        item.Item,
		})
	}
	return result, nil
}

func (s *Store) ListSemanticEvents(ctx context.Context, cutoff string, limit int) ([]domain.SemanticEvent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.id,e.canonical_claim,e.actor,e.action,e.object,e.event_kind,e.event_start,e.event_end,e.aliases_json,
		       (SELECT COUNT(*) FROM semantic_event_reports r WHERE r.event_id=e.id),e.first_seen_at,e.last_seen_at
		FROM semantic_events e WHERE e.last_seen_at>=? ORDER BY e.last_seen_at DESC LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.SemanticEvent
	for rows.Next() {
		var value domain.SemanticEvent
		var start, end sql.NullString
		var aliases string
		if err := rows.Scan(&value.ID, &value.CanonicalClaim, &value.Actor, &value.Action, &value.Object, &value.EventKind, &start, &end, &aliases, &value.ReportCount, &value.FirstSeenAt, &value.LastSeenAt); err != nil {
			return nil, err
		}
		if start.Valid {
			value.EventStart = &start.String
		}
		if end.Valid {
			value.EventEnd = &end.String
		}
		decodeJSON(aliases, &value.Aliases)
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) SemanticConstraints(ctx context.Context, evidenceKeys []string) (map[string]map[string]string, error) {
	result := map[string]map[string]string{}
	if len(evidenceKeys) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(evidenceKeys))
	args := make([]any, len(evidenceKeys))
	for index, value := range evidenceKeys {
		placeholders[index] = "?"
		args[index] = value
	}
	rows, err := s.db.QueryContext(ctx, `SELECT evidence_key,event_id,kind FROM semantic_event_constraints WHERE evidence_key IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var evidence, eventID, kind string
		if err := rows.Scan(&evidence, &eventID, &kind); err != nil {
			return nil, err
		}
		if result[evidence] == nil {
			result[evidence] = map[string]string{}
		}
		result[evidence][eventID] = kind
	}
	return result, rows.Err()
}

func (s *Store) SaveSemanticReports(ctx context.Context, reports []domain.ResolvedSemanticReport) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, report := range reports {
		event := report.Event
		aliases, _ := json.Marshal(event.Aliases)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO semantic_events(id,canonical_claim,actor,action,object,event_kind,event_start,event_end,aliases_json,first_seen_at,last_seen_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(id) DO UPDATE SET last_seen_at=excluded.last_seen_at`,
			event.ID, strings.TrimSpace(event.CanonicalClaim), strings.TrimSpace(event.Actor), strings.TrimSpace(event.Action), strings.TrimSpace(event.Object), defaultString(event.EventKind, "other"), event.EventStart, event.EventEnd, string(aliases), event.FirstSeenAt, event.LastSeenAt); err != nil {
			return err
		}
		corrected := 0
		if report.Corrected {
			corrected = 1
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO semantic_event_reports(id,event_id,timeline_id,session_id,run_id,evidence_key,source,relation,confidence,reason,corrected,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(timeline_id) DO UPDATE SET event_id=excluded.event_id,relation=excluded.relation,confidence=excluded.confidence,reason=excluded.reason,corrected=excluded.corrected`,
			domain.NewID("event_report"), event.ID, report.Candidate.TimelineID, report.Candidate.SessionID, report.Candidate.RunID, report.Candidate.EvidenceKey, report.Candidate.Source, report.Relation, report.Confidence, report.Reason, corrected, domain.Now()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.cleanupOrphanSemanticEvents(ctx)
}

func (s *Store) SaveEventResolutionSummary(ctx context.Context, value domain.EventResolutionSummary) error {
	var failure any
	if value.Error != nil {
		raw, _ := json.Marshal(value.Error)
		failure = string(raw)
	}
	triggerTokens, _ := json.Marshal(value.TriggerTokens)
	resolverInvoked := 0
	if value.ResolverInvoked {
		resolverInvoked = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_resolution_invocations(session_id,status,provider,model,effort,candidate_count,shortlist_count,unique_items,duplicate_reports,duration_ms,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,error_json,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(session_id) DO UPDATE SET status=excluded.status,provider=excluded.provider,model=excluded.model,effort=excluded.effort,candidate_count=excluded.candidate_count,shortlist_count=excluded.shortlist_count,unique_items=excluded.unique_items,duplicate_reports=excluded.duplicate_reports,duration_ms=excluded.duration_ms,input_tokens=excluded.input_tokens,cached_input_tokens=excluded.cached_input_tokens,output_tokens=excluded.output_tokens,reasoning_output_tokens=excluded.reasoning_output_tokens,error_json=excluded.error_json,created_at=excluded.created_at`,
		value.SessionID, value.Status, value.Provider, value.Model, value.Effort, value.CandidateCount, value.ShortlistCount, value.UniqueItems, value.DuplicateReports, value.DurationMS, value.Usage.Input, value.Usage.CachedInput, value.Usage.Output, value.Usage.ReasoningOutput, failure, value.CreatedAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO event_resolution_diagnostics(session_id,historical_event_count,resolver_invoked,trigger_reason,strongest_overlap,trigger_tokens_json)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(session_id) DO UPDATE SET historical_event_count=excluded.historical_event_count,resolver_invoked=excluded.resolver_invoked,trigger_reason=excluded.trigger_reason,strongest_overlap=excluded.strongest_overlap,trigger_tokens_json=excluded.trigger_tokens_json`,
		value.SessionID, value.HistoricalEventCount, resolverInvoked, value.TriggerReason, value.StrongestOverlap, string(triggerTokens)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) EventResolutionSummary(ctx context.Context, sessionID string) (*domain.EventResolutionSummary, error) {
	var value domain.EventResolutionSummary
	var failure sql.NullString
	var historicalEventCount, resolverInvoked, strongestOverlap sql.NullInt64
	var triggerReason, triggerTokens sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT i.session_id,i.status,i.provider,i.model,i.effort,i.candidate_count,i.shortlist_count,i.unique_items,i.duplicate_reports,i.duration_ms,i.input_tokens,i.cached_input_tokens,i.output_tokens,i.reasoning_output_tokens,i.error_json,i.created_at,
		       d.historical_event_count,d.resolver_invoked,d.trigger_reason,d.strongest_overlap,d.trigger_tokens_json
		FROM event_resolution_invocations i LEFT JOIN event_resolution_diagnostics d ON d.session_id=i.session_id WHERE i.session_id=?`, sessionID).
		Scan(&value.SessionID, &value.Status, &value.Provider, &value.Model, &value.Effort, &value.CandidateCount, &value.ShortlistCount, &value.UniqueItems, &value.DuplicateReports, &value.DurationMS, &value.Usage.Input, &value.Usage.CachedInput, &value.Usage.Output, &value.Usage.ReasoningOutput, &failure, &value.CreatedAt, &historicalEventCount, &resolverInvoked, &triggerReason, &strongestOverlap, &triggerTokens)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if failure.Valid {
		var decoded domain.Failure
		decodeJSON(failure.String, &decoded)
		value.Error = &decoded
	}
	if historicalEventCount.Valid {
		value.HistoricalEventCount = int(historicalEventCount.Int64)
	}
	value.ResolverInvoked = resolverInvoked.Valid && resolverInvoked.Int64 == 1
	if triggerReason.Valid {
		value.TriggerReason = triggerReason.String
	}
	if strongestOverlap.Valid {
		value.StrongestOverlap = int(strongestOverlap.Int64)
	}
	if triggerTokens.Valid {
		decodeJSON(triggerTokens.String, &value.TriggerTokens)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(CASE WHEN c.action='not_same_event' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN c.action='same_event' THEN 1 ELSE 0 END),0)
		FROM semantic_event_corrections c
		JOIN timeline_items t ON t.id=c.timeline_id
		WHERE t.session_id=? AND c.undone_at IS NULL`, sessionID).
		Scan(&value.UserSplitCorrections, &value.UserMergeCorrections); err != nil {
		return nil, err
	}
	return &value, nil
}

func (s *Store) attachSemanticEvents(ctx context.Context, items []domain.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	byID := make(map[string]*domain.TimelineItem, len(items))
	placeholders := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	for index := range items {
		byID[items[index].ID] = &items[index]
		placeholders = append(placeholders, "?")
		args = append(args, items[index].ID)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.timeline_id,r.event_id,e.canonical_claim,r.relation,r.confidence,r.reason,r.corrected,
		       (SELECT COUNT(*) FROM semantic_event_reports grouped WHERE grouped.event_id=r.event_id),
		       (SELECT c.id FROM semantic_event_corrections c WHERE c.report_id=r.id AND c.undone_at IS NULL ORDER BY c.created_at DESC LIMIT 1)
		FROM semantic_event_reports r JOIN semantic_events e ON e.id=r.event_id
		WHERE r.timeline_id IN (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var timelineID string
		var value domain.TimelineSemanticEvent
		var corrected int
		var correctionID sql.NullString
		if err := rows.Scan(&timelineID, &value.EventID, &value.CanonicalClaim, &value.Relation, &value.Confidence, &value.Reason, &corrected, &value.ReportCount, &correctionID); err != nil {
			return err
		}
		value.Corrected = corrected == 1
		if correctionID.Valid {
			value.CorrectionID = correctionID.String
		}
		if item := byID[timelineID]; item != nil {
			item.SemanticEvent = &value
		}
	}
	return rows.Err()
}

func (s *Store) SuggestSemanticEvents(ctx context.Context, timelineID string, limit int) ([]domain.EventSuggestion, error) {
	items, err := s.listItems(ctx, `WHERE id=?`, timelineID)
	if err != nil || len(items) != 1 {
		return nil, err
	}
	item := items[0]
	text := strings.Join(append([]string{item.Item.WhatChanged, item.Item.Author, item.Item.EventKey}, item.Assessment.TopicTags...), " ")
	if item.Evidence != nil {
		text += " " + item.Evidence.Text
	}
	current := ""
	if item.SemanticEvent != nil {
		current = item.SemanticEvent.EventID
	}
	events, err := s.ListSemanticEvents(ctx, "", 1000)
	if err != nil {
		return nil, err
	}
	tokens := semanticTokens(text)
	type ranked struct {
		event domain.SemanticEvent
		score int
	}
	var rankedEvents []ranked
	for _, event := range events {
		if event.ID == current {
			continue
		}
		score := semanticOverlap(tokens, semanticTokens(eventSearchText(event)))
		if score > 0 {
			rankedEvents = append(rankedEvents, ranked{event: event, score: score})
		}
	}
	sort.SliceStable(rankedEvents, func(i, j int) bool {
		if rankedEvents[i].score == rankedEvents[j].score {
			return rankedEvents[i].event.LastSeenAt > rankedEvents[j].event.LastSeenAt
		}
		return rankedEvents[i].score > rankedEvents[j].score
	})
	if limit > len(rankedEvents) {
		limit = len(rankedEvents)
	}
	result := make([]domain.EventSuggestion, 0, limit)
	for _, value := range rankedEvents[:limit] {
		result = append(result, domain.EventSuggestion{EventID: value.event.ID, CanonicalClaim: value.event.CanonicalClaim, Actor: value.event.Actor, Object: value.event.Object, ReportCount: value.event.ReportCount, LastSeenAt: value.event.LastSeenAt})
	}
	return result, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func eventSearchText(value domain.SemanticEvent) string {
	return strings.Join(append([]string{value.CanonicalClaim, value.Actor, value.Action, value.Object, value.EventKind}, value.Aliases...), " ")
}

var semanticStopWords = map[string]bool{"about": true, "after": true, "again": true, "against": true, "also": true, "and": true, "are": true, "dari": true, "dengan": true, "for": true, "from": true, "ini": true, "into": true, "itu": true, "karena": true, "new": true, "post": true, "that": true, "the": true, "their": true, "this": true, "untuk": true, "was": true, "were": true, "with": true}

func semanticTokens(value string) map[string]bool {
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	result := map[string]bool{}
	for _, word := range words {
		if len([]rune(word)) >= 3 && !semanticStopWords[word] {
			result[word] = true
		}
	}
	return result
}

func semanticOverlap(left, right map[string]bool) int {
	count := 0
	for value := range left {
		if right[value] {
			count++
		}
	}
	return count
}

func (s *Store) CorrectSemanticEvent(ctx context.Context, timelineID, action, targetEventID string) (domain.EventCorrection, error) {
	if action != "not_same_event" && action != "same_event" {
		return domain.EventCorrection{}, fmt.Errorf("unsupported event correction %q", action)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.EventCorrection{}, err
	}
	defer tx.Rollback()
	var reportID, evidenceKey, fromEventID, fromRelation, itemRaw string
	var alreadyCorrected int
	if err := tx.QueryRowContext(ctx, `
		SELECT r.id,r.evidence_key,r.event_id,r.relation,t.item_json,r.corrected
		FROM semantic_event_reports r JOIN timeline_items t ON t.id=r.timeline_id WHERE r.timeline_id=?`, timelineID).
		Scan(&reportID, &evidenceKey, &fromEventID, &fromRelation, &itemRaw, &alreadyCorrected); err != nil {
		return domain.EventCorrection{}, err
	}
	if alreadyCorrected == 1 {
		return domain.EventCorrection{}, errors.New("existing event correction must be undone first")
	}
	toEventID := targetEventID
	toRelation := "duplicate_report"
	if action == "not_same_event" {
		var item domain.ReasonedItem
		decodeJSON(itemRaw, &item)
		toEventID = domain.NewID("event")
		toRelation = "new_event"
		now := domain.Now()
		aliases, _ := json.Marshal([]string{item.EventKey, item.Author})
		if _, err := tx.ExecContext(ctx, `INSERT INTO semantic_events(id,canonical_claim,actor,event_kind,event_start,aliases_json,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,?,?,?)`, toEventID, item.WhatChanged, item.Author, "other", item.PublishedAt, string(aliases), now, now); err != nil {
			return domain.EventCorrection{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO semantic_event_constraints(evidence_key,event_id,kind,created_at) VALUES(?,?,?,?) ON CONFLICT(evidence_key,event_id) DO UPDATE SET kind=excluded.kind,created_at=excluded.created_at`, evidenceKey, fromEventID, "must_not_merge", now); err != nil {
			return domain.EventCorrection{}, err
		}
	} else {
		var exists int
		if targetEventID == "" || tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_events WHERE id=?`, targetEventID).Scan(&exists) != nil || exists != 1 {
			return domain.EventCorrection{}, errors.New("target semantic event does not exist")
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO semantic_event_constraints(evidence_key,event_id,kind,created_at) VALUES(?,?,?,?) ON CONFLICT(evidence_key,event_id) DO UPDATE SET kind=excluded.kind,created_at=excluded.created_at`, evidenceKey, targetEventID, "must_merge", domain.Now()); err != nil {
			return domain.EventCorrection{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_event_reports SET event_id=?,relation=?,confidence=1,reason='User correction',corrected=1 WHERE id=?`, toEventID, toRelation, reportID); err != nil {
		return domain.EventCorrection{}, err
	}
	value := domain.EventCorrection{ID: domain.NewID("event_correction"), TimelineID: timelineID, Action: action, FromEventID: fromEventID, ToEventID: toEventID, CreatedAt: domain.Now()}
	if _, err := tx.ExecContext(ctx, `INSERT INTO semantic_event_corrections(id,report_id,timeline_id,action,from_event_id,from_relation,to_event_id,to_relation,created_at) VALUES(?,?,?,?,?,?,?,?,?)`, value.ID, reportID, timelineID, action, fromEventID, fromRelation, toEventID, toRelation, value.CreatedAt); err != nil {
		return domain.EventCorrection{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.EventCorrection{}, err
	}
	_ = s.cleanupOrphanSemanticEvents(ctx)
	return value, nil
}

func (s *Store) UndoSemanticCorrection(ctx context.Context, id string) (domain.EventCorrection, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.EventCorrection{}, err
	}
	defer tx.Rollback()
	var value domain.EventCorrection
	var reportID, fromRelation, toRelation string
	if err := tx.QueryRowContext(ctx, `SELECT id,report_id,timeline_id,action,from_event_id,from_relation,to_event_id,to_relation,created_at FROM semantic_event_corrections WHERE id=? AND undone_at IS NULL`, id).
		Scan(&value.ID, &reportID, &value.TimelineID, &value.Action, &value.FromEventID, &fromRelation, &value.ToEventID, &toRelation, &value.CreatedAt); err != nil {
		return domain.EventCorrection{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_event_reports SET event_id=?,relation=?,confidence=1,reason='User correction undone',corrected=0 WHERE id=?`, value.FromEventID, fromRelation, reportID); err != nil {
		return domain.EventCorrection{}, err
	}
	var evidenceKey string
	if err := tx.QueryRowContext(ctx, `SELECT evidence_key FROM semantic_event_reports WHERE id=?`, reportID).Scan(&evidenceKey); err != nil {
		return domain.EventCorrection{}, err
	}
	constraintEventID := value.ToEventID
	if value.Action == "not_same_event" {
		constraintEventID = value.FromEventID
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM semantic_event_constraints WHERE evidence_key=? AND event_id=?`, evidenceKey, constraintEventID); err != nil {
		return domain.EventCorrection{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE semantic_event_corrections SET undone_at=? WHERE id=?`, domain.Now(), id); err != nil {
		return domain.EventCorrection{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.EventCorrection{}, err
	}
	_ = s.cleanupOrphanSemanticEvents(ctx)
	return value, nil
}

func (s *Store) cleanupOrphanSemanticEvents(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM semantic_events
		WHERE NOT EXISTS (SELECT 1 FROM semantic_event_reports r WHERE r.event_id=semantic_events.id)
		  AND NOT EXISTS (SELECT 1 FROM semantic_event_constraints c WHERE c.event_id=semantic_events.id)
		  AND NOT EXISTS (
			SELECT 1 FROM semantic_event_corrections c
			WHERE c.undone_at IS NULL
			  AND (c.from_event_id=semantic_events.id OR c.to_event_id=semantic_events.id)
		  )`)
	return err
}

func (s *Store) EnforceRetention(ctx context.Context, settings domain.Settings) (domain.RetentionResult, error) {
	cutoff := time.Now().UTC().AddDate(0, 0, -settings.KnowledgeRetentionDays).Format(time.RFC3339Nano)
	result := domain.RetentionResult{LimitBytes: int64(settings.KnowledgeStorageLimitMB) * 1024 * 1024}
	var eventsBefore int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_events`).Scan(&eventsBefore)
	if _, err := s.db.ExecContext(ctx, `DELETE FROM semantic_event_reports WHERE created_at<?`, cutoff); err != nil {
		return result, err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM semantic_event_constraints WHERE created_at<? OR NOT EXISTS (SELECT 1 FROM timeline_items t WHERE t.evidence_key=semantic_event_constraints.evidence_key)`, cutoff); err != nil {
		return result, err
	}
	deleted, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE status IN ('completed','partial','failed','cancelled') AND completed_at IS NOT NULL AND completed_at<?`, cutoff)
	if err != nil {
		return result, err
	}
	if count, err := deleted.RowsAffected(); err == nil {
		result.RemovedSessions += int(count)
	}
	_ = s.cleanupOrphanSemanticEvents(ctx)
	_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	result.DatabaseBytes = s.databaseFootprint()
	for result.DatabaseBytes > result.LimitBytes {
		var id string
		err := s.db.QueryRowContext(ctx, `SELECT id FROM sessions WHERE status IN ('completed','partial','failed','cancelled') AND completed_at IS NOT NULL ORDER BY completed_at LIMIT 1`).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return result, err
		}
		if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id=?`, id); err != nil {
			return result, err
		}
		result.RemovedSessions++
		_ = s.cleanupOrphanSemanticEvents(ctx)
		_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
		result.DatabaseBytes = s.databaseFootprint()
	}
	if result.RemovedSessions > 0 {
		_, _ = s.db.ExecContext(ctx, `VACUUM`)
		_, _ = s.db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`)
	}
	var eventsAfter int
	_ = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM semantic_events`).Scan(&eventsAfter)
	result.RemovedEvents = eventsBefore - eventsAfter
	result.DatabaseBytes = s.databaseFootprint()
	return result, nil
}

func (s *Store) databaseFootprint() int64 {
	var total int64
	for _, path := range []string{s.path, s.path + "-wal", s.path + "-shm"} {
		if info, err := os.Stat(path); err == nil {
			total += info.Size()
		}
	}
	return total
}
