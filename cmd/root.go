package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "1.0.0"
)

var rootCmd = &cobra.Command{
	Use:   "docs-crawler",
	Short: "Crawl any documentation site into a searchable AI knowledge base",
	Long: `docs-crawler is a full docs-to-AI pipeline. Crawl any documentation site,
chunk and embed the content, search it semantically, and export it for LLM/RAG use.

Commands:
  crawl    Crawl a documentation site and save clean Markdown
  discover List URLs discoverable from a site without fetching content
  ingest   Crawl, chunk, embed, and index a documentation site
  search   Semantic search over an ingested knowledge base
  export   Export indexed content to JSONL, Parquet, or CSV
  init     Generate a starter config file

Pipeline features:
  - Single binary, zero runtime dependencies
  - Offline-capable with local embeddings via Ollama or TF-IDF fallback
  - Intelligent rate limiting (auto-detects from response headers)
  - Parallel goroutine pools per stage connected by buffered channels
  - Plugin architecture (discoverers, fetchers, extractors, writers)
  - JavaScript-rendered pages via headless Chrome
  - Resume interrupted crawls`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		level := slog.LevelInfo
		if verbose {
			level = slog.LevelDebug
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: level,
		}))
		slog.SetDefault(logger)
	},
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose logging")
	rootCmd.Version = version
}
