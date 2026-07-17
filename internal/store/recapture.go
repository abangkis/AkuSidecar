package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) CreateMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode) (domain.MediaRecapture, error) {
	if mode != domain.MediaRecaptureBackground && mode != domain.MediaRecaptureForeground {
		return domain.MediaRecapture{}, errors.New("media recapture mode must be background or foreground")
	}
	var source domain.Source
	var evidenceKey, itemRaw string
	if err := s.db.QueryRowContext(ctx, `SELECT source,evidence_key,item_json FROM timeline_items WHERE id=?`, timelineID).Scan(&source, &evidenceKey, &itemRaw); err != nil {
		return domain.MediaRecapture{}, err
	}
	block, err := s.timelineEvidence(ctx, timelineID, evidenceKey)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	var item domain.ReasonedItem
	decodeJSON(itemRaw, &item)
	targetURL := strings.TrimSpace(block.Permalink)
	if targetURL == "" {
		targetURL = strings.TrimSpace(item.SourceURL)
	}
	if !nativeSourceURL(source, targetURL) {
		return domain.MediaRecapture{}, errors.New("this item has no recapturable native post URL")
	}
	if len(block.Media) > 0 || stringValue(block.MediaRecovery, "outcome") != "unavailable" {
		return domain.MediaRecapture{}, errors.New("this item does not have unavailable captured media")
	}
	foregroundAuthorized := mode == domain.MediaRecaptureForeground
	if foregroundAuthorized {
		if err := s.requireUnavailableBackgroundRecapture(ctx, timelineID); err != nil {
			return domain.MediaRecapture{}, err
		}
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	job := domain.MediaRecapture{
		ID:          domain.NewID("recapture"),
		TimelineID:  timelineID,
		Source:      source,
		TargetURL:   targetURL,
		EvidenceKey: evidenceKey,
		Status:      "queued",
		CreatedAt:   domain.Now(),
	}
	job.Payload = map[string]any{
		"mode":                    "recapture_media",
		"source":                  source,
		"targetUrl":               targetURL,
		"targetEvidenceKey":       evidenceKey,
		"scrolls":                 0,
		"scrollFraction":          0.75,
		"scrollSettleMs":          100,
		"captureTimeoutMs":        30000,
		"pendingContentPolicy":    "detect_only",
		"sameTabMutationAllowed":  false,
		"pendingContentTimeoutMs": 500,
		"pendingContentSettleMs":  100,
		"sourceFreshnessPolicy":   "preserve_target",
		"captureVisibilityPolicy": settings.CaptureVisibility,
		"foregroundAuthorized":    foregroundAuthorized,
		"captureLeaseId":          job.ID,
		"maxBlocksPerSnapshot":    5,
		"maxBlockCharacters":      4000,
		"qualityReportRequired":   true,
		"qualityRetryBudget":      1,
		"qualityRetrySettleMs":    settings.QualityRetrySettleMS,
		"openIfMissing":           true,
		"tabLifecycle": map[string]any{
			"ownership":            "managed",
			"openedTabDisposition": "close_after_capture",
		},
		"restoreScroll":        false,
		"browserAdapter":       "aku-bridge",
		"acquisitionRound":     1,
		"maxAcquisitionRounds": 1,
	}
	payload, _ := json.Marshal(job.Payload)
	_, err = s.db.ExecContext(ctx, `INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,payload_json,created_at) VALUES(?,?,?,?,?,'queued',?,?)`, job.ID, job.TimelineID, job.Source, job.TargetURL, job.EvidenceKey, string(payload), job.CreatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return domain.MediaRecapture{}, errors.New("a recapture is already active for this item")
		}
		return domain.MediaRecapture{}, err
	}
	return job, nil
}

func (s *Store) requireUnavailableBackgroundRecapture(ctx context.Context, timelineID string) error {
	var outcome, payloadRaw string
	err := s.db.QueryRowContext(ctx, `SELECT outcome,payload_json FROM media_recaptures WHERE timeline_id=? AND status='completed' ORDER BY completed_at DESC LIMIT 1`, timelineID).Scan(&outcome, &payloadRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return errors.New("foreground recapture requires a completed unavailable background attempt")
		}
		return err
	}
	var payload map[string]any
	decodeJSON(payloadRaw, &payload)
	priorForeground, _ := payload["foregroundAuthorized"].(bool)
	if outcome != "unavailable" || priorForeground {
		return errors.New("foreground recapture requires the latest completed attempt to be unavailable in the background")
	}
	return nil
}

func (s *Store) ClaimMediaRecapture(ctx context.Context, id, bridgeID string) (domain.MediaRecapture, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	defer tx.Rollback()
	job, err := mediaRecaptureByID(ctx, tx, id)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	if job.Status != "queued" {
		return domain.MediaRecapture{}, fmt.Errorf("media recapture is %s, not queued", job.Status)
	}
	now := domain.Now()
	result, err := tx.ExecContext(ctx, `UPDATE media_recaptures SET status='claimed',claimed_by=?,claimed_at=? WHERE id=? AND status='queued'`, bridgeID, now, id)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	count, _ := result.RowsAffected()
	if count != 1 {
		return domain.MediaRecapture{}, errors.New("media recapture could not be claimed")
	}
	if err := tx.Commit(); err != nil {
		return domain.MediaRecapture{}, err
	}
	job.Status = "claimed"
	job.ClaimedAt = &now
	return job, nil
}

func (s *Store) CompleteMediaRecapture(ctx context.Context, id string, observation domain.Observation) (domain.MediaRecapture, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	defer tx.Rollback()
	job, err := mediaRecaptureByID(ctx, tx, id)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	if job.Status != "claimed" {
		return domain.MediaRecapture{}, fmt.Errorf("media recapture is %s, not claimed", job.Status)
	}
	if observation.Source != job.Source {
		return domain.MediaRecapture{}, errors.New("recapture observation source does not match the item")
	}
	block, ok := recapturedBlock(observation, job)
	if !ok {
		return domain.MediaRecapture{}, errors.New("recapture did not return the requested native post")
	}
	evidenceRaw, _ := json.Marshal(block)
	resultRaw, _ := json.Marshal(observation)
	outcome := "unavailable"
	if len(block.Media) > 0 {
		outcome = "recovered"
	} else if value := stringValue(block.MediaRecovery, "outcome"); value != "" {
		outcome = value
	}
	now := domain.Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO timeline_evidence_overrides(timeline_id,recapture_id,evidence_json,updated_at) VALUES(?,?,?,?) ON CONFLICT(timeline_id) DO UPDATE SET recapture_id=excluded.recapture_id,evidence_json=excluded.evidence_json,updated_at=excluded.updated_at`, job.TimelineID, job.ID, string(evidenceRaw), now); err != nil {
		return domain.MediaRecapture{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE media_recaptures SET status='completed',outcome=?,result_json=?,completed_at=? WHERE id=?`, outcome, string(resultRaw), now, id); err != nil {
		return domain.MediaRecapture{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MediaRecapture{}, err
	}
	job.Status = "completed"
	job.Outcome = outcome
	job.CompletedAt = &now
	return job, nil
}

func (s *Store) FailMediaRecapture(ctx context.Context, id string, failure domain.Failure) (domain.MediaRecapture, error) {
	job, err := s.MediaRecapture(ctx, id)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	if job.Status != "claimed" && job.Status != "queued" {
		return domain.MediaRecapture{}, fmt.Errorf("media recapture is %s, not active", job.Status)
	}
	if failure.Stage == "" {
		failure.Stage = "media_recapture"
	}
	raw, _ := json.Marshal(failure)
	now := domain.Now()
	if _, err := s.db.ExecContext(ctx, `UPDATE media_recaptures SET status='failed',error_json=?,completed_at=? WHERE id=? AND status IN ('queued','claimed')`, string(raw), now, id); err != nil {
		return domain.MediaRecapture{}, err
	}
	job.Status = "failed"
	job.Error = &failure
	job.CompletedAt = &now
	return job, nil
}

func (s *Store) MediaRecapture(ctx context.Context, id string) (domain.MediaRecapture, error) {
	return mediaRecaptureByID(ctx, s.db, id)
}

func (s *Store) ActiveMediaRecapture(ctx context.Context) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_recaptures WHERE status IN ('queued','claimed')`).Scan(&count)
	return count > 0, err
}

type rowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func mediaRecaptureByID(ctx context.Context, queryer rowQueryer, id string) (domain.MediaRecapture, error) {
	var job domain.MediaRecapture
	var payloadRaw string
	var claimedAt, completedAt, errorRaw sql.NullString
	err := queryer.QueryRowContext(ctx, `SELECT id,timeline_id,source,target_url,evidence_key,status,outcome,payload_json,created_at,claimed_at,completed_at,error_json FROM media_recaptures WHERE id=?`, id).Scan(&job.ID, &job.TimelineID, &job.Source, &job.TargetURL, &job.EvidenceKey, &job.Status, &job.Outcome, &payloadRaw, &job.CreatedAt, &claimedAt, &completedAt, &errorRaw)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	decodeJSON(payloadRaw, &job.Payload)
	if claimedAt.Valid {
		job.ClaimedAt = &claimedAt.String
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.String
	}
	if errorRaw.Valid {
		decodeJSON(errorRaw.String, &job.Error)
	}
	return job, nil
}

func (s *Store) timelineEvidence(ctx context.Context, timelineID, evidenceKey string) (domain.Block, error) {
	var overrideRaw string
	err := s.db.QueryRowContext(ctx, `SELECT evidence_json FROM timeline_evidence_overrides WHERE timeline_id=?`, timelineID).Scan(&overrideRaw)
	if err == nil {
		var block domain.Block
		decodeJSON(overrideRaw, &block)
		return block, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Block{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT o.observation_json FROM timeline_items t JOIN observations o ON o.run_id=t.run_id WHERE t.id=? ORDER BY o.created_at`, timelineID)
	if err != nil {
		return domain.Block{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return domain.Block{}, err
		}
		var observation domain.Observation
		decodeJSON(raw, &observation)
		for _, snapshot := range observation.Snapshots {
			for _, block := range snapshot.Blocks {
				if block.EvidenceKey == evidenceKey {
					return block, nil
				}
			}
		}
	}
	return domain.Block{}, errors.New("timeline evidence is unavailable")
}

func recapturedBlock(observation domain.Observation, job domain.MediaRecapture) (domain.Block, bool) {
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			if block.EvidenceKey == job.EvidenceKey || sameNativeURL(block.Permalink, job.TargetURL) {
				return block, true
			}
		}
	}
	return domain.Block{}, false
}

func nativeSourceURL(source domain.Source, raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return source == domain.SourceX && host == "x.com" && strings.Contains(parsed.Path, "/status/") ||
		source == domain.SourceLinkedIn && host == "www.linkedin.com" && (strings.Contains(parsed.Path, "/posts/") || strings.Contains(parsed.Path, "/feed/update/"))
}

func sameNativeURL(left, right string) bool {
	normalize := func(raw string) string {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil {
			return ""
		}
		parsed.RawQuery = ""
		parsed.Fragment = ""
		return strings.TrimSuffix(parsed.String(), "/")
	}
	return normalize(left) != "" && normalize(left) == normalize(right)
}

func stringValue(values map[string]any, key string) string {
	value, _ := values[key].(string)
	return value
}
