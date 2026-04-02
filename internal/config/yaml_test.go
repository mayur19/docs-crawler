package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const fullYAML = `
source:
  url: "https://docs.example.com"

crawl:
  max_depth: 5
  workers: 20
  rate_limit: 2.5
  includes:
    - "/docs/*"
    - "/api/*"
  excludes:
    - "/blog/*"
  use_browser: true
  user_agent: "custom-agent/1.0"
  timeout_sec: 60
  resume: true

chunking:
  strategy: "paragraph"
  max_tokens: 1024

embedding:
  embedder: "openai"
  model: "text-embedding-3-small"
  batch: 32

output:
  dir: "/tmp/output"
  verbose: true
`

const minimalYAML = `
source:
  url: "https://minimal.example.com"
`

func writeTempYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadYAMLConfig(t *testing.T) {
	path := writeTempYAML(t, fullYAML)

	cfg, err := LoadYAML(path)
	require.NoError(t, err)

	assert.Equal(t, "https://docs.example.com", cfg.SeedURL)
	assert.Equal(t, 5, cfg.MaxDepth)
	assert.Equal(t, 20, cfg.Workers)
	assert.Equal(t, 2.5, cfg.RateLimit)
	assert.Equal(t, []string{"/docs/*", "/api/*"}, cfg.Includes)
	assert.Equal(t, []string{"/blog/*"}, cfg.Excludes)
	assert.True(t, cfg.UseBrowser)
	assert.Equal(t, "custom-agent/1.0", cfg.UserAgent)
	assert.Equal(t, 60*time.Second, cfg.Timeout)
	assert.True(t, cfg.Resume)
	assert.Equal(t, "paragraph", cfg.ChunkStrategy)
	assert.Equal(t, 1024, cfg.MaxTokens)
	assert.Equal(t, "openai", cfg.Embedder)
	assert.Equal(t, "text-embedding-3-small", cfg.EmbeddingModel)
	assert.Equal(t, 32, cfg.EmbeddingBatch)
	assert.Equal(t, "/tmp/output", cfg.OutputDir)
	assert.True(t, cfg.Verbose)
}

func TestLoadYAMLConfigMinimal(t *testing.T) {
	path := writeTempYAML(t, minimalYAML)

	cfg, err := LoadYAML(path)
	require.NoError(t, err)

	assert.Equal(t, "https://minimal.example.com", cfg.SeedURL)

	// Verify all defaults are applied.
	assert.Equal(t, "./docs-output", cfg.OutputDir)
	assert.Equal(t, 10, cfg.Workers)
	assert.Equal(t, 0, cfg.MaxDepth)
	assert.Equal(t, float64(0), cfg.RateLimit)
	assert.Equal(t, "docs-crawler/0.1.0", cfg.UserAgent)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.False(t, cfg.UseBrowser)
	assert.False(t, cfg.Resume)
	assert.False(t, cfg.Verbose)
	assert.Nil(t, cfg.Includes)
	assert.Nil(t, cfg.Excludes)
	assert.Equal(t, "heading", cfg.ChunkStrategy)
	assert.Equal(t, 512, cfg.MaxTokens)
	assert.Equal(t, "auto", cfg.Embedder)
	assert.Equal(t, "nomic-embed-text", cfg.EmbeddingModel)
	assert.Equal(t, 64, cfg.EmbeddingBatch)
}

func TestLoadYAMLConfigFileNotFound(t *testing.T) {
	_, err := LoadYAML("/nonexistent/path/config.yaml")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoadYAMLConfigInvalidYAML(t *testing.T) {
	path := writeTempYAML(t, "source: [invalid: yaml: {content")

	_, err := LoadYAML(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing config file")
}
