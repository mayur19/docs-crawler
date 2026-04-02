package scope_test

import (
	"net/url"
	"testing"

	"github.com/napkin/docs-crawler/internal/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestScope_IsAllowed(t *testing.T) {
	tests := []struct {
		name     string
		cfg      scope.ScopeConfig
		url      string
		depth    int
		expected bool
	}{
		{
			name:     "empty config allows everything",
			cfg:      scope.ScopeConfig{},
			url:      "https://example.com/anything",
			depth:    100,
			expected: true,
		},
		{
			name: "prefix match allows",
			cfg: scope.ScopeConfig{
				Prefix: "https://docs.example.com/api",
			},
			url:      "https://docs.example.com/api/auth",
			depth:    0,
			expected: true,
		},
		{
			name: "prefix mismatch rejects",
			cfg: scope.ScopeConfig{
				Prefix: "https://docs.example.com/api",
			},
			url:      "https://docs.example.com/guide/start",
			depth:    0,
			expected: false,
		},
		{
			name: "max depth allows at limit",
			cfg: scope.ScopeConfig{
				MaxDepth: 3,
			},
			url:      "https://example.com/page",
			depth:    3,
			expected: true,
		},
		{
			name: "max depth rejects over limit",
			cfg: scope.ScopeConfig{
				MaxDepth: 3,
			},
			url:      "https://example.com/page",
			depth:    4,
			expected: false,
		},
		{
			name: "max depth zero means unlimited",
			cfg: scope.ScopeConfig{
				MaxDepth: 0,
			},
			url:      "https://example.com/page",
			depth:    999,
			expected: true,
		},
		{
			name: "same domain allows matching host",
			cfg: scope.ScopeConfig{
				Prefix:     "https://docs.example.com",
				SameDomain: true,
			},
			url:      "https://docs.example.com/page",
			depth:    0,
			expected: true,
		},
		{
			name: "same domain rejects different host",
			cfg: scope.ScopeConfig{
				Prefix:     "https://docs.example.com",
				SameDomain: true,
			},
			url:      "https://other.example.com/page",
			depth:    0,
			expected: false,
		},
		{
			name: "same domain case insensitive",
			cfg: scope.ScopeConfig{
				Prefix:     "https://DOCS.Example.COM",
				SameDomain: true,
			},
			url:      "https://docs.example.com/page",
			depth:    0,
			expected: true,
		},
		{
			name: "include pattern matches",
			cfg: scope.ScopeConfig{
				Includes: []string{"/api/*"},
			},
			url:      "https://example.com/api/auth",
			depth:    0,
			expected: true,
		},
		{
			name: "include pattern rejects non-match",
			cfg: scope.ScopeConfig{
				Includes: []string{"/api/*"},
			},
			url:      "https://example.com/guide/start",
			depth:    0,
			expected: false,
		},
		{
			name: "multiple includes any match allows",
			cfg: scope.ScopeConfig{
				Includes: []string{"/api/*", "/guide/*"},
			},
			url:      "https://example.com/guide/start",
			depth:    0,
			expected: true,
		},
		{
			name: "exclude pattern rejects match",
			cfg: scope.ScopeConfig{
				Excludes: []string{"/internal/*"},
			},
			url:      "https://example.com/internal/debug",
			depth:    0,
			expected: false,
		},
		{
			name: "exclude pattern allows non-match",
			cfg: scope.ScopeConfig{
				Excludes: []string{"/internal/*"},
			},
			url:      "https://example.com/api/auth",
			depth:    0,
			expected: true,
		},
		{
			name: "combined rules all pass",
			cfg: scope.ScopeConfig{
				Prefix:     "https://docs.example.com",
				Includes:   []string{"/api/*"},
				Excludes:   []string{"/api/internal"},
				MaxDepth:   5,
				SameDomain: true,
			},
			url:      "https://docs.example.com/api/auth",
			depth:    2,
			expected: true,
		},
		{
			name: "combined rules depth fails",
			cfg: scope.ScopeConfig{
				Prefix:   "https://docs.example.com",
				MaxDepth: 2,
			},
			url:      "https://docs.example.com/page",
			depth:    3,
			expected: false,
		},
		{
			name: "combined rules exclude overrides include",
			cfg: scope.ScopeConfig{
				Includes: []string{"/api/*"},
				Excludes: []string{"/api/internal"},
			},
			url:      "https://example.com/api/internal",
			depth:    0,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := scope.NewScope(tt.cfg)
			u := mustParseURL(t, tt.url)

			got := s.IsAllowed(u, tt.depth)
			assert.Equal(t, tt.expected, got)
		})
	}
}
