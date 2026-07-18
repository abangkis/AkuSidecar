package reasoning

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/codexruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

type CodexAppServer struct {
	executable   string
	pathDirs     []string
	root         string
	timeout      time.Duration
	planning     config.ModelConfig
	evaluation   config.ModelConfig
	planSchema   any
	resultSchema any

	invokeMu      sync.Mutex
	writeMu       sync.Mutex
	pendingMu     sync.Mutex
	cmd           *exec.Cmd
	stdin         io.WriteCloser
	pending       map[string]chan rpcMessage
	notifications chan rpcMessage
	done          chan error
	nextID        uint64
	stderr        *boundedBuffer
}

const appServerStopWait = 750 * time.Millisecond

type usage struct{ Input, CachedInput, Output, ReasoningOutput *int64 }

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type boundedBuffer struct {
	mu    sync.Mutex
	value []byte
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.value = append(b.value, value...)
	if len(b.value) > 32*1024 {
		b.value = append([]byte(nil), b.value[len(b.value)-32*1024:]...)
	}
	return len(value), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.value))
}

func NewCodexAppServer(cfg config.Config) (*CodexAppServer, error) {
	executable, err := resolveExecutable(cfg.Root, cfg.Reasoning.Executable)
	if err != nil {
		return nil, err
	}
	planSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "acquisition-plan.schema.json"))
	if err != nil {
		return nil, err
	}
	resultSchema, err := readSchema(filepath.Join(cfg.Root, "schemas", "reasoning-result.schema.json"))
	if err != nil {
		return nil, err
	}
	return &CodexAppServer{
		executable:   executable,
		pathDirs:     codexPathDirs(executable),
		root:         cfg.Root,
		timeout:      time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		planning:     cfg.Reasoning.Planning,
		evaluation:   cfg.Reasoning.Evaluation,
		planSchema:   planSchema,
		resultSchema: resultSchema,
		pending:      map[string]chan rpcMessage{},
	}, nil
}

func (c *CodexAppServer) Name() string { return "codex-app-server" }

func (c *CodexAppServer) ProfileOptions() []ProfileOption {
	return []ProfileOption{
		{ID: "luna_high", Label: "Luna High", Model: "gpt-5.6-luna", Effort: "high"},
		{ID: "luna_xhigh", Label: "Luna XHigh", Model: "gpt-5.6-luna", Effort: "xhigh"},
		{ID: "terra_high", Label: "Terra High", Model: "gpt-5.6-terra", Effort: "high"},
		{ID: "terra_xhigh", Label: "Terra XHigh", Model: "gpt-5.6-terra", Effort: "xhigh"},
		{ID: "sol_medium", Label: "Sol Medium", Model: "gpt-5.6-sol", Effort: "medium"},
	}
}

func (c *CodexAppServer) ResolveProfile(id string) (config.ModelConfig, bool) {
	for _, option := range c.ProfileOptions() {
		if option.ID == id {
			return config.ModelConfig{Model: option.Model, Effort: option.Effort}, true
		}
	}
	return config.ModelConfig{}, false
}

// InvokeStructured exposes the shared App Server transport to bounded adapters
// without adding their domain-specific methods to the reasoning Provider.
func (c *CodexAppServer) InvokeStructured(ctx context.Context, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	raw, value, duration, err := c.invoke(ctx, prompt, schema, model)
	return raw, domain.ModelUsage{Input: value.Input, CachedInput: value.CachedInput, Output: value.Output, ReasoningOutput: value.ReasoningOutput}, duration, err
}

func (c *CodexAppServer) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return c.PlanWithModel(ctx, run, observation, knowledge, c.planning)
}

func (c *CodexAppServer) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, model config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	raw, usage, duration, err := c.invoke(ctx, buildPlanningPrompt(run, observation, knowledge), c.planSchema, model)
	telemetry := appServerTelemetry(run, "acquisition_planning", model, duration, usage, err)
	if err != nil {
		return AcquisitionPlan{}, telemetry, err
	}
	var plan AcquisitionPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("decode App Server acquisition plan: %w", err)
	}
	if plan.Decision != "finish" && plan.Decision != "request_follow_up" {
		return AcquisitionPlan{}, telemetry, fmt.Errorf("invalid acquisition decision %q", plan.Decision)
	}
	return plan, telemetry, nil
}

func (c *CodexAppServer) Analyze(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	return c.AnalyzeWithModel(ctx, run, observation, knowledge, c.evaluation)
}

func (c *CodexAppServer) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	request := buildEvaluationRequest(run, observation, knowledge)
	raw, usage, duration, err := c.invoke(ctx, request.prompt, c.resultSchema, model)
	telemetry := appServerTelemetry(run, "candidate_evaluation", model, duration, usage, err)
	if err != nil {
		return domain.ReasoningResult{}, telemetry, err
	}
	var result domain.ReasoningResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return domain.ReasoningResult{}, telemetry, fmt.Errorf("decode App Server reasoning result: %w", err)
	}
	if err := bindEvidenceKeysByPosition(&result, request.evidenceKeys); err != nil {
		return domain.ReasoningResult{}, telemetry, err
	}
	return result, telemetry, nil
}

func (c *CodexAppServer) invoke(parent context.Context, prompt string, schema any, model config.ModelConfig) (string, usage, time.Duration, error) {
	c.invokeMu.Lock()
	defer c.invokeMu.Unlock()
	ctx, cancel := context.WithTimeout(parent, c.timeout)
	defer cancel()
	started := time.Now()
	var totalUsage usage
	for attempt := 1; attempt <= 2; attempt++ {
		raw, attemptUsage, err := c.invokeAttemptLocked(ctx, prompt, schema, model)
		totalUsage = addUsage(totalUsage, attemptUsage)
		if err == nil {
			return raw, totalUsage, time.Since(started), nil
		}
		retry := attempt < 2 && retryableAppServerError(err) && ctx.Err() == nil
		c.stopLocked(true)
		if !retry {
			return "", totalUsage, time.Since(started), err
		}
		// Capacity is process-transient. Retry once through a fresh App Server,
		// with the same model and the same overall invocation deadline.
	}
	panic("unreachable App Server retry loop")
}

func (c *CodexAppServer) invokeAttemptLocked(ctx context.Context, prompt string, schema any, model config.ModelConfig) (string, usage, error) {
	if err := c.ensureStartedLocked(ctx); err != nil {
		return "", usage{}, err
	}
	c.drainNotifications()

	threadResult, err := c.callLocked(ctx, "thread/start", map[string]any{
		"model":            model.Model,
		"cwd":              c.root,
		"approvalPolicy":   "never",
		"sandbox":          "read-only",
		"ephemeral":        true,
		"baseInstructions": "Return only the requested structured result. Do not use tools, browse, execute commands, edit files, or read workspace files.",
		"config":           map[string]any{"web_search": "disabled", "approval_policy": "never", "sandbox_workspace_write": map[string]any{"network_access": false}},
	})
	if err != nil {
		return "", usage{}, err
	}
	var thread struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := json.Unmarshal(threadResult, &thread); err != nil || thread.Thread.ID == "" {
		return "", usage{}, fmt.Errorf("decode App Server thread/start response: %w", err)
	}
	turnResult, err := c.callLocked(ctx, "turn/start", map[string]any{
		"threadId":       thread.Thread.ID,
		"input":          []map[string]any{{"type": "text", "text": prompt}},
		"model":          model.Model,
		"effort":         model.Effort,
		"approvalPolicy": "never",
		"outputSchema":   schema,
	})
	if err != nil {
		return "", usage{}, err
	}
	var turn struct {
		Turn struct {
			ID string `json:"id"`
		} `json:"turn"`
	}
	if err := json.Unmarshal(turnResult, &turn); err != nil || turn.Turn.ID == "" {
		return "", usage{}, fmt.Errorf("decode App Server turn/start response: %w", err)
	}
	final, tokenUsage, err := c.waitTurnLocked(ctx, thread.Thread.ID, turn.Turn.ID)
	if err != nil {
		return "", tokenUsage, err
	}
	return final, tokenUsage, nil
}

func retryableAppServerError(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "model is at capacity")
}

func addUsage(left, right usage) usage {
	return usage{
		Input:           addTokenCount(left.Input, right.Input),
		CachedInput:     addTokenCount(left.CachedInput, right.CachedInput),
		Output:          addTokenCount(left.Output, right.Output),
		ReasoningOutput: addTokenCount(left.ReasoningOutput, right.ReasoningOutput),
	}
}

func addTokenCount(left, right *int64) *int64 {
	if left == nil && right == nil {
		return nil
	}
	var total int64
	if left != nil {
		total += *left
	}
	if right != nil {
		total += *right
	}
	return &total
}

func (c *CodexAppServer) ensureStartedLocked(ctx context.Context) error {
	if c.cmd != nil {
		return nil
	}
	cmd := exec.Command(c.executable, "app-server", "--listen", "stdio://")
	cmd.Dir = c.root
	cmd.Env = codexEnvironment(c.pathDirs)
	configureProcess(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr := &boundedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Codex App Server: %w", err)
	}
	c.cmd, c.stdin, c.stderr = cmd, stdin, stderr
	c.notifications = make(chan rpcMessage, 1024)
	c.done = make(chan error, 1)
	go c.readLoop(cmd, stdout, c.done, c.notifications)
	if _, err := c.callLocked(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "AkuSidecar", "title": "AkuSidecar Go", "version": domain.ApplicationVersion},
		"capabilities": map[string]any{"experimentalApi": false},
	}); err != nil {
		c.stopLocked(true)
		return fmt.Errorf("initialize Codex App Server: %w", err)
	}
	if err := c.write(map[string]any{"method": "initialized"}); err != nil {
		c.stopLocked(true)
		return err
	}
	return nil
}

func (c *CodexAppServer) callLocked(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.nextID++
	id := fmt.Sprintf("aku-%d", c.nextID)
	result := make(chan rpcMessage, 1)
	c.pendingMu.Lock()
	c.pending[id] = result
	c.pendingMu.Unlock()
	if err := c.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		c.removePending(id)
		return nil, err
	}
	select {
	case response := <-result:
		if response.Error != nil {
			return nil, fmt.Errorf("App Server %s error %d: %s", method, response.Error.Code, response.Error.Message)
		}
		return response.Result, nil
	case err := <-c.done:
		c.removePending(id)
		return nil, fmt.Errorf("Codex App Server exited: %w: %s", err, c.stderrText())
	case <-ctx.Done():
		c.removePending(id)
		c.stopLocked(true)
		return nil, fmt.Errorf("Codex App Server %s timed out: %w", method, ctx.Err())
	}
}

func (c *CodexAppServer) waitTurnLocked(ctx context.Context, threadID, turnID string) (string, usage, error) {
	var final string
	var tokenUsage usage
	for {
		select {
		case message := <-c.notifications:
			switch message.Method {
			case "item/completed":
				var event struct {
					ThreadID string         `json:"threadId"`
					TurnID   string         `json:"turnId"`
					Item     map[string]any `json:"item"`
				}
				if json.Unmarshal(message.Params, &event) == nil && event.ThreadID == threadID && event.TurnID == turnID {
					if text := agentMessageText(event.Item); text != "" {
						final = text
					}
				}
			case "thread/tokenUsage/updated":
				var event struct {
					ThreadID   string `json:"threadId"`
					TurnID     string `json:"turnId"`
					TokenUsage struct {
						Last struct {
							Input     int64 `json:"inputTokens"`
							Cached    int64 `json:"cachedInputTokens"`
							Output    int64 `json:"outputTokens"`
							Reasoning int64 `json:"reasoningOutputTokens"`
						} `json:"last"`
					} `json:"tokenUsage"`
				}
				if json.Unmarshal(message.Params, &event) == nil && event.ThreadID == threadID && event.TurnID == turnID {
					tokenUsage = usage{Input: ptr(event.TokenUsage.Last.Input), CachedInput: ptr(event.TokenUsage.Last.Cached), Output: ptr(event.TokenUsage.Last.Output), ReasoningOutput: ptr(event.TokenUsage.Last.Reasoning)}
				}
			case "turn/completed":
				var event struct {
					ThreadID string `json:"threadId"`
					Turn     struct {
						ID     string `json:"id"`
						Status string `json:"status"`
						Error  *struct {
							Message string `json:"message"`
						} `json:"error"`
						Items []map[string]any `json:"items"`
					} `json:"turn"`
				}
				if json.Unmarshal(message.Params, &event) != nil || event.ThreadID != threadID || event.Turn.ID != turnID {
					continue
				}
				if event.Turn.Status != "completed" {
					message := "turn did not complete"
					if event.Turn.Error != nil && event.Turn.Error.Message != "" {
						message = event.Turn.Error.Message
					}
					return "", tokenUsage, errors.New(message)
				}
				if final == "" {
					for _, item := range event.Turn.Items {
						if text := agentMessageText(item); text != "" {
							final = text
						}
					}
				}
				if strings.TrimSpace(final) == "" {
					return "", tokenUsage, errors.New("Codex App Server returned no final response")
				}
				return final, tokenUsage, nil
			}
		case err := <-c.done:
			return "", tokenUsage, fmt.Errorf("Codex App Server exited: %w: %s", err, c.stderrText())
		case <-ctx.Done():
			return "", tokenUsage, fmt.Errorf("Codex App Server turn timed out: %w", ctx.Err())
		}
	}
}

func (c *CodexAppServer) readLoop(cmd *exec.Cmd, stdout io.Reader, done chan<- error, notifications chan<- rpcMessage) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var message rpcMessage
		if err := json.Unmarshal(scanner.Bytes(), &message); err != nil {
			continue
		}
		if len(message.ID) > 0 && (len(message.Result) > 0 || message.Error != nil) {
			id := rawID(message.ID)
			c.pendingMu.Lock()
			channel := c.pending[id]
			delete(c.pending, id)
			c.pendingMu.Unlock()
			if channel != nil {
				channel <- message
			}
			continue
		}
		if len(message.ID) > 0 && message.Method != "" {
			_ = c.write(map[string]any{"id": rawValue(message.ID), "error": map[string]any{"code": -32601, "message": "AkuSidecar does not authorize App Server callbacks"}})
			continue
		}
		notifications <- message
	}
	err := scanner.Err()
	if waitErr := cmd.Wait(); err == nil {
		err = waitErr
	}
	if err == nil {
		err = errors.New("process closed")
	}
	done <- err
}

func (c *CodexAppServer) write(value any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.stdin == nil {
		return errors.New("Codex App Server stdin is unavailable")
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	_, err = c.stdin.Write(raw)
	return err
}

func (c *CodexAppServer) drainNotifications() {
	for {
		select {
		case <-c.notifications:
			continue
		default:
			return
		}
	}
}

func (c *CodexAppServer) removePending(id string) {
	c.pendingMu.Lock()
	delete(c.pending, id)
	c.pendingMu.Unlock()
}

func (c *CodexAppServer) stopLocked(wait bool) {
	if c.cmd == nil && c.stdin == nil {
		c.done = nil
		return
	}
	done := c.done
	alreadyExited := c.cmd != nil && c.cmd.ProcessState != nil && c.cmd.ProcessState.Exited()
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	if wait && done != nil && !alreadyExited {
		select {
		case <-done:
		case <-time.After(appServerStopWait):
		}
	}
	c.cmd, c.stdin, c.done = nil, nil, nil
}

func (c *CodexAppServer) Close() error {
	c.invokeMu.Lock()
	defer c.invokeMu.Unlock()
	c.stopLocked(true)
	return nil
}

func (c *CodexAppServer) stderrText() string {
	if c.stderr == nil {
		return ""
	}
	return c.stderr.String()
}

func codexPathDirs(executable string) []string {
	value := filepath.Join(filepath.Dir(filepath.Dir(executable)), "codex-path")
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		return []string{value}
	}
	return nil
}

func codexEnvironment(pathDirs []string) []string {
	result := make([]string, 0, len(os.Environ())+2)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.EqualFold(key, "PATH") || strings.EqualFold(key, "CODEX_INTERNAL_ORIGINATOR_OVERRIDE") {
			continue
		}
		result = append(result, entry)
	}
	pathValue := os.Getenv("PATH")
	if len(pathDirs) > 0 {
		pathValue = strings.Join(pathDirs, string(os.PathListSeparator)) + string(os.PathListSeparator) + pathValue
	}
	return append(result, "PATH="+pathValue, "CODEX_INTERNAL_ORIGINATOR_OVERRIDE=aku_sidecar_go_app_server")
}

func readSchema(path string) (any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read output schema: %w", err)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("decode output schema: %w", err)
	}
	return value, nil
}

func rawID(value json.RawMessage) string { var id string; _ = json.Unmarshal(value, &id); return id }
func rawValue(value json.RawMessage) any {
	var result any
	_ = json.Unmarshal(value, &result)
	return result
}
func ptr(value int64) *int64 { return &value }
func agentMessageText(item map[string]any) string {
	if item["type"] != "agentMessage" {
		return ""
	}
	value, _ := item["text"].(string)
	return value
}
func appServerTelemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, value usage, runErr error) domain.ReasoningTelemetry {
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: "codex-app-server", Model: model.Model, Effort: model.Effort, DurationMS: duration.Milliseconds(), Status: status, InputTokens: value.Input, CachedInputTokens: value.CachedInput, OutputTokens: value.Output, ReasoningOutputTokens: value.ReasoningOutput, CreatedAt: domain.Now()}
}

func resolveExecutable(root, value string) (string, error) {
	requested := strings.TrimSpace(value)
	if requested != "" && strings.ContainsAny(requested, `\\/`) && !filepath.IsAbs(requested) {
		requested = filepath.Join(root, requested)
	}
	result, err := codexruntime.Discover(context.Background(), requested)
	if err != nil {
		return "", fmt.Errorf("discover Codex App Server runtime: %w", err)
	}
	return result.Executable, nil
}
