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
