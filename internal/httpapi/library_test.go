package httpapi

import (
	"context"
	"encoding/json"
	"errors"
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

func TestLibrarySearchCanIncludeCurrentLivingTopicKnowledge(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	topic, err := state.CreateLivingTopic(ctx, "OpenAI GPT Astra")
	if err != nil {
		t.Fatal(err)
	}
	evidenceInput := libraryHTTPInput("2304")
	evidenceInput.Title = "GPT Astra orchestration evidence"
	evidenceInput.Summary = "Academic agent coordination details"
	evidence, err := state.CreateMemoryRecallStub(ctx, evidenceInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.AddLivingTopicMember(ctx, topic.ID, evidence.ID); err != nil {
		t.Fatal(err)
	}
	digest := "http-library-topic-knowledge"
	snapshot, err := state.SaveLivingTopicSnapshot(ctx, domain.LivingTopicSnapshot{
		TopicID: topic.ID, Status: "ready", InputDigest: digest,
		Overview: "GPT Astra has source-backed orchestration capabilities.",
		Claims: []domain.LivingTopicClaim{
			{Text: "Academic agent coordination is supported.", Assessment: "supported", EvidenceIDs: []string{evidence.ID}},
			{Text: "A release date remains uncertain.", Assessment: "uncertain", EvidenceIDs: []string{evidence.ID}},
		},
		EvidenceIDs: []string{evidence.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.QueueLivingTopicUnderstanding(ctx, topic.ID, "http_test"); err != nil {
		t.Fatal(err)
	}
	job, err := state.ClaimLivingTopicUnderstanding(ctx)
	if err != nil || job == nil {
		t.Fatalf("understanding job=%+v err=%v", job, err)
	}
	if err := state.FinishLivingTopicUnderstanding(ctx, *job, "published", digest, snapshot.ID, nil); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/library/items?query=agent+coordination&limit=10&includeTopicKnowledge=true", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload struct {
		TopicKnowledge []libraryTopicKnowledgeView `json:"topicKnowledge"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.TopicKnowledge) != 1 {
		t.Fatalf("topic knowledge=%+v", payload.TopicKnowledge)
	}
	knowledge := payload.TopicKnowledge[0]
	if knowledge.TopicID != topic.ID || knowledge.TopicName != topic.Name || knowledge.SnapshotVersion != snapshot.Version || knowledge.EvidenceCount != 1 || knowledge.MatchReason == "" {
		t.Fatalf("knowledge=%+v", knowledge)
	}
	if len(knowledge.Claims) != 1 || knowledge.Claims[0].Assessment != "supported" {
		t.Fatalf("supported claims=%+v", knowledge.Claims)
	}
	knowledgeJSON, err := json.Marshal(knowledge)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(knowledgeJSON), evidence.ID) || strings.Contains(string(knowledgeJSON), "release date remains uncertain") {
		t.Fatalf("Library topic knowledge exposed evidence ids or uncertain claims: %s", knowledgeJSON)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/library/items?query=agent+coordination&limit=10", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "topicKnowledge") {
		t.Fatalf("topic knowledge must remain opt-in to the Library surface: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestLibraryStorageHTTPIsBoundedReadOnlyAndPrivacySafe(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()

	request := httptest.NewRequest(http.MethodGet, "/api/library/storage", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("empty storage status=%d body=%s", response.Code, response.Body.String())
	}
	var empty struct {
		Usage           domain.MemoryStorageUsage    `json:"usage"`
		Recommendations []map[string]json.RawMessage `json:"recommendations"`
	}
	if err := json.NewDecoder(response.Body).Decode(&empty); err != nil {
		t.Fatal(err)
	}
	if empty.Usage.ActiveItems != 0 || len(empty.Recommendations) != 0 || empty.Recommendations == nil {
		t.Fatalf("empty storage=%+v", empty)
	}

	larger, err := state.CreateMemoryRecallStub(ctx, libraryHTTPInput("2401"))
	if err != nil {
		t.Fatal(err)
	}
	smaller, err := state.CreateMemoryRecallStub(ctx, libraryHTTPInput("2402"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, larger.ID, domain.MemoryFullCopyInput{Content: strings.Repeat("l", 48)}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, smaller.ID, domain.MemoryFullCopyInput{Content: strings.Repeat("s", 12)}); err != nil {
		t.Fatal(err)
	}
	tombstone, err := state.CreateMemoryRecallStub(ctx, libraryHTTPInput("2403"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.KeepMemoryFullCopy(ctx, tombstone.ID, domain.MemoryFullCopyInput{Content: strings.Repeat("x", 96)}); err != nil {
		t.Fatal(err)
	}
	if _, err := state.DeleteMemory(ctx, tombstone.ID); err != nil {
		t.Fatal(err)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/library/storage?limit=1", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("storage status=%d body=%s", response.Code, response.Body.String())
	}
	var report struct {
		Usage           domain.MemoryStorageUsage    `json:"usage"`
		Recommendations []map[string]json.RawMessage `json:"recommendations"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Usage.Tombstones != 1 || report.Usage.FullCopyItems != 2 || len(report.Recommendations) != 1 {
		t.Fatalf("storage report=%+v", report)
	}
	var recommendationID string
	if err := json.Unmarshal(report.Recommendations[0]["id"], &recommendationID); err != nil || recommendationID != larger.ID {
		t.Fatalf("recommendation=%v err=%v", report.Recommendations[0], err)
	}
	for field := range report.Recommendations[0] {
		switch field {
		case "id", "source", "title", "author", "contentBytes", "reclaimableBytes", "updatedAt", "reasonCode", "reviewAction":
		default:
			t.Fatalf("recommendation exposed private field %q: %s", field, response.Body.String())
		}
	}
	for _, forbidden := range []string{"provenance", "identityDigest", "tombstone", "fullContent", "contentFingerprint", "canonicalPermalink"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("storage response exposed forbidden value %q: %s", forbidden, response.Body.String())
		}
	}

	for _, test := range []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "too many recommendations", method: http.MethodGet, path: "/api/library/storage?limit=13", want: http.StatusBadRequest},
		{name: "invalid limit", method: http.MethodGet, path: "/api/library/storage?limit=nope", want: http.StatusBadRequest},
		{name: "mutation method", method: http.MethodPost, path: "/api/library/storage", want: http.StatusNotFound},
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

func TestLibraryHTTPForgetPermanentlyTombstonesAndSuppressesRecapture(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	input := libraryHTTPInput("2302")
	item, err := state.CreateMemoryRecallStub(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/library/items/"+item.ID+"/forget-permanently", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("forget status=%d body=%s", response.Code, response.Body.String())
	}
	var forgotten struct {
		Forgotten bool   `json:"forgotten"`
		ID        string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&forgotten); err != nil {
		t.Fatal(err)
	}
	if !forgotten.Forgotten || forgotten.ID != item.ID {
		t.Fatalf("forget response=%+v", forgotten)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/library/items?query=HTTP+Library&limit=10", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	var listed struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if response.Code != http.StatusOK {
		t.Fatalf("forgotten item still searchable status=%d body=%s", response.Code, response.Body.String())
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("forgotten item still searchable body=%v", listed.Items)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/library/items/"+item.ID, nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("forgotten detail status=%d body=%s", response.Code, response.Body.String())
	}
	if _, err := state.CreateMemoryRecallStub(ctx, input); !errors.Is(err, store.ErrMemoryTombstoned) {
		t.Fatalf("forgotten item was recaptured err=%v", err)
	}
}

func TestLibraryHTTPRemovePhysicallyClearsAndAllowsRecapture(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	input := libraryHTTPInput("2303")
	item, err := state.CreateMemoryRecallStub(ctx, input)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodDelete, "/api/library/items/"+item.ID, nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("remove status=%d body=%s", response.Code, response.Body.String())
	}
	var removed struct {
		Removed bool   `json:"removed"`
		ID      string `json:"id"`
	}
	if err := json.NewDecoder(response.Body).Decode(&removed); err != nil {
		t.Fatal(err)
	}
	if !removed.Removed || removed.ID != item.ID {
		t.Fatalf("remove response=%+v", removed)
	}

	request = httptest.NewRequest(http.MethodGet, "/api/library/items?query=HTTP+Library&limit=10", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("removed item search status=%d body=%s", response.Code, response.Body.String())
	}
	var listed struct {
		Items []map[string]json.RawMessage `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Items) != 0 {
		t.Fatalf("removed item still searchable body=%v", listed.Items)
	}
	request = httptest.NewRequest(http.MethodGet, "/api/library/items/"+item.ID, nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("removed detail status=%d body=%s", response.Code, response.Body.String())
	}
	recreated, err := state.CreateMemoryRecallStub(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if recreated.ID == item.ID {
		t.Fatalf("physical removal reused item id=%q", recreated.ID)
	}
	usage, err := state.MemoryStorage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if usage.Tombstones != 0 {
		t.Fatalf("ordinary remove created tombstones=%d", usage.Tombstones)
	}
}
