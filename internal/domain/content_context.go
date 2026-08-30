package domain

import (
	"errors"
	"strings"
)

// Content Context is a bounded, local-only lookup from one visible Timeline
// item into Personal Memory. It deliberately returns the existing MemoryItem
// shape so the HTTP layer can apply the same public Library projection.
const (
	ContentContextDefaultLimit  = 3
	ContentContextMinLimit      = 1
	ContentContextMaxLimit      = 5
	ContentContextEngineVersion = "content-context-v2"
)

// ContentContextUpScrollMode controls what happens when the active Timeline
// item leaves the viewport while the user scrolls upward. The drawer remains
// anchored to its post in both modes; preserve only keeps its active state.
type ContentContextUpScrollMode string

const (
	ContentContextUpScrollModeCloseOffscreen ContentContextUpScrollMode = "close_offscreen"
	ContentContextUpScrollModePreserve       ContentContextUpScrollMode = "preserve"
	DefaultContentContextUpScrollMode        ContentContextUpScrollMode = ContentContextUpScrollModeCloseOffscreen
)

func (m ContentContextUpScrollMode) Valid() bool {
	return m == ContentContextUpScrollModeCloseOffscreen || m == ContentContextUpScrollModePreserve
}

type ContentContextMatch struct {
	Item        MemoryItem                   `json:"item"`
	MatchReason string                       `json:"matchReason"`
	Feedback    *ContentContextFeedbackState `json:"feedback,omitempty"`
}

type ContentContextResult struct {
	Matches []ContentContextMatch `json:"matches"`
}

type ContentContextFeedbackVerdict string

const (
	ContentContextFeedbackRelevant    ContentContextFeedbackVerdict = "relevant"
	ContentContextFeedbackNotRelevant ContentContextFeedbackVerdict = "not_relevant"
	ContentContextFeedbackClear       ContentContextFeedbackVerdict = "clear"
)

func (v ContentContextFeedbackVerdict) ValidDecision() bool {
	return v == ContentContextFeedbackRelevant || v == ContentContextFeedbackNotRelevant
}

type ContentContextFeedbackInput struct {
	MemoryItemID string                        `json:"memoryItemId"`
	Verdict      ContentContextFeedbackVerdict `json:"verdict"`
}

func (value ContentContextFeedbackInput) Validate() error {
	if strings.TrimSpace(value.MemoryItemID) == "" {
		return errors.New("memoryItemId is required")
	}
	if !value.Verdict.ValidDecision() {
		return errors.New("content context verdict must be relevant or not_relevant")
	}
	return nil
}

// ContentContextFeedbackEvent is append-only pairwise evidence. ContextKey is
// internal because it may contain a canonical Timeline evidence identity.
type ContentContextFeedbackEvent struct {
	ID            string                        `json:"id"`
	TimelineID    string                        `json:"timelineId"`
	ContextKey    string                        `json:"-"`
	MemoryItemID  string                        `json:"memoryItemId"`
	Verdict       ContentContextFeedbackVerdict `json:"verdict"`
	EngineVersion string                        `json:"engineVersion"`
	ResultRank    int                           `json:"resultRank"`
	MatchReason   string                        `json:"matchReason"`
	SupersedesID  string                        `json:"supersedesId,omitempty"`
	CreatedAt     string                        `json:"createdAt"`
}

type ContentContextFeedbackState struct {
	ID      string                        `json:"id"`
	Verdict ContentContextFeedbackVerdict `json:"verdict"`
}
