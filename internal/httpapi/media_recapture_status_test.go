package httpapi

import (
	"database/sql"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestMediaRecaptureStatusProjectsDurableStatesWithoutPrivateEvidence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status.db")
	state, err := store.Open(path, domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true))
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := domain.Now()
	if _, err = db.Exec(`INSERT INTO timeline_items(id,session_id,run_id,source,evidence_key,rank,item_json,assessment_json,created_at) VALUES('item','session','run','facebook','key',0,'{}','{}',?)`, now); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	defer runtime.Shutdown()
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{"queued", "claimed", "completed", "failed"} {
		t.Run(status, func(t *testing.T) {
			if _, err := db.Exec(`DELETE FROM media_recaptures`); err != nil {
				t.Fatal(err)
			}
			failure := `{"code":"fixture_failure","message":"Capture failed","stage":"capture","retryable":false}`
			if _, err := db.Exec(`INSERT INTO media_recaptures(id,timeline_id,source,target_url,evidence_key,status,outcome,payload_json,result_json,error_json,created_at) VALUES('job','item','facebook','https://www.facebook.com/photo?fbid=123','key',?,'recovered','{"privatePayload":"secret"}','{"privateEvidence":"secret"}',?,?)`, status, failure, now); err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/media-recaptures/job", nil)
			response := httptest.NewRecorder()
			server.http.Handler.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("HTTP %d: %s", response.Code, response.Body.String())
			}
			var body struct {
				Recapture map[string]json.RawMessage `json:"recapture"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if len(body.Recapture) != 4 {
				t.Fatalf("unexpected projection fields: %v", body.Recapture)
			}
			var got string
			if err := json.Unmarshal(body.Recapture["status"], &got); err != nil || got != status {
				t.Fatalf("status=%q err=%v", got, err)
			}
			for _, field := range []string{"id", "outcome", "error"} {
				if _, ok := body.Recapture[field]; !ok {
					t.Fatalf("missing %s", field)
				}
			}
		})
	}
}
