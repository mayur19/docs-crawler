package pipeline

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Source indicates how a URL was discovered.
type Source string

const (
	SourceSitemap Source = "sitemap"
	SourceLink    Source = "link"
	SourceSeed    Source = "seed"
)

// CrawlURL represents a URL to be crawled with discovery metadata.
type CrawlURL struct {
	URL          *url.URL
	Depth        int
	Source       Source
	DiscoveredBy string
}

// NewCrawlURL creates a new CrawlURL.
func NewCrawlURL(u *url.URL, depth int, source Source, discoveredBy string) CrawlURL {
	return CrawlURL{
		URL:          u,
		Depth:        depth,
		Source:       source,
		DiscoveredBy: discoveredBy,
	}
}

// String returns the URL string representation.
func (c CrawlURL) String() string {
	return c.URL.String()
}

// FetchResult holds the raw result of fetching a URL.
type FetchResult struct {
	CrawlURL    CrawlURL
	StatusCode  int
	Headers     http.Header
	Body        []byte
	ContentType string
	FetchedAt   time.Time
}

// NewFetchResult creates a new FetchResult.
func NewFetchResult(
	crawlURL CrawlURL,
	statusCode int,
	headers http.Header,
	body []byte,
	contentType string,
) FetchResult {
	return FetchResult{
		CrawlURL:    crawlURL,
		StatusCode:  statusCode,
		Headers:     headers,
		Body:        body,
		ContentType: contentType,
		FetchedAt:   time.Now(),
	}
}

// ContentHash returns the SHA-256 hash of the body content.
func (f FetchResult) ContentHash() string {
	h := sha256.Sum256(f.Body)
	return fmt.Sprintf("sha256:%x", h)
}

// Heading represents a heading in the document hierarchy.
type Heading struct {
	Level int
	Text  string
}

// Document is the extracted, clean content from a fetched page.
type Document struct {
	URL         string
	Title       string
	Description string
	Markdown    string
	Headings    []Heading
	Links       []string
	WordCount   int
	ContentHash string
	CrawledAt   time.Time
}

// NewDocument creates a new Document.
func NewDocument(
	pageURL string,
	title string,
	description string,
	markdown string,
	headings []Heading,
	links []string,
	wordCount int,
	contentHash string,
) Document {
	return Document{
		URL:         pageURL,
		Title:       title,
		Description: description,
		Markdown:    markdown,
		Headings:    headings,
		Links:       links,
		WordCount:   wordCount,
		ContentHash: contentHash,
		CrawledAt:   time.Now(),
	}
}

// HeadingTexts returns just the text of all headings.
func (d Document) HeadingTexts() []string {
	texts := make([]string, len(d.Headings))
	for i, h := range d.Headings {
		texts[i] = h.Text
	}
	return texts
}
