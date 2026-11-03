package httpapi

import (
	"fmt"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (s *Server) addTimelineFeedback(w http.ResponseWriter, r *http.Request, p string) error {
	ctx := r.Context()
	started := time.Now()
	traceID := feedbackTraceID(r.Header.Get("X-Aku-Feedback-Trace"))
	w.Header().Set("X-Aku-Feedback-Trace", traceID)
	var timing store.FeedbackTiming
	ctx = store.WithFeedbackTiming(ctx, &timing)
	outcome := "error"
	defer func() {
		if s.logger != nil {
			s.logger.Printf("timeline_feedback_trace trace=%s outcome=%s handler_ms=%d store_ms=%d lookup_ms=%d hydration_ms=%d begin_tx_ms=%d transaction_ms=%d commit_ms=%d pool_wait_count_delta=%d pool_wait_ms_delta=%d", traceID, outcome, time.Since(started).Milliseconds(), timing.Total.Milliseconds(), timing.Lookup.Milliseconds(), timing.Hydration.Milliseconds(), timing.BeginTx.Milliseconds(), timing.Transaction.Milliseconds(), timing.Commit.Milliseconds(), timing.PoolWaitCountDelta, timing.PoolWaitDurationDelta.Milliseconds())
		}
	}()
	id := path.Base(strings.TrimSuffix(p, "/feedback"))
	var value domain.Feedback
	if err := readJSON(r, &value); err != nil {
		return err
	}
	feedback, err := s.engine.AddFeedback(ctx, id, value)
	if err != nil {
		return badRequest(err.Error())
	}
	w.Header().Set("Server-Timing", fmt.Sprintf("feedback;dur=%.1f, lookup;dur=%.1f, hydration;dur=%.1f, begin_tx;dur=%.1f, transaction;dur=%.1f, commit;dur=%.1f", float64(timing.Total.Microseconds())/1000, float64(timing.Lookup.Microseconds())/1000, float64(timing.Hydration.Microseconds())/1000, float64(timing.BeginTx.Microseconds())/1000, float64(timing.Transaction.Microseconds())/1000, float64(timing.Commit.Microseconds())/1000))
	if err := writeJSON(w, http.StatusCreated, map[string]any{"feedback": feedback}); err != nil {
		return err
	}
	outcome = "ok"
	return nil
}

func feedbackTraceID(value string) string {
	if len(value) != 36 {
		return domain.NewID("feedbacktrace")
	}
	for index := 0; index < len(value); index++ {
		character := value[index]
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return domain.NewID("feedbacktrace")
			}
			continue
		}
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return domain.NewID("feedbacktrace")
		}
	}
	return value
}
