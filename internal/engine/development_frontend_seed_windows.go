//go:build windows && auth_session_dev && auth_session_fixture

package engine

import (
	"context"
	"errors"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func (r *DevelopmentFixtureRuntime) VerifyFrontend(ctx context.Context) error {
	if r.Engine.developmentVerifyBrowser == nil {
		return errors.New("development frontend authority unavailable")
	}
	return r.Engine.developmentVerifyBrowser(ctx)
}

// SeedFrontendTimeline uses the real Store's finalization/readback path before
// the active batch. No DOM or UI data response is fabricated.
func SeedDevelopmentFrontendTimeline(ctx context.Context, e *Engine) (string, error) {
	return seedDevelopmentFrontendTimeline(ctx, e, false)
}
func SeedDevelopmentFrontendNativeTimeline(ctx context.Context, e *Engine) (string, error) {
	return seedDevelopmentFrontendTimeline(ctx, e, true)
}
func seedDevelopmentFrontendTimeline(ctx context.Context, e *Engine, native bool) (string, error) {
	settings, err := e.store.GetSettings(ctx)
	if err != nil {
		return "", err
	}
	settings.ActiveSources = []domain.Source{domain.SourceX}
	session, err := e.store.CreateUpdateSession(ctx, "Synthetic QA frontend Timeline", settings, domain.UpdatePolicy{Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser})
	if err != nil {
		return "", err
	}
	if len(session.Runs) != 1 {
		return "", errors.New("frontend seed run count invalid")
	}
	run := session.Runs[0]
	reasoned := domain.ReasonedItem{Source: domain.SourceX, EvidenceKey: "x:qa-frontend-1", SourceURL: "https://x.com/aku_fixture_only/status/1", Author: "Synthetic QA only", WhatChanged: "QA frontend: native activation is disabled", WhyItMatters: "This isolated synthetic item tests the actual interface and relay; no real source account is used."}
	if native {
		reasoned.WhatChanged = "QA native frontend: synthetic reader"
		reasoned.WhyItMatters = "This QA link opens only a synthetic loopback post through the real frontend and native broker. No real source account is used."
	}
	item := domain.TimelineItem{ID: domain.NewID("timeline"), SessionID: session.ID, RunID: run.ID, Source: domain.SourceX, EvidenceKey: reasoned.EvidenceKey, Item: reasoned, Coverage: map[string]any{"quality": "complete"}}
	if err := e.store.CompleteRun(ctx, run, domain.ReasoningResult{Summary: "Synthetic frontend fixture", Items: []domain.ReasonedItem{reasoned}}, nil, []domain.TimelineItem{item}, map[string]any{"quality": "complete"}); err != nil {
		return "", err
	}
	if err := e.store.ComposeSession(ctx, session.ID); err != nil {
		return "", err
	}
	if err := e.store.FinalizeSession(ctx, session.ID); err != nil {
		return "", err
	}
	items, err := e.Timeline(ctx, 20, 0)
	if err != nil {
		return "", err
	}
	final, err := e.store.GetSession(ctx, session.ID)
	if err != nil || final.Status != "completed" {
		return "", errors.New("frontend seed session not final")
	}
	for _, v := range items {
		if v.ID == item.ID && v.Item.SourceURL == reasoned.SourceURL {
			return item.ID, nil
		}
	}
	return "", errors.New("frontend final Timeline seed readback missing")
}
