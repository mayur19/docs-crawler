package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mayur19/docs-crawler/internal/pipeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenAIEmbedder_Name(t *testing.T) {
	e := NewOpenAIEmbedder("http://localhost", "test-key", "text-embedding-3-small")
	assert.Equal(t, "openai", e.Name())
}

func TestOpenAIEmbedder_Dimensions(t *testing.T) {
	e := NewOpenAIEmbedder("http://localhost", "test-key", "text-embedding-3-small")
	assert.Equal(t, 0, e.Dimensions())
}

func TestOpenAIEmbedder_BasicEmbed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/embeddings", r.URL.Path)
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Contains(t, r.Header.Get("Authorization"), "Bearer ")

		var req openAIEmbedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "text-embedding-3-small", req.Model)

		resp := openAIEmbedResponse{
			Data: make([]openAIEmbedding, len(req.Input)),
		}
		for i := range req.Input {
			resp.Data[i] = openAIEmbedding{
				Embedding: []float32{0.5, 0.6, 0.7},
				Index:     i,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "test-api-key", "text-embedding-3-small")
	chunks := []pipeline.Chunk{
		pipeline.NewChunk("http://example.com", "Title", []string{"h1"}, "some content", 0, 1),
		pipeline.NewChunk("http://example.com", "Title", []string{"h2"}, "more content", 1, 2),
	}
	results, err := e.Embed(context.Background(), chunks)
	require.NoError(t, err)
	require.Len(t, results, 2)
	for _, r := range results {
		assert.Equal(t, []float32{0.5, 0.6, 0.7}, r.Vector)
	}
}

func TestOpenAIEmbedder_AuthError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "invalid auth"}}`))
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "bad-key", "text-embedding-3-small")
	_, err := e.Embed(context.Background(), makeChunks(1))
	require.Error(t, err)
}

func TestOpenAIEmbedder_Batching(t *testing.T) {
	var callCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req openAIEmbedRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		resp := openAIEmbedResponse{
			Data: make([]openAIEmbedding, len(req.Input)),
		}
		for i := range req.Input {
			resp.Data[i] = openAIEmbedding{
				Embedding: []float32{0.1},
				Index:     i,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	e := NewOpenAIEmbedder(srv.URL, "key", "text-embedding-3-small")
	results, err := e.Embed(context.Background(), makeChunks(100))
	require.NoError(t, err)
	require.Len(t, results, 100)
	assert.Equal(t, 2, callCount)
}
