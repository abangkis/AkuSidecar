package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

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

func (s *Store) AddAICorrection(ctx context.Context, timelineID, verdict string) (domain.AIAssessment, error) {
	var sessionID string
	if err := s.db.QueryRowContext(ctx, `SELECT session_id FROM timeline_items WHERE id=?`, timelineID).Scan(&sessionID); err != nil {
		return domain.AIAssessment{}, err
	}
	status := "user_marked_ai"
	if verdict == "not_ai" {
		status = "user_marked_not_ai"
	} else if verdict != "ai" {
		return domain.AIAssessment{}, errors.New("AI correction verdict must be ai or not_ai")
	}
	value := domain.AIAssessment{
		ID: domain.NewID("ai_assessment"), TimelineID: timelineID, SessionID: sessionID,
		Stage: "user", Status: status, ConfidenceBand: "high", Provider: "user",
		AssessedObject: "social_post", SignalScope: "social_post",
		DetectorVersion: "personal-override-v1", Rationale: "Personal presentation override recorded by the user.", CreatedAt: domain.Now(),
	}
	if status == "user_marked_not_ai" {
		value.SignalScope = "none"
	}
	if err := s.SaveAIAssessments(ctx, []domain.AIAssessment{value}); err != nil {
		return domain.AIAssessment{}, err
	}
	return value, nil
}

func (s *Store) UndoAICorrection(ctx context.Context, id string) (domain.AIAssessment, error) {
	var value domain.AIAssessment
	var evidenceRaw string
	var supersedes sql.NullString
	err := s.db.QueryRowContext(ctx, `
		SELECT id,timeline_id,session_id,stage,status,confidence_band,evidence_json,assessed_object,signal_scope,provider,detector_version,content_fingerprint,rationale,supersedes_id,created_at
		FROM ai_assessments WHERE id=? AND stage='user' AND undone_at IS NULL`, id).
		Scan(&value.ID, &value.TimelineID, &value.SessionID, &value.Stage, &value.Status, &value.ConfidenceBand,
			&evidenceRaw, &value.AssessedObject, &value.SignalScope, &value.Provider, &value.DetectorVersion,
			&value.ContentFingerprint, &value.Rationale, &supersedes, &value.CreatedAt)
	if err != nil {
		return domain.AIAssessment{}, err
	}
	decodeJSON(evidenceRaw, &value.EvidenceCodes)
	value.SupersedesID = supersedes.String
	now := domain.Now()
	if _, err := s.db.ExecContext(ctx, `UPDATE ai_assessments SET undone_at=? WHERE id=? AND undone_at IS NULL`, now, id); err != nil {
		return domain.AIAssessment{}, err
	}
	value.UndoneAt = &now
	return value, nil
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
		UPDATE ai_detection_jobs SET status=?,duration_ms=?,input_tokens=?,cached_input_tokens=?,output_tokens=?,reasoning_output_tokens=?,error=?,completed_at=? WHERE id=?`,
		status, durationMS, usage.Input, usage.CachedInput, usage.Output, usage.ReasoningOutput, message, domain.Now(), id)
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
	for timelineID, item := range itemByID {
		item.AIDetection = resolveAIDetection(byTimeline[timelineID], jobs[item.SessionID], mediaByTimeline[timelineID])
	}
	return nil
}

func resolveAIDetection(history []domain.AIAssessment, deepStatus string, mediaHistory []domain.MediaProvenanceAssessment) *domain.TimelineAIDetection {
	if len(history) == 0 && deepStatus == "" && len(mediaHistory) == 0 {
		return nil
	}
	value := &domain.TimelineAIDetection{HistoryCount: len(history), DeepStatus: deepStatus, PendingDeep: deepStatus == "queued" || deepStatus == "running"}
	var fast, deep, user *domain.AIAssessment
	for index := range history {
		assessment := &history[index]
		switch assessment.Stage {
		case "fast":
			fast = assessment
		case "deep":
			deep = assessment
		case "user":
			user = assessment
		}
	}
	current := fast
	if deep != nil {
		current = deep
	}
	if user != nil {
		current = user
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

	if user != nil {
		value.UserOverride = true
		value.CorrectionID = user.ID
		if user.Status == "user_marked_ai" {
			value.BadgeLabel = "Marked as AI by you"
			value.RouteToSignals = true
			value.HideEligible = true
		} else {
			value.BadgeLabel = "Marked not AI by you"
			value.Corrected = (fast != nil && fast.Status == "strong_signals") || (deep != nil && deep.Status == "strong_signals")
		}
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

func containsEvidence(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
