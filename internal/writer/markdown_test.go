package writer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/mayur19/docs-crawler/internal/writer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSeedURL = "https://docs.example.com"

func newTestDocument(urlPath, title, markdown string) pipeline.Document {
	return pipeline.Document{
		URL:         testSeedURL + urlPath,
		Title:       title,
		Description: "Test description for " + title,
		Markdown:    markdown,
		Headings: []pipeline.Heading{
			{Level: 1, Text: title},
			{Level: 2, Text: "Section"},
		},
		Links:       []string{"../other", "../page"},
		WordCount:   42,
		ContentHash: "sha256:abc123",
		CrawledAt:   time.Date(2026, 4, 2, 10, 30, 0, 0, time.UTC),
	}
}

func TestNewMarkdownWriter(t *testing.T) {
	tests := []struct {
		name      string
		outputDir string
		seedURL   string
		wantErr   string
	}{
		{
			name:      "valid inputs create writer",
			outputDir: t.TempDir(),
			seedURL:   testSeedURL,
		},
		{
			name:      "empty output dir returns error",
			outputDir: "",
			seedURL:   testSeedURL,
			wantErr:   "output directory must not be empty",
		},
		{
			name:      "empty seed URL returns error",
			outputDir: t.TempDir(),
			seedURL:   "",
			wantErr:   "seed URL must not be empty",
		},
		{
			name:      "creates output dir if missing",
			outputDir: filepath.Join(t.TempDir(), "nested", "output"),
			seedURL:   testSeedURL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w, err := writer.NewMarkdownWriter(tt.outputDir, tt.seedURL)
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, w)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, w)
			assert.Equal(t, "markdown", w.Name())
		})
	}
}

func TestName(t *testing.T) {
	w, err := writer.NewMarkdownWriter(t.TempDir(), testSeedURL)
	require.NoError(t, err)

	assert.Equal(t, "markdown", w.Name())
}

func TestWrite_CreatesDirectoryStructure(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	doc := newTestDocument("/api/auth", "Auth API", "# Auth API\n\nContent here.")
	err = w.Write(context.Background(), doc)
	require.NoError(t, err)

	// Verify directory was created
	dirPath := filepath.Join(outputDir, "pages", "api")
	info, err := os.Stat(dirPath)
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestWrite_CreatesMarkdownFile(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	expectedContent := "# Auth API\n\nContent here."
	doc := newTestDocument("/api/auth", "Auth API", expectedContent)
	err = w.Write(context.Background(), doc)
	require.NoError(t, err)

	mdPath := filepath.Join(outputDir, "pages", "api", "auth.md")
	content, err := os.ReadFile(mdPath)
	require.NoError(t, err)
	assert.Equal(t, expectedContent, string(content))
}

func TestWrite_CreatesMetaJSON(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	doc := newTestDocument("/api/auth", "Auth API", "# Auth API")
	err = w.Write(context.Background(), doc)
	require.NoError(t, err)

	metaPath := filepath.Join(outputDir, "pages", "api", "auth.meta.json")
	data, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var meta writer.PageMeta
	err = json.Unmarshal(data, &meta)
	require.NoError(t, err)

	assert.Equal(t, testSeedURL+"/api/auth", meta.URL)
	assert.Equal(t, "Auth API", meta.Title)
	assert.Equal(t, "Test description for Auth API", meta.Description)
	assert.Equal(t, []string{"Auth API", "Section"}, meta.Headings)
	assert.Equal(t, 42, meta.WordCount)
	assert.Equal(t, "2026-04-02T10:30:00Z", meta.CrawledAt)
	assert.Equal(t, "sha256:abc123", meta.ContentHash)
	assert.Equal(t, []string{"../other", "../page"}, meta.Links)
}

func TestWrite_RootURL(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	doc := newTestDocument("/", "Home", "# Home")
	err = w.Write(context.Background(), doc)
	require.NoError(t, err)

	mdPath := filepath.Join(outputDir, "pages", "index.md")
	_, err = os.Stat(mdPath)
	require.NoError(t, err)

	metaPath := filepath.Join(outputDir, "pages", "index.meta.json")
	_, err = os.Stat(metaPath)
	require.NoError(t, err)
}

func TestClose_WritesManifest(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	err = w.Close()
	require.NoError(t, err)

	manifestPath := filepath.Join(outputDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var manifest writer.Manifest
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err)

	assert.Equal(t, testSeedURL, manifest.SeedURL)
	assert.NotEmpty(t, manifest.StartedAt)
	assert.NotEmpty(t, manifest.CompletedAt)
	assert.Equal(t, 0, manifest.PagesCrawled)
	assert.Equal(t, 0, manifest.Errors)
}

func TestMultipleWrites_AccumulatePageCount(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	pages := []struct {
		path  string
		title string
	}{
		{"/api/auth", "Auth"},
		{"/api/users", "Users"},
		{"/guide/start", "Getting Started"},
	}

	for _, p := range pages {
		doc := newTestDocument(p.path, p.title, "# "+p.title)
		err := w.Write(context.Background(), doc)
		require.NoError(t, err)
	}

	err = w.Close()
	require.NoError(t, err)

	manifestPath := filepath.Join(outputDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var manifest writer.Manifest
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err)

	assert.Equal(t, 3, manifest.PagesCrawled)
	assert.Equal(t, 0, manifest.Errors)
}

func TestConcurrentWrites(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	const numGoroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path := fmt.Sprintf("/page/%d", idx)
			title := fmt.Sprintf("Page %d", idx)
			doc := newTestDocument(path, title, "# "+title)
			if writeErr := w.Write(context.Background(), doc); writeErr != nil {
				errs <- writeErr
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	for writeErr := range errs {
		t.Errorf("concurrent write error: %v", writeErr)
	}

	err = w.Close()
	require.NoError(t, err)

	manifestPath := filepath.Join(outputDir, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)

	var manifest writer.Manifest
	err = json.Unmarshal(data, &manifest)
	require.NoError(t, err)

	assert.Equal(t, numGoroutines, manifest.PagesCrawled)
	assert.Equal(t, 0, manifest.Errors)

	// Verify all files were created
	for i := 0; i < numGoroutines; i++ {
		mdPath := filepath.Join(outputDir, "pages", "page", fmt.Sprintf("%d.md", i))
		_, err := os.Stat(mdPath)
		assert.NoError(t, err, "missing md file for page %d", i)
	}
}

func TestWrite_NilHeadingsAndLinks(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	doc := pipeline.Document{
		URL:         testSeedURL + "/empty",
		Title:       "Empty",
		Description: "No headings or links",
		Markdown:    "# Empty",
		Headings:    nil,
		Links:       nil,
		WordCount:   1,
		ContentHash: "sha256:000",
		CrawledAt:   time.Now(),
	}

	err = w.Write(context.Background(), doc)
	require.NoError(t, err)

	metaPath := filepath.Join(outputDir, "pages", "empty.meta.json")
	data, err := os.ReadFile(metaPath)
	require.NoError(t, err)

	var meta writer.PageMeta
	err = json.Unmarshal(data, &meta)
	require.NoError(t, err)

	// Should be empty slices, not null in JSON
	assert.NotNil(t, meta.Headings)
	assert.NotNil(t, meta.Links)
	assert.Empty(t, meta.Headings)
	assert.Empty(t, meta.Links)

	// Also verify the raw JSON has [] not null
	assert.Contains(t, string(data), `"headings": []`)
	assert.Contains(t, string(data), `"links": []`)
}

func TestWrite_DeeplyNestedPath(t *testing.T) {
	outputDir := t.TempDir()
	w, err := writer.NewMarkdownWriter(outputDir, testSeedURL)
	require.NoError(t, err)

	doc := newTestDocument("/a/b/c/d/e", "Deep", "# Deep")
	err = w.Write(context.Background(), doc)
	require.NoError(t, err)

	mdPath := filepath.Join(outputDir, "pages", "a", "b", "c", "d", "e.md")
	_, err = os.Stat(mdPath)
	require.NoError(t, err)
}
