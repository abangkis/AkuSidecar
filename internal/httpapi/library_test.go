package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/config"
	"github.com/abangkis/AkuSidecar/internal/domain"
	"github.com/abangkis/AkuSidecar/internal/engine"
	"github.com/abangkis/AkuSidecar/internal/reasoning"
	"github.com/abangkis/AkuSidecar/internal/store"
)

func openLibraryHTTPFixture(t *testing.T) (*Server, *store.Store) {
	t.Helper()
	settings := domain.DefaultSettings("expanded", "quiet", "promote_unused_budget", true)
	state, err := store.Open(filepath.Join(t.TempDir(), "library-http.db"), settings)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { state.Close() })
	cfg := config.Config{Root: t.TempDir(), Server: config.ServerConfig{Host: "127.0.0.1", Port: 0}}
	runtime := engine.New(state, reasoning.Deterministic{}, cfg, log.New(io.Discard, "", 0))
	server, err := New(cfg, state, runtime, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatal(err)
	}
	return server, state
}

func libraryHTTPInput(id string) domain.MemoryItemInput {
	publishedAt := "2026-08-07T00:00:00Z"
	return domain.MemoryItemInput{
		Identity: domain.MemoryIdentity{
			Source:               domain.SourceX,
			CanonicalEvidenceKey: "x:http-library:" + id,
			CanonicalPermalink:   "https://x.com/reader/status/" + id,
			CanonicalPlatformID:  id,
			ContentFingerprint:   "http-library-fingerprint-" + id,
		},
		Title: "HTTP Library marker", Summary: "Read-only detail summary", Author: "Library Author",
		PublishedAt: &publishedAt, Tags: []string{"library"}, Facets: []string{"read_only"},
	}
}

func TestLibraryReadOnlyHTTPListDetailAndPrivacy(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	item, err := state.CreateMemoryRecallStub(ctx, libraryHTTPInput("2301"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, item.ID, domain.MemoryFullCopyInput{Content: "private detail text", CapturedAt: "2026-08-07T01:00:00Z"}); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/library/items?query=HTTP+Library&limit=10", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", response.Code, response.Body.String())
	}
	var listed struct {
		Items      []map[string]json.RawMessage `json:"items"`
		NextCursor string                       `json:"nextCursor"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 1 {
		t.Fatalf("listed items=%v", listed.Items)
	}
	for _, forbidden := range []string{"identityDigest", "contentFingerprint", "lifecycleState", "fullContentVersionId", "contentBytes", "reason", "fullContent"} {
		if _, exists := listed.Items[0][forbidden]; exists {
			t.Fatalf("list exposed internal/private field %q: %s", forbidden, response.Body.String())
		}
	}

	request = httptest.NewRequest(http.MethodGet, "/api/library/items/"+item.ID, nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", response.Code, response.Body.String())
	}
	var detail struct {
		Item map[string]json.RawMessage `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	var content string
	if err := json.Unmarshal(detail.Item["fullContent"], &content); err != nil || content != "private detail text" {
		t.Fatalf("detail full content=%q err=%v", content, err)
	}
	for _, forbidden := range []string{"identityDigest", "contentFingerprint", "lifecycleState", "fullContentVersionId", "contentBytes", "reason"} {
		if _, exists := detail.Item[forbidden]; exists {
			t.Fatalf("detail exposed internal field %q: %s", forbidden, response.Body.String())
		}
	}

	if _, err := state.DeleteMemory(ctx, item.ID); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/library/items/"+item.ID, nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("tombstone detail status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLibraryHTTPBoundsAndReadOnlyBoundary(t *testing.T) {
	server, _ := openLibraryHTTPFixture(t)
	for _, test := range []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "zero limit", method: http.MethodGet, path: "/api/library/items?limit=0", want: http.StatusBadRequest},
		{name: "large limit", method: http.MethodGet, path: "/api/library/items?limit=51", want: http.StatusBadRequest},
		{name: "non numeric limit", method: http.MethodGet, path: "/api/library/items?limit=nope", want: http.StatusBadRequest},
		{name: "oversized query", method: http.MethodGet, path: "/api/library/items?query=" + strings.Repeat("x", 201), want: http.StatusBadRequest},
		{name: "unsupported source", method: http.MethodGet, path: "/api/library/items?source=unknown", want: http.StatusBadRequest},
		{name: "unsupported tier", method: http.MethodGet, path: "/api/library/items?tier=archive", want: http.StatusBadRequest},
		{name: "invalid published from", method: http.MethodGet, path: "/api/library/items?publishedFrom=not-a-date", want: http.StatusBadRequest},
		{name: "invalid published to", method: http.MethodGet, path: "/api/library/items?publishedTo=2026-08-07T00:00:00", want: http.StatusBadRequest},
		{name: "invalid cursor", method: http.MethodGet, path: "/api/library/items?cursor=not-a-cursor", want: http.StatusBadRequest},
		{name: "mutation method", method: http.MethodPost, path: "/api/library/items", want: http.StatusNotFound},
		{name: "put detail", method: http.MethodPut, path: "/api/library/items/example", want: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			response := httptest.NewRecorder()
			server.api().ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
		})
	}
}
