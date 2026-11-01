package engine

import (
	"context"
	"fmt"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func (e *Engine) DiagnosticsExport(ctx context.Context, since, until string, sessionID string) (domain.DiagnosticsExport, error) {
	var sinceTime, untilTime time.Time
	var err error
	if since != "" {
		sinceTime, err = time.Parse(time.RFC3339, since)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
	}
	if until != "" {
		untilTime, err = time.Parse(time.RFC3339, until)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
	} else {
		untilTime = time.Now().UTC()
	}
	if sinceTime.IsZero() {
		sinceTime = untilTime.Add(-24 * time.Hour)
	}

	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return domain.DiagnosticsExport{}, err
	}

	activeSession, _ := e.ActiveSession(ctx)
	var targetSession *domain.Session
	if sessionID != "" {
		s, err := e.Session(ctx, sessionID)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
		targetSession = &s
	} else if activeSession != nil {
		targetSession = activeSession
	}

	var sessions []domain.SessionExport
	if targetSession != nil {
		sessions = []domain.SessionExport{exportSession(*targetSession)}
	} else {
		inbox, _, err := e.store.ListInboxSessions(ctx, 10, 0)
		if err != nil {
			return domain.DiagnosticsExport{}, err
		}
		sessions = make([]domain.SessionExport, len(inbox))
		for i, is := range inbox {
			sessions[i] = exportInboxSession(is)
		}
	}

	reasoningTelemetry := e.collectReasoningTelemetry(ctx, sessions)
	modelUsage, _ := e.store.AggregateModelUsage(ctx, int(untilTime.Sub(sinceTime).Hours()/24+1))
	bridgeStatus := e.BridgeStatus()

	return domain.DiagnosticsExport{
		Metadata: domain.DiagnosticsMetadata{
			Version:       domain.ApplicationVersion,
			InstanceEpoch: e.epoch,
			TimeRange: domain.TimeRange{
				Since: sinceTime.Format(time.RFC3339),
				Until: untilTime.Format(time.RFC3339),
			},
			PIIRedacted: true,
			SessionID:   sessionID,
		},
		Config:      exportConfig(settings, e),
		Sessions:    sessions,
		ModelUsage:  exportModelUsage(modelUsage),
		Reasoning:   reasoningTelemetry,
		Bridge:      convertBridgeStatus(bridgeStatus),
		Errors:      []domain.ErrorEvent{},
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}, nil
}

func convertBridgeStatus(bs BridgeStatus) domain.BridgeStatus {
	return domain.BridgeStatus{
		State:              bs.State,
		Compatible:         bs.Compatible,
		Reasons:            bs.Reasons,
		Warnings:           bs.Warnings,
		Expected:           fmt.Sprintf("%v", bs.Expected),
		Actual:             fmt.Sprintf("%v", bs.Actual),
		NegotiatedProtocol: fmt.Sprintf("%v", bs.NegotiatedProtocol),
	}
}

func exportSession(s domain.Session) domain.SessionExport {
	runs := make([]domain.RunExport, len(s.Runs))
	for i, r := range s.Runs {
		runs[i] = exportRun(r)
	}
	return domain.SessionExport{
		ID:            s.ID,
		Intent:        s.Intent,
		Status:        s.Status,
		CreatedAt:     s.CreatedAt,
		CompletedAt:   s.CompletedAt,
		Runs:          runs,
		Coverage:      s.Coverage,
		PipelineStage: getPipelineStage(s),
	}
}

func exportInboxSession(is domain.InboxSession) domain.SessionExport {
	runs := make([]domain.RunExport, len(is.Runs))
	for i, r := range is.Runs {
		runs[i] = domain.RunExport{
			ID:            r.ID,
			Source:        r.Source,
			Status:        r.Status,
			Stage:         r.Stage,
			ElapsedMillis: r.TotalDurationMS,
		}
	}
	return domain.SessionExport{
		ID:          is.ID,
		Intent:      is.Intent,
		Status:      is.Status,
		CreatedAt:   is.CreatedAt,
		CompletedAt: is.CompletedAt,
		Runs:        runs,
	}
}

func exportRun(r domain.Run) domain.RunExport {
	re := domain.RunExport{
		ID:     r.ID,
		Source: r.Source,
		Status: r.Status,
		Stage:  r.Stage,
	}
	if r.Error != nil {
		re.Error = &domain.RunError{
			Code:    r.Error.Code,
			Message: r.Error.Message,
			Stage:   r.Stage,
		}
	}
	return re
}

func exportConfig(settings domain.Settings, e *Engine) domain.RedactedConfig {
	processes := e.ReasoningProcesses(settings)
	procInfo := make([]domain.ReasoningProcessInfo, len(processes))
	for i, p := range processes {
		procInfo[i] = domain.ReasoningProcessInfo{
			ID:        p.ID,
			Label:     p.Label,
			Provider:  p.Provider,
			Model:     p.Model,
			Effort:    p.Effort,
			Execution: p.Execution,
			ProfileID: p.ProfileID,
		}
	}
	return domain.RedactedConfig{
		ReasoningProvider:       settings.ReasoningProvider,
		ReasoningProcesses:      procInfo,
		ActiveSources:           settings.ActiveSources,
		TimelineCapacity:        settings.TimelineCapacity,
		AutoUpdateEnabled:       settings.AutoUpdateEnabled,
		AutoUpdateMode:          settings.AutoUpdateMode,
		LoadProfile:             settings.LoadProfile,
		AIDetectionEnabled:      settings.AIDetectionEnabled,
		ResurfaceMode:           settings.ResurfaceMode,
		BridgeContractVersion:   domain.BridgeContractVersion,
		DatabaseSchemaVersion:   store.SchemaVersion,
		ReasoningExecutablePath: settings.ReasoningExecutablePath,
	}
}

func exportModelUsage(mu domain.ModelUsageReport) domain.ModelUsageExport {
	cats := make([]domain.ModelUsageCategory, len(mu.Categories))
	for i, c := range mu.Categories {
		entries := make([]domain.ModelUsageEntry, len(c.Entries))
		for j, e := range c.Entries {
			entries[j] = domain.ModelUsageEntry{
				ID:                  e.ID,
				CategoryID:          c.ID,
				Source:              e.Source,
				Status:              e.Status,
				Provider:            e.Provider,
				Model:               e.Model,
				Effort:              e.Effort,
				InvocationCount:     e.InvocationCount,
				DurationMS:          e.DurationMS,
				CallerLatencyMS:     e.CallerLatencyMS,
				QueueWaitMS:         e.QueueWaitMS,
				ProviderExecutionMS: e.ProviderExecutionMS,
				ResponseTotalMS:     e.ResponseTotalMS,
				Usage:               e.Usage,
				UsageCoverage:       e.UsageCoverage,
				CreatedAt:           e.CreatedAt,
			}
		}
		cats[i] = domain.ModelUsageCategory{
			ID:              c.ID,
			Label:           c.Label,
			Execution:       c.Execution,
			Status:          c.Status,
			InvocationCount: c.InvocationCount,
			DurationMS:      c.DurationMS,
			Usage:           c.Usage,
			UsageCoverage:   c.UsageCoverage,
			Note:            c.Note,
			Entries:         entries,
		}
	}
	agg := mu.Usage
	return domain.ModelUsageExport{
		WindowDays:   mu.WindowDays,
		SessionCount: mu.SessionCount,
		GeneratedAt:  mu.GeneratedAt,
		Status:       mu.Status,
		AggregateTokens: domain.TokenUsage{
			InputTokens:           deref(agg.Input),
			CachedInputTokens:     deref(agg.CachedInput),
			OutputTokens:          deref(agg.Output),
			ReasoningOutputTokens: deref(agg.ReasoningOutput),
		},
		Categories: cats,
	}
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

func (e *Engine) collectReasoningTelemetry(ctx context.Context, sessions []domain.SessionExport) []domain.ReasoningTelemetry {
	var telemetry []domain.ReasoningTelemetry
	for i := range sessions {
		invocations, err := e.store.ReasoningInvocationsBySession(ctx, sessions[i].ID)
		if err != nil {
			continue
		}
		byRun := map[string][]domain.ReasoningTelemetry{}
		for _, inv := range invocations {
			byRun[inv.RunID] = append(byRun[inv.RunID], inv)
		}
		for j := range sessions[i].Runs {
			runID := sessions[i].Runs[j].ID
			inv, ok := byRun[runID]
			if !ok || len(inv) == 0 {
				continue
			}
			var elapsed int64
			for _, t := range inv {
				elapsed += t.DurationMS
			}
			re := &sessions[i].Runs[j]
			re.Provider = inv[0].Provider
			re.Model = inv[0].Model
			re.Effort = inv[0].Effort
			re.ElapsedMillis = elapsed
			last := inv[len(inv)-1]
			re.Telemetry = &last
			telemetry = append(telemetry, inv...)
		}
	}
	return telemetry
}

func getPipelineStage(s domain.Session) string {
	if stage, ok := s.Coverage["pipelineStage"].(string); ok {
		return stage
	}
	return ""
}
