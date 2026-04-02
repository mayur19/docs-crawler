package fetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/ratelimit"
)

const (
	// maxBodySize is the maximum response body size (10 MB) to prevent OOM.
	maxBodySize = 10 * 1024 * 1024

	// maxRetries is the maximum number of fetch attempts for retriable errors.
	maxRetries = 3

	// baseBackoff is the initial backoff duration for exponential retry.
	baseBackoff = 500 * time.Millisecond
)

// RetriableError represents an error that should be retried.
type RetriableError struct {
	StatusCode int
	Cause      error
}

func (e *RetriableError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("retriable error (status %d): %s", e.StatusCode, e.Cause.Error())
	}
	return fmt.Sprintf("retriable error (status %d)", e.StatusCode)
}

func (e *RetriableError) Unwrap() error {
	return e.Cause
}

// HTTPFetcher implements pipeline.Fetcher using a standard HTTP client
// with adaptive rate limiting and exponential backoff retry.
type HTTPFetcher struct {
	client    *http.Client
	limiter   *ratelimit.AdaptiveLimiter
	userAgent string
	logger    *slog.Logger
}

// NewHTTPFetcher creates a new HTTPFetcher.
func NewHTTPFetcher(
	client *http.Client,
	limiter *ratelimit.AdaptiveLimiter,
	userAgent string,
) *HTTPFetcher {
	return &HTTPFetcher{
		client:    client,
		limiter:   limiter,
		userAgent: userAgent,
		logger:    slog.Default(),
	}
}

// Name returns the fetcher identifier.
func (f *HTTPFetcher) Name() string {
	return "http"
}

// CanFetch always returns true because the HTTP fetcher is the default.
func (f *HTTPFetcher) CanFetch(_ pipeline.CrawlURL) bool {
	return true
}

// Fetch retrieves the page at the given URL using HTTP, with rate limiting
// and exponential backoff retry for transient errors.
func (f *HTTPFetcher) Fetch(ctx context.Context, u pipeline.CrawlURL) (pipeline.FetchResult, error) {
	var lastErr error

	for attempt := range maxRetries {
		if attempt > 0 {
			backoff := computeBackoff(attempt)
			f.logger.Info("retrying fetch",
				"url", u.URL.String(),
				"attempt", attempt+1,
				"backoff", backoff,
			)
			if err := sleepWithContext(ctx, backoff); err != nil {
				return pipeline.FetchResult{}, fmt.Errorf("fetch: retry wait cancelled: %w", err)
			}
		}

		result, err := f.doFetch(ctx, u)
		if err == nil {
			return result, nil
		}

		lastErr = err
		if !isRetriable(err) {
			return pipeline.FetchResult{}, err
		}
	}

	return pipeline.FetchResult{}, fmt.Errorf("fetch: all %d attempts failed for %s: %w", maxRetries, u.URL.String(), lastErr)
}

// doFetch performs a single HTTP fetch with rate limiting.
func (f *HTTPFetcher) doFetch(ctx context.Context, u pipeline.CrawlURL) (pipeline.FetchResult, error) {
	if err := f.limiter.Wait(ctx); err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("fetch: rate limit: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.URL.String(), nil)
	if err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("fetch: create request: %w", err)
	}

	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("fetch: execute request: %w", err)
	}
	defer resp.Body.Close()

	f.limiter.UpdateFromHeaders(resp.Header)

	if resp.StatusCode == http.StatusTooManyRequests {
		return pipeline.FetchResult{}, f.handleRateLimitResponse(resp)
	}

	if resp.StatusCode >= 500 {
		return pipeline.FetchResult{}, &RetriableError{
			StatusCode: resp.StatusCode,
			Cause:      fmt.Errorf("server error: %s", resp.Status),
		}
	}

	body, err := readLimitedBody(resp.Body)
	if err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("fetch: read body: %w", err)
	}

	contentType := resp.Header.Get("Content-Type")

	return pipeline.NewFetchResult(u, resp.StatusCode, resp.Header, body, contentType), nil
}

// handleRateLimitResponse parses a 429 response and signals the rate limiter.
func (f *HTTPFetcher) handleRateLimitResponse(resp *http.Response) error {
	info := ratelimit.ParseRateLimitHeaders(resp.Header)
	if info.RetryAfter > 0 {
		f.limiter.HandleRetryAfter(info.RetryAfter)
	}

	return &RetriableError{
		StatusCode: http.StatusTooManyRequests,
		Cause:      fmt.Errorf("rate limited (429): retry after %s", info.RetryAfter),
	}
}

// readLimitedBody reads up to maxBodySize bytes from the reader.
func readLimitedBody(r io.Reader) ([]byte, error) {
	limited := io.LimitReader(r, maxBodySize+1)

	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	if len(body) > maxBodySize {
		return nil, fmt.Errorf("response body exceeds maximum size of %d bytes", maxBodySize)
	}

	return body, nil
}

// computeBackoff returns the exponential backoff duration for the given attempt.
func computeBackoff(attempt int) time.Duration {
	return time.Duration(float64(baseBackoff) * math.Pow(2, float64(attempt-1)))
}

// sleepWithContext waits for the given duration or until the context is cancelled.
func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// isRetriable checks whether an error is a RetriableError.
func isRetriable(err error) bool {
	retriable := &RetriableError{}
	return errAs(err, &retriable)
}

// errAs is a thin wrapper around fmt.Errorf-compatible error assertion.
// It exists to keep isRetriable testable without importing errors directly.
func errAs(err error, target interface{}) bool {
	type unwrapper interface{ Unwrap() error }

	switch t := target.(type) {
	case **RetriableError:
		if re, ok := err.(*RetriableError); ok {
			*t = re
			return true
		}
		if u, ok := err.(unwrapper); ok {
			return errAs(u.Unwrap(), target)
		}
	}

	return false
}
