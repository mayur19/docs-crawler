package discover

import (
	"bytes"
	"context"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/PuerkitoBio/goquery"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/scope"
	urlutil "github.com/mayur19/docs-crawler/internal/scope"
)

// LinkFollower discovers URLs by extracting <a href> links from fetched pages.
// It implements pipeline.Discoverer for the initial seed and provides Feed/Close
// for ongoing link extraction.
//
// Internally it uses an unbounded queue between Feed and the output channel
// so that Feed never blocks — this breaks the circular channel dependency
// between the fetch workers and the discovery pipeline.
type LinkFollower struct {
	scope scope.Scope

	mu   sync.Mutex
	seen map[string]bool

	// out is the channel returned by Discover; consumers read from it.
	out chan pipeline.CrawlURL

	// queue + queueCond form an unbounded FIFO between Feed and the relay
	// goroutine that writes to out.
	queue     []pipeline.CrawlURL
	queueCond *sync.Cond
	queueMu   sync.Mutex

	feedWg    sync.WaitGroup // tracks active Feed calls
	closed    atomic.Bool    // set by Close before draining
	relayDone chan struct{}  // closed when relay goroutine exits
}

// NewLinkFollower creates a LinkFollower that filters discovered links through
// the given scope.
func NewLinkFollower(s scope.Scope) *LinkFollower {
	lf := &LinkFollower{
		scope:     s,
		out:       make(chan pipeline.CrawlURL, 64),
		seen:      make(map[string]bool),
		relayDone: make(chan struct{}),
	}
	lf.queueCond = sync.NewCond(&lf.queueMu)
	go lf.relay()
	return lf
}

// relay drains the internal unbounded queue into the output channel.
// It runs until Close signals shutdown and the queue is fully drained.
func (lf *LinkFollower) relay() {
	defer close(lf.relayDone)
	for {
		lf.queueMu.Lock()
		for len(lf.queue) == 0 {
			if lf.closed.Load() {
				lf.queueMu.Unlock()
				return
			}
			lf.queueCond.Wait()
		}
		// Take the entire batch to minimise lock hold time.
		batch := lf.queue
		lf.queue = nil
		lf.queueMu.Unlock()

		for _, cu := range batch {
			lf.out <- cu
		}
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
	lf.enqueue(crawlURL)

	return lf.out, nil
}

// Feed extracts links from a fetch result and emits new, unseen, in-scope URLs
// on the discovery channel. It is safe to call from multiple goroutines and
// never blocks — new URLs are placed in an unbounded internal queue.
// Returns the number of new URLs emitted (used for in-flight accounting).
func (lf *LinkFollower) Feed(result pipeline.FetchResult) int {
	lf.feedWg.Add(1)
	defer lf.feedWg.Done()

	if lf.closed.Load() {
		return 0
	}

	links := extractLinks(result.CrawlURL.URL, result.Body)
	parentDepth := result.CrawlURL.Depth

	lf.mu.Lock()
	var toEmit []pipeline.CrawlURL
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
		toEmit = append(toEmit, crawlURL)
	}
	lf.mu.Unlock()

	if len(toEmit) > 0 {
		lf.enqueueBatch(toEmit)
	}
	return len(toEmit)
}

// enqueue appends a single URL to the internal queue and wakes the relay.
func (lf *LinkFollower) enqueue(cu pipeline.CrawlURL) {
	lf.queueMu.Lock()
	lf.queue = append(lf.queue, cu)
	lf.queueMu.Unlock()
	lf.queueCond.Signal()
}

// enqueueBatch appends multiple URLs to the internal queue and wakes the relay.
func (lf *LinkFollower) enqueueBatch(items []pipeline.CrawlURL) {
	lf.queueMu.Lock()
	lf.queue = append(lf.queue, items...)
	lf.queueMu.Unlock()
	lf.queueCond.Signal()
}

// Close signals all active Feed calls to stop, waits for the relay
// goroutine to drain remaining items, and then closes the output channel.
// Must be called exactly once. The consumer must keep reading from the
// output channel while Close runs, or Close will block.
func (lf *LinkFollower) Close() {
	lf.closed.Store(true)
	lf.queueCond.Signal()

	// Wait for active Feed calls to finish enqueueing.
	lf.feedWg.Wait()

	// Wake the relay to notice the closed flag after the queue is empty.
	lf.queueCond.Signal()

	// Wait for the relay to drain all items and exit.
	<-lf.relayDone

	close(lf.out)
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
