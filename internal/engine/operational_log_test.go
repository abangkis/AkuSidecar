package engine

import (
	"encoding/json"
	"strings"
)

type operationalSample struct {
	Operation string
	Phase     string
	Outcome   string
	Reason    string
	ID        string
	Detail    map[string]any
}

func findActivity(output, operation, phase, outcome string) (operationalSample, bool) {
	for _, line := range strings.Split(output, "\n") {
		_, payload, ok := strings.Cut(line, "sidecar_activity ")
		var sample operationalSample
		if ok && json.Unmarshal([]byte(payload), &sample) == nil && sample.Operation == operation && sample.Phase == phase && sample.Outcome == outcome {
			return sample, true
		}
	}
	return operationalSample{}, false
}
func hasActivity(output, operation, phase, outcome string) bool {
	_, ok := findActivity(output, operation, phase, outcome)
	return ok
}
