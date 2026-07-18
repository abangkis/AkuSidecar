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
	valid := false
	switch source {
	case SourceX:
		valid = host == "x.com" && strings.Contains(parsed.Path, "/status/")
	case SourceLinkedIn:
		valid = host == "www.linkedin.com" && (strings.Contains(parsed.Path, "/posts/") || strings.Contains(parsed.Path, "/feed/update/"))
	case SourceFacebook:
		valid = (host == "www.facebook.com" || host == "facebook.com" || host == "m.facebook.com") && facebookNativePostPath(parsed.Path, parsed.Query())
	}
	if !valid {
		return "", false
	}
	return parsed.String(), true
}

func facebookNativePostPath(path string, query url.Values) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "/posts/") || strings.Contains(path, "/permalink/") ||
		strings.Contains(path, "/story.php") && query.Get("story_fbid") != "" ||
		strings.Contains(path, "/photo") && (query.Get("fbid") != "" || query.Get("photo_id") != "") ||
		strings.Contains(path, "/videos/") || strings.Contains(path, "/reel/")
}
