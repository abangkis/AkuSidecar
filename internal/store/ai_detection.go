package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) SaveAIAssessments(ctx context.Context, values []domain.AIAssessment) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index := range values {
		value := &values[index]
		if value.ID == "" {
			value.ID = domain.NewID("ai_assessment")
		}
		if value.CreatedAt == "" {
			value.CreatedAt = domain.Now()
		}
		if value.SupersedesID == "" {
			err := tx.QueryRowContext(ctx, `SELECT id FROM ai_assessments WHERE timeline_id=? AND undone_at IS NULL ORDER BY created_at DESC,id DESC LIMIT 1`, value.TimelineID).Scan(&value.SupersedesID)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}
		if err := value.Validate(); err != nil {
			return err
		}
		evidence, err := json.Marshal(value.EvidenceCodes)
		if err != nil {
			return err
		}
		var supersedes any
		if value.SupersedesID != "" {
			supersedes = value.SupersedesID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO ai_assessments(id,timeline_id,session_id,stage,status,confidence_band,evidence_json,assessed_object,signal_scope,provider,detector_version,content_fingerprint,rationale,supersedes_id,created_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			value.ID, value.TimelineID, value.SessionID, value.Stage, value.Status, value.ConfidenceBand,
			string(evidence), value.AssessedObject, value.SignalScope, value.Provider, value.DetectorVersion,
			value.ContentFingerprint, value.Rationale, supersedes, value.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) AddAIFeedback(ctx context.Context, timelineID string, input domain.AIFeedbackInput) (domain.AIFeedbackEvent, error) {
	if err := input.Validate(); err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	item, err := s.TimelineItem(ctx, timelineID)
	if err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	targetKey, err := aiFeedbackTargetKey(item, input.TargetType)
	if err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	value := domain.AIFeedbackEvent{
		ID: domain.NewID("ai_feedback"), TimelineID: item.ID, SessionID: item.SessionID, Source: item.Source,
		TargetType: input.TargetType, TargetKey: targetKey, Verdict: input.Verdict,
		SignalScope: input.SignalScope, Reason: input.Reason, CreatedAt: domain.Now(),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM ai_feedback_events
		WHERE target_type=? AND target_key=?
		ORDER BY rowid DESC LIMIT 1`, value.TargetType, value.TargetKey).Scan(&value.SupersedesID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.AIFeedbackEvent{}, err
	}
	var supersedes any
	if value.SupersedesID != "" {
		supersedes = value.SupersedesID
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO ai_feedback_events(
		  id,timeline_id,session_id,source,target_type,target_key,verdict,signal_scope,reason,supersedes_id,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TimelineID, value.SessionID, value.Source, value.TargetType, value.TargetKey,
		value.Verdict, value.SignalScope, value.Reason, supersedes, value.CreatedAt); err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	return value, nil
}

func (s *Store) UndoAIFeedback(ctx context.Context, id string) (domain.AIFeedbackEvent, error) {
	var prior domain.AIFeedbackEvent
	var supersedes sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id,timeline_id,session_id,source,target_type,target_key,verdict,signal_scope,reason,supersedes_id,created_at
		FROM ai_feedback_events WHERE id=? AND verdict<>'clear'`, id).
		Scan(&prior.ID, &prior.TimelineID, &prior.SessionID, &prior.Source, &prior.TargetType, &prior.TargetKey,
			&prior.Verdict, &prior.SignalScope, &prior.Reason, &supersedes, &prior.CreatedAt)
	if err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	prior.SupersedesID = supersedes.String
	var latestID string
	if err := s.db.QueryRowContext(ctx, `
		SELECT id FROM ai_feedback_events
		WHERE target_type=? AND target_key=?
		ORDER BY rowid DESC LIMIT 1`, prior.TargetType, prior.TargetKey).Scan(&latestID); err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	if latestID != prior.ID {
		return domain.AIFeedbackEvent{}, errors.New("only the current AI feedback decision can be cleared")
	}
	value := prior
	value.ID = domain.NewID("ai_feedback")
	value.Verdict = "clear"
	value.Reason = ""
	value.SupersedesID = prior.ID
	value.CreatedAt = domain.Now()
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_feedback_events(
		  id,timeline_id,session_id,source,target_type,target_key,verdict,signal_scope,reason,supersedes_id,created_at
		) VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		value.ID, value.TimelineID, value.SessionID, value.Source, value.TargetType, value.TargetKey,
		value.Verdict, value.SignalScope, value.Reason, value.SupersedesID, value.CreatedAt); err != nil {
		return domain.AIFeedbackEvent{}, err
	}
	return value, nil
}

func (s *Store) AIFeedbackHistory(ctx context.Context, timelineID string) ([]domain.AIFeedbackEvent, error) {
	item, err := s.TimelineItem(ctx, timelineID)
	if err != nil {
		return nil, err
	}
	targets := aiFeedbackTargetKeys(item)
	keys := make([]string, 0, len(targets))
	args := make([]any, 0, len(targets))
	for _, key := range targets {
		keys = append(keys, "?")
		args = append(args, key)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,timeline_id,session_id,source,target_type,target_key,verdict,signal_scope,reason,COALESCE(supersedes_id,''),created_at
		FROM ai_feedback_events WHERE target_key IN (`+strings.Join(keys, ",")+`)
		ORDER BY rowid DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.AIFeedbackEvent
	for rows.Next() {
		var value domain.AIFeedbackEvent
		if err := rows.Scan(&value.ID, &value.TimelineID, &value.SessionID, &value.Source, &value.TargetType,
			&value.TargetKey, &value.Verdict, &value.SignalScope, &value.Reason, &value.SupersedesID, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func aiFeedbackTargetKeys(item domain.TimelineItem) map[string]string {
	result := map[string]string{}
	for _, targetType := range []string{"post", "media", "quote", "account"} {
		if key, err := aiFeedbackTargetKey(item, targetType); err == nil {
			result[targetType] = key
		}
	}
	return result
}

func aiFeedbackTargetKey(item domain.TimelineItem, targetType string) (string, error) {
	base := string(item.Source) + "|" + strings.TrimSpace(item.EvidenceKey)
	switch targetType {
	case "post":
		return "post|" + base, nil
	case "media":
		return "media|" + base, nil
	case "quote":
		return "quote|" + base, nil
	case "account":
		author := item.Item.Author
		if item.Evidence != nil && strings.TrimSpace(item.Evidence.Author) != "" {
			author = item.Evidence.Author
		}
		author = normalizeAIAccountIdentity(author)
		if author == "" {
			return "", errors.New("this item has no stable captured account identity")
		}
		return "account|" + string(item.Source) + "|" + author, nil
	default:
		return "", fmt.Errorf("unsupported AI feedback targetType %q", targetType)
	}
}

func normalizeAIAccountIdentity(value string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(strings.TrimSpace(value)), func(char rune) bool {
		return unicode.IsSpace(char)
	}), " ")
}

func (s *Store) CreateAIDetectionJob(ctx context.Context, value domain.AIDetectionJob) (domain.AIDetectionJob, error) {
	if value.ID == "" {
		value.ID = domain.NewID("ai_job")
	}
	if value.Status == "" {
		value.Status = "queued"
	}
	if value.CreatedAt == "" {
		value.CreatedAt = domain.Now()
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_detection_jobs(id,session_id,status,provider,model,effort,candidate_count,created_at)
		VALUES(?,?,?,?,?,?,?,?)`,
		value.ID, value.SessionID, value.Status, value.Provider, value.Model, value.Effort, value.CandidateCount, value.CreatedAt)
	return value, err
}

func (s *Store) StartAIDetectionJob(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE ai_detection_jobs SET status='running',started_at=? WHERE id=? AND status='queued'`, domain.Now(), id)
	if err != nil {
		return err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) FinishAIDetectionJob(ctx context.Context, id, status string, durationMS int64, usage domain.ModelUsage, runErr error) error {
	if status != "completed" && status != "failed" && status != "cancelled" {
		return fmt.Errorf("unsupported AI detection job status %q", status)
	}
	message := ""
	if runErr != nil {
		message = runErr.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE ai_detection_jobs SET status=?,duration_ms=?,input_tokens=?,cached_input_tokens=?,output_tokens=?,reasoning_output_tokens=?,
		model=CASE WHEN ? <> '' THEN ? ELSE model END,
		effort=CASE WHEN ? <> '' THEN ? ELSE effort END,
		error=?,completed_at=? WHERE id=?`,
		status, durationMS, usage.Input, usage.CachedInput, usage.Output, usage.ReasoningOutput,
		usage.ProviderModel, usage.ProviderModel, usage.NativeReasoning, usage.NativeReasoning,
		message, domain.Now(), id)
	return err
}

func (s *Store) AIDetectionJob(ctx context.Context, sessionID string) (*domain.AIDetectionJob, error) {
	var value domain.AIDetectionJob
	var input, cachedInput, output, reasoningOutput sql.NullInt64
	var startedAt, completedAt sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id,session_id,status,provider,model,effort,candidate_count,duration_ms,
		       input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,error,created_at,started_at,completed_at
		FROM ai_detection_jobs WHERE session_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, sessionID).
		Scan(&value.ID, &value.SessionID, &value.Status, &value.Provider, &value.Model, &value.Effort,
			&value.CandidateCount, &value.DurationMS, &input, &cachedInput, &output, &reasoningOutput,
			&value.Error, &value.CreatedAt, &startedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if input.Valid {
		value.InputTokens = &input.Int64
	}
	if cachedInput.Valid {
		value.CachedInputTokens = &cachedInput.Int64
	}
	if output.Valid {
		value.OutputTokens = &output.Int64
	}
	if reasoningOutput.Valid {
		value.ReasoningOutputTokens = &reasoningOutput.Int64
	}
	value.StartedAt = startedAt.String
	value.CompletedAt = completedAt.String
	return &value, nil
}

func (s *Store) AIDetectorYield(ctx context.Context, sessionID string) (*domain.AIDetectorYield, error) {
	value := &domain.AIDetectorYield{}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*),
		  COALESCE(SUM(CASE WHEN status='strong_signals' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN status='no_signal_detected' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN status='insufficient_evidence' THEN 1 ELSE 0 END),0)
		FROM ai_assessments WHERE session_id=? AND stage='fast' AND undone_at IS NULL`, sessionID).
		Scan(&value.FastReviewed, &value.FastStrong, &value.FastNoSignal, &value.FastInsufficient); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM ai_assessments
		WHERE session_id=? AND stage='deep' AND undone_at IS NULL`, sessionID).
		Scan(&value.DeepReviewed); err != nil {
		return nil, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(candidate_count,0) FROM ai_detection_jobs
		WHERE session_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, sessionID).
		Scan(&value.DeepEligible); err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	value.DeepSkipped = value.FastReviewed - value.DeepEligible
	if value.DeepSkipped < 0 {
		value.DeepSkipped = 0
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN status IN ('completed','failed') THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN status='completed' AND manifest_state='no_manifest' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN status='completed' AND manifest_state IN ('valid','invalid') THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN status='completed' AND ai_origin IN ('generated','edited') THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN status='failed' THEN 1 ELSE 0 END),0)
		FROM media_provenance_assessments WHERE session_id=?`, sessionID).
		Scan(&value.C2PAInspected, &value.C2PANoManifest, &value.C2PAWithManifest, &value.C2PAAIOrigin, &value.C2PAFailed); err != nil {
		return nil, err
	}
	items, err := s.ListSessionItems(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.Evidence == nil {
			continue
		}
		for _, signal := range domain.PlatformOriginSignals(item.Evidence.Presentation) {
			if signal.Kind == "platform_ai_label" {
				value.PlatformAISignals++
				break
			}
		}
	}
	if value.FastReviewed == 0 && value.C2PAInspected == 0 && value.PlatformAISignals == 0 {
		return nil, nil
	}
	return value, nil
}

func (s *Store) attachAIDetections(ctx context.Context, items []domain.TimelineItem) error {
	if len(items) == 0 {
		return nil
	}
	byTimeline := make(map[string][]domain.AIAssessment, len(items))
	itemByID := make(map[string]*domain.TimelineItem, len(items))
	sessions := map[string]bool{}
	placeholders := make([]string, 0, len(items))
	args := make([]any, 0, len(items))
	for index := range items {
		itemByID[items[index].ID] = &items[index]
		sessions[items[index].SessionID] = true
		placeholders = append(placeholders, "?")
		args = append(args, items[index].ID)
	}
	timelineIDs := make([]string, 0, len(items))
	for index := range items {
		timelineIDs = append(timelineIDs, items[index].ID)
	}
	mediaByTimeline, err := s.mediaProvenanceByTimeline(ctx, timelineIDs)
	if err != nil {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,timeline_id,session_id,stage,status,confidence_band,evidence_json,assessed_object,signal_scope,provider,detector_version,content_fingerprint,rationale,COALESCE(supersedes_id,''),created_at
		FROM ai_assessments WHERE undone_at IS NULL AND timeline_id IN (`+strings.Join(placeholders, ",")+`) ORDER BY created_at,id`, args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value domain.AIAssessment
		var evidenceRaw string
		if err := rows.Scan(&value.ID, &value.TimelineID, &value.SessionID, &value.Stage, &value.Status, &value.ConfidenceBand,
			&evidenceRaw, &value.AssessedObject, &value.SignalScope, &value.Provider, &value.DetectorVersion,
			&value.ContentFingerprint, &value.Rationale, &value.SupersedesID, &value.CreatedAt); err != nil {
			rows.Close()
			return err
		}
		decodeJSON(evidenceRaw, &value.EvidenceCodes)
		byTimeline[value.TimelineID] = append(byTimeline[value.TimelineID], value)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	jobs := map[string]string{}
	if len(sessions) > 0 {
		sessionPlaceholders := make([]string, 0, len(sessions))
		sessionArgs := make([]any, 0, len(sessions))
		for sessionID := range sessions {
			sessionPlaceholders = append(sessionPlaceholders, "?")
			sessionArgs = append(sessionArgs, sessionID)
		}
		jobRows, err := s.db.QueryContext(ctx, `
			WITH ranked AS (
			  SELECT session_id,status,ROW_NUMBER() OVER (PARTITION BY session_id ORDER BY created_at DESC,id DESC) AS job_rank
			  FROM ai_detection_jobs WHERE session_id IN (`+strings.Join(sessionPlaceholders, ",")+`)
			)
			SELECT session_id,status FROM ranked WHERE job_rank=1`, sessionArgs...)
		if err != nil {
			return err
		}
		for jobRows.Next() {
			var sessionID, status string
			if err := jobRows.Scan(&sessionID, &status); err != nil {
				jobRows.Close()
				return err
			}
			jobs[sessionID] = status
		}
		if err := jobRows.Err(); err != nil {
			jobRows.Close()
			return err
		}
		if err := jobRows.Close(); err != nil {
			return err
		}
	}
	policies, historyCounts, err := s.personalAIPolicies(ctx, items)
	if err != nil {
		return err
	}
	for timelineID, item := range itemByID {
		var presentation map[string]any
		if item.Evidence != nil {
			presentation = item.Evidence.Presentation
		}
		item.AIDetection = resolveAIDetectionWithPolicy(
			byTimeline[timelineID], jobs[item.SessionID], mediaByTimeline[timelineID],
			presentation, policies[timelineID], historyCounts[timelineID],
		)
	}
	return nil
}

func resolveAIDetection(history []domain.AIAssessment, deepStatus string, mediaHistory []domain.MediaProvenanceAssessment, presentations ...map[string]any) *domain.TimelineAIDetection {
	var presentation map[string]any
	if len(presentations) > 0 {
		presentation = presentations[0]
	}
	return resolveAIDetectionWithPolicy(history, deepStatus, mediaHistory, presentation, nil, 0)
}

func resolveAIDetectionWithPolicy(
	history []domain.AIAssessment,
	deepStatus string,
	mediaHistory []domain.MediaProvenanceAssessment,
	presentation map[string]any,
	policy *domain.PersonalAIPolicy,
	feedbackHistoryCount int,
) (resolved *domain.TimelineAIDetection) {
	defer func() {
		resolved = applyPersonalAIPolicy(resolved, policy, feedbackHistoryCount)
	}()
	var platformSignals []domain.PlatformOriginSignal
	if presentation != nil {
		platformSignals = domain.PlatformOriginSignals(presentation)
	}
	if len(history) == 0 && deepStatus == "" && len(mediaHistory) == 0 && len(platformSignals) == 0 && policy == nil {
		return nil
	}
	value := &domain.TimelineAIDetection{
		HistoryCount: len(history), DeepStatus: deepStatus,
		PendingDeep:     deepStatus == "queued" || deepStatus == "running",
		PlatformSignals: platformSignals,
	}
	var fast, deep *domain.AIAssessment
	for index := range history {
		assessment := &history[index]
		switch assessment.Stage {
		case "fast":
			fast = assessment
		case "deep":
			deep = assessment
		}
	}
	current := fast
	if deep != nil {
		current = deep
	}
	for _, media := range mediaHistory {
		if media.Status == "queued" || media.Status == "running" {
			value.PendingMedia = true
		}
		if media.Status == "completed" && media.ManifestState == "valid" && (media.AIOrigin == "generated" || media.AIOrigin == "edited") {
			value.MediaSignals = append(value.MediaSignals, mediaSignal(media))
		}
	}
	if current != nil {
		value.AssessmentID = current.ID
		value.Stage = current.Stage
		value.Status = current.Status
		value.ConfidenceBand = current.ConfidenceBand
		value.EvidenceCodes = current.EvidenceCodes
		value.AssessedObject = current.AssessedObject
		value.SignalScope = current.SignalScope
		value.DetectorVersion = current.DetectorVersion
		value.LatestAssessedAt = current.CreatedAt
		value.Detail = current.Rationale
	}

	for _, signal := range platformSignals {
		if signal.Kind != "platform_ai_label" {
			continue
		}
		value.Stage = "platform_origin"
		value.Status = "strong_signals"
		value.ConfidenceBand = "high"
		value.EvidenceCodes = []string{"platform_ai_label"}
		value.AssessedObject = signal.Scope
		value.SignalScope = signal.Scope
		value.DetectorVersion = "platform-origin-v1"
		value.BadgeLabel = "Platform AI label"
		value.Detail = "The source platform displays “" + signal.Label + "” for this item."
		if signal.Scope == "attached_media" {
			value.BadgeLabel = "Platform AI media label"
			value.Detail += " This describes the attached media, not authorship of the social-post text."
		}
		value.RouteToSignals = true
		value.HideEligible = true
		value.DirectOriginEvidence = true
		return value
	}

	if len(value.MediaSignals) > 0 {
		signal := value.MediaSignals[len(value.MediaSignals)-1]
		value.Stage = "media_provenance"
		value.Status = "strong_signals"
		value.ConfidenceBand = "high"
		value.EvidenceCodes = signal.EvidenceCodes
		value.AssessedObject = "attached_media"
		value.SignalScope = "attached_media"
		value.DetectorVersion = signal.VerifierVersion
		value.LatestAssessedAt = signal.AssessedAt
		value.BadgeLabel = signal.Label
		value.Detail = signal.Detail + " This describes the attached image, not authorship of the social-post text."
		value.RouteToSignals = true
		value.HideEligible = true
		value.DirectMediaProvenance = true
		value.DirectOriginEvidence = true
		return value
	}

	if current == nil {
		return value
	}

	directPlatform := fast != nil && containsEvidence(fast.EvidenceCodes, "platform_ai_label")
	directProvenance := fast != nil && containsEvidence(fast.EvidenceCodes, "verified_ai_provenance")
	if directPlatform || directProvenance {
		value.Stage = fast.Stage
		value.Status = fast.Status
		value.ConfidenceBand = fast.ConfidenceBand
		value.EvidenceCodes = fast.EvidenceCodes
		value.AssessedObject = fast.AssessedObject
		value.SignalScope = fast.SignalScope
		value.AssessmentID = fast.ID
		value.DetectorVersion = fast.DetectorVersion
		value.BadgeLabel = "Platform AI label"
		if directProvenance {
			value.BadgeLabel = "Verified AI provenance"
		}
		value.RouteToSignals = true
		value.HideEligible = true
		if deep != nil && deep.Status != "strong_signals" {
			value.Detail = "Direct origin evidence remains authoritative; Deep Detection did not independently confirm the content-level signals."
		}
		return value
	}

	if deep != nil {
		if deep.Status == "strong_signals" && deep.DetectorVersion != domain.CurrentAIDeepDetectorVersion {
			value.Status = "no_signal_detected"
			value.ConfidenceBand = "low"
			value.EvidenceCodes = nil
			value.SignalScope = "none"
			value.BadgeLabel = "AI assessment corrected"
			value.Detail = "An earlier strong assessment did not pass the current social-post object-scope contract."
			value.Corrected = true
			value.RouteToSignals = false
			value.HideEligible = false
			return value
		}
		switch deep.Status {
		case "strong_signals":
			value.BadgeLabel = "AI signals confirmed"
			value.RouteToSignals = true
			value.HideEligible = true
		case "conflicting_evidence":
			if fast != nil && fast.Status == "strong_signals" {
				value.BadgeLabel = "AI signals disputed"
				value.Corrected = true
			}
		case "no_signal_detected", "insufficient_evidence":
			if fast != nil && fast.Status == "strong_signals" {
				value.BadgeLabel = "AI assessment corrected"
				value.Detail = "Deep Detection did not confirm the earlier preliminary AI-origin assessment."
				value.Corrected = true
			}
		}
		return value
	}

	if fast != nil && fast.Status == "strong_signals" {
		value.BadgeLabel = "Strong AI signals · Preliminary"
		if containsEvidence(fast.EvidenceCodes, "author_declared_ai") {
			value.BadgeLabel = "Author-declared AI · Preliminary"
		}
		value.RouteToSignals = true
	}
	return value
}

func applyPersonalAIPolicy(value *domain.TimelineAIDetection, policy *domain.PersonalAIPolicy, historyCount int) *domain.TimelineAIDetection {
	if value == nil && (policy == nil || !policy.Applied) {
		return nil
	}
	if value == nil {
		value = &domain.TimelineAIDetection{}
	}
	value.FeedbackHistoryCount = historyCount
	if policy == nil || !policy.Applied {
		return value
	}
	copy := *policy
	if copy.ReviewRequested && value != nil && value.Stage == "deep" && happenedAtOrAfter(value.LatestAssessedAt, copy.RequestedAt) {
		copy.ReviewRequested = false
	}
	value.PersonalPolicy = &copy
	value.CorrectionID = policy.FeedbackEventID
	if copy.ReviewRequested {
		value.BadgeLabel = "AI review requested by you"
		if value.Detail == "" {
			value.Detail = "You marked this item as unsure. AkuBrowser will prioritize it for bounded Deep Detection without treating it as AI-generated."
		} else {
			value.Detail += " You marked this item as unsure, so it is prioritized for bounded Deep Detection."
		}
		return value
	}
	if policy.Verdict == "unsure" {
		return value
	}
	value.UserOverride = true
	if policy.Verdict == "ai" {
		value.RouteToSignals = true
		value.HideEligible = true
		switch policy.TargetType {
		case "media":
			value.BadgeLabel = "AI media marked by you"
		case "quote":
			value.BadgeLabel = "AI quote marked by you"
		case "account":
			value.BadgeLabel = "AI account rule · You"
		default:
			value.BadgeLabel = "Marked as AI by you"
		}
		value.Detail = personalAIPolicyDetail(policy)
		return value
	}
	if policy.Verdict != "not_ai" {
		return value
	}
	applies := false
	switch policy.TargetType {
	case "media":
		applies = value.SignalScope == "attached_media" || value.DirectMediaProvenance
	case "quote":
		applies = value.SignalScope == "quoted_post"
	case "account":
		applies = value.SignalScope != "attached_media" && !value.DirectMediaProvenance
	default:
		applies = value.SignalScope != "attached_media" && !value.DirectMediaProvenance
	}
	if applies {
		value.RouteToSignals = false
		value.HideEligible = false
		value.Corrected = value.Status == "strong_signals" || value.DirectOriginEvidence
		value.BadgeLabel = "Marked not AI by you"
		value.Detail = personalAIPolicyDetail(policy)
	}
	return value
}

func personalAIPolicyDetail(policy *domain.PersonalAIPolicy) string {
	detail := "Your personal AI feedback is authoritative for this presentation scope."
	if policy.AccountRule {
		detail = "Your explicit account-level AI rule is applied to this captured account identity."
	}
	if policy.Reason != "" {
		detail += " Reason: " + strings.ReplaceAll(policy.Reason, "_", " ") + "."
	}
	return detail
}

func (s *Store) personalAIPolicies(ctx context.Context, items []domain.TimelineItem) (map[string]*domain.PersonalAIPolicy, map[string]int, error) {
	result := make(map[string]*domain.PersonalAIPolicy, len(items))
	historyCounts := make(map[string]int, len(items))
	if len(items) == 0 {
		return result, historyCounts, nil
	}
	targetsByTimeline := make(map[string]map[string]string, len(items))
	uniqueTargets := map[string]bool{}
	for _, item := range items {
		targets := aiFeedbackTargetKeys(item)
		targetsByTimeline[item.ID] = targets
		for _, key := range targets {
			uniqueTargets[key] = true
		}
	}
	placeholders := make([]string, 0, len(uniqueTargets))
	args := make([]any, 0, len(uniqueTargets))
	for key := range uniqueTargets {
		placeholders = append(placeholders, "?")
		args = append(args, key)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT rowid,id,timeline_id,session_id,source,target_type,target_key,verdict,signal_scope,reason,COALESCE(supersedes_id,''),created_at
		FROM ai_feedback_events WHERE target_key IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY rowid`, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	latestByTarget := map[string]domain.AIFeedbackEvent{}
	latestSequenceByTarget := map[string]int64{}
	countByTarget := map[string]int{}
	for rows.Next() {
		var value domain.AIFeedbackEvent
		var sequence int64
		if err := rows.Scan(&sequence, &value.ID, &value.TimelineID, &value.SessionID, &value.Source, &value.TargetType,
			&value.TargetKey, &value.Verdict, &value.SignalScope, &value.Reason, &value.SupersedesID, &value.CreatedAt); err != nil {
			return nil, nil, err
		}
		latestByTarget[value.TargetKey] = value
		latestSequenceByTarget[value.TargetKey] = sequence
		countByTarget[value.TargetKey]++
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	for timelineID, targets := range targetsByTimeline {
		var selected *domain.AIFeedbackEvent
		var selectedSequence int64
		for _, targetType := range []string{"post", "media", "quote"} {
			key := targets[targetType]
			historyCounts[timelineID] += countByTarget[key]
			value, ok := latestByTarget[key]
			if !ok || value.Verdict == "clear" {
				continue
			}
			sequence := latestSequenceByTarget[key]
			if selected == nil || sequence > selectedSequence {
				copy := value
				selected = &copy
				selectedSequence = sequence
			}
		}
		accountKey := targets["account"]
		historyCounts[timelineID] += countByTarget[accountKey]
		if selected == nil {
			if value, ok := latestByTarget[accountKey]; ok && value.Verdict != "clear" {
				copy := value
				selected = &copy
			}
		}
		if selected == nil {
			continue
		}
		result[timelineID] = &domain.PersonalAIPolicy{
			Applied: true, Source: map[bool]string{true: "account_rule", false: "exact_feedback"}[selected.TargetType == "account"],
			Verdict: selected.Verdict, TargetType: selected.TargetType, SignalScope: selected.SignalScope,
			Reason: selected.Reason, ReviewRequested: selected.Verdict == "unsure",
			AccountRule: selected.TargetType == "account", FeedbackEventID: selected.ID, RequestedAt: selected.CreatedAt,
		}
	}
	return result, historyCounts, nil
}

func happenedAtOrAfter(value, boundary string) bool {
	if value == "" || boundary == "" {
		return false
	}
	valueTime, valueErr := time.Parse(time.RFC3339Nano, value)
	boundaryTime, boundaryErr := time.Parse(time.RFC3339Nano, boundary)
	return valueErr == nil && boundaryErr == nil && !valueTime.Before(boundaryTime)
}

func containsEvidence(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
