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

// CohereEmbedder calls the Cohere /v2/embed endpoint.
type CohereEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewCohereEmbedder creates a CohereEmbedder for the given base URL, API key, and model.
func NewCohereEmbedder(baseURL, apiKey, model string) *CohereEmbedder {
	return &CohereEmbedder{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns "cohere".
func (e *CohereEmbedder) Name() string { return "cohere" }

// Dimensions returns 0 (model-dependent, determined at runtime).
func (e *CohereEmbedder) Dimensions() int { return 0 }

// cohereEmbedRequest is the JSON body for /v2/embed.
type cohereEmbedRequest struct {
	Model     string   `json:"model"`
	Texts     []string `json:"texts"`
	InputType string   `json:"input_type"`
}

// cohereEmbedResponse is the JSON response from /v2/embed.
type cohereEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed converts chunks to embeddings via the Cohere API, batching in groups
// of defaultBatchSize.
func (e *CohereEmbedder) Embed(ctx context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
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
			return nil, fmt.Errorf("cohere embed batch [%d:%d]: %w", start, end, err)
		}

		for i, chunk := range batch {
			results = append(results, pipeline.NewEmbeddedChunk(chunk, vectors[i]))
		}
	}

	return results, nil
}

func (e *CohereEmbedder) embedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	reqBody := cohereEmbedRequest{
		Model:     e.model,
		Texts:     texts,
		InputType: "search_document",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v2/embed", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.apiKey)

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("cohere API returned status %d", resp.StatusCode)
	}

	var result cohereEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Embeddings, nil
}
