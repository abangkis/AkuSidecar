package aidetector

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type reviewCorpusCase struct {
	Name                string `json:"name"`
	Text                string `json:"text"`
	ExpectedFastStatus  string `json:"expectedFastStatus"`
	ExpectedShortlisted bool   `json:"expectedShortlisted"`
}

func TestAIReviewShortlistCorpus(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "review-shortlist-corpus.json"))
	if err != nil {
		t.Fatal(err)
	}
	var corpus []reviewCorpusCase
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if len(corpus) < 10 {
		t.Fatalf("AI review corpus is too small: %d", len(corpus))
	}

	truePositive, falsePositive, trueNegative, falseNegative := 0, 0, 0, 0
	for _, test := range corpus {
		item := domain.TimelineItem{
			ID: "timeline-" + test.Name, SessionID: "corpus-session",
			Evidence: &domain.Block{Text: test.Text},
		}
		assessment := (FastDetector{}).Detect([]domain.TimelineItem{item})[0]
		if assessment.Status != test.ExpectedFastStatus {
			t.Errorf("%s: fast status=%s want=%s", test.Name, assessment.Status, test.ExpectedFastStatus)
		}
		item.AIDetection = &domain.TimelineAIDetection{
			Stage: assessment.Stage, Status: assessment.Status, ConfidenceBand: assessment.ConfidenceBand,
			EvidenceCodes: assessment.EvidenceCodes, AssessedObject: assessment.AssessedObject,
			SignalScope: assessment.SignalScope,
		}
		shortlisted := len(DeepReviewShortlist([]domain.TimelineItem{item}, 1)) == 1
		switch {
		case shortlisted && test.ExpectedShortlisted:
			truePositive++
		case shortlisted:
			falsePositive++
		case test.ExpectedShortlisted:
			falseNegative++
		default:
			trueNegative++
		}
		if shortlisted != test.ExpectedShortlisted {
			t.Errorf("%s: shortlisted=%t want=%t", test.Name, shortlisted, test.ExpectedShortlisted)
		}
	}
	if truePositive == 0 || trueNegative == 0 {
		t.Fatalf("corpus must exercise positive and negative controls: tp=%d tn=%d", truePositive, trueNegative)
	}
	t.Logf("AI review shortlist corpus: tp=%d fp=%d tn=%d fn=%d", truePositive, falsePositive, trueNegative, falseNegative)
}
