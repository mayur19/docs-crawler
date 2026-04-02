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

func TestSearchFTS5Only(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "search.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Index chunks without vectors
	chunk1 := pipeline.NewChunk("https://example.com/a", "Guide", []string{"Intro"}, "The quick brown fox jumps", 0, 2)
	chunk2 := pipeline.NewChunk("https://example.com/b", "Reference", []string{"API"}, "Lazy dog sleeps soundly", 1, 2)

	ec1 := pipeline.NewEmbeddedChunk(chunk1, nil)
	ec2 := pipeline.NewEmbeddedChunk(chunk2, nil)

	err = store.Index(ctx, []pipeline.EmbeddedChunk{ec1, ec2})
	require.NoError(t, err)

	results, err := store.Search(ctx, "fox", 10)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, chunk1.ID, results[0].Chunk.ID)
	assert.Greater(t, results[0].Score, 0.0)
}

func TestSearchCosineWithFTS5Rerank(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "cosine.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Chunk 1: vector similar to query
	chunk1 := pipeline.NewChunk("https://example.com/a", "Close", []string{"A"}, "machine learning concepts", 0, 2)
	vec1 := []float32{0.9, 0.1, 0.0, 0.0}

	// Chunk 2: vector dissimilar to query
	chunk2 := pipeline.NewChunk("https://example.com/b", "Far", []string{"B"}, "cooking recipes guide", 1, 2)
	vec2 := []float32{0.0, 0.0, 0.9, 0.1}

	ec1 := pipeline.NewEmbeddedChunk(chunk1, vec1)
	ec2 := pipeline.NewEmbeddedChunk(chunk2, vec2)

	err = store.Index(ctx, []pipeline.EmbeddedChunk{ec1, ec2})
	require.NoError(t, err)

	// Query vector is close to vec1
	queryVec := []float32{0.95, 0.05, 0.0, 0.0}

	results, err := store.SearchWithVector(ctx, queryVec, "machine learning", 10)
	require.NoError(t, err)
	require.NotEmpty(t, results)

	// chunk1 (close vector) should rank first
	assert.Equal(t, chunk1.ID, results[0].Chunk.ID)
	assert.Greater(t, results[0].Score, results[len(results)-1].Score)
}

func TestSearchTopK(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "topk.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	// Index 10 chunks all containing the word "golang"
	var ecs []pipeline.EmbeddedChunk
	for i := range 10 {
		chunk := pipeline.NewChunk(
			"https://example.com/docs",
			"Go Guide",
			[]string{"Chapter"},
			"golang programming tutorial example",
			i,
			10,
		)
		vec := []float32{float32(i) * 0.1, 0.0}
		ecs = append(ecs, pipeline.NewEmbeddedChunk(chunk, vec))
	}

	err = store.Index(ctx, ecs)
	require.NoError(t, err)

	results, err := store.Search(ctx, "golang", 3)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestSearchEmptyStore(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "empty.db")

	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	ctx := context.Background()

	results, err := store.Search(ctx, "anything", 10)
	require.NoError(t, err)
	assert.Empty(t, results)

	queryVec := []float32{0.1, 0.2}
	results, err = store.SearchWithVector(ctx, queryVec, "anything", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}
