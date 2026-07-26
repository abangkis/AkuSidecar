package domain

import (
	"net/url"
	"regexp"
	"strings"
)

var linkedInNativeIdentityPattern = regexp.MustCompile(`(?i)(activity|ugcpost|share)(?::|-)(\d+)`)

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

// NativeIdentityFromPermalink returns a source-owned platform identity only
// when the canonical native URL itself proves that identity. Sources without
// such a reversible URL contract deliberately return no inferred identity.
func NativeIdentityFromPermalink(source Source, raw string) string {
	canonical, ok := CanonicalSourceURL(source, raw)
	if !ok {
		return ""
	}
	switch source {
	case SourceLinkedIn:
		return NormalizeNativeIdentity(source, canonical)
	}
	return ""
}

// NormalizeNativeIdentity maps equivalent source-native identity spellings to
// one representation while leaving unknown source formats opaque.
func NormalizeNativeIdentity(source Source, raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	if value == "" {
		return ""
	}
	if source == SourceLinkedIn {
		match := linkedInNativeIdentityPattern.FindStringSubmatch(value)
		if len(match) == 3 {
			return "linkedin:" + strings.ToLower(match[1]) + ":" + match[2]
		}
	}
	return value
}

func facebookNativePostPath(path string, query url.Values) bool {
	path = strings.ToLower(path)
	return strings.Contains(path, "/posts/") || strings.Contains(path, "/permalink/") ||
		strings.Contains(path, "/story.php") && query.Get("story_fbid") != "" ||
		strings.Contains(path, "/photo") && (query.Get("fbid") != "" || query.Get("photo_id") != "") ||
		strings.Contains(path, "/videos/") || facebookNumericReelPath(path)
}

func facebookNumericReelPath(path string) bool {
	value := strings.Trim(strings.TrimPrefix(path, "/reel/"), "/")
	if value == "" || strings.Contains(value, "/") {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}
