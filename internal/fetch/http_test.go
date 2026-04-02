package fetch

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/ratelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLimiter() *ratelimit.AdaptiveLimiter {
	return ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{ExplicitRate: 1000})
}

func newTestCrawlURL(t *testing.T, rawURL string) pipeline.CrawlURL {
	t.Helper()
	parsed, err := url.Parse(rawURL)
	require.NoError(t, err)
	return pipeline.NewCrawlURL(parsed, 0, pipeline.SourceSeed, "test")
}

func TestHTTPFetcher_Name(t *testing.T) {
	t.Parallel()

	fetcher := NewHTTPFetcher(http.DefaultClient, newTestLimiter(), "test-agent")
	assert.Equal(t, "http", fetcher.Name())
}

func TestHTTPFetcher_CanFetch(t *testing.T) {
	t.Parallel()

	fetcher := NewHTTPFetcher(http.DefaultClient, newTestLimiter(), "test-agent")
	crawlURL := newTestCrawlURL(t, "https://example.com")
	assert.True(t, fetcher.CanFetch(crawlURL))
}

func TestHTTPFetcher_Fetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		handler     http.HandlerFunc
		wantErr     bool
		errContains string
		checkResult func(t *testing.T, result pipeline.FetchResult)
	}{
		{
			name: "successful fetch returns body and headers",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusOK)
				fmt.Fprint(w, "<html><body>Hello</body></html>")
			},
			checkResult: func(t *testing.T, result pipeline.FetchResult) {
				t.Helper()
				assert.Equal(t, http.StatusOK, result.StatusCode)
				assert.Equal(t, "text/html; charset=utf-8", result.ContentType)
				assert.Equal(t, "<html><body>Hello</body></html>", string(result.Body))
				assert.NotZero(t, result.FetchedAt)
			},
		},
		{
			name: "429 response returns error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Retry-After", "5")
				w.WriteHeader(http.StatusTooManyRequests)
				fmt.Fprint(w, "rate limited")
			},
			wantErr:     true,
			errContains: "rate limit",
		},
		{
			name: "500 response returns retriable error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprint(w, "internal error")
			},
			wantErr:     true,
			errContains: "all 3 attempts failed",
		},
		{
			name: "404 response is not retried",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, "not found")
			},
			checkResult: func(t *testing.T, result pipeline.FetchResult) {
				t.Helper()
				assert.Equal(t, http.StatusNotFound, result.StatusCode)
				assert.Equal(t, "not found", string(result.Body))
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			fetcher := NewHTTPFetcher(srv.Client(), newTestLimiter(), "test-agent")
			crawlURL := newTestCrawlURL(t, srv.URL+"/page")

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()

			result, err := fetcher.Fetch(ctx, crawlURL)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.errContains)
				return
			}

			require.NoError(t, err)
			if tc.checkResult != nil {
				tc.checkResult(t, result)
			}
		})
	}
}

func TestHTTPFetcher_Fetch_UserAgentHeader(t *testing.T) {
	t.Parallel()

	const expectedUA = "DocsBot/1.0"
	var receivedUA string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(srv.Client(), newTestLimiter(), expectedUA)
	crawlURL := newTestCrawlURL(t, srv.URL+"/page")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := fetcher.Fetch(ctx, crawlURL)
	require.NoError(t, err)
	assert.Equal(t, expectedUA, receivedUA)
}

func TestHTTPFetcher_Fetch_BodySizeLimit(t *testing.T) {
	t.Parallel()

	oversizedBody := strings.Repeat("x", maxBodySize+100)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, oversizedBody)
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(srv.Client(), newTestLimiter(), "test-agent")
	crawlURL := newTestCrawlURL(t, srv.URL+"/large")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := fetcher.Fetch(ctx, crawlURL)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "exceeds maximum size")
}

func TestHTTPFetcher_Fetch_RequestTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(srv.Client(), newTestLimiter(), "test-agent")
	crawlURL := newTestCrawlURL(t, srv.URL+"/slow")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := fetcher.Fetch(ctx, crawlURL)
	require.Error(t, err)
}

func TestHTTPFetcher_Fetch_RetrySucceedsOnThirdAttempt(t *testing.T) {
	t.Parallel()

	attemptCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attemptCount++
		if attemptCount < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Fprint(w, "error")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "success")
	}))
	defer srv.Close()

	fetcher := NewHTTPFetcher(srv.Client(), newTestLimiter(), "test-agent")
	crawlURL := newTestCrawlURL(t, srv.URL+"/flaky")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := fetcher.Fetch(ctx, crawlURL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, result.StatusCode)
	assert.Equal(t, "success", string(result.Body))
	assert.Equal(t, 3, attemptCount)
}

func TestHTTPFetcher_Fetch_RateLimitHeadersUpdated(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Limit", "100")
		w.Header().Set("X-RateLimit-Remaining", "50")
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	limiter := newTestLimiter()
	fetcher := NewHTTPFetcher(srv.Client(), limiter, "test-agent")
	crawlURL := newTestCrawlURL(t, srv.URL+"/page")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := fetcher.Fetch(ctx, crawlURL)
	require.NoError(t, err)

	// The limiter has ExplicitRate set, so headers won't override it,
	// but the call to UpdateFromHeaders should not panic.
}

func TestRetriableError(t *testing.T) {
	t.Parallel()

	t.Run("with cause", func(t *testing.T) {
		t.Parallel()
		cause := fmt.Errorf("connection refused")
		err := &RetriableError{StatusCode: 503, Cause: cause}

		assert.Contains(t, err.Error(), "retriable error")
		assert.Contains(t, err.Error(), "503")
		assert.Contains(t, err.Error(), "connection refused")
		assert.ErrorIs(t, err, cause)
	})

	t.Run("without cause", func(t *testing.T) {
		t.Parallel()
		err := &RetriableError{StatusCode: 429}

		assert.Contains(t, err.Error(), "retriable error")
		assert.Contains(t, err.Error(), "429")
	})
}

func TestIsRetriable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "RetriableError is retriable",
			err:      &RetriableError{StatusCode: 500},
			expected: true,
		},
		{
			name:     "wrapped RetriableError is retriable",
			err:      fmt.Errorf("outer: %w", &RetriableError{StatusCode: 429}),
			expected: true,
		},
		{
			name:     "regular error is not retriable",
			err:      fmt.Errorf("some error"),
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expected, isRetriable(tc.err))
		})
	}
}

func TestComputeBackoff(t *testing.T) {
	t.Parallel()

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{attempt: 1, expected: 500 * time.Millisecond},
		{attempt: 2, expected: 1000 * time.Millisecond},
		{attempt: 3, expected: 2000 * time.Millisecond},
	}

	for _, tc := range tests {
		name := fmt.Sprintf("attempt_%d", tc.attempt)
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			result := computeBackoff(tc.attempt)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestReadLimitedBody(t *testing.T) {
	t.Parallel()

	t.Run("reads body within limit", func(t *testing.T) {
		t.Parallel()
		body := strings.NewReader("hello world")
		data, err := readLimitedBody(body)
		require.NoError(t, err)
		assert.Equal(t, "hello world", string(data))
	})

	t.Run("rejects body exceeding limit", func(t *testing.T) {
		t.Parallel()
		oversized := strings.NewReader(strings.Repeat("x", maxBodySize+1))
		_, err := readLimitedBody(oversized)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum size")
	})
}
