package pipeline

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strings"
)

// Chunk represents a section of a Document, sized for embedding.
type Chunk struct {
	ID          string
	Content     string
	SourceURL   string
	Title       string
	HeadingPath []string
	ChunkIndex  int
	TotalChunks int
	TokenCount  int
	ContentHash string
}

// NewChunk creates a Chunk with a deterministic ID and estimated token count.
func NewChunk(
	sourceURL string,
	title string,
	headingPath []string,
	content string,
	chunkIndex int,
	totalChunks int,
) Chunk {
	idInput := sourceURL + "|" + strings.Join(headingPath, "|") + "|" + fmt.Sprintf("%d", chunkIndex)
	hash := sha256.Sum256([]byte(idInput))
	id := fmt.Sprintf("%x", hash[:8])

	contentHash := fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(content)))

	return Chunk{
		ID:          id,
		Content:     content,
		SourceURL:   sourceURL,
		Title:       title,
		HeadingPath: headingPath,
		ChunkIndex:  chunkIndex,
		TotalChunks: totalChunks,
		TokenCount:  EstimateTokens(content),
		ContentHash: contentHash,
	}
}

// EmbeddedChunk is a Chunk with its vector embedding.
type EmbeddedChunk struct {
	Chunk      Chunk
	Vector     []float32
	Dimensions int
}

// NewEmbeddedChunk wraps a Chunk with its embedding vector.
func NewEmbeddedChunk(chunk Chunk, vector []float32) EmbeddedChunk {
	return EmbeddedChunk{
		Chunk:      chunk,
		Vector:     vector,
		Dimensions: len(vector),
	}
}

// SearchResult is a Chunk with a relevance score.
type SearchResult struct {
	Chunk Chunk
	Score float64
}

// NewSearchResult creates a SearchResult.
func NewSearchResult(chunk Chunk, score float64) SearchResult {
	return SearchResult{Chunk: chunk, Score: score}
}

// EstimateTokens approximates the token count using word count * 1.3.
func EstimateTokens(text string) int {
	words := len(strings.Fields(text))
	return int(math.Ceil(float64(words) * 1.3))
}
