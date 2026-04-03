package index_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/mayur19/docs-crawler/internal/index"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestChunk(sourceURL, title string, headingPath []string, content string, chunkIndex, total int) pipeline.Chunk {
	return pipeline.NewChunk(sourceURL, title, headingPath, content, chunkIndex, total)
}

func newTestEmbeddedChunk(chunk pipeline.Chunk, vector []float32) pipeline.EmbeddedChunk {
	return pipeline.NewEmbeddedChunk(chunk, vector)
}

func TestNewSQLiteStoreCreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	require.NotNil(t, store)
	defer store.Close()
}

func TestSQLiteStoreName(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	assert.Equal(t, "sqlite", store.Name())
}

func TestSQLiteStoreIndexAndRetrieve(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	chunk := newTestChunk("https://example.com/docs", "Example Title", []string{"Section A", "Sub B"}, "Hello world content", 0, 1)
	vector := []float32{0.1, 0.2, 0.3, 0.4}
	ec := newTestEmbeddedChunk(chunk, vector)

	err = store.Index(ctx, []pipeline.EmbeddedChunk{ec})
	require.NoError(t, err)

	// Retrieve all chunks
	chunks, err := store.GetAllChunks(ctx)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, chunk.ID, chunks[0].ID)
	assert.Equal(t, chunk.Content, chunks[0].Content)
	assert.Equal(t, chunk.SourceURL, chunks[0].SourceURL)
	assert.Equal(t, chunk.Title, chunks[0].Title)
	assert.Equal(t, chunk.HeadingPath, chunks[0].HeadingPath)

	// Retrieve embedding
	gotVec, err := store.GetEmbedding(ctx, chunk.ID)
	require.NoError(t, err)
	require.Len(t, gotVec, len(vector))
	for i, v := range vector {
		assert.InDelta(t, v, gotVec[i], 1e-6)
	}
}

func TestSQLiteStoreUpsert(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	chunk := newTestChunk("https://example.com/docs", "Old Title", []string{"Section"}, "Old content", 0, 1)
	ec := newTestEmbeddedChunk(chunk, []float32{0.1, 0.2})

	err = store.Index(ctx, []pipeline.EmbeddedChunk{ec})
	require.NoError(t, err)

	// Re-index with same ID but updated content (use same sourceURL/headingPath/index for same ID)
	updatedChunk := pipeline.Chunk{
		ID:          chunk.ID,
		Content:     "Updated content",
		SourceURL:   chunk.SourceURL,
		Title:       "New Title",
		HeadingPath: chunk.HeadingPath,
		ChunkIndex:  chunk.ChunkIndex,
		TotalChunks: chunk.TotalChunks,
		TokenCount:  5,
		ContentHash: "sha256:updated",
	}
	updatedEC := pipeline.NewEmbeddedChunk(updatedChunk, []float32{0.9, 0.8})

	err = store.Index(ctx, []pipeline.EmbeddedChunk{updatedEC})
	require.NoError(t, err)

	chunks, err := store.GetAllChunks(ctx)
	require.NoError(t, err)

	// Should have only one chunk (upsert, not duplicate)
	assert.Len(t, chunks, 1)
	assert.Equal(t, "New Title", chunks[0].Title)
	assert.Equal(t, "Updated content", chunks[0].Content)
}

func TestSQLiteStoreSetAndGetMeta(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	err = store.SetMeta(ctx, "last_crawled", "2024-01-01")
	require.NoError(t, err)

	val, err := store.GetMeta(ctx, "last_crawled")
	require.NoError(t, err)
	assert.Equal(t, "2024-01-01", val)

	// Upsert existing key
	err = store.SetMeta(ctx, "last_crawled", "2024-12-31")
	require.NoError(t, err)

	val, err = store.GetMeta(ctx, "last_crawled")
	require.NoError(t, err)
	assert.Equal(t, "2024-12-31", val)
}

func TestSQLiteStoreDeleteBySource(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	chunk1 := newTestChunk("https://example.com/page1", "Page 1", []string{"Sec A"}, "Content 1", 0, 1)
	chunk2 := newTestChunk("https://example.com/page2", "Page 2", []string{"Sec B"}, "Content 2", 0, 1)

	ec1 := newTestEmbeddedChunk(chunk1, []float32{0.1})
	ec2 := newTestEmbeddedChunk(chunk2, []float32{0.2})

	err = store.Index(ctx, []pipeline.EmbeddedChunk{ec1, ec2})
	require.NoError(t, err)

	chunks, err := store.GetAllChunks(ctx)
	require.NoError(t, err)
	assert.Len(t, chunks, 2)

	err = store.DeleteBySourceURL(ctx, "https://example.com/page1")
	require.NoError(t, err)

	chunks, err = store.GetAllChunks(ctx)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "https://example.com/page2", chunks[0].SourceURL)
}
