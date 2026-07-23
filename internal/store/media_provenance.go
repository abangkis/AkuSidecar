package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/mediaprovenance"
)

func (s *Store) QueueMediaProvenance(ctx context.Context, items []domain.TimelineItem, provider, verifierVersion string) (int, error) {
	if provider == "" || verifierVersion == "" {
		return 0, errors.New("media provenance queue requires provider and verifier version")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	queued := 0
	for _, item := range items {
		if item.Evidence == nil {
			continue
		}
		for mediaIndex, media := range item.Evidence.Media {
			kind, _ := media["kind"].(string)
			target, _ := media["url"].(string)
			if strings.ToLower(strings.TrimSpace(kind)) != "image" || !strings.HasPrefix(strings.TrimSpace(target), "https://") {
				continue
			}
			sum := sha256.Sum256([]byte(target))
			targetHash := hex.EncodeToString(sum[:])
			result, err := tx.ExecContext(ctx, `
				INSERT OR IGNORE INTO media_provenance_assessments(
				  id,timeline_id,session_id,source,media_index,media_kind,target_url,target_url_hash,
				  status,manifest_state,trust_state,ai_origin,evidence_json,provider,verifier_version,created_at
				) VALUES(?,?,?,?,?,'image',?,?,'queued','pending','pending','unknown','[]',?,?,?)`,
				domain.NewID("media_provenance"), item.ID, item.SessionID, item.Source, mediaIndex,
				target, targetHash, provider, verifierVersion, domain.Now())
			if err != nil {
				return 0, err
			}
			if changed, _ := result.RowsAffected(); changed == 1 {
				queued++
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return queued, nil
}

func (s *Store) ClaimMediaProvenance(ctx context.Context, verifierVersion string) (*domain.MediaProvenanceAssessment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var value domain.MediaProvenanceAssessment
	var evidenceRaw string
	err = tx.QueryRowContext(ctx, `
		SELECT id,timeline_id,session_id,source,media_index,media_kind,target_url,target_url_hash,
		       status,manifest_state,trust_state,ai_origin,evidence_json,asset_sha256,provider,
		       verifier_version,rationale,duration_ms,error,created_at,COALESCE(started_at,''),COALESCE(completed_at,'')
		FROM media_provenance_assessments
		WHERE status='queued' AND verifier_version=?
		ORDER BY created_at,id LIMIT 1`, verifierVersion).
		Scan(&value.ID, &value.TimelineID, &value.SessionID, &value.Source, &value.MediaIndex, &value.MediaKind,
			&value.TargetURL, &value.TargetURLHash, &value.Status, &value.ManifestState, &value.TrustState,
			&value.AIOrigin, &evidenceRaw, &value.AssetSHA256, &value.Provider, &value.VerifierVersion,
			&value.Rationale, &value.DurationMS, &value.Error, &value.CreatedAt, &value.StartedAt, &value.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	decodeJSON(evidenceRaw, &value.EvidenceCodes)
	now := domain.Now()
	result, err := tx.ExecContext(ctx, `UPDATE media_provenance_assessments SET status='running',started_at=? WHERE id=? AND status='queued'`, now, value.ID)
	if err != nil {
		return nil, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return nil, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	value.Status = "running"
	value.StartedAt = now
	return &value, nil
}

func (s *Store) FinishMediaProvenance(ctx context.Context, id string, result mediaprovenance.Result, runErr error) error {
	status := "completed"
	message := ""
	if runErr != nil {
		status = "failed"
		message = runErr.Error()
		if result.ManifestState == "pending" || result.ManifestState == "" {
			result.ManifestState = "unavailable"
		}
		if result.TrustState == "pending" || result.TrustState == "" {
			result.TrustState = "not_applicable"
		}
		if result.AIOrigin == "" {
			result.AIOrigin = "unknown"
		}
	}
	evidence, err := json.Marshal(result.EvidenceCodes)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE media_provenance_assessments SET
		  status=?,manifest_state=?,trust_state=?,ai_origin=?,evidence_json=?,asset_sha256=?,
		  rationale=?,duration_ms=?,error=?,completed_at=?
		WHERE id=?`,
		status, result.ManifestState, result.TrustState, result.AIOrigin, string(evidence), result.AssetSHA256,
		result.Rationale, result.DurationMS, message, domain.Now(), id)
	return err
}

func (s *Store) CancelRunningMediaProvenance(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE media_provenance_assessments
		SET status='queued',started_at=NULL,error=''
		WHERE status='running'`)
	return err
}

func (s *Store) mediaProvenanceByTimeline(ctx context.Context, timelineIDs []string) (map[string][]domain.MediaProvenanceAssessment, error) {
	result := make(map[string][]domain.MediaProvenanceAssessment, len(timelineIDs))
	if len(timelineIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(timelineIDs))
	args := make([]any, len(timelineIDs))
	for index, id := range timelineIDs {
		placeholders[index] = "?"
		args[index] = id
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id,timeline_id,session_id,source,media_index,media_kind,target_url_hash,status,
		       manifest_state,trust_state,ai_origin,evidence_json,asset_sha256,provider,verifier_version,
		       rationale,duration_ms,error,created_at,COALESCE(started_at,''),COALESCE(completed_at,'')
		FROM media_provenance_assessments
		WHERE timeline_id IN (`+strings.Join(placeholders, ",")+`)
		ORDER BY created_at,id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var value domain.MediaProvenanceAssessment
		var evidenceRaw string
		if err := rows.Scan(&value.ID, &value.TimelineID, &value.SessionID, &value.Source, &value.MediaIndex,
			&value.MediaKind, &value.TargetURLHash, &value.Status, &value.ManifestState, &value.TrustState,
			&value.AIOrigin, &evidenceRaw, &value.AssetSHA256, &value.Provider, &value.VerifierVersion,
			&value.Rationale, &value.DurationMS, &value.Error, &value.CreatedAt, &value.StartedAt,
			&value.CompletedAt); err != nil {
			return nil, err
		}
		decodeJSON(evidenceRaw, &value.EvidenceCodes)
		result[value.TimelineID] = append(result[value.TimelineID], value)
	}
	return result, rows.Err()
}

func mediaSignal(value domain.MediaProvenanceAssessment) domain.MediaAISignal {
	label := "AI media provenance"
	if value.TrustState == "trusted" {
		label = "Verified AI media"
	}
	detail := value.Rationale
	if detail == "" {
		detail = fmt.Sprintf("The attached image declares %s C2PA provenance.", value.AIOrigin)
	}
	return domain.MediaAISignal{
		MediaIndex: value.MediaIndex, Origin: value.AIOrigin, ManifestState: value.ManifestState,
		TrustState: value.TrustState, EvidenceCodes: value.EvidenceCodes, Label: label,
		Detail: detail, VerifierVersion: value.VerifierVersion, AssessedAt: value.CompletedAt,
	}
}
