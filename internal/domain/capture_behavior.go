package domain

// CaptureBehaviorStatus is a scheduling diagnostic, never capture authority.
type CaptureBehaviorStatus struct {
	Enabled   bool   `json:"enabled"`
	Source    Source `json:"source"`
	Allowed   bool   `json:"allowed"`
	Reason    string `json:"reason,omitempty"`
	NotBefore string `json:"notBefore,omitempty"`
}
