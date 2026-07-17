package eventengine_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/eventengine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
)

func TestLiveCodexAppServerSemanticAcceptanceCanary(t *testing.T) {
	if os.Getenv("AKU_CODEX_LIVE") != "1" {
		t.Skip("set AKU_CODEX_LIVE=1 for an authenticated semantic App Server canary")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(workingDirectory, "..", ".."))
	cfg, err := config.Load(config.Options{ConfigPath: filepath.Join(root, "config", "sidecar.json"), Provider: "codex-app-server"})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := reasoning.NewCodexAppServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	resolver, err := eventengine.NewAppServerResolver(root, provider, cfg.Reasoning.Evaluation)
	if err != nil {
		t.Fatal(err)
	}
	published := "2026-07-17T00:00:00Z"
	candidates := []domain.SemanticCandidate{
		{Alias: "candidate_001", Source: domain.SourceX, Author: "Moonshot AI", PublishedAt: &published, Text: "Moonshot AI launched the Kimi K3 open-weight model today.", WhatChanged: "Moonshot AI launched the Kimi K3 open-weight model.", EventKey: "moonshot-kimi-k3-launch", TopicTags: []string{"moonshot", "kimi-k3", "model-launch"}},
		{Alias: "candidate_002", Source: domain.SourceLinkedIn, Author: "AI Industry Reporter", PublishedAt: &published, Text: "Moonshot AI launched its Kimi K3 open-weight model today.", WhatChanged: "A second author reported Moonshot AI's Kimi K3 open-weight model launch.", EventKey: "moonshot-kimi-k3-launch", TopicTags: []string{"moonshot", "kimi-k3", "model-launch"}},
		{Alias: "candidate_003", Source: domain.SourceLinkedIn, Author: "Benchmark Lab", PublishedAt: &published, Text: "Benchmark Lab published a new Kimi K3 reasoning benchmark after launch.", WhatChanged: "Benchmark Lab published a new Kimi K3 reasoning benchmark.", EventKey: "kimi-k3-reasoning-benchmark", TopicTags: []string{"moonshot", "kimi-k3", "benchmark"}},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	resolution, usage, duration, err := resolver.Resolve(ctx, candidates, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolution.Decisions) != len(candidates) {
		t.Fatalf("decisions=%+v", resolution.Decisions)
	}
	duplicate := resolution.Decisions[1]
	if duplicate.Relation != "duplicate_report" || duplicate.TargetAlias == nil || *duplicate.TargetAlias != "candidate_001" || duplicate.Confidence < domain.DefaultSemanticMergeThreshold {
		t.Fatalf("true duplicate was not accepted at the default gate: %+v", duplicate)
	}
	if resolution.Decisions[2].Relation == "duplicate_report" {
		t.Fatalf("near-miss benchmark was falsely collapsed: %+v", resolution.Decisions[2])
	}
	if usage.Input == nil || usage.Output == nil || duration <= 0 {
		t.Fatalf("missing live App Server telemetry: usage=%+v duration=%s", usage, duration)
	}
}
