package reasoning

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLiveCodexAppServerStructuredPlan(t *testing.T) {
	if os.Getenv("AKU_CODEX_LIVE") != "1" {
		t.Skip("set AKU_CODEX_LIVE=1 for an authenticated native App Server smoke test")
	}
	root := filepathRoot(t)
	cfg, err := config.Load(config.Options{ConfigPath: filepath.Join(root, "config", "sidecar.json"), Provider: "codex-app-server"})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewCodexAppServer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer provider.Close()
	run := domain.Run{ID: "live-app-server-smoke", Source: domain.SourceX}
	observation := domain.Observation{Source: domain.SourceX, CapturedAt: domain.Now(), Snapshots: []domain.Snapshot{{CapturedAt: domain.Now(), Blocks: []domain.Block{{EvidenceKey: "x:000000000000000000000001", Author: "AkuBrowser fixture", Text: "A complete bounded fixture is available.", Permalink: "https://x.com/example/status/1"}}}}, Coverage: map[string]any{"fixture": true}}
	plan, telemetry, err := provider.Plan(context.Background(), run, observation, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Decision != "finish" && plan.Decision != "request_follow_up" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if telemetry.Provider != "codex-app-server" || telemetry.Status != "completed" || telemetry.InputTokens == nil || telemetry.OutputTokens == nil {
		t.Fatalf("missing App Server telemetry: %+v", telemetry)
	}
}
