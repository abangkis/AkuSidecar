package httpapi

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const (
	libraryHTTPMaxLimit                      = 50
	libraryHTTPMaxCursor                     = 512
	libraryTopicKnowledgeLimit               = 3
	libraryStorageDefaultRecommendationLimit = 6
	libraryStorageMaxRecommendationLimit     = 12
)

// libraryItemView is the public read-only Library projection. Internal
// tombstone state, HMAC identity digests, audit rows, and content-version ids
// never cross this HTTP boundary. FullContent is populated only for detail.
type libraryItemView struct {
	ID                   string                        `json:"id"`
	Source               domain.Source                 `json:"source"`
	CanonicalEvidenceKey string                        `json:"canonicalEvidenceKey,omitempty"`
	CanonicalPermalink   string                        `json:"canonicalPermalink,omitempty"`
	CanonicalPlatformID  string                        `json:"canonicalPlatformId,omitempty"`
	Title                string                        `json:"title,omitempty"`
	Summary              string                        `json:"summary,omitempty"`
	Author               string                        `json:"author,omitempty"`
	PublishedAt          *string                       `json:"publishedAt,omitempty"`
	Tags                 []string                      `json:"tags,omitempty"`
	Facets               []string                      `json:"facets,omitempty"`
	Media                []domain.MemoryMediaReference `json:"media,omitempty"`
	RetentionTier        domain.MemoryTier             `json:"retentionTier"`
	Saved                bool                          `json:"saved"`
	PermanentKeep        bool                          `json:"permanentKeep"`
	FullContent          *string                       `json:"fullContent,omitempty"`
	CreatedAt            string                        `json:"createdAt"`
	UpdatedAt            string                        `json:"updatedAt"`
}

type libraryTopicClaimView struct {
	Text             string `json:"text"`
	Assessment       string `json:"assessment"`
	TemporalStatus   string `json:"temporalStatus"`
	EventStatus      string `json:"eventStatus"`
	LatestEvidenceAt string `json:"latestEvidenceAt,omitempty"`
}

// libraryTopicKnowledgeView exposes only the current supported understanding
// needed for Library discovery. Evidence ids and historical snapshot payloads
// stay behind the Living Topics detail boundary.
type libraryTopicKnowledgeView struct {
	TopicID         string                  `json:"topicId"`
	TopicName       string                  `json:"topicName"`
	Overview        string                  `json:"overview"`
	Claims          []libraryTopicClaimView `json:"claims"`
	SnapshotVersion int                     `json:"snapshotVersion"`
	// UpdatedAt is snapshot regeneration time; EvidenceAsOf is the newest
	// publication time represented by the supplied evidence.
	UpdatedAt     string `json:"updatedAt"`
	EvidenceAsOf  string `json:"evidenceAsOf,omitempty"`
	EvidenceCount int    `json:"evidenceCount"`
	MatchReason   string `json:"matchReason"`
}

func publicLibraryTopicKnowledge(value domain.ContentContextTopicInsight) libraryTopicKnowledgeView {
	claims := make([]libraryTopicClaimView, 0, len(value.Claims))
	for _, claim := range value.Claims {
		if claim.Assessment == "supported" && claim.TemporalStatus == "current" {
			claims = append(claims, libraryTopicClaimView{
				Text: claim.Text, Assessment: claim.Assessment, TemporalStatus: claim.TemporalStatus,
				EventStatus: claim.EventStatus, LatestEvidenceAt: claim.LatestEvidenceAt,
			})
		}
	}
	return libraryTopicKnowledgeView{
		TopicID: value.TopicID, TopicName: value.TopicName, Overview: value.Overview, Claims: claims,
		SnapshotVersion: value.SnapshotVersion, UpdatedAt: value.UpdatedAt, EvidenceAsOf: value.EvidenceAsOf,
		EvidenceCount: value.EvidenceCount, MatchReason: value.MatchReason,
	}
}

func libraryTopicKnowledgeRequested(values url.Values) bool {
	return values.Get("includeTopicKnowledge") == "true"
}

func publicLibraryItem(item domain.MemoryItem, includeFullContent bool) libraryItemView {
	view := libraryItemView{
		ID: item.ID, Source: item.Source,
		CanonicalEvidenceKey: item.CanonicalEvidenceKey,
		CanonicalPermalink:   item.CanonicalPermalink,
		CanonicalPlatformID:  item.CanonicalPlatformID,
		Title:                item.Title, Summary: item.Summary, Author: item.Author,
		PublishedAt: item.PublishedAt, Tags: item.Tags, Facets: item.Facets,
		Media: item.Media, RetentionTier: item.RetentionTier,
		Saved: item.Saved, PermanentKeep: item.PermanentKeep,
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
	if includeFullContent {
		view.FullContent = item.FullContent
	}
	return view
}

func parseLibraryQuery(values url.Values) (domain.MemoryLibraryQuery, error) {
	query := domain.MemoryLibraryQuery{
		Query:         values.Get("query"),
		Source:        domain.Source(values.Get("source")),
		Tier:          domain.MemoryTier(values.Get("tier")),
		SavedOnly:     values.Get("saved") == "true" || values.Get("savedOnly") == "true",
		PublishedFrom: values.Get("publishedFrom"),
		PublishedTo:   values.Get("publishedTo"),
		Cursor:        values.Get("cursor"),
	}
	if query.PublishedFrom == "" {
		query.PublishedFrom = values.Get("from")
	}
	if query.PublishedTo == "" {
		query.PublishedTo = values.Get("to")
	}
	if rawLimit, present := values["limit"]; present {
		if len(rawLimit) != 1 || strings.TrimSpace(rawLimit[0]) == "" {
			return domain.MemoryLibraryQuery{}, badRequest("library limit must be an integer between 1 and 50")
		}
		limit, err := strconv.Atoi(rawLimit[0])
		if err != nil || limit < 1 || limit > libraryHTTPMaxLimit {
			return domain.MemoryLibraryQuery{}, badRequest("library limit must be between 1 and 50")
		}
		query.Limit = limit
	}
	if len(query.Cursor) > libraryHTTPMaxCursor {
		return domain.MemoryLibraryQuery{}, badRequest("library cursor is too long")
	}
	return query, nil
}

func parseLibraryStorageLimit(values url.Values) (int, error) {
	limit := libraryStorageDefaultRecommendationLimit
	rawLimit, present := values["limit"]
	if !present {
		return limit, nil
	}
	if len(rawLimit) != 1 || strings.TrimSpace(rawLimit[0]) == "" {
		return 0, badRequest("storage recommendation limit must be an integer between 1 and 12")
	}
	parsed, err := strconv.Atoi(rawLimit[0])
	if err != nil || parsed < 1 || parsed > libraryStorageMaxRecommendationLimit {
		return 0, badRequest("storage recommendation limit must be between 1 and 12")
	}
	return parsed, nil
}
