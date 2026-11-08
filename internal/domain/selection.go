package domain

type SelectionReason string

const (
	SelectionBaseline              SelectionReason = "baseline"
	SelectionPreferencePromotion   SelectionReason = "preference_promotion"
	SelectionProtected             SelectionReason = "protected"
	SelectionDiscovery             SelectionReason = "discovery"
	SelectionPreviouslyDelivered   SelectionReason = "previously_delivered"
	SelectionInsufficientEvidence  SelectionReason = "insufficient_evidence"
	SelectionBelowAdmission        SelectionReason = "below_admission"
	SelectionPreferenceSuppressed  SelectionReason = "preference_suppressed"
	SelectionBudgetExhausted       SelectionReason = "budget_exhausted"
	SelectionDisplacedByProtection SelectionReason = "displaced_by_protection"
	SelectionDisplacedByDiscovery  SelectionReason = "displaced_by_discovery"
)

// SelectionDecision records the original per-run policy decision. Subsequent
// Timeline composition and user corrections have their own outcomes.
type SelectionDecision struct {
	PolicyVersion string          `json:"policyVersion"`
	Reason        SelectionReason `json:"reason"`
	Selected      bool            `json:"selected"`
}

type ScoredAssessment struct {
	Assessment                             CandidateAssessment
	BaseScore, PreferenceScore, FinalScore float64
	Selected                               bool
	Decision                               *SelectionDecision
}
