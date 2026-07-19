package reasoning

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type Deterministic struct{}

func (Deterministic) Name() string { return "deterministic" }

func (Deterministic) Plan(_ context.Context, run domain.Run, _ domain.Observation, _ []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return AcquisitionPlan{Decision: "finish", Reason: "deterministic provider accepts the bounded first observation"}, telemetry(run, "acquisition_planning", "deterministic", "deterministic", "none", 0, "completed"), nil
}

func (Deterministic) Analyze(_ context.Context, run domain.Run, observation domain.Observation, _ []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	started := time.Now()
	result := domain.ReasoningResult{Summary: fmt.Sprintf("Bounded %s update", run.Source), Limitations: []string{"Deterministic development provider; no model inference was used."}}
	seen := map[string]bool{}
	for _, snapshot := range observation.Snapshots {
		for _, block := range snapshot.Blocks {
			if block.EvidenceKey == "" || seen[block.EvidenceKey] {
				continue
			}
			seen[block.EvidenceKey] = true
			text := strings.TrimSpace(block.Text)
			if text == "" {
				text = fallbackEvidenceSummary(block)
			}
			title := text
			if len(title) > 180 {
				title = title[:180] + "…"
			}
			url := block.Permalink
			kind := "native_post"
			if url == "" {
				url = observation.PageURL
				kind = "source_page"
			}
			result.Items = append(result.Items, domain.ReasonedItem{ID: domain.NewID("item"), WhatChanged: title, WhyItMatters: "Visible source evidence was captured inside the configured bounded session.", Source: run.Source, SourceURL: url, SourceURLKind: kind, EvidenceKey: block.EvidenceKey, EventKey: eventKey(block.EvidenceKey), KnowledgeDelta: "new_event", Author: block.Author, PublishedAt: block.PublishedAt, Confidence: 0.55, EvidenceState: "primary"})
			result.CandidateAssessments = append(result.CandidateAssessments, domain.CandidateAssessment{EvidenceKey: block.EvidenceKey, TopicTags: []string{"unclassified"}, TopicFacets: []string{"other"}, ContentType: "other", Novelty: 0.5, Urgency: 0.2, Actionability: 0.2, Materiality: 0.4, EvidenceStrength: 0.6, Rationale: "Deterministic baseline assessment for provider conformance."})
		}
	}
	return result, telemetry(run, "candidate_evaluation", "deterministic", "deterministic", "none", time.Since(started), "completed"), nil
}

func fallbackEvidenceSummary(block domain.Block) string {
	switch {
	case len(block.Media) > 0:
		return fmt.Sprintf("Visual post with %d captured media item(s); content requires visual-capable evaluation.", len(block.Media))
	case len(block.Attachments) > 0:
		return fmt.Sprintf("Post with %d captured attachment(s).", len(block.Attachments))
	case len(block.QuotedPost) > 0:
		return "Post containing captured quoted-post evidence."
	default:
		return "Captured source post with limited textual evidence."
	}
}

func eventKey(evidenceKey string) string {
	value := strings.NewReplacer(":", "-", "_", "-").Replace(strings.ToLower(evidenceKey))
	return "event-" + value
}
func telemetry(run domain.Run, phase, provider, model, effort string, duration time.Duration, status string) domain.ReasoningTelemetry {
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: provider, Model: model, Effort: effort, DurationMS: duration.Milliseconds(), Status: status, CreatedAt: domain.Now()}
}
