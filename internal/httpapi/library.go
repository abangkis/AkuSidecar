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
	FullContent          *string                       `json:"fullContent,omitempty"`
	CreatedAt            string                        `json:"createdAt"`
	UpdatedAt            string                        `json:"updatedAt"`
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
