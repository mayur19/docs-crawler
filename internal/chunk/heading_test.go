package chunk_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mayur19/docs-crawler/internal/chunk"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDoc(markdown string) pipeline.Document {
	return pipeline.Document{
		URL:       "https://docs.example.com/page",
		Title:     "Test Page",
		Markdown:  markdown,
		CrawledAt: time.Now(),
	}
}

func TestHeadingChunker_Name(t *testing.T) {
	c := chunk.NewHeadingChunker(500)
	assert.Equal(t, "heading", c.Name())
}

func TestHeadingChunker_EmptyDoc(t *testing.T) {
	c := chunk.NewHeadingChunker(500)
	doc := newDoc("")
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestHeadingChunker_NoHeadings(t *testing.T) {
	c := chunk.NewHeadingChunker(500)
	doc := newDoc("Just plain text without any headings.\n\nAnother paragraph here.")
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "Just plain text")
	assert.Equal(t, []string{}, chunks[0].HeadingPath)
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, 1, chunks[0].TotalChunks)
}

func TestHeadingChunker_SimpleDoc(t *testing.T) {
	md := strings.TrimSpace(`
## Introduction

This is the intro section.

## Getting Started

Steps to get started.
`) + "\n"

	c := chunk.NewHeadingChunker(500)
	doc := newDoc(md)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 2)

	assert.Equal(t, []string{"Introduction"}, chunks[0].HeadingPath)
	assert.Contains(t, chunks[0].Content, "Introduction")
	assert.Contains(t, chunks[0].Content, "This is the intro section.")

	assert.Equal(t, []string{"Getting Started"}, chunks[1].HeadingPath)
	assert.Contains(t, chunks[1].Content, "Getting Started")
	assert.Contains(t, chunks[1].Content, "Steps to get started.")

	// Verify indices are correct
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, 2, chunks[0].TotalChunks)
	assert.Equal(t, 1, chunks[1].ChunkIndex)
	assert.Equal(t, 2, chunks[1].TotalChunks)
}

func TestHeadingChunker_NestedHeadings(t *testing.T) {
	md := strings.TrimSpace(`
## Authentication

Overview of auth.

### API Keys

Use API keys to authenticate.

### OAuth

Use OAuth for user flows.

## Billing

Billing information.
`) + "\n"

	c := chunk.NewHeadingChunker(500)
	doc := newDoc(md)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 4)

	assert.Equal(t, []string{"Authentication"}, chunks[0].HeadingPath)
	assert.Contains(t, chunks[0].Content, "Overview of auth.")

	assert.Equal(t, []string{"Authentication", "API Keys"}, chunks[1].HeadingPath)
	assert.Contains(t, chunks[1].Content, "Use API keys to authenticate.")

	assert.Equal(t, []string{"Authentication", "OAuth"}, chunks[2].HeadingPath)
	assert.Contains(t, chunks[2].Content, "Use OAuth for user flows.")

	assert.Equal(t, []string{"Billing"}, chunks[3].HeadingPath)
	assert.Contains(t, chunks[3].Content, "Billing information.")

	// All chunks should have TotalChunks = 4
	for _, ch := range chunks {
		assert.Equal(t, 4, ch.TotalChunks)
	}
}

func TestHeadingChunker_CodeBlockPreservation(t *testing.T) {
	md := strings.TrimSpace(`
## Examples

Here is some code:

` + "```go" + `
func main() {
    ## This looks like a heading but is inside a code block
    ### Also a fake heading
    fmt.Println("Hello, World!")
}
` + "```" + `

After the code block.
`) + "\n"

	c := chunk.NewHeadingChunker(500)
	doc := newDoc(md)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)

	// Should be a single chunk since ## inside code block is not a heading
	require.Len(t, chunks, 1)
	assert.Equal(t, []string{"Examples"}, chunks[0].HeadingPath)
	assert.Contains(t, chunks[0].Content, "## This looks like a heading but is inside a code block")
}

func TestHeadingChunker_OversizedSectionSplitting(t *testing.T) {
	// Build a section that exceeds maxTokens by having many paragraphs
	var sb strings.Builder
	sb.WriteString("## Large Section\n\n")
	// Write many paragraphs to exceed 50-token limit
	for i := 0; i < 20; i++ {
		sb.WriteString("This is paragraph number with some content to make it longer than expected.\n\n")
	}

	c := chunk.NewHeadingChunker(50) // very small maxTokens to force splitting
	doc := newDoc(sb.String())
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)

	// Should be split into multiple chunks
	assert.Greater(t, len(chunks), 1)

	// All chunks should have the same heading path
	for _, ch := range chunks {
		assert.Equal(t, []string{"Large Section"}, ch.HeadingPath)
	}

	// TotalChunks should match the actual count
	total := len(chunks)
	for i, ch := range chunks {
		assert.Equal(t, total, ch.TotalChunks)
		assert.Equal(t, i, ch.ChunkIndex)
	}
}

func TestHeadingChunker_DeterministicIDs(t *testing.T) {
	md := "## Section One\n\nContent here.\n\n## Section Two\n\nMore content.\n"
	c := chunk.NewHeadingChunker(500)
	doc := newDoc(md)

	chunks1, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)

	chunks2, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)

	require.Equal(t, len(chunks1), len(chunks2))
	for i := range chunks1 {
		assert.Equal(t, chunks1[i].ID, chunks2[i].ID, "chunk %d ID should be deterministic", i)
	}
}

func TestHeadingChunker_SourceURL(t *testing.T) {
	md := "## Section\n\nContent.\n"
	c := chunk.NewHeadingChunker(500)
	doc := newDoc(md)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.NotEmpty(t, chunks)
	assert.Equal(t, doc.URL, chunks[0].SourceURL)
}

func TestHeadingChunker_Level2ResetsPath(t *testing.T) {
	md := strings.TrimSpace(`
## Parent A

Content A.

### Child A1

Content A1.

## Parent B

Content B.

### Child B1

Content B1.
`) + "\n"

	c := chunk.NewHeadingChunker(500)
	doc := newDoc(md)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 4)

	assert.Equal(t, []string{"Parent A"}, chunks[0].HeadingPath)
	assert.Equal(t, []string{"Parent A", "Child A1"}, chunks[1].HeadingPath)
	assert.Equal(t, []string{"Parent B"}, chunks[2].HeadingPath)
	assert.Equal(t, []string{"Parent B", "Child B1"}, chunks[3].HeadingPath)
}
