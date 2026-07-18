package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type inboxTraceAssessment struct {
	value    domain.CandidateAssessment
	selected bool
}

type inboxTraceTimeline struct {
	item     domain.ReasonedItem
	relation string
	reason   string
}

func (s *Store) InboxRunTrace(ctx context.Context, runID, stage string, limit, offset int) (domain.InboxFlowTrace, error) {
	run, err := s.GetRun(ctx, runID)
	if err != nil {
		return domain.InboxFlowTrace{}, err
	}
	observations, err := s.Observations(ctx, runID)
	if err != nil {
		return domain.InboxFlowTrace{}, err
	}

	blocks := map[string]domain.Block{}
	order := make([]string, 0)
	for _, observation := range observations {
		for _, snapshot := range observation.Snapshots {
			for _, block := range snapshot.Blocks {
				key := strings.TrimSpace(block.EvidenceKey)
				if key == "" {
					continue
				}
				if _, exists := blocks[key]; !exists {
					order = append(order, key)
				}
				blocks[key] = mergeInboxTraceBlock(blocks[key], block)
			}
		}
	}

	assessments, err := s.inboxTraceAssessments(ctx, runID)
	if err != nil {
		return domain.InboxFlowTrace{}, err
	}
	timeline, err := s.inboxTraceTimeline(ctx, runID)
	if err != nil {
		return domain.InboxFlowTrace{}, err
	}

	trace := domain.InboxFlowTrace{
		RunID:  run.ID,
		Source: run.Source,
		Stage:  stage,
		Counts: domain.InboxFlowCounts{Captured: len(order)},
		Items:  []domain.InboxFlowItem{},
		Limit:  limit,
		Offset: offset,
	}
	filtered := make([]domain.InboxFlowItem, 0, len(order))
	for _, key := range order {
		block := blocks[key]
		assessment, evaluated := assessments[key]
		timelineValue, hasTimeline := timeline[key]
		selected := evaluated && assessment.selected
		added := hasTimeline && timelineValue.relation != "duplicate_report"
		if evaluated {
			trace.Counts.Evaluated++
		}
		if selected {
			trace.Counts.Selected++
		}
		if added {
			trace.Counts.Added++
		}

		item := domain.InboxFlowItem{
			EvidenceKey: key,
			Author:      strings.TrimSpace(block.Author),
			Excerpt:     compactInboxTraceText(block.Text, 220),
			SourceURL:   strings.TrimSpace(block.Permalink),
			Captured:    true,
			Evaluated:   evaluated,
			Selected:    selected,
			Added:       added,
		}
		if item.Author == "" {
			item.Author = strings.TrimSpace(timelineValue.item.Author)
		}
		if item.Excerpt == "" {
			item.Excerpt = compactInboxTraceText(timelineValue.item.WhatChanged, 220)
		}
		if item.SourceURL == "" {
			item.SourceURL = strings.TrimSpace(timelineValue.item.SourceURL)
		}

		switch {
		case hasTimeline && timelineValue.relation == "duplicate_report":
			item.Outcome = "collapsed_duplicate"
			item.Reason = strings.TrimSpace(timelineValue.reason)
			if item.Reason == "" {
				item.Reason = "Matched an already retained semantic event."
			}
		case added:
			item.Outcome = "added"
			item.Reason = strings.TrimSpace(assessment.value.Rationale)
		case selected:
			item.Outcome = "selected"
			item.Reason = strings.TrimSpace(assessment.value.Rationale)
		case evaluated:
			item.Outcome = "not_selected"
			item.Reason = strings.TrimSpace(assessment.value.Rationale)
		default:
			item.Outcome = "captured_only"
			item.Reason = inboxTraceCaptureReason(block, run)
		}
		if item.Reason == "" {
			item.Reason = inboxTraceFallbackReason(item.Outcome)
		}
		item.Reason = compactInboxTraceText(item.Reason, 180)
		if inboxTraceMatchesStage(item, stage) {
			filtered = append(filtered, item)
		}
	}

	trace.Total = len(filtered)
	if offset >= len(filtered) {
		return trace, nil
	}
	end := offset + limit
	if end > len(filtered) {
		end = len(filtered)
	}
	trace.Items = append(trace.Items, filtered[offset:end]...)
	return trace, nil
}

func (s *Store) inboxTraceAssessments(ctx context.Context, runID string) (map[string]inboxTraceAssessment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT evidence_key,assessment_json,selected FROM candidate_assessments WHERE run_id=?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]inboxTraceAssessment{}
	for rows.Next() {
		var key, raw string
		var selected int
		if err := rows.Scan(&key, &raw, &selected); err != nil {
			return nil, err
		}
		var assessment domain.CandidateAssessment
		if err := json.Unmarshal([]byte(raw), &assessment); err != nil {
			return nil, err
		}
		values[key] = inboxTraceAssessment{value: assessment, selected: selected == 1}
	}
	return values, rows.Err()
}

func (s *Store) inboxTraceTimeline(ctx context.Context, runID string) (map[string]inboxTraceTimeline, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t.evidence_key,t.item_json,COALESCE(r.relation,''),COALESCE(r.reason,'')
		FROM timeline_items t
		LEFT JOIN semantic_event_reports r ON r.timeline_id=t.id
		WHERE t.run_id=?`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := map[string]inboxTraceTimeline{}
	for rows.Next() {
		var key, raw, relation, reason string
		if err := rows.Scan(&key, &raw, &relation, &reason); err != nil {
			return nil, err
		}
		var item domain.ReasonedItem
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, err
		}
		values[key] = inboxTraceTimeline{item: item, relation: relation, reason: reason}
	}
	return values, rows.Err()
}

func mergeInboxTraceBlock(current, next domain.Block) domain.Block {
	if strings.TrimSpace(next.Author) == "" {
		next.Author = current.Author
	}
	if strings.TrimSpace(next.Text) == "" {
		next.Text = current.Text
	}
	if strings.TrimSpace(next.Permalink) == "" {
		next.Permalink = current.Permalink
	}
	if len(next.CaptureQuality) == 0 {
		next.CaptureQuality = current.CaptureQuality
	}
	return next
}

func inboxTraceMatchesStage(item domain.InboxFlowItem, stage string) bool {
	switch stage {
	case "evaluated":
		return item.Evaluated
	case "selected":
		return item.Selected
	case "added":
		return item.Added
	default:
		return item.Captured
	}
}

func inboxTraceCaptureReason(block domain.Block, run domain.Run) string {
	if run.Error != nil && strings.TrimSpace(run.Error.Message) != "" {
		return fmt.Sprintf("Run stopped before evaluation: %s", strings.TrimSpace(run.Error.Message))
	}
	for _, key := range []string{"reason", "detail"} {
		if value, ok := block.CaptureQuality[key].(string); ok && strings.TrimSpace(value) != "" {
			return compactInboxTraceText(value, 180)
		}
	}
	if verdict, ok := block.CaptureQuality["verdict"].(string); ok && strings.TrimSpace(verdict) != "" && verdict != "complete" {
		return compactInboxTraceText(verdict, 180)
	}
	if issues, ok := block.CaptureQuality["issues"].([]any); ok {
		values := make([]string, 0, len(issues))
		for _, issue := range issues {
			if value, ok := issue.(string); ok && strings.TrimSpace(value) != "" {
				values = append(values, strings.TrimSpace(value))
			}
		}
		if len(values) > 0 {
			return compactInboxTraceText(strings.Join(values, ", "), 180)
		}
	}
	return "Captured as source evidence but not evaluated in this run."
}

func inboxTraceFallbackReason(outcome string) string {
	switch outcome {
	case "added":
		return "Selected and retained as unique Timeline information."
	case "selected":
		return "Selected by reasoning but not retained as a unique Timeline addition."
	case "not_selected":
		return "Evaluated but not selected within this bounded update."
	default:
		return "Captured as source evidence."
	}
}

func compactInboxTraceText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return strings.TrimSpace(string(runes[:limit])) + "…"
}
