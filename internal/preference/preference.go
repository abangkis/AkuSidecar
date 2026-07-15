package preference

import (
	"math"

	"github.com/abangkis/AkuSidecar/internal/domain"
)

type Signal struct {
	Direction string
	Reason    *string
	Facets    []string
	Origin    string
}

type Profile struct {
	Weights          map[string]float64 `json:"weights"`
	EffectiveSignals int                `json:"effectiveSignals"`
	PositiveSignals  int                `json:"positiveSignals"`
	NegativeSignals  int                `json:"negativeSignals"`
	NeutralSignals   int                `json:"neutralSignals"`
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
			}
			profile.PositiveSignals++
		case signal.Direction == "less" && signal.Reason != nil && *signal.Reason == "not_interested":
			weight = -1
			profile.NegativeSignals++
		case signal.Direction == "less" && signal.Reason == nil:
			weight = -0.5
			profile.NegativeSignals++
		default:
			profile.NeutralSignals++
		}
		if weight == 0 {
			continue
		}
		profile.EffectiveSignals++
		for _, facet := range signal.Facets {
			profile.Weights[facet] += weight
			counts[facet]++
		}
	}
	for facet, value := range profile.Weights {
		normalized := value / math.Max(3, float64(counts[facet]))
		profile.Weights[facet] = math.Max(-1, math.Min(1, normalized))
	}
	profile.PromotionReady = profile.EffectiveSignals >= 8 && profile.PositiveSignals >= 4
	// Suppression remains deliberately conservative. Quality/holdout gates are
	// added with App Server evaluation; signal count alone never enables it.
	profile.SuppressionReady = false
	return profile
}

func Score(profile Profile, assessment domain.CandidateAssessment) float64 {
	if !profile.PromotionReady {
		return 0
	}
	if len(assessment.TopicFacets) == 0 {
		return 0
	}
	sum := 0.0
	for _, facet := range assessment.TopicFacets {
		sum += profile.Weights[facet]
	}
	return 0.12 * (sum / float64(len(assessment.TopicFacets)))
}
