package embed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/mayur19/docs-crawler/internal/pipeline"
)

// defaultBatchSize is the number of texts sent per API request.
// Shared by all HTTP-based embedders.
const defaultBatchSize = 64

// OllamaEmbedder calls the Ollama /api/embed endpoint.
type OllamaEmbedder struct {
	baseURL string
	model   string
	client  *http.Client
}

// NewOllamaEmbedder creates an OllamaEmbedder targeting the given base URL and model.
func NewOllamaEmbedder(baseURL, model string) *OllamaEmbedder {
	return &OllamaEmbedder{
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns "ollama".
func (e *OllamaEmbedder) Name() string { return "ollama" }

// Dimensions returns 0 (model-dependent, determined at runtime).
func (e *OllamaEmbedder) Dimensions() int { return 0 }

// ollamaEmbedRequest is the JSON body sent to /api/embed.
type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ollamaEmbedResponse is the JSON response from /api/embed.
type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed converts chunks to embeddings via the Ollama API, batching in groups
// of defaultBatchSize.
func (e *OllamaEmbedder) Embed(ctx context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return []pipeline.EmbeddedChunk{}, nil
	}

	results := make([]pipeline.EmbeddedChunk, 0, len(chunks))

	for start := 0; start < len(chunks); start += defaultBatchSize {
		end := start + defaultBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]

		texts := extractTexts(batch)
		vectors, err := e.embedBatch(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("ollama embed batch [%d:%d]: %w", start, end, err)
		}

		for i, chunk := range batch {
			results = append(results, pipeline.NewEmbeddedChunk(chunk, vectors[i]))
		}
	}

	return results, nil
}

func (e *OllamaEmbedder) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := ollamaEmbedRequest{
		Model: e.model,
		Input: texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama API returned status %d", resp.StatusCode)
	}

	var result ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Embeddings, nil
}

// OllamaIsAvailable checks if an Ollama server is reachable at the given base URL.
// It issues a GET request with a 2-second timeout.
func OllamaIsAvailable(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// extractTexts returns the Content field of each chunk.
func extractTexts(chunks []pipeline.Chunk) []string {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	return texts
}
