package preference

import (
	"math"
	"strings"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type Signal struct {
	Direction string
	Reason    *string
	Tags      []string
	Facets    []string
	Origin    string
}

type Profile struct {
	Weights          map[string]float64 `json:"weights"`
	EffectiveSignals int                `json:"effectiveSignals"`
	PositiveSignals  int                `json:"positiveSignals"`
	NegativeSignals  int                `json:"negativeSignals"`
	NeutralSignals   int                `json:"neutralSignals"`
	AuthorityReady   bool               `json:"authorityReady"`
	PromotionReady   bool               `json:"promotionReady"`
	SuppressionReady bool               `json:"suppressionReady"`
}

func Fit(signals []Signal) Profile {
	profile := Profile{Weights: map[string]float64{}}
	counts := map[string]int{}
	for _, signal := range signals {
		weight := 0.0
		switch {
		case signal.Direction == "more":
			weight = 1
			if signal.Origin == "calibration" {
				weight = 1.1
			} else if signal.Origin == "selection_correction" {
				weight = 1.25
			}
			profile.PositiveSignals++
		case signal.Direction == "less" && signal.Reason != nil && *signal.Reason == "not_interested":
			weight = -1
			profile.NegativeSignals++
		case signal.Direction == "less" && signal.Reason == nil:
			weight = -0.75
			if signal.Origin == "calibration" {
				weight = -1.1
			}
			profile.NegativeSignals++
		default:
			profile.NeutralSignals++
		}
		if weight == 0 {
			continue
		}
		profile.EffectiveSignals++
		addFeatures(profile.Weights, counts, "tag:", signal.Tags, weight)
		addFeatures(profile.Weights, counts, "facet:", signal.Facets, weight)
	}
	for facet, value := range profile.Weights {
		normalized := value / math.Max(3, float64(counts[facet]))
		profile.Weights[facet] = math.Max(-1, math.Min(1, normalized))
	}
	// Direct labels are the highest-authority relevance signal in AkuBrowser.
	// A small repeated pattern is enough to activate one direction; capture
	// quality and material-update protections remain independent hard gates.
	profile.PromotionReady = profile.EffectiveSignals >= 3 && profile.PositiveSignals >= 2
	profile.SuppressionReady = profile.EffectiveSignals >= 3 && profile.NegativeSignals >= 2
	profile.AuthorityReady = profile.PromotionReady || profile.SuppressionReady
	return profile
}

func Score(profile Profile, assessment domain.CandidateAssessment) float64 {
	if !profile.AuthorityReady {
		return 0
	}
	return 0.45 * Alignment(profile, assessment)
}

// Alignment returns the user's learned relevance signal on a stable [-1, 1]
// scale. It is intentionally separate from generic materiality and evidence.
func Alignment(profile Profile, assessment domain.CandidateAssessment) float64 {
	tagAlignment, tagMatches := featureAlignment(profile, "tag:", assessment.TopicTags, true)
	facetAlignment, _ := featureAlignment(profile, "facet:", assessment.TopicFacets, false)
	if tagMatches > 0 {
		return clamp(0.75*tagAlignment + 0.25*facetAlignment)
	}
	// Broad facets preserve weak generalization when no precise topic tag
	// matches. They cannot inherit the full authority of an explicit label.
	return clamp(0.30 * facetAlignment)
}

func addFeatures(weights map[string]float64, counts map[string]int, prefix string, values []string, weight float64) {
	seen := map[string]bool{}
	for _, value := range values {
		key := featureKey(prefix, value)
		if key == prefix || seen[key] {
			continue
		}
		seen[key] = true
		weights[key] += weight
		counts[key]++
	}
}

func featureAlignment(profile Profile, prefix string, values []string, knownOnly bool) (float64, int) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	matches := 0
	seen := map[string]bool{}
	for _, value := range values {
		key := featureKey(prefix, value)
		if key == prefix || seen[key] {
			continue
		}
		seen[key] = true
		weight, known := profile.Weights[key]
		if known {
			sum += weight
			matches++
		}
	}
	denominator := len(seen)
	if knownOnly {
		denominator = matches
	}
	if denominator == 0 {
		return 0, matches
	}
	return clamp(sum / float64(denominator)), matches
}

func featureKey(prefix, value string) string {
	return prefix + strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func clamp(value float64) float64 {
	return math.Max(-1, math.Min(1, value))
}
