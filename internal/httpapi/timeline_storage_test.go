package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestTimelineStorageHTTPIsReadOnlyAndAggregateOnly(t *testing.T) {
	server, _ := openLibraryHTTPFixture(t)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/timeline/storage", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		TimelineStorage domain.TimelineStorageStatus `json:"timelineStorage"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TimelineStorage.Mode != "observe" || body.TimelineStorage.Policy.MaxItems != 500 || body.TimelineStorage.Policy.MaxLogicalBytes != 10*1024*1024 {
		t.Fatalf("storage=%+v", body.TimelineStorage)
	}
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/timeline/storage", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("mutation status=%d body=%s", response.Code, response.Body.String())
	}
}
