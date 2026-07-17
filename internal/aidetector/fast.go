package aidetector

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

const FastDetectorVersion = "fast-text-v1"

var (
	authorDeclarationPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\b(?:this|the)\s+(?:post|image|video|thread)\s+(?:was|is)\s+(?:generated|created|written|made)\s+(?:by|with|using)\s+(?:an?\s+)?(?:ai|chatgpt|claude|gemini|copilot)\b`),
		regexp.MustCompile(`(?i)\b(?:this|the)\s+(?:post|image|video|thread)\s+(?:was|is)\s+ai[- ]generated\b`),
		regexp.MustCompile(`(?i)\b(?:I|we)\s+(?:generated|created|wrote|made)\s+(?:this|the)\s+(?:post|image|video|thread)\s+(?:with|using)\s+(?:an?\s+)?(?:ai|chatgpt|claude|gemini|copilot)\b`),
		regexp.MustCompile(`(?i)\bI\s+am\s+an?\s+AI\s+(?:agent|assistant|bot)\b`),
	}
	promptResiduePatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)<\|(?:system|assistant|user)\|>`),
		regexp.MustCompile(`(?i)\b(?:system|developer)\s+(?:prompt|instruction)s?\s*:`),
		regexp.MustCompile(`(?i)\bas\s+an?\s+AI\s+language\s+model\b`),
	}
)

type FastDetector struct{}

func (FastDetector) Detect(items []domain.TimelineItem) []domain.AIAssessment {
	result := make([]domain.AIAssessment, 0, len(items))
	for _, item := range items {
		result = append(result, detectItem(item))
	}
	return result
}

func detectItem(item domain.TimelineItem) domain.AIAssessment {
	text := strings.TrimSpace(item.Item.WhatChanged)
	if item.Evidence != nil && strings.TrimSpace(item.Evidence.Text) != "" {
		text = strings.TrimSpace(item.Evidence.Text)
	}
	codes := make([]string, 0, 3)
	if item.Evidence != nil && hasPlatformAILabel(item.Evidence.Presentation) {
		codes = append(codes, "platform_ai_label")
	}
	if matchesAny(text, authorDeclarationPatterns) {
		codes = append(codes, "author_declared_ai")
	}
	if matchesAny(text, promptResiduePatterns) {
		codes = append(codes, "prompt_instruction_residue")
	}
	codes = uniqueCodes(codes, 3)
	status, confidence, rationale := "no_signal_detected", "low", "No deterministic high-authority AI origin signal was found."
	if len([]rune(text)) < 40 && len(codes) == 0 {
		status, rationale = "insufficient_evidence", "The captured authored text is too short for a responsible local assessment."
	}
	if len(codes) > 0 {
		status, confidence = "strong_signals", "medium"
		rationale = "Local deterministic evidence found an explicit AI origin signal. Deep Detection must review inferred signals before Hide can apply."
		if containsCode(codes, "platform_ai_label") {
			confidence = "high"
			rationale = "The captured platform presentation includes an explicit AI-origin label."
		}
	}
	return domain.AIAssessment{
		ID:                 domain.NewID("ai_assessment"),
		TimelineID:         item.ID,
		SessionID:          item.SessionID,
		Stage:              "fast",
		Status:             status,
		ConfidenceBand:     confidence,
		EvidenceCodes:      codes,
		AssessedObject:     "social_post",
		SignalScope:        fastSignalScope(status),
		Provider:           "local-deterministic",
		DetectorVersion:    FastDetectorVersion,
		ContentFingerprint: fingerprint(text),
		Rationale:          rationale,
		CreatedAt:          domain.Now(),
	}
}

func fastSignalScope(status string) string {
	if status == "strong_signals" {
		return "social_post"
	}
	return "none"
}

func hasPlatformAILabel(value map[string]any) bool {
	for key, raw := range value {
		normalizedKey := strings.ToLower(strings.NewReplacer("_", " ", "-", " ").Replace(key))
		if !strings.Contains(normalizedKey, "ai") && !strings.Contains(normalizedKey, "synthetic") && !strings.Contains(normalizedKey, "generated") {
			continue
		}
		switch typed := raw.(type) {
		case bool:
			if typed {
				return true
			}
		case string:
			normalizedValue := strings.ToLower(strings.TrimSpace(typed))
			if normalizedValue == "true" || strings.Contains(normalizedValue, "ai-generated") || strings.Contains(normalizedValue, "generated with ai") || strings.Contains(normalizedValue, "synthetic media") {
				return true
			}
		}
	}
	return false
}

func matchesAny(value string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(value) {
			return true
		}
	}
	return false
}

func uniqueCodes(values []string, limit int) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	sort.Strings(result)
	return result
}

func containsCode(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func fingerprint(value string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(value), " "))
	digest := sha256.Sum256([]byte(normalized))
	return fmt.Sprintf("sha256:%x", digest[:16])
}
