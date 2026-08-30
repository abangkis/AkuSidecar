package httpapi

import (
	"io"
	"net/http"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type livingTopicDetailView struct {
	Topic       domain.LivingTopic             `json:"topic"`
	Members     []libraryItemView              `json:"members"`
	Memberships []domain.LivingTopicMembership `json:"memberships"`
	Candidates  []livingTopicCandidateView     `json:"candidates"`
	Snapshots   []domain.LivingTopicSnapshot   `json:"snapshots"`
}

type livingTopicCandidateView struct {
	MemoryItemID     string          `json:"memoryItemId"`
	CriteriaRevision int             `json:"criteriaRevision"`
	Status           string          `json:"status"`
	MatchMode        string          `json:"matchMode"`
	Confidence       float64         `json:"confidence"`
	Reason           string          `json:"reason"`
	CreatedAt        string          `json:"createdAt"`
	UpdatedAt        string          `json:"updatedAt"`
	ReviewedAt       string          `json:"reviewedAt,omitempty"`
	Item             libraryItemView `json:"item"`
}

func publicLivingTopicDetail(value domain.LivingTopicDetail) livingTopicDetailView {
	members := make([]libraryItemView, 0, len(value.Members))
	for _, item := range value.Members {
		members = append(members, publicLibraryItem(item, false))
	}
	if value.Snapshots == nil {
		value.Snapshots = []domain.LivingTopicSnapshot{}
	}
	if value.Memberships == nil {
		value.Memberships = []domain.LivingTopicMembership{}
	}
	candidates := make([]livingTopicCandidateView, 0, len(value.Candidates))
	for _, candidate := range value.Candidates {
		candidates = append(candidates, livingTopicCandidateView{
			MemoryItemID: candidate.MemoryItemID, CriteriaRevision: candidate.CriteriaRevision, Status: candidate.Status,
			MatchMode: candidate.MatchMode, Confidence: candidate.Confidence, Reason: candidate.Reason,
			CreatedAt: candidate.CreatedAt, UpdatedAt: candidate.UpdatedAt, ReviewedAt: candidate.ReviewedAt,
			Item: publicLibraryItem(candidate.Item, false),
		})
	}
	return livingTopicDetailView{Topic: value.Topic, Members: members, Memberships: value.Memberships, Candidates: candidates, Snapshots: value.Snapshots}
}

func requireEmptyBody(r *http.Request) error {
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 2))
	if err != nil {
		return badRequest("request body could not be read")
	}
	if strings.TrimSpace(string(raw)) != "" {
		return badRequest("request body must be empty")
	}
	return nil
}
