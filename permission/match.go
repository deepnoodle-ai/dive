package permission

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

// MatchGlob performs glob matching between a pattern and a value.
// The * wildcard matches any sequence of characters (including / and .).
// Supports:
//   - "*" matches any sequence of characters
//   - "?" matches a single character
//   - "{a,b,c}" matches any of the alternatives
//
// This is used for tool name patterns (e.g. "mcp__*") and specifier
// patterns (e.g. "go test*"). For file path matching where * should
// not cross directory boundaries, use MatchPath instead.
func MatchGlob(pattern, value string) bool {
	if pattern == value {
		return true
	}
	if pattern == "*" {
		return true
	}
	re := globToRegex(pattern, false)
	matched, err := regexp.MatchString(re, value)
	if err != nil {
		return false
	}
	return matched
}

// MatchDomain checks if a URL's host matches or is a subdomain of the given
// domain. Matching is case-insensitive and tolerates a trailing dot on either
// side (https://EXAMPLE.COM. matches example.com).
func MatchDomain(urlStr, domain string) bool {
	// Ensure the string has a scheme so url.Parse treats it as absolute.
	if !strings.Contains(urlStr, "://") {
		urlStr = "https://" + urlStr
	}
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	host := u.Hostname() // strips port and brackets for IPv6
	// For bracketed IPv6 that Hostname might not fully strip:
	host = strings.Trim(host, "[]")

	if h, _, err := net.SplitHostPort(u.Host); err == nil {
		host = strings.Trim(h, "[]")
	}

	host = strings.ToLower(strings.TrimSuffix(host, "."))
	domain = strings.ToLower(strings.TrimSuffix(domain, "."))
	if host == "" || domain == "" {
		return false
	}

	if host == domain {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}

// MatchURLSpecifier matches a URL specifier pattern against a URL. Three
// pattern forms are supported:
//   - "domain:example.com" matches the URL's host (exact or subdomain)
//   - a bare domain with no wildcards, scheme, or path (e.g. "example.com")
//     is treated the same as "domain:example.com"
//   - anything else is glob-matched against the full URL string
//
// Domain-based forms are preferred: a glob like "*example.com*" also matches
// "https://example.com.attacker.net".
func MatchURLSpecifier(pattern, urlStr string) bool {
	if domain, ok := strings.CutPrefix(pattern, "domain:"); ok {
		return MatchDomain(urlStr, domain)
	}
	if !strings.ContainsAny(pattern, "*?{") &&
		!strings.Contains(pattern, "://") &&
		!strings.Contains(pattern, "/") {
		return MatchDomain(urlStr, pattern)
	}
	return MatchGlob(pattern, urlStr)
}

// MatchPath performs glob-style matching on file paths.
// Unlike MatchGlob, * only matches within a single path segment (does not
// cross / boundaries), while ** matches across segments.
func MatchPath(pattern, path string) bool {
	if pattern == path {
		return true
	}
	re := globToRegex(pattern, true)
	matched, err := regexp.MatchString(re, path)
	if err != nil {
		return false
	}
	return matched
}

// globToRegex converts a glob pattern to a regex pattern.
// If pathMode is true, * matches [^/]* (single segment); otherwise * matches .*.
// ** always matches .* (any path).
// Handles {a,b,c} alternatives.
// The (?s) flag makes . match newlines so wildcards cannot be escaped by
// embedding a newline in the value (relevant for deny rules on commands).
func globToRegex(glob string, pathMode bool) string {
	var result strings.Builder
	result.WriteString("(?s)^")

	i := 0
	for i < len(glob) {
		c := glob[i]
		switch c {
		case '*':
			if i+1 < len(glob) && glob[i+1] == '*' {
				// ** always matches any path including separators
				result.WriteString(".*")
				i++ // Skip the second *
			} else if pathMode {
				// In path mode, * matches within a single segment
				result.WriteString("[^/]*")
			} else {
				// In glob mode, * matches anything
				result.WriteString(".*")
			}
		case '?':
			if pathMode {
				result.WriteString("[^/]")
			} else {
				result.WriteByte('.')
			}
		case '{':
			// Find closing brace and create alternation
			end := strings.IndexByte(glob[i:], '}')
			if end == -1 {
				// No closing brace, treat as literal
				result.WriteString("\\{")
			} else {
				alternatives := glob[i+1 : i+end]
				parts := strings.Split(alternatives, ",")
				result.WriteString("(?:")
				for j, part := range parts {
					if j > 0 {
						result.WriteByte('|')
					}
					result.WriteString(regexp.QuoteMeta(part))
				}
				result.WriteByte(')')
				i += end // Will be incremented at end of loop
			}
		case '.', '+', '^', '$', '|', '(', ')', '[', ']', '\\':
			result.WriteByte('\\')
			result.WriteByte(c)
		default:
			result.WriteByte(c)
		}
		i++
	}

	result.WriteString("$")
	return result.String()
}
