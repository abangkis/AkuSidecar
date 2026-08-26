package domain

import (
	"net/url"
	"regexp"
	"strings"
)

var linkedInNativeIdentityPattern = regexp.MustCompile(`(?i)(activity|ugcpost|share)(?::|-)(\d+)`)
var xNativeStatusPathPattern = regexp.MustCompile(`^/([^/]+)/status/(\d+)(?:/.*)?$`)
var instagramNativeIdentityPattern = regexp.MustCompile(`(?i)/(p|reel|tv)/([a-z0-9_-]+)`)
var instagramNormalizedIdentityPattern = regexp.MustCompile(`(?i)^instagram:(p|reel|tv):([a-z0-9_-]+)$`)

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
		if host == "x.com" {
			match := xNativeStatusPathPattern.FindStringSubmatch(parsed.Path)
			if len(match) == 3 {
				parsed.Path = "/" + match[1] + "/status/" + match[2]
				parsed.RawPath = ""
				parsed.RawQuery = ""
				parsed.Fragment = ""
				valid = true
			}
		}
	case SourceLinkedIn:
		valid = host == "www.linkedin.com" && (strings.Contains(parsed.Path, "/posts/") || strings.Contains(parsed.Path, "/feed/update/"))
	case SourceFacebook:
		valid = (host == "www.facebook.com" || host == "facebook.com" || host == "m.facebook.com") && facebookNativePostPath(parsed.Path, parsed.Query())
	case SourceInstagram:
		valid = (host == "www.instagram.com" || host == "instagram.com") && instagramNativePostPath(parsed.Path)
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
	case SourceLinkedIn, SourceInstagram:
		return NormalizeNativeIdentity(source, canonical)
	}
	return ""
}

// NormalizeNativeIdentity maps equivalent source-native identity spellings to
// one representation while leaving unknown source formats opaque.
func NormalizeNativeIdentity(source Source, raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	value := strings.ToLower(trimmed)
	if source == SourceLinkedIn {
		match := linkedInNativeIdentityPattern.FindStringSubmatch(value)
		if len(match) == 3 {
			return "linkedin:" + strings.ToLower(match[1]) + ":" + match[2]
		}
	}
	if source == SourceInstagram {
		match := instagramNativeIdentityPattern.FindStringSubmatch(trimmed)
		if len(match) == 3 {
			return "instagram:" + strings.ToLower(match[1]) + ":" + match[2]
		}
		match = instagramNormalizedIdentityPattern.FindStringSubmatch(trimmed)
		if len(match) == 3 {
			return "instagram:" + strings.ToLower(match[1]) + ":" + match[2]
		}
	}
	return value
}

func instagramNativePostPath(path string) bool {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || (parts[0] != "p" && parts[0] != "reel" && parts[0] != "tv") || parts[1] == "" {
		return false
	}
	for _, character := range parts[1] {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' || character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
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
