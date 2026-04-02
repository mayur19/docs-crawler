package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/napkin/docs-crawler/internal/config"
	"github.com/napkin/docs-crawler/internal/discover"
	"github.com/napkin/docs-crawler/internal/engine"
	"github.com/napkin/docs-crawler/internal/extract"
	"github.com/napkin/docs-crawler/internal/fetch"
	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/ratelimit"
	"github.com/napkin/docs-crawler/internal/scope"
	"github.com/napkin/docs-crawler/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Test HTML pages
// ---------------------------------------------------------------------------

const indexHTML = `<!DOCTYPE html>
<html>
<head>
  <title>MyDocs - Home</title>
  <meta name="description" content="Welcome to MyDocs, your documentation hub.">
</head>
<body>
  <nav><a href="/">Home</a> | <a href="/getting-started">Getting Started</a> | <a href="/api/auth">API Auth</a></nav>
  <main>
    <h1>Welcome to MyDocs</h1>
    <p>This is the home page for the MyDocs documentation site. Here you will find guides and API references to help you integrate with our platform.</p>
    <h2>Quick Links</h2>
    <ul>
      <li><a href="/getting-started">Getting Started Guide</a></li>
      <li><a href="/api/auth">Authentication API</a></li>
    </ul>
  </main>
  <footer><p>&copy; 2026 MyDocs</p></footer>
</body>
</html>`

const gettingStartedHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Getting Started - MyDocs</title>
  <meta name="description" content="Learn how to get started with MyDocs in just a few steps.">
</head>
<body>
  <nav><a href="/">Home</a> | <a href="/getting-started">Getting Started</a> | <a href="/api/auth">API Auth</a></nav>
  <main>
    <h1>Getting Started</h1>
    <p>Welcome to the getting started guide. Follow these steps to set up your account and make your first API call.</p>
    <h2>Step 1: Create an Account</h2>
    <p>Visit the signup page and create a new account. You will receive an API key via email.</p>
    <h2>Step 2: Install the SDK</h2>
    <p>Install our SDK using your preferred package manager. We support npm, pip, and go modules.</p>
    <h2>Step 3: Authenticate</h2>
    <p>Use your API key to authenticate. See the <a href="/api/auth">Authentication API</a> for details.</p>
  </main>
  <footer><p>&copy; 2026 MyDocs</p></footer>
</body>
</html>`

const apiAuthHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Authentication API - MyDocs</title>
  <meta name="description" content="Learn how to authenticate API requests using tokens and API keys.">
</head>
<body>
  <nav><a href="/">Home</a> | <a href="/getting-started">Getting Started</a> | <a href="/api/auth">API Auth</a></nav>
  <main>
    <h1>Authentication API</h1>
    <p>All API requests must be authenticated using either an API key or a bearer token.</p>
    <h2>API Key Authentication</h2>
    <p>Include your API key in the X-API-Key header with every request.</p>
    <h2>Bearer Token Authentication</h2>
    <p>For user-scoped access, use OAuth2 bearer tokens. Tokens expire after 1 hour.</p>
  </main>
  <footer><p>&copy; 2026 MyDocs</p></footer>
</body>
</html>`

// sitemapXMLTemplate returns a sitemap referencing the given base URL.
func sitemapXMLTemplate(baseURL string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/</loc></url>
  <url><loc>%s/getting-started</loc></url>
  <url><loc>%s/api/auth</loc></url>
</urlset>`, baseURL, baseURL, baseURL)
}

const robotsTXT = `User-agent: *
Allow: /
Crawl-delay: 0
`

// ---------------------------------------------------------------------------
// Test server helpers
// ---------------------------------------------------------------------------

// newDocServer creates an httptest server serving a small documentation site.
func newDocServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	})

	mux.HandleFunc("GET /getting-started", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, gettingStartedHTML)
	})

	mux.HandleFunc("GET /api/auth", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, apiAuthHTML)
	})

	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, robotsTXT)
	})

	server := httptest.NewServer(mux)

	// Register the sitemap handler after server creation so we have the URL.
	mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, sitemapXMLTemplate(server.URL))
	})

	t.Cleanup(server.Close)
	return server
}

// buildPipeline wires the real packages into a crawl engine for integration testing.
func buildPipeline(
	t *testing.T,
	serverURL string,
	outputDir string,
	scopeCfg scope.ScopeConfig,
) *engine.Engine {
	t.Helper()

	s := scope.NewScope(scopeCfg)
	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{DefaultRate: 100})
	httpClient := &http.Client{Timeout: 5 * time.Second}

	fetcher := fetch.NewHTTPFetcher(httpClient, limiter, "integration-test-agent")
	extractor := extract.NewReadabilityExtractor()
	mdWriter, err := writer.NewMarkdownWriter(outputDir, serverURL)
	require.NoError(t, err)

	sitemapDisc := discover.NewSitemapDiscoverer(s)
	linkFollower := discover.NewLinkFollower(s)

	dedup := config.NewDeduplicator()
	pools := engine.PoolSizes{Discovery: 2, Fetch: 3, Extract: 2, Write: 2}

	return engine.New(
		[]pipeline.Discoverer{sitemapDisc, linkFollower},
		[]pipeline.Fetcher{fetcher},
		[]pipeline.Extractor{extractor},
		[]pipeline.Writer{mdWriter},
		linkFollower,
		dedup,
		pools,
	)
}

// readManifest reads and parses the manifest.json from the output directory.
func readManifest(t *testing.T, outputDir string) writer.Manifest {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(outputDir, "manifest.json"))
	require.NoError(t, err, "manifest.json should exist")

	var m writer.Manifest
	require.NoError(t, json.Unmarshal(data, &m))
	return m
}

// readPageMeta reads and parses a .meta.json sidecar file.
func readPageMeta(t *testing.T, path string) writer.PageMeta {
	t.Helper()

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var meta writer.PageMeta
	require.NoError(t, json.Unmarshal(data, &meta))
	return meta
}

// ---------------------------------------------------------------------------
// Integration Tests
// ---------------------------------------------------------------------------

func TestIntegrationFullCrawl(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	server := newDocServer(t)
	outputDir := t.TempDir()

	scopeCfg := scope.ScopeConfig{
		Prefix:     server.URL,
		SameDomain: true,
	}

	e := buildPipeline(t, server.URL, outputDir, scopeCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := config.NewConfig(server.URL).
		WithOutputDir(outputDir).
		WithWorkers(2)

	err := e.Run(ctx, cfg)
	require.NoError(t, err)

	// Verify manifest.
	manifest := readManifest(t, outputDir)
	assert.Equal(t, 3, manifest.PagesCrawled, "should crawl exactly 3 pages")
	assert.Equal(t, 0, manifest.Errors)
	assert.Equal(t, server.URL, manifest.SeedURL)

	// Verify markdown files exist and have content.
	expectedPages := map[string]struct {
		titleContains string
		urlSuffix     string
	}{
		"index.md":          {titleContains: "MyDocs", urlSuffix: "/"},
		"getting-started.md": {titleContains: "Getting Started", urlSuffix: "/getting-started"},
		"api/auth.md":       {titleContains: "Authentication", urlSuffix: "/api/auth"},
	}

	pagesDir := filepath.Join(outputDir, "pages")
	for relPath, expected := range expectedPages {
		mdPath := filepath.Join(pagesDir, relPath)
		metaPath := mdPath[:len(mdPath)-3] + ".meta.json"

		// Markdown file should exist and have content.
		mdContent, err := os.ReadFile(mdPath)
		require.NoError(t, err, "markdown file %s should exist", relPath)
		assert.NotEmpty(t, string(mdContent), "markdown file %s should have content", relPath)

		// Meta file should exist with correct URL and title.
		meta := readPageMeta(t, metaPath)
		assert.Contains(t, meta.URL, expected.urlSuffix,
			"meta URL for %s should contain %s", relPath, expected.urlSuffix)
		assert.Contains(t, meta.Title, expected.titleContains,
			"meta title for %s should contain %q", relPath, expected.titleContains)
	}
}

func TestIntegrationRateLimitedServer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	// Track which URLs have been seen to return 429 on first request.
	var mu sync.Mutex
	seen := make(map[string]bool)

	mux := http.NewServeMux()

	// gate wraps a handler to return 429 on first request per path.
	// Uses Retry-After: 0 to avoid blocking the adaptive rate limiter.
	gate := func(path string, handler http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			first := !seen[path]
			seen[path] = true
			mu.Unlock()

			if first {
				w.Header().Set("Retry-After", "0")
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			handler(w, r)
		}
	}

	mux.HandleFunc("GET /", gate("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, indexHTML)
	}))

	mux.HandleFunc("GET /getting-started", gate("/getting-started",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, gettingStartedHTML)
		}))

	mux.HandleFunc("GET /api/auth", gate("/api/auth",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			fmt.Fprint(w, apiAuthHTML)
		}))

	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, robotsTXT)
	})

	server := httptest.NewServer(mux)

	mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		fmt.Fprint(w, sitemapXMLTemplate(server.URL))
	})

	t.Cleanup(server.Close)

	outputDir := t.TempDir()

	scopeCfg := scope.ScopeConfig{
		Prefix:     server.URL,
		SameDomain: true,
	}

	e := buildPipeline(t, server.URL, outputDir, scopeCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg := config.NewConfig(server.URL).
		WithOutputDir(outputDir).
		WithWorkers(2)

	err := e.Run(ctx, cfg)
	require.NoError(t, err)

	// Despite 429s on first requests, the crawler should retry and complete.
	manifest := readManifest(t, outputDir)
	assert.Equal(t, 3, manifest.PagesCrawled, "should crawl all 3 pages after retries")
	assert.Equal(t, 0, manifest.Errors)
}

func TestIntegrationScopeFiltering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	const docsPageHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Docs Page 1 - MyDocs</title>
  <meta name="description" content="Documentation page one.">
</head>
<body>
  <nav><a href="/docs/page1">Docs 1</a> | <a href="/docs/page2">Docs 2</a> | <a href="/blog/post1">Blog</a></nav>
  <main>
    <h1>Documentation Page 1</h1>
    <p>This is the first documentation page with detailed instructions for installation and setup.</p>
    <h2>Installation</h2>
    <p>Run the installer and follow the on-screen prompts to complete setup.</p>
  </main>
  <footer><p>&copy; 2026 MyDocs</p></footer>
</body>
</html>`

	const docsPage2HTML = `<!DOCTYPE html>
<html>
<head>
  <title>Docs Page 2 - MyDocs</title>
  <meta name="description" content="Documentation page two.">
</head>
<body>
  <nav><a href="/docs/page1">Docs 1</a> | <a href="/docs/page2">Docs 2</a> | <a href="/blog/post1">Blog</a></nav>
  <main>
    <h1>Documentation Page 2</h1>
    <p>This is the second documentation page covering advanced configuration and troubleshooting.</p>
    <h2>Advanced Configuration</h2>
    <p>Modify the config file to customize behavior for your environment.</p>
  </main>
  <footer><p>&copy; 2026 MyDocs</p></footer>
</body>
</html>`

	const blogPostHTML = `<!DOCTYPE html>
<html>
<head>
  <title>Blog Post 1 - MyDocs</title>
  <meta name="description" content="A blog post about our latest features.">
</head>
<body>
  <nav><a href="/docs/page1">Docs 1</a> | <a href="/blog/post1">Blog</a></nav>
  <main>
    <h1>Blog Post: New Features</h1>
    <p>We are excited to announce several new features in this release.</p>
  </main>
  <footer><p>&copy; 2026 MyDocs</p></footer>
</body>
</html>`

	mux := http.NewServeMux()

	mux.HandleFunc("GET /docs/page1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, docsPageHTML)
	})
	mux.HandleFunc("GET /docs/page2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, docsPage2HTML)
	})
	mux.HandleFunc("GET /blog/post1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, blogPostHTML)
	})
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, robotsTXT)
	})

	server := httptest.NewServer(mux)

	// Sitemap lists all pages including blog.
	mux.HandleFunc("GET /sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		sitemapXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>%s/docs/page1</loc></url>
  <url><loc>%s/docs/page2</loc></url>
  <url><loc>%s/blog/post1</loc></url>
</urlset>`, server.URL, server.URL, server.URL)
		fmt.Fprint(w, sitemapXML)
	})

	t.Cleanup(server.Close)

	outputDir := t.TempDir()

	// Scope only allows /docs/* paths.
	scopeCfg := scope.ScopeConfig{
		Prefix:     server.URL,
		SameDomain: true,
		Includes:   []string{"/docs/*"},
	}

	e := buildPipeline(t, server.URL, outputDir, scopeCfg)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg := config.NewConfig(server.URL + "/docs/page1").
		WithOutputDir(outputDir).
		WithWorkers(2)

	err := e.Run(ctx, cfg)
	require.NoError(t, err)

	// Only docs pages should be crawled, not the blog.
	manifest := readManifest(t, outputDir)
	assert.Equal(t, 2, manifest.PagesCrawled, "should crawl only 2 docs pages")
	assert.Equal(t, 0, manifest.Errors)

	// Verify docs pages exist.
	pagesDir := filepath.Join(outputDir, "pages")
	for _, relPath := range []string{"docs/page1.md", "docs/page2.md"} {
		mdPath := filepath.Join(pagesDir, relPath)
		content, err := os.ReadFile(mdPath)
		require.NoError(t, err, "%s should exist", relPath)
		assert.NotEmpty(t, string(content), "%s should have content", relPath)
	}

	// Verify blog page does NOT exist.
	blogPath := filepath.Join(pagesDir, "blog", "post1.md")
	_, err = os.ReadFile(blogPath)
	assert.True(t, os.IsNotExist(err), "blog post should NOT have been crawled")
}
