package writer

import (
	"encoding/json"
	"time"
)

// PageMeta holds per-page metadata written alongside each markdown file.
type PageMeta struct {
	URL         string   `json:"url"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Headings    []string `json:"headings"`
	WordCount   int      `json:"word_count"`
	CrawledAt   string   `json:"crawled_at"`
	ContentHash string   `json:"content_hash"`
	Links       []string `json:"links"`
}

// Manifest holds summary information written at crawl completion.
type Manifest struct {
	SeedURL         string  `json:"seed_url"`
	StartedAt       string  `json:"started_at"`
	CompletedAt     string  `json:"completed_at"`
	DurationSeconds float64 `json:"duration_seconds"`
	PagesCrawled    int     `json:"pages_crawled"`
	Errors          int     `json:"errors"`
}

// marshalJSON encodes a value as indented JSON bytes.
func marshalJSON(v any) ([]byte, error) {
	return json.MarshalIndent(v, "", "  ")
}

// formatTime returns a time formatted as RFC 3339 (UTC).
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}
