package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSavedRetentionHTTPReadLaterKeepAndDone(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	fixture := createTimelineMemoryHTTPFixture(t, state, "completed", "saved HTTP body", "2501")

	request := httptest.NewRequest(http.MethodPost, "/api/timeline/"+fixture.TimelineID+"/read-later", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Read later status=%d body=%s", response.Code, response.Body.String())
	}
	var saved struct {
		Saved         bool              `json:"saved"`
		AlreadySaved  bool              `json:"alreadySaved"`
		RetentionTier domain.MemoryTier `json:"retentionTier"`
	}
	if err := json.NewDecoder(response.Body).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	if !saved.Saved || saved.AlreadySaved || saved.RetentionTier != domain.MemoryTierFullCopy {
		t.Fatalf("Read later response=%+v", saved)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/library/saved?limit=10", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Saved list status=%d body=%s", response.Code, response.Body.String())
	}
	var listed struct {
		Items []struct {
			ID            string `json:"id"`
			Saved         bool   `json:"saved"`
			PermanentKeep bool   `json:"permanentKeep"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 || !listed.Items[0].Saved || listed.Items[0].PermanentKeep {
		t.Fatalf("Saved list=%+v", listed)
	}
	memoryID := listed.Items[0].ID

	request = httptest.NewRequest(http.MethodPost, "/api/timeline/"+fixture.TimelineID+"/read-later", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("idempotent Read later status=%d body=%s", response.Code, response.Body.String())
	}
	var idempotent struct {
		Saved        bool `json:"saved"`
		AlreadySaved bool `json:"alreadySaved"`
	}
	if err := json.NewDecoder(response.Body).Decode(&idempotent); err != nil {
		t.Fatal(err)
	}
	if !idempotent.Saved || !idempotent.AlreadySaved {
		t.Fatalf("idempotent Read later response=%+v", idempotent)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/library/items/"+memoryID+"/keep-in-library", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Keep in Library status=%d body=%s", response.Code, response.Body.String())
	}
	var kept struct {
		Kept          bool              `json:"kept"`
		Saved         bool              `json:"saved"`
		PermanentKeep bool              `json:"permanentKeep"`
		RetentionTier domain.MemoryTier `json:"retentionTier"`
	}
	if err := json.NewDecoder(response.Body).Decode(&kept); err != nil {
		t.Fatal(err)
	}
	if !kept.Kept || kept.Saved || !kept.PermanentKeep || kept.RetentionTier != domain.MemoryTierFullCopy {
		t.Fatalf("Keep in Library response=%+v", kept)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/timeline/"+fixture.TimelineID+"/read-later", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("repeat Read later status=%d body=%s", response.Code, response.Body.String())
	}
	var repeat struct {
		Saved        bool `json:"saved"`
		AlreadySaved bool `json:"alreadySaved"`
	}
	if err := json.NewDecoder(response.Body).Decode(&repeat); err != nil {
		t.Fatal(err)
	}
	if !repeat.Saved || repeat.AlreadySaved {
		t.Fatalf("repeat Read later response=%+v", repeat)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/library/items/"+memoryID+"/done", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("Done status=%d body=%s", response.Code, response.Body.String())
	}
	var done struct {
		Done          bool `json:"done"`
		Saved         bool `json:"saved"`
		PermanentKeep bool `json:"permanentKeep"`
	}
	if err := json.NewDecoder(response.Body).Decode(&done); err != nil {
		t.Fatal(err)
	}
	if !done.Done || done.Saved || !done.PermanentKeep {
		t.Fatalf("Done response=%+v", done)
	}
	if result, err := state.ListSavedMemory(context.Background(), domain.MemoryLibraryQuery{Limit: 10}); err != nil || len(result.Items) != 0 {
		t.Fatalf("Saved after Done=%+v err=%v", result, err)
	}
}
