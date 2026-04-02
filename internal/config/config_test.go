package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfig_Defaults(t *testing.T) {
	cfg := NewConfig("https://example.com/docs")

	assert.Equal(t, "https://example.com/docs", cfg.SeedURL)
	assert.Equal(t, "./docs-output", cfg.OutputDir)
	assert.Equal(t, float64(0), cfg.RateLimit)
	assert.Equal(t, 10, cfg.Workers)
	assert.Equal(t, 0, cfg.MaxDepth)
	assert.Equal(t, "docs-crawler/0.1.0", cfg.UserAgent)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.False(t, cfg.UseBrowser)
	assert.False(t, cfg.Resume)
	assert.False(t, cfg.Verbose)
	assert.Nil(t, cfg.Includes)
	assert.Nil(t, cfg.Excludes)
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  Config
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid config with defaults",
			config:  NewConfig("https://example.com/docs"),
			wantErr: false,
		},
		{
			name:    "valid config with custom workers",
			config:  NewConfig("https://example.com").WithWorkers(5),
			wantErr: false,
		},
		{
			name:    "valid config with rate limit",
			config:  NewConfig("https://example.com").WithRateLimit(2.5),
			wantErr: false,
		},
		{
			name:    "empty seed URL",
			config:  NewConfig(""),
			wantErr: true,
			errMsg:  "seed URL must not be empty",
		},
		{
			name:    "invalid seed URL",
			config:  NewConfig("not-a-url"),
			wantErr: true,
			errMsg:  "seed URL is not a valid URL",
		},
		{
			name:    "zero workers",
			config:  NewConfig("https://example.com").WithWorkers(0),
			wantErr: true,
			errMsg:  "workers must be greater than 0",
		},
		{
			name:    "negative workers",
			config:  NewConfig("https://example.com").WithWorkers(-3),
			wantErr: true,
			errMsg:  "workers must be greater than 0",
		},
		{
			name:    "negative rate limit",
			config:  NewConfig("https://example.com").WithRateLimit(-1.0),
			wantErr: true,
			errMsg:  "rate limit must be >= 0",
		},
		{
			name:    "zero timeout",
			config:  NewConfig("https://example.com").WithTimeout(0),
			wantErr: true,
			errMsg:  "timeout must be greater than 0",
		},
		{
			name:    "negative timeout",
			config:  NewConfig("https://example.com").WithTimeout(-5 * time.Second),
			wantErr: true,
			errMsg:  "timeout must be greater than 0",
		},
		{
			name:    "empty output dir",
			config:  NewConfig("https://example.com").WithOutputDir(""),
			wantErr: true,
			errMsg:  "output directory must not be empty",
		},
		{
			name: "multiple validation errors",
			config: Config{
				SeedURL:   "",
				OutputDir: "",
				Workers:   -1,
				RateLimit: -1,
				Timeout:   -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				require.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestConfig_WithMethods_Immutability(t *testing.T) {
	original := NewConfig("https://example.com")

	tests := []struct {
		name     string
		mutate   func(Config) Config
		checkOld func(t *testing.T, c Config)
		checkNew func(t *testing.T, c Config)
	}{
		{
			name:     "WithWorkers does not mutate original",
			mutate:   func(c Config) Config { return c.WithWorkers(42) },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, 10, c.Workers) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, 42, c.Workers) },
		},
		{
			name:     "WithRateLimit does not mutate original",
			mutate:   func(c Config) Config { return c.WithRateLimit(5.5) },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, float64(0), c.RateLimit) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, 5.5, c.RateLimit) },
		},
		{
			name:     "WithOutputDir does not mutate original",
			mutate:   func(c Config) Config { return c.WithOutputDir("/tmp/out") },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, "./docs-output", c.OutputDir) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, "/tmp/out", c.OutputDir) },
		},
		{
			name:     "WithSeedURL does not mutate original",
			mutate:   func(c Config) Config { return c.WithSeedURL("https://other.com") },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, "https://example.com", c.SeedURL) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, "https://other.com", c.SeedURL) },
		},
		{
			name:     "WithMaxDepth does not mutate original",
			mutate:   func(c Config) Config { return c.WithMaxDepth(5) },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, 0, c.MaxDepth) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, 5, c.MaxDepth) },
		},
		{
			name:     "WithUserAgent does not mutate original",
			mutate:   func(c Config) Config { return c.WithUserAgent("custom/1.0") },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, "docs-crawler/0.1.0", c.UserAgent) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, "custom/1.0", c.UserAgent) },
		},
		{
			name:     "WithTimeout does not mutate original",
			mutate:   func(c Config) Config { return c.WithTimeout(60 * time.Second) },
			checkOld: func(t *testing.T, c Config) { assert.Equal(t, 30*time.Second, c.Timeout) },
			checkNew: func(t *testing.T, c Config) { assert.Equal(t, 60*time.Second, c.Timeout) },
		},
		{
			name:     "WithUseBrowser does not mutate original",
			mutate:   func(c Config) Config { return c.WithUseBrowser(true) },
			checkOld: func(t *testing.T, c Config) { assert.False(t, c.UseBrowser) },
			checkNew: func(t *testing.T, c Config) { assert.True(t, c.UseBrowser) },
		},
		{
			name:     "WithResume does not mutate original",
			mutate:   func(c Config) Config { return c.WithResume(true) },
			checkOld: func(t *testing.T, c Config) { assert.False(t, c.Resume) },
			checkNew: func(t *testing.T, c Config) { assert.True(t, c.Resume) },
		},
		{
			name:     "WithVerbose does not mutate original",
			mutate:   func(c Config) Config { return c.WithVerbose(true) },
			checkOld: func(t *testing.T, c Config) { assert.False(t, c.Verbose) },
			checkNew: func(t *testing.T, c Config) { assert.True(t, c.Verbose) },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			updated := tt.mutate(original)
			tt.checkOld(t, original)
			tt.checkNew(t, updated)
		})
	}
}

func TestConfig_WithIncludes_DeepCopy(t *testing.T) {
	original := NewConfig("https://example.com")
	patterns := []string{"/docs/*", "/api/*"}
	updated := original.WithIncludes(patterns)

	// Mutating the input slice should not affect the config
	patterns[0] = "/changed/*"
	assert.Equal(t, "/docs/*", updated.Includes[0])
	assert.Nil(t, original.Includes)
}

func TestConfig_WithExcludes_DeepCopy(t *testing.T) {
	original := NewConfig("https://example.com")
	patterns := []string{"/blog/*", "/archive/*"}
	updated := original.WithExcludes(patterns)

	// Mutating the input slice should not affect the config
	patterns[0] = "/changed/*"
	assert.Equal(t, "/blog/*", updated.Excludes[0])
	assert.Nil(t, original.Excludes)
}

func TestConfig_Chaining(t *testing.T) {
	cfg := NewConfig("https://example.com").
		WithWorkers(20).
		WithRateLimit(1.5).
		WithMaxDepth(3).
		WithOutputDir("/tmp/crawl").
		WithVerbose(true)

	assert.Equal(t, 20, cfg.Workers)
	assert.Equal(t, 1.5, cfg.RateLimit)
	assert.Equal(t, 3, cfg.MaxDepth)
	assert.Equal(t, "/tmp/crawl", cfg.OutputDir)
	assert.True(t, cfg.Verbose)
	require.NoError(t, cfg.Validate())
}
