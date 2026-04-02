package export_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mayur19/docs-crawler/internal/export"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeChunks builds two EmbeddedChunks for use in tests.
func makeChunks() []pipeline.EmbeddedChunk {
	c1 := pipeline.NewChunk(
		"https://example.com/page1",
		"Page One",
		[]string{"Introduction", "Overview"},
		"Content of page one.",
		0,
		2,
	)
	c2 := pipeline.NewChunk(
		"https://example.com/page2",
		"Page Two",
		[]string{"Details"},
		"Content of page two.",
		1,
		2,
	)
	return []pipeline.EmbeddedChunk{
		pipeline.NewEmbeddedChunk(c1, []float32{0.1, 0.2, 0.3}),
		pipeline.NewEmbeddedChunk(c2, []float32{0.4, 0.5, 0.6}),
	}
}

// TestJSONLExportWithoutVectors verifies that two chunks produce two lines and
// that the vector field is absent when includeVectors is false.
func TestJSONLExportWithoutVectors(t *testing.T) {
	chunks := makeChunks()
	var buf bytes.Buffer

	err := export.WriteJSONL(context.Background(), &buf, chunks, false)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Len(t, lines, 2, "expected 2 lines for 2 chunks")

	for i, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), "line %d must be valid JSON", i)

		assert.Contains(t, record, "id")
		assert.Contains(t, record, "content")
		assert.Contains(t, record, "source_url")
		assert.Contains(t, record, "title")
		assert.Contains(t, record, "heading_path")
		assert.Contains(t, record, "chunk_index")
		assert.Contains(t, record, "total_chunks")
		assert.Contains(t, record, "token_count")
		assert.Contains(t, record, "content_hash")
		assert.Contains(t, record, "crawled_at")
		assert.NotContains(t, record, "vector", "vector must be absent when includeVectors is false")
	}

	// spot-check values for first chunk
	var first map[string]any
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &first))
	assert.Equal(t, "https://example.com/page1", first["source_url"])
	assert.Equal(t, "Page One", first["title"])
}

// TestJSONLExportWithVectors verifies that the vector field is present when
// includeVectors is true.
func TestJSONLExportWithVectors(t *testing.T) {
	chunks := makeChunks()
	var buf bytes.Buffer

	err := export.WriteJSONL(context.Background(), &buf, chunks, true)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Len(t, lines, 2)

	for i, line := range lines {
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record), "line %d must be valid JSON", i)
		assert.Contains(t, record, "vector", "vector must be present when includeVectors is true")

		vRaw, ok := record["vector"].([]any)
		require.True(t, ok, "vector must be a JSON array")
		assert.Len(t, vRaw, 3)
	}
}

// TestJSONLExportEmpty verifies that empty input produces empty output.
func TestJSONLExportEmpty(t *testing.T) {
	var buf bytes.Buffer

	err := export.WriteJSONL(context.Background(), &buf, []pipeline.EmbeddedChunk{}, false)
	require.NoError(t, err)

	assert.Empty(t, buf.String(), "empty input must produce empty output")
}

// TestCSVExport verifies the header row and two data rows.
func TestCSVExport(t *testing.T) {
	chunks := makeChunks()
	var buf bytes.Buffer

	err := export.WriteCSV(context.Background(), &buf, chunks)
	require.NoError(t, err)

	r := csv.NewReader(&buf)
	records, err := r.ReadAll()
	require.NoError(t, err)

	// header + 2 data rows
	require.Len(t, records, 3, "expected header + 2 data rows")

	header := records[0]
	assert.Equal(t, []string{"id", "source_url", "title", "heading_path", "content", "token_count", "content_hash"}, header)

	// first data row
	row1 := records[1]
	assert.NotEmpty(t, row1[0], "id must not be empty")
	assert.Equal(t, "https://example.com/page1", row1[1])
	assert.Equal(t, "Page One", row1[2])
	assert.Equal(t, "Introduction > Overview", row1[3], "heading_path must be joined with ' > '")
	assert.Equal(t, "Content of page one.", row1[4])
	assert.NotEmpty(t, row1[5], "token_count must not be empty")
	assert.NotEmpty(t, row1[6], "content_hash must not be empty")

	// second data row
	row2 := records[2]
	assert.Equal(t, "https://example.com/page2", row2[1])
	assert.Equal(t, "Details", row2[3], "single heading segment must not contain ' > '")
}
