package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abangkis/AkuSidecar/internal/credentials"
)

type Config struct {
	Version             int              `json:"version"`
	Deployment          DeploymentConfig `json:"deployment,omitempty"`
	Server              ServerConfig     `json:"server"`
	Database            DatabaseConfig   `json:"database"`
	Reasoning           ReasoningConfig  `json:"reasoning"`
	Capture             CaptureConfig    `json:"capture"`
	Preference          PreferenceConfig `json:"preference"`
	Bridge              BridgeConfig     `json:"bridge"`
	Root                string           `json:"-"`
	Dev                 bool             `json:"-"`
	RuntimeControlToken string           `json:"-"`
}

type DeploymentConfig struct {
	Mode                  string `json:"mode"`
	RuntimeInstallKind    string `json:"runtimeInstallKind"`
	BridgeIdentityProfile string `json:"bridgeIdentityProfile"`
	ReleaseVersion        string `json:"releaseVersion,omitempty"`
	SourceFreeze          string `json:"sourceFreeze,omitempty"`
	ArtifactID            string `json:"artifactId,omitempty"`
}

func (d DeploymentConfig) PublicStatus() map[string]string {
	mode := strings.TrimSpace(d.Mode)
	if mode == "" {
		mode = "unknown"
	}
	return map[string]string{
		"mode":                  mode,
		"runtimeInstallKind":    strings.TrimSpace(d.RuntimeInstallKind),
		"bridgeIdentityProfile": strings.TrimSpace(d.BridgeIdentityProfile),
		"releaseVersion":        strings.TrimSpace(d.ReleaseVersion),
		"sourceFreeze":          strings.TrimSpace(d.SourceFreeze),
		"artifactId":            strings.TrimSpace(d.ArtifactID),
	}
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

func (s ServerConfig) HostPort() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}

type DatabaseConfig struct {
	Path string `json:"path"`
}

type ReasoningConfig struct {
	ActiveProvider   string                    `json:"activeProvider"`
	ProviderOverride bool                      `json:"-"`
	Providers        map[string]ProviderConfig `json:"providers"`

	Provider         string `json:"-"`
	Executable       string `json:"-"`
	Endpoint         string `json:"-"`
	CredentialRef    string `json:"-"`
	MaxRetries       int    `json:"-"`
	TimeoutMS        int    `json:"-"`
	WarmupTimeoutMS  int    `json:"-"`
	KeepAliveMinutes int    `json:"-"`
	NumCtx           int    `json:"-"`
	// CodexSessionPoolSize opts into independent App Server sessions. Zero
	// keeps the historical single-session adapter; positive values are passed
	// explicitly to the SDK pool and never use its own default.
	CodexSessionPoolSize int `json:"-"`
	// OllamaMaxConcurrentInvocations is provider-side invocation capacity. Zero
	// is intentionally normalized by the SDK to one.
	OllamaMaxConcurrentInvocations int         `json:"-"`
	Planning                       ModelConfig `json:"-"`
	Evaluation                     ModelConfig `json:"-"`
	SemanticEvent                  ModelConfig `json:"-"`
	AIDetection                    ModelConfig `json:"-"`
}

type ProviderConfig struct {
	Executable string `json:"executable"`
	Endpoint   string `json:"endpoint"`
	// HideFromSettings keeps an assessment-only composition available to
	// explicit development overrides without advertising it as user-ready.
	HideFromSettings bool `json:"hideFromSettings,omitempty"`
	// CredentialRef names an entry in AkuSidecar's centralized local credential
	// store. Secret values are never valid provider configuration fields.
	CredentialRef    string `json:"credentialRef,omitempty"`
	MaxRetries       int    `json:"maxRetries,omitempty"`
	TimeoutMS        int    `json:"timeoutMs"`
	WarmupTimeoutMS  int    `json:"warmupTimeoutMs,omitempty"`
	KeepAliveMinutes int    `json:"keepAliveMinutes,omitempty"`
	NumCtx           int    `json:"numCtx,omitempty"`
	// Codex session pooling is opt-in. Leave this unset for one session.
	CodexSessionPoolSize int `json:"codexSessionPoolSize,omitempty"`
	// Ollama concurrency is opt-in; zero retains the effective limit of one.
	MaxConcurrentInvocations int         `json:"maxConcurrentInvocations,omitempty"`
	Planning                 ModelConfig `json:"planning"`
	Evaluation               ModelConfig `json:"evaluation"`
	SemanticEvent            ModelConfig `json:"semanticEvent"`
	AIDetection              ModelConfig `json:"aiDetection"`
}

type ModelConfig struct {
	// ModelID is the provider-owned stable identity used for binding. Model is
	// retained as an explicit wire-name migration alias and telemetry field.
	ModelID string `json:"modelId,omitempty"`
	Model   string `json:"model,omitempty"`
	// MinReasoningTier is the client-owned need. ReasoningOptionID is an
	// optional exact provider option; Effort is the legacy alias for the former.
	MinReasoningTier  string `json:"minReasoningTier,omitempty"`
	ReasoningOptionID string `json:"reasoningOptionId,omitempty"`
	Effort            string `json:"effort,omitempty"`
	// Assurance optionally tightens structured-output resolution for this role.
	Assurance string `json:"assurance,omitempty"`
	// MaxOutputTokens optionally applies a provider/workload-specific output
	// budget. Zero retains AkuSidecar's established default.
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
	// ProfileID is internal invocation identity and is never persisted.
	ProfileID string `json:"-"`
}

func (m ModelConfig) StableModelID() string {
	if strings.TrimSpace(m.ModelID) != "" {
		return strings.TrimSpace(m.ModelID)
	}
	return strings.TrimSpace(m.Model)
}

func (m ModelConfig) MinimumTier() string {
	value := strings.TrimSpace(m.MinReasoningTier)
	if value == "" {
		value = strings.TrimSpace(m.Effort)
	}
	if strings.EqualFold(value, "off") {
		return "none"
	}
	return value
}

func (m ModelConfig) ExactReasoningOption() string {
	return strings.TrimSpace(m.ReasoningOptionID)
}

// DisplayModel and DisplayEffort are compatibility/UI projections only. They
// do not participate in provider capability resolution or binding.
func (m ModelConfig) DisplayModel() string { return m.StableModelID() }

func (m ModelConfig) DisplayEffort() string {
	if value := strings.TrimSpace(m.Effort); value != "" {
		return value
	}
	return strings.TrimSpace(m.MinReasoningTier)
}

// UnmarshalJSON accepts either the multi-provider shape (activeProvider with a
// providers map) or the legacy single-provider shape (provider with flat
// transport and model fields), so existing sidecar.json files keep loading.
func (r *ReasoningConfig) UnmarshalJSON(data []byte) error {
	var probe struct {
		ActiveProvider string          `json:"activeProvider"`
		Providers      json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if probe.ActiveProvider != "" || probe.Providers != nil {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var modern struct {
			ActiveProvider string                    `json:"activeProvider"`
			Providers      map[string]ProviderConfig `json:"providers"`
		}
		if err := decoder.Decode(&modern); err != nil {
			return err
		}
		r.ActiveProvider = modern.ActiveProvider
		r.Providers = modern.Providers
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var legacy struct {
		Provider      string      `json:"provider"`
		Executable    string      `json:"executable"`
		Endpoint      string      `json:"endpoint"`
		MaxRetries    int         `json:"maxRetries"`
		TimeoutMS     int         `json:"timeoutMs"`
		Planning      ModelConfig `json:"planning"`
		Evaluation    ModelConfig `json:"evaluation"`
		SemanticEvent ModelConfig `json:"semanticEvent"`
		AIDetection   ModelConfig `json:"aiDetection"`
	}
	if err := decoder.Decode(&legacy); err != nil {
		return err
	}
	r.ActiveProvider = legacy.Provider
	r.Providers = map[string]ProviderConfig{
		legacy.Provider: {
			Executable:    legacy.Executable,
			Endpoint:      legacy.Endpoint,
			MaxRetries:    legacy.MaxRetries,
			TimeoutMS:     legacy.TimeoutMS,
			Planning:      legacy.Planning,
			Evaluation:    legacy.Evaluation,
			SemanticEvent: legacy.SemanticEvent,
			AIDetection:   legacy.AIDetection,
		},
	}
	return nil
}

// Select resolves providerName as the active provider, projecting its declared
// transport and model configuration onto the flat fields consumed by the
// reasoning factory, resolvers, and engine.
func (r *ReasoningConfig) Select(providerName string) error {
	entry, ok := r.Providers[providerName]
	if !ok {
		return fmt.Errorf("reasoning provider %q is not declared", providerName)
	}
	r.Provider = providerName
	r.Executable = entry.Executable
	r.Endpoint = entry.Endpoint
	r.CredentialRef = entry.CredentialRef
	r.MaxRetries = entry.MaxRetries
	r.TimeoutMS = entry.TimeoutMS
	r.WarmupTimeoutMS = entry.WarmupTimeoutMS
	r.KeepAliveMinutes = entry.KeepAliveMinutes
	r.NumCtx = entry.NumCtx
	r.CodexSessionPoolSize = entry.CodexSessionPoolSize
	r.OllamaMaxConcurrentInvocations = entry.MaxConcurrentInvocations
	r.Planning = entry.Planning
	r.Evaluation = entry.Evaluation
	r.SemanticEvent = entry.SemanticEvent
	r.AIDetection = entry.AIDetection
	return nil
}

type ProviderSummary struct {
	Name                string `json:"name"`
	Label               string `json:"label"`
	RuntimeKind         string `json:"runtimeKind"`
	Configured          bool   `json:"configured"`
	ConfigurationStatus string `json:"configurationStatus"`
	CredentialName      string `json:"credentialName,omitempty"`
}

// IsOllamaProvider reports whether the provider name routes to the Ollama
// transport. Model-scoped provider entries are named ollama-<alias> so each
// declared backend can carry its own model catalog.
func IsOllamaProvider(name string) bool {
	return name == "ollama" || strings.HasPrefix(name, "ollama-")
}

func IsGeminiProvider(name string) bool {
	return name == "gemini" || strings.HasPrefix(name, "gemini-")
}

func ProviderLabel(name string) string {
	switch name {
	case "codex-app-server":
		return "Codex App Server"
	case "ollama":
		return "Ollama"
	case "ollama-nemotron":
		return "Ollama · Nemotron 3.5 Lightning"
	case "ollama-qwen":
		return "Ollama · Qwen 3.8 27B"
	case "groq":
		return "Groq · GPT-OSS 120B"
	case "gemini-flash-lite":
		return "Gemini · 3.5 Flash Lite"
	case "gemini-flash":
		return "Gemini · 3.7 Flash"
	case "deterministic":
		return "Local deterministic"
	default:
		if IsOllamaProvider(name) {
			return "Ollama · " + strings.TrimPrefix(name, "ollama-")
		}
		return name
	}
}

func ProviderRuntimeKind(name string) string {
	switch {
	case name == "codex-app-server":
		return "executable"
	case name == "groq" || IsGeminiProvider(name):
		return "remote_api"
	case IsOllamaProvider(name):
		return "local_endpoint"
	default:
		return "embedded"
	}
}

func (r ReasoningConfig) ProviderSummary() []ProviderSummary {
	names := make([]string, 0, len(r.Providers))
	for name := range r.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	summaries := make([]ProviderSummary, 0, len(names))
	for _, name := range names {
		if r.Providers[name].HideFromSettings {
			continue
		}
		summaries = append(summaries, ProviderSummary{
			Name: name, Label: ProviderLabel(name), RuntimeKind: ProviderRuntimeKind(name),
			Configured: true, ConfigurationStatus: "ready",
		})
	}
	return summaries
}

func (r ReasoningConfig) Validate() error {
	if len(r.Providers) == 0 {
		return errors.New("at least one reasoning provider must be declared")
	}
	if _, ok := r.Providers[r.ActiveProvider]; !ok {
		return fmt.Errorf("active reasoning provider %q is not declared", r.ActiveProvider)
	}
	for name, provider := range r.Providers {
		if err := provider.Validate(name); err != nil {
			return err
		}
	}
	return nil
}

func (p ProviderConfig) Validate(name string) error {
	if name != "deterministic" && name != "codex-app-server" && name != "groq" && !IsGeminiProvider(name) && !IsOllamaProvider(name) {
		return fmt.Errorf("unsupported reasoning provider %q", name)
	}
	if p.MaxRetries < 0 || p.MaxRetries > 5 {
		return fmt.Errorf("reasoning provider %q max retries must be between 0 and 5", name)
	}
	if p.TimeoutMS < int((5*time.Second)/time.Millisecond) {
		return fmt.Errorf("reasoning provider %q timeout must be at least 5000 ms", name)
	}
	if p.WarmupTimeoutMS < 0 {
		return fmt.Errorf("reasoning provider %q warmup timeout must not be negative", name)
	}
	if p.KeepAliveMinutes < 0 {
		return fmt.Errorf("reasoning provider %q keep-alive minutes must not be negative", name)
	}
	if p.NumCtx < 0 {
		return fmt.Errorf("reasoning provider %q context length must not be negative", name)
	}
	if p.CodexSessionPoolSize < 0 {
		return fmt.Errorf("reasoning provider %q Codex session pool size must not be negative", name)
	}
	if p.CodexSessionPoolSize > 64 {
		return fmt.Errorf("reasoning provider %q Codex session pool size must be at most 64", name)
	}
	if p.MaxConcurrentInvocations < 0 || p.MaxConcurrentInvocations > 64 {
		return fmt.Errorf("reasoning provider %q maxConcurrentInvocations must be between 0 and 64", name)
	}
	if !IsOllamaProvider(name) && (p.WarmupTimeoutMS != 0 || p.KeepAliveMinutes != 0 || p.NumCtx != 0) {
		return fmt.Errorf("warmup, keep-alive, and context length apply only to ollama providers (%q)", name)
	}
	if name != "codex-app-server" && p.CodexSessionPoolSize != 0 {
		return fmt.Errorf("Codex session pool settings apply only to codex-app-server")
	}
	if !IsOllamaProvider(name) && p.MaxConcurrentInvocations != 0 {
		return fmt.Errorf("Ollama concurrency settings apply only to ollama providers")
	}
	credentialRef := strings.TrimSpace(p.CredentialRef)
	if name == "groq" {
		if err := credentials.ValidateReference("groq", credentialRef); err != nil {
			return err
		}
	} else if IsGeminiProvider(name) {
		if err := credentials.ValidateReference("gemini", credentialRef); err != nil {
			return err
		}
	} else if credentialRef != "" {
		return fmt.Errorf("credentialRef is not supported for reasoning provider %q", name)
	}
	if name == "codex-app-server" || name == "groq" || IsGeminiProvider(name) || IsOllamaProvider(name) {
		models := map[string]ModelConfig{
			"planning":       p.Planning,
			"evaluation":     p.Evaluation,
			"semantic event": p.SemanticEvent,
			"AI detection":   p.AIDetection,
		}
		for task, model := range models {
			if model.StableModelID() == "" || model.MinimumTier() == "" {
				return fmt.Errorf("%s model_id/model and minimum reasoning tier are required for provider %q", task, name)
			}
			if model.MaxOutputTokens < 0 || model.MaxOutputTokens > 131072 {
				return fmt.Errorf("%s maxOutputTokens must be between 1 and 131072 when set for provider %q", task, name)
			}
		}
	}
	return nil
}

type CaptureConfig struct {
	Profile              string `json:"profile"`
	Visibility           string `json:"visibility"`
	OpenMissingSource    bool   `json:"openMissingSource"`
	MaxAcquisitionRounds int    `json:"maxAcquisitionRounds"`
}

type PreferenceConfig struct {
	Mode string `json:"mode"`
}

type BridgeConfig struct {
	TrustedExtensionOrigins []string `json:"trustedExtensionOrigins"`
}

type Options struct {
	ConfigPath                  string
	CodexPath                   string
	DatabasePath                string
	Provider                    string
	Port                        int
	Dev                         bool
	DiscoverCodex               bool
	DiscoverChromium            bool
	AppShell                    bool
	ChromiumPath                string
	BridgeExtensionPath         string
	BrowserProfilePath          string
	AppUserModelID              string
	AppRelaunchCommand          string
	AppRelaunchDisplayName      string
	RuntimeControlToken         string
	RuntimeCandidateProbe       bool
	RuntimeCandidateProbeSchema int
	BridgeExtensionOrigin       string
}

func ParseFlags() Options {
	var options Options
	flag.StringVar(&options.ConfigPath, "config", "config/sidecar.json", "path to typed AkuSidecar configuration")
	flag.StringVar(&options.CodexPath, "codex-path", "", "override Codex executable for this process")
	flag.StringVar(&options.DatabasePath, "database", "", "override fresh SQLite database path")
	flag.StringVar(&options.Provider, "provider", "", "override reasoning provider for this process")
	flag.IntVar(&options.Port, "port", 0, "override loopback HTTP port for this process")
	flag.BoolVar(&options.Dev, "dev", false, "enable development asset and reload behavior")
	flag.BoolVar(&options.DiscoverCodex, "discover-codex", false, "discover and validate a Codex App Server executable, print JSON, and exit")
	flag.BoolVar(&options.DiscoverChromium, "discover-chromium", false, "discover and validate a pinned-Chromium executable, print JSON, and exit")
	flag.BoolVar(&options.AppShell, "app-shell", false, "open the embedded pinned-Chromium application window after startup")
	flag.StringVar(&options.ChromiumPath, "chromium-path", "", "override pinned-Chromium executable for this process")
	flag.StringVar(&options.BridgeExtensionPath, "bridge-extension-path", "", "unpacked AkuBridge extension directory loaded into the app shell")
	flag.StringVar(&options.BrowserProfilePath, "browser-profile", "", "override the app-shell browser profile directory for this process")
	flag.StringVar(&options.AppUserModelID, "app-user-model-id", "", "explicit Windows application identity for app-shell grouping and pinning")
	flag.StringVar(&options.AppRelaunchCommand, "app-relaunch-command", "", "Windows command used to relaunch a pinned app shell")
	flag.StringVar(&options.AppRelaunchDisplayName, "app-relaunch-display-name", "", "Windows display name for a pinned app shell")
	flag.StringVar(&options.RuntimeControlToken, "runtime-control-token", "", "instance-scoped token used by the signed runtime host")
	flag.BoolVar(&options.RuntimeCandidateProbe, "runtime-candidate-probe", false, "validate the packaged runtime contract and exit")
	flag.IntVar(&options.RuntimeCandidateProbeSchema, "runtime-candidate-probe-schema", 1, "candidate probe response schema (1=legacy, 2=current)")
	flag.StringVar(&options.BridgeExtensionOrigin, "bridge-extension-origin", "", "override the exact trusted AkuBridge chrome-extension origin")
	flag.Parse()
	return options
}

func Load(options Options) (Config, error) {
	absConfig, err := filepath.Abs(options.ConfigPath)
	if err != nil {
		return Config{}, fmt.Errorf("resolve config path: %w", err)
	}
	data, err := os.ReadFile(absConfig)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.Version != 1 {
		return Config{}, fmt.Errorf("unsupported config version %d", cfg.Version)
	}
	cfg.Root = filepath.Dir(filepath.Dir(absConfig))
	cfg.Dev = options.Dev
	cfg.RuntimeControlToken = options.RuntimeControlToken
	if options.CodexPath != "" {
		if entry, ok := cfg.Reasoning.Providers["codex-app-server"]; ok {
			entry.Executable = options.CodexPath
			cfg.Reasoning.Providers["codex-app-server"] = entry
		} else if entry, ok := cfg.Reasoning.Providers[cfg.Reasoning.ActiveProvider]; ok {
			entry.Executable = options.CodexPath
			cfg.Reasoning.Providers[cfg.Reasoning.ActiveProvider] = entry
		}
	}
	if options.Provider != "" {
		cfg.Reasoning.ActiveProvider = options.Provider
		cfg.Reasoning.ProviderOverride = true
	}
	if options.Port != 0 {
		cfg.Server.Port = options.Port
	}
	if options.DatabasePath != "" {
		cfg.Database.Path = options.DatabasePath
	}
	if strings.TrimSpace(options.BridgeExtensionOrigin) != "" {
		cfg.Bridge.TrustedExtensionOrigins = []string{strings.TrimSpace(options.BridgeExtensionOrigin)}
	}
	if !filepath.IsAbs(cfg.Database.Path) {
		cfg.Database.Path = filepath.Join(cfg.Root, cfg.Database.Path)
	}
	if err := cfg.Reasoning.Select(cfg.Reasoning.ActiveProvider); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if err := c.Deployment.Validate(); err != nil {
		return err
	}
	if c.Server.Host != "127.0.0.1" && c.Server.Host != "localhost" {
		return errors.New("server host must remain loopback")
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("server port is invalid")
	}
	if c.RuntimeControlToken != "" {
		if len(c.RuntimeControlToken) != 64 {
			return errors.New("runtime control token must contain 32 bytes")
		}
		if _, err := hex.DecodeString(c.RuntimeControlToken); err != nil {
			return errors.New("runtime control token must be lowercase hexadecimal")
		}
	}
	if c.Database.Path == "" {
		return errors.New("database path is required")
	}
	if len(c.Bridge.TrustedExtensionOrigins) == 0 {
		return errors.New("at least one trusted Bridge extension origin is required")
	}
	seenOrigins := map[string]bool{}
	for _, raw := range c.Bridge.TrustedExtensionOrigins {
		parsed, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || parsed.Scheme != "chrome-extension" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
			return fmt.Errorf("invalid trusted Bridge extension origin %q", raw)
		}
		canonical := "chrome-extension://" + strings.ToLower(parsed.Host)
		if seenOrigins[canonical] {
			return fmt.Errorf("duplicate trusted Bridge extension origin %q", raw)
		}
		seenOrigins[canonical] = true
	}
	if err := c.Reasoning.Validate(); err != nil {
		return err
	}
	if c.Capture.MaxAcquisitionRounds < 1 || c.Capture.MaxAcquisitionRounds > 2 {
		return errors.New("max acquisition rounds must be one or two")
	}
	for providerName, provider := range c.Reasoning.Providers {
		for roleName, role := range map[string]ModelConfig{
			"planning":    provider.Planning,
			"evaluation":  provider.Evaluation,
			"semantic":    provider.SemanticEvent,
			"aiDetection": provider.AIDetection,
		} {
			switch role.Assurance {
			case "", "sdk_validated", "provider_strict":
			default:
				return fmt.Errorf("provider %q role %q has unsupported assurance %q", providerName, roleName, role.Assurance)
			}
		}
	}
	return nil
}

func (d DeploymentConfig) Validate() error {
	if d.Mode == "" {
		// Older local configurations remain runnable but surface as Unknown in the
		// UI. Every release packager projects an explicit deployment identity.
		return nil
	}
	allowed := map[string]struct {
		profile string
		kinds   map[string]bool
	}{
		"development":              {profile: "development", kinds: map[string]bool{"workspace": true}},
		"acceptance":               {profile: "acceptance", kinds: map[string]bool{"installed": true}},
		"production-store":         {profile: "production-store", kinds: map[string]bool{"installed": true}},
		"production-offline":       {profile: "production-offline", kinds: map[string]bool{"portable": true, "installed": true}},
		"production-installed-app": {profile: "production-app", kinds: map[string]bool{"installed": true}},
	}
	rule, ok := allowed[d.Mode]
	if !ok {
		return fmt.Errorf("unsupported deployment mode %q", d.Mode)
	}
	if d.BridgeIdentityProfile != rule.profile {
		return fmt.Errorf("deployment mode %q requires Bridge identity profile %q", d.Mode, rule.profile)
	}
	if !rule.kinds[d.RuntimeInstallKind] {
		return fmt.Errorf("deployment mode %q does not support runtime install kind %q", d.Mode, d.RuntimeInstallKind)
	}
	if d.Mode != "development" && strings.TrimSpace(d.ReleaseVersion) == "" {
		return fmt.Errorf("deployment mode %q requires a release version", d.Mode)
	}
	return nil
}
