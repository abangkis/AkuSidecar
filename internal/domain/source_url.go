package domain

import (
	"net/url"
	"strings"
)

// CanonicalSourceURL accepts only native post permalinks owned by the captured
// source. It deliberately excludes arbitrary external references: link
// destinations are an evidence-bound host responsibility, not model output.
func CanonicalSourceURL(source Source, raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Port() != "" {
		return "", false
	}
	host := strings.ToLower(parsed.Hostname())
	valid := source == SourceX && host == "x.com" && strings.Contains(parsed.Path, "/status/") ||
		source == SourceLinkedIn && host == "www.linkedin.com" && (strings.Contains(parsed.Path, "/posts/") || strings.Contains(parsed.Path, "/feed/update/"))
	if !valid {
		return "", false
	}
	return parsed.String(), true
}
