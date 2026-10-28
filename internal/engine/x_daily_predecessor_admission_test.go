//go:build auth_session_dev

package engine

import (
	"context"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func startDailyLinkedInWithQueuedX(t *testing.T) (*Engine, *syntheticDailyOwner, *domain.Session) {
	t.Helper()
	ctx := context.Background()
	e, owner, _ := dailyBrowserFixture(t, []domain.Source{domain.SourceLinkedIn, domain.SourceX})
	session, err := e.StartVisibleUpdate(ctx, "synthetic LinkedIn predecessor")
	if err != nil {
		t.Fatalf("start mixed daily update: %v", err)
	}
	if len(session.Runs) != 2 || session.Runs[0].Source != domain.SourceLinkedIn || session.Runs[1].Source != domain.SourceX {
		t.Fatalf("unexpected mixed source plan: %+v", session.Runs)
	}
	if session.Runs[0].Status != "waiting_for_bridge" || session.Runs[1].Status != "queued" {
		t.Fatalf("unexpected initial run states: %+v", session.Runs)
	}
	return e, owner, &session
}

func startLinkedInWithQueuedX(t *testing.T) (*Engine, *syntheticDailyOwner, *domain.Session, *domain.BridgeCommand) {
	t.Helper()
	ctx := context.Background()
	e, owner, session := startDailyLinkedInWithQueuedX(t)
	command, err := e.ClaimCommand(ctx, session.Runs[0].ID, "synthetic-linkedin")
	if err != nil || command == nil {
		t.Fatalf("claim LinkedIn predecessor: command=%+v err=%v", command, err)
	}
	return e, owner, session, command
}

func assertDailyXStillQueued(t *testing.T, e *Engine, owner *syntheticDailyOwner, xRunID string) {
	t.Helper()
	run, err := e.store.GetRun(context.Background(), xRunID)
	if err != nil {
		t.Fatalf("read queued X run: %v", err)
	}
	if run.Status != "queued" {
		t.Fatalf("X was started before predecessor drain: status=%s", run.Status)
	}
	e.xDaily.mu.RLock()
	done := e.xDaily.done
	retainedOwner := e.xDaily.owner == owner
	generation := e.xDaily.generation
	e.xDaily.mu.RUnlock()
	if done != nil || generation != 0 {
		t.Fatalf("X daily batch advanced before predecessor drain: done=%t generation=%d", done != nil, generation)
	}
	owner.mu.Lock()
	started := len(owner.generations)
	readerCloses := owner.readerCloses
	owner.mu.Unlock()
	if started != 0 {
		t.Fatalf("native X BeginBatch calls=%d want=0", started)
	}
	if !retainedOwner || !e.xDailyReady() || readerCloses != 0 {
		t.Fatalf("daily reader owner was not retained: attached=%t ready=%t readerCloses=%d", retainedOwner, e.xDailyReady(), readerCloses)
	}
}

func awaitPartialDailyBatch(t *testing.T, e *Engine, sessionID, xRunID string) {
	t.Helper()
	session := waitSession(t, e, sessionID, func(value domain.Session) bool { return value.Status == "partial" })
	var xCompleted bool
	for _, run := range session.Runs {
		if run.ID == xRunID {
			xCompleted = run.Status == "completed"
			break
		}
	}
	if !xCompleted {
		t.Fatalf("X did not complete in the partial session: %+v", session.Runs)
	}
	if !e.WaitForIdle(time.Second) {
		t.Fatal("daily batch and reasoning workers did not retire")
	}
	e.xDaily.mu.RLock()
	done := e.xDaily.done
	e.xDaily.mu.RUnlock()
	if done != nil {
		t.Fatal("daily batch remained registered after partial session completion")
	}
}

func TestXDailyTerminalPredecessorWithUndrainedCommandStillBlocks(t *testing.T) {
	for _, commandState := range []string{"queued", "claimed"} {
		t.Run(commandState, func(t *testing.T) {
			e, owner, session := startDailyLinkedInWithQueuedX(t)
			ctx := context.Background()
			if commandState == "claimed" {
				command, err := e.ClaimCommand(ctx, session.Runs[0].ID, "synthetic-linkedin")
				if err != nil || command == nil {
					t.Fatalf("claim LinkedIn predecessor: command=%+v err=%v", command, err)
				}
			}
			if err := e.store.FailRun(ctx, session.Runs[0].ID, domain.Failure{Code: "synthetic_capture_failed", Stage: "capture"}); err != nil {
				t.Fatalf("retire LinkedIn run: %v", err)
			}
			saved, err := e.store.GetSession(ctx, session.ID)
			if err != nil || saved.Runs[0].Status != "failed" || saved.Runs[0].BridgeCommandStatus != commandState {
				t.Fatalf("predecessor command state was not retained: run=%+v err=%v", saved.Runs[0], err)
			}
			drained, err := e.store.SessionCaptureCommandsDrained(ctx, session.ID, &session.Runs[1].Ordinal)
			if err != nil || drained {
				t.Fatalf("predecessor command unexpectedly drained: drained=%t err=%v", drained, err)
			}
			for attempt := 0; attempt < 3; attempt++ {
				if _, err := e.startNext(ctx, session.ID); err != nil {
					t.Fatalf("scheduler attempt with %s predecessor command: %v", commandState, err)
				}
				assertDailyXStillQueued(t, e, owner, session.Runs[1].ID)
			}
		})
	}
}

func TestXDailyQueuedRunWaitsForClaimedLinkedInPredecessor(t *testing.T) {
	e, owner, session, _ := startLinkedInWithQueuedX(t)
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := e.startNext(context.Background(), session.ID); err != nil {
			t.Fatalf("repeated scheduler attempt %d: %v", attempt+1, err)
		}
		assertDailyXStillQueued(t, e, owner, session.Runs[1].ID)
	}
}

func TestXDailyQueuedRunWaitsForPredecessorWorkerRetirement(t *testing.T) {
	for _, fence := range []string{"active", "pending"} {
		t.Run(fence, func(t *testing.T) {
			e, owner, session, command := startLinkedInWithQueuedX(t)
			ctx := context.Background()
			if _, err := e.store.FailCommand(ctx, command.ID, command.RunID, domain.Failure{Code: "synthetic_capture_failed", Stage: "capture"}); err != nil {
				t.Fatalf("retire LinkedIn capture command: %v", err)
			}
			predecessor, err := e.store.GetRun(ctx, session.Runs[0].ID)
			if err != nil || predecessor.Status != "failed" {
				t.Fatalf("LinkedIn predecessor did not reach a terminal state: run=%+v err=%v", predecessor, err)
			}

			e.mu.Lock()
			switch fence {
			case "active":
				e.active[predecessor.ID] = func() {}
			case "pending":
				e.pending[predecessor.ID] = false
			}
			e.mu.Unlock()
			t.Cleanup(func() {
				e.mu.Lock()
				delete(e.active, predecessor.ID)
				delete(e.pending, predecessor.ID)
				e.mu.Unlock()
			})

			for attempt := 0; attempt < 2; attempt++ {
				if _, err := e.startNext(ctx, session.ID); err != nil {
					t.Fatalf("scheduler attempt with %s predecessor: %v", fence, err)
				}
				assertDailyXStillQueued(t, e, owner, session.Runs[1].ID)
			}

			e.mu.Lock()
			delete(e.active, predecessor.ID)
			delete(e.pending, predecessor.ID)
			e.mu.Unlock()
			if _, err := e.startNext(ctx, session.ID); err != nil {
				t.Fatalf("scheduler after %s predecessor retirement: %v", fence, err)
			}
			awaitPartialDailyBatch(t, e, session.ID, session.Runs[1].ID)
			owner.mu.Lock()
			readerCloses := owner.readerCloses
			owner.mu.Unlock()
			if readerCloses != 0 {
				t.Fatalf("reader owner closed after predecessor retirement: closes=%d", readerCloses)
			}
		})
	}
}

func TestXDailyCompletionAndWorkerRetirementWakeQueuedRun(t *testing.T) {
	ctx := context.Background()
	e, owner, manager := dailyBrowserFixture(t, []domain.Source{domain.SourceLinkedIn, domain.SourceX})
	settings, err := e.Settings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	settings.SourceWaitMode = "full_wait"
	if _, err := e.SaveSettings(ctx, settings); err != nil {
		t.Fatalf("select full-wait scheduling for retirement barrier: %v", err)
	}
	session, err := e.StartVisibleUpdate(ctx, "automatic predecessor drain")
	if err != nil {
		t.Fatal(err)
	}
	e.schedule.Lock()
	scheduleLocked := true
	defer func() {
		if scheduleLocked {
			e.schedule.Unlock()
		}
	}()
	completeDailyBrowserRun(t, e, manager, session.ID, domain.SourceLinkedIn)
	e.mu.RLock()
	_, workerActive := e.active[session.Runs[0].ID]
	e.mu.RUnlock()
	if !workerActive {
		t.Fatal("reasoning worker retired before the scheduler barrier observed it")
	}
	assertDailyXStillQueued(t, e, owner, session.Runs[1].ID)
	e.schedule.Unlock()
	scheduleLocked = false
	awaitDailyBatch(t, e, session.ID)

	owner.mu.Lock()
	generations := append([]uint64(nil), owner.generations...)
	readerCloses := owner.readerCloses
	owner.mu.Unlock()
	if len(generations) != 1 || generations[0] != 1 {
		t.Fatalf("X did not start automatically after predecessor worker retirement: generations=%v", generations)
	}
	if readerCloses != 0 || !e.xDailyReady() {
		t.Fatalf("daily reader owner was not retained through automatic scheduling: closes=%d ready=%t", readerCloses, e.xDailyReady())
	}
}

func TestXDailyCancellationWhileWaitingDoesNotResurrectQueuedRun(t *testing.T) {
	e, owner, session, _ := startLinkedInWithQueuedX(t)
	ctx := context.Background()
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := e.startNext(ctx, session.ID); err != nil {
			t.Fatalf("scheduler attempt before cancellation: %v", err)
		}
		assertDailyXStillQueued(t, e, owner, session.Runs[1].ID)
	}
	if err := e.CancelSession(ctx, session.ID); err != nil {
		t.Fatalf("cancel waiting session: %v", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := e.startNext(ctx, session.ID); err != nil {
			t.Fatalf("scheduler attempt after cancellation: %v", err)
		}
	}
	cancelled, err := e.store.GetRun(ctx, session.Runs[1].ID)
	if err != nil || cancelled.Status != "cancelled" {
		t.Fatalf("queued X was resurrected after cancellation: run=%+v err=%v", cancelled, err)
	}
	owner.mu.Lock()
	started, readerCloses := len(owner.generations), owner.readerCloses
	owner.mu.Unlock()
	if started != 0 || readerCloses != 0 || !e.xDailyReady() {
		t.Fatalf("cancellation changed daily reader/batch state: begins=%d readerCloses=%d ready=%t", started, readerCloses, e.xDailyReady())
	}
	if !e.WaitForIdle(time.Second) {
		t.Fatal("synthetic daily workers did not drain after cancellation")
	}
}
