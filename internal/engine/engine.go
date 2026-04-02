package engine

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mayur19/docs-crawler/internal/config"
	"github.com/mayur19/docs-crawler/internal/discover"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/scope"
)

// PoolSizes configures goroutine counts per pipeline stage.
type PoolSizes struct {
	Discovery int
	Fetch     int
	Extract   int
	Write     int
	Chunk     int
	Embed     int
	Index     int
}

// DefaultPoolSizes returns sensible default pool sizes.
func DefaultPoolSizes() PoolSizes {
	return PoolSizes{Discovery: 2, Fetch: 10, Extract: 5, Write: 3, Chunk: 3, Embed: 2, Index: 1}
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
// or the context is cancelled. It polls on a short ticker instead of
// busy-spinning to avoid wasting CPU.
func waitForInFlightDrain(ctx context.Context, inFlight *atomic.Int64) {
	if inFlight.Load() <= 0 {
		return
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if inFlight.Load() <= 0 {
				return
			}
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

// RunIngest executes the full ingest pipeline: Discover → Fetch → Extract → Chunk → Embed → Index.
// It blocks until all work is done or the context is cancelled.
func (e *Engine) RunIngest(
	ctx context.Context,
	cfg config.Config,
	chunker pipeline.Chunker,
	embedder pipeline.Embedder,
	indexer pipeline.Indexer,
) error {
	seedURL, err := url.Parse(cfg.SeedURL)
	if err != nil {
		return fmt.Errorf("engine: invalid seed URL: %w", err)
	}

	fetchCh := make(chan pipeline.CrawlURL, e.pools.Fetch*2)
	extractCh := make(chan pipeline.FetchResult, e.pools.Extract*2)
	chunkCh := make(chan pipeline.Document, e.pools.Chunk*2)
	embedCh := make(chan []pipeline.Chunk, e.pools.Embed*2)
	indexCh := make(chan []pipeline.EmbeddedChunk, e.pools.Index*2)

	var inFlight atomic.Int64

	// Stage 1: Discovery.
	discoverDone := e.startDiscovery(ctx, seedURL, fetchCh, &inFlight)

	// Stage 2: Fetch.
	fetchDone := e.startFetch(ctx, fetchCh, extractCh, &inFlight)

	// Stage 3: Extract — push to chunkCh instead of writeCh.
	var wg sync.WaitGroup
	extractDone := e.startExtract(ctx, extractCh, chunkCh, &wg)

	// Stage 4: Chunk.
	chunkDone := e.startChunk(ctx, chunkCh, embedCh, chunker)

	// Stage 5: Embed.
	embedDone := e.startEmbed(ctx, embedCh, indexCh, embedder)

	// Stage 6: Index.
	indexDone := e.startIndex(ctx, indexCh, indexer)

	// Cascade close.
	go func() { discoverDone.Wait(); close(fetchCh) }()
	go func() { fetchDone.Wait(); close(extractCh) }()
	go func() { extractDone.Wait(); close(chunkCh) }()
	go func() { chunkDone.Wait(); close(embedCh) }()
	go func() { embedDone.Wait(); close(indexCh) }()

	indexDone.Wait()
	wg.Wait()

	if err := indexer.Close(); err != nil {
		e.logger.Error("failed to close indexer", "indexer", indexer.Name(), "error", err)
	}

	stats := e.Stats()
	e.logger.Info("ingest complete",
		"urls_seen", stats.URLsSeen,
		"content_dups", stats.ContentDups,
		"fetch_errors", stats.FetchErrors,
	)

	return nil
}

// startChunk launches chunk workers that read Documents from chunkCh and
// push []Chunk batches to embedCh.
func (e *Engine) startChunk(
	ctx context.Context,
	chunkCh <-chan pipeline.Document,
	embedCh chan<- []pipeline.Chunk,
	chunker pipeline.Chunker,
) *sync.WaitGroup {
	var done sync.WaitGroup

	for range e.pools.Chunk {
		done.Add(1)
		go func() {
			defer done.Done()
			for doc := range chunkCh {
				if ctx.Err() != nil {
					return
				}
				chunks, err := chunker.Chunk(ctx, doc)
				if err != nil {
					e.logger.Warn("chunk failed", "url", doc.URL, "error", err)
					continue
				}
				select {
				case embedCh <- chunks:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return &done
}

// startEmbed launches embed workers that read []Chunk batches from embedCh and
// push []EmbeddedChunk batches to indexCh.
func (e *Engine) startEmbed(
	ctx context.Context,
	embedCh <-chan []pipeline.Chunk,
	indexCh chan<- []pipeline.EmbeddedChunk,
	embedder pipeline.Embedder,
) *sync.WaitGroup {
	var done sync.WaitGroup

	for range e.pools.Embed {
		done.Add(1)
		go func() {
			defer done.Done()
			for chunks := range embedCh {
				if ctx.Err() != nil {
					return
				}
				embedded, err := embedder.Embed(ctx, chunks)
				if err != nil {
					e.logger.Warn("embed failed", "chunks", len(chunks), "error", err)
					continue
				}
				select {
				case indexCh <- embedded:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	return &done
}

// startIndex launches index workers that read []EmbeddedChunk batches from
// indexCh and call indexer.Index().
func (e *Engine) startIndex(
	ctx context.Context,
	indexCh <-chan []pipeline.EmbeddedChunk,
	indexer pipeline.Indexer,
) *sync.WaitGroup {
	var done sync.WaitGroup

	for range e.pools.Index {
		done.Add(1)
		go func() {
			defer done.Done()
			for embedded := range indexCh {
				if ctx.Err() != nil {
					return
				}
				if err := indexer.Index(ctx, embedded); err != nil {
					e.logger.Error("index failed", "chunks", len(embedded), "error", err)
				}
			}
		}()
	}

	return &done
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
