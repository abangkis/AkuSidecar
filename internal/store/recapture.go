package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

var (
	xCandidateIdentity = regexp.MustCompile(`^x:status:([0-9]{5,30})$`)
	xNumericIdentity   = regexp.MustCompile(`^[0-9]{5,30}$`)
	xStatusPath        = regexp.MustCompile(`/status/([0-9]{5,30})(?:\b|/|$)`)
)

const passiveXMediaEngineVersion = "passive-x-media-enrichment-v2"

func (s *Store) CreateMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode) (domain.MediaRecapture, error) {
	return s.CreateMediaRecaptureForReason(ctx, timelineID, mode, domain.MediaRecaptureMissingMedia)
}

func (s *Store) CreateMediaRecaptureForReason(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode, reason domain.MediaRecaptureReason) (domain.MediaRecapture, error) {
	return s.CreateOwnedMediaRecapture(ctx, timelineID, mode, reason, nil)
}

// CreateOwnedMediaRecapture persists the runtime stamp in the same write that
// admits the job. Legacy callers retain their original unstamped payload.
func (s *Store) CreateOwnedMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode, reason domain.MediaRecaptureReason, runtime map[string]any, collector ...string) (domain.MediaRecapture, error) {
	return s.createOwnedMediaRecapture(ctx, timelineID, mode, reason, runtime, nil, collector...)
}

// CreateOwnedMediaRecaptureWithAdmission creates a durable, unclaimable job
// whose runtime stamp will be bound only after asynchronous Browser admission.
func (s *Store) CreateOwnedMediaRecaptureWithAdmission(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode, reason domain.MediaRecaptureReason, collector string, admission domain.MediaRecaptureCaptureAdmission) (domain.MediaRecapture, error) {
	return s.createOwnedMediaRecapture(ctx, timelineID, mode, reason, nil, &admission, collector)
}

func (s *Store) createOwnedMediaRecapture(ctx context.Context, timelineID string, mode domain.MediaRecaptureMode, reason domain.MediaRecaptureReason, runtime map[string]any, admission *domain.MediaRecaptureCaptureAdmission, collector ...string) (domain.MediaRecapture, error) {
	if mode != domain.MediaRecaptureBackground && mode != domain.MediaRecaptureForeground {
		return domain.MediaRecapture{}, errors.New("media recapture mode must be background or foreground")
	}
	if !reason.Valid() {
		return domain.MediaRecapture{}, errors.New("media recapture reason must be missing_media, playback_error, or unresolved_video")
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
	canonicalTargetURL, ok := domain.CanonicalSourceURL(source, targetURL)
	if !ok {
		return domain.MediaRecapture{}, errors.New("this item has no recapturable native post URL")
	}
	targetURL = canonicalTargetURL
	foregroundAuthorized := mode == domain.MediaRecaptureForeground
	var priorPayload map[string]any
	if foregroundAuthorized {
		priorPayload, err = s.requireUnavailableBackgroundRecapture(ctx, timelineID, reason)
		if err != nil {
			return domain.MediaRecapture{}, err
		}
	}
	failedPlaybackURL := ""
	switch reason {
	case domain.MediaRecaptureUnresolvedVideo:
		if !domain.SupportsPlaybackErrorRecapture(source) || !unresolvedVideo(block, source) {
			return domain.MediaRecapture{}, errors.New("this item does not have unresolved captured video")
		}
	case domain.MediaRecaptureMissingMedia:
		if len(block.Media) > 0 || stringValue(block.MediaRecovery, "outcome") != "unavailable" {
			return domain.MediaRecapture{}, errors.New("this item does not have unavailable captured media")
		}
	case domain.MediaRecapturePlaybackError:
		if !domain.SupportsPlaybackErrorRecapture(source) {
			return domain.MediaRecapture{}, errors.New("this source does not support playback-error recapture")
		}
		if foregroundAuthorized {
			failedPlaybackURL = stringValue(priorPayload, "failedPlaybackUrl")
		} else {
			failedPlaybackURL = inlinePlaybackURL(block, source, "")
		}
		if failedPlaybackURL == "" {
			return domain.MediaRecapture{}, errors.New("this item does not have a recapturable inline playback URL")
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
		"mode":                     "recapture_media",
		"reason":                   reason,
		"source":                   source,
		"sourceHydrationTimeoutMs": settings.SourceHydrationTimeout(source),
		"targetUrl":                targetURL,
		"targetEvidenceKey":        evidenceKey,
		"scrolls":                  0,
		"scrollFraction":           0.75,
		"scrollSettleMs":           100,
		"captureTimeoutMs":         30000,
		"pendingContentPolicy":     "detect_only",
		"sameTabMutationAllowed":   false,
		"pendingContentTimeoutMs":  500,
		"pendingContentSettleMs":   100,
		"sourceFreshnessPolicy":    "preserve_target",
		"captureVisibilityPolicy":  settings.CaptureVisibility,
		"foregroundAuthorized":     foregroundAuthorized,
		"captureLeaseId":           job.ID,
		"maxBlocksPerSnapshot":     5,
		"maxBlockCharacters":       4000,
		"qualityReportRequired":    true,
		"qualityRetryBudget":       1,
		"qualityRetrySettleMs":     settings.QualityRetrySettleMS,
		"openIfMissing":            true,
		"tabLifecycle": map[string]any{
			"ownership":            "managed",
			"openedTabDisposition": "close_after_capture",
		},
		"restoreScroll":        false,
		"browserAdapter":       "aku-bridge",
		"acquisitionRound":     1,
		"maxAcquisitionRounds": 1,
	}
	if failedPlaybackURL != "" {
		job.Payload["failedPlaybackUrl"] = failedPlaybackURL
	}
	if runtime != nil {
		job.Payload["captureRuntime"] = runtime
	}
	if len(collector) > 1 {
		return domain.MediaRecapture{}, errors.New("only one capture collector may own a recapture")
	}
	if len(collector) == 1 {
		job.Payload["captureCollector"] = map[string]any{"backend": collector[0], "version": 1}
		if _, err := CaptureCollector(job.Payload); err != nil {
			return domain.MediaRecapture{}, err
		}
	}
	if admission != nil {
		if runtime != nil || len(collector) != 1 || collector[0] != "bridge" ||
			admission.Policy != domain.MediaRecaptureAdmissionHybridHeadlessV1 || admission.Driver != "browser" || admission.Phase != "waiting" {
			return domain.MediaRecapture{}, errors.New("media recapture admission must start as an unstamped Browser/Bridge wait")
		}
		job.Payload["captureAdmission"] = *admission
	}
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,payload_json,created_at) VALUES(?,?,?,?,?,'queued',?,?)`, job.ID, job.TimelineID, job.Source, job.TargetURL, job.EvidenceKey, string(payload), job.CreatedAt)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return domain.MediaRecapture{}, errors.New("a recapture is already active for this item")
		}
		return domain.MediaRecapture{}, err
	}
	return job, nil
}

// MediaRecaptureAdmission reads the durable owner-admission fence. Unknown or
// malformed markers fail closed so they can never fall through to legacy
// claim behavior.
func MediaRecaptureAdmission(payload map[string]any) (domain.MediaRecaptureCaptureAdmission, bool, error) {
	raw, exists := payload["captureAdmission"]
	if !exists {
		return domain.MediaRecaptureCaptureAdmission{}, false, nil
	}
	var admission domain.MediaRecaptureCaptureAdmission
	switch value := raw.(type) {
	case domain.MediaRecaptureCaptureAdmission:
		admission = value
	case map[string]any:
		admission.Policy, _ = value["policy"].(string)
		admission.Driver, _ = value["driver"].(string)
		admission.Phase, _ = value["phase"].(string)
	default:
		return domain.MediaRecaptureCaptureAdmission{}, true, errors.New("media recapture admission marker is malformed")
	}
	if admission.Policy != domain.MediaRecaptureAdmissionHybridHeadlessV1 || admission.Driver != "browser" ||
		(admission.Phase != "waiting" && admission.Phase != "admitted") {
		return domain.MediaRecaptureCaptureAdmission{}, true, errors.New("media recapture admission marker is unsupported")
	}
	if admission.Phase == "waiting" {
		if _, stamped := payload["captureRuntime"]; stamped {
			return domain.MediaRecaptureCaptureAdmission{}, true, errors.New("waiting media recapture already has a runtime owner")
		}
	} else if _, stamped := payload["captureRuntime"]; !stamped {
		return domain.MediaRecaptureCaptureAdmission{}, true, errors.New("admitted media recapture has no runtime owner")
	}
	route, err := CaptureCollector(payload)
	if err != nil || route != "bridge" {
		return domain.MediaRecaptureCaptureAdmission{}, true, errors.New("hybrid Facebook media recapture must retain its Bridge collector")
	}
	return admission, true, nil
}

// BindMediaRecaptureBrowserAdmission atomically binds the immutable Browser
// owner and advances a waiting job. A concurrent Bridge claim can only observe
// either the waiting fence or the complete admitted stamp.
func (s *Store) BindMediaRecaptureBrowserAdmission(ctx context.Context, id string, runtime map[string]any) (bool, error) {
	epoch, epochOK := runtime["epoch"].(string)
	if runtime == nil || runtime["driver"] != "browser" || !epochOK || strings.TrimSpace(epoch) == "" || !positiveCaptureGeneration(runtime["generation"]) {
		return false, errors.New("Browser media recapture admission requires a complete runtime stamp")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	job, err := mediaRecaptureByID(ctx, tx, id)
	if err != nil {
		return false, err
	}
	if job.Status != "queued" || job.Source != domain.SourceFacebook {
		return false, nil
	}
	admission, exists, err := MediaRecaptureAdmission(job.Payload)
	if err != nil {
		return false, err
	}
	if !exists || admission.Phase != "waiting" {
		return false, nil
	}
	admission.Phase = "admitted"
	job.Payload["captureRuntime"] = runtime
	job.Payload["captureAdmission"] = admission
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return false, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE media_recaptures SET payload_json=? WHERE id=? AND status='queued'`, string(payload), id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if count != 1 {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func positiveCaptureGeneration(value any) bool {
	switch generation := value.(type) {
	case int:
		return generation > 0
	case int64:
		return generation > 0
	case float64:
		return generation > 0 && generation == float64(int64(generation))
	default:
		return false
	}
}

// SetMediaRecaptureCaptureCleanupState persists whether the Bridge surface
// cleanup for an admitted Browser job has been acknowledged. Terminal rows
// remain recoverable until the released state is durable.
func (s *Store) SetMediaRecaptureCaptureCleanupState(ctx context.Context, id, state string) error {
	if state != "pending" && state != "released" {
		return errors.New("media recapture cleanup state must be pending or released")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	job, err := mediaRecaptureByID(ctx, tx, id)
	if err != nil {
		return err
	}
	if job.Status != "completed" && job.Status != "failed" && job.Status != "cancelled" {
		return errors.New("media recapture cleanup can be recorded only after terminal status")
	}
	if admission, exists, err := MediaRecaptureAdmission(job.Payload); err != nil {
		return err
	} else if !exists || admission.Phase != "admitted" {
		return errors.New("media recapture has no admitted Browser owner")
	}
	// A cleanup acknowledgement is final. A pump cycle can have read a stale
	// pending row before the acknowledgement commits, so guard the write below
	// as well as treating an already-released row as an idempotent no-op.
	if state == "pending" && job.Payload["captureCleanup"] == "released" {
		return tx.Commit()
	}
	job.Payload["captureCleanup"] = state
	payload, err := json.Marshal(job.Payload)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE media_recaptures SET payload_json=?
		WHERE id=? AND status IN ('completed','failed','cancelled')
		AND (?='released' OR COALESCE(json_extract(payload_json,'$.captureCleanup'),'')!='released')`, string(payload), id, state)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		if state == "pending" {
			var cleanup sql.NullString
			readErr := tx.QueryRowContext(ctx, `SELECT json_extract(payload_json,'$.captureCleanup')
				FROM media_recaptures WHERE id=? AND status IN ('completed','failed','cancelled')`, id).Scan(&cleanup)
			if readErr == nil && cleanup.Valid && cleanup.String == "released" {
				return tx.Commit()
			}
		}
		return errors.New("media recapture cleanup state could not be saved")
	}
	return tx.Commit()
}

// ApplyPassiveXMediaEvidence persists media evidence that AkuBridge already
// observed without scheduling a browser action. The completed media_recaptures
// row is provenance only; it is never claimable and cannot authorize a
// foreground capture.
func (s *Store) ApplyPassiveXMediaEvidence(ctx context.Context, timelineID, bridgeID string, input domain.PassiveXMediaEvidence) (domain.MediaRecapture, bool, error) {
	if strings.TrimSpace(input.Provenance) != "passive_x_cache" {
		return domain.MediaRecapture{}, false, errors.New("passive media provenance must be passive_x_cache")
	}
	candidateID := normalizeXCandidateID(input.CandidateID)
	if candidateID == "" || candidateID != strings.TrimSpace(input.CandidateID) {
		return domain.MediaRecapture{}, false, errors.New("candidateId must use x:status:<5-30 digits>")
	}
	media, err := normalizePassiveXMedia(input.Media)
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}

	var source domain.Source
	var evidenceKey, itemRaw string
	if err := s.db.QueryRowContext(ctx, `SELECT source,evidence_key,item_json FROM timeline_items WHERE id=?`, timelineID).Scan(&source, &evidenceKey, &itemRaw); err != nil {
		return domain.MediaRecapture{}, false, err
	}
	if source != domain.SourceX {
		return domain.MediaRecapture{}, false, errors.New("passive media enrichment is available only for X items")
	}
	block, err := s.timelineEvidence(ctx, timelineID, evidenceKey)
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}
	var item domain.ReasonedItem
	decodeJSON(itemRaw, &item)
	expectedID, err := authoritativeXCandidateID(block, evidenceKey, item.SourceURL)
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}
	if candidateID != expectedID {
		return domain.MediaRecapture{}, false, errors.New("candidateId does not match the timeline item's X platform identity")
	}

	merged, updated := mergePassiveMedia(block.Media, media)
	if !updated {
		return domain.MediaRecapture{}, false, nil
	}
	block.Media = merged
	mediaRecovery := mergeAnyValues(block.MediaRecovery, map[string]any{
		"outcome":              "recovered",
		"recoveredCount":       len(merged),
		"method":               "passive_cache",
		"acquisitionStage":     "async_evidence_cache",
		"engineVersion":        passiveXMediaEngineVersion,
		"foregroundRequired":   false,
		"foregroundAuthorized": false,
		"trace":                []string{"passive_cache_match", "sanitized_media_persisted"},
	})
	delete(mediaRecovery, "limitation")
	delete(mediaRecovery, "visibilityRequirement")
	block.MediaRecovery = mediaRecovery

	targetURL := strings.TrimSpace(block.Permalink)
	if targetURL == "" {
		targetURL = strings.TrimSpace(item.SourceURL)
	}
	if !nativeSourceURL(domain.SourceX, targetURL) || normalizeXCandidateID(targetURL) != candidateID {
		targetURL = "https://x.com/i/status/" + strings.TrimPrefix(candidateID, "x:status:")
	}
	now := domain.Now()
	job := domain.MediaRecapture{
		ID:          domain.NewID("recapture"),
		TimelineID:  timelineID,
		Source:      source,
		TargetURL:   targetURL,
		EvidenceKey: evidenceKey,
		Status:      "completed",
		Outcome:     "recovered",
		CreatedAt:   now,
		CompletedAt: &now,
		Payload: map[string]any{
			"mode":                    "passive_media_enrichment",
			"source":                  source,
			"candidateId":             candidateID,
			"targetEvidenceKey":       evidenceKey,
			"provenance":              "passive_x_cache",
			"browserOperation":        "none",
			"foregroundAuthorized":    false,
			"captureVisibilityPolicy": "none",
			"maxMedia":                4,
			"engineVersion":           passiveXMediaEngineVersion,
		},
	}
	payloadRaw, err := json.Marshal(job.Payload)
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}
	evidenceRaw, err := json.Marshal(block)
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}
	resultRaw, err := json.Marshal(map[string]any{
		"candidateId": candidateID,
		"media":       media,
		"mediaCount":  len(media),
		"provenance":  "passive_x_cache",
	})
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}
	claimedBy := strings.TrimSpace(bridgeID)
	if claimedBy == "" {
		claimedBy = "aku-bridge"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MediaRecapture{}, false, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,outcome,payload_json,result_json,claimed_by,created_at,claimed_at,completed_at) VALUES(?,?,?,?,?,'completed','recovered',?,?,?,?,?,?)`, job.ID, job.TimelineID, job.Source, job.TargetURL, job.EvidenceKey, string(payloadRaw), string(resultRaw), claimedBy, now, now, now); err != nil {
		return domain.MediaRecapture{}, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO timeline_evidence_overrides(timeline_id,recapture_id,evidence_json,updated_at) VALUES(?,?,?,?) ON CONFLICT(timeline_id) DO UPDATE SET recapture_id=excluded.recapture_id,evidence_json=excluded.evidence_json,updated_at=excluded.updated_at`, job.TimelineID, job.ID, string(evidenceRaw), now); err != nil {
		return domain.MediaRecapture{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MediaRecapture{}, false, err
	}
	return job, true, nil
}

func (s *Store) requireUnavailableBackgroundRecapture(ctx context.Context, timelineID string, reason domain.MediaRecaptureReason) (map[string]any, error) {
	var outcome, payloadRaw string
	err := s.db.QueryRowContext(ctx, `SELECT outcome,payload_json FROM media_recaptures WHERE timeline_id=? AND status='completed' ORDER BY completed_at DESC LIMIT 1`, timelineID).Scan(&outcome, &payloadRaw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("foreground recapture requires a completed unavailable background attempt")
		}
		return nil, err
	}
	var payload map[string]any
	decodeJSON(payloadRaw, &payload)
	priorForeground, _ := payload["foregroundAuthorized"].(bool)
	priorReason := mediaRecaptureReason(payload)
	if outcome != "unavailable" || priorForeground || priorReason != reason {
		return nil, errors.New("foreground recapture requires the latest same-reason attempt to be unavailable in the background")
	}
	return payload, nil
}

func (s *Store) ClaimMediaRecapture(ctx context.Context, id, bridgeID string) (domain.MediaRecapture, error) {
	return s.ClaimMediaRecaptureForCollector(ctx, id, bridgeID, "")
}

func (s *Store) ClaimMediaRecaptureForCollector(ctx context.Context, id, bridgeID, collector string) (domain.MediaRecapture, error) {
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
	if admission, exists, err := MediaRecaptureAdmission(job.Payload); err != nil {
		return domain.MediaRecapture{}, err
	} else if exists && admission.Phase == "waiting" {
		return domain.MediaRecapture{}, nil
	}
	if collector != "" {
		route, err := CaptureCollector(job.Payload)
		if err != nil {
			return domain.MediaRecapture{}, err
		}
		if route != collector {
			return domain.MediaRecapture{}, nil
		}
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
	return s.completeMediaRecapture(ctx, id, observation, false)
}

// CompleteHeadlessMediaRecapture is reserved for the internal owned collector.
// Bridge-submitted coverage cannot authorize a photo-to-parent transition.
func (s *Store) CompleteHeadlessMediaRecapture(ctx context.Context, id string, observation domain.Observation) (domain.MediaRecapture, error) {
	return s.completeMediaRecapture(ctx, id, observation, true)
}

func (s *Store) completeMediaRecapture(ctx context.Context, id string, observation domain.Observation, internalHeadless bool) (domain.MediaRecapture, error) {
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
	// Persisted relation authority is issued here, never accepted from caller maps.
	for si := range observation.Snapshots {
		for bi := range observation.Snapshots[si].Blocks {
			b := &observation.Snapshots[si].Blocks[bi]
			b.MediaRecovery = mergeAnyValues(b.MediaRecovery, nil)
			delete(b.MediaRecovery, "photoParentResolution")
		}
	}
	block, ok := recapturedBlock(observation, job)
	if _, mediaOnly := observation.Coverage["photoMediaRecapture"]; mediaOnly {
		if !internalHeadless {
			return domain.MediaRecapture{}, errors.New("photo media evidence requires the internal headless collector")
		}
		original, loadErr := timelineEvidenceFrom(ctx, tx, job.TimelineID, job.EvidenceKey)
		if loadErr != nil {
			return domain.MediaRecapture{}, loadErr
		}
		block, ok = recapturedPhotoMedia(observation, job, original)
		if !ok {
			return domain.MediaRecapture{}, errors.New("photo media evidence does not match the saved photo")
		}
	}
	if !ok && internalHeadless {
		original, loadErr := timelineEvidenceFrom(ctx, tx, job.TimelineID, job.EvidenceKey)
		if loadErr != nil {
			return domain.MediaRecapture{}, loadErr
		}
		block, ok = recapturedPhotoParent(observation, job, original)
	}
	if !ok {
		return domain.MediaRecapture{}, errors.New("recapture did not return the requested native post")
	}
	if job.Source == domain.SourceFacebook && block.MediaRecovery["photoParentResolution"] == nil {
		// A later direct recapture must not erase an already verified relation.
		original, loadErr := timelineEvidenceFrom(ctx, tx, job.TimelineID, job.EvidenceKey)
		if loadErr != nil {
			return domain.MediaRecapture{}, loadErr
		}
		if proof, valid := original.MediaRecovery["photoParentResolution"].(map[string]any); valid &&
			proof["provenance"] == "internal_headless_and_saved_photo_evidence" && proof["status"] == "verified" &&
			proof["parentPlatformId"] == block.PlatformID && original.PlatformID == block.PlatformID && original.Permalink == block.Permalink &&
			strings.Join(strings.Fields(original.Author), " ") == strings.Join(strings.Fields(block.Author), " ") {
			block.MediaRecovery = mergeAnyValues(block.MediaRecovery, map[string]any{"photoParentResolution": proof})
			block.EvidenceKey = job.EvidenceKey
		}
	}
	now := domain.Now()
	outcome := "unavailable"
	reason := mediaRecaptureReason(job.Payload)
	if reason == domain.MediaRecaptureUnresolvedVideo {
		canonical, valid := domain.CanonicalSourceURL(job.Source, block.Permalink)
		if !valid || canonical != job.TargetURL {
			return domain.MediaRecapture{}, errors.New("video recapture did not return the requested native post URL")
		}
	}
	if reason == domain.MediaRecapturePlaybackError || reason == domain.MediaRecaptureUnresolvedVideo {
		original, loadErr := timelineEvidenceFrom(ctx, tx, job.TimelineID, job.EvidenceKey)
		if loadErr != nil {
			return domain.MediaRecapture{}, loadErr
		}
		failedPlaybackURL := stringValue(job.Payload, "failedPlaybackUrl")
		if inlinePlaybackURL(block, job.Source, failedPlaybackURL) != "" {
			outcome = "recovered"
		}
		recoveryMode := "background"
		if job.Payload["foregroundAuthorized"] == true {
			recoveryMode = "foreground"
		}
		mediaRecovery := mergeAnyValues(block.MediaRecovery, map[string]any{
			"outcome":                     outcome,
			"reason":                      string(reason),
			"method":                      "native_post_recapture",
			"acquisitionStage":            "playback_error_recapture",
			"foregroundAuthorized":        job.Payload["foregroundAuthorized"],
			"foregroundRequired":          outcome == "unavailable" && job.Payload["foregroundAuthorized"] != true,
			"playbackRecoveryRequestedAt": job.CreatedAt,
			"playbackRecoveryCompletedAt": now,
			"playbackReplacementChanged":  outcome == "recovered",
			"playbackRecoveryMode":        recoveryMode,
		})
		if reason == domain.MediaRecaptureUnresolvedVideo {
			mediaRecovery["acquisitionStage"] = "unresolved_video_recapture"
			if outcome == "unavailable" {
				mediaRecovery["expected"] = videoExpectation(original)
				if len(block.Media) == 0 {
					block.Media = original.Media
				}
				block.ContentKind = original.ContentKind
			}
		}
		if outcome == "recovered" || job.Payload["foregroundAuthorized"] == true {
			delete(mediaRecovery, "visibilityRequirement")
			delete(mediaRecovery, "limitation")
		} else {
			mediaRecovery["visibilityRequirement"] = "foreground_window"
		}
		block.MediaRecovery = mediaRecovery
		if outcome == "unavailable" {
			block.Media = withoutInlinePlayback(block.Media)
		}
	} else if len(block.Media) > 0 {
		outcome = "recovered"
	} else if value := stringValue(block.MediaRecovery, "outcome"); value != "" {
		outcome = value
	}
	evidenceRaw, _ := json.Marshal(block)
	resultRaw, _ := json.Marshal(observation)
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
	job.Payload, err = decodeCapturePayload(payloadRaw)
	if err != nil {
		return domain.MediaRecapture{}, err
	}
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
	return timelineEvidenceFrom(ctx, s.db, timelineID, evidenceKey)
}

type evidenceQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func timelineEvidenceFrom(ctx context.Context, queryer evidenceQueryer, timelineID, evidenceKey string) (domain.Block, error) {
	var overrideRaw string
	err := queryer.QueryRowContext(ctx, `SELECT evidence_json FROM timeline_evidence_overrides WHERE timeline_id=?`, timelineID).Scan(&overrideRaw)
	if err == nil {
		var block domain.Block
		decodeJSON(overrideRaw, &block)
		return block, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return domain.Block{}, err
	}
	rows, err := queryer.QueryContext(ctx, `SELECT o.observation_json FROM timeline_items t JOIN observations o ON o.run_id=t.run_id WHERE t.id=? ORDER BY o.created_at`, timelineID)
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
	_, ok := domain.CanonicalSourceURL(source, raw)
	return ok
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

func mediaRecaptureReason(payload map[string]any) domain.MediaRecaptureReason {
	reason := domain.MediaRecaptureReason(stringValue(payload, "reason"))
	if !reason.Valid() {
		return domain.MediaRecaptureMissingMedia
	}
	return reason
}

func inlinePlaybackURL(block domain.Block, source domain.Source, excluded string) string {
	for _, media := range block.Media {
		kind, _ := media["kind"].(string)
		mode, _ := media["playbackMode"].(string)
		raw, _ := media["playbackUrl"].(string)
		canonical, ok := domain.CanonicalInlinePlaybackURL(source, raw)
		if kind == "video" && mode == "inline" && ok && canonical != excluded {
			return canonical
		}
	}
	return ""
}

func videoExpectation(block domain.Block) []string {
	expected := []string{"video"}
	add := func(kind any) {
		if kind == "image" && len(expected) == 1 {
			expected = append(expected, "image")
		}
	}
	switch kinds := block.MediaRecovery["expected"].(type) {
	case []any:
		for _, kind := range kinds {
			add(kind)
		}
	case []string:
		for _, kind := range kinds {
			add(kind)
		}
	}
	return expected
}

// A poster does not satisfy captured video playback. Admit only explicit video
// evidence, and reject posts that already have a source-trusted inline URL.
func unresolvedVideo(block domain.Block, source domain.Source) bool {
	if inlinePlaybackURL(block, source, "") != "" {
		return false
	}
	if block.ContentKind == "video" {
		return true
	}
	for _, media := range block.Media {
		if media["kind"] == "video" || media["kind"] == "video_poster" {
			return true
		}
	}
	switch expected := block.MediaRecovery["expected"].(type) {
	case []any:
		for _, kind := range expected {
			if kind == "video" {
				return true
			}
		}
	case []string:
		for _, kind := range expected {
			if kind == "video" {
				return true
			}
		}
	}
	return false
}

func withoutInlinePlayback(values []map[string]any) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, media := range values {
		copyValue := make(map[string]any, len(media))
		for key, value := range media {
			copyValue[key] = value
		}
		delete(copyValue, "playbackUrl")
		if copyValue["playbackMode"] == "inline" {
			copyValue["playbackMode"] = "native"
		}
		result = append(result, copyValue)
	}
	return result
}

func normalizeXCandidateID(value string) string {
	trimmed := strings.TrimSpace(value)
	if match := xCandidateIdentity.FindStringSubmatch(trimmed); len(match) == 2 {
		return "x:status:" + match[1]
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme != "https" || strings.ToLower(parsed.Hostname()) != "x.com" {
		return ""
	}
	match := xStatusPath.FindStringSubmatch(parsed.Path)
	if len(match) != 2 {
		return ""
	}
	return "x:status:" + match[1]
}

func authoritativeXCandidateID(block domain.Block, evidenceKey, sourceURL string) (string, error) {
	identities := map[string]bool{}
	if platformID := strings.TrimSpace(block.PlatformID); xNumericIdentity.MatchString(platformID) {
		identities["x:status:"+platformID] = true
	}
	for _, value := range []string{block.PlatformID, evidenceKey, block.Permalink, sourceURL} {
		if identity := normalizeXCandidateID(value); identity != "" {
			identities[identity] = true
		}
	}
	if len(identities) == 0 {
		return "", errors.New("timeline item has no authoritative X platform identity")
	}
	if len(identities) != 1 {
		return "", errors.New("timeline item contains conflicting X platform identities")
	}
	for identity := range identities {
		return identity, nil
	}
	return "", errors.New("timeline item has no authoritative X platform identity")
}

func normalizePassiveXMedia(values []domain.PassiveXMediaCandidate) ([]map[string]any, error) {
	if len(values) == 0 || len(values) > 4 {
		return nil, errors.New("media must contain between 1 and 4 candidates")
	}
	result := make([]map[string]any, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if value.Kind != "image" && value.Kind != "video" {
			return nil, errors.New("media kind must be image or video")
		}
		if value.Width < 0 || value.Width > 8192 || value.Height < 0 || value.Height > 8192 {
			return nil, errors.New("media dimensions must be between 0 and 8192")
		}
		if value.ObservedAtMS < 0 {
			return nil, errors.New("media observedAtMs cannot be negative")
		}
		if value.PlaybackMode != "" && value.PlaybackMode != "inline" && value.PlaybackMode != "native" {
			return nil, errors.New("media playbackMode must be inline or native")
		}
		if value.Provenance != "" && value.Provenance != "observed_dom" && value.Provenance != "main_structured_state" && value.Provenance != "x_response_graphql" && value.Provenance != "passive_x_cache" {
			return nil, errors.New("media provenance is unsupported")
		}
		primary, primaryHost, err := normalizePassiveXMediaURL(value.URL)
		if err != nil {
			return nil, fmt.Errorf("media URL is invalid: %w", err)
		}
		if value.Kind == "image" && primaryHost != "pbs.twimg.com" {
			return nil, errors.New("image media URL must use pbs.twimg.com")
		}
		poster := ""
		if value.PosterURL != "" {
			poster, primaryHost, err = normalizePassiveXMediaURL(value.PosterURL)
			if err != nil || primaryHost != "pbs.twimg.com" {
				return nil, errors.New("media posterUrl must use an allowlisted pbs.twimg.com path")
			}
		}
		playback := ""
		if value.PlaybackURL != "" {
			playback, primaryHost, err = normalizePassiveXMediaURL(value.PlaybackURL)
			if err != nil || primaryHost != "video.twimg.com" {
				return nil, errors.New("media playbackUrl must use an allowlisted video.twimg.com path")
			}
		}
		if value.Kind == "image" && (poster != "" || playback != "" || value.PlaybackMode != "") {
			return nil, errors.New("image media cannot contain video playback fields")
		}
		primaryURLHost := ""
		if parsedPrimary, parseErr := url.Parse(primary); parseErr == nil {
			primaryURLHost = strings.ToLower(parsedPrimary.Hostname())
		}
		if value.Kind == "video" && primaryURLHost == "video.twimg.com" && poster == "" {
			return nil, errors.New("video media requires an allowlisted poster image")
		}
		identity := value.Kind + "|" + primary + "|" + playback
		if seen[identity] {
			continue
		}
		seen[identity] = true
		candidate := map[string]any{
			"kind":   value.Kind,
			"url":    primary,
			"width":  value.Width,
			"height": value.Height,
		}
		if poster != "" {
			candidate["posterUrl"] = poster
		}
		if playback != "" {
			candidate["playbackUrl"] = playback
		}
		if value.PlaybackMode != "" {
			candidate["playbackMode"] = value.PlaybackMode
		}
		if value.Provenance != "" {
			candidate["provenance"] = value.Provenance
		}
		if value.ObservedAtMS > 0 {
			candidate["observedAtMs"] = value.ObservedAtMS
		}
		result = append(result, candidate)
	}
	if len(result) == 0 {
		return nil, errors.New("media contains no unique candidates")
	}
	return result, nil
}

func normalizePassiveXMediaURL(raw string) (string, string, error) {
	if len(raw) == 0 || len(raw) > 2048 {
		return "", "", errors.New("URL length is out of bounds")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" {
		return "", "", errors.New("URL must be credential-free HTTPS on the default port")
	}
	host := strings.ToLower(parsed.Hostname())
	allowed := false
	switch host {
	case "pbs.twimg.com":
		for _, prefix := range []string{"/media/", "/card_img/", "/ext_tw_video_thumb/", "/amplify_video_thumb/", "/tweet_video_thumb/", "/semantic_core_img/"} {
			if strings.HasPrefix(parsed.Path, prefix) {
				allowed = true
				break
			}
		}
	case "video.twimg.com":
		for _, prefix := range []string{"/amplify_video/", "/ext_tw_video/", "/tweet_video/"} {
			if strings.HasPrefix(parsed.Path, prefix) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return "", "", errors.New("URL host or path is not allowlisted X post media")
	}
	parsed.Host = host
	parsed.Fragment = ""
	return parsed.String(), host, nil
}

func mergePassiveMedia(existing, incoming []map[string]any) ([]map[string]any, bool) {
	result := make([]map[string]any, 0, 4)
	seen := map[string]bool{}
	for _, value := range existing {
		if len(result) >= 4 {
			break
		}
		result = append(result, value)
		if identity := passiveMediaIdentity(value); identity != "" {
			seen[identity] = true
		}
	}
	updated := false
	for _, value := range incoming {
		if len(result) >= 4 {
			break
		}
		identity := passiveMediaIdentity(value)
		if identity == "" || seen[identity] {
			continue
		}
		seen[identity] = true
		result = append(result, value)
		updated = true
	}
	return result, updated
}

func passiveMediaIdentity(value map[string]any) string {
	kind, _ := value["kind"].(string)
	primary, _ := value["url"].(string)
	playback, _ := value["playbackUrl"].(string)
	if primary == "" {
		return ""
	}
	return kind + "|" + primary + "|" + playback
}

func mergeAnyValues(previous, current map[string]any) map[string]any {
	result := make(map[string]any, len(previous)+len(current))
	for key, value := range previous {
		result[key] = value
	}
	for key, value := range current {
		result[key] = value
	}
	return result
}
