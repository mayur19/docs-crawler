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
