package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	maxMemoryTitleRunes       = 300
	maxMemorySummaryRunes     = 4000
	maxMemoryAuthorRunes      = 300
	maxMemoryReasonRunes      = 500
	maxMemoryIdentityRunes    = 2000
	maxMemoryTagRunes         = 120
	maxMemoryTagCount         = 32
	maxMemoryMediaReferences  = 16
	maxMemoryMediaFieldRunes  = 300
	maxMemoryMediaURLRunes    = 2048
	maxMemoryContentBytes     = 4 * 1024 * 1024
	maxMemoryContextJSONBytes = 16 * 1024
	maxMemoryActionJSONBytes  = 16 * 1024
)

var (
	ErrMemoryNotFound   = errors.New("personal memory item not found")
	ErrMemoryTombstoned = errors.New("personal memory item is tombstoned")
)

const memoryTombstoneKeyMeta = "memory_tombstone_key_v1"

type memoryRowQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type normalizedMemoryInput struct {
	ID             string
	Identity       domain.MemoryIdentity
	IdentityDigest string
	Title          string
	Summary        string
	Author         string
	PublishedAt    *string
	Tags           []string
	Facets         []string
	Media          []domain.MemoryMediaReference
	TagsProvided   bool
	FacetsProvided bool
	MediaProvided  bool
	Reason         string
	Provenance     []domain.MemoryProvenance
}

type memoryStoredRow struct {
	item                 domain.MemoryItem
	identityDigest       string
	tagsJSON             string
	facetsJSON           string
	mediaJSON            string
	publishedAt          sql.NullString
	fullContentVersionID string
}

// memoryTombstoneKey is a per-database key used only to derive deletion
// suppression digests. This keeps raw URLs and evidence keys out of tombstone
// rows and avoids using guessable plain hashes; it is pseudonymization, not a
// secrecy boundary against an actor who can read the entire database. FullReset
// removes the key and all derived tombstone rows together.
func (s *Store) memoryTombstoneKey(ctx context.Context) ([]byte, error) {
	var encoded string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, memoryTombstoneKeyMeta).Scan(&encoded)
	if err == nil {
		key, decodeErr := hex.DecodeString(encoded)
		if decodeErr != nil || len(key) != 32 {
			return nil, errors.New("stored memory tombstone key is invalid")
		}
		return key, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read memory tombstone key: %w", err)
	}
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return nil, fmt.Errorf("generate memory tombstone key: %w", err)
	}
	encoded = hex.EncodeToString(key[:])
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO meta(key,value) VALUES(?,?)
		ON CONFLICT(key) DO NOTHING`, memoryTombstoneKeyMeta, encoded); err != nil {
		return nil, fmt.Errorf("save memory tombstone key: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key=?`, memoryTombstoneKeyMeta).Scan(&encoded); err != nil {
		return nil, fmt.Errorf("read saved memory tombstone key: %w", err)
	}
	decoded, err := hex.DecodeString(encoded)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("saved memory tombstone key is invalid")
	}
	return decoded, nil
}

// UpsertMemoryRecallStub creates a durable recall stub or updates the active
// stub selected by canonical identity aliases. It never creates a full copy;
// that is an explicit KeepMemoryFullCopy action.
func (s *Store) UpsertMemoryRecallStub(ctx context.Context, input domain.MemoryItemInput) (domain.MemoryItem, error) {
	normalized, err := normalizeMemoryInput(input)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	tombstoneKey, err := s.memoryTombstoneKey(ctx)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	normalized.IdentityDigest = memoryIdentityDigest(tombstoneKey, normalized.Identity)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MemoryItem{}, fmt.Errorf("begin memory stub transaction: %w", err)
	}
	defer tx.Rollback()

	memoryID, err := s.upsertMemoryRecallStubTx(ctx, tx, tombstoneKey, normalized)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("commit memory recall stub: %w", err)
	}
	return s.MemoryItem(ctx, memoryID)
}

// upsertMemoryRecallStubTx applies a normalized recall-stub projection inside
// a caller-owned transaction. Keeping this boundary internal lets routine
// Timeline feedback commit its canonical preference event and memory
// projection atomically without opening a nested SQLite transaction.
func (s *Store) upsertMemoryRecallStubTx(ctx context.Context, tx *sql.Tx, tombstoneKey []byte, normalized normalizedMemoryInput) (string, error) {
	if tombstoneID, err := tombstonedMemoryID(ctx, tx, tombstoneKey, normalized.Identity); err != nil {
		return "", err
	} else if tombstoneID != "" {
		return "", fmt.Errorf("%w: %s", ErrMemoryTombstoned, tombstoneID)
	}

	memoryID, err := resolveMemoryIdentity(ctx, tx, normalized.Identity)
	if err != nil {
		return "", err
	}
	now := memoryNow(s)
	created := false
	if memoryID == "" {
		memoryID = domain.NewID("memory")
		created = true
		tagsJSON, _ := json.Marshal(normalized.Tags)
		facetsJSON, _ := json.Marshal(normalized.Facets)
		mediaJSON, _ := json.Marshal(normalized.Media)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO memory_items(
			  id,source,identity_digest,canonical_evidence_key,canonical_permalink,
			  canonical_platform_id,content_fingerprint,title,summary,author,published_at,
			  tags_json,facets_json,media_metadata_json,retention_tier,lifecycle_state,
			  full_content_version_id,content_bytes,reason,created_at,updated_at
			) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,'active','',0,?,?,?)`,
			memoryID, normalized.Identity.Source, normalized.IdentityDigest,
			normalized.Identity.CanonicalEvidenceKey, normalized.Identity.CanonicalPermalink,
			normalized.Identity.CanonicalPlatformID, normalized.Identity.ContentFingerprint,
			normalized.Title, normalized.Summary, normalized.Author, nullableString(normalized.PublishedAt),
			string(tagsJSON), string(facetsJSON), string(mediaJSON), domain.MemoryTierRecall,
			normalized.Reason, now, now)
		if err != nil {
			return "", fmt.Errorf("insert memory recall stub: %w", err)
		}
	} else {
		stored, err := memoryStoredByID(ctx, tx, memoryID)
		if err != nil {
			return "", err
		}
		if stored.item.LifecycleState == domain.MemoryStateTombstone {
			return "", fmt.Errorf("%w: %s", ErrMemoryTombstoned, memoryID)
		}
		if err := updateMemoryStub(ctx, tx, stored, normalized, now); err != nil {
			return "", err
		}
	}

	if err := upsertMemoryAliases(ctx, tx, memoryID, normalized.Identity, now); err != nil {
		return "", err
	}
	if created {
		if err := recordMemoryActionTx(ctx, tx, memoryID, domain.MemoryActionCreateStub, nil, now); err != nil {
			return "", err
		}
	} else if err := recordMemoryActionTx(ctx, tx, memoryID, domain.MemoryActionUpdateStub, nil, now); err != nil {
		return "", err
	}
	for _, provenance := range normalized.Provenance {
		if err := recordMemoryProvenanceTx(ctx, tx, memoryID, provenance, now); err != nil {
			return "", err
		}
	}
	if err := syncMemorySearchIndexTx(ctx, tx, memoryID); err != nil {
		return "", err
	}
	return memoryID, nil
}

// CreateMemoryRecallStub is the explicit create/update entry point used by
// callers that want the product terminology in their code.
func (s *Store) CreateMemoryRecallStub(ctx context.Context, input domain.MemoryRecallStubInput) (domain.MemoryItem, error) {
	return s.UpsertMemoryRecallStub(ctx, input)
}

// CreateMemoryItem is a compatibility-friendly alias for the canonical
// recall-stub operation.
func (s *Store) CreateMemoryItem(ctx context.Context, input domain.MemoryItemInput) (domain.MemoryItem, error) {
	return s.UpsertMemoryRecallStub(ctx, input)
}

// UpdateMemoryRecallStub updates the item identified by id. Missing identity
// fields are filled from the stored row, so a caller may update only metadata.
func (s *Store) UpdateMemoryRecallStub(ctx context.Context, id string, input domain.MemoryItemInput) (domain.MemoryItem, error) {
	stored, err := s.MemoryItem(ctx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if stored.LifecycleState == domain.MemoryStateTombstone {
		return domain.MemoryItem{}, fmt.Errorf("%w: %s", ErrMemoryTombstoned, id)
	}
	input.Identity = mergeMemoryIdentity(input.Identity, domain.MemoryIdentity{
		Source:               stored.Source,
		CanonicalEvidenceKey: stored.CanonicalEvidenceKey,
		CanonicalPermalink:   stored.CanonicalPermalink,
		CanonicalPlatformID:  stored.CanonicalPlatformID,
		ContentFingerprint:   stored.ContentFingerprint,
	})
	if input.Source == "" {
		input.Source = stored.Source
	}
	return s.UpsertMemoryRecallStub(ctx, input)
}

func (s *Store) MemoryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	if strings.TrimSpace(id) == "" {
		return domain.MemoryItem{}, ErrMemoryNotFound
	}
	return memoryItemByQueryer(ctx, s.db, id)
}

// GetMemoryItem is a descriptive alias for MemoryItem.
func (s *Store) GetMemoryItem(ctx context.Context, id string) (domain.MemoryItem, error) {
	return s.MemoryItem(ctx, id)
}

// KeepMemoryFullCopy applies the explicit full-copy decision. Text is bounded
// and media remains metadata-only; no binary payload is accepted or stored.
func (s *Store) KeepMemoryFullCopy(ctx context.Context, id string, input domain.MemoryFullCopyInput) (domain.MemoryItem, error) {
	if strings.TrimSpace(id) == "" {
		return domain.MemoryItem{}, ErrMemoryNotFound
	}
	if len([]byte(input.Content)) > maxMemoryContentBytes {
		return domain.MemoryItem{}, fmt.Errorf("memory full copy exceeds %d bytes", maxMemoryContentBytes)
	}
	if strings.TrimSpace(input.Content) == "" && len(input.Media) == 0 {
		return domain.MemoryItem{}, errors.New("memory full copy requires text or media metadata")
	}
	_, mediaJSON, err := normalizeMemoryMedia(input.Media)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	now := memoryNow(s)
	capturedAt := strings.TrimSpace(input.CapturedAt)
	if capturedAt == "" {
		capturedAt = now
	}
	if len([]rune(input.Reason)) > maxMemoryReasonRunes {
		return domain.MemoryItem{}, fmt.Errorf("memory reason cannot exceed %d characters", maxMemoryReasonRunes)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MemoryItem{}, fmt.Errorf("begin memory full-copy transaction: %w", err)
	}
	defer tx.Rollback()
	stored, err := memoryStoredByID(ctx, tx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if stored.item.LifecycleState == domain.MemoryStateTombstone {
		return domain.MemoryItem{}, fmt.Errorf("%w: %s", ErrMemoryTombstoned, id)
	}
	if len(input.Media) == 0 {
		mediaJSON = stored.mediaJSON
	}
	version := 1
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(version),0)+1 FROM memory_content_versions WHERE memory_item_id=?`, id).Scan(&version); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("read memory content version: %w", err)
	}
	// Keep exactly one active payload. Older versions remain as bounded audit
	// metadata, but their text is released before the new copy is committed.
	if _, err := tx.ExecContext(ctx, `
		UPDATE memory_content_versions
		SET content='',content_bytes=0,released_at=?
		WHERE memory_item_id=? AND released_at IS NULL`, now, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("release previous memory content version: %w", err)
	}
	versionID := domain.NewID("memory_content")
	contentFingerprint := memoryContentFingerprint(input.Content)
	contentBytes := int64(len([]byte(input.Content)))
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO memory_content_versions(
		  id,memory_item_id,version,content,content_fingerprint,media_metadata_json,
		  content_bytes,captured_at,created_at,released_at
		) VALUES(?,?,?,?,?,?,?,?,?,NULL)`,
		versionID, id, version, input.Content, contentFingerprint, mediaJSON,
		contentBytes, capturedAt, now); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("insert memory content version: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE memory_items
		SET retention_tier='full_copy',full_content_version_id=?,content_bytes=?,media_metadata_json=?,updated_at=?
		WHERE id=? AND lifecycle_state='active'`, versionID, contentBytes, mediaJSON, now, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("update memory full-copy state: %w", err)
	}
	detail := map[string]any{"contentBytes": contentBytes, "version": version}
	if input.Reason != "" {
		// Reason is bounded and contains no source/content payload by contract.
		detail["reason"] = input.Reason
	}
	if err := recordMemoryActionTx(ctx, tx, id, domain.MemoryActionKeepFullCopy, detail, now); err != nil {
		return domain.MemoryItem{}, err
	}
	if err := syncMemorySearchIndexTx(ctx, tx, id); err != nil {
		return domain.MemoryItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("commit memory full copy: %w", err)
	}
	return s.MemoryItem(ctx, id)
}

// KeepFullCopy is a short alias for callers using the UI action name.
func (s *Store) KeepFullCopy(ctx context.Context, id string, input domain.MemoryFullCopyInput) (domain.MemoryItem, error) {
	return s.KeepMemoryFullCopy(ctx, id, input)
}

// KeepMemoryContent is a convenience wrapper for text-only callers.
func (s *Store) KeepMemoryContent(ctx context.Context, id, content string) (domain.MemoryItem, error) {
	return s.KeepMemoryFullCopy(ctx, id, domain.MemoryFullCopyInput{Content: content})
}

func (s *Store) ReleaseMemoryFullCopy(ctx context.Context, id string) (domain.MemoryItem, error) {
	if strings.TrimSpace(id) == "" {
		return domain.MemoryItem{}, ErrMemoryNotFound
	}
	now := memoryNow(s)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MemoryItem{}, fmt.Errorf("begin memory release transaction: %w", err)
	}
	defer tx.Rollback()
	stored, err := memoryStoredByID(ctx, tx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if stored.item.LifecycleState == domain.MemoryStateTombstone {
		return domain.MemoryItem{}, fmt.Errorf("%w: %s", ErrMemoryTombstoned, id)
	}
	if stored.item.RetentionTier == domain.MemoryTierFullCopy || stored.item.ContentBytes > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE memory_content_versions
			SET content='',content_bytes=0,released_at=?
			WHERE memory_item_id=? AND released_at IS NULL`, now, id); err != nil {
			return domain.MemoryItem{}, fmt.Errorf("release memory content: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE memory_items
			SET retention_tier='recall',full_content_version_id='',content_bytes=0,updated_at=?
			WHERE id=? AND lifecycle_state='active'`, now, id); err != nil {
			return domain.MemoryItem{}, fmt.Errorf("update released memory state: %w", err)
		}
		if err := recordMemoryActionTx(ctx, tx, id, domain.MemoryActionReleaseFullCopy, nil, now); err != nil {
			return domain.MemoryItem{}, err
		}
	}
	if err := syncMemorySearchIndexTx(ctx, tx, id); err != nil {
		return domain.MemoryItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("commit memory release: %w", err)
	}
	return s.MemoryItem(ctx, id)
}

func (s *Store) ReleaseFullCopy(ctx context.Context, id string) (domain.MemoryItem, error) {
	return s.ReleaseMemoryFullCopy(ctx, id)
}

// RecordMemoryProvenance appends an explainable source/decision record. It is
// intentionally independent from operational session/run foreign keys.
func (s *Store) RecordMemoryProvenance(ctx context.Context, value domain.MemoryProvenance) (domain.MemoryProvenance, error) {
	if strings.TrimSpace(value.MemoryItemID) == "" {
		return domain.MemoryProvenance{}, ErrMemoryNotFound
	}
	now := memoryNow(s)
	if strings.TrimSpace(value.ID) == "" {
		value.ID = domain.NewID("memory_provenance")
	}
	if strings.TrimSpace(value.CreatedAt) == "" {
		value.CreatedAt = now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MemoryProvenance{}, err
	}
	defer tx.Rollback()
	stored, err := memoryStoredByID(ctx, tx, value.MemoryItemID)
	if err != nil {
		return domain.MemoryProvenance{}, err
	}
	if stored.item.LifecycleState == domain.MemoryStateTombstone {
		return domain.MemoryProvenance{}, fmt.Errorf("%w: %s", ErrMemoryTombstoned, value.MemoryItemID)
	}
	if err := recordMemoryProvenanceTx(ctx, tx, value.MemoryItemID, value, now); err != nil {
		return domain.MemoryProvenance{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryProvenance{}, fmt.Errorf("commit memory provenance: %w", err)
	}
	if value.ProvenanceKind == "" {
		value.ProvenanceKind = "unknown"
	}
	if value.Source == "" {
		item, itemErr := s.MemoryItem(ctx, value.MemoryItemID)
		if itemErr == nil {
			value.Source = item.Source
		}
	}
	return value, nil
}

// RecordMemoryAction appends an auditable user/system lifecycle action.
func (s *Store) RecordMemoryAction(ctx context.Context, value domain.MemoryAction) (domain.MemoryAction, error) {
	if strings.TrimSpace(value.MemoryItemID) == "" {
		return domain.MemoryAction{}, ErrMemoryNotFound
	}
	if !value.Action.Valid() {
		return domain.MemoryAction{}, fmt.Errorf("unsupported memory action %q", value.Action)
	}
	now := memoryNow(s)
	if strings.TrimSpace(value.ID) == "" {
		value.ID = domain.NewID("memory_action")
	}
	if strings.TrimSpace(value.CreatedAt) == "" {
		value.CreatedAt = now
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MemoryAction{}, err
	}
	defer tx.Rollback()
	if _, err := memoryStoredByID(ctx, tx, value.MemoryItemID); err != nil {
		return domain.MemoryAction{}, err
	}
	if err := recordMemoryActionTxWithID(ctx, tx, value.MemoryItemID, value.Action, value.Detail, value.ID, value.CreatedAt); err != nil {
		return domain.MemoryAction{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryAction{}, fmt.Errorf("commit memory action: %w", err)
	}
	return value, nil
}

// DeleteMemory replaces all user-identifying content and provenance with an
// opaque tombstone. The tombstone id/digest allow the store to reject an
// accidental recapture without retaining a URL, text, author, or provenance.
func (s *Store) DeleteMemory(ctx context.Context, id string, _ ...string) (domain.MemoryItem, error) {
	if strings.TrimSpace(id) == "" {
		return domain.MemoryItem{}, ErrMemoryNotFound
	}
	tombstoneKey, err := s.memoryTombstoneKey(ctx)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	now := memoryNow(s)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.MemoryItem{}, fmt.Errorf("begin memory delete transaction: %w", err)
	}
	defer tx.Rollback()
	stored, err := memoryStoredByID(ctx, tx, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	if stored.item.LifecycleState == domain.MemoryStateTombstone {
		if err := tx.Commit(); err != nil {
			return domain.MemoryItem{}, err
		}
		return s.MemoryItem(ctx, id)
	}
	identity := stored.item.Identity
	digest := memoryIdentityDigest(tombstoneKey, identity)
	aliases, err := memoryAliasesForItem(ctx, tx, stored.item.ID, identity)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	for _, alias := range aliases {
		aliasDigest := memoryAliasDigest(tombstoneKey, identity.Source, alias.kind, alias.value)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO memory_tombstone_aliases(memory_item_id,alias_kind,alias_digest,created_at)
			VALUES(?,?,?,?) ON CONFLICT(memory_item_id,alias_kind,alias_digest) DO NOTHING`,
			id, alias.kind, aliasDigest, now); err != nil {
			return domain.MemoryItem{}, fmt.Errorf("write memory tombstone alias: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_identity_aliases WHERE memory_item_id=?`, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("delete memory aliases: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_provenance WHERE memory_item_id=?`, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("delete memory provenance: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM memory_content_versions WHERE memory_item_id=?`, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("delete memory content versions: %w", err)
	}
	// Earlier action details are user-controlled metadata. Scrub them as part
	// of the same privacy transition so a keep/update reason cannot retain a
	// URL or copied text after Delete.
	if _, err := tx.ExecContext(ctx, `UPDATE memory_actions SET detail_json='{}' WHERE memory_item_id=?`, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("scrub memory action details: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE memory_items SET
		  source='',canonical_evidence_key='',canonical_permalink='',canonical_platform_id='',
		  content_fingerprint='',title='',summary='',author='',published_at=NULL,
		  tags_json='[]',facets_json='[]',media_metadata_json='[]',retention_tier='recall',
		  lifecycle_state='tombstone',full_content_version_id='',content_bytes=0,reason='',
		  identity_digest=?,updated_at=?
		WHERE id=?`, digest, now, id); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("write memory tombstone: %w", err)
	}
	if err := syncMemorySearchIndexTx(ctx, tx, id); err != nil {
		return domain.MemoryItem{}, err
	}
	// Delete action details intentionally remain empty: an arbitrary user
	// reason must not become a covert URL/content retention channel.
	if err := recordMemoryActionTx(ctx, tx, id, domain.MemoryActionDelete, nil, now); err != nil {
		return domain.MemoryItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("commit memory tombstone: %w", err)
	}
	return s.MemoryItem(ctx, id)
}

func (s *Store) DeleteMemoryItem(ctx context.Context, id string, reason ...string) (domain.MemoryItem, error) {
	return s.DeleteMemory(ctx, id, reason...)
}

// MemoryStorageUsage reports logical bytes held by Personal Memory. It does
// not report SQLite/WAL file size and does not conflate those physical values
// with the bounded text payload estimate.
func (s *Store) MemoryStorageUsage(ctx context.Context) (domain.MemoryStorageUsage, error) {
	var usage domain.MemoryStorageUsage
	if err := s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE(SUM(CASE WHEN lifecycle_state='active' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN lifecycle_state='tombstone' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN lifecycle_state='active' AND retention_tier='recall' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(CASE WHEN lifecycle_state='active' AND retention_tier='full_copy' THEN 1 ELSE 0 END),0),
		  COALESCE(SUM(content_bytes),0)
		FROM memory_items`).Scan(
		&usage.ActiveItems, &usage.Tombstones, &usage.RecallItems,
		&usage.FullCopyItems, &usage.ContentBytes); err != nil {
		return domain.MemoryStorageUsage{}, fmt.Errorf("inspect memory item storage: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
		  length(CAST(source AS BLOB)) + length(CAST(identity_digest AS BLOB)) +
		  length(CAST(canonical_evidence_key AS BLOB)) + length(CAST(canonical_permalink AS BLOB)) +
		  length(CAST(canonical_platform_id AS BLOB)) + length(CAST(content_fingerprint AS BLOB)) +
		  length(CAST(title AS BLOB)) + length(CAST(summary AS BLOB)) + length(CAST(author AS BLOB)) +
		  COALESCE(length(CAST(published_at AS BLOB)),0) + length(CAST(tags_json AS BLOB)) +
		  length(CAST(facets_json AS BLOB)) + length(CAST(media_metadata_json AS BLOB)) +
		  length(CAST(retention_tier AS BLOB)) + length(CAST(lifecycle_state AS BLOB)) +
		  length(CAST(full_content_version_id AS BLOB)) + length(CAST(reason AS BLOB))
		),0) FROM memory_items`).Scan(&usage.MetadataBytes); err != nil {
		return domain.MemoryStorageUsage{}, fmt.Errorf("inspect memory metadata storage: %w", err)
	}
	var aliasBytes int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(CAST(source AS BLOB))+length(CAST(alias_kind AS BLOB))+length(CAST(alias_value AS BLOB))+length(CAST(memory_item_id AS BLOB))),0) FROM memory_identity_aliases`).Scan(&aliasBytes); err != nil {
		return domain.MemoryStorageUsage{}, fmt.Errorf("inspect memory alias storage: %w", err)
	}
	usage.MetadataBytes += aliasBytes
	var tombstoneAliasBytes int64
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(CAST(memory_item_id AS BLOB))+length(CAST(alias_kind AS BLOB))+length(CAST(alias_digest AS BLOB))),0) FROM memory_tombstone_aliases`).Scan(&tombstoneAliasBytes); err != nil {
		return domain.MemoryStorageUsage{}, fmt.Errorf("inspect memory tombstone alias storage: %w", err)
	}
	usage.MetadataBytes += tombstoneAliasBytes
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(
		  length(CAST(memory_item_id AS BLOB)) + length(CAST(provenance_kind AS BLOB)) +
		  length(CAST(source AS BLOB)) + length(CAST(canonical_evidence_key AS BLOB)) +
		  length(CAST(source_url AS BLOB)) + length(CAST(capture_context_json AS BLOB)) + length(CAST(reason AS BLOB))
		),0) FROM memory_provenance`).Scan(&usage.ProvenanceBytes); err != nil {
		return domain.MemoryStorageUsage{}, fmt.Errorf("inspect memory provenance storage: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(SUM(length(CAST(memory_item_id AS BLOB))+length(CAST(action AS BLOB))+length(CAST(detail_json AS BLOB))),0) FROM memory_actions`).Scan(&usage.ActionBytes); err != nil {
		return domain.MemoryStorageUsage{}, fmt.Errorf("inspect memory action storage: %w", err)
	}
	usage.LogicalBytes = usage.ContentBytes + usage.MetadataBytes + usage.ProvenanceBytes + usage.ActionBytes
	return usage, nil
}

func (s *Store) MemoryStorage(ctx context.Context) (domain.MemoryStorageUsage, error) {
	return s.MemoryStorageUsage(ctx)
}

func (s *Store) ListMemoryItems(ctx context.Context, includeTombstones bool, limit int) ([]domain.MemoryItem, error) {
	if limit <= 0 || limit > 1000 {
		limit = 1000
	}
	stateClause := "lifecycle_state='active'"
	if includeTombstones {
		stateClause = "1=1"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM memory_items WHERE `+stateClause+` ORDER BY updated_at DESC,id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var values []domain.MemoryItem
	for _, id := range ids {
		value, err := s.MemoryItem(ctx, id)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	return values, nil
}

func normalizeMemoryInput(input domain.MemoryItemInput) (normalizedMemoryInput, error) {
	identity := mergeMemoryIdentity(input.Identity, domain.MemoryIdentity{
		Source:               input.Source,
		CanonicalEvidenceKey: input.CanonicalEvidenceKey,
		CanonicalPermalink:   input.CanonicalPermalink,
		CanonicalURL:         input.CanonicalURL,
		CanonicalPlatformID:  input.CanonicalPlatformID,
		ContentFingerprint:   input.ContentFingerprint,
	})
	if identity.Source == "" || !identity.Source.Valid() {
		return normalizedMemoryInput{}, fmt.Errorf("memory identity requires a supported source")
	}
	identity.CanonicalEvidenceKey = strings.TrimSpace(identity.CanonicalEvidenceKey)
	identity.CanonicalPermalink = strings.TrimSpace(identity.CanonicalPermalink)
	if identity.CanonicalPermalink == "" {
		identity.CanonicalPermalink = strings.TrimSpace(identity.CanonicalURL)
	}
	if identity.CanonicalPermalink != "" {
		canonical, ok := domain.CanonicalSourceURL(identity.Source, identity.CanonicalPermalink)
		if !ok {
			return normalizedMemoryInput{}, fmt.Errorf("memory canonical permalink is not a valid %s native URL", identity.Source)
		}
		identity.CanonicalPermalink = canonical
	}
	identity.CanonicalPlatformID = domain.NormalizeNativeIdentity(identity.Source, identity.CanonicalPlatformID)
	if identity.CanonicalPlatformID == "" && identity.CanonicalPermalink != "" {
		identity.CanonicalPlatformID = domain.NativeIdentityFromPermalink(identity.Source, identity.CanonicalPermalink)
	}
	identity.ContentFingerprint = strings.TrimSpace(identity.ContentFingerprint)
	if identity.CanonicalEvidenceKey == "" && identity.CanonicalPermalink == "" && identity.CanonicalPlatformID == "" && identity.ContentFingerprint == "" {
		return normalizedMemoryInput{}, errors.New("memory identity requires an evidence key, permalink, platform id, or content fingerprint")
	}
	for name, value := range map[string]string{
		"canonical evidence key": identity.CanonicalEvidenceKey,
		"canonical permalink":    identity.CanonicalPermalink,
		"canonical platform id":  identity.CanonicalPlatformID,
		"content fingerprint":    identity.ContentFingerprint,
	} {
		if len([]rune(value)) > maxMemoryIdentityRunes {
			return normalizedMemoryInput{}, fmt.Errorf("memory %s cannot exceed %d characters", name, maxMemoryIdentityRunes)
		}
	}

	tags, err := normalizeMemoryLabels(input.Tags, "tags")
	if err != nil {
		return normalizedMemoryInput{}, err
	}
	facets, err := normalizeMemoryLabels(input.Facets, "facets")
	if err != nil {
		return normalizedMemoryInput{}, err
	}
	media, _, err := normalizeMemoryMedia(input.Media)
	if err != nil {
		return normalizedMemoryInput{}, err
	}
	if len([]rune(input.Title)) > maxMemoryTitleRunes {
		return normalizedMemoryInput{}, fmt.Errorf("memory title cannot exceed %d characters", maxMemoryTitleRunes)
	}
	if len([]rune(input.Summary)) > maxMemorySummaryRunes {
		return normalizedMemoryInput{}, fmt.Errorf("memory summary cannot exceed %d characters", maxMemorySummaryRunes)
	}
	if len([]rune(input.Author)) > maxMemoryAuthorRunes {
		return normalizedMemoryInput{}, fmt.Errorf("memory author cannot exceed %d characters", maxMemoryAuthorRunes)
	}
	if len([]rune(input.Reason)) > maxMemoryReasonRunes {
		return normalizedMemoryInput{}, fmt.Errorf("memory reason cannot exceed %d characters", maxMemoryReasonRunes)
	}
	var publishedAt *string
	if input.PublishedAt != nil && strings.TrimSpace(*input.PublishedAt) != "" {
		value := strings.TrimSpace(*input.PublishedAt)
		if len([]rune(value)) > 100 {
			return normalizedMemoryInput{}, errors.New("memory publishedAt cannot exceed 100 characters")
		}
		publishedAt = &value
	}
	return normalizedMemoryInput{
		ID: input.ID, Identity: identity,
		Title: strings.TrimSpace(input.Title), Summary: strings.TrimSpace(input.Summary),
		Author: strings.TrimSpace(input.Author), PublishedAt: publishedAt,
		Tags: tags, Facets: facets, Media: media,
		TagsProvided: input.Tags != nil, FacetsProvided: input.Facets != nil, MediaProvided: input.Media != nil,
		Reason: strings.TrimSpace(input.Reason), Provenance: input.Provenance,
	}, nil
}

// routineMoreMemoryInput is the narrow projection from a final visible
// Timeline item into Personal Memory. It deliberately copies only bounded
// recall metadata; full source text remains outside this path and can only be
// retained through the explicit KeepMemoryFullCopy action.
func routineMoreMemoryInput(item domain.TimelineItem) domain.MemoryItemInput {
	source := item.Source
	if source == "" {
		source = item.Item.Source
	}
	block := item.Evidence
	evidenceKey := strings.TrimSpace(item.EvidenceKey)
	if evidenceKey == "" {
		evidenceKey = strings.TrimSpace(item.Item.EvidenceKey)
	}
	permalink := ""
	platformID := ""
	author := strings.TrimSpace(item.Item.Author)
	text := ""
	var publishedAt *string
	var media []domain.MemoryMediaReference
	if block != nil {
		evidenceKey = firstMemoryString(evidenceKey, block.EvidenceKey)
		permalink = strings.TrimSpace(block.Permalink)
		platformID = strings.TrimSpace(block.PlatformID)
		author = firstMemoryString(author, block.Author)
		text = strings.TrimSpace(block.Text)
		publishedAt = block.PublishedAt
		media = timelineMemoryMedia(block.Media)
	}
	permalink = firstMemoryString(permalink, item.Item.SourceURL)
	if canonical, ok := domain.CanonicalSourceURL(source, permalink); ok {
		permalink = canonical
	} else {
		// A malformed or non-native source URL must not make the explicit
		// preference action fail; retain the durable evidence key instead.
		permalink = ""
	}
	if publishedAt == nil {
		publishedAt = item.Item.PublishedAt
	}
	var publishedCopy *string
	if publishedAt != nil {
		value := boundedMemoryText(*publishedAt, 100)
		if value != "" {
			publishedCopy = &value
		}
	}
	contentFingerprint := ""
	if text != "" {
		contentFingerprint = memoryContentFingerprint(text)
	}
	title := boundedMemoryText(item.Item.WhatChanged, maxMemoryTitleRunes)
	summary := boundedMemoryText(item.Item.WhyItMatters, maxMemorySummaryRunes)
	if summary == "" {
		summary = boundedMemoryText(text, maxMemorySummaryRunes)
	}
	return domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{
			Source:               source,
			CanonicalEvidenceKey: boundedMemoryText(evidenceKey, maxMemoryIdentityRunes),
			CanonicalPermalink:   permalink,
			CanonicalPlatformID:  boundedMemoryText(platformID, maxMemoryIdentityRunes),
			ContentFingerprint:   contentFingerprint,
		},
		Title:       title,
		Summary:     summary,
		Author:      boundedMemoryText(author, maxMemoryAuthorRunes),
		PublishedAt: publishedCopy,
		Tags:        boundedMemoryLabels(item.Assessment.TopicTags),
		Facets:      boundedMemoryLabels(item.Assessment.TopicFacets),
		Media:       media,
		Reason:      "routine_more",
		Provenance: []domain.MemoryProvenance{{
			ProvenanceKind:       "explicit_feedback",
			Source:               source,
			CanonicalEvidenceKey: boundedMemoryText(evidenceKey, maxMemoryIdentityRunes),
			SourceURL:            permalink,
			CaptureContext: map[string]any{
				"surface":    "timeline",
				"feedback":   "more",
				"timelineId": item.ID,
				"sessionId":  item.SessionID,
				"runId":      item.RunID,
			},
			Reason: "routine_more",
		}},
	}
}

func firstMemoryString(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func boundedMemoryText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) > maxRunes {
		runes = runes[:maxRunes]
	}
	return string(runes)
}

func boundedMemoryLabels(values []string) []string {
	if values == nil {
		return nil
	}
	if len(values) > maxMemoryTagCount {
		values = values[:maxMemoryTagCount]
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value = boundedMemoryText(value, maxMemoryTagRunes); value != "" {
			result = append(result, value)
		}
	}
	return result
}

func timelineMemoryMedia(values []map[string]any) []domain.MemoryMediaReference {
	if len(values) == 0 {
		return nil
	}
	result := make([]domain.MemoryMediaReference, 0, len(values))
	for _, raw := range values {
		kind := boundedMemoryText(stringValue(raw, "kind"), maxMemoryMediaFieldRunes)
		if kind == "" {
			continue
		}
		mediaURL := firstMemoryString(
			stringValue(raw, "url"), stringValue(raw, "posterUrl"),
			stringValue(raw, "imageUrl"), stringValue(raw, "thumbnailUrl"),
		)
		if mediaURL != "" {
			parsed, err := url.Parse(mediaURL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				// Keep only the media kind when an adapter supplied an unsafe
				// pointer; this path never stores a binary or local-file ref.
				mediaURL = ""
			}
		}
		result = append(result, domain.MemoryMediaReference{
			Kind:    kind,
			URL:     boundedMemoryText(mediaURL, maxMemoryMediaURLRunes),
			Title:   boundedMemoryText(firstMemoryString(stringValue(raw, "title"), stringValue(raw, "caption")), maxMemoryMediaFieldRunes),
			AltText: boundedMemoryText(firstMemoryString(stringValue(raw, "altText"), stringValue(raw, "alt")), maxMemoryMediaFieldRunes),
		})
		if len(result) == maxMemoryMediaReferences {
			break
		}
	}
	return result
}

func mergeMemoryIdentity(primary, fallback domain.MemoryIdentity) domain.MemoryIdentity {
	result := primary
	if result.Source == "" {
		result.Source = fallback.Source
	}
	if result.CanonicalEvidenceKey == "" {
		result.CanonicalEvidenceKey = fallback.CanonicalEvidenceKey
	}
	if result.CanonicalPermalink == "" {
		result.CanonicalPermalink = fallback.CanonicalPermalink
	}
	if result.CanonicalURL == "" {
		result.CanonicalURL = fallback.CanonicalURL
	}
	if result.CanonicalPlatformID == "" {
		result.CanonicalPlatformID = fallback.CanonicalPlatformID
	}
	if result.ContentFingerprint == "" {
		result.ContentFingerprint = fallback.ContentFingerprint
	}
	return result
}

func normalizeMemoryLabels(values []string, label string) ([]string, error) {
	if values == nil {
		return nil, nil
	}
	if len(values) > maxMemoryTagCount {
		return nil, fmt.Errorf("memory %s cannot contain more than %d values", label, maxMemoryTagCount)
	}
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if len([]rune(value)) > maxMemoryTagRunes {
			return nil, fmt.Errorf("memory %s value cannot exceed %d characters", label, maxMemoryTagRunes)
		}
		key := strings.ToLower(value)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, value)
	}
	return result, nil
}

func normalizeMemoryMedia(values []domain.MemoryMediaReference) ([]domain.MemoryMediaReference, string, error) {
	if len(values) > maxMemoryMediaReferences {
		return nil, "", fmt.Errorf("memory media metadata cannot contain more than %d references", maxMemoryMediaReferences)
	}
	result := make([]domain.MemoryMediaReference, 0, len(values))
	for _, value := range values {
		value.Kind = strings.TrimSpace(value.Kind)
		value.URL = strings.TrimSpace(value.URL)
		value.Title = strings.TrimSpace(value.Title)
		value.AltText = strings.TrimSpace(value.AltText)
		if value.Kind == "" {
			return nil, "", errors.New("memory media metadata kind is required")
		}
		if len([]rune(value.Kind)) > maxMemoryMediaFieldRunes || len([]rune(value.Title)) > maxMemoryMediaFieldRunes || len([]rune(value.AltText)) > maxMemoryMediaFieldRunes {
			return nil, "", fmt.Errorf("memory media metadata fields cannot exceed %d characters", maxMemoryMediaFieldRunes)
		}
		if len([]rune(value.URL)) > maxMemoryMediaURLRunes {
			return nil, "", fmt.Errorf("memory media URL cannot exceed %d characters", maxMemoryMediaURLRunes)
		}
		if value.URL != "" {
			parsed, err := url.Parse(value.URL)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				return nil, "", errors.New("memory media URL must be an HTTPS metadata reference")
			}
		}
		result = append(result, value)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, "", fmt.Errorf("marshal memory media metadata: %w", err)
	}
	return result, string(raw), nil
}

func resolveMemoryIdentity(ctx context.Context, tx *sql.Tx, identity domain.MemoryIdentity) (string, error) {
	aliases := memoryAliases(identity)
	if len(aliases) == 0 {
		return "", nil
	}
	var matched string
	strongMatch := false
	for index, alias := range aliases {
		if index == len(aliases)-1 && alias.kind == "content_fingerprint" && strongMatch {
			continue
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT DISTINCT mi.id
			FROM memory_identity_aliases a
			JOIN memory_items mi ON mi.id=a.memory_item_id
			WHERE a.source=? AND a.alias_kind=? AND a.alias_value=? AND mi.lifecycle_state='active'`,
			identity.Source, alias.kind, alias.value)
		if err != nil {
			return "", fmt.Errorf("resolve memory alias %s: %w", alias.kind, err)
		}
		var ids []string
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return "", err
			}
			ids = append(ids, id)
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
		if len(ids) == 0 {
			continue
		}
		if alias.kind == "content_fingerprint" && len(ids) != 1 {
			// A fingerprint is a last-resort hint and must not merge ambiguous
			// source records.
			continue
		}
		if matched != "" && matched != ids[0] {
			return "", errors.New("memory identity aliases resolve to different active items")
		}
		matched = ids[0]
		if alias.kind != "content_fingerprint" {
			strongMatch = true
		}
	}
	return matched, nil
}

type memoryAlias struct {
	kind  string
	value string
}

func memoryAliases(identity domain.MemoryIdentity) []memoryAlias {
	aliases := make([]memoryAlias, 0, 4)
	if value := strings.TrimSpace(identity.CanonicalEvidenceKey); value != "" {
		aliases = append(aliases, memoryAlias{kind: "canonical_evidence_key", value: value})
	}
	if value := strings.TrimSpace(identity.CanonicalPermalink); value != "" {
		aliases = append(aliases, memoryAlias{kind: "canonical_permalink", value: value})
	}
	if value := strings.TrimSpace(identity.CanonicalPlatformID); value != "" {
		aliases = append(aliases, memoryAlias{kind: "canonical_platform_id", value: value})
	}
	if value := strings.TrimSpace(identity.ContentFingerprint); value != "" {
		aliases = append(aliases, memoryAlias{kind: "content_fingerprint", value: value})
	}
	return aliases
}

func memoryAliasesForItem(ctx context.Context, tx *sql.Tx, itemID string, identity domain.MemoryIdentity) ([]memoryAlias, error) {
	aliases := memoryAliases(identity)
	seen := make(map[string]bool, len(aliases))
	for _, alias := range aliases {
		seen[alias.kind+"\x00"+alias.value] = true
	}
	rows, err := tx.QueryContext(ctx, `SELECT alias_kind,alias_value FROM memory_identity_aliases WHERE memory_item_id=?`, itemID)
	if err != nil {
		return nil, fmt.Errorf("read memory aliases for tombstone: %w", err)
	}
	for rows.Next() {
		var alias memoryAlias
		if err := rows.Scan(&alias.kind, &alias.value); err != nil {
			rows.Close()
			return nil, err
		}
		key := alias.kind + "\x00" + alias.value
		if !seen[key] {
			seen[key] = true
			aliases = append(aliases, alias)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return aliases, nil
}

func upsertMemoryAliases(ctx context.Context, tx *sql.Tx, memoryID string, identity domain.MemoryIdentity, now string) error {
	for _, alias := range memoryAliases(identity) {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO memory_identity_aliases(source,alias_kind,alias_value,memory_item_id,created_at,last_seen_at)
			VALUES(?,?,?,?,?,?)
			ON CONFLICT(source,alias_kind,alias_value,memory_item_id)
			DO UPDATE SET last_seen_at=excluded.last_seen_at`,
			identity.Source, alias.kind, alias.value, memoryID, now, now); err != nil {
			return fmt.Errorf("store memory identity alias %s: %w", alias.kind, err)
		}
	}
	return nil
}

func updateMemoryStub(ctx context.Context, tx *sql.Tx, stored memoryStoredRow, input normalizedMemoryInput, now string) error {
	identity := mergeMemoryIdentity(input.Identity, domain.MemoryIdentity{
		Source:               stored.item.Source,
		CanonicalEvidenceKey: stored.item.CanonicalEvidenceKey,
		CanonicalPermalink:   stored.item.CanonicalPermalink,
		CanonicalPlatformID:  stored.item.CanonicalPlatformID,
		ContentFingerprint:   stored.item.ContentFingerprint,
	})
	identityDigest := input.IdentityDigest
	if identityDigest == "" {
		identityDigest = stored.identityDigest
	}
	tagsJSON, facetsJSON, mediaJSON := stored.tagsJSON, stored.facetsJSON, stored.mediaJSON
	if input.TagsProvided {
		raw, _ := json.Marshal(input.Tags)
		tagsJSON = string(raw)
	}
	if input.FacetsProvided {
		raw, _ := json.Marshal(input.Facets)
		facetsJSON = string(raw)
	}
	if input.MediaProvided {
		raw, _ := json.Marshal(input.Media)
		mediaJSON = string(raw)
	}
	title, summary, author, reason := stored.item.Title, stored.item.Summary, stored.item.Author, stored.item.Reason
	if input.Title != "" {
		title = input.Title
	}
	if input.Summary != "" {
		summary = input.Summary
	}
	if input.Author != "" {
		author = input.Author
	}
	if input.Reason != "" {
		reason = input.Reason
	}
	publishedAt := stored.publishedAt
	if input.PublishedAt != nil {
		publishedAt = sql.NullString{String: *input.PublishedAt, Valid: true}
	}
	_, err := tx.ExecContext(ctx, `
		UPDATE memory_items SET
		  source=?,identity_digest=?,canonical_evidence_key=?,canonical_permalink=?,canonical_platform_id=?,
		  content_fingerprint=?,title=?,summary=?,author=?,published_at=?,tags_json=?,facets_json=?,
		  media_metadata_json=?,reason=?,updated_at=?
		WHERE id=? AND lifecycle_state='active'`,
		identity.Source, identityDigest, identity.CanonicalEvidenceKey, identity.CanonicalPermalink,
		identity.CanonicalPlatformID, identity.ContentFingerprint, title, summary, author,
		nullableNullString(publishedAt), tagsJSON, facetsJSON, mediaJSON, reason, now, stored.item.ID)
	if err != nil {
		return fmt.Errorf("update memory recall stub: %w", err)
	}
	return nil
}

func memoryStoredByID(ctx context.Context, q memoryRowQueryer, id string) (memoryStoredRow, error) {
	var stored memoryStoredRow
	var source string
	var state, tier string
	var fullContentVersionID string
	err := q.QueryRowContext(ctx, `
		SELECT id,source,identity_digest,canonical_evidence_key,canonical_permalink,canonical_platform_id,
		  content_fingerprint,title,summary,author,published_at,tags_json,facets_json,media_metadata_json,
		  retention_tier,lifecycle_state,full_content_version_id,content_bytes,reason,created_at,updated_at
		FROM memory_items WHERE id=?`, id).Scan(
		&stored.item.ID, &source, &stored.identityDigest, &stored.item.CanonicalEvidenceKey,
		&stored.item.CanonicalPermalink, &stored.item.CanonicalPlatformID, &stored.item.ContentFingerprint,
		&stored.item.Title, &stored.item.Summary, &stored.item.Author, &stored.publishedAt,
		&stored.tagsJSON, &stored.facetsJSON, &stored.mediaJSON, &tier, &state, &fullContentVersionID,
		&stored.item.ContentBytes, &stored.item.Reason, &stored.item.CreatedAt, &stored.item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return memoryStoredRow{}, ErrMemoryNotFound
	}
	if err != nil {
		return memoryStoredRow{}, fmt.Errorf("read memory item: %w", err)
	}
	stored.item.Source = domain.Source(source)
	stored.item.RetentionTier = domain.MemoryTier(tier)
	stored.item.LifecycleState = domain.MemoryLifecycleState(state)
	stored.item.FullContentVersionID = fullContentVersionID
	stored.item.Identity = domain.MemoryIdentity{
		Source: stored.item.Source, CanonicalEvidenceKey: stored.item.CanonicalEvidenceKey,
		CanonicalPermalink: stored.item.CanonicalPermalink, CanonicalPlatformID: stored.item.CanonicalPlatformID,
		ContentFingerprint: stored.item.ContentFingerprint,
	}
	return stored, nil
}

func memoryItemByQueryer(ctx context.Context, q memoryRowQueryer, id string) (domain.MemoryItem, error) {
	stored, err := memoryStoredByID(ctx, q, id)
	if err != nil {
		return domain.MemoryItem{}, err
	}
	item := stored.item
	if err := json.Unmarshal([]byte(stored.tagsJSON), &item.Tags); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("decode memory tags: %w", err)
	}
	if err := json.Unmarshal([]byte(stored.facetsJSON), &item.Facets); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("decode memory facets: %w", err)
	}
	if err := json.Unmarshal([]byte(stored.mediaJSON), &item.Media); err != nil {
		return domain.MemoryItem{}, fmt.Errorf("decode memory media metadata: %w", err)
	}
	if stored.publishedAt.Valid {
		value := stored.publishedAt.String
		item.PublishedAt = &value
	}
	if item.LifecycleState == domain.MemoryStateActive && item.RetentionTier == domain.MemoryTierFullCopy && item.FullContentVersionID != "" {
		var content string
		if err := q.QueryRowContext(ctx, `
			SELECT content FROM memory_content_versions
			WHERE id=? AND memory_item_id=? AND released_at IS NULL`, item.FullContentVersionID, item.ID).Scan(&content); err == nil {
			item.FullContent = &content
		} else if !errors.Is(err, sql.ErrNoRows) {
			return domain.MemoryItem{}, fmt.Errorf("read memory full content: %w", err)
		}
	}
	return item, nil
}

func recordMemoryProvenanceTx(ctx context.Context, tx *sql.Tx, itemID string, value domain.MemoryProvenance, now string) error {
	kind := strings.ToLower(strings.TrimSpace(value.ProvenanceKind))
	if kind == "" {
		kind = "unknown"
	}
	if kind != "captured" && kind != "explicit_feedback" && kind != "imported" && kind != "manual" && kind != "unknown" {
		return fmt.Errorf("unsupported memory provenance kind %q", value.ProvenanceKind)
	}
	if value.Source != "" && !value.Source.Valid() {
		return fmt.Errorf("unsupported memory provenance source %q", value.Source)
	}
	if len([]rune(value.CanonicalEvidenceKey)) > maxMemoryIdentityRunes || len([]rune(value.SourceURL)) > maxMemoryMediaURLRunes || len([]rune(value.Reason)) > maxMemoryReasonRunes {
		return errors.New("memory provenance field exceeds its bounded length")
	}
	if strings.TrimSpace(value.SourceURL) != "" {
		parsed, err := url.Parse(strings.TrimSpace(value.SourceURL))
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
			return errors.New("memory provenance source URL must be HTTPS")
		}
	}
	contextJSON := "{}"
	if value.CaptureContext != nil {
		raw, err := json.Marshal(value.CaptureContext)
		if err != nil {
			return fmt.Errorf("marshal memory provenance context: %w", err)
		}
		if len(raw) > maxMemoryContextJSONBytes {
			return fmt.Errorf("memory provenance context exceeds %d bytes", maxMemoryContextJSONBytes)
		}
		contextJSON = string(raw)
	}
	createdAt := strings.TrimSpace(value.CreatedAt)
	if createdAt == "" {
		createdAt = now
	}
	provenanceID := strings.TrimSpace(value.ID)
	if provenanceID == "" {
		provenanceID = domain.NewID("memory_provenance")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO memory_provenance(
		  id,memory_item_id,provenance_kind,source,canonical_evidence_key,source_url,
		  capture_context_json,reason,created_at
		) VALUES(?,?,?,?,?,?,?,?,?)`, provenanceID, itemID, kind, value.Source,
		strings.TrimSpace(value.CanonicalEvidenceKey), strings.TrimSpace(value.SourceURL), contextJSON,
		strings.TrimSpace(value.Reason), createdAt)
	if err != nil {
		return fmt.Errorf("insert memory provenance: %w", err)
	}
	return nil
}

func recordMemoryActionTx(ctx context.Context, tx *sql.Tx, itemID string, action domain.MemoryActionKind, detail map[string]any, now string) error {
	return recordMemoryActionTxWithID(ctx, tx, itemID, action, detail, domain.NewID("memory_action"), now)
}

func recordMemoryActionTxWithID(ctx context.Context, tx *sql.Tx, itemID string, action domain.MemoryActionKind, detail map[string]any, actionID, createdAt string) error {
	if !action.Valid() {
		return fmt.Errorf("unsupported memory action %q", action)
	}
	detailJSON := "{}"
	if detail != nil {
		raw, err := json.Marshal(detail)
		if err != nil {
			return fmt.Errorf("marshal memory action detail: %w", err)
		}
		if len(raw) > maxMemoryActionJSONBytes {
			return fmt.Errorf("memory action detail exceeds %d bytes", maxMemoryActionJSONBytes)
		}
		detailJSON = string(raw)
	}
	if strings.TrimSpace(actionID) == "" {
		actionID = domain.NewID("memory_action")
	}
	if strings.TrimSpace(createdAt) == "" {
		return errors.New("memory action timestamp is required")
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO memory_actions(id,memory_item_id,action,detail_json,created_at)
		VALUES(?,?,?,?,?)`, actionID, itemID, action, detailJSON, createdAt)
	if err != nil {
		return fmt.Errorf("insert memory action: %w", err)
	}
	return nil
}

func tombstonedMemoryID(ctx context.Context, q memoryRowQueryer, key []byte, identity domain.MemoryIdentity) (string, error) {
	var matched string
	for _, alias := range memoryAliases(identity) {
		aliasDigest := memoryAliasDigest(key, identity.Source, alias.kind, alias.value)
		var id string
		err := q.QueryRowContext(ctx, `
			SELECT memory_item_id FROM memory_tombstone_aliases
			WHERE alias_kind=? AND alias_digest=? LIMIT 1`, alias.kind, aliasDigest).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return "", fmt.Errorf("inspect memory tombstone aliases: %w", err)
		}
		if matched != "" && matched != id {
			return "", errors.New("memory identity matches different tombstones")
		}
		matched = id
	}
	identityDigest := memoryIdentityDigest(key, identity)
	var identityID string
	err := q.QueryRowContext(ctx, `
		SELECT id FROM memory_items
		WHERE identity_digest=? AND lifecycle_state='tombstone' LIMIT 1`, identityDigest).Scan(&identityID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("inspect memory tombstones: %w", err)
	}
	if err == nil {
		if matched != "" && matched != identityID {
			return "", errors.New("memory identity matches different tombstones")
		}
		matched = identityID
	}
	return matched, nil
}

func memoryAliasDigest(key []byte, source domain.Source, kind, value string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("aku-memory-tombstone-v1"))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strings.TrimSpace(string(source))))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strings.TrimSpace(kind)))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(mac.Sum(nil))
}

func memoryIdentityDigest(key []byte, identity domain.MemoryIdentity) string {
	value := strings.Join([]string{
		strings.TrimSpace(identity.CanonicalEvidenceKey),
		strings.TrimSpace(identity.CanonicalPermalink),
		strings.TrimSpace(identity.CanonicalPlatformID),
		strings.TrimSpace(identity.ContentFingerprint),
	}, "\x00")
	return memoryAliasDigest(key, identity.Source, "identity", value)
}

func memoryContentFingerprint(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

func memoryNow(s *Store) string {
	return s.Now().UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableNullString(value sql.NullString) any {
	if !value.Valid {
		return nil
	}
	return value.String
}
