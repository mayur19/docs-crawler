package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"

	"github.com/napkin/docs-crawler/internal/config"
	"github.com/napkin/docs-crawler/internal/discover"
	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/scope"
)

// PoolSizes configures goroutine counts per pipeline stage.
type PoolSizes struct {
	Discovery int
	Fetch     int
	Extract   int
	Write     int
}

// DefaultPoolSizes returns sensible default pool sizes.
func DefaultPoolSizes() PoolSizes {
	return PoolSizes{Discovery: 2, Fetch: 10, Extract: 5, Write: 3}
}

// Engine orchestrates the crawl pipeline: Discover → Fetch → Extract → Write.
type Engine struct {
	discoverers  []pipeline.Discoverer
	fetchers     []pipeline.Fetcher
	extractors   []pipeline.Extractor
	writers      []pipeline.Writer
	linkFollower *discover.LinkFollower
	dedup        *config.Deduplicator
	pools        PoolSizes
	logger       *slog.Logger

	// stats tracked during crawl
	mu          sync.Mutex
	fetchErrors int
}

// New creates a new Engine.
func New(
	discoverers []pipeline.Discoverer,
	fetchers []pipeline.Fetcher,
	extractors []pipeline.Extractor,
	writers []pipeline.Writer,
	linkFollower *discover.LinkFollower,
	dedup *config.Deduplicator,
	pools PoolSizes,
) *Engine {
	return &Engine{
		discoverers:  discoverers,
		fetchers:     fetchers,
		extractors:   extractors,
		writers:      writers,
		linkFollower: linkFollower,
		dedup:        dedup,
		pools:        pools,
		logger:       slog.Default(),
	}
}

// Stats returns crawl statistics.
func (e *Engine) Stats() CrawlStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	dedupStats := e.dedup.Stats()
	return CrawlStats{
		URLsSeen:    dedupStats.URLsSeen,
		ContentDups: dedupStats.ContentDups,
		FetchErrors: e.fetchErrors,
	}
}

// CrawlStats holds runtime statistics.
type CrawlStats struct {
	URLsSeen    int
	ContentDups int
	FetchErrors int
}

// Run executes the full crawl pipeline. It blocks until all work is done
// or the context is cancelled.
func (e *Engine) Run(ctx context.Context, cfg config.Config) error {
	seedURL, err := url.Parse(cfg.SeedURL)
	if err != nil {
		return fmt.Errorf("engine: invalid seed URL: %w", err)
	}

	fetchCh := make(chan pipeline.CrawlURL, e.pools.Fetch*2)
	extractCh := make(chan pipeline.FetchResult, e.pools.Extract*2)
	writeCh := make(chan pipeline.Document, e.pools.Write*2)

	// inFlight tracks URLs that have been sent to fetchCh but not yet
	// finished fetching. This is used to determine when the LinkFollower
	// can be safely closed (no more Feed calls will occur).
	var inFlight atomic.Int64

	var wg sync.WaitGroup

	// Stage 1: Discovery — merge all discoverer channels into fetchCh.
	discoverDone := e.startDiscovery(ctx, seedURL, fetchCh, &inFlight)

	// Stage 2: Fetch — pull from fetchCh, push to extractCh.
	fetchDone := e.startFetch(ctx, fetchCh, extractCh, &inFlight)

	// Stage 3: Extract — pull from extractCh, push to writeCh.
	extractDone := e.startExtract(ctx, extractCh, writeCh, &wg)

	// Stage 4: Write — pull from writeCh.
	writeDone := e.startWrite(ctx, writeCh, &wg)

	// Cascade: once discovery completes the LinkFollower is closed (which
	// unblocks its reader goroutine), then fetchCh is closed.
	go func() {
		discoverDone.Wait()
		close(fetchCh)
	}()

	// Pipeline cascading close: each stage closes the next channel when done.
	go func() { fetchDone.Wait(); close(extractCh) }()
	go func() { extractDone.Wait(); close(writeCh) }()

	// Wait for all writing to complete.
	writeDone.Wait()
	wg.Wait()

	// Close all writers (flush manifests, etc).
	for _, w := range e.writers {
		if err := w.Close(); err != nil {
			e.logger.Error("failed to close writer", "writer", w.Name(), "error", err)
		}
	}

	stats := e.Stats()
	e.logger.Info("crawl complete",
		"urls_seen", stats.URLsSeen,
		"content_dups", stats.ContentDups,
		"fetch_errors", stats.FetchErrors,
	)

	return nil
}

// startDiscovery launches discoverers and merges their output into fetchCh.
// When a LinkFollower is configured it is handled specially: regular
// discoverers finish first, then the engine waits for all in-flight
// fetches to complete before closing the LinkFollower (since Feed calls
// happen from the fetch stage). This avoids a deadlock where the
// LinkFollower's reader goroutine blocks on a channel that is only
// closed after discovery completes.
func (e *Engine) startDiscovery(
	ctx context.Context,
	seedURL *url.URL,
	fetchCh chan<- pipeline.CrawlURL,
	inFlight *atomic.Int64,
) *sync.WaitGroup {
	var done sync.WaitGroup

	// emitURLs reads from a discoverer channel and sends to fetchCh,
	// incrementing inFlight for each URL sent.
	emitURLs := func(ch <-chan pipeline.CrawlURL) {
		for cu := range ch {
			normalized := scope.NormalizeURL(cu.URL)
			if e.dedup.SeenURL(normalized) {
				continue
			}
			inFlight.Add(1)
			select {
			case fetchCh <- cu:
			case <-ctx.Done():
				inFlight.Add(-1)
				return
			}
		}
	}

	// emitLinkFollowerURLs is like emitURLs but for the LinkFollower.
	// URLs from Feed are pre-accounted in the in-flight counter by the
	// fetch worker (via Feed's return value). Only the initial seed URL
	// from Discover needs a fresh increment. Deduped pre-accounted URLs
	// must be decremented.
	emitLinkFollowerURLs := func(ch <-chan pipeline.CrawlURL) {
		seedConsumed := false
		for cu := range ch {
			normalized := scope.NormalizeURL(cu.URL)
			if e.dedup.SeenURL(normalized) {
				// The seed was never pre-accounted; Feed URLs were.
				if seedConsumed {
					inFlight.Add(-1)
				}
				continue
			}
			if !seedConsumed {
				// First URL is the seed — account for it now.
				inFlight.Add(1)
				seedConsumed = true
			}
			select {
			case fetchCh <- cu:
			case <-ctx.Done():
				inFlight.Add(-1)
				return
			}
		}
	}

	// Launch all non-LinkFollower discoverers.
	var regularDone sync.WaitGroup
	for _, d := range e.discoverers {
		if e.linkFollower != nil && d.Name() == e.linkFollower.Name() {
			continue
		}
		regularDone.Add(1)
		done.Add(1)
		go func(disc pipeline.Discoverer) {
			defer done.Done()
			defer regularDone.Done()
			ch, err := disc.Discover(ctx, seedURL)
			if err != nil {
				e.logger.Error("discoverer failed", "name", disc.Name(), "error", err)
				return
			}
			emitURLs(ch)
		}(d)
	}

	// Launch the LinkFollower discoverer with its own lifecycle.
	if e.linkFollower != nil {
		done.Add(1)
		go func() {
			defer done.Done()
			ch, err := e.linkFollower.Discover(ctx, seedURL)
			if err != nil {
				e.logger.Error("discoverer failed", "name", e.linkFollower.Name(), "error", err)
				return
			}
			emitLinkFollowerURLs(ch)
		}()

		// Close the LinkFollower once regular discoverers are done AND
		// no fetches are in-flight (meaning no more Feed calls will come).
		go func() {
			regularDone.Wait()
			waitForInFlightDrain(ctx, inFlight)
			e.linkFollower.Close()
		}()
	}

	return &done
}

// waitForInFlightDrain blocks until the in-flight counter reaches zero
// or the context is cancelled.
func waitForInFlightDrain(ctx context.Context, inFlight *atomic.Int64) {
	for {
		if ctx.Err() != nil {
			return
		}
		if inFlight.Load() <= 0 {
			return
		}
		// Brief yield to avoid busy-waiting.
		select {
		case <-ctx.Done():
			return
		default:
		}
	}
}

// startFetch launches fetch workers.
func (e *Engine) startFetch(
	ctx context.Context,
	fetchCh <-chan pipeline.CrawlURL,
	extractCh chan<- pipeline.FetchResult,
	inFlight *atomic.Int64,
) *sync.WaitGroup {
	var done sync.WaitGroup

	for range e.pools.Fetch {
		done.Add(1)
		go func() {
			defer done.Done()
			for cu := range fetchCh {
				if ctx.Err() != nil {
					inFlight.Add(-1)
					return
				}
				result, err := e.fetchURL(ctx, cu)
				if err != nil {
					e.mu.Lock()
					e.fetchErrors++
					e.mu.Unlock()
					e.logger.Warn("fetch failed", "url", cu.String(), "error", err)
					inFlight.Add(-1)
					continue
				}

				// Feed links back to link follower for further discovery.
				// The returned count tells us how many new URLs were added
				// to the LinkFollower's channel. We adjust the in-flight
				// counter atomically: +newURLs (pre-account for them) and
				// -1 (mark this URL as done). This ensures the counter
				// never hits zero while new work is pending in the channel.
				if e.linkFollower != nil {
					newURLs := e.linkFollower.Feed(result)
					inFlight.Add(int64(newURLs) - 1)
				} else {
					inFlight.Add(-1)
				}

				// Content dedup.
				hash := result.ContentHash()
				if e.dedup.SeenContent(hash) {
					e.logger.Debug("skipping duplicate content", "url", cu.String())
					continue
				}

				select {
				case extractCh <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return &done
}

// fetchURL tries each fetcher in order and returns the first successful result.
func (e *Engine) fetchURL(ctx context.Context, cu pipeline.CrawlURL) (pipeline.FetchResult, error) {
	for _, f := range e.fetchers {
		if f.CanFetch(cu) {
			return f.Fetch(ctx, cu)
		}
	}
	return pipeline.FetchResult{}, fmt.Errorf("no fetcher available for %s", cu.String())
}

// startExtract launches extract workers.
func (e *Engine) startExtract(
	ctx context.Context,
	extractCh <-chan pipeline.FetchResult,
	writeCh chan<- pipeline.Document,
	_ *sync.WaitGroup,
) *sync.WaitGroup {
	var done sync.WaitGroup

	for range e.pools.Extract {
		done.Add(1)
		go func() {
			defer done.Done()
			for result := range extractCh {
				if ctx.Err() != nil {
					return
				}
				doc, err := e.extractResult(ctx, result)
				if err != nil {
					e.logger.Warn("extract failed",
						"url", result.CrawlURL.String(), "error", err)
					continue
				}
				select {
				case writeCh <- doc:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return &done
}

// extractResult tries the first extractor.
func (e *Engine) extractResult(ctx context.Context, result pipeline.FetchResult) (pipeline.Document, error) {
	if len(e.extractors) == 0 {
		return pipeline.Document{}, fmt.Errorf("no extractors configured")
	}
	return e.extractors[0].Extract(ctx, result)
}

// startWrite launches write workers.
func (e *Engine) startWrite(
	ctx context.Context,
	writeCh <-chan pipeline.Document,
	_ *sync.WaitGroup,
) *sync.WaitGroup {
	var done sync.WaitGroup

	for range e.pools.Write {
		done.Add(1)
		go func() {
			defer done.Done()
			for doc := range writeCh {
				if ctx.Err() != nil {
					return
				}
				for _, w := range e.writers {
					if err := w.Write(ctx, doc); err != nil {
						e.logger.Error("write failed",
							"writer", w.Name(), "url", doc.URL, "error", err)
					}
				}
			}
		}()
	}

	return &done
}
