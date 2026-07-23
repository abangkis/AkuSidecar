package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type AutoUpdateScheduleState struct {
	LastUIAccessAt     string
	LastAttemptAt      string
	LastSuccessAt      string
	LastQueueVacancyAt string
}

type AutoUpdateBudgetUsage struct {
	ActualTotal       int64
	ActualAutomatic   int64
	QuotaTotal        int64
	QuotaAutomatic    int64
	LastManualResetAt string
}

func (s *Store) RecordAutoUpdateUIAccess(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE auto_update_state SET last_ui_access_at=? WHERE id=1`, domain.Now())
	return err
}

func (s *Store) RecordAutoUpdateAttempt(ctx context.Context, message string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE auto_update_state SET last_attempt_at=?,last_error=? WHERE id=1`, domain.Now(), message)
	return err
}

func (s *Store) RecordAutoUpdateSuccess(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE auto_update_state SET last_success_at=?,last_error='' WHERE id=1`, domain.Now())
	return err
}

func (s *Store) AutoUpdateScheduleState(ctx context.Context) (AutoUpdateScheduleState, error) {
	var value AutoUpdateScheduleState
	var access, attempt, success, vacancy sql.NullString
	if err := s.db.QueryRowContext(ctx, `
		SELECT last_ui_access_at,last_attempt_at,last_success_at,
		  (SELECT value FROM meta WHERE key='auto_update_queue_vacancy_at')
		FROM auto_update_state WHERE id=1`).Scan(&access, &attempt, &success, &vacancy); err != nil {
		return value, err
	}
	value.LastUIAccessAt, value.LastAttemptAt, value.LastSuccessAt = access.String, attempt.String, success.String
	value.LastQueueVacancyAt = vacancy.String
	return value, nil
}

func (s *Store) PreparedBatches(ctx context.Context, _ int) ([]domain.PreparedBatch, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	expired, err := s.db.ExecContext(ctx, `UPDATE auto_update_batches SET state='expired' WHERE state='prepared' AND expires_at IS NOT NULL AND expires_at<=?`, now)
	if err != nil {
		return nil, err
	}
	if changed, changedErr := expired.RowsAffected(); changedErr != nil {
		return nil, changedErr
	} else if changed > 0 {
		if err := s.recordAutoUpdateQueueVacancy(ctx, now); err != nil {
			return nil, err
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT b.session_id,b.state,b.prepared_at,b.expires_at,COUNT(t.id),COALESCE(MAX(CAST(json_extract(t.assessment_json,'$.urgency') AS REAL)),0) AS urgency
		FROM auto_update_batches b LEFT JOIN timeline_items t ON t.session_id=b.session_id
		WHERE b.state='prepared'
		GROUP BY b.session_id,b.state,b.prepared_at,b.expires_at
		ORDER BY urgency DESC,b.prepared_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []domain.PreparedBatch{}
	for rows.Next() {
		var batch domain.PreparedBatch
		if err := rows.Scan(&batch.SessionID, &batch.Status, &batch.PreparedAt, &batch.ExpiresAt, &batch.ItemCount, &batch.Urgency); err != nil {
			return nil, err
		}
		result = append(result, batch)
	}
	return result, rows.Err()
}

func (s *Store) RevealPreparedBatch(ctx context.Context, sessionID string) (domain.PreparedBatch, error) {
	now := domain.Now()
	result, err := s.db.ExecContext(ctx, `UPDATE auto_update_batches SET state='visible',revealed_at=? WHERE session_id=? AND state='prepared' AND (expires_at IS NULL OR expires_at>?)`, now, sessionID, now)
	if err != nil {
		return domain.PreparedBatch{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return domain.PreparedBatch{}, err
	}
	if changed != 1 {
		return domain.PreparedBatch{}, errors.New("prepared batch is no longer available")
	}
	if err := s.recordAutoUpdateQueueVacancy(ctx, now); err != nil {
		return domain.PreparedBatch{}, err
	}
	var batch domain.PreparedBatch
	if err := s.db.QueryRowContext(ctx, `SELECT b.session_id,b.state,COALESCE(b.prepared_at,''),COALESCE(b.expires_at,''),COUNT(t.id),COALESCE(MAX(CAST(json_extract(t.assessment_json,'$.urgency') AS REAL)),0) FROM auto_update_batches b LEFT JOIN timeline_items t ON t.session_id=b.session_id WHERE b.session_id=? GROUP BY b.session_id,b.state,b.prepared_at,b.expires_at`, sessionID).Scan(&batch.SessionID, &batch.Status, &batch.PreparedAt, &batch.ExpiresAt, &batch.ItemCount, &batch.Urgency); err != nil {
		return domain.PreparedBatch{}, err
	}
	return batch, nil
}

func (s *Store) recordAutoUpdateQueueVacancy(ctx context.Context, at string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO meta(key,value) VALUES('auto_update_queue_vacancy_at',?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value`, at)
	return err
}

func (s *Store) DailyTokenUsage(ctx context.Context) (total, automatic int64, err error) {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).UTC().Format(time.RFC3339Nano)
	query := `
		WITH usage AS (
		  SELECT r.session_id AS session_id,COALESCE(i.input_tokens,0)+COALESCE(i.output_tokens,0)+COALESCE(i.reasoning_output_tokens,0) AS tokens,i.created_at AS created_at
		  FROM reasoning_invocations i JOIN runs r ON r.id=i.run_id
		  UNION ALL
		  SELECT session_id,COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(reasoning_output_tokens,0),created_at FROM event_resolution_invocations
		  UNION ALL
		  SELECT session_id,COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(reasoning_output_tokens,0),created_at FROM ai_detection_jobs
		)
		SELECT COALESCE(SUM(tokens),0),COALESCE(SUM(CASE WHEN
		  COALESCE(
		    (SELECT json_extract(s.coverage_json,'$.budgetAuthority') FROM sessions s WHERE s.id=usage.session_id),
		    CASE WHEN EXISTS (SELECT 1 FROM auto_update_batches b WHERE b.session_id=usage.session_id) THEN 'automatic' ELSE 'user' END
		  )='automatic'
		  THEN tokens ELSE 0 END),0)
		FROM usage WHERE created_at>=?`
	err = s.db.QueryRowContext(ctx, query, dayStart).Scan(&total, &automatic)
	return
}

func (s *Store) AutoUpdateBudgetUsage(ctx context.Context) (AutoUpdateBudgetUsage, error) {
	total, automatic, err := s.DailyTokenUsage(ctx)
	if err != nil {
		return AutoUpdateBudgetUsage{}, err
	}
	var resetDay, totalBaseline, automaticBaseline, resetAt string
	err = s.db.QueryRowContext(ctx, `
		SELECT
		  COALESCE((SELECT value FROM meta WHERE key='auto_update_budget_reset_day'),''),
		  COALESCE((SELECT value FROM meta WHERE key='auto_update_budget_reset_total'),'0'),
		  COALESCE((SELECT value FROM meta WHERE key='auto_update_budget_reset_automatic'),'0'),
		  COALESCE((SELECT value FROM meta WHERE key='auto_update_budget_reset_at'),'')`).Scan(&resetDay, &totalBaseline, &automaticBaseline, &resetAt)
	if err != nil {
		return AutoUpdateBudgetUsage{}, err
	}
	result := AutoUpdateBudgetUsage{ActualTotal: total, ActualAutomatic: automatic}
	if resetDay == time.Now().Format("2006-01-02") {
		baselineTotal, _ := strconv.ParseInt(totalBaseline, 10, 64)
		baselineAutomatic, _ := strconv.ParseInt(automaticBaseline, 10, 64)
		result.QuotaTotal = maxInt64(total-baselineTotal, 0)
		result.QuotaAutomatic = maxInt64(automatic-baselineAutomatic, 0)
		result.LastManualResetAt = resetAt
	} else {
		result.QuotaTotal = total
		result.QuotaAutomatic = automatic
	}
	return result, nil
}

func (s *Store) ResetAutoUpdateDailyQuota(ctx context.Context) (AutoUpdateBudgetUsage, error) {
	if err := s.requireIdle(ctx); err != nil {
		return AutoUpdateBudgetUsage{}, err
	}
	total, automatic, err := s.DailyTokenUsage(ctx)
	if err != nil {
		return AutoUpdateBudgetUsage{}, err
	}
	now := time.Now()
	values := map[string]string{
		"auto_update_budget_reset_day":       now.Format("2006-01-02"),
		"auto_update_budget_reset_total":     strconv.FormatInt(total, 10),
		"auto_update_budget_reset_automatic": strconv.FormatInt(automatic, 10),
		"auto_update_budget_reset_at":        now.Format(time.RFC3339Nano),
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AutoUpdateBudgetUsage{}, err
	}
	defer tx.Rollback()
	for key, value := range values {
		if _, err := tx.ExecContext(ctx, `INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value); err != nil {
			return AutoUpdateBudgetUsage{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return AutoUpdateBudgetUsage{}, err
	}
	return AutoUpdateBudgetUsage{ActualTotal: total, ActualAutomatic: automatic, LastManualResetAt: values["auto_update_budget_reset_at"]}, nil
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func (s *Store) EstimatedSessionTokens(ctx context.Context) (int64, error) {
	query := `
		WITH usage AS (
		  SELECT r.session_id AS session_id,COALESCE(i.input_tokens,0)+COALESCE(i.output_tokens,0)+COALESCE(i.reasoning_output_tokens,0) AS tokens
		  FROM reasoning_invocations i JOIN runs r ON r.id=i.run_id
		  UNION ALL
		  SELECT session_id,COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(reasoning_output_tokens,0) FROM event_resolution_invocations
		  UNION ALL
		  SELECT session_id,COALESCE(input_tokens,0)+COALESCE(output_tokens,0)+COALESCE(reasoning_output_tokens,0) FROM ai_detection_jobs
		), recent AS (
		  SELECT s.id,COALESCE(SUM(u.tokens),0) AS tokens
		  FROM sessions s LEFT JOIN usage u ON u.session_id=s.id
		  WHERE s.status IN ('completed','partial')
		  GROUP BY s.id,s.completed_at ORDER BY s.completed_at DESC LIMIT 5
		)
		SELECT COALESCE(AVG(tokens),0) FROM recent`
	var estimate float64
	if err := s.db.QueryRowContext(ctx, query).Scan(&estimate); err != nil {
		return 0, err
	}
	return int64(estimate), nil
}

func (s *Store) LastTerminalSessionAt(ctx context.Context) (string, error) {
	var value sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT MAX(completed_at) FROM sessions WHERE status IN ('completed','partial')`).Scan(&value)
	return value.String, err
}

func (s *Store) SessionPolicy(ctx context.Context, sessionID string, coverage map[string]any) (domain.UpdatePolicy, string, error) {
	var state string
	err := s.db.QueryRowContext(ctx, `SELECT state FROM auto_update_batches WHERE session_id=?`, sessionID).Scan(&state)
	hasPreparedDelivery := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.UpdatePolicy{}, "", err
	}
	policy := domain.UpdatePolicy{
		Trigger:         domain.UpdateTriggerUser,
		Delivery:        domain.UpdateDeliveryVisible,
		BudgetAuthority: domain.BudgetAuthorityUser,
	}
	if hasPreparedDelivery {
		policy.Trigger = domain.UpdateTriggerScheduler
		policy.Delivery = domain.UpdateDeliveryPrepared
		policy.BudgetAuthority = domain.BudgetAuthorityAutomatic
	}
	if value, ok := coverage["trigger"].(string); ok && value != "" {
		policy.Trigger = domain.UpdateTrigger(value)
	}
	if value, ok := coverage["delivery"].(string); ok && value != "" {
		policy.Delivery = domain.UpdateDelivery(value)
	}
	if value, ok := coverage["budgetAuthority"].(string); ok && value != "" {
		policy.BudgetAuthority = domain.BudgetAuthority(value)
	}
	if err := policy.Validate(); err != nil {
		return domain.UpdatePolicy{}, "", err
	}
	return policy, state, nil
}
