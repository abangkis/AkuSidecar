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
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"understandingStatus":`) {
		t.Fatalf("understanding refresh status=%d body=%s", response.Code, response.Body.String())
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

func TestLivingTopicsHTTPActivationCriteriaAndCandidateReview(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	ctx := context.Background()
	item, err := state.CreateMemoryRecallStub(ctx, libraryHTTPInput("2303"))
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/living-topics", strings.NewReader(`{"name":"Codex Reset","description":"Track reset timing","aliases":["Codex quota reset"],"includeCriteria":"Reset dates","excludeCriteria":"Generic coding tips"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		Topic domain.LivingTopic `json:"topic"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if len(created.Topic.Aliases) != 1 || created.Topic.IncludeCriteria != "Reset dates" || created.Topic.ExcludeCriteria != "Generic coding tips" || created.Topic.CriteriaRevision != 1 {
		t.Fatalf("topic=%+v", created.Topic)
	}

	decision := domain.LivingTopicRoutingDecision{TopicID: created.Topic.ID, Match: true, Confidence: 0.91, Mode: "llm", Reason: "Reset timing matches"}
	if err := state.SaveLivingTopicCandidateDecision(ctx, created.Topic, item, decision); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/living-topics/"+created.Topic.ID+"/candidates/"+item.ID+"/accept", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"accepted"`) || !strings.Contains(response.Body.String(), `"matchMode":"candidate_accept"`) {
		t.Fatalf("accept status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/living-topics/"+created.Topic.ID+"/candidates/"+item.ID+"/undo", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"suggested"`) || !strings.Contains(response.Body.String(), `"members":[]`) {
		t.Fatalf("undo status=%d body=%s", response.Code, response.Body.String())
	}

	request = httptest.NewRequest(http.MethodPost, "/api/living-topics/"+created.Topic.ID+"/activation", nil)
	response = httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted || !strings.Contains(response.Body.String(), `"routingStatus":`) {
		t.Fatalf("activation status=%d body=%s", response.Code, response.Body.String())
	}

}
