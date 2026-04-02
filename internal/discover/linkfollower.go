package discover

import (
	"bytes"
	"context"
	"log/slog"
	"net/url"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/scope"
	urlutil "github.com/napkin/docs-crawler/internal/scope"
)

// LinkFollower discovers URLs by extracting <a href> links from fetched pages.
// It implements pipeline.Discoverer for the initial seed and provides Feed/Close
// for ongoing link extraction.
type LinkFollower struct {
	scope scope.Scope

	mu   sync.Mutex
	ch   chan pipeline.CrawlURL
	seen map[string]bool
}

// NewLinkFollower creates a LinkFollower that filters discovered links through
// the given scope.
func NewLinkFollower(s scope.Scope) *LinkFollower {
	return &LinkFollower{
		scope: s,
		ch:    make(chan pipeline.CrawlURL, 64),
		seen:  make(map[string]bool),
	}
}

// Name returns the discoverer identifier.
func (lf *LinkFollower) Name() string {
	return "link-follower"
}

// Discover emits the seed URL and keeps the channel open for subsequent
// Feed calls. The caller must eventually call Close to release the channel.
func (lf *LinkFollower) Discover(_ context.Context, seed *url.URL) (<-chan pipeline.CrawlURL, error) {
	normalized := urlutil.NormalizeURL(seed)

	lf.mu.Lock()
	lf.seen[normalized] = true
	lf.mu.Unlock()

	crawlURL := pipeline.NewCrawlURL(seed, 0, pipeline.SourceSeed, "link-follower")
	lf.ch <- crawlURL

	return lf.ch, nil
}

// Feed extracts links from a fetch result and emits new, unseen, in-scope URLs
// on the discovery channel. It is safe to call from multiple goroutines.
// Returns the number of new URLs emitted.
func (lf *LinkFollower) Feed(result pipeline.FetchResult) int {
	links := extractLinks(result.CrawlURL.URL, result.Body)
	parentDepth := result.CrawlURL.Depth

	lf.mu.Lock()
	defer lf.mu.Unlock()

	emitted := 0
	for _, link := range links {
		normalized := urlutil.NormalizeURL(link)

		if lf.seen[normalized] {
			continue
		}
		lf.seen[normalized] = true

		if !lf.scope.IsAllowed(link, parentDepth+1) {
			slog.Debug("link filtered by scope", "url", link.String(), "depth", parentDepth+1)
			continue
		}

		crawlURL := pipeline.NewCrawlURL(link, parentDepth+1, pipeline.SourceLink, "link-follower")
		lf.ch <- crawlURL
		emitted++
	}
	return emitted
}

// Close closes the discovery channel. It must be called exactly once when no
// more Feed calls will be made.
func (lf *LinkFollower) Close() {
	close(lf.ch)
}

// extractLinks parses HTML and returns all valid, absolute <a href> URLs.
func extractLinks(base *url.URL, body []byte) []*url.URL {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		slog.Debug("failed to parse HTML for link extraction", "error", err)
		return nil
	}

	var links []*url.URL
	doc.Find("a[href]").Each(func(_ int, sel *goquery.Selection) {
		href, exists := sel.Attr("href")
		if !exists || href == "" {
			return
		}

		parsed, err := url.Parse(href)
		if err != nil {
			return
		}

		resolved := base.ResolveReference(parsed)

		// Only keep http/https links.
		if resolved.Scheme != "http" && resolved.Scheme != "https" {
			return
		}

		// Strip fragment.
		resolved.Fragment = ""

		links = append(links, resolved)
	})

	return links
}
