package config

import (
	"errors"
	"fmt"
	"net/url"
	"time"
)

// Config holds all CLI configuration fields for the crawler.
// Config is immutable: use With* methods to create modified copies.
type Config struct {
	SeedURL    string
	OutputDir  string
	RateLimit  float64
	Workers    int
	MaxDepth   int
	Includes   []string
	Excludes   []string
	UseBrowser bool
	UserAgent  string
	Timeout    time.Duration
	Resume     bool
	Verbose    bool

	ChunkStrategy  string
	MaxTokens      int
	Embedder       string
	EmbeddingModel string
	EmbeddingBatch int
}

// NewConfig returns a Config with sensible defaults for the given seed URL.
func NewConfig(seedURL string) Config {
	return Config{
		SeedURL:        seedURL,
		OutputDir:      "./docs-output",
		RateLimit:      0,
		Workers:        10,
		MaxDepth:       0,
		UserAgent:      "docs-crawler/0.1.0",
		Timeout:        30 * time.Second,
		ChunkStrategy:  "heading",
		MaxTokens:      512,
		Embedder:       "auto",
		EmbeddingModel: "nomic-embed-text",
		EmbeddingBatch: 64,
	}
}

// Validate checks that all Config fields have acceptable values.
func (c Config) Validate() error {
	var errs []error

	if c.SeedURL == "" {
		errs = append(errs, fmt.Errorf("seed URL must not be empty"))
	} else if _, err := url.ParseRequestURI(c.SeedURL); err != nil {
		errs = append(errs, fmt.Errorf("seed URL is not a valid URL: %w", err))
	}

	if c.Workers <= 0 {
		errs = append(errs, fmt.Errorf("workers must be greater than 0, got %d", c.Workers))
	}

	if c.RateLimit < 0 {
		errs = append(errs, fmt.Errorf("rate limit must be >= 0, got %f", c.RateLimit))
	}

	if c.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("timeout must be greater than 0, got %s", c.Timeout))
	}

	if c.OutputDir == "" {
		errs = append(errs, fmt.Errorf("output directory must not be empty"))
	}

	if c.MaxTokens <= 0 {
		errs = append(errs, fmt.Errorf("max tokens must be greater than 0, got %d", c.MaxTokens))
	}

	validEmbedders := map[string]bool{"auto": true, "ollama": true, "openai": true, "cohere": true, "tfidf": true}
	if !validEmbedders[c.Embedder] {
		errs = append(errs, fmt.Errorf("embedder must be one of auto/ollama/openai/cohere/tfidf, got %q", c.Embedder))
	}

	return errors.Join(errs...)
}

// WithSeedURL returns a new Config with the given seed URL.
func (c Config) WithSeedURL(seedURL string) Config {
	c.SeedURL = seedURL
	return c
}

// WithOutputDir returns a new Config with the given output directory.
func (c Config) WithOutputDir(dir string) Config {
	c.OutputDir = dir
	return c
}

// WithRateLimit returns a new Config with the given rate limit.
func (c Config) WithRateLimit(r float64) Config {
	c.RateLimit = r
	return c
}

// WithWorkers returns a new Config with the given worker count.
func (c Config) WithWorkers(n int) Config {
	c.Workers = n
	return c
}

// WithMaxDepth returns a new Config with the given max depth.
func (c Config) WithMaxDepth(d int) Config {
	c.MaxDepth = d
	return c
}

// WithIncludes returns a new Config with the given include patterns.
func (c Config) WithIncludes(patterns []string) Config {
	copied := make([]string, len(patterns))
	copy(copied, patterns)
	c.Includes = copied
	return c
}

// WithExcludes returns a new Config with the given exclude patterns.
func (c Config) WithExcludes(patterns []string) Config {
	copied := make([]string, len(patterns))
	copy(copied, patterns)
	c.Excludes = copied
	return c
}

// WithUseBrowser returns a new Config with the given browser flag.
func (c Config) WithUseBrowser(use bool) Config {
	c.UseBrowser = use
	return c
}

// WithUserAgent returns a new Config with the given user agent string.
func (c Config) WithUserAgent(ua string) Config {
	c.UserAgent = ua
	return c
}

// WithTimeout returns a new Config with the given timeout.
func (c Config) WithTimeout(t time.Duration) Config {
	c.Timeout = t
	return c
}

// WithResume returns a new Config with the given resume flag.
func (c Config) WithResume(resume bool) Config {
	c.Resume = resume
	return c
}

// WithVerbose returns a new Config with the given verbose flag.
func (c Config) WithVerbose(verbose bool) Config {
	c.Verbose = verbose
	return c
}

// WithChunkStrategy returns a new Config with the given chunk strategy.
func (c Config) WithChunkStrategy(s string) Config { c.ChunkStrategy = s; return c }

// WithMaxTokens returns a new Config with the given max tokens value.
func (c Config) WithMaxTokens(n int) Config { c.MaxTokens = n; return c }

// WithEmbedder returns a new Config with the given embedder.
func (c Config) WithEmbedder(e string) Config { c.Embedder = e; return c }

// WithEmbeddingModel returns a new Config with the given embedding model.
func (c Config) WithEmbeddingModel(m string) Config { c.EmbeddingModel = m; return c }

// WithEmbeddingBatch returns a new Config with the given embedding batch size.
func (c Config) WithEmbeddingBatch(n int) Config { c.EmbeddingBatch = n; return c }
