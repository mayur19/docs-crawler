package pipeline

import (
	"context"
	"net/url"
)

// Discoverer finds URLs to crawl from a seed URL.
type Discoverer interface {
	Name() string
	Discover(ctx context.Context, seed *url.URL) (<-chan CrawlURL, error)
}

// Fetcher retrieves page content for a given URL.
type Fetcher interface {
	Name() string
	CanFetch(u CrawlURL) bool
	Fetch(ctx context.Context, u CrawlURL) (FetchResult, error)
}

// Extractor converts a raw FetchResult into a structured Document.
type Extractor interface {
	Name() string
	Extract(ctx context.Context, result FetchResult) (Document, error)
}

// Writer outputs a Document to storage.
type Writer interface {
	Name() string
	Write(ctx context.Context, doc Document) error
	Close() error
}

// Chunker splits a Document into smaller Chunks for embedding.
type Chunker interface {
	Name() string
	Chunk(ctx context.Context, doc Document) ([]Chunk, error)
}

// Embedder converts Chunks into vector embeddings.
type Embedder interface {
	Name() string
	Embed(ctx context.Context, chunks []Chunk) ([]EmbeddedChunk, error)
	Dimensions() int
}

// Indexer stores embedded chunks and supports search.
type Indexer interface {
	Name() string
	Index(ctx context.Context, chunks []EmbeddedChunk) error
	Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
	Close() error
}
