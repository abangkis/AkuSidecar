package store

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type usageAccumulator struct {
	usage       domain.ModelUsage
	reported    int
	partial     int
	unavailable int
	invocations int
	durationMS  int64
}

func modelUsageCategories() []domain.ModelUsageCategory {
	return []domain.ModelUsageCategory{
		{ID: "acquisition_planning", Label: "Acquisition planning", Execution: "in-run", Entries: []domain.ModelUsageEntry{}},
		{ID: "candidate_evaluation", Label: "Candidate evaluation", Execution: "in-run", Entries: []domain.ModelUsageEntry{}},
		{ID: "semantic_event_resolution", Label: "Semantic event resolution", Execution: "in-run", Entries: []domain.ModelUsageEntry{}},
		{ID: "ai_deep_detection", Label: "AI Deep Detection", Execution: "async", Entries: []domain.ModelUsageEntry{}},
	}
}

func nullableToken(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func usageCoverage(value domain.ModelUsage) string {
	reported := 0
	for _, counter := range []*int64{value.Input, value.CachedInput, value.Output, value.ReasoningOutput} {
		if counter != nil {
			reported++
		}
	}
	switch reported {
	case 0:
		return "unavailable"
	case 4:
		return "complete"
	default:
		return "partial"
	}
}

func addUsageCounter(target **int64, value *int64) {
	if value == nil {
		return
	}
	if *target == nil {
		zero := int64(0)
		*target = &zero
	}
	**target += *value
}

func (a *usageAccumulator) add(entry domain.ModelUsageEntry) {
	if entry.InvocationCount > 0 {
		a.invocations += entry.InvocationCount
		a.durationMS += entry.DurationMS
		switch entry.UsageCoverage {
		case "complete":
			a.reported += entry.InvocationCount
		case "partial":
			a.partial += entry.InvocationCount
		default:
			a.unavailable += entry.InvocationCount
		}
	}
	addUsageCounter(&a.usage.Input, entry.Usage.Input)
	addUsageCounter(&a.usage.CachedInput, entry.Usage.CachedInput)
	addUsageCounter(&a.usage.Output, entry.Usage.Output)
	addUsageCounter(&a.usage.ReasoningOutput, entry.Usage.ReasoningOutput)
}

func (a usageAccumulator) coverage() string {
	if a.invocations == 0 {
		return "not_applicable"
	}
	if a.unavailable == a.invocations {
		return "unavailable"
	}
	if a.unavailable > 0 || a.partial > 0 {
		return "partial"
	}
	return "complete"
}

func aggregateStatus(entries []domain.ModelUsageEntry, sessionStatus string) string {
	if len(entries) == 0 {
		if sessionStatus == "queued" || sessionStatus == "running" {
			return "pending"
		}
		return "not_invoked"
	}
	counts := map[string]int{}
	for _, entry := range entries {
		counts[entry.Status]++
	}
	if counts["queued"] > 0 || counts["running"] > 0 {
		return "running"
	}
	if counts["completed"] == len(entries) {
		return "completed"
	}
	if counts["bypassed"] == len(entries) {
		return "not_invoked"
	}
	if counts["failed"] == len(entries) {
		return "failed"
	}
	if counts["cancelled"] == len(entries) {
		return "cancelled"
	}
	return "partial"
}

func categoryNote(categoryID, status string) string {
	if status == "pending" {
		return "This process has not reached its invocation point yet."
	}
	if status != "not_invoked" {
		return ""
	}
	switch categoryID {
	case "acquisition_planning":
		return "No follow-up planning was needed, or the check used the first-run fast path."
	case "candidate_evaluation":
		return "No captured candidate required model evaluation."
	case "semantic_event_resolution":
		return "The local semantic path was sufficient; no model resolver was invoked."
	case "ai_deep_detection":
		return "Deep Detection was disabled, skipped for onboarding, or had no retained post to review."
	default:
		return "No model invocation was required."
	}
}

func finalizeCategory(category *domain.ModelUsageCategory, sessionStatus string) {
	accumulator := usageAccumulator{}
	for _, entry := range category.Entries {
		accumulator.add(entry)
	}
	category.Status = aggregateStatus(category.Entries, sessionStatus)
	category.InvocationCount = accumulator.invocations
	category.DurationMS = accumulator.durationMS
	category.Usage = accumulator.usage
	category.UsageCoverage = accumulator.coverage()
	category.Note = categoryNote(category.ID, category.Status)
}

func usageEntry(id, categoryID string, source domain.Source, status, provider, model, effort string, duration int64, input, cached, output, reasoning sql.NullInt64, createdAt string) domain.ModelUsageEntry {
	count := 1
	if status == "bypassed" || provider == "local-index" || model == "none" || model == "local-deterministic" {
		count = 0
	}
	entry := domain.ModelUsageEntry{
		ID: id, CategoryID: categoryID, Source: source, Status: status,
		Provider: provider, Model: model, Effort: effort, InvocationCount: count,
		DurationMS: duration, CreatedAt: createdAt,
		Usage: domain.ModelUsage{Input: nullableToken(input), CachedInput: nullableToken(cached), Output: nullableToken(output), ReasoningOutput: nullableToken(reasoning)},
	}
	if count == 0 {
		entry.UsageCoverage = "not_applicable"
	} else {
		entry.UsageCoverage = usageCoverage(entry.Usage)
	}
	return entry
}

// SessionModelUsage projects all provider invocations associated with one
// bounded check. It deliberately reads the existing ledgers rather than
// creating a second token-accounting source of truth.
func (s *Store) SessionModelUsage(ctx context.Context, sessionID string) (domain.ModelUsageReport, error) {
	var sessionStatus, createdAt string
	if err := s.db.QueryRowContext(ctx, `SELECT status,created_at FROM sessions WHERE id=?`, sessionID).Scan(&sessionStatus, &createdAt); err != nil {
		return domain.ModelUsageReport{}, err
	}
	categories := modelUsageCategories()
	byID := map[string]*domain.ModelUsageCategory{}
	for index := range categories {
		byID[categories[index].ID] = &categories[index]
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT i.id,r.source,i.phase,i.status,i.provider,i.model,i.effort,i.duration_ms,
		       i.input_tokens,i.cached_input_tokens,i.output_tokens,i.reasoning_output_tokens,i.created_at
		FROM reasoning_invocations i JOIN runs r ON r.id=i.run_id
		WHERE r.session_id=? ORDER BY i.created_at,i.id`, sessionID)
	if err != nil {
		return domain.ModelUsageReport{}, err
	}
	for rows.Next() {
		var id, phase, status, provider, model, effort, invocationAt string
		var source domain.Source
		var duration int64
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&id, &source, &phase, &status, &provider, &model, &effort, &duration, &input, &cached, &output, &reasoning, &invocationAt); err != nil {
			rows.Close()
			return domain.ModelUsageReport{}, err
		}
		category := byID[phase]
		if category != nil {
			category.Entries = append(category.Entries, usageEntry(id, phase, source, status, provider, model, effort, duration, input, cached, output, reasoning, invocationAt))
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ModelUsageReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ModelUsageReport{}, err
	}

	var semanticID, semanticStatus, semanticProvider, semanticModel, semanticEffort, semanticAt string
	var semanticDuration int64
	var semanticInput, semanticCached, semanticOutput, semanticReasoning sql.NullInt64
	err = s.db.QueryRowContext(ctx, `
		SELECT session_id,status,provider,model,effort,duration_ms,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,created_at
		FROM event_resolution_invocations WHERE session_id=?`, sessionID).
		Scan(&semanticID, &semanticStatus, &semanticProvider, &semanticModel, &semanticEffort, &semanticDuration, &semanticInput, &semanticCached, &semanticOutput, &semanticReasoning, &semanticAt)
	if err != nil && err != sql.ErrNoRows {
		return domain.ModelUsageReport{}, err
	}
	if err == nil {
		byID["semantic_event_resolution"].Entries = append(byID["semantic_event_resolution"].Entries,
			usageEntry(semanticID+":semantic", "semantic_event_resolution", "", semanticStatus, semanticProvider, semanticModel, semanticEffort, semanticDuration, semanticInput, semanticCached, semanticOutput, semanticReasoning, semanticAt))
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT id,status,provider,model,effort,duration_ms,input_tokens,cached_input_tokens,output_tokens,reasoning_output_tokens,created_at
		FROM ai_detection_jobs WHERE session_id=? ORDER BY created_at,id`, sessionID)
	if err != nil {
		return domain.ModelUsageReport{}, err
	}
	for rows.Next() {
		var id, status, provider, model, effort, invocationAt string
		var duration int64
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&id, &status, &provider, &model, &effort, &duration, &input, &cached, &output, &reasoning, &invocationAt); err != nil {
			rows.Close()
			return domain.ModelUsageReport{}, err
		}
		byID["ai_deep_detection"].Entries = append(byID["ai_deep_detection"].Entries,
			usageEntry(id, "ai_deep_detection", "", status, provider, model, effort, duration, input, cached, output, reasoning, invocationAt))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ModelUsageReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ModelUsageReport{}, err
	}

	reportAccumulator := usageAccumulator{}
	for index := range categories {
		finalizeCategory(&categories[index], sessionStatus)
		for _, entry := range categories[index].Entries {
			reportAccumulator.add(entry)
		}
	}
	now := domain.Now()
	return domain.ModelUsageReport{
		Scope: "session", SessionID: sessionID, SessionCount: 1, From: createdAt, To: now, GeneratedAt: now,
		Status: sessionStatus, Usage: reportAccumulator.usage, UsageCoverage: reportAccumulator.coverage(),
		DurationMS: reportAccumulator.durationMS, Categories: categories,
	}, nil
}

func mergeUsageStatus(left, right string) string {
	if left == "" {
		return right
	}
	if left == right {
		return left
	}
	if left == "running" || right == "running" || left == "pending" || right == "pending" {
		return "running"
	}
	if left == "not_invoked" {
		return right
	}
	if right == "not_invoked" {
		return left
	}
	return "partial"
}

func aggregateModelUsageEntries(entries []domain.ModelUsageEntry) []domain.ModelUsageEntry {
	grouped := map[string]*domain.ModelUsageEntry{}
	for _, entry := range entries {
		key := strings.Join([]string{string(entry.Source), entry.Provider, entry.Model, entry.Effort}, "\x00")
		current := grouped[key]
		if current == nil {
			copy := entry
			copy.ID = fmt.Sprintf("aggregate:%s:%d", entry.CategoryID, len(grouped))
			copy.CreatedAt = ""
			grouped[key] = &copy
			continue
		}
		current.InvocationCount += entry.InvocationCount
		current.DurationMS += entry.DurationMS
		current.Status = mergeUsageStatus(current.Status, entry.Status)
		if current.UsageCoverage != entry.UsageCoverage {
			current.UsageCoverage = "partial"
		}
		addUsageCounter(&current.Usage.Input, entry.Usage.Input)
		addUsageCounter(&current.Usage.CachedInput, entry.Usage.CachedInput)
		addUsageCounter(&current.Usage.Output, entry.Usage.Output)
		addUsageCounter(&current.Usage.ReasoningOutput, entry.Usage.ReasoningOutput)
	}
	result := make([]domain.ModelUsageEntry, 0, len(grouped))
	for _, entry := range grouped {
		result = append(result, *entry)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		if result[i].Provider != result[j].Provider {
			return result[i].Provider < result[j].Provider
		}
		if result[i].Model != result[j].Model {
			return result[i].Model < result[j].Model
		}
		return result[i].Effort < result[j].Effort
	})
	return result
}

// AggregateModelUsage reports only locally retained AkuBrowser telemetry. A
// database reset or retention trim intentionally narrows this window.
func (s *Store) AggregateModelUsage(ctx context.Context, windowDays int) (domain.ModelUsageReport, error) {
	if windowDays != 7 && windowDays != 30 && windowDays != 90 {
		return domain.ModelUsageReport{}, fmt.Errorf("model usage window must be 7, 30, or 90 days")
	}
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -windowDays)
	fromValue := from.Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx, `SELECT status FROM sessions WHERE created_at>=? ORDER BY created_at`, fromValue)
	if err != nil {
		return domain.ModelUsageReport{}, err
	}
	sessionCount := 0
	status := "not_invoked"
	for rows.Next() {
		var sessionStatus string
		if err := rows.Scan(&sessionStatus); err != nil {
			rows.Close()
			return domain.ModelUsageReport{}, err
		}
		sessionCount++
		status = mergeUsageStatus(status, sessionStatus)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ModelUsageReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ModelUsageReport{}, err
	}

	categories := modelUsageCategories()
	byID := map[string]*domain.ModelUsageCategory{}
	for index := range categories {
		byID[categories[index].ID] = &categories[index]
	}
	rows, err = s.db.QueryContext(ctx, `
		SELECT i.id,r.source,i.phase,i.status,i.provider,i.model,i.effort,i.duration_ms,
		       i.input_tokens,i.cached_input_tokens,i.output_tokens,i.reasoning_output_tokens,i.created_at
		FROM reasoning_invocations i
		JOIN runs r ON r.id=i.run_id
		JOIN sessions s ON s.id=r.session_id
		WHERE s.created_at>=? ORDER BY i.created_at,i.id`, fromValue)
	if err != nil {
		return domain.ModelUsageReport{}, err
	}
	for rows.Next() {
		var id, phase, invocationStatus, provider, model, effort, invocationAt string
		var source domain.Source
		var duration int64
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&id, &source, &phase, &invocationStatus, &provider, &model, &effort, &duration, &input, &cached, &output, &reasoning, &invocationAt); err != nil {
			rows.Close()
			return domain.ModelUsageReport{}, err
		}
		if category := byID[phase]; category != nil {
			category.Entries = append(category.Entries, usageEntry(id, phase, source, invocationStatus, provider, model, effort, duration, input, cached, output, reasoning, invocationAt))
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ModelUsageReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ModelUsageReport{}, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT e.session_id,e.status,e.provider,e.model,e.effort,e.duration_ms,
		       e.input_tokens,e.cached_input_tokens,e.output_tokens,e.reasoning_output_tokens,e.created_at
		FROM event_resolution_invocations e
		JOIN sessions s ON s.id=e.session_id
		WHERE s.created_at>=? ORDER BY e.created_at,e.session_id`, fromValue)
	if err != nil {
		return domain.ModelUsageReport{}, err
	}
	for rows.Next() {
		var sessionID, invocationStatus, provider, model, effort, invocationAt string
		var duration int64
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&sessionID, &invocationStatus, &provider, &model, &effort, &duration, &input, &cached, &output, &reasoning, &invocationAt); err != nil {
			rows.Close()
			return domain.ModelUsageReport{}, err
		}
		byID["semantic_event_resolution"].Entries = append(byID["semantic_event_resolution"].Entries,
			usageEntry(sessionID+":semantic", "semantic_event_resolution", "", invocationStatus, provider, model, effort, duration, input, cached, output, reasoning, invocationAt))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ModelUsageReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ModelUsageReport{}, err
	}

	rows, err = s.db.QueryContext(ctx, `
		SELECT a.id,a.status,a.provider,a.model,a.effort,a.duration_ms,
		       a.input_tokens,a.cached_input_tokens,a.output_tokens,a.reasoning_output_tokens,a.created_at
		FROM ai_detection_jobs a
		JOIN sessions s ON s.id=a.session_id
		WHERE s.created_at>=? ORDER BY a.created_at,a.id`, fromValue)
	if err != nil {
		return domain.ModelUsageReport{}, err
	}
	for rows.Next() {
		var id, invocationStatus, provider, model, effort, invocationAt string
		var duration int64
		var input, cached, output, reasoning sql.NullInt64
		if err := rows.Scan(&id, &invocationStatus, &provider, &model, &effort, &duration, &input, &cached, &output, &reasoning, &invocationAt); err != nil {
			rows.Close()
			return domain.ModelUsageReport{}, err
		}
		byID["ai_deep_detection"].Entries = append(byID["ai_deep_detection"].Entries,
			usageEntry(id, "ai_deep_detection", "", invocationStatus, provider, model, effort, duration, input, cached, output, reasoning, invocationAt))
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.ModelUsageReport{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.ModelUsageReport{}, err
	}

	reportAccumulator := usageAccumulator{}
	for index := range categories {
		categories[index].Entries = aggregateModelUsageEntries(categories[index].Entries)
		finalizeCategory(&categories[index], status)
		for _, entry := range categories[index].Entries {
			reportAccumulator.add(entry)
		}
	}
	generatedAt := domain.Now()
	return domain.ModelUsageReport{
		Scope: "aggregate", WindowDays: windowDays, SessionCount: sessionCount,
		From: from.Format(time.RFC3339Nano), To: generatedAt, GeneratedAt: generatedAt,
		Status: status, Usage: reportAccumulator.usage, UsageCoverage: reportAccumulator.coverage(),
		DurationMS: reportAccumulator.durationMS, Categories: categories,
	}, nil
}
