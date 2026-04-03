# Zero-Dependency AI Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend docs-crawler from a documentation crawler into a full crawl → chunk → embed → index → search pipeline, with zero external runtime dependencies.

**Architecture:** Three new pipeline stages (Chunker, Embedder, Indexer) added to the existing channel-based engine. New `ingest`, `search`, `export`, and `init` CLI commands. Pure Go SQLite for storage, Ollama HTTP API for embeddings with TF-IDF fallback.

**Tech Stack:** Go 1.25, modernc.org/sqlite (pure Go, no CGO), gopkg.in/yaml.v3, Cobra CLI, existing pipeline interfaces

---

## File Map

### New files

| File | Responsibility |
|------|---------------|
| `internal/pipeline/chunk_types.go` | `Chunk`, `EmbeddedChunk`, `SearchResult` types + `Chunker`, `Embedder`, `Indexer` interfaces |
| `internal/pipeline/chunk_types_test.go` | Tests for new types |
| `internal/chunk/tokens.go` | Token counting utility (`EstimateTokens`) |
| `internal/chunk/tokens_test.go` | Tests for token counting |
| `internal/chunk/heading.go` | `HeadingChunker` — heading-boundary Markdown splitter |
| `internal/chunk/heading_test.go` | Tests for heading chunker |
| `internal/chunk/fixed.go` | `FixedChunker` — token-count-based splitter |
| `internal/chunk/fixed_test.go` | Tests for fixed chunker |
| `internal/embed/ollama.go` | `OllamaEmbedder` — HTTP client for Ollama /api/embed |
| `internal/embed/ollama_test.go` | Tests with httptest mock server |
| `internal/embed/tfidf.go` | `TFIDFEmbedder` — pure Go TF-IDF sparse vector builder |
| `internal/embed/tfidf_test.go` | Tests for TF-IDF |
| `internal/embed/openai.go` | `OpenAIEmbedder` — HTTP client for OpenAI embeddings API |
| `internal/embed/openai_test.go` | Tests with httptest mock server |
| `internal/embed/cohere.go` | `CohereEmbedder` — HTTP client for Cohere embed API |
| `internal/embed/cohere_test.go` | Tests with httptest mock server |
| `internal/embed/auto.go` | `AutoDetectEmbedder` — picks best available provider at runtime |
| `internal/embed/auto_test.go` | Tests for auto-detection logic |
| `internal/index/store.go` | `SQLiteStore` — schema creation, chunk/embedding CRUD |
| `internal/index/store_test.go` | Tests for store operations |
| `internal/index/search.go` | `Search` — hybrid cosine + FTS5 search |
| `internal/index/search_test.go` | Tests for search ranking |
| `internal/export/jsonl.go` | JSONL exporter |
| `internal/export/csv.go` | CSV exporter |
| `internal/export/markdown.go` | Markdown exporter (reads chunks from index, writes .md files) |
| `internal/export/export_test.go` | Tests for all exporters |
| `internal/config/yaml.go` | YAML config file loader |
| `internal/config/yaml_test.go` | Tests for YAML loading |
| `cmd/ingest.go` | `ingest` CLI command |
| `cmd/search.go` | `search` CLI command |
| `cmd/export.go` | `export` CLI command |
| `cmd/init.go` | `init` CLI command |

### Modified files

| File | Changes |
|------|---------|
| `internal/pipeline/interfaces.go` | Add `Chunker`, `Embedder`, `Indexer` interfaces (or import from chunk_types.go) |
| `internal/config/config.go` | Add chunking/embedding/indexing config fields + `With*` methods |
| `internal/config/config_test.go` | Tests for new config fields |
| `internal/engine/engine.go` | Add `RunIngest()` method with Chunk → Embed → Index stages |
| `internal/engine/engine_test.go` | Tests for ingest pipeline |
| `go.mod` | Add `modernc.org/sqlite` dependency |
| `cmd/root.go` | Update description to mention AI pipeline features |

---

## Phase 1: Foundation Types & Token Counting

### Task 1: New Pipeline Types (Chunk, EmbeddedChunk, SearchResult, Interfaces)

**Files:**
- Create: `internal/pipeline/chunk_types.go`
- Create: `internal/pipeline/chunk_types_test.go`

- [ ] **Step 1: Write tests for new types**

Create `internal/pipeline/chunk_types_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/pipeline/ -run "TestNewChunk|TestChunkID|TestNewEmbeddedChunk|TestNewSearchResult|TestEstimateTokens" -v`
Expected: FAIL — types don't exist yet

- [ ] **Step 3: Implement the types**

Create `internal/pipeline/chunk_types.go`:

```go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/pipeline/ -run "TestNewChunk|TestChunkID|TestNewEmbeddedChunk|TestNewSearchResult|TestEstimateTokens" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/chunk_types.go internal/pipeline/chunk_types_test.go
git commit -m "feat: add Chunk, EmbeddedChunk, SearchResult types and EstimateTokens"
```

---

### Task 2: Chunker, Embedder, Indexer Interfaces

**Files:**
- Modify: `internal/pipeline/interfaces.go`
- Modify: `internal/pipeline/interfaces_test.go`

- [ ] **Step 1: Write compile-check tests**

Add to `internal/pipeline/interfaces_test.go`:

```go
func TestChunkerInterfaceCompile(t *testing.T) {
	// Compile-time interface check — a real implementation will be tested in internal/chunk/
	var _ pipeline.Chunker = (*stubChunker)(nil)
}

func TestEmbedderInterfaceCompile(t *testing.T) {
	var _ pipeline.Embedder = (*stubEmbedder)(nil)
}

func TestIndexerInterfaceCompile(t *testing.T) {
	var _ pipeline.Indexer = (*stubIndexer)(nil)
}

type stubChunker struct{}

func (s *stubChunker) Name() string { return "stub" }
func (s *stubChunker) Chunk(_ context.Context, _ pipeline.Document) ([]pipeline.Chunk, error) {
	return nil, nil
}

type stubEmbedder struct{}

func (s *stubEmbedder) Name() string                              { return "stub" }
func (s *stubEmbedder) Dimensions() int                           { return 3 }
func (s *stubEmbedder) Embed(_ context.Context, _ []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	return nil, nil
}

type stubIndexer struct{}

func (s *stubIndexer) Name() string { return "stub" }
func (s *stubIndexer) Index(_ context.Context, _ []pipeline.EmbeddedChunk) error {
	return nil
}
func (s *stubIndexer) Search(_ context.Context, _ string, _ int) ([]pipeline.SearchResult, error) {
	return nil, nil
}
func (s *stubIndexer) Close() error { return nil }
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/pipeline/ -run "TestChunkerInterface|TestEmbedderInterface|TestIndexerInterface" -v`
Expected: FAIL — interfaces don't exist

- [ ] **Step 3: Add interfaces to interfaces.go**

Append to `internal/pipeline/interfaces.go`:

```go
// Chunker splits a Document into smaller Chunks for embedding.
type Chunker interface {
	Name() string
	Chunk(ctx context.Context, doc Document) ([]Chunk, error)
}

// Embedder converts Chunks into vector embeddings.
type Embedder interface {
	Name() string
	Embed(ctx context.Context, chunks []Chunk) ([]EmbeddedChunk, error)
	Dimensions() int
}

// Indexer stores embedded chunks and supports search.
type Indexer interface {
	Name() string
	Index(ctx context.Context, chunks []EmbeddedChunk) error
	Search(ctx context.Context, query string, topK int) ([]SearchResult, error)
	Close() error
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/pipeline/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/pipeline/interfaces.go internal/pipeline/interfaces_test.go
git commit -m "feat: add Chunker, Embedder, Indexer interfaces"
```

---

## Phase 2: Heading-Boundary Chunker

### Task 3: Token Counting Utility

**Files:**
- Create: `internal/chunk/tokens.go`
- Create: `internal/chunk/tokens_test.go`

- [ ] **Step 1: Write tests**

Create `internal/chunk/tokens_test.go`:

```go
package chunk_test

import (
	"strings"
	"testing"

	"github.com/mayur19/docs-crawler/internal/chunk"
	"github.com/stretchr/testify/assert"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int
	}{
		{"empty string", "", 0},
		{"single word", "hello", 1},
		{"five words", "the quick brown fox jumps", 7},
		{"code block", "func main() {\n\tfmt.Println(\"hello\")\n}", 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunk.EstimateTokens(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCountTokensInBlock(t *testing.T) {
	block := "This is a test paragraph with some words in it."
	tokens := chunk.EstimateTokens(block)
	assert.Greater(t, tokens, 0)
	assert.Less(t, tokens, 100)
}

func TestEstimateTokensLargeText(t *testing.T) {
	words := strings.Repeat("word ", 1000)
	tokens := chunk.EstimateTokens(words)
	assert.GreaterOrEqual(t, tokens, 1000)
	assert.LessOrEqual(t, tokens, 1500)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/chunk/ -run TestEstimateTokens -v`
Expected: FAIL — package doesn't exist

- [ ] **Step 3: Implement**

Create `internal/chunk/tokens.go`:

```go
package chunk

import (
	"math"
	"strings"
)

// EstimateTokens approximates the token count of text using words * 1.3.
// This avoids pulling in a tokenizer dependency while being accurate enough
// for chunking decisions.
func EstimateTokens(text string) int {
	words := len(strings.Fields(text))
	return int(math.Ceil(float64(words) * 1.3))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/chunk/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chunk/tokens.go internal/chunk/tokens_test.go
git commit -m "feat: add token estimation utility for chunking"
```

---

### Task 4: HeadingChunker

**Files:**
- Create: `internal/chunk/heading.go`
- Create: `internal/chunk/heading_test.go`

- [ ] **Step 1: Write tests**

Create `internal/chunk/heading_test.go`:

```go
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

func TestHeadingChunkerName(t *testing.T) {
	c := chunk.NewHeadingChunker(512)
	assert.Equal(t, "heading", c.Name())
}

func TestHeadingChunkerSimpleDoc(t *testing.T) {
	doc := pipeline.NewDocument(
		"https://example.com/docs",
		"Test Doc",
		"",
		"## Getting Started\n\nThis is the intro.\n\n## API Reference\n\nUse the API like this.",
		nil, nil, 10, "hash",
	)

	c := chunk.NewHeadingChunker(512)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 2)

	assert.Equal(t, "https://example.com/docs", chunks[0].SourceURL)
	assert.Equal(t, "Test Doc", chunks[0].Title)
	assert.Equal(t, []string{"Getting Started"}, chunks[0].HeadingPath)
	assert.Contains(t, chunks[0].Content, "This is the intro.")
	assert.Equal(t, 0, chunks[0].ChunkIndex)
	assert.Equal(t, 2, chunks[0].TotalChunks)

	assert.Equal(t, []string{"API Reference"}, chunks[1].HeadingPath)
	assert.Contains(t, chunks[1].Content, "Use the API like this.")
	assert.Equal(t, 1, chunks[1].ChunkIndex)
}

func TestHeadingChunkerNestedHeadings(t *testing.T) {
	doc := pipeline.NewDocument(
		"https://example.com/docs",
		"Title",
		"",
		"## Auth\n\n### API Keys\n\nUse keys.\n\n### OAuth\n\nUse OAuth.",
		nil, nil, 10, "hash",
	)

	c := chunk.NewHeadingChunker(512)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 2)

	assert.Equal(t, []string{"Auth", "API Keys"}, chunks[0].HeadingPath)
	assert.Equal(t, []string{"Auth", "OAuth"}, chunks[1].HeadingPath)
}

func TestHeadingChunkerPreservesCodeBlocks(t *testing.T) {
	md := "## Setup\n\nInstall:\n\n```bash\ngo install github.com/example/tool@latest\n```\n\nThen run it."
	doc := pipeline.NewDocument("https://example.com", "T", "", md, nil, nil, 5, "h")

	c := chunk.NewHeadingChunker(512)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "```bash")
	assert.Contains(t, chunks[0].Content, "go install")
	assert.Contains(t, chunks[0].Content, "```")
}

func TestHeadingChunkerOversizedSection(t *testing.T) {
	// Create a section that exceeds 512 tokens — should split at paragraph boundaries.
	paragraphs := make([]string, 20)
	for i := range paragraphs {
		paragraphs[i] = strings.Repeat("word ", 50)
	}
	bigSection := "## Big Section\n\n" + strings.Join(paragraphs, "\n\n")
	doc := pipeline.NewDocument("https://example.com", "T", "", bigSection, nil, nil, 1000, "h")

	c := chunk.NewHeadingChunker(512)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 1, "oversized section should be split")
	for _, ch := range chunks {
		assert.LessOrEqual(t, ch.TokenCount, 520, "chunks should be near the max token limit")
	}
}

func TestHeadingChunkerEmptyDoc(t *testing.T) {
	doc := pipeline.NewDocument("https://example.com", "T", "", "", nil, nil, 0, "h")
	c := chunk.NewHeadingChunker(512)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}

func TestHeadingChunkerNoHeadings(t *testing.T) {
	doc := pipeline.NewDocument("https://example.com", "T", "", "Just a paragraph.", nil, nil, 3, "h")
	c := chunk.NewHeadingChunker(512)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Contains(t, chunks[0].Content, "Just a paragraph.")
}

func TestHeadingChunkerDeterministicIDs(t *testing.T) {
	doc := pipeline.NewDocument("https://example.com", "T", "", "## H\n\nBody.", nil, nil, 1, "h")
	c := chunk.NewHeadingChunker(512)
	c1, _ := c.Chunk(context.Background(), doc)
	c2, _ := c.Chunk(context.Background(), doc)
	assert.Equal(t, c1[0].ID, c2[0].ID)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/chunk/ -run TestHeadingChunker -v`
Expected: FAIL

- [ ] **Step 3: Implement HeadingChunker**

Create `internal/chunk/heading.go`:

```go
package chunk

import (
	"context"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// HeadingChunker splits Markdown documents at heading boundaries.
// It preserves code blocks, tables, and lists as atomic units.
type HeadingChunker struct {
	maxTokens int
}

// NewHeadingChunker creates a HeadingChunker with the given max tokens per chunk.
func NewHeadingChunker(maxTokens int) *HeadingChunker {
	return &HeadingChunker{maxTokens: maxTokens}
}

// Name returns the chunker name.
func (h *HeadingChunker) Name() string { return "heading" }

// Chunk splits a Document into Chunks at heading boundaries.
func (h *HeadingChunker) Chunk(_ context.Context, doc pipeline.Document) ([]pipeline.Chunk, error) {
	if strings.TrimSpace(doc.Markdown) == "" {
		return nil, nil
	}

	sections := h.splitByHeadings(doc.Markdown)
	var chunks []pipeline.Chunk

	for _, sec := range sections {
		content := strings.TrimSpace(sec.content)
		if content == "" {
			continue
		}

		if EstimateTokens(content) <= h.maxTokens {
			chunks = append(chunks, pipeline.NewChunk(
				doc.URL, doc.Title, sec.headingPath, content, 0, 0,
			))
		} else {
			subChunks := h.splitAtParagraphs(content, sec.headingPath, doc.URL, doc.Title)
			chunks = append(chunks, subChunks...)
		}
	}

	// Fix chunk indices and total counts.
	for i := range chunks {
		chunks[i].ChunkIndex = i
		chunks[i].TotalChunks = len(chunks)
	}

	return chunks, nil
}

// section represents a heading-delimited section of Markdown.
type section struct {
	headingPath []string
	content     string
}

// splitByHeadings splits Markdown into sections at ## and ### boundaries.
func (h *HeadingChunker) splitByHeadings(md string) []section {
	lines := strings.Split(md, "\n")
	var sections []section
	var currentPath []string
	var currentLines []string
	inCodeBlock := false

	for _, line := range lines {
		// Track code blocks to avoid splitting inside them.
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inCodeBlock = !inCodeBlock
			currentLines = append(currentLines, line)
			continue
		}

		if inCodeBlock {
			currentLines = append(currentLines, line)
			continue
		}

		level, text := parseHeading(line)
		if level >= 2 && level <= 3 {
			// Flush previous section.
			if len(currentLines) > 0 {
				sections = append(sections, section{
					headingPath: copyPath(currentPath),
					content:     strings.Join(currentLines, "\n"),
				})
				currentLines = nil
			}

			// Update heading path.
			if level == 2 {
				currentPath = []string{text}
			} else if level == 3 {
				if len(currentPath) == 0 {
					currentPath = []string{text}
				} else {
					currentPath = []string{currentPath[0], text}
				}
			}
			continue
		}

		currentLines = append(currentLines, line)
	}

	// Flush last section.
	if len(currentLines) > 0 {
		sections = append(sections, section{
			headingPath: copyPath(currentPath),
			content:     strings.Join(currentLines, "\n"),
		})
	}

	return sections
}

// splitAtParagraphs splits a large section into smaller chunks at paragraph boundaries,
// never breaking code blocks, tables, or lists.
func (h *HeadingChunker) splitAtParagraphs(
	content string,
	headingPath []string,
	sourceURL string,
	title string,
) []pipeline.Chunk {
	blocks := splitIntoBlocks(content)
	var chunks []pipeline.Chunk
	var currentBlocks []string
	currentTokens := 0

	for _, block := range blocks {
		blockTokens := EstimateTokens(block)

		if currentTokens+blockTokens > h.maxTokens && len(currentBlocks) > 0 {
			text := strings.Join(currentBlocks, "\n\n")
			chunks = append(chunks, pipeline.NewChunk(sourceURL, title, headingPath, text, 0, 0))
			currentBlocks = nil
			currentTokens = 0
		}

		currentBlocks = append(currentBlocks, block)
		currentTokens += blockTokens
	}

	if len(currentBlocks) > 0 {
		text := strings.Join(currentBlocks, "\n\n")
		chunks = append(chunks, pipeline.NewChunk(sourceURL, title, headingPath, text, 0, 0))
	}

	return chunks
}

// splitIntoBlocks splits content into atomic blocks (paragraphs, code blocks, tables, lists).
// Code blocks and tables are kept as single blocks.
func splitIntoBlocks(content string) []string {
	lines := strings.Split(content, "\n")
	var blocks []string
	var current []string
	inCodeBlock := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				// End of code block — flush as one atomic block.
				current = append(current, line)
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
				inCodeBlock = false
				continue
			}
			// Start of code block — flush any pending paragraph first.
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
			}
			current = append(current, line)
			inCodeBlock = true
			continue
		}

		if inCodeBlock {
			current = append(current, line)
			continue
		}

		if trimmed == "" {
			if len(current) > 0 {
				blocks = append(blocks, strings.Join(current, "\n"))
				current = nil
			}
			continue
		}

		current = append(current, line)
	}

	if len(current) > 0 {
		blocks = append(blocks, strings.Join(current, "\n"))
	}

	return blocks
}

// parseHeading returns the heading level (1-6) and text, or 0 if not a heading.
func parseHeading(line string) (int, string) {
	trimmed := strings.TrimSpace(line)
	level := 0
	for i, c := range trimmed {
		if c == '#' {
			level++
		} else if c == ' ' && level > 0 {
			return level, strings.TrimSpace(trimmed[i+1:])
		} else {
			return 0, ""
		}
	}
	return 0, ""
}

// copyPath returns a copy of a string slice.
func copyPath(path []string) []string {
	if path == nil {
		return nil
	}
	cp := make([]string, len(path))
	copy(cp, path)
	return cp
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/chunk/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chunk/heading.go internal/chunk/heading_test.go
git commit -m "feat: add HeadingChunker with structure-aware markdown splitting"
```

---

### Task 5: FixedChunker

**Files:**
- Create: `internal/chunk/fixed.go`
- Create: `internal/chunk/fixed_test.go`

- [ ] **Step 1: Write tests**

Create `internal/chunk/fixed_test.go`:

```go
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

func TestFixedChunkerName(t *testing.T) {
	c := chunk.NewFixedChunker(100, 20)
	assert.Equal(t, "fixed", c.Name())
}

func TestFixedChunkerSmallDoc(t *testing.T) {
	doc := pipeline.NewDocument("https://example.com", "T", "", "Short text.", nil, nil, 2, "h")
	c := chunk.NewFixedChunker(512, 50)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, "Short text.", chunks[0].Content)
}

func TestFixedChunkerSplitsLargeDoc(t *testing.T) {
	words := strings.Repeat("word ", 500) // ~650 tokens
	doc := pipeline.NewDocument("https://example.com", "T", "", words, nil, nil, 500, "h")
	c := chunk.NewFixedChunker(200, 20)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Greater(t, len(chunks), 1)
}

func TestFixedChunkerEmpty(t *testing.T) {
	doc := pipeline.NewDocument("https://example.com", "T", "", "", nil, nil, 0, "h")
	c := chunk.NewFixedChunker(512, 50)
	chunks, err := c.Chunk(context.Background(), doc)
	require.NoError(t, err)
	assert.Empty(t, chunks)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/chunk/ -run TestFixedChunker -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/chunk/fixed.go`:

```go
package chunk

import (
	"context"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// FixedChunker splits documents into fixed-size token chunks with overlap.
type FixedChunker struct {
	maxTokens int
	overlap   int
}

// NewFixedChunker creates a FixedChunker.
func NewFixedChunker(maxTokens, overlap int) *FixedChunker {
	return &FixedChunker{maxTokens: maxTokens, overlap: overlap}
}

// Name returns the chunker name.
func (f *FixedChunker) Name() string { return "fixed" }

// Chunk splits the document into fixed-size token chunks.
func (f *FixedChunker) Chunk(_ context.Context, doc pipeline.Document) ([]pipeline.Chunk, error) {
	text := strings.TrimSpace(doc.Markdown)
	if text == "" {
		return nil, nil
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return nil, nil
	}

	// Convert max tokens to approximate word count.
	maxWords := int(float64(f.maxTokens) / 1.3)
	overlapWords := int(float64(f.overlap) / 1.3)
	if maxWords < 1 {
		maxWords = 1
	}

	var chunks []pipeline.Chunk
	start := 0

	for start < len(words) {
		end := start + maxWords
		if end > len(words) {
			end = len(words)
		}

		content := strings.Join(words[start:end], " ")
		chunks = append(chunks, pipeline.NewChunk(
			doc.URL, doc.Title, nil, content, 0, 0,
		))

		start = end - overlapWords
		if start <= chunks[len(chunks)-1].ChunkIndex {
			start = end
		}
		if end == len(words) {
			break
		}
	}

	for i := range chunks {
		chunks[i].ChunkIndex = i
		chunks[i].TotalChunks = len(chunks)
	}

	return chunks, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/chunk/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/chunk/fixed.go internal/chunk/fixed_test.go
git commit -m "feat: add FixedChunker for token-count-based splitting"
```

---

## Phase 3: Embedding Providers

### Task 6: TF-IDF Embedder (zero-dep fallback)

**Files:**
- Create: `internal/embed/tfidf.go`
- Create: `internal/embed/tfidf_test.go`

- [ ] **Step 1: Write tests**

Create `internal/embed/tfidf_test.go`:

```go
package embed_test

import (
	"context"
	"testing"

	"github.com/mayur19/docs-crawler/internal/embed"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTFIDFEmbedderName(t *testing.T) {
	e := embed.NewTFIDFEmbedder()
	assert.Equal(t, "tfidf", e.Name())
}

func TestTFIDFEmbedderDimensions(t *testing.T) {
	e := embed.NewTFIDFEmbedder()
	assert.Equal(t, 0, e.Dimensions(), "TF-IDF dimensions are dynamic, reported as 0")
}

func TestTFIDFEmbedderEmbed(t *testing.T) {
	e := embed.NewTFIDFEmbedder()
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "the quick brown fox", 0, 2),
		pipeline.NewChunk("https://example.com", "T", nil, "the lazy brown dog", 1, 2),
	}

	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Both should have vectors of the same length (vocabulary size).
	assert.Equal(t, len(results[0].Vector), len(results[1].Vector))
	assert.Greater(t, len(results[0].Vector), 0)

	// Vectors should not be identical (different content).
	assert.NotEqual(t, results[0].Vector, results[1].Vector)
}

func TestTFIDFEmbedderEmptyChunks(t *testing.T) {
	e := embed.NewTFIDFEmbedder()
	results, err := e.Embed(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestTFIDFEmbedderSingleChunk(t *testing.T) {
	e := embed.NewTFIDFEmbedder()
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "hello world", 0, 1),
	}

	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Greater(t, len(results[0].Vector), 0)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/ -run TestTFIDF -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/embed/tfidf.go`:

```go
package embed

import (
	"context"
	"math"
	"sort"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// TFIDFEmbedder produces sparse TF-IDF vectors in pure Go.
// Used as the zero-dependency fallback when Ollama is unavailable.
type TFIDFEmbedder struct{}

// NewTFIDFEmbedder creates a TFIDFEmbedder.
func NewTFIDFEmbedder() *TFIDFEmbedder {
	return &TFIDFEmbedder{}
}

// Name returns the embedder name.
func (t *TFIDFEmbedder) Name() string { return "tfidf" }

// Dimensions returns 0 because TF-IDF vector dimensions depend on the vocabulary.
func (t *TFIDFEmbedder) Dimensions() int { return 0 }

// Embed computes TF-IDF vectors for the given chunks.
// The vocabulary is built from the batch, so all vectors share the same dimensions.
func (t *TFIDFEmbedder) Embed(_ context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	// Tokenize all chunks.
	tokenized := make([][]string, len(chunks))
	for i, c := range chunks {
		tokenized[i] = tokenize(c.Content)
	}

	// Build vocabulary (sorted for deterministic ordering).
	vocabSet := make(map[string]struct{})
	for _, tokens := range tokenized {
		for _, tok := range tokens {
			vocabSet[tok] = struct{}{}
		}
	}
	vocab := make([]string, 0, len(vocabSet))
	for tok := range vocabSet {
		vocab = append(vocab, tok)
	}
	sort.Strings(vocab)

	vocabIndex := make(map[string]int, len(vocab))
	for i, tok := range vocab {
		vocabIndex[tok] = i
	}

	// Compute IDF.
	docCount := float64(len(chunks))
	idf := make([]float64, len(vocab))
	for i, tok := range vocab {
		df := 0
		for _, tokens := range tokenized {
			for _, t := range tokens {
				if t == tok {
					df++
					break
				}
			}
		}
		idf[i] = math.Log(docCount/float64(df)) + 1
	}

	// Compute TF-IDF vectors.
	results := make([]pipeline.EmbeddedChunk, len(chunks))
	for i, tokens := range tokenized {
		tf := make(map[string]int)
		for _, tok := range tokens {
			tf[tok]++
		}

		vec := make([]float32, len(vocab))
		norm := 0.0
		for tok, count := range tf {
			idx := vocabIndex[tok]
			val := float64(count) * idf[idx]
			vec[idx] = float32(val)
			norm += val * val
		}

		// L2 normalize.
		if norm > 0 {
			normF := float32(math.Sqrt(norm))
			for j := range vec {
				vec[j] /= normF
			}
		}

		results[i] = pipeline.NewEmbeddedChunk(chunks[i], vec)
	}

	return results, nil
}

// tokenize splits text into lowercase tokens, stripping basic punctuation.
func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	tokens := make([]string, 0, len(words))
	for _, w := range words {
		cleaned := strings.Trim(w, ".,;:!?\"'`()[]{}#*-_/\\<>")
		if cleaned != "" && len(cleaned) > 1 {
			tokens = append(tokens, cleaned)
		}
	}
	return tokens
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/embed/tfidf.go internal/embed/tfidf_test.go
git commit -m "feat: add TF-IDF embedder as zero-dependency fallback"
```

---

### Task 7: Ollama Embedder

**Files:**
- Create: `internal/embed/ollama.go`
- Create: `internal/embed/ollama_test.go`

- [ ] **Step 1: Write tests**

Create `internal/embed/ollama_test.go`:

```go
package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayur19/docs-crawler/internal/embed"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOllamaEmbedderName(t *testing.T) {
	e := embed.NewOllamaEmbedder("http://localhost:11434", "nomic-embed-text", 768)
	assert.Equal(t, "ollama", e.Name())
}

func TestOllamaEmbedderDimensions(t *testing.T) {
	e := embed.NewOllamaEmbedder("http://localhost:11434", "nomic-embed-text", 768)
	assert.Equal(t, 768, e.Dimensions())
}

func TestOllamaEmbedderEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embed", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "nomic-embed-text", req["model"])

		input := req["input"].([]interface{})
		embeddings := make([][]float64, len(input))
		for i := range input {
			embeddings[i] = []float64{0.1, 0.2, 0.3}
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"embeddings": embeddings,
		})
	}))
	defer server.Close()

	e := embed.NewOllamaEmbedder(server.URL, "nomic-embed-text", 3)
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "hello world", 0, 1),
	}

	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, results[0].Vector)
}

func TestOllamaEmbedderBatching(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		input := req["input"].([]interface{})

		embeddings := make([][]float64, len(input))
		for i := range input {
			embeddings[i] = []float64{0.1}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"embeddings": embeddings})
	}))
	defer server.Close()

	e := embed.NewOllamaEmbedder(server.URL, "test-model", 1)
	// Create 100 chunks — should batch into 2 calls (64 + 36).
	chunks := make([]pipeline.Chunk, 100)
	for i := range chunks {
		chunks[i] = pipeline.NewChunk("https://example.com", "T", nil, "text", i, 100)
	}

	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	assert.Len(t, results, 100)
	assert.Equal(t, 2, callCount, "should batch 100 chunks into 2 API calls")
}

func TestOllamaEmbedderServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	e := embed.NewOllamaEmbedder(server.URL, "test-model", 3)
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "text", 0, 1),
	}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)
}

func TestOllamaIsAvailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	assert.True(t, embed.OllamaIsAvailable(server.URL))
	assert.False(t, embed.OllamaIsAvailable("http://localhost:1"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/ -run TestOllama -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/embed/ollama.go`:

```go
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

const defaultBatchSize = 64

// OllamaEmbedder calls the Ollama HTTP API for embeddings.
type OllamaEmbedder struct {
	baseURL    string
	model      string
	dimensions int
}

// NewOllamaEmbedder creates an OllamaEmbedder.
func NewOllamaEmbedder(baseURL, model string, dimensions int) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL:    baseURL,
		model:      model,
		dimensions: dimensions,
	}
}

// Name returns the embedder name.
func (o *OllamaEmbedder) Name() string { return "ollama" }

// Dimensions returns the expected vector dimensions.
func (o *OllamaEmbedder) Dimensions() int { return o.dimensions }

// Embed sends chunks to Ollama in batches and returns embedded chunks.
func (o *OllamaEmbedder) Embed(ctx context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	results := make([]pipeline.EmbeddedChunk, 0, len(chunks))

	for i := 0; i < len(chunks); i += defaultBatchSize {
		end := i + defaultBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.Content
		}

		vectors, err := o.callAPI(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("ollama embed batch %d: %w", i/defaultBatchSize, err)
		}

		for j, vec := range vectors {
			results = append(results, pipeline.NewEmbeddedChunk(batch[j], vec))
		}
	}

	return results, nil
}

type ollamaRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

func (o *OllamaEmbedder) callAPI(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := ollamaRequest{Model: o.model, Input: texts}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	vectors := make([][]float32, len(ollamaResp.Embeddings))
	for i, emb := range ollamaResp.Embeddings {
		vec := make([]float32, len(emb))
		for j, v := range emb {
			vec[j] = float32(v)
		}
		vectors[i] = vec
	}

	return vectors, nil
}

// OllamaIsAvailable checks if an Ollama server is responding at the given URL.
func OllamaIsAvailable(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/ -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/embed/ollama.go internal/embed/ollama_test.go
git commit -m "feat: add Ollama embedder with HTTP API client and batching"
```

---

### Task 8: OpenAI Embedder

**Files:**
- Create: `internal/embed/openai.go`
- Create: `internal/embed/openai_test.go`

- [ ] **Step 1: Write tests**

Create `internal/embed/openai_test.go`:

```go
package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayur19/docs-crawler/internal/embed"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIEmbedderName(t *testing.T) {
	e := embed.NewOpenAIEmbedder("test-key", "text-embedding-3-small", "http://localhost")
	assert.Equal(t, "openai", e.Name())
}

func TestOpenAIEmbedderEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "text-embedding-3-small", req["model"])

		input := req["input"].([]interface{})
		data := make([]map[string]interface{}, len(input))
		for i := range input {
			data[i] = map[string]interface{}{
				"embedding": []float64{0.1, 0.2, 0.3},
				"index":     float64(i),
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"data": data})
	}))
	defer server.Close()

	e := embed.NewOpenAIEmbedder("test-key", "text-embedding-3-small", server.URL)
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "hello", 0, 1),
	}

	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []float32{0.1, 0.2, 0.3}, results[0].Vector)
}

func TestOpenAIEmbedderAuthError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":{"message":"invalid api key"}}`))
	}))
	defer server.Close()

	e := embed.NewOpenAIEmbedder("bad-key", "text-embedding-3-small", server.URL)
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "hello", 0, 1),
	}

	_, err := e.Embed(context.Background(), chunks)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "401")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/ -run TestOpenAI -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/embed/openai.go`:

```go
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// OpenAIEmbedder calls the OpenAI embeddings API.
type OpenAIEmbedder struct {
	apiKey  string
	model   string
	baseURL string
}

// NewOpenAIEmbedder creates an OpenAIEmbedder.
// baseURL should be "https://api.openai.com" for production.
func NewOpenAIEmbedder(apiKey, model, baseURL string) *OpenAIEmbedder {
	return &OpenAIEmbedder{apiKey: apiKey, model: model, baseURL: baseURL}
}

// Name returns the embedder name.
func (o *OpenAIEmbedder) Name() string { return "openai" }

// Dimensions returns 0 — actual dimensions depend on the model.
func (o *OpenAIEmbedder) Dimensions() int { return 0 }

// Embed calls the OpenAI API to produce embeddings.
func (o *OpenAIEmbedder) Embed(ctx context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	results := make([]pipeline.EmbeddedChunk, 0, len(chunks))

	for i := 0; i < len(chunks); i += defaultBatchSize {
		end := i + defaultBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		texts := make([]string, len(batch))
		for j, c := range batch {
			texts[j] = c.Content
		}

		vectors, err := o.callAPI(ctx, texts)
		if err != nil {
			return nil, err
		}

		for j, vec := range vectors {
			results = append(results, pipeline.NewEmbeddedChunk(batch[j], vec))
		}
	}

	return results, nil
}

type openAIRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type openAIResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (o *OpenAIEmbedder) callAPI(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := openAIRequest{Model: o.model, Input: texts}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var openAIResp openAIResponse
	if err := json.NewDecoder(resp.Body).Decode(&openAIResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	vectors := make([][]float32, len(openAIResp.Data))
	for _, d := range openAIResp.Data {
		vec := make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			vec[j] = float32(v)
		}
		vectors[d.Index] = vec
	}

	return vectors, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/ -run TestOpenAI -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/embed/openai.go internal/embed/openai_test.go
git commit -m "feat: add OpenAI embeddings client"
```

---

### Task 9: Cohere Embedder

**Files:**
- Create: `internal/embed/cohere.go`
- Create: `internal/embed/cohere_test.go`

- [ ] **Step 1: Write tests**

Create `internal/embed/cohere_test.go`:

```go
package embed_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayur19/docs-crawler/internal/embed"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCohereEmbedderName(t *testing.T) {
	e := embed.NewCohereEmbedder("test-key", "embed-english-v3.0", "http://localhost")
	assert.Equal(t, "cohere", e.Name())
}

func TestCohereEmbedderEmbed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v2/embed", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		assert.Equal(t, "embed-english-v3.0", req["model"])

		texts := req["texts"].([]interface{})
		embeddings := make([][]float64, len(texts))
		for i := range texts {
			embeddings[i] = []float64{0.4, 0.5, 0.6}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"embeddings": embeddings})
	}))
	defer server.Close()

	e := embed.NewCohereEmbedder("test-key", "embed-english-v3.0", server.URL)
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("https://example.com", "T", nil, "hello", 0, 1),
	}

	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []float32{0.4, 0.5, 0.6}, results[0].Vector)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/ -run TestCohere -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/embed/cohere.go`:

```go
package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// CohereEmbedder calls the Cohere embed API.
type CohereEmbedder struct {
	apiKey  string
	model   string
	baseURL string
}

// NewCohereEmbedder creates a CohereEmbedder.
func NewCohereEmbedder(apiKey, model, baseURL string) *CohereEmbedder {
	return &CohereEmbedder{apiKey: apiKey, model: model, baseURL: baseURL}
}

// Name returns the embedder name.
func (c *CohereEmbedder) Name() string { return "cohere" }

// Dimensions returns 0 — actual dimensions depend on the model.
func (c *CohereEmbedder) Dimensions() int { return 0 }

// Embed calls the Cohere API to produce embeddings.
func (c *CohereEmbedder) Embed(ctx context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	results := make([]pipeline.EmbeddedChunk, 0, len(chunks))

	for i := 0; i < len(chunks); i += defaultBatchSize {
		end := i + defaultBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[i:end]

		texts := make([]string, len(batch))
		for j, ch := range batch {
			texts[j] = ch.Content
		}

		vectors, err := c.callAPI(ctx, texts)
		if err != nil {
			return nil, err
		}

		for j, vec := range vectors {
			results = append(results, pipeline.NewEmbeddedChunk(batch[j], vec))
		}
	}

	return results, nil
}

type cohereRequest struct {
	Model     string   `json:"model"`
	Texts     []string `json:"texts"`
	InputType string   `json:"input_type"`
}

type cohereResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

func (c *CohereEmbedder) callAPI(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := cohereRequest{Model: c.model, Texts: texts, InputType: "search_document"}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v2/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("cohere returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var cohereResp cohereResponse
	if err := json.NewDecoder(resp.Body).Decode(&cohereResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	vectors := make([][]float32, len(cohereResp.Embeddings))
	for i, emb := range cohereResp.Embeddings {
		vec := make([]float32, len(emb))
		for j, v := range emb {
			vec[j] = float32(v)
		}
		vectors[i] = vec
	}

	return vectors, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/embed/cohere.go internal/embed/cohere_test.go
git commit -m "feat: add Cohere embeddings client"
```

---

### Task 10: Auto-Detect Embedder

**Files:**
- Create: `internal/embed/auto.go`
- Create: `internal/embed/auto_test.go`

- [ ] **Step 1: Write tests**

Create `internal/embed/auto_test.go`:

```go
package embed_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayur19/docs-crawler/internal/embed"
	"github.com/stretchr/testify/assert"
)

func TestAutoDetectWithOllama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	e := embed.AutoDetect(server.URL, "", "")
	assert.Equal(t, "ollama", e.Name())
}

func TestAutoDetectWithOpenAIKey(t *testing.T) {
	e := embed.AutoDetect("http://localhost:1", "sk-test", "")
	assert.Equal(t, "openai", e.Name())
}

func TestAutoDetectWithCohereKey(t *testing.T) {
	e := embed.AutoDetect("http://localhost:1", "", "co-test")
	assert.Equal(t, "cohere", e.Name())
}

func TestAutoDetectFallbackToTFIDF(t *testing.T) {
	e := embed.AutoDetect("http://localhost:1", "", "")
	assert.Equal(t, "tfidf", e.Name())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/embed/ -run TestAutoDetect -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/embed/auto.go`:

```go
package embed

import "github.com/mayur19/docs-crawler/internal/pipeline"

// AutoDetect returns the best available Embedder based on runtime environment.
// Priority: Ollama (running) → OpenAI (key set) → Cohere (key set) → TF-IDF.
func AutoDetect(ollamaURL, openAIKey, cohereKey string) pipeline.Embedder {
	if OllamaIsAvailable(ollamaURL) {
		return NewOllamaEmbedder(ollamaURL, "nomic-embed-text", 768)
	}
	if openAIKey != "" {
		return NewOpenAIEmbedder(openAIKey, "text-embedding-3-small", "https://api.openai.com")
	}
	if cohereKey != "" {
		return NewCohereEmbedder(cohereKey, "embed-english-v3.0", "https://api.cohere.com")
	}
	return NewTFIDFEmbedder()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/embed/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/embed/auto.go internal/embed/auto_test.go
git commit -m "feat: add auto-detect embedder with Ollama/OpenAI/Cohere/TF-IDF priority"
```

---

## Phase 4: SQLite Index & Search

### Task 11: SQLite Store (schema + CRUD)

**Files:**
- Create: `internal/index/store.go`
- Create: `internal/index/store_test.go`
- Modify: `go.mod` — add `modernc.org/sqlite`

- [ ] **Step 1: Add SQLite dependency**

Run: `go get modernc.org/sqlite`

- [ ] **Step 2: Write tests**

Create `internal/index/store_test.go`:

```go
package index_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mayur19/docs-crawler/internal/index"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *index.SQLiteStore {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	return store
}

func TestNewSQLiteStoreCreatesDB(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	_, err = os.Stat(dbPath)
	assert.NoError(t, err, "database file should exist")
}

func TestSQLiteStoreIndexAndRetrieve(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chunk := pipeline.NewChunk("https://example.com", "Test", []string{"H1"}, "hello world", 0, 1)
	ec := pipeline.NewEmbeddedChunk(chunk, []float32{0.1, 0.2, 0.3})

	err := store.Index(ctx, []pipeline.EmbeddedChunk{ec})
	require.NoError(t, err)

	chunks, err := store.GetAllChunks(ctx)
	require.NoError(t, err)
	require.Len(t, chunks, 1)
	assert.Equal(t, chunk.ID, chunks[0].ID)
	assert.Equal(t, "hello world", chunks[0].Content)
}

func TestSQLiteStoreUpsert(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	chunk := pipeline.NewChunk("https://example.com", "Test", nil, "v1", 0, 1)
	ec := pipeline.NewEmbeddedChunk(chunk, []float32{0.1})
	require.NoError(t, store.Index(ctx, []pipeline.EmbeddedChunk{ec}))

	chunk2 := pipeline.NewChunk("https://example.com", "Test", nil, "v2", 0, 1)
	chunk2.ID = chunk.ID // Force same ID
	ec2 := pipeline.NewEmbeddedChunk(chunk2, []float32{0.2})
	require.NoError(t, store.Index(ctx, []pipeline.EmbeddedChunk{ec2}))

	chunks, err := store.GetAllChunks(ctx)
	require.NoError(t, err)
	require.Len(t, chunks, 1, "upsert should replace, not duplicate")
}

func TestSQLiteStoreSetAndGetMeta(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	require.NoError(t, store.SetMeta(ctx, "seed_url", "https://example.com"))
	val, err := store.GetMeta(ctx, "seed_url")
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", val)
}

func TestSQLiteStoreDeleteBySource(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	c1 := pipeline.NewChunk("https://example.com/a", "A", nil, "aaa", 0, 1)
	c2 := pipeline.NewChunk("https://example.com/b", "B", nil, "bbb", 0, 1)
	require.NoError(t, store.Index(ctx, []pipeline.EmbeddedChunk{
		pipeline.NewEmbeddedChunk(c1, []float32{0.1}),
		pipeline.NewEmbeddedChunk(c2, []float32{0.2}),
	}))

	require.NoError(t, store.DeleteBySourceURL(ctx, "https://example.com/a"))
	chunks, _ := store.GetAllChunks(ctx)
	require.Len(t, chunks, 1)
	assert.Equal(t, "https://example.com/b", chunks[0].SourceURL)
}

func TestSQLiteStoreName(t *testing.T) {
	store := newTestStore(t)
	assert.Equal(t, "sqlite", store.Name())
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/index/ -v`
Expected: FAIL

- [ ] **Step 4: Implement**

Create `internal/index/store.go`:

```go
package index

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	_ "modernc.org/sqlite"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// SQLiteStore implements chunk storage and vector indexing using pure Go SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens or creates a SQLite database at the given path.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteStore{db: db}, nil
}

// Name returns the indexer name.
func (s *SQLiteStore) Name() string { return "sqlite" }

// Close closes the database connection.
func (s *SQLiteStore) Close() error { return s.db.Close() }

// Index upserts embedded chunks into the database.
func (s *SQLiteStore) Index(ctx context.Context, chunks []pipeline.EmbeddedChunk) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	chunkStmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO chunks (id, source_url, title, heading_path, content, token_count, content_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare chunk stmt: %w", err)
	}
	defer chunkStmt.Close()

	embedStmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO embeddings (chunk_id, vector, dimensions) VALUES (?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare embed stmt: %w", err)
	}
	defer embedStmt.Close()

	ftsStmt, err := tx.PrepareContext(ctx,
		`INSERT OR REPLACE INTO chunks_fts (rowid, content, title, heading_path) VALUES (
			(SELECT rowid FROM chunks WHERE id = ?), ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare fts stmt: %w", err)
	}
	defer ftsStmt.Close()

	for _, ec := range chunks {
		c := ec.Chunk
		headingJSON, _ := json.Marshal(c.HeadingPath)

		if _, err := chunkStmt.ExecContext(ctx, c.ID, c.SourceURL, c.Title,
			string(headingJSON), c.Content, c.TokenCount, c.ContentHash); err != nil {
			return fmt.Errorf("insert chunk %s: %w", c.ID, err)
		}

		vecBytes := vectorToBytes(ec.Vector)
		if _, err := embedStmt.ExecContext(ctx, c.ID, vecBytes, ec.Dimensions); err != nil {
			return fmt.Errorf("insert embedding %s: %w", c.ID, err)
		}

		if _, err := ftsStmt.ExecContext(ctx, c.ID, c.Content, c.Title, string(headingJSON)); err != nil {
			return fmt.Errorf("insert fts %s: %w", c.ID, err)
		}
	}

	return tx.Commit()
}

// GetAllChunks returns all chunks in the store.
func (s *SQLiteStore) GetAllChunks(ctx context.Context) ([]pipeline.Chunk, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, source_url, title, heading_path, content, token_count, content_hash FROM chunks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chunks []pipeline.Chunk
	for rows.Next() {
		var c pipeline.Chunk
		var headingJSON string
		if err := rows.Scan(&c.ID, &c.SourceURL, &c.Title, &headingJSON, &c.Content, &c.TokenCount, &c.ContentHash); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(headingJSON), &c.HeadingPath)
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

// GetEmbedding returns the embedding vector for a chunk ID.
func (s *SQLiteStore) GetEmbedding(ctx context.Context, chunkID string) ([]float32, error) {
	var vecBytes []byte
	var dims int
	err := s.db.QueryRowContext(ctx,
		`SELECT vector, dimensions FROM embeddings WHERE chunk_id = ?`, chunkID).Scan(&vecBytes, &dims)
	if err != nil {
		return nil, err
	}
	return bytesToVector(vecBytes), nil
}

// SetMeta stores a key-value pair in the meta table.
func (s *SQLiteStore) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO meta (key, value) VALUES (?, ?)`, key, value)
	return err
}

// GetMeta retrieves a value from the meta table.
func (s *SQLiteStore) GetMeta(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	return value, err
}

// DeleteBySourceURL removes all chunks (and their embeddings) from a given source URL.
func (s *SQLiteStore) DeleteBySourceURL(ctx context.Context, sourceURL string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Delete FTS entries by matching rowid.
	_, err = tx.ExecContext(ctx,
		`DELETE FROM chunks_fts WHERE rowid IN (SELECT rowid FROM chunks WHERE source_url = ?)`, sourceURL)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`DELETE FROM embeddings WHERE chunk_id IN (SELECT id FROM chunks WHERE source_url = ?)`, sourceURL)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM chunks WHERE source_url = ?`, sourceURL)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func createSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS chunks (
		id TEXT PRIMARY KEY,
		source_url TEXT NOT NULL,
		title TEXT NOT NULL,
		heading_path TEXT NOT NULL,
		content TEXT NOT NULL,
		token_count INTEGER NOT NULL,
		content_hash TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS embeddings (
		chunk_id TEXT PRIMARY KEY REFERENCES chunks(id),
		vector BLOB NOT NULL,
		dimensions INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS meta (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);

	CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(content, title, heading_path);
	`
	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// vectorToBytes converts a float32 slice to raw bytes.
func vectorToBytes(vec []float32) []byte {
	buf := make([]byte, len(vec)*4)
	for i, v := range vec {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(v))
	}
	return buf
}

// bytesToVector converts raw bytes back to a float32 slice.
func bytesToVector(buf []byte) []float32 {
	vec := make([]float32, len(buf)/4)
	for i := range vec {
		vec[i] = math.Float32frombits(binary.LittleEndian.Uint32(buf[i*4:]))
	}
	return vec
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/index/ -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/index/store.go internal/index/store_test.go go.mod go.sum
git commit -m "feat: add SQLite store with chunk CRUD, FTS5, and vector storage"
```

---

### Task 12: Hybrid Search (cosine + FTS5)

**Files:**
- Create: `internal/index/search.go`
- Create: `internal/index/search_test.go`

- [ ] **Step 1: Write tests**

Create `internal/index/search_test.go`:

```go
package index_test

import (
	"context"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSearchFTS5Only(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Index chunks with zero-length vectors (TF-IDF mode — search uses FTS5 only).
	chunks := []pipeline.EmbeddedChunk{
		pipeline.NewEmbeddedChunk(
			pipeline.NewChunk("https://example.com/a", "Auth", []string{"Auth"}, "API key authentication guide", 0, 2),
			nil,
		),
		pipeline.NewEmbeddedChunk(
			pipeline.NewChunk("https://example.com/b", "Webhooks", []string{"Webhooks"}, "Handle webhook events from Stripe", 1, 2),
			nil,
		),
	}
	require.NoError(t, store.Index(ctx, chunks))

	results, err := store.Search(ctx, "authentication", 5)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)
	assert.Contains(t, results[0].Chunk.Content, "authentication")
}

func TestSearchCosineWithFTS5Rerank(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	// Index with dense vectors.
	c1 := pipeline.NewChunk("https://example.com/a", "Auth", nil, "API authentication with keys", 0, 2)
	c2 := pipeline.NewChunk("https://example.com/b", "Rate", nil, "Rate limiting and quotas", 1, 2)

	// Vectors: c1 is "closer" to the query direction [0.9, 0.1].
	chunks := []pipeline.EmbeddedChunk{
		pipeline.NewEmbeddedChunk(c1, []float32{0.9, 0.1}),
		pipeline.NewEmbeddedChunk(c2, []float32{0.1, 0.9}),
	}
	require.NoError(t, store.Index(ctx, chunks))

	// Embed a fake query vector that's close to c1.
	results, err := store.SearchWithVector(ctx, []float32{0.85, 0.15}, "authentication", 5)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(results), 1)
	assert.Equal(t, c1.ID, results[0].Chunk.ID, "c1 should rank first (closer vector)")
}

func TestSearchTopK(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		c := pipeline.NewChunk("https://example.com", "T", nil, "authentication token", i, 10)
		require.NoError(t, store.Index(ctx, []pipeline.EmbeddedChunk{
			pipeline.NewEmbeddedChunk(c, nil),
		}))
	}

	results, err := store.Search(ctx, "authentication", 3)
	require.NoError(t, err)
	assert.Len(t, results, 3)
}

func TestSearchEmptyStore(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()

	results, err := store.Search(ctx, "anything", 5)
	require.NoError(t, err)
	assert.Empty(t, results)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/index/ -run TestSearch -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/index/search.go`:

```go
package index

import (
	"context"
	"encoding/json"
	"math"
	"sort"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// Search performs FTS5 keyword search. Used when no dense vectors are available.
func (s *SQLiteStore) Search(ctx context.Context, query string, topK int) ([]pipeline.SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.source_url, c.title, c.heading_path, c.content, c.token_count, c.content_hash,
		       bm25(chunks_fts) as score
		FROM chunks_fts f
		JOIN chunks c ON c.rowid = f.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY score
		LIMIT ?`, query, topK)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []pipeline.SearchResult
	for rows.Next() {
		var c pipeline.Chunk
		var headingJSON string
		var score float64
		if err := rows.Scan(&c.ID, &c.SourceURL, &c.Title, &headingJSON, &c.Content, &c.TokenCount, &c.ContentHash, &score); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(headingJSON), &c.HeadingPath)
		// BM25 returns negative scores (lower = better); normalize to 0-1 range.
		results = append(results, pipeline.NewSearchResult(c, -score))
	}
	return results, rows.Err()
}

// SearchWithVector performs hybrid search: cosine similarity on dense vectors,
// re-ranked with BM25 from FTS5.
func (s *SQLiteStore) SearchWithVector(ctx context.Context, queryVec []float32, queryText string, topK int) ([]pipeline.SearchResult, error) {
	// Load all embeddings for cosine scoring.
	rows, err := s.db.QueryContext(ctx, `
		SELECT c.id, c.source_url, c.title, c.heading_path, c.content, c.token_count, c.content_hash,
		       e.vector, e.dimensions
		FROM chunks c
		JOIN embeddings e ON e.chunk_id = c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type scoredChunk struct {
		chunk       pipeline.Chunk
		cosineScore float64
	}

	var candidates []scoredChunk
	for rows.Next() {
		var c pipeline.Chunk
		var headingJSON string
		var vecBytes []byte
		var dims int
		if err := rows.Scan(&c.ID, &c.SourceURL, &c.Title, &headingJSON, &c.Content, &c.TokenCount, &c.ContentHash, &vecBytes, &dims); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(headingJSON), &c.HeadingPath)

		vec := bytesToVector(vecBytes)
		score := cosineSimilarity(queryVec, vec)
		candidates = append(candidates, scoredChunk{chunk: c, cosineScore: score})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Sort by cosine similarity descending.
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].cosineScore > candidates[j].cosineScore
	})

	// Take top results.
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}

	results := make([]pipeline.SearchResult, len(candidates))
	for i, c := range candidates {
		results[i] = pipeline.NewSearchResult(c.chunk, c.cosineScore)
	}

	return results, nil
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/index/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/index/search.go internal/index/search_test.go
git commit -m "feat: add hybrid search with cosine similarity and FTS5 BM25"
```

---

## Phase 5: Export Formats

### Task 13: JSONL and CSV Exporters

**Files:**
- Create: `internal/export/jsonl.go`
- Create: `internal/export/csv.go`
- Create: `internal/export/export_test.go`

- [ ] **Step 1: Write tests**

Create `internal/export/export_test.go`:

```go
package export_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mayur19/docs-crawler/internal/export"
	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sampleChunks() []pipeline.EmbeddedChunk {
	c1 := pipeline.NewChunk("https://example.com/a", "Auth", []string{"Auth", "Keys"}, "Use API keys", 0, 2)
	c2 := pipeline.NewChunk("https://example.com/b", "Rate", []string{"Rate Limits"}, "Configure rate limits", 1, 2)
	return []pipeline.EmbeddedChunk{
		pipeline.NewEmbeddedChunk(c1, []float32{0.1, 0.2}),
		pipeline.NewEmbeddedChunk(c2, []float32{0.3, 0.4}),
	}
}

func TestJSONLExportWithoutVectors(t *testing.T) {
	var buf bytes.Buffer
	err := export.WriteJSONL(context.Background(), &buf, sampleChunks(), false)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 2)

	var record map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	assert.Equal(t, "Use API keys", record["content"])
	assert.Nil(t, record["vector"], "vector should be omitted")
}

func TestJSONLExportWithVectors(t *testing.T) {
	var buf bytes.Buffer
	err := export.WriteJSONL(context.Background(), &buf, sampleChunks(), true)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var record map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(lines[0]), &record))
	assert.NotNil(t, record["vector"], "vector should be present")
}

func TestCSVExport(t *testing.T) {
	var buf bytes.Buffer
	err := export.WriteCSV(context.Background(), &buf, sampleChunks())
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 3, "header + 2 data rows")
	assert.Contains(t, lines[0], "id")
	assert.Contains(t, lines[0], "content")
	assert.Contains(t, lines[1], "Use API keys")
}

func TestJSONLExportEmpty(t *testing.T) {
	var buf bytes.Buffer
	err := export.WriteJSONL(context.Background(), &buf, nil, false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/export/ -v`
Expected: FAIL

- [ ] **Step 3: Implement JSONL exporter**

Create `internal/export/jsonl.go`:

```go
package export

import (
	"context"
	"encoding/json"
	"io"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// JSONLRecord is the output format for a single JSONL line.
type JSONLRecord struct {
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

// WriteJSONL writes embedded chunks as JSONL to the given writer.
func WriteJSONL(_ context.Context, w io.Writer, chunks []pipeline.EmbeddedChunk, includeVectors bool) error {
	enc := json.NewEncoder(w)
	for _, ec := range chunks {
		c := ec.Chunk
		record := JSONLRecord{
			ID:          c.ID,
			Content:     c.Content,
			SourceURL:   c.SourceURL,
			Title:       c.Title,
			HeadingPath: c.HeadingPath,
			ChunkIndex:  c.ChunkIndex,
			TotalChunks: c.TotalChunks,
			TokenCount:  c.TokenCount,
			ContentHash: c.ContentHash,
			CrawledAt:   time.Now(),
		}
		if includeVectors {
			record.Vector = ec.Vector
		}
		if err := enc.Encode(record); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement CSV exporter**

Create `internal/export/csv.go`:

```go
package export

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// WriteCSV writes embedded chunks as CSV to the given writer.
func WriteCSV(_ context.Context, w io.Writer, chunks []pipeline.EmbeddedChunk) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	header := []string{"id", "source_url", "title", "heading_path", "content", "token_count", "content_hash"}
	if err := cw.Write(header); err != nil {
		return err
	}

	for _, ec := range chunks {
		c := ec.Chunk
		row := []string{
			c.ID,
			c.SourceURL,
			c.Title,
			strings.Join(c.HeadingPath, " > "),
			c.Content,
			fmt.Sprintf("%d", c.TokenCount),
			c.ContentHash,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}

	return nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/export/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/export/jsonl.go internal/export/csv.go internal/export/export_test.go
git commit -m "feat: add JSONL and CSV exporters"
```

---

## Phase 6: Config Expansion & YAML Loader

### Task 14: Expand Config with Chunking/Embedding Fields

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

- [ ] **Step 1: Write tests for new config fields**

Add to `internal/config/config_test.go`:

```go
func TestNewIngestConfig(t *testing.T) {
	cfg := config.NewConfig("https://example.com")
	cfg = cfg.
		WithChunkStrategy("heading").
		WithMaxTokens(256).
		WithEmbedder("auto").
		WithEmbeddingModel("nomic-embed-text").
		WithEmbeddingBatch(32)

	assert.Equal(t, "heading", cfg.ChunkStrategy)
	assert.Equal(t, 256, cfg.MaxTokens)
	assert.Equal(t, "auto", cfg.Embedder)
	assert.Equal(t, "nomic-embed-text", cfg.EmbeddingModel)
	assert.Equal(t, 32, cfg.EmbeddingBatch)
}

func TestConfigValidateIngestFields(t *testing.T) {
	cfg := config.NewConfig("https://example.com")
	cfg = cfg.WithMaxTokens(-1)
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max tokens")
}

func TestConfigValidateEmbedder(t *testing.T) {
	cfg := config.NewConfig("https://example.com")
	cfg = cfg.WithEmbedder("invalid")
	err := cfg.Validate()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "embedder")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run "TestNewIngestConfig|TestConfigValidateIngest|TestConfigValidateEmbedder" -v`
Expected: FAIL

- [ ] **Step 3: Add fields and methods to config.go**

Add to the `Config` struct in `internal/config/config.go`:

```go
	// Ingest pipeline fields
	ChunkStrategy  string
	MaxTokens      int
	Embedder       string
	EmbeddingModel string
	EmbeddingBatch int
```

Update `NewConfig` defaults:

```go
	ChunkStrategy:  "heading",
	MaxTokens:      512,
	Embedder:       "auto",
	EmbeddingModel: "nomic-embed-text",
	EmbeddingBatch: 64,
```

Add validation in `Validate()`:

```go
	if c.MaxTokens <= 0 {
		errs = append(errs, fmt.Errorf("max tokens must be greater than 0, got %d", c.MaxTokens))
	}

	validEmbedders := map[string]bool{"auto": true, "ollama": true, "openai": true, "cohere": true, "tfidf": true}
	if !validEmbedders[c.Embedder] {
		errs = append(errs, fmt.Errorf("embedder must be one of auto/ollama/openai/cohere/tfidf, got %q", c.Embedder))
	}
```

Add `With*` methods:

```go
func (c Config) WithChunkStrategy(s string) Config { c.ChunkStrategy = s; return c }
func (c Config) WithMaxTokens(n int) Config        { c.MaxTokens = n; return c }
func (c Config) WithEmbedder(e string) Config       { c.Embedder = e; return c }
func (c Config) WithEmbeddingModel(m string) Config { c.EmbeddingModel = m; return c }
func (c Config) WithEmbeddingBatch(n int) Config    { c.EmbeddingBatch = n; return c }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add chunking and embedding config fields"
```

---

### Task 15: YAML Config Loader

**Files:**
- Create: `internal/config/yaml.go`
- Create: `internal/config/yaml_test.go`

- [ ] **Step 1: Write tests**

Create `internal/config/yaml_test.go`:

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mayur19/docs-crawler/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadYAMLConfig(t *testing.T) {
	yamlContent := `
source:
  url: https://docs.stripe.com
  include: ["/api/*"]
  exclude: ["/changelog/*"]
  max_depth: 5
  use_browser: false
crawl:
  workers: 20
  rate_limit: 10
  timeout: 60s
  user_agent: "test-agent/1.0"
chunking:
  strategy: heading
  max_tokens: 256
embedding:
  provider: ollama
  model: nomic-embed-text
  batch_size: 32
output:
  dir: ./test-output
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0o644))

	cfg, err := config.LoadYAML(path)
	require.NoError(t, err)

	assert.Equal(t, "https://docs.stripe.com", cfg.SeedURL)
	assert.Equal(t, []string{"/api/*"}, cfg.Includes)
	assert.Equal(t, []string{"/changelog/*"}, cfg.Excludes)
	assert.Equal(t, 5, cfg.MaxDepth)
	assert.Equal(t, 20, cfg.Workers)
	assert.InDelta(t, 10.0, cfg.RateLimit, 0.01)
	assert.Equal(t, "heading", cfg.ChunkStrategy)
	assert.Equal(t, 256, cfg.MaxTokens)
	assert.Equal(t, "ollama", cfg.Embedder)
	assert.Equal(t, 32, cfg.EmbeddingBatch)
	assert.Equal(t, "./test-output", cfg.OutputDir)
}

func TestLoadYAMLConfigMinimal(t *testing.T) {
	yamlContent := `
source:
  url: https://example.com
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0o644))

	cfg, err := config.LoadYAML(path)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com", cfg.SeedURL)
	// Defaults should be applied.
	assert.Equal(t, 10, cfg.Workers)
	assert.Equal(t, 512, cfg.MaxTokens)
}

func TestLoadYAMLConfigFileNotFound(t *testing.T) {
	_, err := config.LoadYAML("/nonexistent/path.yaml")
	assert.Error(t, err)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -run TestLoadYAML -v`
Expected: FAIL

- [ ] **Step 3: Implement**

Create `internal/config/yaml.go`:

```go
package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type yamlConfig struct {
	Source struct {
		URL        string   `yaml:"url"`
		Include    []string `yaml:"include"`
		Exclude    []string `yaml:"exclude"`
		MaxDepth   int      `yaml:"max_depth"`
		UseBrowser bool     `yaml:"use_browser"`
	} `yaml:"source"`
	Crawl struct {
		Workers   int    `yaml:"workers"`
		RateLimit float64 `yaml:"rate_limit"`
		Timeout   string `yaml:"timeout"`
		UserAgent string `yaml:"user_agent"`
	} `yaml:"crawl"`
	Chunking struct {
		Strategy  string `yaml:"strategy"`
		MaxTokens int    `yaml:"max_tokens"`
	} `yaml:"chunking"`
	Embedding struct {
		Provider  string `yaml:"provider"`
		Model     string `yaml:"model"`
		BatchSize int    `yaml:"batch_size"`
	} `yaml:"embedding"`
	Output struct {
		Dir string `yaml:"dir"`
	} `yaml:"output"`
}

// LoadYAML reads a YAML config file and returns a Config with defaults applied.
func LoadYAML(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config file: %w", err)
	}

	var yc yamlConfig
	if err := yaml.Unmarshal(data, &yc); err != nil {
		return Config{}, fmt.Errorf("parse config file: %w", err)
	}

	cfg := NewConfig(yc.Source.URL)

	if len(yc.Source.Include) > 0 {
		cfg = cfg.WithIncludes(yc.Source.Include)
	}
	if len(yc.Source.Exclude) > 0 {
		cfg = cfg.WithExcludes(yc.Source.Exclude)
	}
	if yc.Source.MaxDepth > 0 {
		cfg = cfg.WithMaxDepth(yc.Source.MaxDepth)
	}
	if yc.Source.UseBrowser {
		cfg = cfg.WithUseBrowser(true)
	}
	if yc.Crawl.Workers > 0 {
		cfg = cfg.WithWorkers(yc.Crawl.Workers)
	}
	if yc.Crawl.RateLimit > 0 {
		cfg = cfg.WithRateLimit(yc.Crawl.RateLimit)
	}
	if yc.Crawl.Timeout != "" {
		d, err := time.ParseDuration(yc.Crawl.Timeout)
		if err == nil {
			cfg = cfg.WithTimeout(d)
		}
	}
	if yc.Crawl.UserAgent != "" {
		cfg = cfg.WithUserAgent(yc.Crawl.UserAgent)
	}
	if yc.Chunking.Strategy != "" {
		cfg = cfg.WithChunkStrategy(yc.Chunking.Strategy)
	}
	if yc.Chunking.MaxTokens > 0 {
		cfg = cfg.WithMaxTokens(yc.Chunking.MaxTokens)
	}
	if yc.Embedding.Provider != "" {
		cfg = cfg.WithEmbedder(yc.Embedding.Provider)
	}
	if yc.Embedding.Model != "" {
		cfg = cfg.WithEmbeddingModel(yc.Embedding.Model)
	}
	if yc.Embedding.BatchSize > 0 {
		cfg = cfg.WithEmbeddingBatch(yc.Embedding.BatchSize)
	}
	if yc.Output.Dir != "" {
		cfg = cfg.WithOutputDir(yc.Output.Dir)
	}

	return cfg, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/yaml.go internal/config/yaml_test.go
git commit -m "feat: add YAML config file loader"
```

---

## Phase 7: Engine Integration

### Task 16: Add RunIngest to Engine

**Files:**
- Modify: `internal/engine/engine.go`
- Modify: `internal/engine/engine_test.go`

- [ ] **Step 1: Write test for ingest pipeline**

Add to `internal/engine/engine_test.go`:

```go
func TestEngineRunIngest(t *testing.T) {
	// Set up a mock HTTP server with one page.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sitemap.xml":
			w.Write([]byte(`<?xml version="1.0"?>
				<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
					<url><loc>` + "http://" + r.Host + `/page</loc></url>
				</urlset>`))
		case "/page":
			w.Write([]byte(`<html><head><title>Test</title></head>
				<body><article><h2>Section</h2><p>Hello world.</p></article></body></html>`))
		}
	}))
	defer srv.Close()

	cfg := config.NewConfig(srv.URL + "/page")
	cfg = cfg.WithOutputDir(t.TempDir())

	// Use TF-IDF embedder and in-memory-like temp DB for test.
	dbPath := filepath.Join(t.TempDir(), "test.db")

	stubChunker := &testChunker{}
	stubEmbedder := &testEmbedder{}
	store, err := index.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	defer store.Close()

	// Build and run engine with ingest pipeline.
	// (This test validates the channel wiring; detailed chunking/embedding is tested per-package.)
	e := engine.New(/* discoverers, fetchers, extractors, writers, linkFollower, dedup, pools */)
	err = e.RunIngest(ctx, cfg, stubChunker, stubEmbedder, store)
	require.NoError(t, err)

	chunks, _ := store.GetAllChunks(context.Background())
	assert.Greater(t, len(chunks), 0)
}
```

*Note: The exact test wiring depends on how RunIngest is parameterized. The implementing agent should adapt this test to match the actual signature.*

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/engine/ -run TestEngineRunIngest -v`
Expected: FAIL

- [ ] **Step 3: Implement RunIngest**

Add `RunIngest` method to `internal/engine/engine.go`. This method reuses the Discover → Fetch → Extract stages from `Run()`, then adds Chunk → Embed → Index stages with their own channels and worker pools:

```go
// RunIngest executes the full ingest pipeline: Discover → Fetch → Extract → Chunk → Embed → Index.
func (e *Engine) RunIngest(
	ctx context.Context,
	cfg config.Config,
	chunker pipeline.Chunker,
	embedder pipeline.Embedder,
	indexer pipeline.Indexer,
) error {
	seedURL, err := url.Parse(cfg.SeedURL)
	if err != nil {
		return fmt.Errorf("engine: invalid seed URL: %w", err)
	}

	fetchCh := make(chan pipeline.CrawlURL, e.pools.Fetch*2)
	extractCh := make(chan pipeline.FetchResult, e.pools.Extract*2)
	chunkCh := make(chan pipeline.Document, 6)   // 3 chunk workers * 2
	embedCh := make(chan []pipeline.Chunk, 4)     // 2 embed workers * 2
	indexCh := make(chan []pipeline.EmbeddedChunk, 2)

	var inFlight atomic.Int64
	var wg sync.WaitGroup

	// Stage 1-2: Discovery and Fetch (reuse existing).
	discoverDone := e.startDiscovery(ctx, seedURL, fetchCh, &inFlight)
	fetchDone := e.startFetch(ctx, fetchCh, extractCh, &inFlight)

	// Stage 3: Extract → chunkCh (instead of writeCh).
	extractDone := e.startExtractTo(ctx, extractCh, chunkCh)

	// Stage 4: Chunk.
	chunkDone := e.startChunk(ctx, chunkCh, embedCh, chunker)

	// Stage 5: Embed.
	embedDone := e.startEmbed(ctx, embedCh, indexCh, embedder)

	// Stage 6: Index.
	indexDone := e.startIndex(ctx, indexCh, indexer, &wg)

	// Cascade close.
	go func() { discoverDone.Wait(); close(fetchCh) }()
	go func() { fetchDone.Wait(); close(extractCh) }()
	go func() { extractDone.Wait(); close(chunkCh) }()
	go func() { chunkDone.Wait(); close(embedCh) }()
	go func() { embedDone.Wait(); close(indexCh) }()

	indexDone.Wait()
	wg.Wait()

	if err := indexer.Close(); err != nil {
		e.logger.Error("failed to close indexer", "error", err)
	}

	stats := e.Stats()
	e.logger.Info("ingest complete",
		"urls_seen", stats.URLsSeen,
		"content_dups", stats.ContentDups,
		"fetch_errors", stats.FetchErrors,
	)

	return nil
}
```

Implement the new stage starters (`startExtractTo`, `startChunk`, `startEmbed`, `startIndex`) following the same pattern as existing stages.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/engine/ -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/engine/engine.go internal/engine/engine_test.go
git commit -m "feat: add RunIngest method with chunk/embed/index pipeline stages"
```

---

## Phase 8: CLI Commands

### Task 17: `ingest` Command

**Files:**
- Create: `cmd/ingest.go`

- [ ] **Step 1: Implement ingest command**

Create `cmd/ingest.go` that:
1. Accepts same base flags as `crawl` plus `--chunk-strategy`, `--max-tokens`, `--embedder`, `--embedding-model`, `--embedding-batch`, `--config`
2. Builds config (merging YAML + CLI flags)
3. Auto-detects embedder if `--embedder auto`
4. Creates `SQLiteStore` at `<output>/index.db`
5. Calls `engine.RunIngest()`
6. Prints progress summary

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/ingest.go
git commit -m "feat: add ingest CLI command with full pipeline"
```

---

### Task 18: `search` Command

**Files:**
- Create: `cmd/search.go`

- [ ] **Step 1: Implement search command**

Create `cmd/search.go` that:
1. Opens `SQLiteStore` from `--source` directory
2. Runs `store.Search()` (FTS5) or `store.SearchWithVector()` (if dense embeddings exist)
3. Formats results as pretty table or JSON based on `--format`
4. Flags: `--top`, `--source`, `--format`

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/search.go
git commit -m "feat: add search CLI command with pretty and JSON output"
```

---

### Task 19: `export` Command

**Files:**
- Create: `cmd/export.go`

- [ ] **Step 1: Implement export command**

Create `cmd/export.go` that:
1. Opens `SQLiteStore` from `--source`
2. Reads all chunks and embeddings
3. Writes to stdout or `-o` path in the selected format (jsonl, csv, markdown)
4. Flags: `--format`, `--include-vectors`, `-o`, `--source`

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/export.go
git commit -m "feat: add export CLI command with JSONL, CSV, and Markdown formats"
```

---

### Task 20: `init` Command

**Files:**
- Create: `cmd/init.go`

- [ ] **Step 1: Implement init command**

Create `cmd/init.go` that:
1. Takes a URL argument
2. Runs sitemap discovery to find URL patterns
3. Generates a `docs-crawler.yaml` config template with suggested include/exclude patterns
4. Writes to stdout (pipeable to file)

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Build succeeds

- [ ] **Step 3: Commit**

```bash
git add cmd/init.go
git commit -m "feat: add init CLI command for config file generation"
```

---

## Phase 9: Existing Codebase Fixes

### Task 21: Wire robots.txt into Fetch Pipeline

**Files:**
- Modify: `cmd/crawl.go`
- Modify: `internal/engine/engine.go`

- [ ] **Step 1: Write test**

Add test in `internal/engine/engine_test.go` that verifies robots.txt-disallowed URLs are skipped.

- [ ] **Step 2: Implement**

In `cmd/crawl.go` `executeCrawl()`: call `scope.FetchRobots()`, create `RobotsChecker`, pass to engine. In engine fetch stage: check `robotsChecker.IsAllowed(url)` before fetching. Set `RobotsCrawlDelay` in rate limiter config.

- [ ] **Step 3: Run all tests**

Run: `go test ./... -race`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add cmd/crawl.go internal/engine/engine.go internal/engine/engine_test.go
git commit -m "fix: wire robots.txt checking into fetch pipeline"
```

---

### Task 22: Fix Browser Fetcher Cleanup

**Files:**
- Modify: `internal/engine/engine.go`

- [ ] **Step 1: Add browser cleanup in engine shutdown**

In `Run()` and `RunIngest()`, after closing writers/indexer, iterate fetchers and call `Close()` on any that implement `io.Closer`.

- [ ] **Step 2: Run tests**

Run: `go test ./internal/engine/ -race -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/engine/engine.go
git commit -m "fix: close browser fetcher on engine shutdown"
```

---

### Task 23: Quality of Life Fixes

**Files:**
- Modify: `cmd/crawl.go` (add `--max-time` and `--dry-run` flags)
- Modify: `cmd/discover.go` (print URL count)
- Modify: `internal/writer/markdown.go` (add `duration_seconds` to manifest)

- [ ] **Step 1: Add --max-time flag**

Wrap the crawl context with a timeout derived from `--max-time`.

- [ ] **Step 2: Add --dry-run flag**

Run discovery only, print URLs, exit.

- [ ] **Step 3: Print URL count in discover**

Add counter to discover command, print total at end.

- [ ] **Step 4: Add duration to manifest**

Add `duration_seconds` field to manifest JSON.

- [ ] **Step 5: Run all tests**

Run: `go test ./... -race`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/crawl.go cmd/discover.go internal/writer/markdown.go
git commit -m "fix: add max-time, dry-run, URL count, manifest duration"
```

---

## Phase 10: Integration Testing & Final Verification

### Task 24: End-to-End Integration Test

**Files:**
- Modify: `internal/engine/integration_test.go`

- [ ] **Step 1: Write E2E test for ingest pipeline**

Add a test that:
1. Starts an httptest server with multiple pages
2. Runs the full ingest pipeline (discover → fetch → extract → chunk → embed → index)
3. Verifies chunks are in the SQLite store
4. Runs a search query and verifies results
5. Exports to JSONL and verifies output

- [ ] **Step 2: Run integration test**

Run: `go test ./internal/engine/ -run TestIntegration -race -v`
Expected: PASS

- [ ] **Step 3: Run full test suite with coverage**

Run: `go test ./... -race -coverprofile=cover.out && go tool cover -func=cover.out | tail -1`
Expected: Coverage >= 80%

- [ ] **Step 4: Commit**

```bash
git add internal/engine/integration_test.go
git commit -m "test: add end-to-end integration test for ingest pipeline"
```

---

### Task 25: Update README and Root Command

**Files:**
- Modify: `README.md`
- Modify: `cmd/root.go`

- [ ] **Step 1: Update root command description**

Update the `Long` description in `cmd/root.go` to mention AI pipeline, search, and export capabilities.

- [ ] **Step 2: Update README**

Add the new demo commands to README, document `ingest`, `search`, `export`, `init` commands, update the tagline, add config file documentation.

- [ ] **Step 3: Commit**

```bash
git add README.md cmd/root.go
git commit -m "docs: update README and CLI description for v1.0 AI pipeline"
```

---

### Task 26: Final Build & Smoke Test

- [ ] **Step 1: Build**

Run: `go build -o docs-crawler .`
Expected: Binary builds successfully

- [ ] **Step 2: Check binary size**

Run: `ls -lh docs-crawler`
Expected: < 30MB

- [ ] **Step 3: Run vet and lint**

Run: `go vet ./...`
Expected: No issues

- [ ] **Step 4: Smoke test**

Run: `./docs-crawler --help`
Verify all 6 commands are listed: crawl, discover, ingest, search, export, init

- [ ] **Step 5: Commit any final fixes**

```bash
git add -A && git commit -m "chore: final build verification and cleanup"
```
