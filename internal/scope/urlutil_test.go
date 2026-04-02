package scope_test

import (
	"net/url"
	"testing"

	"github.com/napkin/docs-crawler/internal/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "lowercase host",
			input:    "https://DOCS.Example.COM/path",
			expected: "https://docs.example.com/path",
		},
		{
			name:     "remove fragment",
			input:    "https://docs.example.com/path#section",
			expected: "https://docs.example.com/path",
		},
		{
			name:     "remove trailing slash",
			input:    "https://docs.example.com/path/",
			expected: "https://docs.example.com/path",
		},
		{
			name:     "sort query parameters",
			input:    "https://docs.example.com/search?z=1&a=2",
			expected: "https://docs.example.com/search?a=2&z=1",
		},
		{
			name:     "root path stays as slash",
			input:    "https://docs.example.com",
			expected: "https://docs.example.com/",
		},
		{
			name:     "root with trailing slash",
			input:    "https://docs.example.com/",
			expected: "https://docs.example.com/",
		},
		{
			name:     "combined normalization",
			input:    "HTTPS://Docs.Example.COM/API/Auth/?z=1&a=2#top",
			expected: "https://docs.example.com/API/Auth?a=2&z=1",
		},
		{
			name:     "empty query string removed",
			input:    "https://docs.example.com/path?",
			expected: "https://docs.example.com/path",
		},
		{
			name:     "multiple query values for same key",
			input:    "https://docs.example.com/path?a=2&a=1",
			expected: "https://docs.example.com/path?a=1&a=2",
		},
		{
			name:     "scheme lowercased",
			input:    "HTTP://docs.example.com/path",
			expected: "http://docs.example.com/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.input)
			require.NoError(t, err)

			got := scope.NormalizeURL(u)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestURLToFilepath(t *testing.T) {
	tests := []struct {
		name     string
		baseURL  string
		pageURL  string
		expected string
	}{
		{
			name:     "simple path",
			baseURL:  "https://docs.example.com",
			pageURL:  "https://docs.example.com/api/auth",
			expected: "api/auth",
		},
		{
			name:     "root path returns index",
			baseURL:  "https://docs.example.com",
			pageURL:  "https://docs.example.com/",
			expected: "index",
		},
		{
			name:     "root without slash returns index",
			baseURL:  "https://docs.example.com",
			pageURL:  "https://docs.example.com",
			expected: "index",
		},
		{
			name:     "trailing slash appends index",
			baseURL:  "https://docs.example.com",
			pageURL:  "https://docs.example.com/api/",
			expected: "api/index",
		},
		{
			name:     "base with path prefix",
			baseURL:  "https://example.com/docs",
			pageURL:  "https://example.com/docs/guide/start",
			expected: "guide/start",
		},
		{
			name:     "deeply nested path",
			baseURL:  "https://docs.example.com",
			pageURL:  "https://docs.example.com/a/b/c/d",
			expected: "a/b/c/d",
		},
		{
			name:     "base with trailing slash",
			baseURL:  "https://docs.example.com/",
			pageURL:  "https://docs.example.com/api/auth",
			expected: "api/auth",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := scope.URLToFilepath(tt.baseURL, tt.pageURL)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestURLToFilepath_Errors(t *testing.T) {
	_, err := scope.URLToFilepath("://bad-url", "https://example.com")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing base URL")

	_, err = scope.URLToFilepath("https://example.com", "://bad-url")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing page URL")
}
