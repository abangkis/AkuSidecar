package domain

// Content Context is a bounded, local-only lookup from one visible Timeline
// item into Personal Memory. It deliberately returns the existing MemoryItem
// shape so the HTTP layer can apply the same public Library projection.
const (
	ContentContextDefaultLimit = 3
	ContentContextMinLimit     = 1
	ContentContextMaxLimit     = 5
)

type ContentContextMatch struct {
	Item        MemoryItem `json:"item"`
	MatchReason string     `json:"matchReason"`
}

type ContentContextResult struct {
	Matches []ContentContextMatch `json:"matches"`
}
