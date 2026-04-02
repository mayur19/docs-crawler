package writer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/scope"
)

// Compile-time check: MarkdownWriter implements pipeline.Writer.
var _ pipeline.Writer = (*MarkdownWriter)(nil)

// MarkdownWriter writes crawled documents as markdown files with metadata.
// It implements the pipeline.Writer interface.
type MarkdownWriter struct {
	outputDir string
	seedURL   string
	startedAt time.Time

	mu           sync.Mutex
	pagesWritten int
	errors       int
}

// NewMarkdownWriter creates a MarkdownWriter that writes files under outputDir.
// The output directory is created if it does not exist.
func NewMarkdownWriter(outputDir string, seedURL string) (*MarkdownWriter, error) {
	if outputDir == "" {
		return nil, fmt.Errorf("output directory must not be empty")
	}
	if seedURL == "" {
		return nil, fmt.Errorf("seed URL must not be empty")
	}

	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return nil, fmt.Errorf("creating output directory %q: %w", outputDir, err)
	}

	return &MarkdownWriter{
		outputDir: outputDir,
		seedURL:   seedURL,
		startedAt: time.Now(),
	}, nil
}

// Name returns the writer's identifier.
func (w *MarkdownWriter) Name() string {
	return "markdown"
}

// Write persists a document as a .md file and a .meta.json sidecar.
// It is safe for concurrent use by multiple goroutines.
func (w *MarkdownWriter) Write(ctx context.Context, doc pipeline.Document) error {
	relPath, err := scope.URLToFilepath(w.seedURL, doc.URL)
	if err != nil {
		w.recordError()
		return fmt.Errorf("converting URL to filepath: %w", err)
	}

	if err := w.writeFiles(relPath, doc); err != nil {
		w.recordError()
		return err
	}

	w.recordPage()
	slog.Info("wrote page", "path", relPath, "url", doc.URL)
	return nil
}

// Close writes the manifest.json summary to the output root.
func (w *MarkdownWriter) Close() error {
	w.mu.Lock()
	pages := w.pagesWritten
	errs := w.errors
	w.mu.Unlock()

	completedAt := time.Now()
	manifest := Manifest{
		SeedURL:         w.seedURL,
		StartedAt:       formatTime(w.startedAt),
		CompletedAt:     formatTime(completedAt),
		DurationSeconds: completedAt.Sub(w.startedAt).Seconds(),
		PagesCrawled:    pages,
		Errors:          errs,
	}

	data, err := marshalJSON(manifest)
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	manifestPath := filepath.Join(w.outputDir, "manifest.json")
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		return fmt.Errorf("writing manifest %q: %w", manifestPath, err)
	}

	slog.Info("wrote manifest", "path", manifestPath, "pages", pages, "errors", errs)
	return nil
}

// writeFiles creates the .md and .meta.json files for a single document.
func (w *MarkdownWriter) writeFiles(relPath string, doc pipeline.Document) error {
	mdPath := filepath.Join(w.outputDir, "pages", relPath+".md")
	metaPath := filepath.Join(w.outputDir, "pages", relPath+".meta.json")

	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return fmt.Errorf("creating directory for %q: %w", mdPath, err)
	}

	if err := os.WriteFile(mdPath, []byte(doc.Markdown), 0o644); err != nil {
		return fmt.Errorf("writing markdown file %q: %w", mdPath, err)
	}

	meta := buildPageMeta(doc)
	data, err := marshalJSON(meta)
	if err != nil {
		return fmt.Errorf("marshaling metadata for %q: %w", doc.URL, err)
	}

	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		return fmt.Errorf("writing metadata file %q: %w", metaPath, err)
	}

	return nil
}

// buildPageMeta constructs a PageMeta from a Document.
func buildPageMeta(doc pipeline.Document) PageMeta {
	headings := doc.HeadingTexts()
	if headings == nil {
		headings = []string{}
	}

	links := doc.Links
	if links == nil {
		links = []string{}
	}

	return PageMeta{
		URL:         doc.URL,
		Title:       doc.Title,
		Description: doc.Description,
		Headings:    headings,
		WordCount:   doc.WordCount,
		CrawledAt:   formatTime(doc.CrawledAt),
		ContentHash: doc.ContentHash,
		Links:       links,
	}
}

// recordPage increments the pages-written counter.
func (w *MarkdownWriter) recordPage() {
	w.mu.Lock()
	w.pagesWritten++
	w.mu.Unlock()
}

// recordError increments the error counter.
func (w *MarkdownWriter) recordError() {
	w.mu.Lock()
	w.errors++
	w.mu.Unlock()
}
