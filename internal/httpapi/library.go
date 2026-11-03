package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
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

func (s *Server) libraryItems(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	query, err := parseLibraryQuery(r.URL.Query())
	if err != nil {
		return err
	}
	result, err := s.engine.Library(ctx, query)
	if err != nil {
		if errors.Is(err, store.ErrMemoryLibraryQuery) {
			return badRequest(err.Error())
		}
		return err
	}
	items := make([]libraryItemView, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, publicLibraryItem(item, false))
	}
	payload := map[string]any{"items": items, "nextCursor": result.NextCursor}
	if libraryTopicKnowledgeRequested(r.URL.Query()) && strings.TrimSpace(query.Query) != "" && query.Cursor == "" && !query.SavedOnly {
		insights, err := s.engine.LibraryTopicKnowledge(ctx, query.Query, libraryTopicKnowledgeLimit)
		if err != nil {
			if errors.Is(err, store.ErrMemoryLibraryQuery) {
				return badRequest(err.Error())
			}
			return err
		}
		knowledge := make([]libraryTopicKnowledgeView, 0, len(insights))
		for _, insight := range insights {
			knowledge = append(knowledge, publicLibraryTopicKnowledge(insight))
		}
		payload["topicKnowledge"] = knowledge
	}
	return writeJSON(w, http.StatusOK, payload)
}

func (s *Server) savedLibraryItems(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	query, err := parseLibraryQuery(r.URL.Query())
	if err != nil {
		return err
	}
	query.SavedOnly = true
	result, err := s.engine.SavedLibrary(ctx, query)
	if err != nil {
		if errors.Is(err, store.ErrMemoryLibraryQuery) {
			return badRequest(err.Error())
		}
		return err
	}
	items := make([]libraryItemView, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, publicLibraryItem(item, false))
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "nextCursor": result.NextCursor})
}

func (s *Server) libraryStorage(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	limit, err := parseLibraryStorageLimit(r.URL.Query())
	if err != nil {
		return err
	}
	report, err := s.engine.LibraryStorage(ctx, limit)
	if errors.Is(err, store.ErrMemoryStorageRecommendationLimit) {
		return badRequest(err.Error())
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, report)
}

func (s *Server) releaseLibraryFullCopy(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/release-full-copy")
	if id == "" || strings.Contains(id, "/") {
		return notFound("library item")
	}
	item, err := s.engine.ReleaseLibraryItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) || errors.Is(err, store.ErrMemoryTombstoned) {
		return notFound("library item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"released": true, "id": id, "retentionTier": item.RetentionTier})
}

func (s *Server) keepLibraryItem(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/keep-in-library")
	if id == "" || strings.Contains(id, "/") {
		return notFound("library item")
	}
	item, err := s.engine.KeepMemoryInLibrary(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) || errors.Is(err, store.ErrMemoryNotSaved) {
		return notFound("library item")
	}
	if errors.Is(err, store.ErrSavedMemoryTextUnavailable) {
		return apiError{Status: http.StatusConflict, Code: "saved_memory_text_unavailable", Message: "The saved source text is unavailable, so it cannot be kept as a full copy."}
	}
	if errors.Is(err, store.ErrMemoryTombstoned) {
		return notFound("library item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"kept": true, "id": id, "saved": item.Saved, "permanentKeep": item.PermanentKeep, "retentionTier": item.RetentionTier})
}

func (s *Server) doneSavedMemory(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/done")
	if id == "" || strings.Contains(id, "/") {
		return notFound("library item")
	}
	item, err := s.engine.DoneSavedMemory(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
		return notFound("library item")
	}
	if errors.Is(err, store.ErrMemoryTombstoned) {
		return notFound("library item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"done": true, "id": id, "saved": item.Saved, "permanentKeep": item.PermanentKeep, "retentionTier": item.RetentionTier})
}

func (s *Server) forgetLibraryItem(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/library/items/"), "/forget-permanently")
	if id == "" || strings.Contains(id, "/") {
		return notFound("library item")
	}
	_, err := s.engine.ForgetLibraryItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
		return notFound("library item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"forgotten": true, "id": id})
}

func (s *Server) removeLibraryItem(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimPrefix(p, "/api/library/items/")
	if id == "" || strings.Contains(id, "/") {
		return notFound("library item")
	}
	err := s.engine.RemoveLibraryItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
		return notFound("library item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"removed": true, "id": id})
}

func (s *Server) libraryItem(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(p)
	if id == "" || id == "items" {
		return notFound("library item")
	}
	item, err := s.engine.LibraryItem(ctx, id)
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, store.ErrMemoryNotFound) {
		return notFound("library item")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"item": publicLibraryItem(item, true)})
}
