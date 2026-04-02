package engine_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/mayur19/docs-crawler/internal/config"
	"github.com/mayur19/docs-crawler/internal/engine"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Mock plugins ---

type stubDiscoverer struct {
	urls []string
}

func (d *stubDiscoverer) Name() string { return "stub-discoverer" }
func (d *stubDiscoverer) Discover(_ context.Context, seed *url.URL) (<-chan pipeline.CrawlURL, error) {
	ch := make(chan pipeline.CrawlURL, len(d.urls)+1)
	ch <- pipeline.NewCrawlURL(seed, 0, pipeline.SourceSeed, "stub")
	for _, u := range d.urls {
		parsed, _ := url.Parse(u)
		ch <- pipeline.NewCrawlURL(parsed, 1, pipeline.SourceLink, "stub")
	}
	close(ch)
	return ch, nil
}

type stubFetcher struct{}

func (f *stubFetcher) Name() string                      { return "stub-fetcher" }
func (f *stubFetcher) CanFetch(_ pipeline.CrawlURL) bool { return true }
func (f *stubFetcher) Fetch(_ context.Context, u pipeline.CrawlURL) (pipeline.FetchResult, error) {
	// Return unique body per URL so content dedup doesn't filter them.
	body := fmt.Sprintf("<html><body>Content for %s</body></html>", u.URL.String())
	return pipeline.NewFetchResult(
		u, 200, http.Header{},
		[]byte(body), "text/html",
	), nil
}

type stubExtractor struct{}

func (e *stubExtractor) Name() string { return "stub-extractor" }
func (e *stubExtractor) Extract(_ context.Context, r pipeline.FetchResult) (pipeline.Document, error) {
	return pipeline.NewDocument(
		r.CrawlURL.String(),
		"Test Page",
		"A test page",
		"# Test\n\nContent",
		[]pipeline.Heading{{Level: 1, Text: "Test"}},
		nil,
		2,
		r.ContentHash(),
	), nil
}

type stubWriter struct {
	mu   sync.Mutex
	docs []pipeline.Document
}

func (w *stubWriter) Name() string { return "stub-writer" }
func (w *stubWriter) Write(_ context.Context, doc pipeline.Document) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.docs = append(w.docs, doc)
	return nil
}
func (w *stubWriter) Close() error { return nil }

func (w *stubWriter) DocCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.docs)
}

// --- Tests ---

func TestEngineRunBasic(t *testing.T) {
	disc := &stubDiscoverer{}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}
	writer := &stubWriter{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{writer},
		nil, // no link follower
		config.NewDeduplicator(),
		engine.PoolSizes{Discovery: 1, Fetch: 2, Extract: 2, Write: 1},
	)

	cfg := config.NewConfig("https://docs.example.com")

	err := e.Run(context.Background(), cfg)
	require.NoError(t, err)

	// Should have written 1 doc (the seed URL).
	assert.Equal(t, 1, writer.DocCount())
}

func TestEngineRunMultipleURLs(t *testing.T) {
	disc := &stubDiscoverer{
		urls: []string{
			"https://docs.example.com/page1",
			"https://docs.example.com/page2",
		},
	}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}
	writer := &stubWriter{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{writer},
		nil,
		config.NewDeduplicator(),
		engine.PoolSizes{Discovery: 1, Fetch: 2, Extract: 2, Write: 1},
	)

	cfg := config.NewConfig("https://docs.example.com")

	err := e.Run(context.Background(), cfg)
	require.NoError(t, err)

	// seed + 2 discovered pages = 3
	assert.Equal(t, 3, writer.DocCount())
}

func TestEngineDeduplicatesURLs(t *testing.T) {
	disc := &stubDiscoverer{
		urls: []string{
			"https://docs.example.com/page1",
			"https://docs.example.com/page1", // duplicate URL
		},
	}
	extractor := &stubExtractor{}
	writer := &stubWriter{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{&stubFetcher{}},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{writer},
		nil,
		config.NewDeduplicator(),
		engine.PoolSizes{Discovery: 1, Fetch: 2, Extract: 2, Write: 1},
	)

	cfg := config.NewConfig("https://docs.example.com")

	err := e.Run(context.Background(), cfg)
	require.NoError(t, err)

	// seed + 1 unique page = 2 (duplicate URL filtered by dedup)
	assert.Equal(t, 2, writer.DocCount())
}

func TestEngineCancellation(t *testing.T) {
	disc := &stubDiscoverer{}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}
	writer := &stubWriter{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{writer},
		nil,
		config.NewDeduplicator(),
		engine.DefaultPoolSizes(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cfg := config.NewConfig("https://docs.example.com")
	err := e.Run(ctx, cfg)

	// Should complete without hanging, even with cancelled context.
	assert.NoError(t, err)
}

func TestEngineStats(t *testing.T) {
	disc := &stubDiscoverer{
		urls: []string{"https://docs.example.com/page1"},
	}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}
	writer := &stubWriter{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{writer},
		nil,
		config.NewDeduplicator(),
		engine.PoolSizes{Discovery: 1, Fetch: 2, Extract: 2, Write: 1},
	)

	cfg := config.NewConfig("https://docs.example.com")
	err := e.Run(context.Background(), cfg)
	require.NoError(t, err)

	stats := e.Stats()
	assert.Equal(t, 2, stats.URLsSeen) // seed + page1
	assert.Equal(t, 0, stats.FetchErrors)
}

func TestDefaultPoolSizes(t *testing.T) {
	pools := engine.DefaultPoolSizes()
	assert.Equal(t, 2, pools.Discovery)
	assert.Equal(t, 10, pools.Fetch)
	assert.Equal(t, 5, pools.Extract)
	assert.Equal(t, 3, pools.Write)
	assert.Equal(t, 3, pools.Chunk)
	assert.Equal(t, 2, pools.Embed)
	assert.Equal(t, 1, pools.Index)
}

func TestEngineRunInvalidSeedURL(t *testing.T) {
	e := engine.New(nil, nil, nil, nil, nil, config.NewDeduplicator(), engine.DefaultPoolSizes())
	cfg := config.NewConfig("://invalid")
	err := e.Run(context.Background(), cfg)
	assert.Error(t, err)
}

func TestEngineTimeout(t *testing.T) {
	disc := &stubDiscoverer{}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}
	writer := &stubWriter{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{writer},
		nil,
		config.NewDeduplicator(),
		engine.DefaultPoolSizes(),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg := config.NewConfig("https://docs.example.com")
	err := e.Run(ctx, cfg)
	assert.NoError(t, err)
}

// --- Ingest stubs ---

type stubChunkerImpl struct{}

func (s *stubChunkerImpl) Name() string { return "stub-chunker" }
func (s *stubChunkerImpl) Chunk(_ context.Context, doc pipeline.Document) ([]pipeline.Chunk, error) {
	return []pipeline.Chunk{
		pipeline.NewChunk(doc.URL, doc.Title, nil, doc.Markdown, 0, 1),
	}, nil
}

type stubEmbedderImpl struct{}

func (s *stubEmbedderImpl) Name() string       { return "stub-embedder" }
func (s *stubEmbedderImpl) Dimensions() int    { return 3 }
func (s *stubEmbedderImpl) Embed(_ context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	result := make([]pipeline.EmbeddedChunk, len(chunks))
	for i, c := range chunks {
		result[i] = pipeline.NewEmbeddedChunk(c, []float32{0.1, 0.2, 0.3})
	}
	return result, nil
}

type stubIndexerImpl struct {
	mu     sync.Mutex
	chunks []pipeline.EmbeddedChunk
}

func (s *stubIndexerImpl) Name() string { return "stub-indexer" }
func (s *stubIndexerImpl) Index(_ context.Context, chunks []pipeline.EmbeddedChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.chunks = append(s.chunks, chunks...)
	return nil
}
func (s *stubIndexerImpl) Search(_ context.Context, _ string, _ int) ([]pipeline.SearchResult, error) {
	return nil, nil
}
func (s *stubIndexerImpl) Close() error { return nil }

func (s *stubIndexerImpl) ChunkCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.chunks)
}

// --- RunIngest tests ---

func TestEngineRunIngest(t *testing.T) {
	disc := &stubDiscoverer{
		urls: []string{
			"https://docs.example.com/page1",
			"https://docs.example.com/page2",
		},
	}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}
	chunker := &stubChunkerImpl{}
	embedder := &stubEmbedderImpl{}
	indexer := &stubIndexerImpl{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		nil, // no writers for ingest path
		nil,
		config.NewDeduplicator(),
		engine.PoolSizes{Discovery: 1, Fetch: 2, Extract: 2, Write: 1, Chunk: 2, Embed: 2, Index: 1},
	)

	cfg := config.NewConfig("https://docs.example.com")
	err := e.RunIngest(context.Background(), cfg, chunker, embedder, indexer)
	require.NoError(t, err)

	// seed + page1 + page2 = 3 docs, each produces 1 chunk → 3 embedded chunks
	assert.Equal(t, 3, indexer.ChunkCount())
}

func TestEngineRunIngestInvalidSeedURL(t *testing.T) {
	e := engine.New(nil, nil, nil, nil, nil, config.NewDeduplicator(), engine.DefaultPoolSizes())
	cfg := config.NewConfig("://invalid")
	err := e.RunIngest(context.Background(), cfg, &stubChunkerImpl{}, &stubEmbedderImpl{}, &stubIndexerImpl{})
	assert.Error(t, err)
}

func TestEngineRunIngestCancellation(t *testing.T) {
	disc := &stubDiscoverer{}
	fetcher := &stubFetcher{}
	extractor := &stubExtractor{}

	e := engine.New(
		[]pipeline.Discoverer{disc},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		nil,
		nil,
		config.NewDeduplicator(),
		engine.DefaultPoolSizes(),
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	cfg := config.NewConfig("https://docs.example.com")
	err := e.RunIngest(ctx, cfg, &stubChunkerImpl{}, &stubEmbedderImpl{}, &stubIndexerImpl{})
	assert.NoError(t, err)
}
