package discover

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSitemapDiscoverer_Name(t *testing.T) {
	d := NewSitemapDiscoverer(scope.NewScope(scope.ScopeConfig{}))
	assert.Equal(t, "sitemap", d.Name())
}

func TestSitemapDiscoverer_Discover(t *testing.T) {
	tests := []struct {
		name         string
		status       int
		bodyTemplate string
		scopeCfg     scope.ScopeConfig
		wantPaths    []string
	}{
		{
			name:   "standard urlset format",
			status: http.StatusOK,
			bodyTemplate: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>{{BASE}}/docs/getting-started</loc></url>
  <url><loc>{{BASE}}/docs/api</loc></url>
  <url><loc>{{BASE}}/docs/faq</loc></url>
</urlset>`,
			scopeCfg: scope.ScopeConfig{},
			wantPaths: []string{
				"/docs/api",
				"/docs/faq",
				"/docs/getting-started",
			},
		},
		{
			name:   "sitemapindex format",
			status: http.StatusOK,
			bodyTemplate: `<?xml version="1.0" encoding="UTF-8"?>
<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <sitemap><loc>{{BASE}}/sitemap-docs.xml</loc></sitemap>
  <sitemap><loc>{{BASE}}/sitemap-blog.xml</loc></sitemap>
</sitemapindex>`,
			scopeCfg: scope.ScopeConfig{},
			wantPaths: []string{
				"/sitemap-blog.xml",
				"/sitemap-docs.xml",
			},
		},
		{
			name:         "404 returns empty channel",
			status:       http.StatusNotFound,
			bodyTemplate: "not found",
			scopeCfg:     scope.ScopeConfig{},
			wantPaths:    nil,
		},
		{
			name:   "scope filtering excludes non-matching URLs",
			status: http.StatusOK,
			bodyTemplate: `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>{{BASE}}/docs/guide</loc></url>
  <url><loc>{{BASE}}/blog/post-1</loc></url>
  <url><loc>{{BASE}}/docs/reference</loc></url>
</urlset>`,
			scopeCfg: scope.ScopeConfig{
				Excludes: []string{"/blog/*"},
			},
			wantPaths: []string{
				"/docs/guide",
				"/docs/reference",
			},
		},
		{
			name:         "invalid XML returns empty channel",
			status:       http.StatusOK,
			bodyTemplate: `<not-a-sitemap>broken`,
			scopeCfg:     scope.ScopeConfig{},
			wantPaths:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use atomic pointer so the handler can read the base URL
			// after the server starts.
			var baseURL atomic.Value
			baseURL.Store("")

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				base := baseURL.Load().(string)
				body := strings.ReplaceAll(tt.bodyTemplate, "{{BASE}}", base)
				w.Header().Set("Content-Type", "application/xml")
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			baseURL.Store(srv.URL)

			cfg := tt.scopeCfg
			if cfg.Prefix == "" {
				cfg.Prefix = srv.URL
			}
			s := scope.NewScope(cfg)

			d := newSitemapDiscovererWithClient(s, srv.Client())

			seed, err := url.Parse(srv.URL + "/docs")
			require.NoError(t, err)

			ch, err := d.Discover(context.Background(), seed)
			require.NoError(t, err)

			got := drainChannel(ch)
			gotPaths := extractPaths(got, srv.URL)

			sort.Strings(gotPaths)
			sort.Strings(tt.wantPaths)

			assert.Equal(t, tt.wantPaths, gotPaths)

			for _, cu := range got {
				assert.Equal(t, pipeline.SourceSitemap, cu.Source)
				assert.Equal(t, 0, cu.Depth)
				assert.Equal(t, "sitemap", cu.DiscoveredBy)
			}
		})
	}
}

func TestSitemapDiscoverer_ContextCancellation(t *testing.T) {
	var baseURL atomic.Value
	baseURL.Store("")

	bodyTemplate := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
  <url><loc>{{BASE}}/page1</loc></url>
  <url><loc>{{BASE}}/page2</loc></url>
  <url><loc>{{BASE}}/page3</loc></url>
</urlset>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := baseURL.Load().(string)
		body := strings.ReplaceAll(bodyTemplate, "{{BASE}}", base)
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	baseURL.Store(srv.URL)

	s := scope.NewScope(scope.ScopeConfig{Prefix: srv.URL})
	d := newSitemapDiscovererWithClient(s, srv.Client())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	seed, err := url.Parse(srv.URL)
	require.NoError(t, err)

	ch, err := d.Discover(ctx, seed)
	require.NoError(t, err)

	got := drainChannel(ch)
	// With an immediately cancelled context we may get 0 or some URLs
	// depending on timing, but we must not hang.
	assert.LessOrEqual(t, len(got), 3)
}

// --- helpers ---

func drainChannel(ch <-chan pipeline.CrawlURL) []pipeline.CrawlURL {
	var results []pipeline.CrawlURL
	for cu := range ch {
		results = append(results, cu)
	}
	return results
}

func extractPaths(urls []pipeline.CrawlURL, base string) []string {
	if len(urls) == 0 {
		return nil
	}
	paths := make([]string, 0, len(urls))
	for _, cu := range urls {
		raw := cu.URL.String()
		if len(raw) > len(base) {
			paths = append(paths, raw[len(base):])
		} else {
			paths = append(paths, raw)
		}
	}
	return paths
}
