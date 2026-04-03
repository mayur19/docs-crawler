package pipeline_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
)

// mockDiscoverer verifies the Discoverer interface can be implemented.
type mockDiscoverer struct{}

func (m *mockDiscoverer) Name() string { return "mock-discoverer" }
func (m *mockDiscoverer) Discover(_ context.Context, _ *url.URL) (<-chan pipeline.CrawlURL, error) {
	ch := make(chan pipeline.CrawlURL)
	close(ch)
	return ch, nil
}

// mockFetcher verifies the Fetcher interface can be implemented.
type mockFetcher struct{}

func (m *mockFetcher) Name() string                { return "mock-fetcher" }
func (m *mockFetcher) CanFetch(_ pipeline.CrawlURL) bool { return true }
func (m *mockFetcher) Fetch(_ context.Context, _ pipeline.CrawlURL) (pipeline.FetchResult, error) {
	return pipeline.FetchResult{}, nil
}

// mockExtractor verifies the Extractor interface can be implemented.
type mockExtractor struct{}

func (m *mockExtractor) Name() string { return "mock-extractor" }
func (m *mockExtractor) Extract(_ context.Context, _ pipeline.FetchResult) (pipeline.Document, error) {
	return pipeline.Document{}, nil
}

// mockWriter verifies the Writer interface can be implemented.
type mockWriter struct{}

func (m *mockWriter) Name() string                                    { return "mock-writer" }
func (m *mockWriter) Write(_ context.Context, _ pipeline.Document) error { return nil }
func (m *mockWriter) Close() error                                    { return nil }

func TestInterfaceCompliance(t *testing.T) {
	var d pipeline.Discoverer = &mockDiscoverer{}
	var f pipeline.Fetcher = &mockFetcher{}
	var e pipeline.Extractor = &mockExtractor{}
	var w pipeline.Writer = &mockWriter{}

	assert.Equal(t, "mock-discoverer", d.Name())
	assert.Equal(t, "mock-fetcher", f.Name())
	assert.Equal(t, "mock-extractor", e.Name())
	assert.Equal(t, "mock-writer", w.Name())
}

func TestDiscovererReturnsChannel(t *testing.T) {
	d := &mockDiscoverer{}
	ch, err := d.Discover(context.Background(), nil)
	assert.NoError(t, err)

	// Channel should be closed (empty)
	_, ok := <-ch
	assert.False(t, ok)
}

func TestFetcherCanFetch(t *testing.T) {
	f := &mockFetcher{}
	assert.True(t, f.CanFetch(pipeline.CrawlURL{}))
}

func TestWriterClose(t *testing.T) {
	w := &mockWriter{}
	assert.NoError(t, w.Close())
}

// mockChunker verifies the Chunker interface can be implemented.
type mockChunker struct{}

func (m *mockChunker) Name() string { return "mock-chunker" }
func (m *mockChunker) Chunk(_ context.Context, _ pipeline.Document) ([]pipeline.Chunk, error) {
	return []pipeline.Chunk{}, nil
}

// mockEmbedder verifies the Embedder interface can be implemented.
type mockEmbedder struct{}

func (m *mockEmbedder) Name() string { return "mock-embedder" }
func (m *mockEmbedder) Embed(_ context.Context, _ []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	return []pipeline.EmbeddedChunk{}, nil
}
func (m *mockEmbedder) Dimensions() int { return 1536 }

// mockIndexer verifies the Indexer interface can be implemented.
type mockIndexer struct{}

func (m *mockIndexer) Name() string { return "mock-indexer" }
func (m *mockIndexer) Index(_ context.Context, _ []pipeline.EmbeddedChunk) error { return nil }
func (m *mockIndexer) Search(_ context.Context, _ string, _ int) ([]pipeline.SearchResult, error) {
	return []pipeline.SearchResult{}, nil
}
func (m *mockIndexer) Close() error { return nil }

func TestChunkerEmbedderIndexerCompliance(t *testing.T) {
	var c pipeline.Chunker = &mockChunker{}
	var e pipeline.Embedder = &mockEmbedder{}
	var i pipeline.Indexer = &mockIndexer{}

	assert.Equal(t, "mock-chunker", c.Name())
	assert.Equal(t, "mock-embedder", e.Name())
	assert.Equal(t, "mock-indexer", i.Name())
}

func TestEmbedderDimensions(t *testing.T) {
	e := &mockEmbedder{}
	assert.Equal(t, 1536, e.Dimensions())
}

func TestIndexerClose(t *testing.T) {
	i := &mockIndexer{}
	assert.NoError(t, i.Close())
}

func TestChunkerReturnsSlice(t *testing.T) {
	c := &mockChunker{}
	chunks, err := c.Chunk(context.Background(), pipeline.Document{})
	assert.NoError(t, err)
	assert.NotNil(t, chunks)
}

func TestIndexerSearch(t *testing.T) {
	i := &mockIndexer{}
	results, err := i.Search(context.Background(), "test query", 5)
	assert.NoError(t, err)
	assert.NotNil(t, results)
}
