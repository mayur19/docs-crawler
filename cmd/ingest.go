package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/mayur19/docs-crawler/internal/chunk"
	"github.com/mayur19/docs-crawler/internal/config"
	"github.com/mayur19/docs-crawler/internal/discover"
	"github.com/mayur19/docs-crawler/internal/embed"
	"github.com/mayur19/docs-crawler/internal/engine"
	"github.com/mayur19/docs-crawler/internal/extract"
	"github.com/mayur19/docs-crawler/internal/fetch"
	"github.com/mayur19/docs-crawler/internal/index"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/ratelimit"
	"github.com/mayur19/docs-crawler/internal/scope"
	"github.com/mayur19/docs-crawler/internal/writer"
	"github.com/spf13/cobra"
)

var ingestCmd = &cobra.Command{
	Use:   "ingest [url]",
	Short: "Crawl, chunk, embed, and index documentation",
	Long:  "Crawl a documentation site and build a searchable knowledge base with vector embeddings.",
	Args:  cobra.ExactArgs(1),
	RunE:  runIngest,
}

func init() {
	rootCmd.AddCommand(ingestCmd)

	// Shared crawl flags.
	ingestCmd.Flags().StringP("output", "o", "./docs-output", "Output directory")
	ingestCmd.Flags().Float64("rate-limit", 0, "Requests per second (0 = auto-detect)")
	ingestCmd.Flags().Int("workers", 10, "Concurrent fetch workers")
	ingestCmd.Flags().Int("max-depth", 0, "Max link depth (0 = unlimited)")
	ingestCmd.Flags().StringSlice("include", nil, "URL include glob patterns")
	ingestCmd.Flags().StringSlice("exclude", nil, "URL exclude glob patterns")
	ingestCmd.Flags().Bool("use-browser", false, "Enable headless Chrome for JS-rendered sites")
	ingestCmd.Flags().String("user-agent", "docs-crawler/0.1.0", "Custom User-Agent string")
	ingestCmd.Flags().Duration("timeout", 30*time.Second, "Per-request timeout")
	ingestCmd.Flags().Bool("resume", false, "Resume from previous crawl state")

	// Ingest-specific flags.
	ingestCmd.Flags().String("chunk-strategy", "heading", "Chunking strategy: heading or fixed")
	ingestCmd.Flags().Int("max-tokens", 512, "Maximum tokens per chunk")
	ingestCmd.Flags().String("embedder", "auto", "Embedder: auto, ollama, openai, cohere, tfidf")
	ingestCmd.Flags().String("embedding-model", "nomic-embed-text", "Embedding model name")
	ingestCmd.Flags().Int("embedding-batch", 64, "Embedding batch size")
	ingestCmd.Flags().String("config", "", "Path to YAML config file")
}

func runIngest(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()
	configPath, _ := flags.GetString("config")

	var (
		cfg config.Config
		err error
	)

	if configPath != "" {
		cfg, err = config.LoadYAML(configPath)
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		// Override with any explicitly set CLI flags.
		cfg = applyIngestFlagOverrides(cmd, cfg)
	} else {
		cfg, err = buildIngestConfig(cmd, args[0])
		if err != nil {
			return err
		}
	}

	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	slog.Info("starting ingest",
		"seed_url", cfg.SeedURL,
		"output", cfg.OutputDir,
		"workers", cfg.Workers,
		"chunker", cfg.ChunkStrategy,
		"embedder", cfg.Embedder,
	)

	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	return executeIngest(ctx, cfg)
}

// buildIngestConfig builds a Config from CLI flags for the ingest command.
func buildIngestConfig(cmd *cobra.Command, seedURL string) (config.Config, error) {
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
	chunkStrategy, _ := flags.GetString("chunk-strategy")
	maxTokens, _ := flags.GetInt("max-tokens")
	embedder, _ := flags.GetString("embedder")
	embeddingModel, _ := flags.GetString("embedding-model")
	embeddingBatch, _ := flags.GetInt("embedding-batch")

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
		WithVerbose(verbose).
		WithChunkStrategy(chunkStrategy).
		WithMaxTokens(maxTokens).
		WithEmbedder(embedder).
		WithEmbeddingModel(embeddingModel).
		WithEmbeddingBatch(embeddingBatch)

	return cfg, nil
}

// applyIngestFlagOverrides overlays CLI flags on top of a YAML-loaded config.
// Only flags that were explicitly set by the user override the config file values.
func applyIngestFlagOverrides(cmd *cobra.Command, cfg config.Config) config.Config {
	flags := cmd.Flags()

	if flags.Changed("output") {
		v, _ := flags.GetString("output")
		cfg = cfg.WithOutputDir(v)
	}
	if flags.Changed("rate-limit") {
		v, _ := flags.GetFloat64("rate-limit")
		cfg = cfg.WithRateLimit(v)
	}
	if flags.Changed("workers") {
		v, _ := flags.GetInt("workers")
		cfg = cfg.WithWorkers(v)
	}
	if flags.Changed("max-depth") {
		v, _ := flags.GetInt("max-depth")
		cfg = cfg.WithMaxDepth(v)
	}
	if flags.Changed("include") {
		v, _ := flags.GetStringSlice("include")
		cfg = cfg.WithIncludes(v)
	}
	if flags.Changed("exclude") {
		v, _ := flags.GetStringSlice("exclude")
		cfg = cfg.WithExcludes(v)
	}
	if flags.Changed("use-browser") {
		v, _ := flags.GetBool("use-browser")
		cfg = cfg.WithUseBrowser(v)
	}
	if flags.Changed("user-agent") {
		v, _ := flags.GetString("user-agent")
		cfg = cfg.WithUserAgent(v)
	}
	if flags.Changed("timeout") {
		v, _ := flags.GetDuration("timeout")
		cfg = cfg.WithTimeout(v)
	}
	if flags.Changed("resume") {
		v, _ := flags.GetBool("resume")
		cfg = cfg.WithResume(v)
	}
	if flags.Changed("chunk-strategy") {
		v, _ := flags.GetString("chunk-strategy")
		cfg = cfg.WithChunkStrategy(v)
	}
	if flags.Changed("max-tokens") {
		v, _ := flags.GetInt("max-tokens")
		cfg = cfg.WithMaxTokens(v)
	}
	if flags.Changed("embedder") {
		v, _ := flags.GetString("embedder")
		cfg = cfg.WithEmbedder(v)
	}
	if flags.Changed("embedding-model") {
		v, _ := flags.GetString("embedding-model")
		cfg = cfg.WithEmbeddingModel(v)
	}
	if flags.Changed("embedding-batch") {
		v, _ := flags.GetInt("embedding-batch")
		cfg = cfg.WithEmbeddingBatch(v)
	}

	cfg = cfg.WithVerbose(verbose)
	return cfg
}

func executeIngest(ctx context.Context, cfg config.Config) error {
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

	// Build writers (also write Markdown for the ingest pipeline).
	mdWriter, err := writer.NewMarkdownWriter(cfg.OutputDir, cfg.SeedURL)
	if err != nil {
		return fmt.Errorf("create writer: %w", err)
	}
	writers := []pipeline.Writer{mdWriter}

	// Build deduplicator.
	dedup := config.NewDeduplicator()

	// Fetch and parse robots.txt; failures are non-fatal (permissive by default).
	seedParsed, err := url.Parse(cfg.SeedURL)
	if err != nil {
		return fmt.Errorf("parsing seed URL: %w", err)
	}

	var robotsChecker *scope.RobotsChecker
	robotsData, robotsErr := scope.FetchRobots(ctx, seedParsed, httpClient)
	if robotsErr != nil {
		slog.Warn("could not fetch robots.txt, crawling without restrictions",
			"error", robotsErr)
	} else {
		checker, parseErr := scope.NewRobotsChecker(robotsData, cfg.UserAgent)
		if parseErr != nil {
			slog.Warn("could not parse robots.txt, crawling without restrictions",
				"error", parseErr)
		} else {
			robotsChecker = &checker
		}
	}

	// Build pool sizes.
	pools := engine.PoolSizes{
		Discovery: 2,
		Fetch:     cfg.Workers,
		Extract:   cfg.Workers / 2,
		Write:     3,
		Chunk:     3,
		Embed:     2,
		Index:     1,
	}
	if pools.Extract < 1 {
		pools.Extract = 1
	}

	// Build chunker.
	chunker, err := buildChunker(cfg.ChunkStrategy, cfg.MaxTokens)
	if err != nil {
		return err
	}

	// Build embedder.
	embedder, err := buildEmbedder(cfg)
	if err != nil {
		return err
	}

	// Build SQLite store.
	dbPath := filepath.Join(cfg.OutputDir, "index.db")
	store, err := index.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("create index store: %w", err)
	}

	// Build engine and run.
	e := engine.New(discoverers, fetchers, extractors, writers, linkFollower, dedup, pools, robotsChecker)

	if err := e.RunIngest(ctx, cfg, chunker, embedder, store); err != nil {
		return err
	}

	fmt.Printf("\nIngest complete:\n")
	fmt.Printf("  Embedder : %s\n", embedder.Name())
	fmt.Printf("  Index    : %s\n", dbPath)
	return nil
}

// buildChunker creates a Chunker based on the strategy name.
func buildChunker(strategy string, maxTokens int) (pipeline.Chunker, error) {
	switch strategy {
	case "heading", "":
		return chunk.NewHeadingChunker(maxTokens), nil
	case "fixed":
		return chunk.NewFixedChunker(maxTokens, 50), nil
	default:
		return nil, fmt.Errorf("unknown chunk strategy %q: must be heading or fixed", strategy)
	}
}

// buildEmbedder creates a pipeline.Embedder based on cfg.Embedder.
func buildEmbedder(cfg config.Config) (pipeline.Embedder, error) {
	switch cfg.Embedder {
	case "auto", "":
		ollamaURL := os.Getenv("OLLAMA_HOST")
		if ollamaURL == "" {
			ollamaURL = "http://localhost:11434"
		}
		openAIKey := os.Getenv("OPENAI_API_KEY")
		cohereKey := os.Getenv("COHERE_API_KEY")
		return embed.AutoDetect(ollamaURL, openAIKey, cohereKey), nil

	case "ollama":
		ollamaURL := os.Getenv("OLLAMA_HOST")
		if ollamaURL == "" {
			ollamaURL = "http://localhost:11434"
		}
		return embed.NewOllamaEmbedder(ollamaURL, cfg.EmbeddingModel), nil

	case "openai":
		apiKey := os.Getenv("OPENAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("OPENAI_API_KEY environment variable is required for openai embedder")
		}
		return embed.NewOpenAIEmbedder("https://api.openai.com", apiKey, cfg.EmbeddingModel), nil

	case "cohere":
		apiKey := os.Getenv("COHERE_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("COHERE_API_KEY environment variable is required for cohere embedder")
		}
		return embed.NewCohereEmbedder("https://api.cohere.com", apiKey, cfg.EmbeddingModel), nil

	case "tfidf":
		return embed.NewTFIDFEmbedder(), nil

	default:
		return nil, fmt.Errorf("unknown embedder %q: must be auto, ollama, openai, cohere, or tfidf", cfg.Embedder)
	}
}
