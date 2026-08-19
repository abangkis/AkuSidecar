package config

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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

type DatabaseConfig struct {
	Path string `json:"path"`
}

type ReasoningConfig struct {
	Provider      string      `json:"provider"`
	Executable    string      `json:"executable"`
	Endpoint      string      `json:"endpoint"`
	MaxRetries    int         `json:"maxRetries,omitempty"`
	TimeoutMS     int         `json:"timeoutMs"`
	Planning      ModelConfig `json:"planning"`
	Evaluation    ModelConfig `json:"evaluation"`
	SemanticEvent ModelConfig `json:"semanticEvent"`
	AIDetection   ModelConfig `json:"aiDetection"`
}

type ModelConfig struct {
	Model  string `json:"model"`
	Effort string `json:"effort"`
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
		cfg.Reasoning.Executable = options.CodexPath
	}
	if options.Provider != "" {
		cfg.Reasoning.Provider = options.Provider
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
	if c.Reasoning.Provider != "deterministic" && c.Reasoning.Provider != "codex-app-server" && c.Reasoning.Provider != "ollama" {
		return fmt.Errorf("unsupported reasoning provider %q", c.Reasoning.Provider)
	}
	if c.Reasoning.MaxRetries < 0 || c.Reasoning.MaxRetries > 5 {
		return errors.New("reasoning max retries must be between 0 and 5")
	}
	if c.Reasoning.TimeoutMS < int((5*time.Second)/time.Millisecond) {
		return errors.New("reasoning timeout must be at least 5000 ms")
	}
	if c.Reasoning.Provider == "codex-app-server" || c.Reasoning.Provider == "ollama" {
		models := map[string]ModelConfig{
			"planning":       c.Reasoning.Planning,
			"evaluation":     c.Reasoning.Evaluation,
			"semantic event": c.Reasoning.SemanticEvent,
			"AI detection":   c.Reasoning.AIDetection,
		}
		for name, model := range models {
			if model.Model == "" || model.Effort == "" {
				return fmt.Errorf("%s model and effort are required", name)
			}
		}
	}
	if c.Capture.MaxAcquisitionRounds < 1 || c.Capture.MaxAcquisitionRounds > 2 {
		return errors.New("max acquisition rounds must be one or two")
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
		"development":        {profile: "development", kinds: map[string]bool{"workspace": true}},
		"acceptance":         {profile: "acceptance", kinds: map[string]bool{"installed": true}},
		"production-store":   {profile: "production-store", kinds: map[string]bool{"installed": true}},
		"production-offline": {profile: "production-offline", kinds: map[string]bool{"portable": true, "installed": true}},
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
