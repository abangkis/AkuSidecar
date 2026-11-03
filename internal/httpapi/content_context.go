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

func parseContentContextLimit(values url.Values) (int, error) {
	raw, present := values["limit"]
	if !present {
		return domain.ContentContextDefaultLimit, nil
	}
	if len(raw) != 1 || strings.TrimSpace(raw[0]) == "" {
		return 0, badRequest("content context limit must be an integer between 1 and 5")
	}
	limit, err := strconv.Atoi(raw[0])
	if err != nil || limit < domain.ContentContextMinLimit || limit > domain.ContentContextMaxLimit {
		return 0, badRequest("content context limit must be between 1 and 5")
	}
	return limit, nil
}

type contentContextMatchView struct {
	Item        libraryItemView                     `json:"item"`
	MatchReason string                              `json:"matchReason"`
	Feedback    *domain.ContentContextFeedbackState `json:"feedback,omitempty"`
}

func publicContentContextMatch(match domain.ContentContextMatch) contentContextMatchView {
	return contentContextMatchView{
		Item:        publicLibraryItem(match.Item, false),
		MatchReason: match.MatchReason,
		Feedback:    match.Feedback,
	}
}

type contentContextTopicInsightView struct {
	TopicID         string                    `json:"topicId"`
	TopicName       string                    `json:"topicName"`
	Overview        string                    `json:"overview"`
	Claims          []domain.LivingTopicClaim `json:"claims"`
	SnapshotVersion int                       `json:"snapshotVersion"`
	UpdatedAt       string                    `json:"updatedAt"`
	EvidenceAsOf    string                    `json:"evidenceAsOf,omitempty"`
	EvidenceCount   int                       `json:"evidenceCount"`
	MatchReason     string                    `json:"matchReason"`
}

func publicContentContextTopicInsight(value domain.ContentContextTopicInsight) contentContextTopicInsightView {
	return contentContextTopicInsightView{
		TopicID: value.TopicID, TopicName: value.TopicName, Overview: value.Overview, Claims: value.Claims,
		SnapshotVersion: value.SnapshotVersion, UpdatedAt: value.UpdatedAt, EvidenceAsOf: value.EvidenceAsOf,
		EvidenceCount: value.EvidenceCount, MatchReason: value.MatchReason,
	}
}

func (s *Server) contentContext(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := strings.TrimSuffix(strings.TrimPrefix(p, "/api/timeline/"), "/content-context")
	if id == "" || strings.Contains(id, "/") {
		return notFound("timeline item")
	}
	limit, err := parseContentContextLimit(r.URL.Query())
	if err != nil {
		return err
	}
	result, err := s.engine.ContentContext(ctx, id, limit)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("timeline item")
	}
	if errors.Is(err, store.ErrContentContextNotEligible) {
		return apiError{Status: http.StatusConflict, Code: "timeline_content_context_not_eligible", Message: "Only a final visible Timeline item can find related local context."}
	}
	if err != nil {
		return err
	}
	matches := make([]contentContextMatchView, 0, len(result.Matches))
	for _, match := range result.Matches {
		matches = append(matches, publicContentContextMatch(match))
	}
	topicInsights := make([]contentContextTopicInsightView, 0, len(result.TopicInsights))
	for _, insight := range result.TopicInsights {
		topicInsights = append(topicInsights, publicContentContextTopicInsight(insight))
	}
	direct := result.DirectContext
	if direct == nil {
		direct = []domain.DirectContext{}
	}
	if len(direct) > domain.DirectContextLimit {
		direct = direct[:domain.DirectContextLimit]
	}
	return writeJSON(w, http.StatusOK, map[string]any{"matches": matches, "topicInsights": topicInsights, "directContext": direct})
}

func (s *Server) addContentContextFeedback(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/content-context-feedback"))
	var body domain.ContentContextFeedbackInput
	if err := readJSON(r, &body); err != nil {
		return err
	}
	feedback, err := s.engine.AddContentContextFeedback(ctx, id, body)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("Timeline item or related context")
	}
	if errors.Is(err, store.ErrContentContextNotEligible) {
		return apiError{Status: http.StatusConflict, Code: "timeline_content_context_not_eligible", Message: "Only a final visible Timeline item can receive related context feedback."}
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"feedback": feedback})
}

func (s *Server) undoContentContextFeedback(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	id := path.Base(strings.TrimSuffix(p, "/undo"))
	feedback, err := s.engine.UndoContentContextFeedback(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("content context feedback")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"feedback": feedback})
}
