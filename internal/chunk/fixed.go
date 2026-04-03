package chunk

import (
	"context"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// FixedChunker splits a Document into Chunks of roughly maxTokens size with
// a configurable overlap (in tokens) between consecutive chunks.
// Splitting always occurs on word boundaries.
type FixedChunker struct {
	maxTokens   int
	overlapTokens int
}

// NewFixedChunker creates a FixedChunker with the given max token size and overlap.
func NewFixedChunker(maxTokens, overlapTokens int) *FixedChunker {
	return &FixedChunker{
		maxTokens:     maxTokens,
		overlapTokens: overlapTokens,
	}
}

// Name returns the chunker's identifier.
func (f *FixedChunker) Name() string {
	return "fixed"
}

// Chunk splits the document into fixed-size word-boundary chunks with overlap.
func (f *FixedChunker) Chunk(ctx context.Context, doc pipeline.Document) ([]pipeline.Chunk, error) {
	text := strings.TrimSpace(doc.Markdown)
	if text == "" {
		return nil, nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil, nil
	}

	// Convert maxTokens and overlapTokens to approximate word counts.
	// Since tokens = ceil(words * 1.3), words = floor(tokens / 1.3).
	maxWords := int(float64(f.maxTokens) / 1.3)
	if maxWords < 1 {
		maxWords = 1
	}
	overlapWords := int(float64(f.overlapTokens) / 1.3)
	if overlapWords < 0 {
		overlapWords = 0
	}

	var rawChunks []string
	start := 0
	for start < len(words) {
		end := start + maxWords
		if end > len(words) {
			end = len(words)
		}
		rawChunks = append(rawChunks, strings.Join(words[start:end], " "))

		advance := maxWords - overlapWords
		if advance < 1 {
			advance = 1
		}
		start += advance
	}

	total := len(rawChunks)
	result := make([]pipeline.Chunk, total)
	for i, content := range rawChunks {
		result[i] = pipeline.NewChunk(
			doc.URL,
			doc.Title,
			nil,
			content,
			i,
			total,
		)
	}
	return result, nil
}
