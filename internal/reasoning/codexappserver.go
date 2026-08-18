package reasoning

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/abangkis/AkuSidecar/internal/codexruntime"
	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/ai4u-inference-sdk-go/inference"
	"github.com/abangkis/ai4u-inference-sdk-go/providers/codexappserver"
)

// appServerThreadLimit mirrors the Codex App Server transport's thread budget.
// Structured invocations never reuse an ephemeral thread, so the transport
// recycles its managed process once this many threads have been started.
const appServerThreadLimit = 4

type CodexAppServer struct {
	executable   string
	pathDirs     []string
	root         string
	timeout      time.Duration
	planning     config.ModelConfig
	evaluation   config.ModelConfig
	planSchema   any
	resultSchema any
	transport    *codexappserver.Adapter

	exeMu sync.Mutex
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
	provider := &CodexAppServer{
		executable:   executable,
		pathDirs:     codexPathDirs(executable),
		root:         cfg.Root,
		timeout:      time.Duration(cfg.Reasoning.TimeoutMS) * time.Millisecond,
		planning:     cfg.Reasoning.Planning,
		evaluation:   cfg.Reasoning.Evaluation,
		planSchema:   planSchema,
		resultSchema: resultSchema,
	}
	transport, err := codexappserver.New(codexappserver.Config{
		WorkingDir:    provider.root,
		Timeout:       provider.timeout,
		ClientName:    "AkuSidecar",
		ClientVersion: domain.ApplicationVersion,
		Start:         provider.startSession,
	})
	if err != nil {
		return nil, err
	}
	provider.transport = transport
	return provider, nil
}

func (c *CodexAppServer) Name() string { return "codex-app-server" }

func (c *CodexAppServer) ExecutablePath() string {
	c.exeMu.Lock()
	defer c.exeMu.Unlock()
	return c.executable
}

func (c *CodexAppServer) DiscoverExecutable(ctx context.Context, requested string) (string, error) {
	result, err := codexruntime.Discover(ctx, requested)
	if err != nil {
		return "", err
	}
	return result.Executable, nil
}

func (c *CodexAppServer) UseExecutable(executable string) {
	c.exeMu.Lock()
	if filepath.Clean(executable) == filepath.Clean(c.executable) {
		c.exeMu.Unlock()
		return
	}
	c.exeMu.Unlock()
	if c.transport != nil {
		_ = c.transport.Close()
	}
	c.exeMu.Lock()
	c.executable = executable
	c.pathDirs = codexPathDirs(executable)
	c.exeMu.Unlock()
}

func (c *CodexAppServer) ProfileOptions() []ProfileOption {
	return []ProfileOption{
		{ID: "luna_high", Label: "Luna High", Model: "gpt-5.6-luna", Effort: "high"},
		{ID: "luna_xhigh", Label: "Luna XHigh", Model: "gpt-5.6-luna", Effort: "xhigh"},
		{ID: "luna_max", Label: "Luna Max", Model: "gpt-5.6-luna", Effort: "max"},
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

func (c *CodexAppServer) invoke(parent context.Context, prompt string, schema any, model config.ModelConfig) (string, codexappserver.Usage, time.Duration, error) {
	return c.transport.InvokeStructured(parent, prompt, schema, codexappserver.ModelConfig{
		Model:  model.Model,
		Effort: inference.ReasoningEffort(model.Effort),
	})
}

// IsUsageLimitError identifies account-level Codex exhaustion. Unlike model
// capacity, these failures will not recover through an immediate retry or a
// different source lane and must be acknowledged by the user before automatic
// work resumes.
func IsUsageLimitError(err error) bool {
	return codexappserver.IsUsageLimitError(err)
}

// startSession launches a fresh managed App Server process and returns its
// stdio session. The transport adapter owns the RPC protocol and lifecycle;
// AkuSidecar retains process discovery, configuration, environment, and
// descendant cleanup so the wire behavior is unchanged.
func (c *CodexAppServer) startSession() (codexappserver.Session, error) {
	c.exeMu.Lock()
	executable := c.executable
	pathDirs := c.pathDirs
	c.exeMu.Unlock()
	cmd := exec.Command(executable, "app-server", "--listen", "stdio://")
	cmd.Dir = c.root
	cmd.Env = codexEnvironment(pathDirs)
	configureProcess(cmd)
	ownership, err := newProcessOwnership()
	if err != nil {
		return codexappserver.Session{}, err
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		ownership.close()
		return codexappserver.Session{}, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		ownership.close()
		return codexappserver.Session{}, err
	}
	stderr := &boundedBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		ownership.close()
		return codexappserver.Session{}, fmt.Errorf("start Codex App Server: %w", err)
	}
	if err := ownership.attach(cmd); err != nil {
		_ = cmd.Process.Kill()
		ownership.terminate()
		ownership.close()
		return codexappserver.Session{}, err
	}
	var stopOnce sync.Once
	return codexappserver.Session{
		Stdin:  stdin,
		Stdout: stdout,
		Wait:   cmd.Wait,
		Stderr: stderr.String,
		Stop: func() {
			stopOnce.Do(func() {
				if cmd.Process != nil {
					_ = cmd.Process.Kill()
				}
				ownership.terminate()
				ownership.close()
			})
		},
	}, nil
}

func (c *CodexAppServer) Close() error {
	if c.transport == nil {
		return nil
	}
	return c.transport.Close()
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

func appServerTelemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, value codexappserver.Usage, runErr error) domain.ReasoningTelemetry {
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