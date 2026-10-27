package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"

	"github.com/abangkis/AkuSidecar/internal/engine"
)

// Consent has a deliberately small exact shape. Generic JSON decoding permits
// duplicate fields and trailing values, which are ambiguous for a one-use grant.
func readXManualStart(r *http.Request) (engine.XManualStartRequest, error) {
	v, err := readXConsent(r, "allowOneSessionDerivation")
	return engine.XManualStartRequest{AcknowledgeLimitations: v["acknowledgeLimitations"], AllowOneSessionDerivation: v["allowOneSessionDerivation"]}, err
}
func readXDailyStart(r *http.Request) (engine.XDailyStartRequest, error) {
	v, err := readXConsent(r, "allowProcessSessionDerivation")
	return engine.XDailyStartRequest{AcknowledgeLimitations: v["acknowledgeLimitations"], AllowProcessSessionDerivation: v["allowProcessSessionDerivation"]}, err
}
func readXConsent(r *http.Request, permission string) (map[string]bool, error) {
	out := map[string]bool{}
	defer r.Body.Close()
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || kind != "application/json" {
		return out, apiError{Status: 415, Code: "unsupported_media_type", Message: "Content-Type must be application/json"}
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1025))
	if err != nil || len(raw) > 1024 {
		return out, badRequest("manual X consent is invalid")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return out, badRequest("manual X consent is invalid")
	}
	seen := map[string]bool{}
	for d.More() {
		token, err = d.Token()
		key, ok := token.(string)
		if err != nil || !ok || seen[key] || key != "acknowledgeLimitations" && key != permission {
			return out, badRequest("manual X consent is invalid")
		}
		seen[key] = true
		var value bool
		if d.Decode(&value) != nil {
			return out, badRequest("manual X consent is invalid")
		}
		out[key] = value
	}
	if _, err = d.Token(); err != nil || len(seen) != 2 {
		return out, badRequest("manual X consent is invalid")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return out, badRequest("manual X consent is invalid")
	}
	return out, nil
}
