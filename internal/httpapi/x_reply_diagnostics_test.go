package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abangkis/AkuSidecar/internal/diagnostics/xreply"
	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestBridgeObservationPersistsBoundedXReplyDiagnostics(t *testing.T) {
	const candidate = `{"snapshotIndex":0,"candidateId":"x:status:12345","dom":"missing","lookup":"miss","merge":"no_relation"}`
	const foreign = `{"snapshotIndex":0,"candidateId":"x:status:78901","dom":"parent_found","lookup":"hit","merge":"retained_dom"}`
	const report = `{"schema":"aku.x.reply-diagnostics.v1","complete":true,"candidates":[`
	for _, tc := range []struct {
		name, input string
		retained    bool
		complete    bool
	}{
		{name: "legacy"},
		{name: "exact owner", input: report + candidate + "]}", retained: true, complete: true},
		{name: "unbound row", input: report + candidate + "," + foreign + "]}", retained: true},
		{name: "unknown field", input: report + strings.TrimSuffix(candidate, "}") + `,"raw":"private"}]}`},
		{name: "unknown outcome", input: report + strings.Replace(candidate, "miss", "unknown", 1) + "]}"},
		{name: "duplicate row", input: report + candidate + "," + candidate + "]}"},
		{name: "over limit", input: report + strings.Repeat(candidate+",", 64) + candidate + "]}"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, token := splitTestServer(t)
			t.Cleanup(func() {
				server.engine.Shutdown()
				if !server.engine.WaitForIdle(3 * time.Second) {
					t.Error("fixture workers did not stop")
				}
			})
			ctx := context.Background()
			settings, err := server.store.GetSettings(ctx)
			if err != nil {
				t.Fatal(err)
			}
			settings.ActiveSources = []domain.Source{domain.SourceX}
			session, err := server.store.CreateUpdateSession(ctx, "X reply diagnostics fixture", settings, domain.UpdatePolicy{
				Trigger: domain.UpdateTriggerUser, Delivery: domain.UpdateDeliveryVisible, BudgetAuthority: domain.BudgetAuthorityUser,
			})
			if err != nil {
				t.Fatal(err)
			}
			run := session.Runs[0]
			command, err := server.store.StartRun(ctx, run.ID, map[string]any{"source": "x"})
			if err != nil {
				t.Fatal(err)
			}
			if claimed, err := server.store.ClaimCommand(ctx, run.ID, "fixture"); err != nil || claimed == nil {
				t.Fatalf("claim: %v %v", claimed, err)
			}
			coverage := map[string]any{"status": "partial"}
			if tc.input != "" {
				coverage[xreply.CoverageKey] = json.RawMessage(tc.input)
			}
			observation := domain.Observation{Source: domain.SourceX, PageURL: "https://x.com/home", CapturedAt: domain.Now(),
				Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{PlatformID: "x:status:12345",
					Permalink: "https://x.com/fixture/status/12345", Text: "An observed timeline post."}}}}, Coverage: coverage}
			payload, err := json.Marshal(map[string]any{"runId": run.ID, "observation": observation})
			if err != nil {
				t.Fatal(err)
			}
			target := "/api/bridge/commands/" + command.ID + "/observation"
			bridgeCommandsJSON(t, bridgeCommandsServe(server, bridgeCommandsRequest(server, token, http.MethodPost, target, string(payload))), http.StatusAccepted)
			retained, err := server.store.Observations(ctx, run.ID)
			if err != nil || len(retained) != 1 {
				t.Fatalf("observation was not retained: %d, %v", len(retained), err)
			}
			value, exists := retained[0].Coverage[xreply.CoverageKey]
			if exists != tc.retained {
				t.Fatalf("unexpected diagnostic retention: %v", value)
			}
			if tc.retained {
				got := value.(map[string]any)
				rows := got["candidates"].([]any)
				if got["complete"] != tc.complete || len(rows) != 1 || rows[0].(map[string]any)["candidateId"] != "x:status:12345" {
					t.Fatalf("unexpected diagnostic report: %v", value)
				}
			}
			if len(retained[0].Snapshots[0].Blocks[0].DirectContext) != 0 {
				t.Fatal("diagnostics must not create a reply edge")
			}
			if !server.engine.WaitForIdle(3 * time.Second) {
				t.Fatal("run did not settle")
			}
			gotRun, err := server.store.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			public, err := json.Marshal(gotRun.Coverage)
			if err != nil || strings.Contains(string(public), xreply.CoverageKey) {
				t.Fatalf("diagnostic escaped observation storage: %v %s", err, public)
			}
		})
	}
}
