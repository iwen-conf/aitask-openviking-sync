package openviking

import (
	"errors"
	"net/url"
	"strings"
)

// NormalizeServerURL canonicalizes an OpenViking server URL to an API base
// (scheme+host+optional base path) and strips trailing slashes/query/fragment.
func NormalizeServerURL(raw string) (string, error) {
	candidates, err := ServerURLCandidates(raw)
	if err != nil {
		return "", err
	}
	return candidates[0], nil
}

// ServerURLCandidates returns ordered base-URL candidates for API probing.
// It keeps the user-provided root path first, and adds a compatible fallback:
// - root -> /ov-api
// - /ov-api -> root
//
// It also rewrites known console-only paths (/console...) to /ov-api.
func ServerURLCandidates(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("serverUrl cannot be empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, errors.New("serverUrl must include scheme and host")
	}

	primaryPath := canonicalServerBasePath(parsed.Path)
	pathCandidates := []string{primaryPath}
	if primaryPath == "" {
		pathCandidates = append(pathCandidates, "/ov-api")
	} else if strings.EqualFold(primaryPath, "/ov-api") {
		pathCandidates = append(pathCandidates, "")
	}

	out := make([]string, 0, len(pathCandidates))
	seen := make(map[string]struct{}, len(pathCandidates))
	for _, candidatePath := range pathCandidates {
		u := *parsed
		if candidatePath == "" {
			u.Path = ""
		} else {
			u.Path = candidatePath
		}
		u.RawPath = ""
		u.RawQuery = ""
		u.Fragment = ""

		base := strings.TrimRight(strings.TrimSpace(u.String()), "/")
		if base == "" {
			continue
		}
		if _, exists := seen[base]; exists {
			continue
		}
		seen[base] = struct{}{}
		out = append(out, base)
	}
	if len(out) == 0 {
		return nil, errors.New("serverUrl must include scheme and host")
	}
	return out, nil
}

func canonicalServerBasePath(rawPath string) string {
	clean := strings.ReplaceAll(strings.TrimSpace(rawPath), "\\", "/")
	clean = pathClean("/" + strings.Trim(clean, "/"))
	if clean == "/" {
		return ""
	}

	lower := strings.ToLower(clean)
	if lower == "/console" || strings.HasPrefix(lower, "/console/") {
		return "/ov-api"
	}

	if strings.HasSuffix(lower, "/api/v1") {
		clean = clean[:len(clean)-len("/api/v1")]
		clean = strings.TrimRight(clean, "/")
	}
	if clean == "" || clean == "/" {
		return ""
	}
	clean = pathClean(clean)
	if clean == "/" {
		return ""
	}
	return clean
}
