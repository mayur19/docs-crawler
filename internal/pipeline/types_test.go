package pipeline_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)
	return u
}

func TestNewCrawlURL(t *testing.T) {
	u := mustParseURL(t, "https://docs.example.com/guide")
	cu := pipeline.NewCrawlURL(u, 2, pipeline.SourceLink, "crawler")

	assert.Equal(t, "https://docs.example.com/guide", cu.String())
	assert.Equal(t, 2, cu.Depth)
	assert.Equal(t, pipeline.SourceLink, cu.Source)
	assert.Equal(t, "crawler", cu.DiscoveredBy)
}

func TestNewFetchResult(t *testing.T) {
	u := mustParseURL(t, "https://docs.example.com/api")
	cu := pipeline.NewCrawlURL(u, 0, pipeline.SourceSeed, "seed")
	body := []byte("<html><body>Hello</body></html>")
	headers := http.Header{"Content-Type": {"text/html"}}

	fr := pipeline.NewFetchResult(cu, 200, headers, body, "text/html")

	assert.Equal(t, 200, fr.StatusCode)
	assert.Equal(t, body, fr.Body)
	assert.Equal(t, "text/html", fr.ContentType)
	assert.NotEmpty(t, fr.FetchedAt)
}

func TestFetchResultContentHash(t *testing.T) {
	cu := pipeline.NewCrawlURL(mustParseURL(t, "https://example.com"), 0, pipeline.SourceSeed, "")
	body := []byte("test content")

	fr := pipeline.NewFetchResult(cu, 200, nil, body, "text/html")
	hash := fr.ContentHash()

	assert.Contains(t, hash, "sha256:")
	assert.Len(t, hash, 71) // "sha256:" + 64 hex chars

	// Same content produces same hash
	fr2 := pipeline.NewFetchResult(cu, 200, nil, body, "text/html")
	assert.Equal(t, hash, fr2.ContentHash())

	// Different content produces different hash
	fr3 := pipeline.NewFetchResult(cu, 200, nil, []byte("other content"), "text/html")
	assert.NotEqual(t, hash, fr3.ContentHash())
}

func TestNewDocument(t *testing.T) {
	headings := []pipeline.Heading{
		{Level: 1, Text: "Introduction"},
		{Level: 2, Text: "Getting Started"},
	}
	links := []string{"/api", "/guide"}

	doc := pipeline.NewDocument(
		"https://docs.example.com",
		"My Docs",
		"Documentation for my project",
		"# Introduction\n\n## Getting Started",
		headings,
		links,
		42,
		"sha256:abc123",
	)

	assert.Equal(t, "https://docs.example.com", doc.URL)
	assert.Equal(t, "My Docs", doc.Title)
	assert.Equal(t, "Documentation for my project", doc.Description)
	assert.Equal(t, 42, doc.WordCount)
	assert.Equal(t, "sha256:abc123", doc.ContentHash)
	assert.NotEmpty(t, doc.CrawledAt)
	assert.Equal(t, []string{"Introduction", "Getting Started"}, doc.HeadingTexts())
}

func TestDocumentHeadingTextsEmpty(t *testing.T) {
	doc := pipeline.NewDocument("https://example.com", "Title", "", "content", nil, nil, 1, "")
	assert.Empty(t, doc.HeadingTexts())
}

func TestSourceConstants(t *testing.T) {
	assert.Equal(t, pipeline.Source("sitemap"), pipeline.SourceSitemap)
	assert.Equal(t, pipeline.Source("link"), pipeline.SourceLink)
	assert.Equal(t, pipeline.Source("seed"), pipeline.SourceSeed)
}
