package engine

import (
	"context"
	"testing"
)

func TestPrepareReasoningExecutableChangePreservesSessionReadError(t *testing.T) {
	runtime, state, p, before := executableChangeFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, wantErr := state.ActiveSession(ctx)
	if wantErr != context.Canceled {
		t.Fatalf("cancelled-session query fixture: %v", wantErr)
	}
	runtime.operation.Lock()
	prepared, path, err := runtime.prepareReasoningExecutableChange(ctx, before.ReasoningExecutablePath, "changed")
	runtime.operation.Unlock()
	if prepared != nil || path != "" || err != wantErr || len(p.requests) != 0 {
		t.Fatalf("session-read error was changed or discovery ran: runtime=%v path=%q err=%v provider=%+v", prepared, path, err, p)
	}
	assertExecutableChangeUnchanged(t, state, p, before)
}
