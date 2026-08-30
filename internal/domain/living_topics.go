package domain

// LivingTopic is a manually named, user-owned view over criteria-routed and
// explicitly selected Personal Memory evidence. Membership is intentionally
// independent from Saved, Keep, preference learning, Timeline selection, and
// Content Context feedback.
type LivingTopic struct {
	ID                       string               `json:"id"`
	Name                     string               `json:"name"`
	Description              string               `json:"description"`
	MemberCount              int                  `json:"memberCount"`
	LatestSnapshot           *LivingTopicSnapshot `json:"latestSnapshot,omitempty"`
	UnderstandingStatus      string               `json:"understandingStatus"`
	UnderstandingCheckedAt   string               `json:"understandingCheckedAt,omitempty"`
	UnderstandingTrigger     string               `json:"understandingTrigger,omitempty"`
	UnderstandingLastError   string               `json:"understandingLastError,omitempty"`
	UnderstandingInputDigest string               `json:"-"`
	CreatedAt                string               `json:"createdAt"`
	UpdatedAt                string               `json:"updatedAt"`
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

type LivingTopicUnderstandingJob struct {
	ID      string `json:"id"`
	TopicID string `json:"topicId"`
	Trigger string `json:"trigger"`
}

type LivingTopicClaim struct {
	Text        string   `json:"text"`
	Assessment  string   `json:"assessment"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type LivingTopicDelta struct {
	Kind        string   `json:"kind"`
	Text        string   `json:"text"`
	EvidenceIDs []string `json:"evidenceIds"`
}

type LivingTopicSnapshot struct {
	ID                 string             `json:"id"`
	TopicID            string             `json:"topicId"`
	Version            int                `json:"version"`
	Status             string             `json:"status"`
	Overview           string             `json:"overview"`
	Claims             []LivingTopicClaim `json:"claims"`
	Deltas             []LivingTopicDelta `json:"deltas"`
	EvidenceIDs        []string           `json:"evidenceIds"`
	InputDigest        string             `json:"-"`
	Provider           string             `json:"provider"`
	Model              string             `json:"model"`
	Effort             string             `json:"effort"`
	DurationMS         int64              `json:"durationMs"`
	Usage              ModelUsage         `json:"usage"`
	PreviousSnapshotID string             `json:"previousSnapshotId,omitempty"`
	CreatedAt          string             `json:"createdAt"`
}

type LivingTopicDetail struct {
	Topic       LivingTopic             `json:"topic"`
	Members     []MemoryItem            `json:"members"`
	Memberships []LivingTopicMembership `json:"memberships"`
	Snapshots   []LivingTopicSnapshot   `json:"snapshots"`
}

type LivingTopicSnapshotResult struct {
	Status   string             `json:"status"`
	Overview string             `json:"overview"`
	Claims   []LivingTopicClaim `json:"claims"`
	Deltas   []LivingTopicDelta `json:"deltas"`
}
