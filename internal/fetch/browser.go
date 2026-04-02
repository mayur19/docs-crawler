package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/ratelimit"
)

const (
	// defaultPageTimeout is the maximum time to wait for a page to load.
	defaultPageTimeout = 30 * time.Second
)

// BrowserFetcher implements pipeline.Fetcher using a headless Chrome browser
// via go-rod. The browser is lazily initialized on the first Fetch call.
type BrowserFetcher struct {
	limiter   *ratelimit.AdaptiveLimiter
	userAgent string
	logger    *slog.Logger

	mu      sync.Mutex
	browser *rod.Browser
}

// NewBrowserFetcher creates a new BrowserFetcher. The browser instance is
// not launched until the first Fetch call.
func NewBrowserFetcher(
	limiter *ratelimit.AdaptiveLimiter,
	userAgent string,
) *BrowserFetcher {
	return &BrowserFetcher{
		limiter:   limiter,
		userAgent: userAgent,
		logger:    slog.Default(),
	}
}

// Name returns the fetcher identifier.
func (f *BrowserFetcher) Name() string {
	return "browser"
}

// CanFetch always returns true because the browser fetcher can handle any URL.
func (f *BrowserFetcher) CanFetch(_ pipeline.CrawlURL) bool {
	return true
}

// Fetch navigates to the URL using headless Chrome, waits for the page to
// become idle, and returns the rendered HTML.
func (f *BrowserFetcher) Fetch(ctx context.Context, u pipeline.CrawlURL) (pipeline.FetchResult, error) {
	if err := f.limiter.Wait(ctx); err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("browser fetch: rate limit: %w", err)
	}

	browser, err := f.ensureBrowser()
	if err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("browser fetch: launch browser: %w", err)
	}

	page, err := browser.Page(proto.TargetCreateTarget{URL: ""})
	if err != nil {
		return pipeline.FetchResult{}, fmt.Errorf("browser fetch: create page: %w", err)
	}
	defer page.Close()

	html, err := f.navigateAndRender(ctx, page, u.URL.String())
	if err != nil {
		return pipeline.FetchResult{}, err
	}

	headers := http.Header{"Content-Type": {"text/html; charset=utf-8"}}

	return pipeline.NewFetchResult(
		u,
		http.StatusOK,
		headers,
		[]byte(html),
		"text/html; charset=utf-8",
	), nil
}

// Close shuts down the browser instance if it was launched.
func (f *BrowserFetcher) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.browser == nil {
		return nil
	}

	err := f.browser.Close()
	f.browser = nil

	if err != nil {
		return fmt.Errorf("browser fetch: close browser: %w", err)
	}

	return nil
}

// ensureBrowser lazily initializes the headless Chrome browser.
func (f *BrowserFetcher) ensureBrowser() (*rod.Browser, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.browser != nil {
		return f.browser, nil
	}

	controlURL, err := launcher.New().
		Headless(true).
		Launch()
	if err != nil {
		return nil, fmt.Errorf("launch chrome: %w", err)
	}

	browser := rod.New().ControlURL(controlURL)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connect to chrome: %w", err)
	}

	if f.userAgent != "" {
		if err := browser.SetCookies(nil); err != nil {
			f.logger.Warn("failed to clear cookies", "error", err)
		}
	}

	f.browser = browser
	f.logger.Info("browser launched for fetching")

	return f.browser, nil
}

// navigateAndRender loads the URL in the page and returns the rendered HTML.
func (f *BrowserFetcher) navigateAndRender(
	ctx context.Context,
	page *rod.Page,
	targetURL string,
) (string, error) {
	page = page.Context(ctx).Timeout(defaultPageTimeout)

	if f.userAgent != "" {
		if err := page.SetUserAgent(&proto.NetworkSetUserAgentOverride{
			UserAgent: f.userAgent,
		}); err != nil {
			return "", fmt.Errorf("browser fetch: set user agent: %w", err)
		}
	}

	if err := page.Navigate(targetURL); err != nil {
		return "", fmt.Errorf("browser fetch: navigate to %s: %w", targetURL, err)
	}

	if err := page.WaitDOMStable(time.Second, 0.1); err != nil {
		f.logger.Warn("DOM not fully stable, proceeding with current state",
			"url", targetURL,
			"error", err,
		)
	}

	html, err := page.HTML()
	if err != nil {
		return "", fmt.Errorf("browser fetch: get HTML from %s: %w", targetURL, err)
	}

	return html, nil
}
