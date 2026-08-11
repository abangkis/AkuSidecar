package store

import (
	"context"
	"sort"
	"time"
)

type AutoUpdateAdaptiveSignals struct {
	ConsumptionPace    time.Duration
	ConsumptionSamples int
	PreparationLead    time.Duration
	GenerationAttempts []time.Time
	LastYieldItems     int
	EmptyYieldStreak   int
	SupplyBackoffUntil time.Time
}

func (s *Store) AutoUpdateAdaptiveSignals(ctx context.Context, generationWindow, defaultPreparationLead time.Duration) (AutoUpdateAdaptiveSignals, error) {
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
	latestCompletedAt, yields, err := s.recentPreparedUpdateYields(ctx, 3)
	if err != nil {
		return result, err
	}
	if len(yields) > 0 {
		result.LastYieldItems = yields[0]
		for _, itemCount := range yields {
			if itemCount > 0 {
				break
			}
			result.EmptyYieldStreak++
		}
	}
	if result.EmptyYieldStreak > 0 && !latestCompletedAt.IsZero() {
		backoff := 15 * time.Minute
		if result.EmptyYieldStreak == 2 {
			backoff = 30 * time.Minute
		} else if result.EmptyYieldStreak >= 3 {
			backoff = 60 * time.Minute
		}
		result.SupplyBackoffUntil = latestCompletedAt.Add(backoff)
	}
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

func medianDuration(values []time.Duration) time.Duration {
	ordered := append([]time.Duration(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}
