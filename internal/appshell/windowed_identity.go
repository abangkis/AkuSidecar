package appshell

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
)

// ReadWindowedIdentity reads only browser-supplied UA data from the exact
// capture host. It does not navigate, inspect a source tab or retain its URL.
func ReadWindowedIdentity(ctx context.Context, protocol CaptureProtocol, hostURL string) (json.RawMessage, error) {
	if protocol == nil || hostURL == "" {
		return nil, errors.New("Windowed identity host unavailable")
	}
	targetID, err := findCaptureHost(ctx, protocol, hostURL)
	if err != nil {
		return nil, err
	}
	rawVersion, err := protocol.Call(ctx, "Browser.getVersion", nil, "")
	var version struct {
		Product string `json:"product"`
	}
	if err != nil || json.Unmarshal(rawVersion, &version) != nil || len(version.Product) == 0 || len(version.Product) > 64 {
		return nil, errors.New("Windowed browser version unavailable")
	}
	expectedURL, _ := json.Marshal(hostURL)
	expression := `(async () => {
	  if (location.href !== ` + string(expectedURL) + ` || !navigator.userAgentData) return null;
	  const data = await navigator.userAgentData.getHighEntropyValues([
	    'architecture', 'bitness', 'fullVersionList', 'model', 'platformVersion', 'wow64', 'formFactors',
	  ]);
	  if (location.href !== ` + string(expectedURL) + `) return null;
	  return { userAgent: navigator.userAgent, userAgentMetadata: data };
	})()`
	var identity struct {
		Product           string          `json:"product"`
		UserAgent         string          `json:"userAgent"`
		UserAgentMetadata json.RawMessage `json:"userAgentMetadata"`
	}
	err = withCaptureHost(ctx, protocol, targetID, func(sessionID string) error {
		raw, callErr := protocol.Call(ctx, "Runtime.evaluate", map[string]any{
			"expression": expression, "awaitPromise": true, "returnByValue": true,
		}, sessionID)
		var evaluation struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
			Exception json.RawMessage `json:"exceptionDetails"`
		}
		if callErr != nil || len(raw) > 16<<10 || json.Unmarshal(raw, &evaluation) != nil ||
			len(evaluation.Exception) != 0 || json.Unmarshal(evaluation.Result.Value, &identity) != nil ||
			identity.UserAgent == "" || len(identity.UserAgent) > 1024 ||
			strings.Contains(strings.ToLower(identity.UserAgent), "headlesschrome/") ||
			len(identity.UserAgentMetadata) == 0 || identity.UserAgentMetadata[0] != '{' {
			return errors.New("Windowed User-Agent or Client Hints unavailable")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	identity.Product = version.Product
	return json.Marshal(identity)
}
