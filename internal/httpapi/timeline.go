package httpapi

import "net/http"

func (s *Server) timeline(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	limit := boundedInt(r.URL.Query().Get("limit"), 24, 1, 50)
	offset := boundedInt(r.URL.Query().Get("offset"), 0, 0, 100000)
	items, err := s.engine.Timeline(ctx, limit, offset)
	if err != nil {
		return err
	}
	latestCheck, err := s.engine.LatestTimelineCheck(ctx)
	if err != nil {
		return err
	}
	timelineBatches, err := s.engine.TimelineBatchSummaries(ctx)
	if err != nil {
		return err
	}
	autoUpdate, err := s.engine.AutoUpdateStatus(ctx)
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"items": items, "timelineBatches": timelineBatches, "latestCheck": latestCheck, "autoUpdate": autoUpdate})
}
