package store

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestAutoUpdateSchedulerReceiptsPersistCompleteAndStayBounded(t *testing.T) {
	state := openTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	receipt := domain.AutoUpdateTickReceipt{
		ID: "tick-initial", TickAt: now.Format(time.RFC3339Nano),
		Mode: "adaptive", CadenceTier: "warm", CadenceMinutes: 15,
		NextTickAt: now.Add(15 * time.Minute).Format(time.RFC3339Nano), Outcome: "checking",
	}
	if err := state.RecordAutoUpdateSchedulerTick(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	pending, err := state.AutoUpdateSchedulerReceipts(ctx, 10)
	if err != nil || len(pending) != 1 || pending[0].Outcome != "checking" || pending[0].DecidedAt != "" {
		t.Fatalf("pending receipts=%+v err=%v", pending, err)
	}
	schedule, err := state.AutoUpdateScheduleState(ctx)
	if err != nil || schedule.LastSchedulerTickAt != receipt.TickAt {
		t.Fatalf("schedule=%+v err=%v", schedule, err)
	}
	receipt.Outcome = "started"
	receipt.DecidedAt = now.Add(time.Second).Format(time.RFC3339Nano)
	receipt.SessionID = "session-test"
	if err := state.CompleteAutoUpdateSchedulerTick(ctx, receipt); err != nil {
		t.Fatal(err)
	}
	receipts, err := state.AutoUpdateSchedulerReceipts(ctx, 10)
	if err != nil || len(receipts) != 1 || receipts[0].Outcome != "started" || receipts[0].SessionID != "session-test" {
		t.Fatalf("receipts=%+v err=%v", receipts, err)
	}

	for index := 0; index < maxAutoUpdateSchedulerReceipts+3; index++ {
		tickAt := now.Add(time.Duration(index+1) * time.Minute)
		value := domain.AutoUpdateTickReceipt{
			ID: fmt.Sprintf("tick-%02d", index), TickAt: tickAt.Format(time.RFC3339Nano),
			Mode: "fixed", CadenceTier: "continuous", CadenceMinutes: 15,
			NextTickAt: tickAt.Add(15 * time.Minute).Format(time.RFC3339Nano), Outcome: "checking",
		}
		if err := state.RecordAutoUpdateSchedulerTick(ctx, value); err != nil {
			t.Fatal(err)
		}
	}
	receipts, err = state.AutoUpdateSchedulerReceipts(ctx, maxAutoUpdateSchedulerReceipts+10)
	if err != nil || len(receipts) != maxAutoUpdateSchedulerReceipts {
		t.Fatalf("bounded receipts=%+v err=%v", receipts, err)
	}
	if receipts[0].ID != "tick-34" {
		t.Fatalf("newest bounded receipt=%+v", receipts[0])
	}
}
