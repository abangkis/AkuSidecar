package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (s *Store) CreateUpdateSession(ctx context.Context, intent string, settings domain.Settings, policy domain.UpdatePolicy) (domain.Session, error) {
	return s.createUpdateSession(ctx, intent, settings, policy, nil)
}

// The daily X route changes only X's frozen driver, never saved preferences or
// the ordinary routing decisions for the other selected sources.
func (s *Store) CreateDailyXUpdateSession(ctx context.Context, intent string, settings domain.Settings, policy domain.UpdatePolicy) (domain.Session, error) {
	return s.createUpdateSession(ctx, intent, settings, policy, []domain.Source{domain.SourceX})
}

func (s *Store) CreateDailySourceUpdateSession(ctx context.Context, intent string, settings domain.Settings, policy domain.UpdatePolicy, sources []domain.Source) (domain.Session, error) {
	for _, source := range sources {
		if !supportedDailySource(source) {
			return domain.Session{}, errManualPilot
		}
	}
	return s.createUpdateSession(ctx, intent, settings, policy, sources)
}

func (s *Store) createUpdateSession(ctx context.Context, intent string, settings domain.Settings, policy domain.UpdatePolicy, dailySources []domain.Source) (domain.Session, error) {
	if err := domain.ValidateIntent(intent); err != nil {
		return domain.Session{}, err
	}
	if err := policy.Validate(); err != nil {
		return domain.Session{}, err
	}
	open, err := s.ActiveSession(ctx)
	if err != nil {
		return domain.Session{}, err
	}
	if open != nil {
		return domain.Session{}, errors.New("an active session already exists")
	}
	now := s.Now().UTC().Format(time.RFC3339Nano)
	sessionID := domain.NewID("session")
	configuredSources := append([]domain.Source(nil), settings.ActiveSources...)
	sources := append([]domain.Source(nil), configuredSources...)
	coverageValue := map[string]any{
		"sourceWaitMode":  settings.SourceWaitMode,
		"trigger":         policy.Trigger,
		"delivery":        policy.Delivery,
		"budgetAuthority": policy.BudgetAuthority,
	}
	if settings.CollectionMode == "headless" {
		// Freeze the experimental hybrid source plan with the session. Facebook
		// remains a Bridge capture; all other admitted sources use headless.
		ordered := make([]domain.Source, 0, len(sources))
		for _, source := range sources {
			if source != domain.SourceFacebook {
				ordered = append(ordered, source)
			}
		}
		for _, source := range sources {
			if source == domain.SourceFacebook {
				ordered = append(ordered, source)
			}
		}
		sources = ordered
		drivers := make(map[string]string, len(sources))
		for _, source := range sources {
			driver := "headless"
			if source == domain.SourceFacebook {
				driver = "browser"
			}
			drivers[string(source)] = driver
		}
		coverageValue["collectionPolicy"] = domain.SessionCollectionPolicyHybridHeadlessV1
		coverageValue["collectionSourceDrivers"] = drivers
		coverageValue["collectionSources"] = configuredSources
	}
	if len(dailySources) > 0 {
		drivers, ok := coverageValue["collectionSourceDrivers"].(map[string]string)
		if !ok {
			drivers = map[string]string{}
			for _, source := range sources {
				drivers[string(source)] = "browser"
			}
		}
		for _, source := range dailySources {
			if _, ok := drivers[string(source)]; ok {
				drivers[string(source)] = "headless"
			}
		}
		coverageValue["collectionPolicy"] = domain.SessionCollectionPolicyHybridHeadlessV1
		coverageValue["collectionSourceDrivers"] = drivers
		coverageValue["collectionSources"] = configuredSources
	}
	coverage, err := json.Marshal(coverageValue)
	if err != nil {
		return domain.Session{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Session{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO sessions(id,intent,status,max_items_per_source,max_items_total,coverage_json,created_at) VALUES(?,?,'queued',?,?,?,?)`, sessionID, strings.TrimSpace(intent), settings.MaxItemsPerSource, settings.MaxItemsTotal, string(coverage), now); err != nil {
		return domain.Session{}, err
	}
	if policy.Delivery == domain.UpdateDeliveryPrepared {
		if _, err := tx.ExecContext(ctx, `INSERT INTO auto_update_batches(session_id,state,created_at) VALUES(?,'preparing',?)`, sessionID, now); err != nil {
			return domain.Session{}, err
		}
	}
	for ordinal, source := range sources {
		if _, err := tx.ExecContext(ctx, `INSERT INTO runs(id,session_id,source,ordinal,status,stage,created_at) VALUES(?,?,?,?,'queued','queued',?)`, domain.NewID("run"), sessionID, source, ordinal, now); err != nil {
			return domain.Session{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.Session{}, err
	}
	return s.GetSession(ctx, sessionID)
}
