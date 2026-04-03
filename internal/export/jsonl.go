package export

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// chunkRecord is the JSON representation of a single EmbeddedChunk line in a
// JSONL file. The vector field is omitted when nil (i.e. when includeVectors
// is false).
type chunkRecord struct {
	ID          string    `json:"id"`
	Content     string    `json:"content"`
	SourceURL   string    `json:"source_url"`
	Title       string    `json:"title"`
	HeadingPath []string  `json:"heading_path"`
	ChunkIndex  int       `json:"chunk_index"`
	TotalChunks int       `json:"total_chunks"`
	TokenCount  int       `json:"token_count"`
	ContentHash string    `json:"content_hash"`
	CrawledAt   time.Time `json:"crawled_at"`
	Vector      []float32 `json:"vector,omitempty"`
}

// WriteJSONL encodes each EmbeddedChunk as a JSON object on its own line.
// The vector field is included only when includeVectors is true.
// An error from the underlying writer terminates encoding immediately.
func WriteJSONL(ctx context.Context, w io.Writer, chunks []pipeline.EmbeddedChunk, includeVectors bool) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	for i, ec := range chunks {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled at chunk %d: %w", i, err)
		}

		rec := buildRecord(ec, includeVectors)

		if err := enc.Encode(rec); err != nil {
			return fmt.Errorf("encoding chunk %d (%s): %w", i, ec.Chunk.ID, err)
		}
	}

	return nil
}

// buildRecord converts an EmbeddedChunk into a chunkRecord ready for JSON
// encoding. It creates a new record value and never mutates the input.
func buildRecord(ec pipeline.EmbeddedChunk, includeVectors bool) chunkRecord {
	var vector []float32
	if includeVectors && len(ec.Vector) > 0 {
		// Copy slice to avoid sharing the underlying array.
		vector = make([]float32, len(ec.Vector))
		copy(vector, ec.Vector)
	}

	return chunkRecord{
		ID:          ec.Chunk.ID,
		Content:     ec.Chunk.Content,
		SourceURL:   ec.Chunk.SourceURL,
		Title:       ec.Chunk.Title,
		HeadingPath: ec.Chunk.HeadingPath,
		ChunkIndex:  ec.Chunk.ChunkIndex,
		TotalChunks: ec.Chunk.TotalChunks,
		TokenCount:  ec.Chunk.TokenCount,
		ContentHash: ec.Chunk.ContentHash,
		CrawledAt:   time.Now().UTC(),
		Vector:      vector,
	}
}
