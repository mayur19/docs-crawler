package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mayur19/docs-crawler/internal/index"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Search a local knowledge base",
	Args:  cobra.ExactArgs(1),
	RunE:  runSearch,
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().Int("top", 5, "Number of results to return")
	searchCmd.Flags().String("source", "./docs-output", "Path to the knowledge base directory")
	searchCmd.Flags().String("format", "pretty", "Output format: pretty or json")
}

func runSearch(cmd *cobra.Command, args []string) error {
	query := args[0]

	flags := cmd.Flags()
	topK, _ := flags.GetInt("top")
	source, _ := flags.GetString("source")
	format, _ := flags.GetString("format")

	if format != "pretty" && format != "json" {
		return fmt.Errorf("invalid format %q: must be pretty or json", format)
	}

	dbPath := filepath.Join(source, "index.db")
	store, err := index.NewSQLiteStore(dbPath)
	if err != nil {
		return fmt.Errorf("open index at %s: %w", dbPath, err)
	}
	defer store.Close()

	ctx, cancel := signal.NotifyContext(
		context.Background(), os.Interrupt, syscall.SIGTERM,
	)
	defer cancel()

	results, err := store.Search(ctx, query, topK)
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	if len(results) == 0 {
		fmt.Fprintln(os.Stderr, "No results found.")
		return nil
	}

	switch format {
	case "json":
		return printSearchJSON(results)
	default:
		printSearchPretty(query, results)
		return nil
	}
}

// printSearchPretty renders results as a box-style table.
func printSearchPretty(query string, results []pipeline.SearchResult) {
	const (
		separator = "──────────────────────────────────────────────────────────────────────────"
		snippetLen = 200
	)

	fmt.Printf("Search: %q   (%d result(s))\n", query, len(results))
	fmt.Println(separator)

	for i, r := range results {
		heading := strings.Join(r.Chunk.HeadingPath, " › ")
		if heading == "" {
			heading = "(no heading)"
		}

		snippet := r.Chunk.Content
		if len(snippet) > snippetLen {
			snippet = snippet[:snippetLen] + "…"
		}
		// Flatten newlines for display.
		snippet = strings.ReplaceAll(snippet, "\n", " ")

		fmt.Printf("#%d  Score: %.4f\n", i+1, r.Score)
		fmt.Printf("    Title   : %s\n", r.Chunk.Title)
		fmt.Printf("    Heading : %s\n", heading)
		fmt.Printf("    URL     : %s\n", r.Chunk.SourceURL)
		fmt.Printf("    Snippet : %s\n", snippet)
		fmt.Println(separator)
	}
}

// searchResultJSON is the JSON representation of a single search result.
type searchResultJSON struct {
	Rank    int      `json:"rank"`
	Score   float64  `json:"score"`
	Title   string   `json:"title"`
	URL     string   `json:"url"`
	Heading []string `json:"heading_path"`
	Snippet string   `json:"snippet"`
}

// printSearchJSON writes the results as a JSON array to stdout.
func printSearchJSON(results []pipeline.SearchResult) error {
	const snippetLen = 200

	out := make([]searchResultJSON, len(results))
	for i, r := range results {
		snippet := r.Chunk.Content
		if len(snippet) > snippetLen {
			snippet = snippet[:snippetLen] + "…"
		}
		out[i] = searchResultJSON{
			Rank:    i + 1,
			Score:   r.Score,
			Title:   r.Chunk.Title,
			URL:     r.Chunk.SourceURL,
			Heading: r.Chunk.HeadingPath,
			Snippet: snippet,
		}
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode JSON: %w", err)
	}
	return nil
}
