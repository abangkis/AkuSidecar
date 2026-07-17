package reasoning

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func fakeCodexAppServer() {
	scanner := bufio.NewScanner(os.Stdin)
	thread := 0
	turn := 0
	for scanner.Scan() {
		var request map[string]any
		if json.Unmarshal(scanner.Bytes(), &request) != nil {
			continue
		}
		method, _ := request["method"].(string)
		id := request["id"]
		switch method {
		case "":
			if marker := os.Getenv("AKU_FAKE_CODEX_CALLBACK_MARKER"); marker != "" && request["error"] != nil {
				_ = os.WriteFile(marker, []byte("callback rejected"), 0o600)
			}
		case "initialized":
			continue
		case "initialize":
			if os.Getenv("AKU_FAKE_CODEX_CALLBACK_MARKER") != "" {
				fakeRPC(map[string]any{"id": "callback-1", "method": "item/requestToolCall", "params": map[string]any{"tool": "shell"}})
			}
			fakeRPC(map[string]any{"id": id, "result": map[string]any{"userAgent": "fake"}})
		case "thread/start":
			if marker := os.Getenv("AKU_FAKE_CODEX_EXIT_ONCE"); marker != "" {
				if _, err := os.Stat(marker); os.IsNotExist(err) {
					_ = os.WriteFile(marker, []byte("exited once"), 0o600)
					os.Exit(23)
				}
			}
			thread++
			fakeRPC(map[string]any{"id": id, "result": map[string]any{"thread": map[string]any{"id": fmt.Sprintf("thread-%d", thread)}}})
		case "turn/start":
			turn++
			turnID := fmt.Sprintf("turn-%d", turn)
			params, _ := request["params"].(map[string]any)
			threadID, _ := params["threadId"].(string)
			fakeRPC(map[string]any{"id": id, "result": map[string]any{"turn": map[string]any{"id": turnID}}})
			if os.Getenv("AKU_FAKE_CODEX_HANG_TURN") == fmt.Sprint(turn) {
				if marker := os.Getenv("AKU_FAKE_CODEX_HANG_MARKER"); marker != "" {
					_ = os.WriteFile(marker, []byte("hung"), 0o600)
				}
				for {
					time.Sleep(time.Hour)
				}
			}
			var output any
			schema, _ := params["outputSchema"].(map[string]any)
			properties, _ := schema["properties"].(map[string]any)
			capacityMarker := os.Getenv("AKU_FAKE_CODEX_CAPACITY_ONCE")
			if _, planning := properties["decision"]; !planning && capacityMarker != "" {
				if _, err := os.Stat(capacityMarker); os.IsNotExist(err) {
					_ = os.WriteFile(capacityMarker, []byte("failed once"), 0o600)
					fakeRPC(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "failed", "error": map[string]any{"message": "Selected model is at capacity. Please try a different model."}}}})
					continue
				}
			}
			if _, planning := properties["decision"]; planning {
				output = AcquisitionPlan{Decision: "finish", Reason: "enough bounded evidence"}
			} else {
				if os.Getenv("AKU_FAKE_CODEX_EMPTY_FINAL") == "1" {
					fakeRPC(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "completed", "items": []any{}}}})
					continue
				}
				output = domain.ReasoningResult{Summary: "fake app server", Items: []domain.ReasonedItem{{ID: "item-1", WhatChanged: "Changed", WhyItMatters: "Matters", Source: domain.SourceX, SourceURL: "https://x.com/post", SourceURLKind: "native_post", EvidenceKey: "candidate_001", EventKey: "event-one", KnowledgeDelta: "new_event", Author: "author", Confidence: .9, EvidenceState: "primary"}}, CandidateAssessments: []domain.CandidateAssessment{{EvidenceKey: "candidate_001", TopicTags: []string{"ai"}, TopicFacets: []string{"ai_models"}, ContentType: "release", Novelty: .8, Urgency: .4, Actionability: .6, Materiality: .8, EvidenceStrength: .9, Rationale: "fixture"}}, Limitations: []string{}}
			}
			raw, _ := json.Marshal(output)
			if marker := os.Getenv("AKU_FAKE_CODEX_MALFORMED_ONCE"); marker != "" {
				if _, err := os.Stat(marker); os.IsNotExist(err) {
					_ = os.WriteFile(marker, []byte("malformed once"), 0o600)
					raw = []byte(`{"incomplete"`)
				}
			}
			item := map[string]any{"id": "message-1", "type": "agentMessage", "text": string(raw), "phase": "finalAnswer"}
			fakeRPC(map[string]any{"method": "thread/tokenUsage/updated", "params": map[string]any{"threadId": threadID, "turnId": turnID, "tokenUsage": map[string]any{"last": map[string]any{"inputTokens": 11, "cachedInputTokens": 3, "outputTokens": 7, "reasoningOutputTokens": 2, "totalTokens": 18}, "total": map[string]any{"inputTokens": 11, "cachedInputTokens": 3, "outputTokens": 7, "reasoningOutputTokens": 2, "totalTokens": 18}}}})
			fakeRPC(map[string]any{"method": "item/completed", "params": map[string]any{"threadId": threadID, "turnId": turnID, "completedAtMs": 1, "item": item}})
			fakeRPC(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": threadID, "turn": map[string]any{"id": turnID, "status": "completed", "items": []any{item}}}})
		}
	}
}

func TestCodexAppServerRestartsAfterUnexpectedExitOnNextInvocation(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	marker := filepath.Join(t.TempDir(), "exit-once")
	t.Setenv("AKU_FAKE_CODEX_EXIT_ONCE", marker)
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Plan(context.Background(), run, observation, nil); err == nil {
		t.Fatal("unexpected App Server exit must fail the active invocation")
	}
	if provider.cmd != nil {
		t.Fatal("exited App Server process must be discarded")
	}
	plan, _, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" {
		t.Fatalf("fresh App Server did not recover the next invocation: plan=%+v err=%v", plan, err)
	}
}

func TestCodexAppServerRejectsCallbacks(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	marker := filepath.Join(t.TempDir(), "callback-rejected")
	t.Setenv("AKU_FAKE_CODEX_CALLBACK_MARKER", marker)
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Plan(context.Background(), run, observation, nil); err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("App Server callback was not rejected with a protocol error")
}

func TestCodexAppServerRejectsCompletedTurnWithoutFinalResponse(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	t.Setenv("AKU_FAKE_CODEX_EMPTY_FINAL", "1")
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Analyze(context.Background(), run, observation, nil); err == nil || !strings.Contains(err.Error(), "no final response") {
		t.Fatalf("empty completed turn error=%v", err)
	}
	if provider.cmd != nil {
		t.Fatal("invalid App Server process must be discarded after a protocol failure")
	}
}

func TestCodexAppServerDeadlineDiscardsProcessAndRecovers(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	t.Setenv("AKU_FAKE_CODEX_HANG_TURN", "2")
	marker := filepath.Join(t.TempDir(), "hung-turn")
	t.Setenv("AKU_FAKE_CODEX_HANG_MARKER", marker)
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Plan(context.Background(), run, observation, nil); err != nil {
		t.Fatalf("warm invocation failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, telemetry, err := provider.Plan(ctx, run, observation, nil); err == nil || !strings.Contains(err.Error(), "deadline exceeded") || telemetry.Status != "failed" {
		t.Fatalf("deadline result telemetry=%+v err=%v", telemetry, err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("fake App Server never reached the hanging turn: %v", err)
	}
	if provider.cmd != nil {
		t.Fatal("timed-out App Server process must be discarded")
	}
	plan, _, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" {
		t.Fatalf("fresh process did not recover after deadline: plan=%+v err=%v", plan, err)
	}
}

func TestCodexAppServerMalformedStructuredResultDoesNotPoisonTransport(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	marker := filepath.Join(t.TempDir(), "malformed-once")
	t.Setenv("AKU_FAKE_CODEX_MALFORMED_ONCE", marker)
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	if _, _, err := provider.Analyze(context.Background(), run, observation, nil); err == nil || !strings.Contains(err.Error(), "decode App Server reasoning result") {
		t.Fatalf("malformed structured result error=%v", err)
	}
	if provider.cmd == nil {
		t.Fatal("a model validation failure must not restart the healthy transport implicitly")
	}
	result, _, err := provider.Analyze(context.Background(), run, observation, nil)
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("transport did not remain reusable after malformed result: result=%+v err=%v", result, err)
	}
}

func TestCodexAppServerSerializesConcurrentAdaptersOnOneTransport(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	provider := newFakeCodexAppServer(t)
	defer provider.Close()
	run, observation := fakeAppServerInput()
	const workers = 6
	errorsFound := make(chan error, workers)
	var group sync.WaitGroup
	for index := 0; index < workers; index++ {
		group.Add(1)
		go func() {
			defer group.Done()
			plan, _, err := provider.Plan(context.Background(), run, observation, nil)
			if err == nil && plan.Decision != "finish" {
				err = fmt.Errorf("unexpected plan: %+v", plan)
			}
			errorsFound <- err
		}()
	}
	group.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
	if provider.cmd == nil || provider.nextID != 1+workers*2 {
		t.Fatalf("concurrent adapters did not share one serialized transport: cmd=%v nextID=%d", provider.cmd, provider.nextID)
	}
}

func TestAppServerRetryExcludesCancellationAndDeadline(t *testing.T) {
	if retryableAppServerError(context.Canceled) || retryableAppServerError(context.DeadlineExceeded) {
		t.Fatal("cancellation and deadline must never retry")
	}
	if !retryableAppServerError(fmt.Errorf("Selected model is at capacity. Please try a different model.")) {
		t.Fatal("explicit capacity failure must retry")
	}
}

func TestCodexAppServerCloseHasBoundedProcessWait(t *testing.T) {
	provider := &CodexAppServer{cmd: &exec.Cmd{}, done: make(chan error)}
	started := time.Now()
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 1500*time.Millisecond {
		t.Fatalf("App Server close exceeded shutdown budget: %s", elapsed)
	}
}

func fakeRPC(value any) {
	raw, _ := json.Marshal(value)
	fmt.Println(string(raw))
}

func TestCodexAppServerUsesOneManagedStructuredTransport(t *testing.T) {
	t.Setenv("AKU_FAKE_CODEX_APP_SERVER", "1")
	marker := filepath.Join(t.TempDir(), "capacity-once")
	t.Setenv("AKU_FAKE_CODEX_CAPACITY_ONCE", marker)
	root := filepathRoot(t)
	// Windows endpoint scanning can delay creation of a second copy of the Go
	// test executable. Keep the protocol fixture bounded without coupling its
	// deadline to that host-specific process-start latency.
	cfg := config.Config{Root: root, Reasoning: config.ReasoningConfig{Executable: os.Args[0], TimeoutMS: 60000, Planning: config.ModelConfig{Model: "fake", Effort: "high"}, Evaluation: config.ModelConfig{Model: "fake", Effort: "high"}}}
	provider, err := NewCodexAppServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run := domain.Run{ID: "run-1", Source: domain.SourceX}
	observation := domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Text: "Changed"}}}}, Coverage: map[string]any{}}
	plan, planTelemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil || plan.Decision != "finish" || planTelemetry.Provider != "codex-app-server" || planTelemetry.InputTokens == nil {
		t.Fatalf("plan=%+v telemetry=%+v err=%v", plan, planTelemetry, err)
	}
	result, telemetry, err := provider.Analyze(context.Background(), run, observation, nil)
	if err != nil || len(result.Items) != 1 || telemetry.Provider != "codex-app-server" || telemetry.OutputTokens == nil {
		t.Fatalf("result=%+v telemetry=%+v err=%v", result, telemetry, err)
	}
	if result.Items[0].EvidenceKey != "x:000000000000000000000001" || result.CandidateAssessments[0].EvidenceKey != "x:000000000000000000000001" {
		t.Fatalf("candidate aliases were not restored: %+v", result)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("capacity fixture did not execute: %v", err)
	}
	if _, _, err := provider.Analyze(context.Background(), run, observation, nil); err != nil {
		t.Fatalf("managed process was not reusable after recovery: %v", err)
	}
	if provider.cmd == nil || provider.nextID < 10 {
		t.Fatalf("managed process was not reused: cmd=%v nextID=%d", provider.cmd, provider.nextID)
	}
}

func newFakeCodexAppServer(t *testing.T) *CodexAppServer {
	t.Helper()
	cfg := config.Config{Root: filepathRoot(t), Reasoning: config.ReasoningConfig{Executable: os.Args[0], TimeoutMS: 60000, Planning: config.ModelConfig{Model: "fake", Effort: "high"}, Evaluation: config.ModelConfig{Model: "fake", Effort: "high"}}}
	provider, err := NewCodexAppServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func fakeAppServerInput() (domain.Run, domain.Observation) {
	return domain.Run{ID: "run-durability", Source: domain.SourceX}, domain.Observation{Source: domain.SourceX, Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Text: "Changed"}}}}, Coverage: map[string]any{}}
}
