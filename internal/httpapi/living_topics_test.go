package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestLivingTopicsHTTPManualWorkflowAndPrivacy(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	item, err := state.CreateMemoryRecallStub(ctx, libraryHTTPInput("2302"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, item.ID, domain.MemoryFullCopyInput{Content: "private full text", CapturedAt: "2026-08-25T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/living-topics", strings.NewReader(`{"name":"GPT Astra","description":"Track releases and capabilities"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		Topic struct {
			ID string `json:"id"`
		} `json:"topic"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/living-topics/"+created.Topic.ID+"/snapshots", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !strings.Contains(response.Body.String(), `"status":"insufficient_evidence"`) {
		t.Fatalf("empty snapshot status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/living-topics/"+created.Topic.ID+"/members", strings.NewReader(`{"memoryItemId":"`+item.ID+`"}`))
	request.Header.Set("Content-Type", "application/json")
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("member status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "private full text") || strings.Contains(response.Body.String(), "fullContent") || !strings.Contains(response.Body.String(), `"origin":"manual"`) {
		t.Fatalf("topic detail exposed retained content: %s", response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/api/living-topics/"+created.Topic.ID, nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"memberCount":1`) || !strings.Contains(response.Body.String(), `"description":"Track releases and capabilities"`) {
		t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/living-topics/"+created.Topic.ID+"/snapshots", strings.NewReader(`{}`))
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("snapshot body status=%d body=%s", response.Code, response.Body.String())
	}
}
