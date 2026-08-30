package httpapi

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
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
	Item        libraryItemView `json:"item"`
	MatchReason string          `json:"matchReason"`
}

func publicContentContextMatch(match domain.ContentContextMatch) contentContextMatchView {
	return contentContextMatchView{
		Item:        publicLibraryItem(match.Item, false),
		MatchReason: match.MatchReason,
	}
}
