package fetch

import (
	"testing"

	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/ratelimit"
	"github.com/stretchr/testify/assert"
)

func TestBrowserFetcher_Name(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 100})
	fetcher := NewBrowserFetcher(limiter, "test-agent")

	assert.Equal(t, "browser", fetcher.Name())
}

func TestBrowserFetcher_CanFetch(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 100})
	fetcher := NewBrowserFetcher(limiter, "test-agent")

	crawlURL := newTestCrawlURL(t, "https://example.com/docs")
	assert.True(t, fetcher.CanFetch(crawlURL))
}

func TestBrowserFetcher_CanFetch_AnyURL(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 100})
	fetcher := NewBrowserFetcher(limiter, "test-agent")

	tests := []struct {
		name string
		url  string
	}{
		{name: "https URL", url: "https://example.com"},
		{name: "http URL", url: "http://example.com"},
		{name: "deep path", url: "https://example.com/a/b/c"},
		{name: "with query", url: "https://example.com?q=test"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			crawlURL := newTestCrawlURL(t, tc.url)
			assert.True(t, fetcher.CanFetch(crawlURL))
		})
	}
}

func TestBrowserFetcher_ImplementsFetcher(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 100})
	fetcher := NewBrowserFetcher(limiter, "test-agent")

	// Verify the BrowserFetcher satisfies the pipeline.Fetcher interface at compile time.
	var _ pipeline.Fetcher = fetcher
}

func TestBrowserFetcher_CloseWithoutLaunch(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 100})
	fetcher := NewBrowserFetcher(limiter, "test-agent")

	// Closing without ever launching the browser should not error.
	err := fetcher.Close()
	assert.NoError(t, err)
}

func TestBrowserFetcher_LazyInitialization(t *testing.T) {
	t.Parallel()

	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 100})
	fetcher := NewBrowserFetcher(limiter, "test-agent")

	// Browser should be nil before any fetch call.
	fetcher.mu.Lock()
	assert.Nil(t, fetcher.browser, "browser should not be initialized before first fetch")
	fetcher.mu.Unlock()
}
