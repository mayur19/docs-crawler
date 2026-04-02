package extract

import (
	"context"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const selectorTestHTML = `<!DOCTYPE html>
<html>
<head>
	<title>Selector Test Page</title>
	<meta name="description" content="Testing CSS selector extraction">
</head>
<body>
	<nav>
		<a href="/home">Home</a>
		<a href="/docs">Docs</a>
	</nav>
	<div class="sidebar">
		<h3>Sidebar Heading</h3>
		<p>Sidebar content that should not appear in main extraction.</p>
	</div>
	<main>
		<h1>Main Content Title</h1>
		<p>This is the main content of the page that we want to extract.</p>
		<h2>First Section</h2>
		<p>Details about the first section with <a href="https://example.com/link1">a link</a>.</p>
		<h3>Subsection</h3>
		<p>More details in the subsection.</p>
	</main>
	<footer>
		<p>Footer content</p>
		<a href="/terms">Terms</a>
	</footer>
</body>
</html>`

func TestSelectorExtractor_Name(t *testing.T) {
	ext := NewSelectorExtractor("main")
	assert.Equal(t, "selector", ext.Name())
}

func TestSelectorExtractor_Selector(t *testing.T) {
	ext := NewSelectorExtractor("article")
	assert.Equal(t, "article", ext.Selector())
}

func TestSelectorExtractor_DefaultSelector(t *testing.T) {
	ext := NewSelectorExtractor("")
	assert.Equal(t, "body", ext.Selector())
}

func TestSelectorExtractor_Extract(t *testing.T) {
	tests := []struct {
		name      string
		selector  string
		html      string
		url       string
		wantTitle string
		wantDesc  string
		wantErr   bool
		checkFunc func(t *testing.T, doc pipeline.Document)
	}{
		{
			name:      "select main element",
			selector:  "main",
			html:      selectorTestHTML,
			url:       "https://example.com/test",
			wantTitle: "Selector Test Page",
			wantDesc:  "Testing CSS selector extraction",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Contains(t, doc.Markdown, "Main Content Title")
				assert.Contains(t, doc.Markdown, "main content of the page")
				// Sidebar content should NOT be in the selected main area.
				assert.NotContains(t, doc.Markdown, "Sidebar content that should not appear")
				assert.Greater(t, doc.WordCount, 0)
			},
		},
		{
			name:     "select by class",
			selector: ".sidebar",
			html:     selectorTestHTML,
			url:      "https://example.com/test",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Contains(t, doc.Markdown, "Sidebar content")
				assert.NotContains(t, doc.Markdown, "main content of the page")
			},
		},
		{
			name:     "selector not found falls back to body",
			selector: "#nonexistent",
			html:     selectorTestHTML,
			url:      "https://example.com/test",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				// Body includes everything.
				assert.Contains(t, doc.Markdown, "Main Content Title")
				assert.Contains(t, doc.Markdown, "Sidebar content")
			},
		},
		{
			name:     "article selector with content",
			selector: "article",
			html: `<!DOCTYPE html><html><head><title>Article Page</title></head><body>
				<nav><a href="/x">X</a></nav>
				<article>
					<h1>Article Heading</h1>
					<p>Article body text with enough words to be meaningful.</p>
					<a href="/related">Related</a>
				</article>
				<footer><p>Footer</p></footer>
			</body></html>`,
			url:       "https://example.com/article",
			wantTitle: "Article Page",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Contains(t, doc.Markdown, "Article Heading")
				assert.Contains(t, doc.Markdown, "Article body text")
				assert.NotContains(t, doc.Markdown, "Footer")
			},
		},
		{
			name:     "empty body",
			selector: "main",
			html:     `<!DOCTYPE html><html><head><title>Empty</title></head><body></body></html>`,
			url:      "https://example.com/empty",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Equal(t, 0, doc.WordCount)
			},
		},
		{
			name:     "no title in HTML",
			selector: "body",
			html:     `<!DOCTYPE html><html><head></head><body><h1>Only H1</h1><p>Content here.</p></body></html>`,
			url:      "https://example.com/notitle",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Equal(t, "Only H1", doc.Title)
			},
		},
		{
			name:     "no headings at all",
			selector: "body",
			html:     `<!DOCTYPE html><html><head><title>No Headings</title></head><body><p>Just a paragraph.</p></body></html>`,
			url:      "https://example.com/noheadings",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Empty(t, doc.Headings)
				assert.Greater(t, doc.WordCount, 0)
			},
		},
		{
			name:     "headings extraction from full document",
			selector: "main",
			html:     selectorTestHTML,
			url:      "https://example.com/headings",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				// Headings are extracted from the full document, not just the selector.
				require.GreaterOrEqual(t, len(doc.Headings), 4)
				texts := make([]string, len(doc.Headings))
				for i, h := range doc.Headings {
					texts[i] = h.Text
				}
				assert.Contains(t, texts, "Main Content Title")
				assert.Contains(t, texts, "First Section")
				assert.Contains(t, texts, "Subsection")
				assert.Contains(t, texts, "Sidebar Heading")
			},
		},
		{
			name:     "links extraction includes all document links",
			selector: "main",
			html:     selectorTestHTML,
			url:      "https://example.com/links",
			checkFunc: func(t *testing.T, doc pipeline.Document) {
				t.Helper()
				assert.Contains(t, doc.Links, "https://example.com/home")
				assert.Contains(t, doc.Links, "https://example.com/docs")
				assert.Contains(t, doc.Links, "https://example.com/link1")
				assert.Contains(t, doc.Links, "https://example.com/terms")
			},
		},
	}

	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext := NewSelectorExtractor(tt.selector)
			result := newFetchResult(t, tt.url, tt.html)
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

func TestSelectorExtractor_ImplementsInterface(t *testing.T) {
	var _ pipeline.Extractor = SelectorExtractor{}
}
