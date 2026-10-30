package engine

import (
	"context"
	"testing"
	"time"
)

func TestExpiredUnclaimedBridgeCommandAdvancesSession(t *testing.T) {
	e, _ := testEngine(t)
	ctx := context.Background()
	session, err := e.StartVisibleUpdate(ctx, "synthetic unclaimed dispatch")
	if err != nil {
		t.Fatal(err)
	}
	if err = e.recoverExpiredBridgeCommands(ctx, time.Now().Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	got, err := e.Session(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Runs[0].Status != "failed" || got.Runs[0].Error == nil || got.Runs[0].Error.Code != "capture_dispatch_timeout" {
		t.Fatalf("first: %+v", got.Runs[0])
	}
	if got.Runs[1].Status != "waiting_for_bridge" {
		t.Fatalf("next: %+v", got.Runs[1])
	}
}
