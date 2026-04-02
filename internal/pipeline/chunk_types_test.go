package pipeline_test

import (
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChunk(t *testing.T) {
	c := pipeline.NewChunk(
		"https://docs.example.com/auth",
		"Auth Guide",
		[]string{"Getting Started", "Authentication"},
		"Use API keys to authenticate...",
		0,
		3,
	)

	assert.Equal(t, "Auth Guide", c.Title)
	assert.Equal(t, []string{"Getting Started", "Authentication"}, c.HeadingPath)
	assert.Equal(t, "Use API keys to authenticate...", c.Content)
	assert.Equal(t, 0, c.ChunkIndex)
	assert.Equal(t, 3, c.TotalChunks)
	assert.NotEmpty(t, c.ID, "ID should be generated")
	assert.NotEmpty(t, c.ContentHash, "ContentHash should be generated")
	assert.Greater(t, c.TokenCount, 0, "TokenCount should be estimated")
}

func TestChunkIDDeterministic(t *testing.T) {
	c1 := pipeline.NewChunk("https://example.com/a", "T", []string{"H1"}, "body", 0, 1)
	c2 := pipeline.NewChunk("https://example.com/a", "T", []string{"H1"}, "body", 0, 1)
	assert.Equal(t, c1.ID, c2.ID, "same inputs should produce same ID")

	c3 := pipeline.NewChunk("https://example.com/b", "T", []string{"H1"}, "body", 0, 1)
	assert.NotEqual(t, c1.ID, c3.ID, "different URL should produce different ID")
}

func TestNewEmbeddedChunk(t *testing.T) {
	c := pipeline.NewChunk("https://example.com", "T", nil, "body", 0, 1)
	vec := []float32{0.1, 0.2, 0.3}
	ec := pipeline.NewEmbeddedChunk(c, vec)

	assert.Equal(t, c.ID, ec.Chunk.ID)
	assert.Equal(t, vec, ec.Vector)
	assert.Equal(t, 3, ec.Dimensions)
}

func TestNewSearchResult(t *testing.T) {
	c := pipeline.NewChunk("https://example.com", "T", []string{"H"}, "body", 0, 1)
	r := pipeline.NewSearchResult(c, 0.95)

	assert.Equal(t, c, r.Chunk)
	assert.InDelta(t, 0.95, r.Score, 0.001)
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name    string
		text    string
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"single word", "hello", 1, 2},
		{"sentence", "the quick brown fox jumps", 6, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pipeline.EstimateTokens(tt.text)
			require.GreaterOrEqual(t, got, tt.wantMin)
			require.LessOrEqual(t, got, tt.wantMax)
		})
	}
}
