package discover

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/napkin/docs-crawler/internal/pipeline"
	"github.com/napkin/docs-crawler/internal/scope"
)

// sitemapURLSet represents the standard <urlset> sitemap format.
type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	URLs    []sitemapLoc `xml:"url"`
}

// sitemapIndex represents the <sitemapindex> format that references sub-sitemaps.
type sitemapIndex struct {
	XMLName  xml.Name     `xml:"sitemapindex"`
	Sitemaps []sitemapLoc `xml:"sitemap"`
}

// sitemapLoc holds a single <loc> element from a sitemap.
type sitemapLoc struct {
	Loc string `xml:"loc"`
}

// SitemapDiscoverer discovers URLs by parsing a site's sitemap.xml.
type SitemapDiscoverer struct {
	scope  scope.Scope
	client *http.Client
}

// NewSitemapDiscoverer creates a SitemapDiscoverer that filters URLs through
// the given scope.
func NewSitemapDiscoverer(s scope.Scope) *SitemapDiscoverer {
	return &SitemapDiscoverer{
		scope:  s,
		client: http.DefaultClient,
	}
}

// newSitemapDiscovererWithClient creates a SitemapDiscoverer using a custom
// HTTP client, primarily for testing.
func newSitemapDiscovererWithClient(s scope.Scope, client *http.Client) *SitemapDiscoverer {
	return &SitemapDiscoverer{
		scope:  s,
		client: client,
	}
}

// Name returns the discoverer identifier.
func (d *SitemapDiscoverer) Name() string {
	return "sitemap"
}

// Discover fetches sitemap.xml from the seed URL's root and emits allowed URLs.
// If the sitemap is missing or unparseable the returned channel is closed
// immediately with no error -- the discoverer simply finds nothing.
func (d *SitemapDiscoverer) Discover(ctx context.Context, seed *url.URL) (<-chan pipeline.CrawlURL, error) {
	ch := make(chan pipeline.CrawlURL)

	sitemapURL := buildSitemapURL(seed)

	go func() {
		defer close(ch)

		locs, err := d.fetchSitemapLocs(ctx, sitemapURL)
		if err != nil {
			slog.Info("sitemap unavailable, skipping", "url", sitemapURL, "error", err)
			return
		}

		d.emitAllowed(ctx, ch, locs)
	}()

	return ch, nil
}

// buildSitemapURL constructs the sitemap.xml URL from the seed's scheme and host.
func buildSitemapURL(seed *url.URL) string {
	return fmt.Sprintf("%s://%s/sitemap.xml", seed.Scheme, seed.Host)
}

// fetchSitemapLocs retrieves and parses the sitemap, returning raw location strings.
// It supports both standard urlset and sitemapindex formats.
func (d *SitemapDiscoverer) fetchSitemapLocs(ctx context.Context, sitemapURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", sitemapURL, err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", sitemapURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("sitemap not found (404)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d for %s", resp.StatusCode, sitemapURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading body of %s: %w", sitemapURL, err)
	}

	return parseSitemapBody(body)
}

// parseSitemapBody tries to decode the body as a standard urlset first, then
// as a sitemapindex.
func parseSitemapBody(body []byte) ([]string, error) {
	locs, err := parseURLSet(body)
	if err == nil && len(locs) > 0 {
		return locs, nil
	}

	locs, err = parseSitemapIndex(body)
	if err == nil && len(locs) > 0 {
		return locs, nil
	}

	return nil, fmt.Errorf("body is neither a valid urlset nor sitemapindex")
}

// parseURLSet decodes a standard <urlset> document.
func parseURLSet(data []byte) ([]string, error) {
	var us sitemapURLSet
	if err := xml.Unmarshal(data, &us); err != nil {
		return nil, err
	}

	locs := make([]string, 0, len(us.URLs))
	for _, u := range us.URLs {
		locs = append(locs, u.Loc)
	}
	return locs, nil
}

// parseSitemapIndex decodes a <sitemapindex> document and collects the sub-sitemap locations.
func parseSitemapIndex(data []byte) ([]string, error) {
	var si sitemapIndex
	if err := xml.Unmarshal(data, &si); err != nil {
		return nil, err
	}

	locs := make([]string, 0, len(si.Sitemaps))
	for _, s := range si.Sitemaps {
		locs = append(locs, s.Loc)
	}
	return locs, nil
}

// emitAllowed parses each location string, filters it through scope, and sends
// it on the channel.
func (d *SitemapDiscoverer) emitAllowed(ctx context.Context, ch chan<- pipeline.CrawlURL, locs []string) {
	for _, loc := range locs {
		parsed, err := url.Parse(loc)
		if err != nil {
			slog.Debug("skipping invalid sitemap loc", "loc", loc, "error", err)
			continue
		}

		if !d.scope.IsAllowed(parsed, 0) {
			slog.Debug("sitemap URL filtered by scope", "url", loc)
			continue
		}

		crawlURL := pipeline.NewCrawlURL(parsed, 0, pipeline.SourceSitemap, "sitemap")

		select {
		case <-ctx.Done():
			return
		case ch <- crawlURL:
		}
	}
}
