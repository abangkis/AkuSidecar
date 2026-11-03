package httpapi

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
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

func (s *Server) livingTopics(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	topics, err := s.engine.LivingTopics(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"topics": topics})
}

func (s *Server) livingTopicNotifications(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	summary, err := s.engine.LivingTopicNotifications(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"notifications": summary})
}

func (s *Server) createLivingTopic(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	var body struct {
		Name            string   `json:"name"`
		Description     string   `json:"description"`
		Aliases         []string `json:"aliases"`
		IncludeCriteria string   `json:"includeCriteria"`
		ExcludeCriteria string   `json:"excludeCriteria"`
	}
	if err := readJSON(r, &body); err != nil {
		return err
	}
	topic, err := s.engine.CreateLivingTopicWithRoutingCriteria(ctx, domain.LivingTopicCriteriaInput{Name: body.Name, Description: body.Description, Aliases: body.Aliases, IncludeCriteria: body.IncludeCriteria, ExcludeCriteria: body.ExcludeCriteria})
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusCreated, map[string]any{"topic": topic})
}

func (s *Server) undoLivingTopicMove(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	moveID := path.Base(strings.TrimSuffix(p, "/undo"))
	if err := requireEmptyBody(r); err != nil {
		return err
	}
	move, err := s.engine.UndoLivingTopicMemberMove(ctx, moveID)
	if errors.Is(err, store.ErrLivingTopicMoveNotFound) {
		return notFound("living topic move")
	}
	if errors.Is(err, store.ErrLivingTopicMoveNotCurrent) || errors.Is(err, store.ErrLivingTopicMoveSourceConflict) || errors.Is(err, store.ErrLivingTopicMemberMax) {
		return conflict(err.Error())
	}
	if errors.Is(err, store.ErrMemoryNotFound) {
		return notFound("library item")
	}
	if err != nil {
		return badRequest(err.Error())
	}
	return writeJSON(w, http.StatusOK, map[string]any{"move": move})
}

func (s *Server) livingTopicRoute(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	parts := strings.Split(strings.Trim(strings.TrimPrefix(p, "/api/living-topics/"), "/"), "/")
	if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodGet {
		detail, err := s.engine.LivingTopic(ctx, parts[0])
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, publicLivingTopicDetail(detail))
	}
	if len(parts) == 1 && parts[0] != "" && r.Method == http.MethodPatch {
		var body struct {
			Name            string   `json:"name"`
			Description     string   `json:"description"`
			Aliases         []string `json:"aliases"`
			IncludeCriteria string   `json:"includeCriteria"`
			ExcludeCriteria string   `json:"excludeCriteria"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		topic, err := s.engine.UpdateLivingTopicRoutingCriteria(ctx, parts[0], domain.LivingTopicCriteriaInput{Name: body.Name, Description: body.Description, Aliases: body.Aliases, IncludeCriteria: body.IncludeCriteria, ExcludeCriteria: body.ExcludeCriteria})
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"topic": topic})
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "members" && r.Method == http.MethodPost {
		var body struct {
			MemoryItemID string `json:"memoryItemId"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		detail, err := s.engine.AddLivingTopicMember(ctx, parts[0], body.MemoryItemID)
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if errors.Is(err, store.ErrLivingTopicMemberMax) {
			return conflict(err.Error())
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, publicLivingTopicDetail(detail))
	}
	if len(parts) == 3 && parts[0] != "" && parts[1] == "members" && parts[2] != "" && r.Method == http.MethodDelete {
		detail, err := s.engine.RemoveLivingTopicMember(ctx, parts[0], parts[2])
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if err != nil {
			return err
		}
		return writeJSON(w, http.StatusOK, publicLivingTopicDetail(detail))
	}
	if len(parts) == 4 && parts[0] != "" && parts[1] == "members" && parts[2] != "" && parts[3] == "move" && r.Method == http.MethodPost {
		var body struct {
			ToTopicID string `json:"toTopicId"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		move, err := s.engine.MoveLivingTopicMember(ctx, parts[0], body.ToTopicID, parts[2])
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if errors.Is(err, store.ErrLivingTopicMoveNotFound) {
			return notFound("living topic membership")
		}
		if errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if errors.Is(err, store.ErrLivingTopicMemberMax) || errors.Is(err, store.ErrLivingTopicMoveSameTopic) {
			return conflict(err.Error())
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"move": move})
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "snapshots" && r.Method == http.MethodPost {
		if err := requireEmptyBody(r); err != nil {
			return err
		}
		detail, err := s.engine.RequestLivingTopicUnderstanding(ctx, parts[0], "refresh_now")
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"detail": publicLivingTopicDetail(detail)})
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "activation" && r.Method == http.MethodPost {
		if err := requireEmptyBody(r); err != nil {
			return err
		}
		detail, err := s.engine.RequestLivingTopicActivation(ctx, parts[0], "rescan_now")
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if err != nil {
			return conflict(err.Error())
		}
		return writeJSON(w, http.StatusAccepted, map[string]any{"detail": publicLivingTopicDetail(detail)})
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "seen" && r.Method == http.MethodPost {
		var body struct {
			SeenThrough string `json:"seenThrough"`
		}
		if err := readJSON(r, &body); err != nil {
			return err
		}
		topic, err := s.engine.AcknowledgeLivingTopicEvidence(ctx, parts[0], body.SeenThrough)
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, map[string]any{"topic": topic})
	}
	if len(parts) == 4 && parts[0] != "" && parts[1] == "candidates" && parts[2] != "" && r.Method == http.MethodPost {
		if err := requireEmptyBody(r); err != nil {
			return err
		}
		action := parts[3]
		if action != "accept" && action != "reject" && action != "undo" {
			return notFound("living topic candidate action")
		}
		detail, err := s.engine.ReviewLivingTopicCandidate(ctx, parts[0], parts[2], action)
		if errors.Is(err, store.ErrLivingTopicNotFound) {
			return notFound("living topic")
		}
		if errors.Is(err, store.ErrLivingTopicCandidateNotFound) {
			return notFound("living topic candidate")
		}
		if errors.Is(err, store.ErrMemoryNotFound) {
			return notFound("library item")
		}
		if errors.Is(err, store.ErrLivingTopicMemberMax) || errors.Is(err, store.ErrLivingTopicCandidateReview) {
			return conflict(err.Error())
		}
		if err != nil {
			return badRequest(err.Error())
		}
		return writeJSON(w, http.StatusOK, publicLivingTopicDetail(detail))
	}
	return notFound("living topic route")
}
