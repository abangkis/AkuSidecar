package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) visionEvaluationJobs(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	runID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/inbox/runs/"), "/vision-evaluations")
	if runID == "" || strings.Contains(runID, "/") {
		return notFound("run")
	}
	jobs, err := s.engine.VisionEvaluationJobs(ctx, runID)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("run")
	}
	if err != nil {
		return err
	}
	return writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

func (s *Server) retryVisionEvaluation(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	jobID := strings.TrimSuffix(strings.TrimPrefix(p, "/api/vision-evaluations/"), "/retry")
	if jobID == "" || strings.Contains(jobID, "/") {
		return notFound("vision evaluation")
	}
	if err := s.engine.RetryVisionEvaluation(ctx, jobID); errors.Is(err, sql.ErrNoRows) {
		return notFound("vision evaluation")
	} else if err != nil {
		return conflict(err.Error())
	}
	return writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "jobId": jobID})
}
