package embed

import (
	"context"
	"math"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFIDFEmbedder_Name(t *testing.T) {
	e := NewTFIDFEmbedder()
	assert.Equal(t, "tfidf", e.Name())
}

func TestTFIDFEmbedder_Dimensions(t *testing.T) {
	e := NewTFIDFEmbedder()
	assert.Equal(t, 0, e.Dimensions())
}

func TestTFIDFEmbedder_Empty(t *testing.T) {
	e := NewTFIDFEmbedder()
	results, err := e.Embed(context.Background(), []pipeline.Chunk{})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestTFIDFEmbedder_SingleChunk(t *testing.T) {
	e := NewTFIDFEmbedder()
	chunk := pipeline.NewChunk("http://example.com", "Test", []string{"h1"}, "hello world foo bar", 0, 1)
	results, err := e.Embed(context.Background(), []pipeline.Chunk{chunk})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, chunk.ID, results[0].Chunk.ID)
	assert.NotEmpty(t, results[0].Vector)
	// L2 norm should be ~1 (normalized)
	norm := l2Norm(results[0].Vector)
	assert.InDelta(t, 1.0, norm, 1e-5)
}

func TestTFIDFEmbedder_TwoChunks_DifferentVectors(t *testing.T) {
	e := NewTFIDFEmbedder()
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("http://example.com", "Doc1", []string{"h1"}, "apple banana cherry", 0, 2),
		pipeline.NewChunk("http://example.com", "Doc2", []string{"h2"}, "dog elephant fox", 1, 2),
	}
	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Vectors must differ (different content)
	assert.NotEqual(t, results[0].Vector, results[1].Vector)

	// Both must be L2-normalized
	assert.InDelta(t, 1.0, l2Norm(results[0].Vector), 1e-5)
	assert.InDelta(t, 1.0, l2Norm(results[1].Vector), 1e-5)
}

func TestTFIDFEmbedder_SameContent_SameVector(t *testing.T) {
	e := NewTFIDFEmbedder()
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("http://example.com", "Doc1", []string{"h1"}, "go programming language", 0, 2),
		pipeline.NewChunk("http://example.com", "Doc2", []string{"h2"}, "go programming language", 1, 2),
	}
	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Same content → same TF-IDF vector
	assert.Equal(t, results[0].Vector, results[1].Vector)
}

// l2Norm computes the L2 norm of a float32 slice.
func l2Norm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}
