package index

import (
	"context"
	"fmt"
	"math"
	"sort"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// Search performs FTS5 keyword search using BM25 scoring and returns up to topK results.
func (s *SQLiteStore) Search(ctx context.Context, query string, topK int) ([]pipeline.SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			c.id, c.source_url, c.title, c.heading_path,
			c.content, c.token_count, c.content_hash,
			-bm25(chunks_fts) AS score
		FROM chunks_fts
		JOIN chunks c ON chunks_fts.rowid = c.rowid
		WHERE chunks_fts MATCH ?
		ORDER BY score DESC
		LIMIT ?
	`, query, topK)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var results []pipeline.SearchResult
	for rows.Next() {
		var score float64
		var (
			id, sourceURL, title, headingJSON, content, contentHash string
			tokenCount                                               int
		)
		if err := rows.Scan(&id, &sourceURL, &title, &headingJSON, &content, &tokenCount, &contentHash, &score); err != nil {
			return nil, fmt.Errorf("scan fts result: %w", err)
		}

		chunk, err := unmarshalChunk(id, sourceURL, title, headingJSON, content, tokenCount, contentHash)
		if err != nil {
			return nil, err
		}
		results = append(results, pipeline.NewSearchResult(chunk, score))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("fts rows error: %w", err)
	}
	return results, nil
}

// SearchWithVector loads all embeddings, computes cosine similarity against queryVec,
// sorts by descending score, and returns up to topK results.
func (s *SQLiteStore) SearchWithVector(ctx context.Context, queryVec []float32, queryText string, topK int) ([]pipeline.SearchResult, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT
			c.id, c.source_url, c.title, c.heading_path,
			c.content, c.token_count, c.content_hash,
			e.vector
		FROM embeddings e
		JOIN chunks c ON e.chunk_id = c.id
	`)
	if err != nil {
		return nil, fmt.Errorf("load embeddings: %w", err)
	}
	defer rows.Close()

	type candidate struct {
		chunk pipeline.Chunk
		score float64
	}

	var candidates []candidate
	for rows.Next() {
		var blob []byte
		var (
			id, sourceURL, title, headingJSON, content, contentHash string
			tokenCount                                               int
		)
		if err := rows.Scan(&id, &sourceURL, &title, &headingJSON, &content, &tokenCount, &contentHash, &blob); err != nil {
			return nil, fmt.Errorf("scan embedding row: %w", err)
		}

		chunk, err := unmarshalChunk(id, sourceURL, title, headingJSON, content, tokenCount, contentHash)
		if err != nil {
			return nil, err
		}

		vec := bytesToVector(blob)
		score := cosineSimilarity(queryVec, vec)
		candidates = append(candidates, candidate{chunk: chunk, score: score})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("embedding rows error: %w", err)
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	limit := topK
	if limit > len(candidates) {
		limit = len(candidates)
	}

	results := make([]pipeline.SearchResult, limit)
	for i, c := range candidates[:limit] {
		results[i] = pipeline.NewSearchResult(c.chunk, c.score)
	}
	return results, nil
}

// cosineSimilarity returns the cosine similarity between two float32 vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		fa := float64(a[i])
		fb := float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// unmarshalChunk reconstructs a pipeline.Chunk from raw DB fields.
func unmarshalChunk(id, sourceURL, title, headingJSON, content string, tokenCount int, contentHash string) (pipeline.Chunk, error) {
	row := &rowScanner{values: []any{id, sourceURL, title, headingJSON, content, tokenCount, contentHash}}
	return scanChunk(row)
}

// rowScanner adapts a slice of values to the scanner interface.
type rowScanner struct {
	values []any
	pos    int
}

func (r *rowScanner) Scan(dest ...any) error {
	for i, d := range dest {
		if r.pos+i >= len(r.values) {
			return fmt.Errorf("rowScanner: not enough values at position %d", r.pos+i)
		}
		switch p := d.(type) {
		case *string:
			if v, ok := r.values[r.pos+i].(string); ok {
				*p = v
			} else {
				return fmt.Errorf("rowScanner: expected string at index %d", r.pos+i)
			}
		case *int:
			switch v := r.values[r.pos+i].(type) {
			case int:
				*p = v
			case int64:
				*p = int(v)
			default:
				return fmt.Errorf("rowScanner: expected int at index %d", r.pos+i)
			}
		default:
			return fmt.Errorf("rowScanner: unsupported type at index %d", r.pos+i)
		}
	}
	r.pos += len(dest)
	return nil
}
