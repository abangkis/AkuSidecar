package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/capture"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

const contentIdentityVersion = "content-identity-v1"
const contentIdentityBackfillKey = "content_identity_backfill_v1"
const contentIdentityFallbackWindow = 30 * time.Minute

type contentIdentityAlias struct {
	canonicalEvidenceKey string
	nativeIdentity       contentNativeIdentity
	metadata             contentIdentityMetadata
	firstSeenAt          string
	ambiguous            bool
}

type contentIdentityMetadata struct {
	contentKind string
	publishedAt string
}

type contentNativeIdentity struct {
	platformID string
	permalink  string
	conflicted bool
}

type contentIdentitySummary struct {
	Version            string `json:"version"`
	NativePresent      int    `json:"nativePresent"`
	NativeMissing      int    `json:"nativeMissing"`
	AliasesReused      int    `json:"aliasesReused"`
	FallbacksPromoted  int    `json:"fallbacksPromoted"`
	NativeConflicts    int    `json:"nativeConflicts"`
	AmbiguousFallbacks int    `json:"ambiguousFallbacks"`
}

type identityBlockRef struct {
	snapshot int
	block    int
}

func (s *Store) resolveObservationContentIdentity(
	ctx context.Context,
	tx *sql.Tx,
	runID string,
	observation *domain.Observation,
	observedAt string,
) (contentIdentitySummary, error) {
	summary := contentIdentitySummary{Version: contentIdentityVersion}
	observation.Snapshots = capture.ReconcileSnapshots(observation.Source, observation.Snapshots)

	seenEvidence := map[string]bool{}
	groups := map[string][]identityBlockRef{}
	for snapshotIndex := range observation.Snapshots {
		for blockIndex := range observation.Snapshots[snapshotIndex].Blocks {
			block := &observation.Snapshots[snapshotIndex].Blocks[blockIndex]
			evidenceKey := strings.TrimSpace(block.EvidenceKey)
			if evidenceKey != "" && !seenEvidence[evidenceKey] {
				seenEvidence[evidenceKey] = true
				if stableNativeIdentity(observation.Source, *block).empty() {
					summary.NativeMissing++
				} else {
					summary.NativePresent++
				}
			}
			signature := capture.ContentSignature(observation.Source, *block)
			if signature == "" {
				continue
			}
			fingerprint := contentIdentityFingerprint(signature)
			groups[fingerprint] = append(groups[fingerprint], identityBlockRef{snapshot: snapshotIndex, block: blockIndex})
		}
	}

	for fingerprint, refs := range groups {
		currentIdentity := contentNativeIdentity{}
		currentMetadata := contentIdentityMetadata{}
		currentIdentityAmbiguous := false
		currentMetadataAmbiguous := false
		currentFallbackPresent := false
		currentEvidenceKey := ""
		for _, ref := range refs {
			block := observation.Snapshots[ref.snapshot].Blocks[ref.block]
			if currentEvidenceKey == "" {
				currentEvidenceKey = strings.TrimSpace(block.EvidenceKey)
			}
			var metadataConflict bool
			currentMetadata, metadataConflict = mergeContentIdentityMetadata(
				currentMetadata,
				contentIdentityMetadataFromBlock(block),
			)
			currentMetadataAmbiguous = currentMetadataAmbiguous || metadataConflict
			blockIdentity := stableNativeIdentity(observation.Source, block)
			if blockIdentity.empty() {
				currentFallbackPresent = true
				continue
			}
			switch nativeIdentityRelation(currentIdentity, blockIdentity) {
			case "conflict", "incomparable":
				currentIdentityAmbiguous = true
			default:
				currentIdentity = mergeNativeIdentity(currentIdentity, blockIdentity)
			}
		}
		if currentEvidenceKey == "" {
			continue
		}
		if currentIdentityAmbiguous ||
			(currentMetadataAmbiguous && (currentFallbackPresent || currentIdentity.empty())) {
			if currentIdentityAmbiguous {
				summary.NativeConflicts++
			} else {
				summary.AmbiguousFallbacks++
			}
			if err := markContentIdentityAmbiguous(ctx, tx, observation.Source, fingerprint, currentEvidenceKey, currentIdentity, currentMetadata, observedAt, runID); err != nil {
				return contentIdentitySummary{}, err
			}
			continue
		}

		alias, found, err := lookupContentIdentityAlias(ctx, tx, observation.Source, fingerprint)
		if err != nil {
			return contentIdentitySummary{}, err
		}
		if !found {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO content_identity_aliases(
				  source,identity_fingerprint,canonical_evidence_key,canonical_platform_id,
				  canonical_permalink,canonical_content_kind,canonical_published_at,
				  ambiguous,first_seen_at,last_seen_at,last_run_id,seen_count
				) VALUES(?,?,?,?,?,?,?,0,?,?,?,1)`,
				observation.Source, fingerprint, currentEvidenceKey,
				currentIdentity.platformID, currentIdentity.permalink,
				currentMetadata.contentKind, currentMetadata.publishedAt,
				observedAt, observedAt, runID); err != nil {
				return contentIdentitySummary{}, err
			}
			continue
		}

		if alias.ambiguous {
			switch nativeIdentityRelation(alias.nativeIdentity, currentIdentity) {
			case "empty":
				summary.AmbiguousFallbacks++
			case "conflict", "incomparable":
				summary.NativeConflicts++
			default:
				if rewriteIdentityEvidenceKeys(observation, refs, alias.canonicalEvidenceKey) {
					summary.AliasesReused++
				}
			}
			if err := touchContentIdentityAlias(ctx, tx, observation.Source, fingerprint, observedAt, runID); err != nil {
				return contentIdentitySummary{}, err
			}
			continue
		}

		relation := nativeIdentityRelation(alias.nativeIdentity, currentIdentity)
		if relation == "conflict" || relation == "incomparable" {
			summary.NativeConflicts++
			if err := markContentIdentityAmbiguous(ctx, tx, observation.Source, fingerprint, currentEvidenceKey, mergeNativeIdentity(alias.nativeIdentity, currentIdentity), currentMetadata, observedAt, runID); err != nil {
				return contentIdentitySummary{}, err
			}
			continue
		}
		if relation == "empty" && !contentIdentityMetadataCompatible(alias.metadata, currentMetadata, alias.firstSeenAt, observedAt) {
			summary.AmbiguousFallbacks++
			if err := markContentIdentityAmbiguous(ctx, tx, observation.Source, fingerprint, alias.canonicalEvidenceKey, alias.nativeIdentity, alias.metadata, observedAt, runID); err != nil {
				return contentIdentitySummary{}, err
			}
			continue
		}
		if alias.nativeIdentity.empty() && !currentIdentity.empty() {
			summary.FallbacksPromoted++
		}
		alias.nativeIdentity = mergeNativeIdentity(alias.nativeIdentity, currentIdentity)
		alias.metadata, _ = mergeContentIdentityMetadata(alias.metadata, currentMetadata)
		if rewriteIdentityEvidenceKeys(observation, refs, alias.canonicalEvidenceKey) {
			summary.AliasesReused++
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE content_identity_aliases
			SET canonical_platform_id=?,canonical_permalink=?,canonical_content_kind=?,
			    canonical_published_at=?,last_seen_at=?,last_run_id=?,seen_count=seen_count+1
			WHERE source=? AND identity_fingerprint=?`,
			alias.nativeIdentity.platformID, alias.nativeIdentity.permalink,
			alias.metadata.contentKind, alias.metadata.publishedAt,
			observedAt, runID, observation.Source, fingerprint); err != nil {
			return contentIdentitySummary{}, err
		}
	}
	return summary, nil
}

func lookupContentIdentityAlias(
	ctx context.Context,
	tx *sql.Tx,
	source domain.Source,
	fingerprint string,
) (contentIdentityAlias, bool, error) {
	var alias contentIdentityAlias
	var ambiguous int
	err := tx.QueryRowContext(ctx, `
		SELECT canonical_evidence_key,canonical_platform_id,canonical_permalink,
		       canonical_content_kind,canonical_published_at,first_seen_at,ambiguous
		FROM content_identity_aliases
		WHERE source=? AND identity_fingerprint=?`, source, fingerprint).
		Scan(
			&alias.canonicalEvidenceKey,
			&alias.nativeIdentity.platformID,
			&alias.nativeIdentity.permalink,
			&alias.metadata.contentKind,
			&alias.metadata.publishedAt,
			&alias.firstSeenAt,
			&ambiguous,
		)
	if err == sql.ErrNoRows {
		return contentIdentityAlias{}, false, nil
	}
	if err != nil {
		return contentIdentityAlias{}, false, err
	}
	alias.ambiguous = ambiguous != 0
	return alias, true, nil
}

func rewriteIdentityEvidenceKeys(observation *domain.Observation, refs []identityBlockRef, canonical string) bool {
	changed := false
	for _, ref := range refs {
		block := &observation.Snapshots[ref.snapshot].Blocks[ref.block]
		if block.EvidenceKey != canonical {
			block.EvidenceKey = canonical
			changed = true
		}
	}
	return changed
}

func markContentIdentityAmbiguous(
	ctx context.Context,
	tx *sql.Tx,
	source domain.Source,
	fingerprint, canonicalEvidenceKey string,
	nativeIdentity contentNativeIdentity,
	metadata contentIdentityMetadata,
	observedAt, runID string,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO content_identity_aliases(
		  source,identity_fingerprint,canonical_evidence_key,canonical_platform_id,
		  canonical_permalink,canonical_content_kind,canonical_published_at,
		  ambiguous,first_seen_at,last_seen_at,last_run_id,seen_count
		) VALUES(?,?,?,?,?,?,?,1,?,?,?,1)
		ON CONFLICT(source,identity_fingerprint) DO UPDATE SET
		  ambiguous=1,last_seen_at=excluded.last_seen_at,last_run_id=excluded.last_run_id,
		  seen_count=content_identity_aliases.seen_count+1`,
		source, fingerprint, canonicalEvidenceKey, nativeIdentity.platformID, nativeIdentity.permalink,
		metadata.contentKind, metadata.publishedAt,
		observedAt, observedAt, runID)
	return err
}

func touchContentIdentityAlias(
	ctx context.Context,
	tx *sql.Tx,
	source domain.Source,
	fingerprint, observedAt, runID string,
) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE content_identity_aliases
		SET last_seen_at=?,last_run_id=?,seen_count=seen_count+1
		WHERE source=? AND identity_fingerprint=?`,
		observedAt, runID, source, fingerprint)
	return err
}

func stableNativeIdentity(source domain.Source, block domain.Block) contentNativeIdentity {
	result := contentNativeIdentity{
		platformID: domain.NormalizeNativeIdentity(source, block.PlatformID),
	}
	if permalink, ok := domain.CanonicalSourceURL(source, block.Permalink); ok {
		result.permalink = permalink
		if derived := domain.NativeIdentityFromPermalink(source, permalink); derived != "" {
			if result.platformID != "" && result.platformID != derived {
				result.conflicted = true
			} else {
				result.platformID = derived
			}
		}
	}
	return result
}

func (value contentNativeIdentity) empty() bool {
	return value.platformID == "" && value.permalink == ""
}

func nativeIdentityRelation(previous, current contentNativeIdentity) string {
	if previous.conflicted || current.conflicted {
		return "conflict"
	}
	if previous.empty() || current.empty() {
		return "empty"
	}
	if previous.platformID != "" && current.platformID != "" {
		if previous.platformID != current.platformID {
			return "conflict"
		}
		// A source-proven platform identity outranks harmless permalink
		// spelling differences such as trailing slashes or tracking queries.
		return "match"
	}
	if previous.permalink != "" && current.permalink != "" {
		if previous.permalink != current.permalink {
			return "conflict"
		}
		return "match"
	}
	return "incomparable"
}

func mergeNativeIdentity(previous, current contentNativeIdentity) contentNativeIdentity {
	result := previous
	result.conflicted = result.conflicted || current.conflicted
	if result.platformID == "" {
		result.platformID = current.platformID
	}
	if result.permalink == "" {
		result.permalink = current.permalink
	}
	return result
}

func contentIdentityMetadataFromBlock(block domain.Block) contentIdentityMetadata {
	result := contentIdentityMetadata{
		contentKind: strings.ToLower(strings.TrimSpace(block.ContentKind)),
	}
	estimated, _ := block.Presentation["timestampEstimated"].(bool)
	if block.PublishedAt != nil && !estimated {
		result.publishedAt = normalizeContentIdentityTimestamp(*block.PublishedAt)
	}
	return result
}

func normalizeContentIdentityTimestamp(value string) string {
	value = strings.TrimSpace(value)
	if parsed, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return parsed.UTC().Format(time.RFC3339Nano)
	}
	return value
}

func mergeContentIdentityMetadata(previous, current contentIdentityMetadata) (contentIdentityMetadata, bool) {
	result := previous
	conflict := false
	if result.contentKind == "" {
		result.contentKind = current.contentKind
	} else if current.contentKind != "" && result.contentKind != current.contentKind {
		conflict = true
	}
	if result.publishedAt == "" {
		result.publishedAt = current.publishedAt
	} else if current.publishedAt != "" && result.publishedAt != current.publishedAt {
		conflict = true
	}
	return result, conflict
}

func contentIdentityMetadataCompatible(previous, current contentIdentityMetadata, firstSeenAt, observedAt string) bool {
	if previous.contentKind != "" && current.contentKind != "" && previous.contentKind != current.contentKind {
		return false
	}
	if previous.publishedAt != "" && current.publishedAt != "" {
		return previous.publishedAt == current.publishedAt
	}
	firstSeen, firstErr := time.Parse(time.RFC3339Nano, firstSeenAt)
	observed, observedErr := time.Parse(time.RFC3339Nano, observedAt)
	if firstErr != nil || observedErr != nil {
		return false
	}
	return !observed.Before(firstSeen) && observed.Sub(firstSeen) <= contentIdentityFallbackWindow
}

func contentIdentityFingerprint(signature string) string {
	sum := sha256.Sum256([]byte(signature))
	return hex.EncodeToString(sum[:])
}

func (s *Store) backfillContentIdentityAliases(ctx context.Context) error {
	var completed string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, contentIdentityBackfillKey).Scan(&completed)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT o.run_id,o.observation_json,o.captured_at
		FROM observations o JOIN runs r ON r.id=o.run_id
		ORDER BY o.created_at,r.ordinal`)
	if err != nil {
		return err
	}
	type historicalObservation struct {
		runID       string
		observation domain.Observation
		capturedAt  string
	}
	values := []historicalObservation{}
	for rows.Next() {
		var value historicalObservation
		var raw string
		if err := rows.Scan(&value.runID, &raw, &value.capturedAt); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(raw), &value.observation); err != nil {
			rows.Close()
			return err
		}
		values = append(values, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index := range values {
		if _, err := s.resolveObservationContentIdentity(
			ctx,
			tx,
			values[index].runID,
			&values[index].observation,
			values[index].capturedAt,
		); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		contentIdentityBackfillKey, contentIdentityVersion); err != nil {
		return err
	}
	return tx.Commit()
}
