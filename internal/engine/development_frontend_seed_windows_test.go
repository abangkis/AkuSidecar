//go:build windows && auth_session_dev && auth_session_fixture

package engine

import "testing"

func TestDevelopmentFrontendNativeSeedTruthfulStoreItem(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	id, err := SeedDevelopmentFrontendNativeTimeline(h.ctx, h.e)
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.state.TimelineItem(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if item.Item.WhatChanged != "QA native frontend: synthetic reader" || item.Item.SourceURL != "https://x.com/aku_fixture_only/status/1" || item.Item.WhyItMatters != "This QA link opens only a synthetic loopback post through the real frontend and native broker. No real source account is used." {
		t.Fatal("positive seed/presentation/source mapping changed")
	}
}

func TestDevelopmentFrontendSeedFinalStoreTimeline(t *testing.T) {
	h := newSyntheticSidecarHarness(t, "", &syntheticCaptureBackend{})
	id, err := SeedDevelopmentFrontendTimeline(h.ctx, h.e)
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.state.TimelineItem(h.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	session, err := h.state.GetSession(h.ctx, item.SessionID)
	if err != nil || session.Status != "completed" || item.Item.SourceURL != "https://x.com/aku_fixture_only/status/1" {
		t.Fatal("not an actual final synthetic Timeline item", err)
	}
	active, err := h.state.ActiveSession(h.ctx)
	if err != nil || active != nil {
		t.Fatal("seed left active session", err)
	}
}
