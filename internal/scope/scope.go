package scope

import (
	"net/url"
	"path"
	"strings"
)

// ScopeConfig defines the filtering rules that control which URLs are
// allowed during a crawl.
type ScopeConfig struct {
	Prefix     string
	Includes   []string
	Excludes   []string
	MaxDepth   int
	SameDomain bool
}

// Scope applies URL filtering rules to decide whether a URL should be crawled.
// It is immutable after construction.
type Scope struct {
	prefix     *url.URL
	includes   []string
	excludes   []string
	maxDepth   int
	sameDomain bool
}

// NewScope creates a Scope from the given configuration. If the Prefix is not
// a valid URL the prefix check is effectively disabled. The prefix scheme and
// host are lowercased for consistent comparison.
func NewScope(cfg ScopeConfig) Scope {
	parsed, _ := url.Parse(cfg.Prefix)

	if parsed != nil {
		normalized := &url.URL{
			Scheme:   strings.ToLower(parsed.Scheme),
			Host:     strings.ToLower(parsed.Host),
			Path:     parsed.Path,
			RawQuery: parsed.RawQuery,
		}
		parsed = normalized
	}

	return Scope{
		prefix:     parsed,
		includes:   cfg.Includes,
		excludes:   cfg.Excludes,
		maxDepth:   cfg.MaxDepth,
		sameDomain: cfg.SameDomain,
	}
}

// IsAllowed returns true when the URL passes every configured filter.
func (s Scope) IsAllowed(u *url.URL, depth int) bool {
	if !s.checkDepth(depth) {
		return false
	}

	if !s.checkSameDomain(u) {
		return false
	}

	if !s.checkPrefix(u) {
		return false
	}

	if !s.checkIncludes(u) {
		return false
	}

	if !s.checkExcludes(u) {
		return false
	}

	return true
}

// checkDepth verifies the crawl depth does not exceed the configured maximum.
// A MaxDepth of 0 means unlimited.
func (s Scope) checkDepth(depth int) bool {
	if s.maxDepth <= 0 {
		return true
	}
	return depth <= s.maxDepth
}

// checkSameDomain ensures the URL shares the same host as the prefix when
// SameDomain is enabled.
func (s Scope) checkSameDomain(u *url.URL) bool {
	if !s.sameDomain {
		return true
	}
	if s.prefix == nil {
		return true
	}
	return strings.EqualFold(u.Host, s.prefix.Host)
}

// checkPrefix ensures the URL string starts with the configured prefix.
func (s Scope) checkPrefix(u *url.URL) bool {
	if s.prefix == nil || s.prefix.String() == "" {
		return true
	}
	return strings.HasPrefix(u.String(), s.prefix.String())
}

// checkIncludes verifies the URL path matches at least one include pattern.
// If no include patterns are configured every URL is included.
func (s Scope) checkIncludes(u *url.URL) bool {
	if len(s.includes) == 0 {
		return true
	}
	for _, pattern := range s.includes {
		if matched, _ := path.Match(pattern, u.Path); matched {
			return true
		}
	}
	return false
}

// checkExcludes rejects the URL if its path matches any exclude pattern.
func (s Scope) checkExcludes(u *url.URL) bool {
	for _, pattern := range s.excludes {
		if matched, _ := path.Match(pattern, u.Path); matched {
			return false
		}
	}
	return true
}
