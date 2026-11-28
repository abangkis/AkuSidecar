package httpapi

import (
	"cmp"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type activityLine struct {
	Producer, Operation, Phase, Outcome, Action, Poll string
	RequestID                                         string `json:"request_id"`
	Sequence                                          int
	Elapsed                                           *int64 `json:"elapsed_ms"`
	Queue                                             *struct {
		Total       int
		NativeReady int `json:"native_ready"`
	}
}

func readActivityLines(t *testing.T, output string) []activityLine {
	t.Helper()
	var events []activityLine
	for _, line := range strings.Split(output, "\n") {
		if payload, ok := strings.CutPrefix(line, "capture_activity "); ok {
			var event activityLine
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				t.Fatal(err)
			}
			events = append(events, event)
		}
	}
	return events
}

type failedActivitySink struct{}

func (failedActivitySink) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestSplitActionActivityInterleavedClaimsAndLateResult(t *testing.T) {
	s, token := splitTestServer(t)
	manager := attachSplitLeaseManager(t, s)
	var output strings.Builder
	// Capture the attempted bytes, then fail every write, including admission and settlement.
	s.logger = log.New(io.MultiWriter(&output, failedActivitySink{}), "", 0)
	s.SetSplitReaderPreparation(func(context.Context, string) (func(context.Context) error, error) {
		return func(context.Context) error { return nil }, nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	requests := []string{domain.NewID("broker"), domain.NewID("broker")}
	key := s.splitCapture.key
	var ids []string
	var waiters []chan *httptest.ResponseRecorder
	for i, id := range requests {
		done := make(chan *httptest.ResponseRecorder, 1)
		waiters = append(waiters, done)
		requestCtx := t.Context()
		if i == 0 {
			requestCtx = ctx
		}
		go func() {
			done <- splitRequestContext(requestCtx, s, token, "", "POST", "/api/split-capture/actions",
				`{"type":"open_native_post","source":"x","url":"https://private.example/PRIVATE_POST","requestId":"`+id+`"}`)
		}()
		waitForSplitTest(t, "native action admission", func() bool {
			s.splitCapture.mu.Lock()
			defer s.splitCapture.mu.Unlock()
			return len(s.splitCapture.actions) == i+1
		})
		reply := splitRequest(s, token, key, "GET", "/api/bridge/split-capture/next", "")
		var claim struct{ Action splitCaptureAction }
		if reply.Code != 200 || json.Unmarshal(reply.Body.Bytes(), &claim) != nil || claim.Action.RequestID != id {
			t.Fatalf("claim %s: %d %s", id, reply.Code, reply.Body)
		}
		ids = append(ids, claim.Action.ID)
		if i == 0 {
			// The reader gate admits the next click once this caller leaves.
			// Its claimed action stays in the queue until the late result.
			cancel()
			if w := <-done; w.Code != 500 || !strings.Contains(w.Body.String(), `"error":"internal_error"`) {
				t.Fatalf("cancelled request: %d %s", w.Code, w.Body)
			}
		}
	}
	if ids[0] == ids[1] {
		t.Fatal("repeated poll claimed the same action")
	}
	for _, phase := range []string{"prepare", "foreground"} {
		w := splitRequest(s, token, key, "POST", "/api/bridge/split-capture/reader/"+phase+"/"+ids[1], "{}")
		if w.Code != 200 {
			t.Fatalf("%s: %d %s", phase, w.Code, w.Body)
		}
	}
	const result = `{"ok":true,"result":{"windowId":2}}`
	postResult := func(id, body string) int {
		return splitRequest(s, token, key, "POST", "/api/bridge/split-capture/results/"+id, body).Code
	}
	if code := postResult(ids[1], result); code != 204 {
		t.Fatal("accepted result", code)
	}
	if w := <-waiters[1]; w.Code != 200 || w.Body.String() != result {
		t.Fatalf("UI response: %d %s", w.Code, w.Body)
	}
	if code := postResult(ids[0], `{"ok":false,"message":"PRIVATE_ERROR"}`); code != 204 {
		t.Fatal("late result", code)
	}
	if code := postResult(ids[0], result); code != 404 || manager.Snapshot().ActiveLeases != 0 {
		t.Fatal("replay or retained lease", code, manager.Snapshot())
	}

	lines := readActivityLines(t, output.String())
	polls := map[string]bool{}
	for i, id := range ids {
		var action []activityLine
		claims := 0
		for _, event := range lines {
			if event.Action != id {
				continue
			}
			if event.RequestID != requests[i] {
				t.Fatalf("crossed correlation: %+v", event)
			}
			switch event.Operation {
			case "split_action":
				action = append(action, event)
				if event.Producer != "sidecar" || event.Elapsed == nil || *event.Elapsed < 0 {
					t.Fatalf("invalid observation: %+v", event)
				}
			case "capture_poll":
				claims++
				if event.Outcome != "claimed" || event.Poll == "" || polls[event.Poll] {
					t.Fatalf("invalid claim: %+v", event)
				}
				polls[event.Poll] = true
			}
		}
		if claims != 1 || len(action) != 3-i {
			t.Fatalf("%s: claims=%d observations=%+v", id, claims, action)
		}
		slices.SortFunc(action, func(a, b activityLine) int { return cmp.Compare(a.Sequence, b.Sequence) })
		for index, event := range action {
			if event.Sequence != index+1 || index > 0 && *event.Elapsed < *action[index-1].Elapsed {
				t.Fatalf("invalid local sequence or elapsed time: %+v", action)
			}
		}
		first, last := action[0], action[len(action)-1]
		if first.Phase != "queued" || first.Outcome != "accepted" || first.Queue == nil || first.Queue.Total != i+1 ||
			first.Queue.NativeReady != 1 || *first.Elapsed != 0 || last.Phase != "result" || last.Queue != nil {
			t.Fatalf("invalid lifecycle: %+v", action)
		}
		if i == 0 && (action[1].Phase != "wait_ended" || action[1].Outcome != "client_cancelled" || last.Outcome != "rejected") ||
			i == 1 && last.Outcome != "accepted" {
			t.Fatalf("unexpected settlement: %+v", action)
		}
	}
	audit, err := s.engine.ReadSplitActionAudit(t.Context(), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for i, id := range ids {
		counts := map[string]int{}
		for _, row := range audit {
			if row.ActionID == id {
				counts[row.Phase+"/"+row.Outcome]++
			}
		}
		outcome := []string{"rejected", "accepted"}[i]
		if counts["queued/accepted"] != 1 || counts["claimed/accepted"] != 1 || counts["result/"+outcome] != 1 {
			t.Fatal("audit mismatch", counts)
		}
	}
	for _, secret := range []string{"PRIVATE_POST", "PRIVATE_ERROR", token, key} {
		if strings.Contains(output.String(), secret) {
			t.Fatal("private data in diagnostics")
		}
	}
}

func TestSplitActionActivityTimeoutHasNoInventedResultOrBrokerID(t *testing.T) {
	s, token := splitTestServer(t)
	s.splitCapture.actionTimeout = 10 * time.Millisecond
	var output strings.Builder
	s.logger = log.New(&output, "", 0)
	w := splitRequest(s, token, "", "POST", "/api/split-capture/actions",
		`{"type":"open_native_post","source":"x","url":"https://private.example","requestId":"PRIVATE_ID"}`)
	if w.Code != 504 || !strings.Contains(w.Body.String(), `"error":"capture_timeout"`) || len(s.splitCapture.actions) != 0 {
		t.Fatalf("timeout: %d %s", w.Code, w.Body)
	}
	events := readActivityLines(t, output.String())
	if len(events) != 2 || events[0].Action == "" || events[0].Action != events[1].Action ||
		events[1].Phase != "wait_ended" || events[1].Outcome != "timed_out" || events[1].Sequence != 2 ||
		strings.Contains(output.String(), "PRIVATE_ID") || strings.Contains(output.String(), `"request_id"`) {
		t.Fatal(events, output.String())
	}
}
