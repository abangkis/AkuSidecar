package domain

// Content Context is a bounded, local-only lookup from one visible Timeline
// item into Personal Memory. It deliberately returns the existing MemoryItem
// shape so the HTTP layer can apply the same public Library projection.
const (
	ContentContextDefaultLimit = 3
	ContentContextMinLimit     = 1
	ContentContextMaxLimit     = 5
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
	Item        MemoryItem `json:"item"`
	MatchReason string     `json:"matchReason"`
}

type ContentContextResult struct {
	Matches []ContentContextMatch `json:"matches"`
}
