package domain

// LivingTopic is a manually named, user-owned view over criteria-routed and
// explicitly selected Personal Memory evidence. Membership is intentionally
// independent from Saved, Keep, preference learning, Timeline selection, and
// Content Context feedback.
type LivingTopic struct {
	ID                       string               `json:"id"`
	Name                     string               `json:"name"`
	Description              string               `json:"description"`
	CriteriaRevision         int                  `json:"criteriaRevision"`
	Aliases                  []string             `json:"aliases"`
	IncludeCriteria          string               `json:"includeCriteria"`
	ExcludeCriteria          string               `json:"excludeCriteria"`
	MemberCount              int                  `json:"memberCount"`
	SuggestedCount           int                  `json:"suggestedCount"`
	RoutingStatus            string               `json:"routingStatus"`
	RoutingCheckedAt         string               `json:"routingCheckedAt,omitempty"`
	RoutingLastError         string               `json:"routingLastError,omitempty"`
	NewEvidenceCount         int                  `json:"newEvidenceCount"`
	NewEvidenceAt            string               `json:"newEvidenceAt,omitempty"`
	EvidenceSeenAt           string               `json:"evidenceSeenAt,omitempty"`
	LatestSnapshot           *LivingTopicSnapshot `json:"latestSnapshot,omitempty"`
	UnderstandingStatus      string               `json:"understandingStatus"`
	UnderstandingCheckedAt   string               `json:"understandingCheckedAt,omitempty"`
	UnderstandingTrigger     string               `json:"understandingTrigger,omitempty"`
	UnderstandingLastError   string               `json:"understandingLastError,omitempty"`
	UnderstandingInputDigest string               `json:"-"`
	CreatedAt                string               `json:"createdAt"`
	UpdatedAt                string               `json:"updatedAt"`
	// RoutingContext is host-populated with a bounded, newest-first selection
	// of attached evidence. It is an internal resolver input and must never be
	// serialized as part of the public topic contract.
	RoutingContext []MemoryItem `json:"-"`
}

type LivingTopicNotificationSummary struct {
	NewEvidenceCount      int    `json:"newEvidenceCount"`
	TopicsWithNewEvidence int    `json:"topicsWithNewEvidence"`
	LatestEvidenceAt      string `json:"latestEvidenceAt,omitempty"`
}

type LivingTopicCriteriaInput struct {
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Aliases         []string `json:"aliases"`
	IncludeCriteria string   `json:"includeCriteria"`
	ExcludeCriteria string   `json:"excludeCriteria"`
}

// LivingTopicMembership keeps automatic routing explainable without changing
// the Personal Memory item contract used by Library and snapshots.
type LivingTopicMembership struct {
	MemoryItemID string  `json:"memoryItemId"`
	Origin       string  `json:"origin"`
	MatchMode    string  `json:"matchMode"`
	Confidence   float64 `json:"confidence"`
	Reason       string  `json:"reason"`
	AddedAt      string  `json:"addedAt"`
	MoveID       string  `json:"moveId,omitempty"`
}

// LivingTopicMembershipMove is a reversible correction receipt. It preserves
// membership provenance but never owns or copies the underlying Memory item.
type LivingTopicMembershipMove struct {
	ID                  string  `json:"id"`
	MemoryItemID        string  `json:"memoryItemId"`
	FromTopicID         string  `json:"fromTopicId"`
	ToTopicID           string  `json:"toTopicId"`
	SourceOrigin        string  `json:"sourceOrigin"`
	SourceMatchMode     string  `json:"sourceMatchMode"`
	SourceConfidence    float64 `json:"sourceConfidence"`
	SourceReason        string  `json:"sourceReason"`
	SourceAddedAt       string  `json:"sourceAddedAt"`
	SourceNewEvidence   bool    `json:"sourceNewEvidence"`
	SourceNewEvidenceAt string  `json:"sourceNewEvidenceAt,omitempty"`
	SourceMoveID        string  `json:"sourceMoveId,omitempty"`
	TargetPreexisted    bool    `json:"targetPreexisted"`
	CreatedAt           string  `json:"createdAt"`
	UndoneAt            string  `json:"undoneAt,omitempty"`
}

type LivingTopicRoutingFeedback struct {
	TopicID      string `json:"topicId"`
	MemoryItemID string `json:"memoryItemId"`
	Verdict      string `json:"verdict"`
	CreatedAt    string `json:"createdAt"`
}

type LivingTopicRoutingDecision struct {
	TopicID    string  `json:"topicId"`
	Match      bool    `json:"match"`
	Confidence float64 `json:"confidence"`
	Mode       string  `json:"mode"`
	Reason     string  `json:"reason"`
}

type LivingTopicRoutingExample struct {
	TopicID string     `json:"topicId"`
	Verdict string     `json:"verdict"`
	Item    MemoryItem `json:"item"`
}

type LivingTopicRoutingJob struct {
	ID         string `json:"id"`
	SessionID  string `json:"sessionId"`
	TimelineID string `json:"timelineId"`
}

type LivingTopicActivationJob struct {
	ID               string `json:"id"`
	TopicID          string `json:"topicId"`
	CriteriaRevision int    `json:"criteriaRevision"`
	Trigger          string `json:"trigger"`
}

// LivingTopicCandidate is a bounded, local-only suggestion. It does not
// become topic evidence until the user accepts it.
type LivingTopicCandidate struct {
	TopicID          string     `json:"topicId"`
	MemoryItemID     string     `json:"memoryItemId"`
	CriteriaRevision int        `json:"criteriaRevision"`
	Status           string     `json:"status"`
	MatchMode        string     `json:"matchMode"`
	Confidence       float64    `json:"confidence"`
	Reason           string     `json:"reason"`
	CreatedAt        string     `json:"createdAt"`
	UpdatedAt        string     `json:"updatedAt"`
	ReviewedAt       string     `json:"reviewedAt,omitempty"`
	Item             MemoryItem `json:"item"`
}

type LivingTopicUnderstandingJob struct {
	ID      string `json:"id"`
	TopicID string `json:"topicId"`
	Trigger string `json:"trigger"`
}

type LivingTopicClaim struct {
	Key              string   `json:"key,omitempty"`
	MaterialValue    string   `json:"materialValue,omitempty"`
	Text             string   `json:"text"`
	Assessment       string   `json:"assessment"`
	Centrality       string   `json:"centrality,omitempty"`
	Subtopic         string   `json:"subtopic,omitempty"`
	TemporalStatus   string   `json:"temporalStatus"`
	EventStatus      string   `json:"eventStatus"`
	LatestEvidenceAt string   `json:"latestEvidenceAt,omitempty"`
	EvidenceIDs      []string `json:"evidenceIds"`
}

// LivingTopicEvidenceRole is topic-relative. Membership answers whether an
// item belongs to a topic; this role answers how much that item may shape the
// topic's current understanding.
type LivingTopicEvidenceRole struct {
	MemoryItemID   string `json:"memoryItemId"`
	Role           string `json:"role"`
	Subtopic       string `json:"subtopic,omitempty"`
	SourceCluster  string `json:"sourceCluster,omitempty"`
	EpistemicClass string `json:"epistemicClass,omitempty"`
}

type LivingTopicDelta struct {
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type LivingTopicSnapshot struct {
	ID                      string                    `json:"id"`
	TopicID                 string                    `json:"topicId"`
	Version                 int                       `json:"version"`
	Status                  string                    `json:"status"`
	Overview                string                    `json:"overview"`
	Claims                  []LivingTopicClaim        `json:"claims"`
	Deltas                  []LivingTopicDelta        `json:"deltas"`
	EvidenceIDs             []string                  `json:"evidenceIds"`
	EvidenceRoles           []LivingTopicEvidenceRole `json:"evidenceRoles,omitempty"`
	InputDigest             string                    `json:"-"`
	ContractVersion         string                    `json:"contractVersion,omitempty"`
	MaterialChange          bool                      `json:"materialChange"`
	CoverageState           string                    `json:"coverageState,omitempty"`
	SourceDiversityState    string                    `json:"sourceDiversityState,omitempty"`
	SourcePlatformCount     int                       `json:"sourcePlatformCount"`
	IndependentSourceCount  int                       `json:"independentSourceCount"`
	IndependentClusterCount int                       `json:"independentClusterCount"`
	Provider                string                    `json:"provider"`
	Model                   string                    `json:"model"`
	Effort                  string                    `json:"effort"`
	DurationMS              int64                     `json:"durationMs"`
	Usage                   ModelUsage                `json:"usage"`
	PreviousSnapshotID      string                    `json:"previousSnapshotId,omitempty"`
	CreatedAt               string                    `json:"createdAt"`
	IsCurrent               bool                      `json:"isCurrent"`
	ActiveEvidenceCount     int                       `json:"activeEvidenceCount"`
	EvidenceAvailability    string                    `json:"evidenceAvailability"`
	EvidenceAsOf            string                    `json:"evidenceAsOf,omitempty"`
}

type LivingTopicDetail struct {
	Topic       LivingTopic             `json:"topic"`
	Members     []MemoryItem            `json:"members"`
	Memberships []LivingTopicMembership `json:"memberships"`
	Candidates  []LivingTopicCandidate  `json:"candidates"`
	Snapshots   []LivingTopicSnapshot   `json:"snapshots"`
}

type LivingTopicSnapshotResult struct {
	Status        string                    `json:"status"`
	Overview      string                    `json:"overview"`
	Claims        []LivingTopicClaim        `json:"claims"`
	Deltas        []LivingTopicDelta        `json:"deltas"`
	EvidenceRoles []LivingTopicEvidenceRole `json:"evidenceRoles"`
	CoverageState string                    `json:"coverageState"`
}
