package extract

import (
	"bytes"
	"context"
	"fmt"

	"github.com/PuerkitoBio/goquery"
	"github.com/napkin/docs-crawler/internal/pipeline"
)

// SelectorExtractor uses a CSS selector to identify the content area,
// then converts the selected HTML to Markdown.
type SelectorExtractor struct {
	selector string
}

// NewSelectorExtractor creates a new SelectorExtractor with the given CSS selector.
// Common selectors: "article", "main", ".content", "#content".
func NewSelectorExtractor(selector string) SelectorExtractor {
	if selector == "" {
		selector = "body"
	}
	return SelectorExtractor{selector: selector}
}

// Name returns the extractor identifier.
func (e SelectorExtractor) Name() string {
	return "selector"
}

// Selector returns the CSS selector this extractor uses.
func (e SelectorExtractor) Selector() string {
	return e.selector
}

// Extract parses the fetched HTML, selects the content area using the CSS
// selector, and returns a structured Document with markdown content.
func (e SelectorExtractor) Extract(ctx context.Context, result pipeline.FetchResult) (pipeline.Document, error) {
	pageURL := result.CrawlURL.URL.String()

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(result.Body))
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("selector: parse HTML: %w", err)
	}

	title := extractTitle(doc)
	description := extractDescription(doc)
	headings := extractHeadings(doc)
	links := extractLinks(doc, pageURL)

	// Select the content area; fall back to <body> if selector doesn't match.
	selection := doc.Find(e.selector)
	if selection.Length() == 0 {
		selection = doc.Find("body")
	}

	selectedHTML, err := selection.Html()
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("selector: extract HTML from %q: %w", e.selector, err)
	}

	markdown, err := toMarkdown(selectedHTML)
	if err != nil {
		return pipeline.Document{}, fmt.Errorf("selector: convert to markdown: %w", err)
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
