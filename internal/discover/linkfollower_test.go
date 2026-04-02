package discover

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/scope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLinkFollower_Name(t *testing.T) {
	lf := NewLinkFollower(scope.NewScope(scope.ScopeConfig{}))
	assert.Equal(t, "link-follower", lf.Name())
}

func TestLinkFollower_Discover_EmitsSeed(t *testing.T) {
	s := scope.NewScope(scope.ScopeConfig{})
	lf := NewLinkFollower(s)

	seed, err := url.Parse("https://docs.example.com/getting-started")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Read the seed URL from the channel.
	cu := <-ch
	assert.Equal(t, seed.String(), cu.URL.String())
	assert.Equal(t, 0, cu.Depth)
	assert.Equal(t, pipeline.SourceSeed, cu.Source)
	assert.Equal(t, "link-follower", cu.DiscoveredBy)

	lf.Close()

	// Channel should be closed after Close.
	_, open := <-ch
	assert.False(t, open, "channel should be closed after Close()")
}

func TestLinkFollower_Feed_ExtractsLinks(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain the seed.
	<-ch

	html := []byte(`<html><body>
		<a href="/docs/api">API</a>
		<a href="/docs/guide">Guide</a>
		<a href="https://external.com/page">External</a>
	</body></html>`)

	parentURL, _ := url.Parse(baseURL + "/")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	// Should have /docs/api and /docs/guide (external is out of scope with prefix).
	gotPaths := make([]string, 0, len(got))
	for _, cu := range got {
		gotPaths = append(gotPaths, cu.URL.Path)
	}
	sort.Strings(gotPaths)

	assert.Equal(t, []string{"/docs/api", "/docs/guide"}, gotPaths)

	for _, cu := range got {
		assert.Equal(t, pipeline.SourceLink, cu.Source)
		assert.Equal(t, 1, cu.Depth)
		assert.Equal(t, "link-follower", cu.DiscoveredBy)
	}
}

func TestLinkFollower_Feed_Deduplication(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	html := []byte(`<html><body>
		<a href="/docs/api">API</a>
		<a href="/docs/api">API again</a>
		<a href="/docs/api#section">API with fragment</a>
	</body></html>`)

	parentURL, _ := url.Parse(baseURL + "/")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	// All three hrefs resolve to the same normalized URL, so only one emitted.
	assert.Len(t, got, 1)
	assert.Equal(t, "/docs/api", got[0].URL.Path)
}

func TestLinkFollower_Feed_ScopeFiltering(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{
		Prefix:   baseURL,
		Excludes: []string{"/internal/*"},
	})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	html := []byte(`<html><body>
		<a href="/docs/public">Public</a>
		<a href="/internal/secret">Internal</a>
	</body></html>`)

	parentURL, _ := url.Parse(baseURL + "/")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	require.Len(t, got, 1)
	assert.Equal(t, "/docs/public", got[0].URL.Path)
}

func TestLinkFollower_Feed_DepthIncrement(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed (depth 0).
	<-ch

	html := []byte(`<html><body><a href="/page-a">A</a></body></html>`)
	parentURL, _ := url.Parse(baseURL + "/start")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 3, pipeline.SourceLink, "link-follower"),
		Body:     html,
	}

	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	require.Len(t, got, 1)
	assert.Equal(t, 4, got[0].Depth, "depth should be parent depth + 1")
}

func TestLinkFollower_Feed_ConcurrentSafety(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	// Feed from multiple goroutines concurrently.
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			html := []byte(fmt.Sprintf(
				`<html><body><a href="/page-%d">Page %d</a></body></html>`, idx, idx,
			))
			parentURL, _ := url.Parse(baseURL + "/")
			result := pipeline.FetchResult{
				CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
				Body:     html,
			}
			lf.Feed(result)
		}(i)
	}

	go func() {
		wg.Wait()
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	// Each goroutine feeds a unique link, so we should get all 10.
	assert.Len(t, got, 10)

	// Verify no duplicates.
	seen := make(map[string]bool)
	for _, cu := range got {
		key := cu.URL.String()
		assert.False(t, seen[key], "duplicate URL: %s", key)
		seen[key] = true
	}
}

func TestLinkFollower_Feed_SeedNotDuplicated(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	// Feed a page that links back to the seed URL.
	html := []byte(`<html><body>
		<a href="/">Home</a>
		<a href="/new-page">New</a>
	</body></html>`)

	parentURL, _ := url.Parse(baseURL + "/start")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	// Only /new-page should appear; / was already emitted as the seed.
	require.Len(t, got, 1)
	assert.Equal(t, "/new-page", got[0].URL.Path)
}

func TestLinkFollower_Feed_ManyLinksNonBlocking(t *testing.T) {
	// Regression test: Feed with more links than the channel buffer (64)
	// must not deadlock. Before the fix, Feed held the mutex while sending
	// on the channel, causing all concurrent Feed callers to block.
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	// Build a page with 100 unique links (exceeds 64-item channel buffer).
	var links string
	for i := range 100 {
		links += fmt.Sprintf(`<a href="/page/%d">Page %d</a>`, i, i)
	}
	html := []byte("<html><body>" + links + "</body></html>")

	parentURL, _ := url.Parse(baseURL + "/")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	// Feed in one goroutine while concurrent Feed calls from others.
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		lf.Feed(result)
	}()

	// Simulate concurrent fetch workers calling Feed simultaneously.
	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			smallHTML := []byte(fmt.Sprintf(
				`<html><body><a href="/extra/%d">Extra</a></body></html>`, idx,
			))
			r := pipeline.FetchResult{
				CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
				Body:     smallHTML,
			}
			lf.Feed(r)
		}(i)
	}

	go func() {
		wg.Wait()
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	// 100 from the main page + 5 extra pages = 105.
	assert.Len(t, got, 105)
}

func TestLinkFollower_Close_DuringFeed(t *testing.T) {
	// Regression test: Close must not panic even when Feed goroutines are
	// actively enqueuing items.
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	// Build a page with many links.
	var links string
	for i := range 200 {
		links += fmt.Sprintf(`<a href="/page/%d">Page %d</a>`, i, i)
	}
	html := []byte("<html><body>" + links + "</body></html>")
	parentURL, _ := url.Parse(baseURL + "/")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	// Feed in background.
	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	// Consumer keeps reading until channel closes — all 200 items arrive.
	var got []pipeline.CrawlURL
	assert.NotPanics(t, func() {
		for cu := range ch {
			got = append(got, cu)
		}
	})

	assert.Len(t, got, 200)
}

func TestLinkFollower_Feed_IgnoresNonHTTP(t *testing.T) {
	baseURL := "https://docs.example.com"
	s := scope.NewScope(scope.ScopeConfig{Prefix: baseURL})
	lf := NewLinkFollower(s)

	seed, err := url.Parse(baseURL + "/")
	require.NoError(t, err)

	ch, err := lf.Discover(context.Background(), seed)
	require.NoError(t, err)

	// Drain seed.
	<-ch

	html := []byte(`<html><body>
		<a href="mailto:test@example.com">Email</a>
		<a href="javascript:void(0)">JS</a>
		<a href="ftp://files.example.com/doc">FTP</a>
		<a href="/valid-page">Valid</a>
	</body></html>`)

	parentURL, _ := url.Parse(baseURL + "/")
	result := pipeline.FetchResult{
		CrawlURL: pipeline.NewCrawlURL(parentURL, 0, pipeline.SourceSeed, "link-follower"),
		Body:     html,
	}

	go func() {
		lf.Feed(result)
		lf.Close()
	}()

	var got []pipeline.CrawlURL
	for cu := range ch {
		got = append(got, cu)
	}

	require.Len(t, got, 1)
	assert.Equal(t, "/valid-page", got[0].URL.Path)
}
