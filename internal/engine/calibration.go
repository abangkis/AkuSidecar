package engine

import (
	"context"
	"errors"
	"fmt"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/preference"
)

const calibrationMaxItemsPerSource = 5

func (e *Engine) CalibrationOverview(ctx context.Context) (domain.CalibrationOverview, error) {
	if _, err := e.ensurePendingFirstCalibration(ctx, ""); err != nil {
		return domain.CalibrationOverview{}, err
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.CalibrationOverview{}, err
	}
	status, err := e.store.CalibrationFirstRunStatus(ctx)
	if err != nil {
		return domain.CalibrationOverview{}, err
	}
	active, err := e.store.ActiveCalibration(ctx)
	if err != nil {
		return domain.CalibrationOverview{}, err
	}
	profile, err := e.preferenceProfile(ctx, false)
	if err != nil {
		return domain.CalibrationOverview{}, err
	}
	return domain.CalibrationOverview{
		FirstRunStatus: status,
		Active:         active,
		Enabled:        settings.CalibrationEnabled,
		TriggerPolicy:  "first_run_after_first_update",
		BatchSize:      settings.CalibrationBatchSize,
		LiveInfluence:  profile.PromotionReady,
	}, nil
}

func (e *Engine) Calibration(ctx context.Context, id string) (domain.CalibrationSession, error) {
	return e.store.Calibration(ctx, id)
}

func (e *Engine) StartCalibration(ctx context.Context, sessionID, triggerKind string) (domain.CalibrationSession, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	return e.startCalibrationLocked(ctx, sessionID, triggerKind)
}

func (e *Engine) startCalibrationLocked(ctx context.Context, sessionID, triggerKind string) (domain.CalibrationSession, error) {
	if triggerKind == "" {
		triggerKind = "first_run"
	}
	if triggerKind != "first_run" {
		return domain.CalibrationSession{}, errors.New("only first_run calibration is enabled")
	}
	if existing, err := e.store.CalibrationBySession(ctx, sessionID); err != nil {
		return domain.CalibrationSession{}, err
	} else if existing != nil {
		return *existing, nil
	}
	if existing, err := e.store.CalibrationByTrigger(ctx, triggerKind); err != nil {
		return domain.CalibrationSession{}, err
	} else if existing != nil {
		return *existing, nil
	}
	session, err := e.store.GetSession(ctx, sessionID)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	if session.Status != "completed" && session.Status != "partial" {
		return domain.CalibrationSession{}, errors.New("calibration requires a completed or partial unified session")
	}
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	if !settings.CalibrationEnabled {
		return domain.CalibrationSession{}, errors.New("first-run calibration is disabled")
	}
	candidates, err := e.store.CalibrationCandidates(ctx, sessionID)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	samples := sampleCalibrationCandidates(candidates, session.Runs, settings.CalibrationBatchSize)
	if len(samples) == 0 {
		return domain.CalibrationSession{}, errors.New("calibration requires at least one validated candidate")
	}
	return e.store.CreateCalibration(ctx, domain.CalibrationSession{
		ID: domain.NewID("calibration"), UnifiedSessionID: sessionID,
		TriggerKind: triggerKind, MaxItems: settings.CalibrationBatchSize, Samples: samples,
	})
}

func (e *Engine) ensurePendingFirstCalibration(ctx context.Context, sessionID string) (*domain.CalibrationSession, error) {
	e.operation.Lock()
	defer e.operation.Unlock()

	status, err := e.store.CalibrationFirstRunStatus(ctx)
	if err != nil || status != "pending" {
		return nil, err
	}
	if existing, err := e.store.CalibrationByTrigger(ctx, "first_run"); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}
	if sessionID == "" {
		sessionID, err = e.store.LatestCalibrationEligibleSessionID(ctx)
		if err != nil || sessionID == "" {
			return nil, err
		}
	}
	calibration, err := e.startCalibrationLocked(ctx, sessionID, "first_run")
	if err != nil {
		return nil, err
	}
	return &calibration, nil
}

func (e *Engine) DecideCalibration(ctx context.Context, id string, ordinal int, decision domain.CalibrationDecision) (domain.CalibrationSession, error) {
	e.operation.Lock()
	defer e.operation.Unlock()
	if err := decision.Validate(); err != nil {
		return domain.CalibrationSession{}, err
	}
	current, err := e.store.Calibration(ctx, id)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	if current.Status != "reviewing" {
		return domain.CalibrationSession{}, errors.New("calibration session is unavailable or already completed")
	}
	updated, err := e.store.RecordCalibrationDecision(ctx, id, ordinal, decision)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	if updated.ResolvedCount != updated.SampleCount {
		return updated, nil
	}
	snapshot := buildCalibrationSnapshot(updated)
	updated, err = e.store.CompleteCalibration(ctx, id, snapshot)
	if err != nil {
		return domain.CalibrationSession{}, err
	}
	if _, err := e.preferenceProfile(ctx, true); err != nil {
		return domain.CalibrationSession{}, fmt.Errorf("fit preference model after calibration: %w", err)
	}
	return updated, nil
}

func sampleCalibrationCandidates(candidates []domain.CalibrationCandidate, runs []domain.Run, maxItems int) []domain.CalibrationSample {
	bySource := map[domain.Source][]domain.CalibrationCandidate{}
	for _, candidate := range candidates {
		if len(bySource[candidate.Source]) < calibrationMaxItemsPerSource {
			bySource[candidate.Source] = append(bySource[candidate.Source], candidate)
		}
	}
	indexes := map[domain.Source]int{}
	var samples []domain.CalibrationSample
	for len(samples) < maxItems {
		added := false
		for _, run := range runs {
			queue := bySource[run.Source]
			index := indexes[run.Source]
			if index >= len(queue) {
				continue
			}
			candidate := queue[index]
			indexes[run.Source]++
			samples = append(samples, domain.CalibrationSample{
				Ordinal: len(samples), RunID: candidate.RunID, EvidenceKey: candidate.EvidenceKey,
				Source: candidate.Source, Candidate: candidate,
			})
			added = true
			if len(samples) == maxItems {
				break
			}
		}
		if !added {
			break
		}
	}
	return samples
}

func buildCalibrationSnapshot(session domain.CalibrationSession) domain.CalibrationSnapshot {
	labels := map[string]int{"moreLikeThis": 0, "neutral": 0, "lessLikeThis": 0, "captureIssues": 0}
	seenSources := map[domain.Source]bool{}
	var sources []domain.Source
	for _, sample := range session.Samples {
		if sample.IssueCode != nil {
			labels["captureIssues"]++
			continue
		}
		if sample.Label == nil {
			continue
		}
		switch *sample.Label {
		case "more_like_this":
			labels["moreLikeThis"]++
		case "neutral":
			labels["neutral"]++
		case "less_like_this":
			labels["lessLikeThis"]++
		}
		if !seenSources[sample.Source] {
			seenSources[sample.Source] = true
			sources = append(sources, sample.Source)
		}
	}
	return domain.CalibrationSnapshot{
		Version: 0, Origin: "calibration", CalibrationSessionID: session.ID,
		CreatedAt: domain.Now(), Labels: labels, Sources: sources,
		LiveInfluence: false, ActivationState: "feeds_local_fit",
	}
}

func (e *Engine) preferenceProfile(ctx context.Context, persist bool) (preference.Profile, error) {
	signals, err := e.store.PreferenceSignals(ctx)
	if err != nil {
		return preference.Profile{}, err
	}
	converted := make([]preference.Signal, 0, len(signals))
	for _, signal := range signals {
		converted = append(converted, preference.Signal{
			Direction: signal.Direction, Reason: signal.Reason,
			Facets: signal.Assessment.TopicFacets, Origin: signal.Origin,
		})
	}
	profile := preference.Fit(converted)
	if persist {
		if err := e.store.SavePreferenceModel(ctx, profile, len(signals)); err != nil {
			return preference.Profile{}, err
		}
	}
	return profile, nil
}
