package domain

// SourceCapturePause deliberately uses the local X scope: a verified account
// identity is not available on every capture lane. It stores no page or secret.
type SourceCapturePause struct {
	ID       string `json:"id"`
	Source   Source `json:"source"`
	Code     string `json:"code"`
	Category string `json:"category,omitempty"`
	Message  string `json:"message"`
	PausedAt string `json:"pausedAt"`
}

const XChallengeMessage = "X requires human verification. Capture is paused. Complete verification manually in X, then explicitly confirm recovery here."

func IsAccessChallenge(code string) bool {
	return code == "challenge_required" || code == "challenge_detected"
}

func XChallengeCategory(failure Failure) string {
	category, _ := failure.Details["challengeCategory"].(string)
	if category == "cloudflare" || category == "x_human_verification" {
		return category
	}
	return ""
}

func SafeXChallenge(failure Failure) Failure {
	category := XChallengeCategory(failure)
	failure.Code, failure.Retryable, failure.Message, failure.Details = "challenge_required", false, XChallengeMessage, nil
	if category != "" {
		failure.Details = map[string]any{"challengeCategory": category}
	}
	return failure
}
