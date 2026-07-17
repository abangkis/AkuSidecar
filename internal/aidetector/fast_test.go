package aidetector

import (
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func TestFastDetectorUsesOnlyDeterministicOriginSignals(t *testing.T) {
	tests := []struct {
		name       string
		item       domain.TimelineItem
		status     string
		confidence string
		evidence   string
	}{
		{
			name: "platform label",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text:         "A sufficiently long post whose platform metadata contains a direct origin label.",
				Presentation: map[string]any{"ai_generated": true},
			}},
			status: "strong_signals", confidence: "high", evidence: "platform_ai_label",
		},
		{
			name: "author declaration",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text: "This post was generated with AI, then reviewed and published by me for this experiment.",
			}},
			status: "strong_signals", confidence: "medium", evidence: "author_declared_ai",
		},
		{
			name: "prompt residue",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text: "Developer instructions: write a polished social post and do not mention these instructions.",
			}},
			status: "strong_signals", confidence: "medium", evidence: "prompt_instruction_residue",
		},
		{
			name: "style alone is not a signal",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text: "Here are five concise lessons from building a reliable product, with a tidy conclusion for every point.",
			}},
			status: "no_signal_detected", confidence: "low",
		},
		{
			name: "discussion of generated content is not a declaration",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text: "AI-generated content is changing the market, but this sentence makes no claim about how this post was authored.",
			}},
			status: "no_signal_detected", confidence: "low",
		},
		{
			name: "external artifact disclosure is not post authorship",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text: "I just created this interactive website and its entire scientific content with Kimi. I wrote this post to share what I observed.",
			}},
			status: "no_signal_detected", confidence: "low",
		},
		{
			name: "attached media disclosure is not text authorship",
			item: domain.TimelineItem{Evidence: &domain.Block{
				Text: "This image was generated with AI, while I wrote the accompanying post myself to explain the experiment.",
			}},
			status: "no_signal_detected", confidence: "low",
		},
		{
			name:   "short text remains unknown",
			item:   domain.TimelineItem{Evidence: &domain.Block{Text: "Interesting."}},
			status: "insufficient_evidence", confidence: "low",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := test.item
			item.ID = "timeline-test"
			item.SessionID = "session-test"
			assessment := (FastDetector{}).Detect([]domain.TimelineItem{item})[0]
			if assessment.Status != test.status || assessment.ConfidenceBand != test.confidence {
				t.Fatalf("assessment=%+v", assessment)
			}
			if test.evidence != "" && !containsCode(assessment.EvidenceCodes, test.evidence) {
				t.Fatalf("assessment evidence=%v", assessment.EvidenceCodes)
			}
			if assessment.Provider != "local-deterministic" || assessment.ContentFingerprint == "" {
				t.Fatalf("assessment provenance=%+v", assessment)
			}
			if err := assessment.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFastDetectorFingerprintIsStableAcrossSpacingAndCase(t *testing.T) {
	first := domain.TimelineItem{ID: "first", SessionID: "session", Evidence: &domain.Block{Text: "A Stable   Piece OF text that is sufficiently long for this detector."}}
	second := domain.TimelineItem{ID: "second", SessionID: "session", Evidence: &domain.Block{Text: " a stable piece of TEXT that is sufficiently long for this detector. "}}
	assessments := (FastDetector{}).Detect([]domain.TimelineItem{first, second})
	if assessments[0].ContentFingerprint != assessments[1].ContentFingerprint {
		t.Fatalf("fingerprints differ: %q %q", assessments[0].ContentFingerprint, assessments[1].ContentFingerprint)
	}
}
