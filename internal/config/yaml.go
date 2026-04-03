package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// yamlConfig is the intermediate struct used to decode YAML config files.
// It mirrors the documented spec sections: source, crawl, chunking, embedding, output.
type yamlConfig struct {
	Source struct {
		URL string `yaml:"url"`
	} `yaml:"source"`
	Crawl struct {
		MaxDepth   int      `yaml:"max_depth"`
		Workers    int      `yaml:"workers"`
		RateLimit  float64  `yaml:"rate_limit"`
		Includes   []string `yaml:"includes"`
		Excludes   []string `yaml:"excludes"`
		UseBrowser bool     `yaml:"use_browser"`
		UserAgent  string   `yaml:"user_agent"`
		TimeoutSec int      `yaml:"timeout_sec"`
		Resume     bool     `yaml:"resume"`
	} `yaml:"crawl"`
	Chunking struct {
		Strategy  string `yaml:"strategy"`
		MaxTokens int    `yaml:"max_tokens"`
	} `yaml:"chunking"`
	Embedding struct {
		Embedder string `yaml:"embedder"`
		Model    string `yaml:"model"`
		Batch    int    `yaml:"batch"`
	} `yaml:"embedding"`
	Output struct {
		Dir     string `yaml:"dir"`
		Verbose bool   `yaml:"verbose"`
	} `yaml:"output"`
}

// LoadYAML reads a YAML config file at path and returns a Config with defaults
// applied for any fields not specified in the file.
func LoadYAML(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file %q: %w", path, err)
	}

	var raw yamlConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("parsing config file %q: %w", path, err)
	}

	cfg := NewConfig(raw.Source.URL)

	if raw.Crawl.MaxDepth != 0 {
		cfg = cfg.WithMaxDepth(raw.Crawl.MaxDepth)
	}
	if raw.Crawl.Workers != 0 {
		cfg = cfg.WithWorkers(raw.Crawl.Workers)
	}
	if raw.Crawl.RateLimit != 0 {
		cfg = cfg.WithRateLimit(raw.Crawl.RateLimit)
	}
	if len(raw.Crawl.Includes) > 0 {
		cfg = cfg.WithIncludes(raw.Crawl.Includes)
	}
	if len(raw.Crawl.Excludes) > 0 {
		cfg = cfg.WithExcludes(raw.Crawl.Excludes)
	}
	if raw.Crawl.UseBrowser {
		cfg = cfg.WithUseBrowser(true)
	}
	if raw.Crawl.UserAgent != "" {
		cfg = cfg.WithUserAgent(raw.Crawl.UserAgent)
	}
	if raw.Crawl.TimeoutSec != 0 {
		cfg = cfg.WithTimeout(time.Duration(raw.Crawl.TimeoutSec) * time.Second)
	}
	if raw.Crawl.Resume {
		cfg = cfg.WithResume(true)
	}

	if raw.Chunking.Strategy != "" {
		cfg = cfg.WithChunkStrategy(raw.Chunking.Strategy)
	}
	if raw.Chunking.MaxTokens != 0 {
		cfg = cfg.WithMaxTokens(raw.Chunking.MaxTokens)
	}

	if raw.Embedding.Embedder != "" {
		cfg = cfg.WithEmbedder(raw.Embedding.Embedder)
	}
	if raw.Embedding.Model != "" {
		cfg = cfg.WithEmbeddingModel(raw.Embedding.Model)
	}
	if raw.Embedding.Batch != 0 {
		cfg = cfg.WithEmbeddingBatch(raw.Embedding.Batch)
	}

	if raw.Output.Dir != "" {
		cfg = cfg.WithOutputDir(raw.Output.Dir)
	}
	if raw.Output.Verbose {
		cfg = cfg.WithVerbose(true)
	}

	return cfg, nil
}
