package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeChunks(n int) []pipeline.Chunk {
	chunks := make([]pipeline.Chunk, n)
	for i := 0; i < n; i++ {
		chunks[i] = pipeline.NewChunk("http://example.com", "Title", []string{"h1"}, "content text here", i, n)
	}
	return chunks
}

func TestOllamaEmbedder_Name(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:11434", "nomic-embed-text")
	assert.Equal(t, "ollama", e.Name())
}

func TestOllamaEmbedder_Dimensions(t *testing.T) {
	e := NewOllamaEmbedder("http://localhost:11434", "nomic-embed-text")
	assert.Equal(t, 0, e.Dimensions())
}

func TestOllamaEmbedder_BasicEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/embed", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)

		var req ollamaEmbedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "nomic-embed-text", req.Model)

		resp := ollamaEmbedResponse{
			Embeddings: make([][]float32, len(req.Input)),
		}
		for i := range req.Input {
			resp.Embeddings[i] = []float32{0.1, 0.2, 0.3}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	chunks := makeChunks(3)
	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 3)
	for _, r := range results {
		assert.Equal(t, []float32{0.1, 0.2, 0.3}, r.Vector)
	}
}

func TestOllamaEmbedder_Batching(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		var req ollamaEmbedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		resp := ollamaEmbedResponse{
			Embeddings: make([][]float32, len(req.Input)),
		}
		for i := range req.Input {
			resp.Embeddings[i] = []float32{float32(i), 0.0, 0.0}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	// 100 chunks → 2 batches of 64 and 36
	chunks := makeChunks(100)
	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 100)
	assert.Equal(t, int32(2), callCount.Load())
}

func TestOllamaEmbedder_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	e := NewOllamaEmbedder(srv.URL, "nomic-embed-text")
	_, err := e.Embed(context.Background(), makeChunks(1))
	require.Error(t, err)
}

func TestOllamaIsAvailable_Available(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	assert.True(t, OllamaIsAvailable(srv.URL))
}

func TestOllamaIsAvailable_NotAvailable(t *testing.T) {
	// Use a port that is not listening
	assert.False(t, OllamaIsAvailable("http://127.0.0.1:19999"))
}
