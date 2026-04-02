package extract

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const realisticHTML = `<!DOCTYPE html>
<html>
<head>
	<title>Getting Started with Go</title>
	<meta name="description" content="A comprehensive guide to Go programming">
</head>
<body>
	<nav>
		<a href="/home">Home</a>
		<a href="/docs">Docs</a>
		<a href="/blog">Blog</a>
	</nav>
	<aside class="sidebar">
		<h3>Related Articles</h3>
		<ul>
			<li><a href="/related-1">Related One</a></li>
		</ul>
	</aside>
	<article>
		<h1>Getting Started with Go</h1>
		<p>Go is a statically typed compiled programming language designed at Google.
		It is syntactically similar to C but with memory safety and garbage collection.</p>
		<h2>Installation</h2>
		<p>Download Go from the <a href="https://go.dev/dl/">official site</a>.
		Follow the installation instructions for your operating system.</p>
		<h3>Verify Installation</h3>
		<p>Run the following command to verify your installation works correctly:</p>
		<pre><code>go version</code></pre>
		<h2>Your First Program</h2>
		<p>Create a file called main.go with the following content and then run it.</p>
		<a href="https://play.golang.org">Try it on the Go Playground</a>
	</article>
	<footer>
		<p>Copyright 2024 Go Team</p>
		<a href="/privacy">Privacy Policy</a>
	</footer>
</body>
</html>`

func newFetchResult(t *testing.T, rawURL string, body string) pipeline.FetchResult {
	t.Helper()
	u, err := url.Parse(rawURL)
	require.NoError(t, err)
	crawlURL := pipeline.NewCrawlURL(u, 0, pipeline.SourceSeed, "")
	return pipeline.NewFetchResult(crawlURL, http.StatusOK, http.Header{}, []byte(body), "text/html")
}

func TestReadabilityExtractor_Name(t *testing.T) {
	ext := NewReadabilityExtractor()
	assert.Equal(t, "readability", ext.Name())
}

func TestReadabilityExtractor_Extract(t *testing.T) {
	tests := []struct {
		name        string
		html        string
		wantTitle   string
		wantDesc    string
		wantErr     bool
		checkFunc   func(t *testing.T, doc pipeline.Document)
	}{
		{
			name:      "realistic page with nav sidebar footer",
			html:      realisticHTML,
			wantTitle: "Getting Started with Go",
			wantDesc:  "A comprehensive guide to Go programming",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				// Readability should extract the main article content.
				assert.Contains(t, doc.Markdown, "Go is a statically typed")
				assert.Contains(t, doc.Markdown, "Installation")
				assert.Greater(t, doc.WordCount, 0)
				assert.NotEmpty(t, doc.ContentHash)
				assert.Equal(t, "https://example.com/docs/go", doc.URL)
			},
		},
		{
			name: "page with no title falls back to readability title",
			html: `<!DOCTYPE html><html><head></head><body>
				<article><h1>Fallback Title</h1><p>Some content that is long enough for readability to process properly and not discard as too short.</p>
				<p>More content paragraphs help readability determine this is the main content area of the page.</p></article></body></html>`,
			wantTitle: "Fallback Title",
			wantDesc:  "",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Contains(t, doc.Markdown, "Some content")
			},
		},
		{
			name: "page with headings extracts all levels",
			html: `<!DOCTYPE html><html><head><title>Headings Test</title></head><body>
				<article>
				<h1>Main Title</h1><p>Introductory paragraph with enough text for readability.</p>
				<h2>Section One</h2><p>Content under section one with sufficient text.</p>
				<h3>Subsection</h3><p>Content under subsection with more text.</p>
				<h4>Deep Section</h4><p>Content at depth four.</p>
				<h5>Deeper Section</h5><p>Even deeper content here.</p>
				<h6>Deepest Section</h6><p>The deepest heading level.</p>
				</article></body></html>`,
			wantTitle: "Headings Test",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				require.GreaterOrEqual(t, len(doc.Headings), 6)
				assert.Equal(t, 1, doc.Headings[0].Level)
				assert.Equal(t, "Main Title", doc.Headings[0].Text)
				assert.Equal(t, 2, doc.Headings[1].Level)
				assert.Equal(t, 6, doc.Headings[5].Level)
			},
		},
		{
			name:      "empty body produces empty markdown",
			html:      `<!DOCTYPE html><html><head><title>Empty</title></head><body></body></html>`,
			wantTitle: "Empty",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Equal(t, 0, doc.WordCount)
			},
		},
		{
			name: "links are extracted and resolved",
			html: `<!DOCTYPE html><html><head><title>Links</title></head><body>
				<article>
				<p>Visit <a href="/about">About</a> or <a href="https://other.com/page">Other</a> for more info and details that make this long enough.</p>
				<p>Additional paragraph with enough content for readability extraction to work properly with the text.</p>
				</article></body></html>`,
			wantTitle: "Links",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Contains(t, doc.Links, "https://example.com/about")
				assert.Contains(t, doc.Links, "https://other.com/page")
			},
		},
	}

	ext := NewReadabilityExtractor()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := newFetchResult(t, "https://example.com/docs/go", tt.html)
			doc, err := ext.Extract(ctx, result)

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			if tt.wantTitle != "" {
				assert.Equal(t, tt.wantTitle, doc.Title)
			}
			if tt.wantDesc != "" {
				assert.Equal(t, tt.wantDesc, doc.Description)
			}
			if tt.checkFunc != nil {
				tt.checkFunc(t, doc)
			}
		})
	}
}

func TestReadabilityExtractor_ImplementsInterface(t *testing.T) {
	var _ pipeline.Extractor = ReadabilityExtractor{}
}
