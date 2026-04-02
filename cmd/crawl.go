package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mayur19/docs-crawler/internal/config"
	"github.com/mayur19/docs-crawler/internal/discover"
	"github.com/mayur19/docs-crawler/internal/engine"
	"github.com/mayur19/docs-crawler/internal/extract"
	"github.com/mayur19/docs-crawler/internal/fetch"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/ratelimit"
	"github.com/mayur19/docs-crawler/internal/scope"
	"github.com/mayur19/docs-crawler/internal/writer"
	"github.com/spf13/cobra"
)

var crawlCmd = &cobra.Command{
	Use:   "crawl [url]",
	Short: "Crawl a documentation website",
	Long:  `Crawl a documentation website and output clean Markdown with metadata.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runCrawl,
}

func init() {
	rootCmd.AddCommand(crawlCmd)

	crawlCmd.Flags().StringP("output", "o", "./docs-output", "Output directory")
	crawlCmd.Flags().Float64("rate-limit", 0, "Requests per second (0 = auto-detect)")
	crawlCmd.Flags().Int("workers", 10, "Concurrent fetch workers")
	crawlCmd.Flags().Int("max-depth", 0, "Max link depth (0 = unlimited)")
	crawlCmd.Flags().StringSlice("include", nil, "URL include glob patterns")
	crawlCmd.Flags().StringSlice("exclude", nil, "URL exclude glob patterns")
	crawlCmd.Flags().Bool("use-browser", false, "Enable headless Chrome for JS-rendered sites")
	crawlCmd.Flags().String("user-agent", "docs-crawler/0.1.0", "Custom User-Agent string")
	crawlCmd.Flags().Duration("timeout", 30*time.Second, "Per-request timeout")
	crawlCmd.Flags().Bool("resume", false, "Resume from previous crawl state")
}

func runCrawl(cmd *cobra.Command, args []string) error {
	cfg, err := buildConfig(cmd, args[0])
	if err != nil {
		return err
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	slog.Info("starting crawl",
		"seed_url", cfg.SeedURL,
		"output", cfg.OutputDir,
		"workers", cfg.Workers,
	)

	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	return executeCrawl(ctx, cfg)
}

func buildConfig(cmd *cobra.Command, seedURL string) (config.Config, error) {
	cfg := config.NewConfig(seedURL)

	flags := cmd.Flags()
	output, _ := flags.GetString("output")
	rateLimit, _ := flags.GetFloat64("rate-limit")
	workers, _ := flags.GetInt("workers")
	maxDepth, _ := flags.GetInt("max-depth")
	includes, _ := flags.GetStringSlice("include")
	excludes, _ := flags.GetStringSlice("exclude")
	useBrowser, _ := flags.GetBool("use-browser")
	userAgent, _ := flags.GetString("user-agent")
	timeout, _ := flags.GetDuration("timeout")
	resume, _ := flags.GetBool("resume")

	cfg = cfg.
		WithOutputDir(output).
		WithRateLimit(rateLimit).
		WithWorkers(workers).
		WithMaxDepth(maxDepth).
		WithIncludes(includes).
		WithExcludes(excludes).
		WithUseBrowser(useBrowser).
		WithUserAgent(userAgent).
		WithTimeout(timeout).
		WithResume(resume).
		WithVerbose(verbose)

	return cfg, nil
}

func executeCrawl(ctx context.Context, cfg config.Config) error {
	// Build scope.
	s := scope.NewScope(scope.ScopeConfig{
		Prefix:     cfg.SeedURL,
		Includes:   cfg.Includes,
		Excludes:   cfg.Excludes,
		MaxDepth:   cfg.MaxDepth,
		SameDomain: true,
	})

	// Build rate limiter.
	limiter := ratelimit.NewAdaptiveLimiter(ratelimit.LimiterConfig{
		ExplicitRate: cfg.RateLimit,
		DefaultRate:  5.0,
	})

	// Build discoverers.
	sitemapDisc := discover.NewSitemapDiscoverer(s)
	linkFollower := discover.NewLinkFollower(s)
	discoverers := []pipeline.Discoverer{sitemapDisc, linkFollower}

	// Build fetchers.
	httpClient := &http.Client{Timeout: cfg.Timeout}
	httpFetcher := fetch.NewHTTPFetcher(httpClient, limiter, cfg.UserAgent)
	fetchers := []pipeline.Fetcher{httpFetcher}

	if cfg.UseBrowser {
		browserFetcher := fetch.NewBrowserFetcher(limiter, cfg.UserAgent)
		fetchers = append(fetchers, browserFetcher)
	}

	// Build extractors.
	readabilityExtractor := extract.NewReadabilityExtractor()
	extractors := []pipeline.Extractor{readabilityExtractor}

	// Build writers.
	mdWriter, err := writer.NewMarkdownWriter(cfg.OutputDir, cfg.SeedURL)
	if err != nil {
		return fmt.Errorf("create writer: %w", err)
	}
	writers := []pipeline.Writer{mdWriter}

	// Build deduplicator.
	dedup := config.NewDeduplicator()

	// Build and run engine.
	pools := engine.PoolSizes{
		Discovery: 2,
		Fetch:     cfg.Workers,
		Extract:   cfg.Workers / 2,
		Write:     3,
	}
	if pools.Extract < 1 {
		pools.Extract = 1
	}

	e := engine.New(discoverers, fetchers, extractors, writers, linkFollower, dedup, pools)

	return e.Run(ctx, cfg)
}
