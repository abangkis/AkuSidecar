package store

import (
	"context"
	"math"
	"sort"
	"time"
)

const (
	autoUpdateOutcomeProductive     = "productive"
	autoUpdateOutcomeValidEmpty     = "valid_empty"
	autoUpdateOutcomeTechnical      = "technical_failure"
	autoUpdateOutcomeInterrupted    = "interrupted"
	autoUpdateOutcomeUnknown        = "unknown"
	adaptiveTechnicalBackoffInitial = 5 * time.Minute
	adaptiveTechnicalBackoffSecond  = 15 * time.Minute
	adaptiveTechnicalBackoffMax     = 30 * time.Minute
)

type AutoUpdateAdaptiveOutcome struct {
	CompletedAt   time.Time
	Kind          string
	Trigger       string
	Delivery      string
	ItemCount     int
	TotalRuns     int
	CompletedRuns int
	FailedRuns    int
	CancelledRuns int
}

type AutoUpdateAdaptiveSignals struct {
	ConsumptionPace       time.Duration
	ConsumptionSamples    int
	PreparationLead       time.Duration
	LastRevealAt          time.Time
	GenerationAttempts    []time.Time
	LastYieldItems        int
	EmptyYieldStreak      int
	SupplyBackoffUntil    time.Time
	LastOutcome           AutoUpdateAdaptiveOutcome
	TechnicalStreak       int
	TechnicalBackoffUntil time.Time
	ReplenishmentPressure int
	PressureFromReveals   int
	PressureFromUpdates   int
	PressureFromYield     int
}

func (s *Store) AutoUpdateAdaptiveSignals(ctx context.Context, generationWindow, pressureWindow, pressureHalfLife, defaultPreparationLead time.Duration) (AutoUpdateAdaptiveSignals, error) {
	now := s.Now()
	result := AutoUpdateAdaptiveSignals{PreparationLead: defaultPreparationLead}

	reveals, err := s.autoUpdateRevealTimes(ctx, now.Add(-7*24*time.Hour), 5)
	if err != nil {
		return result, err
	}
	intervals := make([]time.Duration, 0, len(reveals))
	for index := 1; index < len(reveals); index++ {
		interval := reveals[index].Sub(reveals[index-1])
		if interval > 0 && interval <= 2*time.Hour {
			intervals = append(intervals, interval)
		}
	}
	if len(reveals) > 0 {
		result.LastRevealAt = reveals[len(reveals)-1]
	}
	if len(intervals) > 0 {
		result.ConsumptionPace = medianDuration(intervals)
		result.ConsumptionSamples = len(intervals)
	}

	preparationDurations, err := s.autoUpdatePreparationDurations(ctx, 5)
	if err != nil {
		return result, err
	}
	if len(preparationDurations) > 0 {
		sort.Slice(preparationDurations, func(i, j int) bool { return preparationDurations[i] < preparationDurations[j] })
		index := (3*len(preparationDurations) + 3) / 4
		if index > 0 {
			index--
		}
		result.PreparationLead = preparationDurations[index] + 2*time.Minute
		if result.PreparationLead < 3*time.Minute {
			result.PreparationLead = 3 * time.Minute
		}
		if result.PreparationLead > 30*time.Minute {
			result.PreparationLead = 30 * time.Minute
		}
	}

	result.GenerationAttempts, err = s.scheduledAutoUpdateAttempts(ctx, now.Add(-generationWindow))
	if err != nil {
		return result, err
	}
	_, yields, err := s.recentPreparedUpdateYields(ctx, 5)
	if err != nil {
		return result, err
	}
	if len(yields) > 0 {
		result.LastYieldItems = yields[0]
	}
	outcomes, err := s.recentAdaptiveOutcomes(ctx, 5)
	if err != nil {
		return result, err
	}
	if len(outcomes) > 0 {
		result.LastOutcome = outcomes[0]
		if outcomes[0].Kind == autoUpdateOutcomeValidEmpty {
			for _, outcome := range outcomes {
				if outcome.Kind != autoUpdateOutcomeValidEmpty {
					break
				}
				result.EmptyYieldStreak++
			}
		}
		if outcomes[0].Kind == autoUpdateOutcomeTechnical {
			for _, outcome := range outcomes {
				if outcome.Kind != autoUpdateOutcomeTechnical {
					break
				}
				result.TechnicalStreak++
			}
		}
	}
	if result.EmptyYieldStreak > 0 && result.LastOutcome.Kind == autoUpdateOutcomeValidEmpty && !result.LastOutcome.CompletedAt.IsZero() {
		backoff := 15 * time.Minute
		if result.EmptyYieldStreak == 2 {
			backoff = 30 * time.Minute
		} else if result.EmptyYieldStreak >= 3 {
			backoff = 60 * time.Minute
		}
		result.SupplyBackoffUntil = result.LastOutcome.CompletedAt.Add(backoff)
	}
	if result.TechnicalStreak > 0 && result.LastOutcome.Kind == autoUpdateOutcomeTechnical && !result.LastOutcome.CompletedAt.IsZero() {
		backoff := adaptiveTechnicalBackoffInitial
		if result.TechnicalStreak == 2 {
			backoff = adaptiveTechnicalBackoffSecond
		} else if result.TechnicalStreak >= 3 {
			backoff = adaptiveTechnicalBackoffMax
		}
		result.TechnicalBackoffUntil = result.LastOutcome.CompletedAt.Add(backoff)
	}
	pressureReveals, err := s.autoUpdateRevealTimes(ctx, now.Add(-pressureWindow), 20)
	if err != nil {
		return result, err
	}
	pressureOutcomes, err := s.recentAdaptiveOutcomesSince(ctx, now.Add(-pressureWindow), 20)
	if err != nil {
		return result, err
	}
	result.PressureFromReveals, result.PressureFromUpdates, result.PressureFromYield = replenishmentPressureComponents(now, pressureHalfLife, pressureReveals, pressureOutcomes)
	result.ReplenishmentPressure = clampInt(result.PressureFromReveals+result.PressureFromUpdates+result.PressureFromYield, 0, 100)
	return result, nil
}

func (s *Store) autoUpdateRevealTimes(ctx context.Context, since time.Time, limit int) ([]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT revealed_at FROM auto_update_batches WHERE revealed_at IS NOT NULL AND revealed_at>=? ORDER BY revealed_at DESC LIMIT ?`, since.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	newestFirst := []time.Time{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if value, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
			newestFirst = append(newestFirst, value)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	result := make([]time.Time, len(newestFirst))
	for index := range newestFirst {
		result[len(newestFirst)-1-index] = newestFirst[index]
	}
	return result, nil
}

func (s *Store) autoUpdatePreparationDurations(ctx context.Context, limit int) ([]time.Duration, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.started_at,s.completed_at
		FROM sessions s JOIN auto_update_batches b ON b.session_id=s.id
		WHERE json_extract(s.coverage_json,'$.trigger')='scheduler'
		  AND s.started_at IS NOT NULL AND s.completed_at IS NOT NULL
		  AND s.status IN ('completed','partial')
		ORDER BY s.completed_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []time.Duration{}
	for rows.Next() {
		var startedRaw, completedRaw string
		if err := rows.Scan(&startedRaw, &completedRaw); err != nil {
			return nil, err
		}
		started, startedErr := time.Parse(time.RFC3339Nano, startedRaw)
		completed, completedErr := time.Parse(time.RFC3339Nano, completedRaw)
		if startedErr == nil && completedErr == nil {
			duration := completed.Sub(started)
			if duration > 0 && duration <= 2*time.Hour {
				result = append(result, duration)
			}
		}
	}
	return result, rows.Err()
}

func (s *Store) scheduledAutoUpdateAttempts(ctx context.Context, since time.Time) ([]time.Time, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.created_at
		FROM auto_update_batches b JOIN sessions s ON s.id=b.session_id
		WHERE json_extract(s.coverage_json,'$.trigger')='scheduler' AND b.created_at>=?
		ORDER BY b.created_at`, since.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []time.Time{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		if value, parseErr := time.Parse(time.RFC3339Nano, raw); parseErr == nil {
			result = append(result, value)
		}
	}
	return result, rows.Err()
}

func (s *Store) recentPreparedUpdateYields(ctx context.Context, limit int) (time.Time, []int, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.completed_at,COUNT(t.id)
		FROM sessions s
		JOIN auto_update_batches b ON b.session_id=s.id
		LEFT JOIN timeline_items t ON t.session_id=s.id
		WHERE s.completed_at IS NOT NULL AND s.status IN ('completed','partial')
		GROUP BY s.id,s.completed_at
		ORDER BY s.completed_at DESC LIMIT ?`, limit)
	if err != nil {
		return time.Time{}, nil, err
	}
	defer rows.Close()
	var latest time.Time
	yields := []int{}
	for rows.Next() {
		var completedRaw string
		var itemCount int
		if err := rows.Scan(&completedRaw, &itemCount); err != nil {
			return time.Time{}, nil, err
		}
		if len(yields) == 0 {
			latest, _ = time.Parse(time.RFC3339Nano, completedRaw)
		}
		yields = append(yields, itemCount)
	}
	return latest, yields, rows.Err()
}

func (s *Store) recentAdaptiveOutcomes(ctx context.Context, limit int) ([]AutoUpdateAdaptiveOutcome, error) {
	return s.recentAdaptiveOutcomesSince(ctx, time.Time{}, limit)
}

func (s *Store) recentAdaptiveOutcomesSince(ctx context.Context, since time.Time, limit int) ([]AutoUpdateAdaptiveOutcome, error) {
	sinceRaw := ""
	if !since.IsZero() {
		sinceRaw = since.UTC().Format(time.RFC3339Nano)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.completed_at,
		       COALESCE(json_extract(s.coverage_json,'$.trigger'),'user'),
		       COALESCE(json_extract(s.coverage_json,'$.delivery'),'visible'),
		       COUNT(r.id),
		       COALESCE(SUM(CASE WHEN r.status='completed' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN r.status='failed' THEN 1 ELSE 0 END),0),
		       COALESCE(SUM(CASE WHEN r.status='cancelled' THEN 1 ELSE 0 END),0),
		       (SELECT COUNT(*) FROM timeline_items t WHERE t.session_id=s.id)
		FROM sessions s
		LEFT JOIN runs r ON r.session_id=s.id
		WHERE s.completed_at IS NOT NULL
		  AND s.status IN ('completed','partial','failed','cancelled')
		  AND COALESCE(json_extract(s.coverage_json,'$.trigger'),'user') IN ('scheduler','user')
		  AND (?='' OR s.completed_at>=?)
		GROUP BY s.id,s.completed_at,s.coverage_json
		ORDER BY s.completed_at DESC LIMIT ?`, sinceRaw, sinceRaw, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []AutoUpdateAdaptiveOutcome{}
	for rows.Next() {
		var completedRaw, trigger, delivery string
		var totalRuns, completedRuns, failedRuns, cancelledRuns, itemCount int
		if err := rows.Scan(&completedRaw, &trigger, &delivery, &totalRuns, &completedRuns, &failedRuns, &cancelledRuns, &itemCount); err != nil {
			return nil, err
		}
		completedAt, parseErr := time.Parse(time.RFC3339Nano, completedRaw)
		if parseErr != nil {
			continue
		}
		kind := autoUpdateOutcomeUnknown
		switch {
		case itemCount > 0:
			kind = autoUpdateOutcomeProductive
		case failedRuns > 0:
			kind = autoUpdateOutcomeTechnical
		case cancelledRuns > 0:
			kind = autoUpdateOutcomeInterrupted
		case totalRuns > 0 && completedRuns == totalRuns:
			kind = autoUpdateOutcomeValidEmpty
		}
		result = append(result, AutoUpdateAdaptiveOutcome{
			CompletedAt: completedAt, Kind: kind, Trigger: trigger, Delivery: delivery,
			ItemCount: itemCount, TotalRuns: totalRuns, CompletedRuns: completedRuns,
			FailedRuns: failedRuns, CancelledRuns: cancelledRuns,
		})
	}
	return result, rows.Err()
}

func replenishmentPressureComponents(now time.Time, halfLife time.Duration, reveals []time.Time, outcomes []AutoUpdateAdaptiveOutcome) (int, int, int) {
	if halfLife <= 0 {
		halfLife = 30 * time.Minute
	}
	revealPressure := 0
	for index, revealedAt := range reveals {
		points := 18
		if index > 0 {
			interval := revealedAt.Sub(reveals[index-1])
			if interval > 0 && interval <= 5*time.Minute {
				points += 8
			} else if interval > 0 && interval <= 10*time.Minute {
				points += 4
			}
		}
		revealPressure += decayedPressurePoints(points, revealedAt, now, halfLife)
	}
	updatePressure := 0
	yieldPressure := 0
	for _, outcome := range outcomes {
		updatePressure += decayedPressurePoints(10, outcome.CompletedAt, now, halfLife)
		yieldPoints := 0
		switch outcome.ItemCount {
		case 0:
			yieldPoints = 10
		case 1:
			yieldPoints = 7
		case 2:
			yieldPoints = 4
		default:
			if outcome.ItemCount >= 5 {
				yieldPoints = -4
			}
		}
		yieldPressure += decayedPressurePoints(yieldPoints, outcome.CompletedAt, now, halfLife)
	}
	return clampInt(revealPressure, 0, 100), clampInt(updatePressure, 0, 100), clampInt(yieldPressure, -25, 50)
}

func decayedPressurePoints(points int, occurredAt, now time.Time, halfLife time.Duration) int {
	if points == 0 || occurredAt.IsZero() || occurredAt.After(now) {
		return 0
	}
	weight := math.Pow(0.5, now.Sub(occurredAt).Seconds()/halfLife.Seconds())
	return int(math.Round(float64(points) * weight))
}

func clampInt(value, minimum, maximum int) int {
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

func medianDuration(values []time.Duration) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}
