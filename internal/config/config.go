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
	ConfigPath            string
	CodexPath             string
	DatabasePath          string
	Provider              string
	Port                  int
	Dev                   bool
	DiscoverCodex         bool
	RuntimeControlToken   string
	RuntimeCandidateProbe bool
	BridgeExtensionOrigin string
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
	if c.Reasoning.Provider != "deterministic" && c.Reasoning.Provider != "codex-app-server" {
		return fmt.Errorf("unsupported reasoning provider %q", c.Reasoning.Provider)
	}
	if c.Reasoning.TimeoutMS < int((5*time.Second)/time.Millisecond) {
		return errors.New("reasoning timeout must be at least 5000 ms")
	}
	if c.Reasoning.Provider == "codex-app-server" {
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
