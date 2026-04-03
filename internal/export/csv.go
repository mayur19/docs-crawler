package export

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// csvHeader defines the column order for the CSV output.
var csvHeader = []string{
	"id",
	"source_url",
	"title",
	"heading_path",
	"content",
	"token_count",
	"content_hash",
}

// WriteCSV encodes each EmbeddedChunk as a CSV row preceded by a header row.
// heading_path slices are joined with " > ".
// An error from the underlying writer terminates encoding immediately.
func WriteCSV(ctx context.Context, w io.Writer, chunks []pipeline.EmbeddedChunk) error {
	cw := csv.NewWriter(w)

	if err := cw.Write(csvHeader); err != nil {
		return fmt.Errorf("writing CSV header: %w", err)
	}

	for i, ec := range chunks {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled at chunk %d: %w", i, err)
		}

		row := buildRow(ec)

		if err := cw.Write(row); err != nil {
			return fmt.Errorf("writing CSV row %d (%s): %w", i, ec.Chunk.ID, err)
		}
	}

	cw.Flush()
	return cw.Error()
}

// buildRow converts an EmbeddedChunk into a slice of string columns matching
// csvHeader order. It creates new values and never mutates the input.
func buildRow(ec pipeline.EmbeddedChunk) []string {
	return []string{
		ec.Chunk.ID,
		ec.Chunk.SourceURL,
		ec.Chunk.Title,
		strings.Join(ec.Chunk.HeadingPath, " > "),
		ec.Chunk.Content,
		strconv.Itoa(ec.Chunk.TokenCount),
		ec.Chunk.ContentHash,
	}
}
