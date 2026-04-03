package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mayur19/docs-crawler/internal/export"
	"github.com/mayur19/docs-crawler/internal/index"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/spf13/cobra"
)

var exportCmd = &cobra.Command{
	Use:   "export",
	Short: "Export knowledge base as JSONL, CSV, or Markdown",
	RunE:  runExport,
}

func init() {
	rootCmd.AddCommand(exportCmd)

	exportCmd.Flags().String("format", "jsonl", "Export format: jsonl, csv, or markdown")
	exportCmd.Flags().Bool("include-vectors", false, "Include embedding vectors in output (JSONL only)")
	exportCmd.Flags().StringP("output", "o", "", "Output file path (default: stdout)")
	exportCmd.Flags().String("source", "./docs-output", "Path to the knowledge base directory")
}

func runExport(cmd *cobra.Command, _ []string) error {
	flags := cmd.Flags()
	format, _ := flags.GetString("format")
	includeVectors, _ := flags.GetBool("include-vectors")
	outputPath, _ := flags.GetString("output")
	source, _ := flags.GetString("source")

	if format != "jsonl" && format != "csv" && format != "markdown" {
		return fmt.Errorf("invalid format %q: must be jsonl, csv, or markdown", format)
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

	chunks, err := store.GetAllChunks(ctx)
	if err != nil {
		return fmt.Errorf("load chunks: %w", err)
	}

	if format == "markdown" {
		return exportMarkdown(ctx, outputPath, source, chunks)
	}

	// For JSONL/CSV we need EmbeddedChunks. Build them by fetching each vector.
	embedded, err := buildEmbeddedChunks(ctx, store, chunks, includeVectors)
	if err != nil {
		return fmt.Errorf("load embeddings: %w", err)
	}

	w, closeWriter, err := openWriter(outputPath)
	if err != nil {
		return err
	}
	defer closeWriter()

	switch format {
	case "jsonl":
		return export.WriteJSONL(ctx, w, embedded, includeVectors)
	case "csv":
		return export.WriteCSV(ctx, w, embedded)
	}

	return nil
}

// buildEmbeddedChunks wraps chunks with their vectors when includeVectors is true.
func buildEmbeddedChunks(
	ctx context.Context,
	store *index.SQLiteStore,
	chunks []pipeline.Chunk,
	includeVectors bool,
) ([]pipeline.EmbeddedChunk, error) {
	result := make([]pipeline.EmbeddedChunk, len(chunks))
	for i, c := range chunks {
		var vector []float32
		if includeVectors {
			v, err := store.GetEmbedding(ctx, c.ID)
			if err != nil {
				// Embedding may not exist if indexing was skipped; use empty vector.
				v = nil
			}
			vector = v
		}
		result[i] = pipeline.NewEmbeddedChunk(c, vector)
	}
	return result, nil
}

// exportMarkdown writes one Markdown file per chunk when outputPath is a directory,
// or a single combined Markdown file when outputPath is a file (or stdout).
func exportMarkdown(ctx context.Context, outputPath string, source string, chunks []pipeline.Chunk) error {
	if outputPath == "" {
		// Write combined Markdown to stdout.
		return writeMarkdownCombined(os.Stdout, chunks)
	}

	// If the output path looks like a directory (or doesn't end in .md), create
	// individual files per source URL.
	info, err := os.Stat(outputPath)
	isDir := err == nil && info.IsDir()

	if isDir {
		return writeMarkdownDir(ctx, outputPath, chunks)
	}

	// Write combined Markdown to the specified file.
	f, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("create output file: %w", err)
	}
	defer f.Close()

	return writeMarkdownCombined(f, chunks)
}

// writeMarkdownCombined writes all chunks as a single Markdown document.
func writeMarkdownCombined(w io.Writer, chunks []pipeline.Chunk) error {
	for _, c := range chunks {
		heading := strings.Join(c.HeadingPath, " › ")
		if heading == "" {
			heading = c.Title
		}

		if _, err := fmt.Fprintf(w, "# %s\n\n**Source:** %s\n\n%s\n\n---\n\n",
			heading, c.SourceURL, c.Content); err != nil {
			return fmt.Errorf("write chunk %s: %w", c.ID, err)
		}
	}
	return nil
}

// writeMarkdownDir writes one file per unique source URL into a directory.
func writeMarkdownDir(_ context.Context, dir string, chunks []pipeline.Chunk) error {
	// Group chunks by source URL.
	grouped := make(map[string][]pipeline.Chunk)
	var order []string
	for _, c := range chunks {
		if _, seen := grouped[c.SourceURL]; !seen {
			order = append(order, c.SourceURL)
		}
		grouped[c.SourceURL] = append(grouped[c.SourceURL], c)
	}

	for _, url := range order {
		urlChunks := grouped[url]
		filename := urlToFilename(url) + ".md"
		path := filepath.Join(dir, filename)

		f, err := os.Create(path)
		if err != nil {
			return fmt.Errorf("create %s: %w", path, err)
		}

		if err := writeMarkdownCombined(f, urlChunks); err != nil {
			f.Close()
			return fmt.Errorf("write %s: %w", path, err)
		}
		f.Close()
	}
	return nil
}

// urlToFilename converts a URL to a safe filename by replacing non-alphanumeric
// characters with underscores and trimming leading/trailing underscores.
func urlToFilename(rawURL string) string {
	// Strip scheme.
	s := rawURL
	for _, prefix := range []string{"https://", "http://"} {
		s = strings.TrimPrefix(s, prefix)
	}

	// Replace path separators and other non-alphanumeric chars.
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	name := strings.Trim(b.String(), "_")
	if name == "" {
		name = "doc"
	}
	return name
}

// openWriter returns a writer for the given output path, or stdout if path is empty.
// The returned close func must be called when done.
func openWriter(outputPath string) (io.Writer, func(), error) {
	if outputPath == "" {
		return os.Stdout, func() {}, nil
	}

	f, err := os.Create(outputPath)
	if err != nil {
		return nil, nil, fmt.Errorf("create output file %q: %w", outputPath, err)
	}
	return f, func() { f.Close() }, nil
}
