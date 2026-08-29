package domain

import "strings"

// MemoryTier describes how much of an item is retained locally. Recall keeps
// metadata and a bounded pointer back to the source; full_copy additionally
// keeps the item's text in memory_content_versions.
type MemoryTier string

const (
	MemoryTierRecall   MemoryTier = "recall"
	MemoryTierFullCopy MemoryTier = "full_copy"
)

func (value MemoryTier) Valid() bool {
	return value == MemoryTierRecall || value == MemoryTierFullCopy
}

// MemoryLifecycleState is intentionally separate from MemoryTier. A deleted
// item remains as an opaque tombstone so a later capture cannot silently
// resurrect the user's deleted memory.
type MemoryLifecycleState string

const (
	MemoryStateActive    MemoryLifecycleState = "active"
	MemoryStateTombstone MemoryLifecycleState = "tombstone"
)

func (value MemoryLifecycleState) Valid() bool {
	return value == MemoryStateActive || value == MemoryStateTombstone
}

type MemoryActionKind string

const (
	MemoryActionCreateStub      MemoryActionKind = "create_stub"
	MemoryActionUpdateStub      MemoryActionKind = "update_stub"
	MemoryActionKeepFullCopy    MemoryActionKind = "keep_full_copy"
	MemoryActionReleaseFullCopy MemoryActionKind = "release_full_copy"
	MemoryActionReadLater       MemoryActionKind = "read_later"
	MemoryActionMarkRead        MemoryActionKind = "mark_read"
	MemoryActionImport          MemoryActionKind = "import"
	MemoryActionDelete          MemoryActionKind = "delete"
)

// Short names keep action call sites readable while the long names remain
// explicit in serialized audit records.
const (
	MemoryActionKeep        = MemoryActionKeepFullCopy
	MemoryActionRelease     = MemoryActionReleaseFullCopy
	MemoryActionKeepFull    = MemoryActionKeepFullCopy
	MemoryActionReleaseFull = MemoryActionReleaseFullCopy
)

func (value MemoryActionKind) Valid() bool {
	switch value {
	case MemoryActionCreateStub,
		MemoryActionUpdateStub,
		MemoryActionKeepFullCopy,
		MemoryActionReleaseFullCopy,
		MemoryActionReadLater,
		MemoryActionMarkRead,
		MemoryActionImport,
		MemoryActionDelete:
		return true
	default:
		return false
	}
}

// MemoryIdentity contains only source-owned identity material. The store
// normalizes the values and stores aliases separately so equivalent captures
// resolve to one memory item.
type MemoryIdentity struct {
	Source               Source `json:"source"`
	CanonicalEvidenceKey string `json:"canonicalEvidenceKey,omitempty"`
	CanonicalPermalink   string `json:"canonicalPermalink,omitempty"`
	// CanonicalURL is accepted as an ergonomic input alias for
	// CanonicalPermalink. The store persists one canonical_permalink value.
	CanonicalURL        string `json:"canonicalUrl,omitempty"`
	CanonicalPlatformID string `json:"canonicalPlatformId,omitempty"`
	ContentFingerprint  string `json:"contentFingerprint,omitempty"`
}

// MemoryMediaReference is metadata only. It deliberately has no binary or
// local-file payload field; media download/storage belongs to a later product
// decision and is outside the Personal Memory foundation.
type MemoryMediaReference struct {
	Kind    string `json:"kind"`
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	AltText string `json:"altText,omitempty"`
}

// MemoryItemInput is the canonical input for creating or updating a recall
// stub. Flat identity fields are retained for callers that do not want to
// construct MemoryIdentity; when both forms are supplied, Identity wins.
type MemoryItemInput struct {
	ID                   string                 `json:"id,omitempty"`
	Identity             MemoryIdentity         `json:"identity"`
	Source               Source                 `json:"source,omitempty"`
	CanonicalEvidenceKey string                 `json:"canonicalEvidenceKey,omitempty"`
	CanonicalPermalink   string                 `json:"canonicalPermalink,omitempty"`
	CanonicalURL         string                 `json:"canonicalUrl,omitempty"`
	CanonicalPlatformID  string                 `json:"canonicalPlatformId,omitempty"`
	ContentFingerprint   string                 `json:"contentFingerprint,omitempty"`
	Title                string                 `json:"title,omitempty"`
	Summary              string                 `json:"summary,omitempty"`
	Author               string                 `json:"author,omitempty"`
	PublishedAt          *string                `json:"publishedAt,omitempty"`
	Tags                 []string               `json:"tags,omitempty"`
	Facets               []string               `json:"facets,omitempty"`
	Media                []MemoryMediaReference `json:"media,omitempty"`
	Reason               string                 `json:"reason,omitempty"`
	Provenance           []MemoryProvenance     `json:"provenance,omitempty"`
}

// MemoryRecallStubInput is an explicit name for the same wire/domain shape.
type MemoryRecallStubInput = MemoryItemInput

// MemoryFullCopyInput is the explicit user decision to retain full text.
type MemoryFullCopyInput struct {
	Content    string                 `json:"content"`
	Media      []MemoryMediaReference `json:"media,omitempty"`
	CapturedAt string                 `json:"capturedAt,omitempty"`
	Reason     string                 `json:"reason,omitempty"`
}

type MemoryItem struct {
	ID                   string                 `json:"id"`
	Identity             MemoryIdentity         `json:"identity"`
	Source               Source                 `json:"source,omitempty"`
	CanonicalEvidenceKey string                 `json:"canonicalEvidenceKey,omitempty"`
	CanonicalPermalink   string                 `json:"canonicalPermalink,omitempty"`
	CanonicalPlatformID  string                 `json:"canonicalPlatformId,omitempty"`
	ContentFingerprint   string                 `json:"contentFingerprint,omitempty"`
	Title                string                 `json:"title,omitempty"`
	Summary              string                 `json:"summary,omitempty"`
	Author               string                 `json:"author,omitempty"`
	PublishedAt          *string                `json:"publishedAt,omitempty"`
	Tags                 []string               `json:"tags,omitempty"`
	Facets               []string               `json:"facets,omitempty"`
	Media                []MemoryMediaReference `json:"media,omitempty"`
	RetentionTier        MemoryTier             `json:"retentionTier"`
	LifecycleState       MemoryLifecycleState   `json:"lifecycleState"`
	FullContentVersionID string                 `json:"fullContentVersionId,omitempty"`
	ContentBytes         int64                  `json:"contentBytes"`
	Reason               string                 `json:"reason,omitempty"`
	FullContent          *string                `json:"fullContent,omitempty"`
	CreatedAt            string                 `json:"createdAt"`
	UpdatedAt            string                 `json:"updatedAt"`
}

type MemoryContentVersion struct {
	ID                 string                 `json:"id"`
	MemoryItemID       string                 `json:"memoryItemId"`
	Version            int                    `json:"version"`
	Content            string                 `json:"content,omitempty"`
	ContentFingerprint string                 `json:"contentFingerprint,omitempty"`
	Media              []MemoryMediaReference `json:"media,omitempty"`
	ContentBytes       int64                  `json:"contentBytes"`
	CapturedAt         string                 `json:"capturedAt"`
	CreatedAt          string                 `json:"createdAt"`
	ReleasedAt         *string                `json:"releasedAt,omitempty"`
}

type MemoryProvenance struct {
	ID                   string         `json:"id"`
	MemoryItemID         string         `json:"memoryItemId"`
	ProvenanceKind       string         `json:"provenanceKind"`
	Source               Source         `json:"source,omitempty"`
	CanonicalEvidenceKey string         `json:"canonicalEvidenceKey,omitempty"`
	SourceURL            string         `json:"sourceUrl,omitempty"`
	CaptureContext       map[string]any `json:"captureContext,omitempty"`
	Reason               string         `json:"reason,omitempty"`
	CreatedAt            string         `json:"createdAt"`
}

type MemoryAction struct {
	ID           string           `json:"id"`
	MemoryItemID string           `json:"memoryItemId"`
	Action       MemoryActionKind `json:"action"`
	Detail       map[string]any   `json:"detail,omitempty"`
	CreatedAt    string           `json:"createdAt"`
}

// MemoryStorageUsage is a logical estimate, not the SQLite file size. It is
// intentionally explicit about payload bytes so storage pressure never treats
// an unknown value as zero.
type MemoryStorageUsage struct {
	ActiveItems     int   `json:"activeItems"`
	Tombstones      int   `json:"tombstones"`
	RecallItems     int   `json:"recallItems"`
	FullCopyItems   int   `json:"fullCopyItems"`
	ContentBytes    int64 `json:"contentBytes"`
	MetadataBytes   int64 `json:"metadataBytes"`
	ProvenanceBytes int64 `json:"provenanceBytes"`
	ActionBytes     int64 `json:"actionBytes"`
	LogicalBytes    int64 `json:"logicalBytes"`
}

// MemoryStorageRecommendation is a bounded, read-only review suggestion for
// an active full-copy item. It intentionally carries only the fields needed
// to decide whether to open the existing Library detail; it is not a claim
// that the item is stale, unused, duplicated, or safe to delete.
type MemoryStorageRecommendation struct {
	ID               string `json:"id"`
	Source           Source `json:"source"`
	Title            string `json:"title,omitempty"`
	Author           string `json:"author,omitempty"`
	ContentBytes     int64  `json:"contentBytes"`
	ReclaimableBytes int64  `json:"reclaimableBytes"`
	UpdatedAt        string `json:"updatedAt"`
	ReasonCode       string `json:"reasonCode"`
	ReviewAction     string `json:"reviewAction"`
}

// MemoryStorageReport combines the logical usage estimate with bounded,
// provider-free review suggestions for the Library storage surface.
type MemoryStorageReport struct {
	Usage           MemoryStorageUsage            `json:"usage"`
	Recommendations []MemoryStorageRecommendation `json:"recommendations"`
}

// MemoryStorage is kept as a descriptive alias for callers that prefer a
// shorter return type name.
type MemoryStorage = MemoryStorageUsage

// MemoryLibraryQuery is the bounded, provider-free read contract for the
// local Personal Memory Library. PublishedAt filters accept RFC3339
// timestamps or YYYY-MM-DD date-only values; date-only upper bounds include
// the entire day.
type MemoryLibraryQuery struct {
	Query         string     `json:"query,omitempty"`
	Source        Source     `json:"source,omitempty"`
	Tier          MemoryTier `json:"tier,omitempty"`
	PublishedFrom string     `json:"publishedFrom,omitempty"`
	PublishedTo   string     `json:"publishedTo,omitempty"`
	Limit         int        `json:"limit,omitempty"`
	Cursor        string     `json:"cursor,omitempty"`
}

// MemoryLibraryResult contains only active memory items. NextCursor is an
// opaque keyset cursor; an empty value means the result is exhausted.
type MemoryLibraryResult struct {
	Items      []MemoryItem `json:"items"`
	NextCursor string       `json:"nextCursor,omitempty"`
}

func (value MemoryProvenance) NormalizedKind() string {
	kind := strings.ToLower(strings.TrimSpace(value.ProvenanceKind))
	switch kind {
	case "captured", "explicit_feedback", "imported", "manual", "unknown":
		return kind
	default:
		return "unknown"
	}
}
