package domain

// LivingTopic is a manually named, user-owned view over selected Personal
// Memory evidence. Membership is intentionally independent from Saved, Keep,
// preference learning, Timeline selection, and Content Context feedback.
type LivingTopic struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	MemberCount    int                  `json:"memberCount"`
	LatestSnapshot *LivingTopicSnapshot `json:"latestSnapshot,omitempty"`
	CreatedAt      string               `json:"createdAt"`
	UpdatedAt      string               `json:"updatedAt"`
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
	Topic     LivingTopic           `json:"topic"`
	Members   []MemoryItem          `json:"members"`
	Snapshots []LivingTopicSnapshot `json:"snapshots"`
}

type LivingTopicSnapshotResult struct {
	Status   string             `json:"status"`
	Overview string             `json:"overview"`
	Claims   []LivingTopicClaim `json:"claims"`
	Deltas   []LivingTopicDelta `json:"deltas"`
}
