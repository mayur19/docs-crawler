package pipeline_test

import (
	"context"
	"net/url"
	"testing"

	"github.com/napkin/docs-crawler/internal/pipeline"
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
