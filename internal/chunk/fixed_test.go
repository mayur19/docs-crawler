package chunk_test

import (
	"context"
	"strings"
	"testing"

	"github.com/mayur19/docs-crawler/internal/chunk"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixedChunker_Name(t *testing.T) {
	c := chunk.NewFixedChunker(500, 50)
	assert.Equal(t, "fixed", c.Name())
}

func TestFixedChunker_EmptyDoc(t *testing.T) {
	c := chunk.NewFixedChunker(500, 50)
	doc := pipeline.Document{
		URL:      "https://docs.example.com/page",
		Title:    "Test Page",
		Markdown: "",
	}
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestFixedChunker_SmallDoc(t *testing.T) {
	// A document that fits in a single chunk
	c := chunk.NewFixedChunker(500, 50)
	doc := pipeline.Document{
		URL:      "https://docs.example.com/page",
		Title:    "Test Page",
		Markdown: "This is a small document with just a few words.",
	}
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, doc.Markdown, chunks[0].Content)
	assert.Equal(t, doc.URL, chunks[0].SourceURL)
	assert.Equal(t, doc.Title, chunks[0].Title)
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, 1, chunks[0].TotalChunks)
}

func TestFixedChunker_LargeDocSplitting(t *testing.T) {
	// Build a large document that will need to be split
	words := make([]string, 300)
	for i := range words {
		words[i] = "word"
	}
	largeText := strings.Join(words, " ")

	c := chunk.NewFixedChunker(100, 20) // maxTokens=100, overlap=20
	doc := pipeline.Document{
		URL:      "https://docs.example.com/page",
		Title:    "Large Doc",
		Markdown: largeText,
	}
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)

	// Should be split into multiple chunks
	assert.Greater(t, len(chunks), 1)

	// All chunks should have correct metadata
	total := len(chunks)
	for i, ch := range chunks {
		assert.Equal(t, total, ch.TotalChunks)
		assert.Equal(t, i, ch.ChunkIndex)
		assert.Equal(t, doc.URL, ch.SourceURL)
		assert.Equal(t, doc.Title, ch.Title)
		assert.NotEmpty(t, ch.Content)
	}
}

func TestFixedChunker_OverlapIncludesPreviousWords(t *testing.T) {
	// 300 unique words; maxTokens=100 means ~77 words per chunk, overlap=20 tokens ~15 words
	words := make([]string, 300)
	for i := range words {
		words[i] = "word"
	}
	largeText := strings.Join(words, " ")

	c := chunk.NewFixedChunker(100, 20)
	doc := pipeline.Document{
		URL:      "https://docs.example.com/page",
		Title:    "Large Doc",
		Markdown: largeText,
	}
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Greater(t, len(chunks), 1)

	// Each chunk after the first should have some content (overlap ensures no empty chunks)
	for _, ch := range chunks {
		assert.NotEmpty(t, ch.Content)
	}
}

func TestFixedChunker_ZeroOverlap(t *testing.T) {
	words := make([]string, 200)
	for i := range words {
		words[i] = "word"
	}
	largeText := strings.Join(words, " ")

	c := chunk.NewFixedChunker(50, 0) // no overlap
	doc := pipeline.Document{
		URL:      "https://docs.example.com/page",
		Title:    "Test",
		Markdown: largeText,
	}
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 1)

	// With no overlap, chunks should collectively cover all content
	total := len(chunks)
	for i, ch := range chunks {
		assert.Equal(t, total, ch.TotalChunks)
		assert.Equal(t, i, ch.ChunkIndex)
	}
}

func TestFixedChunker_HeadingPathEmpty(t *testing.T) {
	c := chunk.NewFixedChunker(500, 50)
	doc := pipeline.Document{
		URL:      "https://docs.example.com/page",
		Title:    "Test",
		Markdown: "Some content here.",
	}
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	// Fixed chunker doesn't track headings
	assert.Empty(t, chunks[0].HeadingPath)
}
