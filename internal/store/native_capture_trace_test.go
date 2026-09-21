package store

import (
	"context"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/nativetrace"
)

func TestNativeCaptureTraceReadbackCorrelatesDelayedBridgeReceipt(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	settings, err := state.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	session, err := createVisibleUpdateSession(state, ctx, "native trace", settings)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := state.listRuns(ctx, session.ID)
	if err != nil || len(runs) == 0 {
		t.Fatalf("runs=%v err=%v", runs, err)
	}
	run := runs[0]
	for _, event := range []domain.CaptureSurfaceEvent{
		{ID: "native-1", Event: "native_trace", OccurredAt: "2026-09-21T00:00:00.200100Z", Detail: map[string]any{"schema": "windows-capture-native-v1", "native": nativetrace.Sample{Foreground: "0x123", Status: "available", Windows: []nativetrace.Window{{PID: 42, Class: "akubrowser_root"}}}}},
		{ID: "bridge-1", Event: "created", OccurredAt: "2026-09-21T00:00:00.200Z", Detail: map[string]any{"isolation": "per_source"}},
	} {
		event.SessionID, event.RunID, event.Source = session.ID, run.ID, run.Source
		if _, err = state.RecordCaptureSurfaceEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	sessions, _, err := state.ListInboxSessions(ctx, 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var events []domain.CaptureSurfaceEvent
	for _, r := range sessions[0].Runs {
		if r.ID == run.ID {
			events = r.CaptureSurface
		}
	}
	if len(events) != 2 || events[0].Event != "created" || events[1].Event != "native_trace" {
		t.Fatalf("correlation order: %+v", events)
	}
	native := events[1].Detail["native"].(map[string]any)
	if native["foregroundHwnd"] != "0x123" || native["status"] != "available" {
		t.Fatalf("native evidence lost: %+v", native)
	}
}
