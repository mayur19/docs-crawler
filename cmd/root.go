package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

var (
	verbose bool
	version = "0.1.0"
)

var rootCmd = &cobra.Command{
	Use:   "docs-crawler",
	Short: "Crawl documentation websites for LLM/RAG ingestion",
	Long: `docs-crawler intelligently crawls documentation websites and produces
clean Markdown with structured metadata, optimized for LLM and RAG pipelines.

Features:
  - Intelligent rate limiting (auto-detects from response headers)
  - Parallel crawling with configurable worker pools
  - Plugin architecture (discoverers, fetchers, extractors, writers)
  - Support for static HTML and JavaScript-rendered pages
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
