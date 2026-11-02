package engine

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/aidetector"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (e *Engine) SetAIDeepResolver(value aidetector.Resolver) { e.aiDeep = value }

func (e *Engine) runFastDetection(ctx context.Context, sessionID string) error {
	items, err := e.store.ListSessionItems(ctx, sessionID)
	if err != nil {
		return err
	}
	return e.store.SaveAIAssessments(ctx, e.aiFast.Detect(items))
}

func (e *Engine) launchDeepDetection(sessionID string) {
	items, err := e.store.ListSessionItems(context.Background(), sessionID)
	if err != nil || len(items) == 0 {
		if err != nil {
			e.logger.Printf("AI Deep Detection could not load session %s: %v", sessionID, err)
		}
		return
	}
	e.launchDeepDetectionItems(sessionID, items)
}

func (e *Engine) launchDeepDetectionItems(sessionID string, items []domain.TimelineItem) {
	resolver := e.aiDeep
	if resolver == nil {
		return
	}
	items = aidetector.DeepCandidates(items)
	if len(items) == 0 {
		return
	}
	e.mu.Lock()
	for id, cancel := range e.work.active {
		if strings.HasPrefix(id, "ai:"+sessionID+":") {
			cancel()
		}
	}
	e.mu.Unlock()
	settings, settingsErr := e.store.GetSettings(context.Background())
	if settingsErr != nil {
		e.logger.Printf("AI Deep Detection could not load reasoning profile for session %s: %v", sessionID, settingsErr)
		return
	}
	if !settings.AIDetectionEnabled {
		return
	}
	model := resolver.Model()
	if profiled, ok := resolver.(aidetector.ProfiledResolver); ok {
		model = profiled.ModelForProfile(settings.ReasoningAIDeepProfile)
	}
	job, err := e.store.CreateAIDetectionJob(context.Background(), domain.AIDetectionJob{
		SessionID: sessionID, Status: "queued", Provider: resolver.Name(), Model: model.Model,
		Effort: model.Effort, CandidateCount: len(items),
	})
	if err != nil {
		e.logger.Printf("AI Deep Detection could not queue session %s: %v", sessionID, err)
		return
	}
	key := "ai:" + sessionID + ":" + job.ID
	ctx, cancel := context.WithCancel(context.Background())
	e.mu.Lock()
	e.work.registerActive(key, cancel)
	e.mu.Unlock()
	go func() {
		defer func() {
			e.mu.Lock()
			e.work.retireActive(key)
			e.mu.Unlock()
		}()
		if err := e.store.StartAIDetectionJob(ctx, job.ID); err != nil {
			status := "failed"
			if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				status = "cancelled"
			}
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, status, 0, domain.ModelUsage{}, err)
			if status != "cancelled" {
				e.logger.Printf("AI Deep Detection could not start session %s: %v", sessionID, err)
			}
			return
		}
		var result domain.DeepAIResult
		var usage domain.ModelUsage
		var duration time.Duration
		var resolveErr error
		if profiled, ok := resolver.(aidetector.ProfiledResolver); ok {
			result, usage, duration, resolveErr = profiled.ResolveWithProfile(ctx, items, settings.ReasoningAIDeepProfile)
		} else {
			result, usage, duration, resolveErr = resolver.Resolve(ctx, items)
		}
		if resolveErr != nil {
			status := "failed"
			if errors.Is(resolveErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				status = "cancelled"
			}
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, status, duration.Milliseconds(), usage, resolveErr)
			if status != "cancelled" {
				e.logger.Printf("AI Deep Detection for session %s degraded safely: %v", sessionID, resolveErr)
				e.pauseAutoUpdateForUsageLimit(context.Background(), sessionID, resolveErr)
			}
			return
		}
		assessedAt := domain.Now()
		assessments := make([]domain.AIAssessment, 0, len(items))
		for index, value := range result.Assessments {
			supersedes := ""
			if items[index].AIDetection != nil {
				supersedes = items[index].AIDetection.AssessmentID
			}
			assessments = append(assessments, domain.AIAssessment{
				ID: domain.NewID("ai_assessment"), TimelineID: items[index].ID, SessionID: sessionID,
				Stage: "deep", Status: value.Status, ConfidenceBand: value.ConfidenceBand,
				EvidenceCodes: value.EvidenceCodes, AssessedObject: value.AssessedObject, SignalScope: value.SignalScope,
				Provider: resolver.Name(), DetectorVersion: aidetector.DeepDetectorVersion,
				Rationale: value.Rationale, SupersedesID: supersedes, CreatedAt: assessedAt,
			})
		}
		if err := e.store.SaveAIAssessments(context.Background(), assessments); err != nil {
			_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, "failed", duration.Milliseconds(), usage, err)
			e.logger.Printf("AI Deep Detection could not persist session %s: %v", sessionID, err)
			return
		}
		_ = e.store.FinishAIDetectionJob(context.Background(), job.ID, "completed", duration.Milliseconds(), usage, nil)
	}()
}

func (e *Engine) cancelDeepDetections() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id, cancel := range e.work.active {
		if strings.HasPrefix(id, "ai:") || id == "media-provenance" {
			cancel()
		}
	}
}
