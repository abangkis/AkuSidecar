package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Version    int              `json:"version"`
	Server     ServerConfig     `json:"server"`
	Database   DatabaseConfig   `json:"database"`
	Reasoning  ReasoningConfig  `json:"reasoning"`
	Capture    CaptureConfig    `json:"capture"`
	Preference PreferenceConfig `json:"preference"`
	Root       string           `json:"-"`
	Dev        bool             `json:"-"`
}

type ServerConfig struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type DatabaseConfig struct {
	Path string `json:"path"`
}

type ReasoningConfig struct {
	Provider   string      `json:"provider"`
	Executable string      `json:"executable"`
	TimeoutMS  int         `json:"timeoutMs"`
	Planning   ModelConfig `json:"planning"`
	Evaluation ModelConfig `json:"evaluation"`
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

type Options struct {
	ConfigPath   string
	CodexPath    string
	DatabasePath string
	Provider     string
	Port         int
	Dev          bool
}

func ParseFlags() Options {
	var options Options
	flag.StringVar(&options.ConfigPath, "config", "config/sidecar.json", "path to typed AkuSidecar configuration")
	flag.StringVar(&options.CodexPath, "codex-path", "", "override Codex executable for this process")
	flag.StringVar(&options.DatabasePath, "database", "", "override fresh SQLite database path")
	flag.StringVar(&options.Provider, "provider", "", "override reasoning provider for this process")
	flag.IntVar(&options.Port, "port", 0, "override loopback HTTP port for this process")
	flag.BoolVar(&options.Dev, "dev", false, "enable development asset and reload behavior")
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
	if c.Database.Path == "" {
		return errors.New("database path is required")
	}
	if c.Reasoning.Provider != "deterministic" && c.Reasoning.Provider != "codex-app-server" {
		return fmt.Errorf("unsupported reasoning provider %q", c.Reasoning.Provider)
	}
	if c.Reasoning.TimeoutMS < int((5*time.Second)/time.Millisecond) {
		return errors.New("reasoning timeout must be at least 5000 ms")
	}
	if c.Capture.MaxAcquisitionRounds < 1 || c.Capture.MaxAcquisitionRounds > 2 {
		return errors.New("max acquisition rounds must be one or two")
	}
	return nil
}
