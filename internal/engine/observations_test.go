package engine

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

func observationContractFixture() domain.Observation {
	return domain.Observation{Source: domain.SourceX, CapturedAt: "2026-11-02T00:00:00Z", Snapshots: []domain.Snapshot{{Blocks: []domain.Block{{EvidenceKey: "x:fixture", Author: "Fixture", Text: "Offline captured evidence", Permalink: "https://x.com/a/status/42"}}}}, Coverage: map[string]any{}}
}
func observationContractJSON(t *testing.T, value any) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestEngineObservationCharacterizationNormalization(t *testing.T) {
	for _, test := range []struct {
		name   string
		source domain.Source
		block  domain.Block
		want   string
	}{
		{"existing-key", domain.SourceX, domain.Block{EvidenceKey: "  Keep-Key  ", PlatformID: "42", Text: "Body"}, "  Keep-Key  "},
		{"platform-precedence", domain.SourceX, domain.Block{PlatformID: " 42 ", Permalink: "https://x.com/a/status/43", Author: "Alice", Text: "Body"}, "x:1d6fc8095003910a3a02247f"},
		{"permalink-precedence", domain.SourceX, domain.Block{Permalink: " https://x.com/a/status/42 ", Author: "Alice", Text: "Body"}, "x:1c76f9d7e57951dc9dc825e0"},
		{"author-text", domain.SourceX, domain.Block{Author: " ALICE\t SMITH ", Text: " Hello\nWORLD "}, "x:22b00a0402c08ee44a1c764a"},
		{"author-only", domain.SourceX, domain.Block{Author: " ALICE "}, "x:024f305cb81a215120b68390"},
		{"other-source", domain.SourceLinkedIn, domain.Block{PlatformID: "42", Text: "Body"}, "linkedin:28821cc08a255fec8fa583b8"},
		{"media-without-identity", domain.SourceX, domain.Block{Media: []map[string]any{{"kind": "image"}}}, ""},
		{"quoted-without-root-identity", domain.SourceX, domain.Block{QuotedPost: map[string]any{"text": "Quoted body"}}, ""},
		{"empty-identity", domain.SourceX, domain.Block{Author: "\t", Text: " "}, ""},
		{"blank-key-replaced", domain.SourceX, domain.Block{EvidenceKey: " \t", PlatformID: "42", Text: "Body"}, "x:1d6fc8095003910a3a02247f"},
	} {
		t.Run(test.name, func(t *testing.T) {
			value := domain.Observation{Source: test.source, Snapshots: []domain.Snapshot{{CapturedAt: "unchanged-snapshot", Blocks: []domain.Block{test.block}}}, Coverage: map[string]any{"fixture": "unchanged"}}
			before := observationContractJSON(t, value)
			alias := value.Snapshots[0].Blocks
			normalizeObservation(&value)
			if got := value.Snapshots[0].Blocks[0].EvidenceKey; got != test.want || alias[0].EvidenceKey != test.want {
				t.Fatalf("key=%q want=%q", got, test.want)
			}
			value.Snapshots[0].Blocks[0].EvidenceKey = test.block.EvidenceKey
			if observationContractJSON(t, value) != before {
				t.Fatal("normalization changed fields other than evidenceKey")
			}
			value.Snapshots[0].Blocks[0].EvidenceKey = test.want
			once := observationContractJSON(t, value)
			normalizeObservation(&value)
			if observationContractJSON(t, value) != once {
				t.Fatal("normalization is not idempotent")
			}
		})
	}
}

func TestEngineObservationCharacterizationValidationOrder(t *testing.T) {
	for _, mode := range []string{"source", "snapshots", "key", "evidence", "permalink", "context", "attachment-count", "attachment-invalid", "attachment-external-url", "blocks", "coverage", "empty-coverage", "repeated-key", "whitespace-key", "media-only"} {
		t.Run(mode, func(t *testing.T) {
			value := observationContractFixture()
			block := &value.Snapshots[0].Blocks[0]
			want := ""
			switch mode {
			case "source":
				value.Source = "invalid"
				value.Snapshots = nil
				want = "invalid observation source"
			case "snapshots":
				value.Snapshots = nil
				value.Coverage = nil
				want = "observation has no snapshots"
			case "key":
				block.EvidenceKey = ""
				block.Text = ""
				block.Permalink = "https://outside.example/"
				want = "captured block is missing evidenceKey"
			case "evidence":
				block.Text = " "
				block.Permalink = "https://outside.example/"
				want = "captured block has no admissible text, media, attachment, or quoted-post evidence"
			case "permalink":
				block.Permalink = "https://outside.example/"
				block.DirectContext = []domain.DirectContext{{Kind: "invalid"}}
				want = "captured block permalink is not a canonical native source URL"
			case "context":
				block.DirectContext = []domain.DirectContext{{Kind: "invalid"}}
				block.Attachments = make([]domain.Attachment, 4)
				want = "unsupported direct context relation"
			case "attachment-count":
				block.Attachments = make([]domain.Attachment, 4)
				want = "captured block exceeds the attachment limit"
			case "attachment-invalid":
				block.Attachments = []domain.Attachment{{Kind: "invalid"}}
				want = `captured block attachment is invalid: unsupported attachment kind "invalid"`
			case "attachment-external-url":
				block.Text = ""
				block.Attachments = []domain.Attachment{{Kind: "link_preview", Title: "Captured external link", URL: "https://outside.example/article"}}
			case "blocks":
				value.Snapshots[0].Blocks = nil
				value.Coverage = nil
				want = "observation contains no evidence blocks"
			case "coverage":
				value.Coverage = nil
				want = "observation coverage is required"
			case "repeated-key":
				value.Snapshots[0].Blocks = append(value.Snapshots[0].Blocks, *block)
			case "whitespace-key":
				block.EvidenceKey = "   "
			case "media-only":
				block.Text = ""
				block.Media = []map[string]any{{"kind": "image", "url": "https://pbs.twimg.com/media/fixture.jpg"}}
			}
			before := observationContractJSON(t, value)
			err := validateObservation(value)
			if want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != want {
				t.Fatalf("err=%v want=%q", err, want)
			}
			if observationContractJSON(t, value) != before {
				t.Fatal("validation mutated the payload")
			}
		})
	}
}

func TestEngineObservationCharacterizationEvidenceShapes(t *testing.T) {
	for _, test := range []struct {
		name  string
		block domain.Block
		want  bool
	}{
		{"identity-only", domain.Block{PlatformID: "42", Author: "Alice", Permalink: "https://x.com/a/status/42"}, false},
		{"direct-context-only", domain.Block{DirectContext: []domain.DirectContext{{ObservedText: "Context body"}}}, false},
		{"blank-text", domain.Block{Text: " \t"}, false},
		{"text", domain.Block{Text: "Body"}, true},
		{"empty-media", domain.Block{Media: []map[string]any{}}, false},
		{"media-entry", domain.Block{Media: []map[string]any{{}}}, true},
		{"attachment-entry", domain.Block{Attachments: []domain.Attachment{{}}}, true},
		{"unrelated-quote-key", domain.Block{QuotedPost: map[string]any{"author": "Alice"}}, false},
		{"empty-quote-text", domain.Block{QuotedPost: map[string]any{"text": "  "}}, false},
		{"empty-quote-array", domain.Block{QuotedPost: map[string]any{"media": []any{}}}, false},
		{"unsupported-quote-type", domain.Block{QuotedPost: map[string]any{"links": []string{"https://example.test"}}}, false},
		{"numeric-quote-value", domain.Block{QuotedPost: map[string]any{"text": 1}}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := blockHasEvidence(test.block); got != test.want {
				t.Fatalf("hasEvidence=%t want=%t", got, test.want)
			}
		})
	}
	for _, key := range []string{"text", "media", "links"} {
		for _, test := range []struct {
			name  string
			value any
		}{{"string", "Body"}, {"any-array", []any{nil}}, {"map-array", []map[string]any{{}}}} {
			t.Run("quote-"+key+"-"+test.name, func(t *testing.T) {
				if !blockHasEvidence(domain.Block{QuotedPost: map[string]any{key: test.value}}) {
					t.Fatal("supported quoted evidence shape rejected")
				}
			})
		}
	}
}

func TestEngineObservationCharacterizationBridgeCaller(t *testing.T) {
	for _, mode := range []string{"accepted", "owner", "source", "invalid-after-normalize", "cancelled-request"} {
		t.Run(mode, func(t *testing.T) {
			f := newBridgeCommandFixture(t)
			command := f.claim(t)
			value := f.observation()
			alias := value.Snapshots[0].Blocks
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "owner":
				f.e.epoch = "replacement-epoch"
			case "source":
				value.Source = domain.SourceLinkedIn
				if value.Source == f.run.Source {
					value.Source = domain.SourceFacebook
				}
			case "invalid-after-normalize":
				alias[0].PlatformID = "42"
				alias[0].Permalink = "https://outside.example/"
			case "cancelled-request":
				cancel()
			}
			_, err := f.e.AcceptObservation(ctx, command.ID, command.RunID, value)
			if mode == "accepted" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("invalid observation accepted")
			}
			if mode == "owner" && !errors.Is(err, errStaleCaptureRuntime) {
				t.Fatalf("owner guard=%v", err)
			}
			if mode == "source" && err.Error() != "observation source does not match run" {
				t.Fatalf("source guard=%v", err)
			}
			if mode == "cancelled-request" && !errors.Is(err, context.Canceled) {
				t.Fatalf("context=%v", err)
			}
			mutated := alias[0].EvidenceKey != ""
			if mutated != (mode == "accepted" || mode == "invalid-after-normalize") {
				t.Fatalf("guard/normalization order changed: key=%q err=%v", alias[0].EvidenceKey, err)
			}
			if mode == "invalid-after-normalize" && !strings.Contains(err.Error(), "permalink") {
				t.Fatalf("validation=%v", err)
			}
			observations, loadErr := f.state.Observations(context.Background(), f.run.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			want := 0
			if mode == "accepted" {
				want = 1
			}
			if len(observations) != want {
				t.Fatalf("rejected observation persisted: %d", len(observations))
			}
			if want == 1 && observations[0].Snapshots[0].Blocks[0].EvidenceKey != alias[0].EvidenceKey {
				t.Fatal("normalization happened after persistence")
			}
		})
	}
}

func TestEngineObservationCharacterizationRecaptureCaller(t *testing.T) {
	for _, mode := range []string{"accepted", "owner", "invalid-after-normalize", "invalid-source"} {
		t.Run(mode, func(t *testing.T) {
			f := newRecaptureEngineFixture(t, domain.SourceX)
			attachTestCapture(t, f.e)
			job := f.queue(t)
			if _, err := f.e.ClaimMediaRecapture(context.Background(), job.ID, "offline"); err != nil {
				t.Fatal(err)
			}
			value := f.observation
			value.Snapshots[0].Blocks[0].EvidenceKey = ""
			alias := value.Snapshots[0].Blocks
			switch mode {
			case "owner":
				f.e.epoch = "replacement-epoch"
			case "invalid-after-normalize":
				alias[0].Permalink = "https://outside.example/"
			case "invalid-source":
				value.Source = "invalid"
			}
			result, err := f.e.AcceptMediaRecapture(context.Background(), job.ID, value)
			if mode == "accepted" {
				if err != nil || result.Status != "completed" {
					t.Fatalf("recapture=%+v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("invalid recapture accepted")
			}
			if mode == "owner" {
				if !errors.Is(err, errStaleCaptureRuntime) || alias[0].EvidenceKey != "" {
					t.Fatalf("owner guard followed normalization: %q %v", alias[0].EvidenceKey, err)
				}
			} else if alias[0].EvidenceKey == "" {
				t.Fatal("recapture validation preceded normalization")
			}
			if mode == "invalid-source" && err.Error() != "invalid observation source" {
				t.Fatalf("recapture source/validation order=%v", err)
			}
			stored, loadErr := f.e.MediaRecapture(context.Background(), job.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			want := "claimed"
			if mode == "accepted" {
				want = "completed"
			}
			if stored.Status != want {
				t.Fatalf("recapture persistence=%s want=%s", stored.Status, want)
			}
		})
	}
}

func TestEngineObservationCharacterizationRepeatedIdentity(t *testing.T) {
	value := observationContractFixture()
	value.Snapshots[0].Blocks = []domain.Block{{PlatformID: "42", Text: "First"}, {PlatformID: "42", Text: "Second"}}
	normalizeObservation(&value)
	if value.Snapshots[0].Blocks[0].EvidenceKey != value.Snapshots[0].Blocks[1].EvidenceKey {
		t.Fatal("normalization invented per-block identity")
	}
	if err := validateObservation(value); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]string{value.Snapshots[0].Blocks[0].Text, value.Snapshots[0].Blocks[1].Text}, []string{"First", "Second"}) {
		t.Fatal("validation deduplicated or rewrote blocks")
	}
}
