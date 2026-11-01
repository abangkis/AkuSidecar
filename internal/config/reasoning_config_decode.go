package config

import (
	"bytes"
	"encoding/json"
)

// UnmarshalJSON accepts either the multi-provider shape (activeProvider with a
// providers map) or the legacy single-provider shape (provider with flat
// transport and model fields), so existing sidecar.json files keep loading.
func (r *ReasoningConfig) UnmarshalJSON(data []byte) error {
	var probe struct {
		ActiveProvider string          `json:"activeProvider"`
		Providers      json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return err
	}
	if probe.ActiveProvider != "" || probe.Providers != nil {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		var modern struct {
			ActiveProvider string                    `json:"activeProvider"`
			Providers      map[string]ProviderConfig `json:"providers"`
		}
		if err := decoder.Decode(&modern); err != nil {
			return err
		}
		r.ActiveProvider = modern.ActiveProvider
		r.Providers = modern.Providers
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var legacy struct {
		Provider      string      `json:"provider"`
		Executable    string      `json:"executable"`
		Endpoint      string      `json:"endpoint"`
		MaxRetries    int         `json:"maxRetries"`
		TimeoutMS     int         `json:"timeoutMs"`
		Planning      ModelConfig `json:"planning"`
		Evaluation    ModelConfig `json:"evaluation"`
		SemanticEvent ModelConfig `json:"semanticEvent"`
		AIDetection   ModelConfig `json:"aiDetection"`
	}
	if err := decoder.Decode(&legacy); err != nil {
		return err
	}
	r.ActiveProvider = legacy.Provider
	r.Providers = map[string]ProviderConfig{
		legacy.Provider: {
			Executable:    legacy.Executable,
			Endpoint:      legacy.Endpoint,
			MaxRetries:    legacy.MaxRetries,
			TimeoutMS:     legacy.TimeoutMS,
			Planning:      legacy.Planning,
			Evaluation:    legacy.Evaluation,
			SemanticEvent: legacy.SemanticEvent,
			AIDetection:   legacy.AIDetection,
		},
	}
	return nil
}
