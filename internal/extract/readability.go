package extract

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
	readability "github.com/go-shiori/go-readability"
	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// ReadabilityExtractor uses go-readability to strip non-content elements
// (navigation, sidebars, footers) and extract the main article content.
type ReadabilityExtractor struct{}

// NewReadabilityExtractor creates a new ReadabilityExtractor.
func NewReadabilityExtractor() ReadabilityExtractor {
	return ReadabilityExtractor{}
}

// Name returns the extractor identifier.
func (e ReadabilityExtractor) Name() string {
	return "readability"
}

// Extract parses the fetched HTML, applies readability extraction, and
// returns a structured Document with markdown content.
func (e ReadabilityExtractor) Extract(ctx context.Context, result pipeline.FetchResult) (pipeline.Document, error) {
	pageURL := result.CrawlURL.URL.String()

	parsedURL, err := url.Parse(pageURL)
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("readability: invalid URL %q: %w", pageURL, err)
	}

	// Parse the full document for metadata extraction.
	fullDoc, err := goquery.NewDocumentFromReader(bytes.NewReader(result.Body))
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("readability: parse HTML: %w", err)
	}

	title := extractTitle(fullDoc)
	description := extractDescription(fullDoc)
	headings := extractHeadings(fullDoc)
	links := extractLinks(fullDoc, pageURL)

	// Use readability to extract the main content.
	article, err := readability.FromReader(bytes.NewReader(result.Body), parsedURL)
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("readability: extract content: %w", err)
	}

	// Prefer readability's title if the HTML <title> was empty.
	if title == "" {
		title = strings.TrimSpace(article.Title)
	}

	markdown, err := toMarkdown(article.Content)
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("readability: convert to markdown: %w", err)
	}

	wordCount := countWords(markdown)
	contentHash := result.ContentHash()

	return pipeline.NewDocument(
		pageURL,
		title,
		description,
		markdown,
		headings,
		links,
		wordCount,
		contentHash,
	), nil
}
