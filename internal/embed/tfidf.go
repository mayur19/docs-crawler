// Package embed provides Embedder implementations for converting text chunks
// into vector representations suitable for similarity search.
package embed

import (
	"context"
	"math"
	"strings"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// TFIDFEmbedder computes TF-IDF vectors with L2 normalization.
// It is a zero-dependency fallback that requires no external API.
type TFIDFEmbedder struct{}

// NewTFIDFEmbedder returns a new TFIDFEmbedder.
func NewTFIDFEmbedder() *TFIDFEmbedder {
	return &TFIDFEmbedder{}
}

// Name returns the embedder identifier.
func (e *TFIDFEmbedder) Name() string { return "tfidf" }

// Dimensions returns 0 because the vocabulary (and therefore vector size)
// is determined dynamically from each batch.
func (e *TFIDFEmbedder) Dimensions() int { return 0 }

// Embed computes TF-IDF vectors for each chunk in the batch and returns
// L2-normalized EmbeddedChunks.
func (e *TFIDFEmbedder) Embed(_ context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return []pipeline.EmbeddedChunk{}, nil
	}

	// Tokenize each document.
	docs := make([][]string, len(chunks))
	for i, c := range chunks {
		docs[i] = tokenize(c.Content)
	}

	// Build vocabulary (sorted for determinism via map iteration order).
	vocabSet := make(map[string]int)
	for _, tokens := range docs {
		for _, t := range tokens {
			if _, ok := vocabSet[t]; !ok {
				vocabSet[t] = len(vocabSet)
			}
		}
	}
	vocabSize := len(vocabSet)

	// Compute document frequency for each term.
	df := make(map[string]int, vocabSize)
	for _, tokens := range docs {
		seen := make(map[string]bool, len(tokens))
		for _, t := range tokens {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}

	n := float64(len(docs))
	results := make([]pipeline.EmbeddedChunk, len(chunks))

	for i, tokens := range docs {
		// Compute term frequency for this document.
		tf := make(map[string]float64, len(tokens))
		for _, t := range tokens {
			tf[t]++
		}
		docLen := float64(len(tokens))
		for t := range tf {
			if docLen > 0 {
				tf[t] /= docLen
			}
		}

		// Build TF-IDF vector.
		vec := make([]float64, vocabSize)
		for term, idx := range vocabSet {
			if tfVal, ok := tf[term]; ok {
				idf := math.Log(n/float64(df[term])) + 1.0
				vec[idx] = tfVal * idf
			}
		}

		// Convert to float32 and L2-normalize.
		results[i] = pipeline.NewEmbeddedChunk(chunks[i], l2Normalize(vec))
	}

	return results, nil
}

// tokenize lowercases and splits text into words, stripping non-alpha chars.
func tokenize(text string) []string {
	lower := strings.ToLower(text)
	fields := strings.FieldsFunc(lower, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	result := fields[:0:len(fields)]
	for _, f := range fields {
		if f != "" {
			result = append(result, f)
		}
	}
	return result
}

// l2Normalize converts a float64 slice to a L2-normalized float32 slice.
func l2Normalize(vec []float64) []float32 {
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)

	out := make([]float32, len(vec))
	if norm == 0 {
		return out
	}
	for i, v := range vec {
		out[i] = float32(v / norm)
	}
	return out
}
