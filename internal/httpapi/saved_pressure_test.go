package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestSavedPressureHTTPIsCurrentFIFOAndContentSafe(t *testing.T) {
	server, state := openLibraryHTTPFixture(t)
	fullFixture := createTimelineMemoryHTTPFixture(t, state, "completed", "Saved full pressure body", "2601")
	recallFixture := createTimelineMemoryHTTPFixture(t, state, "completed", "", "2602")
	if _, _, err := state.ReadLaterTimeline(t.Context(), fullFixture.TimelineID); err != nil {
		t.Fatal(err)
	}
	recall, _, err := state.ReadLaterTimeline(t.Context(), recallFixture.TimelineID)
	if err != nil {
		t.Fatal(err)
	}
	doneFixture := createTimelineMemoryHTTPFixture(t, state, "completed", "Done pressure body", "2603")
	done, _, err := state.ReadLaterTimeline(t.Context(), doneFixture.TimelineID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := state.DoneSavedMemory(t.Context(), done.ID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/library/storage?limit=10", nil)
	response := httptest.NewRecorder()
	server.api().ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("storage status=%d body=%s", response.Code, response.Body.String())
	}
	body := append([]byte(nil), response.Body.Bytes()...)
	var report struct {
		Usage                domain.MemoryStorageUsage    `json:"usage"`
		Recommendations      []map[string]json.RawMessage `json:"recommendations"`
		SavedPressure        domain.MemorySavedPressure   `json:"savedPressure"`
		SavedRecommendations []struct {
			ID              string            `json:"id"`
			Source          domain.Source     `json:"source"`
			Title           string            `json:"title"`
			Author          string            `json:"author"`
			SavedAt         string            `json:"savedAt"`
			RetentionTier   domain.MemoryTier `json:"retentionTier"`
			ContentBytes    int64             `json:"contentBytes"`
			SourceDependent bool              `json:"sourceDependent"`
			ReasonCode      string            `json:"reasonCode"`
			ReviewAction    string            `json:"reviewAction"`
		} `json:"savedRecommendations"`
	}
	if err := json.NewDecoder(response.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.SavedPressure.ActiveItems != 2 || report.SavedPressure.LocalCopyItems != 1 ||
		report.SavedPressure.SourceDependentItems != 1 || report.SavedPressure.ContentBytes != int64(len("Saved full pressure body")) ||
		report.SavedPressure.OldestClaimedAt == "" {
		t.Fatalf("Saved pressure=%+v", report.SavedPressure)
	}
	if len(report.SavedRecommendations) != 2 || len(report.Recommendations) != 0 {
		t.Fatalf("storage recommendations=%+v Saved=%+v", report.Recommendations, report.SavedRecommendations)
	}
	if report.SavedRecommendations[0].SavedAt > report.SavedRecommendations[1].SavedAt {
		t.Fatalf("Saved recommendations not FIFO=%+v", report.SavedRecommendations)
	}
	if report.SavedRecommendations[0].SavedAt == report.SavedRecommendations[1].SavedAt {
		ids := []string{report.SavedRecommendations[0].ID, report.SavedRecommendations[1].ID}
		sort.Strings(ids)
		if report.SavedRecommendations[0].ID != ids[0] || report.SavedRecommendations[1].ID != ids[1] {
			t.Fatalf("Saved tie-break order=%v", []string{report.SavedRecommendations[0].ID, report.SavedRecommendations[1].ID})
		}
	}
	for _, recommendation := range report.SavedRecommendations {
		wantSourceDependent := recommendation.RetentionTier != domain.MemoryTierFullCopy || recommendation.ContentBytes <= 0
		if recommendation.SourceDependent != wantSourceDependent {
			t.Fatalf("Saved source-dependent flags=%+v", report.SavedRecommendations)
		}
	}
	for _, recommendation := range report.SavedRecommendations {
		if recommendation.ReasonCode != "oldest_saved" || recommendation.ReviewAction != "review_saved" || recommendation.SavedAt == "" {
			t.Fatalf("Saved recommendation metadata=%+v", recommendation)
		}
		if recommendation.Source == "" || recommendation.Title == "" || recommendation.Author == "" {
			t.Fatalf("Saved recommendation bounded identity=%+v", recommendation)
		}
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	var savedRaw []map[string]json.RawMessage
	if err := json.Unmarshal(raw["savedRecommendations"], &savedRaw); err != nil {
		t.Fatal(err)
	}
	for _, item := range savedRaw {
		for _, forbidden := range []string{"fullContent", "provenance", "actions", "contentFingerprint", "claimedAt", "resolvedAt"} {
			if _, exists := item[forbidden]; exists {
				t.Fatalf("Saved recommendation exposed %q: %s", forbidden, body)
			}
		}
	}
	_ = recall
}
