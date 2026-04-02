package scope

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/temoto/robotstxt"
)

// RobotsChecker wraps a parsed robots.txt and a user-agent string to provide
// URL-level allow/disallow decisions and crawl-delay information.
type RobotsChecker struct {
	group     *robotstxt.Group
	userAgent string
}

// NewRobotsChecker parses the given robots.txt data and returns a checker
// scoped to the provided user agent.
func NewRobotsChecker(robotsData []byte, userAgent string) (RobotsChecker, error) {
	robots, err := robotstxt.FromBytes(robotsData)
	if err != nil {
		return RobotsChecker{}, fmt.Errorf("parsing robots.txt: %w", err)
	}

	group := robots.FindGroup(userAgent)

	return RobotsChecker{
		group:     group,
		userAgent: userAgent,
	}, nil
}

// IsAllowed returns true if the robots.txt rules permit the given URL to be
// crawled by the configured user agent.
func (r RobotsChecker) IsAllowed(u *url.URL) bool {
	if r.group == nil {
		return true
	}
	return r.group.Test(u.Path)
}

// CrawlDelay returns the crawl-delay directive for the configured user agent.
// If no delay is specified the returned duration is zero.
func (r RobotsChecker) CrawlDelay() time.Duration {
	if r.group == nil {
		return 0
	}
	return r.group.CrawlDelay
}

// FetchRobots retrieves the robots.txt file from the root of the given base
// URL. A 404 response is treated as an empty (permissive) robots.txt; other
// HTTP errors are returned as errors.
func FetchRobots(ctx context.Context, baseURL *url.URL, client *http.Client) ([]byte, error) {
	robotsURL := &url.URL{
		Scheme: baseURL.Scheme,
		Host:   baseURL.Host,
		Path:   "/robots.txt",
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, robotsURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating robots.txt request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching robots.txt from %s: %w", robotsURL.String(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []byte{}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"unexpected status %d fetching robots.txt from %s",
			resp.StatusCode,
			robotsURL.String(),
		)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading robots.txt body: %w", err)
	}

	return body, nil
}
