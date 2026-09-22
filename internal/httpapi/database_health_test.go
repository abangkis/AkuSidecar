package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/store"
)

func TestDatabaseHealthAndCleanupHTTPContract(t *testing.T) {
	server, _ := openLibraryHTTPFixture(t)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/database/health", nil))
	if response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	var body struct {
		Health store.DatabaseHealth `json:"databaseHealth"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Health.Status != "healthy" || !body.Health.ForeignKeysEnabled || body.Health.QuickCheck != "ok" {
		t.Fatalf("health=%+v", body.Health)
	}
	response = httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/database/cleanup", strings.NewReader(`{"confirmed":false,"fingerprint":"`+body.Health.Fingerprint+`","backup":false}`))
	request.Header.Set("Content-Type", "application/json")
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "explicit cleanup confirmation") {
		t.Fatalf("unconfirmed cleanup status=%d body=%s", response.Code, response.Body.String())
	}
}
