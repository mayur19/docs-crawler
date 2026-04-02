package scope_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/napkin/docs-crawler/internal/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleRobotsTxt = `
User-agent: testbot
Disallow: /private/
Disallow: /admin/
Allow: /public/
Crawl-delay: 2

User-agent: *
Disallow: /secret/
`

func TestRobotsChecker_IsAllowed(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		url       string
		expected  bool
	}{
		{
			name:      "allowed path for testbot",
			userAgent: "testbot",
			url:       "https://example.com/public/page",
			expected:  true,
		},
		{
			name:      "disallowed path for testbot",
			userAgent: "testbot",
			url:       "https://example.com/private/data",
			expected:  false,
		},
		{
			name:      "admin disallowed for testbot",
			userAgent: "testbot",
			url:       "https://example.com/admin/panel",
			expected:  false,
		},
		{
			name:      "unspecified path allowed for testbot",
			userAgent: "testbot",
			url:       "https://example.com/docs/guide",
			expected:  true,
		},
		{
			name:      "secret disallowed for wildcard agent",
			userAgent: "otherbot",
			url:       "https://example.com/secret/file",
			expected:  false,
		},
		{
			name:      "non-secret allowed for wildcard agent",
			userAgent: "otherbot",
			url:       "https://example.com/public/page",
			expected:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker, err := scope.NewRobotsChecker([]byte(sampleRobotsTxt), tt.userAgent)
			require.NoError(t, err)

			u := mustParseURL(t, tt.url)
			got := checker.IsAllowed(u)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestRobotsChecker_CrawlDelay(t *testing.T) {
	tests := []struct {
		name      string
		userAgent string
		expected  time.Duration
	}{
		{
			name:      "testbot has 2 second delay",
			userAgent: "testbot",
			expected:  2 * time.Second,
		},
		{
			name:      "wildcard agent has no delay",
			userAgent: "otherbot",
			expected:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker, err := scope.NewRobotsChecker([]byte(sampleRobotsTxt), tt.userAgent)
			require.NoError(t, err)

			got := checker.CrawlDelay()
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestNewRobotsChecker_EmptyData(t *testing.T) {
	checker, err := scope.NewRobotsChecker([]byte{}, "testbot")
	require.NoError(t, err)

	u := mustParseURL(t, "https://example.com/anything")
	assert.True(t, checker.IsAllowed(u))
	assert.Equal(t, time.Duration(0), checker.CrawlDelay())
}

func TestFetchRobots_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/robots.txt", r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(sampleRobotsTxt))
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	data, err := scope.FetchRobots(context.Background(), baseURL, server.Client())
	require.NoError(t, err)
	assert.Equal(t, sampleRobotsTxt, string(data))
}

func TestFetchRobots_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	data, err := scope.FetchRobots(context.Background(), baseURL, server.Client())
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestFetchRobots_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	_, err = scope.FetchRobots(context.Background(), baseURL, server.Client())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unexpected status 500")
}

func TestFetchRobots_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	baseURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err = scope.FetchRobots(ctx, baseURL, server.Client())
	assert.Error(t, err)
}
