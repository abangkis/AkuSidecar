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
	Snapshots   []domain.LivingTopicSnapshot   `json:"snapshots"`
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
	return livingTopicDetailView{Topic: value.Topic, Members: members, Memberships: value.Memberships, Snapshots: value.Snapshots}
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
