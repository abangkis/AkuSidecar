package reasoning

import (
	"context"
	"encoding/json"
	"errors"
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
	// transport keeps the concrete single-session view used by diagnostics and
	// existing tests. adapter is the lifecycle abstraction that also accepts
	// the SDK PoolAdapter when pooling is explicitly enabled.
	transport *codexappserver.Adapter
	adapter   interface {
		inference.Adapter
		Close() error
	}
	clients      *boundClientPool
	poolSize     int
	transportCfg codexappserver.Config

	exeMu  sync.Mutex
	lifeMu sync.RWMutex
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
	transportConfig := codexappserver.Config{
		WorkingDir:    provider.root,
		Timeout:       provider.timeout,
		ClientName:    "AkuSidecar",
		ClientVersion: domain.ApplicationVersion,
		Start:         provider.startSession,
	}
	transport, err := newCodexTransport(transportConfig, cfg.Reasoning.CodexSessionPoolSize)
	if err != nil {
		return nil, err
	}
	clients, err := newBoundClientPool(transport)
	if err != nil {
		_ = transport.Close()
		return nil, err
	}
	provider.adapter = transport
	if single, ok := transport.(*codexappserver.Adapter); ok {
		provider.transport = single
	}
	provider.clients = clients
	provider.poolSize = cfg.Reasoning.CodexSessionPoolSize
	provider.transportCfg = transportConfig
	return provider, nil
}

func newCodexTransport(cfg codexappserver.Config, poolSize int) (interface {
	inference.Adapter
	Close() error
}, error) {
	if poolSize > 0 {
		return codexappserver.NewSessionPool(codexappserver.PoolConfig{Config: cfg, Size: poolSize})
	}
	return codexappserver.New(cfg)
}

func (c *CodexAppServer) Name() string { return "codex-app-server" }

// Preflight starts the App Server and completes only its initialize handshake.
// The SDK deliberately creates no thread or turn, so this validates the local
// runtime and signed-in session without consuming inference tokens.
func (c *CodexAppServer) Preflight(ctx context.Context) error {
	c.lifeMu.RLock()
	defer c.lifeMu.RUnlock()
	preflight, ok := c.adapter.(interface{ Preflight(context.Context) error })
	if !ok {
		return errors.New("Codex App Server adapter does not expose preflight")
	}
	return preflight.Preflight(ctx)
}

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
	c.exeMu.Lock()
	c.executable = executable
	c.pathDirs = codexPathDirs(executable)
	c.exeMu.Unlock()
	// Recompose the transport so an opt-in SDK pool is not left permanently
	// closed after an executable switch. Readers hold the lifecycle lock across
	// invocation, preventing a close/rebind race with an in-flight request.
	c.lifeMu.Lock()
	defer c.lifeMu.Unlock()
	if c.clients != nil {
		_ = c.clients.Close()
	}
	transport, err := newCodexTransport(c.transportCfg, c.poolSize)
	if err != nil {
		return
	}
	clients, err := newBoundClientPool(transport)
	if err != nil {
		_ = transport.Close()
		return
	}
	c.adapter, c.clients = transport, clients
	if single, ok := transport.(*codexappserver.Adapter); ok {
		c.transport = single
	} else {
		c.transport = nil
	}
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
			modelID, ok := codexStableModelID(option.Model)
			if !ok {
				return config.ModelConfig{}, false
			}
			return config.ModelConfig{ModelID: modelID, Model: option.Model, MinReasoningTier: option.Effort, ReasoningOptionID: option.Effort, Effort: option.Effort, ProfileID: id}, true
		}
	}
	return config.ModelConfig{}, false
}

// InvokeStructured exposes the shared App Server transport to bounded adapters
// without adding their domain-specific methods to the reasoning Provider.
func (c *CodexAppServer) InvokeStructured(ctx context.Context, profileID string, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	return c.invoke(ctx, inference.ProfileID(profileID), prompt, schema, model)
}

func (c *CodexAppServer) Plan(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	return c.PlanWithModel(ctx, run, observation, knowledge, string(ExecutionProfilePlanning), c.planning)
}

func (c *CodexAppServer) PlanWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (AcquisitionPlan, domain.ReasoningTelemetry, error) {
	raw, usage, duration, err := c.invoke(ctx, inference.ProfileID(profileID), buildPlanningPrompt(run, observation, knowledge), c.planSchema, model)
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
	return c.AnalyzeWithModel(ctx, run, observation, knowledge, string(ExecutionProfileEvaluation), c.evaluation)
}

func (c *CodexAppServer) AnalyzeWithModel(ctx context.Context, run domain.Run, observation domain.Observation, knowledge []domain.ReasonedItem, profileID string, model config.ModelConfig) (domain.ReasoningResult, domain.ReasoningTelemetry, error) {
	request := buildEvaluationRequest(run, observation, knowledge)
	raw, usage, duration, err := c.invoke(ctx, inference.ProfileID(profileID), request.prompt, c.resultSchema, model)
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

func (c *CodexAppServer) invoke(parent context.Context, profileID inference.ProfileID, prompt string, schema any, model config.ModelConfig) (string, domain.ModelUsage, time.Duration, error) {
	c.lifeMu.RLock()
	defer c.lifeMu.RUnlock()
	modelID, ok := codexStableModelID(model.ModelID)
	if !ok {
		modelID, ok = codexStableModelID(model.Model)
	}
	if !ok {
		return "", domain.ModelUsage{}, 0, fmt.Errorf("unknown Codex App Server model %q; use a provider-owned stable model ID", model.StableModelID())
	}
	return invokeBound(parent, c.clients, profileID, prompt, schema, model, modelID)
}

func codexStableModelID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	switch value {
	case string(codexappserver.ModelLuna), "gpt-5.6-luna":
		return string(codexappserver.ModelLuna), true
	case string(codexappserver.ModelTerra), "gpt-5.6-terra":
		return string(codexappserver.ModelTerra), true
	case string(codexappserver.ModelSol), "gpt-5.6-sol":
		return string(codexappserver.ModelSol), true
	default:
		return "", false
	}
}

// IsUsageLimitError identifies account-level Codex exhaustion. Unlike model
// capacity, these failures will not recover through an immediate retry or a
// different source lane and must be acknowledged by the user before automatic
// work resumes.
func IsUsageLimitError(err error) bool {
	var infErr *inference.Error
	if errors.As(err, &infErr) {
		return infErr.Code == inference.FailureCodeQuota
	}
	return codexappserver.IsUsageLimitError(err)
}

type ProviderFailure struct {
	Code                   string
	Category               string
	Reason                 string
	ProviderResponseStatus string
	PartialOutputSeen      bool
	DiagnosticCodes        []string
	Stage                  string
	Retry                  string
	RetryTransient         bool
	ProviderStatus         int
	RequestID              string
	Operation              string
	RPCCode                int
	ProcessExitCode        int
	Message                string
}

func ProviderFailureFrom(err error) (ProviderFailure, bool) {
	var infErr *inference.Error
	if !errors.As(err, &infErr) {
		return ProviderFailure{}, false
	}
	return ProviderFailure{
		Code:                   string(infErr.Code),
		Category:               string(infErr.Category),
		Reason:                 string(infErr.Reason),
		ProviderResponseStatus: infErr.ProviderResponseStatus,
		PartialOutputSeen:      infErr.PartialOutputSeen,
		DiagnosticCodes:        append([]string(nil), infErr.DiagnosticCodes...),
		Stage:                  string(infErr.Stage),
		Retry:                  string(infErr.Retry),
		RetryTransient:         infErr.Retry == inference.RetryTransient,
		ProviderStatus:         infErr.ProviderStatus,
		RequestID:              infErr.RequestID,
		Operation:              infErr.Operation,
		RPCCode:                infErr.RPCCode,
		ProcessExitCode:        infErr.ProcessExitCode,
		Message:                infErr.Message,
	}, true
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
	c.lifeMu.Lock()
	defer c.lifeMu.Unlock()
	if c.clients != nil {
		err := c.clients.Close()
		c.clients = nil
		c.adapter = nil
		c.transport = nil
		return err
	}
	if c.adapter == nil {
		return nil
	}
	err := c.adapter.Close()
	c.adapter = nil
	c.transport = nil
	return err
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
	// Keep the validated bytes immutable so the hot invocation path can reuse
	// the static schema without marshaling it for every request.
	return json.RawMessage(append([]byte(nil), raw...)), nil
}

func appServerTelemetry(run domain.Run, phase string, model config.ModelConfig, duration time.Duration, value domain.ModelUsage, runErr error) domain.ReasoningTelemetry {
	status := "completed"
	if runErr != nil {
		status = "failed"
	}
	modelName := model.Model
	if modelName == "" {
		modelName = value.ProviderModel
	}
	effort := model.Effort
	if effort == "" {
		effort = value.NativeReasoning
	}
	callerLatency := value.CallerLatencyMS
	if callerLatency == 0 {
		callerLatency = duration.Milliseconds()
	}
	return domain.ReasoningTelemetry{ID: domain.NewID("reasoning"), RunID: run.ID, Phase: phase, Provider: "codex-app-server", Model: modelName, Effort: effort, ModelDescriptorVersion: value.ModelDescriptorVersion, ModelMaturity: value.ModelMaturity, DurationMS: duration.Milliseconds(), CallerLatencyMS: callerLatency, QueueWaitMS: value.QueueWaitMS, ProviderExecutionMS: value.ProviderExecutionMS, ResponseTotalMS: value.ResponseTotalMS, Status: status, InputTokens: value.Input, CachedInputTokens: value.CachedInput, OutputTokens: value.Output, ReasoningOutputTokens: value.ReasoningOutput, CreatedAt: domain.Now()}
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
