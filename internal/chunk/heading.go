package chunk

import (
	"context"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// HeadingChunker splits a Document at Markdown heading boundaries (## and ###).
// Code blocks (```...```) are treated as atomic units and never split.
// Sections that exceed maxTokens are further split at paragraph boundaries.
type HeadingChunker struct {
	maxTokens int
}

// NewHeadingChunker creates a HeadingChunker with the given token limit per chunk.
func NewHeadingChunker(maxTokens int) *HeadingChunker {
	return &HeadingChunker{maxTokens: maxTokens}
}

// Name returns the chunker's identifier.
func (h *HeadingChunker) Name() string {
	return "heading"
}

// section holds a heading path and its accumulated body lines.
type section struct {
	headingPath []string
	lines       []string
}

// Chunk splits the document into heading-based chunks.
func (h *HeadingChunker) Chunk(ctx context.Context, doc pipeline.Document) ([]pipeline.Chunk, error) {
	if strings.TrimSpace(doc.Markdown) == "" {
		return nil, nil
	}

	sections := h.splitIntoSections(doc.Markdown)
	raw := h.buildRawChunks(doc, sections)

	// Fix ChunkIndex and TotalChunks now that we know the total.
	total := len(raw)
	result := make([]pipeline.Chunk, len(raw))
	for i, c := range raw {
		result[i] = pipeline.NewChunk(
			c.SourceURL,
			c.Title,
			c.HeadingPath,
			c.Content,
			i,
			total,
		)
	}
	return result, nil
}

// splitIntoSections parses the Markdown into sections delimited by ## or ### headings.
// Code blocks are tracked so that heading markers inside them are ignored.
func (h *HeadingChunker) splitIntoSections(markdown string) []section {
	lines := strings.Split(markdown, "\n")

	var sections []section
	var current *section
	inCodeBlock := false
	var h2Heading string // track last level-2 heading text

	flush := func() {
		if current != nil {
			sections = append(sections, *current)
		}
	}

	for _, line := range lines {
		// Detect code-fence toggle (``` at start of line).
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			if current != nil {
				current.lines = append(current.lines, line)
			}
			continue
		}

		if !inCodeBlock {
			if strings.HasPrefix(line, "### ") {
				heading := strings.TrimPrefix(line, "### ")
				flush()
				path := []string{h2Heading, heading}
				if h2Heading == "" {
					path = []string{heading}
				}
				current = &section{headingPath: path, lines: []string{line}}
				continue
			}
			if strings.HasPrefix(line, "## ") {
				heading := strings.TrimPrefix(line, "## ")
				flush()
				h2Heading = heading
				current = &section{headingPath: []string{heading}, lines: []string{line}}
				continue
			}
		}

		if current == nil {
			// Content before any heading — create a no-heading section.
			current = &section{headingPath: []string{}, lines: []string{line}}
		} else {
			current.lines = append(current.lines, line)
		}
	}
	flush()

	return sections
}

// buildRawChunks converts sections into pipeline.Chunk values, splitting oversized
// sections at paragraph boundaries.
func (h *HeadingChunker) buildRawChunks(doc pipeline.Document, sections []section) []pipeline.Chunk {
	var chunks []pipeline.Chunk

	for _, sec := range sections {
		content := strings.TrimSpace(strings.Join(sec.lines, "\n"))
		if content == "" {
			continue
		}

		if EstimateTokens(content) <= h.maxTokens {
			chunks = append(chunks, pipeline.NewChunk(
				doc.URL,
				doc.Title,
				sec.headingPath,
				content,
				len(chunks),  // placeholder index, fixed later
				0,            // placeholder total, fixed later
			))
			continue
		}

		// Split at paragraph boundaries.
		subChunks := h.splitByParagraphs(doc, sec, content)
		chunks = append(chunks, subChunks...)
	}

	return chunks
}

// splitByParagraphs splits a large section's content at blank-line paragraph
// boundaries so that each resulting chunk fits within maxTokens.
func (h *HeadingChunker) splitByParagraphs(doc pipeline.Document, sec section, content string) []pipeline.Chunk {
	paragraphs := splitParagraphs(content)

	var chunks []pipeline.Chunk
	var buf []string
	bufTokens := 0

	flush := func() {
		if len(buf) == 0 {
			return
		}
		text := strings.Join(buf, "\n\n")
		chunks = append(chunks, pipeline.NewChunk(
			doc.URL,
			doc.Title,
			sec.headingPath,
			text,
			len(chunks), // placeholder
			0,           // placeholder
		))
		buf = nil
		bufTokens = 0
	}

	for _, para := range paragraphs {
		paraTokens := EstimateTokens(para)
		if bufTokens+paraTokens > h.maxTokens && len(buf) > 0 {
			flush()
		}
		buf = append(buf, para)
		bufTokens += paraTokens
	}
	flush()

	return chunks
}

// splitParagraphs splits text on blank lines, preserving code blocks as single units.
func splitParagraphs(text string) []string {
	lines := strings.Split(text, "\n")
	var paragraphs []string
	var current []string
	inCodeBlock := false

	flush := func() {
		if len(current) > 0 {
			para := strings.TrimSpace(strings.Join(current, "\n"))
			if para != "" {
				paragraphs = append(paragraphs, para)
			}
			current = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			current = append(current, line)
			if !inCodeBlock {
				// End of code block — keep it together but allow flush after.
				// Don't flush here; let blank lines handle it.
			}
			continue
		}

		if !inCodeBlock && trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()

	return paragraphs
}
