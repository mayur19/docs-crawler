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

// OpenAIEmbedder calls the OpenAI /v1/embeddings endpoint.
type OpenAIEmbedder struct {
	baseURL string
	apiKey  string
	model   string
	client  *http.Client
}

// NewOpenAIEmbedder creates an OpenAIEmbedder for the given base URL, API key, and model.
func NewOpenAIEmbedder(baseURL, apiKey, model string) *OpenAIEmbedder {
	return &OpenAIEmbedder{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// Name returns "openai".
func (e *OpenAIEmbedder) Name() string { return "openai" }

// Dimensions returns 0 (model-dependent, determined at runtime).
func (e *OpenAIEmbedder) Dimensions() int { return 0 }

// openAIEmbedRequest is the JSON body for /v1/embeddings.
type openAIEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// openAIEmbedding is a single embedding item in the response.
type openAIEmbedding struct {
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

// openAIEmbedResponse is the JSON response from /v1/embeddings.
type openAIEmbedResponse struct {
	Data []openAIEmbedding `json:"data"`
}

// Embed converts chunks to embeddings via the OpenAI API, batching in groups
// of defaultBatchSize.
func (e *OpenAIEmbedder) Embed(ctx context.Context, chunks []pipeline.Chunk) ([]pipeline.EmbeddedChunk, error) {
	if len(chunks) == 0 {
		return []pipeline.EmbeddedChunk{}, nil
	}

	results := make([]pipeline.EmbeddedChunk, len(chunks))

	for start := 0; start < len(chunks); start += defaultBatchSize {
		end := start + defaultBatchSize
		if end > len(chunks) {
			end = len(chunks)
		}
		batch := chunks[start:end]

		texts := extractTexts(batch)
		embeddings, err := e.embedBatch(ctx, texts)
		if err != nil {
			return nil, fmt.Errorf("openai embed batch [%d:%d]: %w", start, end, err)
		}

		// Response items may arrive in any index order.
		for _, emb := range embeddings {
			globalIdx := start + emb.Index
			results[globalIdx] = pipeline.NewEmbeddedChunk(chunks[globalIdx], emb.Embedding)
		}
	}

	return results, nil
}

func (e *OpenAIEmbedder) embedBatch(ctx context.Context, texts []string) ([]openAIEmbedding, error) {
	reqBody := openAIEmbedRequest{
		Model: e.model,
		Input: texts,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.baseURL+"/v1/embeddings", bytes.NewReader(body))
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
		return nil, fmt.Errorf("openai API returned status %d", resp.StatusCode)
	}

	var result openAIEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return result.Data, nil
}
