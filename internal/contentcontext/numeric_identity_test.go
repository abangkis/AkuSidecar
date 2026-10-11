package contentcontext

import "testing"

func TestRelatedIdentityRejectsPercentagesAndDurations(t *testing.T) {
	for _, text := range []string{
		"Codex reset when the limit has 10% or less remaining",
		"Codex reset when the limit has10% remaining",
		"Codex reset when the limit has 10.5 % remaining",
		"Codex reset when the limit has 10,5％ remaining",
		"Codex reset when the limit has 10٪ remaining",
		"Codex reset when the limit has 10 percent remaining",
		"Codex reset when the limit has 10 percentage points remaining",
		"Codex reset when the limit has 10 persen remaining",
		"Codex reset in approximately 8 hours and 40 minutes",
		"CODEX USERS: USAGE RESET IN 8 HRS 40 MINNSSSSS",
		"Codex reset in approximately 8h40m",
		"Codex reset in approximately 8.5 hours",
		"Codex reset after waiting 40 seconds",
		"Codex reset after waiting 40ms",
		"Codex reset in approximately 08:40",
		"Codex reset after waiting 2 days",
		"Codex reset setelah menunggu 8 jam dan 40 menit",
	} {
		t.Run(text, func(t *testing.T) {
			query := NewEngine().Extract(relatedTimeline(text, "", "", "", "Codex", "reset"))
			if len(query.IdentityPhrases) != 0 {
				t.Fatalf("measurement became a model identity: %+v", query)
			}
			matches := NewEngine().Match(query, []Candidate{{Item: testMemory("reset", "Codex resets suspended for a month", "Temporary relief from usage limits")}}, 3)
			if len(matches) != 1 {
				t.Fatalf("measurement blocked Codex reset context: query=%+v matches=%+v", query, matches)
			}
		})
	}
}

func TestRelatedMeasurementParsingPreservesAtomicModels(t *testing.T) {
	for _, example := range []struct{ text, identity, positive, negative string }{
		{"GPT-6.1 coding error rate fell by 10% after waiting 8 hours", "gpt 6 1", "GPT-6.1 coding error analysis", "GPT-6.2 coding error analysis"},
		{"Step5Preview coding latency dropped 10.5 percent in 40ms", "step 5 preview", "Step 5 Preview coding latency notes", "Step 5 Beta coding latency notes"},
		{"opus5.5 coding latency improves after waiting 8h40m", "opus 5 5", "Opus 5.5 coding latency notes", "Opus 5.6 coding latency notes"},
		{"GPT-6.1 coding improvements. Hours later, another post appeared", "gpt 6 1", "GPT-6.1 coding notes", "GPT coding notes"},
		{"Coding improvements for GPT-6.1\nHours later, another post appeared", "gpt 6 1", "GPT-6.1 coding notes", "GPT coding notes"},
	} {
		t.Run(example.text, func(t *testing.T) {
			query := NewEngine().Extract(relatedTimeline(example.text, "", "", ""))
			if len(query.IdentityPhrases) != 1 || query.IdentityPhrases[0] != example.identity {
				t.Fatalf("model changed or measurement leaked: %+v", query)
			}
			if len(NewEngine().Match(query, []Candidate{{Item: testMemory("same", example.positive, "")}}, 3)) != 1 {
				t.Fatal("exact model and feature lost")
			}
			if len(NewEngine().Match(query, []Candidate{{Item: testMemory("other", example.negative, "")}}, 3)) != 0 {
				t.Fatal("sibling or incomplete model admitted")
			}
		})
	}
}
